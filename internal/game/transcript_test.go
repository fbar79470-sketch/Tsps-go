package game

import (
	"fmt"
	"strings"
	"testing"

	"tsps-go/internal/definition"
)

// Transcript playback tests: hand-built fixtures only, no network,
// no cache. Chains run through the real DialogueManager with
// Worker A's recording dialogueTestSession (see
// dialogue_manager_test.go), so assertions observe the entries
// the manager presents and the frames they send. Menus run
// through a capturing PresentMenu seam; the test picks options
// by invoking the captured Select callbacks, as the interface
// runtime will.

// --- Helpers -----------------------------------------------------

func transcriptNPCStep(text string) definition.DialogueStep {
	return definition.DialogueStep{NPCText: text, HasNPC: true}
}

func transcriptPlayerStep(text string) definition.DialogueStep {
	return definition.DialogueStep{PlayerText: text, HasPlayer: true}
}

func transcriptTalkEvent(p *Player) TranscriptEvent {
	return TranscriptEvent{
		Player: p, NPCID: 123,
		DefinitionName: "Bob", DefinitionID: 123,
	}
}

// driveDialogue advances the manager until the chain ends, or
// fails when max advances do not end it (a runaway conversation
// must fail the test, not hang it). It returns the advances used.
func driveDialogue(t *testing.T, m *DialogueManager, max int) int {
	t.Helper()
	used := 0
	for m.IsActive() && used < max {
		m.Advance()
		used++
	}
	if m.IsActive() {
		t.Fatalf("dialogue still active after %d advances", max)
	}
	return used
}

func sessionCalls(s *dialogueTestSession) string {
	return strings.Join(s.calls, "\n")
}

func sessionHas(s *dialogueTestSession, substr string) bool {
	return strings.Contains(sessionCalls(s), substr)
}

// currentDialogueText returns the shown entry's text, or "" when
// the current entry carries none.
func currentDialogueText(m *DialogueManager) string {
	switch entry := m.GetCurrent().(type) {
	case *NpcDialogue:
		return entry.GetText()
	case *PlayerDialogue:
		return entry.GetText()
	case *StatementDialogue:
		return entry.GetText()
	case *ItemStatementDialogue:
		return entry.GetText()
	case *DoubleItemStatementDialogue:
		return entry.GetText()
	default:
		return ""
	}
}

// --- Substitutions ----------------------------------------------
// (NpcDialogues.plugin.js:150-218.)

func TestFormatPlayerTextNamePlaceholders(t *testing.T) {
	p, _ := newDialoguePlayer()
	p.DisplayName = "Zezima"
	cases := []struct{ in, want string }{
		{"Hello, [player name]!", "Hello, Zezima!"},
		{"Hello, [playername]!", "Hello, Zezima!"},
		{"Hello, [player]!", "Hello, Zezima!"},
		{"Hello, [player vampyre name]!", "Hello, Zezima!"},
		{"Hello, <player name>!", "Hello, Zezima!"},
		// "[player.]" keeps the sentence's closing period.
		{"Hello, [player name.]", "Hello, Zezima."},
		{"[PLAYER NAME] shouts.", "Zezima shouts."},
		{"No placeholder here.", "No placeholder here."},
	}
	for _, tc := range cases {
		if got := FormatPlayerText(tc.in, p); got != tc.want {
			t.Errorf("FormatPlayerText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatPlayerTextNamePlaceholdersNoPlayer(t *testing.T) {
	// No player (or an empty display name) leaves placeholders
	// alone: the name is only substituted when non-empty.
	if got := FormatPlayerText("Hello, [player name]!", nil); got != "Hello, [player name]!" {
		t.Errorf("nil player: got %q", got)
	}
	p := NewPlayer("", Location{X: 3089, Y: 3524, Z: 0})
	if got := FormatPlayerText("Hello, [player name]!", p); got != "Hello, [player name]!" {
		t.Errorf("empty display name: got %q", got)
	}
}

func TestFormatPlayerTextGenderedAlternatives(t *testing.T) {
	male, _ := newDialoguePlayer()
	male.DisplayName = "Zezima"
	male.Appearance().SetPronouns(0)
	female, _ := newDialoguePlayer()
	female.DisplayName = "Zezima"
	female.Appearance().SetPronouns(1)
	cases := []struct {
		in, maleWant, femaleWant string
	}{
		{"Greetings, [milady/sirrah].", "Greetings, sirrah.", "Greetings, milady."},
		{"Good day, [sir/madam].", "Good day, sir.", "Good day, madam."},
		{"[Greetings, sir/Greetings, madam/Greetings]", "Greetings, sir", "Greetings, madam"},
		// No gendered form: left for quest plugins to fill.
		{"Pick [1/2/3].", "Pick [1/2/3].", "Pick [1/2/3]."},
		// Both genders in one alternative: ungendered, so it is
		// the fallback when no form matches the wanted
		// gender; a matching gendered form still wins.
		{"[sir and madam/lord]", "lord", "sir and madam"},
	}
	for _, tc := range cases {
		if got := FormatPlayerText(tc.in, male); got != tc.maleWant {
			t.Errorf("male FormatPlayerText(%q) = %q, want %q", tc.in, got, tc.maleWant)
		}
		if got := FormatPlayerText(tc.in, female); got != tc.femaleWant {
			t.Errorf("female FormatPlayerText(%q) = %q, want %q", tc.in, got, tc.femaleWant)
		}
	}
}

func TestFormatPlayerTextPlayerNameAlternative(t *testing.T) {
	// A player-name alternative in a group returns the name
	// (or "" for a player with an empty display name); with no
	// player at all the group is left alone.
	p, _ := newDialoguePlayer()
	p.DisplayName = "Zezima"
	if got := FormatPlayerText("[player name/sir]", p); got != "Zezima" {
		t.Errorf("got %q, want %q", got, "Zezima")
	}
	if got := FormatPlayerText("[player name/sir]", nil); got != "[player name/sir]" {
		t.Errorf("nil player: got %q", got)
	}
	empty := NewPlayer("", Location{X: 3089, Y: 3524, Z: 0})
	if got := FormatPlayerText("[player name/sir]", empty); got != "" {
		t.Errorf("empty display name: got %q, want empty", got)
	}
}

func TestFormatPlayerTextNoPreferencePicksFirst(t *testing.T) {
	// Pronouns 2 (no preference): no gender is wanted, so the
	// first alternative wins when every alternative is
	// gendered (pickAlternative's fallthrough).
	p, _ := newDialoguePlayer()
	p.Appearance().SetPronouns(2)
	if got := FormatPlayerText("[milady/sirrah]", p); got != "milady" {
		t.Errorf("got %q, want %q", got, "milady")
	}
}

// --- Paging ------------------------------------------------------
// (characterPages, NpcDialogues.plugin.js:570-575.)

func TestCharacterPages(t *testing.T) {
	if got := CharacterPages("Hello."); len(got) != 1 || got[0] != "Hello." {
		t.Fatalf("short text: got %q", got)
	}
	words := make([]string, 60)
	for i := range words {
		words[i] = fmt.Sprintf("word%d", i)
	}
	text := strings.Join(words, " ")
	pages := CharacterPages(text)
	if len(pages) < 2 {
		t.Fatalf("long text: got %d pages, want several", len(pages))
	}
	// Pages are lines joined with spaces, and wrapped lines
	// keep their trailing space (as TS wrapText does), so the
	// word sequence — not the exact spacing — must survive.
	if got, want := strings.Fields(strings.Join(pages, " ")), strings.Fields(text); len(got) != len(want) {
		t.Fatalf("page words = %d, want %d", len(got), len(want))
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("word %d = %q, want %q", i, got[i], want[i])
			}
		}
	}
	for i, page := range pages {
		// Four 53-character lines a page: no page exceeds
		// 4*53 characters plus the joining spaces.
		if len(page) > 4*53+3 {
			t.Errorf("page %d too long: %d chars", i, len(page))
		}
	}
}

// --- Variant/condition registries --------------------------------
// (Worker C seam: onNpcDialogueVariant / onNpcDialogueCondition.)

func TestNpcDialogueVariantRegistryFirstTruthyWins(t *testing.T) {
	r := NewNpcDialogueVariantRegistry(nil)
	r.Register(nil)
	r.Register(func(NpcDialogueVariantEvent) (NpcDialogueVariantChoice, bool) {
		return NpcDialogueVariantChoice{}, false
	})
	r.Register(func(NpcDialogueVariantEvent) (NpcDialogueVariantChoice, bool) {
		return NpcDialogueVariantChoice{Variant: "quest"}, true
	})
	r.Register(func(NpcDialogueVariantEvent) (NpcDialogueVariantChoice, bool) {
		return NpcDialogueVariantChoice{Variant: "other"}, true
	})
	choice, ok := r.Emit(NpcDialogueVariantEvent{})
	if !ok || choice.Variant != "quest" {
		t.Fatalf("got (%+v, %v), want quest/true", choice, ok)
	}
}

func TestNpcDialogueVariantRegistryPanicIsolated(t *testing.T) {
	var warnings []string
	r := NewNpcDialogueVariantRegistry(func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	})
	r.Register(func(NpcDialogueVariantEvent) (NpcDialogueVariantChoice, bool) {
		panic("boom")
	})
	r.Register(func(NpcDialogueVariantEvent) (NpcDialogueVariantChoice, bool) {
		return NpcDialogueVariantChoice{Variant: "fallback"}, true
	})
	choice, ok := r.Emit(NpcDialogueVariantEvent{})
	if !ok || choice.Variant != "fallback" {
		t.Fatalf("got (%+v, %v), want fallback/true", choice, ok)
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
}

func TestNpcDialogueVariantRegistryNilSafe(t *testing.T) {
	var r *NpcDialogueVariantRegistry
	if _, ok := r.Emit(NpcDialogueVariantEvent{}); ok {
		t.Error("nil registry Emit answered")
	}
	r.Register(func(NpcDialogueVariantEvent) (NpcDialogueVariantChoice, bool) {
		return NpcDialogueVariantChoice{}, true
	})
}

func TestNpcDialogueConditionRegistryFirstAnsweredWins(t *testing.T) {
	r := NewNpcDialogueConditionRegistry(nil)
	r.Register(func(NpcDialogueConditionQuery) (bool, bool) { return false, false })
	r.Register(func(NpcDialogueConditionQuery) (bool, bool) { return false, true })
	r.Register(func(NpcDialogueConditionQuery) (bool, bool) { return true, true })
	answer, answered := r.Emit(NpcDialogueConditionQuery{})
	if !answered || answer {
		t.Fatalf("got (%v, %v), want (false, true)", answer, answered)
	}
}

func TestNpcDialogueConditionRegistryNilSafe(t *testing.T) {
	var r *NpcDialogueConditionRegistry
	if _, answered := r.Emit(NpcDialogueConditionQuery{}); answered {
		t.Error("nil registry Emit answered")
	}
}

// --- Built-in condition fallbacks --------------------------------
// (NpcDialogues.plugin.js:454-474.)

func TestCombatLevelCondition(t *testing.T) {
	p, _ := newDialoguePlayer()
	// A fresh player is combat level 3.
	if level := p.Skills().CombatLevel(); level != 3 {
		t.Fatalf("fresh player combat level = %d, want 3", level)
	}
	answer, answered := combatLevelCondition(p, "If the player's combat level is lower than 50:")
	if !answered || !answer {
		t.Fatalf("got (%v, %v), want (true, true)", answer, answered)
	}
	answer, answered = combatLevelCondition(p, "If the player's combat level is lower than 3:")
	if !answered || answer {
		t.Fatalf("got (%v, %v), want (false, true)", answer, answered)
	}
	if _, answered := combatLevelCondition(p, "If the player is wearing armour:"); answered {
		t.Error("unrelated wording answered")
	}
	if _, answered := combatLevelCondition(nil, "If the player's combat level is lower than 50:"); answered {
		t.Error("nil player answered")
	}
}

func TestSkillLevelCondition(t *testing.T) {
	p, _ := newDialoguePlayer()
	answer, answered := skillLevelCondition(p, "If the player has at least 1 Attack:")
	if !answered || !answer {
		t.Fatalf("got (%v, %v), want (true, true)", answer, answered)
	}
	answer, answered = skillLevelCondition(p, "If the player has at least 99 Attack:")
	if !answered || answer {
		t.Fatalf("got (%v, %v), want (false, true)", answer, answered)
	}
	answer, answered = skillLevelCondition(p, "If the player has under 50 Attack:")
	if !answered || !answer {
		t.Fatalf("got (%v, %v), want (true, true)", answer, answered)
	}
	answer, answered = skillLevelCondition(p, "If the player has less than 1 Attack:")
	if !answered || answer {
		t.Fatalf("got (%v, %v), want (false, true)", answer, answered)
	}
	if _, answered := skillLevelCondition(p, "If the player has at least 1 Nonsense:"); answered {
		t.Error("unknown skill answered")
	}
	if _, answered := skillLevelCondition(p, "If the player has 50 Attack:"); answered {
		t.Error("unrelated wording answered")
	}
}

// --- Resolution --------------------------------------------------
// (resolveTranscript, NpcDialogues.plugin.js:1255-1319.)

func transcriptTestData() *definition.DialogueData {
	return &definition.DialogueData{
		Records: map[string]*definition.TranscriptRecord{
			"Bob": {
				Name:     "Bob",
				Steps:    []definition.DialogueStep{transcriptNPCStep("Bob's flat line.")},
				HasSteps: true,
			},
			"Hops (Biohazard)": {
				Name:     "Hops (Biohazard)",
				Steps:    []definition.DialogueStep{transcriptNPCStep("Hops line.")},
				HasSteps: true,
			},
			"Questy": {
				Name:        "Questy",
				HasVariants: true,
				Variants: map[string][]definition.DialogueStep{
					"standard-dialogue": {transcriptNPCStep("Standard Questy.")},
					"quest":             {transcriptNPCStep("Quest Questy.")},
				},
				VariantOrder: []string{"standard-dialogue", "quest"},
			},
			"Empty": {
				Name:         "Empty",
				HasVariants:  true,
				Variants:     map[string][]definition.DialogueStep{},
				VariantOrder: []string{},
			},
		},
		RecordOrder:     []string{"Bob", "Hops (Biohazard)", "Questy", "Empty"},
		Aliases:         map[string]string{"Hops": "Hops (Biohazard)"},
		Index:           map[int][]definition.DialogueIndexEntry{},
		SpeakerIDByName: map[string]int{},
		Predecessors:    map[int]int{},
	}
}

func TestResolveTranscriptIndexLookup(t *testing.T) {
	p, _ := newDialoguePlayer()
	data := transcriptTestData()
	data.Index[123] = []definition.DialogueIndexEntry{{Page: "Transcript:Bob"}}
	pb := NewDialoguePlayback(TranscriptDeps{Data: data})
	resolved := pb.ResolveTranscript(transcriptTalkEvent(p))
	if resolved == nil || !resolved.StepsPresent {
		t.Fatal("index lookup resolved nothing")
	}
	if len(resolved.Steps) != 1 || resolved.Steps[0].NPCText != "Bob's flat line." {
		t.Fatalf("steps = %+v", resolved.Steps)
	}
	if resolved.Page != "Bob" {
		t.Errorf("Page = %q, want Bob", resolved.Page)
	}
}

func TestResolveTranscriptPredecessorFallback(t *testing.T) {
	// A successor NPC (16821) plays the retired NPC's (3216)
	// transcript until it has its own: PREDECESSOR maps
	// successor -> retired.
	p, _ := newDialoguePlayer()
	data := transcriptTestData()
	data.Index[3216] = []definition.DialogueIndexEntry{{Page: "Transcript:Bob"}}
	data.Predecessors[16821] = 3216
	pb := NewDialoguePlayback(TranscriptDeps{Data: data})
	event := transcriptTalkEvent(p)
	event.NPCID = 16821
	resolved := pb.ResolveTranscript(event)
	if resolved == nil || len(resolved.Steps) != 1 {
		t.Fatalf("predecessor fallback resolved %+v", resolved)
	}
}

func TestResolveTranscriptFirstIDWinsEvenWhenEmpty(t *testing.T) {
	// The first id present in the index wins, even with empty
	// rows: an empty row list is a stored answer, not a miss.
	p, _ := newDialoguePlayer()
	data := transcriptTestData()
	data.Index[123] = []definition.DialogueIndexEntry{}
	data.Index[3216] = []definition.DialogueIndexEntry{{Page: "Transcript:Bob"}}
	data.Predecessors[123] = 3216
	pb := NewDialoguePlayback(TranscriptDeps{Data: data})
	resolved := pb.ResolveTranscript(transcriptTalkEvent(p))
	// No pages from the empty rows; the name fallback still
	// finds "Bob" by DefinitionName.
	if resolved == nil || resolved.Page != "Bob" {
		t.Fatalf("resolved = %+v, want the name-fallback Bob page", resolved)
	}
}

func TestResolveTranscriptNameFallback(t *testing.T) {
	p, _ := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{Data: transcriptTestData()})
	event := transcriptTalkEvent(p)
	event.DefinitionName = "Hops"
	resolved := pb.ResolveTranscript(event)
	if resolved == nil || len(resolved.Steps) != 1 || resolved.Steps[0].NPCText != "Hops line." {
		t.Fatalf("alias fallback resolved %+v", resolved)
	}
}

func TestResolveTranscriptExplicitUnindexedPageBinding(t *testing.T) {
	p, _ := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{Data: transcriptTestData()})
	event := transcriptTalkEvent(p)
	event.Explicit = true
	event.VariantPage = "Questy"
	event.DefinitionName = "Unindexed"
	resolved := pb.ResolveTranscript(event)
	if resolved == nil || resolved.Page != "Questy" {
		t.Fatalf("explicit binding resolved %+v", resolved)
	}
	// Without Explicit, the same request does not bind.
	event.Explicit = false
	if resolved := pb.ResolveTranscript(event); resolved != nil {
		t.Fatalf("non-explicit binding resolved %+v", resolved)
	}
}

func TestResolveTranscriptVariantHookOverride(t *testing.T) {
	p, _ := newDialoguePlayer()
	data := transcriptTestData()
	data.Index[123] = []definition.DialogueIndexEntry{{
		Page:     "Transcript:Questy",
		Variants: []string{"standard-dialogue", "quest"},
	}}
	registry := NewNpcDialogueVariantRegistry(nil)
	registry.Register(func(NpcDialogueVariantEvent) (NpcDialogueVariantChoice, bool) {
		return NpcDialogueVariantChoice{Variant: "quest"}, true
	})
	pb := NewDialoguePlayback(TranscriptDeps{Data: data, VariantHandlers: registry})
	resolved := pb.ResolveTranscript(transcriptTalkEvent(p))
	if resolved == nil || resolved.Variant != "quest" {
		t.Fatalf("resolved = %+v, want the quest variant", resolved)
	}
	if resolved.Steps[0].NPCText != "Quest Questy." {
		t.Fatalf("steps = %+v", resolved.Steps)
	}
}

func TestResolveTranscriptDefaultVariantScan(t *testing.T) {
	// No hook: the default scan finds the first standard*
	// variant.
	p, _ := newDialoguePlayer()
	data := transcriptTestData()
	data.Index[123] = []definition.DialogueIndexEntry{{
		Page:     "Transcript:Questy",
		Variants: []string{"standard-dialogue", "quest"},
	}}
	pb := NewDialoguePlayback(TranscriptDeps{Data: data})
	resolved := pb.ResolveTranscript(transcriptTalkEvent(p))
	if resolved == nil || len(resolved.Steps) != 1 || resolved.Steps[0].NPCText != "Standard Questy." {
		t.Fatalf("resolved = %+v", resolved)
	}
}

func TestResolveTranscriptGapOnlyNamesThePage(t *testing.T) {
	p, _ := newDialoguePlayer()
	data := transcriptTestData()
	data.Index[123] = []definition.DialogueIndexEntry{{Page: "Transcript:Empty"}}
	pb := NewDialoguePlayback(TranscriptDeps{Data: data})
	event := transcriptTalkEvent(p)
	event.DefinitionName = "Empty"
	resolved := pb.ResolveTranscript(event)
	if resolved == nil {
		t.Fatal("expected a gap-only result, got nil")
	}
	if resolved.StepsPresent || resolved.Page != "Empty" {
		t.Fatalf("resolved = %+v, want gap-only naming Empty", resolved)
	}
}

func TestResolveTranscriptNothingReaches(t *testing.T) {
	p, _ := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{Data: transcriptTestData()})
	event := transcriptTalkEvent(p)
	event.DefinitionName = "Nobody"
	if resolved := pb.ResolveTranscript(event); resolved != nil {
		t.Fatalf("resolved = %+v, want nil", resolved)
	}
	if resolved := pb.ResolveTranscript(TranscriptEvent{}); resolved != nil {
		t.Fatalf("no player resolved %+v, want nil", resolved)
	}
}

// --- Talk-to -----------------------------------------------------
// (NpcDialogues.plugin.js:1368-1379.)

func TestStartTalkToSpecialNPCFallsThrough(t *testing.T) {
	p, _ := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{Data: transcriptTestData()})
	for _, name := range []string{"Skully", "Estate agent", "Estate Agent", "Alwyn"} {
		event := transcriptTalkEvent(p)
		event.DefinitionName = name
		if pb.StartTalkTo(event) {
			t.Errorf("%s: StartTalkTo handled, want fall-through", name)
		}
		if p.GetDialogueManager().IsActive() {
			t.Errorf("%s: chain started", name)
		}
	}
}

func TestStartTalkToPlaceholderAndGapWarningOnce(t *testing.T) {
	p, s := newDialoguePlayer()
	var warnings []string
	pb := NewDialoguePlayback(TranscriptDeps{
		Data: transcriptTestData(),
		Logf: func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) },
	})
	event := transcriptTalkEvent(p)
	event.DefinitionName = "Nobody"
	if !pb.StartTalkTo(event) {
		t.Fatal("StartTalkTo returned false")
	}
	if got := currentDialogueText(p.GetDialogueManager()); got != PlaceholderLine {
		t.Fatalf("current text = %q, want placeholder", got)
	}
	if !sessionHas(s, PlaceholderLine) {
		t.Error("placeholder line not sent")
	}
	driveDialogue(t, p.GetDialogueManager(), 10)
	// A second Talk-to warns no more, but still plays the line.
	pb.StartTalkTo(event)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly 1", warnings)
	}
	if !strings.Contains(warnings[0], "no transcript reaches it") {
		t.Errorf("warning = %q, want the no-transcript reason", warnings[0])
	}
}

func TestStartTalkToPlaysResolvedTranscript(t *testing.T) {
	p, _ := newDialoguePlayer()
	data := transcriptTestData()
	data.Index[123] = []definition.DialogueIndexEntry{{Page: "Transcript:Bob"}}
	pb := NewDialoguePlayback(TranscriptDeps{Data: data})
	if !pb.StartTalkTo(transcriptTalkEvent(p)) {
		t.Fatal("StartTalkTo returned false")
	}
	if got := currentDialogueText(p.GetDialogueManager()); got != "Bob's flat line." {
		t.Fatalf("current text = %q", got)
	}
}

// --- Page indexes ------------------------------------------------
// (NpcDialogues.plugin.js:322-417.)

func TestCollectPageOptionsRealBodyWins(t *testing.T) {
	record := &definition.TranscriptRecord{
		Steps: []definition.DialogueStep{{
			Type: "choice",
			Options: []definition.DialogueOptionStep{
				{Text: "Ask about this", Steps: []definition.DialogueStep{{Type: "jump", Reference: "below"}}},
				{Text: "Ask about this!", Steps: []definition.DialogueStep{transcriptNPCStep("The answer.")}},
			},
		}},
	}
	index := CollectPageOptions(record)
	// Both spellings normalize to the same key (normText
	// keeps only alphanumerics); the real body wins over the
	// jump-only one.
	steps := index.ByText["askaboutthis"]
	if len(steps) != 1 || steps[0].NPCText != "The answer." {
		t.Fatalf("ByText = %+v", index.ByText)
	}
	if len(index.List) != 2 {
		t.Fatalf("List has %d entries, want 2", len(index.List))
	}
}

func TestCollectPageLinesFirstWins(t *testing.T) {
	line := transcriptNPCStep("First line.")
	record := &definition.TranscriptRecord{
		Steps: []definition.DialogueStep{
			line, transcriptNPCStep("After first."),
			line, transcriptNPCStep("After second."),
		},
	}
	lines := CollectPageLines(record)
	// First occurrence wins: the steps from the first line
	// onward, line included.
	tail := lines["firstline"]
	if len(tail) != 4 || tail[1].NPCText != "After first." {
		t.Fatalf("tail = %+v", tail)
	}
}

// --- StartPage / StartNamedVariant -------------------------------
// (NpcDialogues.plugin.js:1333-1366.)

func TestStartPageExplicitOnly(t *testing.T) {
	p, _ := newDialoguePlayer()
	data := transcriptTestData()
	data.Records["ItemPage"] = &definition.TranscriptRecord{
		Name: "ItemPage", Activation: "explicit",
		HasVariants: true,
		Variants: map[string][]definition.DialogueStep{
			"standard": {transcriptNPCStep("Item speaks.")},
		},
		VariantOrder: []string{"standard"},
	}
	pb := NewDialoguePlayback(TranscriptDeps{Data: data})
	req := &StartPageRequest{Player: p, Page: "ItemPage", Variant: "standard"}
	pb.StartPage(req)
	if !req.Handled {
		t.Fatal("explicit page not handled")
	}
	if got := currentDialogueText(p.GetDialogueManager()); got != "Item speaks." {
		t.Fatalf("current text = %q", got)
	}
	if req.Cancel == nil || !req.Cancel() {
		t.Error("explicit playback cancel should own the chain")
	}

	// A non-explicit record never plays via StartPage.
	req = &StartPageRequest{Player: p, Page: "Bob", Variant: "standard-dialogue"}
	pb.StartPage(req)
	if req.Handled {
		t.Error("non-explicit page handled")
	}
	// A missing variant never plays.
	req = &StartPageRequest{Player: p, Page: "ItemPage", Variant: "nope"}
	pb.StartPage(req)
	if req.Handled {
		t.Error("missing variant handled")
	}
}

func TestStartNamedVariantExactMismatch(t *testing.T) {
	p, _ := newDialoguePlayer()
	data := transcriptTestData()
	data.Index[123] = []definition.DialogueIndexEntry{{
		Page:     "Transcript:Questy",
		Variants: []string{"standard-dialogue", "quest"},
	}}
	pb := NewDialoguePlayback(TranscriptDeps{Data: data})
	req := &StartNamedRequest{
		Player: p, NPCID: 123, HasNPCID: true,
		DefinitionName: "Questy", DefinitionID: 123,
		Variant: "quest", HasVariant: true, Exact: true,
	}
	pb.StartNamedVariant(req)
	if !req.Handled {
		t.Fatal("exact variant not handled")
	}
	if got := currentDialogueText(p.GetDialogueManager()); got != "Quest Questy." {
		t.Fatalf("current text = %q", got)
	}

	req = &StartNamedRequest{
		Player: p, NPCID: 123, HasNPCID: true,
		DefinitionName: "Questy", DefinitionID: 123,
		Variant: "missing", HasVariant: true, Exact: true,
	}
	pb.StartNamedVariant(req)
	if req.Handled {
		t.Error("exact mismatch handled")
	}
}

// --- Speech lines ------------------------------------------------
// (NpcDialogues.plugin.js:964-1041.)

func TestStartDialogueSpeechLines(t *testing.T) {
	p, s := newDialoguePlayer()
	p.GetDialogueManager().SetNpcNameResolver(func(id int) string {
		if id == 123 {
			return "Bob_The_Builder"
		}
		return "null"
	})
	pb := NewDialoguePlayback(TranscriptDeps{})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		transcriptNPCStep("Hello there."),
		transcriptPlayerStep("Hi!"),
	}, nil, nil)
	m := p.GetDialogueManager()
	if got := currentDialogueText(m); got != "Hello there." {
		t.Fatalf("first entry = %q", got)
	}
	if _, ok := m.GetCurrent().(*NpcDialogue); !ok {
		t.Fatalf("first entry is %T, want *NpcDialogue", m.GetCurrent())
	}
	if !sessionHas(s, "Bob The Builder") {
		t.Errorf("speaker name not resolved through the definition:\n%s", sessionCalls(s))
	}
	if !sessionHas(s, fmt.Sprintf("chatbox(%d,false)", NpcDialogueGroupLeft)) {
		t.Errorf("NPC line did not mount group 231:\n%s", sessionCalls(s))
	}
	m.Advance()
	if got := currentDialogueText(m); got != "Hi!" {
		t.Fatalf("second entry = %q", got)
	}
	if _, ok := m.GetCurrent().(*PlayerDialogue); !ok {
		t.Fatalf("second entry is %T, want *PlayerDialogue", m.GetCurrent())
	}
	driveDialogue(t, m, 10)
	if !sessionHas(s, "removal") {
		t.Error("chain end did not remove the interface")
	}
}

func TestStartDialogueTypedLineSpeakerAndSide(t *testing.T) {
	p, s := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{})
	// A typed line from a named speaker on the right mounts
	// group 217 with that speaker's name; the speaker id
	// resolves through the context's SpeakerIDByName.
	ctx := &TranscriptContext{SpeakerIDByName: map[string]int{"Alice": 456}}
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		Type: "line", Text: "Over here.", HasText: true,
		Speaker: "Alice", HasSpeaker: true, Side: "right",
	}}, nil, ctx)
	m := p.GetDialogueManager()
	entry, ok := m.GetCurrent().(*NpcDialogue)
	if !ok {
		t.Fatalf("current is %T, want *NpcDialogue", m.GetCurrent())
	}
	if entry.GetNpcID() != 456 {
		t.Errorf("speaker id = %d, want 456", entry.GetNpcID())
	}
	if !sessionHas(s, fmt.Sprintf("chatbox(%d,false)", NpcDialogueGroupRight)) {
		t.Errorf("right-side line did not mount group 217:\n%s", sessionCalls(s))
	}
	if !sessionHas(s, "Alice") {
		t.Errorf("speaker name not shown:\n%s", sessionCalls(s))
	}
}

func TestStartDialogueExpressionRule(t *testing.T) {
	p, s := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{})
	// 588 (HAPPY) is applied; -1 and non-integers fall back to
	// CALM (589).
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		NPCText: "Happy.", HasNPC: true, Expression: 588, HasExpression: true,
	}}, nil, nil)
	headUID := (NpcDialogueGroupLeft << 16) | 2
	if !sessionHas(s, fmt.Sprintf("anim(%d,588)", headUID)) {
		t.Errorf("expression 588 not applied:\n%s", sessionCalls(s))
	}
	driveDialogue(t, p.GetDialogueManager(), 10)

	p2, s2 := newDialoguePlayer()
	pb.StartDialogue(transcriptTalkEvent(p2), []definition.DialogueStep{{
		NPCText: "Calm.", HasNPC: true, Expression: -1, HasExpression: true,
	}}, nil, nil)
	if !sessionHas(s2, fmt.Sprintf("anim(%d,589)", headUID)) {
		t.Errorf("invalid expression did not fall back to CALM:\n%s", sessionCalls(s2))
	}
}

func TestStartDialogueLineEventSkipAndText(t *testing.T) {
	p, s := newDialoguePlayer()
	var events []string
	pb := NewDialoguePlayback(TranscriptDeps{
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueLineEvent); ok {
				events = append(events, ev.Text)
				switch ev.Text {
				case "Skip me.":
					ev.Skip = true
				case "Change me.":
					ev.Text = "Changed."
				}
			}
		},
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		transcriptNPCStep("Skip me."),
		transcriptNPCStep("Change me."),
		transcriptNPCStep("Keep me."),
	}, nil, nil)
	if len(events) != 3 {
		t.Fatalf("line events = %v", events)
	}
	// The skipped line never shows; the mutated text does.
	if sessionHas(s, "Skip me.") {
		t.Error("skipped line was shown")
	}
	if got := currentDialogueText(p.GetDialogueManager()); got != "Changed." {
		t.Fatalf("first shown = %q, want Changed.", got)
	}
	driveDialogue(t, p.GetDialogueManager(), 10)
	if !sessionHas(s, "Keep me.") {
		t.Error("third line never shown")
	}
}

func TestStartDialogueLineEventSkipPlaysNestedSteps(t *testing.T) {
	p, _ := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueLineEvent); ok && ev.Text == "Outer." {
				ev.Skip = true
			}
		},
	})
	outer := transcriptNPCStep("Outer.")
	outer.Steps = []definition.DialogueStep{transcriptNPCStep("Nested.")}
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{outer}, nil, nil)
	if got := currentDialogueText(p.GetDialogueManager()); got != "Nested." {
		t.Fatalf("current = %q, want the nested line", got)
	}
}

func TestStartDialogueLineEventBeforeAfter(t *testing.T) {
	p, _ := newDialoguePlayer()
	var order []string
	pb := NewDialoguePlayback(TranscriptDeps{
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueLineEvent); ok {
				ev.Before = func() { order = append(order, "before") }
				ev.After = func() { order = append(order, "after") }
			}
		},
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		transcriptNPCStep("Line."),
	}, nil, nil)
	m := p.GetDialogueManager()
	// The before action runs first, then the line shows.
	driveDialogue(t, m, 10)
	if len(order) != 2 || order[0] != "before" || order[1] != "after" {
		t.Fatalf("order = %v, want [before after]", order)
	}
}

func TestStartDialogueSubstitutionInLines(t *testing.T) {
	p, _ := newDialoguePlayer()
	p.DisplayName = "Zezima"
	p.Appearance().SetPronouns(1)
	pb := NewDialoguePlayback(TranscriptDeps{})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		transcriptNPCStep("Hello, [player name], [milady/sirrah]."),
	}, nil, nil)
	if got := currentDialogueText(p.GetDialogueManager()); got != "Hello, Zezima, milady." {
		t.Fatalf("current = %q", got)
	}
}

// --- Conditions --------------------------------------------------
// (NpcDialogues.plugin.js:420-441, 632-636.)

func transcriptConditionStep(text string, body ...definition.DialogueStep) definition.DialogueStep {
	return definition.DialogueStep{
		Type: "condition", Text: text, HasText: true, Steps: body,
	}
}

func TestStartDialogueConditionHookAnswers(t *testing.T) {
	conditions := NewNpcDialogueConditionRegistry(nil)
	conditions.Register(func(q NpcDialogueConditionQuery) (bool, bool) {
		if q.Text == "wearing armour" {
			return false, true
		}
		return false, false
	})
	p, s := newDialoguePlayer()
	var conditionEvents []string
	pb := NewDialoguePlayback(TranscriptDeps{
		ConditionHandlers: conditions,
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueConditionEvent); ok {
				conditionEvents = append(conditionEvents, ev.Text)
			}
		},
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		transcriptConditionStep("wearing armour", transcriptNPCStep("Armoured.")),
		transcriptConditionStep("anything else", transcriptNPCStep("Fallback.")),
		transcriptNPCStep("After."),
	}, nil, nil)
	driveDialogue(t, p.GetDialogueManager(), 20)
	if sessionHas(s, "Armoured.") {
		t.Error("false-guarded branch played")
	}
	if !sessionHas(s, "Fallback.") {
		t.Error("unanswered condition did not fall back to its branch")
	}
	if !sessionHas(s, "After.") {
		t.Error("continuation did not play")
	}
	if len(conditionEvents) != 1 || conditionEvents[0] != "anything else" {
		t.Fatalf("condition events = %v", conditionEvents)
	}
}

func TestStartDialogueConditionBuiltinFallbacks(t *testing.T) {
	p, s := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		transcriptConditionStep("If the player's combat level is lower than 50:", transcriptNPCStep("Low level.")),
		transcriptConditionStep("If the player has at least 99 Attack:", transcriptNPCStep("Maxed.")),
		transcriptNPCStep("After."),
	}, nil, nil)
	driveDialogue(t, p.GetDialogueManager(), 20)
	if !sessionHas(s, "Low level.") {
		t.Error("combat-level fallback did not play its branch")
	}
	if sessionHas(s, "Maxed.") {
		t.Error("skill-level fallback played a false branch")
	}
}

// --- Menus -------------------------------------------------------
// (presentMenu, NpcDialogues.plugin.js:776-851.)

type transcriptMenuCapture struct {
	prompts []string
	options [][]MenuOption
}

func (c *transcriptMenuCapture) present(player *Player, prompt string, options []MenuOption) bool {
	c.prompts = append(c.prompts, prompt)
	c.options = append(c.options, options)
	return true
}

func (c *transcriptMenuCapture) labels(i int) []string {
	var labels []string
	for _, option := range c.options[i] {
		labels = append(labels, option.Label)
	}
	return labels
}

func TestStartDialogueChoiceMenu(t *testing.T) {
	p, s := newDialoguePlayer()
	capture := &transcriptMenuCapture{}
	var choices []string
	pb := NewDialoguePlayback(TranscriptDeps{
		PresentMenu: capture.present,
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueChoiceEvent); ok {
				choices = append(choices, ev.Option)
			}
		},
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		Type:   "choice",
		Prompt: "Pick one",
		Options: []definition.DialogueOptionStep{
			{Text: "First", Steps: []definition.DialogueStep{transcriptNPCStep("One.")}},
			{Text: "Second", Steps: []definition.DialogueStep{transcriptNPCStep("Two.")}},
		},
	}}, nil, nil)
	if len(capture.options) != 1 {
		t.Fatalf("menus shown = %d, want 1", len(capture.options))
	}
	if capture.prompts[0] != "Pick one" {
		t.Errorf("prompt = %q", capture.prompts[0])
	}
	if got := capture.labels(0); len(got) != 2 || got[0] != "First" || got[1] != "Second" {
		t.Fatalf("labels = %v", got)
	}
	// The manager resets while the menu is up; picking drives
	// the branch.
	if p.GetDialogueManager().IsActive() {
		t.Error("manager still active while the menu is up")
	}
	capture.options[0][1].Select()
	if len(choices) != 1 || choices[0] != "Second" {
		t.Fatalf("choices = %v", choices)
	}
	if got := currentDialogueText(p.GetDialogueManager()); got != "Two." {
		t.Fatalf("current = %q, want Two.", got)
	}
	driveDialogue(t, p.GetDialogueManager(), 10)
	if sessionHas(s, "One.") {
		t.Error("unpicked branch played")
	}
}

func TestStartDialogueMenuPagination(t *testing.T) {
	p, _ := newDialoguePlayer()
	capture := &transcriptMenuCapture{}
	pb := NewDialoguePlayback(TranscriptDeps{PresentMenu: capture.present})
	var options []definition.DialogueOptionStep
	for i := 1; i <= 7; i++ {
		options = append(options, definition.DialogueOptionStep{
			Text:  fmt.Sprintf("Option %d", i),
			Steps: []definition.DialogueStep{transcriptNPCStep(fmt.Sprintf("Picked %d.", i))},
		})
	}
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		Type: "choice", Options: options,
	}}, nil, nil)
	if got := capture.labels(0); len(got) != 5 || got[4] != "More..." {
		t.Fatalf("first page = %v, want 4 options + More...", got)
	}
	capture.options[0][4].Select()
	if len(capture.options) != 2 {
		t.Fatalf("menus shown = %d, want 2", len(capture.options))
	}
	if got := capture.labels(1); len(got) != 3 || got[0] != "Option 5" {
		t.Fatalf("second page = %v", got)
	}
	capture.options[1][2].Select()
	if got := currentDialogueText(p.GetDialogueManager()); got != "Picked 7." {
		t.Fatalf("current = %q", got)
	}
}

func TestStartDialogueMenuSingleOptionGoodbye(t *testing.T) {
	p, _ := newDialoguePlayer()
	capture := &transcriptMenuCapture{}
	closed := 0
	pb := NewDialoguePlayback(TranscriptDeps{
		PresentMenu:  capture.present,
		CloseChatbox: func(*Player) { closed++ },
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		Type:    "choice",
		Options: []definition.DialogueOptionStep{{Text: "Only", Steps: []definition.DialogueStep{transcriptNPCStep("Only.")}}},
	}}, nil, nil)
	if got := capture.labels(0); len(got) != 2 || got[1] != "Goodbye." {
		t.Fatalf("labels = %v, want [Only Goodbye.]", got)
	}
	capture.options[0][1].Select()
	if closed != 1 {
		t.Fatalf("closes = %d, want 1", closed)
	}
}

func TestStartDialogueMenuWarningOnce(t *testing.T) {
	p, s := newDialoguePlayer()
	capture := &transcriptMenuCapture{}
	menuEvents := 0
	pb := NewDialoguePlayback(TranscriptDeps{
		PresentMenu: capture.present,
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueMenuEvent); ok {
				menuEvents++
				ev.Warning = "Careful!"
			}
		},
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		Type:    "choice",
		Options: []definition.DialogueOptionStep{{Text: "Go", Steps: []definition.DialogueStep{transcriptNPCStep("Went.")}}},
	}}, nil, nil)
	// The warning shows as a statement first; the menu has not
	// been presented yet.
	if len(capture.options) != 0 {
		t.Fatal("menu presented before the warning")
	}
	if got := currentDialogueText(p.GetDialogueManager()); got != "Careful!" {
		t.Fatalf("current = %q, want the warning", got)
	}
	driveDialogue(t, p.GetDialogueManager(), 10)
	if menuEvents != 2 {
		t.Fatalf("menu events = %d, want 2 (warning + re-present)", menuEvents)
	}
	if len(capture.options) != 1 {
		t.Fatalf("menus shown = %d, want 1", len(capture.options))
	}
	// The warning showed once, as a statement box.
	if !sessionHas(s, "Careful!") {
		t.Error("warning not shown")
	}
}

func TestStartDialogueMenuHandled(t *testing.T) {
	p, _ := newDialoguePlayer()
	capture := &transcriptMenuCapture{}
	pb := NewDialoguePlayback(TranscriptDeps{
		PresentMenu: capture.present,
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueMenuEvent); ok {
				ev.Handled = true
			}
		},
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		Type:    "choice",
		Options: []definition.DialogueOptionStep{{Text: "Go", Steps: []definition.DialogueStep{transcriptNPCStep("Went.")}}},
	}}, nil, nil)
	if len(capture.options) != 0 {
		t.Fatal("handled menu was presented")
	}
	// Nothing more happens; advancing off the tail action ends
	// the chain.
	driveDialogue(t, p.GetDialogueManager(), 5)
}

func TestStartDialogueMenuNoPresenterUnavailables(t *testing.T) {
	p, s := newDialoguePlayer()
	closed := 0
	pb := NewDialoguePlayback(TranscriptDeps{
		CloseChatbox: func(*Player) { closed++ },
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		Type:    "choice",
		Options: []definition.DialogueOptionStep{{Text: "Go", Steps: []definition.DialogueStep{transcriptNPCStep("Went.")}}},
	}}, nil, nil)
	// The menu counts as played before the presenter is tried,
	// so the fallback closes without the "isn't available"
	// message (unavailable() only messages when nothing
	// played).
	if closed != 1 {
		t.Fatalf("closes = %d, want 1", closed)
	}
	if len(s.messages) != 0 {
		t.Fatalf("messages = %v, want none", s.messages)
	}
}

func TestStartDialogueMenuGuardedOptionHidden(t *testing.T) {
	conditions := NewNpcDialogueConditionRegistry(nil)
	conditions.Register(func(q NpcDialogueConditionQuery) (bool, bool) {
		if q.Text == "secret" {
			return false, true
		}
		return false, false
	})
	p, _ := newDialoguePlayer()
	capture := &transcriptMenuCapture{}
	pb := NewDialoguePlayback(TranscriptDeps{
		PresentMenu: capture.present, ConditionHandlers: conditions,
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		Type: "choice",
		Options: []definition.DialogueOptionStep{
			{Text: "Hidden", Condition: "secret", Steps: []definition.DialogueStep{transcriptNPCStep("Secret.")}},
			{Text: "Shown", Steps: []definition.DialogueStep{transcriptNPCStep("Shown.")}},
		},
	}}, nil, nil)
	// The false-guarded option is hidden; one visible option
	// gains the Goodbye pair.
	if got := capture.labels(0); len(got) != 2 || got[0] != "Shown" {
		t.Fatalf("labels = %v", got)
	}
}

func TestStartDialogueChoiceEndStops(t *testing.T) {
	p, _ := newDialoguePlayer()
	capture := &transcriptMenuCapture{}
	pb := NewDialoguePlayback(TranscriptDeps{
		PresentMenu: capture.present,
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueChoiceEvent); ok {
				ev.End = true
			}
		},
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		Type:    "choice",
		Options: []definition.DialogueOptionStep{{Text: "Go", Steps: []definition.DialogueStep{transcriptNPCStep("Went.")}}},
	}}, nil, nil)
	capture.options[0][0].Select()
	if p.GetDialogueManager().IsActive() {
		t.Error("End choice left a chain running")
	}
}

// --- Random ------------------------------------------------------
// (NpcDialogues.plugin.js:1131-1148.)

func TestStartDialogueRandomStep(t *testing.T) {
	p, s := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{Random: func(n int) int { return 1 }})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		Type: "random",
		Options: []definition.DialogueOptionStep{
			{Text: "a", Steps: []definition.DialogueStep{transcriptNPCStep("A.")}},
			{Text: "b", Steps: []definition.DialogueStep{transcriptNPCStep("B.")}},
		},
	}}, nil, nil)
	driveDialogue(t, p.GetDialogueManager(), 10)
	if !sessionHas(s, "B.") || sessionHas(s, "A.") {
		t.Errorf("random pick wrong:\n%s", sessionCalls(s))
	}
}

func TestStartDialogueRandomGuardedAndEmpty(t *testing.T) {
	conditions := NewNpcDialogueConditionRegistry(nil)
	conditions.Register(func(NpcDialogueConditionQuery) (bool, bool) { return false, true })
	p, s := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{
		ConditionHandlers: conditions,
		Random:            func(n int) int { return 0 },
	})
	// Every alternative is guarded out: the random continues
	// with whatever follows.
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		{
			Type: "random",
			Options: []definition.DialogueOptionStep{
				{Text: "a", Condition: "nope", Steps: []definition.DialogueStep{transcriptNPCStep("A.")}},
			},
		},
		transcriptNPCStep("After."),
	}, nil, nil)
	driveDialogue(t, p.GetDialogueManager(), 10)
	if !sessionHas(s, "After.") || sessionHas(s, "A.") {
		t.Errorf("empty random wrong:\n%s", sessionCalls(s))
	}
}

func TestStartDialogueRandomHookedAlternativeUnavailables(t *testing.T) {
	p, s := newDialoguePlayer()
	closed := 0
	pb := NewDialoguePlayback(TranscriptDeps{
		Random:       func(n int) int { return 0 },
		CloseChatbox: func(*Player) { closed++ },
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		Type:    "random",
		Options: []definition.DialogueOptionStep{{Text: "a", Hook: "quest:x", Steps: []definition.DialogueStep{transcriptNPCStep("A.")}}},
	}}, nil, nil)
	if closed != 1 {
		t.Fatalf("closes = %d, want 1", closed)
	}
	if len(s.messages) != 1 || s.messages[0] != UnavailableMessage {
		t.Fatalf("messages = %v", s.messages)
	}
}

// --- Jumps -------------------------------------------------------
// (NpcDialogues.plugin.js:638-766, MAX_JUMPS at :44.)

func TestStartDialogueJumpCutoff(t *testing.T) {
	// A question whose continuation loops back on itself must
	// stop at MAX_JUMPS, not forever: the page index says the
	// "Hello there" question continues as [player line, jump
	// below], and each jump replays it.
	record := &definition.TranscriptRecord{
		Steps: []definition.DialogueStep{{
			Type: "choice",
			Options: []definition.DialogueOptionStep{{
				Text: "Hello there",
				Steps: []definition.DialogueStep{
					transcriptPlayerStep("Hello there"),
					{Type: "jump", Reference: "below"},
				},
			}},
		}},
	}
	options := CollectPageOptions(record)
	ctx := &TranscriptContext{PageOptions: options.ByText, PageOptionList: options.List}
	p, _ := newDialoguePlayer()
	closed := 0
	pb := NewDialoguePlayback(TranscriptDeps{CloseChatbox: func(*Player) { closed++ }})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		transcriptPlayerStep("Hello there"),
		{Type: "jump", Reference: "below"},
	}, nil, ctx)
	used := driveDialogue(t, p.GetDialogueManager(), 400)
	if closed != 1 {
		t.Fatalf("closes = %d, want 1 (the cutoff)", closed)
	}
	// One advance per jump, plus the final advance off the
	// chain: the cutoff fires just past MAX_JUMPS.
	if used > MaxTranscriptJumps+5 {
		t.Fatalf("used %d advances, want ~%d", used, MaxTranscriptJumps+2)
	}
}

func TestStartDialogueJumpOtherReplaysMenu(t *testing.T) {
	p, _ := newDialoguePlayer()
	capture := &transcriptMenuCapture{}
	var vias []string
	pb := NewDialoguePlayback(TranscriptDeps{
		PresentMenu: capture.present,
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueMenuEvent); ok {
				vias = append(vias, ev.Via)
			}
		},
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		Type: "choice",
		Options: []definition.DialogueOptionStep{
			{Text: "Again", Steps: []definition.DialogueStep{
				transcriptNPCStep("Picked."),
				{Type: "jump", Reference: "other"},
			}},
			{Text: "Done", Steps: []definition.DialogueStep{transcriptNPCStep("Done.")}},
		},
	}}, nil, nil)
	if len(capture.options) != 1 {
		t.Fatalf("menus = %d, want 1", len(capture.options))
	}
	capture.options[0][0].Select()
	if got := currentDialogueText(p.GetDialogueManager()); got != "Picked." {
		t.Fatalf("current = %q", got)
	}
	driveDialogue(t, p.GetDialogueManager(), 10)
	if len(capture.options) != 2 {
		t.Fatalf("menus = %d, want 2 (replay via other)", len(capture.options))
	}
	if len(vias) != 2 || vias[1] != "other" {
		t.Fatalf("vias = %v, want [\"\" other]", vias)
	}
}

// --- Messages, actions, calls, terminal steps --------------------
// (NpcDialogues.plugin.js:1042-1191.)

func TestStartDialogueMessageStep(t *testing.T) {
	p, s := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		{Type: "message", Text: "You find a coin.", HasText: true},
		transcriptNPCStep("After."),
	}, nil, nil)
	driveDialogue(t, p.GetDialogueManager(), 10)
	if len(s.messages) != 1 || s.messages[0] != "You find a coin." {
		t.Fatalf("messages = %v", s.messages)
	}
	if !sessionHas(s, "After.") {
		t.Error("continuation did not play")
	}
}

func TestStartDialogueMessageHandledSuppresses(t *testing.T) {
	p, s := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueActionEvent); ok && ev.Kind == "message" {
				ev.Handled = true
			}
		},
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		{Type: "message", Text: "Suppressed.", HasText: true},
	}, nil, nil)
	driveDialogue(t, p.GetDialogueManager(), 10)
	if len(s.messages) != 0 {
		t.Fatalf("messages = %v, want none", s.messages)
	}
}

func TestStartDialogueMessagePlainPathRereadsStepText(t *testing.T) {
	// TS quirk, preserved: only the replacement path uses the
	// handler-rewritten message text; the plain chat path
	// re-reads the step's text.
	p, s := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueActionEvent); ok && ev.Kind == "message" {
				ev.Text = "Rewritten."
			}
		},
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		{Type: "message", Text: "Original.", HasText: true},
	}, nil, nil)
	driveDialogue(t, p.GetDialogueManager(), 10)
	if len(s.messages) != 1 || s.messages[0] != "Original." {
		t.Fatalf("messages = %v, want the step text", s.messages)
	}
}

func TestStartDialogueMessageBoxPath(t *testing.T) {
	p, s := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueActionEvent); ok && ev.Kind == "message" {
				ev.Box = &NpcDialogueBox{Items: []int{995}}
			}
		},
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		{Type: "message", Text: "You get coins.", HasText: true},
	}, nil, nil)
	entry, ok := p.GetDialogueManager().GetCurrent().(*ItemStatementDialogue)
	if !ok {
		t.Fatalf("current is %T, want *ItemStatementDialogue", p.GetDialogueManager().GetCurrent())
	}
	if entry.GetItemID() != 995 || entry.GetText() != "You get coins." {
		t.Fatalf("box = (%d, %q)", entry.GetItemID(), entry.GetText())
	}
	driveDialogue(t, p.GetDialogueManager(), 10)
	// The box path sends no plain game message (its chat copy
	// is a mesbox typed line, recorded separately).
	if len(s.messages) != 0 {
		t.Fatalf("messages = %v, want none", s.messages)
	}
}

func TestStartDialogueGenericActionHandledSteps(t *testing.T) {
	p, s := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueActionEvent); ok && ev.Kind == "" && ev.Action == "teleport" {
				ev.Handled = true
				ev.Steps = []definition.DialogueStep{transcriptNPCStep("Teleported.")}
			}
		},
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		{Type: "action", Action: "teleport", Target: "Lumbridge"},
		transcriptNPCStep("After."),
	}, nil, nil)
	driveDialogue(t, p.GetDialogueManager(), 10)
	if !sessionHas(s, "Teleported.") || !sessionHas(s, "After.") {
		t.Errorf("handler steps / continuation missing:\n%s", sessionCalls(s))
	}
}

func TestStartDialogueCallBranch(t *testing.T) {
	p, s := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{})
	branches := map[string][]definition.DialogueStep{
		"greeting": {transcriptNPCStep("Called branch.")},
	}
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		{Type: "call", Branch: "greeting"},
		transcriptNPCStep("After."),
	}, branches, nil)
	driveDialogue(t, p.GetDialogueManager(), 10)
	if !sessionHas(s, "Called branch.") || !sessionHas(s, "After.") {
		t.Errorf("call branch missing:\n%s", sessionCalls(s))
	}
}

func TestStartDialogueCallMissingBranchUnavailables(t *testing.T) {
	p, s := newDialoguePlayer()
	closed := 0
	pb := NewDialoguePlayback(TranscriptDeps{CloseChatbox: func(*Player) { closed++ }})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		{Type: "call", Branch: "nope"},
	}, nil, nil)
	if closed != 1 {
		t.Fatalf("closes = %d, want 1", closed)
	}
	if len(s.messages) != 1 || s.messages[0] != UnavailableMessage {
		t.Fatalf("messages = %v", s.messages)
	}
}

func TestStartDialogueEndStep(t *testing.T) {
	p, _ := newDialoguePlayer()
	closed := 0
	pb := NewDialoguePlayback(TranscriptDeps{CloseChatbox: func(*Player) { closed++ }})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		transcriptNPCStep("Hi."),
		{Type: "end"},
		transcriptNPCStep("Never."),
	}, nil, nil)
	driveDialogue(t, p.GetDialogueManager(), 10)
	if closed != 1 {
		t.Fatalf("closes = %d, want 1", closed)
	}
}

func TestStartDialogueUnavailableAfterSpeechSendsNoMessage(t *testing.T) {
	p, s := newDialoguePlayer()
	closed := 0
	pb := NewDialoguePlayback(TranscriptDeps{CloseChatbox: func(*Player) { closed++ }})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		transcriptNPCStep("Hi."),
		{Type: "unavailable"},
	}, nil, nil)
	driveDialogue(t, p.GetDialogueManager(), 10)
	if closed != 1 {
		t.Fatalf("closes = %d, want 1", closed)
	}
	// Something played, so no "isn't available" message.
	if len(s.messages) != 0 {
		t.Fatalf("messages = %v, want none", s.messages)
	}
}

func TestStartDialogueUnknownStepUnavailables(t *testing.T) {
	p, s := newDialoguePlayer()
	closed := 0
	pb := NewDialoguePlayback(TranscriptDeps{CloseChatbox: func(*Player) { closed++ }})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		{Type: "missing"},
	}, nil, nil)
	if closed != 1 {
		t.Fatalf("closes = %d, want 1", closed)
	}
	if len(s.messages) != 1 || s.messages[0] != UnavailableMessage {
		t.Fatalf("messages = %v", s.messages)
	}
}

// --- Shops -------------------------------------------------------
// (NpcDialogues.plugin.js:578-584, 1149-1158; no ShopManager in
// Go, so only the event + version-picking + seams are tested.)

func TestStartDialogueOpenShopDefaultVersion(t *testing.T) {
	p, _ := newDialoguePlayer()
	var shopEvents []*NpcDialogueShopEvent
	var found []string
	var opened []int
	closed := 0
	pb := NewDialoguePlayback(TranscriptDeps{
		CloseChatbox: func(*Player) { closed++ },
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueShopEvent); ok {
				shopEvents = append(shopEvents, ev)
			}
		},
		FindShops: func(name string) []TranscriptShop {
			found = append(found, name)
			return []TranscriptShop{{ID: 7, Source: "core"}}
		},
		OpenShop: func(_ *Player, id int) bool { opened = append(opened, id); return true },
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		Type: "action", Action: "open_shop", Target: "Bob's Shop",
		Shops: []string{"Bob's Shop (Other)", "Bob's Shop (Default)"}, HasShops: true,
	}}, nil, nil)
	if len(shopEvents) != 1 {
		t.Fatalf("shop events = %d, want 1", len(shopEvents))
	}
	if len(found) != 1 || found[0] != "Bob's Shop (Default)" {
		t.Fatalf("FindShops got %v, want the Default version", found)
	}
	if len(opened) != 1 || closed != 1 {
		t.Fatalf("opened = %v, closes = %d", opened, closed)
	}
}

func TestStartDialogueOpenShopHandlerPicksVersion(t *testing.T) {
	p, _ := newDialoguePlayer()
	var found []string
	pb := NewDialoguePlayback(TranscriptDeps{
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueShopEvent); ok {
				picked := "Bob's Shop (Other)"
				ev.Shop = &picked
			}
		},
		FindShops: func(name string) []TranscriptShop {
			found = append(found, name)
			return nil
		},
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		Type: "action", Action: "open_shop", Target: "Bob's Shop",
		Shops: []string{"Bob's Shop (Other)", "Bob's Shop (Default)"}, HasShops: true,
	}, transcriptNPCStep("After.")}, nil, nil)
	driveDialogue(t, p.GetDialogueManager(), 10)
	if len(found) != 1 || found[0] != "Bob's Shop (Other)" {
		t.Fatalf("FindShops got %v, want the handler-picked version", found)
	}
}

func TestStartDialogueOpenShopPrefersCore(t *testing.T) {
	p, _ := newDialoguePlayer()
	var opened []int
	pb := NewDialoguePlayback(TranscriptDeps{
		FindShops: func(string) []TranscriptShop {
			return []TranscriptShop{{ID: 9, Source: "plugin"}, {ID: 7, Source: "core"}}
		},
		OpenShop: func(_ *Player, id int) bool { opened = append(opened, id); return true },
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		Type: "action", Action: "open_shop", Target: "Shop",
	}}, nil, nil)
	if len(opened) != 1 || opened[0] != 7 {
		t.Fatalf("opened = %v, want [7] (the core shop)", opened)
	}
}

func TestStartDialogueOpenShopNoSeamsFallsThrough(t *testing.T) {
	p, s := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		{Type: "action", Action: "open_shop", Target: "Shop"},
		transcriptNPCStep("After."),
	}, nil, nil)
	driveDialogue(t, p.GetDialogueManager(), 10)
	if !sessionHas(s, "After.") {
		t.Error("open_shop without seams did not fall through")
	}
}

// --- Slayer ------------------------------------------------------
// (NpcDialogues.plugin.js:1153-1162.)

func TestStartDialogueSlayerAssignmentLine(t *testing.T) {
	p, _ := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*SlayerAssignmentEvent); ok {
				ev.Line = "Kill 10 goblins."
			}
		},
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		{Type: "action", Action: "slayer_assignment", Target: "Turael"},
	}, nil, nil)
	if got := currentDialogueText(p.GetDialogueManager()); got != "Kill 10 goblins." {
		t.Fatalf("current = %q", got)
	}
}

// --- Overhead ----------------------------------------------------
// (NpcDialogues.plugin.js:61-128, 964-1005.)

func transcriptRegisteredNPC(p *Player, id int, loc Location, visible bool) *NPC {
	npc := NewNPC(id, loc)
	npc.Index = id
	npc.SetRegistered(true)
	npc.Visible = visible
	p.localNPCs = append(p.localNPCs, npc)
	return npc
}

// forcedChatText dereferences a mobile's forced chat, "" when none.
func forcedChatText(m *Mobile) string {
	if text := m.GetForcedChat(); text != nil {
		return *text
	}
	return ""
}

func TestStartDialogueOverheadNPCLine(t *testing.T) {
	p, _ := newDialoguePlayer()
	p.SetRegistered(true)
	npc := transcriptRegisteredNPC(p, 123, Location{X: 3090, Y: 3524, Z: 0}, true)
	pb := NewDialoguePlayback(TranscriptDeps{
		NPCName: func(n *NPC, _ *Player) string {
			if n == npc {
				return "Bob"
			}
			return ""
		},
	})
	event := transcriptTalkEvent(p)
	event.NPC = npc
	pb.StartDialogue(event, []definition.DialogueStep{{
		NPCText: "Halt!", HasNPC: true, Presentation: "overhead",
	}}, nil, nil)
	if got := forcedChatText(&npc.Mobile); got != "Halt!" {
		t.Fatalf("forced chat = %q, want Halt!", got)
	}
	if p.GetDialogueManager().IsActive() {
		t.Error("final overhead left the manager active")
	}
}

func TestStartDialogueOverheadTypedSpeaker(t *testing.T) {
	p, _ := newDialoguePlayer()
	p.SetRegistered(true)
	bob := transcriptRegisteredNPC(p, 123, Location{X: 3090, Y: 3524, Z: 0}, true)
	alice := transcriptRegisteredNPC(p, 456, Location{X: 3091, Y: 3524, Z: 0}, true)
	pb := NewDialoguePlayback(TranscriptDeps{
		NPCName: func(n *NPC, _ *Player) string {
			switch n {
			case bob:
				return "Bob"
			case alice:
				return "Alice"
			}
			return ""
		},
	})
	event := transcriptTalkEvent(p)
	event.NPC = bob
	pb.StartDialogue(event, []definition.DialogueStep{{
		Type: "line", Text: "Over here!", HasText: true,
		Speaker: "Alice", HasSpeaker: true, Presentation: "overhead",
	}}, nil, nil)
	if got := forcedChatText(&alice.Mobile); got != "Over here!" {
		t.Fatalf("Alice forced chat = %q", got)
	}
	if got := forcedChatText(&bob.Mobile); got != "" {
		t.Fatalf("Bob forced chat = %q, want none", got)
	}
}

func TestStartDialogueOverheadPlayerLine(t *testing.T) {
	p, _ := newDialoguePlayer()
	p.SetRegistered(true)
	pb := NewDialoguePlayback(TranscriptDeps{})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		PlayerText: "Hmm.", HasPlayer: true, Presentation: "overhead",
	}}, nil, nil)
	if got := forcedChatText(&p.Mobile); got != "Hmm." {
		t.Fatalf("player forced chat = %q", got)
	}
}

func TestStartDialogueOverheadOutOfViewFallsBackToChat(t *testing.T) {
	p, s := newDialoguePlayer()
	p.SetRegistered(true)
	pb := NewDialoguePlayback(TranscriptDeps{})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		NPCText: "Halt!", HasNPC: true, Presentation: "overhead",
	}}, nil, nil)
	if len(s.messages) != 1 || s.messages[0] != "Bob: Halt!" {
		t.Fatalf("messages = %v, want the marked chat fallback", s.messages)
	}
}

func TestStartDialogueOverheadExplicitCastWithPlayerUnavailables(t *testing.T) {
	// TS quirk, preserved: players have no isVisible, so an
	// explicitly bound cast containing the player fails the
	// availability check and the line unavailables.
	p, s := newDialoguePlayer()
	p.SetRegistered(true)
	closed := 0
	pb := NewDialoguePlayback(TranscriptDeps{
		CloseChatbox: func(*Player) { closed++ },
		EmitCustomEvent: func(name string, payload any) {
			if ev, ok := payload.(*NpcDialogueLineEvent); ok {
				ev.Actors = []DialogueActor{p}
			}
		},
	})
	pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{{
		NPCText: "Halt!", HasNPC: true, Presentation: "overhead",
	}}, nil, nil)
	if closed != 1 {
		t.Fatalf("closes = %d, want 1", closed)
	}
	if len(s.messages) != 1 || s.messages[0] != UnavailableMessage {
		t.Fatalf("messages = %v", s.messages)
	}
}

// --- Cancellation ------------------------------------------------
// (NpcDialogues.plugin.js:580-598.)

func TestStartDialogueCancelExplicit(t *testing.T) {
	p, _ := newDialoguePlayer()
	closed := 0
	pb := NewDialoguePlayback(TranscriptDeps{CloseChatbox: func(*Player) { closed++ }})
	event := transcriptTalkEvent(p)
	event.Explicit = true
	cancel := pb.StartDialogue(event, []definition.DialogueStep{
		transcriptNPCStep("Hi."),
	}, nil, nil)
	if !cancel() {
		t.Fatal("explicit cancel returned false")
	}
	if p.GetDialogueManager().IsActive() {
		t.Error("manager still active after cancel")
	}
	if closed != 1 {
		t.Fatalf("closes = %d, want 1", closed)
	}
	if cancel() {
		t.Error("second cancel returned true")
	}
}

func TestStartDialogueCancelOrdinaryIsNoop(t *testing.T) {
	p, _ := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{})
	cancel := pb.StartDialogue(transcriptTalkEvent(p), []definition.DialogueStep{
		transcriptNPCStep("Hi."),
	}, nil, nil)
	if cancel() {
		t.Error("ordinary cancel returned true")
	}
	if !p.GetDialogueManager().IsActive() {
		t.Error("ordinary cancel stopped the chain")
	}
}

func TestStartDialogueCancelAfterChainEnds(t *testing.T) {
	p, _ := newDialoguePlayer()
	pb := NewDialoguePlayback(TranscriptDeps{})
	event := transcriptTalkEvent(p)
	event.Explicit = true
	cancel := pb.StartDialogue(event, []definition.DialogueStep{
		transcriptNPCStep("Hi."),
	}, nil, nil)
	driveDialogue(t, p.GetDialogueManager(), 10)
	if cancel() {
		t.Error("cancel after the chain ended returned true")
	}
}

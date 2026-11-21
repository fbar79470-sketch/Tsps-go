package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"tsps-go/internal/definition"
	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/plugins"
)

// testDialogueDir writes a minimal definitions dir holding one
// transcript page ("Hudo") indexed under NPC id 12, plus nothing
// else. The content is hand-written for this test; only the file
// shapes mirror the upstream format.
func testDialogueDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"npc-dialogues.json":      `{"Hudo": {"steps": [{"npc": "Hello."}, {"player": "Hi."}]}}`,
		"npc-dialogue-index.json": `{"12": [{"page": "Transcript:Hudo", "variants": ["standard-dialogue"]}]}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func discardLogger() *log.Logger { return log.New(io.Discard, "", 0) }

func TestLoadDialoguePlaybackLoadsAndBuilds(t *testing.T) {
	dir := testDialogueDir(t)
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	hub := plugins.NewHub(nil, plugins.HubOptions{Logger: discardLogger()})
	playback, err := loadDialoguePlayback(dir, nil, world, hub, nil, discardLogger().Printf, nil)
	if err != nil {
		t.Fatalf("loadDialoguePlayback: %v", err)
	}
	if playback == nil {
		t.Fatal("playback is nil")
	}
	// The catalogue resolves NPC 12 to the Hudo page.
	resolved := playback.ResolveTranscript(game.TranscriptEvent{
		Player: game.NewPlayer("alice", game.Location{X: 3000, Y: 3000}),
		NPCID:  12,
	})
	if resolved == nil || len(resolved.Steps) != 2 {
		t.Fatalf("resolved = %+v, want 2 steps", resolved)
	}
}

func TestLoadDialoguePlaybackMissingMainFileFailsBoot(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadDialoguePlayback(dir, nil, nil, nil, nil, discardLogger().Printf, nil); err == nil {
		t.Fatal("missing npc-dialogues.json must fail the load")
	}
}

func TestNpcHooksTalkToGate(t *testing.T) {
	dir := testDialogueDir(t)
	playback, err := loadDialoguePlayback(dir, nil, nil, nil, nil, discardLogger().Printf, nil)
	if err != nil {
		t.Fatalf("loadDialoguePlayback: %v", err)
	}
	hub := plugins.NewHub(nil, plugins.HubOptions{Logger: discardLogger()})
	hooks := NpcHooks(playback, hub)
	player := game.NewPlayer("alice", game.Location{X: 3000, Y: 3000})
	npc := game.NewNPC(12, game.Location{X: 3001, Y: 3000})

	// Talk-to starts a dialogue (the Hudo transcript resolves).
	if !hooks.StartDialogue(&listeners.NpcDialogueRequest{
		Player: player, NPC: npc, NpcID: 12, ClickType: 1, Option: "talk-to", HasOption: true,
	}) {
		t.Fatal("Talk-to click did not start a dialogue")
	}
	if !player.GetDialogueManager().IsActive() {
		t.Fatal("dialogue manager is not active after Talk-to")
	}
	// Any other option falls through, even for the same NPC.
	player.GetDialogueManager().Reset()
	if hooks.StartDialogue(&listeners.NpcDialogueRequest{
		Player: player, NPC: npc, NpcID: 12, ClickType: 2, Option: "trade", HasOption: true,
	}) {
		t.Fatal("Trade click started a dialogue")
	}
	if hooks.StartDialogue(&listeners.NpcDialogueRequest{
		Player: player, NPC: npc, NpcID: 12, ClickType: 1, HasOption: false,
	}) {
		t.Fatal("option-less click started a dialogue")
	}
	// A nil playback never starts.
	if NpcHooks(nil, hub).StartDialogue != nil {
		t.Fatal("nil playback must yield no StartDialogue seam")
	}
}

func TestNpcHooksOperatedDelivery(t *testing.T) {
	hub := plugins.NewHub(nil, plugins.HubOptions{Logger: discardLogger()})
	var delivered *plugins.NpcOperatedEvent
	hub.SubscribeCustomEvent(plugins.NpcOperatedEventName, func(payload any) {
		delivered, _ = payload.(*plugins.NpcOperatedEvent)
	})
	hooks := NpcHooks(nil, hub)
	if hooks.EmitOperated == nil {
		t.Fatal("EmitOperated is nil with a hub wired")
	}
	player := game.NewPlayer("alice", game.Location{X: 3000, Y: 3000})
	hooks.EmitOperated(&listeners.NpcOperatedEvent{Player: player, NpcID: 12, Option: "talk-to", HasOption: true})
	if delivered == nil || delivered.NpcID != 12 || delivered.Option != "talk-to" || !delivered.HasOption {
		t.Fatalf("delivered = %+v", delivered)
	}
	if NpcHooks(nil, nil).EmitOperated != nil {
		t.Fatal("nil hub must yield no EmitOperated seam")
	}
}

func TestSubscribeDialogueStartPlaysNamedVariant(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"npc-dialogues.json": `{"Hudo": {"variants": {"greeting": [{"npc": "Welcome."}]}}}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	playback, err := loadDialoguePlayback(dir, nil, nil, nil, nil, discardLogger().Printf, nil)
	if err != nil {
		t.Fatalf("loadDialoguePlayback: %v", err)
	}
	hub := plugins.NewHub(nil, plugins.HubOptions{Logger: discardLogger()})
	SubscribeDialogueStart(hub, playback)
	player := game.NewPlayer("alice", game.Location{X: 3000, Y: 3000})
	req := &game.StartNamedRequest{
		Player: player, NPCID: 12, HasNPCID: true,
		DefinitionName: "Hudo", DefinitionID: 12,
		Variant: "greeting", HasVariant: true,
	}
	hub.EmitCustomEvent(plugins.NpcDialogueStartEventName, req)
	if !req.Handled {
		t.Fatal("npc-dialogue:start request was not handled")
	}
	if !player.GetDialogueManager().IsActive() {
		t.Fatal("dialogue manager is not active after npc-dialogue:start")
	}
	// A wrong payload shape is ignored, not a panic.
	hub.EmitCustomEvent(plugins.NpcDialogueStartEventName, "not-a-request")
	// Nil hub / nil playback subscribe nothing (no panic).
	SubscribeDialogueStart(nil, playback)
	SubscribeDialogueStart(hub, nil)
}

func TestInstallDialogueListeners(t *testing.T) {
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	hub := plugins.NewHub(nil, plugins.HubOptions{Logger: discardLogger()})
	var deps listeners.Deps
	InstallDialogueListeners(&deps, nil, hub, nil, world.NPCs(), nil)
	if deps.NPCs == nil {
		t.Fatal("deps.NPCs unset")
	}
	if deps.Dialogue == nil {
		t.Fatal("deps.Dialogue unset")
	}
	if deps.NpcHooks.StartDialogue != nil {
		t.Fatal("nil playback must yield no StartDialogue seam")
	}
	// Nil deps is a no-op; nil NPC list leaves the lookup unset.
	InstallDialogueListeners(nil, nil, hub, nil, nil, nil)
	var deps2 listeners.Deps
	InstallDialogueListeners(&deps2, nil, hub, nil, nil, nil)
	if deps2.NPCs != nil {
		t.Fatal("nil NPC list must leave deps.NPCs unset")
	}
}

func TestNpcListLookupAndDefinitionSource(t *testing.T) {
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	npc := game.NewNPC(12, game.Location{X: 3001, Y: 3000})
	index, ok := world.NPCs().Add(npc)
	if !ok {
		t.Fatal("NPC add failed")
	}
	lookup := npcListLookup{list: world.NPCs()}
	if got := lookup.Find(index); got != npc {
		t.Fatalf("Find(%d) = %v, want the NPC", index, got)
	}
	if got := lookup.Find(index + 1); got != nil {
		t.Fatalf("Find(empty slot) = %v, want nil", got)
	}
	if got := (npcListLookup{}).Find(index); got != nil {
		t.Fatalf("nil list Find = %v, want nil", got)
	}

	// A Store with no cache hydrates no definition: every id is
	// absent (the DEFAULT shape), and the source reports it.
	// The zero Store is enough: NPC reads only the nil cache
	// and nil maps.
	store := &definition.Store{}
	if NewNpcDefinitionSource(nil) != nil {
		t.Fatal("nil store must yield a nil source")
	}
	source := NewNpcDefinitionSource(store)
	if _, ok := source.NpcForPlayer(12, nil); ok {
		t.Fatal("no-cache store must report NPC 12 unresolved")
	}
}

func TestDialogueAccessOverManager(t *testing.T) {
	access := dialogueAccess{}
	player := game.NewPlayer("alice", game.Location{X: 3000, Y: 3000})
	if access.IsActive(player) {
		t.Fatal("IsActive = true with no chain")
	}
	if access.Current(player) != nil {
		t.Fatal("Current non-nil with no chain")
	}
	if access.Resume(player, 0, 0) {
		t.Fatal("Resume = true with no chain")
	}
	// Nil player never panics.
	if access.IsActive(nil) || access.Current(nil) != nil || access.Resume(nil, 0, 0) {
		t.Fatal("nil player must read inactive")
	}
	access.HandleOption(nil, 0)

	// Start a two-line chain: Current identifies the first
	// entry, Resume advances it, and the token changes.
	builder := game.NewDialogueChainBuilder()
	builder.Add(game.NewPlayerDialogue(0, "Hello."))
	builder.Add(game.NewPlayerDialogue(1, "Goodbye."))
	player.GetDialogueManager().StartDialogues(builder)
	first := access.Current(player)
	if first == nil {
		t.Fatal("Current is nil with an active chain")
	}
	// No session is attached, so CanContinue is false and the
	// resume must not advance (no chatbox to continue).
	if access.Resume(player, (217<<16)|5, 0) {
		t.Fatal("Resume advanced with no session/chatbox")
	}
	if access.Current(player) != first {
		t.Fatal("Current changed after a refused resume")
	}
}

func TestTranslateVariantAndConditionContexts(t *testing.T) {
	event := game.NpcDialogueVariantEvent{
		Player: game.NewPlayer("alice", game.Location{}),
		NPCID:  12, DefinitionID: 12, DefinitionName: "Hudo",
		Pages: []game.TranscriptPageRef{{Page: "Hudo", Variants: []string{"standard-dialogue"}}},
	}
	ctx := translateVariantContext(event, nil)
	if ctx.NpcID != 12 || ctx.Definition != nil {
		t.Fatalf("context = %+v, want NpcID 12 and nil definition without a store", ctx)
	}
	if len(ctx.Pages) != 1 || ctx.Pages[0].Page != "Hudo" || len(ctx.Pages[0].Variants) != 1 {
		t.Fatalf("pages = %+v", ctx.Pages)
	}
	cond := translateConditionEvent(game.NpcDialogueConditionQuery{
		Player: event.Player, NPCID: 12, Text: "If the player has the items:", StepID: "c1",
	}, nil)
	if cond.Text != "If the player has the items:" || cond.StepID != "c1" || !cond.HasStepID {
		t.Fatalf("condition event = %+v", cond)
	}
	condNoID := translateConditionEvent(game.NpcDialogueConditionQuery{Text: "x"}, nil)
	if condNoID.HasStepID {
		t.Fatal("empty StepID must read as no anchor id")
	}
}

// resumeTestSession is a minimal DialogueSender for the
// resume-order test: it embeds the (nil) SessionPort interface
// and tracks the mounted chatbox group.
type resumeTestSession struct {
	game.SessionPort
	chatboxID int
	removals  int
}

func (s *resumeTestSession) SendChatboxInterface(groupID int, unclamp bool) { s.chatboxID = groupID }
func (s *resumeTestSession) SendNpcHeadOnInterface(int, int)                {}
func (s *resumeTestSession) SendPlayerHeadOnInterface(int)                  {}
func (s *resumeTestSession) SendInterfaceAnimation(int, int)                {}
func (s *resumeTestSession) SendString(string, int)                         {}
func (s *resumeTestSession) SendClientScript(int, ...game.ClientScriptArg)  {}
func (s *resumeTestSession) SendInterfaceFlagsRange(int, int, int, int)     {}
func (s *resumeTestSession) SendItemOnInterfaces(int, int, int)             {}
func (s *resumeTestSession) SendInterfaceScript(int)                        {}
func (s *resumeTestSession) SendInterfaceRemoval() {
	s.removals++
	s.chatboxID = -1
}
func (s *resumeTestSession) IsChatboxInterface(groupID int) bool { return groupID == s.chatboxID }
func (s *resumeTestSession) WithChatboxOwner(owner any, present func()) {
	present()
}

// TestDialogueResumePromptFirst proves the dialogue_continue
// dispatch order: a pending MultiChatboxPrompt consumes the
// resume before the dialogue manager sees it
// (NetworkBuilder.ts:939-951). The manager has no active
// dialogue here, so a manager-only Resume would refuse; the
// prompt pick must be consumed and run its callback.
func TestDialogueResumePromptFirst(t *testing.T) {
	prompts := game.NewMultiChatboxPrompt(nil)
	hub := plugins.NewHub(nil, plugins.HubOptions{Logger: discardLogger()})
	var deps listeners.Deps
	InstallDialogueListeners(&deps, nil, hub, prompts, nil, nil)
	if deps.Dialogue == nil {
		t.Fatal("InstallDialogueListeners left deps.Dialogue unset")
	}
	p := game.NewPlayer("testplayer", game.Location{X: 3089, Y: 3524, Z: 0})
	s := &resumeTestSession{chatboxID: -1}
	p.Session = s
	picked := -1
	if !prompts.ShowPrompt("NpcDialogues", p, "Pick", []game.MenuOption{
		{Label: "A", Select: func() { picked = 0 }},
		{Label: "B", Select: func() { picked = 1 }},
	}) {
		t.Fatal("ShowPrompt reported false")
	}
	if !deps.Dialogue.Resume(p, int32(game.OptionsWidgetID), 2) {
		t.Fatal("Resume did not consume the prompt pick")
	}
	if picked != 1 {
		t.Fatalf("picked = %d, want 1", picked)
	}
	if s.removals != 1 {
		t.Fatalf("removals = %d, want 1", s.removals)
	}
	// The prompt is gone: the same resume now reaches the
	// manager, which refuses it (no active dialogue).
	if deps.Dialogue.Resume(p, int32(game.OptionsWidgetID), 1) {
		t.Fatal("Resume consumed a pick after the prompt was answered")
	}
}

// fakeShopOpener records dialogue shop-step calls for
// TestDialoguePlaybackShopSeams.
type fakeShopOpener struct {
	found  []game.TranscriptShop
	opened []int
}

func (f *fakeShopOpener) FindShops(name string) []game.TranscriptShop {
	if name == "Test Shop" {
		return f.found
	}
	return nil
}

func (f *fakeShopOpener) OpenShop(player *game.Player, shopID int) bool {
	f.opened = append(f.opened, shopID)
	return shopID == 7
}

// TestDialoguePlaybackShopSeams pins the milestone-7 binding: a
// non-nil ShopOpener installs the FindShops/OpenShop seams on
// the playback deps (delegating to the opener), and a nil
// opener leaves them unset (the playback's no-shop fallback).
func TestDialoguePlaybackShopSeams(t *testing.T) {
	opener := &fakeShopOpener{found: []game.TranscriptShop{{ID: 7, Source: "core"}}}
	deps := dialoguePlaybackDeps(nil, nil, nil, nil, nil, discardLogger().Printf, opener)
	if deps.FindShops == nil || deps.OpenShop == nil {
		t.Fatal("shop seams unset with a non-nil opener")
	}
	if found := deps.FindShops("Test Shop"); len(found) != 1 || found[0].ID != 7 {
		t.Errorf("FindShops = %+v, want shop 7", found)
	}
	p := game.NewPlayer("alice", game.Location{X: 3000, Y: 3000})
	if !deps.OpenShop(p, 7) || len(opener.opened) != 1 || opener.opened[0] != 7 {
		t.Errorf("OpenShop did not delegate: opened = %v", opener.opened)
	}
	plain := dialoguePlaybackDeps(nil, nil, nil, nil, nil, discardLogger().Printf, nil)
	if plain.FindShops != nil || plain.OpenShop != nil {
		t.Error("shop seams set with a nil opener")
	}
}

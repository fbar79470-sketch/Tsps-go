package definition

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dialogueFiles writes a definitions dir holding the given dialogue
// files (plus nothing else: LoadDialogueData reads only its three
// files, unlike Load).
func dialogueFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func mustLoadDialogue(t *testing.T, dir string, opts DialogueLoadOptions) *DialogueData {
	t.Helper()
	data, err := LoadDialogueData(dir, opts)
	if err != nil {
		t.Fatalf("LoadDialogueData: %v", err)
	}
	return data
}

func TestLoadDialogueMissingMainFileFails(t *testing.T) {
	dir := dialogueFiles(t, map[string]string{})
	if _, err := LoadDialogueData(dir, DialogueLoadOptions{}); err == nil {
		t.Fatal("missing npc-dialogues.json must fail the load")
	}
}

func TestLoadDialogueMalformedMainFileFails(t *testing.T) {
	dir := dialogueFiles(t, map[string]string{"npc-dialogues.json": `{oops`})
	if _, err := LoadDialogueData(dir, DialogueLoadOptions{}); err == nil {
		t.Fatal("malformed npc-dialogues.json must fail the load")
	}
}

func TestLoadDialogueNonObjectMainFileFails(t *testing.T) {
	for _, content := range []string{`[]`, `"text"`, `42`, `null`} {
		dir := dialogueFiles(t, map[string]string{"npc-dialogues.json": content})
		if _, err := LoadDialogueData(dir, DialogueLoadOptions{}); err == nil {
			t.Fatalf("npc-dialogues.json %s must fail the load", content)
		} else if !strings.Contains(err.Error(), "expected dialogues keyed by transcript name") {
			t.Fatalf("npc-dialogues.json %s: error %q lacks the TS message", content, err)
		}
	}
}

func TestLoadDialogueMissingEventAndIndexFilesOK(t *testing.T) {
	dir := dialogueFiles(t, map[string]string{
		"npc-dialogues.json": `{"Hudo": {"steps": [{"npc": "Hello."}]}}`,
	})
	data := mustLoadDialogue(t, dir, DialogueLoadOptions{})
	if got := data.Record("Hudo"); got == nil || len(got.Steps) != 1 || got.Steps[0].NPCText != "Hello." {
		t.Fatalf("Hudo record = %+v", got)
	}
	if len(data.Index) != 0 || len(data.EventNames) != 0 {
		t.Fatalf("index/event names should be empty: %+v", data)
	}
}

func TestLoadDialogueMalformedEventFileFails(t *testing.T) {
	dir := dialogueFiles(t, map[string]string{
		"npc-dialogues.json":   `{}`,
		"event-dialogues.json": `{oops`,
	})
	if _, err := LoadDialogueData(dir, DialogueLoadOptions{}); err == nil {
		t.Fatal("malformed event-dialogues.json must fail the load")
	}
}

func TestLoadDialogueNonObjectEventFileFails(t *testing.T) {
	dir := dialogueFiles(t, map[string]string{
		"npc-dialogues.json":   `{}`,
		"event-dialogues.json": `[]`,
	})
	if _, err := LoadDialogueData(dir, DialogueLoadOptions{}); err == nil {
		t.Fatal("non-object event-dialogues.json must fail the load")
	} else if !strings.Contains(err.Error(), "expected activity dialogues keyed by transcript name") {
		t.Fatalf("error %q lacks the TS message", err)
	}
}

func TestLoadDialogueEventRecordRequiresExplicitActivation(t *testing.T) {
	for _, event := range []string{
		`{"Xmas": {"steps": []}}`,
		`{"Xmas": {"activation": "auto", "steps": []}}`,
		`{"Xmas": {"activation": null, "steps": []}}`,
		`{"Xmas": "text"}`,
	} {
		dir := dialogueFiles(t, map[string]string{
			"npc-dialogues.json":   `{}`,
			"event-dialogues.json": event,
		})
		if _, err := LoadDialogueData(dir, DialogueLoadOptions{}); err == nil {
			t.Fatalf("event record %s must fail the load", event)
		} else if !strings.Contains(err.Error(), "must have an explicit activation") {
			t.Fatalf("event record %s: error %q lacks the TS message", event, err)
		}
	}
}

func TestLoadDialogueEventConflictFails(t *testing.T) {
	dir := dialogueFiles(t, map[string]string{
		"npc-dialogues.json":   `{"Shared": {"activation": "explicit", "steps": [{"npc": "A"}]}}`,
		"event-dialogues.json": `{"Shared": {"activation": "explicit", "steps": [{"npc": "B"}]}}`,
	})
	if _, err := LoadDialogueData(dir, DialogueLoadOptions{}); err == nil {
		t.Fatal("conflicting event record must fail the load")
	} else if !strings.Contains(err.Error(), "conflicts with the ordinary dialogue catalogue") {
		t.Fatalf("error %q lacks the TS message", err)
	}
}

func TestLoadDialogueEventIdenticalMergeOK(t *testing.T) {
	dir := dialogueFiles(t, map[string]string{
		"npc-dialogues.json":   `{"Shared": {"activation": "explicit", "steps": [{"npc": "A"}]}}`,
		"event-dialogues.json": `{"Shared": {"activation": "explicit", "steps": [{"npc": "A"}]}, "New": {"activation": "explicit", "steps": []}}`,
	})
	data := mustLoadDialogue(t, dir, DialogueLoadOptions{})
	if len(data.EventNames) != 2 || data.EventNames[0] != "Shared" || data.EventNames[1] != "New" {
		t.Fatalf("event names = %v", data.EventNames)
	}
	if len(data.RecordOrder) != 2 || data.RecordOrder[0] != "Shared" || data.RecordOrder[1] != "New" {
		t.Fatalf("record order = %v (spread order: existing keys keep position)", data.RecordOrder)
	}
	if got := data.Record("New"); got == nil || got.Activation != "explicit" {
		t.Fatalf("New record = %+v", got)
	}
}

func TestLoadDialogueValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"unsupported activation", `{"A": {"activation": "auto", "steps": []}}`, "has an unsupported activation"},
		{"null activation", `{"A": {"activation": null, "steps": []}}`, "has an unsupported activation"},
		{"non-array steps", `{"A": {"steps": {}}}`, "has non-array steps"},
		{"null steps", `{"A": {"steps": null}}`, "has non-array steps"},
		{"missing default", `{"A": {"default": "nope", "variants": {"standard-dialogue": []}}}`, "names a missing default variant"},
		{"null record", `{"A": null}`, "has an unsupported activation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := dialogueFiles(t, map[string]string{"npc-dialogues.json": tc.content})
			if _, err := LoadDialogueData(dir, DialogueLoadOptions{}); err == nil {
				t.Fatal("load must fail")
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q lacks %q", err, tc.want)
			}
		})
	}
}

func TestLoadDialogueNonObjectRecordPasses(t *testing.T) {
	// TS reads every field of a string/number/array record as
	// undefined, so validation passes and the record is
	// unplayable.
	dir := dialogueFiles(t, map[string]string{
		"npc-dialogues.json": `{"A": "text", "B": 42, "C": []}`,
	})
	data := mustLoadDialogue(t, dir, DialogueLoadOptions{})
	for _, name := range []string{"A", "B", "C"} {
		if _, playable := data.Record(name).PickVariant(); playable {
			t.Fatalf("%s must be unplayable", name)
		}
	}
}

func TestLoadDialogueIndexAnyFailureYieldsEmpty(t *testing.T) {
	// Missing, malformed, and non-object index files all yield
	// an empty index: the bare try/catch in TS load().
	for _, files := range []map[string]string{
		{"npc-dialogues.json": `{}`},
		{"npc-dialogues.json": `{}`, "npc-dialogue-index.json": `{oops`},
		{"npc-dialogues.json": `{}`, "npc-dialogue-index.json": `[]`},
		{"npc-dialogues.json": `{}`, "npc-dialogue-index.json": `"text"`},
	} {
		data := mustLoadDialogue(t, dialogueFiles(t, files), DialogueLoadOptions{})
		if len(data.Index) != 0 {
			t.Fatalf("index = %+v, want empty", data.Index)
		}
	}
}

func TestLoadDialogueIndexConversion(t *testing.T) {
	dir := dialogueFiles(t, map[string]string{
		"npc-dialogues.json": `{"Hudo": {"steps": []}}`,
		"npc-dialogue-index.json": `{
			"12": [{"page": "Transcript:Hudo", "variants": ["standard-dialogue"]}, {"page": "Transcript:Lost City"}],
			"007": [{"page": "Transcript:Nope"}],
			"abc": [{"page": "Transcript:Nope"}],
			"13": "not-an-array",
			"14": [null, {"page": 42, "variants": "nope"}, {"page": "Transcript:Ok", "variants": ["a", 7]}]
		}`,
	})
	data := mustLoadDialogue(t, dir, DialogueLoadOptions{})
	entries := data.IndexEntries(12)
	if len(entries) != 2 || entries[0].Page != "Transcript:Hudo" || entries[0].PageName() != "Hudo" {
		t.Fatalf("id 12 entries = %+v", entries)
	}
	if len(entries[0].Variants) != 1 || entries[0].Variants[0] != "standard-dialogue" {
		t.Fatalf("id 12 variants = %v", entries[0].Variants)
	}
	if len(entries[1].Variants) != 0 {
		t.Fatalf("missing variants must read as none: %+v", entries[1])
	}
	if data.IndexEntries(7) != nil || len(data.Index) != 2 {
		t.Fatalf("non-canonical/non-numeric keys must be skipped: %+v", data.Index)
	}
	if data.IndexEntries(13) != nil {
		t.Fatalf("non-array index value must yield no entries")
	}
	entries14 := data.IndexEntries(14)
	if len(entries14) != 2 || entries14[0].Page != "42" || len(entries14[0].Variants) != 0 {
		t.Fatalf("id 14 entries = %+v (null row skipped, numeric page stringified)", entries14)
	}
	if len(entries14[1].Variants) != 1 || entries14[1].Variants[0] != "a" {
		t.Fatalf("non-string variant elements must be dropped: %+v", entries14[1])
	}
}

func TestPickVariant(t *testing.T) {
	flat := &TranscriptRecord{Steps: []DialogueStep{{NPCText: "Hi", HasNPC: true}}, HasSteps: true}
	if steps, ok := flat.PickVariant(); !ok || len(steps) != 1 {
		t.Fatalf("flat record: steps=%v ok=%v", steps, ok)
	}
	explicit := &TranscriptRecord{Activation: "explicit", Steps: []DialogueStep{{}}, HasSteps: true}
	if _, ok := explicit.PickVariant(); ok {
		t.Fatal("explicit record must never pick by default")
	}
	variants := &TranscriptRecord{
		Variants:     map[string][]DialogueStep{"overhead-shout": {{}}, "standard-dialogue": {{Type: "line"}}},
		VariantOrder: []string{"overhead-shout", "standard-dialogue"},
	}
	if steps, ok := variants.PickVariant(); !ok || len(steps) != 1 || steps[0].Type != "line" {
		t.Fatalf("standard variant must win over overhead: %v %v", steps, ok)
	}
	overheadOnly := &TranscriptRecord{
		Variants:     map[string][]DialogueStep{"overhead-shout": {{}}},
		VariantOrder: []string{"overhead-shout"},
	}
	if _, ok := overheadOnly.PickVariant(); ok {
		t.Fatal("overhead-only record must be unplayable")
	}
	def := &TranscriptRecord{
		Variants:     map[string][]DialogueStep{"standard-dialogue": {{Type: "line"}}, "quest": {{Type: "message"}}},
		VariantOrder: []string{"standard-dialogue", "quest"},
		Default:      "quest", HasDefault: true,
	}
	if steps, ok := def.PickVariant(); !ok || steps[0].Type != "message" {
		t.Fatalf("named default must win: %v %v", steps, ok)
	}
	// A default naming a NON-ARRAY variant yields unplayable
	// with no fallback to the standard search (TS returns
	// Array.isArray(variants[key]) ? ... : undefined).
	nonArrayDefault := &TranscriptRecord{
		Variants:     map[string][]DialogueStep{"standard-dialogue": {{Type: "line"}}},
		VariantOrder: []string{"standard-dialogue", "broken"},
		Default:      "broken", HasDefault: true,
	}
	if _, ok := nonArrayDefault.PickVariant(); ok {
		t.Fatal("default naming a non-array variant must be unplayable, not fall back")
	}
	empty := &TranscriptRecord{Variants: map[string][]DialogueStep{"standard-dialogue": {}}, VariantOrder: []string{"standard-dialogue"}}
	if steps, ok := empty.PickVariant(); !ok || steps == nil {
		t.Fatalf("empty variant is playable: steps=%v ok=%v", steps, ok)
	}
	none := &TranscriptRecord{Variants: map[string][]DialogueStep{}, VariantOrder: nil}
	if _, ok := none.PickVariant(); ok {
		t.Fatal("record with nothing must be unplayable")
	}
	if _, ok := (*TranscriptRecord)(nil).PickVariant(); ok {
		t.Fatal("nil record must be unplayable")
	}
}

func TestDialogueAliases(t *testing.T) {
	dir := dialogueFiles(t, map[string]string{
		"npc-dialogues.json": `{
			"Hops (Biohazard)": {"steps": [{"npc": "Hi"}]},
			"Hops (Other)": {"steps": [{"npc": "Hi"}]},
			"Silent (Quest)": {"variants": {"overhead-shout": [{"npc": "Hi", "presentation": "overhead"}]}},
			"Plain": {"steps": []}
		}`,
	})
	data := mustLoadDialogue(t, dir, DialogueLoadOptions{})
	if got := data.Aliases["Hops"]; got != "Hops (Biohazard)" {
		t.Fatalf("Hops alias = %q (first usable key wins)", got)
	}
	if _, exists := data.Aliases["Silent"]; exists {
		t.Fatal("unplayable transcript must not create an alias")
	}
	if _, exists := data.Aliases["Plain"]; exists {
		t.Fatal("bare key must not alias itself")
	}
}

func TestSpeakerIDIndex(t *testing.T) {
	dir := dialogueFiles(t, map[string]string{
		"npc-dialogues.json": `{}`,
		"npc-dialogue-index.json": `{
			"12": [{"page": "Transcript:Hudo", "variants": []}],
			"9": [{"page": "Transcript:Hudo", "variants": []}],
			"20": [{"page": "Transcript:Shared Page", "variants": []}]
		}`,
	})
	names := map[int]string{12: "Hudo", 9: "Hudo", 20: "null"}
	data := mustLoadDialogue(t, dir, DialogueLoadOptions{NPCName: func(id int) string { return names[id] }})
	// Cache-name pass: ascending id order, first wins -> 9.
	if got := data.SpeakerIDByName["Hudo"]; got != 9 {
		t.Fatalf("Hudo speaker id = %d, want 9 (first in ascending-id order)", got)
	}
	if _, exists := data.SpeakerIDByName["null"]; exists {
		t.Fatal(`"null" names must be skipped`)
	}
	// Page-name pass: "Shared Page" maps to 20; "Hudo" keeps the
	// cache-name winner (pass 1 ran first).
	if got := data.SpeakerIDByName["Shared Page"]; got != 20 {
		t.Fatalf("Shared Page speaker id = %d", got)
	}
	// Without a name resolver the page-name pass still runs.
	data2 := mustLoadDialogue(t, dir, DialogueLoadOptions{})
	if got := data2.SpeakerIDByName["Hudo"]; got != 9 {
		t.Fatalf("no-resolver Hudo speaker id = %d, want 9 (first indexed id)", got)
	}
}

func TestDialogueStepConversion(t *testing.T) {
	dir := dialogueFiles(t, map[string]string{
		"npc-dialogues.json": `{"Page": {"steps": [
			{"npc": "Hello.", "id": "s1"},
			{"player": "", "id": "s2"},
			{"player": 42},
			{"type": "line", "text": "Typed.", "speaker": "Bob", "presentation": "overhead", "expression": 605, "side": "right"},
			{"type": "line", "text": 7, "speaker": 9},
			{"type": "choice", "prompt": "Pick", "options": [
				{"text": "A", "steps": [{"npc": "A!"}], "condition": "If X:", "condition_id": "c1", "id": "o1", "hook": "quest:x", "quest": "X"},
				{"text": "B"},
				42
			]},
			{"type": "condition", "text": "If X:", "id": "cond1", "steps": [{"npc": "Yes"}]},
			{"type": "jump", "reference": "above", "steps": [{"npc": "Inline"}]},
			{"type": "call", "branch": "intro"},
			{"type": "action", "action": "open_shop", "target": "Shop", "shops": ["A (Default)", "B", 3]},
			{"type": "action", "action": "open_shop", "target": "Shop", "shops": []},
			{"type": "message", "text": "Msg", "target": "T"},
			{"type": "random", "options": [{"text": "R1", "steps": []}]},
			{"type": "end"},
			{"type": "missing"},
			42,
			null
		], "branches": {"intro": [{"npc": "Branch"}], "bad": "nope"}}}`,
	})
	record := mustLoadDialogue(t, dir, DialogueLoadOptions{}).Record("Page")
	steps := record.Steps
	if len(steps) != 17 {
		t.Fatalf("steps = %d", len(steps))
	}
	if !steps[0].HasNPC || steps[0].NPCText != "Hello." || steps[0].ID != "s1" {
		t.Fatalf("step 0 = %+v", steps[0])
	}
	// An explicitly empty player string still counts as speech.
	if !steps[1].HasPlayer || steps[1].PlayerText != "" {
		t.Fatalf("step 1 = %+v (empty string counts as speech)", steps[1])
	}
	// A non-string player field reads as absent.
	if steps[2].HasPlayer {
		t.Fatalf("step 2 = %+v (non-string player must be absent)", steps[2])
	}
	typed := steps[3]
	if !typed.HasText || typed.Text != "Typed." || typed.Speaker != "Bob" || typed.Presentation != "overhead" ||
		!typed.HasExpression || typed.Expression != 605 || typed.Side != "right" {
		t.Fatalf("typed step = %+v", typed)
	}
	if steps[4].HasText || steps[4].Speaker != "" {
		t.Fatalf("non-string text/speaker must read absent: %+v", steps[4])
	}
	choice := steps[5]
	if choice.Type != "choice" || choice.Prompt != "Pick" || len(choice.Options) != 3 {
		t.Fatalf("choice = %+v", choice)
	}
	opt := choice.Options[0]
	if opt.Text != "A" || len(opt.Steps) != 1 || opt.Condition != "If X:" || opt.ConditionID != "c1" ||
		opt.ID != "o1" || opt.Hook != "quest:x" || opt.Quest != "X" {
		t.Fatalf("option 0 = %+v", opt)
	}
	if choice.Options[1].Steps != nil || choice.Options[2].Text != "" {
		t.Fatalf("options 1/2 = %+v %+v", choice.Options[1], choice.Options[2])
	}
	if steps[6].Type != "condition" || !steps[6].HasSteps || len(steps[6].Steps) != 1 {
		t.Fatalf("condition = %+v", steps[6])
	}
	if steps[7].Reference != "above" || !steps[7].HasSteps {
		t.Fatalf("jump with inline body = %+v", steps[7])
	}
	shop := steps[9]
	if !shop.HasShops || len(shop.Shops) != 2 || shop.Shops[0] != "A (Default)" {
		t.Fatalf("shop versions = %+v (non-string element dropped)", shop)
	}
	if !steps[10].HasShops || len(steps[10].Shops) != 0 {
		t.Fatalf("empty shops array must still count as present: %+v", steps[10])
	}
	if steps[14].Type != "missing" {
		t.Fatalf("unknown step type must survive conversion: %+v", steps[14])
	}
	// Non-object steps become empty steps.
	if steps[15].Type != "" || steps[15].HasNPC || steps[15].HasPlayer || steps[15].HasText ||
		steps[16].Type != "" || steps[16].HasNPC || steps[16].HasPlayer || steps[16].HasText {
		t.Fatalf("non-object steps = %+v %+v", steps[15], steps[16])
	}
	if len(record.Branches) != 1 || len(record.Branches["intro"]) != 1 {
		t.Fatalf("branches = %+v (non-array branch dropped)", record.Branches)
	}
}

func TestDialogueJSONOrderAndDuplicates(t *testing.T) {
	value, err := decodeDialogueJSON([]byte(`{"b": 1, "a": 2, "b": 3} trailing`))
	if err == nil {
		t.Fatalf("trailing data must fail: %v", value)
	}
	value, err = decodeDialogueJSON([]byte(`{"b": 1, "a": 2, "b": 3}`))
	if err != nil {
		t.Fatal(err)
	}
	obj := value.(*dialogueObject)
	if len(obj.keys) != 2 || obj.keys[0] != "b" || obj.keys[1] != "a" {
		t.Fatalf("keys = %v (duplicate keeps first position)", obj.keys)
	}
	if num, ok := dialogueJSONNumber(obj.vals["b"]); !ok || num != 3 {
		t.Fatalf("duplicate b = %v, want last value 3", obj.vals["b"])
	}
}

func TestDialogueDeepEqual(t *testing.T) {
	a, err1 := decodeDialogueJSON([]byte(`{"x": [1, 2.0, {"k": "v"}], "y": null}`))
	b, err2 := decodeDialogueJSON([]byte(`{"y": null, "x": [1, 2, {"k": "v"}]}`))
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	if !dialogueDeepEqual(a, b) {
		t.Fatal("key order and 2.0 vs 2 must not matter")
	}
	c, _ := decodeDialogueJSON([]byte(`{"x": [2, 1, {"k": "v"}], "y": null}`))
	if dialogueDeepEqual(a, c) {
		t.Fatal("array order must matter")
	}
	d, _ := decodeDialogueJSON([]byte(`{"x": [1, "2", {"k": "v"}], "y": null}`))
	if dialogueDeepEqual(a, d) {
		t.Fatal("number vs string must differ")
	}
}

func TestParseRetiredNPCSuccessors(t *testing.T) {
	mapping, err := ParseRetiredNPCSuccessors([]byte(`{
		"$comment": "test fixture",
		"successors": {
			"3216": {"id": 16821, "name": "Captain Harlan"},
			"3220": {"id": 16807, "name": "Cadmus"}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if mapping[16821] != 3216 || mapping[16807] != 3220 || len(mapping) != 2 {
		t.Fatalf("mapping = %v (successor -> retired)", mapping)
	}
	for _, bad := range []string{
		`{oops`, `[]`, `{}`, `{"successors": []}`,
		`{"successors": {"abc": {"id": 1}}}`,
		`{"successors": {"1": {"id": "x"}}}`,
		`{"successors": {"1": {}}}`,
		`{"successors": {"1": null}}`,
	} {
		if _, err := ParseRetiredNPCSuccessors([]byte(bad)); err == nil {
			t.Fatalf("input %s must fail", bad)
		}
	}
}

func TestDialogueFilesValid(t *testing.T) {
	// The shipped dialogue files must load cleanly. The
	// directory is absent in a bare-module checkout; skip there
	// rather than failing an unrelated tree.
	dir := filepath.Join("..", "..", "data", "definitions")
	if _, err := os.Stat(filepath.Join(dir, "npc-dialogues.json")); err != nil {
		t.Skipf("definitions dir absent: %v", err)
	}
	data, err := LoadDialogueData(dir, DialogueLoadOptions{})
	if err != nil {
		t.Fatalf("dialogue files must load: %v", err)
	}
	// The verbatim upstream import (Tyler-approved 2026-11-21,
	// SHA-256 f50b9906…) holds 3,374 speaker records; the
	// dialogue-index file is still the empty starter, so the
	// index stays empty until that file is imported too.
	if len(data.Records) != 3374 || len(data.Index) != 0 {
		t.Fatalf("dialogue catalogue = %d records, %d index ids, want 3374 and 0",
			len(data.Records), len(data.Index))
	}
	if data.Record("'Beedy-eye' Jones") == nil {
		t.Fatalf("expected a record for 'Beedy-eye' Jones")
	}
}

package definition

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Dialogue data loading: the transcript catalogue NpcDialogues reads in
// its register() (plugins/npcs/NpcDialogues.plugin.js:1210-1250).
//
// TS reads three files from GameConstants.DEFINITIONS_DIRECTORY, lazily
// on the first Talk-to (the 17 MiB dump expands to ~35 MiB of objects)
// and caches the result. This package loads eagerly when called; the
// coordinator decides when to call LoadDialogueData (at boot or on
// first Talk-to). The file semantics below are TS-exact:
//
//   - npc-dialogues.json: REQUIRED. A missing or malformed file fails
//     the load (readFileSync/JSON.parse throw), as does a top-level
//     value that is not a keyed object.
//   - event-dialogues.json: OPTIONAL when missing (ENOENT yields {}),
//     but a present-but-malformed file fails the load, as does a
//     top-level non-object. Every event record must carry
//     activation "explicit", and a record sharing a name with the
//     ordinary catalogue must be deep-equal to it. Event records
//     merge over the ordinary catalogue.
//   - npc-dialogue-index.json: ANY failure yields an empty index.
//     TS wraps the read+parse in a bare try/catch that discards the
//     error, so a missing file and a malformed file behave the
//     same. The Go port reproduces that asymmetry deliberately.
//   - Merged-catalogue validation (over ordinary + event records):
//     activation, when present, must be "explicit"; steps, when
//     present, must be an array; a non-null default must name an
//     existing variant key.
//
// dialogue-types.json is NOT loaded here: its only consumer is the
// pets content (plugins/npcs/pets/Dialogue.Pets.js:10), which reads
// it to classify transcript pages for pet conversations. It is
// pet-content data, not dialogue-engine data; see the Phase 14
// report. retired-npcs.json is likewise NOT read from the
// definitions directory: it lives with the plugin at
// plugins/npcs/data/retired-npcs.json (plugin-private data, loaded
// relative to the plugin file) and is never copied into this
// repository. The successor mapping it carries reaches playback
// through DialogueData.Predecessors, which the coordinator fills;
// ParseRetiredNPCSuccessors parses the file's shape when the
// coordinator has an approved copy to read.
//
// Step contents are NOT validated at load: TS validates only the
// record shape above and reads step fields loosely at playback.
// The converters below mirror that looseness (non-string fields
// read as absent, non-object steps become empty steps) so a
// malformed step can never fail the load or panic playback; at
// playback an empty step falls through to the "unavailable" path,
// where TS would throw a TypeError reading its fields — the Go
// port stops safely instead of crashing the tick.
//
// JSON key order is preserved (see dialogueObject): aliasKeys and
// speakerIdIndex first-wins rules depend on Object.keys order, and
// variant selection depends on variant key order. encoding/json
// maps would randomize both.

// DialogueStep is one transcript step: the JSON objects inside a
// record's steps/variants/branches arrays and inside choice/random
// options (NpcDialogues.plugin.js, passim). Fields are the union of
// every key the playback engine reads; keys the engine never reads
// (action_source, section, resolved, page_available, reason,
// variant) are dropped at conversion.
//
// Presence matters for speech: TS tests `typeof step.player ===
// "string"` and `typeof step.npc === "string"`, so an explicitly
// empty string still counts as a speech line, while an absent or
// non-string field does not. HasPlayer/HasNPC/HasText carry that
// distinction. The same applies to Steps (a step's nested body,
// which the playback engine appends to the continuation) and
// Shops (an open_shop step's version list, whose mere presence as
// an array selects the version-picking path).
type DialogueStep struct {
	// Type is step.type: "line", "choice", "random", "condition",
	// "jump", "message", "action", "end", "call", "unavailable",
	// "reference", or "" for bare speech steps ({npc: "..."} /
	// {player: "..."}) and untyped steps. Unknown types
	// ("missing", "input", anything else) reach playback's
	// unknown-step fallback.
	Type string
	// Text is step.text for typed lines, conditions, messages and
	// actions. HasText reports a string value was present.
	Text    string
	HasText bool
	// Speaker is step.speaker for typed lines ("" when absent or
	// non-string; TS reads String(step.speaker ?? "")). HasSpeaker
	// reports a string value was present: playback must tell an
	// absent speaker (undefined -> the definition name resolves)
	// from an explicit empty one ("" wins over the definition
	// name, TS ?? semantics), so presence is carried separately.
	Speaker    string
	HasSpeaker bool
	// PlayerText/HasPlayer and NPCText/HasNPC are the bare-speech
	// forms step.player / step.npc.
	PlayerText string
	HasPlayer  bool
	NPCText    string
	HasNPC     bool
	// Expression is step.expression when it is a JSON number;
	// playback applies the TS integer-and->=0 rule. No record in
	// the shipped dump sets it (verified 2026-11-15); it exists
	// for the request.expression override path and hand-written
	// transcripts.
	Expression    float64
	HasExpression bool
	// Side is step.side ("right" selects chat group 217, anything
	// else 231). No record in the shipped dump sets it.
	Side string
	// Presentation is step.presentation ("overhead" for overhead
	// lines, anything else for chatbox lines).
	Presentation string
	// Hook/Quest carry a quest slug on action steps and options
	// ("quest:cook-s-assistant:start"); playback emits
	// npc-dialogue:hook for them.
	Hook  string
	Quest string
	// Action/Target are step.action / step.target for action
	// steps (open_shop, slayer_assignment, slayer_task_tip,
	// receive, teleport, ...) and message targets.
	Action string
	Target string
	// Reference is step.reference for jump steps ("above",
	// "below", "other", "previous2", "initial", "continue...").
	Reference string
	// Branch is step.branch for call steps (a key into the
	// record's branches map).
	Branch string
	// Condition/ConditionID are an option's guard
	// (option.condition / option.condition_id). Condition steps
	// carry their prose in Text, not here.
	Condition   string
	ConditionID string
	// ID is step.id / option.id: the anchor id emitted with
	// npc-dialogue:choice/condition/action events.
	ID string
	// Prompt is a choice step's prompt ("Select an option" when
	// absent at playback).
	Prompt string
	// Shops is an open_shop step's version list. HasShops
	// reports the field was present as an array (even an empty
	// one); only string elements are kept, matching the TS
	// .includes/.endsWith matching (non-strings never match).
	Shops    []string
	HasShops bool
	// Steps is a step's nested body (condition branches, jump
	// inline targets, line continuations). HasSteps reports the
	// field was present as an array.
	Steps    []DialogueStep
	HasSteps bool
	// Options is a choice/random step's option list.
	Options []DialogueOptionStep
}

// DialogueOptionStep is one option of a choice/random step: its
// label, its branch, and its optional guard and quest hook.
type DialogueOptionStep struct {
	Text string
	// Steps is the option's branch ([] when absent or non-array;
	// TS reads option.steps || [] at every use).
	Steps []DialogueStep
	// Condition/ConditionID guard the option; playback hides or
	// re-checks it only when a condition handler answers false.
	Condition   string
	ConditionID string
	ID          string
	Hook        string
	Quest       string
}

// TranscriptRecord is one transcript page: the value keyed by page
// name in npc-dialogues.json / event-dialogues.json
// (NpcDialogues.plugin.js:1210-1250, pickVariant at :219-231).
type TranscriptRecord struct {
	// Name is the catalogue key (the wiki page title without the
	// "Transcript:" prefix, e.g. "Hudo").
	Name string
	// Activation is "" when absent, else "explicit": the only two
	// values LoadDialogueData accepts.
	Activation string
	// Steps is the flat transcript body. HasSteps reports the
	// steps field was present as an array (an empty array still
	// counts: pickVariant returns it and playback plays nothing).
	Steps    []DialogueStep
	HasSteps bool
	// Variants holds the array-valued variants only. A variant
	// key whose value is not an array is unplayable (TS
	// pickVariant's Array.isArray check) but still occupies its
	// VariantOrder slot and still counts for default-key
	// validation. HasVariants reports the variants field was
	// present as an object (even an empty one): resolveTranscript
	// branches on the truthiness of record.variants, and an empty
	// object is truthy in JS.
	Variants map[string][]DialogueStep
	// HasVariants reports the variants field was present as an
	// object, including an empty one.
	HasVariants bool
	// VariantOrder is every variant key in file order, including
	// non-array ones: pickVariant's standard/non-overhead search
	// and collectMenuBefore's family walk depend on it.
	VariantOrder []string
	// Default is the default variant key. HasDefault reports a
	// non-null default was present (TS: npc.default != null).
	Default    string
	HasDefault bool
	// Branches holds the array-valued named branches call steps
	// resolve against (TS requires Array.isArray at the call
	// site; non-array branches are unusable and dropped).
	Branches map[string][]DialogueStep
}

// PickVariant ports pickVariant (NpcDialogues.plugin.js:219-231):
// explicit-activation records never play by default; flat records
// play their steps; otherwise the named default wins when it
// names an existing variant key (even a non-array one, which
// yields unplayable with NO fallback to the standard search),
// else the first "standard*" variant, else the first
// non-"overhead*" variant. It returns false when no playable
// variant exists (only overhead shouts, or nothing at all). A
// true return may still carry an empty step list: an empty
// variant is playable and plays nothing.
func (r *TranscriptRecord) PickVariant() ([]DialogueStep, bool) {
	if r == nil || r.Activation == "explicit" {
		return nil, false
	}
	if r.HasSteps {
		return r.Steps, true
	}
	if len(r.VariantOrder) == 0 {
		return nil, false
	}
	key := ""
	if r.HasDefault && variantKeyExists(r.VariantOrder, r.Default) {
		key = r.Default
	} else {
		for _, name := range r.VariantOrder {
			if strings.HasPrefix(name, "standard") {
				key = name
				break
			}
		}
		if key == "" {
			for _, name := range r.VariantOrder {
				if !strings.HasPrefix(name, "overhead") {
					key = name
					break
				}
			}
		}
	}
	if key == "" {
		return nil, false
	}
	steps, ok := r.Variants[key]
	if !ok {
		return nil, false
	}
	return steps, true
}

func variantKeyExists(order []string, key string) bool {
	for _, name := range order {
		if name == key {
			return true
		}
	}
	return false
}

// DialogueIndexEntry is one row of npc-dialogue-index.json: the
// transcript page(s) an NPC id speaks, with the variant names each
// page offers. Page keeps the raw "Transcript:..." prefix exactly
// as the file holds it; PageName strips it the way resolveTranscript
// and speakerIdIndex do (String(entry.page).replace(/^Transcript:/,
// "")).
type DialogueIndexEntry struct {
	Page     string
	Variants []string
}

// PageName returns the catalogue key for this entry: Page with a
// leading "Transcript:" removed (once, prefix only).
func (e DialogueIndexEntry) PageName() string {
	return strings.TrimPrefix(e.Page, "Transcript:")
}

// DialogueData is the loaded dialogue catalogue: the merged
// transcript records, the name aliases, the NPC-id index, and the
// speaker-name index, as NpcDialogues' load() builds them
// (NpcDialogues.plugin.js:1214-1250). It is safe for concurrent
// reads after LoadDialogueData returns; callers must not mutate
// the maps.
type DialogueData struct {
	// Records maps catalogue keys to transcript pages.
	Records map[string]*TranscriptRecord
	// RecordOrder is the catalogue keys in load order (ordinary
	// file order, then event-only names appended): the JS
	// {...data, ...events} spread order, which aliasKeys depends
	// on.
	RecordOrder []string
	// Aliases maps a bare NPC name to its disambiguated
	// transcript key ("Hops" -> "Hops (Biohazard)"), first
	// usable key wins (aliasKeys).
	Aliases map[string]string
	// Index maps NPC ids to their transcript pages.
	Index map[int][]DialogueIndexEntry
	// SpeakerIDByName maps a speaker name (an NPC name or a
	// transcript page name) to an NPC id that speaks it, so
	// cutscene/paired speakers get their own chathead
	// (speakerIdIndex). First wins, cache names before page
	// names.
	SpeakerIDByName map[string]int
	// Predecessors maps a successor NPC id to the retired NPC id
	// whose transcript it plays until it has its own (the
	// PREDECESSOR map, built from plugins/npcs/data/retired-npcs.json
	// at NpcDialogues.plugin.js:22-24). It is EMPTY unless the
	// coordinator fills it: the source file is plugin-private
	// data that is never copied into this repository, and the
	// mapping must not be hand-recreated here. See
	// ParseRetiredNPCSuccessors.
	Predecessors map[int]int
	// EventNames lists the names merged from event-dialogues.json,
	// in file order, for boot reporting.
	EventNames []string
}

// Record returns the transcript page for name, or nil.
func (d *DialogueData) Record(name string) *TranscriptRecord {
	if d == nil {
		return nil
	}
	return d.Records[name]
}

// IndexEntries returns the index rows for an NPC id, or nil.
func (d *DialogueData) IndexEntries(id int) []DialogueIndexEntry {
	if d == nil {
		return nil
	}
	return d.Index[id]
}

// DialogueLoadOptions carries what LoadDialogueData needs beyond
// the definitions directory.
type DialogueLoadOptions struct {
	// NPCName resolves a cache NPC id to its definition name for
	// the speaker index's first pass (NpcDefinition.forId(id)?
	// .getName(), NpcDialogues.plugin.js:241-252). Nil skips the
	// cache-name pass entirely (the page-name pass still runs);
	// "" and "null" names are skipped, as in TS.
	NPCName func(id int) string
}

// LoadDialogueData loads the dialogue catalogue from dir (the
// definitions directory), following NpcDialogues' load() exactly
// (see the file header for the per-file required/optional table).
// It does not register with the definition source registry: TS
// reads these files directly with readFileSync, not through
// DefinitionLoader, and no DefinitionLoader source type exists
// for them.
func LoadDialogueData(dir string, opts DialogueLoadOptions) (*DialogueData, error) {
	dialogueFile := filepath.Join(dir, "npc-dialogues.json")
	rawData, err := readDialogueJSON(dialogueFile)
	if err != nil {
		return nil, err
	}
	dataObj, ok := rawData.(*dialogueObject)
	if !ok {
		return nil, fmt.Errorf("%s: expected dialogues keyed by transcript name", dialogueFile)
	}

	eventFile := filepath.Join(dir, "event-dialogues.json")
	eventsObj, eventNames, err := loadEventDialogues(eventFile, dataObj)
	if err != nil {
		return nil, err
	}

	// Merge: {...data, ...events}. Existing keys keep their
	// original position; event-only keys append (JS spread
	// order). An event record sharing a name is deep-equal to
	// the ordinary record (checked in loadEventDialogues), so
	// the override changes nothing observable.
	merged := &dialogueObject{vals: make(map[string]any, len(dataObj.keys)+len(eventsObj.keys))}
	merged.keys = append(merged.keys, dataObj.keys...)
	for _, key := range dataObj.keys {
		merged.vals[key] = dataObj.vals[key]
	}
	for _, key := range eventsObj.keys {
		if _, exists := merged.vals[key]; !exists {
			merged.keys = append(merged.keys, key)
		}
		merged.vals[key] = eventsObj.vals[key]
	}

	records := make(map[string]*TranscriptRecord, len(merged.keys))
	order := make([]string, 0, len(merged.keys))
	for _, name := range merged.keys {
		record, err := validateAndConvertRecord(dialogueFile, name, merged.vals[name])
		if err != nil {
			return nil, err
		}
		record.Name = name
		records[name] = record
		order = append(order, name)
	}

	index := loadDialogueIndex(filepath.Join(dir, "npc-dialogue-index.json"))

	data := &DialogueData{
		Records:         records,
		RecordOrder:     order,
		Aliases:         computeDialogueAliases(records, order),
		Index:           index,
		SpeakerIDByName: computeSpeakerIDIndex(index, opts.NPCName),
		Predecessors:    make(map[int]int),
		EventNames:      eventNames,
	}
	return data, nil
}

// loadEventDialogues reads event-dialogues.json with TS's ENOENT
// tolerance: a missing file yields an empty object, any other
// read error or a parse error fails, and a non-object top-level
// value fails. It also enforces the per-record explicit
// activation and the no-conflict rule against the ordinary
// catalogue.
func loadEventDialogues(eventFile string, dataObj *dialogueObject) (*dialogueObject, []string, error) {
	raw, err := os.ReadFile(eventFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &dialogueObject{vals: make(map[string]any)}, nil, nil
		}
		return nil, nil, fmt.Errorf("reading %s: %w", eventFile, err)
	}
	value, err := decodeDialogueJSON(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", eventFile, err)
	}
	eventsObj, ok := value.(*dialogueObject)
	if !ok {
		return nil, nil, fmt.Errorf("%s: expected activity dialogues keyed by transcript name", eventFile)
	}
	for _, name := range eventsObj.keys {
		activation, _ := dialogueObjectStringField(eventsObj.vals[name], "activation")
		if activation != "explicit" {
			// TS: record?.activation !== "explicit" throws,
			// including for null and non-object records
			// (whose activation reads as undefined).
			return nil, nil, fmt.Errorf("%s: %s must have an explicit activation", eventFile, name)
		}
		if ordinary, exists := dataObj.vals[name]; exists && !dialogueDeepEqual(ordinary, eventsObj.vals[name]) {
			return nil, nil, fmt.Errorf("%s: %s conflicts with the ordinary dialogue catalogue", eventFile, name)
		}
	}
	return eventsObj, append([]string(nil), eventsObj.keys...), nil
}

// validateAndConvertRecord enforces the merged-catalogue
// validation loop (NpcDialogues.plugin.js:1241-1249) and converts
// the record. A JSON null record fails: TS reads npc.activation
// off it and throws a TypeError. Other non-object records pass
// validation exactly as TS passes them (every field reads as
// undefined) and convert to an empty, unplayable record.
func validateAndConvertRecord(dialogueFile, name string, raw any) (*TranscriptRecord, error) {
	if raw == nil {
		return nil, fmt.Errorf("%s: %s has an unsupported activation", dialogueFile, name)
	}
	obj, isObj := raw.(*dialogueObject)
	if !isObj {
		return &TranscriptRecord{Variants: make(map[string][]DialogueStep), Branches: make(map[string][]DialogueStep)}, nil
	}
	if activation, present := dialogueObjectField(obj, "activation"); present {
		if text, ok := activation.(string); !ok || text != "explicit" {
			return nil, fmt.Errorf("%s: %s has an unsupported activation", dialogueFile, name)
		}
	}
	if steps, present := dialogueObjectField(obj, "steps"); present {
		if _, ok := steps.([]any); !ok {
			return nil, fmt.Errorf("%s: %s has non-array steps", dialogueFile, name)
		}
	}
	record := convertTranscriptRecord(obj)
	if def, present := dialogueObjectField(obj, "default"); present && def != nil {
		// TS: npc.default != null && !Object.hasOwn(npc.variants
		// ?? {}, npc.default). The key is coerced to a string
		// property key (dialogueJSString); the variant set is
		// the raw key set, including non-array variants.
		key := dialogueJSString(def)
		if !variantKeyExists(record.VariantOrder, key) {
			return nil, fmt.Errorf("%s: %s names a missing default variant", dialogueFile, name)
		}
		record.Default = key
		record.HasDefault = true
	}
	return record, nil
}

// convertTranscriptRecord converts a validated record object.
// Non-array variant/branch values are kept out of the playable
// maps but variant keys are preserved in VariantOrder (see
// TranscriptRecord).
func convertTranscriptRecord(obj *dialogueObject) *TranscriptRecord {
	record := &TranscriptRecord{
		Variants: make(map[string][]DialogueStep),
		Branches: make(map[string][]DialogueStep),
	}
	if activation, present := dialogueObjectField(obj, "activation"); present {
		if text, ok := activation.(string); ok {
			record.Activation = text
		}
	}
	if steps, present := dialogueObjectField(obj, "steps"); present {
		if arr, ok := steps.([]any); ok {
			record.Steps = convertDialogueSteps(arr)
			record.HasSteps = true
		}
	}
	if variants, present := dialogueObjectField(obj, "variants"); present {
		if variantsObj, ok := variants.(*dialogueObject); ok {
			record.HasVariants = true
			for _, key := range variantsObj.keys {
				record.VariantOrder = append(record.VariantOrder, key)
				if arr, ok := variantsObj.vals[key].([]any); ok {
					record.Variants[key] = convertDialogueSteps(arr)
				}
			}
		}
	}
	if branches, present := dialogueObjectField(obj, "branches"); present {
		if branchesObj, ok := branches.(*dialogueObject); ok {
			for _, key := range branchesObj.keys {
				if arr, ok := branchesObj.vals[key].([]any); ok {
					record.Branches[key] = convertDialogueSteps(arr)
				}
			}
		}
	}
	return record
}

// convertDialogueSteps converts a JSON array of steps. Non-object
// elements become empty steps (see the file header).
func convertDialogueSteps(arr []any) []DialogueStep {
	steps := make([]DialogueStep, 0, len(arr))
	for _, raw := range arr {
		steps = append(steps, convertDialogueStep(raw))
	}
	return steps
}

// convertDialogueStep converts one step object. Every field read
// follows the TS playback reads: string fields require a string
// value (a number/boolean/object reads as absent, matching the
// typeof checks and ?? "" fallbacks), Steps/Options/Shops require
// arrays, Expression requires a JSON number.
func convertDialogueStep(raw any) DialogueStep {
	obj, ok := raw.(*dialogueObject)
	if !ok {
		return DialogueStep{}
	}
	var step DialogueStep
	step.Type = dialogueObjectString(obj, "type")
	if text, present := dialogueObjectStringPresent(obj, "text"); present {
		step.Text, step.HasText = text, true
	}
	if speaker, present := dialogueObjectStringPresent(obj, "speaker"); present {
		step.Speaker, step.HasSpeaker = speaker, true
	}
	if text, present := dialogueObjectStringPresent(obj, "player"); present {
		step.PlayerText, step.HasPlayer = text, true
	}
	if text, present := dialogueObjectStringPresent(obj, "npc"); present {
		step.NPCText, step.HasNPC = text, true
	}
	if num, present := dialogueObjectField(obj, "expression"); present {
		if n, ok := dialogueJSONNumber(num); ok {
			step.Expression, step.HasExpression = n, true
		}
	}
	step.Side = dialogueObjectString(obj, "side")
	step.Presentation = dialogueObjectString(obj, "presentation")
	step.Hook = dialogueObjectString(obj, "hook")
	step.Quest = dialogueObjectString(obj, "quest")
	step.Action = dialogueObjectString(obj, "action")
	step.Target = dialogueObjectString(obj, "target")
	step.Reference = dialogueObjectString(obj, "reference")
	step.Branch = dialogueObjectString(obj, "branch")
	step.Condition = dialogueObjectString(obj, "condition")
	step.ConditionID = dialogueObjectString(obj, "condition_id")
	step.ID = dialogueObjectString(obj, "id")
	step.Prompt = dialogueObjectString(obj, "prompt")
	if shops, present := dialogueObjectField(obj, "shops"); present {
		if arr, ok := shops.([]any); ok {
			step.HasShops = true
			for _, elem := range arr {
				if text, ok := elem.(string); ok {
					step.Shops = append(step.Shops, text)
				}
			}
		}
	}
	if steps, present := dialogueObjectField(obj, "steps"); present {
		if arr, ok := steps.([]any); ok {
			step.Steps = convertDialogueSteps(arr)
			step.HasSteps = true
		}
	}
	if options, present := dialogueObjectField(obj, "options"); present {
		if arr, ok := options.([]any); ok {
			for _, rawOption := range arr {
				step.Options = append(step.Options, convertDialogueOption(rawOption))
			}
		}
	}
	return step
}

// convertDialogueOption converts one choice/random option. A
// non-object option becomes an empty option (its branch is empty,
// so playback continues past it, as TS does for an option whose
// steps read as []).
func convertDialogueOption(raw any) DialogueOptionStep {
	obj, ok := raw.(*dialogueObject)
	if !ok {
		return DialogueOptionStep{}
	}
	var option DialogueOptionStep
	option.Text = dialogueObjectString(obj, "text")
	if steps, present := dialogueObjectField(obj, "steps"); present {
		if arr, ok := steps.([]any); ok {
			option.Steps = convertDialogueSteps(arr)
		}
	}
	option.Condition = dialogueObjectString(obj, "condition")
	option.ConditionID = dialogueObjectString(obj, "condition_id")
	option.ID = dialogueObjectString(obj, "id")
	option.Hook = dialogueObjectString(obj, "hook")
	option.Quest = dialogueObjectString(obj, "quest")
	return option
}

// loadDialogueIndex reads npc-dialogue-index.json with TS's total
// failure tolerance: any read error, any parse error, or a
// non-object top-level value yields an empty index. Within a
// well-formed object, rows tolerate shape drift: a non-array
// value yields no entries for that id, a non-object row is
// skipped, a non-array variants field yields no variants, and a
// non-canonical key (one String(id) can never produce) is
// skipped. TS would throw on some of these shapes at playback;
// the Go port degrades to "no transcript reaches it" instead of
// crashing the tick.
func loadDialogueIndex(indexFile string) map[int][]DialogueIndexEntry {
	index := make(map[int][]DialogueIndexEntry)
	raw, err := os.ReadFile(indexFile)
	if err != nil {
		return index
	}
	value, err := decodeDialogueJSON(raw)
	if err != nil {
		return index
	}
	obj, ok := value.(*dialogueObject)
	if !ok {
		return index
	}
	for _, key := range obj.keys {
		id, err := strconv.Atoi(key)
		if err != nil || strconv.Itoa(id) != key {
			continue
		}
		rows, ok := obj.vals[key].([]any)
		if !ok {
			continue
		}
		// An empty array is still an array: resolveTranscript's
		// .find(Array.isArray) stops at it, so it must be
		// stored (as an empty entry list), not skipped.
		entries := []DialogueIndexEntry{}
		for _, rawRow := range rows {
			row, ok := rawRow.(*dialogueObject)
			if !ok {
				continue
			}
			var entry DialogueIndexEntry
			// TS: String(entry.page ?? ""). Null/absent reads
			// as ""; other values stringify (dialogueJSString:
			// the shared jsString helper does not know
			// json.Number, so numbers take their own branch).
			if page, present := dialogueObjectField(row, "page"); present && page != nil {
				entry.Page = dialogueJSString(page)
			}
			if variants, present := dialogueObjectField(row, "variants"); present {
				if arr, ok := variants.([]any); ok {
					for _, elem := range arr {
						if text, ok := elem.(string); ok {
							entry.Variants = append(entry.Variants, text)
						}
					}
				}
			}
			entries = append(entries, entry)
		}
		index[id] = entries
	}
	return index
}

var dialogueAliasSuffix = regexp.MustCompile(` \(.+\)$`)

// computeDialogueAliases ports aliasKeys
// (NpcDialogues.plugin.js:255-262): for each catalogue key with a
// trailing " (...)" disambiguator, the bare base maps to the first
// key (in catalogue order) whose transcript has a playable
// variant.
func computeDialogueAliases(records map[string]*TranscriptRecord, order []string) map[string]string {
	aliases := make(map[string]string)
	for _, key := range order {
		base := dialogueAliasSuffix.ReplaceAllString(key, "")
		if base == key {
			continue
		}
		if _, exists := aliases[base]; exists {
			continue
		}
		if _, playable := records[key].PickVariant(); playable {
			aliases[base] = key
		}
	}
	return aliases
}

// computeSpeakerIDIndex ports speakerIdIndex
// (NpcDialogues.plugin.js:241-252). Pass 1 maps cache NPC names to
// ids (first wins); pass 2 maps transcript page names to the
// first id indexed under them (first wins, cache names keep
// priority because pass 1 ran first). Iteration follows JS
// Object.keys order for integer-like keys: ascending numeric.
// (The file's key order is irrelevant here: JS reorders
// integer-like keys numerically before all others.)
func computeSpeakerIDIndex(index map[int][]DialogueIndexEntry, npcName func(int) string) map[string]int {
	byName := make(map[string]int)
	ids := make([]int, 0, len(index))
	for id := range index {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	if npcName != nil {
		for _, id := range ids {
			name := npcName(id)
			if name != "" && name != "null" {
				if _, exists := byName[name]; !exists {
					byName[name] = id
				}
			}
		}
	}
	for _, id := range ids {
		for _, entry := range index[id] {
			name := entry.PageName()
			if name != "" {
				if _, exists := byName[name]; !exists {
					byName[name] = id
				}
			}
		}
	}
	return byName
}

// ParseRetiredNPCSuccessors parses the shape of
// plugins/npcs/data/retired-npcs.json ({successors: {retiredId:
// {id: successorId, ...}}}) into the PREDECESSOR map
// (successor id -> retired id). It takes bytes, not a path: the
// file is plugin-private data, and only the coordinator may
// decide where an approved copy is read from. A missing or
// malformed successors map is an error, matching require().
// Non-numeric retired keys and non-numeric successor ids are
// errors: TS would key the map with NaN, which no lookup can
// ever match, so failing loudly beats a silently dead mapping.
func ParseRetiredNPCSuccessors(data []byte) (map[int]int, error) {
	value, err := decodeDialogueJSON(data)
	if err != nil {
		return nil, fmt.Errorf("parsing retired-npcs data: %w", err)
	}
	obj, ok := value.(*dialogueObject)
	if !ok {
		return nil, errors.New("retired-npcs data: expected an object")
	}
	successors, present := dialogueObjectField(obj, "successors")
	if !present {
		return nil, errors.New("retired-npcs data: missing successors")
	}
	successorsObj, ok := successors.(*dialogueObject)
	if !ok {
		return nil, errors.New("retired-npcs data: successors must be an object")
	}
	out := make(map[int]int, len(successorsObj.keys))
	for _, key := range successorsObj.keys {
		retired, err := strconv.Atoi(key)
		if err != nil {
			return nil, fmt.Errorf("retired-npcs data: non-numeric retired id %q", key)
		}
		entry, ok := successorsObj.vals[key].(*dialogueObject)
		if !ok {
			return nil, fmt.Errorf("retired-npcs data: successor for %d must be an object", retired)
		}
		idValue, present := dialogueObjectField(entry, "id")
		if !present {
			return nil, fmt.Errorf("retired-npcs data: successor for %d has no id", retired)
		}
		idNum, ok := dialogueJSONNumber(idValue)
		if !ok || !isJSInteger(idNum) {
			return nil, fmt.Errorf("retired-npcs data: successor id for %d must be an integer", retired)
		}
		out[jsTrunc(idNum)] = retired
	}
	return out, nil
}

// --- Order-preserving JSON decoding -------------------------------
//
// dialogueObject is a JSON object that remembers its key order:
// aliasKeys, speakerIdIndex, and variant selection all depend on
// Object.keys order, which encoding/json maps do not preserve.
// Values decode as nil, bool, string, json.Number, []any, or
// *dialogueObject.

// dialogueObject is a JSON object with insertion-ordered keys.
type dialogueObject struct {
	keys []string
	vals map[string]any
}

// dialogueObjectField returns a field and whether the key was
// present (a present null reads as (nil, true)).
func dialogueObjectField(obj *dialogueObject, key string) (any, bool) {
	if obj == nil {
		return nil, false
	}
	value, present := obj.vals[key]
	return value, present
}

// dialogueObjectString returns a field as a string, or "" when
// absent or non-string.
func dialogueObjectString(obj *dialogueObject, key string) string {
	text, _ := dialogueObjectStringPresent(obj, key)
	return text
}

// dialogueObjectStringPresent returns a field's string value and
// whether the field held a string.
func dialogueObjectStringPresent(obj *dialogueObject, key string) (string, bool) {
	value, present := dialogueObjectField(obj, key)
	if !present {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

// dialogueObjectStringField reads a string field off an arbitrary
// decoded value: "" when the value is not an object or the field
// is absent/non-string. Used where TS reads record?.activation.
func dialogueObjectStringField(value any, key string) (string, bool) {
	obj, ok := value.(*dialogueObject)
	if !ok {
		return "", false
	}
	return dialogueObjectStringPresent(obj, key)
}

// dialogueJSString is String(value) over decoded dialogue JSON:
// jsString plus the json.Number branch it lacks.
func dialogueJSString(value any) string {
	if num, ok := dialogueJSONNumber(value); ok {
		return jsFormatNumber(num)
	}
	return jsString(value, true)
}

// dialogueJSONNumber converts a decoded JSON number to float64.
// Only json.Number values count (numbers decode that way under
// UseNumber); strings never coerce.
func dialogueJSONNumber(value any) (float64, bool) {
	switch n := value.(type) {
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

// readDialogueJSON reads and parses one required JSON file. A
// missing or malformed file is an error, as readFileSync +
// JSON.parse throw in TypeScript.
func readDialogueJSON(path string) (any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	value, err := decodeDialogueJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return value, nil
}

// decodeDialogueJSON parses JSON into the order-preserving value
// shape. Trailing data after the top-level value is an error, as
// JSON.parse rejects it.
func decodeDialogueJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	value, err := decodeDialogueValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return nil, errors.New("unexpected trailing data")
		}
		return nil, err
	}
	return value, nil
}

// decodeDialogueValue reads one JSON value from the token stream.
func decodeDialogueValue(dec *json.Decoder) (any, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch tok := token.(type) {
	case json.Delim:
		switch tok {
		case '{':
			obj := &dialogueObject{vals: make(map[string]any)}
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, errors.New("object key must be a string")
				}
				value, err := decodeDialogueValue(dec)
				if err != nil {
					return nil, err
				}
				// Duplicate keys: the last value wins but the
				// key keeps its first position, as in JS
				// object literals and JSON.parse.
				if _, exists := obj.vals[key]; !exists {
					obj.keys = append(obj.keys, key)
				}
				obj.vals[key] = value
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return obj, nil
		case '[':
			var arr []any
			for dec.More() {
				value, err := decodeDialogueValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, value)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			if arr == nil {
				arr = []any{}
			}
			return arr, nil
		default:
			return nil, fmt.Errorf("unexpected delimiter %q", string(tok))
		}
	case string, bool, nil, json.Number, float64:
		return tok, nil
	default:
		return nil, fmt.Errorf("unexpected token %T", token)
	}
}

// dialogueDeepEqual ports isDeepStrictEqual over decoded dialogue
// JSON (the event/ordinary conflict check). Object key order is
// insignificant; array order is significant; numbers compare by
// numeric value (1 and 1.0 are the same JS number); NaN never
// occurs in JSON. Strictness over types holds: 1 != "1",
// null != undefined (undefined cannot occur).
func dialogueDeepEqual(a, b any) bool {
	switch av := a.(type) {
	case *dialogueObject:
		bv, ok := b.(*dialogueObject)
		if !ok || len(av.keys) != len(bv.keys) {
			return false
		}
		for _, key := range av.keys {
			bValue, present := bv.vals[key]
			if !present || !dialogueDeepEqual(av.vals[key], bValue) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !dialogueDeepEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case json.Number:
		bNum, ok := dialogueJSONNumber(b)
		if !ok {
			return false
		}
		aNum, _ := av.Float64()
		return aNum == bNum || (math.IsNaN(aNum) && math.IsNaN(bNum))
	default:
		return a == b
	}
}

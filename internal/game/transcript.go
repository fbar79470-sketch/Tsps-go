package game

import (
	"math"
	"math/rand/v2"
	"regexp"
	"strings"
	"sync"

	"tsps-go/internal/definition"
)

// Transcript playback: the NpcDialogues playback engine
// (server/plugins/npcs/NpcDialogues.plugin.js, commit bfd81e3) as an
// engine-adjacent built-in system. In TS it is a plugin, but
// migration-plan milestone 4 explicitly names its data loading +
// npc-dialogue:* events, so it ports here following the Phase 6
// NpcSpawns precedent: a built-in tolerated source, documented as
// such — NOT a content port. No quest, shop, or other plugin
// content is ported with it.
//
// How it fits together:
//   - internal/definition/dialogue.go loads the catalogue
//     (npc-dialogues.json, npc-dialogue-index.json,
//     event-dialogues.json) into definition.DialogueData.
//   - DialoguePlayback.ResolveTranscript selects the transcript
//     page/variant for an NPC (id index, predecessor fallback,
//     name/alias fallback, variant-hook override).
//   - DialoguePlayback.StartDialogue plays steps through Worker
//     A's DialogueManager: speech lines become NpcDialogue /
//     PlayerDialogue chains, menus go through the
//     TranscriptDeps.PresentMenu seam, conditions/jumps/random
//     resolve live as the chain advances.
//   - The npc-dialogue:* custom events and the
//     onNpcDialogueVariant / onNpcDialogueCondition hook families
//     reach content through TranscriptDeps: EmitCustomEvent (the
//     coordinator wires it to Hub.EmitCustomEvent) and the two
//     registries below, which Worker C wires to the hub families
//     (TODO markers name the family methods).
//
// WORKER A CONTRACT (verified against internal/game/dialogue_*.
// go, which landed while this file was written — no shim): Dialogue
// is an interface with Send(*Player) + GetIndex();
// DialogueChainBuilder has Add(Dialogue) + GetDialogues;
// DialogueManager has StartDialogues/StartDialog/Advance/
// HandleOption/HandleResume/Reset/IsActive plus GetCurrent and
// GetOwner; Player has GetDialogueManager. Entry constructors
// take only their required parameters, with the TS optional
// parameters as chainable setters: NewNpcDialogue(index, npcID,
// text) + WithSpeakerName/WithExpression/WithGroupID,
// NewPlayerDialogue(index, text) + WithExpression,
// NewStatementDialogue(index, text),
// NewItemStatementDialogue(index, itemID, text),
// NewDoubleItemStatementDialogue(index, first, second, text),
// NewActionDialogue(index, DialogueAction) +
// WithSelfAdvancing, NewEndDialogue(index). All constructor
// calls are confined to the new* helpers in
// transcript_steps.go, and the only manager call that starts a
// chain is runner.startChain. The manager's owner parameter
// carries explicit playback's owner token (TS Object.freeze({}));
// only PacketSender.closeChatboxFor / presentOwned have no Go
// equivalent (see runner.cancel).
//
// All playback state lives in the per-playback runner
// (transcript_steps.go) or on the DialoguePlayback instance
// (the once-per-name gap log). No globals hold player state.
// Playback runs on the tick goroutine: chains start from turn
// processing (Talk-to), menu callbacks fire from interface
// handling, and scheduled continuations fire from the tick
// scheduler the coordinator installs. The variant/condition
// registries are the only mutex-guarded state (registration at
// boot, emits on ticks).

// MaxTranscriptJumps is MAX_JUMPS
// (NpcDialogues.plugin.js:44): jumps followed in one conversation
// before it is cut off (a transcript that loops on itself).
const MaxTranscriptJumps = 100

// OverheadContinuationTicks is OVERHEAD_CONTINUATION_TICKS
// (NpcDialogues.plugin.js:46): explicit overhead lines wait this
// many game ticks before the next source step.
const OverheadContinuationTicks = 2

// PlaceholderLine is PLACEHOLDER_LINE
// (NpcDialogues.plugin.js:141): said by an NPC whose Talk-to found
// nothing to play.
const PlaceholderLine = "Sorry, i've nothing interesting to talk about yet"

// UnavailableMessage is the chat line unavailable() sends when a
// conversation had nothing to show (NpcDialogues.plugin.js:602).
const UnavailableMessage = "That conversation isn't available right now."

// SpecialNPCDialogues is SPECIAL_NPC_DIALOGUES
// (NpcDialogues.plugin.js:138): NPCs with executable plugin
// conversations, not an imported prose transcript. Talk-to falls
// through for them (StartTalkTo returns false).
var SpecialNPCDialogues = map[string]bool{
	"Skully":       true,
	"Estate agent": true,
	"Estate Agent": true,
	"Alwyn":        true,
}

// Chat-group ids used by playback. The CALM fallback expression
// is DialogueExpressionCalm (589, Worker A's DialogueExpression
// port); an unset or invalid expression resolves to it.
// NpcDialogue's group is 231 (chat_left), or 217 (chat_right)
// when the line's side is "right" (NpcDialogues.plugin.js:1033).
const (
	// NpcDialogueGroupLeft is NpcDialogue.GROUP_ID (231).
	NpcDialogueGroupLeft = 231
	// NpcDialogueGroupRight is the chat_right group (217) used
	// for side:"right" NPC lines.
	NpcDialogueGroupRight = 217
)

// TranscriptPageRef is one entry of the pages list carried through
// resolution and events: a transcript page name (without the
// "Transcript:" prefix) with the variant names it offers
// (PluginNpcDialogueContext.pages, PluginTypes.ts:207-215).
type TranscriptPageRef struct {
	Page     string
	Variants []string
}

// TranscriptEvent is the conversation being played: the Talk-to /
// npc-dialogue:start event shape (NpcDialogues.plugin.js:1255,
// 1333-1357), reduced to Go values. DefinitionName/DefinitionID
// carry the NPC definition's name/id (TS event.definition);
// NPCID 0 with no NPC means "no NPC" (an item transcript —
// startPage passes npcId null; handlers must check NPC, not
// NPCID, for presence, since 0 is also a real NPC id).
type TranscriptEvent struct {
	Player *Player
	NPC    *NPC
	// NPCID is the NPC definition id (event.npcId). 0 when no
	// NPC speaks (item transcripts).
	NPCID int
	// DefinitionName/DefinitionID are the NPC definition's name
	// and id ("" / 0 when there is no definition, as for item
	// transcripts).
	DefinitionName string
	DefinitionID   int
	// Variant is the requested variant name: the string form of
	// event.variant, or the .variant of the object form.
	Variant string
	// VariantPage is the requested page: the .page of the object
	// form of event.variant.
	VariantPage string
	// HasVariant reports event.variant was supplied at all
	// (string or object form).
	HasVariant bool
	// VariantIsObject reports the object form ({page, variant})
	// was supplied. It matters because TS uses event.variant as
	// a map key in the name fallback: an object keys as
	// "[object Object]" and never matches a variant.
	VariantIsObject bool
	// Explicit marks trusted custom-event activation
	// (npc-dialogue:start / startPage): it enables the
	// unindexed-page binding and the scoped-cancellation path
	// (the manager carries the owner token; only the scoped
	// closeChatboxFor close degenerates — see runner.cancel in
	// transcript_steps.go).
	Explicit bool
}

// TranscriptContext is the per-resolution context object
// (NpcDialogues.plugin.js:1290-1310): the pages list, the speaker
// index, and the page-level option/line/menu indexes resolveJump
// and the menu jumps consult. It is built by ResolveTranscript /
// StartPage and consumed by the runner; callers must not mutate
// it after handing it to StartDialogue.
type TranscriptContext struct {
	Pages           []TranscriptPageRef
	SpeakerIDByName map[string]int
	// PageOptions maps normalized option text to its branch
	// (collectPageOptions' byText); PageOptionList is the same
	// options in walk order with word sets, for near/similar
	// matching.
	PageOptions    map[string][]definition.DialogueStep
	PageOptionList []PageOptionEntry
	// PageLines maps normalized line text to the steps from that
	// line onward, across the page's variants
	// (collectPageLines).
	PageLines map[string][]definition.DialogueStep
	// PageMenuBefore is the menu "above" the played variant on
	// its page (collectMenuBefore), or nil. PageInitialMenu is
	// the same search with the initial-menu guards.
	PageMenuBefore  *definition.DialogueStep
	PageInitialMenu *definition.DialogueStep
}

// PageOptionEntry is one entry of collectPageOptions' list: the
// normalized option text, its significant words, and its branch.
type PageOptionEntry struct {
	Key   string
	Words map[string]bool
	Steps []definition.DialogueStep
}

// PageOptionIndex is CollectPageOptions' result: the byText map
// plus the walk-order list (NpcDialogues.plugin.js:322-338
// returns {byText, list}).
type PageOptionIndex struct {
	ByText map[string][]definition.DialogueStep
	List   []PageOptionEntry
}

// ResolvedTranscript is resolveTranscript's result
// (NpcDialogues.plugin.js:1255-1319). StepsPresent distinguishes
// a resolution that found steps (possibly an empty list — an
// empty variant plays nothing but still counts as "found" for
// the gap warning) from a gap-only result (Page set, no steps)
// and from nil (nothing reaches the NPC at all).
type ResolvedTranscript struct {
	Steps        []definition.DialogueStep
	StepsPresent bool
	Branches     map[string][]definition.DialogueStep
	Context      *TranscriptContext
	// Variant is the chosen variant name ("" when the default
	// variant played).
	Variant string
	// Page is the resolved transcript page name ("" when the
	// resolution came from the name fallback without a page, or
	// when nothing resolved).
	Page string
}

// MenuOption is one option of a presented menu: its label and the
// callback that runs when the player picks it. The callback
// signature mirrors MultiChatboxPrompt's option callbacks
// (player, optionIndex, optionText in TS); the Go callback closes
// over the player and the option, so it takes no arguments.
type MenuOption struct {
	Label  string
	Select func()
}

// MenuPresenter presents a multi-option chatbox prompt and
// reports whether it could (TS api.sendMultiChatboxPrompt ->
// MultiChatboxPrompt.showPrompt, which returns false when the
// prompt cannot be shown; playback treats false as unavailable).
// The production implementation is game.MultiChatboxPrompt
// (ShowPrompt), installed by the coordinator; a nil presenter
// behaves exactly like a presenter that returns false: every
// menu reaches the unavailable path. Menu callbacks may fire
// long after PresentMenu returns (when the player picks); the
// runner stays alive through the closures.
type MenuPresenter func(player *Player, prompt string, options []MenuOption) bool

// TranscriptShop is one shop found by name for an open_shop step:
// its id and its source ("core" for shops.json shops). The
// source matters only for the >1-name disambiguation (TS keeps
// the core shop when a name has several, Sellers.SkillCapes
// turning it into the skillcape copy).
type TranscriptShop struct {
	ID     int
	Source string
}

// TranscriptDeps carries every seam playback needs from the
// engine, the hub, and systems that do not exist yet in Go. Nil
// function fields degrade to the documented TS no-handler /
// no-system behavior; nothing panics on a nil seam.
//
//   - Data: the loaded catalogue. Nil behaves as an empty
//     catalogue (every Talk-to plays the placeholder line).
//   - EmitCustomEvent: the hub custom-event emit (coordinator
//     wires Hub.EmitCustomEvent). Nil means no subscribers: every
//     mutable payload keeps its defaults, exactly as a TS emit
//     with no handlers behaves.
//   - PresentMenu: the chatbox menu presenter (see
//     MenuPresenter). Nil means menus cannot be shown.
//   - CloseChatbox: sendInterfaceRemoval (Worker A / coordinator
//     seam). Nil drops the close.
//   - Schedule: a tick-delayed callback (TS CountdownTask via
//     TaskManager). Nil runs the callback immediately: overhead
//     lines and held boxes continue without their tick delay.
//     The coordinator must install a real tick scheduler for
//     correct overhead cadence; the immediate fallback keeps
//     conversations completable in tests.
//   - PageText: chatbox paging (TS ChatTextLayout.pages, with
//     the characterPages fallback). Nil uses CharacterPages: the
//     TS fallback when ChatTextLayout is absent, which is the
//     honest Go behavior until font-advance paging lands.
//   - NPCName: the NPC's current definition name for a player
//     (TS npcName: getCurrentDefinition(player)?.getName()).
//     Nil answers "" (TS with no definition).
//   - Random: uniform [0,n) for random steps. Nil uses
//     math/rand/v2.
//   - Logf: gap warnings (reportDialogueGap). Nil discards.
//   - VariantHandlers / ConditionHandlers: the
//     onNpcDialogueVariant / onNpcDialogueCondition registries
//     (Worker C seam — see below). Nil means no handlers: TS's
//     null answers, with the built-in combat/skill condition
//     fallbacks still applying.
//   - FindShops / OpenShop: the shop lookup + open seams for
//     open_shop steps. There is NO ShopManager in Go (milestone
//     7): nil FindShops finds no shops (the step falls through
//     to the next step, as a failed open does in TS), and the
//     npc-dialogue:shop version-picking event is still emitted.
//     A nil OpenShop never opens.
//   - SendHeldStatement: StatementDialogue.send(player, text,
//     false) for held statement boxes (no "Click here to
//     continue"). Nil drops the held send; the hold timer still
//     advances the chain.
type TranscriptDeps struct {
	Data              *definition.DialogueData
	EmitCustomEvent   func(name string, payload any)
	PresentMenu       MenuPresenter
	CloseChatbox      func(player *Player)
	Schedule          func(player *Player, ticks int, fn func())
	PageText          func(text string) []string
	NPCName           func(npc *NPC, player *Player) string
	Random            func(n int) int
	Logf              func(format string, args ...any)
	VariantHandlers   *NpcDialogueVariantRegistry
	ConditionHandlers *NpcDialogueConditionRegistry
	FindShops         func(name string) []TranscriptShop
	OpenShop          func(player *Player, shopID int) bool
	SendHeldStatement func(player *Player, text string)
}

// DialoguePlayback is the transcript engine instance: the seams
// plus the once-per-NPC-name gap log. One instance serves the
// whole server (TS keeps the loaded catalogue and the
// reportedDialogueGaps set at module scope inside the plugin's
// register closure); per-conversation state never lives here.
type DialoguePlayback struct {
	deps TranscriptDeps

	mu           sync.Mutex
	reportedGaps map[string]bool
}

// NewDialoguePlayback returns an engine instance over deps. Deps
// are copied; later mutation of the caller's struct does not
// affect the instance.
func NewDialoguePlayback(deps TranscriptDeps) *DialoguePlayback {
	return &DialoguePlayback{deps: deps, reportedGaps: make(map[string]bool)}
}

// emit runs one custom event through the installed emit seam, or
// nowhere when no seam is installed (TS emit with no handlers).
func (pb *DialoguePlayback) emit(name string, payload any) {
	if pb != nil && pb.deps.EmitCustomEvent != nil {
		pb.deps.EmitCustomEvent(name, payload)
	}
}

func (pb *DialoguePlayback) logf(format string, args ...any) {
	if pb != nil && pb.deps.Logf != nil {
		pb.deps.Logf(format, args...)
	}
}

func (pb *DialoguePlayback) npcName(npc *NPC, player *Player) string {
	if npc == nil || pb == nil || pb.deps.NPCName == nil {
		return ""
	}
	return pb.deps.NPCName(npc, player)
}

func (pb *DialoguePlayback) pageText(text string) []string {
	if pb != nil && pb.deps.PageText != nil {
		return pb.deps.PageText(text)
	}
	return CharacterPages(text)
}

func (pb *DialoguePlayback) random(n int) int {
	if pb != nil && pb.deps.Random != nil {
		return pb.deps.Random(n)
	}
	return rand.IntN(n)
}

// --- Variant / condition hook registries (Worker C seam) --------
//
// TODO(Worker C): wire these registries to the hub families
// onNpcDialogueVariant / onNpcDialogueCondition
// (PluginTypes.ts:1091-1100, PluginManager.ts:1289-1324): hub
// registration methods must append handlers here (in
// registration order, first answer wins, panics isolated with a
// warning), and the hub table counts in internal/plugins/hooks.go
// must move both families from "no registration method" to
// ported. Until then, content registers directly on these
// registries and the default behavior (no handlers) is exactly
// TS's no-handler behavior: variant emit returns null (the
// default variant plays), condition emit returns null (the
// built-in combat/skill fallbacks run, then the first-unknown
// branch wins).
//
// game must never import plugins, so the registries live here
// and the coordinator/hub adapter calls Register/Emit across
// the seam, following the Phase 13 object-adapter precedent.

// NpcDialogueVariantEvent is the PluginNpcDialogueContext payload
// (PluginTypes.ts:207-215): who is talking, which NPC, and the
// transcript pages registered for the NPC id.
type NpcDialogueVariantEvent struct {
	Player         *Player
	NPC            *NPC
	NPCID          int
	DefinitionName string
	DefinitionID   int
	Pages          []TranscriptPageRef
}

// NpcDialogueVariantChoice is a variant hook's answer: a variant
// name, optionally on a specific page (the string-or-{page,
// variant} return of onNpcDialogueVariant).
type NpcDialogueVariantChoice struct {
	Variant string
	Page    string
}

// NpcDialogueVariantHandler picks a transcript variant. The bool
// reports whether the handler returned a truthy answer (TS:
// `if (result)` — "" and null fall through, a {} object does
// not). Handlers that do not care about this NPC return false.
type NpcDialogueVariantHandler func(event NpcDialogueVariantEvent) (NpcDialogueVariantChoice, bool)

// NpcDialogueVariantRegistry holds the onNpcDialogueVariant
// handlers in registration order. Safe for concurrent use:
// registration happens at boot, emits on ticks.
type NpcDialogueVariantRegistry struct {
	mu       sync.RWMutex
	handlers []NpcDialogueVariantHandler
	logf     func(format string, args ...any)
}

// NewNpcDialogueVariantRegistry returns an empty registry. Logf
// receives the per-hook panic warnings (TS console.warn); nil
// discards them.
func NewNpcDialogueVariantRegistry(logf func(format string, args ...any)) *NpcDialogueVariantRegistry {
	return &NpcDialogueVariantRegistry{logf: logf}
}

// Register appends a variant handler. Nil handlers are ignored.
func (r *NpcDialogueVariantRegistry) Register(handler NpcDialogueVariantHandler) {
	if r == nil || handler == nil {
		return
	}
	r.mu.Lock()
	r.handlers = append(r.handlers, handler)
	r.mu.Unlock()
}

// Emit asks the handlers, first truthy answer wins, mirroring
// emitNpcDialogueVariant (PluginManager.ts:1289-1304). A
// panicking handler is recovered, warned about, and skipped. No
// handlers (or no truthy answer) returns false: TS's null.
func (r *NpcDialogueVariantRegistry) Emit(event NpcDialogueVariantEvent) (choice NpcDialogueVariantChoice, ok bool) {
	if r == nil {
		return NpcDialogueVariantChoice{}, false
	}
	r.mu.RLock()
	handlers := append([]NpcDialogueVariantHandler(nil), r.handlers...)
	r.mu.RUnlock()
	for _, handler := range handlers {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					if r.logf != nil {
						r.logf("[plugins] npc dialogue variant hook threw: %v", recovered)
					}
				}
			}()
			if c, answered := handler(event); answered {
				choice, ok = c, true
			}
		}()
		if ok {
			return choice, true
		}
	}
	return NpcDialogueVariantChoice{}, false
}

// NpcDialogueConditionQuery is the PluginNpcDialogueConditionEvent
// payload (PluginTypes.ts:217-222): the variant context plus the
// wiki prose and the condition step's anchor id.
type NpcDialogueConditionQuery struct {
	Player         *Player
	NPC            *NPC
	NPCID          int
	DefinitionName string
	DefinitionID   int
	Pages          []TranscriptPageRef
	Text           string
	StepID         string
}

// NpcDialogueConditionHandler answers a wiki prose condition.
// The bool pair is (answer, answered): answered=false falls
// through to the next handler (TS: null/undefined), answered=true
// settles the condition.
type NpcDialogueConditionHandler func(query NpcDialogueConditionQuery) (answer bool, answered bool)

// NpcDialogueConditionRegistry holds the onNpcDialogueCondition
// handlers in registration order. Safe for concurrent use.
type NpcDialogueConditionRegistry struct {
	mu       sync.RWMutex
	handlers []NpcDialogueConditionHandler
	logf     func(format string, args ...any)
}

// NewNpcDialogueConditionRegistry returns an empty registry. See
// NewNpcDialogueVariantRegistry for logf.
func NewNpcDialogueConditionRegistry(logf func(format string, args ...any)) *NpcDialogueConditionRegistry {
	return &NpcDialogueConditionRegistry{logf: logf}
}

// Register appends a condition handler. Nil handlers are ignored.
func (r *NpcDialogueConditionRegistry) Register(handler NpcDialogueConditionHandler) {
	if r == nil || handler == nil {
		return
	}
	r.mu.Lock()
	r.handlers = append(r.handlers, handler)
	r.mu.Unlock()
}

// Emit asks the handlers, first boolean wins, mirroring
// emitNpcDialogueCondition (PluginManager.ts:1306-1324). A
// panicking handler is recovered, warned about, and skipped. No
// handlers (or no boolean answer) returns answered=false: TS's
// null.
func (r *NpcDialogueConditionRegistry) Emit(query NpcDialogueConditionQuery) (answer bool, answered bool) {
	if r == nil {
		return false, false
	}
	r.mu.RLock()
	handlers := append([]NpcDialogueConditionHandler(nil), r.handlers...)
	r.mu.RUnlock()
	for _, handler := range handlers {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					if r.logf != nil {
						r.logf("[plugins] npc dialogue condition hook threw: %v", recovered)
					}
				}
			}()
			if a, ok := handler(query); ok {
				answer, answered = a, true
			}
		}()
		if answered {
			return answer, true
		}
	}
	return false, false
}

// --- npc-dialogue:* custom event payloads ------------------------
//
// The payloads are passed by pointer through the opaque any
// custom-event channel (handlers receive *NpcDialogueLineEvent
// etc., as the Phase 12 ObjectExamineEvent precedent), so
// handlers can mutate the documented fields. Field names follow
// the TS request objects; Go zero values are the TS defaults.
// Event names are the exact TS custom-event names and must match
// the isPluginCustomEventName rule (exactly one colon).

// NpcDialogueLineEvent is the "npc-dialogue:line" payload
// (NpcDialogues.plugin.js:1013): one speech line, emitted while
// the chain is built, before the line is shown. Handlers may:
//   - set Skip to drop the line (a hand-over line for an item
//     the player no longer carries);
//   - rewrite Text to fill wiki "[number]"-style blanks;
//   - set Before/After callbacks (Before runs when the line's
//     chain entry is reached, After once the player continues
//     past it);
//   - set Actors to bind overhead lines to a scene cast (nil =
//     unset: the speaker is resolved from the step; a non-nil
//     slice, even empty, is an explicit cast and changes the
//     availability rules — see transcript_steps.go);
//   - set SpeakerID/SpeakerName/Expression/Side/HoldTicks to
//     override the chathead, the head animation, the chat side,
//     or to hold the continue for HoldTicks ticks.
type NpcDialogueLineEvent struct {
	Player         *Player
	NPC            *NPC
	NPCID          int
	DefinitionName string
	DefinitionID   int
	Step           definition.DialogueStep
	Text           string
	Pages          []TranscriptPageRef
	Skip           bool
	Before         func()
	After          func()
	Actors         []DialogueActor
	SpeakerID      *int
	SpeakerName    *string
	Expression     *int
	Side           string
	HasSide        bool
	HoldTicks      int
}

// NpcDialogueChoiceEvent is the "npc-dialogue:choice" payload
// (NpcDialogues.plugin.js:821): a menu option was picked.
// Handlers run game logic (set stage, hand in items); setting
// End stops the conversation (the native prompt is already
// closed, so a replacement UI is preserved).
type NpcDialogueChoiceEvent struct {
	Player         *Player
	NPC            *NPC
	NPCID          int
	DefinitionName string
	DefinitionID   int
	Option         string
	StepID         string
	Pages          []TranscriptPageRef
	End            bool
}

// NpcDialogueConditionEvent is the "npc-dialogue:condition"
// payload (NpcDialogues.plugin.js:900): a condition branch was
// chosen and its body is about to play, so quests can run their
// own game logic. No mutable fields are read back.
type NpcDialogueConditionEvent struct {
	Player         *Player
	NPC            *NPC
	NPCID          int
	DefinitionName string
	DefinitionID   int
	Text           string
	StepID         string
}

// NpcDialogueBox is the message.box shape a
// "npc-dialogue:action" handler sets to show a message step as an
// item/statement box instead of a chat message: Items holds one
// or two item ids (one -> ItemStatementDialogue, two ->
// DoubleItemStatementDialogue); Statement shows a plain
// StatementDialogue box; HoldTicks/Advance control the held
// continue (see NpcDialogueActionEvent).
type NpcDialogueBox struct {
	Items     []int
	Statement bool
	HoldTicks int
	Advance   bool
}

// NpcDialogueActionEvent is the "npc-dialogue:action" payload
// (NpcDialogues.plugin.js:1066, 1105): a message or action step.
// Kind is "message" for message steps (emitted instead of, not
// in addition to, the generic event, so id-keyed handlers do not
// act twice) and "" for other steps. Handlers may:
//   - set Handled (generic steps) and optionally Steps to play
//     first, and End to close instead of continuing;
//   - set Handled (message steps) to suppress the chat message,
//     End to close, Box to show an item/statement box, and
//     After to run once the player continues past the box;
//   - rewrite Text (read back only on the replacement path, for
//     message steps — the plain chat path re-reads the step's
//     text, exactly as TS does).
type NpcDialogueActionEvent struct {
	Player         *Player
	NPC            *NPC
	NPCID          int
	DefinitionName string
	DefinitionID   int
	Step           definition.DialogueStep
	Pages          []TranscriptPageRef
	Text           string
	Action         string
	Target         string
	StepID         string
	Kind           string
	Handled        bool
	End            bool
	Steps          []definition.DialogueStep
	Box            *NpcDialogueBox
	After          func()
}

// NpcDialogueShopEvent is the "npc-dialogue:shop" payload
// (NpcDialogues.plugin.js:580-584): an open_shop step with
// versions. Handlers pick one by setting Shop; nil leaves the
// Default version to open (or nothing, when no Default version
// exists).
type NpcDialogueShopEvent struct {
	Player *Player
	NPCID  int
	Target string
	Shops  []string
	Shop   *string
}

// NpcDialogueMenuEvent is the "npc-dialogue:menu" payload
// (NpcDialogues.plugin.js:793-796): a menu is about to be shown.
// Handlers may set Handled (they asked the question themselves),
// Warning (shown once per menu step before the menu), or replace
// Options with option nodes from this or an earlier menu
// (History carries the menus shown so far; Chosen is the option
// just picked, "" when none; Via is "other" for a menu reached
// through a "shows other options" jump, "" otherwise). A nil
// Options after the emit falls back to the menu's own options;
// an empty non-nil slice shows no options.
type NpcDialogueMenuEvent struct {
	Player         *Player
	NPCID          int
	DefinitionName string
	DefinitionID   int
	Prompt         string
	Warning        string
	Options        []definition.DialogueOptionStep
	History        []NpcDialogueMenuHistory
	Via            string
	Chosen         string
	Handled        bool
}

// NpcDialogueMenuHistory is one shown menu, for handlers that
// borrow options from earlier menus: the choice step itself
// (its Options are the borrowable nodes).
type NpcDialogueMenuHistory struct {
	Step definition.DialogueStep
}

// NpcDialogueHookEvent is the "npc-dialogue:hook" payload
// (NpcDialogues.plugin.js:815, 1053): a quest slug on an option
// (Option set) or an action step (Action set). Handlers run the
// quest action; setting End stops the conversation (option
// hooks: the native prompt is already closed, so a replacement
// UI is preserved; action hooks: the chatbox closes).
type NpcDialogueHookEvent struct {
	Player         *Player
	NPC            *NPC
	NPCID          int
	DefinitionName string
	DefinitionID   int
	Hook           string
	Quest          string
	Option         string
	Action         string
	End            bool
}

// SlayerAssignmentEvent is the "slayer:assignment" payload
// (NpcDialogues.plugin.js:1153): a slayer_assignment action step.
// Handlers set Line to inject an NPC line saying the assignment.
type SlayerAssignmentEvent struct {
	Player       *Player
	Master       string
	NPCID        int
	DefinitionID int
	NPCName      string
	Line         string
}

// SlayerTaskTipEvent is the "slayer:task-tip" payload
// (NpcDialogues.plugin.js:1158): a slayer_task_tip action step.
// Handlers set Line to inject an NPC line with the tip.
type SlayerTaskTipEvent struct {
	Player *Player
	Master string
	Line   string
}

// --- Resolution --------------------------------------------------

// ResolveTranscript ports resolveTranscript
// (NpcDialogues.plugin.js:1255-1319): the id index first (with
// the plugin variant selector), the cache name second. See
// ResolvedTranscript for the result shapes. A nil Data behaves
// as an empty catalogue (nil result); a nil Player returns nil.
func (pb *DialoguePlayback) ResolveTranscript(event TranscriptEvent) *ResolvedTranscript {
	data := pb.deps.Data
	if data == nil || event.Player == nil {
		return nil
	}
	// A transform the index doesn't know falls back to the
	// spawned parent (the live NPC's id), and a successor of a
	// retired NPC to the NPC it replaced (PREDECESSOR).
	ids := []int{event.NPCID}
	if event.NPC != nil {
		ids = append(ids, event.NPC.ID)
	}
	if predecessor, ok := data.Predecessors[event.NPCID]; ok {
		ids = append(ids, predecessor)
	}
	var entries []definition.DialogueIndexEntry
	for _, id := range ids {
		if rows, ok := data.Index[id]; ok {
			entries = rows
			break
		}
	}
	pages := []TranscriptPageRef{}
	for _, entry := range entries {
		page := entry.PageName()
		if data.Record(page) != nil {
			pages = append(pages, TranscriptPageRef{Page: page, Variants: entry.Variants})
		}
	}
	// Only trusted custom-event activation may bind an
	// unindexed actor to a named source page.
	if event.Explicit && event.VariantPage != "" {
		if record := data.Record(event.VariantPage); record != nil && !pagesContain(pages, event.VariantPage) {
			pages = append(pages, TranscriptPageRef{Page: event.VariantPage, Variants: record.VariantOrder})
		}
	}
	// A named transform missing from the id index still needs
	// variant selectors: use the name/alias path, but through
	// the same pages list.
	if len(pages) == 0 {
		name := event.DefinitionName
		page := ""
		if data.Record(name) != nil {
			page = name
		}
		if _, playable := data.Record(page).PickVariant(); !playable {
			if alias, ok := data.Aliases[name]; ok {
				page = alias
			}
		}
		if page != "" {
			if _, playable := data.Record(page).PickVariant(); playable {
				pages = append(pages, TranscriptPageRef{Page: page, Variants: data.Record(page).VariantOrder})
			}
		}
	}
	context := &TranscriptContext{Pages: pages, SpeakerIDByName: data.SpeakerIDByName}
	withOptions := func(page string, result *ResolvedTranscript) *ResolvedTranscript {
		result.Context = context
		if page != "" {
			if record := data.Record(page); record != nil {
				result.Page = page
				options := CollectPageOptions(record)
				context.PageOptions = options.ByText
				context.PageOptionList = options.List
				context.PageLines = CollectPageLines(record)
				// Only a known variant has a well-defined
				// "menu above"; otherwise a page shared by
				// several NPCs could replay an unrelated
				// menu.
				if result.Variant != "" {
					context.PageMenuBefore = CollectMenuBefore(record, result.Variant, false, "")
					context.PageInitialMenu = CollectMenuBefore(record, result.Variant, true, event.DefinitionName)
				}
			}
		}
		return result
	}

	if len(pages) > 0 {
		choice := pb.variantChoice(event, pages)
		wanted, wantedPage := "", ""
		if choice != nil {
			wanted, wantedPage = choice.Variant, choice.Page
		}
		if wanted != "" || wantedPage != "" {
			var pageEntry *TranscriptPageRef
			if wantedPage != "" {
				pageEntry = findTranscriptPage(pages, wantedPage)
			} else {
				// The id index sometimes slugifies a variant
				// differently from the dump (drops words),
				// so fall back to whichever page actually
				// has it.
				pageEntry = findTranscriptPageWithVariant(pages, wanted)
				if pageEntry == nil {
					pageEntry = findTranscriptPageWithRecordVariant(pages, wanted, data)
				}
			}
			if pageEntry != nil {
				record := data.Record(pageEntry.Page)
				steps, stepsOK := wantedSteps(record, wanted)
				if stepsOK {
					return withOptions(pageEntry.Page, &ResolvedTranscript{
						Steps: steps, StepsPresent: true, Branches: record.Branches,
						Variant: wanted,
					})
				}
				if record != nil && record.Activation == "explicit" {
					return nil
				}
			}
		}
		for _, page := range pages {
			if steps, ok := data.Record(page.Page).PickVariant(); ok {
				return withOptions(page.Page, &ResolvedTranscript{
					Steps: steps, StepsPresent: true, Branches: data.Record(page.Page).Branches,
				})
			}
		}
	}

	name := event.DefinitionName
	record := data.Record(name)
	if _, playable := record.PickVariant(); !playable {
		if alias, ok := data.Aliases[name]; ok {
			record = data.Record(alias)
		}
	}
	// A requested variant found by name is still that variant
	// (npc-dialogue:start's `exact`). An object-form variant
	// keys as "[object Object]" in TS and never matches; only
	// the string form is looked up.
	var forced []definition.DialogueStep
	forcedOK := false
	if event.HasVariant && !event.VariantIsObject && record != nil {
		if steps, ok := record.Variants[event.Variant]; ok {
			forced, forcedOK = steps, true
		}
	}
	var steps []definition.DialogueStep
	var ok bool
	if forcedOK {
		steps, ok = forced, true
	} else {
		steps, ok = record.PickVariant()
	}
	variant := ""
	if forcedOK {
		variant = event.Variant
	}
	if ok {
		pageName := ""
		if record != nil && record == data.Record(name) {
			pageName = name
		}
		return withOptions(pageName, &ResolvedTranscript{
			Steps: steps, StepsPresent: true, Branches: record.Branches, Variant: variant,
		})
	}
	// A transcript reaches the NPC but pickVariant found
	// nothing in it to play: name it for the warning.
	var page string
	if record != nil {
		if record == data.Record(name) {
			page = name
		} else {
			page = data.Aliases[name]
		}
	} else if len(pages) > 0 {
		page = pages[0].Page
	}
	if page != "" {
		return &ResolvedTranscript{Page: page, Context: context}
	}
	return nil
}

// variantChoice returns the requested-or-selected variant: the
// event's own variant when supplied (event.variant ?? emit),
// else the first truthy variant-hook answer, else nil.
func (pb *DialoguePlayback) variantChoice(event TranscriptEvent, pages []TranscriptPageRef) *NpcDialogueVariantChoice {
	if event.HasVariant {
		return &NpcDialogueVariantChoice{Variant: event.Variant, Page: event.VariantPage}
	}
	if pb.deps.VariantHandlers == nil {
		return nil
	}
	choice, ok := pb.deps.VariantHandlers.Emit(NpcDialogueVariantEvent{
		Player: event.Player, NPC: event.NPC, NPCID: event.NPCID,
		DefinitionName: event.DefinitionName, DefinitionID: event.DefinitionID, Pages: pages,
	})
	if !ok {
		return nil
	}
	return &choice
}

// wantedSteps resolves the steps for a wanted variant on one
// page: the variant itself when the record has a variants
// object, the flat steps for "standard-dialogue" on a flat page,
// else pickVariant when no variant was wanted (page-only
// selection). It reports whether the result is an array (TS
// Array.isArray); a missing variant reports false.
func wantedSteps(record *definition.TranscriptRecord, wanted string) ([]definition.DialogueStep, bool) {
	if wanted != "" {
		if record != nil && record.HasVariants {
			steps, ok := record.Variants[wanted]
			return steps, ok
		}
		if wanted == "standard-dialogue" && record != nil && record.HasSteps {
			return record.Steps, true
		}
		return nil, false
	}
	if record == nil {
		return nil, false
	}
	return record.PickVariant()
}

func pagesContain(pages []TranscriptPageRef, page string) bool {
	for _, entry := range pages {
		if entry.Page == page {
			return true
		}
	}
	return false
}

func findTranscriptPage(pages []TranscriptPageRef, page string) *TranscriptPageRef {
	for i := range pages {
		if pages[i].Page == page {
			return &pages[i]
		}
	}
	return nil
}

func findTranscriptPageWithVariant(pages []TranscriptPageRef, wanted string) *TranscriptPageRef {
	for i := range pages {
		for _, variant := range pages[i].Variants {
			if variant == wanted {
				return &pages[i]
			}
		}
	}
	return nil
}

func findTranscriptPageWithRecordVariant(pages []TranscriptPageRef, wanted string, data *definition.DialogueData) *TranscriptPageRef {
	for i := range pages {
		if record := data.Record(pages[i].Page); record != nil {
			if _, ok := record.Variants[wanted]; ok {
				return &pages[i]
			}
		}
	}
	return nil
}

// StartTalkTo plays the Talk-to conversation for an NPC: special
// NPCs fall through (false), anything else plays its resolved
// transcript or the placeholder line (true). It ports the
// onAnyNpcInteraction "Talk-to" handler
// (NpcDialogues.plugin.js:1368-1379). The NPC-click routing that
// calls it (NPCOptionPacketListener, walk-to-NPC) belongs to the
// coordinator/Worker C wiring, not to this file.
func (pb *DialoguePlayback) StartTalkTo(event TranscriptEvent) bool {
	if event.Player == nil {
		return false
	}
	if SpecialNPCDialogues[event.DefinitionName] {
		return false
	}
	resolved := pb.ResolveTranscript(event)
	if resolved == nil || len(resolved.Steps) == 0 {
		pb.reportDialogueGap(event, resolved)
	}
	steps := []definition.DialogueStep{{NPCText: PlaceholderLine, HasNPC: true}}
	var branches map[string][]definition.DialogueStep
	var context *TranscriptContext
	if resolved != nil && len(resolved.Steps) > 0 {
		steps, branches, context = resolved.Steps, resolved.Branches, resolved.Context
	} else if resolved != nil {
		branches, context = resolved.Branches, resolved.Context
	}
	pb.StartDialogue(event, steps, branches, context)
	return true
}

// reportDialogueGap warns once per NPC name that Talk-to fell
// back to the placeholder line (NpcDialogues.plugin.js:233-242).
// The once-per-name set lives on the DialoguePlayback instance
// (TS keeps it at module scope); it holds names, never player
// state.
func (pb *DialoguePlayback) reportDialogueGap(event TranscriptEvent, resolved *ResolvedTranscript) {
	name := event.DefinitionName
	pb.mu.Lock()
	if pb.reportedGaps[name] {
		pb.mu.Unlock()
		return
	}
	pb.reportedGaps[name] = true
	pb.mu.Unlock()
	var reason string
	switch {
	case resolved != nil && resolved.StepsPresent:
		reason = "its transcript variant is empty"
	case resolved != nil && resolved.Page != "":
		reason = "transcript \"" + resolved.Page + "\" has no playable variant"
	default:
		reason = "no transcript reaches it"
	}
	pb.logf("[npc-dialogues] %s (npc %d) says the placeholder line: %s", name, event.NPCID, reason)
}

// StartPageRequest is the item-transcript activation
// (NpcDialogues.plugin.js:1325-1351): an item's own transcript,
// named by {page, variant} alone, with no NPC. Only
// explicit-activation records play. Select may narrow the
// variant to the steps that apply. On success Cancel receives
// the scoped cancellation handle and Handled is set.
type StartPageRequest struct {
	Player  *Player
	Page    string
	Variant string
	Select  func([]definition.DialogueStep) []definition.DialogueStep
	Cancel  func() bool
	Handled bool
}

// StartPage plays one named variant of an item transcript. It
// ports startPage (NpcDialogues.plugin.js:1333-1351).
func (pb *DialoguePlayback) StartPage(req *StartPageRequest) {
	if req == nil || req.Player == nil {
		return
	}
	data := pb.deps.Data
	if data == nil {
		return
	}
	record := data.Record(req.Page)
	if record == nil || record.Activation != "explicit" {
		return
	}
	var steps []definition.DialogueStep
	stepsOK := false
	if record.HasVariants {
		if s, ok := record.Variants[req.Variant]; ok {
			steps, stepsOK = s, true
		}
	} else if req.Variant == "standard-dialogue" && record.HasSteps {
		steps, stepsOK = record.Steps, true
	}
	if !stepsOK {
		return
	}
	selected := steps
	if req.Select != nil {
		selected = req.Select(steps)
	}
	if len(selected) == 0 {
		return
	}
	options := CollectPageOptions(record)
	variants := record.VariantOrder
	if !record.HasVariants {
		variants = []string{"standard-dialogue"}
	}
	context := &TranscriptContext{
		Pages:           []TranscriptPageRef{{Page: req.Page, Variants: variants}},
		SpeakerIDByName: data.SpeakerIDByName,
		PageOptions:     options.ByText,
		PageOptionList:  options.List,
		PageLines:       CollectPageLines(record),
		PageMenuBefore:  CollectMenuBefore(record, req.Variant, false, ""),
		PageInitialMenu: CollectMenuBefore(record, req.Variant, true, ""),
	}
	event := TranscriptEvent{
		Player: req.Player, Explicit: true,
		HasVariant: true, Variant: req.Variant, VariantPage: req.Page, VariantIsObject: true,
	}
	req.Cancel = pb.StartDialogue(event, selected, record.Branches, context)
	req.Handled = true
}

// StartNamedRequest is the "npc-dialogue:start" activation
// (NpcDialogues.plugin.js:1321-1331, 1353-1366): play one named
// variant of an NPC's transcript outside Talk-to. Select may
// narrow the variant; Exact requires the resolved variant (and
// page, when one was requested) to match exactly, else nothing
// plays. On success Cancel receives the scoped cancellation
// handle and Handled is set. The custom-event subscription
// itself belongs to the coordinator/Worker C wiring, which
// translates the event payload into this request; the
// item-transcript form (no NPC id, object variant with a page)
// delegates to StartPage, as TS does.
type StartNamedRequest struct {
	Player          *Player
	NPC             *NPC
	NPCID           int
	HasNPCID        bool
	DefinitionName  string
	DefinitionID    int
	Variant         string
	VariantPage     string
	HasVariant      bool
	VariantIsObject bool
	Select          func([]definition.DialogueStep) []definition.DialogueStep
	Exact           bool
	Cancel          func() bool
	Handled         bool
}

// StartNamedVariant plays one named variant outside Talk-to. It
// ports the "npc-dialogue:start" handler
// (NpcDialogues.plugin.js:1353-1366).
func (pb *DialoguePlayback) StartNamedVariant(req *StartNamedRequest) {
	if req == nil || req.Player == nil {
		return
	}
	if !req.HasNPCID && req.VariantIsObject {
		pageReq := &StartPageRequest{
			Player: req.Player, Page: req.VariantPage, Variant: req.Variant, Select: req.Select,
		}
		pb.StartPage(pageReq)
		req.Cancel, req.Handled = pageReq.Cancel, pageReq.Handled
		return
	}
	// TS looks the definition up (NpcDefinition.forId) and
	// returns when there is none. Go has no definition lookup
	// in this package; the caller supplies the definition, and
	// an absent id with neither a name nor a live NPC is the
	// observable "no definition" case.
	if !req.HasNPCID || (req.DefinitionName == "" && req.NPC == nil) {
		return
	}
	event := TranscriptEvent{
		Player: req.Player, NPC: req.NPC, NPCID: req.NPCID,
		DefinitionName: req.DefinitionName, DefinitionID: req.DefinitionID,
		Variant: req.Variant, VariantPage: req.VariantPage,
		HasVariant: req.HasVariant, VariantIsObject: req.VariantIsObject, Explicit: true,
	}
	resolved := pb.ResolveTranscript(event)
	wanted := req.Variant
	wantedPage := ""
	if req.VariantIsObject {
		wantedPage = req.VariantPage
	}
	if req.Exact {
		if resolved == nil || resolved.Variant != wanted || (wantedPage != "" && resolved.Page != wantedPage) {
			return
		}
	}
	var steps []definition.DialogueStep
	if resolved != nil {
		steps = resolved.Steps
		if req.Select != nil && resolved.StepsPresent {
			steps = req.Select(resolved.Steps)
		}
	}
	if len(steps) == 0 {
		return
	}
	req.Cancel = pb.StartDialogue(event, steps, resolved.Branches, resolved.Context)
	req.Handled = true
}

// --- Player-text substitution ------------------------------------
//
// Wiki transcripts leave parts of a line to the reader's
// character: their name ("[player name]", "<player name>") and
// gendered alternatives ("[sir/madam]",
// "[Greetings, sir/Greetings, madam/Greetings]"). Resolve them
// from the player seeing the dialogue; alternatives without a
// gendered form (numbers, item picks, NPC choices) stay for
// quest plugins to fill (NpcDialogues.plugin.js:150-218).

var (
	playerNamePlaceholder = regexp.MustCompile(`(?i)\[(?:player name|playername|player|player vampyre name)(\.)?\]|<player name>`)
	playerNameAlternative = regexp.MustCompile(`(?i)^(?:player name|playername|player vampyre name)$`)
	genderedAlternative   = regexp.MustCompile(`\[([^\[\]]*/[^\[\]]*)\]`)
	playerSpeakerPattern  = regexp.MustCompile(`(?i)^(?:player|\[(?:player name|playername|player)\]|<player name>)$`)
	// typedPlayerSpeaker is the narrower isPlayer test for typed
	// lines (NpcDialogues.plugin.js:1010): unlike
	// PLAYER_SPEAKER, a bare "player" speaker does NOT count.
	typedPlayerSpeaker = regexp.MustCompile(`(?i)^\[(?:player name|playername|player)\]$|^<player name>$`)
	wordPattern        = regexp.MustCompile(`[a-z']+`)
)

// maleAddressWords / femaleAddressWords are the address words the
// wiki templates use for each gender
// (NpcDialogues.plugin.js:163-171), copied verbatim.
var maleAddressWords = map[string]bool{
	"he": true, "him": true, "his": true, "himself": true, "sir": true,
	"sirrah": true, "mister": true, "master": true, "milord": true,
	"lord": true, "lad": true, "laddie": true, "man": true, "men": true,
	"boy": true, "boys": true, "brother": true, "fellow": true,
	"fella": true, "chap": true, "guy": true, "prince": true,
	"strongman": true, "craftsman": true, "monsieur": true,
}

var femaleAddressWords = map[string]bool{
	"she": true, "her": true, "hers": true, "herself": true,
	"madam": true, "madame": true, "ma'am": true, "m'am": true,
	"miss": true, "lady": true, "milady": true, "m'lady": true,
	"mistress": true, "lass": true, "lassie": true, "woman": true,
	"women": true, "girl": true, "girls": true, "gal": true,
	"sister": true, "princess": true, "strongwoman": true,
	"craftswoman": true,
}

// genderedForm returns the gender an alternative addresses:
// "male", "female", or "" when it addresses neither or both
// (genderedForm, NpcDialogues.plugin.js:175-180).
func genderedForm(text string) string {
	words := wordPattern.FindAllString(strings.ToLower(text), -1)
	male, female := false, false
	for _, word := range words {
		if maleAddressWords[word] {
			male = true
		}
		if femaleAddressWords[word] {
			female = true
		}
	}
	if male == female {
		return ""
	}
	if male {
		return "male"
	}
	return "female"
}

// pickAlternative returns the alternative to show a player of
// this gender, or false to leave the group alone
// (pickAlternative, NpcDialogues.plugin.js:183-191). Pronouns
// follow Appearance.getPronouns: 0 male, 1 female, anything else
// (2, unset) no preference. hasName distinguishes a player with
// an empty display name (the group is replaced with "") from no
// player at all (the group is left alone).
func pickAlternative(parts []string, name string, hasName bool, pronouns int) (string, bool) {
	for _, part := range parts {
		if playerNameAlternative.MatchString(part) {
			if !hasName {
				return "", false
			}
			return name, true
		}
	}
	type form struct {
		text   string
		gender string
	}
	forms := make([]form, len(parts))
	anyGendered := false
	for i, part := range parts {
		forms[i] = form{text: part, gender: genderedForm(part)}
		if forms[i].gender != "" {
			anyGendered = true
		}
	}
	if !anyGendered {
		return "", false
	}
	wanted := ""
	switch pronouns {
	case 0:
		wanted = "male"
	case 1:
		wanted = "female"
	}
	for _, f := range forms {
		if f.gender == wanted {
			return f.text, true
		}
	}
	for _, f := range forms {
		if f.gender == "" {
			return f.text, true
		}
	}
	return forms[0].text, true
}

// FormatPlayerText ports formatPlayerText
// (NpcDialogues.plugin.js:193-203): player-name placeholders,
// then gendered alternatives. A nil player resolves no name and
// reads pronouns as male (TS: appearance undefined ->
// getPronouns undefined -> isMale check false -> 0).
func FormatPlayerText(text string, player *Player) string {
	out := text
	name, hasName := "", false
	pronouns := 0
	if player != nil {
		name, hasName = player.GetDisplayName(), true
		if appearance := player.Appearance(); appearance != nil {
			pronouns = appearance.Pronouns()
		}
	}
	// "[player.]" keeps the sentence's closing period.
	if name != "" {
		out = replacePlayerNamePlaceholders(out, name)
	}
	if !strings.Contains(out, "/") {
		return out
	}
	return genderedAlternative.ReplaceAllStringFunc(out, func(whole string) string {
		groups := genderedAlternative.FindStringSubmatch(whole)
		if len(groups) < 2 {
			return whole
		}
		rawParts := strings.Split(groups[1], "/")
		parts := make([]string, len(rawParts))
		for i, part := range rawParts {
			parts[i] = strings.TrimSpace(part)
		}
		if len(parts) < 2 {
			return whole
		}
		if picked, ok := pickAlternative(parts, name, hasName, pronouns); ok {
			return picked
		}
		return whole
	})
}

// replacePlayerNamePlaceholders substitutes the display name for
// every player-name placeholder, preserving a captured closing
// period. It is written as an explicit loop (not
// ReplaceAllString) because the replacement name may contain "$"
// and because the period capture is absent on the
// "<player name>" alternative.
func replacePlayerNamePlaceholders(text, name string) string {
	matches := playerNamePlaceholder.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	last := 0
	for _, match := range matches {
		out.WriteString(text[last:match[0]])
		out.WriteString(name)
		if match[2] >= 0 {
			out.WriteString(text[match[2]:match[3]])
		}
		last = match[1]
	}
	out.WriteString(text[last:])
	return out.String()
}

// --- Chatbox paging (character fallback) -------------------------

// CharacterPages ports characterPages
// (NpcDialogues.plugin.js:570-575): the layout before
// ChatTextLayout — Misc.wrapText at 53 characters, four lines a
// page, lines joined with a space. Playback uses it whenever no
// PageText seam is installed, which is the TS behavior when
// ChatTextLayout is absent
// (NpcDialogues.plugin.js:1023: `?? characterPages(formatted)`).
func CharacterPages(text string) []string {
	lines := WrapDialogueText(text, 53)
	var pages []string
	for start := 0; start < len(lines); start += 4 {
		end := start + 4
		if end > len(lines) {
			end = len(lines)
		}
		pages = append(pages, strings.Join(lines[start:end], " "))
	}
	return pages
}

// dialogueTextEffects is the EFFECTS list in Misc.wrapText
// (Misc.ts:860-897), copied verbatim including the duplicated
// "<str" entry: effect codes found anywhere in the text are
// concatenated (in list order) and prepended to every wrapped
// line that does not already start with them.
var dialogueTextEffects = []string{
	"@gre@", "@cya@", "@red@", "chalreq", "tradereq", "@bro@",
	"@yel@", "@blu@", "@gr1@", "@gr2@", "@gr3@", "@str@", "@mag@",
	"@dre@", "@dbl@", "@or1@", "@or2@", "@or3@", "@whi@", "@bla@",
	"@cr", "<col", "<shad", "<str", "<u", "<br", "<trans",
	"duelreq", "<img", "@lre@", ":clan:", "]cr", "::summ", "<str",
}

// WrapDialogueText ports Misc.wrapText (Misc.ts:859-963): wrap at
// word boundaries so no line exceeds len characters, carrying the
// text-effect prefix onto every line. Lengths and splitting are
// in runes; TS counts UTF-16 code units, which differs only for
// characters outside the Basic Multilingual Plane (no dialogue
// text uses them).
func WrapDialogueText(text string, length int) []string {
	if length <= 0 {
		return []string{text}
	}
	chars := []rune(text)
	if len(chars) <= length {
		return []string{text}
	}
	var effects *string
	for _, code := range dialogueTextEffects {
		if strings.Contains(text, code) {
			if effects == nil {
				empty := ""
				effects = &empty
			}
			*effects += code
		}
	}
	applyEffects := func(line string) string {
		if effects != nil && !strings.HasPrefix(line, *effects) {
			return *effects + line
		}
		return line
	}
	var lines []string
	var line, word strings.Builder
	lineLen, wordLen := 0, 0
	flushLine := func() {
		lines = append(lines, applyEffects(line.String()))
		line.Reset()
		lineLen = 0
	}
	for _, ch := range chars {
		word.WriteRune(ch)
		wordLen++
		if ch == ' ' {
			if lineLen+wordLen > length {
				flushLine()
			}
			line.WriteString(word.String())
			lineLen += wordLen
			word.Reset()
			wordLen = 0
		}
	}
	if wordLen > 0 {
		if lineLen+wordLen > length {
			flushLine()
		}
		line.WriteString(word.String())
		lineLen += wordLen
	}
	if lineLen > 0 {
		lines = append(lines, applyEffects(line.String()))
	}
	return lines
}

// --- Page-level indexes ------------------------------------------

// normText normalizes option/line text so punctuation/prefix
// drift still matches ("Well," == "Well")
// (NpcDialogues.plugin.js:301-303).
func normText(value string) string {
	lower := strings.ToLower(value)
	var out strings.Builder
	for _, ch := range lower {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			out.WriteRune(ch)
		}
	}
	return out.String()
}

// transcriptWords returns the significant words of an option
// (longer than 2 characters) for similarity matching
// (NpcDialogues.plugin.js:306-310).
func transcriptWords(value string) map[string]bool {
	lower := strings.ToLower(value)
	var spaced strings.Builder
	for _, ch := range lower {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			spaced.WriteRune(ch)
		} else {
			spaced.WriteRune(' ')
		}
	}
	words := make(map[string]bool)
	for _, word := range strings.Split(spaced.String(), " ") {
		if len(word) > 2 {
			words[word] = true
		}
	}
	return words
}

// wordSimilarity is the Jaccard word overlap, 0..1
// (NpcDialogues.plugin.js:313-319).
func wordSimilarity(a, b map[string]bool) float64 {
	inter := 0
	for word := range a {
		if b[word] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// jumpOnly reports whether a branch is only navigation jumps (no
// dialogue of its own), including an empty/missing branch
// (NpcDialogues.plugin.js:287-289).
func jumpOnly(steps []definition.DialogueStep) bool {
	if len(steps) == 0 {
		return true
	}
	for _, step := range steps {
		if step.Type != "jump" {
			return false
		}
	}
	return true
}

// realBody returns a step body that actually says something
// (usable as a jump target), or nil
// (NpcDialogues.plugin.js:292-294).
func realBody(steps []definition.DialogueStep) []definition.DialogueStep {
	if len(steps) == 0 || jumpOnly(steps) {
		return nil
	}
	return steps
}

// continues reports whether a branch ends in a wiki "continues"
// marker (NpcDialogues.plugin.js:265-276).
func continues(steps []definition.DialogueStep) bool {
	for _, step := range steps {
		if step.Type == "jump" {
			if strings.HasPrefix(strings.ToLower(step.Reference), "continue") {
				return true
			}
			continue
		}
		if continues(step.Steps) {
			return true
		}
		for _, option := range step.Options {
			if continues(option.Steps) {
				return true
			}
		}
	}
	return false
}

// isLinearConditionBranch reports whether a branch is a linear
// condition body whose continuation is not controlled by menus
// or jumps (NpcDialogues.plugin.js:279-284).
func isLinearConditionBranch(steps []definition.DialogueStep) bool {
	for _, step := range steps {
		switch step.Type {
		case "choice", "condition", "jump":
			return false
		}
		if !isLinearConditionBranch(step.Steps) {
			return false
		}
	}
	return true
}

// CollectPageOptions ports collectPageOptions
// (NpcDialogues.plugin.js:322-338): option text -> steps across
// every variant of a transcript page; real bodies win over
// jump-only ones for the same normalized text. Non-array
// variants hold no steps and are skipped (TS would throw
// iterating one; the shipped dump has none).
func CollectPageOptions(record *definition.TranscriptRecord) PageOptionIndex {
	byText := make(map[string][]definition.DialogueStep)
	var list []PageOptionEntry
	var walk func(steps []definition.DialogueStep)
	walk = func(steps []definition.DialogueStep) {
		for _, step := range steps {
			for _, option := range step.Options {
				key := normText(option.Text)
				body := option.Steps
				if body == nil {
					body = []definition.DialogueStep{}
				}
				if key != "" {
					list = append(list, PageOptionEntry{Key: key, Words: transcriptWords(option.Text), Steps: body})
					if existing, ok := byText[key]; !ok || (jumpOnly(existing) && !jumpOnly(body)) {
						byText[key] = body
					}
				}
				walk(option.Steps)
			}
			walk(step.Steps)
		}
	}
	if record != nil {
		walk(record.Steps)
		for _, name := range record.VariantOrder {
			if steps, ok := record.Variants[name]; ok {
				walk(steps)
			}
		}
	}
	return PageOptionIndex{ByText: byText, List: list}
}

// CollectPageLines ports collectPageLines
// (NpcDialogues.plugin.js:341-359): normalized line text -> the
// steps that follow it (from the line onward), across a page's
// variants. First occurrence wins per normalized text.
func CollectPageLines(record *definition.TranscriptRecord) map[string][]definition.DialogueStep {
	lines := make(map[string][]definition.DialogueStep)
	var walk func(steps []definition.DialogueStep)
	walk = func(steps []definition.DialogueStep) {
		for i := range steps {
			step := steps[i]
			text, ok := lineSpeechText(step)
			if ok {
				key := normText(text)
				if key != "" {
					if _, exists := lines[key]; !exists {
						lines[key] = steps[i:]
					}
				}
			}
			walk(step.Steps)
			for _, option := range step.Options {
				walk(option.Steps)
			}
		}
	}
	if record != nil {
		walk(record.Steps)
		for _, name := range record.VariantOrder {
			if steps, ok := record.Variants[name]; ok {
				walk(steps)
			}
		}
	}
	return lines
}

// lineSpeechText returns the speech text of a step in the
// collectPageLines priority: player, npc, typed line text.
func lineSpeechText(step definition.DialogueStep) (string, bool) {
	if step.HasPlayer {
		return step.PlayerText, true
	}
	if step.HasNPC {
		return step.NPCText, true
	}
	if step.Type == "line" && step.HasText {
		return step.Text, true
	}
	return "", false
}

// topLevelMenu ports topLevelMenu
// (NpcDialogues.plugin.js:362-375): the first menu, including
// condition containers (nested only) but never another menu's
// options. It returns the menu step, or (nil, true) when the
// nested search found several menus and the choice is ambiguous
// (TS null), or (nil, false) when none was found.
func topLevelMenu(steps []definition.DialogueStep, nested bool) (*definition.DialogueStep, bool) {
	for i := range steps {
		if steps[i].Type == "choice" && len(steps[i].Options) > 0 {
			return &steps[i], false
		}
	}
	if !nested {
		return nil, false
	}
	var menus []*definition.DialogueStep
	var visit func(nodes []definition.DialogueStep)
	visit = func(nodes []definition.DialogueStep) {
		for i := range nodes {
			if nodes[i].Type == "choice" && len(nodes[i].Options) > 0 {
				menus = append(menus, &nodes[i])
			} else if nodes[i].Type == "condition" {
				visit(nodes[i].Steps)
			}
		}
	}
	visit(steps)
	switch len(menus) {
	case 0:
		return nil, false
	case 1:
		return menus[0], false
	default:
		// A recovered historical menu has no guard context:
		// never choose among alternatives.
		return nil, true
	}
}

// sourceSpeakers ports sourceSpeakers
// (NpcDialogues.plugin.js:378-389): the named source actors of a
// variant (normalized speaker names other than the player).
func sourceSpeakers(steps []definition.DialogueStep) map[string]bool {
	speakers := make(map[string]bool)
	var visit func(nodes []definition.DialogueStep)
	visit = func(nodes []definition.DialogueStep) {
		for _, step := range nodes {
			speaker := normText(step.Speaker)
			if speaker != "" && speaker != "player" && speaker != "playername" {
				speakers[speaker] = true
			}
			visit(step.Steps)
			for _, option := range step.Options {
				visit(option.Steps)
			}
		}
	}
	visit(steps)
	return speakers
}

// CollectMenuBefore ports collectMenuBefore
// (NpcDialogues.plugin.js:396-417): the menu "above" the given
// variant on its page — the nearest earlier variant with a menu,
// within the same variant family. With initial=true the guards
// for the initial-menu search apply (single-speaker pages only,
// matching the caller). It returns the menu step, or nil when
// there is none or the nearest candidate is ambiguous.
func CollectMenuBefore(record *definition.TranscriptRecord, variantName string, initial bool, callerName string) *definition.DialogueStep {
	if record == nil {
		return nil
	}
	keys := record.VariantOrder
	var currentSpeakers map[string]bool
	if initial {
		if steps, ok := record.Variants[variantName]; ok {
			currentSpeakers = sourceSpeakers(steps)
		} else {
			currentSpeakers = sourceSpeakers(nil)
		}
	}
	actor := normText(callerName)
	if actor == "" && len(currentSpeakers) == 1 {
		for speaker := range currentSpeakers {
			actor = speaker
		}
	}
	if initial {
		if actor == "" || len(currentSpeakers) > 1 || (len(currentSpeakers) == 1 && !currentSpeakers[actor]) {
			return nil
		}
	}
	family := ""
	if variantName != "" {
		family = strings.Split(variantName, "-")[0]
	}
	index := -1
	for i, key := range keys {
		if key == variantName {
			index = i
			break
		}
	}
	upto := len(keys)
	if index != -1 {
		upto = index
	}
	var menu *definition.DialogueStep
	for i := 0; i < upto; i++ {
		// Stay within the same variant family
		// ("sir-prysin-*"), or a page shared by several NPCs
		// would continue into an unrelated NPC's menu.
		if family != "" && strings.Split(keys[i], "-")[0] != family {
			continue
		}
		steps, ok := record.Variants[keys[i]]
		if !ok {
			continue
		}
		if initial {
			speakers := sourceSpeakers(steps)
			if len(speakers) != 1 || !speakers[actor] {
				continue
			}
		}
		found, ambiguous := topLevelMenu(steps, initial)
		if ambiguous {
			// An ambiguous nearer variant rules out older
			// menus.
			menu = nil
		} else if found != nil {
			menu = found
		}
	}
	return menu
}

// combatLevelCondition ports combatLevelCondition
// (NpcDialogues.plugin.js:454-457): the quest-start warning's
// exact wiki condition, answered from the player's combat level.
// It returns answered=false for any other wording.
var combatLevelConditionPattern = regexp.MustCompile(`^If the player's combat level is lower than (\d+):$`)

func combatLevelCondition(player *Player, text string) (bool, bool) {
	match := combatLevelConditionPattern.FindStringSubmatch(text)
	if match == nil || player == nil || player.Skills() == nil {
		return false, false
	}
	var level int
	for _, ch := range match[1] {
		level = level*10 + int(ch-'0')
	}
	return player.Skills().CombatLevel() < level, true
}

// skillLevelCondition ports skillLevelCondition
// (NpcDialogues.plugin.js:466-474): the bare skill-level wording,
// answered from the player's base (max) level in that skill.
// Plugins answer boostable conditions first; this settles only
// the bare form. It returns answered=false for any other wording
// or an unknown skill name.
var skillLevelConditionPattern = regexp.MustCompile(`^If the player has (at least|under|less than) (\d+) ([A-Z][a-z]+):$`)

func skillLevelCondition(player *Player, text string) (bool, bool) {
	match := skillLevelConditionPattern.FindStringSubmatch(text)
	if match == nil || player == nil || player.Skills() == nil {
		return false, false
	}
	skill := -1
	for id, name := range equipmentRequirementSkillNames {
		if strings.EqualFold(name, match[3]) {
			skill = id
			break
		}
	}
	if skill < 0 {
		return false, false
	}
	var level int
	for _, ch := range match[2] {
		level = level*10 + int(ch-'0')
	}
	if match[1] == "at least" {
		return player.Skills().MaxLevel(skill) >= level, true
	}
	return player.Skills().MaxLevel(skill) < level, true
}

// jsNumberToInt converts a JSON-decoded expression value to the
// integer TS Number.isInteger would accept, or reports it
// invalid. Expression values arrive as float64 from the loader.
func dialogueExpressionInt(value float64) (int, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) {
		return 0, false
	}
	return int(value), true
}

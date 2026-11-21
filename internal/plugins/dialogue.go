// Dialogue hooks: the Go form of the two dialogue families of
// PluginApi (PluginTypes.ts) that the transcript playback can
// fire since phase 14: onNpcDialogueVariant and
// onNpcDialogueCondition, plus the npc:operated custom event the
// NPC-click dispatch emits and the npc-dialogue:* custom-event
// names the playback uses.
//
// Every emit below ports its PluginManager.emit* body, including
// the first-answer shape and the throw-and-continue failure mode
// — the dialogue families differ from every other family in both
// respects, and those differences are behavior content relies on:
//
//   - emitNpcDialogueVariant (PluginManager.ts:1289-1304):
//     hooks run in registration order; the first TRUTHY result
//     wins (a non-empty variant name, or a {page?, variant}
//     object — an object is truthy even with an empty variant)
//     and later hooks never run; no answer yields null. A
//     throwing hook is warned as "[plugins] npc dialogue
//     variant hook threw (<pluginName>)" and the gate continues.
//   - emitNpcDialogueCondition (PluginManager.ts:1306-1320):
//     hooks run in registration order; the first BOOLEAN result
//     wins and later hooks never run; no answer yields null. A
//     throwing hook is warned as "[plugins] npc dialogue
//     condition hook threw (<pluginName>)" and the gate
//     continues.
//
// Both emits run through runFirstDialogueAnswer (gates.go),
// whose contract names these two families. Dialogue hooks do
// NOT go through executeHook: the "threw" warning (not "hook
// failed") and the continue-on-throw are TS-exact.
//
// Go shapes for the TS return unions: a variant handler returns
// *NpcDialogueVariantChoice — nil means "no answer"
// (null/undefined, or the falsy empty string), non-nil means
// "answer" (a name, or a {page, variant} object, including an
// object with an empty variant, which TS treats as truthy).
// DialogueVariant and DialogueVariantOnPage build the two TS
// answer forms. A condition handler returns *bool — nil means
// "no answer" (null/undefined), non-nil means the branch choice.
// The emits return the same shapes: nil when no hook answered.
//
// EMIT-SITE SEAM (game never imports plugins): the transcript
// playback lives in internal/game (Worker B) and cannot call
// these emits. It accepts func fields — installed by the
// cmd/server adapter, following the exact Phase 13 pattern
// (listeners.ObjectHooks + cmd/server/objectlistenerwiring.go:
// narrow func fields on the caller's side, translation adapters
// in the package that imports both sides):
//
//   - Variant selection: playback calls its installed selector
//     with its own context; the adapter translates the fields
//     below into an NpcDialogueContext, calls
//     Hub.EmitNpcDialogueVariant, and translates the choice
//     back (nil = no hook answered; the playback default
//     applies). Field map: player, npc, npcId, definition
//     (through the NpcDefinition interface —
//     *definition.NpcDefinition satisfies it structurally),
//     pages (page name + variant names each page offers).
//   - Condition answers: playback calls its installed answerer
//     with its context + the prose text + the anchor step id;
//     the adapter builds an NpcDialogueConditionEvent, calls
//     Hub.EmitNpcDialogueCondition, and returns the answer
//     (nil = no hook answered; the playback defaults to the
//     first branch).
//   - npc-dialogue:* custom events: playback calls its
//     installed emitter with (name, payload); the adapter calls
//     Hub.EmitCustomEvent. Payload types are owned by the
//     playback (internal/game); the hub passes them opaquely.
//   - npc-dialogue:start subscriptions: content subscribes
//     through Api.OnCustomEvent (see below); the coordinator
//     subscribes the playback entry point the same way, in
//     cmd/server, where both sides are importable.
//
// npc-dialogue:* CUSTOM EVENTS: NpcDialogues.plugin.js uses
// ONLY the custom-event surface for these — there is no
// dedicated registration helper in TS (the :start subscription
// is api.onCustomEvent("npc-dialogue:start", ...),
// NpcDialogues.plugin.js:1378). The names below are the eight
// the playback emits/subscribes, grounded in that file; each
// matches the custom-event name rule, so Api.OnCustomEvent and
// Hub.EmitCustomEvent cover them with no new mechanism.
// Payload shapes live with the playback; see the seam note
// above.
//
// NOT PORTED — PluginApi/PluginCoreApi dialogue members:
// there are NO dialogue members on PluginApi to port.
// getDialogueManager exists only as a Player method
// (Player.ts:1642: getDialogueManager(): DialogueManager);
// content reaches it through the *game.Player in event
// payloads once Worker B adds the Go method — it is game API,
// not hub API. PluginCoreApi exposes the dialogue classes
// (PluginTypes.ts:1745-1754: DialogueChainBuilder, NpcDialogue,
// PlayerDialogue, OptionDialogue, StatementDialogue,
// ItemStatementDialogue, DoubleItemStatementDialogue,
// ActionDialogue, EndDialogue, CreationMenu), but none of those
// types exists in Go yet (Worker B's DialogueManager work),
// so no Core accessor is added: core.go keeps them in its
// NOT-mirrored list until the game types land. The PluginApi
// cross-plugin emits likewise have no dialogue members —
// emitNpcDialogueVariant/emitNpcDialogueCondition are
// PluginManager statics only, called by the playback, never
// exposed on PluginApi — so the non-hook member count is
// unchanged (11 of 95). The two hook families move the firable
// count to 21 of 108 (see the unfired-family table in hooks.go).

package plugins

import (
	"tsps-go/internal/game"
)

// NpcDefinition is the minimal definition surface the dialogue
// context needs: the NpcDefinition accessors the playback and
// variant/condition handlers read (NpcDefinition.ts: getName,
// getActions, getSize, isAttackable). The resolved (current)
// definition is what the playback supplies. The definition
// package's NpcDefinition satisfies this interface structurally;
// this package never imports that package.
type NpcDefinition interface {
	// ID is the definition's id.
	ID() int
	// Name is the definition's name (getName).
	Name() string
	// Actions are the menu actions (getActions), or nil when
	// the NPC has none. Slots with no action read "".
	Actions() []string
	// Size is the footprint size in tiles (getSize).
	Size() int
	// IsAttackable reports whether the NPC can be attacked
	// (isAttackable).
	IsAttackable() bool
}

// NpcDialoguePage is one transcript page registered for an NPC
// id, with the variant names it offers: one entry of the
// `pages` array of PluginNpcDialogueContext
// (PluginTypes.ts:206-213).
type NpcDialoguePage struct {
	Page     string
	Variants []string
}

// NpcDialogueContext is the payload of the
// onNpcDialogueVariant hooks. It mirrors
// PluginNpcDialogueContext (PluginTypes.ts:206-213): the
// player, the NPC, the NPC id, the resolved definition (nil
// when unknown — TS definition is optional), and the
// transcript pages registered for the id. NpcID is the
// content id (NPC.getContentId); for Go NPCs without
// transforms it equals the NPC's definition id.
type NpcDialogueContext struct {
	Player     *game.Player
	NPC        *game.NPC
	NpcID      int
	Definition NpcDefinition
	Pages      []NpcDialoguePage
}

// NpcDialogueConditionEvent is the payload of the
// onNpcDialogueCondition hooks. It mirrors
// PluginNpcDialogueConditionEvent (PluginTypes.ts:216-222):
// the dialogue context, plus the wiki prose text the runtime
// needs answered and the anchor id of the condition step,
// when the export provides one (HasStepID distinguishes
// "unset" from an empty id).
type NpcDialogueConditionEvent struct {
	NpcDialogueContext
	// Text is the wiki prose, e.g. "If the player already
	// has the necessary items:".
	Text string
	// StepID is the anchor id of the condition step, when
	// the export provides one.
	StepID    string
	HasStepID bool
}

// NewNpcDialogueConditionEvent builds a condition event from a
// dialogue context, the prose text, and the anchor step id.
// hasStepID reports whether stepID was provided by the export.
func NewNpcDialogueConditionEvent(ctx NpcDialogueContext, text, stepID string, hasStepID bool) *NpcDialogueConditionEvent {
	return &NpcDialogueConditionEvent{
		NpcDialogueContext: ctx,
		Text:               text,
		StepID:             stepID,
		HasStepID:          hasStepID,
	}
}

// NpcDialogueVariantChoice is one variant answer: the TS
// `string | { page?: string; variant: string }` union
// (PluginTypes.ts:1112-1114) as a struct. A bare-name answer
// carries Variant with HasPage false; a page-qualified answer
// carries both. Variant may be "" — a non-nil choice with an
// empty variant is the TS `{ variant: "" }` object, which is
// truthy and therefore an answer (the playback then applies
// its no-wanted-variant default).
type NpcDialogueVariantChoice struct {
	Page    string
	HasPage bool
	Variant string
}

// DialogueVariant builds a bare-name variant answer, mirroring
// the TS `return "<name>"` form. An empty name yields nil:
// the empty string is falsy in TS and falls through to the
// next hook (PluginManager.ts:1295).
func DialogueVariant(variant string) *NpcDialogueVariantChoice {
	if variant == "" {
		return nil
	}
	return &NpcDialogueVariantChoice{Variant: variant}
}

// DialogueVariantOnPage builds a page-qualified variant
// answer, mirroring the TS `return { page, variant }` form.
// The result is always non-nil: an object is truthy in TS
// even with an empty variant (PluginManager.ts:1295).
func DialogueVariantOnPage(page, variant string) *NpcDialogueVariantChoice {
	return &NpcDialogueVariantChoice{Page: page, HasPage: true, Variant: variant}
}

// NpcDialogueVariantHandler picks which transcript variant a
// Talk-to should play. It mirrors the onNpcDialogueVariant
// handler shape (PluginTypes.ts:1111-1114): return a variant
// name (DialogueVariant), a page-qualified choice
// (DialogueVariantOnPage), or nil to fall through to the next
// hook. A non-nil choice wins immediately; later hooks never
// run.
type NpcDialogueVariantHandler func(*NpcDialogueContext) *NpcDialogueVariantChoice

// NpcDialogueConditionHandler answers a wiki prose condition
// step. It mirrors the onNpcDialogueCondition handler shape
// (PluginTypes.ts:1120-1122): return a *bool to choose the
// branch, or nil to fall through (the runtime then defaults
// to the first branch). The first non-nil answer wins; later
// hooks never run.
type NpcDialogueConditionHandler func(*NpcDialogueConditionEvent) *bool

// OnNpcDialogueVariant registers a transcript-variant hook,
// mirroring onNpcDialogueVariant (PluginTypes.ts:1108-1114).
// Hooks run in registration order until one answers
// (EmitNpcDialogueVariant). A nil handler is ignored
// silently, mirroring the createApi guard
// (PluginManager.ts:3511-3515).
func (a *Api) OnNpcDialogueVariant(handler NpcDialogueVariantHandler) {
	if handler == nil {
		return
	}
	if !a.canRegister("onNpcDialogueVariant") {
		return
	}
	a.hub.hooks.addNpcDialogueVariant(dialogueHook[*NpcDialogueContext, *NpcDialogueVariantChoice]{pluginName: a.name, handler: handler})
}

// OnNpcDialogueCondition registers a condition-answer hook,
// mirroring onNpcDialogueCondition (PluginTypes.ts:1116-1122).
// Hooks run in registration order until one answers
// (EmitNpcDialogueCondition). A nil handler is ignored
// silently, mirroring the createApi guard
// (PluginManager.ts:3517-3521).
func (a *Api) OnNpcDialogueCondition(handler NpcDialogueConditionHandler) {
	if handler == nil {
		return
	}
	if !a.canRegister("onNpcDialogueCondition") {
		return
	}
	a.hub.hooks.addNpcDialogueCondition(dialogueHook[*NpcDialogueConditionEvent, *bool]{pluginName: a.name, handler: handler})
}

// EmitNpcDialogueVariant asks the registered variant hooks
// which transcript variant to play and returns the first
// answer, mirroring PluginManager.emitNpcDialogueVariant
// (PluginManager.ts:1289-1304): the first truthy result wins,
// later hooks never run, and no answer yields nil. A throwing
// hook is warned and skipped (see runFirstDialogueAnswer).
// A nil Hub, a Hub without a registry, or a nil event yields
// nil (the nil-event guard is Go-only; TS has no nil-event
// equivalent).
//
// The production call site is the transcript playback, through
// the cmd/server adapter (see the file header): the playback
// never calls this method directly, because game never
// imports plugins.
func (h *Hub) EmitNpcDialogueVariant(event *NpcDialogueContext) *NpcDialogueVariantChoice {
	if h == nil || h.hooks == nil || event == nil {
		return nil
	}
	choice, answered := runFirstDialogueAnswer(h.Logger(), "npc dialogue variant", h.hooks.snapshotNpcDialogueVariant(), event,
		func(choice *NpcDialogueVariantChoice) bool { return choice != nil })
	if !answered {
		return nil
	}
	return choice
}

// EmitNpcDialogueCondition asks the registered condition hooks
// to answer a wiki prose condition and returns the first
// boolean answer, mirroring
// PluginManager.emitNpcDialogueCondition
// (PluginManager.ts:1306-1320): the first boolean wins, later
// hooks never run, and no answer yields nil. A throwing hook
// is warned and skipped (see runFirstDialogueAnswer). A nil
// Hub, a Hub without a registry, or a nil event yields nil
// (the nil-event guard is Go-only; TS has no nil-event
// equivalent).
//
// The production call site is the transcript playback, through
// the cmd/server adapter (see the file header).
func (h *Hub) EmitNpcDialogueCondition(event *NpcDialogueConditionEvent) *bool {
	if h == nil || h.hooks == nil || event == nil {
		return nil
	}
	answer, answered := runFirstDialogueAnswer(h.Logger(), "npc dialogue condition", h.hooks.snapshotNpcDialogueCondition(), event,
		func(answer *bool) bool { return answer != nil })
	if !answered {
		return nil
	}
	return answer
}

// npc-dialogue:* custom-event names, grounded in
// NpcDialogues.plugin.js. Content subscribes through
// Api.OnCustomEvent and the playback emits through
// Hub.EmitCustomEvent (via the cmd/server adapter); there is
// no dedicated registration helper in TS. Payload shapes are
// owned by the playback (internal/game) and pass through the
// hub opaquely as `any`.
const (
	// NpcDialogueStartEventName plays one named transcript
	// variant on demand (NpcDialogues.plugin.js:1378).
	NpcDialogueStartEventName = "npc-dialogue:start"
	// NpcDialogueShopEventName picks one shop when a shop
	// step offers several (NpcDialogues.plugin.js:572).
	NpcDialogueShopEventName = "npc-dialogue:shop"
	// NpcDialogueMenuEventName lets a handler ask a menu
	// question itself, warn first, or let the menu play
	// (NpcDialogues.plugin.js:782).
	NpcDialogueMenuEventName = "npc-dialogue:menu"
	// NpcDialogueHookEventName runs hook steps
	// (NpcDialogues.plugin.js:827, 1056).
	NpcDialogueHookEventName = "npc-dialogue:hook"
	// NpcDialogueChoiceEventName observes choice steps
	// (NpcDialogues.plugin.js:832).
	NpcDialogueChoiceEventName = "npc-dialogue:choice"
	// NpcDialogueConditionEventName observes condition steps
	// before the condition hooks answer
	// (NpcDialogues.plugin.js:899).
	NpcDialogueConditionEventName = "npc-dialogue:condition"
	// NpcDialogueLineEventName observes speech lines, with a
	// mutable skip (NpcDialogues.plugin.js:928).
	NpcDialogueLineEventName = "npc-dialogue:line"
	// NpcDialogueActionEventName observes message/action
	// steps (NpcDialogues.plugin.js:1071, 1120).
	NpcDialogueActionEventName = "npc-dialogue:action"
)

// NpcOperatedEvent is the payload of the "npc:operated"
// custom event (NPCOptionPacketListener.ts:106-112): an NPC
// menu option something answered — a dialogue start, or a
// plugin-claimed click. A click nothing answered stays
// silent and emits nothing (the fallback message is not an
// operated event). Option is the lower-cased option text; it
// may be absent (HasOption false) when the clicked slot
// carries no action text.
type NpcOperatedEvent struct {
	Player    *game.Player
	NPC       *game.NPC
	NpcID     int
	Option    string
	HasOption bool
}

// NpcOperatedEventName is the custom-event name for operated
// NPCs (NPCOptionPacketListener.OPERATED_EVENT).
const NpcOperatedEventName = "npc:operated"

// EmitNpcOperated emits the npc:operated custom event with the
// given payload. A nil hub or a nil event emits nothing.
func (h *Hub) EmitNpcOperated(event *NpcOperatedEvent) {
	if event == nil {
		return
	}
	h.EmitCustomEvent(NpcOperatedEventName, event)
}

// SetMultiChatboxPrompt installs the MultiChatboxPrompt registry
// content shows prompts through Api.SendMultiChatboxPrompt. The
// coordinator calls it once at boot, after the registry exists;
// calling it with nil clears the surface. It is a no-op on a
// nil Hub or a Hub without a registry.
func (h *Hub) SetMultiChatboxPrompt(registry *game.MultiChatboxPrompt) {
	if h == nil || h.hooks == nil {
		return
	}
	h.hooks.setMultiChatboxPrompt(registry)
}

// MultiChatboxPrompt returns the installed prompt registry, or
// nil when none is installed (or the Hub is nil).
func (h *Hub) MultiChatboxPrompt() *game.MultiChatboxPrompt {
	if h == nil || h.hooks == nil {
		return nil
	}
	return h.hooks.multiChatboxPrompt()
}

// PromptCallback is the option callback shape of
// sendMultiChatboxPrompt (PluginTypes.ts:1225-1231): the player,
// the zero-based option index, and the option text.
type PromptCallback func(player *game.Player, optionIndex int, optionText string)

// SendMultiChatboxPrompt shows a multi-option chatbox prompt,
// mirroring the PluginApi sendMultiChatboxPrompt member
// (PluginTypes.ts:1225, PluginManager.ts:4360-4372). The pairs
// alternate option text (string) and callback (PromptCallback),
// exactly as the TS variadic; an odd count, a non-string text,
// or a non-callback value reports false without showing
// anything, as TS showPrompt's pair validation does. The
// registry re-validates (blank title, blank labels, more than 5
// options) and pads a single option with "Cancel". A nil Api,
// a nil hub, or no installed registry reports false. This is a
// call, not a registration: it has no load-phase gate,
// mirroring the TS closure, which has no phase check.
func (a *Api) SendMultiChatboxPrompt(player *game.Player, title string, pairs ...any) bool {
	if a == nil || a.hub == nil || player == nil {
		return false
	}
	registry := a.hub.MultiChatboxPrompt()
	if registry == nil {
		return false
	}
	if len(pairs) < 2 || len(pairs)%2 != 0 {
		return false
	}
	options := make([]game.MenuOption, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		text, ok := pairs[i].(string)
		if !ok {
			return false
		}
		callback, ok := pairs[i+1].(PromptCallback)
		if !ok {
			return false
		}
		index, label, cb := len(options), text, callback
		options = append(options, game.MenuOption{
			Label:  label,
			Select: func() { cb(player, index, label) },
		})
	}
	return registry.ShowPrompt(a.name, player, title, options)
}

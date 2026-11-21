package game

import (
	"strings"
	"sync"
	"time"
)

// MultiChatboxPrompt is the port of the TS MultiChatboxPrompt
// (game/model/menu/MultiChatboxPrompt.ts): the multi-option chatbox
// prompt behind PluginApi.sendMultiChatboxPrompt
// (PluginTypes.ts:1225, PluginManager.ts:4360-4372). It is a
// SEPARATE mechanism from the DialogueManager's OptionDialogue
// entry: showing a prompt resets the manager, mounts group 219
// with the option list, and records the pending prompt; the
// player's pick arrives as a dialogue_continue packet, which the
// dispatch routes through emitInterfaceActionClick BEFORE the
// manager's handleResume (NetworkBuilder.ts:939-951,
// PluginManager.ts:2089), so a pending prompt consumes the resume
// and a native dialogue never sees it.
//
// This is the presenter the transcript playback's choice steps
// need (NpcDialogues.plugin.js:846-848 calls
// api.sendMultiChatboxPrompt for every menu): TranscriptDeps'
// PresentMenu seam is satisfied by ShowPrompt, and the resume
// dispatch checks HandleInterfaceActionClick first (see
// cmd/server/dialoguewiring.go).
//
// DEVIATION: TS keeps pending prompts in a static WeakMap keyed by
// player, so entries vanish with the player. Go has no weak map,
// so this registry holds a mutex-guarded map keyed by *Player
// (never by player index — indexes are reused), prunes expired
// entries on every access, and offers Forget for disconnect
// cleanup. A disconnected player's entry lingers until the next
// access past its 10-minute TTL prunes it; the map is bounded by
// the number of players who opened a prompt within the TTL.
//
// The registry is safe for concurrent use: ShowPrompt runs on the
// tick goroutine (playback) or the login goroutine (content
// login hooks), while HandleInterfaceActionClick runs on the
// packet-dispatch path. Callbacks run WITHOUT the registry lock
// held: a callback re-enters playback, which may show another
// prompt.
type MultiChatboxPrompt struct {
	mu      sync.Mutex
	pending map[*Player]*pendingPrompt

	// now returns the current time; injectable for tests. A nil
	// value reads as time.Now.
	now func() time.Time
	// logf reports validation failures and callback panics
	// (TS console.warn / console.error); nil drops them.
	logf func(format string, args ...any)
}

// pendingPrompt is one shown prompt awaiting a pick
// (PendingMultiChatboxPrompt in TS).
type pendingPrompt struct {
	pluginName string
	title      string
	options    []MenuOption
	expiresAt  time.Time
}

// PromptTTL is the pending prompt lifetime (PROMPT_TTL_MS = 10
// minutes, MultiChatboxPrompt.ts:26).
const PromptTTL = 10 * time.Minute

// NewMultiChatboxPrompt returns an empty prompt registry. A nil
// logf drops validation/callback diagnostics.
func NewMultiChatboxPrompt(logf func(format string, args ...any)) *MultiChatboxPrompt {
	return &MultiChatboxPrompt{pending: make(map[*Player]*pendingPrompt), logf: logf}
}

func (r *MultiChatboxPrompt) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

func (r *MultiChatboxPrompt) warn(format string, args ...any) {
	if r.logf != nil {
		r.logf(format, args...)
	}
}

// ShowPrompt shows a multi-option chatbox prompt and records it
// pending (MultiChatboxPrompt.showPrompt, lines 28-105). It
// reports false, showing nothing, when the player has no
// DialogueSender, the title is blank, the option list is empty,
// any option label is blank or has no callback, or more than 5
// options are offered (TS logs a warning for the invalid
// registration in each case). A single option is padded with a
// no-op "Cancel" option, because cache script 58 always renders
// at least two options. Showing a prompt resets the player's
// dialogue manager first: a multi prompt takes ownership from
// any native dialogue that showed it.
//
// The frames are exactly the OptionDialogue frame sequence
// (script 2379, chatbox group 219 unclamped, client script 58
// with title + "|"-joined options, flags range over the option
// slots), sent through SendOptionDialogue.
func (r *MultiChatboxPrompt) ShowPrompt(pluginName string, player *Player, title string, options []MenuOption) bool {
	if r == nil || player == nil {
		return false
	}
	sender := player.dialogueSender()
	if sender == nil || strings.TrimSpace(title) == "" {
		return false
	}
	if len(options) == 0 {
		r.warn("[plugins] %s attempted invalid sendMultiChatboxPrompt registration", pluginName)
		return false
	}
	for i, option := range options {
		if strings.TrimSpace(option.Label) == "" || option.Select == nil {
			r.warn("[plugins] %s attempted invalid sendMultiChatboxPrompt option pair at index %d", pluginName, i*2)
			return false
		}
	}
	if len(options) > 5 {
		r.warn("[plugins] %s attempted unsupported sendMultiChatboxPrompt option count=%d", pluginName, len(options))
		return false
	}
	shown := options
	if len(shown) == 1 {
		shown = append(append([]MenuOption(nil), shown...), MenuOption{Label: "Cancel", Select: func() {}})
	}

	// A multi prompt takes ownership from any native dialogue.
	player.GetDialogueManager().Reset()
	labels := make([]string, len(shown))
	for i, option := range shown {
		labels[i] = option.Label
	}
	SendOptionDialogue(player, title, labels)

	r.mu.Lock()
	r.pruneLocked(r.clock())
	r.pending[player] = &pendingPrompt{
		pluginName: pluginName,
		title:      title,
		options:    shown,
		expiresAt:  r.clock().Add(PromptTTL),
	}
	r.mu.Unlock()
	return true
}

// GetPending returns the prompt the player has open, if any
// (MultiChatboxPrompt.getPending). A stale prompt (expired, or
// superseded by a native dialogue or another chatbox group) is
// dropped and reported absent, exactly as the ownership check
// does.
func (r *MultiChatboxPrompt) GetPending(player *Player) (title string, options []string, ok bool) {
	pending := r.ownedPrompt(player)
	if pending == nil {
		return "", nil, false
	}
	labels := make([]string, len(pending.options))
	for i, option := range pending.options {
		labels[i] = option.Label
	}
	return pending.title, labels, true
}

// HandleInterfaceActionClick answers an interface action click
// (MultiChatboxPrompt.handleInterfaceActionClick, lines
// 130-148): the dialogue_continue dispatch calls it, through
// emitInterfaceActionClick, before the dialogue manager's
// handleResume. action is the resume packet's childIndex —
// option n is picked with childIndex n (the OPTIONS_WIDGET_ID
// comment, line 23) — so the selected option is action-1. It
// reports false when no prompt is owned, when the click is not
// on the options widget or group 219, or when the option number
// is out of range; a consumed click closes the interface and
// runs the callback.
//
// NOTE: TS takes the option number from event.slot when it
// is an integer, else event.action. The hub emit
// (Hub.EmitInterfaceActionClick) resolves that choice and
// passes the option number as this method's action argument,
// so the slot form arrives here through the interface-action
// dispatch (phase 18) and the dialogue_continue (action) form
// arrives through the resume path.
func (r *MultiChatboxPrompt) HandleInterfaceActionClick(player *Player, buttonID, action, groupID int) bool {
	pending := r.ownedPrompt(player)
	if pending == nil {
		return false
	}
	if buttonID != OptionsWidgetID && groupID != optionDialogueGroupID {
		return false
	}
	return r.selectOption(player, pending, action-1)
}

// selectOption runs the option at index, if in range
// (MultiChatboxPrompt.selectOption, lines 149-174): the pending
// prompt is deleted first, the interface is removed, and the
// callback runs. A callback panic is recovered and logged — TS
// catches callback errors and logs them — and the click still
// counts as consumed.
func (r *MultiChatboxPrompt) selectOption(player *Player, pending *pendingPrompt, index int) bool {
	if index < 0 || index >= len(pending.options) {
		return false
	}
	r.mu.Lock()
	// Only delete when the pending prompt is still this one: a
	// re-shown prompt must not be dropped by a stale pick.
	if r.pending[player] == pending {
		delete(r.pending, player)
	}
	r.mu.Unlock()
	selected := pending.options[index]
	if sender := player.dialogueSender(); sender != nil {
		sender.SendInterfaceRemoval()
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				r.warn("[plugins] multi_chatbox_prompt callback failed (%s): %v", pending.pluginName, recovered)
			}
		}()
		selected.Select()
	}()
	return true
}

// ownedPrompt returns the player's live pending prompt, or nil
// (MultiChatboxPrompt.ownedPrompt, lines 117-127). A prompt is
// stale — and deleted — when expired, when the dialogue manager
// is active (a later native dialogue must not resume this
// prompt's callbacks), or when group 219 is no longer the
// mounted chatbox group.
func (r *MultiChatboxPrompt) ownedPrompt(player *Player) *pendingPrompt {
	if r == nil || player == nil {
		return nil
	}
	r.mu.Lock()
	pending, ok := r.pending[player]
	r.mu.Unlock()
	if !ok {
		return nil
	}
	sender := player.dialogueSender()
	stale := pending.expiresAt.Before(r.clock()) ||
		player.GetDialogueManager().IsActive() ||
		sender == nil || !sender.IsChatboxInterface(optionDialogueGroupID)
	if stale {
		r.mu.Lock()
		if r.pending[player] == pending {
			delete(r.pending, player)
		}
		r.mu.Unlock()
		return nil
	}
	return pending
}

// Forget drops the player's pending prompt, if any: the Go
// stand-in for the TS WeakMap entry vanishing with the player.
// Call it when the player disconnects; expiry pruning covers
// callers that never do.
func (r *MultiChatboxPrompt) Forget(player *Player) {
	if r == nil || player == nil {
		return
	}
	r.mu.Lock()
	delete(r.pending, player)
	r.mu.Unlock()
}

// pruneLocked drops expired entries. The caller holds r.mu.
func (r *MultiChatboxPrompt) pruneLocked(now time.Time) {
	for player, pending := range r.pending {
		if pending.expiresAt.Before(now) {
			delete(r.pending, player)
		}
	}
}

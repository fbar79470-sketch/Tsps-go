// Chat state on mobiles and players: overhead (forced) chat, chat
// icons, and the public-chat receive gate.
//
// TS grounding (commit bfd81e3):
//   - Mobile.forcedChat + forceChat/getForcedChat/setForcedChat,
//     Mobile.ts:76, 334-338, 693-700; cleared by resetUpdating,
//     Mobile.ts:264-278 (Go: Mobile.ResetUpdating in entity.go).
//   - Player.chatIcons + getChatIcons/setChatIcons, Player.ts:237,
//     1015-1024.
//   - PlayerRelations.canReceivePublicChatFrom, PlayerRelations.ts:90-97.
//
// Threading: forced chat is tick-goroutine-only, like Flags and the
// step directions. It is written during turn processing (the chat
// listener runs from Player.Process in the process_players phase),
// read by the flush phase later in the same tick (the flush source
// snapshots it into the sync view), and cleared by ResetUpdating in
// the reset phase after every observer has built its frame
// (World.process order, World.ts:1400-1445; Go: World.Process).
// No mutex is needed: unlike the attribute store (attributes.go),
// which login-goroutine emits also write, no path outside the tick
// goroutine touches forced chat. Chat icons use the attribute
// store's lazy-pointer-plus-mutex pattern instead: content may
// set them from a login-goroutine hook while the tick goroutine
// reads them for a chat line. The receive gate reads the live
// relations state (relations.go), which is tick-goroutine-owned
// like the rest of Player state; see the threading note there.

package game

import "sync"

// ForceChat sets this tick's overhead chat text and flags
// FlagForcedChat, so the next sync frame carries the text in the
// actor's update block (Mobile.ts:334-338: setForcedChat, then
// flag FORCED_CHAT). A second call in the same tick replaces the
// text; the flag stays set. The text is cleared, with the flag, by
// ResetUpdating at the end of the tick — overhead chat lasts
// exactly one sync frame. Mobile-level, as in TS: NPCs force-chat
// through the same field and flag (their producers are combat/NPC
// content, NOT STARTED, so no NPC sets one yet).
func (m *Mobile) ForceChat(message string) {
	m.forcedChat = &message
	m.Flags = m.Flags.Set(FlagForcedChat)
}

// GetForcedChat returns this tick's overhead chat text, or nil when
// the mobile has none (Mobile.ts:693-695: getForcedChat returns
// the field, null until set). The sync view includes the text only
// when the FlagForcedChat flag is also set AND the text is non-nil
// (PlayerSession.ts:818-820, createActorUpdates); callers must
// check both, as the flag is the "send this tick" marker and the
// text is the payload.
//
// There is deliberately no SetForcedChat: TS defines one
// (Mobile.ts:697-700) but its only caller is forceChat itself
// (verified by grep at commit bfd81e3), and setting the text
// without the flag would produce state the sync gate never sends.
func (m *Mobile) GetForcedChat() *string {
	return m.forcedChat
}

// chatIconStore is one player's chat-icon list plus its guarding
// mutex, allocated lazily on the first SetChatIcons (see the file
// header). The pointer form keeps Player copyable by value: the
// mutex itself is never copied.
type chatIconStore struct {
	mu    sync.RWMutex
	icons []int
}

// chatIconAllocMu serializes lazy allocation of chat-icon stores
// and every load of a Player's store pointer, mirroring
// attributeAllocMu in attributes.go.
var chatIconAllocMu sync.Mutex

// iconStore returns p's chat-icon store, allocating it when
// create is true and none exists yet. It returns nil when no
// store exists and create is false. A nil receiver has no store.
func (p *Player) iconStore(create bool) *chatIconStore {
	if p == nil {
		return nil
	}
	chatIconAllocMu.Lock()
	defer chatIconAllocMu.Unlock()
	if p.chatIcons == nil && create {
		p.chatIcons = &chatIconStore{}
	}
	return p.chatIcons
}

// ChatIcons returns the chat-icon (crown) ids rendered before the
// player's name in chat (Player.ts:1015-1018, getChatIcons). The
// result is a copy: TS returns the live array, but Go callers must
// not mutate player state through the accessor. Empty until
// content sets icons through SetChatIcons — no core code ever
// sets them, so crowns never render from core behavior alone and
// autochat takes MODAUTOTYPER only when content has set icons.
func (p *Player) ChatIcons() []int {
	s := p.iconStore(false)
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.icons) == 0 {
		return nil
	}
	return append([]int(nil), s.icons...)
}

// SetChatIcons replaces the player's chat icons, keeping only
// integer ids 0..255, at most 255 (Player.ts:1019-1024,
// setChatIcons). Go ints are always integers, so only the range
// filter and the cap apply. No core callers exist; content sets
// icons (e.g. staff crowns) through this method.
func (p *Player) SetChatIcons(icons []int) {
	s := p.iconStore(true)
	if s == nil {
		return
	}
	kept := make([]int, 0, len(icons))
	for _, icon := range icons {
		if icon >= 0 && icon <= 255 {
			kept = append(kept, icon)
		}
	}
	if len(kept) > 255 {
		kept = kept[:255]
	}
	s.mu.Lock()
	s.icons = kept
	s.mu.Unlock()
}

// CanReceivePublicChatFrom reports whether p's client accepts a
// public-chat line from other: the PlayerRelations gate
// (PlayerRelations.ts:90-96) read live from p's relations state
// (relations.go) — the ignore list, the friend list, and the public
// chat mode, with the autoChat rule.
//
// TS rule: ignored senders never deliver; mode 4 (Show autochat)
// accepts everything including autochat; autochat is otherwise
// rejected; modes 0 (Show standard) and 1 (Show friends, friends
// only) accept standard chat, modes 2/3 accept nothing. A nil
// receiver or a nil sender never delivers. The method does not
// exempt the speaker; the broadcast loop applies the self exemption
// (ChatPacketListener.ts:63 skips the gate for recipient == speaker),
// and the sync-path overhead gate calls it with autoChat=false
// (PlayerSession.ts:815-817, createActorUpdates).
//
// This closed the Phase 12 degeneration, in which relations state
// did not exist and the rule degenerated to "!autoChat" for any
// non-nil sender. The degeneration was exact for a fresh player
// (empty lists, mode 0); the live gate agrees with it there and
// follows the lists and mode once they change.
func (p *Player) CanReceivePublicChatFrom(other *Player, autoChat bool) bool {
	if p == nil || other == nil {
		return false
	}
	return p.Relations().CanReceivePublicChatFrom(other, autoChat)
}

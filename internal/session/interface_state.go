package session

import (
	"sort"

	"tsps-go/internal/definition"
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Interface runtime state: the session side of PacketSender's
// interface-state machinery (PacketSender.ts), holding the
// sub-interface tracking map and the wiring seams the sender
// methods in interface_sender.go use.
//
// The chatbox group/owner fields themselves live on Session
// (session.go, ported by the dialogue work); this file owns
// only the mount map, the spell-unlock flag, and the deps. All
// interfaceState fields are guarded by Session.mu: sender
// methods run from both the login goroutine (hooks) and the
// tick goroutine, and SendPacket takes mu itself, so methods
// snapshot or mutate state under mu, release it, and only then
// queue frames or invoke deps — never holding mu across a dep
// call or a hub emit, both of which can call back into the
// session.
//
// DEPS. PacketSender holds its player directly; Session holds
// none, so every player read/write arrives as a func field on
// InterfaceDeps, installed once at entry by the coordinator
// wiring (cmd/server/interfacewiring.go) and never reassigned
// afterwards. Every field is nil-safe: an unwired session
// degrades to send-and-track-only, with no player state touched
// and no events emitted. The deps snapshot is copied under mu,
// so a setter racing a send cannot tear a read.

// SubInterfaceMount is one tracked mount (SubInterfaceMount,
// PacketSender.ts:165-169): the target component, the mounted
// group, and the mount type. Mounts are stored as pointers so
// CloseSubInterfaceMount can compare identity: remounting the
// same group creates a new mount, and a delayed owner may close
// only its own still-current mount.
type SubInterfaceMount struct {
	TargetUID int
	GroupID   int
	Type      int
}

// InterfaceDeps is the player/hub seam for the interface
// runtime. See the file header: installed once at entry,
// nil-safe throughout.
//
//   - EmitInterfaceClosed / EmitInterfaceOpened emit the
//     "interface:closed" / "interface:opened" custom events
//     (PacketSender.ts:913-917, 1613-1618). The wiring builds
//     the game.InterfaceClosedEvent / InterfaceOpenedEvent
//     payloads with the owning player.
//   - GetInterfaceID / SetInterfaceID read and write the
//     player's interface id (Player.get/setInterfaceId); the
//     setter emits interface:closed/changed itself.
//   - ResetDialogue resets the dialogue manager;
//     DialogueOwner reports its chain owner (nil when
//     inactive), for closeChatboxFor's reset decision.
//   - HasInputPrompt / AbortEnteredActions are the player's
//     prompt reads (PacketSender.hasInputPrompt / the player
//     half of abortInputPrompt).
//   - CloseShop closes the player's active shop
//     (ShopManager.close, called from resetInterfaceState).
//   - SetStatusNone resets the player status to NONE. No Go
//     PlayerStatus field exists, so the wiring leaves this
//     nil; the slot exists so a future status port can fill
//     it without changing this API.
//   - SetDestroyPending sets the "destroy-item:pending"
//     attribute to -1 (resetInterfaceState).
//   - SetCanChangeAppearance clears the appearance-change
//     flag (resetInterfaceState).
//   - EndBankSearch ends the client's bank search when the
//     bank (interface 12) closes (PacketSender.endBankSearch,
//     PacketSender.ts:902-907). GAP: Go's Bank holds no search
//     state (only the search-tab container; no searching flag
//     or closeSearch), so the wiring leaves this nil and the
//     close sends no meslayer-close script. Wiring it needs a
//     bank-search port, not a seam fill.
//   - GameframeRoot reports the gameframe root id for tab
//     setup frames (resolveGameframeRoot). Nil means the 161
//     default (the stock resizable root); the bootstrap's
//     computed root is not retained on the session.
type InterfaceDeps struct {
	EmitInterfaceClosed    func(interfaceID int)
	EmitInterfaceOpened    func(groupID, targetUID, typ int)
	GetInterfaceID         func() int
	SetInterfaceID         func(id int)
	ResetDialogue          func()
	DialogueOwner          func() any
	HasInputPrompt         func() bool
	AbortEnteredActions    func()
	CloseShop              func()
	SetStatusNone          func()
	SetDestroyPending      func()
	SetCanChangeAppearance func(can bool)
	// SetCreationMenu installs or clears the player's open
	// creation menu (Player.setCreationMenu, Player.ts:1602-1603):
	// the send installs it (PacketSender.ts:1386), the close and
	// the interface-state reset clear it (PacketSender.ts:1418,
	// 894). Nil when unwired: the frames still go out, but the
	// player field is untouched.
	SetCreationMenu func(menu *game.CreationMenu)
	EndBankSearch   func(closedInterfaceID int)
	GameframeRoot   func() int
}

// interfaceState is the per-session interface runtime state.
// See the file header for the locking rules.
type interfaceState struct {
	// mounts tracks mounted groups by group id
	// (subInterfaceTargets, PacketSender.ts:224). A group
	// lives at one target: mounting it elsewhere closes the
	// old mount first (see rememberMountLocked).
	mounts map[int]*SubInterfaceMount
	// spellUnlocksSent records whether the tab-6 spell-unlock
	// varps/varbits went out (PacketSender.ts:223): the first
	// spellbook mount only.
	spellUnlocksSent bool
	deps             InterfaceDeps
	// layouts resolves configured-interface references for
	// SendConfiguredInterface. Nil means no layouts are
	// loaded and every reference is unknown.
	layouts definition.LayoutLookup
}

func newInterfaceState() *interfaceState {
	return &interfaceState{mounts: make(map[int]*SubInterfaceMount)}
}

// SetInterfaceDeps installs deps on the session. Called once at
// entry, before the session is shared; later calls replace the
// whole struct (the snapshot copy in interfaceDeps keeps reads
// consistent).
func (s *Session) SetInterfaceDeps(deps InterfaceDeps) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.iface == nil {
		s.iface = newInterfaceState()
	}
	s.iface.deps = deps
}

// SetInterfaceLayouts installs the layout lookup for
// SendConfiguredInterface. Called once at entry with the loaded
// definition Store (nil when no definitions loaded).
func (s *Session) SetInterfaceLayouts(lookup definition.LayoutLookup) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.iface == nil {
		s.iface = newInterfaceState()
	}
	s.iface.layouts = lookup
}

// interfaceDeps snapshots the installed deps under mu. An
// uninitialized state yields the zero deps (all nil).
func (s *Session) interfaceDeps() InterfaceDeps {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.iface == nil {
		return InterfaceDeps{}
	}
	return s.iface.deps
}

// interfaceLayouts returns the installed layout lookup, or nil.
func (s *Session) interfaceLayouts() definition.LayoutLookup {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.iface == nil {
		return nil
	}
	return s.iface.layouts
}

// rememberMountLocked tracks a mount (rememberSubInterface,
// PacketSender.ts:1578-1592). Call with mu held. A group lives
// at one target, so a mount of the same group at another target
// is closed there first: its close-sub frame is returned for
// the caller to queue. A different group mounted at this target
// is forgotten silently (the new mount replaces it visually;
// TS sends no close frame for it). The returned mount is the
// new identity for this mount, even if an opened hook replaces
// it later.
//
// GAP: releaseInterfaceInventories (PacketSender.ts:1748-1752,
// sending inventory stop-transmit for inventories the outgoing
// group owned) has no Go equivalent: no inventory-owner
// tracking exists, so nothing is released. The gap is
// documented at every TS call site, here and in
// forgetMountsAtLocked / resetInterfaceState.
func (s *Session) rememberMountLocked(targetUID, groupID, typ int) (*SubInterfaceMount, [][]byte) {
	if s.iface == nil {
		s.iface = newInterfaceState()
	}
	var frames [][]byte
	if existing, ok := s.iface.mounts[groupID]; ok && existing.TargetUID != targetUID {
		frames = append(frames, protocol.EncodeWidgetCloseSub(existing.TargetUID))
	}
	for mountedGroupID, mounted := range s.iface.mounts {
		if mounted.TargetUID == targetUID {
			delete(s.iface.mounts, mountedGroupID)
			// releaseInterfaceInventories(mountedGroupID):
			// gap, see above.
		}
	}
	mount := &SubInterfaceMount{TargetUID: targetUID, GroupID: groupID, Type: typ}
	s.iface.mounts[groupID] = mount
	return mount, frames
}

// forgetMountsAtLocked drops every mount at targetUID
// (closeSubInterface's bookkeeping, PacketSender.ts:1680-1685).
// Call with mu held.
func (s *Session) forgetMountsAtLocked(targetUID int) {
	if s.iface == nil {
		return
	}
	for groupID, entry := range s.iface.mounts {
		if entry.TargetUID == targetUID {
			delete(s.iface.mounts, groupID)
			// releaseInterfaceInventories(groupID): gap, see
			// rememberMountLocked.
		}
	}
}

// setChatboxUnclampedLocked records the new unclamped state and
// reports whether the unclamp varbit must be sent, and its value
// (setChatboxUnclamped, PacketSender.ts:1670-1676): the varbit
// goes out exactly when `unclamped || wasUnclamped`. Call with
// mu held; the caller sends the varbit (via SendVarbit, so the
// store records it) after releasing mu.
func (s *Session) setChatboxUnclampedLocked(unclamped bool) (send bool, value int) {
	if !unclamped && !s.chatboxUnclamped {
		return false, 0
	}
	s.chatboxUnclamped = unclamped
	if unclamped {
		return true, 1
	}
	return true, 0
}

// closableTargetUIDsLocked returns the sorted target uids of the
// closable mounts (type 0 or 3, PacketSender.ts:920-922).
// Sorted for determinism: TS iterates its insertion-ordered Map,
// Go maps iterate randomly, and the close order across
// independent targets is unobservable. Call with mu held.
func (s *Session) closableTargetUIDsLocked() []int {
	var targets []int
	if s.iface != nil {
		seen := make(map[int]bool)
		for _, entry := range s.iface.mounts {
			if (entry.Type == 0 || entry.Type == 3) && !seen[entry.TargetUID] {
				seen[entry.TargetUID] = true
				targets = append(targets, entry.TargetUID)
			}
		}
	}
	sort.Ints(targets)
	return targets
}

// emitInterfaceClosed emits "interface:closed" for interfaceID,
// unless it is negative (PacketSender.emitInterfaceClosed,
// PacketSender.ts:913-917, which returns early for id < 0).
// Runs without holding mu: the wiring's emit runs hub hooks,
// which may call back into the session.
func (s *Session) emitInterfaceClosed(interfaceID int) {
	if interfaceID < 0 {
		return
	}
	if deps := s.interfaceDeps(); deps.EmitInterfaceClosed != nil {
		deps.EmitInterfaceClosed(interfaceID)
	}
}

// emitInterfaceOpened emits "interface:opened" for a mount
// (PacketSender.emitInterfaceOpened, PacketSender.ts:1613-1618).
// Runs without holding mu, as emitInterfaceClosed does.
func (s *Session) emitInterfaceOpened(groupID, targetUID, typ int) {
	if deps := s.interfaceDeps(); deps.EmitInterfaceOpened != nil {
		deps.EmitInterfaceOpened(groupID, targetUID, typ)
	}
}

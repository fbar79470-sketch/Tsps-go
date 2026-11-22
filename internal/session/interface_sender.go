package session

import (
	"fmt"

	"tsps-go/internal/definition"
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Interface sender methods: the PacketSender interface-state
// surface (PacketSender.ts) over the tracking state in
// interface_state.go. Each method mirrors one PacketSender
// method, cited at the method; the wire encoders live in
// internal/protocol/widgets.go (dialogue_encode.go for the
// chatbox sequences).
//
// Two PacketSender methods already exist on *Session in other
// files and are NOT redefined here: SendChatboxInterface and
// SendInterfaceRemoval (dialogue_sender.go), and
// SendSubInterface (trade_sender.go). Their full runtime
// behavior lives here as mountChatboxInterface /
// removeInterfaces / MountSubInterface, and the coordinator's
// integration edits delegate the existing methods to them (see
// the unification notes in the phase report): the existing
// methods keep their signatures and wire output, and gain the
// tracking + event behavior. Until those edits land, mounts
// through the existing methods are untracked and emit nothing.
//
// Locking follows interface_state.go: state under mu, frames
// and dep calls after releasing it. Hub emits run without mu
// held, in the TS position relative to the frames for each
// path (before the mount frames for a chatbox replace, after
// the close frames for a close).

var _ game.InterfaceRuntimeSender = (*Session)(nil)
var _ definition.InterfacePacketSender = (*Session)(nil)

// mainModalTargetUID is the main modal container a main
// interface mounts into: group 161's child 16
// (MAIN_MODAL_TARGET_UID, PacketSender.ts:114).
const mainModalTargetUID = (161 << 16) | 16

// The spell-unlock tables (SPELL_UNLOCK_VARPS /
// SPELL_UNLOCK_VARBITS, PacketSender.ts:123-150) already live
// in this package as spellUnlockVarps / spellUnlockVarbits
// (bootstrap.go); the tab-6 branch sends them from there.

// SendInterface opens a main modal interface (PacketSender
// .sendInterface, PacketSender.ts:408-415): it records the id
// on the player first (which emits interface:closed/changed),
// then mounts the group at the main modal target with type 0.
//
// DEVIATION: the unwatched-bot short-circuit
// (PacketSender.ts:409-411, skipping the mount when
// isUnwatchedBot) is not ported: Go has no unwatched-bot
// distinction at the session layer — a Session implies a
// watched client, and unwatched bots have no Session (the
// message.go bot guard: session presence is the watched
// distinction). The branch is dead for every Session receiver.
func (s *Session) SendInterface(id int) {
	if deps := s.interfaceDeps(); deps.SetInterfaceID != nil {
		deps.SetInterfaceID(id)
	}
	s.MountSubInterface(mainModalTargetUID, id, 0)
}

// SendWalkableInterface opens a walkable overlay interface
// (PacketSender.sendWalkableInterface, PacketSender.ts:422-425):
// a non-modal widget open, untracked.
func (s *Session) SendWalkableInterface(interfaceID int) {
	s.SendPacket(protocol.EncodeWidgetOpen(interfaceID, false))
}

// SendSidebarInterface opens a sidebar overlay interface
// (PacketSender.sendSidebarInterface, PacketSender.ts:584-587):
// a non-modal widget open, untracked, like the walkable
// overlay.
func (s *Session) SendSidebarInterface(interfaceID int) {
	s.SendPacket(protocol.EncodeWidgetOpen(interfaceID, false))
}

// SendInterfaceSet opens a main interface plus a sidebar
// interface (PacketSender.sendInterfaceSet,
// PacketSender.ts:623-627), in that order.
func (s *Session) SendInterfaceSet(interfaceID, sidebarInterfaceID int) {
	s.SendInterface(interfaceID)
	s.SendSidebarInterface(sidebarInterfaceID)
}

// SendInterfaceDisplayState shows or hides a component
// (PacketSender.sendInterfaceDisplayState,
// PacketSender.ts:427-430). uid is the full component uid
// (group << 16 | child).
func (s *Session) SendInterfaceDisplayState(uid int, hide bool) {
	s.SendPacket(protocol.EncodeWidgetSetHidden(uid, hide))
}

// SendTabInterface mounts interfaceID in a gameframe tab
// (PacketSender.sendTabInterface, PacketSender.ts:530-573).
// The tab branches are verbatim:
//   - tab 0 mounts the fixed group 593 at 161:76 (the
//     interfaceID argument is ignored), unhiding its children
//     23 and 28;
//   - tab 5 mounts the prayer tab (77 for the 17200 variant,
//     else 541) at 161:81 and re-sends its tab setup;
//   - tab 6 mounts the spellbook frame 218 at 161:82, sends
//     the spell-unlock varps/varbits once, sets the spellbook
//     varbit 4070 (Ancient 1, Lunar 2, Arceuus 3, else 0),
//     and re-sends the tab setup;
//   - any other tab mounts interfaceID at 161:tabId.
//
// The tab 5/6/default paths remember the mount and send the
// open-sub frame directly (TS does not route them through
// mountSubInterface), so they emit no interface:opened event.
// The tab-0 path does route through sendSubInterface, so it
// tracks and emits like any mount.
func (s *Session) SendTabInterface(tabID, interfaceID int) {
	switch tabID {
	case 0:
		targetUID := (161 << 16) | 76
		s.CloseSubInterface(targetUID)
		s.MountSubInterface(targetUID, 593, 1)
		s.SendInterfaceDisplayState((593<<16)|23, false)
		s.SendInterfaceDisplayState((593<<16)|28, false)
	case 5:
		group := 541
		if interfaceID == 17200 {
			group = 77
		}
		s.mountTabInterface((161<<16)|81, group)
		s.sendGameframeTabSetup(group)
	case 6:
		spellbook := 0
		switch interfaceID {
		case 12855:
			spellbook = 1
		case 29999:
			spellbook = 2
		case 39999:
			spellbook = 3
		}
		if s.takeSpellUnlocks() {
			for _, entry := range spellUnlockVarbits {
				s.SendVarbit(entry[0], entry[1])
			}
			for _, entry := range spellUnlockVarps {
				s.SendVarp(entry[0], entry[1])
			}
		}
		s.SendVarbit(4070, spellbook)
		s.mountTabInterface((161<<16)|82, 218)
		s.sendGameframeTabSetup(218)
	default:
		s.mountTabInterface((161<<16)|tabID, interfaceID)
	}
}

// mountTabInterface remembers a tab mount and sends its
// open-sub frame directly, without the mountSubInterface emit
// or inventory-flags behavior (see SendTabInterface).
func (s *Session) mountTabInterface(targetUID, groupID int) {
	s.mu.Lock()
	_, displaced := s.rememberMountLocked(targetUID, groupID, 1)
	s.mu.Unlock()
	for _, frame := range displaced {
		s.SendPacket(frame)
	}
	s.SendPacket(protocol.EncodeWidgetOpenSub(targetUID, groupID, 1, protocol.WidgetOpenSubOptions{}))
}

// SendMagicTabInterface mounts interfaceID in the magic tab's
// slot (161:82) in place of the spellbook
// (PacketSender.sendMagicTabInterface, PacketSender.ts:578-582),
// remembered and sent directly like the other tab paths; no
// tab setup is re-sent.
func (s *Session) SendMagicTabInterface(interfaceID int) {
	s.mountTabInterface((161<<16)|82, interfaceID)
}

// takeSpellUnlocks reports whether the spell-unlock tables
// still need sending, claiming them exactly once
// (spellUnlocksSent, PacketSender.ts:223, 553-554).
func (s *Session) takeSpellUnlocks() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.iface == nil {
		s.iface = newInterfaceState()
	}
	if s.iface.spellUnlocksSent {
		return false
	}
	s.iface.spellUnlocksSent = true
	return true
}

// sendGameframeTabSetup re-sends the login's setup for one side
// tab after it is mounted again
// (PacketSender.sendGameframeTabSetup, PacketSender.ts:329-333),
// over the gameframe root from the deps (161 default).
func (s *Session) sendGameframeTabSetup(group int) {
	root := 161
	if deps := s.interfaceDeps(); deps.GameframeRoot != nil {
		root = deps.GameframeRoot()
	}
	for _, packet := range protocol.EncodeGameframeTabSetup(group, root) {
		s.SendPacket(packet)
	}
}

// MountSubInterface mounts groupID at targetUID with type,
// returning the mount identity owned by this call
// (PacketSender.mountSubInterface, PacketSender.ts:1555-1574):
// remember the mount, send the open-sub frame, send the
// main-inventory flags range when mounting group 149, and emit
// interface:opened. The existing SendSubInterface
// (trade_sender.go) delegates to this at integration; until
// then, mounts through it are untracked and emit nothing.
// The FrameUpdater clearGroup has no Go equivalent (no
// FrameUpdater; see dialogue_sender.go). The options form is
// MountSubInterfaceWithOptions; this is the no-options form.
func (s *Session) MountSubInterface(targetUID, groupID, typ int) *SubInterfaceMount {
	return s.MountSubInterfaceWithOptions(targetUID, groupID, typ, protocol.WidgetOpenSubOptions{})
}

// MountSubInterfaceWithOptions mounts groupID at targetUID
// with type and the TS sendSubInterface options (varps,
// varbits, hidden uids, pre/post scripts —
// PacketSender.ts:1555-1574, encodeWidgetOpenSub,
// ClientProtocol.ts:2286-2311): the options ride the
// open-sub frame itself, so the client applies them with the
// mount instead of after it. The bank open uses it for the
// main 12 / side 15 mounts (varps {548:1, 262:sideWearOpsMask,
// 4611:MAX_SLOTS-capacity} and the bank varbit bundle,
// Bank.ts:1354-1414); every other caller passes the empty
// options through MountSubInterface. The mount tracking,
// main-inventory flags range, and interface:opened emit are
// the same as MountSubInterface.
func (s *Session) MountSubInterfaceWithOptions(targetUID, groupID, typ int, options protocol.WidgetOpenSubOptions) *SubInterfaceMount {
	s.mu.Lock()
	mount, displaced := s.rememberMountLocked(targetUID, groupID, typ)
	s.mu.Unlock()
	for _, frame := range displaced {
		s.SendPacket(frame)
	}
	s.SendPacket(protocol.EncodeWidgetOpenSub(targetUID, groupID, typ, options))
	if groupID == protocol.MainInventoryGroupID {
		s.SendPacket(protocol.EncodeWidgetSetFlagsRange(protocol.MainInventoryWidgetUID, 0, 27, protocol.MainInventorySlotFlags))
	}
	s.emitInterfaceOpened(groupID, targetUID, typ)
	return mount
}

// SendSideOps sends the bank side-panel wearable-ops mask
// (Bank.sendSideOps, Bank.ts:854-861): varp 262, one bit per
// inventory slot holding something wearable, so the client
// shows the wear op on those side-inventory items. The
// caller passes the inventory item ids in slot order and a
// wearable predicate over item ids (the coordinator wires it
// over the definition Store: EquipmentSlot() >= 0); a nil
// predicate reads every item as unwearable. Only the first
// 28 slots (the inventory) can set bits — further entries
// are ignored, matching the 28-slot side panel.
func (s *Session) SendSideOps(inventoryItemIDs []int, wearable func(itemID int) bool) {
	s.SendVarp(sideOpsVarpID, sideWearOpsMask(inventoryItemIDs, wearable))
}

// sideOpsVarpID is the side-ops mask varp (Bank.ts:854-861).
const sideOpsVarpID = 262

// sideWearOpsMask computes the varp-262 mask for
// inventoryItemIDs: bit i is set when slot i holds an item
// (id >= 0) the wearable predicate accepts. Slots past the
// 28-slot inventory are ignored.
func sideWearOpsMask(inventoryItemIDs []int, wearable func(itemID int) bool) int {
	mask := 0
	for slot, id := range inventoryItemIDs {
		if slot >= 28 {
			break
		}
		if id >= 0 && wearable != nil && wearable(id) {
			mask |= 1 << uint(slot)
		}
	}
	return mask
}

// GetSubInterfaceAt returns the mount at targetUID, if any
// (PacketSender.getSubInterfaceAt, PacketSender.ts:1594-1599).
func (s *Session) GetSubInterfaceAt(targetUID int) (*SubInterfaceMount, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.iface == nil {
		return nil, false
	}
	for _, mount := range s.iface.mounts {
		if mount.TargetUID == targetUID {
			return mount, true
		}
	}
	return nil, false
}

// CloseSubInterfaceMount closes only the still-current mount:
// a delayed owner may close its own mount, never a replacement
// (PacketSender.closeSubInterfaceMount, PacketSender.ts:1602-1606).
// It reports whether the mount was still current.
func (s *Session) CloseSubInterfaceMount(mount *SubInterfaceMount) bool {
	if mount == nil {
		return false
	}
	s.mu.Lock()
	current := false
	if s.iface != nil {
		current = s.iface.mounts[mount.GroupID] == mount
	}
	s.mu.Unlock()
	if !current {
		return false
	}
	s.CloseSubInterface(mount.TargetUID)
	return true
}

// IsSubInterfaceOpen reports whether groupID is mounted as a
// sub-interface right now (PacketSender.isSubInterfaceOpen,
// PacketSender.ts:1630-1632).
func (s *Session) IsSubInterfaceOpen(groupID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.iface == nil {
		return false
	}
	_, ok := s.iface.mounts[groupID]
	return ok
}

// CloseSubInterface closes the mount at targetUID
// (PacketSender.closeSubInterface, PacketSender.ts:1678-1688):
// closing the chatbox modal target resets the unclamp varbit,
// the mounts at the target are forgotten, and the close-sub
// frame goes out. The frame is sent unconditionally, even when
// nothing was tracked at the target, as in TS.
func (s *Session) CloseSubInterface(targetUID int) {
	s.mu.Lock()
	sendVarbit, value := false, 0
	if targetUID == protocol.ChatboxModalTargetUID {
		sendVarbit, value = s.setChatboxUnclampedLocked(false)
	}
	s.forgetMountsAtLocked(targetUID)
	s.mu.Unlock()
	if sendVarbit {
		s.SendVarbit(protocol.ChatboxUnclampVarbit, value)
	}
	s.SendPacket(protocol.EncodeWidgetCloseSub(targetUID))
}

// GetChatboxGroupId returns the mounted chatbox group, or -1
// (PacketSender.getChatboxGroupId, PacketSender.ts:936-938).
func (s *Session) GetChatboxGroupId() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chatboxGroupId
}

// GetChatboxOwner returns the owner of the currently mounted
// chatbox, if its presentation was scoped
// (PacketSender.getChatboxOwner, PacketSender.ts:941-943).
func (s *Session) GetChatboxOwner() any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chatboxOwner
}

// CloseChatboxFor closes this owner's chatbox without removing
// an unrelated main modal (PacketSender.closeChatboxFor,
// PacketSender.ts:954-958). It reports false when no chatbox is
// mounted or the mounted chatbox belongs to another owner. The
// dialogue manager resets only when it belongs to the same
// owner (closeCurrentChatbox's resetDialogue argument).
func (s *Session) CloseChatboxFor(owner any) bool {
	s.mu.Lock()
	if s.chatboxGroupId < 0 || s.chatboxOwner != owner {
		s.mu.Unlock()
		return false
	}
	s.mu.Unlock()
	resetDialogue := false
	if deps := s.interfaceDeps(); deps.DialogueOwner != nil {
		resetDialogue = deps.DialogueOwner() == owner
	}
	s.closeCurrentChatbox(resetDialogue)
	return true
}

// closeCurrentChatbox closes the mounted chatbox
// (PacketSender.closeCurrentChatbox, PacketSender.ts:960-970):
// clear the group and owner, reset the dialogue manager when
// asked, close the modal target, hide it, and emit
// interface:closed for the group. Unlike
// closeTrackedInterfaces, this DOES clear the owner.
func (s *Session) closeCurrentChatbox(resetDialogue bool) {
	s.mu.Lock()
	group := s.chatboxGroupId
	if group < 0 {
		s.mu.Unlock()
		return
	}
	s.chatboxGroupId = -1
	s.chatboxOwner = nil
	s.mu.Unlock()
	if resetDialogue {
		if deps := s.interfaceDeps(); deps.ResetDialogue != nil {
			deps.ResetDialogue()
		}
	}
	s.CloseSubInterface(protocol.ChatboxModalTargetUID)
	s.SendPacket(protocol.EncodeWidgetSetHidden(protocol.ChatboxModalTargetUID, true))
	s.emitInterfaceClosed(group)
}

// mountChatboxInterface is the full sendChatboxInterface
// (PacketSender.ts:598-620): record the presenting owner and
// the group, emit interface:closed for a replaced chatbox,
// mount the group in the chatbox modal container (type 0),
// and track the mount. The existing SendChatboxInterface
// (dialogue_sender.go) delegates to this at integration; the
// wire output is identical to its current frames (the
// EncodeChatboxInterfaceFrames bundle), plus a close-sub frame
// only when the same group was mounted at another target —
// which the bundle's open-sub frame must follow, as in TS.
// The closed event emits before the frames, in the TS
// position. The FrameUpdater clearGroup has no Go equivalent
// (see MountSubInterface).
func (s *Session) mountChatboxInterface(groupID int, unclamp bool) {
	s.mu.Lock()
	previous := s.chatboxGroupId
	s.chatboxOwner = s.presentingChatboxOwner
	s.chatboxGroupId = groupID
	wasUnclamped := s.chatboxUnclamped
	s.chatboxUnclamped = unclamp
	_, displaced := s.rememberMountLocked(protocol.ChatboxModalTargetUID, groupID, 0)
	s.mu.Unlock()
	if previous >= 0 && previous != groupID {
		s.emitInterfaceClosed(previous)
	}
	frames := protocol.EncodeChatboxInterfaceFrames(groupID, unclamp, wasUnclamped)
	// The bundle is [unclamp varbit?], unhide, open-sub; a
	// displaced close-sub frame goes between the unhide and
	// the open-sub, where TS's rememberSubInterface call sits.
	for _, frame := range frames[:len(frames)-1] {
		s.SendPacket(frame)
	}
	for _, frame := range displaced {
		s.SendPacket(frame)
	}
	s.SendPacket(frames[len(frames)-1])
}

// HasInterruptibleInterface reports whether a chatbox or a
// closable (type 0 or 3) sub-interface is mounted
// (PacketSender.hasInterruptibleInterface,
// PacketSender.ts:976-979).
func (s *Session) HasInterruptibleInterface() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.chatboxGroupId >= 0 {
		return true
	}
	if s.iface == nil {
		return false
	}
	for _, entry := range s.iface.mounts {
		if entry.Type == 0 || entry.Type == 3 {
			return true
		}
	}
	return false
}

// HasInputPrompt reports whether an amount or text action is
// armed (PacketSender.hasInputPrompt, PacketSender.ts:455-457).
// Without the wiring dep it reads false: no player is attached.
func (s *Session) HasInputPrompt() bool {
	deps := s.interfaceDeps()
	return deps.HasInputPrompt != nil && deps.HasInputPrompt()
}

// AbortInputPrompt drops an unanswered chatbox prompt and the
// action waiting on its answer (PacketSender.abortInputPrompt,
// PacketSender.ts:465-475): when a prompt is armed, the
// trigger_ondialogabort frame goes out first (it closes the
// prompt without answering), then the player holders clear and
// the amount action's Cancel runs (via the wiring dep).
func (s *Session) AbortInputPrompt() {
	deps := s.interfaceDeps()
	if deps.HasInputPrompt != nil && deps.HasInputPrompt() {
		if frame := triggerOnDialogAbortFrame(); frame != nil {
			s.SendPacket(frame)
		}
	}
	if deps.AbortEnteredActions != nil {
		deps.AbortEnteredActions()
	}
}

// triggerOnDialogAbortFrame renders the empty
// TRIGGER_ONDIALOGABORT frame (encodeTriggerOnDialogAbort,
// ClientProtocol.ts).
func triggerOnDialogAbortFrame() []byte {
	return protocol.EncodeTriggerOnDialogAbort()
}

// CloseInterruptibleInterfaces closes every tracked interface
// (PacketSender.closeInterruptibleInterfaces,
// PacketSender.ts:981-995). Its body is identical to
// sendInterfaceRemoval's; both run removeInterfaces.
func (s *Session) CloseInterruptibleInterfaces() {
	s.removeInterfaces()
}

// CloseInterface closes one interface by group id
// (PacketSender.closeInterface, PacketSender.ts:996-1009):
// the player's main interface closes through the full removal,
// the mounted chatbox through closeCurrentChatbox, and a
// tracked sub-interface through its target close plus an
// interface:closed emit. An invalid id, or an id that names
// nothing open, is a no-op.
func (s *Session) CloseInterface(groupID int) {
	if groupID < 0 {
		return
	}
	deps := s.interfaceDeps()
	if deps.GetInterfaceID != nil && groupID == deps.GetInterfaceID() {
		s.removeInterfaces()
		return
	}
	s.mu.Lock()
	chatbox := s.chatboxGroupId
	var target *SubInterfaceMount
	if s.iface != nil {
		target = s.iface.mounts[groupID]
	}
	s.mu.Unlock()
	if groupID == chatbox {
		s.closeCurrentChatbox(true)
		return
	}
	if target != nil {
		s.CloseSubInterface(target.TargetUID)
		s.emitInterfaceClosed(groupID)
	}
}

// removeInterfaces is the shared body of
// closeInterruptibleInterfaces and sendInterfaceRemoval
// (PacketSender.ts:981-995, 1011-1025): reset the player-side
// interface state, close the tracked interfaces, end a bank
// search, emit interface:closed for the outgoing main
// interface, and either restore the main inventory group (when
// something tracked closed and the outgoing interface was a
// shop/trade screen: 300/334/335) or send a widget close for
// the outgoing id (when nothing tracked closed). The existing
// SendInterfaceRemoval (dialogue_sender.go) delegates to this
// at integration.
func (s *Session) removeInterfaces() {
	interfaceID := s.resetInterfaceState()
	closed := s.closeTrackedInterfaces()
	if deps := s.interfaceDeps(); deps.EndBankSearch != nil {
		deps.EndBankSearch(interfaceID)
	}
	s.emitInterfaceClosed(interfaceID)
	if closed {
		if interfaceID == 300 || interfaceID == 334 || interfaceID == 335 {
			s.MountSubInterface((161<<16)|79, protocol.MainInventoryGroupID, 1)
		}
		return
	}
	if interfaceID >= 0 {
		s.SendPacket(protocol.EncodeWidgetClose(interfaceID))
	}
}

// resetInterfaceState resets the player-side interface state
// and returns the outgoing main interface id
// (PacketSender.resetInterfaceState, PacketSender.ts:876-900),
// in TS order: release the interface inventories, clear the
// bank varp/varbit when closing the bank, close the shop,
// reset the status, abort the input prompt, reset the dialogue
// manager, clear the destroy-item pending marker, clear the
// interface id (which emits interface:changed), clear the
// creation menu / bank-search / teleport-interface state, and
// clear the appearance-change flag.
//
// GAPS (no Go state exists; nothing is invented):
//   - releaseInterfaceInventories: no inventory-owner tracking.
//   - teleport-interface-open: no Go equivalent.
//   - bank-search flag: the flag exists (phase 46,
//     game.BankSearching) and clears through the
//     EndBankSearch dep in removeInterfaces, which stays nil
//     until the coordinator wires it in
//     installInterfaceRuntime.
//   - status reset: the PlayerStatus field exists (phase 46,
//     status.go) but the SetStatusNone dep stays nil until the
//     coordinator wires it in installInterfaceRuntime.
func (s *Session) resetInterfaceState() int {
	deps := s.interfaceDeps()
	interfaceID := -1
	if deps.GetInterfaceID != nil {
		interfaceID = deps.GetInterfaceID()
	}
	// releaseInterfaceInventories(interfaceID): gap, see above.
	if interfaceID == 12 {
		s.SendVarp(548, 0)
		s.SendVarbit(12393, 0)
	}
	if deps.CloseShop != nil {
		deps.CloseShop()
	}
	if deps.SetStatusNone != nil {
		deps.SetStatusNone()
	}
	s.AbortInputPrompt()
	if deps.ResetDialogue != nil {
		deps.ResetDialogue()
	}
	if deps.SetDestroyPending != nil {
		deps.SetDestroyPending()
	}
	if deps.SetInterfaceID != nil {
		deps.SetInterfaceID(-1)
	}
	// Creation menu: cleared in TS order, after the interface id
	// (PacketSender.ts:894).
	if deps.SetCreationMenu != nil {
		deps.SetCreationMenu(nil)
	}
	// Searching-bank / teleport-interface-open: gaps, see above.
	if deps.SetCanChangeAppearance != nil {
		deps.SetCanChangeAppearance(false)
	}
	return interfaceID
}

// closeTrackedInterfaces closes every closable (type 0 or 3)
// sub-interface and the mounted chatbox, and reports whether
// anything closed (PacketSender.closeTrackedInterfaces,
// PacketSender.ts:919-936). The chatbox group id clears but
// its owner is left stale, as in TS; the modal target hides
// after the closes, and interface:closed emits for the
// chatbox group.
func (s *Session) closeTrackedInterfaces() bool {
	s.mu.Lock()
	targets := s.closableTargetUIDsLocked()
	chatbox := s.chatboxGroupId
	hadChatbox := chatbox >= 0
	if hadChatbox {
		s.chatboxGroupId = -1
		found := false
		for _, target := range targets {
			if target == protocol.ChatboxModalTargetUID {
				found = true
				break
			}
		}
		if !found {
			targets = append(targets, protocol.ChatboxModalTargetUID)
		}
	}
	s.mu.Unlock()
	if len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		s.CloseSubInterface(target)
	}
	if hadChatbox {
		s.SendPacket(protocol.EncodeWidgetSetHidden(protocol.ChatboxModalTargetUID, true))
		s.emitInterfaceClosed(chatbox)
	}
	return true
}

// SendConfiguredInterface opens a configured interface layout
// by key or id (PacketSender.sendConfiguredInterface,
// PacketSender.ts:417-420, via InterfaceLayoutRegistry.open).
// The reference is a layout key (string) or an interface id
// (int); anything else, or an unknown reference, returns an
// error and sends nothing. With no layouts installed, every
// reference is unknown.
func (s *Session) SendConfiguredInterface(reference any) (*definition.InterfaceLayoutDefinition, error) {
	lookup := s.interfaceLayouts()
	if lookup == nil {
		return nil, fmt.Errorf("Unknown interface layout: %v", reference)
	}
	return definition.OpenLayout(lookup, s, reference)
}

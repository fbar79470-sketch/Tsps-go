package session

import (
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// DialogueSender implementation: the session side of
// game.DialogueSender, porting the PacketSender surface dialogue
// itself needs (PacketSender.ts). Each method mirrors one
// PacketSender method, cited at the method; the wire encoders live
// in internal/protocol (widgets.go for the primitives,
// dialogue_encode.go for the multi-packet chatbox sequences).
// Dialogue entries rely on call order (a chatbox mount precedes the
// strings written into it), and SendPacket preserves queue order.
//
// Chatbox-state methods snapshot and update the guarded fields
// under mu, then queue frames via SendPacket after releasing it
// (SendPacket takes mu itself). The interface-closed event, the
// tracked-sub-interface bookkeeping, and the main-interface close
// live in the interface runtime (interface_sender.go, phase 18);
// SendChatboxInterface and SendInterfaceRemoval delegate to it.
//
// Session must satisfy the game's dialogue seam structurally, as
// it satisfies game.SessionPort and game.ChatMessageSender (see
// sessionport_test.go and chat_sender_test.go): if either side
// drifts, the build fails here rather than in the coordinator.
var _ game.DialogueSender = (*Session)(nil)

// SendChatboxInterface mounts groupID in the chatbox modal
// container, porting PacketSender.sendChatboxInterface
// (PacketSender.ts:598-620). It delegates to the interface
// runtime's mountChatboxInterface (interface_sender.go), which
// records the presenting owner and the group id, emits
// interface:closed for a replaced chatbox, tracks the mount,
// and sends the unclamp/unhide/open-sub frames. The FrameUpdater
// cache clear (clearGroup) has no Go equivalent — Go has no
// FrameUpdater, so every SendString sends (see the
// DialogueSender contract).
func (s *Session) SendChatboxInterface(groupID int, unclamp bool) {
	s.mountChatboxInterface(groupID, unclamp)
}

// SendNpcHeadOnInterface sets an NPC head on a component, porting
// PacketSender.sendNpcHeadOnInterface (PacketSender.ts:437-440).
func (s *Session) SendNpcHeadOnInterface(npcID, uid int) {
	s.SendPacket(protocol.EncodeWidgetSetNpcHead(uid, npcID))
}

// SendPlayerHeadOnInterface sets the player's head on a component,
// porting PacketSender.sendPlayerHeadOnInterface
// (PacketSender.ts:432-435).
func (s *Session) SendPlayerHeadOnInterface(uid int) {
	s.SendPacket(protocol.EncodeWidgetSetPlayerHead(uid))
}

// SendInterfaceAnimation sets a component's animation, porting
// PacketSender.sendInterfaceAnimation (PacketSender.ts:495-498).
func (s *Session) SendInterfaceAnimation(uid, animationID int) {
	s.SendPacket(protocol.EncodeWidgetSetAnimation(uid, animationID))
}

// SendString writes text into a component, porting
// PacketSender.sendString (PacketSender.ts:677-684) with no
// serverOwned argument. DEVIATION: TS dedups through the
// per-player FrameUpdater; Go has none, so every call sends (see
// the DialogueSender contract).
func (s *Session) SendString(text string, uid int) {
	s.SendPacket(protocol.EncodeWidgetSetText(uid, text, nil))
}

// SendClientScript runs a client script, porting
// PacketSender.sendClientScript (PacketSender.ts:1754-1757).
func (s *Session) SendClientScript(scriptID int, args ...game.ClientScriptArg) {
	translated := make([]protocol.ScriptArg, 0, len(args))
	for _, arg := range args {
		if arg.IsString() {
			translated = append(translated, protocol.ScriptArgString(arg.StringValue()))
		} else {
			translated = append(translated, protocol.ScriptArgInt(arg.IntValue()))
		}
	}
	s.SendPacket(protocol.EncodeRunClientScript(scriptID, translated))
}

// SendInterfaceFlagsRange sets option flags over a slot range,
// porting PacketSender.sendInterfaceFlagsRange
// (PacketSender.ts:1699-1705). Dialogue never passes ops, so this
// is never the V2 form.
func (s *Session) SendInterfaceFlagsRange(uid, fromSlot, toSlot, flags int) {
	s.SendPacket(protocol.EncodeWidgetSetFlagsRange(uid, fromSlot, toSlot, flags))
}

// SendItemOnInterfaces sets an item on a component, porting
// PacketSender.sendItemOnInterfaces (PacketSender.ts:629-633).
// Dialogue passes the item zoom (400) as the amount.
func (s *Session) SendItemOnInterfaces(uid, itemID, amount int) {
	s.SendPacket(protocol.EncodeWidgetSetItem(uid, itemID, amount))
}

// SendInterfaceScript runs a widget script with no arguments,
// porting PacketSender.sendInterfaceScript
// (PacketSender.ts:1706-1718). Dialogue's only call is script 2379
// with no args, varps, varbits, or inventories.
func (s *Session) SendInterfaceScript(scriptID int) {
	s.SendPacket(protocol.EncodeWidgetRunScript(scriptID, nil, nil, nil, nil))
}

// SendInterfaceRemoval closes the player's interfaces: the full
// PacketSender.sendInterfaceRemoval (PacketSender.ts:1011-1025),
// delegating to the interface runtime's removeInterfaces
// (interface_sender.go): reset the player-side interface state
// (which resets the dialogue manager, aborts any input prompt,
// and closes the shop, per resetInterfaceState,
// PacketSender.ts:876-900), close the tracked interfaces,
// emit interface:closed, and either restore the main inventory
// group or send a widget close for the outgoing id. TS leaves
// chatboxOwner stale on this path (only closeCurrentChatbox
// clears it), and so does removeInterfaces.
func (s *Session) SendInterfaceRemoval() {
	s.removeInterfaces()
}

// IsChatboxInterface reports whether groupID is the mounted
// chatbox group, porting PacketSender.isChatboxInterface
// (PacketSender.ts:972-974).
func (s *Session) IsChatboxInterface(groupID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return groupID == s.chatboxGroupId
}

// WithChatboxOwner scopes owner to chatboxes mounted
// synchronously by present, restoring the previous presenting
// owner afterwards, porting PacketSender.withChatboxOwner
// (PacketSender.ts:946-951). The set/restore happen under mu;
// present runs outside it, because mounts inside it queue packets
// through SendPacket, which takes mu itself.
func (s *Session) WithChatboxOwner(owner any, present func()) {
	s.mu.Lock()
	previous := s.presentingChatboxOwner
	s.presentingChatboxOwner = owner
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.presentingChatboxOwner = previous
		s.mu.Unlock()
	}()
	present()
}

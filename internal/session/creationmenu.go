package session

import (
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// CreationMenu sender methods: the session side of
// PacketSender.sendCreationMenu / closeCreationMenu
// (PacketSender.ts:1377-1422).
//
// SEND ORDER, exactly as TS sends it (PacketSender.ts:1397-1414):
// interface script 2379, chatbox unclamped, modal-target display
// state shown, sub-interface group 270 at the modal target, one
// flags range per item button, varp 2673 when a last item is
// remembered, interface script 2046 with the menu arguments.
//
// TWO DEVIATIONS FROM THE PLAN's §3 READING, both grounded in the
// TS source rather than the plan text:
//
//   - The mount uses the primitives — setChatboxUnclamped(true),
//     SendInterfaceDisplayState, SendSubInterface — not
//     SendChatboxInterface. TS sendCreationMenu never calls
//     sendChatboxInterface (PacketSender.ts:1397-1400 calls the
//     three primitives directly); routing through
//     SendChatboxInterface would additionally record group 270 as
//     the mounted chatbox (chatboxGroupId) and emit
//     interface:closed for a replaced chatbox, neither of which
//     TS does here. The frames are identical either way; only the
//     session-side bookkeeping differs, and the primitive form
//     keeps it TS-faithful. CloseCreationMenu mirrors TS the same
//     way (closeSubInterface + display state, PacketSender
//     .ts:1420-1421), without touching chatboxGroupId.
//   - Script 2046 goes out through EncodeWidgetRunScript (opcode
//     110, WIDGET_RUN_SCRIPT), NOT through SendClientScript /
//     EncodeRunClientScript (opcode 170, RUN_CLIENT_SCRIPT). TS
//     sendCreationMenu calls sendInterfaceScript(2046, [...])
//     (PacketSender.ts:1413), and sendInterfaceScript writes
//     encodeWidgetRunScript (PacketSender.ts:1706-1718;
//     ClientProtocol.ts:2471-2496), while sendClientScript writes
//     the different encodeRunClientScript (ClientProtocol
//     .ts:2498-2503). The plan's "the same frame" claim is wrong
//     for this call: opcode 110 carries a 4-byte script id plus
//     the varp/varbit sections, opcode 170 a 2-byte id with
//     neither. SendInterfaceScriptWithArgs below is the mixed-arg
//     form of sendInterfaceScript; the session tests pin the
//     2046 frame byte-for-byte, including the 110 opcode.
//
// The menu install/clear on the player travels through the
// InterfaceDeps.SetCreationMenu seam (PacketSender holds its
// player directly; Session holds none): the send installs the
// menu (PacketSender.ts:1386) and the close clears it
// (PacketSender.ts:1418), each before its frames, as in TS. An
// unwired session still sends the frames; only the player field
// is untouched.

var _ game.CreationMenuSender = (*Session)(nil)

// SendCreationMenu opens menu on the client, porting
// PacketSender.sendCreationMenu (PacketSender.ts:1377-1414). A
// nil menu, or a menu offering no items, sends nothing (the TS
// early return, PacketSender.ts:1380-1385) — and installs nothing
// on the player either, since the install sits after the early
// return in TS.
//
// Item names come from itemName (the caller's Player.ItemName):
// a per-item label wins, else the cache name, else "null" — the
// TS fallback for an id CacheDefinitions.hasItem rejects
// (CacheDefinitions.ts:589-595: false when the name is empty or
// "null", which is exactly when the Go name source reads "").
// A nil itemName names every item "null".
//
// lastItem/hasLastItem are the remembered last-chosen index from
// Player.CreationMenuLastItem; the varp goes out only when
// hasLastItem is true, mirroring TS's Number.isInteger check on
// the attribute (PacketSender.ts:1412).
func (s *Session) SendCreationMenu(menu *game.CreationMenu, itemName func(id int) string, lastItem int, hasLastItem bool) {
	if menu == nil {
		return
	}
	items := menu.Items()
	if len(items) > game.CreationMenuMaxItems {
		items = items[:game.CreationMenuMaxItems]
	}
	if len(items) == 0 {
		return
	}
	if deps := s.interfaceDeps(); deps.SetCreationMenu != nil {
		deps.SetCreationMenu(menu)
	}

	options := menu.Options()
	names := make([]string, len(items))
	for i, id := range items {
		if i < len(options.Labels) {
			names[i] = options.Labels[i]
			continue
		}
		name := ""
		if itemName != nil {
			name = itemName(id)
		}
		if name == "" {
			name = "null"
		}
		names[i] = name
	}
	maxAmount := menu.EffectiveMaxAmount()
	lastAmount := menu.EffectiveLastAmount()
	paddedIDs := make([]int, game.CreationMenuMaxItems)
	for i := range paddedIDs {
		paddedIDs[i] = -1
	}
	copy(paddedIDs, items)

	s.SendInterfaceScript(2379)
	s.mu.Lock()
	sendUnclamp, unclampValue := s.setChatboxUnclampedLocked(true)
	s.mu.Unlock()
	if sendUnclamp {
		s.SendVarbit(protocol.ChatboxUnclampVarbit, unclampValue)
	}
	s.SendInterfaceDisplayState(protocol.ChatboxModalTargetUID, false)
	s.SendSubInterface(protocol.ChatboxModalTargetUID, game.CreationMenuGroupID, 0)
	for i := range items {
		buttonID := (game.CreationMenuGroupID << 16) | (game.CreationMenuFirstItemComponent + i)
		s.SendInterfaceFlagsRange(buttonID, 0, game.CreationMenuMaxQuantity, game.CreationMenuOpFlags)
	}
	if hasLastItem {
		s.SendVarp(game.CreationMenuLastItemVarp, lastItem)
	}
	args := make([]game.ClientScriptArg, 0, 3+len(paddedIDs)+1)
	args = append(args,
		game.ClientScriptInt(options.Mode),
		game.ClientScriptString(menu.Title()+"|"+strings.Join(names, "|")),
		game.ClientScriptInt(maxAmount),
	)
	for _, id := range paddedIDs {
		args = append(args, game.ClientScriptInt(id))
	}
	args = append(args, game.ClientScriptInt(lastAmount))
	s.SendInterfaceScriptWithArgs(2046, args...)
}

// SendInterfaceScriptWithArgs runs a widget script with mixed
// string/int arguments, porting PacketSender.sendInterfaceScript
// (PacketSender.ts:1706-1718) — the form that writes
// encodeWidgetRunScript (opcode 110), unlike SendClientScript,
// which writes encodeRunClientScript (opcode 170). The existing
// SendInterfaceScript covers only the no-argument form and
// SendInterfaceScriptArgs only integer arguments; creation menus
// need both (script 2046 takes a mode int, a "title|names"
// string, then ints).
func (s *Session) SendInterfaceScriptWithArgs(scriptID int, args ...game.ClientScriptArg) {
	translated := make([]protocol.ScriptArg, 0, len(args))
	for _, arg := range args {
		if arg.IsString() {
			translated = append(translated, protocol.ScriptArgString(arg.StringValue()))
		} else {
			translated = append(translated, protocol.ScriptArgInt(arg.IntValue()))
		}
	}
	s.SendPacket(protocol.EncodeWidgetRunScript(scriptID, translated, nil, nil, nil))
}

// CloseCreationMenu closes the open creation menu, porting
// PacketSender.closeCreationMenu (PacketSender.ts:1417-1422):
// the player field clears first, then the modal-target
// sub-interface closes (which sends the unclamp varbit back to 0
// when the menu was unclamped) and the modal target hides.
func (s *Session) CloseCreationMenu() {
	if deps := s.interfaceDeps(); deps.SetCreationMenu != nil {
		deps.SetCreationMenu(nil)
	}
	s.CloseSubInterface(protocol.ChatboxModalTargetUID)
	s.SendInterfaceDisplayState(protocol.ChatboxModalTargetUID, true)
}

package listeners

import (
	"encoding/binary"
	"strings"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

func testPlayer() *game.Player {
	return game.NewPlayer("testplayer", game.Location{X: 3089, Y: 3524, Z: 0})
}

func zeros(n int) []byte { return make([]byte, n) }

// TestHandleRecognizedOpcodes pins the decided Outcome for every
// opcode Handle recognizes. Payloads are zero-filled at the table
// length (or the minimal valid variable length): field extraction is
// pinned by the protocol decoder tests; these tests pin dispatch and
// the drop reason. Every case in this table drops with a zero
// payload, an empty inventory, no definition source, and no
// ground-item store: the implemented routes drop with ReasonNoAction
// (gates refused: slot/id mismatch, equal slots, no operable stack)
// or ReasonGroundItemsUnwired (a take that would otherwise walk).
// Acted outcomes are pinned by TestHandleVarpTransmitToggle and by
// the implemented-listener tests, not here; the varp_transmit rows
// below use other varpIds.
func TestHandleRecognizedOpcodes(t *testing.T) {
	p := testPlayer()
	cases := []struct {
		name    string
		opcode  byte
		payload []byte
		reason  DropReason
	}{
		{"appearance", protocol.NativeAppearanceSet, zeros(20), ReasonChangeAppearance},

		{"chat public empty text refused", protocol.ClientChat, []byte{0, 0}, ReasonNoAction},
		{"chat game kind ignored", protocol.ClientChat, []byte{1, 'g', 0}, ReasonNoAction},
		// The CHAT clan/friends kinds are NOT here: they act
		// (the social-packet emit), pinned by
		// TestHandleChatSocialKindsEmit.
		{"chat command prefix", protocol.ClientChat, []byte{0, ':', ':', 'x', 0}, ReasonCommand},
		{"chat command prefix with brackets", protocol.ClientChat, []byte{0, '<', ':', ':', 'x', '>', 0}, ReasonCommand},
		{"chat command prefix with spaces", protocol.ClientChat, []byte{0, ' ', ' ', ':', ':', 'x', 0}, ReasonCommand},

		{"examine npc unwired", protocol.NativeExamineNpc, zeros(2), ReasonExamineUnwired},
		{"examine object unwired", protocol.NativeExamineLoc, zeros(2), ReasonExamineUnwired},

		{"npc option 1", protocol.NativeOpNpc1Alt, zeros(3), ReasonNpcOption},
		{"npc option 2", protocol.NativeOpNpc2, zeros(3), ReasonNpcOption},
		{"npc option 3", protocol.NativeOpNpc3, zeros(3), ReasonNpcOption},
		{"npc option 4", protocol.NativeOpNpc4, zeros(3), ReasonNpcOption},
		{"npc option 5", protocol.NativeOpNpc1, zeros(3), ReasonNpcOption},
		{"spell on npc", protocol.NativeOpNpcT, zeros(11), ReasonNpcOption},

		{"object option 1", protocol.NativeOpLoc1, zeros(7), ReasonObjectAction},
		{"object option 2", protocol.NativeOpLoc2, zeros(7), ReasonObjectAction},
		{"object option 3", protocol.NativeOpLoc3, zeros(7), ReasonObjectAction},
		{"object option 4", protocol.NativeOpLoc4, zeros(7), ReasonObjectAction},
		{"object option 5", protocol.NativeOpLoc5, zeros(7), ReasonObjectAction},
		{"loc interact", protocol.ClientLocInteract, zeros(9), ReasonObjectAction},

		{"player option 1 attack", protocol.NativeOpPlayer1, zeros(3), ReasonPlayerOption},
		{"player option 2 trade", protocol.NativeOpPlayer2, zeros(3), ReasonTradeRequest},
		{"player option 3 follow", protocol.NativeOpPlayer3, zeros(3), ReasonFollowPlayer},
		{"player option 4", protocol.NativeOpPlayer4, zeros(3), ReasonPlayerOption},
		{"player option 5", protocol.NativeOpPlayer5, zeros(3), ReasonPlayerOption},
		{"player option 6", protocol.NativeOpPlayer6, zeros(3), ReasonPlayerOption},
		{"player option 7", protocol.NativeOpPlayer7, zeros(3), ReasonPlayerOption},
		{"player option 8", protocol.NativeOpPlayer8, zeros(3), ReasonPlayerOption},
		{"interact byte 0 follow", protocol.ClientInteract, []byte{0, 0, 0}, ReasonFollowPlayer},
		{"interact byte 1 trade", protocol.ClientInteract, []byte{1, 0, 0}, ReasonTradeRequest},

		{"item on player", protocol.NativeOpPlayerU, zeros(11), ReasonUseItem},
		{"item on npc", protocol.NativeOpNpcU, zeros(11), ReasonUseItem},
		{"item on object", protocol.NativeOpLocU, zeros(15), ReasonUseItem},
		{"item on ground", protocol.NativeOpObjU, zeros(15), ReasonUseItem},
		{"spell on object", protocol.NativeOpLocT, zeros(15), ReasonUseItem},
		{"inventory use on npc", protocol.ClientInventoryUseOn, zeros(9), ReasonUseItem},
		// The inventory kind is implemented since phase 33:
		// this all-zero payload has empty slots, so the use
		// is refused with noAction (not a ReasonUseItem drop).
		{"inventory use on inventory", protocol.ClientInventoryUseOn, []byte{0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0}, ReasonNoAction},

		{"spell on player", protocol.NativeOpPlayerT, zeros(11), ReasonMagicOnPlayer},
		{"spell on ground", protocol.NativeOpLocTAlt, zeros(15), ReasonMagicOnItem},

		{"inventory use", protocol.ClientInventoryUse, zeros(9), ReasonNoAction},

		{"inventory move", protocol.ClientInventoryMove, zeros(4), ReasonNoAction},
		{"widget drag native", protocol.NativeIfButtond, zeros(16), ReasonNoAction},
		{"widget drag high", protocol.ClientIfButtond, zeros(16), ReasonNoAction},

		// The bank opcodes (phase 46): zero payloads decode
		// (empty for the deposits, from/to/mode/tab for the
		// move) and drop with no-action because the bank is
		// closed for this player.
		{"bank deposit inventory", protocol.ClientBankDepositInventory, zeros(0), ReasonNoAction},
		{"bank deposit equipment", protocol.ClientBankDepositEquipment, zeros(0), ReasonNoAction},
		{"bank move", protocol.ClientBankMove, zeros(8), ReasonNoAction},

		{"ground option 1", protocol.NativeOpObj1, zeros(7), ReasonNoAction},
		{"ground option 2", protocol.NativeOpObj2, zeros(7), ReasonNoAction},
		{"ground option 3", protocol.NativeOpObj3, zeros(7), ReasonGroundItemsUnwired},
		{"ground option 4", protocol.NativeOpObj4, zeros(7), ReasonNoAction},
		{"ground option 5", protocol.NativeOpObj5, zeros(7), ReasonNoAction},
		{"ground examine", protocol.NativeExamineObj, zeros(6), ReasonGroundItemExamine},
		{"ground item action high", protocol.ClientGroundItemAction, zeros(17), ReasonGroundItemsUnwired},

		{"widget if_button", protocol.NativeIfButton, zeros(4), ReasonNoAction},
		{"widget if_button_sub", protocol.NativeIfButtonSub, zeros(10), ReasonNoAction},
		{"widget if_buttonx", protocol.NativeIfButtonX, zeros(9), ReasonNoAction},
		{"widget if_button1", protocol.NativeIfButton1, zeros(8), ReasonNoAction},
		{"widget if_button2", protocol.NativeIfButton2, zeros(8), ReasonNoAction},
		{"widget if_button3", protocol.NativeIfButton3, zeros(8), ReasonNoAction},
		{"widget if_button4", protocol.NativeIfButton4, zeros(8), ReasonNoAction},
		{"widget if_button5", protocol.NativeIfButton5, zeros(8), ReasonNoAction},
		{"widget if_button6", protocol.NativeIfButton6, zeros(8), ReasonNoAction},
		{"widget if_button7", protocol.NativeIfButton7, zeros(8), ReasonNoAction},
		{"widget if_button8", protocol.NativeIfButton8, zeros(8), ReasonNoAction},
		{"widget if_button9", protocol.NativeIfButton9, zeros(8), ReasonNoAction},
		{"widget if_button10", protocol.NativeIfButton10, zeros(8), ReasonNoAction},
		{"widget action high", protocol.ClientWidgetAction, zeros(21), ReasonNoAction},
		{"widget target", protocol.NativeIfButtont, zeros(16), ReasonWidgetTarget},
		{"local trigger", protocol.NativeIfTriggeroplocal, append([]byte{0, 12}, zeros(12)...), ReasonNoAction},

		{"varp transmit unported 44", protocol.ClientVarpTransmit, []byte{0, 44, 0, 0, 0, 0}, ReasonVarpTransmitOther},
		{"varp transmit unported 171", protocol.ClientVarpTransmit, []byte{0, 171, 0, 0, 0, 0}, ReasonVarpTransmitOther},
		{"varp transmit audio varp", protocol.ClientVarpTransmit, []byte{0, 0, 0, 0, 0, 1}, ReasonVarpTransmitOther},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Handle(p, game.TurnPacket{Opcode: tc.opcode, Payload: tc.payload}, 7, Deps{})
			want := Outcome{Recognized: true, Reason: tc.reason}
			if got != want {
				t.Errorf("Handle(opcode %d): got %+v, want %+v", tc.opcode, got, want)
			}
		})
	}
}

// TestHandleUnrecognizedOpcodes pins Recognized=false for opcodes no
// listener file owns, including the traps: 50 (OPNPC5, no decode
// branch), 180 (trade_action, Trading system directly), 195/255
// (no-branch "raw" opcodes), and the WALK/MOVE_GAMECLICK opcodes
// the coordinator handles before Handle. The examine opcodes 9/85
// are NOT here: dispatch answers them directly (see
// TestHandleExamine), neither is 62 (dialogue continue, answered
// directly since phase 14 — see TestHandleDialogueContinue), and
// neither are 220-225/214 (world-entity options and set_heading,
// answered directly since phase 21 — see the worldentity tests),
// nor are 243/244/245 (bank deposit/move, answered directly since
// phase 46 — see the bank tests).
func TestHandleUnrecognizedOpcodes(t *testing.T) {
	p := testPlayer()
	opcodes := []byte{
		0, 99, 127,
		protocol.NativeOpNpc5,        // 50
		protocol.ClientMapEdit,       // 195
		protocol.ClientDebug,         // 255
		protocol.ClientWalk,          // 210: coordinator handles first
		protocol.NativeMoveGameclick, // 16: coordinator handles first
		// The social opcodes 181/196/197/198/199 are NOT here:
		// dispatch answers them directly with the social-packet
		// emit (see TestHandleSocialPackets).
	}
	for _, opcode := range opcodes {
		got := Handle(p, game.TurnPacket{Opcode: opcode, Payload: zeros(8)}, 7, Deps{})
		if got != (Outcome{}) {
			t.Errorf("opcode %d: got %+v, want zero Outcome (unrecognized)", opcode, got)
		}
	}
}

// TestHandleMalformedPayloads pins the malformed contract: a known
// opcode with an undecodable payload is recognized and dropped with
// ReasonMalformedPayload, never a panic and never a disconnect (Handle
// has no channel). Empty, truncated, and over-long (fixed-length)
// payloads are covered for every recognized opcode family.
func TestHandleMalformedPayloads(t *testing.T) {
	p := testPlayer()
	// One representative per decoder family: empty, 1-byte, and
	// over-long payloads.
	families := []struct {
		opcode byte
		full   int // table length for fixed opcodes; 0 for variable
	}{
		{protocol.NativeAppearanceSet, 20},
		{protocol.ClientChat, 0},
		{protocol.NativeOpNpc1Alt, 3},
		{protocol.NativeOpNpcT, 11},
		{protocol.NativeOpLoc1, 7},
		{protocol.ClientLocInteract, 0},
		{protocol.NativeOpPlayer1, 3},
		{protocol.ClientInteract, 0},
		{protocol.ClientTradeAction, 0},
		{protocol.NativeOpPlayerU, 11},
		{protocol.NativeOpLocT, 15},
		{protocol.ClientInventoryUseOn, 0},
		{protocol.NativeOpPlayerT, 11},
		{protocol.ClientInventoryUse, 0},
		{protocol.ClientInventoryMove, 4},
		{protocol.ClientBankMove, 0},
		{protocol.NativeIfButtond, 16},
		{protocol.NativeOpObj1, 7},
		{protocol.NativeExamineObj, 6},
		{protocol.ClientGroundItemAction, 0},
		{protocol.NativeIfButton, 4},
		{protocol.ClientWidgetAction, 0},
		{protocol.NativeIfButtont, 16},
		{protocol.NativeIfTriggeroplocal, 0},
		{protocol.ClientVarpTransmit, 6},
		{protocol.NativeOpWorldEntity1, 3},
	}
	for _, fam := range families {
		payloads := [][]byte{nil, {}, {0}}
		if fam.full > 0 {
			payloads = append(payloads, zeros(fam.full-1), zeros(fam.full+1))
		}
		for _, payload := range payloads {
			got := Handle(p, game.TurnPacket{Opcode: fam.opcode, Payload: payload}, 7, Deps{})
			want := Outcome{Recognized: true, Reason: ReasonMalformedPayload}
			if got != want {
				t.Errorf("opcode %d payload len %d: got %+v, want %+v", fam.opcode, len(payload), got, want)
			}
		}
	}
	// A bad IF_TRIGGEROPLOCAL block length is malformed, not a panic.
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeIfTriggeroplocal, Payload: append([]byte{0, 11}, zeros(11)...)}, 7, Deps{})
	if got != (Outcome{Recognized: true, Reason: ReasonMalformedPayload}) {
		t.Errorf("bad block length: got %+v", got)
	}
	// Unterminated chat string is malformed.
	got = Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: []byte{0, 'x', 'y'}}, 7, Deps{})
	if got != (Outcome{Recognized: true, Reason: ReasonMalformedPayload}) {
		t.Errorf("unterminated chat: got %+v", got)
	}
}

// TestHandleGarbageFullLength pins that garbage bytes at the table
// length decode (no field is range-checked) and drop with the
// listener reason, not the malformed reason.
func TestHandleGarbageFullLength(t *testing.T) {
	p := testPlayer()
	full := []byte{0xFF, 0xFF, 0xFF}
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpNpc1Alt, Payload: full}, 7, Deps{})
	if got != (Outcome{Recognized: true, Reason: ReasonNpcOption}) {
		t.Errorf("garbage npc option: got %+v", got)
	}
}

// TestHandleNeverWalks pins the walk-to-target decision for the
// listeners that stay drops: NPC, object, player-option, item-use,
// and spell packets never start a walk-to interaction and never
// queue movement, because each TS walk in those files needs an
// unported lookup plus an arrival callback. Ground-item packets
// are excluded: pickup DOES walk, through the queue's walk-to
// machinery, and is pinned by the pickup tests. (The former
// Deps.WalkTo seam was removed in phase 9: the queue carries
// walk-to itself. See Deps.)
func TestHandleNeverWalks(t *testing.T) {
	walkOpcodes := []struct {
		opcode  byte
		payload []byte
	}{
		{protocol.NativeOpNpc1Alt, zeros(3)},
		{protocol.NativeOpLoc1, zeros(7)},
		{protocol.ClientLocInteract, zeros(9)},
		{protocol.NativeOpPlayer2, zeros(3)},
		{protocol.NativeOpPlayer3, zeros(3)},
		{protocol.NativeOpPlayerU, zeros(11)},
		{protocol.NativeOpNpcU, zeros(11)},
		{protocol.NativeOpLocU, zeros(15)},
		{protocol.NativeOpLocT, zeros(15)},
		{protocol.NativeOpLocTAlt, zeros(15)},
		{protocol.ClientInventoryUseOn, zeros(9)},
	}
	for _, tc := range walkOpcodes {
		p := testPlayer()
		Handle(p, game.TurnPacket{Opcode: tc.opcode, Payload: tc.payload}, 7, Deps{})
		if p.MovementQueue().WalkToPending() {
			t.Errorf("opcode %d: started a walk-to interaction", tc.opcode)
		}
		if p.MovementQueue().HasPendingWork() {
			t.Errorf("opcode %d: queued movement work", tc.opcode)
		}
	}
	// A ground-item take with no store wired also walks nowhere:
	// the unwired outcome is returned before any queue work.
	p := testPlayer()
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpObj3, Payload: zeros(7)}, 7, Deps{})
	if got != (Outcome{Recognized: true, Reason: ReasonGroundItemsUnwired}) {
		t.Errorf("unwired take: got %+v", got)
	}
	if p.MovementQueue().WalkToPending() || p.MovementQueue().HasPendingWork() {
		t.Error("unwired take: queued walk-to/movement work")
	}
}

// TestHandleNilSafety pins that Handle never dereferences the player:
// a nil player and zero deps still yield the documented drop.
func TestHandleNilSafety(t *testing.T) {
	got := Handle(nil, game.TurnPacket{Opcode: protocol.NativeOpNpc1Alt, Payload: zeros(3)}, 7, Deps{})
	if got != (Outcome{Recognized: true, Reason: ReasonNpcOption}) {
		t.Errorf("nil player: got %+v", got)
	}
	got = Handle(nil, game.TurnPacket{Opcode: 99, Payload: nil}, 7, Deps{})
	if got != (Outcome{}) {
		t.Errorf("nil player unknown opcode: got %+v", got)
	}
}

func TestStatuses(t *testing.T) {
	rows := Statuses()
	wantListeners := []string{
		"ChangeAppearancePacketListener",
		"ChatPacketListener",
		"CommandPacketListener",
		"DropItemPacketListener",
		"EquipPacketListener",
		"FollowPlayerPacketListener",
		"InterfaceActionClickOpcode",
		"ItemActionPacketListener",
		"MagicOnItemPacketListener",
		"MagicOnPlayerPacketListener",
		"NPCOptionPacketListener",
		"ObjectActionPacketListener",
		"PickupItemPacketListener",
		"PlayerOptionPacketListener",
		"SecondGroundItemOptionPacketListener",
		"SwitchItemSlotPacketListener",
		"TradeRequestPacketListener",
		"UseItemPacketListener",
		"DialoguePromptAnswers",
		"InterfaceCloseDispatch",
		"VarpTransmitRunToggle",
		"SocialPacketDispatch",
	}
	if len(rows) != len(wantListeners) {
		t.Fatalf("Statuses() = %d rows, want %d", len(rows), len(wantListeners))
	}
	// Implemented rows: Chat, Command, DropItem, Equip, ItemAction,
	// InterfaceActionClickOpcode, NPCOption, ObjectAction,
	// PickupItem, PlayerOption (options 1-2 attack/trade;
	// options 3-8 decode-drop), SwitchItemSlot, TradeRequest,
	// UseItem (item-on-object only; the other item-use paths
	// still decode-drop), the dialogue prompt answers, the
	// interface-close dispatch, the run toggle, the
	// social-packet dispatch, and the spell listeners
	// (MagicOnItem: alchemy + telegrab; MagicOnPlayer).
	// Every other row is a decode-drop naming its missing
	// system.
	implemented := map[string]bool{
		"ChatPacketListener": true, "CommandPacketListener": true, "DropItemPacketListener": true, "EquipPacketListener": true,
		"ItemActionPacketListener": true, "InterfaceActionClickOpcode": true, "NPCOptionPacketListener": true, "ObjectActionPacketListener": true, "PickupItemPacketListener": true,
		"PlayerOptionPacketListener": true, "SwitchItemSlotPacketListener": true, "TradeRequestPacketListener": true, "UseItemPacketListener": true, "DialoguePromptAnswers": true, "InterfaceCloseDispatch": true, "VarpTransmitRunToggle": true, "SocialPacketDispatch": true, "MagicOnItemPacketListener": true, "MagicOnPlayerPacketListener": true,
	}
	for i, want := range wantListeners {
		if rows[i].Listener != want {
			t.Errorf("row %d: Listener %q, want %q", i, rows[i].Listener, want)
		}
		if rows[i].Opcodes == "" {
			t.Errorf("row %s: empty Opcodes", want)
		}
		if implemented[want] {
			if !strings.HasPrefix(rows[i].Action, "implemented: ") {
				t.Errorf("row %s: Action %q, want implemented prefix", want, rows[i].Action)
			}
		} else {
			if !strings.HasPrefix(rows[i].Action, "decode-drop: ") {
				t.Errorf("row %s: Action %q, want decode-drop prefix", want, rows[i].Action)
			}
		}
		if rows[i].Action == "decode-drop: " {
			t.Errorf("row %s: empty drop reason", want)
		}
	}
	// The run-toggle row pins the exact implemented wording the
	// coordinator reports in PORT_STATUS.md. It is found by
	// name, not by position: dispatch-inline rows append after
	// it (the social-packet dispatch is last).
	var toggleRow Status
	for _, row := range rows {
		if row.Listener == "VarpTransmitRunToggle" {
			toggleRow = row
		}
	}
	wantToggleAction := "implemented: setRunning(value != 0 && getRunEnergy() > 0), then send run status; player:run-toggled plugin event and audio-setting application NOT STARTED"
	if toggleRow.Action != wantToggleAction {
		t.Errorf("run-toggle Action:\n got %q\nwant %q", toggleRow.Action, wantToggleAction)
	}
	if !strings.Contains(toggleRow.Opcodes, "191 (VARP_TRANSMIT)") {
		t.Errorf("run-toggle Opcodes: %q", toggleRow.Opcodes)
	}
	// Spot checks on the grounded details.
	if !strings.Contains(rows[2].Opcodes, "none of its own") {
		t.Errorf("Command row opcodes: %q", rows[2].Opcodes)
	}
	if !strings.Contains(rows[6].Opcodes, "not a registered listener") {
		t.Errorf("InterfaceActionClickOpcode row opcodes: %q", rows[6].Opcodes)
	}
	if !strings.Contains(rows[10].Opcodes, "50/OPNPC5 has no decode branch") {
		t.Errorf("NPC row opcodes: %q", rows[10].Opcodes)
	}
	if !strings.Contains(rows[16].Opcodes, "OPPLAYER2") {
		t.Errorf("TradeRequest row opcodes: %q", rows[16].Opcodes)
	}
	// Per-listener reasons are distinct: each row names its own system.
	seen := map[string]string{}
	for _, row := range rows {
		if prev, dup := seen[row.Action]; dup {
			t.Errorf("duplicate Action %q for %s and %s", row.Action, prev, row.Listener)
		}
		seen[row.Action] = row.Listener
	}
}

func TestDropReasonsNonEmpty(t *testing.T) {
	reasons := []DropReason{
		ReasonChangeAppearance, ReasonCommand,
		ReasonFollowPlayer, ReasonInterfaceAction, ReasonItemAction,
		ReasonMagicOnItem, ReasonMagicOnPlayer, ReasonNpcOption, ReasonObjectAction,
		ReasonPlayerOption, ReasonSecondGroundItemOption,
		ReasonTradeRequest, ReasonUseItem,
		ReasonGroundItemExamine,
		ReasonWidgetAction, ReasonWidgetTarget,
		ReasonVarpTransmitOther, ReasonNoPlayer,
		ReasonMalformedPayload,
		ReasonNoAction, ReasonGroundItemsUnwired, ReasonDestroyInterface,
		ReasonItemExamine,
	}
	for _, r := range reasons {
		if r == "" {
			t.Error("empty DropReason constant")
		}
	}
}

// runEnergyCall is one recorded SendRunEnergy call: the exact
// arguments the run-energy packet is sent with.
type runEnergyCall struct {
	units, weightKg, staminaTicks int
	running                       bool
}

// recordingSession is a game.SessionPort test double recording
// SendRunEnergy calls, so the run-toggle tests can pin the exact
// packet arguments (units, running, weight, stamina).
type recordingSession struct {
	runEnergy []runEnergyCall
}

var _ game.SessionPort = (*recordingSession)(nil)

func (s *recordingSession) SendPacket([]byte) {}
func (s *recordingSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
	s.runEnergy = append(s.runEnergy, runEnergyCall{units: units, running: running, weightKg: weightKg, staminaTicks: staminaTicks})
}
func (s *recordingSession) SendDestination(int, int) {}
func (s *recordingSession) SendVarp(int, int)        {}
func (s *recordingSession) FlushTick(int64) error    { return nil }
func (s *recordingSession) Close(int, string)        {}
func (s *recordingSession) IsOpen() bool             { return true }

// varpPayload encodes a varp_transmit payload the way the client
// sends it: varpId unsigned short, value signed int, big-endian
// (ClientProtocol.ts line 905).
func varpPayload(varpID int, value int32) []byte {
	payload := make([]byte, 6)
	binary.BigEndian.PutUint16(payload[0:2], uint16(varpID))
	binary.BigEndian.PutUint32(payload[2:6], uint32(value))
	return payload
}

// weightDefs is an ItemDefinitionSource fake for the weight
// pass-through case: only the three methods the weight
// computation reads are overridden; the embedded nil interface
// is never called.
type weightDefs struct {
	game.ItemDefinitionSource
	grams map[int]int
}

func (d weightDefs) IsStackable(int) bool   { return false }
func (d weightDefs) IsNoted(int) bool       { return false }
func (d weightDefs) WeightGrams(id int) int { return d.grams[id] }

// TestHandleVarpTransmitToggle pins the first ACTED outcome: varpId
// 173 applies setRunning(value != 0 && getRunEnergy() > 0) and sends
// exactly one run-energy packet with live units, live running state,
// live weight, and stamina 0 — unconditionally, including when zero
// energy blocks the toggle (NetworkBuilder.ts lines 1161-1163).
func TestHandleVarpTransmitToggle(t *testing.T) {
	cases := []struct {
		name     string
		startRun bool
		units    int
		weightKg int
		value    int32
		wantRun  bool
	}{
		{"toggle on", false, 10000, 0, 1, true},
		{"toggle on nonzero value", false, 10000, 0, 2, true},
		{"toggle on negative value", false, 10000, 0, -1, true},
		{"toggle on partial energy", false, 50, 0, 1, true},
		{"toggle off", true, 10000, 0, 0, false},
		{"toggle off already off", false, 10000, 0, 0, false},
		{"zero energy blocks toggle on", false, 0, 0, 1, false},
		{"zero energy value zero turns off", true, 0, 0, 0, false},
		{"weight passes through", false, 5000, 12, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := testPlayer()
			p.SetRunning(tc.startRun)
			p.SetRunEnergyUnits(tc.units)
			// Weight is the live carried weight: one inventory
			// item weighing weightKg kilograms.
			p.SetItemDefinitions(weightDefs{grams: map[int]int{1: tc.weightKg * 1000}})
			if tc.weightKg != 0 {
				p.Inventory().SetItem(0, game.NewItem(1, 1))
			}
			sess := &recordingSession{}
			p.Session = sess
			got := Handle(p, game.TurnPacket{Opcode: protocol.ClientVarpTransmit, Payload: varpPayload(173, tc.value)}, 7, Deps{})
			if got != (Outcome{Recognized: true, Implemented: true}) {
				t.Fatalf("got %+v, want acted outcome", got)
			}
			if p.IsRunning() != tc.wantRun {
				t.Errorf("running = %v, want %v", p.IsRunning(), tc.wantRun)
			}
			want := []runEnergyCall{{units: tc.units, running: tc.wantRun, weightKg: tc.weightKg, staminaTicks: 0}}
			if len(sess.runEnergy) != len(want) {
				t.Fatalf("run-energy sends = %v, want %v", sess.runEnergy, want)
			}
			for i := range want {
				if sess.runEnergy[i] != want[i] {
					t.Errorf("send %d = %+v, want %+v", i, sess.runEnergy[i], want[i])
				}
			}
		})
	}
}

// TestHandleVarpTransmitOtherVarpIDs pins that every varpId other
// than 173, 43 and 172 decode-drops with the named reason, changes
// nothing, and sends no packet — the packet IS recognized and
// decoded, never "unrecognized". (43 and 172 act now — combat style
// and auto-retaliate — pinned by TestHandleVarpTransmitCombatStyle
// and TestHandleVarpTransmitAutoRetaliate.)
func TestHandleVarpTransmitOtherVarpIDs(t *testing.T) {
	for _, varpID := range []int{0, 1, 174, 1000, 65535} {
		for _, value := range []int32{0, 1, -1} {
			p := testPlayer()
			p.SetRunning(false)
			sess := &recordingSession{}
			p.Session = sess
			got := Handle(p, game.TurnPacket{Opcode: protocol.ClientVarpTransmit, Payload: varpPayload(varpID, value)}, 7, Deps{})
			if got != (Outcome{Recognized: true, Reason: ReasonVarpTransmitOther}) {
				t.Errorf("varpId %d value %d: got %+v", varpID, value, got)
			}
			if p.IsRunning() {
				t.Errorf("varpId %d value %d: changed running", varpID, value)
			}
			if len(sess.runEnergy) != 0 {
				t.Errorf("varpId %d value %d: sent %d run-energy packets", varpID, value, len(sess.runEnergy))
			}
		}
	}
}

// TestHandleVarpTransmitMalformed pins that an undecodable
// varp_transmit payload drops malformed and acts on nothing.
func TestHandleVarpTransmitMalformed(t *testing.T) {
	payloads := [][]byte{nil, {}, {0}, {0, 173}, {0, 173, 0, 0, 0}, append(varpPayload(173, 1), 0)}
	for _, payload := range payloads {
		p := testPlayer()
		p.SetRunning(false)
		sess := &recordingSession{}
		p.Session = sess
		got := Handle(p, game.TurnPacket{Opcode: protocol.ClientVarpTransmit, Payload: payload}, 7, Deps{})
		if got != (Outcome{Recognized: true, Reason: ReasonMalformedPayload}) {
			t.Errorf("payload len %d: got %+v", len(payload), got)
		}
		if p.IsRunning() || len(sess.runEnergy) != 0 {
			t.Errorf("payload len %d: malformed packet acted", len(payload))
		}
	}
}

// TestHandleVarpTransmitNilSession pins that the toggle still flips
// the setting for a session-less player (Player.SendRunEnergy is
// nil-safe): no panic, Implemented outcome, setting changed.
func TestHandleVarpTransmitNilSession(t *testing.T) {
	p := testPlayer() // Session is nil: no client attached.
	p.SetRunning(false)
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientVarpTransmit, Payload: varpPayload(173, 1)}, 7, Deps{})
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("got %+v, want acted outcome", got)
	}
	if !p.IsRunning() {
		t.Error("nil-session toggle did not flip the setting")
	}
}

// TestHandleVarpTransmitNilPlayer pins the explicit nil-player
// decision: every varpId drops with the no-player reason (TS guards
// the case on `if (this.player)`, NetworkBuilder.ts line 1148, and
// the nil check precedes the varpId switch), and malformed stays
// malformed. None may panic.
func TestHandleVarpTransmitNilPlayer(t *testing.T) {
	got := Handle(nil, game.TurnPacket{Opcode: protocol.ClientVarpTransmit, Payload: varpPayload(173, 1)}, 7, Deps{})
	if got != (Outcome{Recognized: true, Reason: ReasonNoPlayer}) {
		t.Errorf("nil player toggle: got %+v", got)
	}
	got = Handle(nil, game.TurnPacket{Opcode: protocol.ClientVarpTransmit, Payload: varpPayload(43, 1)}, 7, Deps{})
	if got != (Outcome{Recognized: true, Reason: ReasonNoPlayer}) {
		t.Errorf("nil player combat style: got %+v", got)
	}
	got = Handle(nil, game.TurnPacket{Opcode: protocol.ClientVarpTransmit, Payload: []byte{0}}, 7, Deps{})
	if got != (Outcome{Recognized: true, Reason: ReasonMalformedPayload}) {
		t.Errorf("nil player malformed: got %+v", got)
	}
}

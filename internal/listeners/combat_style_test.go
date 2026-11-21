package listeners

import (
	"encoding/binary"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// combatStyleSession is a game.SessionPort + game.CombatStyleSender
// test double recording the combat-style sends, so the listener
// wiring tests can pin the exact varp/varbit/tab/string traffic.
type combatStyleSession struct {
	varps         [][2]int
	varbits       [][2]int
	tabInterfaces [][2]int
	strings       [][2]any
	displayStates [][2]any
}

var _ game.SessionPort = (*combatStyleSession)(nil)

func (s *combatStyleSession) SendPacket([]byte)                 {}
func (s *combatStyleSession) SendRunEnergy(int, bool, int, int) {}
func (s *combatStyleSession) SendDestination(int, int)          {}
func (s *combatStyleSession) SendVarp(id, value int) {
	s.varps = append(s.varps, [2]int{id, value})
}
func (s *combatStyleSession) FlushTick(int64) error { return nil }
func (s *combatStyleSession) Close(int, string)     {}
func (s *combatStyleSession) IsOpen() bool          { return true }
func (s *combatStyleSession) SendVarbit(id, value int) {
	s.varbits = append(s.varbits, [2]int{id, value})
}
func (s *combatStyleSession) SendTabInterface(tabID, interfaceID int) {
	s.tabInterfaces = append(s.tabInterfaces, [2]int{tabID, interfaceID})
}
func (s *combatStyleSession) SendString(text string, uid int) {
	s.strings = append(s.strings, [2]any{text, uid})
}
func (s *combatStyleSession) SendInterfaceDisplayState(uid int, hide bool) {
	s.displayStates = append(s.displayStates, [2]any{uid, hide})
}

func (s *combatStyleSession) lastVarp(id int) (int, bool) {
	for i := len(s.varps) - 1; i >= 0; i-- {
		if s.varps[i][0] == id {
			return s.varps[i][1], true
		}
	}
	return 0, false
}

func (s *combatStyleSession) lastVarbit(id int) (int, bool) {
	for i := len(s.varbits) - 1; i >= 0; i-- {
		if s.varbits[i][0] == id {
			return s.varbits[i][1], true
		}
	}
	return 0, false
}

// installUnarmedStyles installs the UNARMED-only fallback
// registry for one test (unit tests use hand-built fixtures
// only, so the shipped file is never loaded here) and restores
// the previous registry afterwards.
func installUnarmedStyles(t *testing.T) {
	t.Helper()
	previous := game.InstallCombatStyles(nil)
	t.Cleanup(func() { game.InstallCombatStyles(previous) })
}

// styleListenerDefs is an ItemDefinitionSource fake for the
// equip wiring tests: the scimitar is a wieldable weapon with
// a WHIP interface name (unresolvable through the UNARMED-only
// fallback, so the assign falls back to UNARMED — the test
// pins that the assign runs on equip, not the category).
type styleListenerDefs struct {
	game.ItemDefinitionSource
}

func (styleListenerDefs) IsStackable(int) bool    { return false }
func (styleListenerDefs) IsNoted(int) bool        { return false }
func (styleListenerDefs) WeightGrams(int) int     { return 0 }
func (styleListenerDefs) IsDoubleHanded(int) bool { return false }
func (styleListenerDefs) EquipmentSlot(id int) int {
	if id == 1333 {
		return game.WeaponSlot
	}
	return -1
}
func (styleListenerDefs) Requirements(int) []int    { return nil }
func (styleListenerDefs) CombatRequirement(int) int { return 0 }
func (styleListenerDefs) IsTradeable(int) bool      { return true }
func (styleListenerDefs) IsSellable(int) bool       { return true }
func (styleListenerDefs) IsDropable(int) bool       { return true }
func (styleListenerDefs) UnNote(id int) int         { return id }
func (styleListenerDefs) PlaceholderID(int) int     { return -1 }
func (styleListenerDefs) WeaponInterfaceName(id int) string {
	if id == 1333 {
		return "WHIP"
	}
	return ""
}
func (styleListenerDefs) ItemName(id int) string {
	if id == 1333 {
		return "Rune scimitar"
	}
	return ""
}
func (styleListenerDefs) ItemBonuses(int) []float64 { return nil }

func TestHandleVarpTransmitCombatStyle(t *testing.T) {
	installUnarmedStyles(t)
	p := testPlayer()
	session := &combatStyleSession{}
	p.Session = session
	// Slot 1 on the UNARMED interface is KICK: the varp acts.
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientVarpTransmit, Payload: varpPayload(43, 1)}, 7, Deps{})
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("varp 43 outcome = %+v, want acted", got)
	}
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "UNARMED_KICK" {
		t.Fatalf("fight type = %+v, want UNARMED_KICK", ft)
	}
	if v, ok := session.lastVarp(43); !ok || v != 1 {
		t.Fatalf("varp 43 sent = %d/%v, want 1/true", v, ok)
	}
	// Slot 2 does not exist on UNARMED: the dispatch still
	// acts (TS ignores changeCombatStyle's return) but the
	// style does not move.
	got = Handle(p, game.TurnPacket{Opcode: protocol.ClientVarpTransmit, Payload: varpPayload(43, 2)}, 7, Deps{})
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("varp 43 slot 2 outcome = %+v, want acted", got)
	}
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "UNARMED_KICK" {
		t.Fatalf("fight type after slot 2 = %+v, want UNARMED_KICK", ft)
	}
	// No player: the drop names the missing player.
	got = Handle(nil, game.TurnPacket{Opcode: protocol.ClientVarpTransmit, Payload: varpPayload(43, 1)}, 7, Deps{})
	if got.Reason != ReasonNoPlayer {
		t.Fatalf("nil-player varp 43 reason = %q, want ReasonNoPlayer", got.Reason)
	}
}

func TestHandleVarpTransmitAutoRetaliate(t *testing.T) {
	installUnarmedStyles(t)
	p := testPlayer()
	session := &combatStyleSession{}
	p.Session = session
	if !p.GetCombat().AutoRetaliate() {
		t.Fatal("auto-retaliate must default on")
	}
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientVarpTransmit, Payload: varpPayload(172, 1)}, 7, Deps{})
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("varp 172 outcome = %+v, want acted", got)
	}
	if p.GetCombat().AutoRetaliate() {
		t.Fatal("varp 172 = 1 must disable auto-retaliate")
	}
	if v, ok := session.lastVarp(172); !ok || v != 1 {
		t.Fatalf("varp 172 echo = %d/%v, want 1/true", v, ok)
	}
	got = Handle(p, game.TurnPacket{Opcode: protocol.ClientVarpTransmit, Payload: varpPayload(172, 0)}, 7, Deps{})
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("varp 172 = 0 outcome = %+v, want acted", got)
	}
	if !p.GetCombat().AutoRetaliate() {
		t.Fatal("varp 172 = 0 must enable auto-retaliate")
	}
	// An unrelated varp still drops with the named reason.
	got = Handle(p, game.TurnPacket{Opcode: protocol.ClientVarpTransmit, Payload: varpPayload(999, 1)}, 7, Deps{})
	if got.Reason != ReasonVarpTransmitOther {
		t.Fatalf("varp 999 reason = %q, want ReasonVarpTransmitOther", got.Reason)
	}
}

func TestEquipWeaponAssignsInterface(t *testing.T) {
	installUnarmedStyles(t)
	p := testPlayer()
	p.SetItemDefinitions(styleListenerDefs{})
	session := &combatStyleSession{}
	p.Session = session
	p.Inventory().SetItem(0, game.NewItem(1333, 1))
	if !equip(p, 1333, 0, game.InventoryInterfaceID) {
		t.Fatal("equip must succeed")
	}
	if p.Equipment().ItemAt(game.WeaponSlot).ID() != 1333 {
		t.Fatal("scimitar must be wielded")
	}
	// The assign ran on equip: category varbit, tab mount and
	// varp 43 went out (UNARMED fallback: the WHIP name does
	// not resolve without the shipped registry).
	if v, ok := session.lastVarbit(357); !ok || v != 0 {
		t.Fatalf("varbit 357 = %d/%v, want 0/true", v, ok)
	}
	if len(session.tabInterfaces) != 1 || session.tabInterfaces[0] != [2]int{0, 5855} {
		t.Fatalf("tab interfaces = %v, want [[0 5855]]", session.tabInterfaces)
	}
	if v, ok := session.lastVarp(43); !ok || v != 0 {
		t.Fatalf("varp 43 = %d/%v, want 0/true", v, ok)
	}
	// Unequip re-assigns (back to unarmed, same sends again).
	session.varps = nil
	session.varbits = nil
	session.tabInterfaces = nil
	if !unequip(p, game.WeaponSlot) {
		t.Fatal("unequip must succeed")
	}
	if v, ok := session.lastVarp(43); !ok || v != 0 {
		t.Fatalf("varp 43 after unequip = %d/%v, want 0/true", v, ok)
	}
	if len(session.tabInterfaces) != 1 {
		t.Fatalf("tab interfaces after unequip = %v, want one mount", session.tabInterfaces)
	}
}

func TestWidgetStyleAndAutoRetaliateButtons(t *testing.T) {
	installUnarmedStyles(t)
	p := testPlayer()
	session := &combatStyleSession{}
	p.Session = session
	button := func(child int) game.TurnPacket {
		payload := make([]byte, 4)
		binary.BigEndian.PutUint32(payload, uint32((593<<16)|child))
		return game.TurnPacket{Opcode: protocol.NativeIfButton, Payload: payload}
	}
	// Style button 593:10 is slot 1 (UNARMED_KICK).
	got := Handle(p, button(10), 7, Deps{})
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("style button outcome = %+v, want acted", got)
	}
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "UNARMED_KICK" {
		t.Fatalf("fight type = %+v, want UNARMED_KICK", ft)
	}
	if v, ok := session.lastVarp(43); !ok || v != 1 {
		t.Fatalf("varp 43 = %d/%v, want 1/true", v, ok)
	}
	// Auto-retaliate button 593:32 toggles the setting.
	got = Handle(p, button(32), 7, Deps{})
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("auto-retaliate button outcome = %+v, want acted", got)
	}
	if p.GetCombat().AutoRetaliate() {
		t.Fatal("auto-retaliate button must toggle the setting off")
	}
	if v, ok := session.lastVarp(172); !ok || v != 1 {
		t.Fatalf("varp 172 = %d/%v, want 1/true", v, ok)
	}
	// A combat-tab child that is neither a style button nor
	// the auto-retaliate button falls through unclaimed.
	got = Handle(p, button(7), 7, Deps{})
	if got.Implemented {
		t.Fatalf("unclaimed combat button outcome = %+v, want not implemented", got)
	}
}

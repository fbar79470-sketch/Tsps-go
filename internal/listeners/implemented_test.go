package listeners

import (
	"encoding/binary"
	"encoding/json"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Fixture item ids. Hand-built; none of these resolve through a real
// cache.
const (
	itemBronzeSword  = 100 // weapon slot, not stackable
	itemIronSword    = 101 // weapon slot, not stackable
	itemBronzeShield = 102 // shield slot
	itemTwoHander    = 103 // weapon slot, double-handed
	itemArrows       = 104 // ammunition slot, stackable
	itemRing         = 105 // ring slot
	itemReqSword     = 106 // weapon slot, Attack 40 requirement
	itemPlain        = 107 // no equipment slot, droppable
	itemUndroppable  = 108 // no equipment slot, undroppable
	itemSearchable   = 110 // custom ground actions: Search at index 2
	itemReplacement  = 200 // pickup-replacement id
)

// defConfig is one fixture item's definition surface.
type defConfig struct {
	slot                      int
	stackable, doubleHanded   bool
	dropable, tradeable       bool
	requirements              []int
	combatReq                 int
	invActions, groundActions []string
	shiftClick                int
	shiftClickSet             bool
}

// defTable implements the 12 ItemDefinitionSource methods over a
// fixture map. Unknown ids read the DEFAULT answers: no slot, not
// stackable, droppable, tradeable, no requirements.
type defTable struct {
	cfgs map[int]defConfig
}

func (t *defTable) cfg(id int) defConfig {
	if cfg, ok := t.cfgs[id]; ok {
		return cfg
	}
	return defConfig{slot: -1, dropable: true, tradeable: true}
}

func (t *defTable) IsStackable(id int) bool      { return t.cfg(id).stackable }
func (t *defTable) IsNoted(int) bool             { return false }
func (t *defTable) WeightGrams(int) int          { return 0 }
func (t *defTable) EquipmentSlot(id int) int     { return t.cfg(id).slot }
func (t *defTable) IsDoubleHanded(id int) bool   { return t.cfg(id).doubleHanded }
func (t *defTable) Requirements(id int) []int    { return t.cfg(id).requirements }
func (t *defTable) CombatRequirement(id int) int { return t.cfg(id).combatReq }
func (t *defTable) IsTradeable(id int) bool      { return t.cfg(id).tradeable }
func (t *defTable) IsSellable(id int) bool       { return t.cfg(id).tradeable }
func (t *defTable) IsDropable(id int) bool       { return t.cfg(id).dropable }
func (t *defTable) UnNote(id int) int            { return id }
func (t *defTable) PlaceholderID(int) int        { return -1 }

// fakeDefs is a defTable that also implements ItemActionSource: the
// configured action lists, or the ObjType defaults when unset, with
// the ObjType default shift-click rule (option 5 when it reads
// Drop) when no shift-click index is configured.
type fakeDefs struct{ *defTable }

func (d fakeDefs) InventoryActions(id int) []string {
	if actions := d.cfg(id).invActions; actions != nil {
		out := make([]string, len(actions))
		copy(out, actions)
		return out
	}
	return game.DefaultInventoryActions()
}

func (d fakeDefs) GroundActions(id int) []string {
	if actions := d.cfg(id).groundActions; actions != nil {
		out := make([]string, len(actions))
		copy(out, actions)
		return out
	}
	return game.DefaultGroundActions()
}

func (d fakeDefs) ShiftClickIndex(id int) int {
	cfg := d.cfg(id)
	if cfg.shiftClickSet {
		return cfg.shiftClick
	}
	if actions := d.InventoryActions(id); len(actions) == 5 && actions[4] == "Drop" {
		return 4
	}
	return -1
}

// noActionDefs is a defTable WITHOUT ItemActionSource: the routing
// degeneration path (ObjType defaults) without a nil definition
// source.
type noActionDefs struct{ *defTable }

// fixtureDefs builds the standard fixture table.
func fixtureDefs() fakeDefs {
	req := make([]int, 24)
	req[game.SkillAttack] = 40
	return fakeDefs{&defTable{cfgs: map[int]defConfig{
		itemBronzeSword:  {slot: game.WeaponSlot, dropable: true, tradeable: true, invActions: []string{"Wield", "", "", "", "Drop"}},
		itemIronSword:    {slot: game.WeaponSlot, dropable: true, tradeable: true, invActions: []string{"Wield", "", "", "", "Drop"}},
		itemBronzeShield: {slot: game.ShieldSlot, dropable: true, tradeable: true, invActions: []string{"Wear", "", "", "", "Drop"}},
		itemTwoHander:    {slot: game.WeaponSlot, doubleHanded: true, dropable: true, tradeable: true, invActions: []string{"Wield", "", "", "", "Drop"}},
		itemArrows:       {slot: game.AmmunitionSlot, stackable: true, dropable: true, tradeable: true, invActions: []string{"Wield", "", "", "", "Drop"}},
		itemRing:         {slot: game.RingSlot, dropable: true, tradeable: true, invActions: []string{"Wear", "", "", "", "Drop"}},
		itemReqSword:     {slot: game.WeaponSlot, dropable: true, tradeable: true, requirements: req, invActions: []string{"Wield", "", "", "", "Drop"}},
		itemPlain:        {slot: -1, dropable: true, tradeable: true},
		itemUndroppable:  {slot: -1, dropable: false, tradeable: true},
		itemSearchable:   {slot: -1, dropable: true, tradeable: true, groundActions: []string{"", "Search", "Take", "", ""}},
		itemReplacement:  {slot: -1, dropable: true, tradeable: true},
	}}}
}

// managerStore is a GroundItems seam backed by the REAL
// GroundItemManager (real stack ids, real CanSee) plus an
// enumeration list. It is constructed with no stackable function,
// so Register never merges and the enumeration list is exactly the
// manager's list. Tests must not register the same stack twice.
type managerStore struct {
	m         *game.GroundItemManager
	items     []*game.ItemOnGround
	performed []performedOp
}

type performedOp struct {
	stack *game.ItemOnGround
	op    game.GroundItemOp
}

func newManagerStore() *managerStore {
	return &managerStore{m: game.NewGroundItemManager(game.GroundItemManagerConfig{})}
}

func newTradeableStore() *managerStore {
	return &managerStore{m: game.NewGroundItemManager(game.GroundItemManagerConfig{
		TradeableDropable: func(int) (bool, bool) { return true, true },
	})}
}

func (s *managerStore) Register(item *game.ItemOnGround) {
	s.m.Register(item)
	s.items = append(s.items, item)
}

func (s *managerStore) Deregister(item *game.ItemOnGround) { s.m.Deregister(item) }

func (s *managerStore) Perform(item *game.ItemOnGround, op game.GroundItemOp) {
	s.performed = append(s.performed, performedOp{stack: item, op: op})
	s.m.Perform(item, op)
}

func (s *managerStore) CanSee(player *game.Player, item *game.ItemOnGround, includePendingRemoval bool) bool {
	return s.m.CanSee(player, item, includePendingRemoval)
}

func (s *managerStore) Items() []*game.ItemOnGround {
	out := make([]*game.ItemOnGround, len(s.items))
	copy(out, s.items)
	return out
}

// tickPlayerQueue mirrors the game package's tick harness:
// begin the cycle, then process when the queue has pending work.
func tickPlayerQueue(p *game.Player, cycle int64) {
	q := p.MovementQueue()
	q.BeginCycle()
	if q.HasPendingWork() {
		q.Process(cycle)
	}
}

func setInv(p *game.Player, slot, id, amount int) {
	p.Inventory().SetItem(slot, game.NewItem(id, amount))
}

func setEq(p *game.Player, slot, id, amount int) {
	p.Equipment().SetItem(slot, game.NewItem(id, amount))
}

func killPlayer(p *game.Player) {
	p.Skills().SetCurrentLevel(game.SkillHitpoints, 0, false)
}

func itemWithMeta(id, amount int, meta map[string]json.RawMessage) game.Item {
	item := game.NewItem(id, amount)
	item.SetMeta(meta)
	return item
}

// Payload encoders. Each inverts the documented decoder reads; the
// decoders themselves are pinned by the protocol tests.

func shortBE(v int) []byte {
	out := make([]byte, 2)
	binary.BigEndian.PutUint16(out, uint16(v))
	return out
}

func intBE(v int32) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(v))
	return out
}

func inventoryUsePayload(slot, itemID int, option string) []byte {
	payload := shortBE(slot)
	payload = append(payload, shortBE(itemID)...)
	payload = append(payload, 0, 0, 0, 0) // discarded int
	payload = append(payload, option...)
	return append(payload, 0)
}

func inventoryMovePayload(from, to int) []byte {
	payload := shortBE(from)
	return append(payload, shortBE(to)...)
}

// groundHighPayload encodes GROUND_ITEM_ACTION (232): stack id int,
// x/y shorts, one skipped byte, item id short, one skipped int, an
// option string, and an option-index byte.
func groundHighPayload(stackID int32, x, y, itemID int, option string, optionIndex byte) []byte {
	payload := intBE(stackID)
	payload = append(payload, shortBE(x)...)
	payload = append(payload, shortBE(y)...)
	payload = append(payload, 0)
	payload = append(payload, shortBE(itemID)...)
	payload = append(payload, 0, 0, 0, 0)
	payload = append(payload, option...)
	payload = append(payload, 0, optionIndex)
	return payload
}

// opObj3Payload encodes native OPOBJ3 (103): itemId shortLE, x
// shortAdd, y shortAddLE, modifierFlags byteSub.
func opObj3Payload(itemID, x, y, mods int) []byte {
	return []byte{
		byte(itemID), byte(itemID >> 8), // shortLE
		byte(x + 128), byte(x >> 8), // shortAdd: low first with +128
		byte(y >> 8), byte(y + 128), // shortAddLE: high first, low +128
		byte(128 - mods), // byteSub
	}
}

// ifButtonPayload encodes a native IF_BUTTONn packet: widget id int,
// slot short, item id short.
func ifButtonPayload(widgetID int32, slot, itemID int) []byte {
	payload := intBE(widgetID)
	payload = append(payload, shortBE(slot)...)
	return append(payload, shortBE(itemID)...)
}

func TestEquipBasic(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setInv(p, 0, itemBronzeSword, 1)
	if !equip(p, itemBronzeSword, 0, game.InventoryInterfaceID) {
		t.Fatal("equip refused a basic wield")
	}
	if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() != itemBronzeSword || got.Amount() != 1 {
		t.Errorf("weapon slot = %+v, want bronze sword x1", got)
	}
	if got := p.Inventory().ItemAt(0); got.ID() != -1 {
		t.Errorf("inventory slot 0 = %+v, want empty", got)
	}
}

func TestEquipSwap(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setEq(p, game.WeaponSlot, itemBronzeSword, 1)
	setInv(p, 3, itemIronSword, 1)
	if !equip(p, itemIronSword, 3, game.InventoryInterfaceID) {
		t.Fatal("equip refused a swap")
	}
	if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() != itemIronSword {
		t.Errorf("weapon slot = %+v, want iron sword", got)
	}
	if got := p.Inventory().ItemAt(3); got.ID() != itemBronzeSword {
		t.Errorf("inventory slot 3 = %+v, want displaced bronze sword", got)
	}
}

func TestEquipStackableMerge(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setEq(p, game.AmmunitionSlot, itemArrows, 5)
	setInv(p, 0, itemArrows, 10)
	if !equip(p, itemArrows, 0, game.InventoryInterfaceID) {
		t.Fatal("equip refused a stackable merge")
	}
	if got := p.Equipment().ItemAt(game.AmmunitionSlot); got.ID() != itemArrows || got.Amount() != 15 {
		t.Errorf("ammunition slot = %+v, want arrows x15", got)
	}
	if got := p.Inventory().ItemAt(0); got.ID() != -1 {
		t.Errorf("inventory slot 0 = %+v, want empty", got)
	}
}

func TestEquipStackableMergeNoRoom(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setEq(p, game.AmmunitionSlot, itemArrows, game.MaxStackAmount-5)
	setInv(p, 0, itemArrows, 10)
	if equip(p, itemArrows, 0, game.InventoryInterfaceID) {
		t.Fatal("equip merged past the stack cap")
	}
	if got := p.Equipment().ItemAt(game.AmmunitionSlot); got.Amount() != game.MaxStackAmount-5 {
		t.Errorf("ammunition slot changed: %+v", got)
	}
	if got := p.Inventory().ItemAt(0); got.ID() != itemArrows || got.Amount() != 10 {
		t.Errorf("inventory slot 0 changed: %+v", got)
	}
}

func TestEquipStackableDifferentMetaDoesNotMerge(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	worn := itemWithMeta(itemArrows, 5, map[string]json.RawMessage{"a": json.RawMessage(`1`)})
	p.Equipment().SetItem(game.AmmunitionSlot, worn)
	setInv(p, 0, itemArrows, 10)
	if !equip(p, itemArrows, 0, game.InventoryInterfaceID) {
		t.Fatal("equip refused a swap onto a different-meta stack")
	}
	// Not the same stack (meta differs), so the worn stack is
	// displaced into the clicked slot and the new stack is worn.
	if got := p.Equipment().ItemAt(game.AmmunitionSlot); got.ID() != itemArrows || got.Amount() != 10 {
		t.Errorf("ammunition slot = %+v, want arrows x10", got)
	}
	if got := p.Inventory().ItemAt(0); got.ID() != itemArrows || got.Amount() != 5 || got.Meta() == nil {
		t.Errorf("inventory slot 0 = %+v, want the displaced meta stack x5", got)
	}
}

func TestEquipTwoHanded(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setEq(p, game.WeaponSlot, itemBronzeSword, 1)
	setEq(p, game.ShieldSlot, itemBronzeShield, 1)
	setInv(p, 2, itemTwoHander, 1)
	if !equip(p, itemTwoHander, 2, game.InventoryInterfaceID) {
		t.Fatal("equip refused a two-hander with room for the shield")
	}
	if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() != itemTwoHander {
		t.Errorf("weapon slot = %+v, want two-hander", got)
	}
	if got := p.Equipment().ItemAt(game.ShieldSlot); got.ID() != -1 {
		t.Errorf("shield slot = %+v, want empty", got)
	}
	// The displaced weapon lands in the clicked slot; the shield
	// takes a free slot.
	if got := p.Inventory().ItemAt(2); got.ID() != itemBronzeSword {
		t.Errorf("inventory slot 2 = %+v, want displaced sword", got)
	}
	if p.Inventory().GetAmount(itemBronzeShield) != 1 {
		t.Error("shield did not return to the inventory")
	}
}

func TestEquipTwoHandedNoFreeSlot(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setEq(p, game.WeaponSlot, itemBronzeSword, 1)
	setEq(p, game.ShieldSlot, itemBronzeShield, 1)
	for slot := 0; slot < p.Inventory().Capacity(); slot++ {
		setInv(p, slot, itemPlain, 1)
	}
	setInv(p, 2, itemTwoHander, 1)
	if equip(p, itemTwoHander, 2, game.InventoryInterfaceID) {
		t.Fatal("equip wielded a two-hander with no free slot for the shield")
	}
	if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() != itemBronzeSword {
		t.Errorf("weapon slot changed: %+v", got)
	}
	if got := p.Equipment().ItemAt(game.ShieldSlot); got.ID() != itemBronzeShield {
		t.Errorf("shield slot changed: %+v", got)
	}
}

func TestEquipTwoHandedEmptyHands(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setEq(p, game.ShieldSlot, itemBronzeShield, 1)
	setInv(p, 2, itemTwoHander, 1)
	if !equip(p, itemTwoHander, 2, game.InventoryInterfaceID) {
		t.Fatal("equip refused a two-hander into empty hands")
	}
	if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() != itemTwoHander {
		t.Errorf("weapon slot = %+v, want two-hander", got)
	}
	if got := p.Equipment().ItemAt(game.ShieldSlot); got.ID() != -1 {
		t.Errorf("shield slot = %+v, want empty", got)
	}
	// The vacated slot is cleared and the stowed shield takes the
	// first empty inventory slot (Add fills from slot 0), as in
	// TS inventory.add.
	if got := p.Inventory().ItemAt(2); got.ID() != -1 {
		t.Errorf("inventory slot 2 = %+v, want empty", got)
	}
	if p.Inventory().GetAmount(itemBronzeShield) != 1 {
		t.Error("shield did not return to the inventory")
	}
}

func TestEquipShieldAgainstTwoHander(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setEq(p, game.WeaponSlot, itemTwoHander, 1)
	setInv(p, 4, itemBronzeShield, 1)
	if !equip(p, itemBronzeShield, 4, game.InventoryInterfaceID) {
		t.Fatal("equip refused a shield against a two-hander")
	}
	if got := p.Equipment().ItemAt(game.ShieldSlot); got.ID() != itemBronzeShield {
		t.Errorf("shield slot = %+v, want shield", got)
	}
	if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() != -1 {
		t.Errorf("weapon slot = %+v, want empty", got)
	}
	if got := p.Inventory().ItemAt(4); got.ID() != itemTwoHander {
		t.Errorf("inventory slot 4 = %+v, want displaced two-hander", got)
	}
}

func TestEquipRequirements(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setInv(p, 0, itemReqSword, 1)
	if equip(p, itemReqSword, 0, game.InventoryInterfaceID) {
		t.Fatal("equip wielded an Attack-40 sword at Attack 1")
	}
	if got := p.Inventory().ItemAt(0); got.ID() != itemReqSword {
		t.Errorf("inventory slot 0 changed: %+v", got)
	}
	p.Skills().SetMaxLevels(game.SkillAttack, 40, false)
	if !equip(p, itemReqSword, 0, game.InventoryInterfaceID) {
		t.Fatal("equip refused an Attack-40 sword at Attack 40")
	}
	if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() != itemReqSword {
		t.Errorf("weapon slot = %+v, want requirement sword", got)
	}
}

func TestEquipGates(t *testing.T) {
	cases := []struct {
		name            string
		setup           func(p *game.Player)
		slot, id, iface int
	}{
		{"slot below range", func(p *game.Player) { setInv(p, 0, itemBronzeSword, 1) }, -1, itemBronzeSword, game.InventoryInterfaceID},
		{"slot above range", func(p *game.Player) { setInv(p, 0, itemBronzeSword, 1) }, 28, itemBronzeSword, game.InventoryInterfaceID},
		{"slot id mismatch", func(p *game.Player) { setInv(p, 0, itemIronSword, 1) }, 0, itemBronzeSword, game.InventoryInterfaceID},
		{"empty slot", func(p *game.Player) {}, 0, itemBronzeSword, game.InventoryInterfaceID},
		{"not wearable", func(p *game.Player) { setInv(p, 0, itemPlain, 1) }, 0, itemPlain, game.InventoryInterfaceID},
		{"wrong interface", func(p *game.Player) { setInv(p, 0, itemBronzeSword, 1) }, 0, itemBronzeSword, game.EquipmentInventoryInterfaceID},
		{"dead player", func(p *game.Player) { setInv(p, 0, itemBronzeSword, 1); killPlayer(p) }, 0, itemBronzeSword, game.InventoryInterfaceID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := testPlayer()
			p.SetItemDefinitions(fixtureDefs())
			tc.setup(p)
			if equip(p, tc.id, tc.slot, tc.iface) {
				t.Error("equip acted despite the gate")
			}
		})
	}
}

func TestUnequip(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setEq(p, game.WeaponSlot, itemBronzeSword, 1)
	if !unequip(p, game.WeaponSlot) {
		t.Fatal("unequip refused a worn sword")
	}
	if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() != -1 {
		t.Errorf("weapon slot = %+v, want empty", got)
	}
	if p.Inventory().GetAmount(itemBronzeSword) != 1 {
		t.Error("sword did not return to the inventory")
	}
}

func TestUnequipIntoExistingStack(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setEq(p, game.AmmunitionSlot, itemArrows, 5)
	setInv(p, 7, itemArrows, 10)
	if !unequip(p, game.AmmunitionSlot) {
		t.Fatal("unequip refused a stackable unequip")
	}
	if got := p.Inventory().ItemAt(7); got.ID() != itemArrows || got.Amount() != 15 {
		t.Errorf("inventory slot 7 = %+v, want arrows x15", got)
	}
	if got := p.Equipment().ItemAt(game.AmmunitionSlot); got.ID() != -1 {
		t.Errorf("ammunition slot = %+v, want empty", got)
	}
}

func TestUnequipNoSpace(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setEq(p, game.WeaponSlot, itemBronzeSword, 1)
	for slot := 0; slot < p.Inventory().Capacity(); slot++ {
		setInv(p, slot, itemPlain, 1)
	}
	if unequip(p, game.WeaponSlot) {
		t.Fatal("unequip removed a sword into a full inventory")
	}
	if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() != itemBronzeSword {
		t.Errorf("weapon slot changed: %+v", got)
	}
}

func TestUnequipGates(t *testing.T) {
	cases := []struct {
		name  string
		setup func(p *game.Player)
		slot  int
	}{
		{"empty slot", func(p *game.Player) {}, game.WeaponSlot},
		{"slot below range", func(p *game.Player) { setEq(p, game.WeaponSlot, itemBronzeSword, 1) }, -1},
		{"slot above range", func(p *game.Player) { setEq(p, game.WeaponSlot, itemBronzeSword, 1) }, 14},
		{"dead player", func(p *game.Player) { setEq(p, game.WeaponSlot, itemBronzeSword, 1); killPlayer(p) }, game.WeaponSlot},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := testPlayer()
			p.SetItemDefinitions(fixtureDefs())
			tc.setup(p)
			if unequip(p, tc.slot) {
				t.Error("unequip acted despite the gate")
			}
		})
	}
}

func TestDropItemRegisters(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setInv(p, 5, itemPlain, 3)
	store := newManagerStore()
	if got := dropItem(p, itemPlain, game.InventoryInterfaceID, 5, store); got != dropDone {
		t.Fatalf("dropItem = %v, want dropDone", got)
	}
	if got := p.Inventory().ItemAt(5); got.ID() != -1 {
		t.Errorf("inventory slot 5 = %+v, want empty", got)
	}
	stacks := store.Items()
	if len(stacks) != 1 {
		t.Fatalf("store has %d stacks, want 1", len(stacks))
	}
	stack := stacks[0]
	if stack.State != game.GroundItemSeenByPlayer {
		t.Errorf("stack state = %v, want SEEN_BY_PLAYER (registers)", stack.State)
	}
	if stack.Owner != p.Username {
		t.Errorf("stack owner = %q, want %q", stack.Owner, p.Username)
	}
	if stack.Position != p.Location() {
		t.Errorf("stack position = %+v, want player location %+v", stack.Position, p.Location())
	}
	if stack.ItemID != itemPlain || stack.Amount != 3 {
		t.Errorf("stack = id %d x%d, want id %d x3", stack.ItemID, stack.Amount, itemPlain)
	}
	if !stack.GoesGlobal || stack.RespawnTimer != -1 {
		t.Errorf("stack goesGlobal=%v respawn=%d, want true/-1", stack.GoesGlobal, stack.RespawnTimer)
	}
	if stack.ID() == 0 {
		t.Error("stack has no world id")
	}
}

func TestDropItemWildernessRegistersGlobally(t *testing.T) {
	// Wilderness.isIn selects registerGlobal (SEEN_BY_EVERYONE,
	// goesGlobal=false) over registers() (DropItemPacketListener
	// .ts:107-111; ItemOnGroundManager.ts:366-392).
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setInv(p, 5, itemPlain, 3)
	store := newManagerStore()
	inWilderness := func(game.Location) bool { return true }
	if got := dropItemChecked(p, itemPlain, game.InventoryInterfaceID, 5, store, inWilderness); got != dropDone {
		t.Fatalf("dropItemChecked = %v, want dropDone", got)
	}
	stacks := store.Items()
	if len(stacks) != 1 {
		t.Fatalf("store has %d stacks, want 1", len(stacks))
	}
	stack := stacks[0]
	if stack.State != game.GroundItemSeenByEveryone {
		t.Errorf("stack state = %v, want SEEN_BY_EVERYONE (registerGlobal)", stack.State)
	}
	if stack.GoesGlobal || stack.RespawnTimer != -1 {
		t.Errorf("stack goesGlobal=%v respawn=%d, want false/-1", stack.GoesGlobal, stack.RespawnTimer)
	}
}

func TestDropItemOutsideWildernessRegisters(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setInv(p, 5, itemPlain, 3)
	store := newManagerStore()
	outside := func(game.Location) bool { return false }
	if got := dropItemChecked(p, itemPlain, game.InventoryInterfaceID, 5, store, outside); got != dropDone {
		t.Fatalf("dropItemChecked = %v, want dropDone", got)
	}
	stack := store.Items()[0]
	if stack.State != game.GroundItemSeenByPlayer {
		t.Errorf("stack state = %v, want SEEN_BY_PLAYER (registers)", stack.State)
	}
	if !stack.GoesGlobal {
		t.Error("stack goesGlobal must be true outside the Wilderness")
	}
}

func TestDropItemMetaPreserved(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	meta := map[string]json.RawMessage{"charges": json.RawMessage(`7`)}
	p.Inventory().SetItem(0, itemWithMeta(itemPlain, 1, meta))
	store := newManagerStore()
	if got := dropItem(p, itemPlain, game.InventoryInterfaceID, 0, store); got != dropDone {
		t.Fatalf("dropItem = %v, want dropDone", got)
	}
	stack := store.Items()[0]
	if !metaMatchesGround(meta, stack.Meta) {
		t.Errorf("stack meta = %v, want %v", stack.Meta, meta)
	}
}

func TestDropItemOutcomes(t *testing.T) {
	cases := []struct {
		name            string
		setup           func(p *game.Player)
		slot, id, iface int
		store           func() GroundItems
		want            dropOutcome
	}{
		{"undroppable", func(p *game.Player) { setInv(p, 0, itemUndroppable, 1) }, 0, itemUndroppable, game.InventoryInterfaceID, func() GroundItems { return newManagerStore() }, dropDestroy},
		{"unwired store", func(p *game.Player) { setInv(p, 0, itemPlain, 1) }, 0, itemPlain, game.InventoryInterfaceID, func() GroundItems { return nil }, dropNoGroundItems},
		{"slot below range", func(p *game.Player) { setInv(p, 0, itemPlain, 1) }, -1, itemPlain, game.InventoryInterfaceID, func() GroundItems { return newManagerStore() }, dropRefused},
		{"slot above range", func(p *game.Player) { setInv(p, 0, itemPlain, 1) }, 28, itemPlain, game.InventoryInterfaceID, func() GroundItems { return newManagerStore() }, dropRefused},
		{"slot id mismatch", func(p *game.Player) { setInv(p, 0, itemBronzeSword, 1) }, 0, itemPlain, game.InventoryInterfaceID, func() GroundItems { return newManagerStore() }, dropRefused},
		{"empty slot", func(p *game.Player) {}, 0, itemPlain, game.InventoryInterfaceID, func() GroundItems { return newManagerStore() }, dropRefused},
		{"wrong interface", func(p *game.Player) { setInv(p, 0, itemPlain, 1) }, 0, itemPlain, game.EquipmentInventoryInterfaceID, func() GroundItems { return newManagerStore() }, dropRefused},
		{"dead player", func(p *game.Player) { setInv(p, 0, itemPlain, 1); killPlayer(p) }, 0, itemPlain, game.InventoryInterfaceID, func() GroundItems { return newManagerStore() }, dropRefused},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := testPlayer()
			p.SetItemDefinitions(fixtureDefs())
			tc.setup(p)
			store := tc.store()
			if got := dropItem(p, tc.id, tc.iface, tc.slot, store); got != tc.want {
				t.Errorf("dropItem = %v, want %v", got, tc.want)
			}
			if store != nil {
				if ms, ok := store.(*managerStore); ok && len(ms.Items()) != 0 {
					t.Errorf("store has %d stacks after a refused drop", len(ms.Items()))
				}
			}
		})
	}
}

func TestDropItemViaHandle(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setInv(p, 0, itemPlain, 2)
	store := newManagerStore()
	payload := inventoryUsePayload(0, itemPlain, "Drop")
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientInventoryUse, Payload: payload}, 7, Deps{GroundItems: store})
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("Handle = %+v, want acted", got)
	}
	if got := p.Inventory().ItemAt(0); got.ID() != -1 {
		t.Errorf("inventory slot 0 = %+v, want empty", got)
	}
	if len(store.Items()) != 1 {
		t.Fatalf("store has %d stacks, want 1", len(store.Items()))
	}
}

func TestDropItemViaHandleUndroppable(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setInv(p, 0, itemUndroppable, 1)
	store := newManagerStore()
	payload := inventoryUsePayload(0, itemUndroppable, "Drop")
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientInventoryUse, Payload: payload}, 7, Deps{GroundItems: store})
	if got != (Outcome{Recognized: true, Reason: ReasonDestroyInterface}) {
		t.Fatalf("Handle = %+v, want destroy-interface reason", got)
	}
	if got := p.Inventory().ItemAt(0); got.ID() != itemUndroppable {
		t.Errorf("inventory slot 0 changed: %+v", got)
	}
}

func TestSwitchItemSlotInventory(t *testing.T) {
	p := testPlayer()
	setInv(p, 0, itemBronzeSword, 1)
	setInv(p, 1, itemIronSword, 1)
	if !switchItemSlot(p, game.InventoryInterfaceID, 0, 1) {
		t.Fatal("switchItemSlot refused an inventory swap")
	}
	if got := p.Inventory().ItemAt(0); got.ID() != itemIronSword {
		t.Errorf("slot 0 = %+v, want iron sword", got)
	}
	if got := p.Inventory().ItemAt(1); got.ID() != itemBronzeSword {
		t.Errorf("slot 1 = %+v, want bronze sword", got)
	}
}

func TestSwitchItemSlotInventoryInterfaces(t *testing.T) {
	for _, iface := range []int{game.InventoryInterfaceID, game.BankInventoryInterfaceID, game.EquipmentInventoryInterfaceID} {
		p := testPlayer()
		setInv(p, 0, itemBronzeSword, 1)
		setInv(p, 1, itemIronSword, 1)
		if !switchItemSlot(p, iface, 0, 1) {
			t.Errorf("interface %d: switchItemSlot refused", iface)
		}
	}
}

func TestSwitchItemSlotBank(t *testing.T) {
	p := testPlayer()
	bank := p.Bank(0)
	bank.SetItem(0, game.NewItem(itemBronzeSword, 1))
	bank.SetItem(1, game.NewItem(itemIronSword, 1))
	bank.SetItem(2, game.NewItem(itemRing, 1))
	// Swap mode (insert mode false): slots 0 and 2 exchange.
	if !switchItemSlot(p, game.BankContainerStart, 0, 2) {
		t.Fatal("switchItemSlot refused a bank rearrange")
	}
	if got := bank.ItemAt(0); got.ID() != itemRing {
		t.Errorf("bank slot 0 = %+v, want ring", got)
	}
	if got := bank.ItemAt(1); got.ID() != itemIronSword {
		t.Errorf("bank slot 1 = %+v, want iron sword", got)
	}
	if got := bank.ItemAt(2); got.ID() != itemBronzeSword {
		t.Errorf("bank slot 2 = %+v, want bronze sword", got)
	}
}

func TestSwitchItemSlotBankInsertMode(t *testing.T) {
	// Insert mode follows the bank:insert-mode attribute
	// (Player.insertModeReturn, Player.ts:1452-1454): strictly
	// true bubbles the item toward the target, shifting the
	// items between; anything else swaps.
	setup := func() (*game.Player, *game.Bank) {
		p := testPlayer()
		bank := p.Bank(0)
		bank.SetItem(0, game.NewItem(itemBronzeSword, 1))
		bank.SetItem(1, game.NewItem(itemIronSword, 1))
		bank.SetItem(2, game.NewItem(itemRing, 1))
		return p, bank
	}
	ids := func(bank *game.Bank) [3]int {
		return [3]int{bank.ItemAt(0).ID(), bank.ItemAt(1).ID(), bank.ItemAt(2).ID()}
	}
	// Attribute true: 0 -> 2 bubbles (bronze shifts right twice).
	p, bank := setup()
	p.SetAttribute(game.BankInsertModeAttribute, true)
	if !switchItemSlot(p, game.BankContainerStart, 0, 2) {
		t.Fatal("switchItemSlot refused an insert-mode rearrange")
	}
	if got := ids(bank); got != [3]int{itemIronSword, itemRing, itemBronzeSword} {
		t.Errorf("insert mode slots = %v, want [iron ring bronze]", got)
	}
	// Attribute 1 (truthy but not === true): swaps.
	p, bank = setup()
	p.SetAttribute(game.BankInsertModeAttribute, 1)
	if !switchItemSlot(p, game.BankContainerStart, 0, 2) {
		t.Fatal("switchItemSlot refused a rearrange")
	}
	if got := ids(bank); got != [3]int{itemRing, itemIronSword, itemBronzeSword} {
		t.Errorf("attribute=1 slots = %v, want [ring iron bronze]", got)
	}
	// Attribute deleted: swaps (the unset default).
	p, bank = setup()
	p.SetAttribute(game.BankInsertModeAttribute, true)
	p.SetAttribute(game.BankInsertModeAttribute, nil)
	if !switchItemSlot(p, game.BankContainerStart, 0, 2) {
		t.Fatal("switchItemSlot refused a rearrange")
	}
	if got := ids(bank); got != [3]int{itemRing, itemIronSword, itemBronzeSword} {
		t.Errorf("attribute deleted slots = %v, want [ring iron bronze]", got)
	}
}

func TestSwitchItemSlotBankTabFromInterface(t *testing.T) {
	p := testPlayer()
	bank1 := p.Bank(1)
	bank1.SetItem(0, game.NewItem(itemBronzeSword, 1))
	bank1.SetItem(1, game.NewItem(itemIronSword, 1))
	if !switchItemSlot(p, game.BankContainerStart+1, 0, 1) {
		t.Fatal("switchItemSlot refused a tab-1 rearrange")
	}
	if got := bank1.ItemAt(0); got.ID() != itemIronSword {
		t.Errorf("bank tab 1 slot 0 = %+v, want iron sword", got)
	}
	// Tab 0 is untouched.
	if got := p.Bank(0).ItemAt(0); got.ID() != -1 {
		t.Errorf("bank tab 0 slot 0 = %+v, want empty", got)
	}
}

func TestSwitchItemSlotGates(t *testing.T) {
	cases := []struct {
		name            string
		setup           func(p *game.Player)
		iface, from, to int
	}{
		{"same slot", func(p *game.Player) { setInv(p, 0, itemBronzeSword, 1) }, game.InventoryInterfaceID, 0, 0},
		{"from below range", func(p *game.Player) {}, game.InventoryInterfaceID, -1, 1},
		{"to above range", func(p *game.Player) {}, game.InventoryInterfaceID, 0, 28},
		{"unknown interface", func(p *game.Player) { setInv(p, 0, itemBronzeSword, 1) }, 9999, 0, 1},
		{"bank range end", func(p *game.Player) {}, game.BankContainerStart + game.TotalBankTabs, 0, 1},
		{"dead player", func(p *game.Player) { setInv(p, 0, itemBronzeSword, 1); killPlayer(p) }, game.InventoryInterfaceID, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := testPlayer()
			tc.setup(p)
			if switchItemSlot(p, tc.iface, tc.from, tc.to) {
				t.Error("switchItemSlot acted despite the gate")
			}
		})
	}
}

func TestSwitchItemSlotViaHandle(t *testing.T) {
	p := testPlayer()
	setInv(p, 0, itemBronzeSword, 1)
	setInv(p, 1, itemIronSword, 1)
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientInventoryMove, Payload: inventoryMovePayload(0, 1)}, 7, Deps{})
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("Handle = %+v, want acted", got)
	}
	if got := p.Inventory().ItemAt(0); got.ID() != itemIronSword {
		t.Errorf("slot 0 = %+v, want iron sword", got)
	}
}

func TestIsDropOption(t *testing.T) {
	five, one := 5, 1
	cases := []struct {
		option string
		index  *int
		want   bool
	}{
		{"drop", nil, true},
		{"destroy", nil, true},
		{"discard", nil, true},
		{"release", nil, true},
		{"eat", nil, false},
		{"wield", nil, false},
		{"", &five, true},
		{"", &one, false},
		{"", nil, false},
	}
	for _, tc := range cases {
		if got := isDropOption(tc.option, tc.index); got != tc.want {
			t.Errorf("isDropOption(%q, %v) = %v, want %v", tc.option, tc.index, got, tc.want)
		}
	}
}

func TestWieldOptionExactMatch(t *testing.T) {
	for _, verb := range []string{"wield", "wear", "equip", "hold"} {
		if !wieldOption(verb) {
			t.Errorf("wieldOption(%q) = false, want true", verb)
		}
	}
	for _, verb := range []string{"wielding", "wielded", "wearable", "equipment", "holden", "eat", ""} {
		if wieldOption(verb) {
			t.Errorf("wieldOption(%q) = true, want false (anchored regex)", verb)
		}
	}
}

func TestResolveInventoryWidgetAction(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	cases := []struct {
		name      string
		itemID    int
		widgetOp  int
		wantIndex int
		wantOpt   string
		wantOK    bool
	}{
		{"op 10 examine", itemBronzeSword, 10, 10, "Examine", true},
		{"op 2 first action", itemBronzeSword, 2, 1, "Wield", true},
		{"op 3 empty", itemBronzeSword, 3, 0, "", false},
		{"op 4 empty", itemBronzeSword, 4, 0, "", false},
		{"op 5 use never resolves", itemBronzeSword, 5, 0, "", false},
		{"op 6 empty", itemBronzeSword, 6, 0, "", false},
		{"op 7 drop", itemBronzeSword, 7, 5, "Drop", true},
		{"op 1 shift-click drop", itemBronzeSword, 1, 5, "Drop", true},
		{"op 0 no action", itemBronzeSword, 0, 0, "", false},
		{"op 8 no action", itemBronzeSword, 8, 0, "", false},
		{"op 9 no action", itemBronzeSword, 9, 0, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			index, option, ok := resolveInventoryWidgetAction(p, tc.itemID, tc.widgetOp)
			if ok != tc.wantOK || index != tc.wantIndex || option != tc.wantOpt {
				t.Errorf("resolve(%d, op %d) = (%d, %q, %v), want (%d, %q, %v)",
					tc.itemID, tc.widgetOp, index, option, ok, tc.wantIndex, tc.wantOpt, tc.wantOK)
			}
		})
	}
}

func TestResolveInventoryWidgetActionDefaults(t *testing.T) {
	// No definition source at all: only op 10 (Examine) and the
	// default Drop option resolve.
	p := testPlayer()
	if index, option, ok := resolveInventoryWidgetAction(p, itemPlain, 10); !ok || index != 10 || option != "Examine" {
		t.Errorf("op 10 = (%d, %q, %v), want (10, Examine, true)", index, option, ok)
	}
	if _, _, ok := resolveInventoryWidgetAction(p, itemPlain, 2); ok {
		t.Error("op 2 resolved without a definition source")
	}
	if index, option, ok := resolveInventoryWidgetAction(p, itemPlain, 7); !ok || index != 5 || option != "Drop" {
		t.Errorf("op 7 = (%d, %q, %v), want (5, Drop, true)", index, option, ok)
	}
	// A definition source without ItemActionSource behaves the same.
	p.SetItemDefinitions(noActionDefs{fixtureDefs().defTable})
	if _, _, ok := resolveInventoryWidgetAction(p, itemBronzeSword, 2); ok {
		t.Error("op 2 resolved without an action source")
	}
}

func TestHandleFirstActionEquipFallback(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setInv(p, 0, itemBronzeSword, 1)
	got := handleFirstAction(p, game.InventoryInterfaceID, itemBronzeSword, 0, "", ItemActionHooks{})
	if got != itemActionHandled {
		t.Fatalf("handleFirstAction = %v, want handled", got)
	}
	if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() != itemBronzeSword {
		t.Errorf("weapon slot = %+v, want bronze sword", got)
	}
}

func TestHandleFirstActionNonWieldable(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setInv(p, 0, itemPlain, 1)
	got := handleFirstAction(p, game.InventoryInterfaceID, itemPlain, 0, "", ItemActionHooks{})
	if got != itemActionPlugin {
		t.Fatalf("handleFirstAction = %v, want plugin", got)
	}
	if got := p.Inventory().ItemAt(0); got.ID() != itemPlain {
		t.Errorf("inventory slot 0 changed: %+v", got)
	}
}

func TestHandleFirstActionNoFallbackOffInventory(t *testing.T) {
	// The equip fallback requires interface 3214: the same click
	// arriving on the equipment interface (1688) reads the
	// equipment container and never equips.
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setEq(p, game.WeaponSlot, itemBronzeSword, 1)
	got := handleFirstAction(p, game.EquipmentInventoryInterfaceID, itemBronzeSword, game.WeaponSlot, "", ItemActionHooks{})
	if got != itemActionPlugin {
		t.Fatalf("handleFirstAction = %v, want plugin", got)
	}
}

func TestHandleActionCancelsWalkTo(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	p.SetRunning(false)
	setInv(p, 0, itemPlain, 1)
	// Start a walk-to interaction, then act on an inventory item:
	// the interaction cancels but the walk queue keeps its points.
	p.MovementQueue().WalkToGroundItem(game.Location{X: 3095, Y: 3524, Z: 0}, func() {}, 0)
	if !p.MovementQueue().WalkToPending() {
		t.Fatal("walk-to did not start")
	}
	points := p.MovementQueue().Size()
	if points == 0 {
		t.Fatal("walk-to routed no checkpoints")
	}
	handleAction(p, game.InventoryInterfaceID, itemPlain, 0, 2, "Eat", ItemActionHooks{})
	if p.MovementQueue().WalkToPending() {
		t.Error("handleAction did not cancel the pending walk-to")
	}
	if p.MovementQueue().Size() != points {
		t.Errorf("queue size = %d, want %d (walk continues)", p.MovementQueue().Size(), points)
	}
}

func TestInventoryActionRoutingViaHandle(t *testing.T) {
	cases := []struct {
		name     string
		option   string
		itemID   int
		want     Outcome
		equipped bool
	}{
		{"wield equips", "Wield", itemBronzeSword, Outcome{Recognized: true, Implemented: true}, true},
		{"wear equips", "Wear", itemBronzeShield, Outcome{Recognized: true, Implemented: true}, true},
		{"wielding is not a wield verb", "Wielding", itemBronzeSword, Outcome{Recognized: true, Reason: ReasonNoAction}, false},
		{"unknown verb returns silently", "Eat", itemBronzeSword, Outcome{Recognized: true, Reason: ReasonNoAction}, false},
		{"examine needs the definition source", "Examine", itemBronzeSword, Outcome{Recognized: true, Reason: ReasonItemExamine}, false},
		{"empty option non-wieldable is a plugin action", "", itemPlain, Outcome{Recognized: true, Reason: ReasonItemAction}, false},
		{"empty option wieldable equips", "", itemBronzeSword, Outcome{Recognized: true, Implemented: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := testPlayer()
			p.SetItemDefinitions(fixtureDefs())
			setInv(p, 0, tc.itemID, 1)
			store := newManagerStore()
			got := Handle(p, game.TurnPacket{Opcode: protocol.ClientInventoryUse, Payload: inventoryUsePayload(0, tc.itemID, tc.option)}, 7, Deps{GroundItems: store})
			if got != tc.want {
				t.Errorf("Handle = %+v, want %+v", got, tc.want)
			}
			equipped := p.Equipment().ItemAt(game.WeaponSlot).ID() == tc.itemID ||
				p.Equipment().ItemAt(game.ShieldSlot).ID() == tc.itemID
			if equipped != tc.equipped {
				t.Errorf("equipped = %v, want %v", equipped, tc.equipped)
			}
		})
	}
}

func TestInventoryActionRoutingNoActionSource(t *testing.T) {
	// Without ItemActionSource, wield routing still works from the
	// option text + equipment slot, and Drop still routes from the
	// default actions.
	p := testPlayer()
	p.SetItemDefinitions(noActionDefs{fixtureDefs().defTable})
	setInv(p, 0, itemBronzeSword, 1)
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientInventoryUse, Payload: inventoryUsePayload(0, itemBronzeSword, "Wield")}, 7, Deps{})
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Errorf("wield without action source: got %+v", got)
	}
	p2 := testPlayer()
	p2.SetItemDefinitions(noActionDefs{fixtureDefs().defTable})
	setInv(p2, 0, itemPlain, 1)
	store := newManagerStore()
	got = Handle(p2, game.TurnPacket{Opcode: protocol.ClientInventoryUse, Payload: inventoryUsePayload(0, itemPlain, "Drop")}, 7, Deps{GroundItems: store})
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Errorf("drop without action source: got %+v", got)
	}
	if len(store.Items()) != 1 {
		t.Errorf("store has %d stacks, want 1", len(store.Items()))
	}
}

func TestTakeOperation(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	if got := takeOperation(p, itemPlain); got != 3 {
		t.Errorf("takeOperation(default) = %d, want 3", got)
	}
	if got := takeOperation(p, itemSearchable); got != 3 {
		t.Errorf("takeOperation(searchable) = %d, want 3", got)
	}
	// Custom actions with Take first.
	defs := fixtureDefs()
	defs.cfgs[itemPlain] = defConfig{slot: -1, dropable: true, tradeable: true, groundActions: []string{"Take", "", "", "", ""}}
	p.SetItemDefinitions(defs)
	if got := takeOperation(p, itemPlain); got != 1 {
		t.Errorf("takeOperation(take first) = %d, want 1", got)
	}
	// No Take anywhere, third action set: no take operation.
	defs.cfgs[itemPlain] = defConfig{slot: -1, dropable: true, tradeable: true, groundActions: []string{"", "Search", "Examine", "", ""}}
	if got := takeOperation(p, itemPlain); got != 0 {
		t.Errorf("takeOperation(no take) = %d, want 0", got)
	}
	// No Take anywhere, third action unset: operation 3.
	defs.cfgs[itemPlain] = defConfig{slot: -1, dropable: true, tradeable: true, groundActions: []string{"", "Search", "", "", ""}}
	if got := takeOperation(p, itemPlain); got != 3 {
		t.Errorf("takeOperation(third unset) = %d, want 3", got)
	}
}

func TestGetOperableGroundItemPreference(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	loc := p.Location()
	store := newTradeableStore()
	// Another player's public stack, registered first.
	other := game.NewItemOnGround(game.GroundItemSeenByEveryone, "someone_else", loc, itemPlain, 1, nil, false, -1, false, nil)
	store.Register(other)
	// An unowned public stack.
	unowned := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemPlain, 1, nil, false, -1, false, nil)
	store.Register(unowned)
	// The player's own stack, registered last: preferred anyway.
	own := game.NewItemOnGround(game.GroundItemSeenByPlayer, p.Username, loc, itemPlain, 1, nil, true, -1, false, nil)
	store.Register(own)
	got := getOperableGroundItem(store, p, itemPlain, loc, 3, 0, false)
	if got != own {
		t.Errorf("operable = %v, want the player's own stack", got)
	}
	// Without the own stack, the unowned stack wins over the
	// other player's.
	store2 := newTradeableStore()
	store2.Register(other)
	store2.Register(unowned)
	if got := getOperableGroundItem(store2, p, itemPlain, loc, 3, 0, false); got != unowned {
		t.Errorf("operable = %v, want the unowned stack", got)
	}
	// With only the other player's stack, it is operable (public
	// and tradeable).
	store3 := newTradeableStore()
	store3.Register(other)
	if got := getOperableGroundItem(store3, p, itemPlain, loc, 3, 0, false); got != other {
		t.Errorf("operable = %v, want the other player's stack", got)
	}
}

func TestGetOperableGroundItemStackID(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	loc := p.Location()
	store := newManagerStore()
	first := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemPlain, 1, nil, false, -1, false, nil)
	second := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemPlain, 1, nil, false, -1, false, nil)
	store.Register(first)
	store.Register(second)
	if first.ID() == second.ID() {
		t.Fatalf("fixture stacks share id %d", first.ID())
	}
	if got := getOperableGroundItem(store, p, itemPlain, loc, 3, second.ID(), true); got != second {
		t.Errorf("stack-id lookup = %v, want the second stack", got)
	}
	if got := getOperableGroundItem(store, p, itemPlain, loc, 3, 9999, true); got != nil {
		t.Errorf("unknown stack id returned %v", got)
	}
	// A pending-removal stack is skipped by the id lookup.
	store.Deregister(second)
	if got := getOperableGroundItem(store, p, itemPlain, loc, 3, second.ID(), true); got != nil {
		t.Errorf("pending-removal stack returned by id lookup: %v", got)
	}
}

func TestGetOperableGroundItemExclusions(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	loc := p.Location()
	// Wrong tile.
	store := newManagerStore()
	away := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", game.Location{X: loc.X + 5, Y: loc.Y, Z: 0}, itemPlain, 1, nil, false, -1, false, nil)
	store.Register(away)
	if got := getOperableGroundItem(store, p, itemPlain, loc, 3, 0, false); got != nil {
		t.Errorf("wrong-tile stack returned: %v", got)
	}
	// Wrong id.
	store2 := newManagerStore()
	otherID := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemBronzeSword, 1, nil, false, -1, false, nil)
	store2.Register(otherID)
	if got := getOperableGroundItem(store2, p, itemPlain, loc, 3, 0, false); got != nil {
		t.Errorf("wrong-id stack returned: %v", got)
	}
	// Take operation disabled.
	store3 := newManagerStore()
	disabled := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemPlain, 1, nil, false, -1, false, nil)
	store3.Register(disabled)
	if err := disabled.SetEnabledOps(0x1f&^(1<<2), nil); err != nil {
		t.Fatalf("SetEnabledOps: %v", err)
	}
	if got := getOperableGroundItem(store3, p, itemPlain, loc, 3, 0, false); got != nil {
		t.Errorf("operation-disabled stack returned: %v", got)
	}
	// Another player's private stack is invisible.
	store4 := newManagerStore()
	private := game.NewItemOnGround(game.GroundItemSeenByPlayer, "someone_else", loc, itemPlain, 1, nil, true, -1, false, nil)
	store4.Register(private)
	if got := getOperableGroundItem(store4, p, itemPlain, loc, 3, 0, false); got != nil {
		t.Errorf("another player's private stack returned: %v", got)
	}
}

func TestPickupWalkToArrival(t *testing.T) {
	start := game.Location{X: 3100, Y: 3500, Z: 0}
	p := game.NewPlayer("testplayer", start)
	p.SetItemDefinitions(fixtureDefs())
	p.SetRunning(false)
	stackLoc := game.Location{X: 3103, Y: 3500, Z: 0}
	store := newManagerStore()
	stack := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", stackLoc, itemPlain, 1, nil, false, -1, false, nil)
	store.Register(stack)
	if !pickupItem(p, itemPlain, stackLoc.X, stackLoc.Y, 0, false, 0, store) {
		t.Fatal("pickupItem refused an operable stack")
	}
	if !p.MovementQueue().WalkToPending() {
		t.Fatal("pickup did not start a walk-to")
	}
	// Three walking ticks reach the tile; the arrival callback
	// fires on the cycle AFTER arrival, so after tick 3 the
	// item is still on the floor and the walk-to is pending.
	for cycle := int64(1); cycle <= 3; cycle++ {
		tickPlayerQueue(p, cycle)
	}
	if p.Location() != stackLoc {
		t.Fatalf("player at %+v, want %+v", p.Location(), stackLoc)
	}
	if p.Inventory().GetAmount(itemPlain) != 0 {
		t.Error("item picked up on the arrival cycle; TS resolves on the next cycle")
	}
	if !p.MovementQueue().WalkToPending() {
		t.Error("walk-to cleared on the arrival cycle; TS resolves on the next cycle")
	}
	tickPlayerQueue(p, 4)
	if p.Inventory().GetAmount(itemPlain) != 1 {
		t.Error("item not picked up on the cycle after arrival")
	}
	if !stack.PendingRemoval {
		t.Error("stack not deregistered after a full take")
	}
	if p.MovementQueue().WalkToPending() {
		t.Error("walk-to still pending after the pickup")
	}
}

func TestPickupImmediateWhenStandingOnTile(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	loc := p.Location()
	store := newManagerStore()
	stack := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemPlain, 2, nil, false, -1, false, nil)
	store.Register(stack)
	// Standing exactly on the tile: WalkToTile runs the action
	// immediately, before any walk-to state exists.
	if !pickupItem(p, itemPlain, loc.X, loc.Y, 0, false, 0, store) {
		t.Fatal("pickupItem refused a stack under the player")
	}
	if p.Inventory().GetAmount(itemPlain) != 2 {
		t.Error("standing-on-tile pickup did not transfer immediately")
	}
	if p.MovementQueue().WalkToPending() {
		t.Error("immediate pickup left a walk-to pending")
	}
}

func TestPickupAdjacentWithoutStep(t *testing.T) {
	// The didMoveThisCycle gate: adjacent counts only when the
	// cycle produced no step. Block movement after the walk-to
	// starts: the player never steps, stays adjacent, and the
	// pickup still resolves.
	p := game.NewPlayer("testplayer", game.Location{X: 3100, Y: 3500, Z: 0})
	p.SetItemDefinitions(fixtureDefs())
	p.SetRunning(false)
	stackLoc := game.Location{X: 3101, Y: 3500, Z: 0}
	store := newManagerStore()
	stack := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", stackLoc, itemPlain, 1, nil, false, -1, false, nil)
	store.Register(stack)
	if !pickupItem(p, itemPlain, stackLoc.X, stackLoc.Y, 0, false, 0, store) {
		t.Fatal("pickupItem refused an adjacent stack")
	}
	p.MovementQueue().SetBlockMovement(true)
	tickPlayerQueue(p, 1)
	tickPlayerQueue(p, 2)
	if p.Location().X != 3100 {
		t.Fatalf("blocked player moved to %+v", p.Location())
	}
	if p.Inventory().GetAmount(itemPlain) != 1 {
		t.Error("adjacent-without-step pickup did not resolve")
	}
	if p.MovementQueue().WalkToPending() {
		t.Error("walk-to still pending after the pickup")
	}
}

func TestReachedGroundItemRules(t *testing.T) {
	p := game.NewPlayer("testplayer", game.Location{X: 3100, Y: 3500, Z: 0})
	p.SetRunning(false)
	q := p.MovementQueue()
	// Exact tile always counts.
	q.BeginCycle()
	if !q.ReachedGroundItem(game.Location{X: 3100, Y: 3500, Z: 0}) {
		t.Error("standing on the tile does not count")
	}
	// Orthogonal adjacency without a step counts.
	if !q.ReachedGroundItem(game.Location{X: 3101, Y: 3500, Z: 0}) {
		t.Error("orthogonal adjacency without a step does not count")
	}
	// Diagonal adjacency never counts (reachRectangle1).
	if q.ReachedGroundItem(game.Location{X: 3101, Y: 3501, Z: 0}) {
		t.Error("diagonal adjacency counts; TS reachRectangle1 is orthogonal-only")
	}
	// Adjacency after a step does not count.
	q.RequestWalk(game.Location{X: 3102, Y: 3500, Z: 0}, nil)
	tickPlayerQueue(p, 1)
	if p.Location().X != 3101 || !q.DidMoveThisCycle() {
		t.Fatalf("setup step failed: at %+v, didMove=%v", p.Location(), q.DidMoveThisCycle())
	}
	if q.ReachedGroundItem(game.Location{X: 3102, Y: 3500, Z: 0}) {
		t.Error("adjacency after a step counts; the didMoveThisCycle gate must refuse it")
	}
	// But the exact tile still counts after a step.
	if !q.ReachedGroundItem(p.Location()) {
		t.Error("exact tile after a step does not count")
	}
}

func TestWalkToUnreachableFails(t *testing.T) {
	// Every tile fully walled: the route fails, and the walk-to
	// ends without running its action.
	p := game.NewPlayer("testplayer", game.Location{X: 3100, Y: 3500, Z: 0})
	blocked := func(x, y, level int) int32 { return -1 }
	q := game.NewMovementQueue(p, nil, blocked)
	called := false
	q.WalkToGroundItem(game.Location{X: 3130, Y: 3500, Z: 0}, func() { called = true }, 0)
	if q.Size() != 0 {
		t.Fatalf("unreachable route queued %d checkpoints", q.Size())
	}
	q.BeginCycle()
	if q.HasPendingWork() {
		q.Process(1)
	}
	if q.WalkToPending() {
		t.Error("unreachable walk-to still pending after its tick")
	}
	if called {
		t.Error("unreachable walk-to ran its action")
	}
}

func TestWalkToTileImmediateAndMobility(t *testing.T) {
	p := testPlayer()
	q := p.MovementQueue()
	// Standing on the tile: the action runs synchronously.
	called := false
	q.WalkToTile(p.Location(), func() { called = true }, nil, false, 0)
	if !called {
		t.Error("exact-tile walk-to did not run immediately")
	}
	if q.WalkToPending() {
		t.Error("immediate walk-to left state pending")
	}
	// Blocked movement: the queue resets, no walk-to starts, the
	// action never runs.
	q.SetBlockMovement(true)
	called = false
	q.WalkToTile(game.Location{X: 3095, Y: 3524, Z: 0}, func() { called = true }, nil, false, 0)
	if q.WalkToPending() {
		t.Error("mobility-refused walk-to left state pending")
	}
	q.BeginCycle()
	if q.HasPendingWork() {
		q.Process(1)
	}
	if called {
		t.Error("mobility-refused walk-to ran its action")
	}
}

func TestRequestWalkCancelsWalkTo(t *testing.T) {
	p := testPlayer()
	p.SetRunning(false)
	q := p.MovementQueue()
	called := false
	q.WalkToGroundItem(game.Location{X: 3095, Y: 3524, Z: 0}, func() { called = true }, 0)
	if !q.WalkToPending() {
		t.Fatal("walk-to did not start")
	}
	q.RequestWalk(game.Location{X: 3080, Y: 3524, Z: 0}, nil)
	if q.WalkToPending() {
		t.Error("walk click did not cancel the pending walk-to")
	}
	if q.Size() == 0 {
		t.Error("walk click queued no checkpoints")
	}
	for cycle := int64(1); cycle <= 12; cycle++ {
		tickPlayerQueue(p, cycle)
	}
	if called {
		t.Error("cancelled walk-to ran its action")
	}
}

func TestPickupInventoryFull(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	for slot := 0; slot < p.Inventory().Capacity(); slot++ {
		setInv(p, slot, itemPlain, 1)
	}
	loc := p.Location()
	store := newManagerStore()
	stack := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemBronzeSword, 1, nil, false, -1, false, nil)
	store.Register(stack)
	// Immediate path (standing on the tile): the transfer credits
	// nothing, the stack stays.
	if !pickupItem(p, itemBronzeSword, loc.X, loc.Y, 0, false, 0, store) {
		t.Fatal("pickupItem refused a full-inventory take at the gate")
	}
	if stack.PendingRemoval {
		t.Error("full-inventory take deregistered the stack")
	}
	if stack.Amount != 1 {
		t.Errorf("stack amount = %d, want 1", stack.Amount)
	}
}

func TestPickupPartial(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	// One free slot; the stack holds 5 non-stackable swords.
	for slot := 1; slot < p.Inventory().Capacity(); slot++ {
		setInv(p, slot, itemPlain, 1)
	}
	loc := p.Location()
	store := newManagerStore()
	stack := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemBronzeSword, 5, nil, false, -1, false, nil)
	store.Register(stack)
	if !pickupItem(p, itemBronzeSword, loc.X, loc.Y, 0, false, 0, store) {
		t.Fatal("pickupItem refused a partial take")
	}
	if p.Inventory().GetAmount(itemBronzeSword) != 1 {
		t.Errorf("inventory holds %d swords, want 1", p.Inventory().GetAmount(itemBronzeSword))
	}
	if stack.PendingRemoval {
		t.Error("partial take deregistered the stack")
	}
	if stack.Amount != 4 || stack.OldAmount != 5 {
		t.Errorf("stack amount/old = %d/%d, want 4/5", stack.Amount, stack.OldAmount)
	}
	alters := 0
	for _, perf := range store.performed {
		if perf.stack == stack && perf.op == game.GroundItemAlter {
			alters++
		}
	}
	if alters != 1 {
		t.Errorf("ALTER performs = %d, want 1", alters)
	}
}

func TestPickupReplacementAndMeta(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	loc := p.Location()
	store := newManagerStore()
	meta := map[string]any{"charges": json.Number("3")}
	stack := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemPlain, 2, meta, false, -1, false,
		&game.GroundPickupReplacement{ID: itemReplacement})
	store.Register(stack)
	if !pickupItem(p, itemPlain, loc.X, loc.Y, 0, false, 0, store) {
		t.Fatal("pickupItem refused a replacement take")
	}
	if p.Inventory().GetAmount(itemReplacement) != 2 {
		t.Errorf("inventory holds %d replacement items, want 2", p.Inventory().GetAmount(itemReplacement))
	}
	if p.Inventory().GetAmount(itemPlain) != 0 {
		t.Error("stack id credited instead of the replacement id")
	}
	held := p.Inventory().ItemAt(0)
	if !metaMatchesGround(held.Meta(), meta) {
		t.Errorf("credited meta = %v, want %v", held.Meta(), meta)
	}
}

func TestPickupDeadPlayerRefused(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	killPlayer(p)
	loc := p.Location()
	store := newManagerStore()
	stack := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemPlain, 1, nil, false, -1, false, nil)
	store.Register(stack)
	if pickupItem(p, itemPlain, loc.X, loc.Y, 0, false, 0, store) {
		t.Error("dead player picked up an item")
	}
	if p.Inventory().GetAmount(itemPlain) != 0 || stack.PendingRemoval {
		t.Error("dead-player pickup changed state")
	}
}

func TestPickupThrottleSameCycle(t *testing.T) {
	// The 300ms throttle (PickupItemPacketListener.ts:18), ported
	// as a cycle stamp: two immediate takes on the same cycle —
	// the second is refused, and the same take on the next cycle
	// is allowed. Ticks are 600ms and transfers happen only on
	// ticks, so same-cycle is exactly "under 300ms".
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	loc := p.Location()
	store := newManagerStore()
	first := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemPlain, 1, nil, false, -1, false, nil)
	second := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemBronzeSword, 1, nil, false, -1, false, nil)
	store.Register(first)
	store.Register(second)
	if !pickupItem(p, itemPlain, loc.X, loc.Y, 0, false, 0, store) {
		t.Fatal("first immediate pickup refused")
	}
	if pickupItem(p, itemBronzeSword, loc.X, loc.Y, 0, false, 0, store) {
		t.Fatal("second same-cycle pickup allowed (throttle must refuse it)")
	}
	if p.Inventory().GetAmount(itemPlain) != 1 || p.Inventory().GetAmount(itemBronzeSword) != 0 {
		t.Error("throttled pickup transferred the second item")
	}
	// Next cycle: the throttle has expired.
	if !pickupItem(p, itemBronzeSword, loc.X, loc.Y, 0, false, 1, store) {
		t.Fatal("next-cycle pickup refused")
	}
	if p.Inventory().GetAmount(itemBronzeSword) != 1 {
		t.Error("next-cycle pickup did not transfer")
	}
}

func TestDropThenPickupRoundTrip(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setInv(p, 0, itemPlain, 4)
	store := newManagerStore()
	dropPayload := inventoryUsePayload(0, itemPlain, "Drop")
	if got := Handle(p, game.TurnPacket{Opcode: protocol.ClientInventoryUse, Payload: dropPayload}, 7, Deps{GroundItems: store}); got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("drop Handle = %+v", got)
	}
	if len(store.Items()) != 1 {
		t.Fatalf("store has %d stacks after the drop", len(store.Items()))
	}
	loc := p.Location()
	// The high packet always carries a stack id (the decoder sets
	// HasStackID unconditionally), so the take names the dropped
	// stack's real world id — the stack-id lookup path.
	takePayload := groundHighPayload(int32(store.Items()[0].ID()), loc.X, loc.Y, itemPlain, "take", 3)
	if got := Handle(p, game.TurnPacket{Opcode: protocol.ClientGroundItemAction, Payload: takePayload}, 8, Deps{GroundItems: store}); got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("take Handle = %+v", got)
	}
	if p.Inventory().GetAmount(itemPlain) != 4 {
		t.Errorf("inventory holds %d plain items after the round trip, want 4", p.Inventory().GetAmount(itemPlain))
	}
	if !store.Items()[0].PendingRemoval {
		t.Error("stack not deregistered after the round-trip take")
	}
}

func TestGroundItemRouting(t *testing.T) {
	newRoutedPlayer := func() (*game.Player, *managerStore) {
		p := testPlayer()
		p.SetItemDefinitions(fixtureDefs())
		return p, newManagerStore()
	}
	route := func(p *game.Player, msg protocol.GroundItemAction, store GroundItems) Outcome {
		drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
		acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
		noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
		return routeGroundItemAction(p, msg, 0, Deps{GroundItems: store}, drop, acted, noAction)
	}
	loc := testPlayer().Location()

	t.Run("take option picks up", func(t *testing.T) {
		p, store := newRoutedPlayer()
		stack := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemPlain, 1, nil, false, -1, false, nil)
		store.Register(stack)
		msg := protocol.GroundItemAction{ItemID: itemPlain, X: loc.X, Y: loc.Y, Option: "Take", HasOption: true, OptionIndex: 1, HasOptionIndex: true}
		if got := route(p, msg, store); got != (Outcome{Recognized: true, Implemented: true}) {
			t.Errorf("take route = %+v", got)
		}
	})
	t.Run("examine needs the definition source", func(t *testing.T) {
		p, store := newRoutedPlayer()
		msg := protocol.GroundItemAction{ItemID: itemPlain, X: loc.X, Y: loc.Y, Option: "Examine", HasOption: true}
		if got := route(p, msg, store); got != (Outcome{Recognized: true, Reason: ReasonGroundItemExamine}) {
			t.Errorf("examine route = %+v", got)
		}
	})
	t.Run("named option matching cache routes to second option", func(t *testing.T) {
		p, store := newRoutedPlayer()
		msg := protocol.GroundItemAction{ItemID: itemSearchable, X: loc.X, Y: loc.Y, Option: "Search", HasOption: true, OptionIndex: 2, HasOptionIndex: true}
		if got := route(p, msg, store); got != (Outcome{Recognized: true, Reason: ReasonSecondGroundItemOption}) {
			t.Errorf("search route = %+v", got)
		}
	})
	t.Run("named option disagreeing index returns silently", func(t *testing.T) {
		p, store := newRoutedPlayer()
		msg := protocol.GroundItemAction{ItemID: itemSearchable, X: loc.X, Y: loc.Y, Option: "Search", HasOption: true, OptionIndex: 4, HasOptionIndex: true}
		if got := route(p, msg, store); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Errorf("disagreeing index route = %+v", got)
		}
	})
	t.Run("unknown named option returns silently", func(t *testing.T) {
		p, store := newRoutedPlayer()
		msg := protocol.GroundItemAction{ItemID: itemPlain, X: loc.X, Y: loc.Y, Option: "Smell", HasOption: true}
		if got := route(p, msg, store); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Errorf("unknown option route = %+v", got)
		}
	})
	t.Run("unnamed index 3 with no action picks up", func(t *testing.T) {
		p, store := newRoutedPlayer()
		stack := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemPlain, 1, nil, false, -1, false, nil)
		store.Register(stack)
		msg := protocol.GroundItemAction{ItemID: itemPlain, X: loc.X, Y: loc.Y, OptionIndex: 3, HasOptionIndex: true}
		if got := route(p, msg, store); got != (Outcome{Recognized: true, Implemented: true}) {
			t.Errorf("unnamed index 3 route = %+v", got)
		}
	})
	t.Run("unnamed index 2 with named action routes to second option", func(t *testing.T) {
		p, store := newRoutedPlayer()
		msg := protocol.GroundItemAction{ItemID: itemSearchable, X: loc.X, Y: loc.Y, OptionIndex: 2, HasOptionIndex: true}
		if got := route(p, msg, store); got != (Outcome{Recognized: true, Reason: ReasonSecondGroundItemOption}) {
			t.Errorf("unnamed index 2 route = %+v", got)
		}
	})
	t.Run("unnamed index 1 with no action returns silently", func(t *testing.T) {
		p, store := newRoutedPlayer()
		msg := protocol.GroundItemAction{ItemID: itemPlain, X: loc.X, Y: loc.Y, OptionIndex: 1, HasOptionIndex: true}
		if got := route(p, msg, store); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Errorf("unnamed index 1 route = %+v", got)
		}
	})
	t.Run("unwired store", func(t *testing.T) {
		p, _ := newRoutedPlayer()
		msg := protocol.GroundItemAction{ItemID: itemPlain, X: loc.X, Y: loc.Y, Option: "take", HasOption: true}
		if got := route(p, msg, nil); got != (Outcome{Recognized: true, Reason: ReasonGroundItemsUnwired}) {
			t.Errorf("unwired route = %+v", got)
		}
	})
	t.Run("nil player", func(t *testing.T) {
		_, store := newRoutedPlayer()
		msg := protocol.GroundItemAction{ItemID: itemPlain, X: loc.X, Y: loc.Y, Option: "take", HasOption: true}
		if got := route(nil, msg, store); got != (Outcome{Recognized: true, Reason: ReasonNoPlayer}) {
			t.Errorf("nil-player route = %+v", got)
		}
	})
}

func TestGroundItemNativeOpcodeViaHandle(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	loc := p.Location()
	store := newManagerStore()
	stack := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemPlain, 1, nil, false, -1, false, nil)
	store.Register(stack)
	payload := opObj3Payload(itemPlain, loc.X, loc.Y, 0)
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpObj3, Payload: payload}, 7, Deps{GroundItems: store})
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("native OPOBJ3 Handle = %+v", got)
	}
	if p.Inventory().GetAmount(itemPlain) != 1 {
		t.Error("native take did not transfer the standing-on-tile stack")
	}
}

func TestClickRunOverrideAppliesToPickupWalk(t *testing.T) {
	// A Ctrl-click on a ground item inverts the run mode for the
	// walk to it: over two ticks the Ctrl-clicked player covers
	// strictly more ground than the plain-clicked one.
	walk := func(mods int) int {
		p := game.NewPlayer("testplayer", game.Location{X: 3100, Y: 3500, Z: 0})
		p.SetItemDefinitions(fixtureDefs())
		p.SetRunning(false)
		stackLoc := game.Location{X: 3104, Y: 3500, Z: 0}
		store := newManagerStore()
		stack := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", stackLoc, itemPlain, 1, nil, false, -1, false, nil)
		store.Register(stack)
		msg := protocol.GroundItemAction{ItemID: itemPlain, X: stackLoc.X, Y: stackLoc.Y, OptionIndex: 3, HasOptionIndex: true, ModifierFlags: mods}
		drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
		acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
		noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
		if got := routeGroundItemAction(p, msg, 0, Deps{GroundItems: store}, drop, acted, noAction); got != (Outcome{Recognized: true, Implemented: true}) {
			t.Fatalf("route with mods %d = %+v", mods, got)
		}
		// Drive through Player.Process so the pending override
		// activates at the end of the turn, as in production.
		p.Process(1)
		p.Process(2)
		return p.Location().X
	}
	plain := walk(0)
	ctrl := walk(1)
	if ctrl <= plain {
		t.Errorf("Ctrl-click reached x=%d, plain click x=%d: override did not invert the run mode", ctrl, plain)
	}
}

func TestWidgetEquipmentBranch(t *testing.T) {
	route := func(p *game.Player, msg protocol.WidgetAction) Outcome {
		drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
		acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
		noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
		return routeWidgetAction(p, msg, Deps{}, drop, acted, noAction)
	}
	// Group 387, first child 15: weapon is child index 3.
	widgetID := int32(387<<16 | 18)
	t.Run("op1 unequips", func(t *testing.T) {
		p := testPlayer()
		p.SetItemDefinitions(fixtureDefs())
		setEq(p, game.WeaponSlot, itemBronzeSword, 1)
		msg := protocol.WidgetAction{WidgetID: widgetID, GroupID: 387, ChildID: 18, ButtonNum: 1, HasButtonNum: true}
		if got := route(p, msg); got != (Outcome{Recognized: true, Implemented: true}) {
			t.Fatalf("route = %+v", got)
		}
		if p.Inventory().GetAmount(itemBronzeSword) != 1 {
			t.Error("sword did not return to the inventory")
		}
	})
	t.Run("op1 on empty slot does nothing", func(t *testing.T) {
		p := testPlayer()
		p.SetItemDefinitions(fixtureDefs())
		msg := protocol.WidgetAction{WidgetID: widgetID, GroupID: 387, ChildID: 18, ButtonNum: 1, HasButtonNum: true}
		if got := route(p, msg); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Errorf("route = %+v", got)
		}
	})
	t.Run("op10 examines", func(t *testing.T) {
		p := testPlayer()
		p.SetItemDefinitions(fixtureDefs())
		setEq(p, game.WeaponSlot, itemBronzeSword, 1)
		msg := protocol.WidgetAction{WidgetID: widgetID, GroupID: 387, ChildID: 18, ButtonNum: 10, HasButtonNum: true}
		if got := route(p, msg); got != (Outcome{Recognized: true, Reason: ReasonItemExamine}) {
			t.Errorf("route = %+v", got)
		}
	})
	t.Run("op2 on worn item sends nothing-interesting", func(t *testing.T) {
		p, sess := messageTestPlayer()
		setEq(p, game.WeaponSlot, itemBronzeSword, 1)
		msg := protocol.WidgetAction{WidgetID: widgetID, GroupID: 387, ChildID: 18, ButtonNum: 2, HasButtonNum: true}
		if got := route(p, msg); got != (Outcome{Recognized: true, Implemented: true}) {
			t.Errorf("route = %+v", got)
		}
		if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() != itemBronzeSword {
			t.Errorf("op2 removed the sword: %+v", got)
		}
		if len(sess.messages) != 1 || sess.messages[0] != "Nothing interesting happens." {
			t.Errorf("messages = %v, want [\"Nothing interesting happens.\"]", sess.messages)
		}
	})
	t.Run("group 84 tab resolves", func(t *testing.T) {
		p := testPlayer()
		p.SetItemDefinitions(fixtureDefs())
		setEq(p, game.WeaponSlot, itemBronzeSword, 1)
		// Group 84, first child 10: weapon is child 13.
		msg := protocol.WidgetAction{WidgetID: int32(84<<16 | 13), GroupID: 84, ChildID: 13, ButtonNum: 1, HasButtonNum: true}
		if got := route(p, msg); got != (Outcome{Recognized: true, Implemented: true}) {
			t.Errorf("route = %+v", got)
		}
	})
	t.Run("group 12 never resolves without bank-open state", func(t *testing.T) {
		p := testPlayer()
		p.SetItemDefinitions(fixtureDefs())
		setEq(p, game.WeaponSlot, itemBronzeSword, 1)
		// Group 12, first child 68: weapon would be child 71.
		msg := protocol.WidgetAction{WidgetID: int32(12<<16 | 71), GroupID: 12, ChildID: 71, ButtonNum: 1, HasButtonNum: true}
		if got := route(p, msg); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Errorf("route = %+v", got)
		}
		if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() != itemBronzeSword {
			t.Errorf("group-12 click removed the sword: %+v", got)
		}
	})
}

func TestWidgetInventoryBranch(t *testing.T) {
	route := func(p *game.Player, msg protocol.WidgetAction, deps Deps) Outcome {
		drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
		acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
		noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
		return routeWidgetAction(p, msg, deps, drop, acted, noAction)
	}
	widgetID := int32(149 << 16)
	t.Run("button 2 wield equips", func(t *testing.T) {
		p := testPlayer()
		p.SetItemDefinitions(fixtureDefs())
		setInv(p, 0, itemBronzeSword, 1)
		msg := protocol.WidgetAction{WidgetID: widgetID, GroupID: 149, ChildID: 0, ButtonNum: 2, HasButtonNum: true, Slot: 0, ItemID: itemBronzeSword}
		if got := route(p, msg, Deps{}); got != (Outcome{Recognized: true, Implemented: true}) {
			t.Fatalf("route = %+v", got)
		}
		if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() != itemBronzeSword {
			t.Errorf("weapon slot = %+v, want bronze sword", got)
		}
	})
	t.Run("button 10 examines", func(t *testing.T) {
		p := testPlayer()
		p.SetItemDefinitions(fixtureDefs())
		setInv(p, 0, itemBronzeSword, 1)
		msg := protocol.WidgetAction{WidgetID: widgetID, GroupID: 149, ChildID: 0, ButtonNum: 10, HasButtonNum: true, Slot: 0, ItemID: itemBronzeSword}
		if got := route(p, msg, Deps{}); got != (Outcome{Recognized: true, Reason: ReasonItemExamine}) {
			t.Errorf("route = %+v", got)
		}
	})
	t.Run("unresolved op drops with the dispatch reason", func(t *testing.T) {
		p := testPlayer()
		p.SetItemDefinitions(fixtureDefs())
		setInv(p, 0, itemBronzeSword, 1)
		// Button 3 resolves to the empty second action.
		msg := protocol.WidgetAction{WidgetID: widgetID, GroupID: 149, ChildID: 0, ButtonNum: 3, HasButtonNum: true, Slot: 0, ItemID: itemBronzeSword}
		if got := route(p, msg, Deps{}); got != (Outcome{Recognized: true, Reason: ReasonWidgetAction}) {
			t.Errorf("route = %+v", got)
		}
	})
	t.Run("slot item mismatch drops", func(t *testing.T) {
		p := testPlayer()
		p.SetItemDefinitions(fixtureDefs())
		setInv(p, 0, itemIronSword, 1)
		msg := protocol.WidgetAction{WidgetID: widgetID, GroupID: 149, ChildID: 0, ButtonNum: 2, HasButtonNum: true, Slot: 0, ItemID: itemBronzeSword}
		if got := route(p, msg, Deps{}); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Errorf("route = %+v", got)
		}
	})
	t.Run("button 7 drop drops the item", func(t *testing.T) {
		p := testPlayer()
		p.SetItemDefinitions(fixtureDefs())
		setInv(p, 0, itemPlain, 1)
		store := newManagerStore()
		msg := protocol.WidgetAction{WidgetID: widgetID, GroupID: 149, ChildID: 0, ButtonNum: 7, HasButtonNum: true, Slot: 0, ItemID: itemPlain}
		if got := route(p, msg, Deps{GroundItems: store}); got != (Outcome{Recognized: true, Implemented: true}) {
			t.Fatalf("route = %+v", got)
		}
		if len(store.Items()) != 1 {
			t.Errorf("store has %d stacks, want 1", len(store.Items()))
		}
	})
}

func TestWidgetEquipmentViaHandle(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setEq(p, game.WeaponSlot, itemBronzeSword, 1)
	widgetID := int32(387<<16 | 18)
	payload := ifButtonPayload(widgetID, 0, 0xffff)
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeIfButton1, Payload: payload}, 7, Deps{})
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("Handle = %+v, want acted", got)
	}
	if p.Inventory().GetAmount(itemBronzeSword) != 1 {
		t.Error("sword did not return to the inventory")
	}
}

func TestHandleNilPlayerImplementedRoutes(t *testing.T) {
	cases := []struct {
		name    string
		opcode  byte
		payload []byte
	}{
		{"inventory use", protocol.ClientInventoryUse, inventoryUsePayload(0, itemPlain, "Drop")},
		{"inventory move", protocol.ClientInventoryMove, inventoryMovePayload(0, 1)},
		{"ground take", protocol.ClientGroundItemAction, groundHighPayload(0, 3089, 3524, itemPlain, "take", 3)},
		{"widget action", protocol.NativeIfButton1, ifButtonPayload(int32(387<<16|18), 0, 0xffff)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Handle(nil, game.TurnPacket{Opcode: tc.opcode, Payload: tc.payload}, 7, Deps{GroundItems: newManagerStore()})
			if got != (Outcome{Recognized: true, Reason: ReasonNoPlayer}) {
				t.Errorf("Handle = %+v, want no-player reason", got)
			}
		})
	}
}

func TestSameStackMetaEquality(t *testing.T) {
	a := itemWithMeta(itemPlain, 1, map[string]json.RawMessage{"a": json.RawMessage(`1`), "b": json.RawMessage(`"x"`)})
	b := itemWithMeta(itemPlain, 1, map[string]json.RawMessage{"b": json.RawMessage(`"x"`), "a": json.RawMessage(`1`)})
	if !sameStack(a, b) {
		t.Error("equal meta in different key order did not compare equal")
	}
	// Number literals are preserved verbatim, matching
	// game.Item.metaFingerprint: 1 and 1.0 are different
	// fingerprints here, as in the container layer.
	b2 := itemWithMeta(itemPlain, 1, map[string]json.RawMessage{"a": json.RawMessage(`1.0`), "b": json.RawMessage(`"x"`)})
	if sameStack(a, b2) {
		t.Error("1 vs 1.0 meta compared equal; literals are preserved verbatim")
	}
	c := itemWithMeta(itemPlain, 1, map[string]json.RawMessage{"a": json.RawMessage(`2`)})
	if sameStack(a, c) {
		t.Error("different meta compared equal")
	}
	d := itemWithMeta(itemBronzeSword, 1, map[string]json.RawMessage{"a": json.RawMessage(`1`), "b": json.RawMessage(`"x"`)})
	if sameStack(a, d) {
		t.Error("different ids compared equal")
	}
	// Amounts are NOT part of stack identity: TS sameStack
	// compares id + meta only, and the merge path adds amounts.
	if !sameStack(game.NewItem(itemPlain, 1), game.NewItem(itemPlain, 5)) {
		t.Error("same id + meta with different amounts did not compare equal")
	}
}

func TestMetaConversionRoundTrip(t *testing.T) {
	meta := map[string]json.RawMessage{
		"num":    json.RawMessage(`42`),
		"str":    json.RawMessage(`"hello"`),
		"bool":   json.RawMessage(`true`),
		"nested": json.RawMessage(`{"k":[1,2]}`),
	}
	ground := itemMetaToGround(meta)
	if ground == nil {
		t.Fatal("conversion dropped the meta map")
	}
	back := groundMetaToItem(ground)
	if metaFingerprint(meta) != metaFingerprint(back) {
		t.Errorf("round trip changed meta: %v -> %v", meta, back)
	}
	if itemMetaToGround(nil) != nil || groundMetaToItem(nil) != nil {
		t.Error("nil meta did not stay nil through conversion")
	}
}

func TestIsOwnerName(t *testing.T) {
	if !isOwnerName("Test Player", "test_player") {
		t.Error("case/space-insensitive owner match failed")
	}
	if isOwnerName("", "test_player") {
		t.Error("empty owner matched a player")
	}
	if isOwnerName("someone_else", "test_player") {
		t.Error("different owner matched")
	}
}

func TestResolveEquipmentSlot(t *testing.T) {
	cases := []struct {
		group, child, want int
	}{
		{387, 15, game.HeadSlot},
		{387, 18, game.WeaponSlot},
		{387, 25, game.AmmunitionSlot},
		{387, 26, -1},
		{387, 14, -1},
		{84, 10, game.HeadSlot},
		{84, 13, game.WeaponSlot},
		{12, 68, game.HeadSlot},
		{12, 71, game.WeaponSlot},
		{149, 0, -1},
		{0, 0, -1},
	}
	for _, tc := range cases {
		if got := resolveEquipmentSlot(tc.group, tc.child); got != tc.want {
			t.Errorf("resolveEquipmentSlot(%d, %d) = %d, want %d", tc.group, tc.child, got, tc.want)
		}
	}
}

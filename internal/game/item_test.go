package game

import (
	"encoding/json"
	"testing"
)

// testItemDefs is a hand-built ItemDefinitionSource for container
// tests: per-id flags and values set by the test, defaults false /
// zero / -1 / identity, matching the nil-source defaults.
type testItemDefs struct {
	stackable    map[int]bool
	noted        map[int]bool
	weights      map[int]int
	slots        map[int]int
	doubleHanded map[int]bool
	requirements map[int][]int
	combatReq    map[int]int
	tradeable    map[int]bool
	sellable     map[int]bool
	dropable     map[int]bool
	unNote       map[int]int
	placeholder  map[int]int
}

func (s *testItemDefs) IsStackable(id int) bool { return s.stackable[id] }
func (s *testItemDefs) IsNoted(id int) bool     { return s.noted[id] }
func (s *testItemDefs) WeightGrams(id int) int  { return s.weights[id] }
func (s *testItemDefs) IsDoubleHanded(id int) bool {
	return s.doubleHanded[id]
}
func (s *testItemDefs) EquipmentSlot(id int) int {
	if slot, ok := s.slots[id]; ok {
		return slot
	}
	return -1
}
func (s *testItemDefs) Requirements(id int) []int {
	if req, ok := s.requirements[id]; ok {
		return append([]int(nil), req...)
	}
	return nil
}
func (s *testItemDefs) CombatRequirement(id int) int { return s.combatReq[id] }
func (s *testItemDefs) IsTradeable(id int) bool      { return s.tradeable[id] }
func (s *testItemDefs) IsSellable(id int) bool       { return s.sellable[id] }
func (s *testItemDefs) IsDropable(id int) bool       { return s.dropable[id] }
func (s *testItemDefs) UnNote(id int) int {
	if un, ok := s.unNote[id]; ok {
		return un
	}
	return id
}
func (s *testItemDefs) PlaceholderID(id int) int {
	if ph, ok := s.placeholder[id]; ok {
		return ph
	}
	return -1
}

func TestItemNewAndAccessors(t *testing.T) {
	it := NewItem(995, 100)
	if it.ID() != 995 || it.Amount() != 100 {
		t.Fatalf("NewItem(995, 100) = id %d amount %d", it.ID(), it.Amount())
	}
	if !it.IsValid() || it.IsEmpty() {
		t.Fatalf("valid stack reads IsValid=%v IsEmpty=%v", it.IsValid(), it.IsEmpty())
	}
	empty := NewItem(-1, 0)
	if empty.IsValid() || !empty.IsEmpty() {
		t.Fatalf("empty filler reads IsValid=%v IsEmpty=%v", empty.IsValid(), empty.IsEmpty())
	}
	// A zero-amount stack is not valid (isValid, Item.ts:151-153).
	zero := NewItem(995, 0)
	if zero.IsValid() || !zero.IsEmpty() {
		t.Fatalf("zero-amount stack reads IsValid=%v IsEmpty=%v", zero.IsValid(), zero.IsEmpty())
	}
	it.SetID(4151)
	it.SetAmount(3)
	if it.ID() != 4151 || it.Amount() != 3 {
		t.Fatalf("after SetID/SetAmount: id %d amount %d", it.ID(), it.Amount())
	}
}

func TestItemAmountAdjustments(t *testing.T) {
	it := NewItem(1, MaxStackAmount)
	it.IncrementAmount()
	if it.Amount() != MaxStackAmount {
		t.Fatalf("IncrementAmount at cap = %d, want %d", it.Amount(), MaxStackAmount)
	}
	it = NewItem(1, 0)
	it.DecrementAmount()
	if it.Amount() != 0 {
		t.Fatalf("DecrementAmount at 0 = %d", it.Amount())
	}
	it = NewItem(1, MaxStackAmount-1)
	it.IncrementAmountBy(5)
	if it.Amount() != MaxStackAmount {
		t.Fatalf("IncrementAmountBy past cap = %d, want %d", it.Amount(), MaxStackAmount)
	}
	it = NewItem(1, 3)
	it.DecrementAmountBy(5)
	if it.Amount() != 0 {
		t.Fatalf("DecrementAmountBy below 1 = %d, want 0", it.Amount())
	}
	it = NewItem(1, 10)
	it.DecrementAmountBy(3)
	if it.Amount() != 7 {
		t.Fatalf("DecrementAmountBy(3) from 10 = %d, want 7", it.Amount())
	}
}

func rawJSON(t *testing.T, s string) json.RawMessage {
	t.Helper()
	var raw json.RawMessage = json.RawMessage(s)
	if !json.Valid(raw) {
		t.Fatalf("invalid fixture JSON: %s", s)
	}
	return raw
}

func TestItemMetaAccessors(t *testing.T) {
	it := NewItemWithMeta(1, 1, map[string]json.RawMessage{"a": rawJSON(t, "1")})
	// Meta returns a copy: mutating it must not touch the item.
	meta := it.Meta()
	meta["a"] = rawJSON(t, "2")
	meta["b"] = rawJSON(t, "true")
	if got, ok := it.GetMetaValue("a"); !ok || string(got) != "1" {
		t.Fatalf("GetMetaValue(a) = %s,%v after mutating Meta() copy", got, ok)
	}
	if _, ok := it.GetMetaValue("b"); ok {
		t.Fatal("GetMetaValue(b) found a key added only to the Meta() copy")
	}
	// Empty key: no-op both ways.
	if _, ok := it.GetMetaValue(""); ok {
		t.Fatal("GetMetaValue(\"\") returned a value")
	}
	it.SetMetaValue("", rawJSON(t, "1"))
	if _, ok := it.GetMetaValue(""); ok {
		t.Fatal("SetMetaValue(\"\") stored a value")
	}
	// Missing key: absent.
	if _, ok := it.GetMetaValue("nope"); ok {
		t.Fatal("GetMetaValue(missing) returned a value")
	}
	// Set then delete via nil value; last-key deletion nulls meta.
	it.SetMetaValue("x", rawJSON(t, `{"n":1}`))
	if it.Meta() == nil {
		t.Fatal("Meta() nil after SetMetaValue")
	}
	it.SetMetaValue("a", nil)
	it.SetMetaValue("x", nil)
	if it.Meta() != nil {
		t.Fatalf("Meta() = %v after deleting all keys, want nil", it.Meta())
	}
	// JSON null is stored, not treated as delete.
	it.SetMetaValue("n", rawJSON(t, "null"))
	if got, ok := it.GetMetaValue("n"); !ok || string(got) != "null" {
		t.Fatalf("GetMetaValue(n) = %s,%v, want null,true", got, ok)
	}
	// SetMeta clones: later caller mutation must not leak in.
	external := map[string]json.RawMessage{"k": rawJSON(t, "1")}
	it.SetMeta(external)
	external["k"] = rawJSON(t, "9")
	if got, _ := it.GetMetaValue("k"); string(got) != "1" {
		t.Fatalf("SetMeta aliased caller map: k = %s", got)
	}
	it.SetMeta(nil)
	if it.Meta() != nil {
		t.Fatal("SetMeta(nil) did not clear meta")
	}
}

func TestItemCloneDeepMeta(t *testing.T) {
	it := NewItemWithMeta(1, 2, map[string]json.RawMessage{"o": rawJSON(t, `{"x":[1,2]}`)})
	clone := it.Clone()
	clone.SetMetaValue("o", rawJSON(t, `{"x":[3]}`))
	clone.SetAmount(9)
	if got, _ := it.GetMetaValue("o"); string(got) != `{"x":[1,2]}` {
		t.Fatalf("Clone shared meta: o = %s", got)
	}
	if it.Amount() != 2 {
		t.Fatalf("Clone shared amount: %d", it.Amount())
	}
	if !it.Equals(NewItemWithMeta(1, 2, map[string]json.RawMessage{"o": rawJSON(t, `{"x":[1,2]}`)})) {
		t.Fatal("Equals: identical items compare unequal")
	}
}

func TestItemEqualsMetaCanonical(t *testing.T) {
	a := NewItemWithMeta(1, 1, map[string]json.RawMessage{
		"a": rawJSON(t, "1"), "b": rawJSON(t, `{"y": 2, "x": 1}`),
	})
	// Same keys/values, different order and spacing: equal, as
	// JSON.stringify comparison is order-sensitive in TS only for
	// nested insertion order — top-level order is normalized by
	// Go's encoder; nested order is normalized too (documented
	// deviation in metaFingerprint).
	b := NewItemWithMeta(1, 1, map[string]json.RawMessage{
		"b": rawJSON(t, `{"x":1,"y":2}`), "a": rawJSON(t, "1"),
	})
	if !a.Equals(b) {
		t.Fatal("Equals: canonically equal metas compare unequal")
	}
	c := NewItemWithMeta(1, 1, map[string]json.RawMessage{"a": rawJSON(t, "2")})
	if a.Equals(c) {
		t.Fatal("Equals: different meta value compares equal")
	}
	if a.Equals(NewItem(1, 1)) {
		t.Fatal("Equals: meta vs no-meta compares equal")
	}
	if a.Equals(NewItem(2, 1)) || a.Equals(NewItemWithMeta(1, 2, a.Meta())) {
		t.Fatal("Equals: different id/amount compares equal")
	}
	// Nil meta vs empty meta differ (TS: "null" vs "{}").
	if NewItem(1, 1).Equals(NewItemWithMeta(1, 1, map[string]json.RawMessage{})) {
		t.Fatal("Equals: nil meta vs empty meta compares equal")
	}
}

func TestMigrateSavedMeta(t *testing.T) {
	in := map[string]json.RawMessage{
		"toxicBlowpipe":   rawJSON(t, "5"),
		"tridentCharges":  rawJSON(t, "3"),
		"trident-charges": rawJSON(t, "9"),
		"lootingBag":      rawJSON(t, "1"),
	}
	out := MigrateSavedMeta(in)
	if got, ok := out["toxic-blowpipe"]; !ok || string(got) != "5" {
		t.Fatalf("toxic-blowpipe = %s,%v", got, ok)
	}
	if _, ok := out["toxicBlowpipe"]; ok {
		t.Fatal("old key toxicBlowpipe survived migration")
	}
	// New key wins when both are saved.
	if got := string(out["trident-charges"]); got != "9" {
		t.Fatalf("trident-charges = %s, want 9 (new key wins)", got)
	}
	// Unrelated camelCase keys are untouched.
	if _, ok := out["lootingBag"]; !ok {
		t.Fatal("lootingBag key lost in migration")
	}
	// Input is not mutated.
	if _, ok := in["toxic-blowpipe"]; ok {
		t.Fatal("MigrateSavedMeta mutated its input")
	}
	if MigrateSavedMeta(nil) != nil {
		t.Fatal("MigrateSavedMeta(nil) != nil")
	}
}

func TestCloneMeta(t *testing.T) {
	if CloneMeta(nil) != nil {
		t.Fatal("CloneMeta(nil) != nil")
	}
	in := map[string]json.RawMessage{"k": rawJSON(t, "[1]")}
	out := CloneMeta(in)
	out["k"][1] = '2'
	if string(in["k"]) != "[1]" {
		t.Fatalf("CloneMeta shared value bytes: %s", in["k"])
	}
}

func boolMeta(t *testing.T, key string, value string) Item {
	t.Helper()
	return NewItemWithMeta(100, 1, map[string]json.RawMessage{key: rawJSON(t, value)})
}

func TestItemTradeablePredicates(t *testing.T) {
	src := &testItemDefs{
		tradeable: map[int]bool{100: true, 101: false},
		sellable:  map[int]bool{100: true},
		dropable:  map[int]bool{100: true},
	}
	plain := NewItem(100, 1)
	if !plain.IsTradeable(src) || !plain.IsSellable(src) || !plain.IsDropable(src) {
		t.Fatal("tradeable/sellable/dropable item reads false")
	}
	if plain.IsUntradeable() || plain.IsUnbankable() || plain.IsPresetItem() || plain.IsLostOnDeath() {
		t.Fatal("plain item reads flagged")
	}
	flagged := boolMeta(t, "untradeable", "true")
	if flagged.IsTradeable(src) || flagged.IsSellable(src) || flagged.IsDropable(src) {
		t.Fatal("untradeable-flagged item reads tradeable/sellable/dropable")
	}
	if !flagged.IsUntradeable() || !flagged.IsLostOnDeath() {
		t.Fatal("untradeable-flagged item does not read untradeable/lost-on-death")
	}
	// The flag test is === true: the string "true" does not count.
	strFlag := boolMeta(t, "untradeable", `"true"`)
	if strFlag.IsUntradeable() {
		t.Fatal("string \"true\" meta counts as untradeable flag")
	}
	if !boolMeta(t, "unbankable", "true").IsUnbankable() {
		t.Fatal("unbankable flag not read")
	}
	if !boolMeta(t, "preset", "true").IsPresetItem() {
		t.Fatal("preset flag not read")
	}
	// Definition says no: predicates follow the definition.
	no := NewItem(101, 1)
	if no.IsTradeable(src) {
		t.Fatal("untradeable-by-definition item reads tradeable")
	}
	// Nil source: DEFAULT-definition falsy behavior.
	if plain.IsTradeable(nil) || plain.IsSellable(nil) || plain.IsDropable(nil) {
		t.Fatal("nil-source predicates read true")
	}
}

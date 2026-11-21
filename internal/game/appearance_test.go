package game

import (
	"reflect"
	"testing"
)

func newTestAppearance() (*Player, *Appearance) {
	p := NewPlayer("testplayer", Location{X: 3089, Y: 3524, Z: 0})
	return p, p.Appearance()
}

func TestNewAppearanceMaleDefaults(t *testing.T) {
	p, a := newTestAppearance()
	// The constructor runs set() with gender 0 (Appearance.ts:38-41,
	// 77-101): the male defaults, and Flag.APPEARANCE set.
	want := []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0}
	if got := a.Look(); !reflect.DeepEqual(got, want) {
		t.Errorf("Look() = %v, want %v", got, want)
	}
	if !a.IsMale() {
		t.Error("IsMale = false for gender 0, want true")
	}
	if !p.Flags.Has(FlagAppearance) {
		t.Error("constructor did not flag APPEARANCE")
	}
	if got := a.HeadHint(); got != -1 {
		t.Errorf("HeadHint = %d, want -1", got)
	}
	if a.CanChangeAppearance() {
		t.Error("CanChangeAppearance = true, want false")
	}
}

func TestAppearanceSetFemaleDefaults(t *testing.T) {
	_, a := newTestAppearance()
	a.SetLook(LookGender, 1)
	a.Set()
	want := []int{1, 48, 57, 65, 68, 77, 80, -1, 2, 14, 5, 4, 0}
	if got := a.Look(); !reflect.DeepEqual(got, want) {
		t.Errorf("female Look() = %v, want %v", got, want)
	}
	if a.IsMale() {
		t.Error("IsMale = true for gender 1, want false")
	}
	// Set() does not touch the gender slot itself: switching back
	// to male and calling Set() restores the male kits.
	a.SetLook(LookGender, 0)
	a.Set()
	if got := a.Look()[LookHead]; got != 3 {
		t.Errorf("male head after re-Set = %d, want 3", got)
	}
}

func TestAppearanceSetLook(t *testing.T) {
	p, a := newTestAppearance()
	p.Flags = p.Flags.Reset()
	a.SetLook(LookHead, 5)
	if got := a.Look()[LookHead]; got != 5 {
		t.Errorf("head = %d, want 5", got)
	}
	if !p.Flags.Has(FlagAppearance) {
		t.Error("SetLook did not flag APPEARANCE")
	}
	// Out-of-range indices are ignored (no panic, no flag).
	p.Flags = p.Flags.Reset()
	a.SetLook(-1, 9)
	a.SetLook(LookLength, 9)
	if p.Flags.Has(FlagAppearance) {
		t.Error("out-of-range SetLook flagged APPEARANCE")
	}
	if got := len(a.Look()); got != LookLength {
		t.Errorf("look length = %d, want %d", got, LookLength)
	}
}

func TestAppearanceSetLookArray(t *testing.T) {
	p, a := newTestAppearance()
	look := []int{1, 48, 57, 65, 68, 77, 80, -1, 2, 14, 5, 4, 0}
	p.Flags = p.Flags.Reset()
	if err := a.SetLookArray(look); err != nil {
		t.Fatalf("SetLookArray: %v", err)
	}
	if got := a.Look(); !reflect.DeepEqual(got, look) {
		t.Errorf("Look() = %v, want %v", got, look)
	}
	if !p.Flags.Has(FlagAppearance) {
		t.Error("SetLookArray did not flag APPEARANCE")
	}
	// Defensive copy: mutating the input afterwards changes nothing.
	look[LookHead] = 0
	if got := a.Look()[LookHead]; got != 48 {
		t.Errorf("head after input mutation = %d, want 48", got)
	}
	// Look() is a defensive copy too.
	got := a.Look()
	got[LookHead] = 0
	if a.Look()[LookHead] != 48 {
		t.Error("mutating Look() result changed live state")
	}
	// Length < 12 errors (TS throws) and changes nothing.
	p.Flags = p.Flags.Reset()
	if err := a.SetLookArray([]int{0, 1, 2}); err == nil {
		t.Error("SetLookArray(3 entries) = nil error, want an error")
	}
	if got := a.Look()[LookHead]; got != 48 {
		t.Errorf("head after failed SetLookArray = %d, want 48", got)
	}
	if p.Flags.Has(FlagAppearance) {
		t.Error("failed SetLookArray flagged APPEARANCE")
	}
	// A 12-entry input is accepted (TS threshold is < 12) and
	// gains a 0 skin colour, keeping the look 13 entries.
	twelve := []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4}
	if err := a.SetLookArray(twelve); err != nil {
		t.Fatalf("SetLookArray(12 entries): %v", err)
	}
	if got := a.Look(); len(got) != LookLength || got[LookSkinColour] != 0 {
		t.Errorf("12-entry look = %v, want length 13 with skin 0", got)
	}
	// Over-long inputs truncate to 13.
	long := make([]int, 20)
	if err := a.SetLookArray(long); err != nil {
		t.Fatalf("SetLookArray(20 entries): %v", err)
	}
	if got := len(a.Look()); got != LookLength {
		t.Errorf("look length after long input = %d, want %d", got, LookLength)
	}
}

func TestAppearanceHeadHintAndCanChange(t *testing.T) {
	p, a := newTestAppearance()
	p.Flags = p.Flags.Reset()
	a.SetHeadHint(3)
	if got := a.HeadHint(); got != 3 {
		t.Errorf("HeadHint = %d, want 3", got)
	}
	if !p.Flags.Has(FlagAppearance) {
		t.Error("SetHeadHint did not flag APPEARANCE")
	}
	// setCanChangeAppearance does NOT flag (Appearance.ts:56-58).
	p.Flags = p.Flags.Reset()
	a.SetCanChangeAppearance(true)
	if !a.CanChangeAppearance() {
		t.Error("CanChangeAppearance = false after set, want true")
	}
	if p.Flags.Has(FlagAppearance) {
		t.Error("SetCanChangeAppearance flagged APPEARANCE")
	}
}

func TestAppearancePronouns(t *testing.T) {
	p, a := newTestAppearance()
	// Unset: the gender fallback (male 0 / female 1).
	if got := a.Pronouns(); got != 0 {
		t.Errorf("male default pronouns = %d, want 0", got)
	}
	a.SetLook(LookGender, 1)
	if got := a.Pronouns(); got != 1 {
		t.Errorf("female default pronouns = %d, want 1", got)
	}
	// A valid value stores and flags.
	p.Flags = p.Flags.Reset()
	a.SetPronouns(2)
	if got := a.Pronouns(); got != 2 {
		t.Errorf("pronouns = %d, want 2", got)
	}
	if !p.Flags.Has(FlagAppearance) {
		t.Error("SetPronouns did not flag APPEARANCE")
	}
	// Out-of-range values return silently: no change, no flag.
	p.Flags = p.Flags.Reset()
	a.SetPronouns(3)
	a.SetPronouns(-1)
	if got := a.Pronouns(); got != 2 {
		t.Errorf("pronouns after invalid sets = %d, want 2", got)
	}
	if p.Flags.Has(FlagAppearance) {
		t.Error("invalid SetPronouns flagged APPEARANCE")
	}
	if PronounsAttribute != "appearance:pronouns" {
		t.Errorf("PronounsAttribute = %q, want %q", PronounsAttribute, "appearance:pronouns")
	}
}

func TestAppearanceSkinUnlocks(t *testing.T) {
	_, a := newTestAppearance()
	// Default: base palette 0-7 + 12 unlocked (Appearance.ts:26-30).
	want := []int{0, 1, 2, 3, 4, 5, 6, 7, 12}
	if got := a.UnlockedSkinColours(); !reflect.DeepEqual(got, want) {
		t.Errorf("UnlockedSkinColours() = %v, want %v", got, want)
	}
	if a.IsSkinColourUnlocked(8) || a.IsSkinColourUnlocked(9) ||
		a.IsSkinColourUnlocked(10) || a.IsSkinColourUnlocked(11) {
		t.Error("gated colour unlocked by default")
	}
	if !a.IsSkinColourUnlocked(12) {
		t.Error("colour 12 locked by default, want unlocked")
	}
	// Out-of-palette values are never unlocked.
	if a.IsSkinColourUnlocked(-1) || a.IsSkinColourUnlocked(13) {
		t.Error("out-of-palette colour reported unlocked")
	}
	// Loading the hw07 unlock opens green (8) only.
	a.SetSkinUnlock("appearance:hw07-skin-colours", true)
	if !a.IsSkinColourUnlocked(8) {
		t.Error("colour 8 locked after hw07 unlock")
	}
	if a.IsSkinColourUnlocked(9) {
		t.Error("colour 9 unlocked by hw07 alone")
	}
	// The hw15 unlock opens black and white (9, 10).
	a.SetSkinUnlock("appearance:hw15-skin-colours", true)
	if !a.IsSkinColourUnlocked(9) || !a.IsSkinColourUnlocked(10) {
		t.Error("colours 9/10 locked after hw15 unlock")
	}
	// Unknown attributes are ignored.
	a.SetSkinUnlock("appearance:nope", true)
	if a.IsSkinColourUnlocked(11) {
		t.Error("colour 11 unlocked by an unknown attribute")
	}
	// Unlocks can be revoked by loading false.
	a.SetSkinUnlock("appearance:hw07-skin-colours", false)
	if a.IsSkinColourUnlocked(8) {
		t.Error("colour 8 unlocked after revoke")
	}
	if SkinColourCount != 13 {
		t.Errorf("SkinColourCount = %d, want 13", SkinColourCount)
	}
	if len(SkinUnlocks) != 3 {
		t.Errorf("SkinUnlocks entries = %d, want 3", len(SkinUnlocks))
	}
}

func TestStepSkinColour(t *testing.T) {
	unlocked := []int{0, 1, 2, 3, 4, 5, 6, 7, 12}
	cases := []struct {
		current, direction, want int
		unlocked                 []int
	}{
		{0, 1, 1, unlocked},
		{7, 1, 12, unlocked},  // skips gated 8-11
		{12, 1, 0, unlocked},  // wraps
		{0, -1, 12, unlocked}, // wraps backwards
		{12, -1, 7, unlocked},
		{8, 1, 12, unlocked},   // a locked current steps off it
		{8, -1, 7, unlocked},   // ... in either direction
		{99, 1, 0, unlocked},   // out-of-palette current, forward
		{99, -1, 12, unlocked}, // out-of-palette current, backward
		{0, 0, 1, unlocked},    // direction 0 steps forward (TS: < 0 ? -1 : 1)
		{5, 1, 5, nil},         // nothing unlocked: returns current
		{5, -1, 5, []int{}},    // ... also for an empty list
	}
	for _, tc := range cases {
		if got := StepSkinColour(tc.current, tc.direction, tc.unlocked); got != tc.want {
			t.Errorf("StepSkinColour(%d, %d, %v) = %d, want %d",
				tc.current, tc.direction, tc.unlocked, got, tc.want)
		}
	}
}

func TestSyncAnimationsDefaults(t *testing.T) {
	p, _ := newTestAppearance()
	want := []int{808, 823, 819, 820, 821, 822, 824}
	if got := p.SyncAnimations(); !reflect.DeepEqual(got, want) {
		t.Errorf("SyncAnimations() = %v, want %v", got, want)
	}
	// Freshly allocated on every call: mutating one result must not
	// affect the next.
	first := p.SyncAnimations()
	first[0] = 0
	if got := p.SyncAnimations()[0]; got != 808 {
		t.Errorf("SyncAnimations()[0] after mutation = %d, want 808", got)
	}
}

// animTestDefs is an ItemDefinitionSource that also implements
// ItemAnimationSource, for the weapon-override test: only the
// animation methods are ever called, so the embedded nil
// ItemDefinitionSource is never dereferenced.
type animTestDefs struct {
	ItemDefinitionSource
	stand map[int]int
	walk  map[int]int
	run   map[int]int
}

func (s animTestDefs) StandAnim(id int) int { return s.stand[id] }
func (s animTestDefs) WalkAnim(id int) int  { return s.walk[id] }
func (s animTestDefs) RunAnim(id int) int   { return s.run[id] }

func TestSyncAnimationsWeaponOverride(t *testing.T) {
	p, _ := newTestAppearance()
	// A source without the animation capability yields the
	// defaults even with a weapon equipped.
	p.SetItemDefinitions(&testItemDefs{})
	p.Equipment().Set(WeaponSlot, NewItem(4151, 1))
	want := []int{808, 823, 819, 820, 821, 822, 824}
	if got := p.SyncAnimations(); !reflect.DeepEqual(got, want) {
		t.Fatalf("no animation source = %v, want %v", got, want)
	}
	// With the capability, the weapon's overrides replace the
	// stand/walk/run slots only (PlayerSession.ts:685-694).
	p.SetItemDefinitions(animTestDefs{
		stand: map[int]int{4151: 1000},
		walk:  map[int]int{4151: 1001},
		run:   map[int]int{4151: 1002},
	})
	want = []int{1000, 823, 1001, 820, 821, 822, 1002}
	if got := p.SyncAnimations(); !reflect.DeepEqual(got, want) {
		t.Fatalf("weapon override = %v, want %v", got, want)
	}
	// Unequipped, the defaults return without consulting the
	// source.
	p.Equipment().Set(WeaponSlot, NewItem(-1, 0))
	want = []int{808, 823, 819, 820, 821, 822, 824}
	if got := p.SyncAnimations(); !reflect.DeepEqual(got, want) {
		t.Fatalf("empty weapon slot = %v, want %v", got, want)
	}
}

func TestOnEquipmentChangedInvokesHook(t *testing.T) {
	// The scaffold contract the appearance adapter relies on
	// (player.go): onEquipmentChanged flags APPEARANCE and
	// invokes the coordinator-installed OnEquipmentChanged hook.
	// Equipment refresh calls it (container.go RefreshItems).
	p, _ := newTestAppearance()
	p.Flags = p.Flags.Reset()
	called := 0
	p.OnEquipmentChanged = func(pl *Player) {
		called++
		if pl != p {
			t.Errorf("hook player = %p, want %p", pl, p)
		}
	}
	p.onEquipmentChanged()
	if called != 1 {
		t.Fatalf("hook calls = %d, want 1", called)
	}
	if !p.Flags.Has(FlagAppearance) {
		t.Error("onEquipmentChanged did not flag APPEARANCE")
	}
	// No hook installed: flags, does not panic.
	p.OnEquipmentChanged = nil
	p.Flags = p.Flags.Reset()
	p.onEquipmentChanged()
	if !p.Flags.Has(FlagAppearance) {
		t.Error("onEquipmentChanged without hook did not flag APPEARANCE")
	}
}

func TestLookIndexConstants(t *testing.T) {
	// Appearance.ts:4-16.
	if LookGender != 0 || LookHead != 1 || LookChest != 2 || LookArms != 3 ||
		LookHands != 4 || LookLegs != 5 || LookFeet != 6 || LookBeard != 7 ||
		LookHairColour != 8 || LookTorsoCol != 9 || LookLegColour != 10 ||
		LookFeetColour != 11 || LookSkinColour != 12 {
		t.Error("look index constants do not match Appearance.ts:4-16")
	}
	if LookLength != 13 {
		t.Errorf("LookLength = %d, want 13", LookLength)
	}
}

func TestAppearancePronounsFromAttribute(t *testing.T) {
	p, a := newTestAppearance()
	// Pronouns reads the player attribute directly: content (or
	// entry loading) may set it without going through SetPronouns.
	p.SetAttribute(PronounsAttribute, 2)
	if got := a.Pronouns(); got != 2 {
		t.Errorf("pronouns from int attribute = %d, want 2", got)
	}
	// JSON-decoded saves carry numbers as float64: an integral
	// float64 counts (TS typeof number + Number.isInteger).
	p.SetAttribute(PronounsAttribute, float64(1))
	if got := a.Pronouns(); got != 1 {
		t.Errorf("pronouns from float64 attribute = %d, want 1", got)
	}
	// Non-integral, out-of-range, and non-numeric values fall
	// back to gender (male 0 here), as TS does on every read.
	for _, value := range []any{float64(2.5), 3, -1, "2", true, nil} {
		p.SetAttribute(PronounsAttribute, value)
		if got := a.Pronouns(); got != 0 {
			t.Errorf("pronouns from %#v = %d, want 0 (gender fallback)", value, got)
		}
	}
	// SetPronouns writes the attribute: the store holds the value.
	p.Flags = p.Flags.Reset()
	a.SetPronouns(2)
	if got := p.GetAttribute(PronounsAttribute); got != 2 {
		t.Errorf("attribute after SetPronouns = %#v, want 2", got)
	}
	if !p.HasAttribute(PronounsAttribute) {
		t.Error("attribute missing after SetPronouns")
	}
	// An invalid SetPronouns leaves the attribute untouched.
	a.SetPronouns(3)
	if got := p.GetAttribute(PronounsAttribute); got != 2 {
		t.Errorf("attribute after invalid SetPronouns = %#v, want 2", got)
	}
}

func TestAppearanceSkinUnlocksFromAttribute(t *testing.T) {
	p, a := newTestAppearance()
	// The unlock reads the attribute with strict === true: only
	// the boolean true unlocks.
	p.SetAttribute("appearance:hw07-skin-colours", true)
	if !a.IsSkinColourUnlocked(8) {
		t.Error("colour 8 locked with attribute true")
	}
	for _, value := range []any{1, "true", false} {
		p.SetAttribute("appearance:hw07-skin-colours", value)
		if a.IsSkinColourUnlocked(8) {
			t.Errorf("colour 8 unlocked with attribute %#v, want locked", value)
		}
	}
	// SetSkinUnlock(false) deletes the key, restoring the absent
	// state entry loading starts from — it does not store false.
	p.SetAttribute("appearance:hw07-skin-colours", true)
	a.SetSkinUnlock("appearance:hw07-skin-colours", false)
	if p.HasAttribute("appearance:hw07-skin-colours") {
		t.Error("SetSkinUnlock(false) left the key stored")
	}
	if a.IsSkinColourUnlocked(8) {
		t.Error("colour 8 unlocked after revoke")
	}
	// SetSkinUnlock(true) stores true; unknown attributes create
	// no key.
	a.SetSkinUnlock("appearance:hw15-skin-colours", true)
	if got := p.GetAttribute("appearance:hw15-skin-colours"); got != true {
		t.Errorf("attribute after SetSkinUnlock(true) = %#v, want true", got)
	}
	a.SetSkinUnlock("appearance:nope", true)
	if p.HasAttribute("appearance:nope") {
		t.Error("unknown attribute stored a key")
	}
}

func TestAppearanceNilPlayer(t *testing.T) {
	// A bare Appearance with no player reads attributes as
	// absent and never panics: pronouns fall back to gender,
	// gated colours read locked, writes store nothing.
	a := &Appearance{look: make([]int, LookLength)}
	if got := a.Pronouns(); got != 0 {
		t.Errorf("nil-player pronouns = %d, want 0 (male fallback)", got)
	}
	if a.IsSkinColourUnlocked(8) {
		t.Error("nil-player gated colour unlocked, want locked")
	}
	if !a.IsSkinColourUnlocked(0) {
		t.Error("nil-player open colour locked, want unlocked")
	}
	a.SetPronouns(2)
	if got := a.Pronouns(); got != 0 {
		t.Errorf("nil-player pronouns after set = %d, want 0", got)
	}
	a.SetSkinUnlock("appearance:hw07-skin-colours", true)
	if a.IsSkinColourUnlocked(8) {
		t.Error("nil-player gated colour unlocked after set, want locked")
	}
}

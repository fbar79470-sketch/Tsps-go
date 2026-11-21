package game

import (
	"reflect"
	"testing"
)

// soundFakeSession records sound sends through the SoundSender
// extension, on top of the package fakeSession's SessionPort.
type soundFakeSession struct {
	*fakeSession
	effects []soundEffectCall
	areas   []areaSoundCall
}

type soundEffectCall struct {
	soundID, loopType, delay, volume int
}

type areaSoundCall struct {
	soundID, x, y, level, loops, delay, radius, attenuation int
}

func newSoundFakeSession() *soundFakeSession {
	return &soundFakeSession{fakeSession: newFakeSession()}
}

func (f *soundFakeSession) SendSoundEffect(soundID, loopType, delay, volume int) {
	f.effects = append(f.effects, soundEffectCall{soundID, loopType, delay, volume})
}

func (f *soundFakeSession) SendAreaSound(soundID, x, y, level, loops, delay, radius, attenuation int) {
	f.areas = append(f.areas, areaSoundCall{soundID, x, y, level, loops, delay, radius, attenuation})
}

func soundTestPlayer(name string, loc Location) (*Player, *soundFakeSession) {
	p := NewPlayer(name, loc)
	sess := newSoundFakeSession()
	p.Session = sess
	return p, sess
}

func TestSoundConstructorOrderAndGetters(t *testing.T) {
	// (id, volume, delay, loopType): delay precedes loopType.
	s := NewSound(100, 7, 30, 1)
	if s.ID() != 100 || s.Volume() != 7 || s.Delay() != 30 || s.LoopType() != 1 {
		t.Errorf("NewSound getters = %d/%d/%d/%d, want 100/7/30/1", s.ID(), s.Volume(), s.Delay(), s.LoopType())
	}
	if s.ClientVolume() != 7 {
		t.Errorf("ClientVolume = %d, want 7", s.ClientVolume())
	}
	if NewSound(1, -3, 0, 0).ClientVolume() != 0 {
		t.Error("ClientVolume -3: want 0")
	}
	if NewSound(1, 99, 0, 0).ClientVolume() != 10 {
		t.Error("ClientVolume 99: want 10")
	}
}

func TestSoundTableSpotValues(t *testing.T) {
	cases := []struct {
		sound                       Sound
		id, volume, delay, loopType int
	}{
		{SoundCutting, 2605, 1, 0, 0},
		{SoundDragonBattleaxeSpecial, 2538, 1, 0, 1},
		{SoundDragonAxeSpecial, 2531, 1, 0, 1},
		{SoundDragonClawsSpecial2, 4140, 1, 30, 0},
		{SoundToxicBlowpipeSpecial, 800, 1, 32, 0},
		{SoundVoidwakerSpecial2, 6182, 1, 30, 0},
		{SoundLevelUp, 2396, 1, 0, 0},
		{SoundPrayerProtectMelee, 2676, 1, 0, 0},
		{SoundWeaponInfernalTecpatl, 11456, 1, 0, 0},
		{SoundHomeTeleport, 193, 1, 0, 0},
		{SoundXpReward, 2655, 1, 0, 0},
	}
	for _, tc := range cases {
		if tc.sound.ID() != tc.id || tc.sound.Volume() != tc.volume || tc.sound.Delay() != tc.delay || tc.sound.LoopType() != tc.loopType {
			t.Errorf("sound = %d/%d/%d/%d, want %d/%d/%d/%d",
				tc.sound.ID(), tc.sound.Volume(), tc.sound.Delay(), tc.sound.LoopType(),
				tc.id, tc.volume, tc.delay, tc.loopType)
		}
	}
}

func TestSoundAliasesEqualTargets(t *testing.T) {
	pairs := [][2]Sound{
		{SoundImpAttacking, SoundNpcAttacking},
		{SoundWeaponBow, SoundShootArrow},
		{SoundWeaponDragonDaggerStab, SoundWeaponDaggerStab},
		{SoundWeaponGuthanWarspear, SoundWeaponSpearStab},
		{SoundWeaponGraniteMaul, SoundWeaponWarhammer},
		{SoundIcaBarrageImpact, SoundIceBarrageImpact},
		{SoundRunecrafting, SoundCraftRunes},
		{SoundHomeTeleportAlt, SoundHomeTeleport},
	}
	for _, pair := range pairs {
		if pair[0] != pair[1] {
			t.Errorf("alias %v != target %v", pair[0], pair[1])
		}
	}
}

func TestKnownSoundRegistryCount(t *testing.T) {
	names := KnownSoundNames()
	// 161 names: 153 constructions + 8 aliases.
	if len(names) != 161 {
		t.Fatalf("KnownSoundNames count = %d, want 161", len(names))
	}
	if len(knownSounds) != 161 {
		t.Fatalf("knownSounds count = %d, want 161", len(knownSounds))
	}
	sorted := append([]string(nil), names...)
	// Sorted check via reflect: KnownSoundNames sorts.
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("names not sorted at %d: %q >= %q", i, names[i-1], names[i])
		}
	}
	_ = sorted
	if !reflect.DeepEqual(names, KnownSoundNames()) {
		t.Error("KnownSoundNames not stable")
	}
}

func TestResolveKnownSound(t *testing.T) {
	if s, ok := ResolveKnownSound(2396); !ok || s != SoundLevelUp {
		t.Errorf("ResolveKnownSound(2396) = %v/%v, want LEVEL_UP", s, ok)
	}
	if s, ok := ResolveKnownSound("LEVEL_UP"); !ok || s != SoundLevelUp {
		t.Errorf("ResolveKnownSound LEVEL_UP = %v/%v", s, ok)
	}
	if s, ok := ResolveKnownSound("  sound.level_up "); !ok || s != SoundLevelUp {
		t.Errorf("ResolveKnownSound normalized = %v/%v", s, ok)
	}
	if s, ok := ResolveKnownSound("2396"); !ok || s != SoundLevelUp {
		t.Errorf("ResolveKnownSound numeric string = %v/%v", s, ok)
	}
	if _, ok := ResolveKnownSound(999999); ok {
		t.Error("ResolveKnownSound unknown id: want false")
	}
	if _, ok := ResolveKnownSound("NO_SUCH_SOUND"); ok {
		t.Error("ResolveKnownSound unknown name: want false")
	}
	if _, ok := ResolveKnownSound(""); ok {
		t.Error("ResolveKnownSound empty: want false")
	}
	if _, ok := ResolveKnownSound("SOUND."); ok {
		t.Error("ResolveKnownSound bare prefix: want false")
	}
	if _, ok := ResolveKnownSound(nil); ok {
		t.Error("ResolveKnownSound nil: want false")
	}
	if _, ok := ResolveKnownSound(2396.5); ok {
		t.Error("ResolveKnownSound float: want false")
	}
	// Aliases resolve by name to the shared value.
	if s, ok := ResolveKnownSound("IMP_ATTACKING"); !ok || s != SoundNpcAttacking {
		t.Errorf("ResolveKnownSound alias = %v/%v", s, ok)
	}
}

func TestSendSoundToPlayer(t *testing.T) {
	p, sess := soundTestPlayer("alice", Location{X: 3200, Y: 3200})
	SendSound(p, SoundLevelUp)
	if len(sess.effects) != 1 {
		t.Fatalf("effects = %d, want 1", len(sess.effects))
	}
	want := soundEffectCall{2396, 0, 0, 1}
	if sess.effects[0] != want {
		t.Errorf("effect = %+v, want %+v", sess.effects[0], want)
	}
}

func TestSendSoundSkipsBots(t *testing.T) {
	p, sess := soundTestPlayer("bot", Location{X: 3200, Y: 3200})
	p.IsBot = true
	SendSound(p, SoundLevelUp)
	if len(sess.effects) != 0 {
		t.Errorf("bot heard %d effects, want 0", len(sess.effects))
	}
	// The direct static has no guard, exactly as in TS.
	SendSoundEffect(p, 2396, 0, 0, 1)
	if len(sess.effects) != 1 {
		t.Errorf("direct send to bot = %d, want 1", len(sess.effects))
	}
}

func TestSendSoundNilAndSessionless(t *testing.T) {
	// None of these panic or send.
	SendSound(nil, SoundLevelUp)
	var nilPlayer *Player
	SendSound(nilPlayer, SoundLevelUp)
	p := NewPlayer("nobody", Location{X: 3200, Y: 3200})
	SendSound(p, SoundLevelUp)
	SendSoundEffect(nil, 1, 0, 0, 1)
	SendSoundEffect(p, 1, 0, 0, 1)
	// A session without the SoundSender extension drops.
	p.Session = newFakeSession()
	SendSound(p, SoundLevelUp)
	SendSoundEffect(p, 1, 0, 0, 1)
}

func TestSendSoundNpcBecomesAreaSound(t *testing.T) {
	npc := NewNPC(1, Location{X: 3200, Y: 3200})
	hearer, hearerSess := soundTestPlayer("hearer", Location{X: 3202, Y: 3200})
	old := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player { return []*Player{hearer} })
	defer SetNetworkPlayerSource(old)
	SendSound(npc, SoundNpcAttacking)
	if len(hearerSess.areas) != 1 {
		t.Fatalf("area sends = %d, want 1", len(hearerSess.areas))
	}
	want := areaSoundCall{394, 3200, 3200, 0, 0, 0, 0, 0}
	if hearerSess.areas[0] != want {
		t.Errorf("area send = %+v, want %+v", hearerSess.areas[0], want)
	}
}

func TestPlayAreaSoundGates(t *testing.T) {
	near, nearSess := soundTestPlayer("near", Location{X: 3205, Y: 3200})
	far, farSess := soundTestPlayer("far", Location{X: 3220, Y: 3200})
	otherLevel, otherSess := soundTestPlayer("up", Location{X: 3205, Y: 3200, Z: 1})
	bot, botSess := soundTestPlayer("bot", Location{X: 3205, Y: 3200})
	bot.IsBot = true
	excluded, exclSess := soundTestPlayer("excl", Location{X: 3205, Y: 3200})
	old := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player { return []*Player{near, far, otherLevel, bot, excluded} })
	defer SetNetworkPlayerSource(old)

	PlayAreaSound(PlayAreaSoundOptions{
		SoundID: 2907, X: 3200, Y: 3200, Level: 0, Loops: 1, Delay: 3, Radius: 5, Exclude: excluded,
	})
	if len(nearSess.areas) != 1 {
		t.Errorf("near heard %d, want 1", len(nearSess.areas))
	} else if nearSess.areas[0] != (areaSoundCall{2907, 3200, 3200, 0, 1, 3, 5, 0}) {
		t.Errorf("near send = %+v", nearSess.areas[0])
	}
	if len(farSess.areas) != 0 {
		t.Errorf("far (20 tiles, broadcast radius 15) heard %d, want 0", len(farSess.areas))
	}
	if len(otherSess.areas) != 0 {
		t.Errorf("other level heard %d, want 0", len(otherSess.areas))
	}
	if len(botSess.areas) != 0 {
		t.Errorf("bot heard %d, want 0", len(botSess.areas))
	}
	if len(exclSess.areas) != 0 {
		t.Errorf("excluded heard %d, want 0", len(exclSess.areas))
	}
	// A radius of 20 widens the broadcast to 21: far hears it.
	PlayAreaSound(PlayAreaSoundOptions{SoundID: 2907, X: 3200, Y: 3200, Radius: 20})
	if len(farSess.areas) != 1 {
		t.Errorf("far with radius 20 heard %d, want 1", len(farSess.areas))
	}
	// Radius clamps to 31 in the send.
	PlayAreaSound(PlayAreaSoundOptions{SoundID: 2907, X: 3200, Y: 3200, Radius: 99})
	if nearSess.areas[len(nearSess.areas)-1].radius != 31 {
		t.Errorf("clamped radius = %d, want 31", nearSess.areas[len(nearSess.areas)-1].radius)
	}
	// Sound id 0 sends nothing.
	before := len(nearSess.areas)
	PlayAreaSound(PlayAreaSoundOptions{SoundID: 0, X: 3200, Y: 3200})
	if len(nearSess.areas) != before {
		t.Error("sound id 0 sent")
	}
	// No source installed: nothing sends.
	SetNetworkPlayerSource(nil)
	PlayAreaSound(PlayAreaSoundOptions{SoundID: 2907, X: 3200, Y: 3200})
	if len(nearSess.areas) != before {
		t.Error("unwired source sent")
	}
}

func TestPlayAreaSoundAreaMatching(t *testing.T) {
	areaA := NewArea("a", nil)
	areaB := NewArea("b", nil)
	inA, sessA := soundTestPlayer("inA", Location{X: 3200, Y: 3200})
	inA.SetArea(areaA)
	inB, sessB := soundTestPlayer("inB", Location{X: 3200, Y: 3200})
	inB.SetArea(areaB)
	mainWorld, sessMain := soundTestPlayer("main", Location{X: 3200, Y: 3200})
	source, _ := soundTestPlayer("source", Location{X: 3200, Y: 3200})
	source.SetArea(areaA)
	old := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player { return []*Player{inA, inB, mainWorld} })
	defer SetNetworkPlayerSource(old)

	// Source in area A: only A members hear it.
	PlayAreaSound(PlayAreaSoundOptions{SoundID: 100, X: 3200, Y: 3200, Source: source})
	if len(sessA.areas) != 1 || len(sessB.areas) != 0 || len(sessMain.areas) != 0 {
		t.Errorf("area A sound heard A=%d B=%d main=%d, want 1/0/0", len(sessA.areas), len(sessB.areas), len(sessMain.areas))
	}
	// Source in no area (nil == nil): only main-world players hear it.
	PlayAreaSound(PlayAreaSoundOptions{SoundID: 100, X: 3200, Y: 3200, Source: mainWorld})
	if len(sessA.areas) != 1 || len(sessB.areas) != 0 || len(sessMain.areas) != 1 {
		t.Errorf("main-world sound heard A=%d B=%d main=%d, want 1/0/1", len(sessA.areas), len(sessB.areas), len(sessMain.areas))
	}
	// No source: no area filtering; everyone hears it.
	PlayAreaSound(PlayAreaSoundOptions{SoundID: 100, X: 3200, Y: 3200})
	if len(sessA.areas) != 2 || len(sessB.areas) != 1 || len(sessMain.areas) != 2 {
		t.Errorf("sourceless sound heard A=%d B=%d main=%d, want 2/1/2", len(sessA.areas), len(sessB.areas), len(sessMain.areas))
	}
}

func TestPlayAreaSoundAt(t *testing.T) {
	npc := NewNPC(1, Location{X: 3200, Y: 3200, Z: 1})
	hearer, sess := soundTestPlayer("hearer", Location{X: 3201, Y: 3200, Z: 1})
	old := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player { return []*Player{hearer} })
	defer SetNetworkPlayerSource(old)
	PlayAreaSoundAt(npc, SoundVengeance, 10)
	if len(sess.areas) != 1 {
		t.Fatalf("areas = %d, want 1", len(sess.areas))
	}
	want := areaSoundCall{2907, 3200, 3200, 1, 0, 0, 10, 0}
	if sess.areas[0] != want {
		t.Errorf("area send = %+v, want %+v", sess.areas[0], want)
	}
	// No location, no send, no panic.
	PlayAreaSoundAt(nil, SoundVengeance, 5)
	PlayAreaSoundAt("not-an-entity", SoundVengeance, 5)
	var nilNPC *NPC
	PlayAreaSoundAt(nilNPC, SoundVengeance, 5)
	if len(sess.areas) != 1 {
		t.Errorf("nil/entity-less sends leaked: %d", len(sess.areas))
	}
}

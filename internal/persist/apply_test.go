package persist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// applyTestSave builds a save with a distinctive value in every field,
// for preservation tests. It is deliberately NOT normalized: nil vs
// empty distinctions matter to the clone tests.
func applyTestSave() *PlayerSave {
	lastVenom := 6
	protection := 12
	briefPoison := 1.5
	briefVenom := 2.5
	yellTag := "Tester"
	return &PlayerSave{
		DisplayName:              "Apply Test",
		CreationDate:             1700000000000,
		PasswordHashWithSalt:     "salt:hash",
		IsDiscordLogin:           true,
		CachedDiscordAccessToken: "token-123",
		Rights:                   Rights{ID: 1, SpriteID: 618, YellTag: &yellTag},
		DonatorRights:            DonatorRights{ID: 1, SpriteID: 622, YellDelay: 40, YellTag: "[Donator]"},
		Position:                 &Location{X: 3089, Y: 3524, Z: 0},
		SpellBook:                SpellBook{InterfaceID: 12855, TeleportType: json.RawMessage(`{"custom":true}`)},
		FightType:                json.RawMessage(`{"animation":422,"parentId":43}`),
		AutocastSpellId:          42,
		AutoRetaliate:            false,
		AudioSettings:            map[int]int{18: 3, 19: 1},
		HasVengeance:             true,
		LastVengeanceTimer:       9,
		SpecPercentage:           55,
		PoisonDamage:             8,
		Venomed:                  true,
		LastVenomDamage:          &lastVenom,
		PoisonImmunityTimer:      30,
		PoisonClock:              &PoisonClockState{Counter: 2, TicksUntilPulse: 7, ProtectionTicksRemaining: &protection, BriefPoisonSeconds: &briefPoison, BriefVenomSeconds: &briefVenom},
		FireImmunityTimer:        45,
		TeleblockTimer:           23,
		SkullTimer:               100,
		SkullType:                SkullType{IconID: 1},
		Running:                  true,
		RunEnergy:                77.5,
		Inventory:                []Item{{ID: 995, Amount: 100, Meta: map[string]json.RawMessage{"k": json.RawMessage(`1`)}}, EmptyItem()},
		Equipment:                []Item{{ID: 1163, Amount: 1}},
		Appearance:               []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0},
		Skills:                   Skills{Level: []int{10, 20}, MaxLevel: []int{11, 21}, Experience: []int{100, 200}},
		QuickPrayers:             []*QuickPrayer{{Requirement: 1, ButtonID: 5609, ConfigID: 4104, DrainEffect: 1, Hint: -1, PrayerID: 0, Name: "Thick Skin"}, nil},
		Friends:                  []string{"12345"},
		Ignores:                  []string{"67890"},
		FriendRanks:              map[string]int{"12345": 3},
		FriendsChatChannelName:   "channel",
		FriendsChatLastOwner:     "owner",
		FriendsChatEntryRank:     1,
		FriendsChatTalkRank:      2,
		FriendsChatKickRank:      3,
		Banks:                    Banks{0: {{ID: 995, Amount: 50, Meta: map[string]json.RawMessage{"m": json.RawMessage(`"v"`)}}}},
		Flags:                    []string{"flag-a"},
		Sailing:                  json.RawMessage(`{"boats":[],"custom":1}`),
		Attributes: map[string]json.RawMessage{
			"shop:points":              json.RawMessage(`5`),
			"login:tokens":             json.RawMessage(`[]`),
			"ring-of-recoil:damage":    json.RawMessage(`0`),
			"bank:note-withdrawal":     json.RawMessage(`false`),
			"combat:style-by-category": json.RawMessage(`{}`),
		},
		Unmade:  false,
		Profile: ProfileLeague,
		Extra: map[string]json.RawMessage{
			"barrowsCrypt": json.RawMessage(`3`),
			"futureField":  json.RawMessage(`{"x":[1,2]}`),
		},
	}
}

func mustMarshalSave(t *testing.T, save *PlayerSave) []byte {
	t.Helper()
	data, err := json.Marshal(save)
	if err != nil {
		t.Fatalf("marshal save: %v", err)
	}
	return data
}

func TestOwnedState(t *testing.T) {
	save := applyTestSave()
	state := save.OwnedState()
	// RunEnergy 77.5 -> 7750 units (setRunEnergy rule).
	// SpecPercentage 55 is owned since phase 16; AutocastSpellId
	// 42 is owned since phase 27 (the live combat autocast
	// spell id, or -1 when none is set).
	// Relations fields are owned since phase 19: the lists and
	// ranks extract as clones, the channel scalars as pointers
	// to fresh copies (DeepEqual compares pointed-to values).
	// Sailing is owned since phase 21: the raw JSON extracts
	// as a clone.
	channelName, lastOwner := "channel", "owner"
	entryRank, talkRank, kickRank := 1, 2, 3
	want := OwnedState{
		Position:               Location{X: 3089, Y: 3524, Z: 0},
		Running:                true,
		RunEnergyUnits:         7750,
		SpecPercentage:         55,
		AutocastSpellID:        42,
		Inventory:              save.Inventory,
		Equipment:              save.Equipment,
		Appearance:             save.Appearance,
		Skills:                 save.Skills,
		Banks:                  save.Banks,
		Attributes:             save.Attributes,
		Friends:                save.Friends,
		Ignores:                save.Ignores,
		FriendRanks:            save.FriendRanks,
		FriendsChatChannelName: &channelName,
		FriendsChatLastOwner:   &lastOwner,
		FriendsChatEntryRank:   &entryRank,
		FriendsChatTalkRank:    &talkRank,
		FriendsChatKickRank:    &kickRank,
		Sailing:                save.Sailing,
	}
	if !reflect.DeepEqual(state, want) {
		t.Fatalf("OwnedState = %+v, want %+v", state, want)
	}
	// Mutating the extracted state must not affect the save.
	state.Position.X = 1
	if save.Position.X != 3089 {
		t.Fatal("OwnedState must copy the position")
	}
	state.Sailing[0] = 'x'
	if save.Sailing[0] != '{' {
		t.Fatal("OwnedState must clone the sailing JSON")
	}
}

func TestOwnedStateNilPosition(t *testing.T) {
	// Pinned behavior: a nil Position yields the zero Location.
	// Callers guarantee normalized saves; this documents the extraction
	// rather than silently substituting the spawn. A missing percent
	// (0) yields 0 units. Empty containers extract as empty, not nil:
	// cloneItems/cloneInts/cloneBanks preserve the nil/empty shape
	// of the source, and a zero PlayerSave has nil slices.
	save := &PlayerSave{Running: false}
	state := save.OwnedState()
	// The channel scalars always extract as pointers, even for
	// a zero save (pointing at "" and 0); the lists stay nil.
	channelName, lastOwner := "", ""
	entryRank, talkRank, kickRank := 0, 0, 0
	want := OwnedState{
		Position: Location{}, Running: false, RunEnergyUnits: 0,
		AutocastSpellID:        0,
		FriendsChatChannelName: &channelName,
		FriendsChatLastOwner:   &lastOwner,
		FriendsChatEntryRank:   &entryRank,
		FriendsChatTalkRank:    &talkRank,
		FriendsChatKickRank:    &kickRank,
	}
	if !reflect.DeepEqual(state, want) {
		t.Fatalf("OwnedState with nil position = %+v, want %+v", state, want)
	}
}

func TestOwnedStateRunEnergyConversion(t *testing.T) {
	// setRunEnergy rule (Player.ts:1261-1274): floor(percent*100 +
	// 1e-6), clamped 0..10000.
	cases := []struct {
		percent float64
		want    int
	}{
		{0, 0}, {100, 10000}, {77.5, 7750}, {0.29, 29}, {0.01, 1},
		{99.99, 9999}, {100.5, 10000}, {1000, 10000}, {-5, 0},
		{50.005, 5000}, // floor, not round: 5000.5 + hair floors to 5000
	}
	for _, c := range cases {
		save := &PlayerSave{RunEnergy: c.percent}
		if got := save.OwnedState().RunEnergyUnits; got != c.want {
			t.Errorf("OwnedState().RunEnergyUnits for percent %v = %d, want %d", c.percent, got, c.want)
		}
	}
}

func TestAgilityIndex(t *testing.T) {
	// Declaration order in Skill.ts: ATTACK=0..AGILITY=16; the index
	// must be a valid slot in the 24-skill arrays.
	if AgilityIndex != 16 {
		t.Fatalf("AgilityIndex = %d, want 16", AgilityIndex)
	}
	if AgilityIndex >= SkillCount {
		t.Fatalf("AgilityIndex %d out of range for SkillCount %d", AgilityIndex, SkillCount)
	}
	skills := DefaultSkills()
	if len(skills.Level) != SkillCount || skills.Level[AgilityIndex] != 1 {
		t.Fatalf("DefaultSkills agility slot = %v, want level 1 at index 16", skills.Level)
	}
}

func TestWithOwned(t *testing.T) {
	base := applyTestSave()
	before := mustMarshalSave(t, base)
	moved := OwnedState{Position: Location{X: 3200, Y: 3200, Z: 1}, Running: false, RunEnergyUnits: 5000, SpecPercentage: 40, AutocastSpellID: 1152}

	clone := base.WithOwned(moved)

	// Owned fields overwritten; teleblock zeroed.
	if clone.Position == nil || *clone.Position != moved.Position {
		t.Fatalf("clone position = %+v, want %+v", clone.Position, moved.Position)
	}
	if clone.Running != false {
		t.Fatal("clone running must be false")
	}
	if clone.RunEnergy != 50 {
		t.Fatalf("clone runEnergy = %v, want 50 (5000 units / 100)", clone.RunEnergy)
	}
	if clone.SpecPercentage != 40 {
		t.Fatalf("clone specPercentage = %d, want 40", clone.SpecPercentage)
	}
	if clone.AutocastSpellId != 1152 {
		t.Fatalf("clone autocastSpellId = %d, want 1152", clone.AutocastSpellId)
	}
	if clone.TeleblockTimer != 0 {
		t.Fatalf("clone teleblockTimer = %d, want 0", clone.TeleblockTimer)
	}
	// Default-valued attributes stripped, non-defaults kept.
	if _, ok := clone.Attributes["login:tokens"]; ok {
		t.Fatal("default login:tokens [] must be stripped")
	}
	if _, ok := clone.Attributes["ring-of-recoil:damage"]; ok {
		t.Fatal("default ring-of-recoil:damage 0 must be stripped")
	}
	if _, ok := clone.Attributes["bank:note-withdrawal"]; ok {
		t.Fatal("default bank:note-withdrawal false must be stripped")
	}
	if _, ok := clone.Attributes["combat:style-by-category"]; ok {
		t.Fatal("default combat:style-by-category {} must be stripped")
	}
	if string(clone.Attributes["shop:points"]) != "5" {
		t.Fatalf("non-default attribute lost: %v", clone.Attributes)
	}
	// Every unowned field preserved.
	if clone.DisplayName != base.DisplayName || clone.CreationDate != base.CreationDate ||
		clone.PasswordHashWithSalt != base.PasswordHashWithSalt || clone.IsDiscordLogin != base.IsDiscordLogin ||
		clone.CachedDiscordAccessToken != base.CachedDiscordAccessToken ||
		!reflect.DeepEqual(clone.Rights, base.Rights) || clone.DonatorRights != base.DonatorRights ||
		clone.SpellBook.InterfaceID != base.SpellBook.InterfaceID ||
		clone.AutoRetaliate != base.AutoRetaliate ||
		clone.HasVengeance != base.HasVengeance || clone.LastVengeanceTimer != base.LastVengeanceTimer ||
		clone.PoisonDamage != base.PoisonDamage ||
		clone.Venomed != base.Venomed || clone.PoisonImmunityTimer != base.PoisonImmunityTimer ||
		clone.FireImmunityTimer != base.FireImmunityTimer || clone.SkullTimer != base.SkullTimer ||
		clone.SkullType != base.SkullType ||
		clone.FriendsChatChannelName != base.FriendsChatChannelName ||
		clone.FriendsChatLastOwner != base.FriendsChatLastOwner ||
		clone.FriendsChatEntryRank != base.FriendsChatEntryRank ||
		clone.FriendsChatTalkRank != base.FriendsChatTalkRank ||
		clone.FriendsChatKickRank != base.FriendsChatKickRank ||
		clone.Unmade != base.Unmade || clone.Profile != base.Profile {
		t.Fatal("unowned scalar field changed by WithOwned")
	}
	if string(clone.SpellBook.TeleportType) != string(base.SpellBook.TeleportType) ||
		string(clone.FightType) != string(base.FightType) ||
		string(clone.Sailing) != string(base.Sailing) {
		t.Fatal("RawMessage field changed by WithOwned")
	}
	if !reflect.DeepEqual(clone.AudioSettings, base.AudioSettings) ||
		!reflect.DeepEqual(clone.Inventory, base.Inventory) ||
		!reflect.DeepEqual(clone.Equipment, base.Equipment) ||
		!reflect.DeepEqual(clone.Appearance, base.Appearance) ||
		!reflect.DeepEqual(clone.Skills, base.Skills) ||
		!reflect.DeepEqual(clone.QuickPrayers, base.QuickPrayers) ||
		!reflect.DeepEqual(clone.Friends, base.Friends) ||
		!reflect.DeepEqual(clone.Ignores, base.Ignores) ||
		!reflect.DeepEqual(clone.FriendRanks, base.FriendRanks) ||
		!reflect.DeepEqual(clone.Banks, base.Banks) ||
		!reflect.DeepEqual(clone.Flags, base.Flags) ||
		!reflect.DeepEqual(clone.Extra, base.Extra) {
		t.Fatal("unowned collection field changed by WithOwned")
	}
	if clone.LastVenomDamage == nil || *clone.LastVenomDamage != 6 ||
		!reflect.DeepEqual(clone.PoisonClock, base.PoisonClock) {
		t.Fatal("pointer field changed by WithOwned")
	}
	// Receiver unmutated, byte for byte.
	if after := mustMarshalSave(t, base); string(after) != string(before) {
		t.Fatalf("receiver mutated by WithOwned:\nbefore %s\nafter  %s", before, after)
	}
}

func TestWithOwnedAutocastClears(t *testing.T) {
	// Phase 27: the live autocast spell id is owned state, so a
	// snapshot with no live spell (-1) overwrites a carried pick
	// (fromPlayer writes ?? -1, PlayerSave.ts:756) — the clear
	// path, mirroring the set path in TestWithOwned.
	base := applyTestSave()
	base.AutocastSpellId = 1152
	clone := base.WithOwned(OwnedState{
		Position: *base.Position, Running: base.Running, RunEnergyUnits: 7750,
		AutocastSpellID: -1,
	})
	if clone.AutocastSpellId != -1 {
		t.Fatalf("clone autocastSpellId = %d, want -1 (live clear overwrites the carried pick)", clone.AutocastSpellId)
	}
	if base.AutocastSpellId != 1152 {
		t.Fatal("receiver mutated by WithOwned")
	}
}

func TestWithOwnedFightType(t *testing.T) {
	// Phase 30: the live fight type is owned state (fromPlayer
	// writes player.getFightType(), PlayerSave.ts:754), so a
	// snapshot overwrites the carried fight type — and a nil
	// FightType preserves it, like Sailing.
	base := applyTestSave()
	lash := json.RawMessage(`{"animation":1658,"parentId":43,"childId":1,"bonusType":1,"style":{},"attackSound":{"id":2720,"volume":1,"delay":0,"loopType":0},"rapid":false,"npcAnimation":-1}`)
	clone := base.WithOwned(OwnedState{
		Position: *base.Position, Running: base.Running, RunEnergyUnits: 7750,
		FightType: lash,
	})
	if string(clone.FightType) != string(lash) {
		t.Fatalf("clone fightType = %s, want %s", clone.FightType, lash)
	}
	if string(base.FightType) != `{"animation":422,"parentId":43}` {
		t.Fatal("receiver mutated by WithOwned")
	}
	// The clone owns its bytes: mutating them leaves the
	// supplied value alone.
	clone.FightType[0] = 'X'
	if lash[0] == 'X' {
		t.Fatal("WithOwned aliased the supplied fightType bytes")
	}
	// Nil preserves the carried value.
	preserved := base.WithOwned(OwnedState{
		Position: *base.Position, Running: base.Running, RunEnergyUnits: 7750,
	})
	if string(preserved.FightType) != `{"animation":422,"parentId":43}` {
		t.Fatalf("nil fightType preserved = %s, want %s", preserved.FightType, `{"animation":422,"parentId":43}`)
	}
}

func TestWithOwnedCloneIndependence(t *testing.T) {
	base := applyTestSave()
	clone := base.WithOwned(OwnedState{Position: Location{X: 1, Y: 2, Z: 0}, Running: true})

	// Mutate every mutable corner of the clone.
	clone.Position.X = 999
	clone.Appearance[1] = 99
	clone.Skills.Level[0] = 99
	clone.Inventory[0].ID = 99
	clone.Inventory[0].Meta["k"] = json.RawMessage(`99`)
	clone.Banks[0][0].ID = 99
	clone.Banks[0][0].Meta["m"] = json.RawMessage(`99`)
	clone.Attributes["shop:points"] = json.RawMessage(`99`)
	clone.Attributes["new-key"] = json.RawMessage(`1`)
	clone.Extra["barrowsCrypt"] = json.RawMessage(`99`)
	clone.FightType[0] = 'X'
	clone.Sailing[0] = 'X'
	clone.SpellBook.TeleportType[0] = 'X'
	clone.AudioSettings[18] = 99
	clone.FriendRanks["12345"] = 99
	clone.Friends[0] = "999"
	clone.Flags[0] = "mutated"
	clone.QuickPrayers[0].Requirement = 99
	*clone.LastVenomDamage = 99
	*clone.PoisonClock.ProtectionTicksRemaining = 99
	*clone.Rights.YellTag = "mutated"

	fresh := applyTestSave()
	if !reflect.DeepEqual(base, fresh) {
		t.Fatalf("mutating the clone affected the base:\nbase %+v\nwant %+v", base, fresh)
	}
}

func TestWithOwnedNilFieldsPreserved(t *testing.T) {
	base := &PlayerSave{DisplayName: "Nil Fields", TeleblockTimer: 5}
	clone := base.WithOwned(OwnedState{Position: Location{X: 5, Y: 6, Z: 0}, Running: true})
	if clone.Position == nil || *clone.Position != (Location{X: 5, Y: 6, Z: 0}) {
		t.Fatal("WithOwned must set the position even when the base has none")
	}
	if clone.Inventory != nil || clone.Attributes != nil || clone.Extra != nil ||
		clone.Appearance != nil || clone.FightType != nil || clone.Sailing != nil {
		t.Fatal("WithOwned must preserve nil reference fields as nil")
	}
	if clone.TeleblockTimer != 0 {
		t.Fatal("teleblock must be zeroed")
	}
}

func TestWithOwnedKeepsNonDefaultTokens(t *testing.T) {
	base := applyTestSave()
	base.Attributes["login:tokens"] = json.RawMessage(`[{"hash":"abc"}]`)
	clone := base.WithOwned(OwnedState{Position: Location{X: 1, Y: 1, Z: 0}, Running: true})
	if string(clone.Attributes["login:tokens"]) != `[{"hash":"abc"}]` {
		t.Fatal("non-default login:tokens must be kept")
	}
}

func TestWithOwnedRunEnergyRoundTrip(t *testing.T) {
	// Energy persists across WithOwned + entry application
	// (OwnedState): percent -> units -> percent, with preserved
	// fields — including unknown keys — byte-identical apart from
	// the owned fields.
	base := applyTestSave()
	base.RunEnergy = 0.29
	base.TeleblockTimer = 0 // WithOwned zeroes it; neutralize so the diff is energy-only.
	base.Attributes = map[string]json.RawMessage{"shop:points": json.RawMessage(`5`)}
	before := mustMarshalSave(t, base)

	// Entry application derives units; the snapshot writes them back.
	// The state is the base's own extraction, so every owned field
	// round-trips (including the autocast spell id, owned since
	// phase 27); only the energy is re-derived.
	units := base.OwnedState().RunEnergyUnits
	if units != 29 {
		t.Fatalf("units = %d, want 29", units)
	}
	state := base.OwnedState()
	state.RunEnergyUnits = units
	clone := base.WithOwned(state)
	if clone.RunEnergy != 0.29 {
		t.Fatalf("round-tripped runEnergy = %v, want 0.29", clone.RunEnergy)
	}
	// Everything preserved: the clone marshals byte-identically to
	// the base when the owned state is unchanged.
	if after := mustMarshalSave(t, clone); string(after) != string(before) {
		t.Fatalf("preserved fields changed:\nbefore %s\nafter  %s", before, after)
	}
	// A changed live energy overwrites only runEnergy: compare
	// against a base with the new percent substituted. The state
	// is the base's own extraction (see above), so the autocast
	// spell id and every other owned field round-trip.
	drainedState := base.OwnedState()
	drainedState.RunEnergyUnits = 1234
	drained := base.WithOwned(drainedState)
	if drained.RunEnergy != 12.34 {
		t.Fatalf("drained runEnergy = %v, want 12.34", drained.RunEnergy)
	}
	want := applyTestSave()
	want.RunEnergy = 12.34
	want.TeleblockTimer = 0
	want.Attributes = map[string]json.RawMessage{"shop:points": json.RawMessage(`5`)}
	if got, wantJSON := mustMarshalSave(t, drained), mustMarshalSave(t, want); string(got) != string(wantJSON) {
		t.Fatalf("drained snapshot differs beyond runEnergy:\n got %s\nwant %s", got, wantJSON)
	}
	// Unknown keys survive byte-identically through the energy
	// overwrite.
	if string(drained.Extra["barrowsCrypt"]) != `3` || string(drained.Extra["futureField"]) != `{"x":[1,2]}` {
		t.Fatalf("unknown keys changed: %v", drained.Extra)
	}
}

func TestWithOwnedContainersSkillsAppearanceBanks(t *testing.T) {
	// Phase 9: the live containers, skills, appearance, and banks
	// are owned — WithOwned overwrites them from live state
	// (fromPlayer, PlayerSave.ts:783-851) while every other field
	// stays preserved.
	base := applyTestSave()
	before := mustMarshalSave(t, base)
	state := OwnedState{
		Position:       *base.Position,
		Running:        base.Running,
		RunEnergyUnits: 7750,
		Inventory:      []Item{{ID: 4151, Amount: 1}},
		Equipment:      []Item{{ID: 1163, Amount: 1}, {ID: -1, Amount: 0}},
		Appearance:     []int{1, 48, 57, 65, 68, 77, 80, -1, 2, 14, 5, 4, 0},
		Skills:         Skills{Level: []int{99}, MaxLevel: []int{99}, Experience: []int{13034431}},
		Banks:          Banks{0: {{ID: 995, Amount: 7}}, 2: {}},
	}
	clone := base.WithOwned(state)
	if !reflect.DeepEqual(clone.Inventory, state.Inventory) {
		t.Fatalf("clone inventory = %+v, want %+v", clone.Inventory, state.Inventory)
	}
	if !reflect.DeepEqual(clone.Equipment, state.Equipment) {
		t.Fatalf("clone equipment = %+v, want %+v", clone.Equipment, state.Equipment)
	}
	if !reflect.DeepEqual(clone.Appearance, SanitizeAppearance(state.Appearance)) {
		t.Fatalf("clone appearance = %v, want sanitized %v", clone.Appearance, SanitizeAppearance(state.Appearance))
	}
	if !reflect.DeepEqual(clone.Skills, state.Skills) {
		t.Fatalf("clone skills = %+v, want %+v", clone.Skills, state.Skills)
	}
	if !reflect.DeepEqual(clone.Banks, state.Banks) {
		t.Fatalf("clone banks = %+v, want %+v", clone.Banks, state.Banks)
	}
	// The base is untouched and the clone shares no mutable state
	// with the supplied state: mutating the state afterwards
	// leaves the clone alone.
	if after := mustMarshalSave(t, base); string(after) != string(before) {
		t.Fatalf("base mutated: %s", after)
	}
	state.Inventory[0].ID = 1
	state.Skills.Level[0] = 1
	state.Banks[0][0].Amount = 1
	if clone.Inventory[0].ID != 4151 || clone.Skills.Level[0] != 99 || clone.Banks[0][0].Amount != 7 {
		t.Fatalf("clone aliases supplied state: %+v %+v %+v", clone.Inventory, clone.Skills, clone.Banks)
	}
	// A partial OwnedState (new fields nil) preserves the carried
	// containers/skills/appearance/banks.
	partial := base.WithOwned(OwnedState{Position: *base.Position, Running: base.Running, RunEnergyUnits: 7750})
	if !reflect.DeepEqual(partial.Inventory, base.Inventory) || !reflect.DeepEqual(partial.Equipment, base.Equipment) ||
		!reflect.DeepEqual(partial.Appearance, base.Appearance) || !reflect.DeepEqual(partial.Skills, base.Skills) ||
		!reflect.DeepEqual(partial.Banks, base.Banks) {
		t.Fatal("partial OwnedState must preserve carried containers/skills/appearance/banks")
	}
}

func TestOwnedStateCopiesContainers(t *testing.T) {
	// OwnedState extraction copies the container/skill/bank
	// slices: loading live state from them must never alias the
	// carried save.
	base := applyTestSave()
	state := base.OwnedState()
	state.Inventory[0].ID = 1
	state.Equipment[0].ID = 1
	state.Appearance[0] = 1
	state.Skills.Level[0] = 1
	state.Banks[0][0].Amount = 1
	if base.Inventory[0].ID != 995 || base.Equipment[0].ID != 1163 || base.Appearance[0] != 0 ||
		base.Skills.Level[0] != 10 || base.Banks[0][0].Amount != 50 {
		t.Fatal("OwnedState aliases the save")
	}
}

func TestSanitizeAppearance(t *testing.T) {
	male := []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0}
	female := []int{1, 48, 57, 65, 68, 77, 80, -1, 2, 14, 5, 4, 0}
	cases := []struct {
		name string
		in   []int
		want []int
	}{
		{"nil input is the male default", nil, male},
		{"empty input is the male default", []int{}, male},
		{"male default unchanged", male, male},
		{"female default unchanged", female, female},
		{"gender only, female", []int{1}, female},
		{"gender only, male", []int{0}, male},
		{"gender coercion: 2 is male", []int{2, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0}, male},
		{"gender coercion: -1 is male", []int{-1}, male},
		{"short input fills defaults", []int{0, 3}, male},
		{"long input ignores extras", append(append([]int(nil), male...), 99, 99), male},
		{"head clamped high", []int{0, 99, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0}, []int{0, 8, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0}},
		{"head clamped low", []int{0, -5, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0}, []int{0, 0, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0}},
		{"beard clamped high", []int{0, 3, 18, 26, 34, 38, 42, 99, 2, 14, 5, 4, 0}, []int{0, 3, 18, 26, 34, 38, 42, 17, 2, 14, 5, 4, 0}},
		{"minus-one jaw kept (male)", []int{0, 3, 18, 26, 34, 38, 42, -1, 2, 14, 5, 4, 0}, []int{0, 3, 18, 26, 34, 38, 42, -1, 2, 14, 5, 4, 0}},
		{"female beard clamps to -1", []int{1, 48, 57, 65, 68, 77, 80, 5, 2, 14, 5, 4, 0}, female},
		{"female head clamped", []int{1, 99, 57, 65, 68, 77, 80, -1, 2, 14, 5, 4, 0}, []int{1, 54, 57, 65, 68, 77, 80, -1, 2, 14, 5, 4, 0}},
		{"female head clamped low", []int{1, 0, 57, 65, 68, 77, 80, -1, 2, 14, 5, 4, 0}, []int{1, 45, 57, 65, 68, 77, 80, -1, 2, 14, 5, 4, 0}},
		{"unknown skin 13 resets to default", []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 13}, male},
		{"unknown skin -1 resets to default", []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, -1}, male},
		{"skin 12 kept", []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 12}, []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 12}},
		{"hair colour clamped", []int{0, 3, 18, 26, 34, 38, 42, 14, 99, 14, 5, 4, 0}, []int{0, 3, 18, 26, 34, 38, 42, 14, 29, 14, 5, 4, 0}},
		{"torso colour clamped", []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 99, 5, 4, 0}, []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 28, 5, 4, 0}},
		{"leg colour clamped low", []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, -3, 4, 0}, []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 0, 4, 0}},
		{"feet colour clamped", []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 99, 0}, []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 5, 0}},
		{"in-range custom values kept", []int{0, 5, 20, 28, 33, 37, 43, 12, 10, 20, 15, 3, 7}, []int{0, 5, 20, 28, 33, 37, 43, 12, 10, 20, 15, 3, 7}},
		// Clamp-only fallback pin (isSelectableKit GAP): a kit outside
		// the legacy range is clamped even if the cache might offer it.
		{"out-of-range kit clamped, no cache check", []int{0, 30, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0}, []int{0, 8, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeAppearance(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("SanitizeAppearance(%v) = %v, want %v", tc.in, got, tc.want)
			}
			if len(got) != 13 {
				t.Fatalf("length = %d, want 13", len(got))
			}
		})
	}
	// The result never aliases the input.
	in := append([]int(nil), male...)
	got := SanitizeAppearance(in)
	got[1] = 99
	if in[1] != 3 {
		t.Fatal("SanitizeAppearance must not alias its input")
	}
}

func TestToKebabCase(t *testing.T) {
	cases := map[string]string{
		"bankNoteWithdrawal":     "bank-note-withdrawal",
		"xpLocked":               "xp-locked",
		"aBC":                    "a-bc",
		"ABC":                    "abc",
		"already-kebab":          "already-kebab",
		"x1Y":                    "x1-y",
		"combat:styleByCategory": "combat:style-by-category",
		"":                       "",
	}
	for in, want := range cases {
		if got := toKebabCase(in); got != want {
			t.Errorf("toKebabCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMigrateAttributeKey(t *testing.T) {
	// Already persistent: kept.
	if got := migrateAttributeKey("bank:note-withdrawal"); got != "bank:note-withdrawal" {
		t.Fatalf("persistent key renamed: %q", got)
	}
	// CamelCase with a persistent kebab form: renamed. The migration
	// handles namespaced keys: the colon survives kebab-casing, so
	// "ringOfRecoil:damage" reaches "ring-of-recoil:damage".
	if got := migrateAttributeKey("ringOfRecoil:damage"); got != "ring-of-recoil:damage" {
		t.Fatalf("ringOfRecoil:damage -> %q", got)
	}
	if got := migrateAttributeKey("killstreaks:totalKills"); got != "killstreaks:total-kills" {
		t.Fatalf("killstreaks:totalKills -> %q", got)
	}
	// A bare legacy FIELD name as an attribute key does NOT migrate:
	// its kebab form ("bank-note-withdrawal") lacks the colon, so it
	// is not a persistent key. TS behaves identically
	// (migrateAttributeKey only renames onto a persistent kebab form);
	// the field-to-attribute mapping is the legacy fold's job.
	if got := migrateAttributeKey("bankNoteWithdrawal"); got != "bankNoteWithdrawal" {
		t.Fatalf("bankNoteWithdrawal -> %q, want unchanged", got)
	}
	if got := migrateAttributeKey("xpLocked"); got != "xpLocked" {
		t.Fatalf("xpLocked -> %q, want unchanged", got)
	}
	// FromPlayer-registered key: the kebab form is persistent too.
	if got := migrateAttributeKey("combat:styleByCategory"); got != "combat:style-by-category" {
		t.Fatalf("combat:styleByCategory -> %q", got)
	}
	// Kebab form not persistent: kept as-is.
	if got := migrateAttributeKey("someCustomKey"); got != "someCustomKey" {
		t.Fatalf("non-persistent key renamed: %q", got)
	}
	if got := migrateAttributeKey("combatStyleByCategory"); got != "combatStyleByCategory" {
		t.Fatalf("combatStyleByCategory -> %q (kebab form lacks the colon)", got)
	}
	// All 22 legacy values are persistent keys (live registry).
	if len(legacyFieldAttributes) != 22 {
		t.Fatalf("legacy map has %d entries, want 22", len(legacyFieldAttributes))
	}
	for field, key := range legacyFieldAttributes {
		if !isPersistentAttributeKey(key) {
			t.Fatalf("legacy value %q (field %q) not in persistent set", key, field)
		}
	}
}

func TestMigrateForEntry(t *testing.T) {
	t.Run("legacy fold, attribute absent", func(t *testing.T) {
		save := &PlayerSave{
			Attributes: map[string]json.RawMessage{},
			Extra: map[string]json.RawMessage{
				"title":  json.RawMessage(`"Lord"`),
				"points": json.RawMessage(`7`),
			},
		}
		save.MigrateForEntry()
		if string(save.Attributes["loyalty:title"]) != `"Lord"` {
			t.Fatalf("title not folded: %v", save.Attributes)
		}
		if string(save.Attributes["shop:points"]) != `7` {
			t.Fatalf("points not folded: %v", save.Attributes)
		}
		if len(save.Extra) != 0 {
			t.Fatalf("migrated legacy keys must be dropped from Extra: %v", save.Extra)
		}
	})
	t.Run("attribute wins when present", func(t *testing.T) {
		save := &PlayerSave{
			Attributes: map[string]json.RawMessage{"shop:points": json.RawMessage(`99`)},
			Extra:      map[string]json.RawMessage{"points": json.RawMessage(`7`)},
		}
		save.MigrateForEntry()
		if string(save.Attributes["shop:points"]) != `99` {
			t.Fatal("present attribute must win over the legacy field")
		}
		if _, ok := save.Extra["points"]; ok {
			t.Fatal("consumed legacy key must be dropped from Extra")
		}
	})
	t.Run("null legacy value folds nothing", func(t *testing.T) {
		save := &PlayerSave{
			Attributes: map[string]json.RawMessage{},
			Extra:      map[string]json.RawMessage{"xpLocked": json.RawMessage(`null`)},
		}
		save.MigrateForEntry()
		if _, ok := save.Attributes["skills:xp-locked"]; ok {
			t.Fatal("null legacy value must fold nothing")
		}
		if _, ok := save.Extra["xpLocked"]; ok {
			t.Fatal("null legacy key must still be dropped from Extra")
		}
	})
	t.Run("nil attributes map allocated on fold", func(t *testing.T) {
		save := &PlayerSave{
			Extra: map[string]json.RawMessage{"deaths": json.RawMessage(`2`)},
		}
		save.MigrateForEntry()
		if string(save.Attributes["killstreaks:deaths"]) != `2` {
			t.Fatalf("fold into nil attributes failed: %v", save.Attributes)
		}
	})
	t.Run("camelCase renames, both forms present, kebab wins", func(t *testing.T) {
		save := &PlayerSave{
			Attributes: map[string]json.RawMessage{
				"ringOfRecoil:damage":    json.RawMessage(`5`),
				"ring-of-recoil:damage":  json.RawMessage(`9`),
				"killstreaks:totalKills": json.RawMessage(`3`),
				"someCustomKey":          json.RawMessage(`1`),
			},
		}
		save.MigrateForEntry()
		if string(save.Attributes["ring-of-recoil:damage"]) != `9` {
			t.Fatalf("kebab form must win: %v", save.Attributes)
		}
		if _, ok := save.Attributes["ringOfRecoil:damage"]; ok {
			t.Fatal("camelCase form must be dropped when kebab wins")
		}
		if string(save.Attributes["killstreaks:total-kills"]) != `3` {
			t.Fatalf("killstreaks:totalKills not renamed: %v", save.Attributes)
		}
		if string(save.Attributes["someCustomKey"]) != `1` {
			t.Fatal("non-persistent key must be untouched")
		}
	})
	t.Run("appearance sanitized in place", func(t *testing.T) {
		save := &PlayerSave{Appearance: []int{0, 99, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 13}}
		save.MigrateForEntry()
		want := []int{0, 8, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0}
		if !reflect.DeepEqual(save.Appearance, want) {
			t.Fatalf("appearance = %v, want %v", save.Appearance, want)
		}
	})
	t.Run("non-legacy Extra keys untouched", func(t *testing.T) {
		save := &PlayerSave{
			Attributes: map[string]json.RawMessage{},
			Extra: map[string]json.RawMessage{
				"barrowsCrypt": json.RawMessage(`3`),
				"title":        json.RawMessage(`"X"`),
			},
		}
		save.MigrateForEntry()
		if string(save.Extra["barrowsCrypt"]) != `3` {
			t.Fatalf("non-legacy Extra key changed: %v", save.Extra)
		}
		if len(save.Extra) != 1 {
			t.Fatalf("Extra = %v", save.Extra)
		}
	})
	t.Run("legacy fold sees migrated keys", func(t *testing.T) {
		// The attribute arrives in camelCase and migrates first, so
		// the legacy field must not overwrite it.
		save := &PlayerSave{
			Attributes: map[string]json.RawMessage{"crystalBow:shotsInStage": json.RawMessage(`4`)},
			Extra:      map[string]json.RawMessage{"crystalBowShotsInStage": json.RawMessage(`9`)},
		}
		save.MigrateForEntry()
		if string(save.Attributes["crystal-bow:shots-in-stage"]) != `4` {
			t.Fatalf("migrated attribute must win: %v", save.Attributes)
		}
	})
}

func TestUnmake(t *testing.T) {
	tokens := json.RawMessage(`[{"hash":"abc","lastUsed":1700000000000}]`)
	save := applyTestSave()
	save.Attributes = map[string]json.RawMessage{
		"login:tokens": tokens,
		"shop:points":  json.RawMessage(`5`),
		"custom:thing": json.RawMessage(`true`),
	}
	save.Inventory = []Item{{ID: 995, Amount: 100}, {ID: 1163, Amount: 1}, EmptyItem()}
	save.Equipment = nil
	save.Banks = Banks{0: {{ID: 995, Amount: 1}}, 2: {{ID: 1, Amount: 1}}}
	save.Skills = Skills{
		Level:      []int{99, 99, 99},
		MaxLevel:   []int{99, 99, 99},
		Experience: []int{13034431, 13034431, 13034431},
	}
	positionBefore := *save.Position

	save.Unmake()

	if !save.Unmade {
		t.Fatal("unmade must be true")
	}
	if len(save.Attributes) != 1 || string(save.Attributes[LoginTokensAttribute]) != string(tokens) {
		t.Fatalf("only login:tokens must survive: %v", save.Attributes)
	}
	if len(save.Inventory) != 3 {
		t.Fatalf("inventory length = %d, want 3", len(save.Inventory))
	}
	for i, item := range save.Inventory {
		if item.ID != -1 || item.Amount != 0 || item.Meta != nil {
			t.Fatalf("inventory[%d] = %+v, want empty item", i, item)
		}
	}
	if save.Equipment == nil || len(save.Equipment) != 0 {
		t.Fatalf("nil equipment must become an empty array, got %#v", save.Equipment)
	}
	if save.Banks == nil || len(save.Banks) != 0 {
		t.Fatalf("banks must be emptied, got %#v", save.Banks)
	}
	// Fresh Skills with level = maxLevel: 1s, hitpoints 10, hitpoints
	// experience 1154 (NOT DefaultSkills' 1184 — see Unmake).
	wantSkills := Skills{
		Level:      make([]int, SkillCount),
		MaxLevel:   make([]int, SkillCount),
		Experience: make([]int, SkillCount),
	}
	for i := range wantSkills.Level {
		wantSkills.Level[i] = 1
		wantSkills.MaxLevel[i] = 1
	}
	wantSkills.Level[HitpointsIndex] = 10
	wantSkills.MaxLevel[HitpointsIndex] = 10
	wantSkills.Experience[HitpointsIndex] = freshSkillsHitpointsXP
	if !reflect.DeepEqual(save.Skills, wantSkills) {
		t.Fatalf("skills = %+v, want %+v", save.Skills, wantSkills)
	}
	if *save.Position != positionBefore || save.DisplayName != "Apply Test" {
		t.Fatal("Unmake must not touch position or identity fields")
	}
}

func TestUnmakeWithoutTokens(t *testing.T) {
	save := &PlayerSave{Attributes: map[string]json.RawMessage{"shop:points": json.RawMessage(`1`)}}
	save.Unmake()
	if save.Attributes == nil || len(save.Attributes) != 0 {
		t.Fatalf("attributes must become empty, got %#v", save.Attributes)
	}
	saveNil := &PlayerSave{}
	saveNil.Unmake()
	if saveNil.Attributes == nil || len(saveNil.Attributes) != 0 {
		t.Fatalf("nil attributes must become empty, got %#v", saveNil.Attributes)
	}
	if saveNil.Inventory == nil || len(saveNil.Inventory) != 0 {
		t.Fatal("nil inventory must become an empty array")
	}
}

func TestNewCharacterSave(t *testing.T) {
	now := time.Date(2026, 11, 14, 16, 0, 0, 0, time.UTC)
	save := NewCharacterSave(NewCharacterParams{
		Username:     "New Player",
		PasswordHash: "salt:hash",
		Profile:      ProfileMain,
		Spawn:        Location{X: 3089, Y: 3524, Z: 0},
		Developer:    false,
		Now:          now,
	})
	if err := validateSaveForWrite(save); err != nil {
		t.Fatalf("new-character save must pass validation: %v", err)
	}
	if save.DisplayName != "New Player" || save.PasswordHashWithSalt != "salt:hash" {
		t.Fatal("username/password not set")
	}
	if save.CreationDate != now.UnixMilli() {
		t.Fatalf("creationDate = %d, want %d", save.CreationDate, now.UnixMilli())
	}
	if save.Profile != ProfileMain {
		t.Fatalf("profile = %q", save.Profile)
	}
	if save.IsDiscordLogin || save.CachedDiscordAccessToken != "" {
		t.Fatal("discord fields must be default")
	}
	if save.Rights.ID != 0 || save.Rights.SpriteID != -1 || save.Rights.YellTag == nil || save.Rights.YellTagString() != "" {
		t.Fatalf("rights = %+v, want NONE", save.Rights)
	}
	if save.DonatorRights != (DonatorRights{ID: 0, SpriteID: -1, YellDelay: -1, YellTag: ""}) {
		t.Fatalf("donatorRights = %+v, want NONE", save.DonatorRights)
	}
	if save.Position == nil || *save.Position != (Location{X: 3089, Y: 3524, Z: 0}) {
		t.Fatalf("position = %+v", save.Position)
	}
	if save.SpellBook.InterfaceID != SpellbookNormalInterfaceID {
		t.Fatalf("spellBook interface = %d", save.SpellBook.InterfaceID)
	}
	if string(save.SpellBook.TeleportType) != defaultTeleportTypeJSON {
		t.Fatalf("teleportType = %s", save.SpellBook.TeleportType)
	}
	if string(save.FightType) != defaultFightTypeJSON {
		t.Fatalf("fightType = %s", save.FightType)
	}
	if save.AutocastSpellId != -1 || !save.AutoRetaliate {
		t.Fatal("autocast/auto-retaliate defaults wrong")
	}
	wantAudio := map[int]int{18: 0, 19: 1, 168: 100, 169: 100, 872: 100, 3796: 100, 3109: 0, 3797: 0}
	if !reflect.DeepEqual(save.AudioSettings, wantAudio) {
		t.Fatalf("audioSettings = %v, want %v", save.AudioSettings, wantAudio)
	}
	if save.HasVengeance || save.LastVengeanceTimer != 0 || save.SpecPercentage != 100 ||
		save.PoisonDamage != 0 || save.Venomed || save.LastVenomDamage != nil ||
		save.PoisonImmunityTimer != 0 || save.PoisonClock != nil || save.FireImmunityTimer != 0 ||
		save.TeleblockTimer != 0 || save.SkullTimer != 0 || save.SkullType != (SkullType{IconID: 0}) {
		t.Fatal("combat/timer defaults wrong")
	}
	if !save.Running || save.RunEnergy != 100 {
		t.Fatal("running/runEnergy defaults wrong")
	}
	if len(save.Inventory) != InventorySize || len(save.Equipment) != EquipmentSize {
		t.Fatal("container sizes wrong")
	}
	for i, item := range save.Inventory {
		if item.ID != -1 || item.Amount != 0 || item.Meta != nil {
			t.Fatalf("inventory[%d] = %+v, want empty", i, item)
		}
	}
	for i, item := range save.Equipment {
		if item.ID != -1 || item.Amount != 0 || item.Meta != nil {
			t.Fatalf("equipment[%d] = %+v, want empty", i, item)
		}
	}
	if !reflect.DeepEqual(save.Appearance, defaultMaleAppearance) {
		t.Fatalf("appearance = %v", save.Appearance)
	}
	if !reflect.DeepEqual(save.Skills, DefaultSkills()) {
		t.Fatalf("skills = %+v, want DefaultSkills()", save.Skills)
	}
	if len(save.QuickPrayers) != 30 {
		t.Fatalf("quickPrayers length = %d, want 30", len(save.QuickPrayers))
	}
	for i, prayer := range save.QuickPrayers {
		if prayer != nil {
			t.Fatalf("quickPrayers[%d] must be null", i)
		}
	}
	if save.Friends == nil || len(save.Friends) != 0 || save.Ignores == nil || len(save.Ignores) != 0 {
		t.Fatal("friends/ignores must be empty arrays")
	}
	if save.FriendRanks == nil || len(save.FriendRanks) != 0 {
		t.Fatal("friendRanks must be an empty object")
	}
	if save.FriendsChatChannelName != "" || save.FriendsChatLastOwner != "" ||
		save.FriendsChatEntryRank != -1 || save.FriendsChatTalkRank != -1 || save.FriendsChatKickRank != 2 {
		t.Fatal("friends-chat defaults wrong")
	}
	if save.Banks == nil || len(save.Banks) != 0 || save.Flags == nil || len(save.Flags) != 0 {
		t.Fatal("banks/flags must be empty")
	}
	if string(save.Sailing) != emptySailingStateJSON {
		t.Fatalf("sailing = %s", save.Sailing)
	}
	if save.Attributes == nil || len(save.Attributes) != 0 {
		t.Fatal("attributes must be an empty object")
	}
	if save.Unmade || save.Extra != nil {
		t.Fatal("unmade/Extra defaults wrong")
	}
	// A new-character save round-trips through JSON and still validates.
	data := mustMarshalSave(t, save)
	var reloaded PlayerSave
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatalf("unmarshal new-character save: %v", err)
	}
	if err := validateSaveForWrite(&reloaded); err != nil {
		t.Fatalf("reloaded new-character save must pass validation: %v", err)
	}
	if string(reloaded.FightType) != defaultFightTypeJSON || string(reloaded.Sailing) != emptySailingStateJSON {
		t.Fatal("RawMessage defaults must survive JSON round-trip verbatim")
	}
}

func TestNewCharacterSaveDeveloper(t *testing.T) {
	save := NewCharacterSave(NewCharacterParams{
		Username:  "Dev",
		Profile:   ProfileHardcore,
		Spawn:     Location{X: 1, Y: 2, Z: 0},
		Developer: true,
		Now:       time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC),
	})
	if save.Rights.ID != 2 || save.Rights.SpriteID != 619 || save.Rights.YellTag != nil {
		t.Fatalf("developer rights = %+v, want DEVELOPER (id 2, sprite 619, no yell tag)", save.Rights)
	}
	if save.Profile != ProfileHardcore {
		t.Fatalf("profile = %q", save.Profile)
	}
}

func TestNewCharacterSaveZeroNow(t *testing.T) {
	before := time.Now()
	save := NewCharacterSave(NewCharacterParams{Username: "Zero", Spawn: Location{X: 1, Y: 1, Z: 0}})
	if save.CreationDate < before.UnixMilli() || save.CreationDate > time.Now().UnixMilli()+1000 {
		t.Fatalf("zero Now must fall back to now, got %d", save.CreationDate)
	}
}

func TestNewCharacterSaveIndependent(t *testing.T) {
	params := NewCharacterParams{Username: "A", Spawn: Location{X: 1, Y: 1, Z: 0}, Now: time.Now()}
	a := NewCharacterSave(params)
	b := NewCharacterSave(params)
	a.Inventory[0].ID = 99
	a.Appearance[1] = 99
	a.AudioSettings[18] = 99
	a.Position.X = 99
	if b.Inventory[0].ID != -1 || b.Appearance[1] != 3 || b.AudioSettings[18] != 0 || b.Position.X != 1 {
		t.Fatal("NewCharacterSave calls must not share mutable state")
	}
}

func TestSaveRoundTrip(t *testing.T) {
	// JSON save with unknown keys, legacy fields, and unowned data.
	base := applyTestSave()
	base.Inventory = emptyItems(InventorySize)
	base.Inventory[0] = Item{ID: 995, Amount: 1000}
	base.Equipment = emptyItems(EquipmentSize)
	base.Equipment[0] = Item{ID: 1163, Amount: 1}
	base.Skills = DefaultSkills()
	base.Skills.MaxLevel[0] = 50
	base.TeleblockTimer = 17
	base.Attributes = map[string]json.RawMessage{
		"shop:points":         json.RawMessage(`42`),
		"login:tokens":        json.RawMessage(`[]`),
		"ringOfRecoil:damage": json.RawMessage(`5`),
	}
	base.Extra = map[string]json.RawMessage{
		"title":        json.RawMessage(`"Sir"`),
		"xpLocked":     json.RawMessage(`null`),
		"barrowsCrypt": json.RawMessage(`3`),
		"futureField":  json.RawMessage(`{"x":1}`),
	}
	raw := mustMarshalSave(t, base)

	var loaded PlayerSave
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	loaded.MigrateForEntry()
	// The owned state carries the loaded energy forward (as a live
	// player at unchanged energy would): position/running move,
	// energy survives the round trip.
	owned := loaded.OwnedState()
	owned.Position = Location{X: 3200, Y: 3200, Z: 0}
	owned.Running = false
	moved := loaded.WithOwned(owned)
	out := mustMarshalSave(t, moved)

	var final PlayerSave
	if err := json.Unmarshal(out, &final); err != nil {
		t.Fatalf("unmarshal round-tripped save: %v", err)
	}

	// Owned fields updated.
	if final.Position == nil || *final.Position != (Location{X: 3200, Y: 3200, Z: 0}) {
		t.Fatalf("position = %+v", final.Position)
	}
	if final.Running {
		t.Fatal("running must be false after WithOwned")
	}
	if final.TeleblockTimer != 0 {
		t.Fatal("teleblock must be zeroed")
	}
	// Legacy fields migrated, then gone.
	if string(final.Attributes["loyalty:title"]) != `"Sir"` {
		t.Fatalf("legacy title not folded: %v", final.Attributes)
	}
	if _, ok := final.Attributes["skills:xp-locked"]; ok {
		t.Fatal("null legacy value must fold nothing")
	}
	if _, ok := final.Extra["title"]; ok {
		t.Fatal("legacy key title must be gone from Extra")
	}
	if _, ok := final.Extra["xpLocked"]; ok {
		t.Fatal("legacy key xpLocked must be gone from Extra")
	}
	// Attribute migration + default stripping.
	if string(final.Attributes["ring-of-recoil:damage"]) != `5` {
		t.Fatalf("camelCase attribute not migrated: %v", final.Attributes)
	}
	if _, ok := final.Attributes["ringOfRecoil:damage"]; ok {
		t.Fatal("camelCase attribute form must be gone after migration")
	}
	if _, ok := final.Attributes["login:tokens"]; ok {
		t.Fatal("default login:tokens must be stripped by WithOwned")
	}
	if string(final.Attributes["shop:points"]) != `42` {
		t.Fatal("unowned attribute must survive")
	}
	// Unknown keys survive.
	if string(final.Extra["barrowsCrypt"]) != `3` || string(final.Extra["futureField"]) != `{"x":1}` {
		t.Fatalf("unknown Extra keys lost: %v", final.Extra)
	}
	// Unowned data identical.
	if !reflect.DeepEqual(final.Inventory, base.Inventory) ||
		!reflect.DeepEqual(final.Equipment, base.Equipment) ||
		!reflect.DeepEqual(final.Skills, base.Skills) ||
		!reflect.DeepEqual(final.Banks, base.Banks) ||
		string(final.FightType) != string(base.FightType) ||
		string(final.Sailing) != string(base.Sailing) ||
		final.DisplayName != base.DisplayName ||
		final.RunEnergy != base.RunEnergy ||
		!reflect.DeepEqual(final.QuickPrayers, base.QuickPrayers) {
		t.Fatal("unowned data changed across the round-trip")
	}
	if err := validateSaveForWrite(&final); err != nil {
		t.Fatalf("round-tripped save must validate: %v", err)
	}
}

// Legacy button-indexed skill unmapping tests live here (not
// store_test.go) because the unmapping was added with the apply/from
// work; normalizeLoadedSave is exercised through the store and
// directly.

func legacySkillJSON(t *testing.T, levelAt, maxAt, expAt map[int]int) string {
	t.Helper()
	build := func(values map[int]int) string {
		entries := make([]string, 18830)
		for i := range entries {
			entries[i] = "null"
		}
		for index, value := range values {
			entries[index] = strconv.Itoa(value)
		}
		return "[" + strings.Join(entries, ",") + "]"
	}
	return `{"displayName":"Legacy","skills":{"level":` + build(levelAt) +
		`,"maxLevel":` + build(maxAt) + `,"experience":` + build(expAt) + `}}`
}

func TestLegacyButtonIndexedSkillUnmapping(t *testing.T) {
	// Values at button indices: Attack 50, Hitpoints 60, Slayer 70.
	buttons := map[int]int{8654: 50, 8655: 60, 12162: 70}
	raw := legacySkillJSON(t, buttons, buttons, map[int]int{8654: 1000, 8655: 2000})
	var save PlayerSave
	if err := json.Unmarshal([]byte(raw), &save); err != nil {
		t.Fatalf("unmarshal legacy save: %v", err)
	}
	normalizeLoadedSave(&save, ProfileMain)
	if save.Skills.Level[0] != 50 || save.Skills.Level[HitpointsIndex] != 60 || save.Skills.Level[18] != 70 {
		t.Fatalf("level unmapping wrong: %v", save.Skills.Level)
	}
	if save.Skills.MaxLevel[0] != 50 || save.Skills.MaxLevel[18] != 70 {
		t.Fatalf("maxLevel unmapping wrong: %v", save.Skills.MaxLevel)
	}
	if save.Skills.Experience[0] != 1000 || save.Skills.Experience[HitpointsIndex] != 2000 {
		t.Fatalf("experience unmapping wrong: %v", save.Skills.Experience)
	}
	// Untouched indices keep the defaults, including Sailing (no
	// button): level 1, hitpoints-adjacent defaults intact.
	if save.Skills.Level[1] != 1 || save.Skills.Level[23] != 1 {
		t.Fatalf("fallback indices wrong: %v", save.Skills.Level)
	}
	if save.Skills.Experience[1] != 0 || save.Skills.Experience[23] != 0 {
		t.Fatalf("fallback experience wrong: %v", save.Skills.Experience)
	}
}

func TestLegacyButtonIndexedSkillsViaStore(t *testing.T) {
	dir := t.TempDir()
	raw := legacySkillJSON(t, map[int]int{8654: 33}, map[int]int{8654: 33}, map[int]int{})
	if err := os.WriteFile(filepath.Join(dir, "legacy.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewJSONFileStore(dir).Load("Legacy", ProfileMain)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Skills.Level[0] != 33 {
		t.Fatalf("attack level = %d, want 33", loaded.Skills.Level[0])
	}
	if len(loaded.Skills.Level) != SkillCount {
		t.Fatalf("skill array length = %d", len(loaded.Skills.Level))
	}
}

func TestNonLegacyLongSkillArrayTruncates(t *testing.T) {
	// Longer than the skill count but with no value at any button
	// index: the ordinary merge path truncates to the first 24.
	raw := make([]int, 30)
	for i := range raw {
		raw[i] = i + 1
	}
	save := &PlayerSave{Skills: Skills{Level: raw, MaxLevel: raw, Experience: raw}}
	normalizeLoadedSave(save, ProfileMain)
	if len(save.Skills.Level) != SkillCount || save.Skills.Level[0] != 1 || save.Skills.Level[23] != 24 {
		t.Fatalf("non-legacy long array wrong: %v", save.Skills.Level)
	}
	if isLegacyButtonIndexedSkillArray(raw) {
		t.Fatal("array with no button-index value must not detect as legacy")
	}
	var short []int
	if isLegacyButtonIndexedSkillArray(short) || isLegacyButtonIndexedSkillArray(make([]int, SkillCount)) {
		t.Fatal("short arrays must not detect as legacy")
	}
}

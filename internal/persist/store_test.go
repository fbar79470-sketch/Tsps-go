package persist

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// validTestSave builds a save that passes write validation.
func validTestSave(name string) *PlayerSave {
	inv := make([]Item, InventorySize)
	for i := range inv {
		inv[i] = EmptyItem()
	}
	inv[0] = Item{ID: 995, Amount: 1000}
	eq := make([]Item, EquipmentSize)
	for i := range eq {
		eq[i] = EmptyItem()
	}
	eq[3] = Item{ID: 1321, Amount: 1, Meta: map[string]json.RawMessage{"toxic-blowpipe": json.RawMessage(`{"darts": 5}`)}}
	return &PlayerSave{
		DisplayName:          name,
		CreationDate:         1700000000000,
		PasswordHashWithSalt: "test-stored-credential",
		Rights:               Rights{ID: 0, SpriteID: -1, YellTag: strptr("")},
		DonatorRights:        DonatorRights{ID: 0, SpriteID: -1, YellDelay: -1},
		Position:             &Location{X: 3089, Y: 3524, Z: 0},
		SpellBook:            SpellBook{InterfaceID: SpellbookNormalInterfaceID},
		FightType:            json.RawMessage(`{"animation":422,"parentId":43,"childId":0,"bonusType":1}`),
		AutocastSpellId:      -1,
		AutoRetaliate:        true,
		AudioSettings:        map[int]int{0: 4},
		SpecPercentage:       100,
		RunEnergy:            100,
		Inventory:            inv,
		Equipment:            eq,
		Appearance:           []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0},
		Skills:               DefaultSkills(),
		QuickPrayers:         []*QuickPrayer{},
		Friends:              []string{},
		Ignores:              []string{},
		FriendRanks:          map[string]int{},
		FriendsChatEntryRank: -1,
		FriendsChatTalkRank:  -1,
		FriendsChatKickRank:  2,
		Banks:                Banks{0: {{ID: 995, Amount: 500}}},
		Flags:                []string{},
		Attributes:           map[string]json.RawMessage{},
	}
}

// tsSaveFixture is a hand-written save in the exact TS JSON shape:
// field names from PlayerSave.ts, banks in the Map replacer shape,
// rights/spellbook/skull as serialized class instances, a legacy
// top-level field (barrowsCrypt) that TS carries through via
// Object.assign, and a login:tokens attribute.
const tsSaveFixture = `{
  "displayName": "Fixture Player",
  "creationDate": 1700000000000,
  "passwordHashWithSalt": "test-stored-credential",
  "isDiscordLogin": false,
  "cachedDiscordAccessToken": "",
  "rights": {"id": 1, "spriteId": 618},
  "donatorRights": {"id": 2, "spriteId": 623, "yellDelay": 25, "yellTag": "[Super Donator]"},
  "position": {"x": 3222, "y": 3218, "z": 0},
  "spellBook": {"interfaceId": 12855, "teleportType": {"marker": "preserved-verbatim"}},
  "fightType": {"animation": 422, "parentId": 43, "childId": 0, "bonusType": 1},
  "autocastSpellId": -1,
  "autoRetaliate": true,
  "audioSettings": {"0": 4, "1": 3},
  "hasVengeance": false,
  "lastVengeanceTimer": 0,
  "specPercentage": 100,
  "poisonDamage": 0,
  "venomed": false,
  "poisonImmunityTimer": 0,
  "fireImmunityTimer": 0,
  "teleblockTimer": 0,
  "skullTimer": 0,
  "skullType": {"iconId": 1},
  "running": true,
  "runEnergy": 87.5,
  "inventory": [{"id": 995, "amount": 100, "meta": null}, {"id": -1, "amount": 0, "meta": null}],
  "equipment": [{"id": 1321, "amount": 1, "meta": {"toxicBlowpipe": {"darts": 5}}}],
  "appearance": [0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0],
  "skills": {"level": [1, 1, 1, 10], "maxLevel": [1, 1, 1, 10], "experience": [0, 0, 0, 1184]},
  "quickPrayers": [null, {"requirement": 1, "buttonId": 5609, "configId": 4104, "drainEffect": 1, "hint": -1, "prayerId": 0, "name": "Thick Skin"}],
  "friends": ["12345", "12345", "0", "notanumber", "67890"],
  "ignores": [],
  "friendRanks": {"12345": 2},
  "friendsChatChannelName": "",
  "friendsChatLastOwner": "",
  "friendsChatEntryRank": -1,
  "friendsChatTalkRank": -1,
  "friendsChatKickRank": 2,
  "banks": {"__type": "Map", "entries": [[0, [{"id": 995, "amount": 500, "meta": null}]], [2, []]]},
  "flags": ["  alpha ", "alpha", "", "beta"],
  "sailing": {"boats": [], "activeBoatSlot": null},
  "attributes": {"login:tokens": [], "ring-of-recoil:damage": 0, "custom:key": "kept"},
  "barrowsCrypt": 3
}`

func TestPlayerSaveFixtureRoundTrip(t *testing.T) {
	var save PlayerSave
	if err := json.Unmarshal([]byte(tsSaveFixture), &save); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if save.DisplayName != "Fixture Player" || save.Position.X != 3222 || save.Position.Z != 0 {
		t.Fatalf("basic fields wrong: %+v", save)
	}
	if save.Rights.ID != 1 || save.Rights.SpriteID != 618 || save.Rights.YellTag != nil {
		t.Fatalf("rights wrong: %+v (yellTag must be absent for moderator)", save.Rights)
	}
	if save.DonatorRights.YellTag != "[Super Donator]" || save.DonatorRights.YellDelay != 25 {
		t.Fatalf("donator rights wrong: %+v", save.DonatorRights)
	}
	if save.SpellBook.InterfaceID != SpellbookAncientInterfaceID {
		t.Fatalf("spellbook wrong: %+v", save.SpellBook)
	}
	if !strings.Contains(string(save.SpellBook.TeleportType), "preserved-verbatim") {
		t.Fatal("teleportType not preserved verbatim")
	}
	if save.SkullType.IconID != 1 {
		t.Fatal("skullType wrong")
	}
	if save.RunEnergy != 87.5 || !save.Running {
		t.Fatal("run fields wrong")
	}
	// Legacy top-level field preserved in Extra.
	if raw, ok := save.Extra["barrowsCrypt"]; !ok || string(raw) != "3" {
		t.Fatalf("Extra barrowsCrypt = %q, %v", raw, ok)
	}
	// Banks Map-wrapper shape decoded.
	if len(save.Banks) != 2 || len(save.Banks[0]) != 1 || save.Banks[0][0].ID != 995 {
		t.Fatalf("banks wrong: %+v", save.Banks)
	}
	// Item meta key migration: toxicBlowpipe -> toxic-blowpipe.
	meta := save.Equipment[0].Meta
	if _, ok := meta["toxic-blowpipe"]; !ok {
		t.Fatalf("meta migration missing: %v", meta)
	}
	if _, ok := meta["toxicBlowpipe"]; ok {
		t.Fatal("old meta key must be removed")
	}
	// Quick prayers: null slot preserved, object slot typed.
	if len(save.QuickPrayers) != 2 || save.QuickPrayers[0] != nil || save.QuickPrayers[1].ConfigID != 4104 {
		t.Fatalf("quickPrayers wrong: %+v", save.QuickPrayers)
	}
	// Re-marshal: same field names, Extra re-emitted, banks re-wrapped.
	data, err := json.Marshal(&save)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	for _, key := range []string{
		"displayName", "creationDate", "passwordHashWithSalt", "isDiscordLogin",
		"cachedDiscordAccessToken", "rights", "donatorRights", "position", "spellBook",
		"fightType", "autocastSpellId", "autoRetaliate", "audioSettings", "hasVengeance",
		"lastVengeanceTimer", "specPercentage", "poisonDamage", "venomed",
		"poisonImmunityTimer", "fireImmunityTimer", "teleblockTimer", "skullTimer",
		"skullType", "running", "runEnergy", "inventory", "equipment", "appearance",
		"skills", "quickPrayers", "friends", "ignores", "friendRanks",
		"friendsChatChannelName", "friendsChatLastOwner", "friendsChatEntryRank",
		"friendsChatTalkRank", "friendsChatKickRank", "banks", "flags", "sailing",
		"attributes", "barrowsCrypt",
	} {
		if _, ok := m[key]; !ok {
			t.Fatalf("marshaled save missing key %q", key)
		}
	}
	// Optional absent fields must stay absent: no lastVenomDamage,
	// poisonClock, or unmade keys when unset.
	for _, key := range []string{"lastVenomDamage", "poisonClock", "unmade", "profile", "extra"} {
		if _, ok := m[key]; ok {
			t.Fatalf("marshaled save must not contain %q", key)
		}
	}
	banks, ok := m["banks"].(map[string]any)
	if !ok || banks["__type"] != "Map" {
		t.Fatalf("banks must re-marshal in Map-wrapper shape: %v", m["banks"])
	}
	// Second round-trip is stable.
	var save2 PlayerSave
	if err := json.Unmarshal(data, &save2); err != nil {
		t.Fatalf("second unmarshal: %v", err)
	}
	data2, _ := json.Marshal(&save2)
	if string(data) != string(data2) {
		t.Fatal("marshal is not stable across round-trips")
	}
}

func TestPlayerSaveNumericRights(t *testing.T) {
	var save PlayerSave
	if err := json.Unmarshal([]byte(`{"rights": 2, "donatorRights": 1}`), &save); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if save.Rights.ID != 2 || save.Rights.SpriteID != 619 {
		t.Fatalf("numeric rights wrong: %+v", save.Rights)
	}
	if save.DonatorRights.ID != 1 || save.DonatorRights.SpriteID != 622 || save.DonatorRights.YellTag != "[Donator]" {
		t.Fatalf("numeric donator rights wrong: %+v", save.DonatorRights)
	}
}

func TestStripDefaultAttributes(t *testing.T) {
	attrs := map[string]json.RawMessage{
		"login:tokens":             json.RawMessage(`[]`),
		"ring-of-recoil:damage":    json.RawMessage(`0`),
		"bank:note-withdrawal":     json.RawMessage(`false`),
		"bank:quantity-mode":       json.RawMessage(`3`),
		"combat:style-by-category": json.RawMessage(`{}`),
		"custom:key":               json.RawMessage(`"kept"`),
	}
	out := StripDefaultAttributes(attrs)
	for _, gone := range []string{"login:tokens", "ring-of-recoil:damage", "bank:note-withdrawal", "combat:style-by-category"} {
		if _, ok := out[gone]; ok {
			t.Fatalf("default attribute %q not stripped", gone)
		}
	}
	if _, ok := out["bank:quantity-mode"]; !ok {
		t.Fatal("non-default bank:quantity-mode stripped")
	}
	if _, ok := out["custom:key"]; !ok {
		t.Fatal("custom attribute stripped")
	}
	if _, ok := AttributeDefaultJSON("login:tokens"); !ok {
		t.Fatal("login:tokens must have a registered default")
	}
	if _, ok := AttributeDefaultJSON("custom:key"); ok {
		t.Fatal("custom:key must not have a registered default")
	}
}

func TestPlayerSaveTokensHelpers(t *testing.T) {
	now := time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC)
	save := validTestSave("Token Player")
	if got := save.Tokens(now); len(got) != 0 {
		t.Fatal("save without tokens attribute must yield none")
	}
	entries := []TokenEntry{{Hash: HashToken("t1"), LastUsed: now}}
	if err := save.SetTokens(entries); err != nil {
		t.Fatalf("SetTokens: %v", err)
	}
	if got := save.Tokens(now); len(got) != 1 || got[0].Hash != HashToken("t1") {
		t.Fatalf("Tokens = %+v", got)
	}
	if err := save.SetTokens(nil); err != nil {
		t.Fatalf("SetTokens(nil): %v", err)
	}
	if string(save.Attributes[LoginTokensAttribute]) != "[]" {
		t.Fatalf("cleared tokens must store []: %s", save.Attributes[LoginTokensAttribute])
	}
}

func TestProfileStorageKey(t *testing.T) {
	if got := ProfileStorageKey("zezima", ProfileMain); got != "zezima" {
		t.Fatalf("main key = %q", got)
	}
	if got := ProfileStorageKey("zezima", ProfileLeague); got != "league:zezima" {
		t.Fatalf("league key = %q", got)
	}
	if got := ProfileStorageKey("zezima", ProfileHardcore); got != "hardcore:zezima" {
		t.Fatalf("hardcore key = %q", got)
	}
	if got := StorageKey("Test Player", ProfileLeague); got != "league:test_player" {
		t.Fatalf("StorageKey = %q", got)
	}
	if got := OtherProfiles(ProfileMain); len(got) != 2 || got[0] != ProfileLeague || got[1] != ProfileHardcore {
		t.Fatalf("OtherProfiles = %v", got)
	}
}

func TestJSONFileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewJSONFileStore(dir)
	save := validTestSave("Round Trip")
	if err := store.Save(save, "logout"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// File exists at the expected path, indented, no temp leftovers.
	path := filepath.Join(dir, "round_trip.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("save file missing: %v", err)
	}
	if !strings.Contains(string(data), "\n  \"displayName\"") {
		t.Fatal("save file must be 2-space indented JSON")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("dir has %d entries after save, want 1 (no temp leftovers)", len(entries))
	}
	if !store.Exists("Round Trip", ProfileMain) || store.Exists("Round Trip", ProfileLeague) {
		t.Fatal("Exists wrong")
	}
	loaded, err := store.Load("round_trip", ProfileMain)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.DisplayName != "Round Trip" || loaded.Profile != ProfileMain {
		t.Fatalf("loaded identity wrong: %+v", loaded)
	}
	if loaded.Inventory[0].ID != 995 || loaded.Inventory[0].Amount != 1000 {
		t.Fatal("inventory not round-tripped")
	}
	if len(loaded.Banks[0]) != 1 || loaded.Banks[0][0].Amount != 500 {
		t.Fatal("banks not round-tripped")
	}
	if loaded.Skills.Experience[HitpointsIndex] != DefaultHitpointsXP {
		t.Fatal("skills not round-tripped")
	}
	// Overwriting works and leaves no temp files.
	save.RunEnergy = 42.5
	if err := store.Save(save, "autosave"); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	loaded, _ = store.Load("Round Trip", ProfileMain)
	if loaded.RunEnergy != 42.5 {
		t.Fatal("overwrite did not take effect")
	}
	entries, _ = os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("dir has %d entries after overwrite", len(entries))
	}
}

func TestJSONFileStoreProfiles(t *testing.T) {
	dir := t.TempDir()
	store := NewJSONFileStore(dir)
	league := validTestSave("League Player")
	league.Profile = ProfileLeague
	if err := store.Save(league, "logout"); err != nil {
		t.Fatalf("Save league: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "league", "league_player.json")); err != nil {
		t.Fatalf("league save not in profile subdir: %v", err)
	}
	loaded, err := store.Load("League Player", ProfileLeague)
	if err != nil || loaded.Profile != ProfileLeague {
		t.Fatalf("league load: %+v, %v", loaded, err)
	}
	if _, err := store.Load("League Player", ProfileMain); !errors.Is(err, ErrSaveNotFound) {
		t.Fatalf("main load of league-only character: err = %v, want ErrSaveNotFound", err)
	}
	if _, err := store.Load("Ghost", ProfileMain); !errors.Is(err, ErrSaveNotFound) {
		t.Fatalf("missing load: err = %v, want ErrSaveNotFound", err)
	}
	if err := store.Delete("League Player", ProfileLeague); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.Exists("League Player", ProfileLeague) {
		t.Fatal("deleted save still exists")
	}
}

func TestJSONFileStoreValidation(t *testing.T) {
	dir := t.TempDir()
	store := NewJSONFileStore(dir)
	bad := validTestSave("Bad Save")
	bad.Inventory = bad.Inventory[:10]
	if err := store.Save(bad, "logout"); err == nil {
		t.Fatal("short inventory must be refused")
	}
	if store.Exists("Bad Save", ProfileMain) {
		t.Fatal("refused save must not create a file")
	}
	bad2 := validTestSave("Bad Save 2")
	bad2.Position = nil
	if err := store.Save(bad2, "logout"); err == nil {
		t.Fatal("missing position must be refused")
	}
	bad3 := validTestSave("Bad Save 3")
	bad3.Skills.Level = bad3.Skills.Level[:5]
	if err := store.Save(bad3, "logout"); err == nil {
		t.Fatal("short skill array must be refused")
	}
	if err := store.Save(nil, "logout"); err == nil {
		t.Fatal("nil save must be refused")
	}
	empty := validTestSave("")
	if err := store.Save(empty, "logout"); err == nil {
		t.Fatal("empty display name must be refused")
	}
}

func TestJSONFileStoreLoadNormalization(t *testing.T) {
	dir := t.TempDir()
	// Write the TS fixture directly as the character file.
	if err := os.WriteFile(filepath.Join(dir, "fixture_player.json"), []byte(tsSaveFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewJSONFileStore(dir)
	loaded, err := store.Load("Fixture Player", ProfileMain)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Inventory/equipment padded to full size.
	if len(loaded.Inventory) != InventorySize || len(loaded.Equipment) != EquipmentSize {
		t.Fatalf("containers not padded: %d/%d", len(loaded.Inventory), len(loaded.Equipment))
	}
	if loaded.Inventory[2].ID != -1 || loaded.Inventory[2].Amount != 0 {
		t.Fatal("padding items must be empty")
	}
	// Skills merged over defaults: index 4+ default to 1/1/0.
	if len(loaded.Skills.Level) != SkillCount || loaded.Skills.Level[4] != 1 || loaded.Skills.Experience[4] != 0 {
		t.Fatalf("skills not merged: %+v", loaded.Skills)
	}
	if loaded.Skills.Level[HitpointsIndex] != 10 {
		t.Fatal("saved hitpoints level lost in merge")
	}
	// Friends deduped, invalid dropped.
	if len(loaded.Friends) != 2 || loaded.Friends[0] != "12345" || loaded.Friends[1] != "67890" {
		t.Fatalf("friends wrong: %v", loaded.Friends)
	}
	// Flags trimmed/deduped.
	if len(loaded.Flags) != 2 || loaded.Flags[0] != "alpha" || loaded.Flags[1] != "beta" {
		t.Fatalf("flags wrong: %v", loaded.Flags)
	}
	// Legacy field survived the load.
	if _, ok := loaded.Extra["barrowsCrypt"]; !ok {
		t.Fatal("Extra lost on load")
	}
}

func TestJSONFileStoreLoadDefaultsMissingPosition(t *testing.T) {
	dir := t.TempDir()
	minimal := `{"displayName": "Minimal", "inventory": [], "equipment": [], "skills": {}}`
	if err := os.WriteFile(filepath.Join(dir, "minimal.json"), []byte(minimal), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewJSONFileStore(dir)
	loaded, err := store.Load("Minimal", ProfileMain)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Position.X != DefaultSpawnX || loaded.Position.Y != DefaultSpawnY || loaded.Position.Z != DefaultSpawnZ {
		t.Fatalf("missing position must default to spawn: %+v", loaded.Position)
	}
	if loaded.Attributes == nil || loaded.Banks == nil {
		t.Fatal("attributes/banks must default to empty, not nil")
	}
	if len(loaded.Skills.Level) != SkillCount {
		t.Fatal("missing skills must default")
	}
	// Malformed JSON is an error naming the problem.
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("Broken", ProfileMain); err == nil || errors.Is(err, ErrSaveNotFound) {
		t.Fatalf("malformed save must error (not ErrSaveNotFound): %v", err)
	}
}

func TestMemoryStore(t *testing.T) {
	var store SaveStore = NewMemoryStore()
	if _, err := store.Load("Nobody", ProfileMain); !errors.Is(err, ErrSaveNotFound) {
		t.Fatalf("empty load: %v", err)
	}
	save := validTestSave("Memory Player")
	save.Profile = ProfileHardcore
	if err := store.Save(save, "test"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !store.Exists("Memory Player", ProfileHardcore) || store.Exists("Memory Player", ProfileMain) {
		t.Fatal("Exists wrong")
	}
	loaded, err := store.Load("MEMORY PLAYER", ProfileHardcore)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Profile != ProfileHardcore {
		t.Fatal("profile not preserved")
	}
	// Mutating the loaded copy must not affect the stored copy.
	loaded.RunEnergy = 1
	loaded2, _ := store.Load("Memory Player", ProfileHardcore)
	if loaded2.RunEnergy != 100 {
		t.Fatal("store did not deep-copy on load")
	}
	// Mutating the saved struct after Save must not affect the store.
	save.RunEnergy = 2
	loaded3, _ := store.Load("Memory Player", ProfileHardcore)
	if loaded3.RunEnergy != 100 {
		t.Fatal("store did not deep-copy on save")
	}
}

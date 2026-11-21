package fishing

import (
	"strings"
	"testing"
)

// LoadIDs tests: resolution of every field, the imbued-ring
// order, and fail-closed behaviour. Fixture only.

func TestLoadIDsFixture(t *testing.T) {
	names := map[string]int{
		"spirit_flakes":                                 7001,
		"zeah_blessing_easy":                            7002,
		"zeah_blessing_medium":                          7003,
		"zeah_blessing_hard":                            7004,
		"zeah_blessing_elite":                           7005,
		"fishing_clue_bottle_beginner":                  7010,
		"fishing_clue_bottle_easy":                      7011,
		"fishing_clue_bottle_medium":                    7012,
		"fishing_clue_bottle_hard":                      7013,
		"fishing_clue_bottle_elite":                     7014,
		"trawler_reward_hat":                            7020,
		"trawler_reward_top":                            7021,
		"trawler_reward_legs":                           7022,
		"trawler_reward_boots":                          7023,
		"spirit_angler_hat":                             7024,
		"spirit_angler_top":                             7025,
		"spirit_angler_legs":                            7026,
		"spirit_angler_boots":                           7027,
		"prif_crystal_shard":                            7030,
		"smouldering_stone":                             7031,
		"dragon_harpoon":                                7032,
		"ice_gloves":                                    7033,
		"smithing_uniform_gloves_ice":                   7034,
		"elven_signet":                                  7040,
		"cert_elven_signet":                             7041,
		"placeholder_elven_signet":                      7042,
		"celestial_signet_charged":                      7043,
		"celestial_signet":                              7044,
		"ring_of_wealth_i":                              7050,
		"ring_of_wealth_i1":                             7051,
		"ring_of_wealth_i2":                             7052,
		"ring_of_wealth_i3":                             7053,
		"ring_of_wealth_i4":                             7054,
		"ring_of_wealth_i5":                             7055,
		"placeholder_ring_of_wealth_i1":                 7056,
		"placeholder_ring_of_wealth_i5":                 7057,
		"tbwt_karambwan_vessel":                         7060,
		"tbwt_karambwan_vessel_loaded_with_karambwanji": 7061,
		"tbwt_raw_karambwanji":                          7062,
	}
	ids, err := LoadIDs(fixtureResolver{obj: names})
	if err != nil {
		t.Fatalf("LoadIDs(fixture) = %v", err)
	}
	cases := []struct {
		field string
		got   int
		want  int
	}{
		{"SpiritFlakes", ids.SpiritFlakes, 7001},
		{"RadasBlessing1", ids.RadasBlessing1, 7002},
		{"RadasBlessing2", ids.RadasBlessing2, 7003},
		{"RadasBlessing3", ids.RadasBlessing3, 7004},
		{"RadasBlessing4", ids.RadasBlessing4, 7005},
		{"ClueBottleBeginner", ids.ClueBottleBeginner, 7010},
		{"ClueBottleEasy", ids.ClueBottleEasy, 7011},
		{"ClueBottleMedium", ids.ClueBottleMedium, 7012},
		{"ClueBottleHard", ids.ClueBottleHard, 7013},
		{"ClueBottleElite", ids.ClueBottleElite, 7014},
		{"AnglerHat", ids.AnglerHat, 7020},
		{"AnglerTop", ids.AnglerTop, 7021},
		{"AnglerWaders", ids.AnglerWaders, 7022},
		{"AnglerBoots", ids.AnglerBoots, 7023},
		{"SpiritAnglerHeadband", ids.SpiritAnglerHeadband, 7024},
		{"SpiritAnglerTop", ids.SpiritAnglerTop, 7025},
		{"SpiritAnglerWaders", ids.SpiritAnglerWaders, 7026},
		{"SpiritAnglerBoots", ids.SpiritAnglerBoots, 7027},
		{"CrystalShard", ids.CrystalShard, 7030},
		{"SmoulderingStone", ids.SmoulderingStone, 7031},
		{"DragonHarpoon", ids.DragonHarpoon, 7032},
		{"IceGloves", ids.IceGloves, 7033},
		{"SmithsGlovesI", ids.SmithsGlovesI, 7034},
		{"ElvenSignet", ids.ElvenSignet, 7040},
		{"ElvenSignet2", ids.ElvenSignet2, 7041},
		{"ElvenSignet3", ids.ElvenSignet3, 7042},
		{"CelestialSignet", ids.CelestialSignet, 7043},
		{"CelestialSignetUncharged", ids.CelestialSignetUncharged, 7044},
		{"KarambwanVessel", ids.KarambwanVessel, 7060},
		{"KarambwanVesselBaited", ids.KarambwanVesselBaited, 7061},
		{"RawKarambwanji", ids.RawKarambwanji, 7062},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.field, c.got, c.want)
		}
	}
	// Imbued rings in TS order: I_, I1_..I5_, I1__2, I5__2.
	wantRings := []int{7050, 7051, 7052, 7053, 7054, 7055, 7056, 7057}
	if len(ids.ImbuedRings) != len(wantRings) {
		t.Fatalf("ImbuedRings = %v, want %v", ids.ImbuedRings, wantRings)
	}
	for i, want := range wantRings {
		if ids.ImbuedRings[i] != want {
			t.Errorf("ImbuedRings[%d] = %d, want %d", i, ids.ImbuedRings[i], want)
		}
	}
}

func TestLoadIDsFailsClosed(t *testing.T) {
	names := map[string]int{"spirit_flakes": 7001}
	// Only one name resolves: every other name must be listed.
	if _, err := LoadIDs(fixtureResolver{obj: names}); err == nil {
		t.Fatal("LoadIDs(partial) = nil error, want an error")
	} else if !strings.Contains(err.Error(), "spirit_angler_hat") {
		t.Errorf("LoadIDs(partial) error = %v, want it to list a missing name", err)
	}
	if _, err := LoadIDs(nil); err == nil {
		t.Error("LoadIDs(nil) = nil error, want an error")
	}
	if _, err := LoadIDsFromCache(nil); err == nil {
		t.Error("LoadIDsFromCache(nil) = nil error, want an error")
	}
}

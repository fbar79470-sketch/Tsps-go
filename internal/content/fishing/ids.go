// FishingIDs is the resolved id set the Fishing sub-units
// need beyond the tables: the angler outfit pieces, the
// spirit flakes, Rada's blessings, the clue bottles, the
// crystal/infernal harpoon consumables and signets, the
// karambwan vessel, and the imbued rings of wealth. Every
// field resolves by cache gameval name through LoadIDs —
// fail-closed on unresolvable names, the tables.go Load
// precedent. No numeric id here is guessed: the gameval
// names below were verified against the real rev-242 cache
// by probe (2026-11-20), each resolving to the
// ItemIdentifiers.ts id in the comment.
//
// The cache's gameval names are Jagex's internal names, not
// the obvious lowercase forms: Rada's blessings are
// "zeah_blessing_<tier>", clue bottles
// "fishing_clue_bottle_<tier>", the angler pieces
// "trawler_reward_<slot>", the spirit pieces
// "spirit_angler_<slot>", the crystal shard
// "prif_crystal_shard", the karambwan vessel
// "tbwt_karambwan_vessel(_loaded_with_karambwanji)", the
// charged celestial signet "celestial_signet_charged"
// (the bare "celestial_signet" is the UNCHARGED one),
// ELVEN_SIGNET_2/3 are the cert/placeholder forms
// ("cert_elven_signet" / "placeholder_elven_signet"), and
// the two charged ring variants are placeholders
// ("placeholder_ring_of_wealth_i1/i5"). The TS ids are
// authoritative — the names are just how the cache knows
// them — so the cert/placeholder forms are resolved exactly
// as TS lists them; they can never be equipped, so the
// signet and ring checks simply never match them, as in TS.
package fishing

import (
	"errors"
	"fmt"

	"tsps-go/internal/cache"
)

// FishingIDs is the resolved id set for the Fishing
// sub-units (see the package comment for the gameval
// grounding of each name).
type FishingIDs struct {
	// SpiritFlakes is ItemIds.SPIRIT_FLAKES
	// ("spirit_flakes", ItemIdentifiers.ts:23608).
	SpiritFlakes int
	// RadasBlessing1 is ItemIds.RADAS_BLESSING_1
	// ("zeah_blessing_easy", ItemIdentifiers.ts:21140).
	RadasBlessing1 int
	// RadasBlessing2 is ItemIds.RADAS_BLESSING_2
	// ("zeah_blessing_medium", ItemIdentifiers.ts:21142).
	RadasBlessing2 int
	// RadasBlessing3 is ItemIds.RADAS_BLESSING_3
	// ("zeah_blessing_hard", ItemIdentifiers.ts:21144).
	RadasBlessing3 int
	// RadasBlessing4 is ItemIds.RADAS_BLESSING_4
	// ("zeah_blessing_elite", ItemIdentifiers.ts:21146).
	RadasBlessing4 int
	// ClueBottleBeginner is ItemIds.CLUE_BOTTLE_BEGINNER_
	// ("fishing_clue_bottle_beginner",
	// ItemIdentifiers.ts:21319).
	ClueBottleBeginner int
	// ClueBottleEasy is ItemIds.CLUE_BOTTLE_EASY_
	// ("fishing_clue_bottle_easy",
	// ItemIdentifiers.ts:12093).
	ClueBottleEasy int
	// ClueBottleMedium is ItemIds.CLUE_BOTTLE_MEDIUM_
	// ("fishing_clue_bottle_medium",
	// ItemIdentifiers.ts:12094).
	ClueBottleMedium int
	// ClueBottleHard is ItemIds.CLUE_BOTTLE_HARD_
	// ("fishing_clue_bottle_hard",
	// ItemIdentifiers.ts:12095).
	ClueBottleHard int
	// ClueBottleElite is ItemIds.CLUE_BOTTLE_ELITE_
	// ("fishing_clue_bottle_elite",
	// ItemIdentifiers.ts:12096).
	ClueBottleElite int
	// AnglerHat is ItemIds.ANGLER_HAT ("trawler_reward_hat",
	// ItemIdentifiers.ts:11713).
	AnglerHat int
	// AnglerTop is ItemIds.ANGLER_TOP ("trawler_reward_top",
	// ItemIdentifiers.ts:11714).
	AnglerTop int
	// AnglerWaders is ItemIds.ANGLER_WADERS
	// ("trawler_reward_legs", ItemIdentifiers.ts:11715).
	AnglerWaders int
	// AnglerBoots is ItemIds.ANGLER_BOOTS
	// ("trawler_reward_boots", ItemIdentifiers.ts:11716).
	AnglerBoots int
	// SpiritAnglerHeadband is ItemIds.SPIRIT_ANGLER_HEADBAND
	// ("spirit_angler_hat", ItemIdentifiers.ts:23612).
	SpiritAnglerHeadband int
	// SpiritAnglerTop is ItemIds.SPIRIT_ANGLER_TOP
	// ("spirit_angler_top", ItemIdentifiers.ts:23614).
	SpiritAnglerTop int
	// SpiritAnglerWaders is ItemIds.SPIRIT_ANGLER_WADERS
	// ("spirit_angler_legs", ItemIdentifiers.ts:23616).
	SpiritAnglerWaders int
	// SpiritAnglerBoots is ItemIds.SPIRIT_ANGLER_BOOTS
	// ("spirit_angler_boots", ItemIdentifiers.ts:23618).
	SpiritAnglerBoots int
	// CrystalShard is ItemIds.CRYSTAL_SHARD
	// ("prif_crystal_shard", ItemIdentifiers.ts:22111).
	CrystalShard int
	// SmoulderingStone is ItemIds.SMOULDERING_STONE
	// ("smouldering_stone", ItemIdentifiers.ts:11688).
	SmoulderingStone int
	// DragonHarpoon is ItemIds.DRAGON_HARPOON
	// ("dragon_harpoon", ItemIdentifiers.ts:19375).
	DragonHarpoon int
	// IceGloves is ItemIds.ICE_GLOVES ("ice_gloves",
	// ItemIdentifiers.ts:1514).
	IceGloves int
	// SmithsGlovesI is ItemIds.SMITHS_GLOVES_I_
	// ("smithing_uniform_gloves_ice",
	// ItemIdentifiers.ts:24978; the gameval was already
	// proven by the tables' INFERNAL_EEL_ROD worn list).
	SmithsGlovesI int
	// ElvenSignet is ItemIds.ELVEN_SIGNET ("elven_signet",
	// ItemIdentifiers.ts:22092).
	ElvenSignet int
	// ElvenSignet2 is ItemIds.ELVEN_SIGNET_2
	// ("cert_elven_signet", ItemIdentifiers.ts:22093): the
	// certificate form — see the package comment.
	ElvenSignet2 int
	// ElvenSignet3 is ItemIds.ELVEN_SIGNET_3
	// ("placeholder_elven_signet",
	// ItemIdentifiers.ts:22094): the placeholder form — see
	// the package comment.
	ElvenSignet3 int
	// CelestialSignet is ItemIds.CELESTIAL_SIGNET
	// ("celestial_signet_charged",
	// ItemIdentifiers.ts:23565).
	CelestialSignet int
	// CelestialSignetUncharged is
	// ItemIds.CELESTIAL_SIGNET_UNCHARGED_
	// ("celestial_signet", ItemIdentifiers.ts:23563).
	CelestialSignetUncharged int
	// ImbuedRings are the imbued ring of wealth ids the
	// Wilderness clue-bottle check tests
	// (ClueBottles.Fishing.js:56-58), in TS order:
	// RING_OF_WEALTH_I_ ("ring_of_wealth_i", 12785),
	// I1_ ("ring_of_wealth_i1", 20790), I2_
	// ("ring_of_wealth_i2", 20789), I3_
	// ("ring_of_wealth_i3", 20788), I4_
	// ("ring_of_wealth_i4", 20787), I5_
	// ("ring_of_wealth_i5", 20786), I1__2
	// ("placeholder_ring_of_wealth_i1", 21459), I5__2
	// ("placeholder_ring_of_wealth_i5", 21458).
	// (ItemIdentifiers.ts:11249, 19133-19137, 19779-19780.)
	ImbuedRings []int
	// KarambwanVessel is ItemIds.KARAMBWAN_VESSEL
	// ("tbwt_karambwan_vessel", ItemIdentifiers.ts:3000).
	KarambwanVessel int
	// KarambwanVesselBaited is ItemIds.KARAMBWAN_VESSEL_3
	// ("tbwt_karambwan_vessel_loaded_with_karambwanji",
	// ItemIdentifiers.ts:3002).
	KarambwanVesselBaited int
	// RawKarambwanji is ItemIds.RAW_KARAMBWANJI
	// ("tbwt_raw_karambwanji", ItemIdentifiers.ts:2993; the
	// gameval was already proven by the tables'
	// KARAMBWANJI fish entry).
	RawKarambwanji int
}

// idResolver collects unresolved names so LoadIDs fails
// closed with every missing name listed, instead of
// stopping at the first — the tables.go resolver
// precedent.
type idResolver struct {
	r       IDResolver
	missing []string
}

func (rs *idResolver) obj(name string) int {
	id, ok, err := rs.r.ObjID(name)
	if err != nil {
		rs.missing = append(rs.missing, fmt.Sprintf("obj %q: %v", name, err))
		return 0
	}
	if !ok {
		rs.missing = append(rs.missing, fmt.Sprintf("obj %q", name))
		return 0
	}
	return id
}

func (rs *idResolver) err() error {
	if len(rs.missing) == 0 {
		return nil
	}
	return fmt.Errorf("fishing: unresolved cache ids: %v", rs.missing)
}

// LoadIDs resolves every FishingIDs field through r, or an
// error listing every gameval name the cache does not know.
// No id is invented: an unresolvable name fails the load.
func LoadIDs(r IDResolver) (*FishingIDs, error) {
	if r == nil {
		return nil, errors.New("fishing: nil id resolver")
	}
	rs := &idResolver{r: r}
	ids := &FishingIDs{
		SpiritFlakes:             rs.obj("spirit_flakes"),
		RadasBlessing1:           rs.obj("zeah_blessing_easy"),
		RadasBlessing2:           rs.obj("zeah_blessing_medium"),
		RadasBlessing3:           rs.obj("zeah_blessing_hard"),
		RadasBlessing4:           rs.obj("zeah_blessing_elite"),
		ClueBottleBeginner:       rs.obj("fishing_clue_bottle_beginner"),
		ClueBottleEasy:           rs.obj("fishing_clue_bottle_easy"),
		ClueBottleMedium:         rs.obj("fishing_clue_bottle_medium"),
		ClueBottleHard:           rs.obj("fishing_clue_bottle_hard"),
		ClueBottleElite:          rs.obj("fishing_clue_bottle_elite"),
		AnglerHat:                rs.obj("trawler_reward_hat"),
		AnglerTop:                rs.obj("trawler_reward_top"),
		AnglerWaders:             rs.obj("trawler_reward_legs"),
		AnglerBoots:              rs.obj("trawler_reward_boots"),
		SpiritAnglerHeadband:     rs.obj("spirit_angler_hat"),
		SpiritAnglerTop:          rs.obj("spirit_angler_top"),
		SpiritAnglerWaders:       rs.obj("spirit_angler_legs"),
		SpiritAnglerBoots:        rs.obj("spirit_angler_boots"),
		CrystalShard:             rs.obj("prif_crystal_shard"),
		SmoulderingStone:         rs.obj("smouldering_stone"),
		DragonHarpoon:            rs.obj("dragon_harpoon"),
		IceGloves:                rs.obj("ice_gloves"),
		SmithsGlovesI:            rs.obj("smithing_uniform_gloves_ice"),
		ElvenSignet:              rs.obj("elven_signet"),
		ElvenSignet2:             rs.obj("cert_elven_signet"),
		ElvenSignet3:             rs.obj("placeholder_elven_signet"),
		CelestialSignet:          rs.obj("celestial_signet_charged"),
		CelestialSignetUncharged: rs.obj("celestial_signet"),
		ImbuedRings: []int{
			rs.obj("ring_of_wealth_i"),
			rs.obj("ring_of_wealth_i1"),
			rs.obj("ring_of_wealth_i2"),
			rs.obj("ring_of_wealth_i3"),
			rs.obj("ring_of_wealth_i4"),
			rs.obj("ring_of_wealth_i5"),
			rs.obj("placeholder_ring_of_wealth_i1"),
			rs.obj("placeholder_ring_of_wealth_i5"),
		},
		KarambwanVessel:       rs.obj("tbwt_karambwan_vessel"),
		KarambwanVesselBaited: rs.obj("tbwt_karambwan_vessel_loaded_with_karambwanji"),
		RawKarambwanji:        rs.obj("tbwt_raw_karambwanji"),
	}
	if err := rs.err(); err != nil {
		return nil, err
	}
	return ids, nil
}

// LoadIDsFromCache resolves the FishingIDs against the real
// cache definitions. A nil cache fails. It mirrors
// LoadFromCache in tables.go; Worker B's wiring calls it
// and sets f.ids.
func LoadIDsFromCache(defs *cache.CacheDefinitions) (*FishingIDs, error) {
	if defs == nil {
		return nil, errors.New("fishing: nil cache definitions")
	}
	return LoadIDs(cacheResolver{defs: defs})
}

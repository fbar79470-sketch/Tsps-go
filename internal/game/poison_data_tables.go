package game

// Poisoned-item table, script-extracted from upstream
// server/src/main/typescript/elvarg/game/task/impl/CombatPoisonEffect.ts
// (CombatPoisonData.init, 258 types.set calls) with every id resolved
// through server/src/main/typescript/elvarg/util/ItemIdentifiers.ts
// (32,286 identifiers; zero unresolved, zero duplicate ids). The
// upstream file carries the table in declaration order; this file
// keeps that order. Verified by count + spot-checks in
// poison_data_test.go: 258 entries, 43 VERY_WEAK / 43 WEAK /
// 75 MILD / 32 EXTRA / 62 SUPER / 3 VENOM — the upstream
// distribution exactly. The (kp) spear/hasta rows and the
// dragon-knife/hasta and abyssal-dagger (bh) rows are upstream
// additions documented in the TS comments (Wiki: Karambwan
// paste); the abyssal tentacle is NOT in the table upstream
// and is not here either. Toxic blowpipe (12926) and the two
// noxious halberds (29796, 33178) are the three VENOM rows.
//
// The table is data only: PoisonTypeForItemID reads it,
// GetMeleeSeverity / GetRangedSeverity / GetMeleeRollRange
// (poison.go) carry the severity and roll rules.

// poisonedItemType is one CombatPoisonData.types entry: a
// poisoned item id and its PoisonType.
type poisonedItemType struct {
	itemID int
	typ    PoisonType
}

// poisonedItemTypes is every CombatPoisonData.types entry, in
// upstream declaration order: 258 entries.
var poisonedItemTypes = []poisonedItemType{
	{itemID: 812, typ: PoisonTypeVeryWeak},   // BRONZE_DART_P_
	{itemID: 813, typ: PoisonTypeVeryWeak},   // IRON_DART_P_
	{itemID: 814, typ: PoisonTypeVeryWeak},   // STEEL_DART_P_
	{itemID: 815, typ: PoisonTypeVeryWeak},   // MITHRIL_DART_P_
	{itemID: 816, typ: PoisonTypeVeryWeak},   // ADAMANT_DART_P_
	{itemID: 817, typ: PoisonTypeVeryWeak},   // RUNE_DART_P_
	{itemID: 831, typ: PoisonTypeVeryWeak},   // BRONZE_JAVELIN_P_
	{itemID: 832, typ: PoisonTypeVeryWeak},   // IRON_JAVELIN_P_
	{itemID: 833, typ: PoisonTypeVeryWeak},   // STEEL_JAVELIN_P_
	{itemID: 834, typ: PoisonTypeVeryWeak},   // MITHRIL_JAVELIN_P_
	{itemID: 835, typ: PoisonTypeVeryWeak},   // ADAMANT_JAVELIN_P_
	{itemID: 836, typ: PoisonTypeVeryWeak},   // RUNE_JAVELIN_P_
	{itemID: 870, typ: PoisonTypeVeryWeak},   // BRONZE_KNIFE_P_
	{itemID: 871, typ: PoisonTypeVeryWeak},   // IRON_KNIFE_P_
	{itemID: 872, typ: PoisonTypeVeryWeak},   // STEEL_KNIFE_P_
	{itemID: 873, typ: PoisonTypeVeryWeak},   // MITHRIL_KNIFE_P_
	{itemID: 874, typ: PoisonTypeVeryWeak},   // BLACK_KNIFE_P_
	{itemID: 875, typ: PoisonTypeVeryWeak},   // ADAMANT_KNIFE_P_
	{itemID: 876, typ: PoisonTypeVeryWeak},   // RUNE_KNIFE_P_
	{itemID: 878, typ: PoisonTypeVeryWeak},   // BRONZE_BOLTS_P_
	{itemID: 883, typ: PoisonTypeVeryWeak},   // BRONZE_ARROW_P_
	{itemID: 885, typ: PoisonTypeVeryWeak},   // IRON_ARROW_P_
	{itemID: 887, typ: PoisonTypeVeryWeak},   // STEEL_ARROW_P_
	{itemID: 889, typ: PoisonTypeVeryWeak},   // MITHRIL_ARROW_P_
	{itemID: 891, typ: PoisonTypeVeryWeak},   // ADAMANT_ARROW_P_
	{itemID: 893, typ: PoisonTypeVeryWeak},   // RUNE_ARROW_P_
	{itemID: 1219, typ: PoisonTypeMild},      // IRON_DAGGER_P_
	{itemID: 1221, typ: PoisonTypeMild},      // BRONZE_DAGGER_P_
	{itemID: 1223, typ: PoisonTypeMild},      // STEEL_DAGGER_P_
	{itemID: 1225, typ: PoisonTypeMild},      // MITHRIL_DAGGER_P_
	{itemID: 1227, typ: PoisonTypeMild},      // ADAMANT_DAGGER_P_
	{itemID: 1229, typ: PoisonTypeMild},      // RUNE_DAGGER_P_
	{itemID: 1231, typ: PoisonTypeMild},      // DRAGON_DAGGER_P_
	{itemID: 1233, typ: PoisonTypeMild},      // BLACK_DAGGER_P_
	{itemID: 1235, typ: PoisonTypeMild},      // POISONED_DAGGER_P_
	{itemID: 1251, typ: PoisonTypeMild},      // BRONZE_SPEAR_P_
	{itemID: 1253, typ: PoisonTypeMild},      // IRON_SPEAR_P_
	{itemID: 1255, typ: PoisonTypeMild},      // STEEL_SPEAR_P_
	{itemID: 1257, typ: PoisonTypeMild},      // MITHRIL_SPEAR_P_
	{itemID: 1259, typ: PoisonTypeMild},      // ADAMANT_SPEAR_P_
	{itemID: 1261, typ: PoisonTypeMild},      // RUNE_SPEAR_P_
	{itemID: 1263, typ: PoisonTypeMild},      // DRAGON_SPEAR_P_
	{itemID: 3094, typ: PoisonTypeVeryWeak},  // BLACK_DART_P_
	{itemID: 4582, typ: PoisonTypeMild},      // BLACK_SPEAR_P_
	{itemID: 5616, typ: PoisonTypeWeak},      // BRONZE_ARROW_P_PLUS_
	{itemID: 5617, typ: PoisonTypeWeak},      // IRON_ARROW_P_PLUS_
	{itemID: 5618, typ: PoisonTypeWeak},      // STEEL_ARROW_P_PLUS_
	{itemID: 5619, typ: PoisonTypeWeak},      // MITHRIL_ARROW_P_PLUS_
	{itemID: 5620, typ: PoisonTypeWeak},      // ADAMANT_ARROW_P_PLUS_
	{itemID: 5621, typ: PoisonTypeWeak},      // RUNE_ARROW_P_PLUS_
	{itemID: 5622, typ: PoisonTypeMild},      // BRONZE_ARROW_P_PLUS_PLUS_
	{itemID: 5623, typ: PoisonTypeMild},      // IRON_ARROW_P_PLUS_PLUS_
	{itemID: 5624, typ: PoisonTypeMild},      // STEEL_ARROW_P_PLUS_PLUS_
	{itemID: 5625, typ: PoisonTypeMild},      // MITHRIL_ARROW_P_PLUS_PLUS_
	{itemID: 5626, typ: PoisonTypeMild},      // ADAMANT_ARROW_P_PLUS_PLUS_
	{itemID: 5627, typ: PoisonTypeMild},      // RUNE_ARROW_P_PLUS_PLUS_
	{itemID: 5628, typ: PoisonTypeWeak},      // BRONZE_DART_P_PLUS_
	{itemID: 5629, typ: PoisonTypeWeak},      // IRON_DART_P_PLUS_
	{itemID: 5630, typ: PoisonTypeWeak},      // STEEL_DART_P_PLUS_
	{itemID: 5631, typ: PoisonTypeWeak},      // BLACK_DART_P_PLUS_
	{itemID: 5632, typ: PoisonTypeWeak},      // MITHRIL_DART_P_PLUS_
	{itemID: 5633, typ: PoisonTypeWeak},      // ADAMANT_DART_P_PLUS_
	{itemID: 5634, typ: PoisonTypeWeak},      // RUNE_DART_P_PLUS_
	{itemID: 5635, typ: PoisonTypeMild},      // BRONZE_DART_P_PLUS_PLUS_
	{itemID: 5636, typ: PoisonTypeMild},      // IRON_DART_P_PLUS_PLUS_
	{itemID: 5637, typ: PoisonTypeMild},      // STEEL_DART_P_PLUS_PLUS_
	{itemID: 5638, typ: PoisonTypeMild},      // BLACK_DART_P_PLUS_PLUS_
	{itemID: 5639, typ: PoisonTypeMild},      // MITHRIL_DART_P_PLUS_PLUS_
	{itemID: 5640, typ: PoisonTypeMild},      // ADAMANT_DART_P_PLUS_PLUS_
	{itemID: 5641, typ: PoisonTypeMild},      // RUNE_DART_P_PLUS_PLUS_
	{itemID: 5642, typ: PoisonTypeWeak},      // BRONZE_JAVELIN_P_PLUS_
	{itemID: 5643, typ: PoisonTypeWeak},      // IRON_JAVELIN_P_PLUS_
	{itemID: 5644, typ: PoisonTypeWeak},      // STEEL_JAVELIN_P_PLUS_
	{itemID: 5645, typ: PoisonTypeWeak},      // MITHRIL_JAVELIN_P_PLUS_
	{itemID: 5646, typ: PoisonTypeWeak},      // ADAMANT_JAVELIN_P_PLUS_
	{itemID: 5647, typ: PoisonTypeWeak},      // RUNE_JAVELIN_P_PLUS_
	{itemID: 5648, typ: PoisonTypeMild},      // BRONZE_JAVELIN_P_PLUS_PLUS_
	{itemID: 5649, typ: PoisonTypeMild},      // IRON_JAVELIN_P_PLUS_PLUS_
	{itemID: 5650, typ: PoisonTypeMild},      // STEEL_JAVELIN_P_PLUS_PLUS_
	{itemID: 5651, typ: PoisonTypeMild},      // MITHRIL_JAVELIN_P_PLUS_PLUS_
	{itemID: 5652, typ: PoisonTypeMild},      // ADAMANT_JAVELIN_P_PLUS_PLUS_
	{itemID: 5653, typ: PoisonTypeMild},      // RUNE_JAVELIN_P_PLUS_PLUS_
	{itemID: 5654, typ: PoisonTypeWeak},      // BRONZE_KNIFE_P_PLUS_
	{itemID: 5655, typ: PoisonTypeWeak},      // IRON_KNIFE_P_PLUS_
	{itemID: 5656, typ: PoisonTypeWeak},      // STEEL_KNIFE_P_PLUS_
	{itemID: 5657, typ: PoisonTypeWeak},      // MITHRIL_KNIFE_P_PLUS_
	{itemID: 5658, typ: PoisonTypeWeak},      // BLACK_KNIFE_P_PLUS_
	{itemID: 5659, typ: PoisonTypeWeak},      // ADAMANT_KNIFE_P_PLUS_
	{itemID: 5660, typ: PoisonTypeWeak},      // RUNE_KNIFE_P_PLUS_
	{itemID: 5661, typ: PoisonTypeMild},      // BRONZE_KNIFE_P_PLUS_PLUS_
	{itemID: 5662, typ: PoisonTypeMild},      // IRON_KNIFE_P_PLUS_PLUS_
	{itemID: 5663, typ: PoisonTypeMild},      // STEEL_KNIFE_P_PLUS_PLUS_
	{itemID: 5664, typ: PoisonTypeMild},      // MITHRIL_KNIFE_P_PLUS_PLUS_
	{itemID: 5665, typ: PoisonTypeMild},      // BLACK_KNIFE_P_PLUS_PLUS_
	{itemID: 5666, typ: PoisonTypeMild},      // ADAMANT_KNIFE_P_PLUS_PLUS_
	{itemID: 5667, typ: PoisonTypeMild},      // RUNE_KNIFE_P_PLUS_PLUS_
	{itemID: 5668, typ: PoisonTypeExtra},     // IRON_DAGGER_P_PLUS_
	{itemID: 5670, typ: PoisonTypeExtra},     // BRONZE_DAGGER_P_PLUS_
	{itemID: 5672, typ: PoisonTypeExtra},     // STEEL_DAGGER_P_PLUS_
	{itemID: 5674, typ: PoisonTypeExtra},     // MITHRIL_DAGGER_P_PLUS_
	{itemID: 5676, typ: PoisonTypeExtra},     // ADAMANT_DAGGER_P_PLUS_
	{itemID: 5678, typ: PoisonTypeExtra},     // RUNE_DAGGER_P_PLUS_
	{itemID: 5680, typ: PoisonTypeExtra},     // DRAGON_DAGGER_P_PLUS_
	{itemID: 5682, typ: PoisonTypeExtra},     // BLACK_DAGGER_P_PLUS_
	{itemID: 5684, typ: PoisonTypeExtra},     // POISON_DAGGER_P_PLUS_
	{itemID: 5686, typ: PoisonTypeSuper},     // IRON_DAGGER_P_PLUS_PLUS_
	{itemID: 5688, typ: PoisonTypeSuper},     // BRONZE_DAGGER_P_PLUS_PLUS_
	{itemID: 5690, typ: PoisonTypeSuper},     // STEEL_DAGGER_P_PLUS_PLUS_
	{itemID: 5692, typ: PoisonTypeSuper},     // MITHRIL_DAGGER_P_PLUS_PLUS_
	{itemID: 5694, typ: PoisonTypeSuper},     // ADAMANT_DAGGER_P_PLUS_PLUS_
	{itemID: 5696, typ: PoisonTypeSuper},     // RUNE_DAGGER_P_PLUS_PLUS_
	{itemID: 5698, typ: PoisonTypeSuper},     // DRAGON_DAGGER_P_PLUS_PLUS_
	{itemID: 5700, typ: PoisonTypeSuper},     // BLACK_DAGGER_P_PLUS_PLUS_
	{itemID: 5702, typ: PoisonTypeSuper},     // POISON_DAGGER_P_PLUS_PLUS_
	{itemID: 5704, typ: PoisonTypeExtra},     // BRONZE_SPEAR_P_PLUS_
	{itemID: 5706, typ: PoisonTypeExtra},     // IRON_SPEAR_P_PLUS_
	{itemID: 5708, typ: PoisonTypeExtra},     // STEEL_SPEAR_P_PLUS_
	{itemID: 5710, typ: PoisonTypeExtra},     // MITHRIL_SPEAR_P_PLUS_
	{itemID: 5712, typ: PoisonTypeExtra},     // ADAMANT_SPEAR_P_PLUS_
	{itemID: 5714, typ: PoisonTypeExtra},     // RUNE_SPEAR_P_PLUS_
	{itemID: 5716, typ: PoisonTypeExtra},     // DRAGON_SPEAR_P_PLUS_
	{itemID: 5718, typ: PoisonTypeSuper},     // BRONZE_SPEAR_P_PLUS_PLUS_
	{itemID: 5720, typ: PoisonTypeSuper},     // IRON_SPEAR_P_PLUS_PLUS_
	{itemID: 5722, typ: PoisonTypeSuper},     // STEEL_SPEAR_P_PLUS_PLUS_
	{itemID: 5724, typ: PoisonTypeSuper},     // MITHRIL_SPEAR_P_PLUS_PLUS_
	{itemID: 5726, typ: PoisonTypeSuper},     // ADAMANT_SPEAR_P_PLUS_PLUS_
	{itemID: 5728, typ: PoisonTypeSuper},     // RUNE_SPEAR_P_PLUS_PLUS_
	{itemID: 5730, typ: PoisonTypeSuper},     // DRAGON_SPEAR_P_PLUS_PLUS_
	{itemID: 5734, typ: PoisonTypeExtra},     // BLACK_SPEAR_P_PLUS_
	{itemID: 5736, typ: PoisonTypeSuper},     // BLACK_SPEAR_P_PLUS_PLUS_
	{itemID: 6061, typ: PoisonTypeWeak},      // BRONZE_BOLTS_P_PLUS_
	{itemID: 6062, typ: PoisonTypeMild},      // BRONZE_BOLTS_P_PLUS_PLUS_
	{itemID: 6593, typ: PoisonTypeMild},      // WHITE_DAGGER_P_
	{itemID: 6595, typ: PoisonTypeExtra},     // WHITE_DAGGER_P_PLUS_
	{itemID: 6597, typ: PoisonTypeSuper},     // WHITE_DAGGER_P_PLUS_PLUS_
	{itemID: 8874, typ: PoisonTypeMild},      // BONE_DAGGER_P_
	{itemID: 8876, typ: PoisonTypeExtra},     // BONE_DAGGER_P_PLUS_
	{itemID: 8878, typ: PoisonTypeSuper},     // BONE_DAGGER_P_PLUS_PLUS_
	{itemID: 9286, typ: PoisonTypeVeryWeak},  // BLURITE_BOLTS_P_
	{itemID: 9287, typ: PoisonTypeVeryWeak},  // IRON_BOLTS_P_
	{itemID: 9288, typ: PoisonTypeVeryWeak},  // STEEL_BOLTS_P_
	{itemID: 9289, typ: PoisonTypeVeryWeak},  // MITHRIL_BOLTS_P_
	{itemID: 9290, typ: PoisonTypeVeryWeak},  // ADAMANT_BOLTS_P_
	{itemID: 9291, typ: PoisonTypeVeryWeak},  // RUNITE_BOLTS_P_
	{itemID: 9292, typ: PoisonTypeVeryWeak},  // SILVER_BOLTS_P_
	{itemID: 9293, typ: PoisonTypeWeak},      // BLURITE_BOLTS_P_PLUS_
	{itemID: 9294, typ: PoisonTypeWeak},      // IRON_BOLTS_P_PLUS_
	{itemID: 9295, typ: PoisonTypeWeak},      // STEEL_BOLTS_P_PLUS_
	{itemID: 9296, typ: PoisonTypeWeak},      // MITHRIL_BOLTS_P_PLUS_
	{itemID: 9297, typ: PoisonTypeWeak},      // ADAMANT_BOLTS_P_PLUS_
	{itemID: 9298, typ: PoisonTypeWeak},      // RUNITE_BOLTS_P_PLUS_
	{itemID: 9299, typ: PoisonTypeWeak},      // SILVER_BOLTS_P_PLUS_
	{itemID: 9300, typ: PoisonTypeMild},      // BLURITE_BOLTS_P_PLUS_PLUS_
	{itemID: 9301, typ: PoisonTypeMild},      // IRON_BOLTS_P_PLUS_PLUS_
	{itemID: 9302, typ: PoisonTypeMild},      // STEEL_BOLTS_P_PLUS_PLUS_
	{itemID: 9303, typ: PoisonTypeMild},      // MITHRIL_BOLTS_P_PLUS_PLUS_
	{itemID: 9304, typ: PoisonTypeMild},      // ADAMANT_BOLTS_P_PLUS_PLUS_
	{itemID: 9305, typ: PoisonTypeMild},      // RUNITE_BOLTS_P_PLUS_PLUS_
	{itemID: 9306, typ: PoisonTypeMild},      // SILVER_BOLTS_P_PLUS_PLUS_
	{itemID: 10582, typ: PoisonTypeMild},     // KERIS_P_
	{itemID: 10583, typ: PoisonTypeExtra},    // KERIS_P_PLUS_
	{itemID: 10584, typ: PoisonTypeSuper},    // KERIS_P_PLUS_PLUS_
	{itemID: 11227, typ: PoisonTypeVeryWeak}, // DRAGON_ARROW_P_
	{itemID: 11228, typ: PoisonTypeWeak},     // DRAGON_ARROW_P_PLUS_
	{itemID: 11229, typ: PoisonTypeMild},     // DRAGON_ARROW_P_PLUS_PLUS_
	{itemID: 11231, typ: PoisonTypeVeryWeak}, // DRAGON_DART_P_
	{itemID: 11233, typ: PoisonTypeWeak},     // DRAGON_DART_P_PLUS_
	{itemID: 11234, typ: PoisonTypeMild},     // DRAGON_DART_P_PLUS_PLUS_
	{itemID: 11379, typ: PoisonTypeMild},     // BRONZE_HASTA_P_
	{itemID: 11382, typ: PoisonTypeExtra},    // BRONZE_HASTA_P_PLUS_
	{itemID: 11384, typ: PoisonTypeSuper},    // BRONZE_HASTA_P_PLUS_PLUS_
	{itemID: 11386, typ: PoisonTypeMild},     // IRON_HASTA_P_
	{itemID: 11389, typ: PoisonTypeExtra},    // IRON_HASTA_P_PLUS_
	{itemID: 11391, typ: PoisonTypeSuper},    // IRON_HASTA_P_PLUS_PLUS_
	{itemID: 11393, typ: PoisonTypeMild},     // STEEL_HASTA_P_
	{itemID: 11396, typ: PoisonTypeExtra},    // STEEL_HASTA_P_PLUS_
	{itemID: 11398, typ: PoisonTypeSuper},    // STEEL_HASTA_P_PLUS_PLUS_
	{itemID: 11400, typ: PoisonTypeMild},     // MITHRIL_HASTA_P_
	{itemID: 11403, typ: PoisonTypeExtra},    // MITHRIL_HASTA_P_PLUS_
	{itemID: 11405, typ: PoisonTypeSuper},    // MITHRIL_HASTA_P_PLUS_PLUS_
	{itemID: 11407, typ: PoisonTypeMild},     // ADAMANT_HASTA_P_
	{itemID: 11410, typ: PoisonTypeExtra},    // ADAMANT_HASTA_P_PLUS_
	{itemID: 11412, typ: PoisonTypeSuper},    // ADAMANT_HASTA_P_PLUS_PLUS_
	{itemID: 11414, typ: PoisonTypeMild},     // RUNE_HASTA_P_
	{itemID: 11417, typ: PoisonTypeExtra},    // RUNE_HASTA_P_PLUS_
	{itemID: 11419, typ: PoisonTypeSuper},    // RUNE_HASTA_P_PLUS_PLUS_
	{itemID: 13267, typ: PoisonTypeMild},     // ABYSSAL_DAGGER_P_
	{itemID: 13269, typ: PoisonTypeExtra},    // ABYSSAL_DAGGER_P_PLUS_
	{itemID: 13271, typ: PoisonTypeSuper},    // ABYSSAL_DAGGER_P_PLUS_PLUS_
	{itemID: 22806, typ: PoisonTypeVeryWeak}, // DRAGON_KNIFE_P_
	{itemID: 22807, typ: PoisonTypeVeryWeak}, // DRAGON_KNIFE_P__2
	{itemID: 22808, typ: PoisonTypeWeak},     // DRAGON_KNIFE_P_PLUS_
	{itemID: 22809, typ: PoisonTypeWeak},     // DRAGON_KNIFE_P_PLUS__2
	{itemID: 22810, typ: PoisonTypeMild},     // DRAGON_KNIFE_P_PLUS_PLUS_
	{itemID: 22811, typ: PoisonTypeMild},     // DRAGON_KNIFE_P_PLUS_PLUS__2
	{itemID: 22734, typ: PoisonTypeMild},     // DRAGON_HASTA_P_
	{itemID: 22735, typ: PoisonTypeMild},     // DRAGON_HASTA_P__2
	{itemID: 22736, typ: PoisonTypeMild},     // DRAGON_HASTA_P__3
	{itemID: 22737, typ: PoisonTypeExtra},    // DRAGON_HASTA_P_PLUS_
	{itemID: 22738, typ: PoisonTypeExtra},    // DRAGON_HASTA_P_PLUS__2
	{itemID: 22739, typ: PoisonTypeExtra},    // DRAGON_HASTA_P_PLUS__3
	{itemID: 22740, typ: PoisonTypeSuper},    // DRAGON_HASTA_P_PLUS_PLUS_
	{itemID: 22741, typ: PoisonTypeSuper},    // DRAGON_HASTA_P_PLUS_PLUS__2
	{itemID: 22742, typ: PoisonTypeSuper},    // DRAGON_HASTA_P_PLUS_PLUS__3
	{itemID: 27863, typ: PoisonTypeMild},     // ABYSSAL_DAGGER_BH_P_
	{itemID: 27864, typ: PoisonTypeMild},     // ABYSSAL_DAGGER_BH_P__2
	{itemID: 27865, typ: PoisonTypeExtra},    // ABYSSAL_DAGGER_BH_P_PLUS_
	{itemID: 27866, typ: PoisonTypeExtra},    // ABYSSAL_DAGGER_BH_P_PLUS__2
	{itemID: 27867, typ: PoisonTypeSuper},    // ABYSSAL_DAGGER_BH_P_PLUS_PLUS_
	{itemID: 27868, typ: PoisonTypeSuper},    // ABYSSAL_DAGGER_BH_P_PLUS_PLUS__2
	{itemID: 3170, typ: PoisonTypeSuper},     // BRONZE_SPEAR_KP_
	{itemID: 3171, typ: PoisonTypeSuper},     // IRON_SPEAR_KP_
	{itemID: 3172, typ: PoisonTypeSuper},     // STEEL_SPEAR_KP_
	{itemID: 3173, typ: PoisonTypeSuper},     // MITHRIL_SPEAR_KP_
	{itemID: 3174, typ: PoisonTypeSuper},     // ADAMANT_SPEAR_KP_
	{itemID: 3175, typ: PoisonTypeSuper},     // RUNE_SPEAR_KP_
	{itemID: 3176, typ: PoisonTypeSuper},     // DRAGON_SPEAR_KP_
	{itemID: 4584, typ: PoisonTypeSuper},     // BLACK_SPEAR_KP_
	{itemID: 14796, typ: PoisonTypeSuper},    // BRONZE_SPEAR_KP__2
	{itemID: 14797, typ: PoisonTypeSuper},    // IRON_SPEAR_KP__2
	{itemID: 14798, typ: PoisonTypeSuper},    // STEEL_SPEAR_KP__2
	{itemID: 14800, typ: PoisonTypeSuper},    // MITHRIL_SPEAR_KP__2
	{itemID: 14801, typ: PoisonTypeSuper},    // ADAMANT_SPEAR_KP__2
	{itemID: 14802, typ: PoisonTypeSuper},    // RUNE_SPEAR_KP__2
	{itemID: 14803, typ: PoisonTypeSuper},    // DRAGON_SPEAR_KP__2
	{itemID: 14799, typ: PoisonTypeSuper},    // BLACK_SPEAR_KP__2
	{itemID: 11381, typ: PoisonTypeSuper},    // BRONZE_HASTA_KP_
	{itemID: 11388, typ: PoisonTypeSuper},    // IRON_HASTA_KP_
	{itemID: 11395, typ: PoisonTypeSuper},    // STEEL_HASTA_KP_
	{itemID: 11402, typ: PoisonTypeSuper},    // MITHRIL_HASTA_KP_
	{itemID: 11409, typ: PoisonTypeSuper},    // ADAMANT_HASTA_KP_
	{itemID: 11416, typ: PoisonTypeSuper},    // RUNE_HASTA_KP_
	{itemID: 22743, typ: PoisonTypeSuper},    // DRAGON_HASTA_KP_
	{itemID: 15561, typ: PoisonTypeSuper},    // BRONZE_HASTA_KP__2
	{itemID: 15565, typ: PoisonTypeSuper},    // IRON_HASTA_KP__2
	{itemID: 15569, typ: PoisonTypeSuper},    // STEEL_HASTA_KP__2
	{itemID: 15573, typ: PoisonTypeSuper},    // MITHRIL_HASTA_KP__2
	{itemID: 15577, typ: PoisonTypeSuper},    // ADAMANT_HASTA_KP__2
	{itemID: 15581, typ: PoisonTypeSuper},    // RUNE_HASTA_KP__2
	{itemID: 22745, typ: PoisonTypeSuper},    // DRAGON_HASTA_KP__2
	{itemID: 19486, typ: PoisonTypeVeryWeak}, // DRAGON_JAVELIN_P_
	{itemID: 19488, typ: PoisonTypeWeak},     // DRAGON_JAVELIN_P_PLUS_
	{itemID: 19490, typ: PoisonTypeMild},     // DRAGON_JAVELIN_P_PLUS_PLUS_
	{itemID: 21320, typ: PoisonTypeVeryWeak}, // AMETHYST_JAVELIN_P_
	{itemID: 21322, typ: PoisonTypeWeak},     // AMETHYST_JAVELIN_P_PLUS_
	{itemID: 21324, typ: PoisonTypeMild},     // AMETHYST_JAVELIN_P_PLUS_PLUS_
	{itemID: 21332, typ: PoisonTypeVeryWeak}, // AMETHYST_ARROW_P_
	{itemID: 21334, typ: PoisonTypeWeak},     // AMETHYST_ARROW_P_PLUS_
	{itemID: 21336, typ: PoisonTypeMild},     // AMETHYST_ARROW_P_PLUS_PLUS_
	{itemID: 21924, typ: PoisonTypeVeryWeak}, // DRAGON_BOLTS_P_
	{itemID: 21926, typ: PoisonTypeWeak},     // DRAGON_BOLTS_P_PLUS_
	{itemID: 21928, typ: PoisonTypeMild},     // DRAGON_BOLTS_P_PLUS_PLUS_
	{itemID: 25851, typ: PoisonTypeVeryWeak}, // AMETHYST_DART_P_
	{itemID: 25855, typ: PoisonTypeWeak},     // AMETHYST_DART_P_PLUS_
	{itemID: 25857, typ: PoisonTypeMild},     // AMETHYST_DART_P_PLUS_PLUS_
	{itemID: 12926, typ: PoisonTypeVenom},    // TOXIC_BLOWPIPE
	{itemID: 29796, typ: PoisonTypeVenom},    // NOXIOUS_HALBERD
	{itemID: 33178, typ: PoisonTypeVenom},    // NOXIOUS_HALBERD_4
}

// poisonTypeByItemID is the table as a map, built once at init.
var poisonTypeByItemID = func() map[int]PoisonType {
	m := make(map[int]PoisonType, len(poisonedItemTypes))
	for _, entry := range poisonedItemTypes {
		m[entry.itemID] = entry.typ
	}
	return m
}()

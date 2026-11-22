// Transcribed exception tables for the door catalog. The numeric ids
// come from the read-only ObjectIdentifiers.ts (see the per-table
// citations); the file itself is not copied. See catalog.go for the
// scan that consumes these tables.
package doors

// DoorNames is the set of cache display names whose locs the catalog
// scans for an "Open" action. Doors.plugin.js:30-52 (DOOR_NAMES).
var DoorNames = map[string]bool{
	"Door": true, "Doors": true, "Large door": true, "Large doors": true,
	"Castle door": true, "Cell Door": true, "Cell door": true,
	"Glass door": true, "Magic door": true, "Magic guild door": true,
	"Metal door": true, "Mind Door": true, "Mithril door": true,
	"Tent door": true, "Wall": true, "Guild door": true,
	"Gate": true, "Metal gate": true, "Wooden gate": true, "Doorway": true,
	"Tree Door": true, "Storeroom Door": true, "Ardougne Wall Door": true,
	"Vitur door": true, "Myrmel door": true, "Shadum door": true,
	"Jovkai door": true, "Chained door": true, "Trapdoor": true,
	"Longhall door": true, "Prison door": true, "Arena Entrance": true,
	"Arena Exit": true, "Curtain": true, "Tower door": true, "Kitchen gate": true,
}

// Nameless open door leaves: cache locs the generated
// ObjectIdentifiers has no property for, each the actionless,
// same-model leaf of a named closed door.
// Doors.plugin.js:56-96 (NAMELESS_DOOR_LEAF_IDS).
const (
	// Closed O.DOOR_47 (2000); same models, cache-derived.
	namelessDoor47Open = 2001
	// Closed O.DOOR_48 (2002); same models, cache-derived.
	namelessDoor48Open = 2003
	// plagueelenadoorshut, a multi (varbit 9016) with no name of its own.
	namelessPlagueHouseDoor = 37321
	// plagueelenadooropen.
	namelessPlagueHouseDoorOpen = 37322
	// Sins of the Father Arboretum; capture 1637.
	namelessLargeDoor100Open = 39252
	namelessLargeDoor101Open = 39254
	// guidorgatelopen.
	namelessGuidorGateLeftOpen = 2052
	// guidorgateropen.
	namelessGuidorGateRightOpen = 2053
	// inacmetalgateopenr, capture 1638.
	namelessMortMyreGateLeftOpen = 1574
	// inacmetalgateopenl, capture 1638.
	namelessMortMyreGateRightOpen = 1573
	// troll_stronghold_arena_entrance_inactive.
	namelessArenaEntranceOpen = 3784
	// troll_stronghold_arena_exit_inactive.
	namelessArenaExitOpen = 3787
	// troll_stronghold_prison_door_open, capture 740.
	namelessPrisonDoor9Open = 3781
	// Same non-mirrored gate model as 2438, with no options.
	namelessShipyardGateLeftOpen = 1566
	// Same mirrored gate model as 2439, with no options.
	namelessShipyardGateRightOpen = 1565
	// Grum's Gold Exchange; capture 465.
	namelessGrumDoorOpen = 1542
	// inacpip_underground_door; Priest in Peril capture 2026-11-02.
	namelessPipMausoleumGateOpen = 3446
	// inactivetempledoorl/r: Paterdomus' large doors open onto them.
	namelessPipTempleDoorLeftOpen  = 3491
	namelessPipTempleDoorRightOpen = 3492
	// inactive_pipprisondoor: Drezel's cell gate.
	namelessPipCellGateOpen = 3464
	// dragonsecretdoor_open; capture 2951.
	namelessCrandorSecretDoorOpen = 25039
	// Actionless collision blocker; captures 2951 and 740.
	namelessInviswall = 38848
	// inactiveposhdoor: Harold's door (3747) open, 233/03247.txt:4726.
	namelessHaroldDoorOpen = 1542
	// castledoor_inactive: the Death Plateau equipment room door (3743) open.
	namelessCastleDoorOpen = 3744
	// mournerquaters_gatelopen; Biohazard capture tick 557.
	namelessMournerCageGateLeftOpen = 2059
	// mournerquaters_gateropen.
	namelessMournerCageGateRightOpen = 2061
	// inactivepoordoor: Melzar's front door and the Mining Guild doors
	// open onto it.
	namelessMelzarDoorOpen = 1539
	// grim_inactiveposhdoor: Witch's House front and back doors open onto it.
	namelessWitchHouseDoorOpen = 24719
	// kebos_farming_guild_door_left_open/_right_open.
	namelessFarmingGuildDoorLeftOpen  = 34465
	namelessFarmingGuildDoorRightOpen = 34466
)

// selfOpeningDoorIDs: doors with no open variant in the cache; OSRS
// opens them by rotating the same loc (open id == closed id).
// Doors.plugin.js:111-154 (SELF_OPENING_DOOR_IDS). TS lists O.DOOR_67
// twice (37 entries); the duplicate is dropped here, leaving 36 unique.
var selfOpeningDoorIDs = map[int]bool{
	9398:  true, // O.DOOR_223
	9709:  true, // O.DOOR_225
	9710:  true, // O.DOOR_226
	9716:  true, // O.DOOR_227
	9721:  true, // O.DOOR_228
	9722:  true, // O.DOOR_229
	9723:  true, // O.DOOR_230
	9724:  true, // O.DOOR_231
	2184:  true, // O.DOOR_67
	11724: true, // O.DOOR_283
	11665: true, // O.GUILD_DOOR
	11728: true, // O.DOOR_287
	15056: true, // O.DOOR_358
	2394:  true, // O.GATE_52
	2606:  true, // O.WALL_9
	3747:  true, // O.DOOR_115
	3743:  true, // O.LARGE_DOOR_23
	61824: true, // O.DOOR_831
	61838: true, // O.VITUR_DOOR_4
	61821: true, // O.JOVKAI_DOOR
	61825: true, // O.MYRMEL_DOOR
	61829: true, // O.MYRMEL_DOOR_3
	61832: true, // O.SHADUM_DOOR_3
	61834: true, // O.VITUR_DOOR
	61835: true, // O.VITUR_DOOR_2
	48774: true, // O.GATE_270
	61854: true, // O.GATE_327
	8738:  true, // O.ARDOUGNE_WALL_DOOR
	8739:  true, // O.ARDOUGNE_WALL_DOOR_2
	49140: true, // O.GATE_282
	2631:  true, // O.DOOR_99
	2623:  true, // O.GATE_57
	1805:  true, // O.DOOR_45
	22:    true, // O.DOOR_3
	24:    true, // O.DOOR_4
	40178: true, // O.DOOR_624
}

// singleDoorOpenIDs: single doors the scan cannot pair (open variant
// is not closedId+1, has no "Close", or is inverted). Applied after the
// scan; a shared open leaf keeps the door the scan already paired it
// with (first wins on the reverse map). Doors.plugin.js:159-208
// (SINGLE_DOOR_OPEN_IDS). STOREROOM_DOOR / TOWER_DOOR open to id+1;
// DOOR_47 / DOOR_48 open to the nameless leaves 2001 / 2003.
var singleDoorOpenIDs = map[int]int{
	1579:  1581,               // O.TRAPDOOR_4 -> O.TRAPDOOR_6
	1580:  1582,               // O.TRAPDOOR_5 -> O.TRAPDOOR_7
	1517:  1520,               // O.LARGE_DOOR_11 -> O.LARGE_DOOR_14
	6977:  6978,               // O.DOOR_180 -> O.DOOR_181
	24686: 24685,              // O.DOOR_499 -> O.DOOR_498
	18168: 11617,              // O.DOOR_389 -> O.DOOR_279
	4636:  4639,               // O.DOOR_129 -> O.DOOR_132
	4637:  4639,               // O.DOOR_130 -> O.DOOR_132
	11196: 4639,               // O.DOOR_271 -> O.DOOR_132
	11197: 4639,               // O.DOOR_272 -> O.DOOR_132
	61842: 61843,              // O.DOOR_837 -> O.DOOR_838
	53441: 53442,              // O.GATE_301 -> O.GATE_302
	48301: 48302,              // O.GATE_268 -> O.GATE_269
	3810:  3811,               // O.STOREROOM_DOOR -> O.STOREROOM_DOOR + 1
	21814: 21815,              // O.TOWER_DOOR -> O.TOWER_DOOR + 1
	39512: 39513,              // O.GATE_210 -> O.GATE_211
	2000:  namelessDoor47Open, // O.DOOR_47 -> nameless 2001
	2002:  namelessDoor48Open, // O.DOOR_48 -> nameless 2003
	11620: 11624,              // O.LONGHALL_DOOR -> O.LONGHALL_DOOR_3
	11621: 11625,              // O.LONGHALL_DOOR_2 -> O.LONGHALL_DOOR_4
	12348: 1544,               // O.DOOR_302 -> O.DOOR_31
}

// inPlaceDoorIDs: curtains and floor trapdoors whose open loc replaces
// the closed one on the same tile. Doors.plugin.js:213 (IN_PLACE_DOOR_IDS).
var inPlaceDoorIDs = map[int]bool{
	1533: true, // O.CURTAIN_3
	1579: true, // O.TRAPDOOR_4
	1580: true, // O.TRAPDOOR_5
}

// ownTileDoorIDs: doors that swing to the far side of their wall; the
// open leaf stays on the closed leaf's tile, turned a quarter. The scan
// adds every door whose gameval name ends in "_reverse" to this set at
// runtime. Doors.plugin.js:221 (OWN_TILE_DOOR_IDS).
var ownTileDoorIDs = map[int]bool{
	21403: true, // O.GATE_155
	21405: true, // O.GATE_157
	11728: true, // O.DOOR_287
	4636:  true, // O.DOOR_129
	4637:  true, // O.DOOR_130
	11196: true, // O.DOOR_271
	11197: true, // O.DOOR_272
}

// compositeDoorOpenings: one loc opens into two (or one) leaves, each
// [id, dx, dy, face] relative to the closed loc's tile.
// Doors.plugin.js:225-234 (COMPOSITE_DOOR_OPENINGS).
var compositeDoorOpenings = map[int][]CompositeLeaf{
	190:  {{ID: 191, DX: 0, DY: 0, Face: 2}, {ID: 192, DX: 3, DY: 0, Face: 0}}, // O.GATE_16: Tree Gnome Stronghold main gate, one 5x2 loc -> two nameless 2x2 leaves
	2054: {{ID: 2055, DX: 0, DY: -1, Face: 2}},                                 // O.DOOR_55: cabinet house shortcut, nameless open leaf swings east
	2537: {{ID: 2538, DX: 0, DY: 1, Face: 0}},                                  // O.DOOR_85 -> O.DOOR_86: Rehnison entrance shortcut, swing west
}

// dorgeshCityDoorIDs: the Dorgesh-Kaan city entrance leaves belong to
// LocTeleports, not to this catalog's swing.
// Doors.plugin.js:236 (DORGESH_CITY_DOOR_IDS).
var dorgeshCityDoorIDs = map[int]bool{
	6919: true, // O.DOOR_176
	6920: true, // O.DOOR_177
}

// woodenGates: OSRS wooden gates are two locs (hinge + extension)
// that pivot together around the hinge post; opening/closing moves both
// pieces. Doors.plugin.js:247-266 (WOODEN_GATES). The nameless open
// extensions 4314, 12819, 2263, 2264 have no ObjectIdentifiers property.
var woodenGates = []WoodenGate{
	{ClosedHinge: 47, ClosedExtension: 48, OpenHinge: 49, OpenExtension: 50},             // O.GATE_4, O.GATE_5, O.GATE_6, O.GATE_7
	{ClosedHinge: 883, ClosedExtension: 23917, OpenHinge: 23918, OpenExtension: 23919},   // O.GATE_17, O.GATE_167, O.GATE_168, O.GATE_169
	{ClosedHinge: 1558, ClosedExtension: 1560, OpenHinge: 1559, OpenExtension: 1567},     // O.GATE_18, O.GATE_20, O.GATE_19, O.GATE_25
	{ClosedHinge: 1561, ClosedExtension: 1562, OpenHinge: 1563, OpenExtension: 1564},     // O.GATE_21, O.GATE_22, O.GATE_23, O.GATE_24
	{ClosedHinge: 3725, ClosedExtension: 3726, OpenHinge: 3727, OpenExtension: 3728},     // O.GATE_74, O.GATE_75, O.GATE_76, O.GATE_77
	{ClosedHinge: 4311, ClosedExtension: 4312, OpenHinge: 4313, OpenExtension: 4314},     // O.GATE_78, O.GATE_79, O.GATE_80, nameless
	{ClosedHinge: 8810, ClosedExtension: 8811, OpenHinge: 8812, OpenExtension: 8813},     // O.GATE_83, O.GATE_84, O.GATE_85, O.GATE_86
	{ClosedHinge: 12816, ClosedExtension: 12817, OpenHinge: 12818, OpenExtension: 12819}, // O.GATE_113, O.GATE_114, O.GATE_115, nameless
	{ClosedHinge: 12986, ClosedExtension: 12987, OpenHinge: 12988, OpenExtension: 12989}, // O.GATE_116, O.GATE_117, O.GATE_118, O.GATE_119
	{ClosedHinge: 15514, ClosedExtension: 15516, OpenHinge: 15511, OpenExtension: 15513}, // O.GATE_140, O.GATE_142, O.GATE_137, O.GATE_139
	{ClosedHinge: 44920, ClosedExtension: 44921, OpenHinge: 44922, OpenExtension: 44923}, // O.GATE_243, O.GATE_244, O.GATE_245, O.GATE_246
	{ClosedHinge: 60763, ClosedExtension: 60760, OpenHinge: 60761, OpenExtension: 60762}, // O.GATE_322, O.GATE_319, O.GATE_320, O.GATE_321
	{ClosedHinge: 9470, ClosedExtension: 9708, OpenHinge: 8812, OpenExtension: 8813},     // O.GATE_90, O.GATE_91, O.GATE_85, O.GATE_86
	{ClosedHinge: 2261, ClosedExtension: 2262, OpenHinge: 2263, OpenExtension: 2264},     // O.WOODEN_GATE, O.WOODEN_GATE_2, nameless 2263, nameless 2264
}

// doubleDoorIDFamilies: closed and open leaves of double doors found
// three ways in TS (this table, SPECIAL_DOUBLE_DOOR_PAIRS, and the
// scan's same-model / mirrored-leaf detection). A leaf only acts as a
// double door where both leaves stand side by side; a lone one stays a
// single door. Doors.plugin.js:277-332 (DOUBLE_DOOR_ID_FAMILIES).
var doubleDoorIDFamilies = [][]int{
	{24059, 24064, 24065, 24066}, // O.CASTLE_DOOR, O.CASTLE_DOOR_6, O.CASTLE_DOOR_7, O.CASTLE_DOOR_8
	{32404, 32405},               // O.LARGE_DOORS, O.LARGE_DOORS_2
	{32406, 32407},               // O.LARGE_DOORS_3, O.LARGE_DOORS_4
	{1511, 1513, 1512, 1516},     // O.LARGE_DOOR_7, O.LARGE_DOOR_9, O.LARGE_DOOR_8, O.LARGE_DOOR_10
	{1516, 1519},                 // O.LARGE_DOOR_10, O.LARGE_DOOR_13
	{1727, 1728, 1571, 1572},     // O.GATE_33, O.GATE_34, O.GATE_29, O.GATE_30
	{14751, 14752, 14753, 14754}, // O.DOOR_354, O.DOOR_355, O.DOOR_356, O.DOOR_357
	{1521, 1522, 1524, 1525},     // O.LARGE_DOOR_15, O.LARGE_DOOR_16, O.LARGE_DOOR_17, O.LARGE_DOOR_18
	{1551, 1552},                 // O.DOOR_36, O.DOOR_37
	{1558, 1559},                 // O.GATE_18, O.GATE_19
	{1568, 1569, 1571, 1572},     // O.GATE_26, O.GATE_27, O.GATE_29, O.GATE_30
	{1589, 1590, 1591},           // O.DOORWAY_2, O.DOORWAY_3, O.DOORWAY_4
	{4423, 4424, 4425, 4426},     // O.LARGE_DOOR_24, O.LARGE_DOOR_25, O.LARGE_DOOR_26, O.LARGE_DOOR_27
	{4427, 4428, 4429, 4430},     // O.LARGE_DOOR_28, O.LARGE_DOOR_29, O.LARGE_DOOR_30, O.LARGE_DOOR_31
	{2039, 2041, 1571, 1572},     // O.GATE_40, O.GATE_41, O.GATE_29, O.GATE_30
	{25748, 25750, 25749, 25751}, // O.LARGE_DOOR_79, O.LARGE_DOOR_80, nameless, nameless
	{26130, 26131, 26132, 26133}, // O.GATE_177, O.GATE_178, O.GATE_179, O.GATE_180
	{39251, 39253, 39252, 39254}, // O.LARGE_DOOR_100, O.LARGE_DOOR_101, NAMELESS_LARGE_DOOR_100_OPEN, NAMELESS_LARGE_DOOR_101_OPEN
	{9717, 9718, 1571, 1572},     // O.GATE_92, O.GATE_93, O.GATE_29, O.GATE_30
	{9719, 9720, 1571, 1572},     // O.GATE_94, O.GATE_95, O.GATE_29, O.GATE_30
	{52, 53, 28853, 28854},       // O.GATE_8, O.GATE_9, nameless, nameless
	{30387, 30388, 1522, 1525},   // O.LARGE_DOOR_98, O.LARGE_DOOR_99, O.LARGE_DOOR_16, O.LARGE_DOOR_18
	{3489, 3490, 3491, 3492},     // O.LARGE_DOOR_21, O.LARGE_DOOR_22, NAMELESS_PIP_TEMPLE_DOOR_LEFT_OPEN, NAMELESS_PIP_TEMPLE_DOOR_RIGHT_OPEN
	{44036, 44038, 44037, 44039}, // O.GATE_232, O.GATE_233, nameless, nameless
	{48278, 48279, 48282, 48283}, // O.DOORS_21, O.DOORS_22, O.DOORS_25, O.DOORS_26
	{48289, 48290, 48291, 48292}, // O.DOORS_29, O.DOORS_30, O.DOORS_31, O.DOORS_32
}

// specialDoubleDoorPairs: both leaves of each pair are partners of the
// other. Doors.plugin.js:333-375 (SPECIAL_DOUBLE_DOOR_PAIRS). 36
// entries / 35 unique: [O.GATE_232, O.GATE_233] is listed twice.
var specialDoubleDoorPairs = [][2]int{
	{24059, 24064}, // O.CASTLE_DOOR, O.CASTLE_DOOR_6
	{24065, 24066}, // O.CASTLE_DOOR_7, O.CASTLE_DOOR_8
	{32404, 32405}, // O.LARGE_DOORS, O.LARGE_DOORS_2
	{32406, 32407}, // O.LARGE_DOORS_3, O.LARGE_DOORS_4
	{1568, 1569},   // O.GATE_26, O.GATE_27
	{1571, 1572},   // O.GATE_29, O.GATE_30
	{1727, 1728},   // O.GATE_33, O.GATE_34
	{14751, 14752}, // O.DOOR_354, O.DOOR_355
	{14751, 14754}, // O.DOOR_354, O.DOOR_357
	{14752, 14753}, // O.DOOR_355, O.DOOR_356
	{14753, 14754}, // O.DOOR_356, O.DOOR_357
	{2039, 2041},   // O.GATE_40, O.GATE_41
	{9717, 9718},   // O.GATE_92, O.GATE_93
	{9719, 9720},   // O.GATE_94, O.GATE_95
	{52, 53},       // O.GATE_8, O.GATE_9
	{44036, 44038}, // O.GATE_232, O.GATE_233
	{48278, 48279}, // O.DOORS_21, O.DOORS_22
	{48282, 48283}, // O.DOORS_25, O.DOORS_26
	{48289, 48290}, // O.DOORS_29, O.DOORS_30
	{48291, 48292}, // O.DOORS_31, O.DOORS_32
	{44037, 44039}, // 44037, 44039
	{4423, 4424},   // O.LARGE_DOOR_24, O.LARGE_DOOR_25
	{4425, 4426},   // O.LARGE_DOOR_26, O.LARGE_DOOR_27
	{4428, 4427},   // O.LARGE_DOOR_29, O.LARGE_DOOR_28
	{4430, 4429},   // O.LARGE_DOOR_31, O.LARGE_DOOR_30
	{30387, 30388}, // O.LARGE_DOOR_98, O.LARGE_DOOR_99
	{3489, 3490},   // O.LARGE_DOOR_21, O.LARGE_DOOR_22
	{44036, 44038}, // O.GATE_232, O.GATE_233
	{1522, 1525},   // O.LARGE_DOOR_16, O.LARGE_DOOR_18
	{1511, 1513},   // O.LARGE_DOOR_7, O.LARGE_DOOR_9
	{1512, 1516},   // O.LARGE_DOOR_8, O.LARGE_DOOR_10
	{25748, 25750}, // O.LARGE_DOOR_79, O.LARGE_DOOR_80
	{25749, 25751}, // 25749, 25751
	{26130, 26131}, // O.GATE_177, O.GATE_178
	{26132, 26133}, // O.GATE_179, O.GATE_180
	{39251, 39253}, // O.LARGE_DOOR_100, O.LARGE_DOOR_101
}

// specialDoubleDoorOpenIDsByClosedID: the open leaf each closed double-
// door leaf swings into, where it is not closedId+1.
// Doors.plugin.js:381-418 (SPECIAL_DOUBLE_DOOR_OPEN_IDS_BY_CLOSED_ID).
var specialDoubleDoorOpenIDsByClosedID = map[int]int{
	24059: 24065, // O.CASTLE_DOOR -> O.CASTLE_DOOR_7
	24064: 24066, // O.CASTLE_DOOR_6 -> O.CASTLE_DOOR_8
	32404: 32404, // O.LARGE_DOORS -> O.LARGE_DOORS
	32405: 32405, // O.LARGE_DOORS_2 -> O.LARGE_DOORS_2
	32406: 32406, // O.LARGE_DOORS_3 -> O.LARGE_DOORS_3
	32407: 32407, // O.LARGE_DOORS_4 -> O.LARGE_DOORS_4
	1568:  1571,  // O.GATE_26 -> O.GATE_29
	1569:  1572,  // O.GATE_27 -> O.GATE_30
	1727:  1571,  // O.GATE_33 -> O.GATE_29
	1728:  1572,  // O.GATE_34 -> O.GATE_30
	14751: 14753, // O.DOOR_354 -> O.DOOR_356
	14752: 14754, // O.DOOR_355 -> O.DOOR_357
	2039:  1571,  // O.GATE_40 -> O.GATE_29
	2041:  1572,  // O.GATE_41 -> O.GATE_30
	9717:  1571,  // O.GATE_92 -> O.GATE_29
	9718:  1572,  // O.GATE_93 -> O.GATE_30
	9719:  1571,  // O.GATE_94 -> O.GATE_29
	9720:  1572,  // O.GATE_95 -> O.GATE_30
	52:    28853, // O.GATE_8 -> nameless
	53:    28854, // O.GATE_9 -> nameless
	4423:  4425,  // O.LARGE_DOOR_24 -> O.LARGE_DOOR_26
	4424:  4426,  // O.LARGE_DOOR_25 -> O.LARGE_DOOR_27
	4428:  4430,  // O.LARGE_DOOR_29 -> O.LARGE_DOOR_31
	4427:  4429,  // O.LARGE_DOOR_28 -> O.LARGE_DOOR_30
	30387: 1522,  // O.LARGE_DOOR_98 -> O.LARGE_DOOR_16
	3489:  3491,  // O.LARGE_DOOR_21 -> NAMELESS_PIP_TEMPLE_DOOR_LEFT_OPEN
	3490:  3492,  // O.LARGE_DOOR_22 -> NAMELESS_PIP_TEMPLE_DOOR_RIGHT_OPEN
	44036: 44037, // O.GATE_232 -> nameless
	44038: 44039, // O.GATE_233 -> nameless
	48278: 48282, // O.DOORS_21 -> O.DOORS_25
	48279: 48283, // O.DOORS_22 -> O.DOORS_26
	48289: 48291, // O.DOORS_29 -> O.DOORS_31
	48290: 48292, // O.DOORS_30 -> O.DOORS_32
	26130: 26132, // O.GATE_177 -> O.GATE_179
	26131: 26133, // O.GATE_178 -> O.GATE_180
	30388: 1525,  // O.LARGE_DOOR_99 -> O.LARGE_DOOR_18
	1511:  1512,  // O.LARGE_DOOR_7 -> O.LARGE_DOOR_8
	1513:  1516,  // O.LARGE_DOOR_9 -> O.LARGE_DOOR_10
}

// passThroughGates: gates that let the player through and shut behind
// them, never staying open. Each leaf is [id, x, y, z, face] of a
// shape-0 wall loc (a sixth entry names another shape, for a closed
// centrepiece); crossX/crossY is the boundary the player crosses.
// Doors.plugin.js:427-860 (PASS_THROUGH_GATES). TS lists 29 array
// elements; the Mining Guild spread expands to 3 gates, so 31 gates
// land here. OpenSoundID 0 means TS sets no sound for the gate.
var passThroughGates = []*PassThroughGate{
	// Death Plateau: Harold's door (233/03247.txt:4726-4744).
	{
		Closed: []PassThroughLeaf{{ID: 3747, X: 2906, Y: 3543, Z: 1, Face: 3, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 2906, Y: 3543, Z: 1, Face: 3, Shape: 0}, {ID: namelessHaroldDoorOpen, X: 2906, Y: 3542, Z: 1, Face: 0, Shape: 0}},
		CrossY: 3543, HasCrossY: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 81,
	},
	// Death Plateau equipment room door (233/03252.txt:36053-36090).
	{
		Closed: []PassThroughLeaf{{ID: 3743, X: 2894, Y: 3566, Z: 0, Face: 1, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 2894, Y: 3566, Z: 0, Face: 1, Shape: 0}, {ID: namelessCastleDoorOpen, X: 2894, Y: 3567, Z: 0, Face: 2, Shape: 0}},
		CrossY: 3567, HasCrossY: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 81,
	},
	// Grand Tree prison door.
	{
		Closed: []PassThroughLeaf{{ID: 3367, X: 2465, Y: 3496, Z: 3, Face: 0, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 2465, Y: 3496, Z: 3, Face: 0, Shape: 0}, {ID: 3367, X: 2466, Y: 3496, Z: 3, Face: 1, Shape: 0}},
		CrossX: 2465, HasCrossX: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 69,
	},
	// Shipyard gate.
	{
		Closed: []PassThroughLeaf{{ID: 2438, X: 2945, Y: 3041, Z: 0, Face: 0, Shape: 0}, {ID: 2439, X: 2945, Y: 3042, Z: 0, Face: 0, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 2945, Y: 3041, Z: 0, Face: 0, Shape: 0}, {ID: namelessInviswall, X: 2945, Y: 3042, Z: 0, Face: 0, Shape: 0}, {ID: namelessShipyardGateLeftOpen, X: 2946, Y: 3041, Z: 0, Face: 3, Shape: 0}, {ID: namelessShipyardGateRightOpen, X: 2946, Y: 3042, Z: 0, Face: 1, Shape: 0}},
		CrossX: 2945, HasCrossX: true,
		WalkOver:   true,
		CloseTicks: 2,
	},
	// Mourner HQ upstairs cage (Biohazard capture ticks 557, 580).
	{
		Closed: []PassThroughLeaf{{ID: 2060, X: 2551, Y: 3325, Z: 1, Face: 2, Shape: 0}, {ID: 2058, X: 2551, Y: 3326, Z: 1, Face: 2, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 2551, Y: 3325, Z: 1, Face: 2, Shape: 0}, {ID: namelessInviswall, X: 2551, Y: 3326, Z: 1, Face: 2, Shape: 0}, {ID: namelessMournerCageGateLeftOpen, X: 2552, Y: 3325, Z: 1, Face: 3, Shape: 0}, {ID: namelessMournerCageGateRightOpen, X: 2552, Y: 3326, Z: 1, Face: 1, Shape: 0}},
		CrossX: 2552, HasCrossX: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 69,
	},
	// Farming Guild entrance (rsprox 239 ticks 1648-1651, 1692 ticks 939-942).
	{
		Closed: []PassThroughLeaf{{ID: 34463, X: 1248, Y: 3723, Z: 0, Face: 3, Shape: 0}, {ID: 34464, X: 1249, Y: 3723, Z: 0, Face: 3, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 1249, Y: 3723, Z: 0, Face: 3, Shape: 0}, {ID: namelessFarmingGuildDoorRightOpen, X: 1249, Y: 3722, Z: 0, Face: 2, Shape: 0}, {ID: namelessInviswall, X: 1248, Y: 3723, Z: 0, Face: 3, Shape: 0}, {ID: namelessFarmingGuildDoorLeftOpen, X: 1248, Y: 3722, Z: 0, Face: 0, Shape: 0}},
		CrossY: 3723, HasCrossY: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 62,
	},
	// Observatory Quest kitchen gate (RSProx 20261110T110141 ticks 856-858, 888-890, 914-920).
	{
		Closed: []PassThroughLeaf{{ID: 2199, X: 2327, Y: 9393, Z: 0, Face: 1, Shape: 0}, {ID: 2200, X: 2326, Y: 9393, Z: 0, Face: 1, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 2327, Y: 9393, Z: 0, Face: 1, Shape: 0}, {ID: namelessMortMyreGateLeftOpen, X: 2327, Y: 9394, Z: 0, Face: 2, Shape: 0}, {ID: namelessInviswall, X: 2326, Y: 9393, Z: 0, Face: 1, Shape: 0}, {ID: namelessMortMyreGateRightOpen, X: 2326, Y: 9394, Z: 0, Face: 0, Shape: 0}},
		CrossY: 9394, HasCrossY: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 71,
	},
	// Mining Guild door O.DOOR_562 (233/03241.txt tick 12674; 233/03239.txt ticks 581, 2234).
	{
		Closed: []PassThroughLeaf{{ID: 30364, X: 3046, Y: 9756, Z: 0, Face: 1, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 3046, Y: 9756, Z: 0, Face: 1, Shape: 0}, {ID: namelessMelzarDoorOpen, X: 3046, Y: 9757, Z: 0, Face: 2, Shape: 0}},
		CrossY: 9757, HasCrossY: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 81,
	},
	// Mining Guild door O.DOOR_563 (233/03241.txt tick 12674; 233/03239.txt ticks 581, 2234).
	{
		Closed: []PassThroughLeaf{{ID: 30365, X: 3019, Y: 9732, Z: 0, Face: 1, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 3019, Y: 9732, Z: 0, Face: 1, Shape: 0}, {ID: namelessMelzarDoorOpen, X: 3019, Y: 9733, Z: 0, Face: 2, Shape: 0}},
		CrossY: 9733, HasCrossY: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 81,
	},
	// Mining Guild door O.DOOR_564 (233/03241.txt tick 12674; 233/03239.txt ticks 581, 2234).
	{
		Closed: []PassThroughLeaf{{ID: 30366, X: 3043, Y: 9729, Z: 0, Face: 1, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 3043, Y: 9729, Z: 0, Face: 1, Shape: 0}, {ID: namelessMelzarDoorOpen, X: 3043, Y: 9730, Z: 0, Face: 2, Shape: 0}},
		CrossY: 9730, HasCrossY: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 81,
	},
	// Waterfall: Golrie's panel (xWBSnGkQTi4 3:53.2-3:54.4).
	{
		Closed: []PassThroughLeaf{{ID: 1991, X: 2515, Y: 9575, Z: 0, Face: 1, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 2515, Y: 9575, Z: 0, Face: 1, Shape: 0}, {ID: 1991, X: 2515, Y: 9576, Z: 0, Face: 2, Shape: 0}},
		CrossY: 9576, HasCrossY: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 62,
	},
	// Guidor's gate, south-east Varrock (rsprox 1663, 3218, 3220).
	{
		Closed: []PassThroughLeaf{{ID: 2050, X: 3264, Y: 3405, Z: 0, Face: 0, Shape: 0}, {ID: 2051, X: 3264, Y: 3406, Z: 0, Face: 0, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessGuidorGateLeftOpen, X: 3263, Y: 3405, Z: 0, Face: 3, Shape: 0}, {ID: namelessGuidorGateRightOpen, X: 3262, Y: 3405, Z: 0, Face: 3, Shape: 0}},
		CrossX: 3264, HasCrossX: true,
		CloseTicks: 2,
	},
	// West Ardougne wall gate (NOT CAPTURED: crossing, timing and open faces are inferred).
	{
		Closed:       []PassThroughLeaf{{ID: 8739, X: 2557, Y: 3299, Z: 0, Face: 2, Shape: 10}, {ID: 8738, X: 2558, Y: 3299, Z: 0, Face: 0, Shape: 10}, {ID: 8738, X: 2557, Y: 3300, Z: 0, Face: 2, Shape: 10}, {ID: 8739, X: 2558, Y: 3300, Z: 0, Face: 0, Shape: 10}},
		Opened:       []PassThroughLeaf{{ID: 8741, X: 2557, Y: 3299, Z: 0, Face: 3, Shape: 0}, {ID: 8740, X: 2558, Y: 3299, Z: 0, Face: 3, Shape: 0}, {ID: 8740, X: 2557, Y: 3300, Z: 0, Face: 1, Shape: 0}, {ID: 8741, X: 2558, Y: 3300, Z: 0, Face: 1, Shape: 0}},
		CrossBetween: [2]int{2556, 2559}, HasCrossBetween: true,
		CrossRows:  []int{3299, 3300},
		CloseTicks: 5,
	},
	// Mort Myre gate (capture 1638 ticks 1005/1014/1016).
	{
		Closed: []PassThroughLeaf{{ID: 3507, X: 3443, Y: 3458, Z: 0, Face: 3, Shape: 0}, {ID: 3506, X: 3444, Y: 3458, Z: 0, Face: 3, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 3443, Y: 3458, Z: 0, Face: 3, Shape: 0}, {ID: namelessMortMyreGateLeftOpen, X: 3443, Y: 3457, Z: 0, Face: 0, Shape: 0}, {ID: namelessInviswall, X: 3444, Y: 3458, Z: 0, Face: 3, Shape: 0}, {ID: namelessMortMyreGateRightOpen, X: 3444, Y: 3457, Z: 0, Face: 2, Shape: 0}},
		CrossY: 3458, HasCrossY: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 71,
	},
	// Troll arena entrance, west wall (no capture; Mort Myre layout).
	{
		Closed: []PassThroughLeaf{{ID: 3782, X: 2897, Y: 3618, Z: 0, Face: 0, Shape: 0}, {ID: 3783, X: 2897, Y: 3619, Z: 0, Face: 0, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessArenaEntranceOpen, X: 2896, Y: 3618, Z: 0, Face: 3, Shape: 0}, {ID: namelessArenaEntranceOpen, X: 2896, Y: 3619, Z: 0, Face: 1, Shape: 0}},
		CrossX: 2897, HasCrossX: true,
		CloseTicks: 2,
	},
	// Troll arena exit, north wall (no capture; Mort Myre layout).
	{
		Closed: []PassThroughLeaf{{ID: 3785, X: 2916, Y: 3629, Z: 0, Face: 3, Shape: 0}, {ID: 3786, X: 2917, Y: 3629, Z: 0, Face: 3, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessArenaExitOpen, X: 2916, Y: 3628, Z: 0, Face: 0, Shape: 0}, {ID: namelessArenaExitOpen, X: 2917, Y: 3628, Z: 0, Face: 2, Shape: 0}, {ID: namelessInviswall, X: 2916, Y: 3629, Z: 0, Face: 3, Shape: 0}, {ID: namelessInviswall, X: 2917, Y: 3629, Z: 0, Face: 3, Shape: 0}},
		CrossY: 3629, HasCrossY: true,
		Carry:      true,
		CloseTicks: 2,
	},
	// Troll Stronghold prison door (capture 740 ticks 1470-1473).
	{
		Closed: []PassThroughLeaf{{ID: 3780, X: 2848, Y: 10107, Z: 1, Face: 0, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessPrisonDoor9Open, X: 2847, Y: 10107, Z: 1, Face: 1, Shape: 0}, {ID: namelessInviswall, X: 2848, Y: 10107, Z: 1, Face: 0, Shape: 0}},
		CrossX: 2848, HasCrossX: true,
		Carry:       true,
		CloseTicks:  2,
		OpenSoundID: 62,
	},
	// Grum's Gold Exchange, Port Sarim (capture 465 ticks 9416-9421 in, 9476-9479 out).
	{
		Closed: []PassThroughLeaf{{ID: 59, X: 3016, Y: 3246, Z: 0, Face: 0, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessGrumDoorOpen, X: 3015, Y: 3246, Z: 0, Face: 1, Shape: 0}},
		CrossX: 3016, HasCrossX: true,
		CloseTicks:  2,
		OpenSoundID: 62,
	},
	// Priest in Peril mausoleum gate (RSProx 2026-10-17 tick 1117; 2026-11-02 ticks 11016, 11664-11666, 11708-11710).
	{
		Closed: []PassThroughLeaf{{ID: 3444, X: 3405, Y: 9895, Z: 0, Face: 3, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 3405, Y: 9895, Z: 0, Face: 3, Shape: 0}, {ID: namelessPipMausoleumGateOpen, X: 3405, Y: 9894, Z: 0, Face: 0, Shape: 0}},
		CrossY: 9895, HasCrossY: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 69,
	},
	// Priest in Peril mausoleum gate, second.
	{
		Closed: []PassThroughLeaf{{ID: 3445, X: 3431, Y: 9897, Z: 0, Face: 2, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 3431, Y: 9897, Z: 0, Face: 2, Shape: 0}, {ID: namelessPipMausoleumGateOpen, X: 3432, Y: 9897, Z: 0, Face: 3, Shape: 0}},
		CrossX: 3432, HasCrossX: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 69,
	},
	// Paterdomus large doors (Priest in Peril capture 2026-11-02, in ticks 10840-10847, out 10980-10982).
	{
		Closed: []PassThroughLeaf{{ID: 3490, X: 3408, Y: 3488, Z: 0, Face: 2, Shape: 0}, {ID: 3489, X: 3408, Y: 3489, Z: 0, Face: 2, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 3408, Y: 3488, Z: 0, Face: 2, Shape: 0}, {ID: namelessPipTempleDoorRightOpen, X: 3409, Y: 3488, Z: 0, Face: 3, Shape: 0}, {ID: namelessInviswall, X: 3408, Y: 3489, Z: 0, Face: 2, Shape: 0}, {ID: namelessPipTempleDoorLeftOpen, X: 3409, Y: 3489, Z: 0, Face: 1, Shape: 0}},
		CrossX: 3409, HasCrossX: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 62,
	},
	// Drezel's cell gate (capture 2026-10-17 ticks 1311-1314 in, 1315-1317 out).
	{
		Closed: []PassThroughLeaf{{ID: 3463, X: 3415, Y: 3489, Z: 2, Face: 2, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 3415, Y: 3489, Z: 2, Face: 2, Shape: 0}, {ID: namelessPipCellGateOpen, X: 3416, Y: 3489, Z: 2, Face: 3, Shape: 0}},
		CrossX: 3416, HasCrossX: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 71,
	},
	// Necrovarus's tomb door, Temple of Phasmatys (Ghosts Ahoy capture 20261112T092603 ticks 1691-1693 in, 1711-1713 out).
	{
		Closed: []PassThroughLeaf{{ID: 5244, X: 3656, Y: 3514, Z: 1, Face: 0, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 3656, Y: 3514, Z: 1, Face: 0, Shape: 0}, {ID: 5245, X: 3655, Y: 3514, Z: 1, Face: 1, Shape: 0}},
		CrossX: 3656, HasCrossX: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 56,
	},
	// Champions' Guild door, Varrock (revision 242 capture 20261106T101002, ticks 969-973 in, 1029-1031 and 1106-1108 out).
	{
		Closed: []PassThroughLeaf{{ID: 1805, X: 3191, Y: 3363, Z: 0, Face: 3, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 3191, Y: 3363, Z: 0, Face: 3, Shape: 0}, {ID: 23637, X: 3191, Y: 3362, Z: 0, Face: 0, Shape: 0}},
		CrossY: 3363, HasCrossY: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 46,
	},
	// Tower of Life entrance (no capture; mirrors the Champions' Guild door).
	{
		Closed: []PassThroughLeaf{{ID: 21814, X: 2649, Y: 3225, Z: 0, Face: 3, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 2649, Y: 3225, Z: 0, Face: 3, Shape: 0}, {ID: 21815, X: 2649, Y: 3224, Z: 0, Face: 0, Shape: 0}},
		CrossY: 3225, HasCrossY: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 62,
	},
	// Witch's House front door (revision 242 capture 20261108T030409, ticks 154-156).
	{
		Closed: []PassThroughLeaf{{ID: 2861, X: 2900, Y: 3473, Z: 0, Face: 2, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 2900, Y: 3473, Z: 0, Face: 2, Shape: 0}, {ID: namelessWitchHouseDoorOpen, X: 2901, Y: 3473, Z: 0, Face: 3, Shape: 0}},
		CrossX: 2901, HasCrossX: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 62,
	},
	// Witch's House back door (same capture, ticks 389-391).
	{
		Closed: []PassThroughLeaf{{ID: 2862, X: 2901, Y: 3465, Z: 0, Face: 1, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 2901, Y: 3465, Z: 0, Face: 1, Shape: 0}, {ID: namelessWitchHouseDoorOpen, X: 2900, Y: 3466, Z: 0, Face: 2, Shape: 0}},
		CrossY: 3466, HasCrossY: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 62,
	},
	// Witch's House shed door (same capture, ticks 652-654).
	{
		Closed: []PassThroughLeaf{{ID: 2863, X: 2934, Y: 3463, Z: 0, Face: 0, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 2934, Y: 3463, Z: 0, Face: 0, Shape: 0}, {ID: namelessMelzarDoorOpen, X: 2933, Y: 3463, Z: 0, Face: 1, Shape: 0}},
		CrossX: 2934, HasCrossX: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 62,
	},
	// Witch's House basement shock gate (same capture, ticks 215-217).
	{
		Closed: []PassThroughLeaf{{ID: 2866, X: 2902, Y: 9873, Z: 0, Face: 2, Shape: 0}, {ID: 2865, X: 2902, Y: 9874, Z: 0, Face: 2, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessInviswall, X: 2902, Y: 9873, Z: 0, Face: 2, Shape: 0}, {ID: namelessMortMyreGateLeftOpen, X: 2903, Y: 9873, Z: 0, Face: 3, Shape: 0}, {ID: namelessInviswall, X: 2902, Y: 9874, Z: 0, Face: 2, Shape: 0}, {ID: namelessMortMyreGateRightOpen, X: 2903, Y: 9874, Z: 0, Face: 1, Shape: 0}},
		CrossX: 2903, HasCrossX: true,
		WalkOver:    true,
		CloseTicks:  2,
		OpenSoundID: 71,
	},
	// Ranging Guild entrance (captures 229/02601.txt ticks 59-69, 230/02966.txt ticks 1560-1567, 232/03142.txt).
	{
		Closed:               []PassThroughLeaf{{ID: 11665, X: 2658, Y: 3438, Z: 0, Face: 0, Shape: 9}},
		Opened:               []PassThroughLeaf{{ID: namelessInviswall, X: 2658, Y: 3438, Z: 0, Face: 0, Shape: 9}, {ID: namelessMelzarDoorOpen, X: 2658, Y: 3439, Z: 0, Face: 1, Shape: 9}},
		CrossDiagonalOutside: [2]int{2657, 3439}, CrossDiagonalInside: [2]int{2659, 3437}, HasCrossDiagonal: true,
		CrossDiagonalEastward: true, // eastwardFrom: here.x-here.y <= closed.x-closed.y
		CloseTicks:            2,
		OpenSoundID:           67,
	},
	// Dragon Slayer secret wall into Crandor's dungeon (capture 2951 ticks 85-89).
	{
		Closed: []PassThroughLeaf{{ID: 2606, X: 2836, Y: 9600, Z: 0, Face: 3, Shape: 0}},
		Opened: []PassThroughLeaf{{ID: namelessCrandorSecretDoorOpen, X: 2836, Y: 9599, Z: 0, Face: 0, Shape: 0}, {ID: namelessInviswall, X: 2836, Y: 9600, Z: 0, Face: 3, Shape: 0}},
		CrossY: 9600, HasCrossY: true,
		Carry:       true,
		CloseTicks:  2,
		OpenSoundID: 2410,
	},
}

// passThroughFamilies: pass-through doors placed many times, as a
// family: every placement of the door works like a PASS_THROUGH_GATES
// entry built from its own tile and face. Open puts INVISWALL on the
// door's tile and its open leaf on the tile past the edge the door
// blocks, turned a quarter (face + 1); the opener walks over the
// blocker and the door is back two ticks later. The open leaf is the
// cache's `<name>_open` loc where no explicit leaf is given (empty
// OpenLeaf: resolve it from the gameval name at runtime).
// Doors.plugin.js:871-889 (PASS_THROUGH_FAMILIES). Defaults:
// OpenSoundID 62, CloseTicks 2.
var passThroughFamilies = map[int]*PassThroughFamily{
	2002:  {OpenLeaf: namelessDoor48Open, HasOpenLeaf: true, OpenSoundID: 62, CloseTicks: 2},          // O.DOOR_48 -> namelessDoor48Open
	2595:  {OpenLeaf: namelessMelzarDoorOpen, HasOpenLeaf: true, OpenSoundID: 62, CloseTicks: 2},      // O.DOOR_91 -> namelessMelzarDoorOpen
	2596:  {OpenSoundID: 62, CloseTicks: 2},                                                           // O.RED_DOOR: open leaf is the cache <name>_open loc
	2597:  {OpenSoundID: 62, CloseTicks: 2},                                                           // O.ORANGE_DOOR: open leaf is the cache <name>_open loc
	2598:  {OpenSoundID: 62, CloseTicks: 2},                                                           // O.YELLOW_DOOR: open leaf is the cache <name>_open loc
	2599:  {OpenSoundID: 62, CloseTicks: 2},                                                           // O.BLUE_DOOR: open leaf is the cache <name>_open loc
	2600:  {OpenSoundID: 62, CloseTicks: 2},                                                           // O.MAGENTA_DOOR: open leaf is the cache <name>_open loc
	2601:  {OpenSoundID: 62, CloseTicks: 2},                                                           // O.GREEN_DOOR: open leaf is the cache <name>_open loc
	2602:  {OpenSoundID: 62, CloseTicks: 2},                                                           // O.EXIT_DOOR: open leaf is the cache <name>_open loc
	2036:  {OpenLeaf: 2037, HasOpenLeaf: true, OpenSoundID: 62, CloseTicks: 2},                        // O.DOOR_51 -> O.DOOR_52
	2034:  {OpenLeaf: 2035, HasOpenLeaf: true, OpenSoundID: 62, CloseTicks: 2},                        // O.DOOR_49 -> O.DOOR_50
	2537:  {OpenLeaf: 2538, HasOpenLeaf: true, OpenSoundID: 62, CloseTicks: 2},                        // O.DOOR_85 -> O.DOOR_86
	2528:  {OpenLeaf: 2529, HasOpenLeaf: true, OpenSoundID: 62, CloseTicks: 2},                        // O.DOOR_80 -> O.DOOR_81
	37321: {OpenLeaf: namelessPlagueHouseDoorOpen, HasOpenLeaf: true, OpenSoundID: 81, CloseTicks: 2}, // namelessPlagueHouseDoor -> namelessPlagueHouseDoorOpen
	2526:  {OpenLeaf: 2527, HasOpenLeaf: true, OpenSoundID: 67, CloseTicks: 2},                        // O.DOOR_78 -> O.DOOR_79
}

package game

import "strings"

// Autocasting: the port of game/content/combat/magic/Autocasting.ts
// (333 lines) — the autocast spell map, the combat-tab /
// autocast-selector widget actions, the selector, the
// indicators, and the set/clear paths. Spell casts themselves
// are the magic combat method (combat_magic.go); this file is
// the selection front-end only.
//
// SPELL RESOLUTION. The map below is slot → spell id; the ids
// were script-extracted (brace-counted) from CombatSpells.ts at
// bfd81e3 — the plan's warning applies: a windowed regex bled
// across statics (WATER_STRIKE read as 1156, BIND as 1163).
// AutocastSpell resolves through game.GetCombatSpell (contract
// C1, Worker A): standard-book slots resolve once the standard
// table lands; ancient slots (31-46) and arceuus slots (53-58)
// resolve to nil until their books land, and are documented as
// such — no spell is invented for them. A slot whose spell is
// not in the registry reads nil exactly as TS's map lookup
// reads null for an unregistered static.
//
// SPELLBOOK OF A SPELL. The CombatSpell interface carries no
// spellbook accessor (Worker A's contract adds SpellID() only),
// so the book is derived from the slot: 31-46 ancient, 53-58
// arceuus, everything else normal. A spell that also implements
// Spellbook() Spellbook answers directly (a future interface
// addition costs nothing here).
//
// WHAT IS NOT HERE. BonusManager.update (setAutocast's last
// call, Autocasting.ts:278) has no Go form and needs none:
// Go computes bonuses live through ComputeBonuses (bonus.go),
// so there is no cached array to refresh. Barrows.hasDamnedSet
// (the Ahrim's-set exception to the ancient-staff refusal) has
// no Go Barrows system, so the ancient refusal stands even with
// the set — a named gap. ItemAliases (baseOf/same/lookup) has
// no Go port and no registrations at bfd81e3 (no
// ItemAliases.register call exists in the checked-out source),
// so id sets match base ids only; a Last Man Standing copy
// (br_ancient_staff, br_sotd) does not resolve to its base.

// Autocast widget ids (Autocasting.ts): the combat tab group
// (593, CombatInterfaceGroup) and the autocast selector group
// (201) with its container child (1).
const (
	// autocastInterfaceGroup is AUTOCAST_INTERFACE (201).
	autocastInterfaceGroup = 201
	// autocastContainerChild is AUTOCAST_CONTAINER (1).
	autocastContainerChild = 1
	// autocastSelectorVarp is varp 664: the selector's spell
	// list (cache scripts 2098 and 243).
	autocastSelectorVarp = 664
	// autocastCombatModeVarp is COM_MODE_VARP (43): 0-3 for
	// the weapon's styles, 4 while a spell is autocast.
	// It is CombatStyleVarp; named here for the selector's
	// fight-type send.
	autocastCombatModeVarp = CombatStyleVarp
)

// autocastSpellIDs is AUTOCAST_SPELLS (Autocasting.ts) as slot
// → spell id. Slots 1-16 are the strikes/bolts/blasts/waves,
// 48-51 the surges, 17 Crumble Undead, 18 Magic Dart, 19/20/52
// the god spells, 47 Iban Blast, 31-46 the ancients, 53-58 the
// arceuus spells (enum_1986 for the god slots; cache script
// 4133 for the arceuus rows).
var autocastSpellIDs = map[int]int{
	1:  1152,  // WIND_STRIKE
	2:  1154,  // WATER_STRIKE
	3:  1156,  // EARTH_STRIKE
	4:  1158,  // FIRE_STRIKE
	5:  1160,  // WIND_BOLT
	6:  1163,  // WATER_BOLT
	7:  1166,  // EARTH_BOLT
	8:  1169,  // FIRE_BOLT
	9:  1172,  // WIND_BLAST
	10: 1175,  // WATER_BLAST
	11: 1177,  // EARTH_BLAST
	12: 1181,  // FIRE_BLAST
	13: 1183,  // WIND_WAVE
	14: 1185,  // WATER_WAVE
	15: 1188,  // EARTH_WAVE
	16: 1189,  // FIRE_WAVE
	48: 21876, // WIND_SURGE
	49: 21877, // WATER_SURGE
	50: 21878, // EARTH_SURGE
	51: 21879, // FIRE_SURGE
	17: 1171,  // CRUMBLE_UNDEAD
	18: 12037, // MAGIC_DART
	19: 1191,  // CLAWS_OF_GUTHIX
	20: 1192,  // FLAMES_OF_ZAMORAK
	52: 1190,  // SARADOMIN_STRIKE
	47: 1539,  // IBAN_BLAST
	// Ancients (CombatAncientSpellExtend, 11th positional arg
	// is the spell id): resolve to nil until the Ancient book
	// lands.
	31: 12939, // SMOKE_RUSH
	32: 12987, // SHADOW_RUSH
	33: 12901, // BLOOD_RUSH
	34: 12861, // ICE_RUSH
	35: 12963, // SMOKE_BURST
	36: 13011, // SHADOW_BURST
	37: 12919, // BLOOD_BURST
	38: 12881, // ICE_BURST
	39: 12951, // SMOKE_BLITZ
	40: 12999, // SHADOW_BLITZ
	41: 12911, // BLOOD_BLITZ
	42: 12871, // ICE_BLITZ
	43: 12975, // SMOKE_BARRAGE
	44: 13023, // SHADOW_BARRAGE
	45: 12929, // BLOOD_BARRAGE
	46: 12891, // ICE_BARRAGE
	// Arceuus: resolve to nil until the Arceuus book lands.
	53: 20398, // INFERIOR_DEMONBANE
	54: 20399, // SUPERIOR_DEMONBANE
	55: 20400, // DARK_DEMONBANE
	56: 21826, // GHOSTLY_GRASP
	57: 21829, // SKELETAL_GRASP
	58: 21832, // UNDEAD_GRASP
}

// ancientAutocastStaffs is ANCIENT_SPELL_AUTOCAST_STAFFS
// (Autocasting.ts): the weapons that autocast Ancient
// Magicks, by base id (ItemIdentifiers.ts). The Last Man
// Standing copy (29849, BLUE_MOON_SPEAR_4) is listed
// verbatim, as TS does.
var ancientAutocastStaffs = map[int]bool{
	21006: true, // KODAI_WAND
	6914:  true, // MASTER_WAND
	4675:  true, // ANCIENT_STAFF
	24422: true, // NIGHTMARE_STAFF
	24424: true, // VOLATILE_NIGHTMARE_STAFF
	24425: true, // ELDRITCH_NIGHTMARE_STAFF
	22647: true, // ZURIELS_STAFF
	27920: true, // ZURIELS_STAFF_BH_
	28988: true, // BLUE_MOON_SPEAR
	29849: true, // BLUE_MOON_SPEAR_4
	33253: true, // SHADOWFLAME_QUADRANT
}

// arceuusAutocastWeapons is ARCEUUS_AUTOCAST_WEAPONS
// (Autocasting.ts): the weapons that can autocast Arceuus
// spells, by name (any charge or degrade). Matched exactly or
// as a name prefix followed by a space (canAutocastArceuus).
var arceuusAutocastWeapons = []string{
	"ahrim's staff",
	"blue moon spear",
	"kodai wand",
	"master wand",
	"purging staff",
	"skull sceptre",
	"slayer's staff",
	"staff of the dead",
	"toxic staff of the dead",
	"shadowflame quadrant",
}

// ownSelectorLists is OWN_SELECTOR_LISTS (Autocasting.ts): the
// weapons with their own selector list, by base id
// (ItemIdentifiers.ts) — the list id the weapon copies, or its
// own id. The bladed staves and Iban's staff have their own
// lists (cache autocast_spellpos); live sends 11791 for the
// Staff of the Dead.
var ownSelectorLists = map[int]int{
	11791: 11791, // STAFF_OF_THE_DEAD
	12902: 11791, // (12902, an unnamed Staff of the Dead variant)
	12904: 11791, // TOXIC_STAFF_OF_THE_DEAD
	22296: 22296, // STAFF_OF_LIGHT
	24144: 24144, // STAFF_OF_BALANCE
	8841:  8841,  // VOID_KNIGHT_MACE
	24181: 8841,  // VOID_KNIGHT_MACE_L_
	1409:  1409,  // IBANS_STAFF
	12658: 1409,  // IBANS_STAFF_U_
	33330: 1409,  // IBANS_STAFF_O_
	33332: 1409,  // IBANS_STAFF_U_O_
	27788: 27788, // THAMMARONS_SCEPTRE_A_
	27679: 27679, // ACCURSED_SCEPTRE_A_
}

// godAutocastSlots is GOD_SLOTS (Autocasting.ts): the god-spell
// slots, which only the matching god's staff may choose.
var godAutocastSlots = map[int]bool{19: true, 20: true, 52: true}

// standardSelectorSlots is STANDARD_SLOTS (Autocasting.ts):
// the slots the standard (varp 664 = -1) list places.
var standardSelectorSlots = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 48, 49, 50, 51}

// waveAndSurgeSlots is WAVE_AND_SURGE_SLOTS (Autocasting.ts).
var waveAndSurgeSlots = []int{13, 14, 15, 16, 48, 49, 50, 51}

// selectorSlotsForList is SELECTOR_SLOTS (Autocasting.ts): the
// slots each varp 664 list places (cache script 243
// autocast_spellpos, with 4512 the standard list and the
// ancient_ and arceuus_autocast_spellpos procs); the others are
// hidden. An unknown list falls back to the standard set, as
// TS's `??` does.
func selectorSlotsForList(list int) map[int]bool {
	slots := make(map[int]bool)
	add := func(ids ...int) {
		for _, id := range ids {
			slots[id] = true
		}
	}
	switch list {
	case -1:
		add(standardSelectorSlots...)
	case 4675: // ANCIENT_STAFF
		add(31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46)
	case 9013: // SKULL_SCEPTRE (arceuus)
		add(53, 54, 55, 56, 57, 58)
	case 4170: // SLAYERS_STAFF
		add(17, 18)
		add(waveAndSurgeSlots...)
	case 8841: // VOID_KNIGHT_MACE
		add(17, 19)
		add(waveAndSurgeSlots...)
	case 1409: // IBANS_STAFF
		add(standardSelectorSlots...)
		add(47)
	case 11791, 22296, 24144: // STAFF_OF_THE_DEAD, STAFF_OF_LIGHT, STAFF_OF_BALANCE
		add(standardSelectorSlots...)
		add(17, 18, 19, 20, 52)
	case 27788, 27679: // THAMMARONS_SCEPTRE_A_, ACCURSED_SCEPTRE_A_
		add(standardSelectorSlots...)
		add(20)
	default:
		add(standardSelectorSlots...)
	}
	return slots
}

// AutocastSpell returns the spell for an autocast slot
// (Autocasting.autocastSpell): nil for an unlisted slot or a
// spell the registry does not hold — ancient/arceuus slots
// read nil until their books land.
func AutocastSpell(index int) CombatSpell {
	id, ok := autocastSpellIDs[index]
	if !ok {
		return nil
	}
	return GetCombatSpell(id)
}

// autocastSpellIndex is resolveAutocastIndex: the slot a spell
// sits in, or 0 when it is not in the table (also the TS
// fallback). The spell's id is read through SpellID() when the
// implementation carries it (contract C1); otherwise the slot
// is found by pointer identity against the registry's
// singletons, as TS compares mappedSpell === spell.
func autocastSpellIndex(spell CombatSpell) int {
	if spell == nil {
		return 0
	}
	if withID, ok := spell.(interface{ SpellID() int }); ok {
		id := withID.SpellID()
		for index, spellID := range autocastSpellIDs {
			if spellID == id {
				return index
			}
		}
		return 0
	}
	for index, id := range autocastSpellIDs {
		if GetCombatSpell(id) == spell {
			return index
		}
	}
	return 0
}

// autocastSpellBookOfIndex returns the book a slot's spell
// belongs to: 31-46 ancient, 53-58 arceuus, everything else
// normal.
func autocastSpellBookOfIndex(index int) Spellbook {
	switch {
	case index >= 31 && index <= 46:
		return SpellbookAncient
	case index >= 53 && index <= 58:
		return SpellbookArceuus
	default:
		return SpellbookNormal
	}
}

// spellbookOfSpell returns the book a spell belongs to: the
// spell's own answer when it carries one, else the book of
// the slot it sits in (normal for a spell outside the table).
func spellbookOfSpell(spell CombatSpell) Spellbook {
	if spell == nil {
		return SpellbookNormal
	}
	if withBook, ok := spell.(interface{ Spellbook() Spellbook }); ok {
		return withBook.Spellbook()
	}
	return autocastSpellBookOfIndex(autocastSpellIndex(spell))
}

// ownSelectorList is ownList: the weapon's own selector list,
// or the list of the item it copies. No ItemAliases port
// exists (see the file header), so only the base ids in
// ownSelectorLists match.
func ownSelectorList(weaponID int) (int, bool) {
	list, ok := ownSelectorLists[weaponID]
	return list, ok
}

// selectorList is selectorList: what varp 664 is set to when
// the selector opens, for this spellbook and weapon. -1 the
// standard spellbook, 4675 the ancient one, 4170 the Slayer's
// staff's list, 9013 (the skull sceptre's entry) the Arceuus
// one, plus per-weapon own lists.
func selectorList(book Spellbook, weaponID int) int {
	if book == SpellbookAncient {
		return 4675 // ANCIENT_STAFF
	}
	if book == SpellbookArceuus {
		return 9013 // SKULL_SCEPTRE
	}
	if weaponID == 4170 || weaponID == 21255 { // SLAYERS_STAFF, SLAYERS_STAFF_E_
		return 4170
	}
	if list, ok := ownSelectorList(weaponID); ok {
		return list
	}
	return -1
}

// canAutocastArceuus is canAutocastArceuus: whether a weapon
// name may autocast Arceuus spells (exact match, or the name
// followed by a space — a charged/degraded form).
func canAutocastArceuus(weaponName string) bool {
	name := strings.TrimSpace(strings.ToLower(weaponName))
	for _, weapon := range arceuusAutocastWeapons {
		if name == weapon || strings.HasPrefix(name, weapon+" ") {
			return true
		}
	}
	return false
}

// weaponRefusal is weaponRefusal: why the wielded weapon can't
// autocast spells from book, or "" when it can. Only the
// ancient staff family autocasts Ancient Magicks (the Staff of
// the Dead and Staff of Light don't), and the Ancient staff
// autocasts nothing else. The Barrows damned-set exception
// (Ahrim's) is a named gap: no Barrows system exists in Go, so
// the refusal stands even with the set.
func weaponRefusal(p *Player, book Spellbook) string {
	weaponID := equippedWeaponID(p)
	if book == SpellbookAncient && !ancientAutocastStaffs[weaponID] {
		return "You cannot autocast Ancient Magicks with that."
	}
	if book == SpellbookNormal && weaponID == 4675 { // ANCIENT_STAFF
		return "You can only autocast ancient magicks with that."
	}
	if book == SpellbookArceuus && !canAutocastArceuus(p.ItemName(weaponID)) {
		return "You can only autocast regular offensive spells with this staff."
	}
	return ""
}

// godSpellID is godSpell: the god spell a weapon casts, by its
// spell id — Saradomin (1190), Guthix (1191), Zamorak (1192) —
// or 0 when the weapon casts no god spell. Ids are
// ItemIdentifiers.ts; the base id only (no ItemAliases port,
// see the file header).
func godSpellID(weaponID int) int {
	switch weaponID {
	case 2415, 22296: // SARADOMIN_STAFF, STAFF_OF_LIGHT
		return 1190
	case 2416, 24144, 8841, 24181: // GUTHIX_STAFF, STAFF_OF_BALANCE, VOID_KNIGHT_MACE, VOID_KNIGHT_MACE_L_
		return 1191
	case 2417, 11791, 12902, 12904, 27788, 27679: // ZAMORAK_STAFF, STAFF_OF_THE_DEAD, 12902, TOXIC_STAFF_OF_THE_DEAD, THAMMARONS_SCEPTRE_A_, ACCURSED_SCEPTRE_A_
		return 1192
	default:
		return 0
	}
}

// godSpell returns the god spell a weapon casts, or nil.
func godSpell(weaponID int) CombatSpell {
	if id := godSpellID(weaponID); id != 0 {
		return GetCombatSpell(id)
	}
	return nil
}

// equippedWeaponID returns the wielded weapon's item id, or -1
// with nothing wielded.
func equippedWeaponID(p *Player) int {
	if p == nil || p.Equipment() == nil {
		return -1
	}
	return p.Equipment().GetWeapon().ID()
}

// autocastSender is the outbound sender surface the autocast
// selector uses, after CombatStyleSender: the sub-interface
// mount and the flags range, plus the varbit/tab sends the
// selector shares with the style paths. *Session implements
// all four methods.
type autocastSender interface {
	SendVarbit(id, value int)
	SendTabInterface(tabID, interfaceID int)
	SendSubInterface(targetUID, groupID, typ int)
	SendInterfaceFlagsRange(uid, fromSlot, toSlot, flags int)
}

// autocastSenderOf returns the player's autocast sender, or
// nil when the player is nil or the session does not
// implement it (the combatStyleSender assertion pattern).
func (p *Player) autocastSender() autocastSender {
	if p == nil {
		return nil
	}
	sender, _ := p.Session.(autocastSender)
	return sender
}

// HandleAutocastWidgetAction answers the autocast widget
// actions (Autocasting.handleWidgetAction): combat interface
// 593 child 26 clears autocast; children 23/28 open the
// selector; autocast interface 201 child 1 picks a slot
// (slot 0 = back to the combat interface). Anything else
// returns false so the click falls through to the next
// handler. slot is the widget-action slot, or -1 when the
// packet carries none (the decoder's absent-slot value); a
// 201 pick without a slot is not a pick.
//
// The listeners' routeWidgetAction calls this beside
// HandleStyleButton (listeners.go): the one-line call is
// Worker B's integration step — this function is the
// complete branch.
func HandleAutocastWidgetAction(p *Player, groupID, childID, slot int) bool {
	if p == nil {
		return false
	}
	if groupID == CombatInterfaceGroup && childID == 26 {
		SetAutocast(p, nil)
		return true
	}
	if groupID == CombatInterfaceGroup && (childID == 23 || childID == 28) {
		return openAutocastSelector(p, childID == 23)
	}
	if groupID != autocastInterfaceGroup || childID != autocastContainerChild || slot < 0 {
		return false
	}
	if slot == 0 {
		openCombatInterface(p)
		return true
	}
	spell := AutocastSpell(slot)
	// A god spell is in the bladed staves' own lists only,
	// each with its own god's spell; the plain god staves'
	// standard list has no god slots (Wiki: the Zamorak staff
	// cannot autocast Flames of Zamorak).
	weaponID := equippedWeaponID(p)
	if spell != nil && godAutocastSlots[slot] {
		if _, ok := ownSelectorList(weaponID); !ok || godSpell(weaponID) != spell {
			spell = nil
		}
	}
	// Only a slot the open list places, from the player's own
	// book, can be chosen (a modified client can send any).
	if spell != nil && (spellbookOfSpell(spell) != p.Spellbook() ||
		!selectorSlotsForList(selectorList(p.Spellbook(), weaponID))[slot]) {
		spell = nil
	}
	if spell != nil {
		SetAutocast(p, spell)
	}
	openCombatInterface(p)
	return true
}

// openAutocastSelector is openSelector: Lunar refusal, staff
// check, per-book weapon refusal, then the fight-type config
// + varp 664 selector list + sub-interface 201 + interface
// flags. It always returns true: every path claims the click.
func openAutocastSelector(p *Player, defensive bool) bool {
	if p.Spellbook() == SpellbookLunar {
		// rsprox_transcripts/234/03297.txt tick 212.
		p.SendMessage("You cannot autocast Lunar Magicks.")
		clearAutocastForRefusal(p)
		return true
	}
	if !hasStaffEquipped(p) {
		p.SendMessage("You need to equip a staff to autocast spells.")
		return true
	}

	weaponID := equippedWeaponID(p)
	if refusal := weaponRefusal(p, p.Spellbook()); refusal != "" {
		p.SendMessage(refusal)
		clearAutocastForRefusal(p)
		return true
	}

	fightType := selectorFightType(p, defensive)
	p.GetCombat().SetFightType(fightType)
	p.SendVarp(autocastCombatModeVarp, fightType.ChildID)
	p.SendVarp(autocastSelectorVarp, selectorList(p.Spellbook(), weaponID))
	if sender := p.autocastSender(); sender != nil {
		sender.SendSubInterface((161<<16)|76, autocastInterfaceGroup, 1)
		// No line: none of the 53 recordings that open the
		// selector sends one.
		sender.SendInterfaceFlagsRange((autocastInterfaceGroup<<16)|autocastContainerChild, 0, 64, 1<<1)
	}
	return true
}

// selectorFightType is selectorFightType: the weapon's own
// defensive or aggressive style (Focus/Pound on a staff,
// Fend/Swipe on a bladed staff), falling back to STAFF_FOCUS
// / STAFF_POUND when the weapon offers no style of the wanted
// stance. The registry fallback (no installed registry) has
// no STAFF_* types, so the fallback chain continues to the
// weapon's lowest style, then UnarmedPunch — a degeneration
// TS never hits, since its statics always exist.
func selectorFightType(p *Player, defensive bool) *FightType {
	wanted := FightStyleAggressive
	fallbackName := "STAFF_POUND"
	if defensive {
		wanted = FightStyleDefensive
		fallbackName = "STAFF_FOCUS"
	}
	weapon := CurrentWeaponInterface(p)
	for _, ft := range weapon.FightTypes {
		if ft != nil && ft.Style == wanted {
			return ft
		}
	}
	if ft := CombatStyles().FightType(fallbackName); ft != nil {
		return ft
	}
	if ft := weapon.LowestFightType(); ft != nil {
		return ft
	}
	return UnarmedPunch
}

// openCombatInterface is openCombatInterface: tab 0 mounts
// the combat interface (SendTabInterface(0, ...) mounts the
// fixed group 593 in Go, as in TS).
func openCombatInterface(p *Player) {
	if sender := p.autocastSender(); sender != nil {
		sender.SendTabInterface(0, CombatInterfaceGroup)
	}
}

// IsActive is Autocasting.isActive: whether the player is
// autocasting now — an autocast spell is set AND a staff is
// equipped. A default spell kept without a staff ("Default
// spell set. Please equip a staff to use autocast.") is only
// remembered, not cast. This closes the Phase 16 deviation:
// combat_magic.go's autocastingActive checked the spell only.
func IsActive(p *Player) bool {
	return p != nil && p.GetCombat().AutoCastSpell() != nil && hasStaffEquipped(p)
}

// canAutocast is Autocasting.canAutocast: whether the wielded
// weapon is a staff that can autocast spell.
func canAutocast(p *Player, spell CombatSpell) bool {
	return hasStaffEquipped(p) && weaponRefusal(p, spellbookOfSpell(spell)) == ""
}

// SetAutocast sets (or, after an equipment switch or login,
// restores) the autocast spell. A staff that can't cast it
// clears it rather than keeping it, so switching from an
// Ancient staff to a Staff of the Dead doesn't go on casting
// Ice Barrage. A spell set without a staff is remembered as
// the default ("Default spell set. Please equip a staff to use
// autocast.") and the indicators refresh; BonusManager.update
// has no Go form and needs none (bonuses are live through
// ComputeBonuses, bonus.go).
func SetAutocast(p *Player, spell CombatSpell) {
	if p == nil {
		return
	}
	if spell != nil && hasStaffEquipped(p) && !canAutocast(p, spell) {
		spell = nil
	}
	p.GetCombat().SetAutoCastSpell(spell)
	activeSpell := spell
	if !hasStaffEquipped(p) {
		activeSpell = nil
	}
	if activeSpell == nil && spell != nil {
		p.SendMessage("Default spell set. Please equip a staff to use autocast.")
	}

	RefreshAutocastIndicators(p)
}

// clearAutocastForRefusal is clearForRefusal: a refused
// autocast button turns an existing autocast off (the refusal
// line goes out with autocast_set, autocast_defmode and
// autocast_spell at 0, and com_mode back to the style chosen
// before autocasting). The selector's own style (Focus/Fend)
// is not remembered, so the category still holds the
// pre-autocast slot, which is restored through
// ChangeCombatStyle.
func clearAutocastForRefusal(p *Player) {
	if p == nil || p.GetCombat().AutoCastSpell() == nil {
		return
	}
	SetAutocast(p, nil)
	if slot, ok := RememberedCombatStyleSlot(p, CurrentWeaponInterface(p).Category); ok {
		ChangeCombatStyle(p, slot)
	}
}

// RestoreAutocastOnLogin restores the saved autocast spell on
// login (Player.onLogin, Player.ts:795-800): a spell whose
// book is not the player's clears it; otherwise the spell is
// restored through setAutocast when a staff is equipped.
//
// DEVIATION (grounded in the missing hydration): TS hydrates
// the combat's autocast spell from the save before onLogin
// runs, so with no staff equipped onLogin calls nothing — the
// spell is already in the slot. Go has no such hydration (no
// call site ever wrote the save's autocastSpellId onto the
// live Combat), so the no-staff branch writes the slot
// directly (SetAutoCastSpell + indicator refresh) instead of
// dropping the saved spell. It sends no "Default spell set"
// line: TS's onLogin sends none either.
func RestoreAutocastOnLogin(p *Player, spellID int) {
	if p == nil || spellID < 0 {
		return
	}
	spell := GetCombatSpell(spellID)
	if spell == nil {
		// Unknown spell id (an unlanded book's spell, or a
		// corrupt save): TS hydration reads null, so the
		// slot stays empty.
		return
	}
	if spellbookOfSpell(spell) != p.Spellbook() {
		SetAutocast(p, nil)
		return
	}
	if hasStaffEquipped(p) {
		SetAutocast(p, spell)
		return
	}
	p.GetCombat().SetAutoCastSpell(spell)
	RefreshAutocastIndicators(p)
}

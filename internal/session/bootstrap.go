package session

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"tsps-go/internal/persist"
	"tsps-go/internal/protocol"
)

// This file ports sendPlayerBootstrap (NetworkBuilder.ts:1703-1732) and
// the per-player state it reads: getPlayerAppearance
// (NetworkBuilder.ts:1885-1912), resolveGameframeRoot / dataOrbsHidden
// (ClientProtocol.ts:2674-2697), and the PacketSender calls it chains
// (sendTabInterface, sendInterfaceScript, sendItemContainer,
// sendSkillsSnapshot, sendRunEnergy, QuickPrayers.sync).
//
// The bootstrap runs at the end of enterWorldLocked, exactly where TS
// calls sendPlayerBootstrap at the end of enterWorld, and sends through
// the session queue (Connection.sendLocked). Every default below is
// grounded in the TS constructor or save-application path it mirrors;
// where the Go port cannot reproduce TS state (no plugin system;
// the bootstrap's varp sends bypass the session varp store —
// they go direct to the channel during the handshake, before
// the session wiring exists; item definitions reach the
// bootstrap only through the optional ItemWeights source), the
// gap is documented at the site and the value sent is the one
// TS would send for a fresh character with no plugin
// intervention.

// Appearance look indices (Appearance.ts:4-16). The save's appearance
// array is 13 entries: gender, 7 kits, 5 colours.
const (
	lookGender     = 0
	lookHead       = 1
	lookChest      = 2
	lookArms       = 3
	lookHands      = 4
	lookLegs       = 5
	lookFeet       = 6
	lookBeard      = 7
	lookHairColour = 8
	lookTorsoCol   = 9
	lookLegColour  = 10
	lookFeetColour = 11
	lookSkinColour = 12
)

// Default looks and sanitize ranges, verbatim from PlayerSave.ts:104-141
// (DEFAULT_MALE/FEMALE_APPEARANCE, MALE/FEMALE_LOOK_RANGES,
// COLOR_RANGES). They match Appearance.set() (Appearance.ts:77-98),
// which builds the same arrays for a new character.
var (
	defaultMaleLook   = [13]int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0}
	defaultFemaleLook = [13]int{1, 48, 57, 65, 68, 77, 80, -1, 2, 14, 5, 4, 0}
	// In body-part order: head, beard, chest, arms, hands, legs, feet.
	maleLookRanges   = [7][2]int{{0, 8}, {10, 17}, {18, 25}, {26, 31}, {33, 34}, {36, 40}, {42, 43}}
	femaleLookRanges = [7][2]int{{45, 54}, {-1, -1}, {56, 60}, {61, 65}, {67, 68}, {70, 77}, {79, 80}}
	// In colour order: hair, torso, legs, feet, skin.
	colorRanges = [5][2]int{{0, 29}, {0, 28}, {0, 28}, {0, 5}, {0, 12}}
	lookIndices = [7]int{lookHead, lookBeard, lookChest, lookArms, lookHands, lookLegs, lookFeet}
)

// spellUnlockVarps / spellUnlockVarbits are SPELL_UNLOCK_VARPS and
// SPELL_UNLOCK_VARBITS (PacketSender.ts:125-158), in TS order: varbits
// first, then varps, so the canonical quest varps win where storage is
// shared (PacketSender.ts:551-562). Values are the completed states —
// the server does not enforce quest requirements yet, but the client
// must render the spells as available.
var spellUnlockVarps = [][2]int{
	{68, 0x7fffffff}, {139, 180}, {161, 0x7fffffff}, {165, 0x7fffffff},
	{212, 0x7fffffff}, {267, 0x7fffffff}, {302, 0x7fffffff}, {335, 0x7fffffff},
	{365, 0x7fffffff}, {440, 15}, {823, 190}, {980, 0x7fffffff}, {1003, 0x7fffffff},
}

var spellUnlockVarbits = [][2]int{
	{358, 0x7fffffff}, {2448, 0x7fffffff}, {3618, 0x7fffffff}, {1914, 50},
	{4533, 1}, {1505, 1}, {4896, 1000}, {6067, 6}, {5619, 9}, {9133, 1},
	{9631, 1}, {9649, 127}, {12296, 0x7fffffff}, {18314, 6},
}

// quickPrayerSlotByID is QUICK_PRAYER_SLOT_BY_ID (QuickPrayers.ts:143-146):
// the selection-mask slot per prayer id. Prayers past the end of this
// table (Zeal) have no slot and cannot be shown as selected.
var quickPrayerSlotByID = []int{
	0, 1, 2, 18, 19, 3, 4, 5, 6, 7, 8, 20, 21, 9, 10,
	11, 12, 13, 14, 22, 23, 15, 16, 17, 28, 25, 26, 24, 27,
}

// skillClientIDs maps skill array index to the client id SkillView
// carries (Skill.ts: getClientId returns clientId ?? index; only
// Construction (index 21 -> 22) and Hunter (index 22 -> 21) differ).
var skillClientIDs = [persist.SkillCount]int{
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 22, 21, 23,
}

// experienceTable is SkillManager.EXP_ARRAY (SkillManager.ts:28-37): the
// experience threshold for each level 1..99 (index 0 is level 1).
var experienceTable = [99]int{
	0, 83, 174, 276, 388, 512, 650, 801, 969, 1154, 1358, 1584, 1833, 2107,
	2411, 2746, 3115, 3523, 3973, 4470, 5018, 5624, 6291, 7028, 7842, 8740, 9730, 10824, 12031, 13363, 14833,
	16456, 18247, 20224, 22406, 24815, 27473, 30408, 33648, 37224, 41171, 45529, 50339, 55649, 61512, 67983,
	75127, 83014, 91721, 101333, 111945, 123660, 136594, 150872, 166636, 184040, 203254, 224466, 247886, 273742,
	302288, 333804, 368599, 407015, 449428, 496254, 547953, 605032, 668051, 737627, 814445, 899257, 992895,
	1096278, 1210421, 1336443, 1475581, 1629200, 1798808, 1986068, 2192818, 2421087, 2673114, 2951373, 3258594,
	3597792, 3972294, 4385776, 4842295, 5346332, 5902831, 6517253, 7195629, 7944614, 8771558, 9684577, 10692629,
	11805606, 13034431,
}

const (
	maxExperience      = 200000000 // SkillManager.MAX_EXPERIENCE
	experienceFor99    = 13034431  // SkillManager.EXPERIENCE_FOR_99
	spellbookVarbit    = 4070      // sendTabInterface(6) spellbook varbit (PacketSender.ts:563)
	quickPrayerMaskVar = 4102      // QUICK_PRAYER_SELECTED_VARBIT (QuickPrayers.ts:142)
	quickPrayerOnVar   = 4103      // sendQuickPrayersState varbit (PacketSender.ts:390)
	wornInventoryID    = 94        // WORN_INVENTORY_ID (PacketSender.ts:182)
)

// sendBootstrapLocked sends the post-login packet sequence, porting
// sendPlayerBootstrap (NetworkBuilder.ts:1703-1732). It runs at the end
// of enterWorldLocked and is idempotent: Bootstrap() calls it too, and
// a second call sends nothing. The caller holds c.mu; packets go
// through sendLocked into the session queue.
//
// Sequence (TS line references in the step comments):
//  1. HANDSHAKE, 2. default animations, 3. gameframe bootstrap,
//  4. tab interface 6 (spellbook) + script 828 + inventory/equipment
//     containers + skills snapshot + run energy,
//  5. quick-prayer sync. The player:bootstrap-complete plugin event
//     (step 6 in TS) is NOT ported: there is no plugin system, and
//     the spell-unlock varp recovery it triggers is deliberately not
//     faked beyond what step 4 sends.
func (c *Connection) sendBootstrapLocked() {
	if c.bootstrapped || !c.entered {
		return
	}
	c.bootstrapped = true

	// Store this session's own encoded appearance block so other
	// players' FlushTick calls can read it via SyncAppearance when
	// building sync views (see flush.go). The block embeds the
	// display name (TS createPlayerView: player.getDisplayName()),
	// falling back to the username when no display name was
	// retained, and the save's skill levels with the default
	// animation array (equipment-driven animations need the
	// equipment system, NOT STARTED — same fallback as
	// defaultAppearanceBlock). This runs first, before any packet
	// is sent: the player is already registered, so a tick
	// interleaving before this store would make observers cache
	// the default fallback block for this index; the adapter
	// additionally marks snapshots appearance-dirty while no block
	// is stored, so the real block is picked up on the next tick
	// regardless (see cmd/server/syncsource.go).
	_, syncTotalLevel, syncCombatLevel := bootstrapSkills(c.bootstrapSave)
	syncName := c.bootstrapDisplayName
	if syncName == "" {
		syncName = c.bootstrapUsername
	}
	c.session.SetSyncAppearance(protocol.EncodePlayerAppearance(
		*bootstrapAppearance(c.bootstrapSave), syncName, syncCombatLevel, syncTotalLevel, defaultSyncAnimations))
	members, label, devUsernames := c.bootstrapWorld()
	save := c.bootstrapSave
	username := c.bootstrapUsername

	// The process cycle the gameframe opens on (map_clock): TS reads
	// World.getProcessCycle() live (NetworkBuilder.ts:1720). The world
	// is non-nil whenever entered is true; the guard only covers a
	// world that reports no cycle, which cannot happen through
	// WorldPort.
	var cycle int64
	if c.world != nil {
		cycle = c.world.ProcessCycle()
	}

	// 1. HANDSHAKE (NetworkBuilder.ts:1705-1713). Chat icons are empty:
	// Player.chatIcons defaults to [] (Player.ts:237) and no
	// save-application or enterWorld step sets them.
	c.sendLocked(protocol.EncodeHandshake(
		c.playerIndex, username,
		bootstrapAdmin(save, username, devUsernames),
		bootstrapAppearance(save), nil, members))

	// 2. Default animations (NetworkBuilder.ts:1714).
	c.sendLocked(protocol.EncodeDefaultAnimations())

	// 3. Gameframe bootstrap (NetworkBuilder.ts:1719-1720). Orbs are
	// shown: dataOrbsHidden reads varbit 4084, which defaults to 0 in
	// a fresh PacketSender varp store (getVarp returns 0 for an unset
	// varp, PacketSender.ts:340-341), and nothing in the Go port sets
	// it before bootstrap. GAP: the Settings plugin restores a saved
	// "Show data orbs" off-setting (settings:values) during the
	// player:login event in TS; without plugins that restore cannot
	// happen here, so a player who hid their orbs sees them until the
	// settings milestone lands.
	attrs := saveAttributes(save)
	root := resolveGameframeRoot(c.bootstrapClientType, attrs)
	xpDrops := attributeBool(attrs, "xp-drops:enabled")
	for _, packet := range protocol.EncodeGameframeBootstrap(username, root, xpDrops, label, true, int(cycle)) {
		c.sendLocked(packet)
	}

	// 4a. Tab interface 6 (spellbook): sendTabInterface(6, interfaceId)
	// (PacketSender.ts:548-568). Spell-unlock varbits, then varps, then
	// the spellbook varbit, then the 218 mount, then the tab setup.
	// The spellUnlocksSent guard is always false at bootstrap (a fresh
	// PacketSender), so the unlocks always send here.
	for _, pair := range spellUnlockVarbits {
		c.sendLocked(protocol.EncodeVarbit(pair[0], pair[1]))
	}
	for _, pair := range spellUnlockVarps {
		c.sendLocked(protocol.EncodeVarp(pair[0], pair[1]))
	}
	c.sendLocked(protocol.EncodeVarbit(spellbookVarbit, spellbookVarbitValue(save)))
	c.sendLocked(protocol.EncodeWidgetOpenSub((161<<16)|82, 218, 1, protocol.WidgetOpenSubOptions{}))
	for _, packet := range protocol.EncodeGameframeTabSetup(218, root) {
		c.sendLocked(packet)
	}

	// 4b. Interface script 828 (members flag -> varc 103)
	// (NetworkBuilder.ts:1726).
	membersArg := 0
	if members {
		membersArg = 1
	}
	c.sendLocked(protocol.EncodeWidgetRunScript(828, []protocol.ScriptArg{protocol.ScriptArgInt(membersArg)}, nil, nil, nil))

	// 4c. Inventory container 3214: sendItemContainer always sends the
	// full 28-slot snapshot, empty slots included
	// (PacketSender.ts:1042-1050). The interface id is ignored by the
	// Inventory branch; the snapshot carries no inventory id.
	c.sendLocked(protocol.EncodeInventorySnapshot(inventorySlots(save)))

	// 4d. Equipment container 94: ONLY for native clients. The browser
	// builds worn equipment from the appearance block; the official
	// client gets inventory 94 itself, quantities included, with
	// empty slots filtered out (PacketSender.ts:1052-1067). A browser
	// client receives no packet here at all — including for a fresh
	// character, where TS sends nothing rather than an empty 94.
	if c.bootstrapCapabilities&protocol.HandshakeCapabilityNativeClient != 0 {
		c.sendLocked(protocol.EncodeWidgetRunScript(-1, nil, nil, nil,
			map[int]protocol.ScriptInventorySnapshot{
				wornInventoryID: {Capacity: persist.EquipmentSize, Slots: equipmentSlots(save)},
			}))
	}

	// 4e. Skills snapshot + run energy (NetworkBuilder.ts:1730-1731).
	views, totalLevel, combatLevel := bootstrapSkills(save)
	c.sendLocked(protocol.EncodeSkillsSnapshot(views, totalLevel, combatLevel))
	units, running := runEnergyState(save)
	c.sendLocked(protocol.EncodeRunEnergy(units, running, carriedWeightKg(c.deps.ItemWeights, save), 0))

	// 5. Quick-prayer sync: QuickPrayers.sync() = sendChecks() +
	// sendQuickPrayersState(enabled) (QuickPrayers.ts:291-294).
	// enabled is always false at bootstrap: setPrayers (called by
	// applyToPlayer) sets it false, and nothing re-enables it before
	// the bootstrap runs.
	c.sendLocked(protocol.EncodeVarbit(quickPrayerMaskVar, quickPrayerMask(save)))
	c.sendLocked(protocol.EncodeVarbit(quickPrayerOnVar, 0))

	// 6. The player:bootstrap-complete plugin event (NetworkBuilder.ts:
	// 1735-1736) is NOT ported: no plugin system exists in Go. Its
	// spell-unlock varp recovery is deliberately not reproduced beyond
	// step 4a; a quest plugin would re-send its own varps on login.
}

// bootstrapWorld resolves the world-level bootstrap facts from
// LoginDeps, defaulting to the TS world defaults (members world,
// "REGULAR" label, no developers) when unset.
func (c *Connection) bootstrapWorld() (members bool, label string, devUsernames []string) {
	members, label = true, "REGULAR"
	if w := c.deps.BootstrapWorld; w != nil {
		members = w.MembersWorld
		if w.TypeLabel != "" {
			label = w.TypeLabel
		}
		devUsernames = w.DevUsernames
	}
	return members, label, devUsernames
}

func saveAttributes(save *persist.PlayerSave) map[string]json.RawMessage {
	if save == nil {
		return nil
	}
	return save.Attributes
}

// bootstrapAdmin ports PlayerRights.hasAdminRights as enterWorld
// applies it: rights id >= ADMINISTRATOR (2), where the rights come
// from the save (applyToPlayer sets them; a new character has NONE),
// unless the username is a configured developer, in which case
// enterWorld overwrites the rights with DEVELOPER (id 4) after
// applying the save (NetworkBuilder.ts:1670, PlayerRights.ts:48-53,
// isConfiguredDeveloperUsername, NetworkBuilder.ts:335-340: DEV_USERNAME
// split on commas, trimmed, empty entries dropped, compared
// lowercased).
func bootstrapAdmin(save *persist.PlayerSave, username string, devUsernames []string) bool {
	rightsID := 0
	if save != nil {
		rightsID = save.Rights.ID
	}
	if rightsID >= 2 {
		return true
	}
	name := strings.ToLower(username)
	for _, dev := range devUsernames {
		if trimmed := strings.TrimSpace(dev); trimmed != "" && strings.ToLower(trimmed) == name {
			return true
		}
	}
	return false
}

// bootstrapAppearance ports getPlayerAppearance
// (NetworkBuilder.ts:1885-1912) over a save snapshot (or no save):
// the look array sanitized as applyToPlayer sanitizes it, equipment
// ids/quantities from the worn items, the skull icon from the skull
// timer/type, and the prayer head hint (-1: Appearance.headHint
// defaults to -1 and no save field sets it).
//
// Hidden is always false: appearanceHidden is an ephemeral field
// defaulting to false (Player.ts:1061-1062), never persisted.
// NpcTransformationID is always nil: transformations are transient
// and no save field carries one.
func bootstrapAppearance(save *persist.PlayerSave) *protocol.PlayerAppearance {
	var raw []int
	var equip []persist.Item
	var attrs map[string]json.RawMessage
	skull := -1
	if save != nil {
		raw = save.Appearance
		equip = save.Equipment
		attrs = save.Attributes
		// getHeadSkullIcon: skulled (timer > 0) shows the skull
		// type's icon id (0 white, 1 red); overrides are transient
		// and null at login (Player.ts:1571-1576, SkullType.ts).
		if save.SkullTimer > 0 {
			skull = save.SkullType.IconID
		}
	}
	look := sanitizeLook(raw)
	gender := look[lookGender]
	// The gender default: 0 for male, 1 for female (getPronouns falls
	// back to isMale() ? 0 : 1, Appearance.ts:103-106).
	pronouns := gender
	// getPronouns: the appearance:pronouns attribute when it is an
	// integer 0..2, else 0 for male / 1 for female (Appearance.ts:103-106).
	if v, ok := attributeInt(attrs, "appearance:pronouns"); ok && v >= 0 && v <= 2 {
		pronouns = v
	}
	equipIDs := make([]int, persist.EquipmentSize)
	equipQty := make([]int, persist.EquipmentSize)
	for i := range equipIDs {
		equipIDs[i] = -1
		if i < len(equip) {
			equipIDs[i] = equip[i].ID
			equipQty[i] = equip[i].Amount
		}
	}
	return &protocol.PlayerAppearance{
		Gender:   gender,
		Pronouns: &pronouns,
		Colors:   []int{look[lookHairColour], look[lookTorsoCol], look[lookLegColour], look[lookFeetColour], look[lookSkinColour]},
		Kits:     []int{look[lookHead], look[lookBeard], look[lookChest], look[lookArms], look[lookHands], look[lookLegs], look[lookFeet]},
		Equip:    equipIDs,
		EquipQty: equipQty,
		HeadIcons: &protocol.HeadIcons{
			Skull:  skull,
			Prayer: -1,
		},
	}
}

// sanitizeLook ports PlayerSave.sanitizeAppearance
// (PlayerSave.ts:581-629) without the cache-dependent kit check: a
// candidate kit is kept only when it is the optional -1 jaw, else
// clamped into its legacy range. TS keeps a kit the cache offers for
// the body part (isSelectableKit); with no cache loaded that check
// returns false (PlayerSave.ts:559-573), so clamping is exactly the
// TS no-cache behavior. A missing or short array yields the
// gender-appropriate defaults, as toFiniteInt's fallbacks do in TS.
func sanitizeLook(raw []int) [13]int {
	gender := 0
	if len(raw) > lookGender && raw[lookGender] == 1 {
		gender = 1
	}
	defaults := defaultMaleLook
	ranges := maleLookRanges
	if gender == 1 {
		defaults = defaultFemaleLook
		ranges = femaleLookRanges
	}
	sanitized := defaults
	at := func(index int) int {
		if index < len(raw) {
			return raw[index]
		}
		return defaults[index]
	}
	for i, index := range lookIndices {
		candidate := at(index)
		if index == lookBeard && candidate == -1 {
			sanitized[index] = -1
			continue
		}
		sanitized[index] = clampInt(candidate, ranges[i][0], ranges[i][1])
	}
	for i, index := range []int{lookHairColour, lookTorsoCol, lookLegColour, lookFeetColour, lookSkinColour} {
		candidate := at(index)
		if index == lookSkinColour && (candidate < colorRanges[i][0] || candidate > colorRanges[i][1]) {
			// An unknown skin (a native client's red, 13) goes back
			// to the default rather than clamping to a neighbour.
			sanitized[index] = defaults[index]
			continue
		}
		sanitized[index] = clampInt(candidate, colorRanges[i][0], colorRanges[i][1])
	}
	sanitized[lookGender] = gender
	return sanitized
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// resolveGameframeRoot ports resolveGameframeRoot
// (ClientProtocol.ts:2674-2681): mobile clients (handshake clientType
// 1, recorded as the transient mobile-client attribute) resolve to the
// stock mobile toplevel 601; otherwise the saved client-layout-root
// attribute wins when it names a standard root (548/164/161), else
// 161. The Settings plugin persists client-layout-root with default
// 161 (Settings.plugin.js:1313).
func resolveGameframeRoot(clientType byte, attrs map[string]json.RawMessage) int {
	if clientType == 1 {
		return protocol.MobileGameframeRoot
	}
	if v, ok := attributeNumber(attrs, "client-layout-root"); ok {
		switch v {
		case 548, 164, 161:
			return int(v)
		}
	}
	return 161
}

// spellbookVarbitValue ports the spellbook mapping in sendTabInterface
// (PacketSender.ts:549): Ancient 1, Lunar 2, Arceuus 3, anything else
// (including the default Normal book, interface 1151, and unknown
// interface ids) 0. The default spellbook is NORMAL (Player.ts:201,
// MagicSpellbook.NORMAL, interface 1151).
func spellbookVarbitValue(save *persist.PlayerSave) int {
	interfaceID := persist.SpellbookNormalInterfaceID
	if save != nil {
		interfaceID = save.SpellBook.InterfaceID
	}
	switch interfaceID {
	case persist.SpellbookAncientInterfaceID:
		return 1
	case persist.SpellbookLunarInterfaceID:
		return 2
	case persist.SpellbookArceusInterfaceID:
		return 3
	default:
		return 0
	}
}

// inventorySlots builds the 28-slot snapshot sendItemContainer sends
// for an Inventory (PacketSender.ts:1042-1050): every slot, empty
// slots as id -1 / quantity 0. A missing or short save array reads as
// empty slots, matching a fresh container.
func inventorySlots(save *persist.PlayerSave) []protocol.ItemSlot {
	var items []persist.Item
	if save != nil {
		items = save.Inventory
	}
	slots := make([]protocol.ItemSlot, persist.InventorySize)
	for i := range slots {
		slots[i] = protocol.ItemSlot{Slot: i, ItemID: -1}
		if i < len(items) {
			slots[i].ItemID = items[i].ID
			slots[i].Quantity = items[i].Amount
		}
	}
	return slots
}

// equipmentSlots builds the worn-inventory slots for a native client
// (PacketSender.ts:1057-1064): only slots holding an item (id >= 0);
// empty slots are filtered out before the run-script packet is built.
func equipmentSlots(save *persist.PlayerSave) []protocol.ItemSlot {
	var items []persist.Item
	if save != nil {
		items = save.Equipment
	}
	var slots []protocol.ItemSlot
	for i := 0; i < persist.EquipmentSize && i < len(items); i++ {
		if items[i].ID >= 0 {
			slots = append(slots, protocol.ItemSlot{Slot: i, ItemID: items[i].ID, Quantity: items[i].Amount})
		}
	}
	return slots
}

// bootstrapSkills ports the skills snapshot: skillView per skill plus
// getTotalLevel/getCombatLevel (PacketSender.ts:1499-1518,
// SkillManager.ts:301-327, 398-403), over the save's skill arrays as
// SkillManager.setSkills normalizes them (SkillManager.ts:655-686), or
// the SkillManager constructor defaults for a new character (all 1,
// Hitpoints 10 with 1184 XP; SkillManager.ts:53-64).
//
// Normalization (setSkills): experience is sanitized (<=0 -> 0,
// >=200M -> 200M); a skill's base level is at least the level its
// experience implies (getMaxLevel applies the same floor at send
// time, SkillManager.ts:441-448); a missing current level rests at
// the base level; Hitpoints keeps at least level 10, and — for a
// save — at least 1154 XP (getExperienceForLevel(10),
// EXP_ARRAY[9]). A new character instead gets the constructor's 1184
// XP (the JSON backend's defaultSkills value, matching
// persist.DefaultSkills). The 1154/1184 distinction only shows for a
// save whose Hitpoints experience is below 1154; real saves always
// carry at least 1184.
func bootstrapSkills(save *persist.PlayerSave) ([]protocol.SkillView, int, int) {
	var skills persist.Skills
	fromSave := save != nil
	if fromSave {
		skills = save.Skills
	} else {
		skills = persist.DefaultSkills()
	}
	hpMinXP := 1184
	if fromSave {
		hpMinXP = experienceTable[9] // getExperienceForLevel(10) = 1154
	}
	views := make([]protocol.SkillView, persist.SkillCount)
	maxLevels := make([]int, persist.SkillCount)
	total := 0
	for i := 0; i < persist.SkillCount; i++ {
		exp := 0
		if i < len(skills.Experience) {
			exp = sanitizeExperience(skills.Experience[i])
		}
		fromExp := levelForExperience(exp)
		if fromExp < 1 {
			fromExp = 1
		}
		base := fromExp
		if i < len(skills.MaxLevel) {
			if skills.MaxLevel[i] > base {
				base = skills.MaxLevel[i]
			}
		}
		current := base
		if i < len(skills.Level) {
			current = skills.Level[i]
			if current < 0 {
				current = 0
			}
		}
		if i == persist.HitpointsIndex {
			if exp < hpMinXP {
				exp = hpMinXP
			}
			if base < 10 {
				base = 10
			}
			if current < 10 {
				current = 10
			}
		}
		maxLevels[i] = base
		total += base
		views[i] = protocol.SkillView{
			ID:           skillClientIDs[i],
			XP:           exp,
			BaseLevel:    base,
			VirtualLevel: base,
			Boost:        current - base,
			CurrentLevel: current,
		}
	}
	return views, total, combatLevel(maxLevels)
}

// sanitizeExperience ports SkillManager.sanitizeExperienceValue
// (SkillManager.ts:111-119) for integer experience: non-positive
// values become 0, values at the cap stay at the cap. (The TS
// tenth-rounding is a no-op for integers.)
func sanitizeExperience(exp int) int {
	if exp <= 0 {
		return 0
	}
	if exp >= maxExperience {
		return maxExperience
	}
	return exp
}

// levelForExperience ports SkillManager.getLevelForExperience
// (SkillManager.ts:82-100). The caller passes sanitized (>= 0)
// experience, as getExperience does in TS.
func levelForExperience(exp int) int {
	if exp <= experienceFor99 {
		for j := len(experienceTable) - 1; j >= 0; j-- {
			if experienceTable[j] <= exp {
				return j + 1
			}
		}
	} else {
		points, output := 0, 0
		for lvl := 1; lvl <= 99; lvl++ {
			points += int(math.Floor(float64(lvl) + 300.0*math.Pow(2.0, float64(lvl)/7.0)))
			output = points / 4
			if output >= exp {
				return lvl
			}
		}
	}
	return 99
}

// combatLevel ports SkillManager.calculateCombatLevel
// (SkillManager.ts:312-327), clamped to 3..126.
func combatLevel(maxLevels []int) int {
	attack := maxLevels[0]
	defence := maxLevels[1]
	strength := maxLevels[2]
	hitpoints := maxLevels[persist.HitpointsIndex]
	ranged := maxLevels[4]
	prayer := maxLevels[5]
	magic := maxLevels[6]
	base := float64(defence+hitpoints+prayer/2) * 0.25
	melee := float64(attack+strength) * 0.325
	ranger := math.Floor(float64(ranged)*1.5) * 0.325
	mage := math.Floor(float64(magic)*1.5) * 0.325
	level := int(math.Floor(base + math.Max(melee, math.Max(ranger, mage))))
	if level < 3 {
		return 3
	}
	if level > 126 {
		return 126
	}
	return level
}

// runEnergyState ports the run-energy inputs sendRunEnergyPacket reads
// (PacketSender.ts:379-388): exact units and the running flag. The
// carried weight comes from carriedWeightKg below (via
// LoginDeps.ItemWeights when definitions are loaded, else 0), and
// stamina ticks are sent as 0: no stamina state survives a login
// (applyRunEnergyStatusModifiers starts from 0 ticks and only
// plugin modifiers change it, EquipmentEffects.ts:311-316).
//
// Units: a new character starts at RUN_ENERGY_MAX_UNITS (Player.ts:185);
// a save's percent converts through setRunEnergy, which floors
// percent*100 + 1e-6 into units and clamps to 0..10000
// (Player.ts:1263-1274). Running: a new character starts running
// (Player.isRunning defaults true, Player.ts:160); a save overrides
// it.
func runEnergyState(save *persist.PlayerSave) (units int, running bool) {
	if save == nil {
		return 10000, true
	}
	units = int(math.Floor(save.RunEnergy*100 + 1e-6))
	if units < 0 {
		units = 0
	}
	if units > 10000 {
		units = 10000
	}
	return units, save.Running
}

// carriedWeightKg ports Player.getWeightKg (Player.ts:1314-1316):
// Math.trunc(getCarriedWeightGrams() / 1000). Go integer division
// truncates toward zero, matching Math.trunc exactly, including for
// negative totals (graceful-style negative weights can make the sum
// negative; Player.ts:1280-1286).
//
// The grams total ports getCarriedWeightGrams (Player.ts:1287-1311):
// equipment slots first, then inventory slots, each occupied slot
// contributing its definition weight once regardless of amount, with
// stackable and noted items contributing 0 (the per-slot rule is
// slotGrams; the stackable/noted decision itself lives in the
// ItemWeightSource implementation). The slot lists are the same save
// lists the inventory/equipment snapshots read (inventorySlots,
// equipmentSlots), capped at the TS container sizes: entries past
// the capacity cannot exist in a TS container.
//
// Two TS inputs are deliberately absent. The "equipment:weights"
// plugin overrides (Player.ts:1288-1289, 1300-1305) do not exist in
// Go: there is no plugin system, so every worn slot uses its
// definition weight and graceful pieces weigh their (negative) cache
// weight only if the cache carries it. And with no weight source
// (nil weights: no definitions loaded) or no save, the weight is 0,
// which is exact for empty containers.
func carriedWeightKg(weights ItemWeightSource, save *persist.PlayerSave) int {
	if weights == nil || save == nil {
		return 0
	}
	grams := 0
	for i := 0; i < len(save.Equipment) && i < persist.EquipmentSize; i++ {
		grams += slotGrams(weights, save.Equipment[i])
	}
	for i := 0; i < len(save.Inventory) && i < persist.InventorySize; i++ {
		grams += slotGrams(weights, save.Inventory[i])
	}
	return grams / 1000
}

// slotGrams ports the itemGrams closure in getCarriedWeightGrams
// (Player.ts:1290-1296): an empty slot (id < 0) or a zero-amount
// stack contributes 0 without consulting definitions; any other
// slot contributes its definition weight once, whatever the amount.
func slotGrams(weights ItemWeightSource, item persist.Item) int {
	if item.ID < 0 || item.Amount <= 0 {
		return 0
	}
	return weights.ItemWeightGrams(item.ID)
}

// quickPrayerMask ports QuickPrayers.selectedPrayerMask
// (QuickPrayers.ts:173-181): one bit per selected prayer at its slot
// in QUICK_PRAYER_SLOT_BY_ID. The save's quick-prayer array is
// positional (setPrayers indexes by prayer id, QuickPrayers.ts:418-424);
// a nil entry means not selected. A new character has no selections.
func quickPrayerMask(save *persist.PlayerSave) int {
	if save == nil {
		return 0
	}
	mask := 0
	for i, prayer := range save.QuickPrayers {
		if prayer != nil && i < len(quickPrayerSlotByID) {
			mask |= 1 << uint(quickPrayerSlotByID[i])
		}
	}
	return mask
}

// attributeBool reads a persisted player attribute as a strict JSON
// boolean: only the literal true counts, matching TS `=== true`
// checks (e.g. XPDROPS_ENABLED_ATTRIBUTE, NetworkBuilder.ts:1720).
func attributeBool(attrs map[string]json.RawMessage, key string) bool {
	raw, ok := attrs[key]
	if !ok {
		return false
	}
	var v bool
	return json.Unmarshal(raw, &v) == nil && v
}

// attributeInt reads a persisted attribute as a JSON integer, for
// checks that require a number (getPronouns, Appearance.ts:103-106).
// Non-numbers and non-integers report absent.
func attributeInt(attrs map[string]json.RawMessage, key string) (int, bool) {
	raw, ok := attrs[key]
	if !ok {
		return 0, false
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil || v != math.Trunc(v) {
		return 0, false
	}
	return int(v), true
}

// attributeNumber reads a persisted attribute as TS Number(value)
// would: JSON numbers as-is, numeric strings parsed (JS Number trims
// surrounding whitespace; "" is 0), booleans as 1/0, null as 0, and
// anything else absent (NaN). resolveGameframeRoot applies Number()
// to the client-layout-root attribute (ClientProtocol.ts:2678-2680).
func attributeNumber(attrs map[string]json.RawMessage, key string) (float64, bool) {
	raw, ok := attrs[key]
	if !ok {
		return 0, false
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		trimmed := strings.TrimSpace(n)
		if trimmed == "" {
			return 0, true
		}
		f, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	case bool:
		if n {
			return 1, true
		}
		return 0, true
	case nil:
		return 0, true
	default:
		return 0, false
	}
}

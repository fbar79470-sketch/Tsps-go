package protocol

import "encoding/binary"

// Login bootstrap encoders, ported from ClientProtocol.ts (TSPS server,
// commit bfd81e3): the handshake, the default animations, the gameframe
// bootstrap, and the logout response. The bootstrap sequence itself lives
// in sendPlayerBootstrap (NetworkBuilder.ts lines 1703-1737): handshake,
// default animations, gameframe bootstrap packets, then sendTabInterface(6,
// spellbook), sendInterfaceScript(828, [members]), the inventory and
// equipment containers, the skills snapshot, and run energy.

// Gameframe constants (ClientProtocol.ts lines 2539-2710).
const (
	// MainInventoryGroupID is MAIN_INVENTORY_GROUP_ID (149).
	MainInventoryGroupID = 149
	// MainInventoryWidgetUID is MAIN_INVENTORY_WIDGET_UID (149 << 16).
	MainInventoryWidgetUID = MainInventoryGroupID << 16
	// MainInventorySlotFlags is MAIN_INVENTORY_SLOT_FLAGS.
	MainInventorySlotFlags = 0x1207fe
	// DisplaySettingsDropdownButtonsUID is the "Game client layout"
	// dropdown buttons (116:40).
	DisplaySettingsDropdownButtonsUID = (116 << 16) | 40
	// AttackPriorityPlayerButtonsUID is the player "Attack" options
	// dropdown (116:38).
	AttackPriorityPlayerButtonsUID = (116 << 16) | 38
	// AttackPriorityNPCButtonsUID is the NPC "Attack" options dropdown
	// (116:39).
	AttackPriorityNPCButtonsUID = (116 << 16) | 39
	// MobileGameframeRoot is MOBILE_GAMEFRAME_ROOT (601): the stock
	// mobile toplevel.
	MobileGameframeRoot = 601
	// OrbsGroupID is ORBS_GROUP_ID (160): the minimap orbs pane.
	OrbsGroupID = 160
	// MobileOrbsGroupID is MOBILE_ORBS_GROUP_ID (897).
	MobileOrbsGroupID = 897
	// OrbsDisabledVarbit is ORBS_DISABLED_VARBIT (4084): `orbs_disabled`.
	OrbsDisabledVarbit = 4084
)

// OrbsGroupFor returns the orbs pane group for a gameframe root: the
// mobile orbs group on the mobile toplevel, the desktop orbs otherwise
// (orbsGroupFor, ClientProtocol.ts lines 2698-2700).
func OrbsGroupFor(root int) int {
	if root == MobileGameframeRoot {
		return MobileOrbsGroupID
	}
	return OrbsGroupID
}

// EncodeHandshake encodes the HANDSHAKE packet (opcode 2, 1-byte length
// prefix) sent at login (encodeHandshake, ClientProtocol.ts lines
// 2862-2901): the player index, the username, an appearance-present byte
// plus the short appearance form (gender, then colors as bytes and
// kits/equip as shorts, each length-prefixed), the chat icons, an empty
// string, and the admin and members-world bytes.
//
// appearance is a pointer because TS takes it as optional: nil writes
// the absent byte (0) and no appearance data. chatIcons are filtered to
// integers in 0..255 and capped at 255 entries, as in TS. The single
// production caller (NetworkBuilder sendPlayerBootstrap) always passes
// an appearance.
func EncodeHandshake(index int, username string, admin bool, appearance *PlayerAppearance, chatIcons []int, members bool) []byte {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload[0:4], uint32(index))
	payload = append(payload, latin1Bytes(username)...)
	if appearance == nil {
		payload = append(payload, 0)
	} else {
		payload = append(payload, 1, byte(appearance.Gender))
		payload = append(payload, byte(len(appearance.Colors)))
		for _, value := range appearance.Colors {
			payload = append(payload, byte(value))
		}
		appendShorts := func(values []int) {
			payload = append(payload, byte(len(values)))
			for _, value := range values {
				var short [2]byte
				binary.BigEndian.PutUint16(short[:], uint16(value))
				payload = append(payload, short[:]...)
			}
		}
		appendShorts(appearance.Kits)
		appendShorts(appearance.Equip)
	}
	icons := make([]int, 0, len(chatIcons))
	for _, icon := range chatIcons {
		if icon >= 0 && icon <= 255 {
			icons = append(icons, icon)
		}
	}
	if len(icons) > 255 {
		icons = icons[:255]
	}
	payload = append(payload, byte(len(icons)))
	for _, icon := range icons {
		payload = append(payload, byte(icon))
	}
	payload = append(payload, latin1Bytes("")...)
	if admin {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}
	if members {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}
	return mustEncode(ServerHandshake, payload)
}

// EncodeDefaultAnimations encodes the ANIM packet (opcode 22, fixed
// length 22, no length prefix) sent at login (encodeDefaultAnimations,
// ClientProtocol.ts lines 3778-3783): the 11 default animation ids as
// int16s.
func EncodeDefaultAnimations() []byte {
	ids := []int{808, 819, 820, 821, 822, 824, 824, 824, 824, 823, 823}
	payload := make([]byte, len(ids)*2)
	for i, id := range ids {
		binary.BigEndian.PutUint16(payload[i*2:], uint16(int16(id)))
	}
	return mustEncode(ServerAnim, payload)
}

// EncodeLogoutResponse encodes the LOGOUT_RESPONSE packet (opcode 4,
// 1-byte length prefix) (encodeLogoutResponse, ClientProtocol.ts lines
// 3785-3787): a success byte plus the reason string. TS defaults are
// reason "" and success true; the caller passes them explicitly here.
func EncodeLogoutResponse(reason string, success bool) []byte {
	payload := []byte{0}
	if success {
		payload[0] = 1
	}
	payload = append(payload, latin1Bytes(reason)...)
	return mustEncode(ServerLogoutResponse, payload)
}

// encodeOpenSub mounts group into (root, child) with an optional
// post-mount script (encodeOpenSub, ClientProtocol.ts lines 2524-2533).
// It writes the open-sub layout directly: the 7-byte header, zero counts
// for varps/varbits/hidden/pre-scripts, a post-script count of 0 or 1,
// and, when a script is given, its id plus an empty argument list. It is
// byte-identical to EncodeWidgetOpenSub with the equivalent options for
// these mounts.
func encodeOpenSub(root, child, group int, postScript ...int) []byte {
	fixed := make([]byte, 12)
	binary.BigEndian.PutUint32(fixed[0:4], uint32((root<<16)|child))
	binary.BigEndian.PutUint16(fixed[4:6], uint16(group))
	fixed[6] = 1
	if len(postScript) == 0 {
		return mustEncode(ServerWidgetOpenSub, fixed)
	}
	fixed[11] = 1
	script := make([]byte, 5)
	binary.BigEndian.PutUint32(script[0:4], uint32(postScript[0]))
	return mustEncode(ServerWidgetOpenSub, append(fixed, script...))
}

// Gameframe mount/flag constants (ClientProtocol.ts lines 2541-2630).
const (
	displaySettingsDropdownOptionSlots = 5
	firstOptionFlag                    = 1 << 1
	attackPriorityPlayerOptions        = 5
	attackPriorityNPCOptions           = 4
	sideJournalGroupID                 = 629
	sideJournalTabContainerChildID     = 43
	interfaceCharacterSummaryID        = 712
	sideChannelsGroupID                = 707
	sideChannelsContentChildID         = 7
	chatChannelGroupID                 = 7
	questTabIconChildID                = 61
	// questTabIconFlags mirrors WidgetFlags.ts: op1 plus op2/op3/op4
	// (Character Summary / Quest List / Achievement Diaries submenu).
	questTabIconFlags = (1 << 1) | (1 << 2) | (1 << 3) | (1 << 4)
	// Account summary (712) dynamic row list, from live click logs.
	accountSummaryEntryListUID     = (interfaceCharacterSummaryID << 16) | 3
	accountSummaryQuestsRow        = 3
	accountSummaryAchievementsRow  = 4
	accountSummaryCombatTasksRow   = 5
	accountSummaryCollectionLogRow = 6
	accountSummaryPlaytimeRow      = 7
	// The worn equipment tab's quiver slot (387:28).
	equipmentQuiverSlotUID   = (387 << 16) | 28
	equipmentQuiverSlotFlags = (1 << 1) | (1 << 2) | (1 << 10)
	// Prayer and spellbook filter menus: rows the cache scripts create
	// at runtime.
	prayerbookFilterMenuUID     = (541 << 16) | 42
	prayerbookFilterLastRow     = 4
	magicSpellbookFilterMenuUID = (218 << 16) | 206
	magicSpellbookFilterLastRow = 6
)

// questTabIconChildByRoot is QUEST_TAB_ICON_CHILD_BY_ROOT
// (ClientProtocol.ts lines 2567-2572): the quest tab icon child per
// gameframe root, from the cache pane redirect enums.
var questTabIconChildByRoot = map[int]int{161: 61, 548: 66, 164: 54, 601: 118}

// gameframeFlagEntry is one login widget flag with the component it is on.
type gameframeFlagEntry struct {
	uid    int
	packet []byte
}

// gameframeFlagEntries returns the login's widget flags, each with the
// component it is on (gameframeFlagEntries, ClientProtocol.ts lines
// 2614-2630).
func gameframeFlagEntries(root int) []gameframeFlagEntry {
	questTabChild := questTabIconChildID
	if child, ok := questTabIconChildByRoot[root]; ok {
		questTabChild = child
	}
	questTabIcon := (root << 16) | questTabChild
	return []gameframeFlagEntry{
		{equipmentQuiverSlotUID, EncodeWidgetSetFlagsRange(equipmentQuiverSlotUID, -1, -1, equipmentQuiverSlotFlags)},
		{MainInventoryWidgetUID, EncodeWidgetSetFlagsRange(MainInventoryWidgetUID, 0, 27, MainInventorySlotFlags)},
		{prayerbookFilterMenuUID, EncodeWidgetSetFlagsRange(prayerbookFilterMenuUID, 0, prayerbookFilterLastRow, firstOptionFlag)},
		{magicSpellbookFilterMenuUID, EncodeWidgetSetFlagsRange(magicSpellbookFilterMenuUID, 0, magicSpellbookFilterLastRow, firstOptionFlag)},
		{questTabIcon, EncodeWidgetSetFlags(questTabIcon, questTabIconFlags)},
		{accountSummaryEntryListUID, EncodeWidgetSetFlagsRange(accountSummaryEntryListUID, accountSummaryQuestsRow, accountSummaryAchievementsRow, 1<<1)},
		{accountSummaryEntryListUID, EncodeWidgetSetFlagsRange(accountSummaryEntryListUID, accountSummaryCombatTasksRow, accountSummaryCombatTasksRow, (1<<1)|(1<<2)|(1<<3)|(1<<4))},
		{accountSummaryEntryListUID, EncodeWidgetSetFlagsRange(accountSummaryEntryListUID, accountSummaryCollectionLogRow, accountSummaryCollectionLogRow, (1<<1)|(1<<2))},
		{accountSummaryEntryListUID, EncodeWidgetSetFlagsRange(accountSummaryEntryListUID, accountSummaryPlaytimeRow, accountSummaryPlaytimeRow, 1<<1)},
		{DisplaySettingsDropdownButtonsUID, EncodeWidgetSetFlagsRange(DisplaySettingsDropdownButtonsUID, 0, displaySettingsDropdownOptionSlots, firstOptionFlag)},
		{AttackPriorityPlayerButtonsUID, EncodeWidgetSetFlagsRange(AttackPriorityPlayerButtonsUID, 1, attackPriorityPlayerOptions, firstOptionFlag)},
		{AttackPriorityNPCButtonsUID, EncodeWidgetSetFlagsRange(AttackPriorityNPCButtonsUID, 1, attackPriorityNPCOptions, firstOptionFlag)},
	}
}

// EncodeGameframeFlags returns the login's widget flag packets
// (encodeGameframeFlags, ClientProtocol.ts lines 2632-2634).
func EncodeGameframeFlags(root int) [][]byte {
	entries := gameframeFlagEntries(root)
	packets := make([][]byte, len(entries))
	for i, entry := range entries {
		packets[i] = entry.packet
	}
	return packets
}

// EncodeGameframeTabSetup returns what a login sends for one side tab so
// it can be mounted again (encodeGameframeTabSetup, ClientProtocol.ts
// lines 2641-2656): the client drops an interface's flags when it closes,
// so a tab the tutorial kept closed needs its flags (and the quest tab
// its journal, with the character summary's) once more when it opens.
// PacketSender's sendGameframeTabSetup sends these after mounting the
// prayer (541), magic (218), journal (629), and side-channels (707) tabs.
func EncodeGameframeTabSetup(group, root int) [][]byte {
	var packets [][]byte
	groups := []int{group}
	if group == sideJournalGroupID {
		packets = append(packets, encodeOpenSub(sideJournalGroupID, sideJournalTabContainerChildID, interfaceCharacterSummaryID))
		groups = append(groups, interfaceCharacterSummaryID)
	}
	if group == sideChannelsGroupID {
		packets = append(packets, encodeOpenSub(sideChannelsGroupID, sideChannelsContentChildID, chatChannelGroupID))
	}
	for _, entry := range gameframeFlagEntries(root) {
		for _, g := range groups {
			if entry.uid>>16 == g {
				packets = append(packets, entry.packet)
				break
			}
		}
	}
	return packets
}

// gameframeMount is one gameframe bootstrap mount: a child of the
// OSRS-stretch (161) root, the group mounted there, and an optional
// post-mount script.
type gameframeMount struct {
	child         int
	group         int
	postScript    int
	hasPostScript bool
}

// EncodeGameframeBootstrap returns the gameframe a login (or the welcome
// screen's Play) opens (encodeGameframeBootstrap, ClientProtocol.ts lines
// 2708-2737): a camera script packet, the root packet, the pane mounts
// (addressed by the 161 child for every layout; the client maps them onto
// the active root's components), the side journal and side channels
// content mounts, the widget flags, and script 876, whose name and
// world-type strings go to script 5450 (live sends "REGULAR", or
// "HARDCORE WORLD" on a Hardcore world). With orbsShown false the orbs
// are not opened, as live logs in while `orbs_disabled` is set. mapClock
// is the tick the gameframe opens on, as live sends it. TS defaults are
// root 161, xpDropsEnabled false, worldTypeLabel "REGULAR", orbsShown
// true, mapClock 0; the caller passes them explicitly here.
func EncodeGameframeBootstrap(playerName string, root int, xpDropsEnabled bool, worldTypeLabel string, orbsShown bool, mapClock int) [][]byte {
	// pm_chat -> toplevel_osrs_stretch:pm_container (93), xp_drops ->
	// :xp_drops (9). 161:7 (hud_container_back) has no fixed-layout
	// pane, so XP drops mounted there vanished. The modified stats
	// overlay (stat_boosts_hud -> :stat_boosts_hud, 5) opens next to the
	// buff bar.
	mounts := []gameframeMount{{96, 162, 0, false}, {93, 163, 0, false}}
	if orbsShown {
		mounts = append(mounts, gameframeMount{33, OrbsGroupFor(root), 0, false})
	}
	if xpDropsEnabled {
		mounts = append(mounts, gameframeMount{9, 122, 0, false})
	}
	mounts = append(mounts,
		gameframeMount{6, 651, 5929, true},
		gameframeMount{5, 708, 0, false},
		gameframeMount{76, 593, 0, false},
		gameframeMount{77, 320, 0, false},
		gameframeMount{78, 629, 0, false},
		gameframeMount{79, MainInventoryGroupID, 0, false},
		gameframeMount{80, 387, 0, false},
		gameframeMount{81, 541, 0, false},
		gameframeMount{82, 218, 0, false},
		gameframeMount{83, sideChannelsGroupID, 0, false},
		gameframeMount{84, 109, 0, false},
		gameframeMount{85, 429, 0, false},
		gameframeMount{86, 182, 0, false},
		gameframeMount{87, 116, 0, false},
		gameframeMount{88, 216, 0, false},
		gameframeMount{89, 239, 0, false},
		// The boss HP bar HUD (hpbar_hud), as OSRS opens it at login
		// (164:2 resizable).
		gameframeMount{2, 303, 0, false},
	)
	// The camera script packet is EncodeRunClientScript(626, []): a u16
	// script id plus an empty argument list. The login script packet is
	// EncodeWidgetRunScript(876, [mapClock, 0, playerName,
	// worldTypeLabel]) with no varps/varbits/inventories: TS appends
	// four zero bytes after the arguments, which is exactly the two
	// empty var sections that encoder writes.
	packets := [][]byte{
		EncodeRunClientScript(626, nil),
		EncodeWidgetSetRoot(root),
	}
	for _, mount := range mounts {
		if mount.hasPostScript {
			packets = append(packets, encodeOpenSub(161, mount.child, mount.group, mount.postScript))
		} else {
			packets = append(packets, encodeOpenSub(161, mount.child, mount.group))
		}
	}
	packets = append(packets, encodeOpenSub(sideJournalGroupID, sideJournalTabContainerChildID, interfaceCharacterSummaryID))
	packets = append(packets, encodeOpenSub(sideChannelsGroupID, sideChannelsContentChildID, chatChannelGroupID))
	packets = append(packets, EncodeGameframeFlags(root)...)
	packets = append(packets, EncodeWidgetRunScript(876,
		[]ScriptArg{ScriptArgInt(mapClock), ScriptArgInt(0), ScriptArgString(playerName), ScriptArgString(worldTypeLabel)},
		nil, nil, nil))
	return packets
}

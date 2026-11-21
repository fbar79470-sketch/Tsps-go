package persist

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"time"
)

// PlayerSave is the JSON-serializable snapshot of a character: port of
// game/entity/impl/player/persistence/PlayerSave.ts.
//
// Field names match the TS JSON shape exactly. TS serializes the class
// instance with JSON.stringify, so every field below is a key in the
// save file, in the order the fields are assigned by fromPlayer
// (PlayerSave.ts:738-855). Go marshals struct fields in declaration
// order, which follows fromPlayer's assignment order here; JSON object
// key order is not semantically significant, so minor ordering
// differences from TS (e.g. map keys, which Go sorts) do not affect
// compatibility.
//
// FIELD COVERAGE vs PlayerSave.ts — every field on the TS class is
// represented:
//   - displayName, creationDate, passwordHashWithSalt, isDiscordLogin,
//     cachedDiscordAccessToken, rights, donatorRights, position,
//     spellBook, fightType, autocastSpellId, autoRetaliate,
//     audioSettings, hasVengeance, lastVengeanceTimer, specPercentage,
//     poisonDamage, venomed, lastVenomDamage, poisonImmunityTimer,
//     poisonClock, fireImmunityTimer, teleblockTimer, skullTimer,
//     skullType, running, runEnergy, inventory, equipment, appearance,
//     skills, quickPrayers, friends, ignores, friendRanks,
//     friendsChatChannelName, friendsChatLastOwner, friendsChatEntryRank,
//     friendsChatTalkRank, friendsChatKickRank, banks, flags, sailing,
//     attributes, unmade: all present below.
//   - fightType is json.RawMessage: the serialized FightTypeClass
//     carries animation/parentId/childId/bonusType plus style/sound
//     objects whose resolution (FightType.resolve, against the combat
//     style definitions) belongs to the combat package, not persistence.
//     The raw JSON is preserved verbatim through round-trips.
//   - sailing is json.RawMessage: SailingState (content/sailing/
//     SailingState.ts) is a large nested shape; it is preserved verbatim
//     and normalized by the sailing package, not here.
//   - spellBook.teleportType is json.RawMessage: the TeleportType object
//     graph (animations, graphics, sounds) is preserved verbatim; only
//     interfaceId is interpreted (as the JSON backend's hydrateSpellbook
//     does).
//   - attributes values are json.RawMessage: preserved verbatim. See
//     AttributeDefaultJSON / StripDefaultAttributes for the omission
//     behavior, which TS applies when building a save from a live
//     player, not when serializing.
//   - Item meta values are json.RawMessage, preserved verbatim apart
//     from the grounded key migration in Item.UnmarshalJSON.
//   - Unknown top-level keys in an old save (legacy fields such as
//     barrowsCrypt, or LEGACY_FIELD_ATTRIBUTES fields like title and
//     xpLocked, which TS carries through via Object.assign in
//     hydratePlayerSave) are preserved in Extra and re-emitted on
//     marshal, so loading and re-saving an old file does not silently
//     drop fields TS would keep. The legacy-field-to-attribute migration
//     itself (PlayerSave.ts:36-57, applyToPlayer) belongs to the player
//     package and is NOT performed here.
//
// NOT PORTED (player/game logic, not persistence):
//   - applyToPlayer / fromPlayer (building a Player from a save and
//     vice versa, including the "persistence:snapshot" plugin
//     event; the trade-escrow fold itself is FoldTradeEscrow in
//     escrow.go, called by the snapshot builder).
//   - sanitizeAppearance (needs cache ident-kit data).
//   - LEGACY_FIELD_ATTRIBUTES migration and migrateAttributeKey.
//   - unmake() (empties a save down to its credentials; trivially
//     expressible by the caller with the fields below, but its exact
//     skill-reset behavior needs the Skills type).
//
// Profile and Extra are Go-side bookkeeping and are never serialized:
// the profile is encoded in the storage key / file path, not in the
// save JSON (PlayerProfile.ts).

// Sizes and caps grounded in the TS save code.
const (
	// InventorySize / EquipmentSize: validated by
	// JsonPlayerPersistence.validateSerializedSave (28 / 14).
	InventorySize = 28
	EquipmentSize = 14
	// SkillCount: SkillManager.AMOUNT_OF_SKILLS == Skill.values().length
	// (24 skills including Sailing; Skill.ts VALUES list).
	SkillCount = 24
	// HitpointsIndex is Skill.HITPOINTS's index in the skill arrays
	// (4th in Skill.ts VALUES).
	HitpointsIndex = 3
	// AgilityIndex is Skill.AGILITY's index in the skill arrays
	// (17th in Skill.ts declaration order: ATTACK=0, DEFENCE=1,
	// STRENGTH=2, HITPOINTS=3, RANGED=4, PRAYER=5, MAGIC=6,
	// COOKING=7, WOODCUTTING=8, FLETCHING=9, FISHING=10,
	// FIREMAKING=11, CRAFTING=12, SMITHING=13, MINING=14,
	// HERBLORE=15, AGILITY=16 — each Skill takes the next index in
	// declaration order, Skill.ts nextIndex). The run-energy
	// formulas read Skills.Level[AgilityIndex] via the minimal
	// Agility current-level path (SkillManager.ts:425-433).
	AgilityIndex = 16
	// DefaultHitpointsXP is the experience for level 10 hitpoints,
	// hardcoded as 1184 in JsonPlayerPersistence.defaultSkills.
	DefaultHitpointsXP = 1184
	// MaxFriends / MaxIgnores: PlayerSave.ts MAX_FRIENDS / MAX_IGNORES.
	MaxFriends = 200
	MaxIgnores = 100
	// DefaultSpawn is the fallback position hydrateLocation uses when a
	// save has no usable position (Lumbridge).
	DefaultSpawnX, DefaultSpawnY, DefaultSpawnZ = 3089, 3524, 0
)

// PlayerSave is a character's saved snapshot.
type PlayerSave struct {
	// DisplayName is the name as the player first typed it, capitals
	// included; later logins keep it. Despite the name, this is the
	// LOGIN username for the save (fromPlayer sets it from
	// player.getUsername()); the shown display name lives in
	// DisplayNames, not here.
	DisplayName string `json:"displayName"`
	// CreationDate is epoch milliseconds.
	CreationDate int64 `json:"creationDate"`
	// PasswordHashWithSalt is "<saltHex>:<hashHex>", a legacy plaintext
	// password, or "" for a new account (see CheckPassword).
	PasswordHashWithSalt     string        `json:"passwordHashWithSalt"`
	IsDiscordLogin           bool          `json:"isDiscordLogin"`
	CachedDiscordAccessToken string        `json:"cachedDiscordAccessToken"`
	Rights                   Rights        `json:"rights"`
	DonatorRights            DonatorRights `json:"donatorRights"`
	Position                 *Location     `json:"position"`
	SpellBook                SpellBook     `json:"spellBook"`
	// FightType is preserved verbatim; see the package coverage note.
	FightType json.RawMessage `json:"fightType"`
	// AutocastSpellId is -1 when no autocast is set.
	AutocastSpellId int  `json:"autocastSpellId"`
	AutoRetaliate   bool `json:"autoRetaliate"`
	// AudioSettings maps varp id -> value (Record<number, number>).
	AudioSettings map[int]int `json:"audioSettings"`
	HasVengeance  bool        `json:"hasVengeance"`
	// LastVengeanceTimer is seconds remaining.
	LastVengeanceTimer int  `json:"lastVengeanceTimer"`
	SpecPercentage     int  `json:"specPercentage"`
	PoisonDamage       int  `json:"poisonDamage"`
	Venomed            bool `json:"venomed"`
	// LastVenomDamage is present only while venomed.
	LastVenomDamage *int `json:"lastVenomDamage,omitempty"`
	// PoisonImmunityTimer is seconds remaining.
	PoisonImmunityTimer int `json:"poisonImmunityTimer"`
	// PoisonClock is present only while poisoned/protected.
	PoisonClock *PoisonClockState `json:"poisonClock,omitempty"`
	// FireImmunityTimer is seconds remaining.
	FireImmunityTimer int `json:"fireImmunityTimer"`
	// TeleblockTimer is always 0 in a save: logging out ends Tele Block
	// (PlayerSave.ts fromPlayer/applyToPlayer comments). Older saves may
	// still hold seconds; the value is preserved verbatim.
	TeleblockTimer int `json:"teleblockTimer"`
	// SkullTimer is in ticks (as stored; TS setSkullTimer takes the
	// saved value directly).
	SkullTimer int       `json:"skullTimer"`
	SkullType  SkullType `json:"skullType"`
	Running    bool      `json:"running"`
	// RunEnergy is percent 0-100 (fractional).
	RunEnergy float64 `json:"runEnergy"`
	Inventory []Item  `json:"inventory"`
	Equipment []Item  `json:"equipment"`
	// Appearance is the 13-entry look array (gender, kits, colours).
	Appearance []int  `json:"appearance"`
	Skills     Skills `json:"skills"`
	// QuickPrayers has one entry per prayer (null = not selected); see
	// QuickPrayer.
	QuickPrayers []*QuickPrayer `json:"quickPrayers"`
	// Friends / Ignores are stringified name-hash bigints.
	Friends []string `json:"friends"`
	Ignores []string `json:"ignores"`
	// FriendRanks maps stringified name-hash -> rank.
	FriendRanks            map[string]int `json:"friendRanks"`
	FriendsChatChannelName string         `json:"friendsChatChannelName"`
	FriendsChatLastOwner   string         `json:"friendsChatLastOwner"`
	FriendsChatEntryRank   int            `json:"friendsChatEntryRank"`
	FriendsChatTalkRank    int            `json:"friendsChatTalkRank"`
	FriendsChatKickRank    int            `json:"friendsChatKickRank"`
	Banks                  Banks          `json:"banks"`
	Flags                  []string       `json:"flags"`
	// Sailing is preserved verbatim; see the package coverage note.
	Sailing json.RawMessage `json:"sailing,omitempty"`
	// Attributes holds the persisted player attributes
	// (Record<string, unknown>), preserved verbatim.
	Attributes map[string]json.RawMessage `json:"attributes"`
	// Unmade marks a save that only vouches for its account (name,
	// password, rights, remembered logins); its character is made at
	// the next login (PlayerSave.ts:196-201).
	Unmade bool `json:"unmade,omitempty"`

	// Profile is the character's profile. It is NOT serialized: the
	// profile is encoded in the storage key / file path. Stores set it
	// on load; callers must set it before saving a non-main save. The
	// zero value "" means ProfileMain.
	Profile Profile `json:"-"`
	// Extra holds unknown top-level keys found when loading, re-emitted
	// verbatim on marshal. See the coverage note above.
	Extra map[string]json.RawMessage `json:"-"`
}

// Location is a tile: {x, y, z} (model/Location.ts).
type Location struct {
	X int `json:"x"`
	Y int `json:"y"`
	Z int `json:"z"`
}

// Skills holds the parallel skill arrays (SkillManager.ts Skills class).
type Skills struct {
	Level      []int `json:"level"`
	MaxLevel   []int `json:"maxLevel"`
	Experience []int `json:"experience"`
}

// DefaultSkills returns a new character's skills: all levels 1, hitpoints
// 10 with 1184 XP (JsonPlayerPersistence.defaultSkills; Skills
// constructor in SkillManager.ts).
func DefaultSkills() Skills {
	s := Skills{
		Level:      make([]int, SkillCount),
		MaxLevel:   make([]int, SkillCount),
		Experience: make([]int, SkillCount),
	}
	for i := range s.Level {
		s.Level[i] = 1
		s.MaxLevel[i] = 1
	}
	s.Level[HitpointsIndex] = 10
	s.MaxLevel[HitpointsIndex] = 10
	s.Experience[HitpointsIndex] = DefaultHitpointsXP
	return s
}

// Item is a saved item: {id, amount, meta} (model/Item.ts). Meta is
// null when the item has none.
type Item struct {
	ID     int `json:"id"`
	Amount int `json:"amount"`
	// Meta values are preserved verbatim. A nil map marshals as null.
	Meta map[string]json.RawMessage `json:"meta"`
}

// EmptyItem is the filler for unused container slots: id -1, amount 0
// (JsonPlayerPersistence.hydrateItems pads with new Item(-1, 0)).
func EmptyItem() Item { return Item{ID: -1, Amount: 0} }

// renamedMetaKeys is Item.RENAMED_META_KEYS (Item.ts): save migration
// renaming camelCase meta keys to kebab-case. Only these keys migrate;
// other camelCase keys are still live.
var renamedMetaKeys = map[string]string{
	"toxicBlowpipe":           "toxic-blowpipe",
	"tridentCharges":          "trident-charges",
	"bloodFury":               "blood-fury",
	"tumekensShadowCharges":   "tumekens-shadow-charges",
	"barrows:lockpickCharges": "barrows:lockpick-charges",
	"farming:saplingAt":       "farming:sapling-at",
}

// UnmarshalJSON implements JsonPlayerPersistence.hydrateItem: id
// defaults to -1, amount defaults to 1 when id > 0 else 0, meta is kept
// only when it is a JSON object (otherwise null), and saved meta keys
// are migrated per Item.migrateSavedMeta (the new key wins if both are
// present).
func (it *Item) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID     *int            `json:"id"`
		Amount *int            `json:"amount"`
		Meta   json.RawMessage `json:"meta"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	it.ID = -1
	if raw.ID != nil {
		it.ID = *raw.ID
	}
	it.Amount = 0
	if raw.Amount != nil {
		it.Amount = *raw.Amount
	} else if it.ID > 0 {
		it.Amount = 1
	}
	it.Meta = nil
	if len(raw.Meta) > 0 && bytes.HasPrefix(bytes.TrimSpace(raw.Meta), []byte("{")) {
		var meta map[string]json.RawMessage
		if err := json.Unmarshal(raw.Meta, &meta); err == nil {
			it.Meta = migrateItemMeta(meta)
		}
	}
	return nil
}

// migrateItemMeta is Item.migrateSavedMeta (Item.ts).
func migrateItemMeta(meta map[string]json.RawMessage) map[string]json.RawMessage {
	for old, renamed := range renamedMetaKeys {
		value, ok := meta[old]
		if !ok {
			continue
		}
		if _, exists := meta[renamed]; !exists {
			meta[renamed] = value
		}
		delete(meta, old)
	}
	return meta
}

// Rights is a player's staff rights as serialized: {id, spriteId,
// yellTag} (model/rights/PlayerRights.ts). YellTag is a pointer because
// TS omits it when undefined (moderator/administrator/owner/developer
// are constructed without one) but serializes "" for NONE, which is
// constructed with "".
type Rights struct {
	ID       int     `json:"id"`
	SpriteID int     `json:"spriteId"`
	YellTag  *string `json:"yellTag,omitempty"`
}

// YellTagString returns the yell tag, "" when absent.
func (r Rights) YellTagString() string {
	if r.YellTag == nil {
		return ""
	}
	return *r.YellTag
}

func strptr(s string) *string { return &s }

// rightsByID is PlayerRights.fromId / the static instances
// (PlayerRights.ts:2-6, 26-34).
func rightsByID(id int) Rights {
	switch id {
	case 1:
		return Rights{ID: 1, SpriteID: 618}
	case 2:
		return Rights{ID: 2, SpriteID: 619}
	case 3:
		return Rights{ID: 3, SpriteID: 620}
	case 4:
		return Rights{ID: 4, SpriteID: 621}
	default:
		return Rights{ID: 0, SpriteID: -1, YellTag: strptr("")}
	}
}

func rightsBySpriteID(spriteID int) Rights {
	switch spriteID {
	case 618:
		return Rights{ID: 1, SpriteID: 618}
	case 619:
		return Rights{ID: 2, SpriteID: 619}
	case 620:
		return Rights{ID: 3, SpriteID: 620}
	case 621:
		return Rights{ID: 4, SpriteID: 621}
	default:
		return Rights{ID: 0, SpriteID: -1, YellTag: strptr("")}
	}
}

// UnmarshalJSON accepts the object form (verbatim) and the legacy
// numeric form (an id, resolved via fromId), matching
// JsonPlayerPersistence.hydrateRights: a bare number or an object with
// only an id resolves through fromId; otherwise fromSpriteId is the
// fallback when no id is present.
func (r *Rights) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && (trimmed[0] == '-' || (trimmed[0] >= '0' && trimmed[0] <= '9')) {
		var id int
		if err := json.Unmarshal(data, &id); err != nil {
			return err
		}
		*r = rightsByID(id)
		return nil
	}
	var raw struct {
		ID       *int    `json:"id"`
		SpriteID *int    `json:"spriteId"`
		YellTag  *string `json:"yellTag"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.ID != nil && raw.SpriteID == nil {
		*r = rightsByID(*raw.ID)
		return nil
	}
	if raw.ID == nil && raw.SpriteID != nil {
		*r = rightsBySpriteID(*raw.SpriteID)
		return nil
	}
	if raw.ID == nil && raw.SpriteID == nil {
		*r = rightsBySpriteID(-1)
		return nil
	}
	*r = Rights{ID: *raw.ID, SpriteID: *raw.SpriteID, YellTag: raw.YellTag}
	return nil
}

// DonatorRights is a player's donator rights as serialized:
// {id, spriteId, yellDelay, yellTag} (model/rights/DonatorRights.ts,
// plain objects, all four fields always present).
type DonatorRights struct {
	ID        int    `json:"id"`
	SpriteID  int    `json:"spriteId"`
	YellDelay int    `json:"yellDelay"`
	YellTag   string `json:"yellTag"`
}

func donatorRightsByID(id int) DonatorRights {
	switch id {
	case 1:
		return DonatorRights{ID: 1, SpriteID: 622, YellDelay: 40, YellTag: "[Donator]"}
	case 2:
		return DonatorRights{ID: 2, SpriteID: 623, YellDelay: 25, YellTag: "[Super Donator]"}
	case 3:
		return DonatorRights{ID: 3, SpriteID: 624, YellDelay: 10, YellTag: "[Uber Donator]"}
	default:
		return DonatorRights{ID: 0, SpriteID: -1, YellDelay: -1, YellTag: ""}
	}
}

func donatorRightsBySpriteID(spriteID int) DonatorRights {
	switch spriteID {
	case 622:
		return DonatorRights{ID: 1, SpriteID: 622, YellDelay: 40, YellTag: "[Donator]"}
	case 623:
		return DonatorRights{ID: 2, SpriteID: 623, YellDelay: 25, YellTag: "[Super Donator]"}
	case 624:
		return DonatorRights{ID: 3, SpriteID: 624, YellDelay: 10, YellTag: "[Uber Donator]"}
	default:
		return DonatorRights{ID: 0, SpriteID: -1, YellDelay: -1, YellTag: ""}
	}
}

// UnmarshalJSON mirrors Rights.UnmarshalJSON for donator rights
// (JsonPlayerPersistence.hydrateDonatorRights).
func (d *DonatorRights) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && (trimmed[0] == '-' || (trimmed[0] >= '0' && trimmed[0] <= '9')) {
		var id int
		if err := json.Unmarshal(data, &id); err != nil {
			return err
		}
		*d = donatorRightsByID(id)
		return nil
	}
	var raw struct {
		ID        *int    `json:"id"`
		SpriteID  *int    `json:"spriteId"`
		YellDelay *int    `json:"yellDelay"`
		YellTag   *string `json:"yellTag"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.ID != nil && raw.SpriteID == nil {
		*d = donatorRightsByID(*raw.ID)
		return nil
	}
	if raw.ID == nil && raw.SpriteID != nil {
		*d = donatorRightsBySpriteID(*raw.SpriteID)
		return nil
	}
	if raw.ID == nil && raw.SpriteID == nil {
		*d = donatorRightsBySpriteID(-1)
		return nil
	}
	*d = DonatorRights{ID: *raw.ID, SpriteID: *raw.SpriteID}
	if raw.YellDelay != nil {
		d.YellDelay = *raw.YellDelay
	}
	if raw.YellTag != nil {
		d.YellTag = *raw.YellTag
	}
	return nil
}

// Spellbook interface ids (model/MagicSpellbook.ts:12-15).
const (
	SpellbookNormalInterfaceID  = 1151
	SpellbookAncientInterfaceID = 12855
	SpellbookLunarInterfaceID   = 29999
	SpellbookArceusInterfaceID  = 39999
)

// SpellBook is a saved spellbook: {interfaceId, teleportType}
// (model/MagicSpellbook.ts). Hydration in TS keys off interfaceId only
// (JsonPlayerPersistence.hydrateSpellbook); teleportType is preserved
// verbatim.
type SpellBook struct {
	InterfaceID  int             `json:"interfaceId"`
	TeleportType json.RawMessage `json:"teleportType,omitempty"`
}

// SkullType is a saved skull: {iconId} (model/SkullType.ts: 0 = white,
// 1 = red).
type SkullType struct {
	IconID int `json:"iconId"`
}

// PoisonClockState is the saved poison/venom pulse state
// (content/combat/PoisonClock.ts:3-9, snapshot()).
type PoisonClockState struct {
	Counter                  int      `json:"counter"`
	TicksUntilPulse          int      `json:"ticksUntilPulse"`
	ProtectionTicksRemaining *int     `json:"protectionTicksRemaining,omitempty"`
	BriefPoisonSeconds       *float64 `json:"briefPoisonSeconds,omitempty"`
	BriefVenomSeconds        *float64 `json:"briefVenomSeconds,omitempty"`
}

// QuickPrayer is one saved quick-prayer slot: a PrayerData object or
// null (content/PrayerHandler.ts PrayerData, fields set by the
// constructor and initializeMaps). Hydration in TS keys off configId
// only (JsonPlayerPersistence.hydrateQuickPrayers/resolvePrayer); the
// remaining fields are preserved verbatim for round-trip fidelity.
type QuickPrayer struct {
	Requirement int    `json:"requirement"`
	ButtonID    int    `json:"buttonId"`
	ConfigID    int    `json:"configId"`
	DrainEffect int    `json:"drainEffect"`
	Hint        int    `json:"hint"`
	PrayerID    int    `json:"prayerId"`
	Name        string `json:"name,omitempty"`
}

// Banks maps bank tab index -> items. On disk it uses the Map
// replacer/reviver shape from JsonPlayerPersistence
// (replacer/reviver): {"__type": "Map", "entries": [[index, [items...]],
// ...]}. Unmarshal additionally accepts a plain array of [index, items]
// pairs and a plain object {"0": [...]}, matching the backend's
// entriesFrom hydration.
type Banks map[int][]Item

// MarshalJSON emits the {"__type": "Map", "entries": [...]} shape, with
// entries sorted by tab index ascending.
func (b Banks) MarshalJSON() ([]byte, error) {
	keys := make([]int, 0, len(b))
	for k := range b {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	entries := make([][2]any, 0, len(keys))
	for _, k := range keys {
		items := b[k]
		if items == nil {
			items = []Item{}
		}
		entries = append(entries, [2]any{k, items})
	}
	return json.Marshal(struct {
		Type    string   `json:"__type"`
		Entries [][2]any `json:"entries"`
	}{Type: "Map", Entries: entries})
}

// UnmarshalJSON accepts the Map-wrapper shape, an array of pairs, or a
// plain object keyed by tab index. Negative indices are dropped, as in
// hydrateBanks. A JSON null yields a nil Banks.
func (b *Banks) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if string(trimmed) == "null" {
		*b = nil
		return nil
	}
	out := Banks{}
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var wrapper struct {
			Type    string            `json:"__type"`
			Entries []json.RawMessage `json:"entries"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return err
		}
		if wrapper.Type == "Map" {
			for _, pairRaw := range wrapper.Entries {
				var pair [2]json.RawMessage
				if err := json.Unmarshal(pairRaw, &pair); err != nil {
					continue
				}
				var index int
				if err := json.Unmarshal(pair[0], &index); err != nil || index < 0 {
					continue
				}
				var items []Item
				if err := json.Unmarshal(pair[1], &items); err != nil {
					continue
				}
				out[index] = items
			}
			*b = out
			return nil
		}
		// Plain object form: {"0": [...], ...}
		var obj map[string][]Item
		if err := json.Unmarshal(data, &obj); err != nil {
			return err
		}
		for k, items := range obj {
			index, err := strconv.Atoi(k)
			if err != nil || index < 0 {
				continue
			}
			out[index] = items
		}
		*b = out
		return nil
	}
	// Array-of-pairs form: [[index, [items]], ...]
	var pairs []json.RawMessage
	if err := json.Unmarshal(data, &pairs); err != nil {
		return err
	}
	for _, pairRaw := range pairs {
		var pair [2]json.RawMessage
		if err := json.Unmarshal(pairRaw, &pair); err != nil {
			continue
		}
		var index int
		if err := json.Unmarshal(pair[0], &index); err != nil || index < 0 {
			continue
		}
		var items []Item
		if err := json.Unmarshal(pair[1], &items); err != nil {
			continue
		}
		out[index] = items
	}
	*b = out
	return nil
}

// knownSaveKeys is the set of JSON keys PlayerSave marshals. Anything
// else in a loaded file is preserved in Extra.
var knownSaveKeys = map[string]bool{
	"displayName": true, "creationDate": true, "passwordHashWithSalt": true,
	"isDiscordLogin": true, "cachedDiscordAccessToken": true, "rights": true,
	"donatorRights": true, "position": true, "spellBook": true, "fightType": true,
	"autocastSpellId": true, "autoRetaliate": true, "audioSettings": true,
	"hasVengeance": true, "lastVengeanceTimer": true, "specPercentage": true,
	"poisonDamage": true, "venomed": true, "lastVenomDamage": true,
	"poisonImmunityTimer": true, "poisonClock": true, "fireImmunityTimer": true,
	"teleblockTimer": true, "skullTimer": true, "skullType": true, "running": true,
	"runEnergy": true, "inventory": true, "equipment": true, "appearance": true,
	"skills": true, "quickPrayers": true, "friends": true, "ignores": true,
	"friendRanks": true, "friendsChatChannelName": true, "friendsChatLastOwner": true,
	"friendsChatEntryRank": true, "friendsChatTalkRank": true, "friendsChatKickRank": true,
	"banks": true, "flags": true, "sailing": true, "attributes": true, "unmade": true,
}

// UnmarshalJSON decodes the known fields and preserves unknown
// top-level keys in Extra (see the coverage note on PlayerSave).
func (s *PlayerSave) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	profile := s.Profile
	type saveAlias PlayerSave
	var alias saveAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*s = PlayerSave(alias)
	s.Profile = profile
	// A save that omits the friends-chat rank fields loads the TS
	// getter defaults (PlayerSave.ts:438-447: entry -1, talk -1,
	// kick 2 when the field is not finite — including absent or
	// null). TS-written saves always carry the fields
	// (fromPlayer writes them unconditionally); only legacy or
	// hand-made saves omit them, and without this they would
	// load as 0/0/0, changing who may enter, talk in, and be
	// kicked from the player's chat-channel. The channel
	// name/owner need no equivalent: their TS getters default
	// to "", the Go zero value.
	if _, ok := raw["friendsChatEntryRank"]; !ok || string(raw["friendsChatEntryRank"]) == "null" {
		s.FriendsChatEntryRank = -1
	}
	if _, ok := raw["friendsChatTalkRank"]; !ok || string(raw["friendsChatTalkRank"]) == "null" {
		s.FriendsChatTalkRank = -1
	}
	if _, ok := raw["friendsChatKickRank"]; !ok || string(raw["friendsChatKickRank"]) == "null" {
		s.FriendsChatKickRank = 2
	}
	s.Extra = nil
	for k, v := range raw {
		if !knownSaveKeys[k] {
			if s.Extra == nil {
				s.Extra = map[string]json.RawMessage{}
			}
			s.Extra[k] = v
		}
	}
	return nil
}

// MarshalJSON encodes the known fields and re-emits Extra keys verbatim.
// Known keys always win over Extra.
func (s PlayerSave) MarshalJSON() ([]byte, error) {
	type saveAlias PlayerSave
	data, err := json.Marshal(saveAlias(s))
	if err != nil {
		return nil, err
	}
	if len(s.Extra) == 0 {
		return data, nil
	}
	keys := make([]string, 0, len(s.Extra))
	for k := range s.Extra {
		if !knownSaveKeys[k] {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return data, nil
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.Write(data[:len(data)-1]) // drop closing '}'
	inner := bytes.TrimSpace(data[1 : len(data)-1])
	if len(inner) > 0 {
		buf.WriteByte(',')
	}
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyJSON, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(keyJSON)
		buf.WriteByte(':')
		buf.Write(s.Extra[k])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// AttributeDefaultJSON returns the JSON encoding of an attribute's
// default value, if the attribute has a registered default, consulting
// the LIVE registry (attributes.go): the core seed (the two
// PlayerSave.attributeDefaults entries, PlayerSave.ts:60-63, plus the
// fromPlayer registrations, :793-798) and every runtime persistAttribute
// registration, including plugin-registered defaults. A value equal to
// its default is NOT saved, and a save without the key loads the
// default (PlayerSave.ts:59 comment; applyAttributeDefaults). The
// omission is applied when building a save from a live player, NOT by
// the JSON stores: stores persist Attributes verbatim. Callers building
// a PlayerSave from live state must call StripDefaultAttributes first.
func AttributeDefaultJSON(key string) (string, bool) {
	return attributeDefaultJSON(key)
}

// StripDefaultAttributes removes entries whose value equals the
// attribute's registered default (compared as canonical JSON, matching
// TS's JSON.stringify comparison in fromPlayer, PlayerSave.ts:811-816),
// consulting the LIVE registry for defaults. It mutates attrs in place
// and returns it. Values that are not valid JSON are left alone.
func StripDefaultAttributes(attrs map[string]json.RawMessage) map[string]json.RawMessage {
	for key, raw := range attrs {
		def, ok := attributeDefaultJSON(key)
		if !ok {
			continue
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			continue
		}
		canonical, err := json.Marshal(value)
		if err != nil {
			continue
		}
		if string(canonical) == def {
			delete(attrs, key)
		}
	}
	return attrs
}

// Tokens reads the remembered-login entries from the save's attributes,
// filtered to those still in date (see ReadTokens). It returns nil when
// the save has no login:tokens attribute.
func (s *PlayerSave) Tokens(now time.Time) []TokenEntry {
	if s.Attributes == nil {
		return nil
	}
	raw, ok := s.Attributes[LoginTokensAttribute]
	if !ok {
		return nil
	}
	return ReadTokens(raw, now)
}

// SetTokens writes entries into the save's login:tokens attribute. An
// empty/nil list is stored as [], matching the credential-clearing write
// in PlayerPersistence.replaceCredentials. The caller is responsible
// for stripping the default (see StripDefaultAttributes) if the save is
// being built from live state; this method always writes the attribute.
func (s *PlayerSave) SetTokens(entries []TokenEntry) error {
	if entries == nil {
		entries = []TokenEntry{}
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	if s.Attributes == nil {
		s.Attributes = map[string]json.RawMessage{}
	}
	s.Attributes[LoginTokensAttribute] = data
	return nil
}

// EffectiveProfile returns s.Profile, defaulting "" to ProfileMain.
func (s *PlayerSave) EffectiveProfile() Profile {
	if s.Profile == "" {
		return ProfileMain
	}
	return s.Profile
}

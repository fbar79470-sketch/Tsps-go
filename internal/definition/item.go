package definition

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"tsps-go/internal/cache"
)

// AttackSpeedParam is ATTACK_SPEED_PARAM (ItemDefinition.ts:15):
// the cache item param for a weapon's attack speed in ticks.
const AttackSpeedParam = 14

// AttackRangeParam is ATTACK_RANGE_PARAM (ItemDefinition.ts:17):
// the cache item param for a weapon's attack range in tiles.
const AttackRangeParam = 13

var itemStatParams = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
var itemRangedStrengthParams = []int{12, 189}

const itemMagicDamageParam = 299
const itemPrayerParam = 11

// coinsItemID is ItemIdentifiers.COINS (995): the one id that is
// tradeable even when the cache says otherwise
// (ItemDefinition.ts:147).
const coinsItemID = 995

// equipmentSlots is EQUIPMENT_SLOTS (ItemDefinition.ts:7): the
// wear positions that make an item wearable.
var equipmentSlots = map[int]bool{0: true, 1: true, 2: true, 3: true, 4: true, 5: true, 7: true, 9: true, 10: true, 12: true, 13: true}

// BonusesFromParams computes an item's equipment stats from its
// cache params, or reports none when it has none of them
// (ItemDefinition.bonusesFromParams, ItemDefinition.ts:30-45).
// The result has 14 entries: stab/slash/crush/magic/ranged attack
// (0-4), the same defences (5-9), melee strength (10), ranged
// strength (11, params 12 + 189), magic damage (12, param 299 in
// tenths of a percent), prayer (13, param 11). Magic damage is
// fractional by construction (param / 10), so bonuses are
// float64.
func BonusesFromParams(params cache.ParamsMap) ([]float64, bool) {
	if params == nil {
		return nil, false
	}
	value := func(param int) float64 {
		if v, ok := cache.ParamInt(params, param); ok {
			return float64(v)
		}
		return 0
	}
	all := append(append(append([]int{}, itemStatParams...), itemRangedStrengthParams...), itemMagicDamageParam, itemPrayerParam)
	present := false
	for _, param := range all {
		if _, ok := params[param]; ok {
			present = true
			break
		}
	}
	if !present {
		return nil, false
	}
	bonuses := make([]float64, 0, 14)
	for _, param := range itemStatParams {
		bonuses = append(bonuses, value(param))
	}
	rangedStrength := 0.0
	for _, param := range itemRangedStrengthParams {
		rangedStrength += value(param)
	}
	bonuses = append(bonuses, rangedStrength, value(itemMagicDamageParam)/10, value(itemPrayerParam))
	return bonuses, true
}

// ItemDefinition is the JSON/gameplay layer over a cache item
// type (ItemDefinition.ts). Instances are built by Store.Item,
// which hydrates from the cache and applies the loaded JSON
// overlays; each call returns a fresh value, so callers may hold
// it without aliasing shared state. (TypeScript caches one
// mutable definition per id; the Go wrappers are values because
// no Go system mutates definitions after load.)
//
// The TS DEFAULT shape (forId for an out-of-range id, or with no
// cache here) has ID -1 — TS leaves id undefined, which Go
// represents as -1, matching the NpcDefinition default — an empty
// name, default animations, and zero/absent values.
type ItemDefinition struct {
	id                   int
	name                 string
	examine              string
	weaponInterface      string
	equipmentSlot        int
	doubleHanded         bool
	stackable            bool
	tradeable            bool
	dropable             bool
	sellable             bool
	noted                bool
	value                int
	grandExchangeValue   int
	grandExchangeSellVal int
	bloodMoneyValue      int
	highAlch             int
	hasHighAlch          bool
	lowAlch              int
	hasLowAlch           bool
	dropValue            int
	noteID               int
	placeholderID        int
	blockAnim            int
	standAnim            int
	walkAnim             int
	runAnim              int
	standTurnAnim        int
	turn180Anim          int
	turn90CWAnim         int
	turn90CCWAnim        int
	attackAnim           int
	attackAnims          map[string]int
	equipSound           int
	deathComponents      []int
	attackSpeed          int
	attackRange          int
	weight               int
	bonuses              []float64
	requirements         []int
	combatRequirement    int
	cacheBonuses         bool
}

// newDefaultItemDefinition returns the TS DEFAULT shape: field
// initializers from ItemDefinition.ts:74-121 with no cache
// hydration. Numeric fields TS leaves undefined read as 0 here;
// the getters that distinguish "unset" (grand exchange values,
// bonuses) use their > 0 / nil checks, as in TS.
func newDefaultItemDefinition() *ItemDefinition {
	return &ItemDefinition{
		id:            -1,
		equipmentSlot: -1,
		noteID:        -1,
		placeholderID: -1,
		blockAnim:     424,
		standAnim:     808,
		walkAnim:      819,
		runAnim:       824,
		standTurnAnim: 823,
		turn180Anim:   820,
		turn90CWAnim:  821,
		turn90CCWAnim: 822,
		attackAnim:    -1,
		equipSound:    -1,
		attackSpeed:   -1,
		attackRange:   -1,
	}
}

// hydrateFromCache fills the definition from a cache item type
// (ItemDefinition.ts:140-171). It runs once per built definition.
func (d *ItemDefinition) hydrateFromCache(id int, cached *cache.ObjType) {
	d.id = id
	d.name = cached.Name
	if cached.Examine != nil {
		d.examine = *cached.Examine
	}
	d.stackable = cached.Stackability == cache.ObjStackabilityAlways
	d.tradeable = cached.IsTradable || id == coinsItemID
	d.dropable = len(cached.InventoryActions) > 4 && strings.ToLower(cached.InventoryActions[4]) == "drop"
	d.noted = cached.NoteTemplate != -1
	d.noteID = cached.Note
	// Placeholders have their own cache entry; items without one
	// leave no placeholder (ItemDefinition.ts:151-153).
	if cached.PlaceholderTemplate == -1 {
		d.placeholderID = cached.Placeholder
	} else {
		d.placeholderID = -1
	}
	d.value = cached.Price
	d.weight = cached.Weight
	if bonuses, ok := BonusesFromParams(cached.Params); ok {
		d.bonuses = bonuses
		d.cacheBonuses = true
	}
	if v, ok := cache.ParamInt(cached.Params, AttackSpeedParam); ok && v > 0 {
		d.attackSpeed = v
	} else {
		d.attackSpeed = -1
	}
	if v, ok := cache.ParamInt(cached.Params, AttackRangeParam); ok && v > 0 {
		d.attackRange = v
	} else {
		d.attackRange = -1
	}
	if d.equipmentSlot == -1 && equipmentSlots[cached.WearPos] {
		d.equipmentSlot = cached.WearPos
	}
	// A two-handed weapon also takes the shield slot (its second
	// wear position).
	d.doubleHanded = cached.WearPos == 3 && cached.WearPos2 == 5
}

// Item returns the item definition for an id, hydrated from the
// cache with the loaded JSON overlays applied (ItemDefinition.
// forId, ItemDefinition.ts:127-138). Out-of-range ids, and every
// id with no cache, return the DEFAULT shape. Custom-registered
// items (registerCustom) are not ported — see the package
// comment — so no out-of-range id ever resolves to a definition.
func (s *Store) Item(id int) *ItemDefinition {
	def := newDefaultItemDefinition()
	if s.cache == nil || id < 0 || id >= s.itemCount() {
		return def
	}
	def.hydrateFromCache(id, s.cache.GetItem(id))
	if overlay, ok := s.itemOverlays[id]; ok {
		overlay.apply(def)
	}
	return def
}

// ID returns the item id (-1 for the DEFAULT shape).
func (d *ItemDefinition) ID() int { return d.id }

// Name returns the item name.
func (d *ItemDefinition) Name() string { return d.name }

// Examine returns the examine text (the cache examine, or an
// override's).
func (d *ItemDefinition) Examine() string { return d.examine }

// Value returns the store value.
func (d *ItemDefinition) Value() int { return d.value }

// GrandExchangeValue returns the OSRS Grand Exchange quote from
// item-prices.json, falling back to the store value.
func (d *ItemDefinition) GrandExchangeValue() int {
	if d.grandExchangeValue > 0 {
		return d.grandExchangeValue
	}
	return d.value
}

// HasGrandExchangeValue reports whether item-prices.json has a
// Grand Exchange quote for this item.
func (d *ItemDefinition) HasGrandExchangeValue() bool { return d.grandExchangeValue > 0 }

// GrandExchangeSellValue returns the guide sell quote, falling
// back to the buy quote when there is none.
func (d *ItemDefinition) GrandExchangeSellValue() int {
	if d.grandExchangeSellVal > 0 {
		return d.grandExchangeSellVal
	}
	return d.GrandExchangeValue()
}

// BloodMoneyValue returns the blood money value.
func (d *ItemDefinition) BloodMoneyValue() int { return d.bloodMoneyValue }

// HighAlchValue returns the high alchemy value.
func (d *ItemDefinition) HighAlchValue() int { return d.highAlch }

// HasHighAlchValue reports whether a high alchemy value was ever
// set for this item by any JSON stage (gameplay, alch, or
// overrides). TS distinguishes an unset value (getHighAlchValue
// returns undefined, and the examine price line falls back to
// 60% of the store value) from a set 0 (an item the alchemy
// spells refuse, such as a bond, whose price line shows no HA
// value); the flag carries that distinction, which the bare
// integer cannot. It is set by apply whenever an overlay stage
// sets the value — including item-alch.json's explicit 0 for an
// entry with no valid number, exactly as TS stores 0 there.
func (d *ItemDefinition) HasHighAlchValue() bool { return d.hasHighAlch }

// LowAlchValue returns the low alchemy value.
func (d *ItemDefinition) LowAlchValue() int { return d.lowAlch }

// HasLowAlchValue reports whether a low alchemy value was ever
// set for this item, as HasHighAlchValue does for the high value
// (unset falls back to 40% of the store value).
func (d *ItemDefinition) HasLowAlchValue() bool { return d.hasLowAlch }

// DropValue returns the drop value.
func (d *ItemDefinition) DropValue() int { return d.dropValue }

// IsStackable reports whether the item stacks.
func (d *ItemDefinition) IsStackable() bool { return d.stackable }

// IsTradeable reports whether the item is tradeable.
func (d *ItemDefinition) IsTradeable() bool { return d.tradeable }

// IsSellable reports whether the item is sellable.
func (d *ItemDefinition) IsSellable() bool { return d.sellable }

// IsDropable reports whether the item is dropable.
func (d *ItemDefinition) IsDropable() bool { return d.dropable }

// IsNoted reports whether the item is a bank note.
func (d *ItemDefinition) IsNoted() bool { return d.noted }

// PlaceholderID returns the faded bank placeholder id, or -1.
func (d *ItemDefinition) PlaceholderID() int { return d.placeholderID }

// NoteID returns the note id.
func (d *ItemDefinition) NoteID() int { return d.noteID }

// IsDoubleHanded reports whether the item takes both hand slots.
func (d *ItemDefinition) IsDoubleHanded() bool { return d.doubleHanded }

// BlockAnim returns the block animation.
func (d *ItemDefinition) BlockAnim() int { return d.blockAnim }

// StandAnim returns the stand animation.
func (d *ItemDefinition) StandAnim() int { return d.standAnim }

// WalkAnim returns the walk animation.
func (d *ItemDefinition) WalkAnim() int { return d.walkAnim }

// RunAnim returns the run animation.
func (d *ItemDefinition) RunAnim() int { return d.runAnim }

// StandTurnAnim returns the stand-turn animation.
func (d *ItemDefinition) StandTurnAnim() int { return d.standTurnAnim }

// Turn180Anim returns the 180-degree turn animation.
func (d *ItemDefinition) Turn180Anim() int { return d.turn180Anim }

// Turn90CWAnim returns the 90-degree clockwise turn animation.
func (d *ItemDefinition) Turn90CWAnim() int { return d.turn90CWAnim }

// Turn90CCWAnim returns the 90-degree counter-clockwise turn
// animation.
func (d *ItemDefinition) Turn90CCWAnim() int { return d.turn90CCWAnim }

// Weight returns the item weight (0 with no cache, as TS leaves
// it undefined until hydration).
func (d *ItemDefinition) Weight() int { return d.weight }

// Bonuses returns the equipment bonuses, or nil when the item has
// none.
func (d *ItemDefinition) Bonuses() []float64 {
	if d.bonuses == nil {
		return nil
	}
	return append([]float64(nil), d.bonuses...)
}

// HasCacheBonuses reports whether Bonuses comes from the cache's
// stat params.
func (d *ItemDefinition) HasCacheBonuses() bool { return d.cacheBonuses }

// Requirements returns the skill requirements, or nil.
func (d *ItemDefinition) Requirements() []int {
	if d.requirements == nil {
		return nil
	}
	return append([]int(nil), d.requirements...)
}

// CombatRequirement returns the combat level needed to wear this
// item, from item-gameplay.json (0 when unset).
func (d *ItemDefinition) CombatRequirement() int { return d.combatRequirement }

// AttackAnim returns this weapon's own animation for an attack of
// attackType ("stab", "slash", "crush", "magic", "range"), or -1
// for its type's (ItemDefinition.ts:308-311). In the per-type map
// form, a missing type — or a stored 0, which is falsy in TS —
// yields -1; in the single-number form the number is returned
// as-is.
func (d *ItemDefinition) AttackAnim(attackType string) int {
	if d.attackAnims == nil {
		return d.attackAnim
	}
	if attackType == "" {
		return -1
	}
	if v, ok := d.attackAnims[attackType]; ok && v != 0 {
		return v
	}
	return -1
}

// EquipSound returns the equip sound id (-1: by kind, see
// EquipmentSounds).
func (d *ItemDefinition) EquipSound() int { return d.equipSound }

// DeathComponents returns the tradeable items this splits into
// when lost in the Wilderness.
func (d *ItemDefinition) DeathComponents() []int {
	return append([]int(nil), d.deathComponents...)
}

// AttackSpeed returns the ticks between attacks from the cache,
// or -1.
func (d *ItemDefinition) AttackSpeed() int { return d.attackSpeed }

// AttackRange returns the weapon reach in tiles from the cache,
// or -1.
func (d *ItemDefinition) AttackRange() int { return d.attackRange }

// WeaponInterfaceName returns the weapon interface name from
// item-gameplay.json, or "" when the item has none. TypeScript
// resolves the name against the WeaponInterfaces table loaded
// from item-combat-styles.json (an unknown name resolves to
// null); that table is not ported, so names are carried
// unvalidated — see the package comment.
func (d *ItemDefinition) WeaponInterfaceName() string { return d.weaponInterface }

// EquipmentSlot returns the equipment slot (-1 when unwearable),
// from the cache wearPos (EquipmentType.getSlot()).
func (d *ItemDefinition) EquipmentSlot() int { return d.equipmentSlot }

// UnNote returns the unnoted id for a note, else the item's own
// id.
func (d *ItemDefinition) UnNote() int {
	if d.noted && d.noteID >= 0 {
		return d.noteID
	}
	return d.id
}

// itemOverlay is the accumulated JSON application for one item
// id, in plugin order: item-gameplay.json, then item-prices.json,
// then item-alch.json, then item-overrides.json
// (ItemDefinitionLoader.plugin.js). Each field is nil when no
// stage set it; apply overwrites the hydrated definition.
type itemOverlay struct {
	weaponInterface   *string
	equipmentSlot     *int
	doubleHanded      *bool
	sellable          *bool
	tradeable         *bool
	bloodMoneyValue   *int
	highAlch          *int
	lowAlch           *int
	dropValue         *int
	value             *int
	grandExchange     *int
	grandExchangeSell *int
	examine           *string
	blockAnim         *int
	standAnim         *int
	walkAnim          *int
	runAnim           *int
	standTurnAnim     *int
	turn180Anim       *int
	turn90CWAnim      *int
	turn90CCWAnim     *int
	attackAnim        *int
	attackAnims       map[string]int
	equipSound        *int
	bonuses           []float64
	requirements      []int
	deathComponents   []int
	combatRequirement *int
	attackSpeed       *int
}

func (o *itemOverlay) apply(d *ItemDefinition) {
	if o.weaponInterface != nil {
		d.weaponInterface = *o.weaponInterface
	}
	if o.equipmentSlot != nil {
		d.equipmentSlot = *o.equipmentSlot
	}
	if o.doubleHanded != nil {
		d.doubleHanded = *o.doubleHanded
	}
	if o.sellable != nil {
		d.sellable = *o.sellable
	}
	if o.tradeable != nil {
		d.tradeable = *o.tradeable
	}
	if o.bloodMoneyValue != nil {
		d.bloodMoneyValue = *o.bloodMoneyValue
	}
	if o.highAlch != nil {
		d.highAlch = *o.highAlch
		d.hasHighAlch = true
	}
	if o.lowAlch != nil {
		d.lowAlch = *o.lowAlch
		d.hasLowAlch = true
	}
	if o.dropValue != nil {
		d.dropValue = *o.dropValue
	}
	if o.value != nil {
		d.value = *o.value
	}
	if o.grandExchange != nil {
		d.grandExchangeValue = *o.grandExchange
	}
	if o.grandExchangeSell != nil {
		d.grandExchangeSellVal = *o.grandExchangeSell
	}
	if o.examine != nil {
		d.examine = *o.examine
	}
	if o.blockAnim != nil {
		d.blockAnim = *o.blockAnim
	}
	if o.standAnim != nil {
		d.standAnim = *o.standAnim
	}
	if o.walkAnim != nil {
		d.walkAnim = *o.walkAnim
	}
	if o.runAnim != nil {
		d.runAnim = *o.runAnim
	}
	if o.standTurnAnim != nil {
		d.standTurnAnim = *o.standTurnAnim
	}
	if o.turn180Anim != nil {
		d.turn180Anim = *o.turn180Anim
	}
	if o.turn90CWAnim != nil {
		d.turn90CWAnim = *o.turn90CWAnim
	}
	if o.turn90CCWAnim != nil {
		d.turn90CCWAnim = *o.turn90CCWAnim
	}
	if o.attackAnims != nil {
		d.attackAnims = o.attackAnims
	} else if o.attackAnim != nil {
		d.attackAnim = *o.attackAnim
	}
	if o.equipSound != nil {
		d.equipSound = *o.equipSound
	}
	if o.bonuses != nil {
		d.bonuses = append([]float64(nil), o.bonuses...)
	}
	if o.requirements != nil {
		d.requirements = append([]int(nil), o.requirements...)
	}
	if o.deathComponents != nil {
		d.deathComponents = append([]int(nil), o.deathComponents...)
	}
	if o.combatRequirement != nil {
		d.combatRequirement = *o.combatRequirement
	}
	if o.attackSpeed != nil {
		d.attackSpeed = *o.attackSpeed
	}
}

// loadItemJSON applies the item JSON files in plugin order. All
// four files are optional when missing: prices, alch and
// overrides check existsSync in TS, and item-gameplay.json —
// which the TS plugin requires with readFileSync — is treated as
// optional here as a deliberate deviation, so a fixture
// definitions directory without the 11.6 MB export still boots;
// a present but malformed file fails Load, as in TS. With no
// cache, no application happens at all: the plugin's name guard
// and forId DEFAULT checks need cache names, and every entry
// would be skipped (or, for an empty-named entry, mutate the
// shared DEFAULT — which Go refuses to reproduce).
func (s *Store) loadItemJSON() error {
	if s.cache == nil {
		return nil
	}
	if err := s.applyItemGameplay(); err != nil {
		return err
	}
	if err := s.applyItemPrices(); err != nil {
		return err
	}
	if err := s.applyItemAlch(); err != nil {
		return err
	}
	if err := s.applyItemOverrides(); err != nil {
		return err
	}
	return nil
}

func (s *Store) itemOverlayFor(id int) *itemOverlay {
	overlay, ok := s.itemOverlays[id]
	if !ok {
		overlay = &itemOverlay{}
		s.itemOverlays[id] = overlay
	}
	return overlay
}

// inCacheRange reports whether an id resolves to a real cache
// definition (the plugin's `definition === ItemDefinition.DEFAULT`
// check, via forId's range test).
func (s *Store) inCacheRange(id int) bool {
	return id >= 0 && id < s.itemCount()
}

// applyItemGameplay ports loadItemDefinitions' gameplay loop
// (ItemDefinitionLoader.plugin.js:239-288): the name guard
// (dump name must equal the cache name, case-insensitively),
// the property copy list, the cache-bonuses precedence, and the
// Barrows degradation. The equipment slot is the cache wearPos,
// never the export's equipmentType; two-handedness, stackable and
// tradeable are the cache's too. The custom-items portion of the
// plugin is NOT ported here (overlay work).
func (s *Store) applyItemGameplay() error {
	value, err := readJSONValueOptional(filepath.Join(s.dir, "item-gameplay.json"))
	if err != nil {
		return err
	}
	if value == nil {
		return nil
	}
	var defs []any
	switch v := value.(type) {
	case []any:
		defs = v
	case map[string]any:
		// Object.values order: integer-like keys ascending,
		// then string keys in insertion order. Go maps lose
		// insertion order, so string keys iterate sorted
		// (sortedNumericKeys); per-entry outcomes are
		// independent, so only iteration order differs.
		for _, key := range sortedNumericKeys(v) {
			defs = append(defs, v[key])
		}
	default:
		// Object.values of a non-object is [] in TS; an array
		// check above already handled arrays.
		return nil
	}
	loaded, unresolved, mismatched := 0, 0, 0
	for _, rawDef := range defs {
		raw, ok := rawDef.(map[string]any)
		if !ok {
			continue
		}
		idNum, ok := numberValue(mustNum(raw, "id"))
		if !ok || !isJSInteger(idNum) || idNum < 0 {
			continue
		}
		id := jsTrunc(idNum)
		if !s.inCacheRange(id) {
			continue
		}
		rawName, _ := mustNum(raw, "name").(string)
		cacheName := s.cache.GetItem(id).Name
		if strings.ToLower(strings.TrimSpace(rawName)) != strings.ToLower(strings.TrimSpace(cacheName)) {
			mismatched++
			continue
		}
		overlay := s.itemOverlayFor(id)
		// weaponInterface: hydrateWeaponInterface — a string
		// name resolves against the WeaponInterfaces table or
		// to null. The table is not ported, so a string name is
		// kept; a non-string non-null value resolves to null.
		if v, present := field(raw, "weaponInterface"); present && v != nil {
			if name, ok := v.(string); ok {
				overlay.weaponInterface = &name
			} else {
				empty := ""
				overlay.weaponInterface = &empty
				unresolved++
			}
		}
		def := s.Item(id) // hydrated, for the cache-bonuses check
		copyInt := func(key string, dst **int) {
			if v, present := field(raw, key); present {
				if n, ok := numberValue(v); ok && isFiniteNumber(n) {
					iv := jsTrunc(n)
					*dst = &iv
				}
			}
		}
		if v, present := field(raw, "sellable"); present {
			if b, ok := v.(bool); ok {
				overlay.sellable = &b
			}
		}
		copyInt("bloodMoneyValue", &overlay.bloodMoneyValue)
		copyInt("highAlch", &overlay.highAlch)
		copyInt("lowAlch", &overlay.lowAlch)
		copyInt("dropValue", &overlay.dropValue)
		copyInt("blockAnim", &overlay.blockAnim)
		copyInt("standAnim", &overlay.standAnim)
		copyInt("walkAnim", &overlay.walkAnim)
		copyInt("runAnim", &overlay.runAnim)
		copyInt("standTurnAnim", &overlay.standTurnAnim)
		copyInt("turn180Anim", &overlay.turn180Anim)
		copyInt("turn90CWAnim", &overlay.turn90CWAnim)
		copyInt("turn90CCWAnim", &overlay.turn90CCWAnim)
		if v, present := field(raw, "attackAnim"); present {
			if n, ok := numberValue(v); ok && isFiniteNumber(n) {
				iv := jsTrunc(n)
				overlay.attackAnim = &iv
				overlay.attackAnims = nil
			} else if obj, ok := v.(map[string]any); ok {
				anims := make(map[string]int)
				for key, val := range obj {
					if n, ok := numberValue(val); ok && isFiniteNumber(n) {
						anims[key] = jsTrunc(n)
					}
				}
				overlay.attackAnims = anims
			}
		}
		copyInt("equipSound", &overlay.equipSound)
		// The cache's stat params are the current game's; the
		// export's bonuses only fill items the cache has none
		// for.
		if !def.HasCacheBonuses() {
			if v, present := field(raw, "bonuses"); present {
				if arr, ok := v.([]any); ok {
					bonuses := make([]float64, len(arr))
					valid := true
					for i, elem := range arr {
						n, ok := numberValue(elem)
						if !ok {
							valid = false
							break
						}
						bonuses[i] = n
					}
					if valid {
						overlay.bonuses = bonuses
					}
				}
			}
		}
		if v, present := field(raw, "requirements"); present {
			if arr, ok := v.([]any); ok {
				reqs := make([]int, len(arr))
				valid := true
				for i, elem := range arr {
					n, ok := numberValue(elem)
					if !ok {
						valid = false
						break
					}
					reqs[i] = jsTrunc(n)
				}
				if valid {
					overlay.requirements = reqs
				}
			}
		}
		if v, present := field(raw, "deathComponents"); present {
			if arr, ok := v.([]any); ok {
				comps := make([]int, 0, len(arr))
				valid := true
				for _, elem := range arr {
					n, ok := numberValue(elem)
					if !ok {
						valid = false
						break
					}
					comps = append(comps, jsTrunc(n))
				}
				if valid {
					overlay.deathComponents = comps
				}
			}
		}
		copyInt("combatRequirement", &overlay.combatRequirement)
		loaded++
	}
	s.applyBarrowsDegradation()
	s.logf("[items] Loaded %d gameplay definitions (mismatched=%d, unresolvedWeaponInterfaces=%d)", loaded, mismatched, unresolved)
	return nil
}

// barrowsBaseItems is BARROWS_BASE_ITEMS
// (ItemDefinitionLoader.plugin.js:68-72).
var barrowsBaseItems = []int{
	4708, 4710, 4712, 4714, 4716, 4718, 4720, 4722,
	4724, 4726, 4728, 4730, 4732, 4734, 4736, 4738,
	4745, 4747, 4749, 4751, 4753, 4755, 4757, 4759,
}

// applyBarrowsDegradation ports the Barrows loop
// (ItemDefinitionLoader.plugin.js:274-286): degraded pieces wear
// as their base piece; the broken ("0", stage 4) pieces get zero
// bonuses. TS spreads base.requirements unconditionally, which
// throws when a base has none; Go copies only when present — a
// hardening, since the shipped export gives every base
// requirements. Variant ids outside the cache range are skipped
// rather than mutating the shared DEFAULT, as TS would.
func (s *Store) applyBarrowsDegradation() {
	for index, baseID := range barrowsBaseItems {
		if !s.inCacheRange(baseID) {
			continue
		}
		base := s.Item(baseID)
		for stage := 0; stage < 5; stage++ {
			variantID := 4856 + index*6 + stage
			if !s.inCacheRange(variantID) {
				continue
			}
			overlay := s.itemOverlayFor(variantID)
			slot := base.EquipmentSlot()
			overlay.equipmentSlot = &slot
			iface := base.WeaponInterfaceName()
			overlay.weaponInterface = &iface
			dh := base.IsDoubleHanded()
			overlay.doubleHanded = &dh
			if reqs := base.Requirements(); reqs != nil {
				overlay.requirements = append([]int(nil), reqs...)
			}
			if stage == 4 {
				overlay.bonuses = make([]float64, 14)
			}
		}
	}
}

// applyItemPrices ports loadItemPrices
// (ItemDefinitionLoader.plugin.js:110-153): {data: {id: {high,
// low}}} quotes, buy = max, sell = min (inverted live quotes
// cannot be arbitraged), propagated to the paired note.
func (s *Store) applyItemPrices() error {
	value, err := readJSONValueOptional(filepath.Join(s.dir, "item-prices.json"))
	if err != nil {
		return err
	}
	if value == nil {
		return nil
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("Invalid item-prices.json: expected a data object")
	}
	data, ok := obj["data"].(map[string]any)
	if !ok {
		return fmt.Errorf("Invalid item-prices.json: expected a data object")
	}
	loaded := 0
	for _, key := range sortedNumericKeys(data) {
		id := jsTrunc(jsParseNumber(key))
		if !isJSInteger(jsParseNumber(key)) || id < 0 || !s.inCacheRange(id) {
			continue
		}
		quote, _ := data[key].(map[string]any)
		var prices []int
		for _, fieldName := range []string{"high", "low"} {
			if n, ok := numberValue(mustNum(quote, fieldName)); ok && isJSSafeInteger(n) && n > 0 {
				prices = append(prices, jsTrunc(n))
			}
		}
		if len(prices) == 0 {
			continue
		}
		buy, sell := prices[0], prices[0]
		for _, p := range prices {
			if p > buy {
				buy = p
			}
			if p < sell {
				sell = p
			}
		}
		overlay := s.itemOverlayFor(id)
		overlay.grandExchange = &buy
		overlay.grandExchangeSell = &sell
		s.propagateToNote(id, func(note *itemOverlay) {
			note.grandExchange = &buy
			note.grandExchangeSell = &sell
		})
		loaded++
	}
	s.logf("[items] Loaded %d price quotes", loaded)
	return nil
}

// applyItemAlch ports loadItemAlch
// (ItemDefinitionLoader.plugin.js:157-192): {id: {highalch,
// lowalch}} values, 0 when not a safe integer, propagated to the
// paired note. It runs after gameplay so these values win over
// the export's.
func (s *Store) applyItemAlch() error {
	value, err := readJSONValueOptional(filepath.Join(s.dir, "item-alch.json"))
	if err != nil {
		return err
	}
	if value == nil {
		return nil
	}
	data, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("Invalid item-alch.json: expected an object keyed by item id")
	}
	loaded := 0
	for _, key := range sortedNumericKeys(data) {
		id := jsTrunc(jsParseNumber(key))
		if !isJSInteger(jsParseNumber(key)) || id < 0 || !s.inCacheRange(id) {
			continue
		}
		values, _ := data[key].(map[string]any)
		high, low := 0, 0
		if n, ok := numberValue(mustNum(values, "highalch")); ok && isJSSafeInteger(n) {
			high = jsTrunc(n)
		}
		if n, ok := numberValue(mustNum(values, "lowalch")); ok && isJSSafeInteger(n) {
			low = jsTrunc(n)
		}
		overlay := s.itemOverlayFor(id)
		overlay.highAlch = &high
		overlay.lowAlch = &low
		s.propagateToNote(id, func(note *itemOverlay) {
			note.highAlch = &high
			note.lowAlch = &low
		})
		loaded++
	}
	s.logf("[items] Loaded %d alchemy values", loaded)
	return nil
}

// propagateToNote applies fn to the paired note's overlay when
// the item is unnoted, has a note id, and the note's own note id
// points back — the pairing check shared by loadItemPrices and
// loadItemAlch.
func (s *Store) propagateToNote(id int, fn func(note *itemOverlay)) {
	def := s.Item(id)
	noteID := def.NoteID()
	if def.IsNoted() || noteID < 0 || !s.inCacheRange(noteID) {
		return
	}
	note := s.Item(noteID)
	if note.IsNoted() && note.NoteID() == id {
		fn(s.itemOverlayFor(noteID))
	}
}

// applyItemOverrides ports loadItemOverrides/applyItemOverrides
// (ItemDefinitionLoader.plugin.js:194-236): a world's own item
// changes, applied last so they win over cache, gameplay, alch
// and prices. Unknown items and invalid fields are skipped with
// a warning rather than stopping the load.
func (s *Store) applyItemOverrides() error {
	value, err := readJSONValueOptional(filepath.Join(s.dir, "item-overrides.json"))
	if err != nil {
		return err
	}
	if value == nil {
		return nil
	}
	overrides := asKeyedObject(value)
	loaded := 0
	for _, key := range sortedNumericKeys(overrides) {
		idNum := jsParseNumber(key)
		if !isJSInteger(idNum) || jsTrunc(idNum) < 0 || !s.inCacheRange(jsTrunc(idNum)) {
			s.logf("[item-overrides] skipped unknown item %s", key)
			continue
		}
		id := jsTrunc(idNum)
		fields, ok := overrides[key].(map[string]any)
		if !ok {
			s.logf("[item-overrides] skipped unknown item %s", key)
			continue
		}
		overlay := s.itemOverlayFor(id)
		for fieldName, fieldValue := range fields {
			if !applyItemOverrideField(overlay, fieldName, fieldValue) {
				s.logf("[item-overrides] skipped invalid %s for item %s", fieldName, key)
			}
		}
		loaded++
	}
	s.logf("[items] Loaded %d item overrides", loaded)
	return nil
}

// applyItemOverrideField validates and applies one override
// field per OVERRIDE_FIELDS (ItemDefinitionLoader.plugin.js:
// 196-204). Unknown fields are invalid, as in TS (the ?.() call
// on a missing validator yields undefined, which is falsy).
func applyItemOverrideField(overlay *itemOverlay, fieldName string, value any) bool {
	amount := func() (int, bool) {
		n, ok := numberValue(value)
		if !ok || !isJSSafeInteger(n) || n < 0 {
			return 0, false
		}
		return jsTrunc(n), true
	}
	boolean := func() (bool, bool) {
		b, ok := value.(bool)
		return b, ok
	}
	switch fieldName {
	case "value", "grandExchangeValue", "highAlch", "lowAlch", "dropValue":
		amountValue, ok := amount()
		if !ok {
			return false
		}
		switch fieldName {
		case "value":
			overlay.value = &amountValue
		case "grandExchangeValue":
			overlay.grandExchange = &amountValue
		case "highAlch":
			overlay.highAlch = &amountValue
		case "lowAlch":
			overlay.lowAlch = &amountValue
		case "dropValue":
			overlay.dropValue = &amountValue
		}
		return true
	case "tradeable", "sellable":
		b, ok := boolean()
		if !ok {
			return false
		}
		if fieldName == "tradeable" {
			overlay.tradeable = &b
		} else {
			overlay.sellable = &b
		}
		return true
	case "attackSpeed":
		n, ok := numberValue(value)
		if !ok || !isJSInteger(n) || n < 1 || n > 20 {
			return false
		}
		v := jsTrunc(n)
		overlay.attackSpeed = &v
		return true
	case "bonuses":
		arr, ok := value.([]any)
		if !ok || len(arr) != 14 {
			return false
		}
		bonuses := make([]float64, 14)
		for i, elem := range arr {
			n, ok := numberValue(elem)
			if !ok || !isJSInteger(n) || math.Abs(n) > 10000 {
				return false
			}
			bonuses[i] = n
		}
		overlay.bonuses = bonuses
		return true
	case "requirements":
		arr, ok := value.([]any)
		if !ok || len(arr) != 23 {
			return false
		}
		reqs := make([]int, 23)
		for i, elem := range arr {
			n, ok := numberValue(elem)
			if !ok || !isJSInteger(n) || n < 0 || n > 99 {
				return false
			}
			reqs[i] = jsTrunc(n)
		}
		overlay.requirements = reqs
		return true
	case "examine":
		text, ok := value.(string)
		if !ok || utf16Len(text) > 200 {
			return false
		}
		overlay.examine = &text
		return true
	default:
		return false
	}
}

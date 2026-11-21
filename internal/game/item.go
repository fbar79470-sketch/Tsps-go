package game

import (
	"bytes"
	"encoding/json"
)

// Item is the port of game/model/Item.ts (244 lines): one container
// slot's contents — an id, an amount, and optional metadata.
//
// VALUE SEMANTICS. TS Item is a mutable reference type: containers
// hold Item objects, and callers mutate the object in a slot through
// the live array getItems() returns. Go Item is a value type:
// containers hold Item values, every accessor that hands an Item out
// (Items, ItemAt, Get, GetValidItems) returns a deep copy, and
// mutation goes through the container (SetItem) or through pointer
// receivers on a caller-owned copy. Code that in TS mutates a
// returned Item in place must read-modify-write through the
// container in Go; nothing here aliases a container slot.
//
// META REPRESENTATION. Meta is map[string]json.RawMessage — the same
// shape as persist.Item.Meta — so the save layer converts losslessly
// by copying the map: load builds NewItemWithMeta(id, amount, meta),
// snapshot reads Meta(). Raw values are copied byte-for-byte, never
// re-encoded, so unknown meta survives the round trip verbatim. The
// one normalization is equality: Equals compares canonical JSON (see
// metaFingerprint), as TS compares JSON.stringify results.
//
// An empty slot holds id -1, amount 0 (ItemContainer.ts:55-56 fills
// every slot with new Item(-1, 0)); see IsEmpty/IsValid.
type Item struct {
	id     int
	amount int
	meta   map[string]json.RawMessage
}

// Meta keys Item.ts:9-11 defines as constants.
const (
	// ItemUntradeableMeta (UNTRADEABLE_META) marks an item that
	// cannot be traded, sold, or dropped (Item.ts:210-212).
	ItemUntradeableMeta = "untradeable"
	// ItemUnbankableMeta (UNBANKABLE_META) marks an item that cannot
	// be banked (Item.ts:214-216).
	ItemUnbankableMeta = "unbankable"
	// ItemPresetMeta (PRESET_META) marks an item spawned by a preset
	// (Item.ts:218-224).
	ItemPresetMeta = "preset"
)

// NewItem returns an Item with id and amount and no meta
// (Item constructor, Item.ts:25-29; the TS amount defaults to 1 when
// omitted, but Go callers always pass it — use NewItem(id, 1) for
// the TS one-argument form). No clamping: TS stores the arguments
// as given.
func NewItem(id, amount int) Item {
	return Item{id: id, amount: amount}
}

// NewItemWithMeta returns an Item with id, amount, and meta, cloning
// the meta (Item constructor, Item.ts:25-29, via cloneMeta). The
// save layer uses it to build live items from persist.Item values;
// meta converts losslessly because both sides use
// map[string]json.RawMessage.
func NewItemWithMeta(id, amount int, meta map[string]json.RawMessage) Item {
	return Item{id: id, amount: amount, meta: CloneMeta(meta)}
}

// ID returns the item id (getId, Item.ts:85-87).
func (it Item) ID() int { return it.id }

// SetID sets the item id (setId, Item.ts:94-97). TS returns this
// for chaining; Go returns nothing.
func (it *Item) SetID(id int) { it.id = id }

// Amount returns the item amount (getAmount, Item.ts:99-101).
func (it Item) Amount() int { return it.amount }

// SetAmount sets the item amount (setAmount, Item.ts:141-144). TS
// returns this for chaining; Go returns nothing.
func (it *Item) SetAmount(amount int) { it.amount = amount }

// Meta returns a deep copy of the item meta, or nil when the item
// has none (getMeta, Item.ts:103-105).
//
// DELIBERATE DEVIATION: TS returns the live meta reference, so
// callers can mutate an item's meta through it. Go returns a copy:
// Item is a value type, and a live map would let a copied Item
// mutate the container slot it came from. Mutate through
// SetMeta/SetMetaValue instead.
func (it Item) Meta() map[string]json.RawMessage { return CloneMeta(it.meta) }

// SetMeta replaces the item meta with a deep copy of meta
// (setMeta, Item.ts:107-110, via cloneMeta). A nil meta clears it.
// TS returns this for chaining; Go returns nothing.
func (it *Item) SetMeta(meta map[string]json.RawMessage) {
	it.meta = CloneMeta(meta)
}

// GetMetaValue returns the raw JSON value stored under key, and
// whether one is stored (getMetaValue, Item.ts:112-117). It returns
// false when the item has no meta, the key is empty, or the key is
// absent — the TS undefined cases. The returned bytes are a copy.
//
// TS getMetaValue is generic (the caller picks T); Go callers
// unmarshal the RawMessage into the type they need.
func (it Item) GetMetaValue(key string) (json.RawMessage, bool) {
	if it.meta == nil || key == "" {
		return nil, false
	}
	value, ok := it.meta[key]
	if !ok {
		return nil, false
	}
	return append(json.RawMessage(nil), value...), true
}

// SetMetaValue stores value under key, cloning the value bytes
// (setMetaValue, Item.ts:119-137). An empty key is a no-op. A nil
// value deletes the key — the Go form of the TS undefined case —
// and when the last key is deleted the meta becomes nil, as TS
// sets it to null. A non-nil value of JSON null (RawMessage("null"))
// is stored, matching TS storing a null value (only undefined
// deletes).
func (it *Item) SetMetaValue(key string, value json.RawMessage) {
	if key == "" {
		return
	}
	if value == nil {
		if it.meta != nil {
			delete(it.meta, key)
			if len(it.meta) == 0 {
				it.meta = nil
			}
		}
		return
	}
	if it.meta == nil {
		it.meta = map[string]json.RawMessage{}
	}
	it.meta[key] = append(json.RawMessage(nil), value...)
}

// IsValid reports whether the item is a real stack: id >= 0 and
// amount > 0 (isValid, Item.ts:151-153).
func (it Item) IsValid() bool { return it.id >= 0 && it.amount > 0 }

// IsEmpty reports whether the item is NOT a real stack: the
// negation of IsValid (id < 0 or amount <= 0). TS has no isEmpty;
// the empty-slot filler is id -1, amount 0 (ItemContainer.ts:55-56),
// and a bank placeholder (id >= 0, amount 0) also reads empty here
// even though the bank treats it as a reserved slot — callers that
// need the id-only check (getFreeSlots counts id == -1) must test
// ID() directly.
func (it Item) IsEmpty() bool { return !it.IsValid() }

// IncrementAmount adds 1 to the amount, stopping at
// MaxStackAmount (incrementAmount, Item.ts:158-163).
func (it *Item) IncrementAmount() {
	if it.amount >= MaxStackAmount {
		return
	}
	it.amount++
}

// DecrementAmount subtracts 1 from the amount, stopping at 0
// (decrementAmount, Item.ts:168-173).
func (it *Item) DecrementAmount() {
	if it.amount-1 < 0 {
		return
	}
	it.amount--
}

// IncrementAmountBy adds amount, capping at MaxStackAmount
// (incrementAmountBy, Item.ts:175-181).
func (it *Item) IncrementAmountBy(amount int) {
	if it.amount+amount >= MaxStackAmount {
		it.amount = MaxStackAmount
	} else {
		it.amount += amount
	}
}

// DecrementAmountBy subtracts amount, flooring at 0: when the
// remainder would be below 1 the amount becomes 0
// (decrementAmountBy, Item.ts:186-192).
func (it *Item) DecrementAmountBy(amount int) {
	if it.amount-amount < 1 {
		it.amount = 0
	} else {
		it.amount -= amount
	}
}

// IsUntradeable reports whether the untradeable meta flag is set
// (isUntradeable, Item.ts:210-212: the meta value === true).
func (it Item) IsUntradeable() bool { return it.metaFlag(ItemUntradeableMeta) }

// IsUnbankable reports whether the unbankable meta flag is set
// (isUnbankable, Item.ts:214-216: the meta value === true).
func (it Item) IsUnbankable() bool { return it.metaFlag(ItemUnbankableMeta) }

// IsPresetItem reports whether the item was spawned by a preset
// (isPresetItem, Item.ts:218-224: the meta value === true).
func (it Item) IsPresetItem() bool { return it.metaFlag(ItemPresetMeta) }

// IsTradeable reports whether the item can be traded: not flagged
// untradeable, and the definition says tradeable (isTradeable,
// Item.ts:198-200). A nil source answers false, matching the TS
// DEFAULT definition whose tradeable field is unset (falsy).
func (it Item) IsTradeable(src ItemDefinitionSource) bool {
	return !it.IsUntradeable() && defBool(src, it.id, ItemDefinitionSource.IsTradeable)
}

// IsSellable reports whether the item can be sold: not flagged
// untradeable, and the definition says sellable (isSellable,
// Item.ts:202-204). A nil source answers false, as for
// IsTradeable.
func (it Item) IsSellable(src ItemDefinitionSource) bool {
	return !it.IsUntradeable() && defBool(src, it.id, ItemDefinitionSource.IsSellable)
}

// IsDropable reports whether the item can be dropped: not flagged
// untradeable, and the definition says dropable (isDropable,
// Item.ts:206-208). A nil source answers false, as for
// IsTradeable.
func (it Item) IsDropable(src ItemDefinitionSource) bool {
	return !it.IsUntradeable() && defBool(src, it.id, ItemDefinitionSource.IsDropable)
}

// IsLostOnDeath reports whether the item is lost on death, which
// TS defines as exactly isUntradeable (isLostOnDeath,
// Item.ts:226-228).
func (it Item) IsLostOnDeath() bool { return it.IsUntradeable() }

// Clone returns a deep copy: same id and amount, cloned meta
// (clone, Item.ts:230-232, via the constructor's cloneMeta).
func (it Item) Clone() Item {
	return Item{id: it.id, amount: it.amount, meta: CloneMeta(it.meta)}
}

// Equals reports whether o is an Item with the same id, amount,
// and meta (equals, Item.ts:234-241). TS compares meta with
// JSON.stringify of each side (null when absent); Go compares the
// canonical JSON of each side (see metaFingerprint).
func (it Item) Equals(o Item) bool {
	return it.id == o.id && it.amount == o.amount && it.metaFingerprint() == o.metaFingerprint()
}

// metaFlag reports whether the meta value under key is JSON true
// (the === true checks in Item.ts:210-224). Any other value —
// including the string "true" or the number 1 — is false.
func (it Item) metaFlag(key string) bool {
	if it.meta == nil {
		return false
	}
	value, ok := it.meta[key]
	return ok && string(bytes.TrimSpace(value)) == "true"
}

// metaFingerprint is the canonical JSON of the meta, or "null"
// when the item has none: the Go form of the
// JSON.stringify(meta ?? null) comparisons in Item.equals
// (Item.ts:240) and the stack-matching checks in ItemContainer
// (ItemContainer.ts:484, 526-527, 849-852). Values are decoded and
// re-encoded (numbers preserved literally via json.Number, object
// keys sorted by the encoder) so byte-level differences in the
// stored raw JSON — whitespace, key order — do not make equal
// metas unequal. A value that fails to decode falls back to its
// raw bytes.
//
// DELIBERATE DEVIATION: TS JSON.stringify preserves nested object
// insertion order; Go sorts nested keys. Two metas whose nested
// objects hold the same keys in different orders compare equal
// here and unequal in TS. Meta values in practice are flat
// scalars, where the two agree.
func (it Item) metaFingerprint() string {
	if it.meta == nil {
		return "null"
	}
	decoded := make(map[string]any, len(it.meta))
	for key, raw := range it.meta {
		var value any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&value); err != nil {
			decoded[key] = string(raw)
			continue
		}
		decoded[key] = value
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return "null"
	}
	return string(encoded)
}

// CloneMeta deep-copies a meta map (cloneMeta, Item.ts:45-50, via
// cloneMetaValue at Item.ts:31-43). A nil map stays nil; every
// value's bytes are copied, so the clone shares nothing with the
// original. TS also returns null for a non-object or array meta;
// Go's map type cannot express those, so there is no equivalent
// branch — the persist layer already keeps meta only when it is a
// JSON object.
func CloneMeta(meta map[string]json.RawMessage) map[string]json.RawMessage {
	if meta == nil {
		return nil
	}
	clone := make(map[string]json.RawMessage, len(meta))
	for key, value := range meta {
		clone[key] = append(json.RawMessage(nil), value...)
	}
	return clone
}

// renamedMetaKeys is Item.RENAMED_META_KEYS (Item.ts:56-63): the
// meta keys issue #226 renamed from camelCase to kebab-case.
var renamedMetaKeys = map[string]string{
	"toxicBlowpipe":           "toxic-blowpipe",
	"tridentCharges":          "trident-charges",
	"bloodFury":               "blood-fury",
	"tumekensShadowCharges":   "tumekens-shadow-charges",
	"barrows:lockpickCharges": "barrows:lockpick-charges",
	"farming:saplingAt":       "farming:sapling-at",
}

// MigrateSavedMeta renames the #226 meta keys in a saved meta map
// (migrateSavedMeta, Item.ts:70-80): a top-level old key loads
// under its new name, the new key wins when both are saved, and
// meta nested in other meta is not migrated. A nil map stays nil.
// The result is always a fresh map.
//
// NOTE: the persist layer already applies this migration when it
// unmarshals a save, so meta arriving through the coordinator is
// migrated before it reaches an Item; this function exists for
// meta built outside that path. DELIBERATE DEVIATION: TS returns
// the same map reference when nothing migrates; Go always returns
// a copy, so callers can never alias the input through the result.
func MigrateSavedMeta(meta map[string]json.RawMessage) map[string]json.RawMessage {
	if meta == nil {
		return nil
	}
	migrated := CloneMeta(meta)
	for old, renamed := range renamedMetaKeys {
		value, ok := migrated[old]
		if !ok {
			continue
		}
		if _, exists := migrated[renamed]; !exists {
			migrated[renamed] = value
		}
		delete(migrated, old)
	}
	return migrated
}

// defBool reads one boolean definition predicate through src,
// answering false when src is nil (the TS DEFAULT-definition
// behavior for the unset boolean fields — see ItemDefinitionSource).
func defBool(src ItemDefinitionSource, id int, fn func(ItemDefinitionSource, int) bool) bool {
	if src == nil {
		return false
	}
	return fn(src, id)
}

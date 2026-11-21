package game

// ItemDefinitionSource is the interface through which ALL
// container and Item definition lookups flow. Package game never
// imports definition/cache (see doc.go layering): the cmd/server
// adapter implements this interface over the loaded definition
// Store, reusing the same Store.Item lookups behind the
// ground-item seams, and the coordinator injects it through
// Player.SetItemDefinitions. Containers read it through their
// player; a nil source (no coordinator, NoCache mode) answers
// every method with the documented no-definition default, which
// matches the TS behavior of ItemDefinition.forId returning the
// DEFAULT definition for an unknown id (ItemDefinition.ts:127-138:
// unset fields read falsy/zero, equipment slot -1, note/placeholder
// ids -1, requirements unset).
//
// The method set is exactly what the ported container/Item code
// reads, each grounded in a TS call site below. Deliberately
// absent from the REQUIRED interface, after verifying the call
// sites:
//   - Name: not a required method — names are read through the
//     optional ItemNameSource capability (trade_hooks.go) by the
//     systems that need them (trade item list, examine price
//     line, shop currency, setLevel refusal, and content via
//     Player.ItemName), with "" for a source without it.
//     Equipment.getItemCount, wearingHalberd, and hasGodsword
//     also read getName, and all three stay documented gaps in
//     equipment.go (name-lookup/combat systems unported).
//   - NoteID: only Bank.switchsItem reads getNoteId, and that
//     withdraw flow is out of scope (bank interface flows).
//   - Value: only getTotalValue reads getValue, and getTotalValue
//     is a documented gap (see container.go).
//   - Bonuses, animations, weapon interface, sounds: read only by
//     combat/appearance/bonus systems, none of which exist in Go.
type ItemDefinitionSource interface {
	// IsStackable reports whether the item stacks (isStackable,
	// ItemDefinition.ts:220-222). Read by canTake
	// (ItemContainer.ts:170), fullBoolean (ItemContainer.ts:417),
	// getAddableAmount (ItemContainer.ts:483), add
	// (ItemContainer.ts:506), deleted (ItemContainer.ts:653),
	// deleteItemContainer (ItemContainer.ts:699), moveItems
	// (ItemContainer.ts:805), and the equip listener's
	// stackable-same-item check (EquipPacketListener.ts:172).
	// Nil-source default: false.
	IsStackable(id int) bool

	// IsNoted reports whether the item is a bank note (isNoted,
	// ItemDefinition.ts:236-238). Read by the carried-weight
	// computation (Player.ts:1296: stackable or noted items weigh
	// 0). ItemContainer/Item do not read it directly — the
	// un-noting paths use UnNote. Nil-source default: false.
	IsNoted(id int) bool

	// WeightGrams returns the item weight in grams (getWeight,
	// ItemDefinition.ts:285-287). Read by the carried-weight
	// computation (Player.ts:1296). Nil-source default: 0.
	WeightGrams(id int) int

	// EquipmentSlot returns the equipment slot the item is worn
	// in, or -1 when it cannot be worn (getEquipmentType().getSlot(),
	// ItemDefinition.ts:333-335 with EquipmentType.ts:14-39).
	// Read by the equip listener (EquipPacketListener.ts:157-161)
	// and the equip-requirements path; no container method reads
	// it, but the lookup belongs here so the listener never
	// touches definitions directly. Nil-source default: -1.
	EquipmentSlot(id int) int

	// IsDoubleHanded reports whether the item takes both hand
	// slots (isDoubleHanded, ItemDefinition.ts:249-251: wearPos
	// weapon + wearPos2 shield, ItemDefinition.ts:168-169). The
	// two-hand rule itself lives in the equip listener
	// (EquipPacketListener.ts:183-185, 209-210), which reads this;
	// no container method reads it. Nil-source default: false.
	IsDoubleHanded(id int) bool

	// Requirements returns the per-skill level requirements, in
	// Skill declaration order (getRequirements,
	// ItemDefinition.ts:298-300), or nil when the item has none.
	// Read by EquipmentRequirements.getUnmet
	// (EquipmentRequirements.ts:30-37), which indexes it by skill
	// index and treats a missing entry as 0. The returned slice
	// is owned by the caller. Nil-source default: nil.
	Requirements(id int) []int

	// CombatRequirement returns the combat level needed to wear
	// the item (getCombatRequirement, ItemDefinition.ts:303-305).
	// Read by EquipmentRequirements.getUnmet
	// (EquipmentRequirements.ts:39-42). Nil-source default: 0.
	CombatRequirement(id int) int

	// IsTradeable reports whether the item is tradeable
	// (isTradeable, ItemDefinition.ts:224-226). Read by
	// Item.isTradeable (Item.ts:198-200) and, through it, by
	// Equipment.properEquipmentForWilderness
	// (Equipment.ts:111-115). Nil-source default: false.
	IsTradeable(id int) bool

	// IsSellable reports whether the item is sellable (isSellable,
	// ItemDefinition.ts:228-230). Read by Item.isSellable
	// (Item.ts:202-204). Nil-source default: false.
	IsSellable(id int) bool

	// IsDropable reports whether the item is dropable (isDropable,
	// ItemDefinition.ts:232-234). Read by Item.isDropable
	// (Item.ts:206-208). Nil-source default: false.
	IsDropable(id int) bool

	// UnNote returns the unnoted id for a note, or the item's own
	// id when it is not a note (unNote, ItemDefinition.ts:337-339).
	// Read by switchItem/switchItems when depositing into a bank
	// (ItemContainer.ts:319, 334, 380, 394) and by Bank.canTake
	// (Bank.ts:264). Nil-source default: the id itself (an
	// un-noted item un-notes to itself).
	UnNote(id int) int

	// PlaceholderID returns the faded bank placeholder id for the
	// item, or -1 when it has none (getPlaceholderId,
	// ItemDefinition.ts:241-243). Read by Bank.placeholderBankId
	// (Bank.ts:125-131) and Bank.displayItemId (Bank.ts:536-543).
	// Nil-source default: -1.
	PlaceholderID(id int) int
}

// ItemDeathComponentsSource is the optional death-components
// capability of an item-definition source: the tradeable items
// an item splits into when lost to another player in the
// Wilderness (getDeathComponents, ItemDefinition.ts:318-320,
// over the deathComponents field, ItemDefinition.ts:110).
// It is separate from ItemDefinitionSource for the same reason
// ItemAnimationSource is: only the death pipeline reads
// components, and a source that does not implement it yields
// no components — exactly as the TS DEFAULT definition does
// (deathComponents defaults to []). Implementations must be
// total over all ids and must return a fresh slice the caller
// owns.
type ItemDeathComponentsSource interface {
	// DeathComponents returns the tradeable items this item
	// splits into on a Wilderness PvP death, or nil/empty
	// when it drops whole.
	DeathComponents(id int) []int
}

// ItemAnimationSource is the optional animation capability of an
// item-definition source: the stand/walk/run sequence ids the
// appearance block derives from the equipped weapon
// (PlayerSession.createPlayerView, PlayerSession.ts:685-694,
// reading ItemDefinition.getStandAnim/getWalkAnim/getRunAnim,
// ItemDefinition.ts:257-267, over the standAnim/walkAnim/runAnim
// fields defaulting to 808/819/824, ItemDefinition.ts:95-97).
// It is separate from ItemDefinitionSource because only the
// appearance path reads animations; a source that does not
// implement it yields the TS `??` defaults (808/819/824) via
// SyncAnimations, exactly as an undefined weapon definition does
// in TS. Implementations must be total over all ids.
type ItemAnimationSource interface {
	// StandAnim returns the stand sequence id (getStandAnim).
	StandAnim(id int) int
	// WalkAnim returns the walk sequence id (getWalkAnim).
	WalkAnim(id int) int
	// RunAnim returns the run sequence id (getRunAnim).
	RunAnim(id int) int
}

// ItemActionSource is the optional action-list capability of an
// item-definition source: the cache ObjType action lists the
// packet-listener routing reads. It is separate from
// ItemDefinitionSource for the same reason ItemAnimationSource
// is: only the listener routing reads actions, and the lists
// live on the cache ObjType (CacheDefinitions.getItem), not on
// the JSON-layered ItemDefinition. A source that does not
// implement it yields the ObjType defaults (see
// DefaultInventoryActions/DefaultGroundActions) in the
// listeners, exactly as a cache item with no action opcodes
// decodes. Implementations must be total over all ids and must
// return fresh slices the caller owns.
type ItemActionSource interface {
	// InventoryActions returns the five inventory action texts
	// in option order (CacheDefinitions.getItem(id)
	// .inventoryActions, read by NetworkBuilder.inventoryAction,
	// NetworkBuilder.ts:1760, and by
	// ItemActionPacketListener.resolveInventoryWidgetAction,
	// ItemActionPacketListener.ts:23-30). Absent actions are "".
	// The returned slice is owned by the caller.
	InventoryActions(id int) []string
	// GroundActions returns the five ground action texts in
	// option order (CacheDefinitions.getItem(id).groundActions,
	// read by NetworkBuilder.groundItemAction,
	// NetworkBuilder.ts:1835, and by
	// ItemOnGroundManager.takeOperation,
	// ItemOnGroundManager.ts:198). Absent actions are "". The
	// returned slice is owned by the caller.
	GroundActions(id int) []string
	// ShiftClickIndex returns the shift-click action index
	// (ObjType.getShiftClickIndex, ObjType.ts:620-632), or -1
	// when the item has no shift-click action. Read by
	// ItemActionPacketListener.resolveInventoryWidgetAction for
	// widget op 1 (ItemActionPacketListener.ts:27).
	ShiftClickIndex(id int) int
}

// DefaultInventoryActions returns the inventory actions of a
// cache item with no inventory-action opcodes: four absent
// actions and "Drop" fifth (CacheDefinitions.ts:676: when the
// decoded type has no inventoryActions array it is set to
// [null, null, null, null, "Drop"]). The listeners use these
// when the installed definition source does not implement
// ItemActionSource. The returned slice is fresh on every call.
func DefaultInventoryActions() []string {
	return []string{"", "", "", "", "Drop"}
}

// DefaultGroundActions returns the ground actions of a cache
// item with no ground-action opcodes: "Take" third, the rest
// absent (ObjType.ts:118: the constructor default is [null,
// null, "Take", null, null]). The listeners use these when the
// installed definition source does not implement
// ItemActionSource. The returned slice is fresh on every call.
func DefaultGroundActions() []string {
	return []string{"", "", "Take", "", ""}
}

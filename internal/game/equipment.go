package game

import "strconv"

// Equipment is the port of game/model/container/impl/Equipment.ts:
// the player's 14-slot worn container (StackType DEFAULT). All
// container behavior lives on the embedded itemContainer core;
// this file holds the equipment-specific surface: slot constants,
// getWeapon, the worn-inventory shape, the predicate helpers that
// are pure container reads, and EquipmentRequirements.
//
// NOT PORTED (gaps, with reasons):
//   - wearingHalberd (Equipment.ts:103-107): reads the weapon
//     definition's name. Name lookup is deliberately absent from
//     ItemDefinitionSource (see itemdef.go).
//   - hasStaffEquipped / canAutocastWith (Equipment.ts:118-139):
//     read the weapon interface (a combat-system table) and the
//     player's weapon interface. Neither exists in Go.
//   - hasGodsword (Equipment.ts:145-147): reads the weapon
//     definition's name. See wearingHalberd.
//   - getItemCount (Equipment.ts:43-60): counts worn/carried
//     items whose definition name contains a substring. See
//     wearingHalberd.
//   - The equipment:changed plugin event in refreshItems
//     (Equipment.ts:76-79): no plugin system exists. The
//     optional change payload (equipped/itemId/slot) fed only
//     that event, so RefreshItems takes no arguments.

const (
	// Equipment slot constants (Equipment.ts:17-27). The gaps at
	// 6, 8, and 11 are real: TS defines no slots there.
	HeadSlot       = 0
	CapeSlot       = 1
	AmuletSlot     = 2
	WeaponSlot     = 3
	BodySlot       = 4
	ShieldSlot     = 5
	LegSlot        = 7
	HandsSlot      = 9
	FeetSlot       = 10
	RingSlot       = 12
	AmmunitionSlot = 13

	// EquipmentInventoryInterfaceID is the equipment tab
	// interface id (INVENTORY_INTERFACE_ID, Equipment.ts:15).
	EquipmentInventoryInterfaceID = 1688
	// EquipmentScreenInterfaceID is the equipment screen
	// interface id (EQUIPMENT_SCREEN_INTERFACE_ID,
	// Equipment.ts:16).
	EquipmentScreenInterfaceID = 84
	// WornInventoryID is the cache's worn inventory id, which the
	// equipment tab's scripts read (WORN_INVENTORY_ID,
	// Equipment.ts:29-30).
	WornInventoryID = 94
	// EquipmentCapacity is the equipment slot count (capacity,
	// Equipment.ts:62-64).
	EquipmentCapacity = 14
)

// Equipment is the player's worn container. Construct with
// NewEquipment; NewPlayer builds one per player.
type Equipment struct {
	*itemContainer
}

// NewEquipment returns an empty 14-slot equipment container owned
// by p (Equipment constructor, Equipment.ts:33-35).
func NewEquipment(p *Player) *Equipment {
	return &Equipment{itemContainer: newItemContainer(ContainerEquipment, p, EquipmentCapacity)}
}

var _ Container = (*Equipment)(nil)

// Set stores item in slot (set, Equipment.ts:38-41: "Convenience
// setter used by some bot commands"). It delegates to the core
// SetItem; TS returns the Equipment for chaining, Go returns
// nothing.
func (e *Equipment) Set(slot int, item Item) { e.itemContainer.SetItem(slot, item) }

// GetWeapon returns a copy of the wielded weapon: the item in the
// weapon slot (getWeapon, Equipment.ts:141-143).
func (e *Equipment) GetWeapon() Item { return e.ItemAt(WeaponSlot) }

// WearingNexAmours reports whether the player wears a full set of
// Torva, Pernix, or Virtus (wearingNexAmours,
// Equipment.ts:93-101, including the TS spelling). TS reads the
// slots through the player's equipment rather than this
// container, and Go does the same when a player with live
// equipment exists, falling back to this container's own slots
// otherwise.
func (e *Equipment) WearingNexAmours() bool {
	slots := e.Items()
	if p := e.player; p != nil {
		if live := p.Equipment(); live != nil && live.itemContainer != nil {
			slots = live.Items()
		}
	}
	head, body, legs := NewItem(-1, 0), NewItem(-1, 0), NewItem(-1, 0)
	if HeadSlot < len(slots) {
		head = slots[HeadSlot]
	}
	if BodySlot < len(slots) {
		body = slots[BodySlot]
	}
	if LegSlot < len(slots) {
		legs = slots[LegSlot]
	}
	torva := head.ID() == 14008 && body.ID() == 14009 && legs.ID() == 14010
	pernix := head.ID() == 14011 && body.ID() == 14012 && legs.ID() == 14013
	virtus := head.ID() == 14014 && body.ID() == 14015 && legs.ID() == 14016
	return torva || pernix || virtus
}

// ProperEquipmentForWilderness reports whether at least 3 worn
// items are tradeable (properEquipmentForWilderness,
// Equipment.ts:109-116). Tradeability reads through the
// definition source (Item.isTradeable, including the untradeable
// meta flag); with no source every item reads untradeable and the
// answer is false.
func (e *Equipment) ProperEquipmentForWilderness() bool {
	count := 0
	src := e.defs()
	for _, item := range e.GetValidItems() {
		if item.IsTradeable(src) {
			count++
		}
	}
	return count >= 3
}

// WornInventorySlot is one slot of the worn-inventory shape
// sendWornInventory builds (Equipment.ts:88-89): the item's id
// and amount when the slot holds a real stack (id > 0 and amount
// > 0), or Present=false for the TS null otherwise. Note the
// id > 0 test: an id-0 item maps to null, as in TS.
type WornInventorySlot struct {
	ID      int
	Amount  int
	Present bool
}

// WornInventorySlots returns the worn-inventory data shape for
// this container: one entry per equipment slot, in slot order
// (Equipment.ts:88-89). The actual send is the adapter's job (see
// SendWornInventory).
func (e *Equipment) WornInventorySlots() []WornInventorySlot {
	items := e.Items()
	slots := make([]WornInventorySlot, len(items))
	for i, item := range items {
		if item.ID() > 0 && item.Amount() > 0 {
			slots[i] = WornInventorySlot{ID: item.ID(), Amount: item.Amount(), Present: true}
		}
	}
	return slots
}

// SendWornInventory delivers the worn inventory through the
// refresh hook (sendWornInventory, Equipment.ts:87-91): the
// client draws the equipment tab from the player's appearance,
// which for a transformed player carries the NPC instead of the
// items, so the worn inventory travels on its own. The event
// kind is ContainerWornInventory and the items are the mapped
// shape (absent slots as empty items, matching the id -1 /
// quantity 0 sendInventory writes for nulls,
// PacketSender.ts:1725-1739); the adapter encodes it as the
// worn-inventory run-script packet. With no hook installed this
// is a no-op. It is never called in production today: the only
// TS caller is the transformed-player branch of refreshItems,
// and no NPC-transformation state exists in Go.
func (e *Equipment) SendWornInventory() {
	if e.refreshHook == nil {
		return
	}
	slots := e.WornInventorySlots()
	items := make([]Item, len(slots))
	for i, slot := range slots {
		if slot.Present {
			items[i] = NewItem(slot.ID, slot.Amount)
		} else {
			items[i] = NewItem(-1, 0)
		}
	}
	e.refreshHook(RefreshEvent{Kind: ContainerWornInventory, Player: e.player, Items: items})
}

// RequirementLevels is the wearer surface EquipmentRequirements
// reads: a SkillManager satisfies it (Worker B), as does any
// preset-stats stand-in. It is the Go form of the RequirementLevels
// interface (EquipmentRequirements.ts:6-11), flattened: TS reads
// the two methods off wearer.getSkillManager(), Go reads them
// directly. skill is the index in Skill declaration order
// (Skill.ts:33-58: Attack 0 ... Sailing 23).
type RequirementLevels interface {
	MaxLevel(skill int) int
	CombatLevel() int
}

// UnmetRequirement is the first requirement not met (the
// UnmetRequirement interface, EquipmentRequirements.ts:13-17):
// the skill display name ("Ranged"), or "combat" for a
// combat-level gate, and the required level.
type UnmetRequirement struct {
	Label string
	Level int
}

// equipmentRequirementSkillNames are the skill display names in
// Skill declaration order (Skill.NAMES, Skill.ts:59-84), indexed
// by skill index. Hardcoded here so the requirements check does
// not depend on Worker B's SkillManager; the order is the VALUES
// order (Skill.ts:33-58), which is also the getIndex order —
// Construction is 21 and Hunter 22 in declaration order, not
// client-id order.
var equipmentRequirementSkillNames = [24]string{
	"Attack", "Defence", "Strength", "Hitpoints", "Ranged",
	"Prayer", "Magic", "Cooking", "Woodcutting", "Fletching",
	"Fishing", "Firemaking", "Crafting", "Smithing", "Mining",
	"Herblore", "Agility", "Thieving", "Slayer", "Farming",
	"Runecrafting", "Construction", "Hunter", "Sailing",
}

// GetUnmetRequirement returns the first requirement the wearer
// does not meet for itemID, or nil when the wearer can put the
// item on (EquipmentRequirements.getUnmet,
// EquipmentRequirements.ts:25-44). Skills are checked in Skill
// declaration order against the definition's requirement array
// (a missing entry counts as 0, and a nil requirements array
// means no skill gates); then the combat requirement is checked.
// A nil definition source means no requirements and no combat
// gate, so the result is nil. A nil levels reader counts every
// level as 0.
//
// The TS label passes the skill name through Misc.formatText
// (EquipmentRequirements.ts:35; Misc.ts:305-318), which
// capitalizes the first letter and letters after non-alphanumeric
// characters. Every Skill.ts name is already a single capitalized
// word, so formatText is the identity on them and the labels here
// are the names verbatim.
func GetUnmetRequirement(levels RequirementLevels, src ItemDefinitionSource, itemID int) *UnmetRequirement {
	var requirements []int
	combatRequirement := 0
	if src != nil {
		requirements = src.Requirements(itemID)
		combatRequirement = src.CombatRequirement(itemID)
	}
	if requirements != nil {
		for skill, name := range equipmentRequirementSkillNames {
			required := 0
			if skill < len(requirements) {
				required = requirements[skill]
			}
			if required > maxLevelOf(levels, skill) {
				return &UnmetRequirement{Label: name, Level: required}
			}
		}
	}
	if combatRequirement > combatLevelOf(levels) {
		return &UnmetRequirement{Label: "combat", Level: combatRequirement}
	}
	return nil
}

// MeetsRequirements reports whether the wearer meets every
// requirement for itemID (EquipmentRequirements.meets,
// EquipmentRequirements.ts:46-51).
func MeetsRequirements(levels RequirementLevels, src ItemDefinitionSource, itemID int) bool {
	return GetUnmetRequirement(levels, src, itemID) == nil
}

// RequirementMessage renders the refusal line for an unmet
// requirement (EquipmentRequirements.message,
// EquipmentRequirements.ts:57-60): "You need an/a <label> level
// of at least <level> to wear <subject>." The article is "an"
// when the label starts with a vowel (case-insensitive), else
// "a". TS defaults subject to "this"; Go callers pass it
// explicitly.
func RequirementMessage(unmet UnmetRequirement, subject string) string {
	article := "a"
	if len(unmet.Label) > 0 {
		switch unmet.Label[0] {
		case 'a', 'e', 'i', 'o', 'u', 'A', 'E', 'I', 'O', 'U':
			article = "an"
		}
	}
	return "You need " + article + " " + unmet.Label + " level of at least " + strconv.Itoa(unmet.Level) + " to wear " + subject + "."
}

// maxLevelOf reads a max level through levels, answering 0 for a
// nil reader.
func maxLevelOf(levels RequirementLevels, skill int) int {
	if levels == nil {
		return 0
	}
	return levels.MaxLevel(skill)
}

// combatLevelOf reads the combat level through levels, answering
// 0 for a nil reader.
func combatLevelOf(levels RequirementLevels) int {
	if levels == nil {
		return 0
	}
	return levels.CombatLevel()
}

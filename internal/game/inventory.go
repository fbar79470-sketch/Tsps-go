package game

// Inventory is the port of game/model/container/impl/Inventory.ts:
// the player's 28-slot carried container (StackType DEFAULT). All
// container behavior lives on the embedded itemContainer core;
// this type exists so the three containers are distinct Go types,
// mirroring the TS class hierarchy, and so the coordinator can
// distinguish them in the Container interface.

const (
	// InventoryInterfaceID is the inventory interface id
	// (Inventory.INTERFACE_ID, Inventory.ts:6).
	InventoryInterfaceID = 3214
	// InventoryCapacity is the inventory slot count (capacity,
	// Inventory.ts:12-14).
	InventoryCapacity = 28
)

// Inventory is the player's carried container. Construct with
// NewInventory; NewPlayer builds one per player.
type Inventory struct {
	*itemContainer
}

// NewInventory returns an empty 28-slot inventory owned by p
// (Inventory constructor, Inventory.ts:8-10).
func NewInventory(p *Player) *Inventory {
	return &Inventory{itemContainer: newItemContainer(ContainerInventory, p, InventoryCapacity)}
}

var _ Container = (*Inventory)(nil)

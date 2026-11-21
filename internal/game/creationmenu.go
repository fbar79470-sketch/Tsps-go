package game

// CreationMenu: the skillmulti production list (CreationMenu.ts,
// 97 lines). A menu is a title, the item ids it offers, the action
// that runs when the player picks an item and an amount, and the
// display options: per-item labels (default: the item names), the
// most that can be chosen and the amount it opens on (default: the
// menu maximum), and clientscript 2046's first argument (default
// 0: Make; callers override it for Cook, Smelt, String, etc.).
//
// The menu is session-only state on the player
// (Player.ts:205): sendCreationMenu installs it
// (PacketSender.ts:1386), closeCreationMenu / stopSkillable /
// resetInterfaceState clear it (PacketSender.ts:1418, 894;
// SkillManager.ts:648). The session side cannot touch the field
// itself — Session holds no *Player — so the install/clear travel
// through the InterfaceDeps.SetCreationMenu seam
// (internal/session/interface_state.go), wired by the coordinator
// (cmd/server/interfacewiring.go) exactly as the other
// player-state deps are.

// CreationMenuGroupID is the widget group the menu mounts into
// (CREATION_MENU_GROUP_ID, PacketSender.ts:161).
const CreationMenuGroupID = 270

// CreationMenuFirstItemComponent is the first item button's
// component inside group 270
// (CREATION_MENU_FIRST_ITEM_COMPONENT, PacketSender.ts:162).
const CreationMenuFirstItemComponent = 15

// CreationMenuMaxQuantity is the most the menu can offer
// (CREATION_MENU_MAX_QUANTITY, PacketSender.ts:163): the send
// clamps maxAmount to it, and it is the receive clamp when the
// menu sets no maxAmount.
const CreationMenuMaxQuantity = 28

// CreationMenuMaxItems is the most items one menu shows: the
// send slices the item list to 18 and pads the script arguments
// to 18 ids (PacketSender.ts:1382-1383, 1409-1410 — an unnamed
// literal in TS, named here so the send and the script agree).
const CreationMenuMaxItems = 18

// CreationMenuLastItemVarp is the varp holding the index of the
// item chosen last, so the space key re-picks it
// (CREATION_MENU_LAST_ITEM_VARP, PacketSender.ts:164).
const CreationMenuLastItemVarp = 2673

// CreationMenuLastItemAttribute is the persistent attribute the
// last-chosen index is remembered under
// (CREATION_MENU_LAST_ITEM_ATTRIBUTE, PacketSender.ts:165). It is
// already in the persist registry
// (internal/persist/attributes.go:40).
const CreationMenuLastItemAttribute = "creation-menu:last-item"

// CreationMenuOpFlags are the widget transmit flags for the item
// buttons (CREATION_MENU_OP_FLAGS, PacketSender.ts:167-168):
// bit (opIndex+1) must be set for that op to reach the server —
// ops 1-5 are Make 1/5/10/X/All.
const CreationMenuOpFlags = (1 << 1) | (1 << 2) | (1 << 3) | (1 << 4) | (1 << 5)

// CreationMenuOptions mirrors CreationMenuOptions
// (CreationMenu.ts:9-14). MaxAmount and LastAmount are pointers
// because TS reads them with Number.isInteger: an absent option
// falls back to the menu maximum, while an explicit 0 is a real
// value (maxAmount 0 offers nothing; lastAmount 0 opens on 0).
// Mode is a plain int because its TS default is 0.
type CreationMenuOptions struct {
	// Labels overrides the item names, positionally; a missing
	// entry falls back to the item's cache name.
	Labels []string
	// MaxAmount is the most that can be chosen, or nil for the
	// menu maximum (28).
	MaxAmount *int
	// LastAmount is the amount the menu opens on, or nil for the
	// menu maximum.
	LastAmount *int
	// Mode is clientscript 2046's first argument (0: Make).
	Mode int
}

// CreationMenu is one open skillmulti menu (CreationMenu.ts:16-97).
type CreationMenu struct {
	title   string
	items   []int
	action  func(itemID, amount int)
	options CreationMenuOptions
}

// NewCreationMenu builds a menu. The items slice is copied, as
// TS callers pass a fresh array; a nil action never runs (TS
// requires one, but a nil Go func must not panic the tick).
func NewCreationMenu(title string, items []int, action func(itemID, amount int), options CreationMenuOptions) *CreationMenu {
	return &CreationMenu{
		title:   title,
		items:   append([]int(nil), items...),
		action:  action,
		options: options,
	}
}

// Title returns the menu title (CreationMenu.getTitle).
func (m *CreationMenu) Title() string {
	if m == nil {
		return ""
	}
	return m.title
}

// Items returns a copy of the offered item ids
// (CreationMenu.getItems). The copy keeps callers from mutating
// the menu's list, as TS callers never mutate the array after
// constructing the menu.
func (m *CreationMenu) Items() []int {
	if m == nil {
		return nil
	}
	return append([]int(nil), m.items...)
}

// ItemAt returns the item at index, or false when the index is
// out of range — the Go form of the receive path's
// Number.isInteger(itemId) check (NetworkBuilder.ts:924-925),
// where an out-of-range child yields undefined and the click
// falls through to the interface-action-click emit.
func (m *CreationMenu) ItemAt(index int) (int, bool) {
	if m == nil || index < 0 || index >= len(m.items) {
		return 0, false
	}
	return m.items[index], true
}

// Options returns the menu's display options
// (CreationMenu.getOptions).
func (m *CreationMenu) Options() CreationMenuOptions {
	if m == nil {
		return CreationMenuOptions{}
	}
	return m.options
}

// EffectiveMaxAmount is the maxAmount the send writes into
// clientscript 2046 (PacketSender.ts:1405-1407): the option
// clamped to [0, 28], or 28 when unset.
func (m *CreationMenu) EffectiveMaxAmount() int {
	if m == nil || m.options.MaxAmount == nil {
		return CreationMenuMaxQuantity
	}
	return clampInt(*m.options.MaxAmount, 0, CreationMenuMaxQuantity)
}

// EffectiveLastAmount is the lastAmount the send writes into
// clientscript 2046 (PacketSender.ts:1408): the option clamped
// to [0, maxAmount], or maxAmount when unset.
func (m *CreationMenu) EffectiveLastAmount() int {
	maxAmount := m.EffectiveMaxAmount()
	if m == nil || m.options.LastAmount == nil {
		return maxAmount
	}
	return clampInt(*m.options.LastAmount, 0, maxAmount)
}

// AmountLimit is the cap the receive path clamps the chosen
// quantity to (NetworkBuilder.ts:927-931): the RAW options
// maxAmount, or 28 when unset. Unlike EffectiveMaxAmount it is
// NOT clamped to 28 — TS reads the raw option here, so a menu
// opened with maxAmount 50 clamps a click to 50 (the client
// cannot offer more than 28 anyway, via the script argument).
func (m *CreationMenu) AmountLimit() int {
	if m == nil || m.options.MaxAmount == nil {
		return CreationMenuMaxQuantity
	}
	return *m.options.MaxAmount
}

// Execute runs the menu's action for itemID and amount, but only
// when itemID is one of the offered items (CreationMenu.execute,
// CreationMenu.ts:60-65: the includes-check). A nil menu or a
// nil action runs nothing.
func (m *CreationMenu) Execute(itemID, amount int) {
	if m == nil || m.action == nil {
		return
	}
	for _, id := range m.items {
		if id == itemID {
			m.action(itemID, amount)
			return
		}
	}
}

// CreationMenuSender is the session-side sender for creation
// menus (PacketSender.sendCreationMenu / closeCreationMenu,
// PacketSender.ts:1377-1422), implemented by *Session. Game and
// content code reach it through a type assertion on the player's
// Session, as with DialogueSender.
//
// SendCreationMenu takes the name resolver and the remembered
// last item as arguments because the session holds no *Player
// and no definition source: itemName is the caller's name
// lookup (Player.ItemName), resolving through the installed
// item-definition source with "" for an unknown id — the send
// falls back to "null" for those, exactly as TS does for an id
// CacheDefinitions.hasItem rejects (CacheDefinitions.ts:589-595:
// false when the name is empty or "null"). A nil itemName names
// every item "null". lastItem/hasLastItem are the remembered
// last-chosen index from Player.CreationMenuLastItem; the varp
// goes out only when hasLastItem is true, mirroring TS's
// Number.isInteger check on the attribute (PacketSender.ts:1412).
// The send installs the menu on the player through the
// InterfaceDeps.SetCreationMenu seam (PacketSender.ts:1386);
// callers do not set the field themselves.
type CreationMenuSender interface {
	// SendCreationMenu opens menu on the client, sending
	// nothing when the menu is nil or offers no items (the TS
	// early return, PacketSender.ts:1380-1385).
	SendCreationMenu(menu *CreationMenu, itemName func(id int) string, lastItem int, hasLastItem bool)
	// CloseCreationMenu closes the open menu: the player field
	// clears first, then the sub-interface closes and the modal
	// target hides (PacketSender.ts:1417-1422).
	CloseCreationMenu()
}

// CreationMenuLastItem returns the remembered last-chosen item
// index for this player, and whether one is remembered: the
// creation-menu:last-item attribute coerced with the
// Number.isInteger rule (attributeInteger), so a value reloaded
// from the save as a float still counts when integral.
// (PacketSender.ts:1412 reads the same attribute at send time.)
func (p *Player) CreationMenuLastItem() (int, bool) {
	if p == nil {
		return 0, false
	}
	return attributeInteger(p.GetAttribute(CreationMenuLastItemAttribute))
}

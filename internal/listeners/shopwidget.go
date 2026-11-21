package listeners

import (
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Shop widget and item-container actions: the listener-side
// translation between decoded widget-action packets and the
// ShopManager, following the TS widget-action chain. This file
// owns ONLY the routing and translation; the quantity-mode,
// examine, price, buy, and sell behavior lives in the
// ShopManager (phase 17, Worker A), reached through the
// ShopWidgetHandler seam. This package never names the
// ShopManager type.
//
// TS grounding (commit bfd81e3):
//
//   - The widget-action chain (NetworkBuilder.ts:975-1145)
//     tries, in order: the player:widget-action custom event,
//     the dialogue-open capture, equipment-slot resolution,
//     the world map, the equipment branch, Bank
//     .handleWidgetAction, ShopManager.handleWidgetAction,
//     quick prayers, prayers, combat, weapon styles, autocast,
//     spell handlers, the main-inventory branch (group 149),
//     the InterfaceActionClickOpcode / button-click fallback,
//     and the dialogue-option tail. The shop branch sits AFTER
//     bank and BEFORE prayer. The coordinator inserts the call
//     to HandleShopWidgetAction at that position in
//     routeWidgetAction (see the bank/shop GAP comment there);
//     this file cannot edit that chain.
//   - ShopManager.handleWidgetAction (ShopManager.ts:356-415)
//     returns true only for group 300 (the shop interface:
//     quantity-mode buttons, and child 16, the stock list)
//     and group 301 child 0 (the shop-side inventory). Every
//     other group falls through with no side effect — not
//     even the isOpen check consumes anything — so the group
//     pre-filter in HandleShopWidgetAction is behavior
//     preserving: it calls the seam only for groups 300/301.
//   - The interface ids (ShopManager.ts:75-79): SHOP_INTERFACE
//     _ID 3824 (the main shop interface, used by isOpen),
//     INVENTORY_INTERFACE_ID 3823 (the shop-side inventory
//     container), ITEMS_INTERFACE_ID 3900 (the stock
//     container), MAIN_INTERFACE_ID 300, SIDE_INTERFACE_ID
//     301. Groups 300/301 are the ROUTING ids; 3824 never
//     routes a packet (it is the open-state check), and 3823
//     /3900 are the CONTAINER ids carried inside item
//     container actions.
//   - ShopManager.handleItemContainerAction
//     (ShopManager.ts:289-354) has NO separate packet or
//     dispatch point: its only callers are the two sites
//     inside handleWidgetAction (plus bot code), which build
//     the container action from the widget packet. Go keeps
//     the same shape: HandleShopContainerAction exists for
//     the seam to be driven directly in tests and for any
//     future caller, but no container-action insertion point
//     exists in the listener chain — the widget handler
//     builds and delegates the container action internally,
//     inside the ShopManager port. Do not add a second
//     routing point for it.

// Shop interface and container ids (ShopManager.ts:75-79).
const (
	// ShopInterfaceID is the main shop interface id (3824):
	// the open-state check in ShopManager.isOpen. It never
	// routes a widget-action packet.
	ShopInterfaceID = 3824
	// ShopInventoryInterfaceID is the shop-side inventory
	// container id (3823), carried by sell-side item
	// container actions.
	ShopInventoryInterfaceID = 3823
	// ShopItemsInterfaceID is the stock container id (3900),
	// carried by buy-side item container actions.
	ShopItemsInterfaceID = 3900
	// ShopMainInterfaceID is the shop interface group id
	// (300): quantity-mode buttons and the stock list
	// (child 16).
	ShopMainInterfaceID = 300
	// ShopSideInterfaceID is the shop-side inventory group
	// id (301): the sell list (child 0).
	ShopSideInterfaceID = 301
)

// ShopWidgetPacket is the listener-facing form of a widget
// action bound for the shop: the fields
// ShopManager.handleWidgetAction reads (ShopManager.ts:356),
// translated from the decoded packet. ButtonNum is 0 when the
// packet carried no button number (TS `packet.buttonNum ?? 0`).
// Slot and ItemID keep the decoder's -1 = absent convention,
// with HasSlot / HasItemID making presence explicit: the SUB
// packet passes a raw 0xffff slot through as present (65535),
// while a 0xffff item id decodes to absent (-1) — see
// protocol.DecodeWidgetAction. Option is passed verbatim; the
// ShopManager trims and lowercases it (ShopManager.ts:377).
type ShopWidgetPacket struct {
	GroupID   int
	ChildID   int
	ButtonNum int
	Option    string
	HasOption bool
	Slot      int
	HasSlot   bool
	ItemID    int
	HasItemID bool
}

// ShopContainerActionKind is the kind of a shop item container
// action (ShopManager.ts:289-354): a value/price check, a
// buy/sell at the selected quantity, or an entered-amount
// (buy/sell X) prompt.
type ShopContainerActionKind int

const (
	// ShopContainerActionValue is a price check ("Value").
	ShopContainerActionValue ShopContainerActionKind = iota
	// ShopContainerActionBuySell is a buy (shop container)
	// or sell (inventory container) at the given amount.
	ShopContainerActionBuySell
	// ShopContainerActionX is an entered-amount prompt
	// ("Buy X" / "Sell X"): the amount is collected from the
	// player, not carried here.
	ShopContainerActionX
)

// ShopItemContainerAction is the listener-facing form of a shop
// item container action: the container (3900 stock / 3823
// inventory), the slot within it, the item id the packet
// carried (or -1 when absent), and the amount for buy/sell
// actions. The slot is the container slot as the widget
// handler computed it: for the stock list, packet.slot - 1
// (ShopManager.ts:390); for the inventory list, the raw slot
// (ShopManager.ts:405). The ShopManager port performs that
// translation internally; this type carries the result.
type ShopItemContainerAction struct {
	Kind        ShopContainerActionKind
	ContainerID int
	Slot        int
	ItemID      int
	Amount      int
}

// ShopWidgetHandler is the seam between the listener chain and
// the ShopManager's widget and container handling. The
// coordinator binds it to the ShopManager port at integration
// (HandleWidgetAction over ShopManager.handleWidgetAction,
// HandleItemContainerAction over
// ShopManager.handleItemContainerAction). Both methods report
// whether the action was consumed, following the TS return
// contracts: handleWidgetAction returns false when the shop
// is not open or the group/child is not a shop branch, and
// handleItemContainerAction returns false when no shop is
// open or the container is neither 3900 nor 3823. A nil
// handler means no shop system: the entry points below return
// false and the chain falls through.
type ShopWidgetHandler interface {
	HandleWidgetAction(player *game.Player, packet ShopWidgetPacket) bool
	HandleItemContainerAction(player *game.Player, action ShopItemContainerAction) bool
}

// HandleShopWidgetAction routes one decoded widget action to
// the shop seam when it belongs to the shop interface groups
// (300/301), translating the packet into a ShopWidgetPacket.
// It returns false — without calling the seam — for every
// other group, for a nil player, and for a nil handler, so
// the chain falls through exactly as TS does. The caller
// (routeWidgetAction, via the coordinator's insertion) is
// responsible for the dialogue-option tail on every return:
// in TS the tail runs on every chain outcome except the
// unresolved-inventory continue path (NetworkBuilder.ts:975
// -1145), so the insertion must sit inside the chain's finish
// wrapper, like the other branches.
func HandleShopWidgetAction(player *game.Player, msg protocol.WidgetAction, handler ShopWidgetHandler) bool {
	if player == nil || handler == nil {
		return false
	}
	if msg.GroupID != ShopMainInterfaceID && msg.GroupID != ShopSideInterfaceID {
		return false
	}
	packet := ShopWidgetPacket{
		GroupID:   msg.GroupID,
		ChildID:   msg.ChildID,
		Option:    msg.Option,
		HasOption: msg.HasOption,
		Slot:      msg.Slot,
		HasSlot:   msg.Slot != -1,
		ItemID:    msg.ItemID,
		HasItemID: msg.ItemID != -1,
	}
	if msg.HasButtonNum {
		packet.ButtonNum = msg.ButtonNum
	}
	return handler.HandleWidgetAction(player, packet)
}

// HandleShopContainerAction delegates one shop item container
// action to the seam unconditionally: unlike the widget entry
// point, it applies NO container pre-filter, because TS
// handleItemContainerAction's first act is reading the current
// shop — which closes stale shop state as a side effect
// (ShopManager.ts:289-293, via the currentShop getter) — and
// filtering here would skip that side effect. It returns false
// for a nil player or a nil handler. There is no listener-chain
// insertion point for this function: in TS the only callers
// are inside handleWidgetAction, and the Go ShopManager port
// keeps that shape (see the file header).
func HandleShopContainerAction(player *game.Player, action ShopItemContainerAction, handler ShopWidgetHandler) bool {
	if player == nil || handler == nil {
		return false
	}
	return handler.HandleItemContainerAction(player, action)
}

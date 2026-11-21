// Anvil.Smithing.js: anvil smithing — the smithing interface
// (312) for the best bar the player can work, its 1/5/10/X/All
// quantity row, and the smith loop. Interface 312 draws its
// own products from varbit smithing_bar_type (client scripts
// 415 -> 430), so the server only sets the varbit.
//
// Captured (rsprox, docs/smithing.md): opening sets
// smithing_bar_type and varp makexcrafting (the remembered
// quantity, All when none has been chosen yet); a click sets
// smithing_bar_lasttype/smithing_item_lasttype, closes the
// interface and plays human_smithing (898) on the same tick,
// then each item comes 3 ticks later and every 5 after, with
// "You hammer the steel and make ten dart tips." on the spam
// channel and human_smithing again, 15 client cycles late.
// Smith with no bar: a message box "You should select an item
// from your inventory and use it on the anvil." A product
// above the player's level closes the interface with a message
// box.
package smithing

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/protocol"
	"tsps-go/internal/session"
)

// ---- interface 312 constants (Anvil.Smithing.js) ----

const (
	// smithIntervalTicks is the smith loop interval (5 ticks).
	smithIntervalTicks = 5
	// uniformIntervalTicks is the interval on a Smiths'
	// uniform roll (4 ticks).
	uniformIntervalTicks = 4
	// smithFirstDelayTicks is the first item's delay (3 ticks).
	smithFirstDelayTicks = 3
	// itemAnimationDelay is how many client cycles into its
	// tick each item's hammering starts (15).
	itemAnimationDelay = 15
	// allQuantityCap is the All quantity cap (28).
	allQuantityCap = 28
	// firstProductComponent / lastProductComponent bound the
	// product row (components 9-38).
	firstProductComponent = 9
	lastProductComponent  = 38
	// noBarMessage is the no-bar message box.
	noBarMessage = "You should select an item from your inventory and use it on the anvil."
	// rustedBronzeOnlyMessage is the rusted-anvil bar refusal.
	rustedBronzeOnlyMessage = "You can only use bronze bars to smith on this anvil."
	// rustedNoBronzeMessage is the rusted-anvil Smith refusal.
	rustedNoBronzeMessage = "You need a bronze bar to smith equipment on this anvil."
)

// quantityComponents maps the quantity row's components to
// their quantities (1/5/10/X/All).
var quantityComponents = map[int]string{
	3: "1", 4: "5", 5: "10", 6: "x", 7: "all",
}

// anvilPacketSender is the session-side sender surface Anvil
// needs (the cooking gnomePacketSender precedent):
// SendVarbit, the interface-flags range, and the
// sub-interface mount/close.
type anvilPacketSender interface {
	SendVarbit(id, value int)
	SendInterfaceFlagsRange(uid, fromSlot, toSlot, flags int)
	MountSubInterface(targetUID, groupID, typ int) *session.SubInterfaceMount
	CloseSubInterface(targetUID int)
}

// anvilSender returns the player's anvilPacketSender, or nil
// when the player has no session (or a session without the
// surface).
func anvilSender(player *game.Player) anvilPacketSender {
	if player == nil || player.Session == nil {
		return nil
	}
	sender, ok := player.Session.(anvilPacketSender)
	if !ok {
		return nil
	}
	return sender
}

// stationSnapshot is the station a smith session started at
// (Anvil.Smithing.js stationSnapshot): the anvil's object id
// and location, and the player's tile when the interface
// opened. Go has no getTeleportSerial / getArea, so the
// snapshot holds the location only (gap 26g): a teleport
// that lands back on the same tile does not stop a session.
type stationSnapshot struct {
	stationObjectID int
	location        game.Location
	startLocation   game.Location
}

// snapshotStation snapshots a station, or nil when there is
// no object (remainsAtStation is then always true, as in TS).
func snapshotStation(player *game.Player, object plugins.GameObject) *stationSnapshot {
	if player == nil || object == nil {
		return nil
	}
	return &stationSnapshot{
		stationObjectID: object.ID(),
		location:        object.Location(),
		startLocation:   player.Location(),
	}
}

// remainsAtStation reports whether the player is still at the
// station the session started at.
func remainsAtStation(player *game.Player, context *stationSnapshot) bool {
	if context == nil || player == nil {
		return true
	}
	return player.Location().Equals(context.startLocation)
}

// anvilBars returns the bars an anvil works, lowest first
// (the layout's bar-type order).
func anvilBars() []int {
	if tables == nil {
		return nil
	}
	var bars []int
	seen := map[int]bool{}
	for i := range tables.Anvil {
		bar := tables.Anvil[i].BarID
		if !seen[bar] {
			seen[bar] = true
			bars = append(bars, bar)
		}
	}
	return bars
}

// barTypeOf is the layout bar type of a bar id, or 0.
func barTypeOf(barID int) int {
	if tables == nil {
		return 0
	}
	for i := range tables.Anvil {
		if tables.Anvil[i].BarID == barID {
			return tables.Anvil[i].BarType
		}
	}
	return 0
}

// isAnvilBar reports whether an item id is a bar the anvil
// interface works (layout bars, plus blurite).
func isAnvilBar(itemID int) bool {
	if barTypeOf(itemID) != 0 {
		return true
	}
	return itemID != 0 && itemID == barIDByName("Blurite bar")
}

// bronzeBarID is the bronze bar's id, from the smelt recipes.
func bronzeBarID() int { return barIDByName("Bronze bar") }

// bluriteBarID is the blurite bar's id, from the smelt recipes.
func bluriteBarID() int { return barIDByName("Blurite bar") }

// selectedQuantityOf is the player's remembered quantity:
// -1 means All (the default), else a count.
func (s *smithing) selectedQuantityOf(player *game.Player) int {
	if q, ok := s.selectedQuantity[player]; ok {
		return q
	}
	return -1
}

// chooseBar is the bar to show: the one used on the anvil,
// else the best carried bar the player can work something
// from, else the lowest carried bar (Anvil.Smithing.js
// chooseBar). 0 when the player carries no bar.
func (s *smithing) chooseBar(player *game.Player, preferredBarID int) int {
	if player == nil || player.Inventory() == nil || tables == nil {
		return 0
	}
	inv := player.Inventory()
	if preferredBarID != 0 && inv.Contains(preferredBarID) && isAnvilBar(preferredBarID) {
		return preferredBarID
	}
	level := s.smithingLevel(player)
	var carried []int
	for _, bar := range anvilBars() {
		if inv.Contains(bar) {
			carried = append(carried, bar)
		}
	}
	var workable []int
	for _, bar := range carried {
		for _, product := range tables.AnvilByBar(bar) {
			if level >= product.Level {
				workable = append(workable, bar)
				break
			}
		}
	}
	if len(workable) > 0 {
		return workable[len(workable)-1]
	}
	if len(carried) > 0 {
		return carried[0]
	}
	return 0
}

// unlocked reports whether a product may be made: members
// products on a free world, and products behind a quest the
// client greys out, are refused with a message
// (Anvil.Smithing.js unlocked).
func (s *smithing) unlocked(player *game.Player, product *AnvilProduct) bool {
	if product == nil {
		return false
	}
	if product.Members && !s.membersWorld {
		player.SendMessage("You need to be on a members' world to make that.")
		return false
	}
	if product.Quest != "" && !s.questComplete(player, product.Quest) {
		player.SendMessage("You need to complete " + questName(product.Quest) + " to make that.")
		return false
	}
	return true
}

// canSmith reports whether the player can smith a product
// right now (Anvil.Smithing.js canSmith).
func (s *smithing) canSmith(player *game.Player, product *AnvilProduct) bool {
	if player == nil || product == nil {
		return false
	}
	inv := player.Inventory()
	if !hasHammer(player) {
		player.SendMessage("You need a hammer to work metal bars.")
		return false
	}
	if s.smithingLevel(player) < product.Level {
		s.closeInterface(player)
		name := player.ItemName(product.ItemID)
		if name == "" {
			name = product.Label
		}
		if name == "" {
			name = "item"
		}
		messageBox(player, "You need a Smithing level of "+itoa(product.Level)+" to make a "+titleCase(name)+".")
		return false
	}
	if inv == nil || inv.GetAmount(product.BarID) < product.Bars {
		player.SendMessage("You don't have enough bars to smith that item.")
		return false
	}
	return true
}

// canGrantAfterConsumption reports whether the product fits
// once its bars are consumed (Anvil.Smithing.js
// canGrantAfterConsumption): a scratch inventory with the
// bars deleted, checked with GetAddableAmount. The scratch
// container never refreshes (Deleted with refresh=false),
// so no bogus container update reaches the client.
func canGrantAfterConsumption(player *game.Player, product *AnvilProduct) bool {
	if player == nil || product == nil || player.Inventory() == nil {
		return false
	}
	preview := game.NewInventory(player)
	preview.SetItems(player.Inventory().Items())
	preview.Deleted(product.BarID, product.Bars, false)
	return preview.GetAddableAmount(game.NewItem(product.ItemID, product.Amount)) >= product.Amount
}

// smithOnce smiths one product (Anvil.Smithing.js smithOnce).
func (s *smithing) smithOnce(player *game.Player, product *AnvilProduct, provenance *stationSnapshot) bool {
	if player == nil || product == nil {
		return false
	}
	if !remainsAtStation(player, provenance) || !s.canSmith(player, product) || !canGrantAfterConsumption(player, product) {
		return false
	}
	inv := player.Inventory()
	inv.DeleteNumber(product.BarID, product.Bars)
	inv.Add(game.NewItem(product.ItemID, product.Amount), true)
	player.PerformAnimation(humanSmithingSeq, itemAnimationDelay)
	addXP(player, game.SkillSmithing, product.XP)
	metal := product.Metal
	if metal == "" {
		metal = metalName(player, product.BarID)
	}
	if metal == "" {
		metal = "metal"
	}
	spam(player, "You hammer the "+metal+" and make "+product.Phrase+".")
	event := &smithingSuccessEvent{
		Player: player, Skill: game.SkillSmithing, ItemID: product.ItemID,
		Amount: product.Amount, BarID: product.BarID, Bars: product.Bars,
		Source: "anvil",
	}
	if provenance != nil {
		event.StationObjectID = provenance.stationObjectID
		event.Location = provenance.location
	}
	s.emitEvent("smithing:success", event)
	s.produced(player, game.SkillSmithing, product.ItemID, product.Amount, product.XP, "smith")
	return true
}

// smithingInterval is the ticks to the next item: 4 on a
// Smiths' uniform roll, otherwise 5 (Anvil.Smithing.js
// smithingInterval).
func (s *smithing) smithingInterval(player *game.Player) int {
	if s.randFloat() < smithsUniformChance(player) {
		return uniformIntervalTicks
	}
	return smithIntervalTicks
}

// startSmithing starts a smith loop (Anvil.Smithing.js
// startSmithing). All and X stop at what the bars cover,
// without an out-of-bars message.
func (s *smithing) startSmithing(player *game.Player, product *AnvilProduct, amount int, provenance *stationSnapshot) bool {
	if player == nil || product == nil || amount <= 0 || tables == nil ||
		!remainsAtStation(player, provenance) || !s.unlocked(player, product) || !s.canSmith(player, product) {
		return false
	}
	most := 0
	if inv := player.Inventory(); inv != nil && product.Bars > 0 {
		most = inv.GetAmount(product.BarID) / product.Bars
	}
	s.startSession(player, sessionOptions{
		kind:       "smith",
		skill:      game.SkillSmithing,
		remaining:  min(amount, most),
		interval:   func(p *game.Player) int { return s.smithingInterval(p) },
		firstDelay: smithFirstDelayTicks,
		begin:      func(smith *game.Player) bool { return remainsAtStation(smith, provenance) },
		step:       func(smith *game.Player) bool { return s.smithOnce(smith, product, provenance) },
	})
	player.PerformAnimation(humanSmithingSeq, 0)
	return true
}

// openBluriteMenu opens the blurite make-X menu: bolts and
// limbs (The Knight's Sword) are not on the smithing
// interface (Anvil.Smithing.js openBluriteMenu).
func (s *smithing) openBluriteMenu(player *game.Player, provenance *stationSnapshot) {
	if player == nil || tables == nil {
		return
	}
	barID := bluriteBarID()
	products := make([]*AnvilProduct, 0, len(tables.Blurite))
	var itemIDs []int
	for i := range tables.Blurite {
		blurite := &tables.Blurite[i]
		product := &AnvilProduct{
			BarID: barID, ItemID: blurite.ItemID, Level: blurite.Level,
			Bars: 1, Amount: blurite.Amount, Metal: "blurite",
			Phrase: blurite.Phrase, XP: blurite.XP,
		}
		products = append(products, product)
		itemIDs = append(itemIDs, blurite.ItemID)
	}
	if len(products) == 0 {
		return
	}
	bars := 0
	if inv := player.Inventory(); inv != nil {
		bars = inv.GetAmount(barID)
	}
	if bars < 1 {
		bars = 1
	}
	byItem := make(map[int]*AnvilProduct, len(products))
	for _, product := range products {
		byItem[product.ItemID] = product
	}
	menu := game.NewCreationMenu(
		"What would you like to make?",
		itemIDs,
		func(itemID, amount int) {
			if product, ok := byItem[itemID]; ok {
				s.startSmithing(player, product, amount, provenance)
			}
		},
		game.CreationMenuOptions{MaxAmount: &bars},
	)
	if player.Session == nil {
		return
	}
	sender, ok := player.Session.(game.CreationMenuSender)
	if !ok {
		return
	}
	lastItem, hasLastItem := player.CreationMenuLastItem()
	sender.SendCreationMenu(menu, player.ItemName, lastItem, hasLastItem)
}

// openInterface opens the smithing interface on a bar
// (Anvil.Smithing.js openInterface).
func (s *smithing) openInterface(player *game.Player, preferredBarID int, station plugins.GameObject) bool {
	if player == nil || tables == nil {
		return false
	}
	provenance := snapshotStation(player, station)
	blurite := bluriteBarID()
	if preferredBarID == blurite ||
		(preferredBarID == 0 && s.chooseBar(player, 0) == 0 && player.Inventory() != nil && player.Inventory().Contains(blurite)) {
		s.openBluriteMenu(player, provenance)
		return true
	}
	bar := s.chooseBar(player, preferredBarID)
	if bar == 0 {
		messageBox(player, noBarMessage)
		return false
	}
	s.openMenus[player] = bar
	s.openStations[player] = provenance
	selected := s.selectedQuantityOf(player)
	highlight := selected
	if selected < 0 {
		highlight = allQuantityCap
		if inv := player.Inventory(); inv != nil && inv.GetAmount(bar) < highlight {
			highlight = inv.GetAmount(bar)
		}
	}
	player.SetInterfaceId(smithingInterfaceID)
	sendVarbit(player, smithingBarTypeVarbit, barTypeOf(bar))
	player.SendVarp(makexcraftingVarp, highlight)
	if sender := anvilSender(player); sender != nil {
		sender.MountSubInterface(mainModalTargetUID, smithingInterfaceID, 0)
		for component := firstProductComponent; component <= lastProductComponent; component++ {
			sender.SendInterfaceFlagsRange((smithingInterfaceID<<16)|component, 0, 0, 2)
		}
	}
	return true
}

// closeInterface closes the smithing interface
// (Anvil.Smithing.js closeInterface).
func (s *smithing) closeInterface(player *game.Player) {
	if player == nil {
		return
	}
	delete(s.openMenus, player)
	delete(s.openStations, player)
	if !player.IsRegistered() || player.GetInterfaceId() != smithingInterfaceID {
		return
	}
	player.SetInterfaceId(-1)
	if sender := anvilSender(player); sender != nil {
		sender.CloseSubInterface(mainModalTargetUID)
	}
}

// closeAbandonedMenus closes the interface of a player who
// walked off, died or logged out (Anvil.Smithing.js
// closeAbandonedMenus), run every tick.
func (s *smithing) closeAbandonedMenus() {
	for player := range s.openMenus {
		if player == nil || !player.IsRegistered() {
			continue
		}
		movement := player.MovementQueue()
		if (movement != nil && movement.Size() > 0) || player.ForceMovement() != nil {
			s.closeInterface(player)
		}
	}
}

// selectQuantity handles a quantity-row click
// (Anvil.Smithing.js selectQuantity).
func (s *smithing) selectQuantity(event *plugins.InterfaceActionClickEvent) bool {
	if event == nil || event.Player == nil || tables == nil {
		return false
	}
	mode, ok := quantityComponents[event.ButtonID&0xffff]
	if !ok {
		return false
	}
	player := event.Player
	switch mode {
	case "x":
		player.SetEnteredAmountAction(&game.EnteredAmountAction{
			Execute: func(amount int) {
				if amount > 0 {
					s.selectedQuantity[player] = amount
				}
			},
		})
		if player.Session != nil {
			player.Session.SendPacket(protocol.EncodeRunClientScript(108, []protocol.ScriptArg{
				protocol.ScriptArgString("How many would you like to make?"),
			}))
		}
		return true
	case "all":
		s.selectedQuantity[player] = -1
		return true
	default:
		quantity := 0
		for _, c := range mode {
			quantity = quantity*10 + int(c-'0')
		}
		if quantity > 0 {
			s.selectedQuantity[player] = quantity
		}
		return true
	}
}

// chooseProduct handles a product click
// (Anvil.Smithing.js chooseProduct).
func (s *smithing) chooseProduct(event *plugins.InterfaceActionClickEvent) bool {
	if event == nil || event.Player == nil || tables == nil {
		return false
	}
	player := event.Player
	bar, ok := s.openMenus[player]
	if !ok || player.GetInterfaceId() != smithingInterfaceID {
		return false
	}
	product := tables.AnvilByComponent(barTypeOf(bar), event.ButtonID&0xffff)
	if product == nil {
		return false
	}
	selected := s.selectedQuantityOf(player)
	amount := selected
	if selected < 0 {
		amount = allQuantityCap
	}
	provenance := s.openStations[player]
	if !s.startSmithing(player, product, amount, provenance) {
		return false
	}
	delete(s.openStations, player)
	sendVarbit(player, smithingBarLastTypeVarbit, barTypeOf(bar))
	sendVarbit(player, smithingItemLastTypeVarbit, product.SlotIndex)
	s.closeInterface(player)
	return true
}

// anvilAccess gates anvil use (Anvil.Smithing.js anvilAccess):
// Dorics Quest listens on "smithing:anvil-access"; unported
// in Go, so it is never denied (gap 26e).
func (s *smithing) anvilAccess(event *plugins.ItemOnObjectEvent) bool {
	if event == nil {
		return false
	}
	req := &anvilAccessRequest{Player: event.Player, Object: event.Object}
	s.emitEvent("smithing:anvil-access", req)
	return !req.Denied
}

// anvilAccessClick is the object-interaction form of the gate.
func (s *smithing) anvilAccessClick(event *plugins.ObjectInteractionEvent) bool {
	if event == nil {
		return false
	}
	req := &anvilAccessRequest{Player: event.Player, Object: event.Object}
	s.emitEvent("smithing:anvil-access", req)
	return !req.Denied
}

// smith is "Smith" on an anvil (Anvil.Smithing.js smith):
// godsword pieces or a visage and anti-dragon shield go
// first, then the interface.
func (s *smithing) smith(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || event.Player == nil || tables == nil {
		return false
	}
	if !s.anvilAccessClick(event) {
		return false
	}
	if TryGodsword(event.Player) || TryDragonShield(event.Player) {
		return true
	}
	s.openInterface(event.Player, 0, event.Object)
	return true
}

// smithRustedAnvil is "Smith" on the rusted anvil in
// Lumbridge, which is bronze-only
// (Anvil.Smithing.js smithRustedAnvil).
func (s *smithing) smithRustedAnvil(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || event.Player == nil || tables == nil {
		return false
	}
	if !s.anvilAccessClick(event) {
		return false
	}
	if player := event.Player; player.Inventory() == nil || !player.Inventory().Contains(bronzeBarID()) {
		event.Player.SendMessage(rustedNoBronzeMessage)
		return true
	}
	s.openInterface(event.Player, bronzeBarID(), event.Object)
	return true
}

// anvilUseOnAnvil is a bar used on an anvil: it opens the
// interface on that bar (Anvil.Smithing.js useOnAnvil).
func (s *smithing) anvilUseOnAnvil(event *plugins.ItemOnObjectEvent) {
	if event == nil || event.Player == nil || tables == nil {
		return
	}
	if !isAnvilBar(event.ItemID) {
		return
	}
	event.Handled = true
	if !s.anvilAccess(event) {
		return
	}
	if s.objectNameOf(event.ObjectID) == "Rusted anvil" && event.ItemID != bronzeBarID() {
		event.Player.SendMessage(rustedBronzeOnlyMessage)
		return
	}
	s.openInterface(event.Player, event.ItemID, event.Object)
}

// forgetAnvil forgets a player's anvil state on disconnect
// (Anvil.Smithing.js forget).
func (s *smithing) forgetAnvil(player *game.Player) {
	if player == nil {
		return
	}
	delete(s.openMenus, player)
	delete(s.openStations, player)
	delete(s.selectedQuantity, player)
}

// registerAnvil wires the anvil hooks (Anvil.Smithing.js
// attach): the interface opens on "Smith"/"Use", the quantity
// row and product row come through OnInterfaceActionButton
// (button ids are (312<<16)|component), and the abandoned
// menus close every tick.
func (s *smithing) registerAnvil(api *plugins.Api) {
	api.OnServerStartup(func(*plugins.ServerLifecycleEvent) {
		s.registerEveryTick(s.closeAbandonedMenus)
	})
	api.OnObjectInteractionNamed("Anvil", map[string]plugins.ObjectInteractionHandler{
		"Smith": s.smith,
	})
	api.OnObjectInteractionNamed("An experimental anvil", map[string]plugins.ObjectInteractionHandler{
		"Use": s.smith,
	})
	api.OnObjectInteractionNamed("Rusted anvil", map[string]plugins.ObjectInteractionHandler{
		"Smith": s.smithRustedAnvil,
	})
	var quantityButtons []int
	for component := range quantityComponents {
		quantityButtons = append(quantityButtons, (smithingInterfaceID<<16)|component)
	}
	api.OnInterfaceActionButton(quantityButtons, s.selectQuantity)
	var productButtons []int
	for component := firstProductComponent; component <= lastProductComponent; component++ {
		productButtons = append(productButtons, (smithingInterfaceID<<16)|component)
	}
	api.OnInterfaceActionButton(productButtons, s.chooseProduct)
}

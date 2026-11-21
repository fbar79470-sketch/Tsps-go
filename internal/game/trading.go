package game

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Trading: the port of game/content/Trading.ts (656 lines) — the
// player-to-player trade state machine, the 28-slot offer
// container, the accept/confirm stages, the accept-delay rule,
// and the trade window's sends.
//
// PLAYER STATUS — DERIVED, NOT STORED. TS keeps a PlayerStatus
// field on Player (Player.ts:155, getStatus/setStatus at
// 1415-1422) and Trading sets TRADING in initiateTrade and
// NONE in resetAttributes; Bank and ShopManager set BANKING
// and SHOPPING, and Duel Arena content sets DUELING. Go has no
// player-status field, and building a general PlayerStatus
// system is out of scope for this phase. For trading the status
// is exactly derivable from the trade state: TRADING holds
// precisely while the state is a trade-screen state
// (TRADE_SCREEN through ACCEPTED_CONFIRM_SCREEN) — initiateTrade
// sets state and status together, every in-trade transition
// keeps both, and resetAttributes clears both together — and
// NONE holds otherwise (REQUESTED_TRADE never sets TRADING in
// TS). Status() computes it from the state; no field exists to
// drift. When the bank/shop systems land and need to set
// BANKING/SHOPPING, a real status field will be needed, and
// Status() must be revisited then.
//
// STORAGE. One Trading per player, reached through Player.Trading().
// TS constructs it eagerly as a Player field (Player.ts:139). The
// Player struct lives outside this phase's ownership, so the
// instance is held in the player's attribute store under a
// NUL-prefixed key that content can never address, allocated
// lazily on first access under the store's own mutex (the
// get-or-create is atomic, so login-goroutine and tick-goroutine
// callers cannot fork the instance). The store is on Mobile, so
// the instance's lifetime is the player's lifetime — no registry
// to leak — and the key is never registered persistent, so the
// save snapshot (which iterates the persistent-key registry,
// never the map) can never see it. The accessor is the only
// coupling point: moving the field onto Player later changes
// one function.
//
// INTERFACE ID — TRADING-LOCAL. TS gates handleItem on
// player.getInterfaceId() == OFFER_INTERFACE and sets the id in
// initiateTrade/confirmScreen; PacketSender.sendInterfaceRemoval
// resets it (PacketSender.ts:1011-1012, resetInterfaceState).
// Go has no open-interface tracking, so Trading tracks the id
// itself: set on the two mounts, cleared wherever Trading sends
// an interface removal. No other system reads or writes it.
//
// SENDS. Trading never encodes packets: trade-window packets
// ride TradeSender, strings/flags/removal ride DialogueSender,
// inventory sends are Inventory.RefreshItems (sendItemContainer
// for an Inventory sends an inventory snapshot and ignores the
// interface id, PacketSender.ts:1042-1050), and chat lines ride
// SendMessage/SendTypedMessage. A session without the sender
// extensions drops the sends silently. Bot hooks
// (getTradingInteraction: auto-accept, addItemsToTrade,
// offerChanged, receivedItems) are NOT ported: no bot trading
// system exists in Go.
//
// PERSISTENCE. Trading persists nothing: no save field carries
// trade state. The escrow fold (offered items returning to the
// inventory in a snapshot) lives in internal/persist
// (FoldTradeEscrow, PlayerSave.ts:782-791), and the post-trade
// save runs through the TradeHookSeam.SavePlayer seam. See
// internal/persist/escrow.go for the full statement.

// PlayerStatus is PlayerStatus.ts: the player's modal-activity
// status. The ordinals match TS (NONE, SHOPPING, TRADING,
// DUELING, BANKING, PRICE_CHECKING). Only NONE and TRADING are
// producible in Go today (see the Trading file comment);
// SHOPPING, BANKING, DUELING, and PRICE_CHECKING name the TS
// values their systems will produce and must never be returned
// by Status() until those systems land.
type PlayerStatus int

const (
	// PlayerStatusNone: no modal activity (NONE).
	PlayerStatusNone PlayerStatus = iota
	// PlayerStatusShopping: shopping (SHOPPING). Not producible.
	PlayerStatusShopping
	// PlayerStatusTrading: trading (TRADING).
	PlayerStatusTrading
	// PlayerStatusDueling: duelling (DUELING). Not producible.
	PlayerStatusDueling
	// PlayerStatusBanking: banking (BANKING). Not producible.
	PlayerStatusBanking
	// PlayerStatusPriceChecking: price checking (PRICE_CHECKING).
	// Not producible.
	PlayerStatusPriceChecking
)

// TradeState is Trading.ts's TradeState enum. The order matters:
// sendState, closeTrade, and sendText compare states with >=
// against CONFIRM_SCREEN, and the offer/confirm stage split
// keys off it.
type TradeState int

const (
	// TradeStateNone: no trade (NONE).
	TradeStateNone TradeState = iota
	// TradeStateRequestedTrade: a request is outstanding
	// (REQUESTED_TRADE).
	TradeStateRequestedTrade
	// TradeStateTradeScreen: the offer screen (TRADE_SCREEN).
	TradeStateTradeScreen
	// TradeStateAcceptedTradeScreen: the offer screen, accepted
	// (ACCEPTED_TRADE_SCREEN).
	TradeStateAcceptedTradeScreen
	// TradeStateConfirmScreen: the confirm screen
	// (CONFIRM_SCREEN).
	TradeStateConfirmScreen
	// TradeStateAcceptedConfirmScreen: the confirm screen,
	// accepted (ACCEPTED_CONFIRM_SCREEN).
	TradeStateAcceptedConfirmScreen
)

// Trade window interface ids (Trading.ts:28-31).
const (
	// TradeInventoryContainerInterface is the interface id
	// Trading passes to sendItemContainer for the inventory
	// (3322, INVENTORY_CONTAINER_INTERFACE). The id is
	// ignored on the wire for inventories (see the file
	// comment); it is exported because TS exports it.
	TradeInventoryContainerInterface = 3322
	tradeOfferInterface              = 335 // OFFER_INTERFACE
	tradeConfirmInterface            = 334 // CONFIRM_INTERFACE
	tradeInventoryInterface          = 336 // INVENTORY_INTERFACE
)

// AcceptedTradeMessage is the completed trade's line
// (ACCEPTED_MESSAGE, Trading.ts:34): sent as the trade-close
// reason and as a trade-type chat line to both players.
const AcceptedTradeMessage = "Accepted trade."

// AcceptDelayMs is the accept trade delay (ACCEPT_DELAY_MS,
// Trading.ts:38): while the setting varbit is 1, Accept waits
// this long after any change to the offers.
const AcceptDelayMs = 3000

// acceptDelayVarbit is the "Accept trade delay" setting varbit
// (ACCEPT_DELAY_VARBIT, Trading.ts:37).
const acceptDelayVarbit = 13130

// chatTypeTrade is ChatType.TRADE (102, ClientProtocol.ts:1115):
// a trade/duel status line. Restated here following the
// chatTypeSpam/chatTypeMesbox precedent (the game layer never
// imports protocol).
const chatTypeTrade = 102

// Trade window text component uids (Trading.ts:41-55): the
// components the cache trade scripts leave to the server.
const (
	tradeOfferFreeSlotsUID      = (tradeOfferInterface << 16) | 9    // OFFER_FREE_SLOTS
	tradeOfferOwnHeadingUID     = (tradeOfferInterface << 16) | 24   // OFFER_OWN_HEADING
	tradeOfferOtherHeadingUID   = (tradeOfferInterface << 16) | 27   // OFFER_OTHER_HEADING
	tradeOfferStatusUID         = (tradeOfferInterface << 16) | 30   // OFFER_STATUS
	tradeOfferTitleUID          = (tradeOfferInterface << 16) | 31   // OFFER_TITLE
	tradeConfirmOpponentUID     = (tradeConfirmInterface << 16) | 30 // CONFIRM_OPPONENT
	tradeConfirmOwnHeadingUID   = (tradeConfirmInterface << 16) | 23 // CONFIRM_OWN_HEADING
	tradeConfirmOtherHeadingUID = (tradeConfirmInterface << 16) | 24 // CONFIRM_OTHER_HEADING
	tradeConfirmStatusUID       = (tradeConfirmInterface << 16) | 4  // CONFIRM_STATUS
)

// skipPersistenceAttribute is ATTR_SKIP_PERSISTENCE
// (Trading.ts:21): a player holding it strictly true is not
// saved after a trade.
const skipPersistenceAttribute = "bot-skip-persistence"

// tradingAttributeKey is the attribute-store key holding a
// player's Trading (see the file comment). The NUL prefix keeps
// it out of content's kebab-case key space.
const tradingAttributeKey = "\x00trading-state"

// containerTradeOffer is the offer container's ContainerKind.
// It extends the kind set in container.go (Inventory 0,
// Equipment 1, Bank 2, WornInventory 3) and must stay distinct
// from every kind declared there: the container core's
// kind-switches treat an unknown kind as a fixed-capacity
// DEFAULT-stacking container (capacity = array length, 28),
// which is exactly the offer container's shape. Declared here
// because container.go is outside this phase's ownership; if
// container.go gains a kind, this value must move.
const containerTradeOffer ContainerKind = 4

// tradeOfferContainer is PlayerItemContainer (Trading.ts:23-45):
// a 28-slot DEFAULT-stacking container whose refresh runs the
// owner's execFunc (offerRefreshed) and whose full() sends "You
// cannot trade more items.". It embeds *itemContainer and
// overrides only RefreshItems and Full; every other Container
// method is the shared core. The refresh hook the coordinator
// installs on the player's containers is never invoked for it:
// the override replaces the hook dispatch, as the TS
// refreshItems override replaces the base send.
type tradeOfferContainer struct {
	*itemContainer
	onRefresh func()
}

// RefreshItems runs the owner's execFunc (PlayerItemContainer
// .refreshItems, Trading.ts:31-35).
func (c *tradeOfferContainer) RefreshItems() {
	if c.onRefresh != nil {
		c.onRefresh()
	}
}

// Full sends the offer-container-full line (PlayerItemContainer
// .full, Trading.ts:37-40).
func (c *tradeOfferContainer) Full() {
	c.player.SendMessage("You cannot trade more items.")
}

// Trading is one player's trade state (Trading.ts:47-70).
type Trading struct {
	player    *Player
	container *tradeOfferContainer
	interact  *Player
	state     TradeState

	buttonDelay  *SecondsTimer
	requestDelay *SecondsTimer
	// offerChangedAt is when either player last changed this
	// trade's offers, as a Unix millisecond timestamp
	// (Date.now in TS), for the accept trade delay.
	offerChangedAt int64
	// interfaceID is the Trading-local open-interface id (see
	// the file comment): -1 when no trade window is open.
	interfaceID int
	// now is the clock offerChangedAt reads. Tests inject it
	// through setClock.
	now func() time.Time
}

// NewTrading returns a Trading for player (Trading.ts:78-86):
// a fresh offer container wired to offerRefreshed, no
// interact, state NONE, both delays stopped. TS constructs it
// as a Player field; Go allocates it lazily through
// Player.Trading().
func NewTrading(player *Player) *Trading {
	t := &Trading{
		player:       player,
		state:        TradeStateNone,
		buttonDelay:  newSecondsTimer(),
		requestDelay: newSecondsTimer(),
		interfaceID:  -1,
		now:          time.Now,
	}
	t.container = &tradeOfferContainer{itemContainer: newItemContainer(containerTradeOffer, player, 28)}
	t.container.onRefresh = t.offerRefreshed
	return t
}

// Trading returns the player's Trading, allocating it on first
// access (see the file comment for the storage decision). A nil
// player yields nil. The get-or-create runs under the attribute
// store's own mutex, so concurrent first access cannot fork the
// instance.
func (p *Player) Trading() *Trading {
	if p == nil {
		return nil
	}
	s := p.store(true)
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.values[tradingAttributeKey]; ok {
		if t, ok := v.(*Trading); ok && t != nil {
			return t
		}
	}
	t := NewTrading(p)
	s.values[tradingAttributeKey] = t
	return t
}

// setClock injects a clock for tests: the offer-changed
// timestamps and both delay timers read it.
func (t *Trading) setClock(now func() time.Time) {
	t.now = now
	t.buttonDelay.now = now
	t.requestDelay.now = now
}

// nowMillis returns the current time in Unix milliseconds
// (Date.now in TS).
func (t *Trading) nowMillis() int64 {
	if t.now != nil {
		return t.now().UnixMilli()
	}
	return time.Now().UnixMilli()
}

// Player returns the trading player.
func (t *Trading) Player() *Player { return t.player }

// State returns the trade state (getState, Trading.ts:527-529).
func (t *Trading) State() TradeState { return t.state }

// SetState sets the trade state (setState, Trading.ts:584-586).
func (t *Trading) SetState(state TradeState) { t.state = state }

// Interact returns the trade partner, or nil (getInteract,
// Trading.ts:590-592).
func (t *Trading) Interact() *Player { return t.interact }

// SetInteract sets the trade partner (setInteract,
// Trading.ts:594-596).
func (t *Trading) SetInteract(interact *Player) { t.interact = interact }

// Container returns the offer container (getContainer,
// Trading.ts:598-600), as a Container: 28 slots, DEFAULT
// stacking, refresh running offerRefreshed.
func (t *Trading) Container() Container { return t.container }

// OfferedItems returns copies of the offered stacks
// (getContainer().getValidItems()): id >= 0 and amount > 0.
// The save escrow fold reads the offers through this.
func (t *Trading) OfferedItems() []Item { return t.container.GetValidItems() }

// Status returns the player's derived trading status (see the
// file comment): TRADING while the state is a trade-screen
// state, NONE otherwise.
func (t *Trading) Status() PlayerStatus {
	if t.state >= TradeStateTradeScreen {
		return PlayerStatusTrading
	}
	return PlayerStatusNone
}

// InterfaceID returns the Trading-local open-interface id (see
// the file comment): 335 on the offer screen, 334 on the
// confirm screen, -1 otherwise.
func (t *Trading) InterfaceID() int { return t.interfaceID }

// ButtonDelay returns the accept-button delay timer
// (getButtonDelay, Trading.ts:551-553). The movement mobility
// gate reads it.
func (t *Trading) ButtonDelay() *SecondsTimer { return t.buttonDelay }

// RequestDelay returns the trade-request delay timer. TS keeps
// it private; Go exposes it for the same reason ButtonDelay is
// exposed — tests and gates read delay state — but no
// production code outside Trading reads it.
func (t *Trading) RequestDelay() *SecondsTimer { return t.requestDelay }

// offerRefreshed is the PlayerItemContainer execFunc
// (Trading.ts:80-85): send the inventory container (an
// inventory snapshot — see the file comment), refresh this
// side's trade state, and refresh the partner's.
func (t *Trading) offerRefreshed() {
	t.player.Inventory().RefreshItems()
	t.sendState(false)
	if t.interact != nil {
		t.interact.Trading().sendState(false)
	}
}

// validateTrade is Trading.validate (Trading.ts:103-143): both
// players non-nil, both holding the given status, each
// pointing at the other as interact, and both in one of the
// given states.
func validateTrade(player, interact *Player, status PlayerStatus, states ...TradeState) bool {
	if player == nil || interact == nil {
		return false
	}
	if player.Trading().Status() != status {
		return false
	}
	if interact.Trading().Status() != status {
		return false
	}
	if player.Trading().Interact() == nil || player.Trading().Interact() != interact {
		return false
	}
	if interact.Trading().Interact() == nil || interact.Trading().Interact() != player {
		return false
	}
	for _, state := range states {
		if player.Trading().State() == state {
			goto partnerState
		}
	}
	return false
partnerState:
	for _, state := range states {
		if interact.Trading().State() == state {
			return true
		}
	}
	return false
}

// canTrade is Trading.canTrade (Trading.ts:145-150): each
// distinct area of the two traders may refuse (a callArea
// "canTrade" returning exactly false), then the onCanTrade
// emit may refuse (returning exactly false). Any other result
// — no area, no opinion, allowed — permits the trade.
//
// DEVIATION: TS routes the area call through
// PluginManager.callArea, which catches a throwing area method
// and returns null (allowed). Go calls area.CanTrade directly:
// the AreaMethodCaller seam has no CanTrade method (rule hooks
// are not routed through it, area.go), so a throwing area hook
// propagates here where TS would log and allow. The area
// CanTradeHook contract is the same either way.
func canTrade(player, target *Player) bool {
	seen := make(map[*Area]bool, 2)
	for _, p := range []*Player{player, target} {
		if p == nil {
			continue
		}
		area := p.Area()
		if area == nil || seen[area] {
			continue
		}
		seen[area] = true
		if allowed := area.CanTrade(player, target); allowed != nil && !*allowed {
			return false
		}
	}
	if TradeHookSeam.EmitCanTrade != nil {
		if allowed := TradeHookSeam.EmitCanTrade(player, target); allowed != nil && !*allowed {
			return false
		}
	}
	return true
}

// RequestTrade sends a trade request to target, or starts the
// trade when target already requested one from this player
// (requestTrade, Trading.ts:152-195).
func (t *Trading) RequestTrade(target *Player) {
	if !canTrade(t.player, target) {
		t.player.SendMessage("You cannot trade here.")
		return
	}
	if t.state == TradeStateNone || t.state == TradeStateRequestedTrade {
		if !t.requestDelay.Finished() {
			seconds := t.requestDelay.SecondsRemaining()
			wait := fmt.Sprintf("%d seconds", seconds)
			if seconds == 1 {
				// TS omits the number for one second:
				// "You must wait another second before ...".
				wait = "second"
			}
			t.player.SendMessage("You must wait another " + wait + " before sending more trade requests.")
			return
		}
		targetTrading := target.Trading()
		targetState := targetTrading.State()
		initiateTrade := false
		t.SetInteract(target)
		t.SetState(TradeStateRequestedTrade)
		if targetState == TradeStateRequestedTrade {
			if targetTrading.Interact() != nil && targetTrading.Interact() == t.player {
				initiateTrade = true
			}
		}
		// A trade-type line, no name — sent when answering a
		// pending request too, on the same tick just before
		// the trade screen.
		t.player.SendTypedMessage(chatTypeTrade, "Sending trade offer...", "")
		if initiateTrade {
			t.InitiateTrade()
			targetTrading.InitiateTrade()
		} else {
			// The client shows "<name> wishes to trade with
			// you." and makes it clickable.
			if sender := target.tradeSender(); sender != nil {
				sender.SendTradeRequestMessage(t.player.GetDisplayName(), t.player.GetIndex())
				sender.SendTradeRequest(t.player.GetIndex(), t.player.GetDisplayName())
			}
			// Player bots auto-accept: no bot trading system
			// exists in Go (see the file comment).
		}
		t.requestDelay.Start(2)
	} else {
		t.player.SendMessage("You cannot do that right now.")
	}
}

// InitiateTrade opens the offer screen (initiateTrade,
// Trading.ts:197-216): state to TRADE_SCREEN (the derived
// status follows), a cleared and refreshed offer container,
// the interface mounts and flags, the inventory container, an
// open state send, and the opening texts.
func (t *Trading) InitiateTrade() {
	t.SetState(TradeStateTradeScreen)
	t.container.ResetItems()
	t.container.RefreshItems()
	t.interfaceID = tradeOfferInterface
	t.player.SetInterfaceId(tradeOfferInterface) // Trading.ts:203
	if sender := t.player.tradeSender(); sender != nil {
		sender.SendSubInterface((161<<16)|16, tradeOfferInterface, 0)
		sender.SendSubInterface((161<<16)|79, tradeInventoryInterface, 1)
		sender.SendInterfaceScriptArgs(3617, []int{tradeInventoryInterface << 16})
	}
	if sender := t.player.dialogueSender(); sender != nil {
		sender.SendInterfaceFlagsRange(tradeInventoryInterface<<16, 0, 27, 1181694)
		sender.SendInterfaceFlagsRange((tradeOfferInterface<<16)|25, 0, 27, 1181694)
		sender.SendInterfaceFlagsRange((tradeOfferInterface<<16)|28, 0, 27, 1<<10)
	}
	t.player.Inventory().RefreshItems()
	t.sendState(true)
	t.sendText(true)
	// Player bots add items: no bot trading system exists in
	// Go (see the file comment).
}

// CloseTrade ends the trade for both players, returning each
// one's offered items (closeTrade, Trading.ts:222-232). Used
// for declines, closing the interface, and logging out. A
// partner still only at REQUESTED_TRADE is left alone: the
// partner abort requires state >= TRADE_SCREEN.
func (t *Trading) CloseTrade() {
	if t.state == TradeStateNone {
		return
	}
	partner := t.interact
	t.abort("Trade declined.")
	if partner != nil && partner.Trading().Interact() == t.player &&
		partner.Trading().State() >= TradeStateTradeScreen {
		partner.Trading().abort("Other player declined trade.")
	}
}

// abort ends this side of the trade (Trading.ts:234-252): the
// close packet, each offered stack back to the inventory (or
// dropped at the player's feet when it does not fit), an
// inventory refresh, attribute reset, the message, the
// full-inventory notice when anything dropped, and interface
// removal. Slots are re-read live during the sweep, as TS
// iterates the live array: a switch that cleared a later slot
// (a multi-unit non-stackable stack) must not re-drop it.
func (t *Trading) abort(message string) {
	if sender := t.player.tradeSender(); sender != nil {
		sender.SendTradeClose(message)
	}
	inventory := t.player.Inventory()
	dropped := false
	for slot := 0; slot < t.container.Capacity(); slot++ {
		item := t.container.ItemAt(slot)
		if item.ID() <= 0 || item.Amount() <= 0 {
			continue
		}
		if canHold(inventory, item) {
			clone := item.Clone()
			t.container.SwitchItem(inventory, &clone, false, slot, false)
		} else {
			// resetAttributes clears the container below.
			if TradeHookSeam.DropGroundItem != nil {
				TradeHookSeam.DropGroundItem(item.Clone(), t.player.Location(), t.player)
			}
			dropped = true
		}
	}
	inventory.RefreshItems()
	t.ResetAttributes()
	t.player.SendMessage(message)
	if dropped {
		t.player.SendMessage("Your inventory is full, so some of your offered items were dropped on the floor.")
	}
	t.interfaceID = -1
	if sender := t.player.dialogueSender(); sender != nil {
		sender.SendInterfaceRemoval()
	}
}

// AcceptTrade accepts the current screen (acceptTrade,
// Trading.ts:254-361): the offer-screen accept (with the
// receiver room check and the confirm-screen promotion when
// both accept) and the confirm-screen accept (with the second
// room check for both sides, the item transfer, the completed
// emits, the saves, and the close). Clicks during the button
// delay or the accept delay do nothing and say nothing. The
// button delay restarts only on paths that reach the end of
// the method: the room-check refusals return before it.
func (t *Trading) AcceptTrade() {
	if t.interact != nil && !canTrade(t.player, t.interact) {
		t.CloseTrade()
		return
	}
	if !validateTrade(t.player, t.interact, PlayerStatusTrading, TradeStateTradeScreen,
		TradeStateAcceptedTradeScreen, TradeStateConfirmScreen, TradeStateAcceptedConfirmScreen) {
		return
	}
	if !t.buttonDelay.Finished() {
		return
	}
	// Ours: a click during the delay does nothing, and says
	// nothing.
	if t.state == TradeStateTradeScreen && t.acceptDelayed() {
		return
	}
	partner := t.interact.Trading()
	partnerState := partner.State()
	if t.state == TradeStateTradeScreen {
		if !ReceiverCanHold(t.interact, t.OfferedItems()) {
			SendCannotHold(t.interact)
			return
		}
		t.state = TradeStateAcceptedTradeScreen
		t.sendState(false)
		partner.sendState(false)
		if t.state == TradeStateAcceptedTradeScreen && partnerState == TradeStateAcceptedTradeScreen {
			t.confirmScreen()
			partner.confirmScreen()
		}
		// A bot partner re-accepts: no bot trading system
		// exists in Go (see the file comment).
	} else if t.state == TradeStateConfirmScreen {
		// Both are in the same state. Do the second-stage
		// accept.
		t.state = TradeStateAcceptedConfirmScreen
		t.sendState(false)
		partner.sendState(false)
		if t.state == TradeStateAcceptedConfirmScreen && partnerState == TradeStateAcceptedConfirmScreen {
			// Room was checked at the first screen; worn
			// gear taken off since can fill it (the
			// equipment tab stays usable through a trade),
			// and a full inventory would drop what it
			// receives. Ask again for both.
			var short *Player
			for _, receiver := range []*Player{t.player, t.interact} {
				giver := receiver.Trading().Interact().Trading()
				if !ReceiverCanHold(receiver, giver.OfferedItems()) {
					short = receiver
					break
				}
			}
			if short != nil {
				t.state = TradeStateConfirmScreen
				partner.SetState(TradeStateConfirmScreen)
				t.sendState(false)
				partner.sendState(false)
				SendCannotHold(short)
				return
			}
			// Give items to both players...
			receiving := partner.OfferedItems()
			for _, item := range receiving {
				t.player.Inventory().AddItem(item)
			}
			giving := t.OfferedItems()
			for _, item := range giving {
				t.interact.Inventory().AddItem(item)
			}
			// Let plugins react to the finished trade
			// before the save below, so anything they
			// change (such as converting a traded bond to
			// its untradeable form) is persisted with the
			// trade.
			if TradeHookSeam.EmitTradeCompleted != nil {
				TradeHookSeam.EmitTradeCompleted(&TradeCompletedEvent{
					Player: t.player, Partner: t.interact, Received: receiving, Given: giving,
				})
				TradeHookSeam.EmitTradeCompleted(&TradeCompletedEvent{
					Player: t.interact, Partner: t.player, Received: giving, Given: receiving,
				})
			}
			// A bot receiving items: no bot trading system
			// exists in Go (see the file comment).
			// Reset attributes for both players... The
			// partner player is captured first: the reset
			// clears the interact pointer TS keeps in a
			// local (interact_, Trading.ts:261) across
			// the save and close below.
			partnerPlayer := t.interact
			t.ResetAttributes()
			partner.ResetAttributes()
			saveAfterTrade(t.player)
			saveAfterTrade(partnerPlayer)
			if sender := t.player.tradeSender(); sender != nil {
				sender.SendTradeClose(AcceptedTradeMessage)
			}
			if sender := partnerPlayer.tradeSender(); sender != nil {
				sender.SendTradeClose(AcceptedTradeMessage)
			}
			// Send interface removal for both players...
			t.interfaceID = -1
			partner.interfaceID = -1
			if sender := t.player.dialogueSender(); sender != nil {
				sender.SendInterfaceRemoval()
			}
			if sender := partnerPlayer.dialogueSender(); sender != nil {
				sender.SendInterfaceRemoval()
			}
			t.player.SendTypedMessage(chatTypeTrade, AcceptedTradeMessage, "")
			partnerPlayer.SendTypedMessage(chatTypeTrade, AcceptedTradeMessage, "")
		}
	}
	// A bot partner re-accepts: no bot trading system exists
	// in Go (see the file comment).
	t.buttonDelay.Start(1)
}

// acceptDelayed reports whether this player's accept trade
// delay is on and the offers changed less than 3 seconds ago
// (acceptDelayed, Trading.ts:367-373). Bots never wait.
func (t *Trading) acceptDelayed() bool {
	if t.player.IsPlayerBot() {
		return false
	}
	varbit := 0
	if TradeHookSeam.GetVarbit != nil {
		varbit = TradeHookSeam.GetVarbit(t.player, acceptDelayVarbit)
	}
	return varbit == 1 && t.nowMillis()-t.offerChangedAt < AcceptDelayMs
}

// confirmScreen moves to the confirm screen (Trading.ts:375-386):
// state, an open state send, the interface mounts, and the
// opening texts.
func (t *Trading) confirmScreen() {
	t.state = TradeStateConfirmScreen
	t.sendState(true)
	t.interfaceID = tradeConfirmInterface
	t.player.SetInterfaceId(tradeConfirmInterface) // Trading.ts:376
	if sender := t.player.tradeSender(); sender != nil {
		sender.SendSubInterface((161<<16)|16, tradeConfirmInterface, 0)
		sender.SendSubInterface((161<<16)|79, 149, 1)
	}
	t.sendText(true)
}

// containerSwitcher is the Container subset moveTradeItems needs
// beyond the Container interface: SwitchItem lives on
// *itemContainer, not on the interface. *Inventory, *Equipment,
// *Bank, and *tradeOfferContainer all satisfy it through the
// embedded core.
type containerSwitcher interface {
	Container
	SwitchItem(to Container, item *Item, sort bool, slot int, refresh bool)
}

// HandleItem moves items between the inventory and the offer
// container for one trade-action click (handleItem,
// Trading.ts:388-443): only on the offer screen, only for a
// validated trade in an offer-screen state, resetting any
// accept on either side when the offer changes, refusing
// unofferable items, clamping unstackable offers to the free
// offer slots, and stamping the offer-changed time on both
// sides. from/to are the clicked-from and clicked-to
// containers (inventory and offer container, in either order).
func (t *Trading) HandleItem(id, amount, slot int, from, to Container) {
	if t.interfaceID != tradeOfferInterface {
		return
	}
	// Validate this trade action..
	if !validateTrade(t.player, t.interact, PlayerStatusTrading,
		TradeStateTradeScreen, TradeStateAcceptedTradeScreen) {
		return
	}
	// Check if the trade was previously accepted (and now
	// modified)...
	modified := false
	if t.state == TradeStateAcceptedTradeScreen {
		t.state = TradeStateTradeScreen
		modified = true
	}
	if t.interact.Trading().State() == TradeStateAcceptedTradeScreen {
		t.interact.Trading().SetState(TradeStateTradeScreen)
		modified = true
	}
	if modified {
		t.sendState(false)
		t.interact.Trading().sendState(false)
	}
	if t.state == TradeStateTradeScreen && t.interact.Trading().State() == TradeStateTradeScreen {
		// Check if the item is in the right place.
		offered := from.ItemAt(slot)
		if offered.ID() == id {
			src := t.player.ItemDefinitions()
			if !canOffer(offered, src) {
				t.player.SendMessage("You cannot trade that item.")
				return
			}
			// Make sure we can fit that amount in the trade.
			if from.Kind() == ContainerInventory {
				if !defBool(src, id, ItemDefinitionSource.IsStackable) {
					if amount > t.container.GetFreeSlots() {
						amount = t.container.GetFreeSlots()
					}
				}
			}
			if amount <= 0 {
				return
			}
			moveTradeItems(from, to, slot, amount, src)
			changedAt := t.nowMillis()
			t.offerChangedAt = changedAt
			t.interact.Trading().offerChangedAt = changedAt
			// A bot partner re-judges the trade: no bot
			// trading system exists in Go (see the file
			// comment).
		}
	} else {
		t.interfaceID = -1
		if sender := t.player.dialogueSender(); sender != nil {
			sender.SendInterfaceRemoval()
		}
	}
}

// canOffer reports whether item may be offered (Trading.ts:461-469):
// tradeable items may; untradeable items may not; anything else
// needs the item-eligibility event to allow it.
func canOffer(item Item, src ItemDefinitionSource) bool {
	if item.IsTradeable(src) {
		return true
	}
	if item.IsUntradeable() {
		return false
	}
	if TradeHookSeam.ItemEligibility != nil {
		return TradeHookSeam.ItemEligibility(item)
	}
	return false
}

// moveTradeItems moves up to amount of the item in slot, slot by
// slot, so each item keeps its own metadata (charges, contents)
// (moveItems, Trading.ts:475-491). A stack moves only from the
// clicked slot; unstackable items start there and continue
// through the other slots holding the same offerable item. Both
// containers refresh afterwards.
func moveTradeItems(from, to Container, slot, amount int, src ItemDefinitionSource) {
	clicked := from.ItemAt(slot)
	id := clicked.ID()
	switcher, ok := from.(containerSwitcher)
	if !ok {
		// No container implementation lacks SwitchItem; the
		// guard keeps a foreign Container from panicking
		// the tick goroutine.
		from.RefreshItems()
		to.RefreshItems()
		return
	}
	if defBool(src, id, ItemDefinitionSource.IsStackable) {
		moving := clicked.Clone()
		movingAmount := amount
		if movingAmount > clicked.Amount() {
			movingAmount = clicked.Amount()
		}
		moving.SetAmount(movingAmount)
		switcher.SwitchItem(to, &moving, false, slot, false)
	} else {
		slots := make([]int, 0, from.Capacity())
		slots = append(slots, slot)
		for index := 0; index < from.Capacity(); index++ {
			if index != slot {
				slots = append(slots, index)
			}
		}
		moved := 0
		for _, index := range slots {
			if moved >= amount || to.GetFreeSlots() <= 0 {
				break
			}
			item := from.ItemAt(index)
			if item.ID() != id || !canOffer(item, src) {
				continue
			}
			clone := item.Clone()
			switcher.SwitchItem(to, &clone, false, index, false)
			moved++
		}
	}
	from.RefreshItems()
	to.RefreshItems()
}

// canHold reports whether item fits in container (Trading.ts:493-499):
// a free slot, or a stack of the same item and metadata.
func canHold(container Container, item Item) bool {
	if container.GetFreeSlots() > 0 {
		return true
	}
	var src ItemDefinitionSource
	if p := container.Player(); p != nil {
		src = p.ItemDefinitions()
	}
	if !defBool(src, item.ID(), ItemDefinitionSource.IsStackable) {
		return false
	}
	meta := item.metaFingerprint()
	for _, held := range container.Items() {
		if held.ID() == item.ID() && held.metaFingerprint() == meta {
			return true
		}
	}
	return false
}

// saveAfterTrade saves player after a completed trade
// (Trading.save, Trading.ts:501-507): skipped for players
// holding the skip-persistence attribute, and silent when no
// save seam is wired (see TradeHooks.SavePlayer).
func saveAfterTrade(player *Player) {
	if player.GetAttribute(skipPersistenceAttribute) == true {
		return
	}
	if TradeHookSeam.SavePlayer != nil {
		TradeHookSeam.SavePlayer(player, "trade")
	}
}

// ResetAttributes resets the trade attributes
// (resetAttributes, Trading.ts:509-522): the offer-changed
// stamp, the interact, the state (the derived status follows),
// and the container. The interface id is NOT reset here — TS
// resets it through the interface-removal send, and the call
// sites that send one clear it themselves.
func (t *Trading) ResetAttributes() {
	t.offerChangedAt = 0
	// Reset trade attributes
	t.SetInteract(nil)
	t.SetState(TradeStateNone)
	// Reset container..
	t.container.ResetItems()
}

// SendCannotHold sends the refusal when receiver lacks room
// (sendCannotHold, Trading.ts:531-538): the same lines at
// either accept, to both sides.
func SendCannotHold(receiver *Player) {
	giver := receiver.Trading().Interact()
	if giver == nil {
		return
	}
	freeSlots := receiver.Inventory().GetFreeSlots()
	giver.SendMessage("")
	giver.SendMessage("@or3@" + receiver.GetDisplayName() + " will not be able to hold that item.")
	giver.SendMessage("@or3@They have " + strconv.Itoa(freeSlots) + " free inventory slot" + pluralSuffix(freeSlots))
	receiver.SendMessage("Trade cannot be accepted, you don't have enough free inventory space.")
}

// pluralSuffix renders the free-slots suffix (Trading.ts:536):
// "." for one slot, "s." otherwise.
func pluralSuffix(freeSlots int) string {
	if freeSlots == 1 {
		return "."
	}
	return "s."
}

// ReceiverCanHold reports whether receiver can take all of
// items (receiverCanHold, Trading.ts:544-559): a slot for each
// non-stackable and each stack not held, and room under the cap
// on a stack that is held (a capped stack would otherwise clamp
// the trade away).
func ReceiverCanHold(receiver *Player, items []Item) bool {
	inventory := receiver.Inventory()
	src := receiver.ItemDefinitions()
	slots := 0
	for _, item := range items {
		held := false
		if defBool(src, item.ID(), ItemDefinitionSource.IsStackable) {
			meta := item.metaFingerprint()
			for _, entry := range inventory.Items() {
				if entry.ID() == item.ID() && entry.metaFingerprint() == meta {
					held = true
					break
				}
			}
		}
		if held {
			if inventory.GetAddableAmount(item) < item.Amount() {
				return false
			}
		} else {
			slots++
		}
	}
	return slots <= inventory.GetFreeSlots()
}

// party builds one side's trade-window view (party,
// Trading.ts:602-613).
func (t *Trading) party(player *Player) TradeParty {
	trading := player.Trading()
	var offers []TradePartyOffer
	for slot, item := range trading.container.Items() {
		if item.ID() >= 0 && item.Amount() > 0 {
			offers = append(offers, TradePartyOffer{Slot: slot, ItemID: item.ID(), Quantity: item.Amount()})
		}
	}
	return TradeParty{
		PlayerID:        player.GetIndex(),
		Name:            player.GetDisplayName(),
		Accepted:        trading.State() == TradeStateAcceptedTradeScreen,
		ConfirmAccepted: trading.State() == TradeStateAcceptedConfirmScreen,
		Offers:          offers,
	}
}

// sendState sends this side's trade-window state (sendState,
// Trading.ts:615-625): an open or update packet carrying both
// parties, plus the texts on an update. The session id sorts
// the two player indices numerically and joins them with ":".
func (t *Trading) sendState(open bool) {
	if t.interact == nil {
		return
	}
	low, high := t.player.GetIndex(), t.interact.GetIndex()
	if low > high {
		low, high = high, low
	}
	sessionID := fmt.Sprintf("%d:%d", low, high)
	stage := TradeStageOffer
	if t.state >= TradeStateConfirmScreen {
		stage = TradeStageConfirm
	}
	self := t.party(t.player)
	other := t.party(t.interact)
	if sender := t.player.tradeSender(); sender != nil {
		if open {
			sender.SendTradeOpen(sessionID, stage, self, other)
		} else {
			sender.SendTradeUpdate(sessionID, stage, self, other)
		}
	}
	if !open {
		t.sendText(false)
	}
}

// tradeText is one sendText write: a component uid and its text.
type tradeText struct {
	uid  int
	text string
}

// sendText sends the headings, values, and status line
// (sendText, Trading.ts:631-663). opened forgets the text sent
// to the last trade window in TS (a FrameUpdater clear per
// uid); Go has no FrameUpdater, so the clear is a no-op and
// every string sends (the DialogueSender deviation).
func (t *Trading) sendText(opened bool) {
	_ = opened
	other := t.interact
	if other == nil {
		return
	}
	sender := t.player.dialogueSender()
	if sender == nil {
		return
	}
	name := other.GetDisplayName()
	otherTrading := other.Trading()
	otherState := otherTrading.State()
	src := t.player.ItemDefinitions()
	var texts []tradeText
	if t.state >= TradeStateConfirmScreen {
		texts = []tradeText{
			{tradeConfirmOpponentUID, "Trading with:<br>" + name},
			{tradeConfirmOwnHeadingUID, "You are about to give:<br>" + formatTradeValue(t.OfferedItems(), src, false)},
			{tradeConfirmOtherHeadingUID, "In return you will receive:<br>" + formatTradeValue(otherTrading.OfferedItems(), src, true)},
			{tradeConfirmStatusUID, tradeStatusText(
				t.state == TradeStateAcceptedConfirmScreen,
				otherState == TradeStateAcceptedConfirmScreen,
				"Are you sure you want to make this trade?")},
		}
	} else {
		texts = []tradeText{
			{tradeOfferTitleUID, "Trading With: " + name},
			{tradeOfferOwnHeadingUID, "You offer:<br>" + formatTradeValue(t.OfferedItems(), src, false)},
			{tradeOfferOtherHeadingUID, name + " offers:<br>" + formatTradeValue(otherTrading.OfferedItems(), src, false)},
			{tradeOfferFreeSlotsUID, freeSlotsText(name, other.Inventory().GetFreeSlots())},
			{tradeOfferStatusUID, tradeStatusText(
				t.state == TradeStateAcceptedTradeScreen,
				otherState == TradeStateAcceptedTradeScreen,
				"")},
		}
	}
	for _, text := range texts {
		sender.SendString(text.text, text.uid)
	}
}

// formatTradeValue renders an offer's value line (formatValue,
// Trading.ts:671-678): the Grand Exchange price per item (the
// store value where there is none), summed over the valid
// stacks. redWhenZero renders a zero value red: the confirm
// screen's "you will receive" value is red at 0; every other
// value, 0 included, stays white.
func formatTradeValue(items []Item, src ItemDefinitionSource, redWhenZero bool) string {
	var value int64
	for _, item := range items {
		value += int64(geValue(src, item.ID())) * int64(item.Amount())
	}
	colour := "ffffff"
	if redWhenZero && value == 0 {
		colour = "ff0000"
	}
	return "(Value: <col=" + colour + ">" + formatGrouped(value) + "</col> coins)"
}

// freeSlotsText renders the partner's free-slots line
// (Trading.ts:680-682).
func freeSlotsText(name string, freeSlots int) string {
	suffix := "s"
	if freeSlots == 1 {
		suffix = ""
	}
	return name + " has " + strconv.Itoa(freeSlots) + " free inventory slot" + suffix + "."
}

// tradeStatusText renders the status line (statusText,
// Trading.ts:684-688).
func tradeStatusText(accepted, otherAccepted bool, idle string) string {
	if accepted && !otherAccepted {
		return "Waiting for other player..."
	}
	if !accepted && otherAccepted {
		return "Other player has accepted."
	}
	return idle
}

// ListTradeItems renders an item list for a trade/confirm
// display (Trading.listItems, Trading.ts:88-101): one line per
// valid stack, "Absolutely nothing!" when empty. Amounts at or
// above a thousand carry a short form plus the full grouped
// amount in parentheses. Item names come from the definition
// source ("" without one, the DEFAULT definition's name);
// underscores in names become spaces. No TS core caller
// exists — the list is plugin-facing surface — but the format
// is pinned here all the same.
func ListTradeItems(items []Item, src ItemDefinitionSource) string {
	var lines []string
	for _, item := range items {
		if !item.IsValid() {
			continue
		}
		amount := item.Amount()
		var formatted string
		switch {
		case amount >= 1_000_000_000:
			formatted = "@gre@" + strconv.Itoa(amount/1_000_000_000) + " billion @whi@(" + formatGrouped(int64(amount)) + ")"
		case amount >= 1_000_000:
			formatted = "@gre@" + strconv.Itoa(amount/1_000_000) + " million @whi@(" + formatGrouped(int64(amount)) + ")"
		case amount >= 1000:
			formatted = "@cya@" + strconv.Itoa(amount/1000) + "K @whi@(" + formatGrouped(int64(amount)) + ")"
		default:
			formatted = formatGrouped(int64(amount))
		}
		name := strings.ReplaceAll(itemName(src, item.ID()), "_", " ")
		lines = append(lines, name+" x @red@"+formatted)
	}
	if len(lines) == 0 {
		return "Absolutely nothing!"
	}
	return strings.Join(lines, "\n")
}

// formatGrouped renders an integer with thousands separators
// (Misc.format, Misc.ts:301-303: Number.toLocaleString(), which
// groups by threes with commas for integer values).
func formatGrouped(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := strconv.FormatInt(n, 10)
	var out strings.Builder
	if neg {
		out.WriteByte('-')
	}
	lead := len(digits) % 3
	if lead == 0 {
		lead = 3
	}
	out.WriteString(digits[:lead])
	for i := lead; i < len(digits); i += 3 {
		out.WriteByte(',')
		out.WriteString(digits[i : i+3])
	}
	return out.String()
}

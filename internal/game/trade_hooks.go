package game

// TradeHooks is the game-side trade event seam, following the
// CombatHooks pattern (combat_hooks.go): the func fields trade
// code calls for the events TS emits through PluginManager, the
// policy hooks TS consults before acting, and the coordinator
// wiring seams trade code needs from outside the game package.
// The game package must never import plugins — the seam is func
// fields only. Every call site checks for nil, so an unpopulated
// seam is inert and core trading runs standalone; the hub-side
// implementation (Worker C) populates the hook fields at boot,
// and the coordinator populates the wiring fields.
//
// GATE SHAPES. Each hook field documents the exact emit shape
// from PluginManager.ts, because the shape — allow-veto,
// handled-replacement, or run-all observer — is part of the
// behavior, not an implementation detail.

// TradeCanTradeEvent is the PluginCanTradeEvent
// (PluginTypes.ts:417-421): the two traders and the allow
// verdict. Hooks set Allow to permit or refuse the trade; a
// nil Allow means "no opinion".
type TradeCanTradeEvent struct {
	Player *Player
	Target *Player
	Allow  *bool
}

// TradeRequestEvent is the PluginTradeRequestEvent
// (PluginTypes.ts:424-428): the requester, the target, and the
// handled flag. A hook sets Handled to replace the default
// walk-and-request flow entirely.
type TradeRequestEvent struct {
	Player  *Player
	Target  *Player
	Handled bool
}

// TradeCompletedEvent is the PluginTradeCompletedEvent
// (PluginTypes.ts:436-444): the player the event is fired for,
// the partner, and the stacks this player received and gave.
// It fires once per player, after the offered items have
// changed hands and before either player is saved, so a hook
// mutating the player's containers (converting a traded bond
// to its untradeable form) has its change persisted.
// Received and Given are copies (Go value semantics): hooks
// observe the stacks and mutate the player's containers, never
// the slices.
type TradeCompletedEvent struct {
	Player   *Player
	Partner  *Player
	Received []Item
	Given    []Item
}

// TradeItemEligibilityRequest is the "trade:item-eligibility"
// custom-event request (Trading.ts:461-469, canOffer): for an
// item that is neither tradeable nor untradeable by definition
// and meta, the request starts with Allowed false and hooks may
// set it true. The hub passes it opaquely, as the shop:*
// requests; the adapter reads Allowed back after the emit.
type TradeItemEligibilityRequest struct {
	Item    Item
	Allowed bool
}

// TradeHooks holds every trade hook and wiring field. All
// fields are optional; trade code calls them nil-safely. The
// wiring populates TradeHookSeam once at boot before ticking
// starts.
type TradeHooks struct {
	// EmitCanTrade is emitCanTrade (PluginManager.ts:1456-1467):
	// the ALLOW-VETO gate. With no hooks it returns null and
	// the trade is allowed. Hooks run in registration order;
	// the FIRST hook to set a non-null allow decides, and
	// later hooks never run. Trading.canTrade allows the
	// trade unless the emit returns exactly false, so the
	// hub adapter must return nil for "no opinion" (no hooks,
	// or no hook set allow), not false.
	//
	// The hub adapter runs the onCanTrade hooks over one
	// TradeCanTradeEvent and returns event.Allow.
	EmitCanTrade func(player, target *Player) *bool

	// EmitTradeRequest is emitTradeRequest
	// (PluginManager.ts:1471-1481): the HANDLED-REPLACEMENT
	// gate. Hooks run in registration order until one sets
	// handled, then stop; the emit returns whether any hook
	// handled the request. A handled request replaces the
	// default walk-and-request flow entirely: the listener
	// returns without walking. A nil event, a handled event,
	// or a missing player/target returns false without
	// running hooks. The listener is the only call site.
	//
	// The hub adapter runs the onTradeRequest hooks over the
	// event and returns event.Handled.
	EmitTradeRequest func(e *TradeRequestEvent) bool

	// EmitTradeCompleted is emitTradeCompleted
	// (PluginManager.ts:1483-1490): the RUN-ALL observer.
	// Every hook runs, in registration order; no hook can
	// veto or replace anything. A nil event or a missing
	// player/partner runs no hooks. Trading fires it twice
	// per completed trade (once per player, mirrored
	// received/given) before saving either player.
	//
	// The hub adapter runs the onTradeCompleted hooks over
	// the event.
	EmitTradeCompleted func(e *TradeCompletedEvent)

	// ItemEligibility is the "trade:item-eligibility" custom
	// event (Trading.ts:461-469, canOffer): for an item that
	// is neither tradeable nor untradeable by definition and
	// meta, the request starts with allowed=false and hooks
	// may set it true. Nil means no hooks: the item cannot be
	// offered. The adapter runs the custom event and returns
	// the request's allowed flag.
	ItemEligibility func(item Item) bool

	// GetVarbit reads a varbit value for the accept-delay rule
	// (PacketSender.getVarbit, PacketSender.ts:344-348, over
	// the PacketSender varp cache). Nil means no varp cache:
	// every varbit reads 0, the fresh-cache default, so the
	// accept delay never triggers. The coordinator wires it
	// to the session's varp store (see installTradeHooks);
	// Trading reads only varbit 13130.
	GetVarbit func(player *Player, id int) int

	// DropGroundItem registers a ground item at a tile
	// (ItemOnGroundManager.registers, called by Trading.abort
	// when a returning offer does not fit the inventory).
	// Core has no ground-item manager reference; the wiring
	// supplies it. Nil drops the item silently (documented
	// gap, following the CombatHooks.DropGroundItem precedent):
	// the coordinator MUST wire this, or aborting a trade
	// with a full inventory destroys items.
	DropGroundItem func(item Item, loc Location, owner *Player)

	// SavePlayer saves a player after a completed trade
	// (Trading.save, Trading.ts:493-500, calling
	// GameConstants.PLAYER_PERSISTENCE.save(player, "trade")).
	// The wiring supplies the save; failures are the wiring's
	// to log (TS logs "[trade] Failed to save ... after a
	// trade"). Nil means no save runs: the coordinator MUST
	// wire this, or completed trades are not persisted until
	// the next autosave/disconnect save. reason is always
	// "trade" from Trading; the parameter keeps the seam
	// honest about which save reason TS passes.
	SavePlayer func(player *Player, reason string)
}

// TradeHookSeam is the populated trade hook seam. The
// coordinator / hub wiring assigns its fields at boot; trade
// code reads it nil-safely field by field. It is a var (not a
// pointer) so a zero seam is a valid inert seam.
var TradeHookSeam TradeHooks

// ---- Trade sender ----
//
// TradeSender is the optional SessionPort extension for the
// trade window: the session side of Trading's packet sends.
// The game layer must never import the protocol package, so —
// as with ChatMessageSender (message.go) and DialogueSender
// (dialogue_entries.go) — the encoding lives in the session
// package, which the coordinator attaches as the SessionPort.
// A session that does not implement TradeSender drops the
// sends silently: session-less and test players have no client
// to inform. The coordinator must implement it on *Session
// (see the per-method wire notes); until then trades run
// statefully but invisibly.

// TradeStage is the trade window stage a TradeSender send
// carries: the offer screen or the confirm screen (Trading.ts
// sendState: state >= CONFIRM_SCREEN is "confirm", else
// "offer"). The session adapter maps it to the protocol stage
// byte (0 offer, 1 confirm).
type TradeStage int

const (
	// TradeStageOffer is the offer screen.
	TradeStageOffer TradeStage = iota
	// TradeStageConfirm is the confirm screen.
	TradeStageConfirm
)

// TradePartyOffer is one offered stack in a TradeParty: the
// offer-container slot, the item id, and the quantity.
type TradePartyOffer struct {
	Slot     int
	ItemID   int
	Quantity int
}

// TradeParty is one side of a trade window as the sender sees
// it (Trading.ts party(): playerId, name, accepted,
// confirmAccepted, offers). The session adapter maps it onto
// protocol.TradePartyView field for field.
type TradeParty struct {
	PlayerID        int
	Name            string
	Accepted        bool
	ConfirmAccepted bool
	Offers          []TradePartyOffer
}

// TradeSender carries the trade window's outbound packets plus
// the two interface primitives Trading needs that no existing
// game seam covers (sub-interface mounts and a script call with
// arguments). The remaining PacketSender calls Trading makes
// ride existing seams: strings, flags ranges, and interface
// removal go through DialogueSender; inventory container sends
// are Inventory.RefreshItems (sendItemContainer for an
// Inventory sends an inventory snapshot and ignores the
// interface id, PacketSender.ts:1042-1050); game/typed messages
// go through SendMessage/SendTypedMessage.
type TradeSender interface {
	// SendTradeRequest sends a trade-request notice
	// (encodeTradeRequest, ClientProtocol.ts:2131-2135):
	// the requester's index and display name.
	SendTradeRequest(playerID int, name string)
	// SendTradeOpen sends a trade-window open
	// (encodeTradeOpen, ClientProtocol.ts:2144-2146) with an
	// empty info string.
	SendTradeOpen(sessionID string, stage TradeStage, self, other TradeParty)
	// SendTradeUpdate sends a trade-window state refresh
	// (encodeTradeUpdate, ClientProtocol.ts:2148-2150) with
	// an empty info string.
	SendTradeUpdate(sessionID string, stage TradeStage, self, other TradeParty)
	// SendTradeClose sends a trade-window close
	// (encodeTradeClose, ClientProtocol.ts:2152-2154) with
	// the reason string.
	SendTradeClose(reason string)
	// SendTradeRequestMessage sends the clickable request
	// line (encodeChatMessage, kind "trade", type 101,
	// Trading.ts:225-228): "wishes to trade with you." from
	// the named player at the given index. The session
	// adapter encodes it as a CHAT_MESSAGE with kind trade,
	// the explicit chat type 101, From set to the name, an
	// empty prefix, and the player id.
	SendTradeRequestMessage(fromName string, fromIndex int)
	// SendSubInterface mounts a widget group into a parent
	// component (PacketSender.sendSubInterface,
	// PacketSender.ts:1544-1551; encodeWidgetOpenSub with no
	// options). Trading mounts the offer/confirm groups and
	// the inventory group through it.
	SendSubInterface(targetUID, groupID, typ int)
	// SendInterfaceScriptArgs runs a widget script with
	// integer arguments (PacketSender.sendInterfaceScript,
	// PacketSender.ts:1706-1718; encodeWidgetRunScript).
	// Trading's only call is script 3617 with the inventory
	// group uid. DialogueSender.SendInterfaceScript covers
	// only the no-argument form.
	SendInterfaceScriptArgs(scriptID int, args []int)
}

// tradeSender returns the player's TradeSender, or nil when
// the player is nil or the session does not implement it (the
// ChatMessageSender assertion pattern, message.go).
func (p *Player) tradeSender() TradeSender {
	if p == nil {
		return nil
	}
	sender, _ := p.Session.(TradeSender)
	return sender
}

// ---- Optional item-definition reads ----
//
// ItemGEValueSource is the optional Grand Exchange value
// capability of an item-definition source: the value Trading's
// formatValue sums (ItemDefinition.getGrandExchangeValue,
// ItemDefinition.ts:190-192: the item-prices.json quote, falling
// back to the store value). It is separate from
// ItemDefinitionSource for the same reason ItemAnimationSource
// is (itemdef.go): only the trade value line reads it, and the
// store value it falls back to is deliberately absent from the
// source (getTotalValue is a documented gap). A source that
// does not implement it yields 0 for every id, so the value
// line reads "(Value: 0 coins)" until the coordinator wires
// GE values. Implementations must be total over all ids.
type ItemGEValueSource interface {
	// GrandExchangeValue returns the GE value of id (the
	// quote, or the store value when there is none).
	GrandExchangeValue(id int) int
}

// ItemNameSource is the optional name capability of an
// item-definition source: the display name ListTradeItems
// prints (ItemDefinition.getName, ItemDefinition.ts:177-179).
// It is separate from ItemDefinitionSource for the same
// reason: only the trade item list reads names. A source that
// does not implement it yields "" for every id, matching the
// DEFAULT definition's empty name (ItemDefinition.ts:75).
// Implementations must be total over all ids.
type ItemNameSource interface {
	// ItemName returns the display name of id.
	ItemName(id int) string
}

// ItemNoteIDSource is the optional note-id capability of an
// item-definition source: the bank-note id of id
// (ItemDefinition.getNoteId, ItemDefinition.ts:189-191), or
// -1 when the item has no note form. It is separate from
// ItemDefinitionSource for the same reason as ItemNameSource:
// only the Ent trunk's noted-log delivery reads note ids
// (EntTrunk.Woodcutting.js: giveLogs). A source that does not
// implement it yields the unnoted id, matching the DEFAULT
// definition's -1 note id (ItemDefinition.ts:127-138).
// Implementations must be total over all ids.
type ItemNoteIDSource interface {
	// NoteID returns the note id of id, or -1 when unnoted.
	NoteID(id int) int
}

// geValue reads the GE value of id through src, or 0 when src
// is nil or lacks the capability.
func geValue(src ItemDefinitionSource, id int) int {
	if src == nil {
		return 0
	}
	if values, ok := src.(ItemGEValueSource); ok {
		return values.GrandExchangeValue(id)
	}
	return 0
}

// itemName reads the display name of id through src, or ""
// when src is nil or lacks the capability.
func itemName(src ItemDefinitionSource, id int) string {
	if src == nil {
		return ""
	}
	if names, ok := src.(ItemNameSource); ok {
		return names.ItemName(id)
	}
	return ""
}

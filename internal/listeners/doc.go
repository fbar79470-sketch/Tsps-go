// Package listeners dispatches turn packets to the port of the packet
// listeners in TS net/packet/impl/* (18 files). It owns only decoding
// (via internal/protocol) and dispatch. Since phase 9, the listeners
// unblocked by the container/appearance/skill systems are
// implemented: Equip (equip/unequip, the 2H/shield rule,
// requirements), DropItem (gates + ground-item registration),
// SwitchItemSlot (inventory swap, bank rearrange), ItemAction (the
// equip fallback; plugin actions stay drops), and PickupItem
// (operable lookup + walk-to arrival + transfer). Phase 12 adds
// Chat (public chat: text normalization, mute/filter gates,
// overhead forceChat, and the local-list broadcast; its world
// seams arrive through Deps.Chat and degrade to no-ops when
// unwired — see chat.go). Every other
// listener decodes its payload exactly as TS does and drops it with
// a named reason, because the system it acts through — combat,
// dialogue, shops, trading, following, MapObjects, plugins — is
// NOT STARTED in Go. Nothing here invents behavior: every gate
// degeneration names the missing system at its site, and every
// plugin event degenerates to its no-hooks default (proceed or
// unhandled), as documented in each listener file.
//
// How TS routes packets (there is no listener registry): the
// ClientConnection decodes each frame with decodeClientPackets
// (ClientProtocol.ts lines 486-506) into typed messages, queues every
// non-immediate message as a turn packet, and runs the typed switch in
// dispatch() (NetworkBuilder.ts lines 661-1386) at the start of the
// player's turn. The switch calls listener static methods (or the two
// instance listeners OBJECT_ACTIONS/NPC_ACTIONS and the MAGIC_ITEMS
// instance, NetworkBuilder.ts lines 108-110) directly, sometimes
// through the inventoryAction/groundItemAction/tradeAction helpers
// (NetworkBuilder.ts lines 1754-1876). Several listeners share one
// message type and are split by decoded content (option text, option
// number, widget group) or by cache data; the per-opcode mapping in
// Handle and the per-listener table in Statuses record exactly which
// sharing exists. FollowPlayer, TradeRequest, and Command have no
// opcode of their own: they are sub-dispatches of the player_option
// and chat messages (see Statuses).
//
// Two seams connect the implemented listeners to the world, both
// supplied by the coordinator in handleTurnPacket (see Deps):
// Deps.GroundItems, the world ground-item store, and the player's
// item-definition source, which the listeners additionally assert
// to game.ItemActionSource when it implements it. The chat
// listener's seams live in Deps.Chat (see ChatHooks in chat.go).
// Without the
// action source, option routing falls back to the ObjType defaults
// (Drop/Take only); the coordinator wires the real source —
// CacheDefinitions.GetItem(id).InventoryActions/GroundActions/
// GetShiftClickIndex() — onto the same object returned as the
// player's ItemDefinitionSource. The walk-to arrival machinery the
// pickup listener uses lives on the movement queue itself
// (MovementQueue.WalkToGroundItem); there is no walk seam in Deps,
// because no TS listener walk reduces to a bare MovementRequest
// (see the Deps comment for the removed WalkTo field's fate).
package listeners

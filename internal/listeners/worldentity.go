package listeners

import (
	"fmt"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// World-entity interaction (no listener file: the
// world_entity_option case inline in dispatch(),
// NetworkBuilder.ts:747-762) and the helm's set_heading case
// (NetworkBuilder.ts:741-743).
//
// A world-entity click is purely hub-emit: TS has no listener
// file for it, no before→main→operated dispatch, no walk-to,
// and no gates beyond the three in the dispatch case — the
// boat exists, the viewer can see it, and the viewer's root
// tile is within 32 of the boat's tile. Forged, foreign, or
// out-of-range indices never reach plugin handlers: they drop
// silently. The gates live behind the Deps seams (the
// BoatManager answers them); the emit goes through
// Deps.WorldEntityEmit, the Deps-adapter pattern — this
// package never imports internal/plugins. The packet IS in
// WORLD_INTERACTIONS and INTERACTION_CLICKS
// (NetworkBuilder.ts:251, 269): the dispatch case runs the
// world-input preamble, and the route sets the click-run
// override with modifier flags 0 (the message carries no
// modifierFlags field, so TS reads 0).
//
// set_heading is not a WORLD_INTERACTIONS packet: no preamble,
// no override. It applies the heading through
// Deps.SetHelmHeading and drops silently with no boats
// installed.

// WorldEntityOption is one decoded "world_entity_option"
// message: the world-entity index clicked and the option
// number (1-6, from the opcode).
type WorldEntityOption struct {
	EntityIndex int
	ClickType   int
}

// decodeWorldEntityOption decodes the six native world-entity
// opcodes (220-225), exactly as ClientProtocol.ts:728-752.
// Each opcode carries the index in a different short form plus
// a discarded byte (ops 1-5); op 6 carries the index alone.
// A trailing byte is malformed, as for the NPC-option decode.
func decodeWorldEntityOption(opcode byte, payload []byte) (WorldEntityOption, error) {
	r := protocol.NewReader(payload)
	var index uint16
	var clickType int
	var err error
	readIndex := func(read func() (uint16, error)) {
		if err == nil {
			index, err = read()
		}
	}
	discardByte := func(read func() (byte, error)) {
		if err == nil {
			_, err = read()
		}
	}
	switch opcode {
	case protocol.NativeOpWorldEntity1: // 220: shortLE + byteSub.
		readIndex(r.ShortLE)
		discardByte(r.ByteSub)
		clickType = 1
	case protocol.NativeOpWorldEntity2: // 221: shortAdd + byteAdd.
		readIndex(r.ShortAdd)
		discardByte(r.ByteAdd)
		clickType = 2
	case protocol.NativeOpWorldEntity3: // 222: short + byteNeg.
		readIndex(r.Short)
		discardByte(r.ByteNeg)
		clickType = 3
	case protocol.NativeOpWorldEntity4: // 223: byteSub + short.
		discardByte(r.ByteSub)
		readIndex(r.Short)
		clickType = 4
	case protocol.NativeOpWorldEntity5: // 224: byteAdd + shortLE.
		discardByte(r.ByteAdd)
		readIndex(r.ShortLE)
		clickType = 5
	case protocol.NativeOpWorldEntity6: // 225: shortAddLE.
		readIndex(r.ShortAddLE)
		clickType = 6
	default:
		return WorldEntityOption{}, fmt.Errorf("listeners: unsupported world_entity_option opcode %d", opcode)
	}
	if err != nil {
		return WorldEntityOption{}, err
	}
	if r.Remaining() != 0 {
		return WorldEntityOption{}, fmt.Errorf("listeners: trailing bytes after WORLD_ENTITY_OPTION: %d remaining", r.Remaining())
	}
	return WorldEntityOption{EntityIndex: int(index), ClickType: clickType}, nil
}

// decodeSetHeading decodes the set_heading message
// (ClientProtocol.ts:726-727): one byte, masked to the 0-15
// packed heading.
func decodeSetHeading(payload []byte) (int, error) {
	r := protocol.NewReader(payload)
	heading, err := r.Byte()
	if err != nil {
		return 0, err
	}
	if r.Remaining() != 0 {
		return 0, fmt.Errorf("listeners: trailing bytes after SET_HEADING: %d remaining", r.Remaining())
	}
	return int(heading) & 15, nil
}

// WorldEntityInteractionEvent is the world-entity interaction
// event the Deps emit func carries to plugin hooks
// (PluginTypes.ts:199-204: {player, entityIndex, clickType,
// handled}). Handled is set by a hook that claims the click;
// the dispatch outcome never depends on it — the emit IS the
// handling, as for social packets.
type WorldEntityInteractionEvent struct {
	Player      *game.Player
	EntityIndex int
	ClickType   int
	Handled     bool
}

// routeWorldEntityAction routes a decoded world-entity click:
// the click-run override first (the message has no
// modifierFlags, so TS reads 0), then the three dispatch
// gates through Deps.WorldEntityInteractable, then the hub
// emit through Deps.WorldEntityEmit. A refused gate drops
// silently (noAction), exactly as TS returns without emitting.
// A routed click counts as acted whether or not a hook
// claimed it, and whether or not any emit is wired: the emit
// IS the handling. A nil emit seam means no boats are
// installed, so the gate refuses everything — no new drop
// reason is needed.
func routeWorldEntityAction(p *game.Player, msg WorldEntityOption, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	if q := p.MovementQueue(); q != nil {
		q.SetClickRunOverride(game.ClickRunOverride(p.IsRunning(), 0))
	}
	if deps.WorldEntityInteractable == nil || !deps.WorldEntityInteractable(p, msg.EntityIndex) {
		return noAction()
	}
	if deps.WorldEntityEmit != nil {
		ev := &WorldEntityInteractionEvent{Player: p, EntityIndex: msg.EntityIndex, ClickType: msg.ClickType}
		deps.WorldEntityEmit(ev)
	}
	return acted()
}

// routeSetHeading routes a decoded set_heading message through
// Deps.SetHelmHeading. A nil seam means no boats are
// installed, so the packet drops silently.
func routeSetHeading(p *game.Player, heading int, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	if deps.SetHelmHeading == nil {
		return noAction()
	}
	deps.SetHelmHeading(p, heading)
	return acted()
}

package session

import "tsps-go/internal/protocol"

// TurnSink is the connection -> player turn-packet bridge: the one
// delivery path by which an entered player's non-immediate packets
// reach the game layer.
//
// TS grounding: in TS the connection queues turn packets and the
// player's turn drains them — Player.process() runs
// session.processPackets() first (Player.ts:489), which calls the
// turn-packet processor the connection installed at enterWorld
// (PlayerSession.ts:219-227, NetworkBuilder.ts:1652), which runs
// processTurnPackets (NetworkBuilder.ts:641-656): drain the queue and
// dispatch each packet inside the player's turn, so input acts on the
// next tick. The Go game layer already has the drain side:
// game.Player.QueueTurnPacket (concurrency-safe) feeds
// game.Player.Process, which drains through TurnHandler on the tick.
// The bridge is the handoff between the two queues.
//
// The world adapter returns the sink from WorldPort.AddPlayer, bound
// to the player it admits; Connection.Dispatch hands each entered,
// non-immediate packet to the sink instead of queueing it locally.
// The sink reports whether the packet was accepted: false means the
// player's turn queue is at its 240-packet cap (PacketQueueLimit),
// and Dispatch answers exactly as the TS cap branch does — cleanup
// and close 1008 ("input limit"). When AddPlayer returns a nil sink
// (tests, or a world that drains the connection queue itself),
// Dispatch falls back to the local turnPackets queue drained via
// TurnPackets. A packet is delivered to exactly one of the two,
// never both.
//
// REJECTED ALTERNATIVE: a World-hook drain, in which a per-tick world
// hook would pull TurnPackets() from every connection into the
// players' queues. It needs the world (or the adapter) to track every
// live connection, adds a second queue hop and a tick-goroutine /
// read-loop coupling for no benefit: the sink preserves arrival order
// (one read loop feeds one player queue) and the "input acts next
// tick" semantics (Player.Process is the only drain), with the
// delivery decision made once, at entry.
type TurnSink func(pkt protocol.Packet) bool

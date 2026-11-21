package session

// NPC-specific effects: viewer-only one-shot packets for one NPC, ported
// from NpcSpecificEffects.ts (the whole file, 15 lines) and the
// PlayerSession sites that own it: the queue field (PlayerSession.ts:126),
// queueNpcSpecificEffect (128-130), clear on player swap (241) and logout
// (324), take at the top of flushClient (355-357), and the post-sync send
// loop (632-641).
//
// This file is plumbing only. The only producers in TS are the three
// PacketSender methods sendNpcHeadIconSpecific, sendNpcAnimationSpecific,
// and sendNpcSpotAnimationSpecific (PacketSender.ts:202-219), all
// content/plugin-driven; no equivalent exists in Go, so nothing in this
// package queues an effect yet. The queue type and the send-loop helper
// below are the honest seam future producers will call through the
// Session methods the coordinator wires up (see the integration note at
// the end of this comment); no producer is invented here. The effects
// carry pre-encoded packets: the head-icon/animation/spotanim-specific
// encoders are separate and NOT STARTED, and the "network:npc-sync"
// plugin event (PlayerSession.ts:644-646) is out of scope — no plugin
// system exists in Go.
//
// A session owns one queue. TS keeps the queue on PlayerSession and
// passes the NPC object itself, capturing the index at queue time
// (NpcSpecificEffects.ts: queue reads npc.getIndex()); the session
// package must not import game, so the Go queue takes the identity and
// the index as separate values. Identity is the *game.NPC pointer, the
// same value the flush adapter stores in SyncNpcSnapshot.Identity, and
// must hold a comparable value (the appearance-cache contract in
// flush.go); the send loop compares identities with ==, mirroring TS
// object identity (Set.has on the NPC object, PlayerSession.ts:634-637),
// so a removed NPC's replacement at the same index inherits nothing.
//
// The TS send loop has four conjuncts (PlayerSession.ts:637-638):
// visible by object identity, registered, live index equals the queued
// index, and encoded this tick. The Go send loop implements three of
// them against the SyncView snapshots; the registered conjunct
// DEGENERATES and is deliberately absent — see sendNpcSpecificEffects
// for the evidence. No snapshot field is needed for it.
//
// Thread-safety: the queue takes no lock, like the other per-tick sync
// state on Session (see flush.go). Queue is called by future producers
// on the tick goroutine (game-layer code, as in TS where producers run
// on the event loop), Take and the send loop run in FlushTick on the
// tick goroutine, so all three share one goroutine. A producer on any
// other goroutine must add locking first.
//
// Coordinator integration (this file deliberately does not edit
// session.go or flush.go): add the field `npcSpecificEffects
// NpcSpecificEffects` to Session (zero value ready, no constructor
// change), add the QueueNpcEffect/takeNpcSpecificEffects Session
// wrappers delegating to it, call take as the FIRST statement of
// FlushTick (before the channel-open gate, as TS takes at :357 before
// the :359 return, so undeliverable one-shots are discarded even on a
// closed channel), and call the send loop after the NPC sync send with
// the SyncView NPC snapshots. TS clears the queue on player swap
// (PlayerSession.ts:241) and logout (:324); Go has no player-swap path
// (a Session is bound to one connection and never reused), so no Clear
// call site is needed — a closed session's queue dies with it.

// NpcSpecificEffect is one queued viewer-specific effect: the NPC it
// targets, that NPC's index captured at queue time, and the pre-encoded
// packet to send. It ports the pending entry shape in
// NpcSpecificEffects.ts ({ npc, index, packet }); the field order
// follows the task contract, not the TS literal.
type NpcSpecificEffect struct {
	// Identity is the NPC object (the *game.NPC pointer), compared
	// with ==. It must hold a comparable value; a nil identity never
	// matches, because TS queue() takes a non-null NPC and a nil
	// identity can never be a live NPC.
	Identity any
	// Index is the NPC's index at queue time (TS: npc.getIndex() in
	// queue, NpcSpecificEffects.ts). The send loop requires the
	// matching snapshot's index to still equal it.
	Index int
	// Packet is the pre-encoded packet bytes, copied on queue so a
	// producer mutating its buffer cannot corrupt a queued effect.
	// SendPacket copies again on send; taken packets are consumed
	// only by the send loop.
	Packet []byte
}

// NpcSpecificEffects is one session's transient effect queue, porting
// the NpcSpecificEffects class (NpcSpecificEffects.ts). The zero value
// is ready to use: Queue appends to a nil slice, Take on an empty queue
// returns nil.
type NpcSpecificEffects struct {
	pending []NpcSpecificEffect
}

// Queue appends an effect, capturing the index the caller supplies
// (TS captures npc.getIndex() itself; the session layer cannot call
// game methods, so the producer passes the index it encoded the
// packet for). The packet bytes are copied: TS stores the Buffer
// reference, but a Go producer reusing its buffer must not corrupt a
// queued effect. Callers must pass the NPC's live index; a stale index
// fails the send loop's index check and the effect is discarded.
func (q *NpcSpecificEffects) Queue(identity any, index int, packet []byte) {
	q.pending = append(q.pending, NpcSpecificEffect{
		Identity: identity,
		Index:    index,
		Packet:   append([]byte(nil), packet...),
	})
}

// Take drains the queue, returning the pending effects in queue order
// and leaving the queue empty (NpcSpecificEffects.ts: take swaps in a
// fresh array). FlushTick takes at the top of every flush, so effects
// that fail the send loop's checks are discarded, never retried.
func (q *NpcSpecificEffects) Take() []NpcSpecificEffect {
	pending := q.pending
	q.pending = nil
	return pending
}

// Clear discards all pending effects without returning them
// (NpcSpecificEffects.ts: clear). TS calls it on player swap and
// logout (PlayerSession.ts:241, 324); Go has no production caller yet
// (see the file header), but the method ports the class faithfully.
func (q *NpcSpecificEffects) Clear() {
	q.pending = nil
}

// sendNpcSpecificEffects ports the post-sync loop
// (PlayerSession.ts:632-641): for each taken effect, SendPacket iff the
// effect's identity is among this tick's NPC snapshots (== comparison),
// the matching snapshot's index equals the queued index, and the index
// is in s.npcSyncState.Indices — the indices EncodeNpcSync actually
// encoded this tick (sync.go updates Indices in place). The npcs
// argument is FlushTick's SyncView slice: the player's local NPCs, the
// same list TS builds visibleNpcs from (PlayerSession.ts:634).
//
// The TS registered conjunct (effect.npc.isRegistered(),
// PlayerSession.ts:637) is deliberately NOT implemented: it degenerates
// in Go. UpdateLocalNPCs requires npc.IsRegistered() AND slot occupancy
// in both its keep and add passes (game/visibility.go), removals drain
// in queue_remove_npcs strictly before the visibility and flush phases
// (game/world.go Process order), the only NPC deregistration path is
// that drain (w.npcs.Remove is called nowhere else), and SyncView is
// built inside FlushTick on the tick goroutine with no interleaving
// removal — so a snapshot can only name a registered NPC. The same
// argument holds in TS (World.ts updateLocalNpcs requires isRegistered
// in both passes, and queue_remove_npcs drains before
// update_players_npcs/flush_network_players): the TS check is defensive
// redundancy. Residual gap, documented honestly: if UpdateLocalNPCs
// panics mid-pass (possible only via a nil/corrupt list entry, which
// production paths cannot produce), the per-player guard recovers and
// the player is still flushed that tick with a possibly stale list; a
// live registered check would catch that case and this loop cannot.
// No producers exist, so the gap has no live effect this phase.
//
// Deviations from TS, both forced by the Go session shape:
//   - TS returns early from flushClient when sendClientPacket fails
//     (PlayerSession.ts:639). Go SendPacket returns nothing and drops
//     silently on a closed channel, so the loop runs to completion and
//     every send on a closed channel is a no-op — the same outcome TS
//     reaches, without the early exit.
//   - A nil npcSyncState (only a Session built without NewSession) is
//     treated as "nothing encoded this tick": no effect sends. TS
//     always constructs the state (PlayerSession.ts:125); the nil
//     check only keeps a zero-value Session from panicking.
func (s *Session) sendNpcSpecificEffects(effects []NpcSpecificEffect, npcs []SyncNpcSnapshot) {
	if len(effects) == 0 {
		return
	}
	// The indices actually encoded this tick (TS: new Set(
	// npcSyncState.indices), PlayerSession.ts:635). Built once; a
	// nil state yields the empty set.
	sentIndices := make(map[int]bool)
	if s.npcSyncState != nil {
		for _, index := range s.npcSyncState.Indices {
			sentIndices[index] = true
		}
	}
	for _, effect := range effects {
		// A nil identity can never be a live NPC (see
		// NpcSpecificEffect.Identity); the Indices check runs
		// before the snapshot scan as a cheap reject. Check
		// order is unobservable: the conjuncts have no side
		// effects and every combination is tested.
		if effect.Identity == nil || !sentIndices[effect.Index] {
			continue
		}
		for _, snap := range npcs {
			// Identity first (== mirrors TS object identity),
			// then the queued-index check: the snapshot's
			// index is the live index of the same object,
			// captured at SyncView time in this same
			// FlushTick call, so it equals what TS reads as
			// effect.npc.getIndex() at :638.
			if snap.Identity == effect.Identity && snap.Index == effect.Index {
				s.SendPacket(effect.Packet)
				break
			}
		}
	}
}

// QueueNpcEffect enqueues a viewer-specific effect for a future
// producer. The guard mirrors queueNpcSpecificEffect
// (PlayerSession.ts:128-130), channel-open half only: Session
// holds no player pointer, so the player-half of the TS guard has
// no Go equivalent (the send-loop gates are the real delivery
// check). Identity is the *game.NPC pointer; index is the NPC's
// live index at queue time. No producer calls this yet (see the
// file header); it is the honest seam future producers use, and
// the live smoke test drives effects through it.
func (s *Session) QueueNpcEffect(identity any, index int, packet []byte) {
	if !s.IsOpen() {
		return
	}
	s.npcSpecificEffects.Queue(identity, index, packet)
}

// takeNpcSpecificEffects drains the effect queue for FlushTick.
func (s *Session) takeNpcSpecificEffects() []NpcSpecificEffect {
	return s.npcSpecificEffects.Take()
}

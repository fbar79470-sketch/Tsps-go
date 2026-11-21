package session

import (
	"bytes"
	"testing"

	"tsps-go/internal/protocol"
)

// effectNPC is a stand-in NPC identity: distinct pointers, like the
// distinct *game.NPC values the flush adapter stores in
// SyncNpcSnapshot.Identity. The session package must not import game,
// so tests use these instead of real NPCs.
type effectNPC struct{ id int }

func TestNpcSpecificEffectsQueueTakeClear(t *testing.T) {
	a, b := &effectNPC{id: 1}, &effectNPC{id: 2}
	var q NpcSpecificEffects

	// Take on an empty queue returns nothing.
	if got := q.Take(); len(got) != 0 {
		t.Fatalf("Take on empty queue = %d effects, want 0", len(got))
	}

	q.Queue(a, 5, []byte{0xAA, 0x01})
	q.Queue(b, 7, []byte{0xBB, 0x02})

	// Take drains in queue order, preserving identity/index/packet.
	got := q.Take()
	if len(got) != 2 {
		t.Fatalf("Take = %d effects, want 2", len(got))
	}
	if got[0].Identity != any(a) || got[0].Index != 5 || !bytes.Equal(got[0].Packet, []byte{0xAA, 0x01}) {
		t.Errorf("effect 0 = %+v, want identity a, index 5, packet [AA 01]", got[0])
	}
	if got[1].Identity != any(b) || got[1].Index != 7 || !bytes.Equal(got[1].Packet, []byte{0xBB, 0x02}) {
		t.Errorf("effect 1 = %+v, want identity b, index 7, packet [BB 02]", got[1])
	}

	// Take drains: a second Take is empty, and queueing after a Take
	// starts a fresh queue (the taken slice is not aliased).
	if got := q.Take(); len(got) != 0 {
		t.Fatalf("second Take = %d effects, want 0 (Take must drain)", len(got))
	}
	q.Queue(a, 5, []byte{0xCC})
	if got := q.Take(); len(got) != 1 || !bytes.Equal(got[0].Packet, []byte{0xCC}) {
		t.Fatalf("Take after re-queue = %+v, want 1 effect [CC]", got)
	}

	// Clear discards without returning.
	q.Queue(a, 5, []byte{0xDD})
	q.Queue(b, 7, []byte{0xEE})
	q.Clear()
	if got := q.Take(); len(got) != 0 {
		t.Fatalf("Take after Clear = %d effects, want 0", len(got))
	}
	// Clear on an empty queue is a no-op, not a panic.
	q.Clear()
}

func TestNpcSpecificEffectsPacketBytesCopied(t *testing.T) {
	a := &effectNPC{id: 1}
	var q NpcSpecificEffects

	// Mutating the producer's buffer after queueing must not corrupt
	// the queued effect.
	buf := []byte{0x01, 0x02, 0x03}
	q.Queue(a, 5, buf)
	buf[0] = 0xFF
	buf[1] = 0xFF
	got := q.Take()
	if len(got) != 1 || !bytes.Equal(got[0].Packet, []byte{0x01, 0x02, 0x03}) {
		t.Fatalf("queued packet = %v, want [01 02 03] (bytes must be copied on queue)", got)
	}

	// Queueing the same backing array twice with a mutation between
	// yields two independent snapshots.
	q.Queue(a, 5, buf)
	buf[0] = 0x09
	q.Queue(a, 5, buf)
	got = q.Take()
	if len(got) != 2 {
		t.Fatalf("Take = %d effects, want 2", len(got))
	}
	if !bytes.Equal(got[0].Packet, []byte{0xFF, 0xFF, 0x03}) || !bytes.Equal(got[1].Packet, []byte{0x09, 0xFF, 0x03}) {
		t.Errorf("packets = %v, %v: queue must copy, not alias", got[0].Packet, got[1].Packet)
	}

	// A nil packet queues without panic and takes back empty.
	q.Queue(a, 5, nil)
	got = q.Take()
	if len(got) != 1 || len(got[0].Packet) != 0 {
		t.Fatalf("nil packet Take = %+v, want 1 effect with empty packet", got)
	}
}

// sendEffects runs the send loop on a fresh session and returns the
// bytes the channel received after a Flush: the concatenated effect
// packets that passed all checks, in queue order.
func sendEffects(t *testing.T, indices []int, effects []NpcSpecificEffect, npcs []SyncNpcSnapshot) []byte {
	t.Helper()
	ch := newFakeChannel()
	s := NewSession(ch)
	s.npcSyncState = protocol.CreateNpcSyncState()
	s.npcSyncState.Indices = indices
	s.sendNpcSpecificEffects(effects, npcs)
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	frames := ch.sentBytes()
	if len(frames) == 0 {
		return nil
	}
	if len(frames) != 1 {
		t.Fatalf("sent %d frames, want at most 1 (single flush concat)", len(frames))
	}
	return frames[0]
}

func TestSendNpcSpecificEffects(t *testing.T) {
	a, b := &effectNPC{id: 1}, &effectNPC{id: 2}
	npcA := SyncNpcSnapshot{Identity: a, Index: 5, TypeID: 100, X: 3200, Y: 3200}
	npcB := SyncNpcSnapshot{Identity: b, Index: 5, TypeID: 200, X: 3201, Y: 3200}

	tests := []struct {
		name    string
		indices []int
		effects []NpcSpecificEffect
		npcs    []SyncNpcSnapshot
		want    []byte
	}{
		{
			name:    "visible identity, matching index, encoded this tick sends",
			indices: []int{5},
			effects: []NpcSpecificEffect{{Identity: a, Index: 5, Packet: []byte{0xAA}}},
			npcs:    []SyncNpcSnapshot{npcA},
			want:    []byte{0xAA},
		},
		{
			// The TS comment at PlayerSession.ts:633: "a removed
			// NPC's replacement inherits nothing." Same index,
			// different object identity: nothing sends.
			name:    "replacement NPC at same index inherits nothing",
			indices: []int{5},
			effects: []NpcSpecificEffect{{Identity: a, Index: 5, Packet: []byte{0xAA}}},
			npcs:    []SyncNpcSnapshot{npcB},
			want:    nil,
		},
		{
			name:    "NPC not in view sends nothing",
			indices: []int{5},
			effects: []NpcSpecificEffect{{Identity: a, Index: 5, Packet: []byte{0xAA}}},
			npcs:    []SyncNpcSnapshot{npcB},
			want:    nil,
		},
		{
			name:    "empty view sends nothing",
			indices: []int{5},
			effects: []NpcSpecificEffect{{Identity: a, Index: 5, Packet: []byte{0xAA}}},
			npcs:    nil,
			want:    nil,
		},
		{
			// The NPC is in view but was not encoded this tick
			// (culled by the 255 cap or the encoder): the
			// Indices check drops the effect.
			name:    "index not encoded this tick sends nothing",
			indices: []int{9},
			effects: []NpcSpecificEffect{{Identity: a, Index: 5, Packet: []byte{0xAA}}},
			npcs:    []SyncNpcSnapshot{npcA},
			want:    nil,
		},
		{
			name:    "empty Indices sends nothing",
			indices: []int{},
			effects: []NpcSpecificEffect{{Identity: a, Index: 5, Packet: []byte{0xAA}}},
			npcs:    []SyncNpcSnapshot{npcA},
			want:    nil,
		},
		{
			// Same object, but its live index no longer equals
			// the queued index (removed and re-added at a new
			// slot): the index check drops the effect.
			name:    "index mismatch sends nothing",
			indices: []int{5, 7},
			effects: []NpcSpecificEffect{{Identity: a, Index: 5, Packet: []byte{0xAA}}},
			npcs:    []SyncNpcSnapshot{{Identity: a, Index: 7, TypeID: 100}},
			want:    nil,
		},
		{
			name:    "nil identity never matches",
			indices: []int{5},
			effects: []NpcSpecificEffect{{Identity: nil, Index: 5, Packet: []byte{0xAA}}},
			npcs:    []SyncNpcSnapshot{{Identity: nil, Index: 5, TypeID: 100}, npcA},
			want:    nil,
		},
		{
			name:    "empty effects is a no-op",
			indices: []int{5},
			effects: nil,
			npcs:    []SyncNpcSnapshot{npcA},
			want:    nil,
		},
		{
			// Only the passing effects send, in queue order,
			// including two effects for the same NPC.
			name:    "mixed effects send only the passing ones in order",
			indices: []int{5},
			effects: []NpcSpecificEffect{
				{Identity: a, Index: 5, Packet: []byte{0x01}},
				{Identity: b, Index: 5, Packet: []byte{0x02}},
				{Identity: a, Index: 9, Packet: []byte{0x03}},
				{Identity: a, Index: 5, Packet: []byte{0x04}},
			},
			npcs: []SyncNpcSnapshot{npcA},
			want: []byte{0x01, 0x04},
		},
		{
			// The queued index must be in Indices even when the
			// snapshot index differs from a *different*
			// encoded index: membership is checked on the
			// effect's index, not the snapshot's.
			name:    "Indices membership uses the effect index",
			indices: []int{7},
			effects: []NpcSpecificEffect{{Identity: a, Index: 5, Packet: []byte{0xAA}}},
			npcs:    []SyncNpcSnapshot{{Identity: a, Index: 7, TypeID: 100}},
			want:    nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sendEffects(t, tc.indices, tc.effects, tc.npcs); !bytes.Equal(got, tc.want) {
				t.Errorf("sent = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSendNpcSpecificEffectsNilSyncState(t *testing.T) {
	// A nil npcSyncState (only a Session built without NewSession)
	// means nothing was encoded: no effect sends, and the loop must
	// not panic.
	a := &effectNPC{id: 1}
	ch := newFakeChannel()
	s := NewSession(ch)
	s.npcSyncState = nil
	s.sendNpcSpecificEffects(
		[]NpcSpecificEffect{{Identity: a, Index: 5, Packet: []byte{0xAA}}},
		[]SyncNpcSnapshot{{Identity: a, Index: 5}},
	)
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if got := ch.sentBytes(); len(got) != 0 {
		t.Fatalf("sent %d frames with nil sync state, want 0", len(got))
	}
}

func TestSendNpcSpecificEffectsClosedChannel(t *testing.T) {
	// On a closed channel SendPacket drops silently, so the loop
	// completes without sending and without panicking (TS returns
	// early on send failure; Go has no failure signal to check).
	a := &effectNPC{id: 1}
	ch := newFakeChannel()
	s := NewSession(ch)
	s.npcSyncState = protocol.CreateNpcSyncState()
	s.npcSyncState.Indices = []int{5}
	s.Close(1000, "logout")
	s.sendNpcSpecificEffects(
		[]NpcSpecificEffect{{Identity: a, Index: 5, Packet: []byte{0xAA}}},
		[]SyncNpcSnapshot{{Identity: a, Index: 5}},
	)
	if got := ch.sentBytes(); len(got) != 0 {
		t.Fatalf("sent %d frames on closed channel, want 0", len(got))
	}
}

func TestNpcSpecificEffectsTakeFeedsSendLoop(t *testing.T) {
	// The FlushTick shape: Take at the top, send loop after the NPC
	// sync send. An effect taken on one flush never survives to the
	// next: take drains even when the send loop drops it.
	a := &effectNPC{id: 1}
	var q NpcSpecificEffects
	q.Queue(a, 5, []byte{0xAA})

	// First flush: the NPC is not in view, so the taken effect is
	// discarded by the loop.
	if got := sendEffects(t, []int{5}, q.Take(), nil); len(got) != 0 {
		t.Fatalf("first flush sent %v, want nothing (NPC not in view)", got)
	}
	// Second flush: the queue is empty — the dropped effect is not
	// retried now that the NPC is visible.
	if got := sendEffects(t, []int{5}, q.Take(), []SyncNpcSnapshot{{Identity: a, Index: 5}}); len(got) != 0 {
		t.Fatalf("second flush sent %v, want nothing (Take must drain)", got)
	}
}

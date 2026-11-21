package session

import (
	"encoding/binary"
	"testing"

	"tsps-go/internal/protocol"
)

// groundPacketSerial reads the serial from a ground-items payload
// (int32 at offset 0, per ClientProtocol.ts:1510-1528).
func groundPacketSerial(t *testing.T, payload []byte) int32 {
	t.Helper()
	if len(payload) < 6 {
		t.Fatalf("ground packet payload too short: %d", len(payload))
	}
	return int32(binary.BigEndian.Uint32(payload[0:4]))
}

// groundPacketUpsertCount reads the stack/upsert count (u16 at
// offset 4).
func groundPacketUpsertCount(t *testing.T, payload []byte) int {
	t.Helper()
	if len(payload) < 6 {
		t.Fatalf("ground packet payload too short: %d", len(payload))
	}
	return int(binary.BigEndian.Uint16(payload[4:6]))
}

// groundPacketRemoveCount reads the remove count from a delta
// payload: the u16 after the upsert records.
func groundPacketRemoveCount(t *testing.T, payload []byte, upserts int) int {
	t.Helper()
	off := 6 + upserts*protocol.GroundItemBytes
	if len(payload) < off+2 {
		t.Fatalf("delta payload too short: %d", len(payload))
	}
	return int(binary.BigEndian.Uint16(payload[off : off+2]))
}

func groundTestStacks(n int) []protocol.GroundItemView {
	stacks := make([]protocol.GroundItemView, n)
	for i := range stacks {
		stacks[i] = protocol.GroundItemView{ID: i + 1, ItemID: 995, Quantity: 1, X: 3200, Y: 3200, EnabledOps: 0x1f}
	}
	return stacks
}

func TestGroundItemsPerPacketValue(t *testing.T) {
	// floor((65536 - 16) / 34) = 1927.
	if GroundItemsPerPacket != 1927 {
		t.Fatalf("GroundItemsPerPacket=%d, want 1927", GroundItemsPerPacket)
	}
}

func TestGroundItemSyncEmptySnapshotStillSends(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	g := NewGroundItemSync(s)
	g.SendGroundItems(nil)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	packets := flushFramePackets(t, ch)
	if len(packets) != 1 || packets[0].Opcode != protocol.ServerGroundItems {
		t.Fatalf("packets=%v, want 1 GROUND_ITEMS", flushOpcodes(packets))
	}
	if groundPacketSerial(t, packets[0].Payload) != 1 {
		t.Fatalf("serial=%d, want 1 (first packet)", groundPacketSerial(t, packets[0].Payload))
	}
	if groundPacketUpsertCount(t, packets[0].Payload) != 0 {
		t.Fatalf("count=%d, want 0", groundPacketUpsertCount(t, packets[0].Payload))
	}
}

func TestGroundItemSyncChunking(t *testing.T) {
	cases := []struct {
		name   string
		stacks int
		want   []struct {
			opcode byte
			serial int32
			count  int
		}
	}{
		{"small", 3, []struct {
			opcode byte
			serial int32
			count  int
		}{{protocol.ServerGroundItems, 1, 3}}},
		{"exact one packet", GroundItemsPerPacket, []struct {
			opcode byte
			serial int32
			count  int
		}{{protocol.ServerGroundItems, 1, GroundItemsPerPacket}}},
		{"one over", GroundItemsPerPacket + 1, []struct {
			opcode byte
			serial int32
			count  int
		}{
			{protocol.ServerGroundItems, 1, GroundItemsPerPacket},
			{protocol.ServerGroundItemsDelta, 2, 1},
		}},
		{"two full plus remainder", 2*GroundItemsPerPacket + 5, []struct {
			opcode byte
			serial int32
			count  int
		}{
			{protocol.ServerGroundItems, 1, GroundItemsPerPacket},
			{protocol.ServerGroundItemsDelta, 2, GroundItemsPerPacket},
			{protocol.ServerGroundItemsDelta, 3, 5},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ch := newFakeChannel()
			s := NewSession(ch)
			g := NewGroundItemSync(s)
			g.SendGroundItems(groundTestStacks(tc.stacks))
			if err := s.Flush(); err != nil {
				t.Fatal(err)
			}
			packets := flushFramePackets(t, ch)
			if len(packets) != len(tc.want) {
				t.Fatalf("packets=%d, want %d", len(packets), len(tc.want))
			}
			for i, w := range tc.want {
				p := packets[i]
				if p.Opcode != w.opcode {
					t.Errorf("packet %d opcode=%d, want %d", i, p.Opcode, w.opcode)
				}
				if got := groundPacketSerial(t, p.Payload); got != w.serial {
					t.Errorf("packet %d serial=%d, want %d", i, got, w.serial)
				}
				if got := groundPacketUpsertCount(t, p.Payload); got != w.count {
					t.Errorf("packet %d count=%d, want %d", i, got, w.count)
				}
				if w.opcode == protocol.ServerGroundItemsDelta {
					if got := groundPacketRemoveCount(t, p.Payload, w.count); got != 0 {
						t.Errorf("packet %d removes=%d, want 0", i, got)
					}
				}
			}
		})
	}
}

func TestGroundItemSyncDeltaSerialSequence(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	g := NewGroundItemSync(s)
	stack := protocol.GroundItemView{ID: 7, ItemID: 995, Quantity: 2, EnabledOps: 0x1f}
	g.SendGroundItemDelta([]protocol.GroundItemView{stack}, nil)
	g.SendGroundItemDelta(nil, []int{7})
	g.SendGroundItems(groundTestStacks(2))
	g.SendGroundItemDelta([]protocol.GroundItemView{stack}, nil)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	packets := flushFramePackets(t, ch)
	if len(packets) != 4 {
		t.Fatalf("packets=%d, want 4", len(packets))
	}
	wantOpcodes := []byte{protocol.ServerGroundItemsDelta, protocol.ServerGroundItemsDelta, protocol.ServerGroundItems, protocol.ServerGroundItemsDelta}
	for i, want := range wantOpcodes {
		if packets[i].Opcode != want {
			t.Errorf("packet %d opcode=%d, want %d", i, packets[i].Opcode, want)
		}
		if got := groundPacketSerial(t, packets[i].Payload); got != int32(i+1) {
			t.Errorf("packet %d serial=%d, want %d", i, got, i+1)
		}
	}
	// The DELETE delta carries one remove and no upserts.
	if got := groundPacketUpsertCount(t, packets[1].Payload); got != 0 {
		t.Errorf("delete upserts=%d, want 0", got)
	}
	if got := groundPacketRemoveCount(t, packets[1].Payload, 0); got != 1 {
		t.Errorf("delete removes=%d, want 1", got)
	}
	if id := binary.BigEndian.Uint32(packets[1].Payload[8:12]); id != 7 {
		t.Errorf("delete id=%d, want 7", id)
	}
	// The CREATE delta carries the upsert record.
	if got := groundPacketUpsertCount(t, packets[0].Payload); got != 1 {
		t.Errorf("create upserts=%d, want 1", got)
	}
	if id := binary.BigEndian.Uint32(packets[0].Payload[6:10]); id != 7 {
		t.Errorf("create id=%d, want 7", id)
	}
}

func TestGroundItemSyncSource(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	g := NewGroundItemSync(s)
	if g.GroundItemSource() != nil {
		t.Fatal("source set before install")
	}
	// No source: region-change send is a no-op.
	g.SendVisibleGroundItems()
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if len(flushFramePackets(t, ch)) != 0 {
		t.Fatal("packets sent with no source")
	}
	g.SetGroundItemSource(func() []protocol.GroundItemView { return groundTestStacks(2) })
	if g.GroundItemSource() == nil {
		t.Fatal("source nil after install")
	}
	g.SendVisibleGroundItems()
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	packets := flushFramePackets(t, ch)
	if len(packets) != 1 || groundPacketUpsertCount(t, packets[0].Payload) != 2 {
		t.Fatalf("packets=%d, want 1 with 2 stacks", len(packets))
	}
	// Clearing the source stops sends again.
	g.SetGroundItemSource(nil)
	g.SendVisibleGroundItems()
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if len(flushFramePackets(t, ch)) != 1 {
		t.Fatal("packets sent after source cleared")
	}
}

func TestGroundItemSyncNilSession(t *testing.T) {
	g := NewGroundItemSync(nil)
	// Must not panic; nothing is sent anywhere.
	g.SendGroundItems(groundTestStacks(1))
	g.SendGroundItemDelta(nil, []int{1})
	g.SendVisibleGroundItems()
	var nilSync *GroundItemSync
	nilSync.SendGroundItems(nil)
	nilSync.SendGroundItemDelta(nil, nil)
	nilSync.SendVisibleGroundItems()
	nilSync.SetGroundItemSource(nil)
	if nilSync.GroundItemSource() != nil {
		t.Fatal("nil sync has a source")
	}
}

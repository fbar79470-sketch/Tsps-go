package session

import (
	"bytes"
	"testing"

	"tsps-go/internal/protocol"
)

// Tests for the options-form sub-interface mount and the bank
// side-ops helper (phase 46, Worker A).

func TestMountSubInterfaceWithOptions(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	options := protocol.WidgetOpenSubOptions{
		Varps:   map[int]int{548: 1, 262: 5, 4611: 1400},
		Varbits: map[int]int{3755: 1, 3958: 0, 6590: 2},
	}
	targetUID := (161 << 16) | 16
	mount := s.MountSubInterfaceWithOptions(targetUID, 12, 0, options)
	if mount == nil || mount.GroupID != 12 || mount.TargetUID != targetUID || mount.Type != 0 {
		t.Fatalf("mount = %+v, want group 12 at 161:16 type 0", mount)
	}
	if got, ok := s.GetSubInterfaceAt(targetUID); !ok || got != mount {
		t.Fatalf("tracked mount = %+v, %v, want the returned mount", got, ok)
	}
	packets := queuedPackets(t, s)
	want := protocol.EncodeWidgetOpenSub(targetUID, 12, 0, options)
	if len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Fatalf("queued = %x, want [%x]", packets, want)
	}
}

func TestMountSubInterfaceSendsEmptyOptions(t *testing.T) {
	// The no-options form is exactly the options form with
	// empty options: same frame, same tracking.
	s := NewSession(NewHeadlessChannel(""))
	targetUID := (161 << 16) | 16
	s.MountSubInterface(targetUID, 12, 0)
	packets := queuedPackets(t, s)
	want := protocol.EncodeWidgetOpenSub(targetUID, 12, 0, protocol.WidgetOpenSubOptions{})
	if len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Fatalf("queued = %x, want [%x]", packets, want)
	}
}

func TestSendSideOps(t *testing.T) {
	// Slots 0 and 2 hold wearable items (ids 1 and 3);
	// slot 1 holds an unwearable item; slot 3 is empty.
	ids := []int{1, 2, 3, -1}
	wearable := func(id int) bool { return id == 1 || id == 3 }
	s := NewSession(NewHeadlessChannel(""))
	s.SendSideOps(ids, wearable)
	if got := s.GetVarp(262); got != 0b0101 {
		t.Fatalf("varp 262 = %d, want 5 (bits 0 and 2)", got)
	}
	packets := queuedPackets(t, s)
	want := protocol.EncodeVarp(262, 0b0101)
	if len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Fatalf("queued = %x, want [%x]", packets, want)
	}
}

func TestSideWearOpsMask(t *testing.T) {
	wearable := func(id int) bool { return id%2 == 1 }
	if got := sideWearOpsMask(nil, wearable); got != 0 {
		t.Fatalf("nil ids mask = %d, want 0", got)
	}
	if got := sideWearOpsMask([]int{1, 2, 3}, nil); got != 0 {
		t.Fatalf("nil predicate mask = %d, want 0", got)
	}
	if got := sideWearOpsMask([]int{1, 2, 3, 4}, wearable); got != 0b0101 {
		t.Fatalf("mask = %d, want 5", got)
	}
	// Slot 27 (the last inventory slot) can set a bit;
	// anything past it is ignored.
	ids := make([]int, 30)
	ids[27] = 1
	ids[28] = 1
	ids[29] = 1
	if got := sideWearOpsMask(ids, wearable); got != 1<<27 {
		t.Fatalf("mask = %d, want %d (only slot 27)", got, 1<<27)
	}
	// Negative ids (empty slots) never set a bit.
	if got := sideWearOpsMask([]int{-1, -1}, wearable); got != 0 {
		t.Fatalf("empty-slot mask = %d, want 0", got)
	}
}

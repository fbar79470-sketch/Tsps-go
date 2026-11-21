package game

import "testing"

func TestPlayerTurnPacketCap(t *testing.T) {
	p := NewPlayer("alice", Location{})
	for i := 0; i < PacketQueueLimit; i++ {
		if !p.QueueTurnPacket(TurnPacket{Opcode: byte(i)}) {
			t.Fatalf("packet %d must queue (cap %d)", i, PacketQueueLimit)
		}
	}
	if p.QueueTurnPacket(TurnPacket{Opcode: 99}) {
		t.Fatalf("packet %d must be rejected at cap %d", PacketQueueLimit+1, PacketQueueLimit)
	}
	if p.TurnPacketCount() != PacketQueueLimit {
		t.Fatalf("count = %d, want %d", p.TurnPacketCount(), PacketQueueLimit)
	}
	drained := p.DrainTurnPackets()
	if len(drained) != PacketQueueLimit || drained[0].Opcode != 0 || drained[PacketQueueLimit-1].Opcode != byte(PacketQueueLimit-1) {
		t.Fatal("drain must return all packets in FIFO order")
	}
	if p.TurnPacketCount() != 0 || p.DrainTurnPackets() != nil {
		t.Fatal("drain must empty the queue")
	}
	// Queue usable again after drain.
	if !p.QueueTurnPacket(TurnPacket{Opcode: 1}) {
		t.Fatal("queue must accept packets after drain")
	}
}

func TestPlayerProcessDrainsViaHandler(t *testing.T) {
	p := NewPlayer("bob", Location{})
	var seen []TurnPacket
	var cycles []int64
	p.TurnHandler = func(pl *Player, pkt TurnPacket, cycle int64) {
		seen = append(seen, pkt)
		cycles = append(cycles, cycle)
	}
	p.QueueTurnPacket(TurnPacket{Opcode: 3, Payload: []byte{1}})
	p.QueueTurnPacket(TurnPacket{Opcode: 4, Payload: []byte{2}})
	p.Process(42)
	if len(seen) != 2 || seen[0].Opcode != 3 || seen[1].Opcode != 4 {
		t.Fatalf("handler saw %v, want opcodes 3,4", seen)
	}
	if cycles[0] != 42 || cycles[1] != 42 {
		t.Fatal("handler must receive the process cycle")
	}
	if p.LastInputCycle != 42 {
		t.Fatalf("LastInputCycle = %d, want 42", p.LastInputCycle)
	}
	if p.TurnPacketCount() != 0 {
		t.Fatal("Process must drain the queue")
	}
	// A turn with no packets leaves LastInputCycle untouched.
	p.Process(43)
	if p.LastInputCycle != 42 {
		t.Fatal("empty turn must not refresh LastInputCycle")
	}
}

func TestPlayerProcessWithoutHandlerDiscards(t *testing.T) {
	p := NewPlayer("carol", Location{})
	p.QueueTurnPacket(TurnPacket{Opcode: 1})
	p.Process(5)
	if p.TurnPacketCount() != 0 {
		t.Fatal("packets must be drained even with no handler")
	}
}

func TestPlayerDisplayNameFallback(t *testing.T) {
	p := NewPlayer("eve", Location{})
	if p.GetDisplayName() != "eve" {
		t.Fatal("empty display name must fall back to username")
	}
	p.DisplayName = "Eve The Great"
	if p.GetDisplayName() != "Eve The Great" {
		t.Fatal("display name must win when set")
	}
}

func TestPlayerReadyToLeaveWorld(t *testing.T) {
	// No combat hook: leave at once.
	p := NewPlayer("frank", Location{})
	if !p.ReadyToLeaveWorld(10) {
		t.Fatal("no combat hold must leave immediately")
	}
	// Permanent combat hold: stay 100 cycles from first observation.
	q := NewPlayer("grace", Location{})
	q.InCombatHold = func(cycle int64) bool { return true }
	if q.ReadyToLeaveWorld(100) {
		t.Fatal("combat hold must not release on first observation")
	}
	if q.ReadyToLeaveWorld(199) {
		t.Fatal("combat hold must not release before 100 cycles")
	}
	if !q.ReadyToLeaveWorld(200) {
		t.Fatal("combat hold must release at 100 cycles (100..200)")
	}
	// Hold ends when combat ends.
	r := NewPlayer("heidi", Location{})
	inCombat := true
	r.InCombatHold = func(cycle int64) bool { return inCombat }
	if r.ReadyToLeaveWorld(50) {
		t.Fatal("must hold while in combat")
	}
	inCombat = false
	if !r.ReadyToLeaveWorld(51) {
		t.Fatal("must release once combat ends")
	}
}

func TestPlayerDisconnectSourceFirstWins(t *testing.T) {
	p := NewPlayer("ivan", Location{})
	p.SetDisconnectSource("logout")
	p.SetDisconnectSource("removed")
	if p.DisconnectSource() != "logout" {
		t.Fatal("first disconnect source must win")
	}
}

func TestNPCFoundation(t *testing.T) {
	loc := Location{X: 5, Y: 6, Z: 0}
	n := NewNPC(1234, loc)
	if n.ID != 1234 || !n.Spawn.Equals(loc) || !n.Loc.Equals(loc) {
		t.Fatal("NewNPC must set id, spawn, location")
	}
	if n.Hitpoints != 0 {
		t.Fatal("foundation hitpoints must stay unset (0) until definitions exist")
	}
	var ran []int64
	n.ProcessHook = func(npc *NPC, cycle int64) { ran = append(ran, cycle) }
	n.Process(3)
	if len(ran) != 1 || ran[0] != 3 {
		t.Fatal("NPC Process must run the hook with the cycle")
	}
	n.Flags = n.Flags.Set(FlagGraphic)
	n.ResetUpdating()
	if n.Flags.IsUpdateRequired() {
		t.Fatal("NPC ResetUpdating must clear flags")
	}
}

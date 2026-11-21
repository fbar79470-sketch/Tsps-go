package game

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
)

func newTestWorld() *World {
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	w.SeedRand(42)
	return w
}

func registerPlayer(t *testing.T, w *World, name string) *Player {
	t.Helper()
	p := NewPlayer(name, w.Spawn())
	p.Session = newFakeSession()
	w.QueueAddPlayer(p)
	w.Process()
	if !p.IsRegistered() {
		t.Fatalf("player %s was not registered", name)
	}
	return p
}

func TestWorldCycleIncrementsMasked(t *testing.T) {
	w := newTestWorld()
	if w.ProcessCycle() != 0 {
		t.Fatal("cycle must start at 0")
	}
	w.Process()
	w.Process()
	if w.ProcessCycle() != 2 {
		t.Fatalf("cycle = %d, want 2", w.ProcessCycle())
	}
}

func TestWorldPhaseOrder(t *testing.T) {
	w := newTestWorld()
	var mu sync.Mutex
	var events []string
	record := func(name string) {
		mu.Lock()
		events = append(events, name)
		mu.Unlock()
	}
	w.Hooks.GroundItems = func(cycle int64) { record("ground_items") }
	w.Hooks.NPCActivation = func(cycle int64) { record("npc_activation") }
	w.Hooks.Boats = func(cycle int64) { record("move_boats") }
	w.Hooks.CombatHits = func(cycle int64) { record("combat_hits") }
	w.Hooks.VisibilityBuckets = func(cycle int64) { record("rebuild_update_buckets") }
	w.Hooks.UpdatePlayersNPCs = func(cycle int64) { record("update_players_npcs") }
	w.Hooks.WalkToSweep = func(cycle int64) { record("walk_to_sweep") }

	// task_manager: a delay-0 task executes on the first Process.
	w.Tasks().Schedule(NewTask(0, func(cycle int64) bool { record("task_manager"); return false }))

	n := NewNPC(1, w.Spawn())
	n.ProcessHook = func(npc *NPC, cycle int64) { record("process_npcs") }
	w.QueueAddNPC(n)
	w.Process() // registers the NPC; its first mechanics turn waits a tick.
	events = nil

	p := NewPlayer("alice", w.Spawn())
	sess := newFakeSession()
	p.Session = sess
	// Observe the process_players phase through the turn handler:
	// the drained packet runs inside Player.Process, after
	// registration, in the same phase the timer hooks used to
	// observe (TimerHooks was removed with the timer port).
	p.TurnHandler = func(pl *Player, pkt TurnPacket, cycle int64) {
		record("process_players")
		// Queue phases must already have run: the player is registered.
		if !p.IsRegistered() {
			t.Error("player must be registered by the process_players phase")
		}
		p.Flags = p.Flags.Set(FlagAppearance)
	}
	p.QueueTurnPacket(TurnPacket{Opcode: 1})
	// Wrap FlushTick recording via a session that records through the hook:
	// fakeSession records flushes; assert order separately below by
	// checking the flush happened (flush phase is after update phases,
	// proven by the flag still being set at flush time is not observable
	// here, so record via a delegating session instead).
	recording := &recordingSession{inner: sess, onFlush: func() { record("flush_network_players") }}
	p.Session = recording
	w.QueueAddPlayer(p)

	w.Process()

	wantOrder := []string{
		"task_manager", "ground_items", "npc_activation", "process_npcs",
		"process_players", "move_boats", "walk_to_sweep", "combat_hits",
		"rebuild_update_buckets", "update_players_npcs", "flush_network_players",
	}
	if !reflect.DeepEqual(events, wantOrder) {
		t.Fatalf("phase order = %v\nwant %v", events, wantOrder)
	}
	// Reset phase: flags set during the turn must be cleared.
	if p.Flags.IsUpdateRequired() {
		t.Fatal("reset_player_updates must clear flags after flush")
	}
	if w.ProcessCycle() != 2 {
		t.Fatalf("cycle = %d, want 2", w.ProcessCycle())
	}
}

// recordingSession delegates to a fakeSession and records flush calls.
type recordingSession struct {
	inner   *fakeSession
	onFlush func()
}

func (s *recordingSession) SendPacket(b []byte) { s.inner.SendPacket(b) }
func (s *recordingSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
	s.inner.SendRunEnergy(units, running, weightKg, staminaTicks)
}
func (s *recordingSession) SendDestination(x, y int) { s.inner.SendDestination(x, y) }
func (s *recordingSession) SendVarp(id, value int)   { s.inner.SendVarp(id, value) }
func (s *recordingSession) FlushTick(c int64) error {
	s.onFlush()
	return s.inner.FlushTick(c)
}
func (s *recordingSession) Close(code int, reason string) { s.inner.Close(code, reason) }
func (s *recordingSession) IsOpen() bool                  { return s.inner.IsOpen() }

func TestWorldQueueDrainCaps(t *testing.T) {
	w := newTestWorld()
	for i := 0; i < 200; i++ {
		w.QueueAddPlayer(NewPlayer(fmt.Sprintf("p%d", i), w.Spawn()))
	}
	w.Process()
	if w.Players().Len() != QueueDrainCap {
		t.Fatalf("after 1 cycle players = %d, want %d", w.Players().Len(), QueueDrainCap)
	}
	w.Process()
	if w.Players().Len() != 200 {
		t.Fatalf("after 2 cycles players = %d, want 200", w.Players().Len())
	}

	// Remove drain cap: queue all 200 removals; 150 leave per cycle.
	for _, p := range w.Players().All() {
		w.QueueRemovePlayer(p.Index)
	}
	w.Process()
	if w.Players().Len() != 50 {
		t.Fatalf("after remove cycle players = %d, want 50", w.Players().Len())
	}
	w.Process()
	if w.Players().Len() != 0 {
		t.Fatalf("after 2nd remove cycle players = %d, want 0", w.Players().Len())
	}

	// NPC queues share the cap.
	for i := 0; i < 200; i++ {
		w.QueueAddNPC(NewNPC(i, w.Spawn()))
	}
	w.Process()
	if w.NPCs().Len() != QueueDrainCap {
		t.Fatalf("after 1 cycle npcs = %d, want %d", w.NPCs().Len(), QueueDrainCap)
	}
}

func TestWorldRemovePlayersBackwards(t *testing.T) {
	w := newTestWorld()
	var order []string
	w.Hooks.OnPlayerDisconnect = func(p *Player, source string) { order = append(order, p.Username) }
	a := registerPlayer(t, w, "a")
	b := registerPlayer(t, w, "b")
	c := registerPlayer(t, w, "c")
	// Queue in forward order; the drain iterates backwards.
	w.QueueRemovePlayer(a.Index)
	w.QueueRemovePlayer(b.Index)
	w.QueueRemovePlayer(c.Index)
	w.Process()
	want := []string{"c", "b", "a"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("disconnect order = %v, want %v (backwards drain)", order, want)
	}
	if w.Players().Len() != 0 {
		t.Fatal("all queued players must be removed")
	}
}

func TestWorldCombatHoldDelaysRemoval(t *testing.T) {
	w := newTestWorld()
	p := registerPlayer(t, w, "held")
	p.InCombatHold = func(cycle int64) bool { return true }
	w.QueueRemovePlayer(p.Index)
	for i := 0; i < 50; i++ {
		w.Process()
	}
	if !p.IsRegistered() {
		t.Fatal("combat-held player must stay in the world")
	}
	for i := 0; i < 60; i++ {
		w.Process()
	}
	if p.IsRegistered() {
		t.Fatal("combat-held player must leave after the 100-cycle hold")
	}
	// Updating forces removal regardless of the hold.
	q := registerPlayer(t, w, "held2")
	q.InCombatHold = func(cycle int64) bool { return true }
	w.QueueRemovePlayer(q.Index)
	w.Updating = true
	w.Process()
	if q.IsRegistered() {
		t.Fatal("Updating must force removal despite the combat hold")
	}
}

func TestWorldNPCRegisteredThisTickWaits(t *testing.T) {
	w := newTestWorld()
	ran := 0
	n := NewNPC(5, w.Spawn())
	n.ProcessHook = func(npc *NPC, cycle int64) { ran++ }
	w.QueueAddNPC(n)
	w.Process()
	if ran != 0 {
		t.Fatal("NPC registered this tick must wait until next tick")
	}
	w.Process()
	if ran != 1 {
		t.Fatal("NPC must process on the following tick")
	}
}

func TestWorldSameNameKick(t *testing.T) {
	w := newTestWorld()
	old := registerPlayer(t, w, "alice")
	nu := NewPlayer("alice", w.Spawn())
	nu.Session = newFakeSession()
	w.QueueAddPlayer(nu)
	w.Process()
	// New player added; old player queued for removal and removed in the
	// same cycle's remove phase (queued before the remove phase runs).
	if !nu.IsRegistered() {
		t.Fatal("new same-name player must be registered")
	}
	if old.IsRegistered() {
		t.Fatal("old same-name player must be kicked")
	}
	if w.Players().Len() != 1 {
		t.Fatalf("players = %d, want 1", w.Players().Len())
	}
}

func TestWorldLogoutRequests(t *testing.T) {
	w := newTestWorld()
	p := registerPlayer(t, w, "logout-1")
	accepted := 0
	w.Hooks.OnLogoutAccepted = func(pl *Player) { accepted++ }
	w.QueueLogout(p.Index)
	w.Process()
	if p.IsRegistered() {
		t.Fatal("accepted logout must remove the player")
	}
	if accepted != 1 {
		t.Fatal("OnLogoutAccepted must run once")
	}

	// Refused logout: player stays.
	q := registerPlayer(t, w, "logout-2")
	refused := ""
	w.Hooks.LogoutRefusal = func(pl *Player) string { return "nope" }
	w.Hooks.OnLogoutRefused = func(pl *Player, reason string) { refused = reason }
	w.QueueLogout(q.Index)
	w.Process()
	if !q.IsRegistered() || refused != "nope" {
		t.Fatal("refused logout must keep the player and report the reason")
	}

	// Disconnected session: request is dropped, player stays.
	w.Hooks.LogoutRefusal = nil
	r := registerPlayer(t, w, "logout-3")
	r.Session.Close(1001, "gone")
	w.QueueLogout(r.Index)
	w.Process()
	if !r.IsRegistered() {
		t.Fatal("logout request with closed session must be dropped")
	}
}

func TestWorldPlayerShuffle(t *testing.T) {
	w := newTestWorld()
	const n = 10
	var mu sync.Mutex
	orders := map[int64][]string{}
	for i := 0; i < n; i++ {
		p := NewPlayer(fmt.Sprintf("p%d", i), w.Spawn())
		p.Session = newFakeSession()
		name := p.Username
		// Observe each turn through the turn handler (the timer
		// hooks are gone with the timer port): the handler
		// re-queues one packet per cycle, so every cycle
		// records exactly one observation per player.
		p.TurnHandler = func(pl *Player, pkt TurnPacket, cycle int64) {
			mu.Lock()
			orders[cycle] = append(orders[cycle], name)
			mu.Unlock()
			pl.QueueTurnPacket(TurnPacket{Opcode: 1})
		}
		p.QueueTurnPacket(TurnPacket{Opcode: 1})
		w.QueueAddPlayer(p)
	}
	// 400 cycles: reshuffles happen every 100-150 cycles, so several
	// distinct orders must appear.
	for i := 0; i < 400; i++ {
		w.Process()
	}
	seen := map[string]bool{}
	for cycle := int64(1); cycle <= 400; cycle++ {
		order := orders[cycle]
		if len(order) != n {
			t.Fatalf("cycle %d processed %d players, want %d", cycle, len(order), n)
		}
		uniq := map[string]bool{}
		for _, name := range order {
			if uniq[name] {
				t.Fatalf("cycle %d processed %s twice", cycle, name)
			}
			uniq[name] = true
		}
		key := fmt.Sprint(order)
		seen[key] = true
	}
	if len(seen) < 2 {
		t.Fatal("player order must change across reshuffles (400 cycles)")
	}
}

func TestWorldPhaseFailureIsolated(t *testing.T) {
	w := newTestWorld()
	var errs []string
	w.Hooks.OnPhaseError = func(phase string, err error) { errs = append(errs, phase) }
	w.Hooks.GroundItems = func(cycle int64) { panic("ground boom") }
	ran := false
	w.Hooks.Boats = func(cycle int64) { ran = true }
	w.Process()
	if len(errs) != 1 || errs[0] != "ground_items" {
		t.Fatalf("phase errors = %v, want [ground_items]", errs)
	}
	if !ran {
		t.Fatal("later phases must still run after a phase panic")
	}
	if w.ProcessCycle() != 1 {
		t.Fatal("cycle must advance despite phase failure")
	}
}

func TestWorldPlayerErrorRequestsLogout(t *testing.T) {
	w := newTestWorld()
	p := NewPlayer("err", w.Spawn())
	p.Session = newFakeSession()
	// A panicking turn handler propagates out of Process to the
	// World's per-player guard, as the timer hooks used to (the
	// hooks are gone with the timer port).
	p.TurnHandler = func(*Player, TurnPacket, int64) { panic("turn boom") }
	p.QueueTurnPacket(TurnPacket{Opcode: 1})
	w.QueueAddPlayer(p)
	w.Process() // registers; first turn runs in the same cycle and panics.
	if !p.IsRegistered() {
		t.Fatal("player must still be registered in the cycle it errored")
	}
	w.Process() // removal phase drops it.
	if p.IsRegistered() {
		t.Fatal("player error must queue removal for the next cycle")
	}
}

func TestWorldFlushSkipsClosedSessions(t *testing.T) {
	w := newTestWorld()
	open := registerPlayer(t, w, "open")
	closed := registerPlayer(t, w, "closed")
	closed.Session.Close(1001, "gone")
	nosess := NewPlayer("nosess", w.Spawn())
	w.QueueAddPlayer(nosess)
	w.Process()
	openSess := open.Session.(*fakeSession)
	if len(openSess.flushes) == 0 {
		t.Fatal("open session must be flushed")
	}
	closedSess := closed.Session.(*fakeSession)
	before := len(closedSess.flushes)
	w.Process()
	if len(closedSess.flushes) != before {
		t.Fatal("closed session must not be flushed")
	}
	if !nosess.IsRegistered() {
		t.Fatal("sessionless player must still be registered")
	}
}

func TestWorldFlushErrorRequestsLogout(t *testing.T) {
	w := newTestWorld()
	p := registerPlayer(t, w, "flusherr")
	p.Session.(*fakeSession).flushErr = fmt.Errorf("write failed")
	w.Process()
	w.Process()
	if p.IsRegistered() {
		t.Fatal("flush error must queue removal")
	}
}

func TestWorldIsFull(t *testing.T) {
	w := NewWorld(Location{})
	if w.IsFull() {
		t.Fatal("empty world must not be full")
	}
	// Fill directly through the list (faster than 2048 queue cycles).
	for i := 0; i < MaxPlayers; i++ {
		if _, ok := w.Players().Add(NewPlayer(fmt.Sprintf("p%d", i), Location{})); !ok {
			t.Fatalf("add %d failed", i)
		}
	}
	if !w.IsFull() {
		t.Fatal("world at 2048 players must be full")
	}
}

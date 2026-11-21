package game

import (
	"testing"

	"tsps-go/internal/definition"
)

// recordingSender is a GroundItemSender test double recording every
// delta call.
type recordingSender struct {
	calls []senderCall
}

type senderCall struct {
	player  *Player
	upserts []GroundItemView
	removes []int
}

func (s *recordingSender) SendGroundItemDelta(player *Player, upserts []GroundItemView, removes []int) {
	s.calls = append(s.calls, senderCall{
		player:  player,
		upserts: append([]GroundItemView(nil), upserts...),
		removes: append([]int(nil), removes...),
	})
}

func (s *recordingSender) reset() { s.calls = nil }

// testGroundPlayer builds a connected player at a tile with a slot index.
func testGroundPlayer(index int, username string, x, y, z int) *Player {
	p := NewPlayer(username, Location{X: x, Y: y, Z: z})
	p.Index = index
	p.Session = newFakeSession()
	return p
}

// testGroundItem builds a private, goes-global, non-respawning stack.
func testGroundItem(state GroundItemState, owner string, x, y, z, itemID, amount int) *ItemOnGround {
	return NewItemOnGround(state, owner, Location{X: x, Y: y, Z: z}, itemID, amount, nil, true, -1, false, nil)
}

func newTestGroundManager(players []*Player, sender *recordingSender, scheduler TaskScheduler, stackable func(int) bool, td func(int) (bool, bool)) *GroundItemManager {
	cfg := GroundItemManagerConfig{
		Players:           func() []*Player { return players },
		Scheduler:         scheduler,
		Stackable:         stackable,
		TradeableDropable: td,
	}
	if sender != nil {
		cfg.Sender = sender
	}
	return NewGroundItemManager(cfg)
}

// coinsStackable treats only item 995 (coins) as stackable, and
// coinsTradeable treats only 995 as tradeable and dropable: the
// minimal definition stand-ins for merge/visibility tests.
func coinsStackable(id int) bool { return id == 995 }

func coinsTradeable(id int) (bool, bool) { return id == 995, id == 995 }

func TestGroundItemConstructorDefaults(t *testing.T) {
	it := NewItemOnGround(GroundItemSeenByPlayer, "Alice", Location{X: 1, Y: 2, Z: 0}, 995, 7, map[string]any{"k": 1}, true, -1, false, nil)
	if it.Tick != 0 || it.PendingRemoval {
		t.Fatalf("tick=%d pending=%v, want 0/false", it.Tick, it.PendingRemoval)
	}
	if it.RespawnAmount != 7 || it.OldAmount != 7 {
		t.Fatalf("respawnAmount=%d oldAmount=%d, want 7/7", it.RespawnAmount, it.OldAmount)
	}
	if it.AgedCycle != -1 {
		t.Fatalf("agedCycle=%d, want -1", it.AgedCycle)
	}
	if got := it.EnabledOps(nil); got != 0x1f {
		t.Fatalf("enabledOps=%#x, want 0x1f", got)
	}
	if it.ID() != 0 {
		t.Fatalf("id=%d, want 0 before Register", it.ID())
	}
	// Meta is deep-copied: mutating the source map must not leak in.
	it.Meta["k"] = 2
	if got := NewItemOnGround(GroundItemSeenByPlayer, "", Location{}, 995, 1, map[string]any{"k": 1}, true, -1, false, nil).Meta["k"]; got != 1 {
		t.Fatalf("meta not copied at construction: %v", got)
	}
	if it.Respawns() {
		t.Fatal("respawnTimer -1 respawns, want false")
	}
	if it.EffectiveDespawnDelay() != DespawnDelay {
		t.Fatalf("effective delay=%d, want %d", it.EffectiveDespawnDelay(), DespawnDelay)
	}
	it.SetDespawnDelay(5)
	if it.EffectiveDespawnDelay() != 5 {
		t.Fatalf("effective delay=%d, want 5", it.EffectiveDespawnDelay())
	}
}

func TestGroundItemRegisterAssignsIDsAndCreates(t *testing.T) {
	alice := testGroundPlayer(1, "Alice", 3200, 3200, 0)
	sender := &recordingSender{}
	mgr := newTestGroundManager([]*Player{alice}, sender, nil, nil, nil)
	a := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 1234, 1)
	b := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 1234, 1)
	mgr.Register(a)
	mgr.Register(b)
	if a.ID() != 1 || b.ID() != 2 {
		t.Fatalf("ids=%d,%d, want 1,2", a.ID(), b.ID())
	}
	if len(mgr.items) != 2 {
		t.Fatalf("items=%d, want 2", len(mgr.items))
	}
	if len(sender.calls) != 2 {
		t.Fatalf("sender calls=%d, want 2 CREATEs", len(sender.calls))
	}
	for i, c := range sender.calls {
		if len(c.upserts) != 1 || len(c.removes) != 0 {
			t.Fatalf("call %d: upserts=%d removes=%d, want 1/0", i, len(c.upserts), len(c.removes))
		}
	}
	// Nil register is a no-op.
	mgr.Register(nil)
	if len(mgr.items) != 2 {
		t.Fatalf("nil register changed list: %d", len(mgr.items))
	}
}

func TestGroundItemRegisterPrivateGoesToOwnerOnly(t *testing.T) {
	alice := testGroundPlayer(1, "Alice", 3200, 3200, 0)
	bob := testGroundPlayer(2, "Bob", 3200, 3200, 0)
	sender := &recordingSender{}
	mgr := newTestGroundManager([]*Player{alice, bob}, sender, nil, nil, nil)
	mgr.Register(testGroundItem(GroundItemSeenByPlayer, "Alice", 3200, 3200, 0, 1234, 1))
	if len(sender.calls) != 1 || sender.calls[0].player != alice {
		t.Fatalf("private CREATE calls=%d, want 1 to owner", len(sender.calls))
	}
	if !sender.calls[0].upserts[0].IsPrivate {
		t.Fatal("private view IsPrivate=false")
	}
	if sender.calls[0].upserts[0].OwnerID != 1 || sender.calls[0].upserts[0].Ownership != GroundItemOwnershipMine {
		t.Fatalf("owner view = %+v, want ownerId 1 ownership 1", sender.calls[0].upserts[0])
	}
}

func TestGroundItemPerformSkipsBotsAndDisconnected(t *testing.T) {
	alice := testGroundPlayer(1, "Alice", 3200, 3200, 0)
	bot := testGroundPlayer(2, "Bot", 3200, 3200, 0)
	bot.IsBot = true
	gone := testGroundPlayer(3, "Gone", 3200, 3200, 0)
	gone.Session.(*fakeSession).open = false
	far := testGroundPlayer(4, "Far", 3300, 3200, 0)
	sender := &recordingSender{}
	mgr := newTestGroundManager([]*Player{alice, bot, gone, far}, sender, nil, nil, nil)
	mgr.Register(testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 1234, 1))
	if len(sender.calls) != 1 || sender.calls[0].player != alice {
		t.Fatalf("calls=%d, want 1 to Alice only (bot/disconnected/far skipped)", len(sender.calls))
	}
}

func TestGroundItemPerformPlayerPendingRemovalForcesDelete(t *testing.T) {
	alice := testGroundPlayer(1, "Alice", 3200, 3200, 0)
	sender := &recordingSender{}
	mgr := newTestGroundManager([]*Player{alice}, sender, nil, nil, nil)
	it := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 1234, 1)
	mgr.Register(it)
	sender.reset()
	it.PendingRemoval = true
	mgr.PerformPlayer(alice, it, GroundItemCreate)
	if len(sender.calls) != 1 || len(sender.calls[0].removes) != 1 || sender.calls[0].removes[0] != it.ID() {
		t.Fatalf("calls=%+v, want 1 DELETE of id %d", sender.calls, it.ID())
	}
	// Unknown operations are dropped, never panicking the tick.
	sender.reset()
	it.PendingRemoval = false
	mgr.PerformPlayer(alice, it, GroundItemOp(99))
	if len(sender.calls) != 0 {
		t.Fatalf("unknown op sent %d calls", len(sender.calls))
	}
}

func TestGroundItemRevealAt100(t *testing.T) {
	alice := testGroundPlayer(1, "Alice", 3200, 3200, 0)
	sender := &recordingSender{}
	mgr := newTestGroundManager([]*Player{alice}, sender, nil, coinsStackable, nil)
	it := testGroundItem(GroundItemSeenByPlayer, "Alice", 3200, 3200, 0, 1234, 1)
	mgr.Register(it)
	sender.reset()
	for cycle := int64(1); cycle <= 99; cycle++ {
		mgr.Process(cycle)
	}
	if it.State != GroundItemSeenByPlayer || it.Tick != 99 {
		t.Fatalf("after 99 ticks: state=%d tick=%d, want private/99", it.State, it.Tick)
	}
	if len(sender.calls) != 0 {
		t.Fatalf("pre-reveal sends: %d", len(sender.calls))
	}
	mgr.Process(100)
	if it.State != GroundItemSeenByEveryone || it.Tick != 100 {
		t.Fatalf("after 100 ticks: state=%d tick=%d, want public/100", it.State, it.Tick)
	}
	// Reveal = DELETE for the owner, then CREATE as public.
	if len(sender.calls) != 2 {
		t.Fatalf("reveal sends=%d, want 2", len(sender.calls))
	}
	if len(sender.calls[0].removes) != 1 || sender.calls[0].removes[0] != it.ID() {
		t.Fatalf("first reveal call=%+v, want DELETE of %d", sender.calls[0], it.ID())
	}
	if len(sender.calls[1].upserts) != 1 || sender.calls[1].upserts[0].IsPrivate {
		t.Fatalf("second reveal call=%+v, want public CREATE", sender.calls[1])
	}
	if mgr.finishedCycle != 100 {
		t.Fatalf("finishedCycle=%d, want 100", mgr.finishedCycle)
	}
}

func TestGroundItemRevealSkippedWhenNotGlobalOrPaused(t *testing.T) {
	alice := testGroundPlayer(1, "Alice", 3200, 3200, 0)
	sender := &recordingSender{}
	mgr := newTestGroundManager([]*Player{alice}, sender, nil, nil, nil)
	kept := testGroundItem(GroundItemSeenByPlayer, "Alice", 3200, 3200, 0, 1234, 1)
	kept.GoesGlobal = false
	paused := testGroundItem(GroundItemSeenByPlayer, "Alice", 3201, 3200, 0, 1234, 1)
	paused.AgingPaused = true
	mgr.Register(kept)
	mgr.Register(paused)
	for cycle := int64(1); cycle <= 150; cycle++ {
		mgr.Process(cycle)
	}
	if kept.State != GroundItemSeenByPlayer {
		t.Fatal("non-global stack went public")
	}
	if paused.State != GroundItemSeenByPlayer || paused.Tick != 0 {
		t.Fatalf("paused stack: state=%d tick=%d, want private/0", paused.State, paused.Tick)
	}
	if paused.AgedCycle != 150 {
		t.Fatalf("paused agedCycle=%d, want 150 (stamped even when paused)", paused.AgedCycle)
	}
}

func TestGroundItemDespawnAt300AndCustomDelay(t *testing.T) {
	alice := testGroundPlayer(1, "Alice", 3200, 3200, 0)
	sender := &recordingSender{}
	mgr := newTestGroundManager([]*Player{alice}, sender, nil, nil, nil)
	def := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 1234, 1)
	custom := testGroundItem(GroundItemSeenByEveryone, "", 3201, 3200, 0, 1234, 1)
	custom.SetDespawnDelay(5)
	mgr.Register(def)
	mgr.Register(custom)
	for cycle := int64(1); cycle <= 4; cycle++ {
		mgr.Process(cycle)
	}
	if len(mgr.items) != 2 {
		t.Fatalf("after 4 ticks items=%d, want 2", len(mgr.items))
	}
	mgr.Process(5)
	if len(mgr.items) != 1 || mgr.items[0] != def {
		t.Fatalf("after 5 ticks items=%d, want only the default-delay stack", len(mgr.items))
	}
	for cycle := int64(6); cycle <= 299; cycle++ {
		mgr.Process(cycle)
	}
	if len(mgr.items) != 1 {
		t.Fatalf("after 299 ticks items=%d, want 1", len(mgr.items))
	}
	mgr.Process(300)
	if len(mgr.items) != 0 {
		t.Fatalf("after 300 ticks items=%d, want 0", len(mgr.items))
	}
}

func TestGroundItemStaticAndRespawningNeverDespawn(t *testing.T) {
	mgr := newTestGroundManager(nil, nil, nil, nil, nil)
	static := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 1234, 1)
	static.StaticSpawn = true
	respawning := testGroundItem(GroundItemSeenByEveryone, "", 3201, 3200, 0, 1234, 1)
	respawning.RespawnTimer = 10
	mgr.Register(static)
	mgr.Register(respawning)
	for cycle := int64(1); cycle <= 500; cycle++ {
		mgr.Process(cycle)
	}
	if len(mgr.items) != 2 {
		t.Fatalf("after 500 ticks items=%d, want 2 (static + respawning persist)", len(mgr.items))
	}
}

func TestGroundItemProcessIteratesCopy(t *testing.T) {
	mgr := newTestGroundManager(nil, nil, nil, nil, nil)
	a := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 1234, 1)
	b := testGroundItem(GroundItemSeenByEveryone, "", 3201, 3200, 0, 1234, 1)
	mgr.Register(a)
	mgr.Register(b)
	mgr.Deregister(a) // pending removal, still in the list
	mgr.Process(1)
	if len(mgr.items) != 1 || mgr.items[0] != b {
		t.Fatalf("items=%d, want only b", len(mgr.items))
	}
	if b.Tick != 1 {
		t.Fatalf("b tick=%d, want 1 (removal mid-loop must not skip it)", b.Tick)
	}
}

func TestGroundItemRespawnTask(t *testing.T) {
	tasks := NewTaskManager()
	sender := &recordingSender{}
	mgr := newTestGroundManager(nil, sender, tasks, coinsStackable, nil)
	orig := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 5)
	orig.StaticSpawn = true
	orig.RespawnTimer = 10
	mgr.Register(orig)
	orig.Amount = 2 // partial pickup: respawn amount stays 5
	origID := orig.ID()
	mgr.Deregister(orig)
	mgr.Process(1)
	if len(mgr.items) != 0 {
		t.Fatal("deregistered stack still listed after Process")
	}
	for cycle := int64(2); cycle <= 10; cycle++ {
		tasks.Process(cycle)
	}
	if len(mgr.items) != 0 {
		t.Fatalf("stack respawned early, after %d task ticks", 9)
	}
	tasks.Process(11)
	if len(mgr.items) != 1 {
		t.Fatalf("items=%d after respawn delay, want 1", len(mgr.items))
	}
	restored := mgr.items[0]
	if restored.ID() == origID {
		t.Fatal("respawned stack kept the old id, want a fresh one")
	}
	if restored.Amount != 5 {
		t.Fatalf("respawned amount=%d, want reset to respawn amount 5", restored.Amount)
	}
	if restored.Tick != 0 || !restored.StaticSpawn || restored.RespawnTimer != 10 {
		t.Fatalf("respawned stack: tick=%d static=%v timer=%d", restored.Tick, restored.StaticSpawn, restored.RespawnTimer)
	}
}

func TestGroundItemRespawnKeepsAmountWhenNotStatic(t *testing.T) {
	tasks := NewTaskManager()
	mgr := newTestGroundManager(nil, nil, tasks, nil, nil)
	orig := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 1234, 5)
	orig.RespawnTimer = 3
	mgr.Register(orig)
	orig.Amount = 2
	mgr.Deregister(orig)
	mgr.Process(1)
	for cycle := int64(2); cycle <= 4; cycle++ {
		tasks.Process(cycle)
	}
	if len(mgr.items) != 1 || mgr.items[0].Amount != 2 {
		t.Fatalf("non-static respawn: items=%d amount=%d, want 1/2 (no reset)", len(mgr.items), func() int {
			if len(mgr.items) > 0 {
				return mgr.items[0].Amount
			}
			return -1
		}())
	}
}

func TestGroundItemRespawnWithoutScheduler(t *testing.T) {
	mgr := newTestGroundManager(nil, nil, nil, nil, nil)
	it := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 1234, 1)
	it.StaticSpawn = true
	it.RespawnTimer = 5
	mgr.Register(it)
	mgr.Deregister(it)
	mgr.Process(1) // must not panic with no scheduler
	if len(mgr.items) != 0 {
		t.Fatal("stack not removed")
	}
}

func TestGroundItemMerge(t *testing.T) {
	newMerged := func() (*GroundItemManager, *recordingSender, *ItemOnGround) {
		alice := testGroundPlayer(1, "Alice", 3200, 3200, 0)
		sender := &recordingSender{}
		mgr := newTestGroundManager([]*Player{alice}, sender, nil, coinsStackable, nil)
		existing := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 10)
		mgr.Register(existing)
		existing.Tick = 50
		sender.reset()
		return mgr, sender, existing
	}

	t.Run("same tile id meta owner merges", func(t *testing.T) {
		mgr, sender, existing := newMerged()
		incoming := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 5)
		mgr.Register(incoming)
		if len(mgr.items) != 1 || existing.Amount != 15 {
			t.Fatalf("items=%d amount=%d, want 1/15", len(mgr.items), existing.Amount)
		}
		if existing.Tick != 0 || existing.OldAmount != 10 {
			t.Fatalf("tick=%d oldAmount=%d, want 0/10", existing.Tick, existing.OldAmount)
		}
		if len(sender.calls) != 1 || len(sender.calls[0].upserts) != 1 || sender.calls[0].upserts[0].Quantity != 15 {
			t.Fatalf("merge sends=%+v, want 1 ALTER with quantity 15", sender.calls)
		}
		if incoming.ID() != 0 {
			t.Fatalf("merged incoming got id %d, want 0 (never appended)", incoming.ID())
		}
	})

	t.Run("different owner never merges", func(t *testing.T) {
		mgr, _, _ := newMerged()
		mgr.Register(testGroundItem(GroundItemSeenByEveryone, "Alice", 3200, 3200, 0, 995, 5))
		if len(mgr.items) != 2 {
			t.Fatalf("items=%d, want 2", len(mgr.items))
		}
	})

	t.Run("different meta never merges", func(t *testing.T) {
		mgr, _, _ := newMerged()
		incoming := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 5)
		incoming.Meta = map[string]any{"k": 1}
		mgr.Register(incoming)
		if len(mgr.items) != 2 {
			t.Fatalf("items=%d, want 2", len(mgr.items))
		}
	})

	t.Run("equal meta different key order merges", func(t *testing.T) {
		mgr, _, existing := newMerged()
		existing.Meta = map[string]any{"a": 1, "b": 2}
		incoming := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 5)
		incoming.Meta = map[string]any{"b": 2, "a": 1}
		mgr.Register(incoming)
		if len(mgr.items) != 1 {
			t.Fatalf("items=%d, want 1 (canonical JSON identity)", len(mgr.items))
		}
	})

	t.Run("non-stackable never merges", func(t *testing.T) {
		mgr, _, _ := newMerged()
		mgr.Register(testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 1234, 5))
		if len(mgr.items) != 2 {
			t.Fatalf("items=%d, want 2", len(mgr.items))
		}
	})

	t.Run("no stackable source never merges", func(t *testing.T) {
		mgr := newTestGroundManager(nil, nil, nil, nil, nil)
		mgr.Register(testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 10))
		mgr.Register(testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 5))
		if len(mgr.items) != 2 {
			t.Fatalf("items=%d, want 2 (nil stackable source)", len(mgr.items))
		}
	})

	t.Run("cap blocks merge, exact cap merges", func(t *testing.T) {
		mgr, _, existing := newMerged()
		existing.Amount = MaxStackAmount - 5
		mgr.Register(testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 6))
		if len(mgr.items) != 2 {
			t.Fatalf("over-cap: items=%d, want 2", len(mgr.items))
		}
		// Remove the unmerged stack and retry at exactly the cap.
		mgr.items = mgr.items[:1]
		mgr.Register(testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 5))
		if len(mgr.items) != 1 || existing.Amount != MaxStackAmount {
			t.Fatalf("exact cap: items=%d amount=%d, want 1/max", len(mgr.items), existing.Amount)
		}
	})

	t.Run("effective despawn delay decides", func(t *testing.T) {
		mgr, _, existing := newMerged()
		existing.SetDespawnDelay(300) // explicit 300 == nil default
		mgr.Register(testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 5))
		if len(mgr.items) != 1 {
			t.Fatalf("nil vs explicit 300: items=%d, want 1", len(mgr.items))
		}
		mgr2, _, existing2 := newMerged()
		existing2.SetDespawnDelay(5)
		mgr2.Register(testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 5))
		if len(mgr2.items) != 2 {
			t.Fatalf("5 vs default: items=%d, want 2", len(mgr2.items))
		}
	})

	t.Run("policy mismatch blocks merge", func(t *testing.T) {
		mgr, _, existing := newMerged()
		if err := existing.SetEnabledOps(0x1e, nil); err != nil {
			t.Fatal(err)
		}
		mgr.Register(testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 5))
		if len(mgr.items) != 2 {
			t.Fatalf("mask mismatch: items=%d, want 2", len(mgr.items))
		}
		mgr2, _, existing2 := newMerged()
		viewer := testGroundPlayer(9, "Viewer", 3200, 3200, 0)
		if err := existing2.SetEnabledOps(0x1f, viewer); err != nil {
			t.Fatal(err)
		}
		mgr2.Register(testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 5))
		if len(mgr2.items) != 2 {
			t.Fatalf("viewer override: items=%d, want 2", len(mgr2.items))
		}
	})

	t.Run("different tile never merges", func(t *testing.T) {
		mgr, _, _ := newMerged()
		mgr.Register(testGroundItem(GroundItemSeenByEveryone, "", 3201, 3200, 0, 995, 5))
		mgr.Register(testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 1, 995, 5))
		if len(mgr.items) != 3 {
			t.Fatalf("items=%d, want 3", len(mgr.items))
		}
	})

	t.Run("pending removal candidate skipped", func(t *testing.T) {
		mgr, _, existing := newMerged()
		existing.PendingRemoval = true
		mgr.Register(testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 5))
		if len(mgr.items) != 2 || existing.Amount != 10 {
			t.Fatalf("items=%d amount=%d, want 2/10", len(mgr.items), existing.Amount)
		}
	})
}

func TestGroundItemMergeRecurringStatic(t *testing.T) {
	setup := func() (*GroundItemManager, *ItemOnGround) {
		mgr := newTestGroundManager(nil, nil, nil, coinsStackable, nil)
		existing := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 10)
		existing.StaticSpawn = true
		existing.RespawnTimer = 10
		mgr.Register(existing)
		return mgr, existing
	}
	recurring := func(amount int) *ItemOnGround {
		it := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, amount)
		it.StaticSpawn = true
		it.RespawnTimer = 10
		return it
	}

	t.Run("matching recurring merges and sums respawn", func(t *testing.T) {
		mgr, existing := setup()
		mgr.Register(recurring(5))
		if len(mgr.items) != 1 || existing.Amount != 15 || existing.RespawnAmount != 15 {
			t.Fatalf("items=%d amount=%d respawn=%d, want 1/15/15", len(mgr.items), existing.Amount, existing.RespawnAmount)
		}
	})

	t.Run("different timer blocks", func(t *testing.T) {
		mgr, _ := setup()
		incoming := recurring(5)
		incoming.RespawnTimer = 20
		mgr.Register(incoming)
		if len(mgr.items) != 2 {
			t.Fatalf("items=%d, want 2", len(mgr.items))
		}
	})

	t.Run("recurring never absorbs a drop", func(t *testing.T) {
		mgr, _ := setup()
		mgr.Register(testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 5))
		if len(mgr.items) != 2 {
			t.Fatalf("items=%d, want 2", len(mgr.items))
		}
	})

	t.Run("drop never absorbs recurring", func(t *testing.T) {
		mgr := newTestGroundManager(nil, nil, nil, coinsStackable, nil)
		mgr.Register(testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 10))
		mgr.Register(recurring(5))
		if len(mgr.items) != 2 {
			t.Fatalf("items=%d, want 2", len(mgr.items))
		}
	})

	t.Run("respawn sum over cap blocks", func(t *testing.T) {
		mgr, existing := setup()
		existing.RespawnAmount = MaxStackAmount - 1
		incoming := recurring(5)
		incoming.RespawnAmount = 5
		mgr.Register(incoming)
		if len(mgr.items) != 2 {
			t.Fatalf("items=%d, want 2", len(mgr.items))
		}
	})

	t.Run("state mismatch blocks", func(t *testing.T) {
		mgr, _ := setup()
		incoming := recurring(5)
		incoming.State = GroundItemSeenByPlayer
		mgr.Register(incoming)
		if len(mgr.items) != 2 {
			t.Fatalf("items=%d, want 2", len(mgr.items))
		}
	})
}

func TestGroundItemRevealMerges(t *testing.T) {
	alice := testGroundPlayer(1, "Alice", 3200, 3200, 0)
	sender := &recordingSender{}
	mgr := newTestGroundManager([]*Player{alice}, sender, nil, coinsStackable, nil)
	// The existing public stack must share the owner: stacks from
	// different owners never merge, so an unowned pile cannot absorb
	// Alice's revealing drop. A viewer override blocks the
	// register-time merge (policy mismatch); removing it before the
	// reveal leaves the reveal-branch merge as the only merge path.
	public := testGroundItem(GroundItemSeenByEveryone, "Alice", 3200, 3200, 0, 995, 10)
	mgr.Register(public)
	if err := public.SetEnabledOps(0x1f, alice); err != nil {
		t.Fatal(err)
	}
	private := testGroundItem(GroundItemSeenByPlayer, "Alice", 3200, 3200, 0, 995, 5)
	mgr.Register(private)
	if len(mgr.items) != 2 {
		t.Fatalf("items=%d after register, want 2 (override blocks merge)", len(mgr.items))
	}
	public.ResetEnabledOps(alice)
	sender.reset()
	for cycle := int64(1); cycle <= 100; cycle++ {
		mgr.Process(cycle)
	}
	if len(mgr.items) != 1 || mgr.items[0] != public || public.Amount != 15 {
		t.Fatalf("items=%d amount=%d, want merged public stack of 15", len(mgr.items), public.Amount)
	}
	// Owner DELETE, then ALTER on the merged stack; no public CREATE
	// for the revealing stack (it merged away).
	if len(sender.calls) != 2 {
		t.Fatalf("reveal-merge sends=%d, want 2", len(sender.calls))
	}
	if len(sender.calls[0].removes) != 1 || sender.calls[0].removes[0] != private.ID() {
		t.Fatalf("first call=%+v, want DELETE of private id", sender.calls[0])
	}
	if len(sender.calls[1].upserts) != 1 || sender.calls[1].upserts[0].ID != public.ID() {
		t.Fatalf("second call=%+v, want ALTER of public id", sender.calls[1])
	}
}

func TestGroundItemCanSee(t *testing.T) {
	alice := testGroundPlayer(1, "Alice", 3200, 3200, 0)
	bob := testGroundPlayer(2, "Bob", 3200, 3200, 0)
	edge := testGroundPlayer(3, "Edge", 3264, 3200, 0) // dx 64: in range
	far := testGroundPlayer(4, "Far", 3265, 3200, 0)   // dx 65: out of range
	above := testGroundPlayer(5, "Above", 3200, 3200, 1)
	td := func(id int) (bool, bool) {
		switch id {
		case 995:
			return true, true
		case 555:
			return true, false // tradeable but not dropable
		default:
			return false, false
		}
	}
	mgr := newTestGroundManager(nil, nil, nil, nil, td)
	at := func(state GroundItemState, owner string, id int) *ItemOnGround {
		return testGroundItem(state, owner, 3200, 3200, 0, id, 1)
	}
	cases := []struct {
		name string
		item *ItemOnGround
		want map[*Player]bool
	}{
		{"private owner only", at(GroundItemSeenByPlayer, "Alice", 995), map[*Player]bool{alice: true, bob: false, edge: false, far: false, above: false}},
		{"private unowned invisible", at(GroundItemSeenByPlayer, "", 995), map[*Player]bool{alice: false, bob: false, edge: false, far: false, above: false}},
		{"public tradeable dropable", at(GroundItemSeenByEveryone, "Alice", 995), map[*Player]bool{alice: true, bob: true, edge: true, far: false, above: false}},
		{"public untradeable owner only", at(GroundItemSeenByEveryone, "Alice", 1234), map[*Player]bool{alice: true, bob: false, edge: false, far: false, above: false}},
		{"public tradeable not dropable owner only", at(GroundItemSeenByEveryone, "Alice", 555), map[*Player]bool{alice: true, bob: false, edge: false, far: false, above: false}},
		{"public unowned visible", at(GroundItemSeenByEveryone, "", 1234), map[*Player]bool{alice: true, bob: true, edge: true, far: false, above: false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for p, want := range tc.want {
				if got := mgr.CanSee(p, tc.item, false); got != want {
					t.Errorf("CanSee(%s)=%v, want %v", p.Username, got, want)
				}
			}
		})
	}
	t.Run("static visible to all in range", func(t *testing.T) {
		it := at(GroundItemSeenByEveryone, "Alice", 1234)
		it.StaticSpawn = true
		if !mgr.CanSee(bob, it, false) || mgr.CanSee(far, it, false) || mgr.CanSee(above, it, false) {
			t.Error("static visibility wrong")
		}
	})
	t.Run("untradeable meta blocks tradeable term", func(t *testing.T) {
		it := at(GroundItemSeenByEveryone, "Alice", 995)
		it.Meta = map[string]any{"untradeable": true}
		if mgr.CanSee(bob, it, false) {
			t.Error("untradeable-meta stack visible to non-owner")
		}
		if !mgr.CanSee(alice, it, false) {
			t.Error("untradeable-meta stack invisible to owner")
		}
		it.Meta = map[string]any{"untradeable": 1} // not strict true
		if !mgr.CanSee(bob, it, false) {
			t.Error("non-boolean untradeable flag blocked visibility")
		}
	})
	t.Run("pending removal gate", func(t *testing.T) {
		it := at(GroundItemSeenByEveryone, "", 1234)
		it.PendingRemoval = true
		if mgr.CanSee(bob, it, false) {
			t.Error("pending removal visible without flag")
		}
		if !mgr.CanSee(bob, it, true) {
			t.Error("pending removal invisible with flag")
		}
	})
	t.Run("owner name normalization", func(t *testing.T) {
		it := at(GroundItemSeenByPlayer, "Alice Smith", 1234)
		aliased := testGroundPlayer(6, "alice_smith", 3200, 3200, 0)
		if !mgr.CanSee(aliased, it, false) {
			t.Error("protocol-form owner match failed")
		}
	})
	t.Run("nil definition source kills tradeable term", func(t *testing.T) {
		noDefs := newTestGroundManager(nil, nil, nil, nil, nil)
		it := at(GroundItemSeenByEveryone, "Alice", 995)
		if noDefs.CanSee(bob, it, false) {
			t.Error("tradeable term true with no definition source")
		}
	})
}

func TestGroundItemGetTiming(t *testing.T) {
	mgr := NewGroundItemManager(GroundItemManagerConfig{})
	private := testGroundItem(GroundItemSeenByPlayer, "Alice", 3200, 3200, 0, 1234, 1)
	private.Tick = 3
	private.AgedCycle = 10
	mgr.finishedCycle = 10
	got := mgr.GetTiming(private, 10)
	if got != (GroundItemTiming{CreatedTick: 7, PrivateUntilTick: 107, ExpiresTick: 307}) {
		t.Fatalf("timing=%+v, want {7 107 307}", got)
	}
	// Not aged this cycle and pass unfinished: one more aging.
	private.AgedCycle = 9
	mgr.finishedCycle = 9
	got = mgr.GetTiming(private, 10)
	if got.CreatedTick != 6 || got.PrivateUntilTick != 106 || got.ExpiresTick != 306 {
		t.Fatalf("unaged timing=%+v, want {6 106 306}", got)
	}
	// Pass finished this cycle: no extra aging even if unaged.
	mgr.finishedCycle = 10
	got = mgr.GetTiming(private, 10)
	if got.CreatedTick != 7 {
		t.Fatalf("finished timing created=%d, want 7", got.CreatedTick)
	}
	// Public stacks never go private; static/respawning never expire.
	public := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 1234, 1)
	public.AgedCycle = 10
	got = mgr.GetTiming(public, 10)
	if got.PrivateUntilTick != 0 || got.ExpiresTick != 310 {
		t.Fatalf("public timing=%+v, want private 0 expires 310", got)
	}
	static := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 1234, 1)
	static.StaticSpawn = true
	static.AgedCycle = 10
	if got := mgr.GetTiming(static, 10); got.ExpiresTick != 0 {
		t.Fatalf("static expires=%d, want 0", got.ExpiresTick)
	}
	respawning := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 1234, 1)
	respawning.RespawnTimer = 5
	respawning.AgedCycle = 10
	if got := mgr.GetTiming(respawning, 10); got.ExpiresTick != 0 {
		t.Fatalf("respawning expires=%d, want 0", got.ExpiresTick)
	}
	nonGlobal := testGroundItem(GroundItemSeenByPlayer, "Alice", 3200, 3200, 0, 1234, 1)
	nonGlobal.GoesGlobal = false
	nonGlobal.AgedCycle = 10
	if got := mgr.GetTiming(nonGlobal, 10); got.PrivateUntilTick != 0 {
		t.Fatalf("non-global privateUntil=%d, want 0", got.PrivateUntilTick)
	}
	custom := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 1234, 1)
	custom.SetDespawnDelay(5)
	custom.AgedCycle = 10
	if got := mgr.GetTiming(custom, 10); got.ExpiresTick != 15 {
		t.Fatalf("custom expires=%d, want 15", got.ExpiresTick)
	}
	// Int32 wrap: past 0x7fffffff reads 0; exactly at the max reads through.
	wrap := testGroundItem(GroundItemSeenByPlayer, "Alice", 3200, 3200, 0, 1234, 1)
	wrap.AgedCycle = 0x7fffffff - 50
	mgr.finishedCycle = 0x7fffffff - 50
	got = mgr.GetTiming(wrap, 0x7fffffff-50)
	if got.CreatedTick != 0x7fffffff-50 || got.PrivateUntilTick != 0 || got.ExpiresTick != 0 {
		t.Fatalf("wrap timing=%+v, want created max-50, private/expires 0", got)
	}
	wrap.AgedCycle = 0x7fffffff - 100
	mgr.finishedCycle = 0x7fffffff - 100
	got = mgr.GetTiming(wrap, 0x7fffffff-100)
	if got.PrivateUntilTick != 0x7fffffff {
		t.Fatalf("boundary privateUntil=%d, want 0x7fffffff", got.PrivateUntilTick)
	}
	if got := mgr.GetTiming(nil, 10); got != (GroundItemTiming{}) {
		t.Fatalf("nil timing=%+v, want zero", got)
	}
}

func TestGroundItemVisibleViews(t *testing.T) {
	alice := testGroundPlayer(1, "Alice", 3200, 3200, 0)
	bob := testGroundPlayer(2, "Bob", 3200, 3200, 0)
	mgr := newTestGroundManager([]*Player{alice, bob}, nil, nil, nil, coinsTradeable)
	owned := testGroundItem(GroundItemSeenByPlayer, "Alice", 3200, 3200, 0, 995, 3)
	shared := testGroundItem(GroundItemSeenByEveryone, "", 3201, 3200, 0, 1234, 1)
	hidden := testGroundItem(GroundItemSeenByEveryone, "Alice", 3202, 3200, 0, 1234, 1)
	mgr.Register(owned)
	mgr.Register(shared)
	mgr.Register(hidden)
	mgr.Process(1)

	aliceViews := mgr.VisibleViews(alice, 1)
	if len(aliceViews) != 3 {
		t.Fatalf("alice views=%d, want 3", len(aliceViews))
	}
	if v := aliceViews[0]; !v.IsPrivate || v.OwnerID != 1 || v.Ownership != GroundItemOwnershipMine || v.Quantity != 3 {
		t.Fatalf("alice owned view=%+v", v)
	}
	if v := aliceViews[1]; v.IsPrivate || v.OwnerID != -1 || v.Ownership != GroundItemOwnershipNone {
		t.Fatalf("alice shared view=%+v", v)
	}
	if v := aliceViews[2]; v.Ownership != GroundItemOwnershipMine || v.OwnerID != 1 {
		t.Fatalf("alice hidden view=%+v, want ownership 1 ownerId 1", v)
	}
	bobViews := mgr.VisibleViews(bob, 1)
	if len(bobViews) != 1 || bobViews[0].ID != shared.ID() {
		t.Fatalf("bob views=%d, want only the shared stack", len(bobViews))
	}
	if v := bobViews[0]; v.EnabledOps != 0x1f || v.CreatedTick != 0 || v.ExpiresTick != 300 {
		t.Fatalf("bob view=%+v, want ops 0x1f created 0 expires 300", v)
	}
	if mgr.VisibleViews(nil, 1) != nil {
		t.Fatal("nil viewer sees views")
	}
}

func TestGroundItemViewStrictOwnerMatch(t *testing.T) {
	// canSee matches protocol-form names, but the view's mine test is
	// strict equality: a normalized-but-different username sees the
	// stack as another player's.
	aliased := testGroundPlayer(6, "alice_smith", 3200, 3200, 0)
	mgr := newTestGroundManager([]*Player{aliased}, nil, nil, nil, nil)
	it := testGroundItem(GroundItemSeenByPlayer, "Alice Smith", 3200, 3200, 0, 1234, 1)
	mgr.Register(it)
	views := mgr.VisibleViews(aliased, 0)
	if len(views) != 1 {
		t.Fatalf("views=%d, want 1 (protocol-form owner)", len(views))
	}
	if views[0].OwnerID != -1 || views[0].Ownership != GroundItemOwnershipOther {
		t.Fatalf("view=%+v, want ownerId -1 ownership 2 (strict mine test)", views[0])
	}
}

func TestGroundItemOwnerLookupNormalization(t *testing.T) {
	aliased := testGroundPlayer(6, "alice_smith", 3200, 3200, 0)
	sender := &recordingSender{}
	mgr := newTestGroundManager([]*Player{aliased}, sender, nil, nil, nil)
	mgr.Register(testGroundItem(GroundItemSeenByPlayer, "Alice Smith", 3200, 3200, 0, 1234, 1))
	if len(sender.calls) != 1 || sender.calls[0].player != aliased {
		t.Fatalf("calls=%d, want private CREATE to the normalized owner", len(sender.calls))
	}
}

func TestGroundItemEnabledOps(t *testing.T) {
	alice := testGroundPlayer(1, "Alice", 3200, 3200, 0)
	bob := testGroundPlayer(2, "Bob", 3200, 3200, 0)
	sender := &recordingSender{}
	mgr := newTestGroundManager([]*Player{alice, bob}, sender, nil, nil, nil)
	it := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 1234, 1)
	// Unregistered: sets apply, no refresh sends.
	if err := it.SetEnabledOps(0x0e, nil); err != nil {
		t.Fatal(err)
	}
	if it.EnabledOps(nil) != 0x0e || len(sender.calls) != 0 {
		t.Fatalf("unregistered set: ops=%#x sends=%d", it.EnabledOps(nil), len(sender.calls))
	}
	mgr.Register(it)
	sender.reset()
	// Viewer override: count, read-back, reset.
	if err := it.SetEnabledOps(0x01, alice); err != nil {
		t.Fatal(err)
	}
	if it.ViewerOverrideCount() != 1 || it.EnabledOps(alice) != 0x01 || it.EnabledOps(bob) != 0x0e {
		t.Fatalf("override: count=%d alice=%#x bob=%#x", it.ViewerOverrideCount(), it.EnabledOps(alice), it.EnabledOps(bob))
	}
	if len(sender.calls) != 1 || sender.calls[0].player != alice {
		t.Fatalf("viewer refresh sends=%d, want 1 to Alice", len(sender.calls))
	}
	if err := it.SetEnabledOps(0x03, alice); err != nil {
		t.Fatal(err)
	}
	if it.ViewerOverrideCount() != 1 {
		t.Fatalf("re-set override count=%d, want 1", it.ViewerOverrideCount())
	}
	it.ResetEnabledOps(alice)
	if it.ViewerOverrideCount() != 0 || it.EnabledOps(alice) != 0x0e {
		t.Fatalf("after reset: count=%d alice=%#x", it.ViewerOverrideCount(), it.EnabledOps(alice))
	}
	// Stack-wide set refreshes every viewer.
	sender.reset()
	if err := it.SetEnabledOps(0x1f, nil); err != nil {
		t.Fatal(err)
	}
	if len(sender.calls) != 2 {
		t.Fatalf("stack refresh sends=%d, want 2", len(sender.calls))
	}
	// Invalid masks error and change nothing.
	if err := it.SetEnabledOps(0x20, nil); err == nil {
		t.Fatal("mask 0x20 accepted")
	}
	if err := it.SetEnabledOps(-1, nil); err == nil {
		t.Fatal("mask -1 accepted")
	}
	if it.EnabledOps(nil) != 0x1f {
		t.Fatalf("ops=%#x after invalid sets, want 0x1f", it.EnabledOps(nil))
	}
	// Operation bits.
	if !it.IsOperationEnabled(alice, 1) || !it.IsOperationEnabled(alice, 5) {
		t.Fatal("ops 1/5 disabled under 0x1f")
	}
	if it.IsOperationEnabled(alice, 0) || it.IsOperationEnabled(alice, 6) {
		t.Fatal("ops 0/6 enabled")
	}
	// Pending removal: no refresh.
	sender.reset()
	it.PendingRemoval = true
	if err := it.SetEnabledOps(0x00, nil); err != nil {
		t.Fatal(err)
	}
	if len(sender.calls) != 0 {
		t.Fatalf("pending-removal refresh sends=%d, want 0", len(sender.calls))
	}
}

func TestGroundItemOperationPolicy(t *testing.T) {
	a := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 1)
	b := testGroundItem(GroundItemSeenByEveryone, "", 3200, 3200, 0, 995, 1)
	if !a.HasSameOperationPolicy(b) {
		t.Fatal("identical stacks differ in policy")
	}
	if err := b.SetEnabledOps(0x1e, nil); err != nil {
		t.Fatal(err)
	}
	if a.HasSameOperationPolicy(b) {
		t.Fatal("mask mismatch shares policy")
	}
	if err := b.SetEnabledOps(0x1f, nil); err != nil {
		t.Fatal(err)
	}
	viewer := testGroundPlayer(1, "Alice", 3200, 3200, 0)
	if err := b.SetEnabledOps(0x1f, viewer); err != nil {
		t.Fatal(err)
	}
	if a.HasSameOperationPolicy(b) {
		t.Fatal("viewer override shares policy")
	}
	b.ResetEnabledOps(viewer)
	a.PickupReplacement = &GroundPickupReplacement{ID: 5}
	if a.HasSameOperationPolicy(b) {
		t.Fatal("replacement mismatch shares policy")
	}
	b.PickupReplacement = &GroundPickupReplacement{ID: 5}
	if !a.HasSameOperationPolicy(b) {
		t.Fatal("equal replacements differ in policy")
	}
	b.PickupReplacement.TelegrabDenial = "no"
	if a.HasSameOperationPolicy(b) {
		t.Fatal("denial mismatch shares policy")
	}
}

func TestGroundItemClone(t *testing.T) {
	orig := NewItemOnGround(GroundItemSeenByPlayer, "Alice", Location{X: 1, Y: 2, Z: 0}, 995, 4, map[string]any{"n": []any{1}}, true, 7, true, &GroundPickupReplacement{ID: 9, TelegrabDenial: "x"})
	orig.Tick = 40
	orig.AgedCycle = 40
	orig.PendingRemoval = true
	orig.SetDespawnDelay(42)
	if err := orig.SetEnabledOps(0x0a, nil); err != nil {
		t.Fatal(err)
	}
	viewer := testGroundPlayer(1, "Alice", 1, 2, 0)
	if err := orig.SetEnabledOps(0x01, viewer); err != nil {
		t.Fatal(err)
	}
	c := orig.Clone()
	if c.State != orig.State || c.Owner != orig.Owner || c.Position != orig.Position ||
		c.ItemID != orig.ItemID || c.Amount != orig.Amount || c.GoesGlobal != orig.GoesGlobal ||
		c.RespawnTimer != 7 || !c.StaticSpawn || c.RespawnAmount != 4 || c.EffectiveDespawnDelay() != 42 ||
		c.EnabledOps(nil) != 0x0a || c.PickupReplacement == nil || c.PickupReplacement.ID != 9 {
		t.Fatalf("clone=%+v, want copied fields", c)
	}
	if c.Tick != 0 || c.AgedCycle != -1 || c.PendingRemoval || c.ID() != 0 || c.ViewerOverrideCount() != 0 {
		t.Fatalf("clone reset fields: tick=%d aged=%d pending=%v id=%d overrides=%d", c.Tick, c.AgedCycle, c.PendingRemoval, c.ID(), c.ViewerOverrideCount())
	}
	// Meta is a deep copy: mutating the clone leaves the original alone.
	c.Meta["n"].([]any)[0] = 99
	if orig.Meta["n"].([]any)[0] != 1 {
		t.Fatal("clone meta shares storage with original")
	}
}

func TestGroundItemApplySpawn(t *testing.T) {
	sender := &recordingSender{}
	mgr := newTestGroundManager([]*Player{testGroundPlayer(1, "Alice", 3200, 3200, 0)}, sender, nil, nil, nil)
	rec := definition.GroundItem{
		Spawn:   definition.GroundItemSpawn{ID: 995, X: 3200, Y: 3200, Z: 0, Amount: 3, Respawn: 0},
		Members: false,
	}
	it := mgr.ApplySpawn(rec)
	if it.State != GroundItemSeenByEveryone || it.Owner != "" || it.GoesGlobal {
		t.Fatalf("spawn: state=%d owner=%q global=%v", it.State, it.Owner, it.GoesGlobal)
	}
	if it.RespawnTimer != -1 || !it.StaticSpawn || it.ItemID != 995 || it.Amount != 3 {
		t.Fatalf("spawn: timer=%d static=%v id=%d amount=%d", it.RespawnTimer, it.StaticSpawn, it.ItemID, it.Amount)
	}
	if len(mgr.items) != 1 || len(sender.calls) != 1 {
		t.Fatalf("spawn: items=%d sends=%d, want 1/1", len(mgr.items), len(sender.calls))
	}
	respawning := mgr.ApplySpawn(definition.GroundItem{
		Spawn:             definition.GroundItemSpawn{ID: 1234, X: 3201, Y: 3200, Z: 0, Amount: 1, Respawn: 50},
		PickupReplacement: &definition.PickupReplacement{ID: 7, TelegrabDenial: "nope"},
	})
	if respawning.RespawnTimer != 50 || !respawning.Respawns() {
		t.Fatalf("respawning timer=%d", respawning.RespawnTimer)
	}
	if respawning.PickupReplacement == nil || respawning.PickupReplacement.ID != 7 || respawning.PickupReplacement.TelegrabDenial != "nope" {
		t.Fatalf("replacement=%+v", respawning.PickupReplacement)
	}
}

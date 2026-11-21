package game

import (
	"fmt"
	"testing"
)

func intPtr(v int) *int { return &v }

// newVisibilityObserver registers a player with an open fake session at
// loc, via the queue + one tick (the same path production logins take).
func newVisibilityObserver(t *testing.T, w *World, name string, loc Location) *Player {
	t.Helper()
	p := NewPlayer(name, loc)
	p.Session = newFakeSession()
	w.QueueAddPlayer(p)
	w.Process()
	if !p.IsRegistered() {
		t.Fatalf("player %s was not registered", name)
	}
	return p
}

// addPlayerDirect registers a sessionless player immediately through the
// list (no queue tick). Sessionless players are bucketed like any other
// player but are skipped as visibility players (no session to update).
func addPlayerDirect(t *testing.T, w *World, name string, loc Location) *Player {
	t.Helper()
	p := NewPlayer(name, loc)
	if _, ok := w.Players().Add(p); !ok {
		t.Fatalf("direct add of player %s failed", name)
	}
	return p
}

// addNPCDirect registers an NPC immediately through the list.
func addNPCDirect(t *testing.T, w *World, id int, loc Location, visible bool) *NPC {
	t.Helper()
	n := NewNPC(id, loc)
	n.Visible = visible
	if _, ok := w.NPCs().Add(n); !ok {
		t.Fatalf("direct add of npc %d failed", id)
	}
	return n
}

func containsPlayer(list []*Player, want *Player) bool {
	for _, p := range list {
		if p == want {
			return true
		}
	}
	return false
}

func containsNPC(list []*NPC, want *NPC) bool {
	for _, n := range list {
		if n == want {
			return true
		}
	}
	return false
}

func TestUpdateBucketKey(t *testing.T) {
	cases := []struct {
		x, y, z int
		want    string
	}{
		{0, 0, 0, "0:0:0"},
		{7, 7, 0, "0:0:0"},
		{8, 8, 0, "0:1:1"},
		{15, 16, 2, "2:1:2"},
		{3089, 3524, 0, "0:386:440"},
		{-1, -1, 0, "0:-1:-1"},
		{-8, -9, 1, "1:-1:-2"},
	}
	for _, c := range cases {
		if got := updateBucketKey(c.x, c.y, c.z); got != c.want {
			t.Fatalf("updateBucketKey(%d,%d,%d) = %q, want %q", c.x, c.y, c.z, got, c.want)
		}
	}
}

func TestCollectFromBucketsOrderBudgetAndFilter(t *testing.T) {
	buckets := map[string][]string{
		"0:0:0":   {"c1", "c2"}, // ring 0
		"0:-1:-1": {"e2"},       // ring 1
		"0:1:0":   {"e1"},       // ring 1
		"0:1:1":   {"inner"},    // ring 1 edge; interior of ring 2 — must appear once
		"0:2:0":   {"f1"},       // ring 2
	}
	origin := Location{X: 0, Y: 0, Z: 0}
	got := collectFromBuckets(buckets, origin, 2, 100, nil)
	// Ring order: center, then ring-1 edge in (dx, dy) order
	// ((-1,-1), (1,0), (1,1)), then ring 2.
	want := []string{"c1", "c2", "e2", "e1", "inner", "f1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("collect order = %v, want %v", got, want)
	}

	// Budget stops collection mid-walk.
	got = collectFromBuckets(buckets, origin, 2, 3, nil)
	if fmt.Sprint(got) != fmt.Sprint([]string{"c1", "c2", "e2"}) {
		t.Fatalf("budget 3 = %v, want [c1 c2 e2]", got)
	}
	// Non-positive budgets collect nothing.
	if n := len(collectFromBuckets(buckets, origin, 2, 0, nil)); n != 0 {
		t.Fatalf("budget 0 collected %d, want 0", n)
	}
	if n := len(collectFromBuckets(buckets, origin, 2, -5, nil)); n != 0 {
		t.Fatalf("budget -5 collected %d, want 0", n)
	}
	// Radius 0 visits only the origin chunk.
	got = collectFromBuckets(buckets, origin, 0, 100, nil)
	if fmt.Sprint(got) != fmt.Sprint([]string{"c1", "c2"}) {
		t.Fatalf("radius 0 = %v, want [c1 c2]", got)
	}
	// The include filter skips entries without consuming budget.
	got = collectFromBuckets(buckets, origin, 2, 100, func(s string) bool { return s != "e1" })
	if fmt.Sprint(got) != fmt.Sprint([]string{"c1", "c2", "e2", "inner", "f1"}) {
		t.Fatalf("filtered = %v, want [c1 c2 e2 inner f1]", got)
	}
}

func TestRebuildUpdateBuckets(t *testing.T) {
	w := newTestWorld()
	a := addPlayerDirect(t, w, "a", Location{X: 100, Y: 100, Z: 0})
	b := addPlayerDirect(t, w, "b", Location{X: 108, Y: 100, Z: 0})
	vis := addNPCDirect(t, w, 1, Location{X: 100, Y: 100, Z: 0}, true)
	inv := addNPCDirect(t, w, 2, Location{X: 100, Y: 100, Z: 0}, false)

	w.RebuildUpdateBuckets()
	if !containsPlayer(w.playerUpdateBuckets["0:12:12"], a) {
		t.Fatal("player a must be bucketed at 0:12:12")
	}
	if !containsPlayer(w.playerUpdateBuckets["0:13:12"], b) {
		t.Fatal("player b must be bucketed at 0:13:12")
	}
	if !containsNPC(w.npcUpdateBuckets["0:12:12"], vis) {
		t.Fatal("visible NPC must be bucketed")
	}
	if containsNPC(w.npcUpdateBuckets["0:12:12"], inv) {
		t.Fatal("invisible NPC must not be bucketed")
	}

	// Rebuild clears stale entries: move a to a new chunk and rebuild.
	a.Loc = Location{X: 200, Y: 200, Z: 0}
	w.RebuildUpdateBuckets()
	if _, ok := w.playerUpdateBuckets["0:12:12"]; ok {
		t.Fatal("old bucket must be gone after rebuild (no stale entries)")
	}
	if !containsPlayer(w.playerUpdateBuckets["0:25:25"], a) {
		t.Fatal("moved player must be bucketed at the new chunk")
	}
}

func TestRebuildIncludesNPCRegisteredThisTick(t *testing.T) {
	w := newTestWorld()
	obs := newVisibilityObserver(t, w, "obs", Location{X: 100, Y: 100, Z: 0})
	n := NewNPC(7, Location{X: 101, Y: 100, Z: 0})
	n.Visible = true
	w.QueueAddNPC(n)
	w.Process() // add phase, then rebuild phase, in the same tick.
	if !containsNPC(obs.LocalNPCs(), n) {
		t.Fatal("NPC registered this tick must be visible to observers this tick")
	}
}

func TestNearbyPlayersRadiusBudgetAndRegistration(t *testing.T) {
	w := newTestWorld()
	obs := newVisibilityObserver(t, w, "obs", Location{X: 100, Y: 100, Z: 0})
	// Chunk +2 (16 tiles): inside the bucket radius for view distance
	// 15 (radius 2 chunks), even though Chebyshev distance 16 exceeds
	// the view distance — Nearby returns candidates, UpdateLocalPlayers
	// applies the exact visibility check.
	edge := addPlayerDirect(t, w, "edge", Location{X: 116, Y: 100, Z: 0})
	far := addPlayerDirect(t, w, "far", Location{X: 124, Y: 100, Z: 0})
	otherPlane := addPlayerDirect(t, w, "plane", Location{X: 100, Y: 100, Z: 1})

	w.RebuildUpdateBuckets()
	nearby := w.NearbyPlayersForUpdate(obs)
	if !containsPlayer(nearby, edge) {
		t.Fatal("chunk+2 candidate must be collected (bucket radius 2)")
	}
	if containsPlayer(nearby, far) {
		t.Fatal("chunk+3 candidate must not be collected")
	}
	if containsPlayer(nearby, otherPlane) {
		t.Fatal("other-plane candidate must not be collected (own plane only)")
	}
	if !containsPlayer(nearby, obs) {
		t.Fatal("observer itself is a candidate (self-exclusion is UpdateLocalPlayers' job)")
	}

	// Fixed view distance widens the collection radius: 64 -> 8 chunks.
	obs.SetFixedViewDistance(intPtr(64))
	nearby = w.NearbyPlayersForUpdate(obs)
	if !containsPlayer(nearby, far) {
		t.Fatal("fixed view distance 64 must collect the chunk+3 candidate")
	}
	// Fixed view distance 1 -> radius 1 chunk: the chunk+2 candidate drops.
	obs.SetFixedViewDistance(intPtr(1))
	nearby = w.NearbyPlayersForUpdate(obs)
	if containsPlayer(nearby, edge) {
		t.Fatal("fixed view distance 1 must not collect the chunk+2 candidate")
	}
	obs.SetFixedViewDistance(nil)
}

func TestNearbyPlayersBudget(t *testing.T) {
	w := newTestWorld()
	obs := newVisibilityObserver(t, w, "obs", Location{X: 100, Y: 100, Z: 0})
	for i := 0; i < 300; i++ {
		addPlayerDirect(t, w, fmt.Sprintf("crowd%d", i), Location{X: 100, Y: 100, Z: 0})
	}
	w.RebuildUpdateBuckets()
	nearby := w.NearbyPlayersForUpdate(obs)
	if len(nearby) != MaxLocalPlayers+MaxNewLocalPlayersPerCycle {
		t.Fatalf("nearby = %d, want budget %d", len(nearby), MaxLocalPlayers+MaxNewLocalPlayersPerCycle)
	}
}

func TestNearbyPlayersExcludesStaleIndex(t *testing.T) {
	w := newTestWorld()
	obs := newVisibilityObserver(t, w, "obs", Location{X: 100, Y: 100, Z: 0})
	stale := addPlayerDirect(t, w, "stale", Location{X: 100, Y: 100, Z: 0})
	w.RebuildUpdateBuckets()
	// Deregister stale and let a new player reuse its slot, without
	// rebuilding: the bucket still holds stale, but the include filter
	// must reject it because the slot now holds someone else.
	w.Players().Remove(stale.Index)
	replacement := NewPlayer("replacement", Location{X: 100, Y: 100, Z: 0})
	if idx, ok := w.Players().Add(replacement); !ok || idx != stale.Index {
		t.Fatalf("replacement must reuse stale index %d (got %d, ok=%v)", stale.Index, idx, ok)
	}
	nearby := w.NearbyPlayersForUpdate(obs)
	if containsPlayer(nearby, stale) {
		t.Fatal("deregistered player must be excluded by the same-index check")
	}
	if containsPlayer(nearby, replacement) {
		t.Fatal("never-bucketed replacement must not appear before a rebuild")
	}
	w.RebuildUpdateBuckets()
	if !containsPlayer(w.NearbyPlayersForUpdate(obs), replacement) {
		t.Fatal("replacement must appear after a rebuild")
	}
}

func TestNearbyNPCsSortRadiusAndViewDistance(t *testing.T) {
	w := newTestWorld()
	obs := newVisibilityObserver(t, w, "obs", Location{X: 100, Y: 100, Z: 0})
	// Register in an order that differs from collection order: index 1
	// sits in a ring-1 chunk, index 3 in the center chunk.
	ring1 := addNPCDirect(t, w, 1, Location{X: 104, Y: 100, Z: 0}, true)
	ring2 := addNPCDirect(t, w, 2, Location{X: 112, Y: 100, Z: 0}, true)
	center := addNPCDirect(t, w, 3, Location{X: 100, Y: 100, Z: 0}, true)
	otherPlane := addNPCDirect(t, w, 4, Location{X: 100, Y: 100, Z: 1}, true)
	addNPCDirect(t, w, 5, Location{X: 100, Y: 100, Z: 0}, false) // invisible

	w.RebuildUpdateBuckets()
	nearby := w.NearbyNPCsForUpdate(obs)
	if len(nearby) != 3 {
		t.Fatalf("nearby NPCs = %d, want 3 (visible, same-plane only)", len(nearby))
	}
	// Sorted index-ascending, not collection order.
	if nearby[0] != ring1 || nearby[1] != ring2 || nearby[2] != center {
		t.Fatalf("nearby NPC order = indices %d,%d,%d, want 1,2,3",
			nearby[0].Index, nearby[1].Index, nearby[2].Index)
	}
	if containsNPC(nearby, otherPlane) {
		t.Fatal("other-plane NPC must not be collected")
	}

	// NPC view distance 1 still collects at the radius floor of 2
	// chunks: the ring-2 NPC stays a candidate.
	obs.SetNpcViewDistance(intPtr(1))
	if !containsNPC(w.NearbyNPCsForUpdate(obs), ring2) {
		t.Fatal("radius floor 2 must keep the ring-2 NPC as a candidate at npcViewDistance 1")
	}
	// Widened NPC view distance reaches far chunks.
	farNPC := addNPCDirect(t, w, 6, Location{X: 200, Y: 100, Z: 0}, true)
	w.RebuildUpdateBuckets()
	if containsNPC(w.NearbyNPCsForUpdate(obs), farNPC) {
		t.Fatal("far NPC must not be collected at npcViewDistance 1")
	}
	obs.SetNpcViewDistance(intPtr(127))
	if !containsNPC(w.NearbyNPCsForUpdate(obs), farNPC) {
		t.Fatal("far NPC must be collected at npcViewDistance 127 (radius 16 chunks)")
	}
}

func TestPlayerViewDistanceDefaultsAndSetters(t *testing.T) {
	p := NewPlayer("v", Location{})
	if p.ViewDistance() != PreferredViewDistance {
		t.Fatalf("default ViewDistance = %d, want %d", p.ViewDistance(), PreferredViewDistance)
	}
	if p.NpcViewDistance() != NPCViewDistance {
		t.Fatalf("default NpcViewDistance = %d, want %d", p.NpcViewDistance(), NPCViewDistance)
	}

	p.SetFixedViewDistance(intPtr(0))
	if p.ViewDistance() != 1 {
		t.Fatalf("fixed 0 must clamp to 1, got %d", p.ViewDistance())
	}
	p.SetFixedViewDistance(intPtr(100))
	if p.ViewDistance() != MaxFixedViewDistance {
		t.Fatalf("fixed 100 must clamp to %d, got %d", MaxFixedViewDistance, p.ViewDistance())
	}
	p.SetFixedViewDistance(intPtr(32))
	if p.ViewDistance() != 32 {
		t.Fatalf("fixed 32 must read back 32, got %d", p.ViewDistance())
	}
	// Resizing adjusts only the adaptive distance; the fixed value wins.
	p.ResizeViewDistance(300)
	if p.ViewDistance() != 32 {
		t.Fatalf("fixed distance must survive resize, got %d", p.ViewDistance())
	}
	p.SetFixedViewDistance(nil)
	if p.ViewDistance() != PreferredViewDistance-1 {
		t.Fatalf("clearing fixed must resume the shrunk adaptive distance 14, got %d", p.ViewDistance())
	}

	p.SetNpcViewDistance(intPtr(0))
	if p.NpcViewDistance() != 1 {
		t.Fatalf("npc 0 must clamp to 1, got %d", p.NpcViewDistance())
	}
	p.SetNpcViewDistance(intPtr(200))
	if p.NpcViewDistance() != MaxNPCViewDistance {
		t.Fatalf("npc 200 must clamp to %d, got %d", MaxNPCViewDistance, p.NpcViewDistance())
	}
	p.SetNpcViewDistance(intPtr(64))
	if p.NpcViewDistance() != 64 {
		t.Fatalf("npc 64 must read back 64, got %d", p.NpcViewDistance())
	}
	p.SetNpcViewDistance(nil)
	if p.NpcViewDistance() != NPCViewDistance {
		t.Fatalf("nil npc distance must reset to %d, got %d", NPCViewDistance, p.NpcViewDistance())
	}
}

func TestResizeViewDistance(t *testing.T) {
	p := NewPlayer("r", Location{})
	// Saturated: shrink once per cycle, down to a floor of 1.
	for i := 0; i < 20; i++ {
		p.ResizeViewDistance(PreferredLocalPlayers)
	}
	if p.ViewDistance() != 1 {
		t.Fatalf("after saturation ViewDistance = %d, want floor 1", p.ViewDistance())
	}
	// 249 locals is quiet, not saturated.
	p.ResizeViewDistance(PreferredLocalPlayers - 1)
	if p.ViewDistance() != 1 {
		t.Fatal("one quiet cycle must not regrow yet")
	}

	// Regrow: 10 quiet cycles per +1.
	q := NewPlayer("q", Location{})
	for i := 0; i < 5; i++ {
		q.ResizeViewDistance(300)
	}
	if q.ViewDistance() != 10 {
		t.Fatalf("after 5 shrinks ViewDistance = %d, want 10", q.ViewDistance())
	}
	for i := 0; i < ViewDistanceRegrowCycles-1; i++ {
		q.ResizeViewDistance(0)
	}
	if q.ViewDistance() != 10 {
		t.Fatalf("after 9 quiet cycles ViewDistance = %d, want 10", q.ViewDistance())
	}
	q.ResizeViewDistance(0)
	if q.ViewDistance() != 11 {
		t.Fatalf("after 10 quiet cycles ViewDistance = %d, want 11", q.ViewDistance())
	}
	// A saturated cycle resets the quiet count.
	for i := 0; i < ViewDistanceRegrowCycles-1; i++ {
		q.ResizeViewDistance(0)
	}
	q.ResizeViewDistance(300) // shrinks to 10, quiet count reset
	if q.ViewDistance() != 10 {
		t.Fatalf("saturated cycle must shrink back to 10, got %d", q.ViewDistance())
	}
	for i := 0; i < ViewDistanceRegrowCycles-1; i++ {
		q.ResizeViewDistance(0)
	}
	if q.ViewDistance() != 10 {
		t.Fatalf("quiet count must have reset: after 9 cycles still 10, got %d", q.ViewDistance())
	}
	q.ResizeViewDistance(0)
	if q.ViewDistance() != 11 {
		t.Fatalf("after 10 fresh quiet cycles ViewDistance = %d, want 11", q.ViewDistance())
	}
	// No overshoot past the preferred distance.
	r := NewPlayer("s", Location{})
	for i := 0; i < 30; i++ {
		r.ResizeViewDistance(0)
	}
	if r.ViewDistance() != PreferredViewDistance {
		t.Fatalf("quiet at preferred must stay %d, got %d", PreferredViewDistance, r.ViewDistance())
	}
}

func TestUpdateLocalPlayersCompaction(t *testing.T) {
	w := newTestWorld()
	obs := newVisibilityObserver(t, w, "obs", Location{X: 100, Y: 100, Z: 0})
	near1 := addPlayerDirect(t, w, "near1", Location{X: 105, Y: 100, Z: 0})
	near2 := addPlayerDirect(t, w, "near2", Location{X: 100, Y: 105, Z: 0})
	farAway := addPlayerDirect(t, w, "farAway", Location{X: 500, Y: 500, Z: 0})
	gone := addPlayerDirect(t, w, "gone", Location{X: 101, Y: 100, Z: 0})
	placing := addPlayerDirect(t, w, "placing", Location{X: 102, Y: 100, Z: 0})
	placing.NeedsPlacement = true
	w.Players().Remove(gone.Index) // deregistered before the update

	// Pre-fill with every drop case plus a duplicate and self.
	obs.localPlayers = []*Player{near1, near2, near2, farAway, gone, placing, obs}
	w.RebuildUpdateBuckets()
	w.UpdateLocalPlayers(obs)
	got := obs.LocalPlayers()
	// Compaction keeps near1/near2 and drops the duplicate, the far
	// player, the deregistered player, the needs-placement player,
	// and self. But the add loop has no needs-placement check
	// (World.ts:778-787), so placing is re-added as a new local in
	// the same tick — the TS quirk, preserved.
	if len(got) != 3 || got[0] != near1 || got[1] != near2 || got[2] != placing {
		t.Fatalf("locals = %d entries, want [near1 near2 placing]", len(got))
	}

	// A kept player that moves out of range drops on the next update
	// (placing is dropped by compaction and re-added again).
	near1.Loc = Location{X: 500, Y: 500, Z: 0}
	w.RebuildUpdateBuckets()
	w.UpdateLocalPlayers(obs)
	if got := obs.LocalPlayers(); len(got) != 2 || got[0] != near2 || got[1] != placing {
		t.Fatalf("after near1 leaves, locals = %d entries, want [near2 placing]", len(got))
	}
	// Clearing NeedsPlacement lets the placing player be kept by
	// compaction itself (no behavioral change in the list, but the
	// keep path now retains it rather than the add loop re-adding it).
	placing.NeedsPlacement = false
	w.RebuildUpdateBuckets()
	w.UpdateLocalPlayers(obs)
	if !containsPlayer(obs.LocalPlayers(), placing) {
		t.Fatal("placing player must stay once NeedsPlacement clears")
	}
}

func TestUpdateLocalPlayersAddCap(t *testing.T) {
	w := newTestWorld()
	obs := newVisibilityObserver(t, w, "obs", Location{X: 100, Y: 100, Z: 0})
	var crowd []*Player
	for i := 0; i < 30; i++ {
		crowd = append(crowd, addPlayerDirect(t, w, fmt.Sprintf("c%d", i), Location{X: 100, Y: 100, Z: 0}))
	}
	w.RebuildUpdateBuckets()
	w.UpdateLocalPlayers(obs)
	if n := len(obs.LocalPlayers()); n != MaxNewLocalPlayersPerCycle {
		t.Fatalf("first update added %d, want cap %d", n, MaxNewLocalPlayersPerCycle)
	}
	if containsPlayer(obs.LocalPlayers(), obs) {
		t.Fatal("observer must never be in its own local list")
	}
	w.RebuildUpdateBuckets()
	w.UpdateLocalPlayers(obs)
	if n := len(obs.LocalPlayers()); n != 30 {
		t.Fatalf("second update must reach all 30, got %d", n)
	}
	for _, c := range crowd {
		if !containsPlayer(obs.LocalPlayers(), c) {
			t.Fatalf("crowd member %s missing after 2 cycles", c.Username)
		}
	}
}

func TestUpdateLocalPlayersCap255(t *testing.T) {
	w := newTestWorld()
	obs := newVisibilityObserver(t, w, "obs", Location{X: 100, Y: 100, Z: 0})
	var crowd []*Player
	for i := 0; i < 260; i++ {
		crowd = append(crowd, addPlayerDirect(t, w, fmt.Sprintf("c%d", i), Location{X: 100, Y: 100, Z: 0}))
	}
	// Pre-fill beyond the cap: compaction must truncate to 255.
	obs.localPlayers = append([]*Player{}, crowd...)
	w.RebuildUpdateBuckets()
	w.UpdateLocalPlayers(obs)
	if n := len(obs.LocalPlayers()); n != MaxLocalPlayers {
		t.Fatalf("locals = %d, want cap %d", n, MaxLocalPlayers)
	}
	for i := 0; i < MaxLocalPlayers; i++ {
		if obs.LocalPlayers()[i] != crowd[i] {
			t.Fatalf("locals[%d] wrong after truncation", i)
		}
	}
}

func TestUpdateLocalNPCs(t *testing.T) {
	w := newTestWorld()
	obs := newVisibilityObserver(t, w, "obs", Location{X: 100, Y: 100, Z: 0})
	near := addNPCDirect(t, w, 1, Location{X: 105, Y: 100, Z: 0}, true)
	hidden := addNPCDirect(t, w, 2, Location{X: 101, Y: 100, Z: 0}, false)
	placing := addNPCDirect(t, w, 3, Location{X: 102, Y: 100, Z: 0}, true)
	placing.NeedsPlacement = true
	farNPC := addNPCDirect(t, w, 4, Location{X: 500, Y: 500, Z: 0}, true)
	gone := addNPCDirect(t, w, 5, Location{X: 103, Y: 100, Z: 0}, true)
	w.NPCs().Remove(gone.Index)

	obs.localNPCs = []*NPC{near, hidden, placing, farNPC, gone, near}
	w.RebuildUpdateBuckets()
	w.UpdateLocalNPCs(obs, w.NearbyNPCsForUpdate(obs))
	got := obs.LocalNPCs()
	if len(got) != 1 || got[0] != near {
		t.Fatalf("locals = %d entries, want [near]", len(got))
	}
	// The placing NPC must not be re-added by the add pass while it
	// needs placement (staysInViewWhenPlaced is always false in Go).
	if containsNPC(got, placing) {
		t.Fatal("needs-placement NPC must not be re-added by the add pass")
	}
	// Becoming visible / clearing placement lets them join.
	hidden.Visible = true
	placing.NeedsPlacement = false
	w.RebuildUpdateBuckets()
	w.UpdateLocalNPCs(obs, w.NearbyNPCsForUpdate(obs))
	if !containsNPC(obs.LocalNPCs(), hidden) || !containsNPC(obs.LocalNPCs(), placing) {
		t.Fatal("visible/placed NPCs must join on the next update")
	}
	if containsNPC(obs.LocalNPCs(), farNPC) || containsNPC(obs.LocalNPCs(), gone) {
		t.Fatal("far/deregistered NPCs must stay out")
	}
}

func TestUpdateLocalNPCsCap255(t *testing.T) {
	w := newTestWorld()
	obs := newVisibilityObserver(t, w, "obs", Location{X: 100, Y: 100, Z: 0})
	for i := 0; i < 260; i++ {
		addNPCDirect(t, w, i+1, Location{X: 100, Y: 100, Z: 0}, true)
	}
	w.RebuildUpdateBuckets()
	w.UpdateLocalNPCs(obs, w.NearbyNPCsForUpdate(obs))
	if n := len(obs.LocalNPCs()); n != MaxLocalNPCs {
		t.Fatalf("local NPCs = %d, want cap %d", n, MaxLocalNPCs)
	}
}

func TestWorldProcessVisibilityIntegration(t *testing.T) {
	w := newTestWorld()
	spawn := Location{X: 100, Y: 100, Z: 0}
	obs := newVisibilityObserver(t, w, "obs", spawn)
	near := newVisibilityObserver(t, w, "near", Location{X: 105, Y: 100, Z: 0})
	far := newVisibilityObserver(t, w, "far", Location{X: 600, Y: 600, Z: 0})
	nearNPC := addNPCDirect(t, w, 1, Location{X: 110, Y: 100, Z: 0}, true)
	addNPCDirect(t, w, 2, Location{X: 600, Y: 600, Z: 0}, true)  // far NPC
	addNPCDirect(t, w, 3, Location{X: 101, Y: 100, Z: 0}, false) // invisible
	addNPCDirect(t, w, 4, Location{X: 101, Y: 100, Z: 1}, true)  // other plane

	w.Process()
	if !containsPlayer(obs.LocalPlayers(), near) {
		t.Fatal("near player must be in the observer's local list after a tick")
	}
	if !containsPlayer(near.LocalPlayers(), obs) {
		t.Fatal("visibility must be mutual for two near players")
	}
	if containsPlayer(obs.LocalPlayers(), far) || containsPlayer(obs.LocalPlayers(), obs) {
		t.Fatal("far player and self must not be in the observer's local list")
	}
	if len(far.LocalPlayers()) != 0 {
		t.Fatal("far player must see nobody")
	}
	if got := obs.LocalNPCs(); len(got) != 1 || got[0] != nearNPC {
		t.Fatalf("observer local NPCs = %d, want only the near visible same-plane NPC", len(got))
	}

	// Leaving range drops the player on the next tick; returning
	// re-adds them.
	near.Loc = Location{X: 600, Y: 600, Z: 0}
	w.Process()
	if containsPlayer(obs.LocalPlayers(), near) {
		t.Fatal("player that left range must drop from the local list")
	}
	near.Loc = Location{X: 105, Y: 100, Z: 0}
	w.Process()
	if !containsPlayer(obs.LocalPlayers(), near) {
		t.Fatal("player that returned must rejoin the local list")
	}
}

func TestWorldProcessVisibilityGate(t *testing.T) {
	w := newTestWorld()
	obs := newVisibilityObserver(t, w, "obs", Location{X: 100, Y: 100, Z: 0})
	// Sessionless players are bucketed (observers see them) but get no
	// local-list maintenance of their own.
	ghost := addPlayerDirect(t, w, "ghost", Location{X: 101, Y: 100, Z: 0})
	closed := newVisibilityObserver(t, w, "closed", Location{X: 102, Y: 100, Z: 0})
	closed.Session.Close(1001, "gone")
	// The closed player built a list while its session was open.
	// Clearing it here isolates the gate: after the tick it must still
	// be empty (no maintenance ran), and the gate must not clear a
	// stale list either (see below).
	closed.localPlayers = nil
	closed.localNPCs = nil
	w.Process()
	if !containsPlayer(obs.LocalPlayers(), ghost) {
		t.Fatal("observer must see the sessionless player (buckets include all players)")
	}
	if len(ghost.LocalPlayers()) != 0 {
		t.Fatal("sessionless player must get no visibility maintenance")
	}
	if len(closed.LocalPlayers()) != 0 {
		t.Fatal("closed-session player must get no visibility maintenance")
	}
	// Stale lists are left untouched, not cleared: give the closed
	// player a stale entry, move everyone away, and confirm the entry
	// survives the tick (maintenance would have dropped it).
	closed.localPlayers = []*Player{obs}
	obs.Loc = Location{X: 900, Y: 900, Z: 0}
	w.Process()
	if got := closed.LocalPlayers(); len(got) != 1 || got[0] != obs {
		t.Fatal("closed-session player's stale list must be left untouched by the gate")
	}
}

func TestWorldProcessViewDistanceShrinkAndRegrow(t *testing.T) {
	w := newTestWorld()
	obs := newVisibilityObserver(t, w, "obs", Location{X: 100, Y: 100, Z: 0})
	var crowd []*Player
	for i := 0; i < PreferredLocalPlayers; i++ {
		crowd = append(crowd, addPlayerDirect(t, w, fmt.Sprintf("c%d", i), Location{X: 100, Y: 100, Z: 0}))
	}
	obs.localPlayers = append([]*Player{}, crowd...)
	w.Process()
	if obs.ViewDistance() != PreferredViewDistance-1 {
		t.Fatalf("saturated view must shrink to 14, got %d", obs.ViewDistance())
	}
	// Empty the view: 10 quiet ticks regrow by one.
	for _, c := range crowd {
		w.Players().Remove(c.Index)
	}
	for i := 0; i < ViewDistanceRegrowCycles-1; i++ {
		w.Process()
	}
	if obs.ViewDistance() != PreferredViewDistance-1 {
		t.Fatalf("after 9 quiet ticks view must still be 14, got %d", obs.ViewDistance())
	}
	w.Process()
	if obs.ViewDistance() != PreferredViewDistance {
		t.Fatalf("after 10 quiet ticks view must regrow to 15, got %d", obs.ViewDistance())
	}
}

func TestLocalListsRefetchEachTick(t *testing.T) {
	w := newTestWorld()
	obs := newVisibilityObserver(t, w, "obs", Location{X: 100, Y: 100, Z: 0})
	stale := obs.LocalPlayers()
	if len(stale) != 0 {
		t.Fatal("fresh observer must have no locals")
	}
	newVisibilityObserver(t, w, "near", Location{X: 101, Y: 100, Z: 0})
	w.Process()
	if len(stale) != 0 {
		t.Fatal("slice fetched before the tick must not gain entries (stale header)")
	}
	if len(obs.LocalPlayers()) != 1 {
		t.Fatal("re-fetching after the tick must show the new local")
	}
}

func TestWorldVisibilityHookOverride(t *testing.T) {
	w := newTestWorld()
	obs := newVisibilityObserver(t, w, "obs", Location{X: 100, Y: 100, Z: 0})
	near := newVisibilityObserver(t, w, "near", Location{X: 101, Y: 100, Z: 0})

	// Both hooks set: hooks run, real implementations do not. Clear
	// the lists first: the registration ticks already ran the real
	// update, and hooks suppress future runs without clearing lists.
	// Move near away first: with no rebuild, the buckets keep their
	// stale registration-tick contents and still return near.
	near.Loc = Location{X: 900, Y: 900, Z: 0}
	bucketsRan, updateRan := false, false
	w.Hooks.VisibilityBuckets = func(cycle int64) { bucketsRan = true }
	w.Hooks.UpdatePlayersNPCs = func(cycle int64) { updateRan = true }
	obs.localPlayers = nil
	w.Process()
	if !bucketsRan || !updateRan {
		t.Fatal("both visibility hooks must run when installed")
	}
	if len(obs.LocalPlayers()) != 0 {
		t.Fatal("hook override must suppress the real local-list update")
	}
	if !containsPlayer(w.NearbyPlayersForUpdate(obs), near) {
		t.Fatal("stale buckets must survive: the bucket hook suppresses the rebuild")
	}

	// Only the bucket hook set: with the bucket maps emptied, the real
	// update phase finds no candidates.
	w.Hooks.UpdatePlayersNPCs = nil
	w.playerUpdateBuckets = make(map[string][]*Player)
	w.npcUpdateBuckets = make(map[string][]*NPC)
	w.Process()
	if len(obs.LocalPlayers()) != 0 {
		t.Fatal("with empty buckets, the real update phase must find nobody")
	}

	// Only the update hook set: buckets rebuild (near is back in range
	// and collected), lists stay untouched.
	near.Loc = Location{X: 101, Y: 100, Z: 0}
	w.Hooks.VisibilityBuckets = nil
	w.Hooks.UpdatePlayersNPCs = func(cycle int64) { updateRan = true }
	w.Process()
	if !containsPlayer(w.NearbyPlayersForUpdate(obs), near) {
		t.Fatal("with no bucket hook, the real rebuild must run")
	}
	if len(obs.LocalPlayers()) != 0 {
		t.Fatal("update hook must suppress the real local-list update")
	}

	// No hooks: the real implementations run.
	w.Hooks.UpdatePlayersNPCs = nil
	w.Process()
	if len(obs.LocalPlayers()) != 1 {
		t.Fatal("with no hooks, the real visibility update must run")
	}
}

func TestWorldVisibilityPerPlayerFailureIsolation(t *testing.T) {
	w := newTestWorld()
	victim := newVisibilityObserver(t, w, "victim", Location{X: 100, Y: 100, Z: 0})
	healthy := newVisibilityObserver(t, w, "healthy", Location{X: 101, Y: 100, Z: 0})
	newVisibilityObserver(t, w, "near", Location{X: 102, Y: 100, Z: 0})

	// Fault injection: a nil entry in the victim's local list. Nil
	// entries cannot occur through production paths (buckets are built
	// from the registered lists, which never hold nil); TS throws the
	// same way on one, and the per-player guard must isolate it.
	victim.localPlayers = []*Player{nil}

	var reported []*Player
	var phases []string
	w.Hooks.OnPlayerError = func(p *Player, phase string, err error) {
		reported = append(reported, p)
		phases = append(phases, phase)
	}
	w.Process()

	if len(reported) != 1 || reported[0] != victim || phases[0] != "update_players_npcs" {
		t.Fatalf("OnPlayerError reports = %v/%v, want [victim]/[update_players_npcs]", reported, phases)
	}
	// The healthy player's visibility update still ran in the same tick.
	if len(healthy.LocalPlayers()) != 2 {
		t.Fatalf("healthy player locals = %d, want 2 (failure must not abort others)", len(healthy.LocalPlayers()))
	}
	// The flush phase still ran after the failed update phase.
	if len(healthy.Session.(*fakeSession).flushes) == 0 {
		t.Fatal("flush phase must still run after a visibility failure")
	}
	// The victim was queued for removal and leaves on the next tick.
	w.Process()
	if victim.IsRegistered() {
		t.Fatal("visibility failure must queue the player for removal")
	}
}

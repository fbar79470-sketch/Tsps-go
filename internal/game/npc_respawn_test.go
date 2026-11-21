package game

import (
	"testing"

	"tsps-go/internal/definition"
)

// recordingScheduler records ScheduleRespawn calls for
// BeginRespawn tests.
type recordingScheduler struct {
	calls []struct {
		npc   *NPC
		ticks int
	}
}

func (s *recordingScheduler) ScheduleRespawn(n *NPC, ticks int) {
	s.calls = append(s.calls, struct {
		npc   *NPC
		ticks int
	}{n, ticks})
}

func installRecordingScheduler(t *testing.T) *recordingScheduler {
	t.Helper()
	s := &recordingScheduler{}
	SetNpcRespawnScheduler(s)
	t.Cleanup(func() { SetNpcRespawnScheduler(nil) })
	return s
}

func TestBeginRespawnUsesDefinitionDelay(t *testing.T) {
	s := installRecordingScheduler(t)
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	d := defaultFakeDef()
	d.respawn = 40
	n.SetDefinition(d)
	n.BeginRespawn()
	if len(s.calls) != 1 || s.calls[0].npc != n || s.calls[0].ticks != 40 {
		t.Fatalf("calls = %+v, want one call with 40 ticks", s.calls)
	}
}

func TestBeginRespawnDefaultDelay(t *testing.T) {
	s := installRecordingScheduler(t)
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	n.BeginRespawn() // default definition: respawn 25
	if len(s.calls) != 1 || s.calls[0].ticks != 25 {
		t.Fatalf("calls = %+v, want one call with 25 ticks", s.calls)
	}
}

func TestBeginRespawnZeroDelayQueuesNothing(t *testing.T) {
	s := installRecordingScheduler(t)
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	d := defaultFakeDef()
	d.respawn = 0
	n.SetDefinition(d)
	n.BeginRespawn()
	n.BeginRespawnWithTicks(0)
	n.BeginRespawnWithTicks(-5)
	if len(s.calls) != 0 {
		t.Fatalf("calls = %+v, want none", s.calls)
	}
}

func TestBeginRespawnSkipFlag(t *testing.T) {
	s := installRecordingScheduler(t)
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	n.SetSkipDefaultRespawn(true)
	n.BeginRespawn()
	if len(s.calls) != 0 {
		t.Fatalf("skipped respawn scheduled: %+v", s.calls)
	}
	// The flag is one-shot: the next death respawns normally.
	n.BeginRespawn()
	if len(s.calls) != 1 {
		t.Fatalf("calls = %+v, want one after the flag cleared", s.calls)
	}
}

func TestBeginRespawnNoSchedulerIsSafe(t *testing.T) {
	SetNpcRespawnScheduler(nil)
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	n.BeginRespawn() // must not panic with no wiring installed
}

func TestCloneKeepsSpawnState(t *testing.T) {
	spawn := Location{X: 100, Y: 100, Z: 0}
	owner := NewPlayer("alice", Location{X: 101, Y: 100, Z: 0})
	lineage := NewNpcSpawnLineage("test-source")
	n := NewNPC(42, spawn)
	n.SetDefinition(aggressiveFakeDef())
	n.SetFace(DirNorth)
	n.MovementCoordinator().SetRadius(3)
	n.SetOwner(owner)
	n.SetOwnerOnly(true)
	n.SetSpawnLineage(lineage)
	n.SetUniqueKey("respawn-clone-key")
	n.Visible = true
	// Life state that must NOT carry over.
	n.SetLocation(Location{X: 105, Y: 103, Z: 0})
	n.Hitpoints = 3
	n.SetScriptedMovement(true)
	n.SetMultiCombat(true)
	n.SetPet(true)
	n.SetAttribute("session:scratch", "gone")
	n.GetTimers().Registers(TimerKeyFreeze, 10)

	clone := n.Clone()
	if clone == n {
		t.Fatal("Clone returned the same NPC")
	}
	if clone.ID != 42 || !clone.Spawn.Equals(spawn) || !clone.Location().Equals(spawn) {
		t.Errorf("clone identity/spawn = %d %+v %+v", clone.ID, clone.Spawn, clone.Location())
	}
	if clone.GetFace() != DirNorth || clone.Facing != DirNorth {
		t.Errorf("clone face = %v/%v, want NORTH", clone.GetFace(), clone.Facing)
	}
	if got := clone.MovementCoordinator().GetRadius(); got != 3 {
		t.Errorf("clone radius = %d, want 3", got)
	}
	if clone.Owner() != owner || !clone.IsOwnerOnly() {
		t.Error("clone lost owner/ownerOnly scope")
	}
	if clone.SpawnLineage() != lineage {
		t.Error("clone lost the spawn lineage")
	}
	if key, ok := clone.UniqueKey(); !ok || key != "respawn-clone-key" {
		t.Errorf("clone unique key = %q, %v", key, ok)
	}
	if !clone.Visible {
		t.Error("clone must be visible")
	}
	if clone.Definition() != n.Definition() {
		t.Error("clone must share the definition")
	}
	if clone.Hitpoints != 10 {
		t.Errorf("clone hitpoints = %d, want the definition's 10", clone.Hitpoints)
	}
	if clone.IsScriptedMovement() || clone.IsMultiCombat() || clone.IsPet() {
		t.Error("clone carried over scripted/multi/pet state")
	}
	if clone.HasAttribute("session:scratch") {
		t.Error("clone carried over attributes")
	}
	if clone.GetTimers().Has(TimerKeyFreeze) {
		t.Error("clone carried over timers")
	}
	if clone.GetFollowing() != nil || clone.GetInteractingMobile() != nil {
		t.Error("clone carried over following/interaction")
	}
}

func TestSpawnLineage(t *testing.T) {
	l := NewNpcSpawnLineage("my-spawn")
	if l.Owner() != "my-spawn" || l.IsRetired() {
		t.Errorf("lineage = %q retired=%v", l.Owner(), l.IsRetired())
	}
	l.Retire()
	if !l.IsRetired() {
		t.Error("Retire did not mark retired")
	}
	l.Retire() // twice is harmless
	var nilLineage *NpcSpawnLineage
	if nilLineage.Owner() != "" || nilLineage.IsRetired() {
		t.Error("nil lineage must read as live with no owner")
	}
	nilLineage.Retire() // must not panic
	if NpcSpawnsLineage() != NpcSpawnsLineage() {
		t.Error("NpcSpawnsLineage must return the shared singleton")
	}
	if NpcSpawnsLineage().Owner() != NpcSpawnsLineageOwner {
		t.Errorf("shared lineage owner = %q, want %q", NpcSpawnsLineage().Owner(), NpcSpawnsLineageOwner)
	}
}

func TestUniqueClaim(t *testing.T) {
	// Distinct keys per test: the holder map is package-global.
	const key = "unique-claim-key"
	a := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	b := NewNPC(2, Location{X: 10, Y: 10, Z: 0})
	a.SetRegistered(true)
	b.SetRegistered(true)
	if !ClaimUnique(key, a) {
		t.Fatal("first claim must succeed")
	}
	if got := UniqueHolder(key); got != a {
		t.Error("UniqueHolder did not return the claimer")
	}
	if ClaimUnique(key, b) {
		t.Error("second claim must fail while the holder is live")
	}
	if !ClaimUnique(key, a) {
		t.Error("re-claim by the same holder must succeed")
	}
	// The holder leaves: the claim is pruned on observation,
	// and a new NPC can claim.
	a.SetRegistered(false)
	if got := UniqueHolder(key); got != nil {
		t.Error("stale holder must be pruned")
	}
	if !ClaimUnique(key, b) {
		t.Error("claim must succeed after the holder left")
	}
	if got := UniqueHolder(key); got != b {
		t.Error("UniqueHolder did not return the new holder")
	}
	if ClaimUnique("nil-npc-key", nil) {
		t.Error("claiming for a nil NPC must fail")
	}
}

func TestRespawnTaskFiresAfterDelay(t *testing.T) {
	original := NewNPC(7, Location{X: 50, Y: 50, Z: 0})
	original.SetDefinition(defaultFakeDef())
	var added []*NPC
	task := newNPCRespawnTask(original, 3, func(n *NPC) { added = append(added, n) })
	if task.Name != "NPCRespawnTask" {
		t.Errorf("task name = %q, want NPCRespawnTask", task.Name)
	}
	tasks := NewTaskManager()
	tasks.Schedule(task)
	tasks.Process(1)
	tasks.Process(2)
	if len(added) != 0 {
		t.Fatal("clone added before the delay elapsed")
	}
	tasks.Process(3)
	if len(added) != 1 {
		t.Fatalf("added = %d, want 1 after 3 ticks", len(added))
	}
	if added[0].ID != 7 || !added[0].Location().Equals(original.Spawn) {
		t.Errorf("clone = %d at %+v", added[0].ID, added[0].Location())
	}
	// The task is done: no second clone on later ticks.
	tasks.Process(4)
	if len(added) != 1 {
		t.Fatalf("added = %d, want still 1", len(added))
	}
}

func TestRespawnTaskRetiredLineageDrops(t *testing.T) {
	lineage := NewNpcSpawnLineage("retire-test")
	original := NewNPC(7, Location{X: 50, Y: 50, Z: 0})
	original.SetSpawnLineage(lineage)
	var added []*NPC
	tasks := NewTaskManager()
	tasks.Schedule(newNPCRespawnTask(original, 1, func(n *NPC) { added = append(added, n) }))
	lineage.Retire()
	tasks.Process(1)
	if len(added) != 0 {
		t.Error("retired lineage must queue no respawn")
	}
}

func TestRespawnTaskUniqueConflictDrops(t *testing.T) {
	const key = "respawn-unique-key"
	holder := NewNPC(9, Location{X: 50, Y: 50, Z: 0})
	holder.SetRegistered(true)
	if !ClaimUnique(key, holder) {
		t.Fatal("setup claim failed")
	}
	original := NewNPC(7, Location{X: 50, Y: 50, Z: 0})
	original.SetUniqueKey(key)
	var added []*NPC
	tasks := NewTaskManager()
	tasks.Schedule(newNPCRespawnTask(original, 1, func(n *NPC) { added = append(added, n) }))
	tasks.Process(1)
	if len(added) != 0 {
		t.Error("a live unique holder must keep the respawn single")
	}
	// Once the holder leaves, a fresh respawn claims and adds.
	holder.SetRegistered(false)
	tasks.Schedule(newNPCRespawnTask(original, 1, func(n *NPC) { added = append(added, n) }))
	tasks.Process(2)
	if len(added) != 1 {
		t.Fatalf("added = %d, want 1 after the holder left", len(added))
	}
}

func TestWorldRespawnWiringEndToEnd(t *testing.T) {
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	w.InstallNpcRespawnWiring()
	t.Cleanup(func() {
		SetNpcRespawnScheduler(nil)
		SetUniqueHolderLiveness(nil)
	})
	original := NewNPC(11, Location{X: 3090, Y: 3524, Z: 0})
	original.SetDefinition(defaultFakeDef())
	w.QueueAddNPC(original)
	w.Process()
	if w.NPCs().Len() != 1 {
		t.Fatalf("world NPCs = %d, want 1", w.NPCs().Len())
	}
	original.BeginRespawnWithTicks(3)
	w.Process()
	w.Process()
	if w.NPCs().Len() != 1 {
		t.Fatalf("world NPCs = %d after 2 ticks, want 1 (no early clone)", w.NPCs().Len())
	}
	w.Process()
	if w.NPCs().Len() != 2 {
		t.Fatalf("world NPCs = %d after 3 ticks, want 2 (clone joined)", w.NPCs().Len())
	}
	var clone *NPC
	for _, n := range w.NPCs().All() {
		if n != original {
			clone = n
		}
	}
	if clone == nil {
		t.Fatal("clone not found in the world")
	}
	if clone.ID != 11 || !clone.Location().Equals(original.Spawn) {
		t.Errorf("clone = %d at %+v", clone.ID, clone.Location())
	}
	if !clone.IsRegistered() {
		t.Error("clone must be registered")
	}
}

func TestSpawnWiringInstallsDriverInputs(t *testing.T) {
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	def := aggressiveFakeDef()
	spawns := []definition.NpcSpawn{
		{ID: 5, X: 3090, Y: 3524, Z: 0, Facing: 1, WanderRadius: 4, Name: "hunter"},
	}
	SpawnNPCsWithHitpointsAndDefinitions(w, spawns, func(int) int { return 10 }, func(id int) NPCDefinition {
		if id == 5 {
			return def
		}
		return nil
	})
	w.Process()
	npcs := w.NPCs().All()
	if len(npcs) != 1 {
		t.Fatalf("spawned %d NPCs, want 1", len(npcs))
	}
	n := npcs[0]
	if n.GetFace() != DirNorth || n.Facing != DirNorth {
		t.Errorf("spawn face = %v/%v, want NORTH (facing id 1)", n.GetFace(), n.Facing)
	}
	if got := n.MovementCoordinator().GetRadius(); got != 4 {
		t.Errorf("spawn radius = %d, want 4", got)
	}
	if n.SpawnLineage() != NpcSpawnsLineage() {
		t.Error("spawned NPC must carry the shared spawn-file lineage")
	}
	if n.Definition() != NPCDefinition(def) {
		t.Error("spawned NPC must carry the installed definition")
	}
	if !n.Visible {
		t.Error("spawned NPC must be visible")
	}
}

func TestSpawnWiringWithoutDefinitionsKeepsDefault(t *testing.T) {
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	spawns := []definition.NpcSpawn{
		{ID: 6, X: 3090, Y: 3524, Z: 0, Facing: 6, WanderRadius: 0},
	}
	SpawnNPCsWithHitpoints(w, spawns, func(int) int { return 10 })
	w.Process()
	npcs := w.NPCs().All()
	if len(npcs) != 1 {
		t.Fatalf("spawned %d NPCs, want 1", len(npcs))
	}
	n := npcs[0]
	if n.Definition().IsAggressive() || n.Definition().Respawn() != 25 {
		t.Error("without a definition source the NPC must keep the DEFAULT definition")
	}
	if n.GetFace() != DirSouth {
		t.Errorf("spawn face = %v, want SOUTH (facing id 6)", n.GetFace())
	}
	if got := n.MovementCoordinator().GetRadius(); got != 0 {
		t.Errorf("spawn radius = %d, want 0", got)
	}
}

// TestCloneClampsDegenerateDefinitionHitpoints pins the
// coordinator clamp in Clone: a definition reporting 0 (or
// negative) hitpoints — possible from a custom-definition
// overlay or the monsters dump — must not produce an
// unattackable respawn; the clone starts at
// DefaultNPCHitpoints, mirroring the spawn path.
func TestCloneClampsDegenerateDefinitionHitpoints(t *testing.T) {
	for _, hp := range []int{0, -5} {
		n := NewNPC(42, Location{X: 100, Y: 100, Z: 0})
		d := defaultFakeDef()
		d.hitpoints = hp
		n.SetDefinition(d)
		clone := n.Clone()
		if clone.Hitpoints != DefaultNPCHitpoints {
			t.Errorf("definition hp %d: clone hitpoints = %d, want %d",
				hp, clone.Hitpoints, DefaultNPCHitpoints)
		}
	}
}

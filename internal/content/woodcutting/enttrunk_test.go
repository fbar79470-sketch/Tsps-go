package woodcutting

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- Ent trunk fixtures ----

// newTestEntTrunk builds the shared woodcutting instance's Ent
// unit over fixture tables, with a capturing emit func and a
// fake note-id mapping (log id + 1000), so noted delivery is
// observable without a cache.
func newTestEntTrunk(t *testing.T) (*woodcutting, *entTrunk, *[]emittedEvent) {
	t.Helper()
	tables := loadFixtureTables(t)
	w, emitted := newTestWoodcutting(t, tables)
	e := w.ent
	e.noteID = func(id int) int { return id + 1000 }
	return w, e, emitted
}

// entPlayer arms a player with a bronze axe at Woodcutting 99.
func entPlayer(t *testing.T, tables *Tables, name string, x, y int) *game.Player {
	t.Helper()
	p := newTestPlayer(name, x, y)
	setLevel(p, game.SkillWoodcutting, 99)
	bronze := axeByItem(tables, "bronze_axe")
	if bronze == nil {
		t.Fatal("fixture tables missing the bronze axe")
	}
	p.Inventory().Adds(bronze.ID, 1)
	return p
}

// killEnt kills an Ent through the unit: returns the NPC and
// the death event entDied read (its Remains included).
func killEnt(t *testing.T, e *entTrunk, killer *game.Player, npcID, x, y int) (*game.NPC, *plugins.NpcDeathEvent) {
	t.Helper()
	npc := game.NewNPC(npcID, game.Location{X: x, Y: y, Z: 0})
	npc.SetRegistered(true)
	event := &plugins.NpcDeathEvent{
		Killer: killer, NPC: npc, NpcID: npcID, Location: npc.Location(),
	}
	e.entDied(event)
	return npc, event
}

// makeEntTrunk kills an Ent and ticks to its transform, so the
// trunk is live: returns the trunk NPC.
func makeEntTrunk(t *testing.T, e *entTrunk, killer *game.Player, npcID int) *game.NPC {
	t.Helper()
	npc, _ := killEnt(t, e, killer, npcID, 3200, 3200)
	e.tick()
	e.tick()
	if got := npc.ContentID(); got != Ents.TrunkID {
		t.Fatalf("after 2 ticks: trunk content id = %d, want %d", got, Ents.TrunkID)
	}
	return npc
}

// fillInventory fills every inventory slot with a distinct
// fake id, so GetFreeSlots reads 0.
func fillInventory(p *game.Player) {
	for i := 0; i < 28; i++ {
		p.Inventory().Adds(9000+i, 1)
	}
}

// TestEntDiedSetsRemains pins entDied's remains
// (EntTrunk.Woodcutting.js:59-67): only the two Ent ids,
// only for a player killer; remains ticks are
// transformTicks-2 + trunkTicks (2 + 101 = 103) and the
// respawn ticks are the Ent row's (50 guild, 15 wild).
func TestEntDiedSetsRemains(t *testing.T) {
	_, e, _ := newTestEntTrunk(t)
	p := newTestPlayer("alice", 3200, 3200)
	_, guildEvent := killEnt(t, e, p, 7234, 3200, 3200)
	if guildEvent.Remains == nil || guildEvent.Remains.Ticks != 103 || guildEvent.Remains.RespawnTicks != 50 || !guildEvent.Remains.HasRespawnTicks {
		t.Errorf("guild remains = %+v, want {103 50 true}", guildEvent.Remains)
	}
	_, wildEvent := killEnt(t, e, p, 6594, 3200, 3200)
	if wildEvent.Remains == nil || wildEvent.Remains.Ticks != 103 || wildEvent.Remains.RespawnTicks != 15 || !wildEvent.Remains.HasRespawnTicks {
		t.Errorf("wilderness remains = %+v, want {103 15 true}", wildEvent.Remains)
	}
	// A non-Ent NPC sets no remains.
	goblin := game.NewNPC(3030, game.Location{X: 3200, Y: 3200, Z: 0})
	goblin.SetRegistered(true)
	other := &plugins.NpcDeathEvent{Killer: p, NPC: goblin, NpcID: 3030, Location: goblin.Location()}
	e.entDied(other)
	if other.Remains != nil {
		t.Errorf("non-Ent remains = %+v, want nil", other.Remains)
	}
	// A nil killer sets no remains and schedules nothing.
	pendingBefore := len(e.pending)
	npc := game.NewNPC(7234, game.Location{X: 3200, Y: 3200, Z: 0})
	npc.SetRegistered(true)
	noKiller := &plugins.NpcDeathEvent{NPC: npc, NpcID: 7234, Location: npc.Location()}
	e.entDied(noKiller)
	if noKiller.Remains != nil || len(e.pending) != pendingBefore {
		t.Errorf("nil killer: remains = %+v, pending %d (want nil, %d)", noKiller.Remains, len(e.pending), pendingBefore)
	}
}

// TestEntTransformAfterTwoTicks pins the +2-tick transform
// (EntTrunk.Woodcutting.js:57-66): nothing after one tick;
// after two, HP 1, HP locked, transformed to the trunk, and
// the trunk registered until now+101.
func TestEntTransformAfterTwoTicks(t *testing.T) {
	tables := loadFixtureTables(t)
	_, e, _ := newTestEntTrunk(t)
	p := entPlayer(t, tables, "alice", 3200, 3200)
	npc, _ := killEnt(t, e, p, 7234, 3200, 3200)
	e.tick()
	if npc.Hitpoints == 1 || npc.ContentID() == Ents.TrunkID {
		t.Error("tick 1: want no transform yet")
	}
	e.tick()
	if npc.Hitpoints != 1 {
		t.Errorf("trunk hitpoints = %d, want 1", npc.Hitpoints)
	}
	if !npc.IsHitpointsLocked() {
		t.Error("trunk: want hitpoints locked")
	}
	if got := npc.ContentID(); got != Ents.TrunkID {
		t.Errorf("trunk content id = %d, want %d", got, Ents.TrunkID)
	}
	trunk, ok := e.trunks[npc]
	if !ok {
		t.Fatal("want the trunk registered after 2 ticks")
	}
	if trunk.killer != p || trunk.ent.Where != "Woodcutting Guild" {
		t.Errorf("trunk = %+v, want killer alice / guild", trunk)
	}
	if trunk.until != e.cycle+int64(Ents.TrunkTicks) {
		t.Errorf("trunk until = %d, want cycle+101 = %d", trunk.until, e.cycle+101)
	}
	// A locked trunk shows hits but keeps its HP
	// (Mobile.ts:732-735, decrementHealth).
	npc.GetCombat()
	game.DecrementHealth(&npc.Mobile, game.NewHitDamage(5, game.HitMaskRed))
	if npc.Hitpoints != 1 {
		t.Errorf("locked trunk hitpoints after a 5 hit = %d, want 1", npc.Hitpoints)
	}
}

// TestEntRollLogWeighting pins rollLog
// (EntTrunk.Woodcutting.js:69-81): level gating plus the
// per-axe-tier magic bonus. Values are hand-computed from
// the table, not copied from the implementation.
func TestEntRollLogWeighting(t *testing.T) {
	tables := loadFixtureTables(t)
	bronze := axeByItem(tables, "bronze_axe")
	dragon := axeByItem(tables, "dragon_axe")
	if bronze == nil || dragon == nil {
		t.Fatal("fixture tables missing an axe")
	}
	// Bronze tier 0, level 99: Logs is out (max 44); Oak 4,
	// Willow 1, Maple 1, Yew 1, Magic 1, total 8.
	// Level 1: only Logs is eligible.
	cases := []struct {
		name  string
		level int
		axe   *Axe
		rand  float64
		want  int
	}{
		{"level 1 any roll is Logs", 1, bronze, 0.99, 1511},
		{"L44 bronze r=0 Logs", 44, bronze, 0, 1511},
		{"L45 bronze r=0 Oak", 45, bronze, 0, 1521},
		{"L99 bronze r=0 Oak", 99, bronze, 0, 1521},
		{"L99 bronze r=0.5 Willow", 99, bronze, 0.5, 1519},
		{"L99 bronze r=0.75 Yew", 99, bronze, 0.75, 1515},
		{"L99 bronze r=0.999 Magic", 99, bronze, 0.999, 1513},
		// Level 60 bronze: Logs is out (max 44); Oak 4,
		// Willow 1, Maple 1, total 6.
		{"L60 bronze r=0 Oak", 60, bronze, 0, 1521},
		{"L60 bronze r=0.99 Maple", 60, bronze, 0.99, 1517},
		// Dragon tier 7: Magic weighs 1+7=8; Oak 4, Willow 1,
		// Maple 1, Yew 1, total 15.
		{"L99 dragon r=0 Oak", 99, dragon, 0, 1521},
		{"L99 dragon r=0.5 Magic", 99, dragon, 0.5, 1513},
		{"L99 dragon r=0.9 Magic", 99, dragon, 0.9, 1513},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log := rollEntLog(tc.level, tc.axe, func() float64 { return tc.rand })
			if log == nil || log.ID != tc.want {
				t.Errorf("rollEntLog = %v, want id %d", log, tc.want)
			}
		})
	}
	if got := rollEntLog(99, nil, func() float64 { return 0 }); got != nil {
		t.Errorf("rollEntLog(nil axe) = %v, want nil", got)
	}
}

// TestChopTrunkKillerOnly pins chopTrunk's guards
// (EntTrunk.Woodcutting.js:83-100): a non-killer claims the
// click but starts no session; a killer without a usable axe
// claims it but starts no session; a killer with an axe
// starts a session (first roll chopTicks out) and sets
// no-retaliate; a full inventory claims it with no session.
func TestChopTrunkKillerOnly(t *testing.T) {
	tables := loadFixtureTables(t)
	w, e, _ := newTestEntTrunk(t)
	killer := entPlayer(t, tables, "alice", 3200, 3200)
	other := entPlayer(t, tables, "bob", 3200, 3200)
	npc := makeEntTrunk(t, e, killer, 7234)
	click := func(p *game.Player) bool {
		return e.chopTrunk(&plugins.NpcInteractionEvent{
			Player: p, NPC: npc, NpcID: Ents.TrunkID, ClickType: 1, Location: npc.Location(),
		})
	}
	// Non-killer: claims, no session.
	if !click(other) {
		t.Error("non-killer click: want claimed (true)")
	}
	if _, ok := e.sessions[other]; ok {
		t.Error("non-killer: want no session")
	}
	// Killer without an axe: claims, no session.
	axeless := newTestPlayer("carol", 3200, 3200)
	setLevel(axeless, game.SkillWoodcutting, 99)
	if !click(axeless) {
		t.Error("no-axe click: want claimed (true)")
	}
	if _, ok := e.sessions[axeless]; ok {
		t.Error("no-axe: want no session")
	}
	// Killer with a full inventory: claims, no session.
	full := entPlayer(t, tables, "dave", 3200, 3200)
	fillInventory(full)
	if !click(full) {
		t.Error("full-inventory click: want claimed (true)")
	}
	if _, ok := e.sessions[full]; ok {
		t.Error("full inventory: want no session")
	}
	// Killer with an axe: session, first roll in 3 ticks,
	// no-retaliate set.
	if !click(killer) {
		t.Error("killer click: want claimed (true)")
	}
	session, ok := e.sessions[killer]
	if !ok {
		t.Fatal("killer: want a session")
	}
	if session.next != e.cycle+int64(Ents.ChopTicks) {
		t.Errorf("session next = %d, want cycle+3 = %d", session.next, e.cycle+3)
	}
	if !session.from.Equals(killer.Location()) {
		t.Errorf("session from = %v, want %v", session.from, killer.Location())
	}
	if !game.HasCombatFlag(&killer.Mobile, game.CombatFlagNoRetaliate) {
		t.Error("chopping: want the no-retaliate flag set")
	}
	_ = w
}

// TestEntTickChopsLogs pins the whole chop flow
// (EntTrunk.Woodcutting.js:118-133, 102-116): a roll every
// three ticks, a noted log, 25 XP, and the
// woodcutting:ent-logs emit with the unnoted id.
func TestEntTickChopsLogs(t *testing.T) {
	tables := loadFixtureTables(t)
	w, e, emitted := newTestEntTrunk(t)
	w.randFloat = func() float64 { return 0 } // every roll succeeds; rollEntLog picks Logs
	killer := entPlayer(t, tables, "alice", 3200, 3200)
	// Level 1: only Logs is eligible (its max is 44), so the
	// roll always pays Logs.
	setLevel(killer, game.SkillWoodcutting, 1)
	npc := makeEntTrunk(t, e, killer, 7234)
	if !e.chopTrunk(&plugins.NpcInteractionEvent{
		Player: killer, NPC: npc, NpcID: Ents.TrunkID, ClickType: 1, Location: npc.Location(),
	}) {
		t.Fatal("killer click: want claimed")
	}
	// Ticks 3-4: before the first roll, nothing lands.
	e.tick()
	e.tick()
	if killer.Inventory().Contains(2511) {
		t.Error("before the first roll: want no log yet")
	}
	// Tick 5: the first roll lands a noted Logs.
	e.tick()
	if !killer.Inventory().Contains(2511) {
		t.Error("tick 5: want a noted Logs (2511) in the inventory")
	}
	if got := killer.Inventory().GetAmount(2511); got != 1 {
		t.Errorf("noted Logs amount = %d, want 1 (guild)", got)
	}
	if got := killer.Skills().Experience(game.SkillWoodcutting); got != 25 {
		t.Errorf("Woodcutting XP = %d, want 25", got)
	}
	found := false
	for _, ev := range *emitted {
		if ev.name != EntLogsEventName {
			continue
		}
		payload, ok := ev.payload.(*EntLogsEvent)
		if !ok || payload.Player != killer || payload.ItemID != 1511 || payload.Where != "Woodcutting Guild" {
			t.Errorf("ent-logs payload = %+v, want {alice 1511 guild}", payload)
		}
		found = true
	}
	if !found {
		t.Error("want a woodcutting:ent-logs emit")
	}
}

// TestEntTickChopsLogsWilderness pins the Wilderness rule:
// two noted logs per success.
func TestEntTickChopsLogsWilderness(t *testing.T) {
	tables := loadFixtureTables(t)
	w, e, _ := newTestEntTrunk(t)
	w.randFloat = func() float64 { return 0 }
	killer := entPlayer(t, tables, "alice", 3200, 3200)
	// Level 1: only Logs is eligible (its max is 44).
	setLevel(killer, game.SkillWoodcutting, 1)
	npc := makeEntTrunk(t, e, killer, 6594)
	e.chopTrunk(&plugins.NpcInteractionEvent{
		Player: killer, NPC: npc, NpcID: Ents.TrunkID, ClickType: 1, Location: npc.Location(),
	})
	e.tick()
	e.tick()
	e.tick()
	if got := killer.Inventory().GetAmount(2511); got != 2 {
		t.Errorf("wilderness noted Logs amount = %d, want 2", got)
	}
}

// TestEntGiveLogsFullInventory pins giveLogs' full rule
// (EntTrunk.Woodcutting.js:107-110): a full inventory that
// does not hold the noted log refuses (false) and the
// session ends; a full inventory that already holds the
// noted log still pays (noted logs stack).
func TestEntGiveLogsFullInventory(t *testing.T) {
	tables := loadFixtureTables(t)
	w, e, _ := newTestEntTrunk(t)
	w.randFloat = func() float64 { return 0 }
	killer := entPlayer(t, tables, "alice", 3200, 3200)
	// Level 1: only Logs is eligible (its max is 44), so the
	// noted log under test is 2511.
	setLevel(killer, game.SkillWoodcutting, 1)
	// Noted logs stack in the cache; with no definition source
	// the container treats every id as non-stackable and an
	// Add into a full inventory refuses even a held stack.
	killer.SetItemDefinitions(stackableDefs{stackable: map[int]bool{2511: true}})
	npc := makeEntTrunk(t, e, killer, 7234)
	trunk := e.trunks[npc]
	bronze := axeByItem(tables, "bronze_axe")
	fillInventory(killer)
	if got := e.giveLogs(killer, trunk, bronze); got {
		t.Error("full inventory without the noted log: giveLogs want false")
	}
	if killer.Inventory().Contains(2511) {
		t.Error("full inventory: want no log delivered")
	}
	// Full but already holding the noted log: pays.
	killer.Inventory().DeleteNumber(9000, 1)
	killer.Inventory().Adds(2511, 1)
	if got := e.giveLogs(killer, trunk, bronze); !got {
		t.Error("full inventory holding the noted log: giveLogs want true")
	}
	if got := killer.Inventory().GetAmount(2511); got != 2 {
		t.Errorf("noted Logs amount = %d, want 2", got)
	}
}

// TestEntTrunkExpiry pins trunk expiry
// (EntTrunk.Woodcutting.js:120): at until the trunk is
// gone, a click starts nothing, and a live session ends.
func TestEntTrunkExpiry(t *testing.T) {
	tables := loadFixtureTables(t)
	w, e, _ := newTestEntTrunk(t)
	w.randFloat = func() float64 { return 1 } // every roll misses
	killer := entPlayer(t, tables, "alice", 3200, 3200)
	npc := makeEntTrunk(t, e, killer, 7234)
	e.chopTrunk(&plugins.NpcInteractionEvent{
		Player: killer, NPC: npc, NpcID: Ents.TrunkID, ClickType: 1, Location: npc.Location(),
	})
	if _, ok := e.sessions[killer]; !ok {
		t.Fatal("want a session before expiry")
	}
	// Trunk registered at cycle 2 until 103: tick to 102,
	// still live; tick 103, expired.
	for e.cycle < 102 {
		e.tick()
	}
	if _, ok := e.trunks[npc]; !ok {
		t.Fatal("cycle 102: want the trunk still live")
	}
	e.tick()
	if _, ok := e.trunks[npc]; ok {
		t.Error("cycle 103: want the trunk expired")
	}
	if _, ok := e.sessions[killer]; ok {
		t.Error("trunk expired: want the session ended")
	}
	if game.HasCombatFlag(&killer.Mobile, game.CombatFlagNoRetaliate) {
		// The flag clears with the session.
		t.Error("trunk expired: want the no-retaliate flag cleared")
	}
	// A click after expiry claims but starts nothing.
	if !e.chopTrunk(&plugins.NpcInteractionEvent{
		Player: killer, NPC: npc, NpcID: Ents.TrunkID, ClickType: 1, Location: npc.Location(),
	}) {
		t.Error("post-expiry click: want claimed (true)")
	}
	if _, ok := e.sessions[killer]; ok {
		t.Error("post-expiry click: want no session")
	}
}

// TestEntSessionEnds pins the session end rules
// (EntTrunk.Woodcutting.js:122-124, 135-137): walking off
// the chop tile ends the session (and clears the flag), and
// disconnecting forgets it.
func TestEntSessionEnds(t *testing.T) {
	tables := loadFixtureTables(t)
	w, e, _ := newTestEntTrunk(t)
	w.randFloat = func() float64 { return 1 }
	killer := entPlayer(t, tables, "alice", 3200, 3200)
	npc := makeEntTrunk(t, e, killer, 7234)
	e.chopTrunk(&plugins.NpcInteractionEvent{
		Player: killer, NPC: npc, NpcID: Ents.TrunkID, ClickType: 1, Location: npc.Location(),
	})
	killer.SetLocation(game.Location{X: 3210, Y: 3200, Z: 0})
	e.tick()
	if _, ok := e.sessions[killer]; ok {
		t.Error("walked away: want the session ended")
	}
	if game.HasCombatFlag(&killer.Mobile, game.CombatFlagNoRetaliate) {
		t.Error("walked away: want the no-retaliate flag cleared")
	}
	// Disconnect forgets.
	killer.SetLocation(game.Location{X: 3200, Y: 3200, Z: 0})
	e.chopTrunk(&plugins.NpcInteractionEvent{
		Player: killer, NPC: npc, NpcID: Ents.TrunkID, ClickType: 1, Location: npc.Location(),
	})
	e.forget(&plugins.PlayerDisconnectEvent{Player: killer})
	if _, ok := e.sessions[killer]; ok {
		t.Error("disconnect: want the session forgotten")
	}
	if game.HasCombatFlag(&killer.Mobile, game.CombatFlagNoRetaliate) {
		t.Error("disconnect: want the no-retaliate flag cleared")
	}
}

// TestEntHubRegistration fires the unit through the hub:
// the death hook sets remains, the module task transforms
// the trunk, the click hook starts a session, and the task
// pays a log — the production wiring, end to end.
func TestEntHubRegistration(t *testing.T) {
	tables := loadFixtureTables(t)
	SetTables(tables)
	defer SetTables(nil)
	tasks := game.NewTaskManager()
	hub := plugins.NewHub([]plugins.Plugin{Plugin}, plugins.HubOptions{MembersWorld: true})
	hub.SetTaskManager(tasks)
	hub.Load()
	p := newTestPlayer("alice", 3200, 3200)
	// Level 1: only Logs is eligible (its max is 44), so the
	// roll always pays Logs.
	setLevel(p, game.SkillWoodcutting, 1)
	bronze := axeByItem(tables, "bronze_axe")
	p.Inventory().Adds(bronze.ID, 1)
	origRandFloat := shared.randFloat
	shared.randFloat = func() float64 { return 0 }
	defer func() { shared.randFloat = origRandFloat }()
	// The shared unit's note resolver needs the test mapping
	// (no cache in this test): the production source carries
	// ItemNoteIDSource; here the override stands in.
	origNoteID := shared.ent.noteID
	shared.ent.noteID = func(id int) int { return id + 1000 }
	defer func() { shared.ent.noteID = origNoteID }()
	npc := game.NewNPC(7234, game.Location{X: 3200, Y: 3200, Z: 0})
	npc.SetRegistered(true)
	death := &plugins.NpcDeathEvent{
		Killer: p, NPC: npc, NpcID: 7234, Location: npc.Location(),
	}
	hub.EmitNpcDeath(death)
	if death.Remains == nil || death.Remains.Ticks != 103 || death.Remains.RespawnTicks != 50 {
		t.Fatalf("hub death: remains = %+v, want {103 50}", death.Remains)
	}
	for cycle := int64(1); cycle <= 2; cycle++ {
		tasks.Process(cycle)
	}
	if got := npc.ContentID(); got != Ents.TrunkID {
		t.Fatalf("after 2 task ticks: trunk content id = %d, want %d", got, Ents.TrunkID)
	}
	click := &plugins.NpcInteractionEvent{
		Player: p, NPC: npc, NpcID: Ents.TrunkID, NpcIndex: npc.Index, ClickType: 1, Location: npc.Location(),
	}
	if !hub.EmitNpcInteraction(click) {
		t.Fatal("hub trunk click: want handled")
	}
	if _, ok := shared.ent.sessions[p]; !ok {
		t.Fatal("hub trunk click: want a session")
	}
	for cycle := int64(3); cycle <= 5; cycle++ {
		tasks.Process(cycle)
	}
	if !p.Inventory().Contains(2511) {
		t.Error("after 3 more task ticks: want a noted Logs in the inventory")
	}
}

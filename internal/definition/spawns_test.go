package definition

import (
	"strconv"
	"strings"
	"testing"

	"tsps-go/internal/config"
)

func loadSpawns(t *testing.T, files map[string]string, opts LoadOptions) *Store {
	t.Helper()
	dir := fixtureDir(t, files)
	store, err := Load(dir, opts)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return store
}

func TestNpcSpawnRealShapeNilCache(t *testing.T) {
	store := loadSpawns(t, withFiles(map[string]string{
		"npc-spawns.json": `[{"level":0,"name":"Goblin","x":3200,"y":3200,"id":5,"wanderRadius":3}]`,
	}), LoadOptions{})
	spawns := store.NPCSpawns()
	if len(spawns) != 1 {
		t.Fatalf("spawns = %v", spawns)
	}
	got := spawns[0]
	want := NpcSpawn{ID: 5, X: 3200, Y: 3200, Z: 0, Facing: 6, WanderRadius: 3, Name: "Goblin", Source: "NpcSpawns"}
	if got != want {
		t.Fatalf("spawn = %+v, want %+v", got, want)
	}
}

func TestNpcSpawnFacing(t *testing.T) {
	for _, tc := range []struct {
		name       string
		entry      string
		wantFacing int
	}{
		{"default south", `{"level":0,"x":1,"y":1,"id":1}`, 6},
		{"direction 0 sw", `{"level":0,"x":1,"y":1,"id":1,"direction":0}`, 5},
		{"direction 2 ne", `{"level":0,"x":1,"y":1,"id":1,"direction":2}`, 7},
		{"direction 6 n", `{"level":0,"x":1,"y":1,"id":1,"direction":6}`, 1},
		{"direction wraps", `{"level":0,"x":1,"y":1,"id":1,"direction":8}`, 5},
		{"direction negative", `{"level":0,"x":1,"y":1,"id":1,"direction":-1}`, 2},
		{"direction truncates", `{"level":0,"x":1,"y":1,"id":1,"direction":2.9}`, 7},
		{"direction string falls back", `{"level":0,"x":1,"y":1,"id":1,"direction":"3"}`, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := loadSpawns(t, withFiles(map[string]string{
				"npc-spawns.json": "[" + tc.entry + "]",
			}), LoadOptions{})
			spawns := store.NPCSpawns()
			if len(spawns) != 1 {
				t.Fatalf("spawns = %v", spawns)
			}
			if spawns[0].Facing != tc.wantFacing {
				t.Fatalf("facing = %d, want %d", spawns[0].Facing, tc.wantFacing)
			}
		})
	}
}

func TestNpcSpawnWanderRadius(t *testing.T) {
	for _, tc := range []struct {
		name  string
		entry string
		want  int
	}{
		{"default", `{"level":0,"x":1,"y":1,"id":1}`, 5},
		{"explicit zero kept", `{"level":0,"x":1,"y":1,"id":1,"wanderRadius":0}`, 0},
		{"negative clamped", `{"level":0,"x":1,"y":1,"id":1,"wanderRadius":-3}`, 0},
		{"string falls back", `{"level":0,"x":1,"y":1,"id":1,"wanderRadius":"7"}`, 5},
		{"truncated", `{"level":0,"x":1,"y":1,"id":1,"wanderRadius":7.9}`, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := loadSpawns(t, withFiles(map[string]string{
				"npc-spawns.json": "[" + tc.entry + "]",
			}), LoadOptions{})
			if got := store.NPCSpawns()[0].WanderRadius; got != tc.want {
				t.Fatalf("radius = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestNpcSpawnInvalidEntriesSkipped(t *testing.T) {
	store := loadSpawns(t, withFiles(map[string]string{
		"npc-spawns.json": `[
			{"level":0,"x":1,"y":1,"id":1},
			{"level":0,"x":1,"y":1},
			{"level":0,"x":1,"y":1,"id":-1},
			{"level":0,"x":"a","y":1,"id":2},
			{"x":1,"y":1,"id":3},
			{"level":5,"x":1,"y":1,"id":4}
		]`,
	}), LoadOptions{})
	spawns := store.NPCSpawns()
	if len(spawns) != 2 {
		t.Fatalf("spawns = %v", spawns)
	}
	if spawns[0].ID != 1 || spawns[1].ID != 4 || spawns[1].Z != 5 {
		t.Fatalf("spawns = %+v", spawns)
	}
}

func TestNpcSpawnKeyedEntryDropped(t *testing.T) {
	var logs logCapture
	store := loadSpawns(t, withFiles(map[string]string{
		"npc-spawns.json": `[{"key":"ELENA","level":0,"x":1,"y":1,"name":"Elena"}]`,
	}), LoadOptions{Logf: logs.logf})
	if got := len(store.NPCSpawns()); got != 0 {
		t.Fatalf("spawns = %d", got)
	}
	if !logs.contains("Unknown NpcIdentifiers key") {
		t.Fatalf("logs = %q", logs.joined())
	}
}

func TestNpcSpawnCacheDirection(t *testing.T) {
	defs, _ := newFakeCache(t, fakeCacheSpec{
		npcs: map[int][]byte{
			0: npcEntry("Goblin", npcSpawnDirection(3)),
			1: npcEntry("Man"),
		},
	})
	store := loadSpawns(t, withFiles(map[string]string{
		"npc-spawns.json": `[
			{"level":0,"x":1,"y":1,"id":0},
			{"level":0,"x":2,"y":2,"id":1},
			{"level":0,"x":3,"y":3,"id":0,"direction":6},
			{"level":0,"x":4,"y":4,"id":9}
		]`,
	}), LoadOptions{Cache: defs})
	spawns := store.NPCSpawns()
	if len(spawns) != 3 {
		t.Fatalf("spawns = %v (id 9 must be dropped as unsupported)", spawns)
	}
	// Cache spawnDirection 3 -> cache table -> client 3 -> facing 3.
	if spawns[0].Facing != 3 {
		t.Fatalf("cached facing = %d, want 3", spawns[0].Facing)
	}
	// Default cache spawnDirection 6 (south) -> facing 6.
	if spawns[1].Facing != 6 {
		t.Fatalf("default cached facing = %d, want 6", spawns[1].Facing)
	}
	// Explicit direction wins over the cache.
	if spawns[2].Facing != 1 {
		t.Fatalf("explicit facing = %d, want 1", spawns[2].Facing)
	}
}

func TestNpcSpawnShippedFileFields(t *testing.T) {
	// Missing name -> "", the entry's `source` provenance field is
	// ignored, duplicate (id,x,y,level) keys inside one source are
	// all kept, and `level` lands on Z.
	store := loadSpawns(t, withFiles(map[string]string{
		"npc-spawns.json": `[
			{"level":2,"x":1,"y":1,"id":14048},
			{"level":0,"x":2,"y":2,"id":5,"name":"A","source":"wiki"},
			{"level":0,"x":2,"y":2,"id":5,"name":"B","source":"rsprox:123"},
			{"level":0,"x":2,"y":2,"id":5,"name":"C"}
		]`,
	}), LoadOptions{})
	spawns := store.NPCSpawns()
	if len(spawns) != 4 {
		t.Fatalf("spawns = %v", spawns)
	}
	if spawns[0].Name != "" || spawns[0].Z != 2 {
		t.Fatalf("nameless spawn = %+v", spawns[0])
	}
	if spawns[1].Source != "NpcSpawns" || spawns[1].Name != "A" {
		t.Fatalf("sourced spawn = %+v", spawns[1])
	}
	if spawns[2].Name != "B" || spawns[3].Name != "C" {
		t.Fatalf("duplicate spawns = %+v", spawns[1:])
	}
}

// TestNpcSpawnShippedFile loads the repo's shipped
// data/definitions/npc-spawns.json with no cache (so the count is
// cache-independent: with no cache bound, no entry is dropped as
// unsupported) and pins the accepted count. The one invalid entry
// is Elena, whose `key` cannot resolve without the generated
// NpcIdentifiers table (see TestNpcSpawnKeyedEntryDropped); every
// other entry is accepted, including the 45 id-0 Tool Leprechauns.
func TestNpcSpawnShippedFile(t *testing.T) {
	var logs logCapture
	store, err := Load("../../data/definitions", LoadOptions{Logf: logs.logf})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	spawns := store.NPCSpawns()
	if len(spawns) != 30827 {
		t.Fatalf("shipped spawns = %d, want 30827 (logs: %s)", len(spawns), logs.joined())
	}
	if !logs.contains("candidates=30828, invalid=1, unsupported=0, failures=0") {
		t.Fatalf("loader counts missing from logs: %s", logs.joined())
	}
	for _, sp := range spawns {
		if sp.Name == "Elena" && sp.X == 2592 && sp.Y == 3336 {
			t.Fatalf("Elena spawn present, want dropped (unresolvable key)")
		}
	}
	lep := 0
	for _, sp := range spawns {
		if sp.ID == 0 && sp.Name == "Tool Leprechaun" {
			lep++
		}
	}
	if lep != 45 {
		t.Fatalf("id-0 Tool Leprechauns = %d, want 45", lep)
	}
	for _, w := range []struct {
		id, x, y, z, radius int
		name                string
	}{
		{12119, 3096, 3238, 0, 0, "Friendly Forester"},
		{3036, 3002, 3206, 0, 5, "Goblin"},
		{1174, 3225, 3300, 0, 5, "Chicken"},
		{1525, 3085, 3230, 0, 0, "Fishing spot"},
	} {
		found := false
		for _, sp := range spawns {
			if sp.ID == w.id && sp.X == w.x && sp.Y == w.y && sp.Z == w.z {
				found = true
				if sp.Name != w.name || sp.WanderRadius != w.radius {
					t.Errorf("spawn %d at (%d,%d): %+v, want name %q radius %d", w.id, w.x, w.y, sp, w.name, w.radius)
				}
				break
			}
		}
		if !found {
			t.Errorf("spawn %d at (%d,%d,%d) not found", w.id, w.x, w.y, w.z)
		}
	}
}

func TestNpcSpawnMergeReplaceAndRemove(t *testing.T) {
	core := `[
		{"level":0,"x":1,"y":1,"id":5,"name":"A"},
		{"level":0,"x":1,"y":1,"id":5,"name":"B"}
	]`
	t.Run("replace oldest from earlier source", func(t *testing.T) {
		registry := NewRegistry()
		registry.Register(npcSpawnsDefinitionType, "Patch", Source{
			Name: "patch", Priority: IntPriority(150),
			Load: func() (any, error) {
				return []any{map[string]any{"level": float64(0), "x": float64(1), "y": float64(1), "id": float64(5), "name": "C"}}, nil
			},
		})
		store := loadSpawns(t, withFiles(map[string]string{"npc-spawns.json": core}), LoadOptions{Sources: registry})
		names := []string{}
		for _, s := range store.NPCSpawns() {
			names = append(names, s.Name)
		}
		if strings.Join(names, ",") != "B,C" {
			t.Fatalf("names = %v, want B,C", names)
		}
		if store.NPCSpawns()[1].Source != "patch" {
			t.Fatalf("source = %q", store.NPCSpawns()[1].Source)
		}
	})
	t.Run("remove drops oldest", func(t *testing.T) {
		registry := NewRegistry()
		registry.Register(npcSpawnsDefinitionType, "Patch", Source{
			Name: "patch", Priority: IntPriority(150),
			Load: func() (any, error) {
				return []any{map[string]any{"level": float64(0), "x": float64(1), "y": float64(1), "id": float64(5), "remove": true}}, nil
			},
		})
		store := loadSpawns(t, withFiles(map[string]string{"npc-spawns.json": core}), LoadOptions{Sources: registry})
		spawns := store.NPCSpawns()
		if len(spawns) != 1 || spawns[0].Name != "B" {
			t.Fatalf("spawns = %v", spawns)
		}
	})
	t.Run("remove then re-add fills freed slot", func(t *testing.T) {
		registry := NewRegistry()
		registry.Register(npcSpawnsDefinitionType, "Patch", Source{
			Name: "patch", Priority: IntPriority(150),
			Load: func() (any, error) {
				return []any{
					map[string]any{"level": float64(0), "x": float64(1), "y": float64(1), "id": float64(5), "remove": true},
					map[string]any{"level": float64(0), "x": float64(1), "y": float64(1), "id": float64(5), "name": "C"},
				}, nil
			},
		})
		store := loadSpawns(t, withFiles(map[string]string{"npc-spawns.json": core}), LoadOptions{Sources: registry})
		names := []string{}
		for _, s := range store.NPCSpawns() {
			names = append(names, s.Name)
		}
		if strings.Join(names, ",") != "B,C" {
			t.Fatalf("names = %v, want B,C", names)
		}
	})
	t.Run("duplicates within one source kept", func(t *testing.T) {
		store := loadSpawns(t, withFiles(map[string]string{"npc-spawns.json": core}), LoadOptions{})
		if got := len(store.NPCSpawns()); got != 2 {
			t.Fatalf("spawns = %d", got)
		}
	})
}

func TestObjectSpawnRealShape(t *testing.T) {
	store := loadSpawns(t, withFiles(map[string]string{
		"object-spawns.json": `[
			{"id":100,"position":{"x":3200,"y":3200,"z":0},"type":10,"face":1},
			{"id":101,"position":{"x":3201,"y":3200,"z":1}},
			{"id":102,"position":{"x":3202,"y":3200,"z":0},"remove":true}
		]`,
	}), LoadOptions{})
	spawns := store.ObjectSpawns()
	if len(spawns) != 2 {
		t.Fatalf("spawns = %v", spawns)
	}
	if spawns[0] != (ObjectSpawn{ID: 100, X: 3200, Y: 3200, Z: 0, Type: 10, Face: 1}) {
		t.Fatalf("spawn 0 = %+v", spawns[0])
	}
	// Defaults: type 10, face 0.
	if spawns[1] != (ObjectSpawn{ID: 101, X: 3201, Y: 3200, Z: 1, Type: 10, Face: 0}) {
		t.Fatalf("spawn 1 = %+v", spawns[1])
	}
	removals := store.ObjectSpawnRemovals()
	if len(removals) != 1 || removals[0].ID != 102 {
		t.Fatalf("removals = %v", removals)
	}
}

func TestObjectSpawnInvalidFailsLoad(t *testing.T) {
	for _, tc := range []struct {
		name  string
		entry string
	}{
		{"z out of range", `{"id":1,"position":{"x":1,"y":1,"z":4}}`},
		{"string id", `{"id":"1","position":{"x":1,"y":1,"z":0}}`},
		{"negative id", `{"id":-1,"position":{"x":1,"y":1,"z":0}}`},
		{"type out of range", `{"id":1,"position":{"x":1,"y":1,"z":0},"type":23}`},
		{"face out of range", `{"id":1,"position":{"x":1,"y":1,"z":0},"face":4}`},
		{"non-boolean remove", `{"id":1,"position":{"x":1,"y":1,"z":0},"remove":"yes"}`},
		{"missing position", `{"id":1}`},
		{"fractional x", `{"id":1,"position":{"x":1.5,"y":1,"z":0}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixtureDir(t, withFiles(map[string]string{"object-spawns.json": "[" + tc.entry + "]"}))
			if _, err := Load(dir, LoadOptions{}); err == nil {
				t.Fatal("invalid object spawn must fail Load")
			} else if !strings.Contains(err.Error(), "Invalid object spawn") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestObjectSpawnContributorAppendsAfterCore(t *testing.T) {
	registry := NewRegistry()
	registry.Register(objectSpawnsDefinitionType, "Patch", Source{
		Name: "patch", Priority: IntPriority(1),
		Load: func() (any, error) {
			return []any{map[string]any{
				"id":       float64(200),
				"position": map[string]any{"x": float64(9), "y": float64(9), "z": float64(0)},
			}}, nil
		},
	})
	store := loadSpawns(t, withFiles(map[string]string{
		"object-spawns.json": `[{"id":100,"position":{"x":1,"y":1,"z":0}}]`,
	}), LoadOptions{Sources: registry})
	spawns := store.ObjectSpawns()
	if len(spawns) != 2 || spawns[0].ID != 100 || spawns[1].ID != 200 {
		t.Fatalf("spawns = %v (contributed must append after core regardless of priority)", spawns)
	}
}

func TestGroundItemRealShapeNilCache(t *testing.T) {
	store := loadSpawns(t, withFiles(map[string]string{
		"ground-items.json": `[
			{"id":995,"location":{"x":3200,"y":3200,"z":0},"amt":5,"respawn":100,"description":"Port Sarim"},
			{"id":1,"location":{"x":3201,"y":3200,"z":0}}
		]`,
	}), LoadOptions{})
	spawns := store.GroundItemSpawns()
	if len(spawns) != 2 {
		t.Fatalf("spawns = %v", spawns)
	}
	if spawns[0] != (GroundItemSpawn{ID: 995, X: 3200, Y: 3200, Z: 0, Amount: 5, Respawn: 100, Description: "Port Sarim"}) {
		t.Fatalf("spawn 0 = %+v", spawns[0])
	}
	if spawns[1] != (GroundItemSpawn{ID: 1, X: 3201, Y: 3200, Z: 0, Amount: 1, Respawn: 0}) {
		t.Fatalf("spawn 1 = %+v (defaults amt 1, respawn 0)", spawns[1])
	}
}

func TestGroundItemValidationAndMembers(t *testing.T) {
	content := `[
		{"id":1,"location":{"x":1,"y":1,"z":0}},
		{"id":2,"location":{"x":1,"y":1,"z":4}},
		{"id":3,"location":{"x":1,"y":1,"z":0},"amt":0},
		{"id":4,"location":{"x":1,"y":1,"z":0},"amt":2.5},
		{"id":5,"location":{"x":1,"y":1,"z":0},"members":"yes"},
		{"id":6,"location":{"x":1,"y":1,"z":0},"members":true},
		{"id":"7","location":{"x":1,"y":1,"z":0}}
	]`
	t.Run("members world keeps members items", func(t *testing.T) {
		store := loadSpawns(t, withFiles(map[string]string{"ground-items.json": content}), LoadOptions{})
		ids := []int{}
		for _, s := range store.GroundItemSpawns() {
			ids = append(ids, s.ID)
		}
		if strings.Join(intsToStrings(ids), ",") != "1,6,7" {
			t.Fatalf("ids = %v", ids)
		}
		if !store.GroundItems()[1].Members {
			t.Fatal("members flag must be preserved")
		}
	})
	t.Run("free world drops members items", func(t *testing.T) {
		store := loadSpawns(t, withFiles(map[string]string{"ground-items.json": content}),
			LoadOptions{World: &config.World{MembersWorld: false}})
		ids := []int{}
		for _, s := range store.GroundItemSpawns() {
			ids = append(ids, s.ID)
		}
		if strings.Join(intsToStrings(ids), ",") != "1,7" {
			t.Fatalf("ids = %v", ids)
		}
	})
}

func intsToStrings(ids []int) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = strconv.Itoa(id)
	}
	return out
}

func TestGroundItemPickupReplacement(t *testing.T) {
	store := loadSpawns(t, withFiles(map[string]string{
		"ground-items.json": `[
			{"id":1,"location":{"x":1,"y":1,"z":0},"pickupReplacement":{"id":2,"telegrabDenial":"Nope."}},
			{"id":3,"location":{"x":2,"y":2,"z":0},"pickupReplacement":{"id":4}},
			{"id":5,"location":{"x":3,"y":3,"z":0},"pickupReplacement":{"id":"6"}},
			{"id":7,"location":{"x":4,"y":4,"z":0},"pickupReplacement":{"id":0}},
			{"id":8,"location":{"x":5,"y":5,"z":0},"pickupReplacement":null}
		]`,
	}), LoadOptions{})
	items := store.GroundItems()
	if len(items) != 2 {
		t.Fatalf("items = %v (string-id, id-0 and null replacements must invalidate)", items)
	}
	rep := items[0].PickupReplacement
	if rep == nil || rep.ID != 2 || rep.TelegrabDenial != "Nope." {
		t.Fatalf("replacement = %+v", rep)
	}
	if rep := items[1].PickupReplacement; rep == nil || rep.ID != 4 || rep.TelegrabDenial != "" {
		t.Fatalf("replacement = %+v", rep)
	}
}

func TestGroundItemCacheFilter(t *testing.T) {
	defs, _ := newFakeCache(t, fakeCacheSpec{
		objs: map[int][]byte{
			0: objEntry("null"),
			1: objEntry("Coins"),
		},
	})
	store := loadSpawns(t, withFiles(map[string]string{
		"ground-items.json": `[
			{"id":1,"location":{"x":1,"y":1,"z":0}},
			{"id":0,"location":{"x":2,"y":2,"z":0}},
			{"id":9,"location":{"x":3,"y":3,"z":0}}
		]`,
	}), LoadOptions{Cache: defs})
	spawns := store.GroundItemSpawns()
	if len(spawns) != 1 || spawns[0].ID != 1 {
		t.Fatalf("spawns = %v", spawns)
	}
}

func TestGroundItemSourceOrdering(t *testing.T) {
	registry := NewRegistry()
	registry.Register(groundItemsDefinitionType, "Patch", Source{
		Name: "aaa", Priority: IntPriority(-1),
		Load: func() (any, error) {
			return []any{map[string]any{
				"id":       float64(50),
				"location": map[string]any{"x": float64(1), "y": float64(1), "z": float64(0)},
			}}, nil
		},
	})
	store := loadSpawns(t, withFiles(map[string]string{
		"ground-items.json": `[{"id":1,"location":{"x":1,"y":1,"z":0}}]`,
	}), LoadOptions{Sources: registry})
	spawns := store.GroundItemSpawns()
	if len(spawns) != 2 || spawns[0].ID != 50 || spawns[1].ID != 1 {
		t.Fatalf("spawns = %v (priority -1 sorts before core priority 0)", spawns)
	}
}

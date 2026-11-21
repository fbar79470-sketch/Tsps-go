package definition

import (
	"strings"
	"testing"
)

func interactionSource(entries ...any) Source {
	return Source{
		Load: func() (any, error) { return entries, nil },
	}
}

func loadInteractions(t *testing.T, registry *Registry) *Store {
	t.Helper()
	dir := fixtureDir(t, minimalFiles())
	store, err := Load(dir, LoadOptions{Sources: registry})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return store
}

func TestInteractionExplicitMethod(t *testing.T) {
	registry := NewRegistry()
	registry.Register(npcInteractionsType, "TestPlugin", interactionSource(
		map[string]any{
			"npcId": float64(5),
			"firstClick": map[string]any{
				"method": "shops.open",
				"args":   map[string]any{"shopId": float64(3)},
			},
		},
	))
	store := loadInteractions(t, registry)
	def, ok := store.Interaction(5)
	if !ok {
		t.Fatal("interaction missing")
	}
	action := def.Action(1)
	if action == nil || action.Method != "shops.open" || action.Source != "TestPlugin" {
		t.Fatalf("action = %+v", action)
	}
	if action.Args["shopId"] != float64(3) {
		t.Fatalf("args = %v", action.Args)
	}
	if def.Action(0) != nil || def.Action(6) != nil || def.Action(2) != nil {
		t.Fatal("other clicks must be nil")
	}
}

func TestInteractionShorthands(t *testing.T) {
	registry := NewRegistry()
	registry.Register(npcInteractionsType, "TestPlugin", interactionSource(
		map[string]any{
			"npcId":      float64(1),
			"firstClick": map[string]any{"shopId": float64(7)},
			"secondClick": map[string]any{
				"teleportLocation": map[string]any{"x": float64(3200), "y": float64(3200)},
			},
			"thirdClick": map[string]any{"dialogueId": float64(9)},
		},
	))
	store := loadInteractions(t, registry)
	def, _ := store.Interaction(1)
	if a := def.Action(1); a == nil || a.Method != "core.shops.open" || a.Args["shopId"] != 7 {
		t.Fatalf("shop shorthand = %+v", a)
	}
	if a := def.Action(2); a == nil || a.Method != "core.player.teleport" {
		t.Fatalf("teleport shorthand = %+v", a)
	} else if dest, ok := a.Args["destination"].(map[string]any); !ok || dest["x"] != 3200 || dest["z"] != 0 {
		t.Fatalf("destination = %v (z defaults 0)", a.Args)
	}
	if a := def.Action(3); a == nil || a.Method != "core.dialogues.start" || a.Args["dialogueId"] != 9 {
		t.Fatalf("dialogue shorthand = %+v", a)
	}
}

func TestInteractionShorthandPriorityAndForms(t *testing.T) {
	registry := NewRegistry()
	registry.Register(npcInteractionsType, "TestPlugin", interactionSource(
		map[string]any{
			"npcId": float64(1),
			// Shop wins over teleport and dialogue.
			"firstClick": map[string]any{
				"shopId":           float64(7),
				"dialogueId":       float64(9),
				"teleportLocation": map[string]any{"x": float64(1), "y": float64(1), "z": float64(0)},
			},
			// Snake_case spellings work.
			"second_click": map[string]any{"shop_id": float64(8)},
			// String ids do not coerce.
			"thirdClick": map[string]any{"shopId": "7"},
		},
	))
	store := loadInteractions(t, registry)
	def, _ := store.Interaction(1)
	if a := def.Action(1); a == nil || a.Method != "core.shops.open" {
		t.Fatalf("action 1 = %+v", a)
	}
	if a := def.Action(2); a == nil || a.Args["shopId"] != 8 {
		t.Fatalf("action 2 = %+v", a)
	}
	if a := def.Action(3); a != nil {
		t.Fatalf("string shopId must not produce an action: %+v", a)
	}
}

func TestInteractionMergeRules(t *testing.T) {
	registry := NewRegistry()
	registry.Register(npcInteractionsType, "First", Source{
		Name: "first", Priority: IntPriority(10),
		Load: func() (any, error) {
			return []any{map[string]any{
				"npcId":       float64(1),
				"firstClick":  map[string]any{"method": "a.first"},
				"secondClick": map[string]any{"method": "a.second"},
			}}, nil
		},
	})
	registry.Register(npcInteractionsType, "Second", Source{
		Name: "second", Priority: IntPriority(20),
		Load: func() (any, error) {
			return []any{map[string]any{
				"npcId": float64(1),
				// Overwrites slot 1; slot 2's bad action must
				// not clear the good one; slot 3 is new.
				"firstClick":  map[string]any{"method": "b.first"},
				"secondClick": map[string]any{"method": "Bad Method!"},
				"thirdClick":  map[string]any{"method": "b.third"},
			}}, nil
		},
	})
	store := loadInteractions(t, registry)
	def, _ := store.Interaction(1)
	if a := def.Action(1); a == nil || a.Method != "b.first" || a.Source != "Second" {
		t.Fatalf("action 1 = %+v", a)
	}
	if a := def.Action(2); a == nil || a.Method != "a.second" {
		t.Fatalf("action 2 must survive a bad overwrite: %+v", a)
	}
	if a := def.Action(3); a == nil || a.Method != "b.third" {
		t.Fatalf("action 3 = %+v", a)
	}
}

func TestInteractionCamelWinsAndInvalidEntries(t *testing.T) {
	registry := NewRegistry()
	registry.Register(npcInteractionsType, "TestPlugin", interactionSource(
		map[string]any{
			"npcId":       float64(1),
			"firstClick":  map[string]any{"method": "camel.wins"},
			"first_click": map[string]any{"method": "snake.loses"},
		},
		// No usable action: invalid, discarded.
		map[string]any{"npcId": float64(2), "firstClick": map[string]any{"method": "Bad!"}},
		// Bad npcId: invalid.
		map[string]any{"npcId": float64(-1), "firstClick": map[string]any{"method": "a.b"}},
		map[string]any{"npcId": "abc", "firstClick": map[string]any{"method": "a.b"}},
		// A numeric string coerces via Number(), as in TS.
		map[string]any{"npcId": "3", "firstClick": map[string]any{"method": "a.b"}},
	))
	store := loadInteractions(t, registry)
	def, _ := store.Interaction(1)
	if a := def.Action(1); a == nil || a.Method != "camel.wins" {
		t.Fatalf("action = %+v", a)
	}
	if _, ok := store.Interaction(2); ok {
		t.Fatal("npc 2 must have no definition")
	}
	if _, ok := store.Interaction(3); !ok {
		t.Fatal("npcId \"3\" must coerce to 3")
	}
	if got := len(store.Interactions()); got != 2 {
		t.Fatalf("interactions = %d", got)
	}
}

func TestInteractionMethodValidation(t *testing.T) {
	for _, tc := range []struct {
		method string
		valid  bool
	}{
		{"a.b", true},
		{"shops.open", true},
		{"core2.dialogues.start", true},
		{"a.b_c.D$e", true},
		{"a", false},
		{"A.b", false},
		{"a.B C", false},
		{"a..b", false},
		{"1a.b", false},
		{"a.b.", false},
	} {
		t.Run(tc.method, func(t *testing.T) {
			registry := NewRegistry()
			registry.Register(npcInteractionsType, "T", interactionSource(
				map[string]any{"npcId": float64(1), "firstClick": map[string]any{"method": tc.method}},
			))
			store := loadInteractions(t, registry)
			_, ok := store.Interaction(1)
			if ok != tc.valid {
				t.Fatalf("method %q valid=%v, got definition=%v", tc.method, tc.valid, ok)
			}
		})
	}
}

func TestInteractionArgsMustBeObject(t *testing.T) {
	registry := NewRegistry()
	registry.Register(npcInteractionsType, "T", interactionSource(
		map[string]any{"npcId": float64(1), "firstClick": map[string]any{"method": "a.b", "args": "nope"}},
		map[string]any{"npcId": float64(2), "firstClick": map[string]any{"method": "a.b", "args": nil}},
	))
	store := loadInteractions(t, registry)
	if _, ok := store.Interaction(1); ok {
		t.Fatal("non-object args must invalidate the action")
	}
	def, ok := store.Interaction(2)
	if !ok {
		t.Fatal("null args must yield an empty args object")
	}
	if len(def.Action(1).Args) != 0 {
		t.Fatalf("args = %v", def.Action(1).Args)
	}
}

func TestLayoutsHappyPath(t *testing.T) {
	store := loadSpawns(t, withFiles(map[string]string{
		"interface-layouts.json": `{
			"bank": {"id": 12, "name": "Bank", "surface": "main"},
			"prayer": {"id": 541, "name": "Prayer", "surface": "sidebar", "tabId": 5},
			"bank2": {"id": 12, "name": "Bank alt", "surface": "main"},
			"trade": {"id": 335, "name": "Trade", "surface": "main-with-sidebar", "sidebarInterfaceId": 336}
		}`,
	}), LoadOptions{})
	if got := len(store.Layouts()); got != 4 {
		t.Fatalf("layouts = %d", got)
	}
	bank, ok := store.Layout("bank")
	if !ok || bank.ID != 12 || bank.Name != "Bank" || bank.Surface != SurfaceMain {
		t.Fatalf("bank = %+v", bank)
	}
	if first, ok := store.LayoutByID(12); !ok || first.Key != "bank" {
		t.Fatalf("LayoutByID(12) = %+v (first in file order wins)", first)
	}
	prayer, _ := store.Layout("prayer")
	if prayer.TabID == nil || *prayer.TabID != 5 {
		t.Fatalf("prayer tab = %+v", prayer.TabID)
	}
	trade, _ := store.Layout("trade")
	if trade.SidebarInterfaceID == nil || *trade.SidebarInterfaceID != 336 {
		t.Fatalf("trade sidebar = %+v", trade.SidebarInterfaceID)
	}
	if _, ok := store.Layout("missing"); ok {
		t.Fatal("missing layout must be absent")
	}
}

func TestLayoutsValidationFailsLoad(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{"negative id", `{"a": {"id": -1, "name": "A", "surface": "main"}}`},
		{"string id", `{"a": {"id": "12", "name": "A", "surface": "main"}}`},
		{"blank name", `{"a": {"id": 1, "name": "  ", "surface": "main"}}`},
		{"bad surface", `{"a": {"id": 1, "name": "A", "surface": "nowhere"}}`},
		{"sidebar without tab", `{"a": {"id": 1, "name": "A", "surface": "sidebar"}}`},
		{"sidebar bad tab", `{"a": {"id": 1, "name": "A", "surface": "sidebar", "tabId": -1}}`},
		{"main-with-sidebar without id", `{"a": {"id": 1, "name": "A", "surface": "main-with-sidebar"}}`},
		{"entry not object", `{"a": 5}`},
		{"blank key", `{"": {"id": 1, "name": "A", "surface": "main"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixtureDir(t, withFiles(map[string]string{"interface-layouts.json": tc.content}))
			if _, err := Load(dir, LoadOptions{}); err == nil {
				t.Fatal("invalid layout must fail Load")
			}
		})
	}
}

func TestLayoutsDuplicateKeyKeepsLast(t *testing.T) {
	// JSON.parse keeps the last value at the first position; the
	// duplicate-key error never fires on parsed JSON.
	store := loadSpawns(t, withFiles(map[string]string{
		"interface-layouts.json": `{
			"a": {"id": 1, "name": "First", "surface": "main"},
			"a": {"id": 2, "name": "Second", "surface": "main"}
		}`,
	}), LoadOptions{})
	layouts := store.Layouts()
	if len(layouts) != 1 || layouts[0].ID != 2 || layouts[0].Name != "Second" {
		t.Fatalf("layouts = %+v", layouts)
	}
}

func TestLayoutsNonObjectFails(t *testing.T) {
	dir := fixtureDir(t, withFiles(map[string]string{"interface-layouts.json": `[1,2]`}))
	_, err := Load(dir, LoadOptions{})
	if err == nil || !strings.Contains(err.Error(), "JSON object") {
		t.Fatalf("error = %v", err)
	}
}

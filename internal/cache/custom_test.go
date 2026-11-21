package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func intPtr(v int) *int { return &v }

// customFixture builds a facade over the shared fake configs:
// 5 NPCs (0 "Goblin"), 7 items (1 "Bronze sword", 0 default
// "null"), 2 objects (0 "Tree").
func customFixture(t *testing.T) *CacheDefinitions {
	t.Helper()
	defs, _, _ := testFacade(t)
	return defs
}

func TestCustomDefinitionBasedNPC(t *testing.T) {
	defs := customFixture(t)
	n, err := defs.RegisterCustomDefinitions([]CustomDefinitionRow{
		{Kind: CustomKindNPC, ID: 50000, BaseID: intPtr(0), Fields: map[string]any{
			"name": "Custom Goblin", "hitpoints": 42, "combatLevel": 3,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("registered = %d, want 1", n)
	}
	// The overlay wins over the loader default for the appended id.
	got := defs.GetNpc(50000)
	if got.Name != "Custom Goblin" || got.Hitpoints != 42 || got.CombatLevel != 3 {
		t.Errorf("GetNpc(50000) = %+v", got)
	}
	if got.ID() != 50000 {
		t.Errorf("ID = %d, want 50000", got.ID())
	}
	// Unoverridden fields come from the base (Goblin: size 1).
	if got.Size != 1 {
		t.Errorf("Size = %d, want 1 (from base)", got.Size)
	}
	// The base definition is untouched by the overlay.
	if base := defs.GetNpc(0); base.Name != "Goblin" || base.Hitpoints != -1 {
		t.Errorf("base changed: %+v", base)
	}
	// An unregistered appended id still answers the loader default.
	if def := defs.GetNpc(49999); def.Name != "null" {
		t.Errorf("GetNpc(49999).Name = %q, want null default", def.Name)
	}
	if !defs.HasCustomDefinition(CustomKindNPC, 50000) {
		t.Error("HasCustomDefinition(npc, 50000) = false")
	}
	if defs.HasCustomDefinition(CustomKindNPC, 0) {
		t.Error("HasCustomDefinition(npc, 0) = true")
	}
}

func TestCustomDefinitionCountsExtended(t *testing.T) {
	defs := customFixture(t)
	if _, err := defs.RegisterCustomDefinitions([]CustomDefinitionRow{
		{Kind: CustomKindNPC, ID: 50000, BaseID: nil, Fields: map[string]any{"name": "Blank"}},
		{Kind: CustomKindLoc, ID: 50002, BaseID: nil, Fields: map[string]any{"name": "Blank loc"}},
		{Kind: CustomKindItem, ID: 50001, BaseID: nil, Fields: map[string]any{"name": "Blank item"}},
	}); err != nil {
		t.Fatal(err)
	}
	counts := defs.GetCounts()
	if counts.NPCs != 50001 || counts.Objects != 50003 || counts.Items != 50002 {
		t.Errorf("GetCounts = %+v", counts)
	}
	// NOTE: Go's GetBaseCounts shares the loader counts, so it
	// also reflects the overlay, unlike TypeScript's getBaseCounts
	// (which excludes custom definitions). That deviation is
	// documented in custom.go; pin the actual behavior here.
	if defs.GetBaseCounts() != counts {
		t.Errorf("GetBaseCounts = %+v, want %+v", defs.GetBaseCounts(), counts)
	}
}

func TestCustomDefinitionBlankBase(t *testing.T) {
	defs := customFixture(t)
	if _, err := defs.RegisterCustomDefinitions([]CustomDefinitionRow{
		{Kind: CustomKindNPC, ID: 50000, BaseID: nil, Fields: map[string]any{"name": "Fresh", "size": 2}},
	}); err != nil {
		t.Fatal(err)
	}
	got := defs.GetNpc(50000)
	if got.Name != "Fresh" || got.Size != 2 {
		t.Errorf("blank NPC = %+v", got)
	}
	// Post ran: FootprintSize derives from size (NpcType.post).
	if got.FootprintSize <= 0 {
		t.Errorf("FootprintSize = %d, want > 0 (Post ran)", got.FootprintSize)
	}
	if got.SpawnDirection != 6 {
		t.Errorf("SpawnDirection = %d, want 6 (default)", got.SpawnDirection)
	}
}

func TestCustomDefinitionLocInteractive(t *testing.T) {
	defs := customFixture(t)
	if _, err := defs.RegisterCustomDefinitions([]CustomDefinitionRow{
		{Kind: CustomKindLoc, ID: 50000, BaseID: intPtr(0), Fields: map[string]any{
			"name": "Custom Tree", "actions": []any{"Chop", nil, nil, nil, nil},
		}},
		{Kind: CustomKindLoc, ID: 50001, BaseID: intPtr(0), Fields: map[string]any{
			"name": "Quiet Rock", "actions": []any{nil, nil, nil, nil, nil},
		}},
		{Kind: CustomKindLoc, ID: 50002, BaseID: intPtr(0), Fields: map[string]any{
			"name": "Forced", "actions": []any{"Use", nil, nil, nil, nil}, "isInteractive": 0,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := defs.GetObject(50000); got.IsInteractive != 1 || got.Actions[0] != "Chop" {
		t.Errorf("interactive loc = %+v", got)
	}
	if got := defs.GetObject(50001); got.IsInteractive != 0 {
		t.Errorf("quiet loc IsInteractive = %d, want 0", got.IsInteractive)
	}
	// An explicit isInteractive wins over the derivation.
	if got := defs.GetObject(50002); got.IsInteractive != 0 {
		t.Errorf("forced loc IsInteractive = %d, want 0", got.IsInteractive)
	}
	// The base loc is untouched.
	if base := defs.GetObject(0); base.Name != "Tree" {
		t.Errorf("base loc changed: %+v", base)
	}
}

func TestCustomDefinitionParams(t *testing.T) {
	defs := customFixture(t)
	if _, err := defs.RegisterCustomDefinitions([]CustomDefinitionRow{
		{Kind: CustomKindNPC, ID: 50000, BaseID: intPtr(0), Fields: map[string]any{
			"params": map[string]any{"10": 77, "11": "s"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	params := defs.GetNpc(50000).Params
	if v, ok := ParamInt(params, 10); !ok || v != 77 {
		t.Errorf("params[10] = %v, %v", v, ok)
	}
	if v, ok := ParamString(params, 11); !ok || v != "s" {
		t.Errorf("params[11] = %v, %v", v, ok)
	}
}

func TestCustomPrecedenceOverArchived(t *testing.T) {
	defs := customFixture(t)
	// The custom layer registers first: with only 5 native NPCs,
	// id 50000 is a valid overlay id.
	if _, err := defs.RegisterCustomDefinitions([]CustomDefinitionRow{
		{Kind: CustomKindNPC, ID: 50000, BaseID: intPtr(0), Fields: map[string]any{"name": "Custom Wins"}},
	}); err != nil {
		t.Fatal(err)
	}
	// An archived bundle appending the SAME id: TypeScript's
	// registerDefinitionArchives would refuse this collision, but
	// the Go guard lives in facade.go and is absent (see
	// custom.go), so the bundle publishes. The getter order still
	// decides: the custom definition wins over the archived one,
	// as in TypeScript's getNpc.
	bundle := archivedBundle("old", archivedRow(ArchivedKindNPC, "old_goblin", 30, 50000, npcData("Archived Goblin", nil)))
	if _, err := defs.RegisterDefinitionArchives([]ArchivedDefinitionBundle{bundle}); err != nil {
		t.Fatal(err)
	}
	if got := defs.GetNpc(50000); got.Name != "Custom Wins" {
		t.Errorf("GetNpc(50000) = %q, want Custom Wins", got.Name)
	}
	// The reverse order is refused by identity validation: once
	// the archived bundle extends the base count to 50001, id
	// 50000 is below max(50000, count).
	defs2 := customFixture(t)
	if _, err := defs2.RegisterDefinitionArchives([]ArchivedDefinitionBundle{bundle}); err != nil {
		t.Fatal(err)
	}
	if _, err := defs2.RegisterCustomDefinitions([]CustomDefinitionRow{
		{Kind: CustomKindNPC, ID: 50000, BaseID: intPtr(0), Fields: map[string]any{"name": "Too Late"}},
	}); err == nil {
		t.Error("custom definition below the extended base count accepted")
	}
}

func TestCustomDefinitionValidation(t *testing.T) {
	cases := []struct {
		name string
		rows []CustomDefinitionRow
	}{
		{"id below range", []CustomDefinitionRow{{Kind: CustomKindNPC, ID: 49999, BaseID: nil, Fields: map[string]any{}}}},
		{"id at max", []CustomDefinitionRow{{Kind: CustomKindNPC, ID: 65535, BaseID: nil, Fields: map[string]any{}}}},
		{"bad kind", []CustomDefinitionRow{{Kind: "seq", ID: 50000, BaseID: nil, Fields: map[string]any{}}}},
		{"base out of range", []CustomDefinitionRow{{Kind: CustomKindNPC, ID: 50000, BaseID: intPtr(5), Fields: map[string]any{}}}},
		{"negative base", []CustomDefinitionRow{{Kind: CustomKindNPC, ID: 50000, BaseID: intPtr(-1), Fields: map[string]any{}}}},
		{"duplicate", []CustomDefinitionRow{
			{Kind: CustomKindNPC, ID: 50000, BaseID: nil, Fields: map[string]any{}},
			{Kind: CustomKindNPC, ID: 50000, BaseID: nil, Fields: map[string]any{}},
		}},
		{"forbidden field", []CustomDefinitionRow{{Kind: CustomKindNPC, ID: 50000, BaseID: nil, Fields: map[string]any{"id": 1}}}},
		{"unknown field", []CustomDefinitionRow{{Kind: CustomKindNPC, ID: 50000, BaseID: nil, Fields: map[string]any{"bogus": 1}}}},
		{"mistyped field", []CustomDefinitionRow{{Kind: CustomKindNPC, ID: 50000, BaseID: nil, Fields: map[string]any{"name": 5}}}},
		{"fractional field", []CustomDefinitionRow{{Kind: CustomKindNPC, ID: 50000, BaseID: nil, Fields: map[string]any{"hitpoints": 1.5}}}},
		{"bad params key", []CustomDefinitionRow{{Kind: CustomKindNPC, ID: 50000, BaseID: nil, Fields: map[string]any{"params": map[string]any{"x": 1}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defs := customFixture(t)
			if _, err := defs.RegisterCustomDefinitions(tc.rows); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestCustomDefinitionFailedRegistrationKeepsOld(t *testing.T) {
	defs := customFixture(t)
	if _, err := defs.RegisterCustomDefinitions([]CustomDefinitionRow{
		{Kind: CustomKindNPC, ID: 50000, BaseID: intPtr(0), Fields: map[string]any{"name": "Good"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := defs.RegisterCustomDefinitions([]CustomDefinitionRow{
		{Kind: CustomKindNPC, ID: 49999, BaseID: nil, Fields: map[string]any{}},
	}); err == nil {
		t.Fatal("expected error")
	}
	if got := defs.GetNpc(50000); got.Name != "Good" {
		t.Errorf("overlay changed after failed registration: %q", got.Name)
	}
}

func TestCustomItemsObjTypeApplication(t *testing.T) {
	defs := customFixture(t)
	err := defs.RegisterCustomItems([]CustomItem{{
		ID:         50000,
		BaseItemID: intPtr(1),
		ObjType: map[string]any{
			"name": "Custom Sword", "examine": "A custom blade.", "model": 1000000,
			"maleModel": 1000001, "femaleModel": 1000001,
			"zoom2d": 1800, "xan2d": 512, "isTradable": false,
			"recolorFrom": []any{}, "recolorTo": []any{},
			"inventoryActions": []any{"Wield", nil, nil, nil, "Drop"},
			"note":             -1, "noteTemplate": -1, "unnotedId": -1, "notedId": -1,
		},
		ItemDef: map[string]any{"tradeable": false},
		Models:  map[string]string{"1000000": "custom/drop.dat", "1000001": "custom/equip.dat"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := defs.GetItem(50000)
	if got.Name != "Custom Sword" || got.Model != 1000000 || got.Zoom2d != 1800 {
		t.Errorf("custom item = %+v", got)
	}
	if got.Examine == nil || *got.Examine != "A custom blade." {
		t.Errorf("examine = %v", got.Examine)
	}
	if len(got.InventoryActions) != 5 || got.InventoryActions[0] != "Wield" || got.InventoryActions[4] != "Drop" {
		t.Errorf("inventoryActions = %v", got.InventoryActions)
	}
	// Custom items do NOT extend the counts (TypeScript getCounts
	// only counts custom definitions).
	if counts := defs.GetCounts(); counts.Items != 7 {
		t.Errorf("GetCounts().Items = %d, want 7", counts.Items)
	}
	// The base item is untouched.
	if base := defs.GetItem(1); base.Name != "Bronze sword" {
		t.Errorf("base item changed: %+v", base)
	}
	items := defs.GetCustomItems()
	if len(items) != 1 || items[0].ID != 50000 || items[0].Models["1000000"] != "custom/drop.dat" {
		t.Errorf("GetCustomItems = %+v", items)
	}
}

func TestCustomItemsNoBase(t *testing.T) {
	defs := customFixture(t)
	if err := defs.RegisterCustomItems([]CustomItem{{
		ID:      50000,
		ObjType: map[string]any{"name": "Fresh Item", "model": 42},
	}}); err != nil {
		t.Fatal(err)
	}
	got := defs.GetItem(50000)
	if got.Name != "Fresh Item" || got.Model != 42 {
		t.Errorf("fresh item = %+v", got)
	}
	// Fresh ObjType defaults survive (price 1, zoom 2000).
	if got.Price != 1 || got.Zoom2d != 2000 {
		t.Errorf("fresh defaults: price=%d zoom=%d", got.Price, got.Zoom2d)
	}
}

func TestCustomItemsValidation(t *testing.T) {
	cases := []struct {
		name string
		rows []CustomItem
	}{
		{"id below range", []CustomItem{{ID: 49999, ObjType: map[string]any{"name": "X"}}}},
		{"id above range", []CustomItem{{ID: 65023, ObjType: map[string]any{"name": "X"}}}},
		{"duplicate", []CustomItem{
			{ID: 50000, ObjType: map[string]any{"name": "A"}},
			{ID: 50000, ObjType: map[string]any{"name": "B"}},
		}},
		{"missing objType", []CustomItem{{ID: 50000}}},
		{"missing name", []CustomItem{{ID: 50000, ObjType: map[string]any{"model": 1}}}},
		{"blank name", []CustomItem{{ID: 50000, ObjType: map[string]any{"name": "  "}}}},
		{"base out of range", []CustomItem{{ID: 50000, BaseItemID: intPtr(7), ObjType: map[string]any{"name": "X"}}}},
		{"base is null-named", []CustomItem{{ID: 50000, BaseItemID: intPtr(0), ObjType: map[string]any{"name": "X"}}}},
		{"forbidden property", []CustomItem{{ID: 50000, ObjType: map[string]any{"name": "X", "cacheInfo": 1}}}},
		{"unknown property", []CustomItem{{ID: 50000, ObjType: map[string]any{"name": "X", "bogus": 1}}}},
		{"short actions", []CustomItem{{ID: 50000, ObjType: map[string]any{"name": "X", "inventoryActions": []any{"Wield"}}}}},
		{"non-integer recolor", []CustomItem{{ID: 50000, ObjType: map[string]any{"name": "X", "recolorFrom": []any{"red"}}}}},
		{"recolor mismatch", []CustomItem{{ID: 50000, ObjType: map[string]any{"name": "X", "recolorFrom": []any{1}, "recolorTo": []any{}}}}},
		{"model id too small", []CustomItem{{ID: 50000, ObjType: map[string]any{"name": "X"}, Models: map[string]string{"999999": "m.dat"}}}},
		{"duplicate model id", []CustomItem{
			{ID: 50000, ObjType: map[string]any{"name": "A"}, Models: map[string]string{"1000000": "a.dat"}},
			{ID: 50001, ObjType: map[string]any{"name": "B"}, Models: map[string]string{"1000000": "b.dat"}},
		}},
		{"bad specialAttack", []CustomItem{{ID: 50000, ObjType: map[string]any{"name": "X"}, SpecialAttack: &CustomItemSpecialAttack{Energy: intPtr(101)}}}},
		{"specialAttack from self", []CustomItem{{ID: 50000, ObjType: map[string]any{"name": "X"}, SpecialAttack: &CustomItemSpecialAttack{From: intPtr(50000)}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defs := customFixture(t)
			if err := defs.RegisterCustomItems(tc.rows); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestCustomItemDefinitionCollision(t *testing.T) {
	// Definition first, then a colliding item.
	defs := customFixture(t)
	if _, err := defs.RegisterCustomDefinitions([]CustomDefinitionRow{
		{Kind: CustomKindItem, ID: 50000, BaseID: nil, Fields: map[string]any{"name": "Def item"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := defs.RegisterCustomItems([]CustomItem{{ID: 50000, ObjType: map[string]any{"name": "X"}}}); err == nil {
		t.Error("colliding custom item accepted")
	}
	// Item first, then a colliding definition.
	defs2 := customFixture(t)
	if err := defs2.RegisterCustomItems([]CustomItem{{ID: 50000, ObjType: map[string]any{"name": "X"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := defs2.RegisterCustomDefinitions([]CustomDefinitionRow{
		{Kind: CustomKindItem, ID: 50000, BaseID: nil, Fields: map[string]any{"name": "Def item"}},
	}); err == nil {
		t.Error("colliding custom definition accepted")
	}
}

func TestCustomItemPrecedenceOverDefinition(t *testing.T) {
	// The two layers cannot share an id (see the collision test),
	// so precedence between them is exercised by registering both
	// layers and confirming each id resolves to its own layer.
	defs := customFixture(t)
	if _, err := defs.RegisterCustomDefinitions([]CustomDefinitionRow{
		{Kind: CustomKindItem, ID: 50001, BaseID: intPtr(1), Fields: map[string]any{"name": "Def item"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := defs.RegisterCustomItems([]CustomItem{{ID: 50000, ObjType: map[string]any{"name": "Custom item"}}}); err != nil {
		t.Fatal(err)
	}
	if got := defs.GetItem(50000); got.Name != "Custom item" {
		t.Errorf("GetItem(50000) = %q", got.Name)
	}
	if got := defs.GetItem(50001); got.Name != "Def item" {
		t.Errorf("GetItem(50001) = %q", got.Name)
	}
	// Re-registering definitions preserves items, and vice versa.
	if _, err := defs.RegisterCustomDefinitions([]CustomDefinitionRow{
		{Kind: CustomKindItem, ID: 50002, BaseID: nil, Fields: map[string]any{"name": "Def item 2"}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := defs.GetItem(50000); got.Name != "Custom item" {
		t.Errorf("item lost after definition re-registration: %q", got.Name)
	}
	if got := defs.GetItem(50001); got.Name != "null" {
		t.Errorf("old definition survived replacement: %q", got.Name)
	}
}

func TestCustomOverlaySurvivesClearCache(t *testing.T) {
	defs := customFixture(t)
	if _, err := defs.RegisterCustomDefinitions([]CustomDefinitionRow{
		{Kind: CustomKindNPC, ID: 50000, BaseID: intPtr(0), Fields: map[string]any{"name": "Kept"}},
	}); err != nil {
		t.Fatal(err)
	}
	defs.npcs.ClearCache()
	defs.npcs.ClearArchivedCache()
	if got := defs.GetNpc(50000); got.Name != "Kept" {
		t.Errorf("overlay lost after ClearCache: %q", got.Name)
	}
}

func TestApplyCustomOverlaysMissingFiles(t *testing.T) {
	defs := customFixture(t)
	dir := t.TempDir()
	counts, err := ApplyCustomOverlays(dir, defs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if counts != (CustomOverlayCounts{}) {
		t.Errorf("counts = %+v, want zero", counts)
	}
	if len(defs.GetCustomDefinitions()) != 0 || len(defs.GetCustomItems()) != 0 {
		t.Error("overlay registered from missing files")
	}
}

func TestApplyCustomOverlaysNoCache(t *testing.T) {
	dir := t.TempDir()
	// Even malformed files must not matter: without a cache the
	// files are not read at all.
	if err := os.WriteFile(filepath.Join(dir, "custom-definitions.json"), []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}
	var logged []string
	counts, err := ApplyCustomOverlays(dir, nil, func(format string, args ...any) {
		logged = append(logged, format)
	})
	if err != nil {
		t.Fatal(err)
	}
	if counts != (CustomOverlayCounts{}) {
		t.Errorf("counts = %+v, want zero", counts)
	}
	if len(logged) != 1 || !strings.Contains(logged[0], "no cache") {
		t.Errorf("log = %v, want a no-cache note", logged)
	}
	// A nil logf suppresses the note without panicking.
	if _, err := ApplyCustomOverlays(dir, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestApplyCustomOverlaysFromFiles(t *testing.T) {
	defs := customFixture(t)
	dir := t.TempDir()
	defsJSON := `{"version":1,"rows":[
		{"kind":"npc","id":50000,"baseId":0,"fields":{"name":"File Goblin","hitpoints":30}},
		{"kind":"loc","id":50000,"baseId":null,"fields":{"name":"File Rock","sizeX":2,"sizeY":2}}
	]}`
	if err := os.WriteFile(filepath.Join(dir, "custom-definitions.json"), []byte(defsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	itemsJSON := `[{"id":50000,"baseItemId":1,"objType":{"name":"File Sword","model":1000000,"inventoryActions":["Wield",null,null,null,"Drop"]},"models":{"1000000":"file/drop.dat"}}]`
	if err := os.WriteFile(filepath.Join(dir, "custom-items.json"), []byte(itemsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	// custom-shops.json is never read by the overlay path: garbage
	// here must not fail the boot.
	if err := os.WriteFile(filepath.Join(dir, "custom-shops.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	counts, err := ApplyCustomOverlays(dir, defs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if counts != (CustomOverlayCounts{Definitions: 2, Items: 1}) {
		t.Errorf("counts = %+v", counts)
	}
	if got := defs.GetNpc(50000); got.Name != "File Goblin" || got.Hitpoints != 30 {
		t.Errorf("file NPC = %+v", got)
	}
	if got := defs.GetObject(50000); got.Name != "File Rock" || got.SizeX != 2 {
		t.Errorf("file loc = %+v", got)
	}
	if got := defs.GetItem(50000); got.Name != "File Sword" || got.Model != 1000000 {
		t.Errorf("file item = %+v", got)
	}
}

func TestApplyCustomOverlaysMalformed(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		content string
	}{
		{"defs bad json", "custom-definitions.json", "{bad"},
		{"defs not object", "custom-definitions.json", `[1,2]`},
		{"defs rows missing", "custom-definitions.json", `{"version":1}`},
		{"defs rows not array", "custom-definitions.json", `{"version":1,"rows":{}}`},
		{"defs row missing baseId", "custom-definitions.json", `{"version":1,"rows":[{"kind":"npc","id":50000,"fields":{}}]}`},
		{"defs row bad id", "custom-definitions.json", `{"version":1,"rows":[{"kind":"npc","id":1.5,"baseId":null,"fields":{}}]}`},
		{"items bad json", "custom-items.json", "{bad"},
		{"items not array or object", "custom-items.json", `"hello"`},
		{"items row not object", "custom-items.json", `[42]`},
		{"items row bad specialAttack key", "custom-items.json", `[{"id":50000,"objType":{"name":"X"},"specialAttack":{"bogus":1}}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defs := customFixture(t)
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, tc.file), []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := ApplyCustomOverlays(dir, defs, nil); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestApplyCustomOverlaysEmptyDocuments(t *testing.T) {
	defs := customFixture(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "custom-definitions.json"), []byte(`{"version":1,"rows":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "custom-items.json"), []byte(`[]`), 0o644); err != nil {
		t.Fatal(err)
	}
	counts, err := ApplyCustomOverlays(dir, defs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if counts != (CustomOverlayCounts{}) {
		t.Errorf("counts = %+v, want zero", counts)
	}
	// The object form of custom-items.json reads as its values.
	if err := os.WriteFile(filepath.Join(dir, "custom-items.json"), []byte(`{"a":{"id":50000,"objType":{"name":"Obj Form"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	counts, err = ApplyCustomOverlays(dir, defs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Items != 1 || defs.GetItem(50000).Name != "Obj Form" {
		t.Errorf("counts = %+v, item = %q", counts, defs.GetItem(50000).Name)
	}
}

func TestCustomGettersCloneIsolation(t *testing.T) {
	defs := customFixture(t)
	if _, err := defs.RegisterCustomDefinitions([]CustomDefinitionRow{
		{Kind: CustomKindNPC, ID: 50000, BaseID: intPtr(0), Fields: map[string]any{"name": "Original"}},
	}); err != nil {
		t.Fatal(err)
	}
	rows := defs.GetCustomDefinitions()
	rows[0].Fields["name"] = "Mutated"
	again := defs.GetCustomDefinitions()
	if again[0].Fields["name"] != "Original" {
		t.Errorf("getter mutation leaked: %v", again[0].Fields["name"])
	}
	if got := defs.GetNpc(50000); got.Name != "Original" {
		t.Errorf("resolved definition changed: %q", got.Name)
	}
}

func TestRegisterCustomNilFacade(t *testing.T) {
	var defs *CacheDefinitions
	if _, err := defs.RegisterCustomDefinitions(nil); err == nil {
		t.Error("RegisterCustomDefinitions on nil facade: expected error")
	}
	if err := defs.RegisterCustomItems(nil); err == nil {
		t.Error("RegisterCustomItems on nil facade: expected error")
	}
	if defs.HasCustomDefinition(CustomKindNPC, 1) {
		t.Error("HasCustomDefinition on nil facade = true")
	}
	if defs.GetCustomDefinitions() != nil || defs.GetCustomItems() != nil {
		t.Error("getters on nil facade returned rows")
	}
}

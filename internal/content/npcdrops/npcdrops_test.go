package npcdrops

import (
	"errors"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- fakes ----

type fakeSource struct {
	tables map[int][]*Table
	names  map[int]string
}

func (s *fakeSource) TablesForNPC(npcID int) []*Table { return s.tables[npcID] }
func (s *fakeSource) NPCName(npcID int) string        { return s.names[npcID] }

type fakeRoller struct {
	drops []Drop
	calls []rollCall
}

type rollCall struct {
	table *Table
	ctx   KillContext
}

func (r *fakeRoller) RollTable(table *Table, ctx KillContext) []Drop {
	r.calls = append(r.calls, rollCall{table: table, ctx: ctx})
	return append([]Drop(nil), r.drops...)
}

// goblinTable is the one-table fake source for Goblin 3036.
func goblinTable() map[int][]*Table {
	return map[int][]*Table{3036: {{ID: "goblin_drop_table1"}}}
}

type fakeDefs struct {
	stackable map[int]bool
	noted     map[int]bool
	noteIDs   map[int]int
}

func (s *fakeDefs) IsStackable(id int) bool      { return s.stackable[id] }
func (s *fakeDefs) IsNoted(id int) bool          { return s.noted[id] }
func (s *fakeDefs) WeightGrams(id int) int       { return 0 }
func (s *fakeDefs) EquipmentSlot(id int) int     { return -1 }
func (s *fakeDefs) IsDoubleHanded(id int) bool   { return false }
func (s *fakeDefs) Requirements(id int) []int    { return nil }
func (s *fakeDefs) CombatRequirement(id int) int { return 0 }
func (s *fakeDefs) IsTradeable(id int) bool      { return false }
func (s *fakeDefs) IsSellable(id int) bool       { return false }
func (s *fakeDefs) IsDropable(id int) bool       { return false }
func (s *fakeDefs) UnNote(id int) int            { return id }
func (s *fakeDefs) PlaceholderID(id int) int     { return -1 }

// NoteID is the game.ItemNoteIDSource capability: the cache Note
// field for the id, or -1 when the item has no note form.
func (s *fakeDefs) NoteID(id int) int {
	if note, ok := s.noteIDs[id]; ok {
		return note
	}
	return -1
}

var (
	_ game.ItemDefinitionSource = (*fakeDefs)(nil)
	_ game.ItemNoteIDSource     = (*fakeDefs)(nil)
)

type scriptCall struct {
	id   int
	args []int
}

// fakeSession implements game.DialogueSender (for script 7192)
// and game.ChatMessageSender (for reloaddrops messages),
// capturing both.
type fakeSession struct {
	scripts  []scriptCall
	messages []string
}

func (s *fakeSession) SendChatboxInterface(groupID int, unclamp bool) {}
func (s *fakeSession) SendNpcHeadOnInterface(npcID, uid int)          {}
func (s *fakeSession) SendPlayerHeadOnInterface(uid int)              {}
func (s *fakeSession) SendInterfaceAnimation(uid, animationID int)    {}
func (s *fakeSession) SendString(text string, uid int)                {}
func (s *fakeSession) SendClientScript(scriptID int, args ...game.ClientScriptArg) {
	call := scriptCall{id: scriptID}
	for _, a := range args {
		call.args = append(call.args, a.IntValue())
	}
	s.scripts = append(s.scripts, call)
}
func (s *fakeSession) SendInterfaceFlagsRange(uid, fromSlot, toSlot, flags int) {}
func (s *fakeSession) SendItemOnInterfaces(uid, itemID, amount int)             {}
func (s *fakeSession) SendInterfaceScript(scriptID int)                         {}
func (s *fakeSession) SendInterfaceRemoval()                                    {}
func (s *fakeSession) IsChatboxInterface(groupID int) bool                      { return false }
func (s *fakeSession) WithChatboxOwner(owner any, present func())               { present() }
func (s *fakeSession) SendGameMessage(message string)                           { s.messages = append(s.messages, message) }
func (s *fakeSession) SendTypedGameMessage(chatType int, message, from string) {
	s.messages = append(s.messages, message)
}

// Close satisfies game.SessionPort (the session is closed on
// disconnect; the fake has nothing to close).
func (s *fakeSession) Close(int, string) {}

// The rest of game.SessionPort, all no-ops for the fake.
func (s *fakeSession) SendPacket(packet []byte) {}
func (s *fakeSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
}
func (s *fakeSession) SendDestination(x, y int) {}
func (s *fakeSession) SendVarp(id, value int)   {}
func (s *fakeSession) FlushTick(cycle int64) error {
	return nil
}
func (s *fakeSession) IsOpen() bool { return true }

var _ game.DialogueSender = (*fakeSession)(nil)

type groundCall struct {
	item game.Item
	loc  game.Location
}

// testPlayer builds a killer with the given defs and a capturing
// session.
func testPlayer(defs game.ItemDefinitionSource) (*game.Player, *fakeSession) {
	p := game.NewPlayer("alice", game.Location{X: 3185, Y: 3244, Z: 0})
	if defs != nil {
		p.SetItemDefinitions(defs)
	}
	s := &fakeSession{}
	p.Session = s
	return p, s
}

// installGround captures DropGroundItem calls, restoring the seam
// on cleanup.
func installGround(t *testing.T) *[]groundCall {
	t.Helper()
	calls := &[]groundCall{}
	prev := game.Hooks.DropGroundItem
	game.Hooks.DropGroundItem = func(item game.Item, loc game.Location, owner *game.Player) {
		*calls = append(*calls, groundCall{item: item, loc: loc})
	}
	t.Cleanup(func() { game.Hooks.DropGroundItem = prev })
	return calls
}

// testDrops builds a plugin instance whose emit records event
// names and runs per-event listeners supplied by the test.
func testDrops(listeners map[string]func(any)) *npcDrops {
	return &npcDrops{
		emit: func(name string, payload any) {
			if fn, ok := listeners[name]; ok {
				fn(payload)
			}
		},
	}
}

func deathEvent(killer *game.Player, npc *game.NPC, npcID int) *plugins.NpcDeathEvent {
	return &plugins.NpcDeathEvent{
		Killer:   killer,
		NPC:      npc,
		NpcID:    npcID,
		Location: game.Location{X: 3185, Y: 3244, Z: 0},
		Damagers: []*game.Player{killer},
	}
}

// ---- handler early returns ----

func TestHandlerEarlyReturns(t *testing.T) {
	roller := &fakeRoller{drops: []Drop{{ItemID: 526, Amount: 1}}}
	killer, _ := testPlayer(nil)
	npc := &game.NPC{ID: 3036}
	called := false
	n := testDrops(map[string]func(any){
		"npc-drops:claim": func(any) { called = true },
	})
	n.source = &fakeSource{tables: goblinTable()}
	n.roller = roller

	n.npcDied(nil)
	n.npcDied(&plugins.NpcDeathEvent{NPC: npc, NpcID: 3036})
	n.npcDied(&plugins.NpcDeathEvent{Killer: killer, NpcID: 3036})
	if called || len(roller.calls) != 0 {
		t.Fatalf("early-return case reached claim/roll: claim=%v rolls=%d", called, len(roller.calls))
	}
}

// ---- no table: no base drops ----

func TestNoTableNoBaseDrops(t *testing.T) {
	calls := installGround(t)
	roller := &fakeRoller{drops: []Drop{{ItemID: 526, Amount: 1}}}
	killer, _ := testPlayer(nil)
	n := testDrops(nil)
	// Vorkath 8058 has 0 tables (PLAN §1.2).
	n.source = &fakeSource{tables: map[int][]*Table{}}
	n.roller = roller

	n.npcDied(deathEvent(killer, &game.NPC{ID: 8058}, 8058))
	if len(roller.calls) != 0 {
		t.Fatalf("roller called for a no-table NPC: %v", roller.calls)
	}
	if len(*calls) != 0 {
		t.Fatalf("ground calls for a no-table NPC: %v", *calls)
	}
}

// ---- first-table default + picker ----

func TestFirstTableDefaultAndPicker(t *testing.T) {
	killer, _ := testPlayer(nil)
	npc := &game.NPC{ID: 13489}
	loc := game.Location{X: 3185, Y: 3244, Z: 0}

	// No picker: the first table rolls.
	roller := &fakeRoller{drops: []Drop{{ItemID: 526, Amount: 1}}}
	n := testDrops(nil)
	n.source = &fakeSource{tables: map[int][]*Table{13489: {{ID: "zombie_pirate"}, {ID: "zombie_pirate_2"}}}}
	n.roller = roller
	n.dropFor(killer, npc, 13489, loc, nil)
	if len(roller.calls) != 1 || roller.calls[0].table.ID != "zombie_pirate" {
		t.Fatalf("no-picker roll = %v, want first table zombie_pirate", roller.calls)
	}

	// A npc-drops:table listener picking the second table wins.
	roller2 := &fakeRoller{drops: []Drop{{ItemID: 526, Amount: 1}}}
	n2 := testDrops(map[string]func(any){
		"npc-drops:table": func(payload any) {
			ev := payload.(*TableEvent)
			if len(ev.Tables) != 2 || ev.Tables[0].ID != "zombie_pirate" {
				t.Errorf("table event tables = %v", ev.Tables)
			}
			ev.Table = ev.Tables[1]
		},
	})
	n2.source = n.source
	n2.roller = roller2
	n2.dropFor(killer, npc, 13489, loc, nil)
	if len(roller2.calls) != 1 || roller2.calls[0].table.ID != "zombie_pirate_2" {
		t.Fatalf("picker roll = %v, want second table zombie_pirate_2", roller2.calls)
	}
}

// ---- claim handled skips the normal path ----

func TestClaimHandledSkipsNormalPath(t *testing.T) {
	calls := installGround(t)
	roller := &fakeRoller{drops: []Drop{{ItemID: 526, Amount: 1}}}
	killer, sess := testPlayer(nil)
	n := testDrops(map[string]func(any){
		"npc-drops:claim": func(payload any) {
			payload.(*ClaimEvent).Handled = true
		},
	})
	n.source = &fakeSource{tables: goblinTable()}
	n.roller = roller

	n.npcDied(deathEvent(killer, &game.NPC{ID: 3036}, 3036))
	if len(roller.calls) != 0 || len(*calls) != 0 || len(sess.scripts) != 0 {
		t.Fatalf("handled claim still rolled/placed/reported: rolls=%d ground=%d scripts=%d",
			len(roller.calls), len(*calls), len(sess.scripts))
	}
}

// ---- generated-handled short-circuits placement ----

func TestGeneratedHandledShortCircuitsPlacement(t *testing.T) {
	calls := installGround(t)
	roller := &fakeRoller{drops: []Drop{{ItemID: 526, Amount: 1}, {ItemID: 995, Amount: 15}}}
	killer, sess := testPlayer(&fakeDefs{stackable: map[int]bool{526: false, 995: true}})
	n := testDrops(map[string]func(any){
		"npc-drops:generated": func(payload any) {
			payload.(*GeneratedEvent).Handled = true
		},
	})
	n.source = &fakeSource{tables: goblinTable()}
	n.roller = roller

	placed := n.dropFor(killer, &game.NPC{ID: 3036}, 3036, game.Location{X: 3185, Y: 3244, Z: 0}, nil)
	if placed != 2 {
		t.Fatalf("dropFor returned %d, want rolled count 2", placed)
	}
	if len(*calls) != 0 {
		t.Fatalf("handled generated still placed: %v", *calls)
	}
	if len(sess.scripts) != 0 {
		t.Fatalf("handled generated still reported loot: %v", sess.scripts)
	}
}

// ---- noted swap uses the grounded lookup ----

func TestNotedSwapUsesNoteLookup(t *testing.T) {
	calls := installGround(t)
	// Grimy ranarr weed 207 (noted form 208) is stackable as a
	// note; the unnoted herb is not.
	defs := &fakeDefs{
		stackable: map[int]bool{208: true},
		noted:     map[int]bool{208: true},
		noteIDs:   map[int]int{207: 208},
	}
	killer, _ := testPlayer(defs)
	roller := &fakeRoller{drops: []Drop{{ItemID: 207, Amount: 2, Noted: true}}}
	n := testDrops(nil)
	n.source = &fakeSource{tables: goblinTable()}
	n.roller = roller

	n.dropFor(killer, &game.NPC{ID: 3036}, 3036, game.Location{X: 3185, Y: 3244, Z: 0}, nil)
	if len(*calls) != 1 {
		t.Fatalf("ground calls = %d, want 1 noted stack", len(*calls))
	}
	if got := (*calls)[0].item.ID(); got != 208 {
		t.Fatalf("placed id = %d, want note id 208", got)
	}
	if got := (*calls)[0].item.Amount(); got != 2 {
		t.Fatalf("placed amount = %d, want 2", got)
	}
}

func TestNotedSwapFallsBackWhenNoteIsNotNoted(t *testing.T) {
	calls := installGround(t)
	// A note id that is not itself a note (the TS
	// forId(noteId).isNoted() guard) lands unnoted.
	defs := &fakeDefs{
		stackable: map[int]bool{207: false},
		noted:     map[int]bool{},
		noteIDs:   map[int]int{207: 9999},
	}
	killer, _ := testPlayer(defs)
	roller := &fakeRoller{drops: []Drop{{ItemID: 207, Amount: 1, Noted: true}}}
	n := testDrops(nil)
	n.source = &fakeSource{tables: goblinTable()}
	n.roller = roller

	n.dropFor(killer, &game.NPC{ID: 3036}, 3036, game.Location{X: 3185, Y: 3244, Z: 0}, nil)
	if len(*calls) != 1 || (*calls)[0].item.ID() != 207 {
		t.Fatalf("ground calls = %v, want one unnoted 207", *calls)
	}
}

// ---- stackable vs non-stackable placement ----

func TestStackableVsNonStackablePlacement(t *testing.T) {
	calls := installGround(t)
	defs := &fakeDefs{stackable: map[int]bool{995: true, 1173: false}}
	killer, _ := testPlayer(defs)
	roller := &fakeRoller{drops: []Drop{
		{ItemID: 995, Amount: 15}, // coins: one stack
		{ItemID: 1173, Amount: 3}, // bronze sq shield: 3 singles
	}}
	n := testDrops(nil)
	n.source = &fakeSource{tables: goblinTable()}
	n.roller = roller

	n.dropFor(killer, &game.NPC{ID: 3036}, 3036, game.Location{X: 3185, Y: 3244, Z: 0}, nil)
	if len(*calls) != 4 {
		t.Fatalf("ground calls = %d, want 1 stack + 3 singles", len(*calls))
	}
	if (*calls)[0].item.ID() != 995 || (*calls)[0].item.Amount() != 15 {
		t.Fatalf("first call = %v, want coins x15", (*calls)[0].item)
	}
	for _, c := range (*calls)[1:] {
		if c.item.ID() != 1173 || c.item.Amount() != 1 {
			t.Fatalf("single call = %v, want shield x1", c.item)
		}
	}
}

// ---- validDrop filtering + location event ----

func TestValidDropFiltering(t *testing.T) {
	calls := installGround(t)
	killer, _ := testPlayer(&fakeDefs{stackable: map[int]bool{526: false, 995: true}})
	roller := &fakeRoller{drops: []Drop{
		{ItemID: -1, Amount: 1},  // invalid id
		{ItemID: 526, Amount: 0}, // invalid amount
		{ItemID: 526, Amount: 1}, // valid
		{ItemID: 995, Amount: 5}, // valid
	}}
	n := testDrops(nil)
	n.source = &fakeSource{tables: goblinTable()}
	n.roller = roller

	n.dropFor(killer, &game.NPC{ID: 3036}, 3036, game.Location{X: 3185, Y: 3244, Z: 0}, nil)
	if len(*calls) != 2 {
		t.Fatalf("ground calls = %d, want 2 valid drops", len(*calls))
	}
}

func TestLocationEventMovesDrops(t *testing.T) {
	calls := installGround(t)
	killer, _ := testPlayer(&fakeDefs{stackable: map[int]bool{526: false}})
	roller := &fakeRoller{drops: []Drop{{ItemID: 526, Amount: 1}}}
	moved := game.Location{X: 3200, Y: 3200, Z: 0}
	n := testDrops(map[string]func(any){
		"npc-drops:location": func(payload any) {
			payload.(*LocationEvent).Location = moved
		},
	})
	n.source = &fakeSource{tables: goblinTable()}
	n.roller = roller

	n.dropFor(killer, &game.NPC{ID: 3036}, 3036, game.Location{X: 3185, Y: 3244, Z: 0}, nil)
	if len(*calls) != 1 || (*calls)[0].loc != moved {
		t.Fatalf("ground calls = %v, want one drop at %v", *calls, moved)
	}
}

// ---- kill context: name + wilderness ----

func TestKillContextCarriesNameAndWilderness(t *testing.T) {
	killer, _ := testPlayer(nil)
	roller := &fakeRoller{drops: []Drop{{ItemID: 526, Amount: 1}}}
	n := testDrops(nil)
	n.source = &fakeSource{
		tables: goblinTable(),
		names:  map[int]string{3036: "Goblin"},
	}
	n.roller = roller
	n.wilderness = func(loc game.Location) bool { return loc.X > 3200 }

	loc := game.Location{X: 3201, Y: 3244, Z: 0}
	n.dropFor(killer, &game.NPC{ID: 3036}, 3036, loc, nil)
	if len(roller.calls) != 1 {
		t.Fatalf("roller calls = %d, want 1", len(roller.calls))
	}
	ctx := roller.calls[0].ctx
	if ctx.NPCName != "Goblin" || !ctx.InWilderness || ctx.MapY != 3244 {
		t.Fatalf("kill context = %+v, want name Goblin + wilderness + mapY 3244", ctx)
	}
}

// ---- reportLoot: script 7192, per-kill key ----

func TestReportLootScript7192(t *testing.T) {
	killer, sess := testPlayer(&fakeDefs{stackable: map[int]bool{526: false, 995: true}})
	n := testDrops(nil)
	drops := []Drop{{ItemID: 526, Amount: 1}, {ItemID: 995, Amount: 15}}

	n.reportLoot(killer, 3036, drops)
	if len(sess.scripts) != 2 {
		t.Fatalf("scripts = %d, want 1 per drop", len(sess.scripts))
	}
	for i, call := range sess.scripts {
		if call.id != LootTrackerScript {
			t.Fatalf("script %d = %d, want 7192", i, call.id)
		}
		if len(call.args) != 4 {
			t.Fatalf("script %d args = %v, want (npc, key, item, amount)", i, call.args)
		}
		if call.args[0] != 3036 {
			t.Fatalf("script %d npc = %d, want 3036", i, call.args[0])
		}
	}
	if sess.scripts[0].args[1] != sess.scripts[1].args[1] {
		t.Fatalf("same kill must share one key: %v", sess.scripts)
	}
	if sess.scripts[0].args[2] != 526 || sess.scripts[0].args[3] != 1 {
		t.Fatalf("first script args = %v, want (3036, key, 526, 1)", sess.scripts[0].args)
	}
	if sess.scripts[1].args[2] != 995 || sess.scripts[1].args[3] != 15 {
		t.Fatalf("second script args = %v, want (3036, key, 995, 15)", sess.scripts[1].args)
	}
	firstKey := sess.scripts[0].args[1]

	// The next kill's key increments by exactly one.
	n.reportLoot(killer, 3036, drops[:1])
	if len(sess.scripts) != 3 || sess.scripts[2].args[1] != firstKey+1 {
		t.Fatalf("second kill key = %v, want %d", sess.scripts, firstKey+1)
	}
}

func TestReportLootSkipsInvalidDrops(t *testing.T) {
	killer, sess := testPlayer(nil)
	n := testDrops(nil)
	n.reportLoot(killer, 3036, []Drop{{ItemID: -1, Amount: 1}, {ItemID: 526, Amount: 0}})
	if len(sess.scripts) != 0 {
		t.Fatalf("scripts sent for invalid drops: %v", sess.scripts)
	}
}

// ---- reloaddrops ----

func TestReloadDropsCommand(t *testing.T) {
	killer, sess := testPlayer(nil)
	n := testDrops(nil)
	n.reloader = func() (int, int, error) { return 3086, 1106, nil }
	if !n.reloadDrops(&game.CommandEvent{Player: killer, Parts: []string{"reloaddrops"}}) {
		t.Fatal("reloadDrops returned false")
	}
	if len(sess.messages) != 1 || sess.messages[0] != "Reloaded drops: 3086 npcs, 1106 tables." {
		t.Fatalf("messages = %v", sess.messages)
	}
}

func TestReloadDropsError(t *testing.T) {
	killer, sess := testPlayer(nil)
	n := testDrops(nil)
	n.reloader = func() (int, int, error) { return 0, 0, errors.New("boom") }
	n.reloadDrops(&game.CommandEvent{Player: killer, Parts: []string{"reloaddrops"}})
	if len(sess.messages) != 1 || sess.messages[0] != "Error reloading npc drops." {
		t.Fatalf("messages = %v", sess.messages)
	}
}

func TestReloadDropsUnavailableWithoutReloader(t *testing.T) {
	killer, sess := testPlayer(nil)
	n := testDrops(nil)
	n.reloadDrops(&game.CommandEvent{Player: killer, Parts: []string{"reloaddrops"}})
	if len(sess.messages) != 1 || sess.messages[0] != "Drops reload is not available." {
		t.Fatalf("messages = %v", sess.messages)
	}
	if n.reloadDrops(nil) {
		t.Fatal("nil event must return false")
	}
}

// ---- hub registration: the plugin loads and fires ----

func TestPluginLoadsAndFiresOnNpcDeath(t *testing.T) {
	calls := installGround(t)
	prevSource, prevRoller := installedSource, installedRoller
	installedSource = &fakeSource{
		tables: goblinTable(),
		names:  map[int]string{3036: "Goblin"},
	}
	roller := &fakeRoller{drops: []Drop{{ItemID: 526, Amount: 1}}}
	installedRoller = roller
	t.Cleanup(func() { installedSource, installedRoller = prevSource, prevRoller })

	hub := plugins.NewHub([]plugins.Plugin{Plugin}, plugins.HubOptions{})
	hub.Load()
	if len(hub.Loaded()) != 1 {
		t.Fatalf("loaded = %v, want [NpcDrops]", hub.Loaded())
	}
	killer, _ := testPlayer(&fakeDefs{stackable: map[int]bool{526: false}})
	hub.EmitNpcDeath(deathEvent(killer, &game.NPC{ID: 3036}, 3036))
	if len(roller.calls) != 1 || roller.calls[0].table.ID != "goblin_drop_table1" {
		t.Fatalf("roller calls = %v, want one goblin_drop_table1 roll", roller.calls)
	}
	if len(*calls) != 1 || (*calls)[0].item.ID() != 526 {
		t.Fatalf("ground calls = %v, want one Bones", *calls)
	}
}

func TestPluginFiresWhenSeamsInstalledAfterLoad(t *testing.T) {
	// Regression: production installs the seams AFTER hub.Load()
	// (cmd/server/server.go: the hub loads at :185, the definition
	// Store — and therefore installNpcDropsWiring — lands at :298).
	// The plugin must resolve the seams lazily at kill time, not
	// snapshot them at Register: installing after Load must still
	// make kills roll. (Phase 50 live leg: the goblin leg saw 8
	// kills land with no drops until this was fixed.)
	calls := installGround(t)
	hub := plugins.NewHub([]plugins.Plugin{Plugin}, plugins.HubOptions{})
	hub.Load()
	if len(hub.Loaded()) != 1 {
		t.Fatalf("loaded = %v, want [NpcDrops]", hub.Loaded())
	}

	prevSource, prevRoller := installedSource, installedRoller
	installedSource = &fakeSource{
		tables: goblinTable(),
		names:  map[int]string{3036: "Goblin"},
	}
	roller := &fakeRoller{drops: []Drop{{ItemID: 526, Amount: 1}}}
	installedRoller = roller
	t.Cleanup(func() { installedSource, installedRoller = prevSource, prevRoller })

	killer, _ := testPlayer(&fakeDefs{stackable: map[int]bool{526: false}})
	hub.EmitNpcDeath(deathEvent(killer, &game.NPC{ID: 3036}, 3036))
	if len(roller.calls) != 1 || roller.calls[0].table.ID != "goblin_drop_table1" {
		t.Fatalf("roller calls = %v, want one goblin_drop_table1 roll", roller.calls)
	}
	if len(*calls) != 1 || (*calls)[0].item.ID() != 526 {
		t.Fatalf("ground calls = %v, want one Bones", *calls)
	}
}

package main

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/persist"
	"tsps-go/internal/plugins"
	"tsps-go/internal/session"
)

// Spellbook/autocast entry-load tests: applyEntrySpellbook and
// restoreEntryAutocast in spellwiring.go. Fixtures only: a
// hand-built player and a hand-built carried save. The
// staff-equipped restore branch is covered in the game package
// (TestRestoreAutocastOnLogin); here the no-staff remembered
// branch and the book-mismatch clear are covered without a
// combat-styles registry.

func TestApplyEntrySpellbook(t *testing.T) {
	p := game.NewPlayer("booker", game.Location{X: 3200, Y: 3200, Z: 0})
	if got := p.Spellbook(); got != game.SpellbookNormal {
		t.Fatalf("fresh player spellbook = %v, want NORMAL", got)
	}
	carried := &persist.PlayerSave{SpellBook: persist.SpellBook{InterfaceID: 12855}}
	applyEntrySpellbook(p, carried)
	if got := p.Spellbook(); got != game.SpellbookAncient {
		t.Fatalf("spellbook = %v, want ANCIENT", got)
	}
	carried.SpellBook.InterfaceID = 29999
	applyEntrySpellbook(p, carried)
	if got := p.Spellbook(); got != game.SpellbookLunar {
		t.Fatalf("spellbook = %v, want LUNAR", got)
	}
	carried.SpellBook.InterfaceID = 4242
	applyEntrySpellbook(p, carried)
	if got := p.Spellbook(); got != game.SpellbookNormal {
		t.Fatalf("spellbook = %v, want NORMAL for an unknown interface id", got)
	}
	// Nil guards: no panic, no change.
	applyEntrySpellbook(nil, carried)
	applyEntrySpellbook(p, nil)
	if got := p.Spellbook(); got != game.SpellbookNormal {
		t.Fatalf("spellbook after nil guards = %v, want NORMAL", got)
	}
}

func TestRestoreEntryAutocast(t *testing.T) {
	// The standard spells register through
	// InstallStandardSpells (Worker A); the boot wiring
	// calls it too, but tests install it directly.
	game.InstallStandardSpells()
	p := game.NewPlayer("caster", game.Location{X: 3200, Y: 3200, Z: 0})
	carried := &persist.PlayerSave{
		SpellBook:       persist.SpellBook{InterfaceID: 1151},
		AutocastSpellId: 1152,
	}
	applyEntrySpellbook(p, carried)
	// No staff equipped: the saved spell is remembered (the
	// no-staff branch writes the slot directly, silently).
	restoreEntryAutocast(p, carried)
	spell := p.GetCombat().AutoCastSpell()
	if spell == nil {
		t.Fatal("restored spell = nil, want Wind Strike remembered")
	}
	if withID, ok := spell.(interface{ SpellID() int }); ok && withID.SpellID() != 1152 {
		t.Fatalf("restored spell id = %d, want 1152", withID.SpellID())
	}
	// Book mismatch: the spell clears.
	carried.SpellBook.InterfaceID = 12855
	applyEntrySpellbook(p, carried)
	restoreEntryAutocast(p, carried)
	if p.GetCombat().AutoCastSpell() != nil {
		t.Fatal("book-mismatched spell was not cleared")
	}
	// Nil guards: no panic, no change.
	restoreEntryAutocast(nil, carried)
	restoreEntryAutocast(p, nil)
	if p.GetCombat().AutoCastSpell() != nil {
		t.Fatal("nil guards changed the autocast spell")
	}
}

// TestSnapshotWritesLiveAutocastSpell covers the Phase 27 save-split
// fix: autocastSpellId is owned state (PlayerSave.ts:756), so a
// snapshot writes the live combat autocast spell id — and -1 once
// it is cleared — instead of preserving the carried value.
func TestSnapshotWritesLiveAutocastSpell(t *testing.T) {
	game.InstallStandardSpells()
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	save.AutocastSpellId = -1
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Save: save, PasswordHash: "h", CredentialName: "Alice"})
	game.SetAutocast(p, game.GetCombatSpell(1152))
	if err := h.coord.snapshot(p, "autosave"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	stored, err := h.store.inner.Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("load stored save: %v", err)
	}
	if stored.AutocastSpellId != 1152 {
		t.Fatalf("stored autocastSpellId = %d, want 1152 (live pick)", stored.AutocastSpellId)
	}
	// Clearing the pick snapshots back to -1 (the ?? -1 case).
	game.SetAutocast(p, nil)
	if err := h.coord.snapshot(p, "autosave"); err != nil {
		t.Fatalf("snapshot after clear: %v", err)
	}
	stored, err = h.store.inner.Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("load stored save after clear: %v", err)
	}
	if stored.AutocastSpellId != -1 {
		t.Fatalf("stored autocastSpellId = %d, want -1 (live clear)", stored.AutocastSpellId)
	}
}

// TestInstallSpellListeners covers the coordinator's Deps wiring
// in spellwiring.go: nil-safe degradation (nil defs/world) and
// forwarding of the gate adapters to the game.Hooks spell fields
// (populated by InstallCombatHooks in buildServer). The Hooks
// fields are process-global, so the test saves and restores the
// four it touches.
func TestInstallSpellListeners(t *testing.T) {
	// Nil deps: no panic.
	InstallSpellListeners(nil, nil, nil, nil)

	prevDisabled, prevBypass := game.Hooks.SpellDisabled, game.Hooks.SpellRuneBypass
	prevCheck, prevConsume := game.Hooks.CheckSpellRuneSource, game.Hooks.ConsumeSpellRuneSource
	defer func() {
		game.Hooks.SpellDisabled, game.Hooks.SpellRuneBypass = prevDisabled, prevBypass
		game.Hooks.CheckSpellRuneSource, game.Hooks.ConsumeSpellRuneSource = prevCheck, prevConsume
	}()
	game.Hooks.SpellDisabled, game.Hooks.SpellRuneBypass = nil, nil
	game.Hooks.CheckSpellRuneSource, game.Hooks.ConsumeSpellRuneSource = nil, nil

	var deps listeners.Deps
	InstallSpellListeners(&deps, nil, nil, nil)
	if deps.SpellHooks.EmitSpellOnItem != nil {
		t.Fatal("EmitSpellOnItem wired with nil hub")
	}
	if deps.SpellHooks.SpellName != nil {
		t.Fatal("SpellName wired with nil definitions")
	}
	if deps.SpellHooks.ScheduleTask != nil {
		t.Fatal("ScheduleTask wired with nil world")
	}
	if got := deps.SpellHooks.SpellbookInterfaceID(nil); got != 1151 {
		t.Fatalf("SpellbookInterfaceID(nil) = %d, want 1151 (NORMAL)", got)
	}
	p := game.NewPlayer("caster", game.Location{X: 3200, Y: 3200, Z: 0})
	if got := deps.SpellHooks.SpellbookInterfaceID(p); got != 1151 {
		t.Fatalf("SpellbookInterfaceID = %d, want 1151 (NORMAL)", got)
	}
	p.SetSpellbook(game.SpellbookAncient)
	if got := deps.SpellHooks.SpellbookInterfaceID(p); got != 12855 {
		t.Fatalf("SpellbookInterfaceID = %d, want 12855 (ANCIENT)", got)
	}
	// Nil-safe gates with unwired Hooks: all refuse nothing.
	if deps.SpellHooks.SpellDisabled(p, 1152, nil) {
		t.Fatal("SpellDisabled true with unwired Hooks")
	}
	if deps.SpellHooks.SpellRuneBypass(p, 1152, 556) {
		t.Fatal("SpellRuneBypass true with unwired Hooks")
	}
	if deps.SpellHooks.CheckSpellRuneSource(p, nil) {
		t.Fatal("CheckSpellRuneSource true with unwired Hooks")
	}
	deps.SpellHooks.ConsumeSpellRuneSource(p, nil)
	deps.SpellHooks.CastSpell(nil, nil, nil)

	// Forwarding: stub the Hooks fields and check the adapters
	// call through with the NORMAL book and the spell passed on.
	game.Hooks.SpellDisabled = func(_ *game.Player, book game.Spellbook, id int, _ game.CombatSpell) bool {
		return book == game.SpellbookNormal && id == 1152
	}
	if !deps.SpellHooks.SpellDisabled(p, 1152, nil) {
		t.Fatal("SpellDisabled did not forward to Hooks")
	}
	if deps.SpellHooks.SpellDisabled(p, 1153, nil) {
		t.Fatal("SpellDisabled forwarded the wrong spell id")
	}
	game.Hooks.SpellRuneBypass = func(_ *game.Player, _ game.Spellbook, _, runeID int) bool {
		return runeID == 556
	}
	if !deps.SpellHooks.SpellRuneBypass(p, 1152, 556) {
		t.Fatal("SpellRuneBypass did not forward to Hooks")
	}
	consumed := false
	game.Hooks.ConsumeSpellRuneSource = func(_ *game.Player, _ []game.Item) { consumed = true }
	deps.SpellHooks.ConsumeSpellRuneSource(p, nil)
	if !consumed {
		t.Fatal("ConsumeSpellRuneSource did not forward to Hooks")
	}

	// A world wires the task seam; a task scheduled through it
	// is visible to the world's TaskManager.
	w := game.NewWorld(game.Location{X: 3200, Y: 3200, Z: 0})
	var deps2 listeners.Deps
	InstallSpellListeners(&deps2, nil, nil, w)
	if deps2.SpellHooks.ScheduleTask == nil {
		t.Fatal("ScheduleTask nil with a world")
	}
	ran := false
	deps2.SpellHooks.ScheduleTask(game.NewTask(0, func(cycle int64) bool { ran = true; return true }))
	w.Tasks().Process(0)
	if !ran {
		t.Fatal("scheduled task did not run on the world tick")
	}

	// A hub wires the spell-on-item emit through to the hub
	// family: a hook claiming the cast reads back handled,
	// and a non-matching cast falls through unclaimed.
	hub := plugins.NewHub([]plugins.Plugin{{
		Name: "SuperheatProbe",
		Register: func(api *plugins.Api) {
			api.OnSpellOnItem(func(ev *plugins.SpellOnItemEvent) {
				if ev.SpellName == "Superheat Item" && ev.ItemID == 440 {
					ev.Handled = true
				}
			})
		},
	}}, plugins.HubOptions{Logger: discardLogger()})
	hub.Load()
	var deps3 listeners.Deps
	InstallSpellListeners(&deps3, hub, nil, nil)
	if deps3.SpellHooks.EmitSpellOnItem == nil {
		t.Fatal("EmitSpellOnItem nil with a hub")
	}
	p2 := game.NewPlayer("caster2", game.Location{X: 3200, Y: 3200, Z: 0})
	p2.Inventory().Adds(440, 1)
	p2.Inventory().Adds(453, 1)
	if !deps3.SpellHooks.EmitSpellOnItem(&listeners.SpellOnItemEvent{Player: p2, SpellName: "Superheat Item", ItemID: 440, Slot: 0}) {
		t.Fatal("EmitSpellOnItem = false, want true (hook claimed the superheat cast)")
	}
	// A cast the hook does not match falls through unclaimed
	// (coal in slot 1, not the iron ore the hook wants).
	if deps3.SpellHooks.EmitSpellOnItem(&listeners.SpellOnItemEvent{Player: p2, SpellName: "Superheat Item", ItemID: 453, Slot: 1}) {
		t.Fatal("EmitSpellOnItem = true, want false (hook matches iron ore only)")
	}
	// A stale slot (the slot no longer holds the named item)
	// is claimed by the emit's slot guard, as TS returns true
	// there — the click never reaches castOnItem.
	if !deps3.SpellHooks.EmitSpellOnItem(&listeners.SpellOnItemEvent{Player: p2, SpellName: "Superheat Item", ItemID: 453, Slot: 0}) {
		t.Fatal("EmitSpellOnItem = false, want true (slot 0 holds iron ore, not coal)")
	}
}

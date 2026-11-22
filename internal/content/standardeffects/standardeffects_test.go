package standardeffects

import (
	"bytes"
	"log"
	"sync"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// The standard spells only exist after registration, and test
// binaries never boot (the smithing superheat_test.go
// precedent).
var installSpellsOnce sync.Once

func installTestSpells() {
	installSpellsOnce.Do(game.InstallStandardSpells)
}

// fakeSession implements game.SessionPort plus the
// ChatMessageSender extension (the cooking helpers_test.go
// precedent), capturing what content sends.
type fakeSession struct {
	messages []string
}

func (s *fakeSession) SendPacket(packet []byte) {}
func (s *fakeSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
}
func (s *fakeSession) SendDestination(x, y int) {}
func (s *fakeSession) SendVarp(id, value int)   {}
func (s *fakeSession) FlushTick(cycle int64) error {
	return nil
}
func (s *fakeSession) Close(code int, reason string) {}
func (s *fakeSession) IsOpen() bool                  { return true }
func (s *fakeSession) SendGameMessage(message string) {
	s.messages = append(s.messages, message)
}
func (s *fakeSession) SendTypedGameMessage(chatType int, message, from string) {
	s.messages = append(s.messages, message)
}

// newTestPlayer is a registered player with a recording
// session, bound to its combat identity as the pipeline does.
func newTestPlayer(name string) *game.Player {
	p := game.NewPlayer(name, game.Location{X: 3200, Y: 3200, Z: 0})
	p.SetRegistered(true)
	p.Session = &fakeSession{}
	p.GetCombat()
	return p
}

// messagesOf reads the recorded plain messages.
func messagesOf(p *game.Player) []string {
	if s, ok := p.Session.(*fakeSession); ok {
		return s.messages
	}
	return nil
}

// setTestLevel sets current and max level together.
func setTestLevel(p *game.Player, skill, level int) {
	p.Skills().SetCurrentLevel(skill, level, true)
	p.Skills().SetMaxLevel(skill, level)
}

// testHub builds a hub with all five plugins loaded and a
// TaskManager installed (the bank testHub precedent).
func testHub(t *testing.T) (*plugins.Hub, *game.TaskManager) {
	t.Helper()
	var buf bytes.Buffer
	hub := plugins.NewHub(
		[]plugins.Plugin{BindingsPlugin, CursesPlugin, FreezesPlugin, TeleBlockPlugin, StavesPlugin},
		plugins.HubOptions{Logger: log.New(&buf, "", 0)},
	)
	hub.Load()
	if got := hub.Loaded(); len(got) != 5 {
		t.Fatalf("loaded = %v, want all five plugins (log: %s)", got, buf.String())
	}
	tm := game.NewTaskManager()
	hub.SetTaskManager(tm)
	return hub, tm
}

// wirePlayer installs the hub's custom-event sink on the
// player, as the coordinator does at login.
func wirePlayer(p *game.Player, hub *plugins.Hub) {
	p.EmitCustomEvent = hub.EmitCustomEvent
}

// spellOrFail resolves a standard spell by id.
func spellOrFail(t *testing.T, id int) game.CombatSpell {
	t.Helper()
	installTestSpells()
	spell := game.GetCombatSpell(id)
	if spell == nil {
		t.Fatalf("GetCombatSpell(%d) = nil", id)
	}
	return spell
}

// equipWeapon wields the item id.
func equipWeapon(p *game.Player, id int) {
	p.Equipment().Set(game.WeaponSlot, game.NewItem(id, 1))
}

// fakeZoneSource is a ZoneBoundarySource backed by a tag map
// (the game wilderness_test.go mapZoneSource precedent).
type fakeZoneSource map[string][]game.Boundary

func (s fakeZoneSource) ZoneBoundaries(tag string) []game.Boundary { return s[tag] }

// pvpWilderness is a Wilderness whose PvP ground is the
// 3000-3300 x 3500-3700 rectangle.
func pvpWilderness() *game.Wilderness {
	return game.NewWilderness(fakeZoneSource{
		"pvp": {game.NewBoundary(3000, 3300, 3500, 3700, 0)},
	}, nil)
}

// ---- Binds: disabled gate ----

func TestBindDisabledAlreadyHeld(t *testing.T) {
	hub, _ := testHub(t)
	caster := newTestPlayer("caster")
	target := newTestPlayer("target")
	caster.GetCombat().SetTarget(&target.Mobile)
	target.GetTimers().Registers(game.TimerKeyFreeze, 5)

	if hub.EmitSpellDisabled(caster, game.SpellbookNormal, spellBindID, nil) != true {
		t.Fatal("EmitSpellDisabled(bind, frozen target) = false, want true")
	}
	if got := messagesOf(caster); len(got) != 1 || got[0] != "Your target is already held by a magical force." {
		t.Fatalf("caster messages = %v, want the already-held line", got)
	}
}

func TestBindDisabledImmunity(t *testing.T) {
	hub, _ := testHub(t)
	caster := newTestPlayer("caster")
	target := newTestPlayer("target")
	caster.GetCombat().SetTarget(&target.Mobile)
	target.GetTimers().Registers(game.TimerKeyFreezeImmunity, 5)

	if hub.EmitSpellDisabled(caster, game.SpellbookNormal, spellSnareID, nil) != true {
		t.Fatal("EmitSpellDisabled(snare, immune target) = false, want true")
	}
	if got := messagesOf(caster); len(got) != 1 || got[0] != "The target is currently immune to that spell." {
		t.Fatalf("caster messages = %v, want the immune line", got)
	}
}

func TestBindDisabledPasses(t *testing.T) {
	hub, _ := testHub(t)
	caster := newTestPlayer("caster")
	target := newTestPlayer("target")
	caster.GetCombat().SetTarget(&target.Mobile)

	if hub.EmitSpellDisabled(caster, game.SpellbookNormal, spellEntangleID, nil) != false {
		t.Fatal("EmitSpellDisabled(entangle, free target) = true, want false")
	}
	// A non-bind spell passes the gate untouched.
	if hub.EmitSpellDisabled(caster, game.SpellbookNormal, spellConfuseID, nil) != false {
		t.Fatal("EmitSpellDisabled(confuse) = true, want false (not a bind)")
	}
	if len(messagesOf(caster)) != 0 {
		t.Fatalf("caster messages = %v, want none", messagesOf(caster))
	}
}

// ---- Binds: committed hold + Swampbark ----

func TestBindCommittedSwampbark(t *testing.T) {
	for _, tc := range []struct {
		name   string
		pieces []int // slots to fill with Swampbark
		want   int
	}{
		{"no pieces", nil, 8},
		{"one piece", []int{game.HeadSlot}, 10},
		{"two pieces", []int{game.HeadSlot, game.BodySlot}, 12},
		{"full set", []int{game.HeadSlot, game.BodySlot, game.LegSlot}, 14},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster := newTestPlayer("caster")
			ids := map[int]int{
				game.HeadSlot: swampbarkHelmID,
				game.BodySlot: swampbarkBodyID,
				game.LegSlot:  swampbarkLegsID,
			}
			for _, slot := range tc.pieces {
				caster.Equipment().Set(slot, game.NewItem(ids[slot], 1))
			}
			ctx := &game.SpellCastContext{Spell: spellOrFail(t, spellBindID), Data: map[string]any{}}
			shared.bindCommitted(&game.MagicCastCommittedPayload{
				Player: caster, Spell: spellOrFail(t, spellBindID), Context: ctx,
			})
			if got, ok := ctx.Data[holdTicksKey].(int); !ok || got != tc.want {
				t.Fatalf("hold = %v, want %d", ctx.Data[holdTicksKey], tc.want)
			}
		})
	}
}

func TestBindCommittedNonBind(t *testing.T) {
	caster := newTestPlayer("caster")
	ctx := &game.SpellCastContext{Spell: spellOrFail(t, spellConfuseID), Data: map[string]any{}}
	shared.bindCommitted(&game.MagicCastCommittedPayload{
		Player: caster, Spell: spellOrFail(t, spellConfuseID), Context: ctx,
	})
	if len(ctx.Data) != 0 {
		t.Fatalf("context data = %v, want empty (not a bind)", ctx.Data)
	}
}

// ---- Binds: impact ----

func bindImpactEvent(t *testing.T, caster, target *game.Player, spellID int, ctx *game.SpellCastContext) *game.MagicUtilityEffectPayload {
	t.Helper()
	return &game.MagicUtilityEffectPayload{
		Attacker: &caster.Mobile,
		Target:   &target.Mobile,
		Spell:    spellOrFail(t, spellID),
		Context:  ctx,
		Allowed:  true,
	}
}

func TestBindImpactBaseHold(t *testing.T) {
	caster := newTestPlayer("caster")
	target := newTestPlayer("target")
	event := bindImpactEvent(t, caster, target, spellSnareID, nil)
	shared.bindImpact(event)
	if !event.Handled {
		t.Fatal("Handled = false, want true")
	}
	if got := target.GetTimers().Left(game.TimerKeyFreeze); got != 16 {
		t.Fatalf("freeze ticks left = %d, want 16 (Snare base hold)", got)
	}
	if got := target.GetTimers().Left(game.TimerKeyFreezeImmunity); got != 21 {
		t.Fatalf("freeze immunity ticks left = %d, want 21 (16 + 5 grace)", got)
	}
	// Binds say nothing when they land.
	if len(messagesOf(target)) != 0 {
		t.Fatalf("target messages = %v, want none", messagesOf(target))
	}
}

func TestBindImpactSwampbarkHold(t *testing.T) {
	caster := newTestPlayer("caster")
	target := newTestPlayer("target")
	spell := spellOrFail(t, spellSnareID)
	ctx := &game.SpellCastContext{Spell: spell, Data: map[string]any{holdTicksKey: 20}}
	event := bindImpactEvent(t, caster, target, spellSnareID, ctx)
	shared.bindImpact(event)
	if got := target.GetTimers().Left(game.TimerKeyFreeze); got != 20 {
		t.Fatalf("freeze ticks left = %d, want 20 (committed Swampbark hold)", got)
	}
}

func TestBindImpactBadSavedHoldFallsBack(t *testing.T) {
	caster := newTestPlayer("caster")
	target := newTestPlayer("target")
	spell := spellOrFail(t, spellSnareID)
	// 13 is out of range (base 16 .. base+6, even steps).
	ctx := &game.SpellCastContext{Spell: spell, Data: map[string]any{holdTicksKey: 13}}
	event := bindImpactEvent(t, caster, target, spellSnareID, ctx)
	shared.bindImpact(event)
	if got := target.GetTimers().Left(game.TimerKeyFreeze); got != 16 {
		t.Fatalf("freeze ticks left = %d, want 16 (base hold fallback)", got)
	}
}

func TestBindImpactVetoedOrHandled(t *testing.T) {
	for _, tc := range []struct {
		name    string
		allowed bool
		handled bool
	}{
		{"vetoed", false, false},
		{"already handled", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster := newTestPlayer("caster")
			target := newTestPlayer("target")
			event := bindImpactEvent(t, caster, target, spellBindID, nil)
			event.Allowed = tc.allowed
			event.Handled = tc.handled
			shared.bindImpact(event)
			if target.GetTimers().Has(game.TimerKeyFreeze) {
				t.Fatal("target frozen, want no freeze")
			}
			if event.Handled != tc.handled {
				t.Fatalf("Handled = %v, want %v (untouched)", event.Handled, tc.handled)
			}
		})
	}
}

func TestBindImpactAlreadyFrozenNoops(t *testing.T) {
	caster := newTestPlayer("caster")
	target := newTestPlayer("target")
	target.GetTimers().Registers(game.TimerKeyFreeze, 5)
	event := bindImpactEvent(t, caster, target, spellBindID, nil)
	shared.bindImpact(event)
	if !event.Handled {
		t.Fatal("Handled = false, want true (TS marks it before the freeze call)")
	}
	if got := target.GetTimers().Left(game.TimerKeyFreeze); got != 5 {
		t.Fatalf("freeze ticks left = %d, want 5 (unchanged)", got)
	}
}

// ---- Binds + Freezes: end to end through the hub ----

func TestBindFreezeEndToEnd(t *testing.T) {
	hub, tm := testHub(t)
	caster := newTestPlayer("caster")
	target := newTestPlayer("target")
	target.Mobile.SetLocation(game.Location{X: 3200, Y: 3200, Z: 0})
	wirePlayer(caster, hub)
	wirePlayer(target, hub)

	hub.EmitCustomEvent("magic:utility-effect", &game.MagicUtilityEffectPayload{
		Attacker: &caster.Mobile,
		Target:   &target.Mobile,
		Spell:    spellOrFail(t, spellBindID),
		Allowed:  true,
	})
	if got := target.GetTimers().Left(game.TimerKeyFreeze); got != 8 {
		t.Fatalf("freeze ticks left = %d, want 8 (Bind base hold)", got)
	}
	// The bind's "combat:frozen" starts the FreezeWatch: drive
	// ticks until the fade warning lands at three ticks left.
	warned := false
	for cycle := int64(1); cycle <= 10 && !warned; cycle++ {
		tm.Process(cycle)
		target.GetTimers().Process()
		for _, m := range messagesOf(target) {
			if m == "<col=ef1020>The holding effect is beginning to fade.</col>" {
				warned = true
			}
		}
	}
	if !warned {
		t.Fatalf("target messages = %v, want the fade warning", messagesOf(target))
	}
}

// ---- Curses ----

func curseImpactEvent(t *testing.T, caster, target *game.Player, spellID int) *game.MagicUtilityEffectPayload {
	t.Helper()
	return &game.MagicUtilityEffectPayload{
		Attacker: &caster.Mobile,
		Target:   &target.Mobile,
		Spell:    spellOrFail(t, spellID),
		Allowed:  true,
	}
}

func TestCurseDrainsPlayer(t *testing.T) {
	for _, tc := range []struct {
		spell   int
		skill   int
		percent int
		want    int // 100 - floor(100*percent/100)
	}{
		{spellConfuseID, game.SkillAttack, 5, 95},
		{spellWeakenID, game.SkillStrength, 5, 95},
		{spellCurseID, game.SkillDefence, 5, 95},
		{spellVulnerabilityID, game.SkillDefence, 10, 90},
		{spellEnfeebleID, game.SkillStrength, 10, 90},
		{spellStunID, game.SkillAttack, 10, 90},
	} {
		t.Run("", func(t *testing.T) {
			caster := newTestPlayer("caster")
			target := newTestPlayer("target")
			setTestLevel(target, tc.skill, 100)
			event := curseImpactEvent(t, caster, target, tc.spell)
			shared.curseImpact(event)
			if !event.Handled {
				t.Fatal("Handled = false, want true")
			}
			if got := target.Skills().CurrentLevel(tc.skill); got != tc.want {
				t.Fatalf("level = %d, want %d (%d%% drain)", got, tc.want, tc.percent)
			}
			if got := messagesOf(target); len(got) != 1 || got[0] != "You feel slightly weakened." {
				t.Fatalf("target messages = %v, want the weakened line", got)
			}
		})
	}
}

func TestCurseAlreadyWeakened(t *testing.T) {
	caster := newTestPlayer("caster")
	target := newTestPlayer("target")
	setTestLevel(target, game.SkillAttack, 99)
	target.Skills().SetCurrentLevel(game.SkillAttack, 90, true)
	event := curseImpactEvent(t, caster, target, spellConfuseID)
	shared.curseImpact(event)
	if !event.Handled {
		t.Fatal("Handled = false, want true")
	}
	if got := target.Skills().CurrentLevel(game.SkillAttack); got != 90 {
		t.Fatalf("level = %d, want 90 (no further drain)", got)
	}
	if got := messagesOf(caster); len(got) != 1 || got[0] != "Your foe's attack has already been weakened." {
		t.Fatalf("caster messages = %v, want the already-weakened line", got)
	}
}

func TestCurseDisabledAlreadyWeakened(t *testing.T) {
	hub, _ := testHub(t)
	caster := newTestPlayer("caster")
	target := newTestPlayer("target")
	caster.GetCombat().SetTarget(&target.Mobile)
	setTestLevel(target, game.SkillDefence, 99)
	target.Skills().SetCurrentLevel(game.SkillDefence, 80, true)

	if hub.EmitSpellDisabled(caster, game.SpellbookNormal, spellCurseID, nil) != true {
		t.Fatal("EmitSpellDisabled(curse, weakened target) = false, want true")
	}
	if got := messagesOf(caster); len(got) != 1 || got[0] != "The spell has no effect because the player is already weakened." {
		t.Fatalf("caster messages = %v, want the player weakened line", got)
	}
}

func TestCurseDisabledFullLevelPasses(t *testing.T) {
	hub, _ := testHub(t)
	caster := newTestPlayer("caster")
	target := newTestPlayer("target")
	caster.GetCombat().SetTarget(&target.Mobile)
	setTestLevel(target, game.SkillAttack, 99)

	if hub.EmitSpellDisabled(caster, game.SpellbookNormal, spellStunID, nil) != false {
		t.Fatal("EmitSpellDisabled(stun, full-level target) = true, want false")
	}
}

func TestCurseOnNpcMarksHandledWithoutDrain(t *testing.T) {
	// NPCs have no mutable skill state in Go (the recorded
	// residual): the cast is marked handled, exactly as TS
	// marks it, but no drain is applied.
	caster := newTestPlayer("caster")
	npc := game.NewNPC(1, game.Location{X: 3201, Y: 3200, Z: 0})
	event := &game.MagicUtilityEffectPayload{
		Attacker: &caster.Mobile,
		Target:   &npc.Mobile,
		Spell:    spellOrFail(t, spellEnfeebleID),
		Allowed:  true,
	}
	shared.curseImpact(event)
	if !event.Handled {
		t.Fatal("Handled = false, want true")
	}
}

// ---- Freezes ----

func TestFreezeAppliedIceMessage(t *testing.T) {
	_, tm := testHub(t)
	caster := newTestPlayer("caster")
	target := newTestPlayer("target")
	shared.freezeApplied(&game.CombatFrozenPayload{
		Target: &target.Mobile, Source: &caster.Mobile, Ticks: 32, Kind: freezeKindIce,
	})
	if got := messagesOf(target); len(got) != 1 || got[0] != "<col=ef1020>You have been frozen!</col>" {
		t.Fatalf("target messages = %v, want the red frozen line", got)
	}
	_ = tm // the watch is scheduled; the warning leg is covered below
}

func TestFreezeAppliedBindNoMessage(t *testing.T) {
	caster := newTestPlayer("caster")
	target := newTestPlayer("target")
	shared.freezeApplied(&game.CombatFrozenPayload{
		Target: &target.Mobile, Source: &caster.Mobile, Ticks: 8, Kind: freezeKindBind,
	})
	if len(messagesOf(target)) != 0 {
		t.Fatalf("target messages = %v, want none (binds say nothing)", messagesOf(target))
	}
}

func TestFreezeAppliedNonPlayer(t *testing.T) {
	caster := newTestPlayer("caster")
	npc := game.NewNPC(1, game.Location{X: 3201, Y: 3200, Z: 0})
	shared.freezeApplied(&game.CombatFrozenPayload{
		Target: &npc.Mobile, Source: &caster.Mobile, Ticks: 8, Kind: freezeKindIce,
	})
	// No panic, no watch, nothing to observe.
}

func TestFreezeWatchWarnsOnceAtThreeTicks(t *testing.T) {
	_, tm := testHub(t)
	target := newTestPlayer("target")
	target.GetTimers().Registers(game.TimerKeyFreeze, 8)
	shared.watchFreeze(target, nil, "<col=ef1020>The freeze cast upon you is beginning to thaw.</col>")
	warnings := 0
	for cycle := int64(1); cycle <= 10; cycle++ {
		tm.Process(cycle)
		target.GetTimers().Process()
		// Recount from scratch each cycle: the message list is
		// cumulative, so the final count is the total sent.
		warnings = 0
		for _, m := range messagesOf(target) {
			if m == "<col=ef1020>The freeze cast upon you is beginning to thaw.</col>" {
				warnings++
			}
		}
	}
	if warnings != 1 {
		t.Fatalf("thaw warnings = %d, want exactly 1", warnings)
	}
	if target.GetTimers().Left(game.TimerKeyFreeze) != 0 {
		t.Fatalf("freeze ticks left = %d, want 0 (expired)", target.GetTimers().Left(game.TimerKeyFreeze))
	}
}

func TestFreezeWatchReleasesOutOfRange(t *testing.T) {
	_, tm := testHub(t)
	caster := newTestPlayer("caster")
	target := newTestPlayer("target")
	target.GetTimers().Registers(game.TimerKeyFreeze, 32)
	shared.watchFreeze(target, caster, "")
	tm.Process(1)
	target.GetTimers().Process()
	if !target.GetTimers().Has(game.TimerKeyFreeze) {
		t.Fatal("freeze released while holder is in range")
	}
	// The holder walks away: over 10 tiles on X.
	caster.Mobile.SetLocation(game.Location{X: 3300, Y: 3200, Z: 0})
	tm.Process(2)
	target.GetTimers().Process()
	if target.GetTimers().Has(game.TimerKeyFreeze) {
		t.Fatal("freeze still held after the holder strayed over 10 tiles")
	}
}

func TestFreezeWatchStopsOnRefreeze(t *testing.T) {
	_, tm := testHub(t)
	target := newTestPlayer("target")
	target.GetTimers().Registers(game.TimerKeyFreeze, 8)
	shared.watchFreeze(target, nil, "<col=ef1020>The freeze cast upon you is beginning to thaw.</col>")
	tm.Process(1)
	target.GetTimers().Process()
	// A fresh freeze replaces the watched one: the old watch
	// stops (the TS timer-identity check, approximated by the
	// grown time-left check) and the warning never fires from it.
	target.GetTimers().Registers(game.TimerKeyFreeze, 24)
	for cycle := int64(2); cycle <= 6; cycle++ {
		tm.Process(cycle)
		target.GetTimers().Process()
	}
	for _, m := range messagesOf(target) {
		if m == "<col=ef1020>The freeze cast upon you is beginning to thaw.</col>" {
			t.Fatal("stale watch warned after a re-freeze")
		}
	}
}

// ---- TeleBlock tick driver ----

func installTestCycle(t *testing.T) *int64 {
	t.Helper()
	cycle := new(int64)
	game.SetCombatCycleSource(func() int64 { return *cycle })
	t.Cleanup(func() { game.SetCombatCycleSource(nil) })
	return cycle
}

func TestTeleblockTickKeepsBlockInPvp(t *testing.T) {
	hub, _ := testHub(t)
	cycle := installTestCycle(t)
	player := newTestPlayer("blocked")
	player.Mobile.SetLocation(game.Location{X: 3200, Y: 3600, Z: 0})
	shared.wilderness = pvpWilderness()
	t.Cleanup(func() { shared.wilderness = nil })
	game.TeleBlockApply(player, "caster")
	*cycle = 100
	shared.teleblockTick(&plugins.PlayerProcessEvent{Player: player})
	if !game.TeleBlockIsBlocked(player) {
		t.Fatal("block removed while in PvP ground")
	}
	if len(messagesOf(player)) != 1 {
		// Only the apply message.
		t.Fatalf("player messages = %v, want only the apply line", messagesOf(player))
	}
	_ = hub
}

func TestTeleblockTickRemovesOutsidePvp(t *testing.T) {
	cycle := installTestCycle(t)
	player := newTestPlayer("blocked")
	player.Mobile.SetLocation(game.Location{X: 3200, Y: 3200, Z: 0})
	shared.wilderness = pvpWilderness()
	t.Cleanup(func() { shared.wilderness = nil })
	game.TeleBlockApply(player, "caster")
	*cycle = 100
	shared.teleblockTick(&plugins.PlayerProcessEvent{Player: player})
	if game.TeleBlockIsBlocked(player) {
		t.Fatal("block kept outside PvP ground")
	}
}

func TestTeleblockTickWarnsOnceAtThirtySeconds(t *testing.T) {
	cycle := installTestCycle(t)
	player := newTestPlayer("blocked")
	player.Mobile.SetLocation(game.Location{X: 3200, Y: 3600, Z: 0})
	shared.wilderness = pvpWilderness()
	t.Cleanup(func() { shared.wilderness = nil })
	game.TeleBlockApply(player, "caster")
	// 300 s is 500 ticks; at cycle 450, 50 ticks (30 s) remain.
	*cycle = 450
	shared.teleblockTick(&plugins.PlayerProcessEvent{Player: player})
	shared.teleblockTick(&plugins.PlayerProcessEvent{Player: player})
	warnings := 0
	for _, m := range messagesOf(player) {
		if m == "<col=c356ef>Your Tele Block will expire in 30 seconds." {
			warnings++
		}
	}
	if warnings != 1 {
		t.Fatalf("warnings = %d, want exactly 1 (messages: %v)", warnings, messagesOf(player))
	}
	if !game.TeleBlockIsBlocked(player) {
		t.Fatal("block removed at the warning tick")
	}
}

func TestTeleblockTickRemovesAtExpiry(t *testing.T) {
	cycle := installTestCycle(t)
	player := newTestPlayer("blocked")
	player.Mobile.SetLocation(game.Location{X: 3200, Y: 3600, Z: 0})
	shared.wilderness = pvpWilderness()
	t.Cleanup(func() { shared.wilderness = nil })
	game.TeleBlockApply(player, "caster")
	*cycle = 500
	shared.teleblockTick(&plugins.PlayerProcessEvent{Player: player})
	if game.TeleBlockIsBlocked(player) {
		t.Fatal("block kept at expiry")
	}
}

func TestTeleblockTickNoBlockNoop(t *testing.T) {
	player := newTestPlayer("free")
	shared.teleblockTick(&plugins.PlayerProcessEvent{Player: player})
	if len(messagesOf(player)) != 0 {
		t.Fatalf("player messages = %v, want none", messagesOf(player))
	}
}

// ---- ElementalStaves ----

func TestStavesBypass(t *testing.T) {
	hub, _ := testHub(t)
	for _, tc := range []struct {
		weapon int
		rune   int
		want   bool
	}{
		{1381, runeAirID, true},    // staff of air
		{1381, runeFireID, false},  // staff of air, wrong rune
		{1395, runeWaterID, true},  // water battlestaff
		{1399, runeEarthID, true},  // earth battlestaff
		{1401, runeFireID, true},   // mystic fire staff
		{3053, runeFireID, true},   // lava battlestaff: fire
		{3053, runeEarthID, true},  // lava battlestaff: earth
		{3053, runeAirID, false},   // lava battlestaff: not air
		{6562, runeWaterID, true},  // mud battlestaff: water
		{11787, runeWaterID, true}, // steam battlestaff: water
		{11998, runeAirID, true},   // smoke battlestaff: air
		{20730, runeAirID, true},   // mist battlestaff: air
		{20736, runeEarthID, true}, // dust battlestaff: earth
		{21006, runeWaterID, true}, // Kodai wand
		{30634, runeFireID, true},  // twinflame staff: fire
		{30634, runeWaterID, true}, // twinflame staff: water
		{33253, runeAirID, true},   // Shadowflame quadrant: air
		{33253, runeFireID, true},  // Shadowflame quadrant: fire
		{33253, runeEarthID, true}, // Shadowflame quadrant: earth
		{33253, runeWaterID, true}, // Shadowflame quadrant: water
		{0, runeAirID, false},      // no weapon
		{1381, 0, false},           // whole-cost query never bypasses
	} {
		player := newTestPlayer("mage")
		if tc.weapon != 0 {
			equipWeapon(player, tc.weapon)
		}
		if got := hub.EmitSpellRuneBypass(player, game.SpellbookNormal, spellBindID, tc.rune); got != tc.want {
			t.Errorf("weapon %d rune %d: bypass = %v, want %v", tc.weapon, tc.rune, got, tc.want)
		}
	}
}

func TestStavesRunesQuery(t *testing.T) {
	player := newTestPlayer("mage")
	equipWeapon(player, 11787) // steam battlestaff
	req := &ElementalStavesRunesRequest{Player: player}
	shared.reportSuppliedRunes(req)
	if len(req.Runes) != 2 || req.Runes[0] != runeFireID || req.Runes[1] != runeWaterID {
		t.Fatalf("runes = %v, want [554 555]", req.Runes)
	}
}

// ---- Swampbark set text (no producer in Go) ----

func TestSwampbarkSetText(t *testing.T) {
	player := newTestPlayer("mage")
	player.Equipment().Set(game.HeadSlot, game.NewItem(swampbarkHelmID, 1))
	player.Equipment().Set(game.BodySlot, game.NewItem(swampbarkBodyID, 1))
	player.Equipment().Set(game.LegSlot, game.NewItem(swampbarkLegsID, 1))
	req := &game.EquipmentEffectsRequest{Player: player}
	shared.addSwampbarkSetText(req)
	if len(req.SetTexts) != 1 || req.SetTexts[0] != swampbarkSetText {
		t.Fatalf("set texts = %v, want [25]", req.SetTexts)
	}
	// One piece missing: no text.
	player.Equipment().Set(game.LegSlot, game.NewItem(0, 0))
	req = &game.EquipmentEffectsRequest{Player: player}
	shared.addSwampbarkSetText(req)
	if len(req.SetTexts) != 0 {
		t.Fatalf("set texts = %v, want none", req.SetTexts)
	}
}

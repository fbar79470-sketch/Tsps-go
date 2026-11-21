package listeners

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Spell tests (phase 27): spell-id resolution through the game
// registry, spell-on-NPC (combat + scripted routes), spell-on-
// player, alchemy, and Telekinetic Grab. Fixtures only: fake
// spells registered in the game registry under test ids (the
// production InstallStandardSpells table is Worker A's and is
// not assumed), fake lookups and definition sources, and the
// real ground-item manager behind managerStore. Decode-level
// tests encode real payloads with the helpers below, each
// inverting the documented decoder reads.

// fakeCombatSpell is a hand-built CombatSpell for the listener
// tests. It records casts and answers from its fields.
type fakeCombatSpell struct {
	spellID       int
	maxHit        int
	baseXP        int
	canCast       bool
	canCastTarget bool
	startCasts    int
}

func (s *fakeCombatSpell) SpellID() int { return s.spellID }

func (s *fakeCombatSpell) MaximumHit() int       { return s.maxHit }
func (s *fakeCombatSpell) BaseExperience() int   { return s.baseXP }
func (s *fakeCombatSpell) SplashExperience() int { return 0 }
func (s *fakeCombatSpell) AttackSpeed() int      { return 5 }
func (s *fakeCombatSpell) HitDelay() int         { return -1 }
func (s *fakeCombatSpell) CanCast(caster *game.Player, consume bool) bool {
	return s.canCast
}
func (s *fakeCombatSpell) CanCastOnTarget(caster *game.Player, target *game.Mobile) bool {
	return s.canCastTarget
}
func (s *fakeCombatSpell) StartCast(caster *game.Mobile, target *game.Mobile) { s.startCasts++ }
func (s *fakeCombatSpell) FinishCast(attacker, target *game.Mobile, accurate bool, damage int) {
}
func (s *fakeCombatSpell) SpellRadius() int { return 0 }

// fakeUtilitySpell is a hand-built SpellCaster (an EffectSpells
// stand-in) for the scripted route and resolve-spell tests.
type fakeUtilitySpell struct {
	baseXP        int
	canCast       bool
	canCastTarget bool
	startCasts    int
}

func (s *fakeUtilitySpell) CanCast(caster *game.Player, consume bool) bool { return s.canCast }
func (s *fakeUtilitySpell) CanCastOnTarget(caster *game.Player, target *game.Mobile) bool {
	return s.canCastTarget
}
func (s *fakeUtilitySpell) StartCast(caster *game.Mobile, target *game.Mobile) { s.startCasts++ }
func (s *fakeUtilitySpell) BaseExperience() int                                { return s.baseXP }

// registerTestCombatSpell registers a fake combat spell under a
// test id and name, returning the fake and a cleanup that
// unregisters it.
func registerTestCombatSpell(t *testing.T, id int, name string) *fakeCombatSpell {
	t.Helper()
	spell := &fakeCombatSpell{spellID: id, maxHit: 8, baseXP: 20, canCast: true, canCastTarget: true}
	game.RegisterCombatSpellNamed(id, name, spell)
	t.Cleanup(func() { game.UnregisterCombatSpell(id) })
	return spell
}

func registerTestUtilitySpell(t *testing.T, id int) *fakeUtilitySpell {
	t.Helper()
	spell := &fakeUtilitySpell{baseXP: 20, canCast: true, canCastTarget: true}
	game.RegisterUtilitySpell(id, spell)
	t.Cleanup(func() { game.UnregisterUtilitySpell(id) })
	return spell
}

// spellTestPlayer builds a player with the given Magic level and
// a capturing session for message assertions.
func spellTestPlayer(magicLevel int) (*game.Player, *capturingSession) {
	p := testPlayer()
	p.Skills().SetMaxLevel(game.SkillMagic, magicLevel)
	p.Skills().SetCurrentLevel(game.SkillMagic, magicLevel, false)
	// Rune names + stackable coins for the missing-rune
	// messages and the alchemy coin credit.
	p.SetItemDefinitions(alchTestDefs())
	session := &capturingSession{}
	p.Session = session
	return p, session
}

// fakeSchedule records the tasks a listener schedules, standing
// in for World.Tasks().Schedule. run executes a recorded task's
// action immediately, as the task manager would when its
// countdown expires.
type fakeSchedule struct {
	tasks []*game.Task
}

func (f *fakeSchedule) schedule(task *game.Task) { f.tasks = append(f.tasks, task) }

func (f *fakeSchedule) run(t *testing.T, index int, cycle int64) {
	t.Helper()
	if index >= len(f.tasks) {
		t.Fatalf("run task %d: only %d scheduled", index, len(f.tasks))
	}
	if f.tasks[index].Action == nil {
		t.Fatalf("run task %d: no action", index)
	}
	f.tasks[index].Action(cycle)
}

// fakeAlchDefs is a fixture definition source with alchemy
// values: the standard fixture table plus high/low values per
// id.
type fakeAlchDefs struct {
	fakeDefs
	high  map[int]int
	low   map[int]int
	names map[int]string
}

func (d fakeAlchDefs) ItemHighAlchValue(id int) int { return d.high[id] }
func (d fakeAlchDefs) ItemLowAlchValue(id int) int  { return d.low[id] }
func (d fakeAlchDefs) ItemName(id int) string       { return d.names[id] }

// alchTestDefs builds definitions where itemPlain alchs for
// 100/60, the runes and coins are stackable, and the runes
// carry display names for the missing-rune messages.
func alchTestDefs() fakeAlchDefs {
	defs := fixtureDefs()
	defs.cfgs[runeNatureID] = defConfig{slot: -1, stackable: true, dropable: true, tradeable: true}
	defs.cfgs[runeFireID] = defConfig{slot: -1, stackable: true, dropable: true, tradeable: true}
	defs.cfgs[runeLawID] = defConfig{slot: -1, stackable: true, dropable: true, tradeable: true}
	defs.cfgs[runeAirID] = defConfig{slot: -1, stackable: true, dropable: true, tradeable: true}
	defs.cfgs[itemCoinsID] = defConfig{slot: -1, stackable: true, dropable: true, tradeable: true}
	return fakeAlchDefs{
		fakeDefs: defs,
		high:     map[int]int{itemPlain: 100, itemCoinsID: 100},
		low:      map[int]int{itemPlain: 60, itemCoinsID: 60},
		names:    map[int]string{runeAirID: "Air rune", runeFireID: "Fire rune", runeLawID: "Law rune", runeNatureID: "Nature rune"},
	}
}

// findSlot returns the slot holding id, or -1.
func findSlot(p *game.Player, id int) int {
	for i, item := range p.Inventory().Items() {
		if item.IsValid() && item.ID() == id {
			return i
		}
	}
	return -1
}

// Payload encoders for the decode-level tests. Each inverts the
// documented decoder reads (internal/protocol/reader.go).

func shortLEEnc(v int) []byte { return []byte{byte(v), byte(v >> 8)} }

func intLEEnc(v int32) []byte {
	return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}
}

func intIMEEnc(v int32) []byte {
	return []byte{byte(v >> 16), byte(v >> 24), byte(v), byte(v >> 8)}
}

func shortAddEnc(v int) []byte { return []byte{byte(v) + 128, byte(v >> 8)} }

func shortAddLEEnc(v int) []byte { return []byte{byte(v >> 8), byte(v) + 128} }

// spellOnNpcPayload encodes OPNPC_T (opcode 75): target short,
// widget intLE, child short, item shortAdd, mods byteAdd.
func spellOnNpcPayload(target int, widget int32, child, item, mods int) []byte {
	payload := shortBE(target)
	payload = append(payload, intLEEnc(widget)...)
	payload = append(payload, shortBE(child)...)
	payload = append(payload, shortAddEnc(item)...)
	return append(payload, byte(mods)+128)
}

// spellOnPlayerPayload encodes OPPLAYER_T (opcode 32): mods
// byteNeg, item shortLE, child shortLE, widget intIME, target
// shortLE.
func spellOnPlayerPayload(target int, widget int32, child, item, mods int) []byte {
	payload := []byte{byte(-mods)}
	payload = append(payload, shortLEEnc(item)...)
	payload = append(payload, shortLEEnc(child)...)
	payload = append(payload, intIMEEnc(widget)...)
	return append(payload, shortLEEnc(target)...)
}

// spellOnGroundPayload encodes OPLOC_T_ALT (opcode 94): widget
// intLE, child shortAdd, ground shortAdd, x shortAddLE, y
// short, mods byte, item shortAddLE.
func spellOnGroundPayload(ground, x, y int, widget int32, child, item, mods int) []byte {
	payload := intLEEnc(widget)
	payload = append(payload, shortAddEnc(child)...)
	payload = append(payload, shortAddEnc(ground)...)
	payload = append(payload, shortAddLEEnc(x)...)
	payload = append(payload, shortBE(y)...)
	payload = append(payload, byte(mods))
	return append(payload, shortAddLEEnc(item)...)
}

// widgetTargetPayload encodes IF_BUTTONT (opcode 90): target
// widget intIME, target slot shortAddLE, source widget intLE,
// source slot shortLE, source item short, target item
// shortAddLE.
func widgetTargetPayload(targetWidget int32, targetSlot int, sourceWidget int32, sourceSlot, sourceItem, targetItem int) []byte {
	payload := intIMEEnc(targetWidget)
	payload = append(payload, shortAddLEEnc(targetSlot)...)
	payload = append(payload, intLEEnc(sourceWidget)...)
	payload = append(payload, shortLEEnc(sourceSlot)...)
	payload = append(payload, shortBE(sourceItem)...)
	return append(payload, shortAddLEEnc(targetItem)...)
}

func TestSpellOnNpcCombatRoute(t *testing.T) {
	const spellID = 42401
	registerTestCombatSpell(t, spellID, "Wind Strike")
	p := testPlayer()
	npc := game.NewNPC(200, game.Location{X: 3090, Y: 3524, Z: 0})
	npc.Hitpoints = 10
	var castTarget *game.Mobile
	var castSpell game.CombatSpell
	deps := npcTestDeps(npc, attackableDef(), NpcHooks{})
	deps.SpellHooks.CastSpell = func(attacker *game.Player, target *game.Mobile, spell game.CombatSpell) {
		castTarget = target
		castSpell = spell
	}
	// Decode-level: a real opcode-75 payload resolving by
	// spellChild.
	payload := spellOnNpcPayload(3, (1151<<16)|1152, spellID, -1, 0)
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpNpcT, Payload: payload}, 7, deps)
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("spell on npc = %+v, want implemented", got)
	}
	if castTarget != &npc.Mobile {
		t.Fatalf("cast target = %v, want the NPC mobile", castTarget)
	}
	if castSpell == nil || castSpell.MaximumHit() != 8 {
		t.Fatalf("cast spell = %v, want the registered Wind Strike", castSpell)
	}
}

func TestSpellOnNpcGates(t *testing.T) {
	const spellID = 42402
	registerTestCombatSpell(t, spellID, "Wind Strike")
	msg := protocol.SpellOnNpc{TargetIndex: 3, SpellWidget: 1151 << 16, SpellChild: spellID, SpellItemID: -1}
	drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
	acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
	noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }

	t.Run("unknown spell refuses silently", func(t *testing.T) {
		p := testPlayer()
		npc := game.NewNPC(200, game.Location{X: 3090, Y: 3524, Z: 0})
		npc.Hitpoints = 10
		deps := npcTestDeps(npc, attackableDef(), NpcHooks{})
		unknown := protocol.SpellOnNpc{TargetIndex: 3, SpellWidget: 1151 << 16, SpellChild: 999999, SpellItemID: -1}
		if got := routeSpellOnNpc(p, unknown, deps, drop, acted, noAction); got != noAction() {
			t.Fatalf("unknown spell = %+v, want noAction", got)
		}
	})
	t.Run("missing NPC refuses silently", func(t *testing.T) {
		p := testPlayer()
		deps := npcTestDeps(nil, attackableDef(), NpcHooks{})
		if got := routeSpellOnNpc(p, msg, deps, drop, acted, noAction); got != noAction() {
			t.Fatalf("missing NPC = %+v, want noAction", got)
		}
	})
	t.Run("out of range refuses silently", func(t *testing.T) {
		p := testPlayer()
		npc := game.NewNPC(200, game.Location{X: 3200, Y: 3600, Z: 0})
		npc.Hitpoints = 10
		deps := npcTestDeps(npc, attackableDef(), NpcHooks{})
		if got := routeSpellOnNpc(p, msg, deps, drop, acted, noAction); got != noAction() {
			t.Fatalf("far NPC = %+v, want noAction", got)
		}
	})
	t.Run("unwired cast seam drops", func(t *testing.T) {
		p := testPlayer()
		npc := game.NewNPC(200, game.Location{X: 3090, Y: 3524, Z: 0})
		npc.Hitpoints = 10
		deps := npcTestDeps(npc, attackableDef(), NpcHooks{})
		if got := routeSpellOnNpc(p, msg, deps, drop, acted, noAction); got.Reason != ReasonNpcOption {
			t.Fatalf("unwired seam = %+v, want ReasonNpcOption", got)
		}
	})
	t.Run("area denial refuses", func(t *testing.T) {
		p := testPlayer()
		npc := game.NewNPC(200, game.Location{X: 3090, Y: 3524, Z: 0})
		npc.Hitpoints = 10
		deny := false
		area := game.NewArea("denial", nil)
		area.CanCastOnNpcHook = func(player *game.Player, n *game.NPC, spell any) *bool {
			return &deny
		}
		npc.SetArea(area)
		deps := npcTestDeps(npc, attackableDef(), NpcHooks{})
		deps.SpellHooks.CastSpell = func(attacker *game.Player, target *game.Mobile, spell game.CombatSpell) {
			t.Fatal("denied cast must not reach the combat seam")
		}
		if got := routeSpellOnNpc(p, msg, deps, drop, acted, noAction); got != noAction() {
			t.Fatalf("area denial = %+v, want noAction", got)
		}
	})
}

func TestSpellOnNpcScriptedRoute(t *testing.T) {
	const spellID = 42403
	spell := registerTestCombatSpell(t, spellID, "Wind Strike")
	msg := protocol.SpellOnNpc{TargetIndex: 3, SpellWidget: 1151 << 16, SpellChild: spellID, SpellItemID: -1}
	drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
	acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
	noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }

	t.Run("non-attackable with no hook says You can't attack this npc", func(t *testing.T) {
		p, session := spellTestPlayer(50)
		npc := game.NewNPC(100, game.Location{X: 3090, Y: 3524, Z: 0})
		npc.Hitpoints = 10
		deps := npcTestDeps(npc, talkToDef(), NpcHooks{})
		if got := routeSpellOnNpc(p, msg, deps, drop, acted, noAction); got != noAction() {
			t.Fatalf("scripted unhandled = %+v, want noAction", got)
		}
		if len(session.gameMessages) != 1 || session.gameMessages[0] != "You can't attack this npc." {
			t.Fatalf("messages = %v, want the refusal line", session.gameMessages)
		}
	})
	t.Run("hook paying through consumeRunes casts", func(t *testing.T) {
		p, _ := spellTestPlayer(50)
		npc := game.NewNPC(100, game.Location{X: 3090, Y: 3524, Z: 0})
		npc.Hitpoints = 10
		deps := npcTestDeps(npc, talkToDef(), NpcHooks{})
		var seen *SpellOnNpcEvent
		deps.SpellHooks.EmitSpellOnNpc = func(ev *SpellOnNpcEvent) bool {
			seen = ev
			if !ev.ConsumeRunes() {
				t.Fatal("consumeRunes must pay")
			}
			ev.Handled = true
			return true
		}
		if got := routeSpellOnNpc(p, msg, deps, drop, acted, noAction); got != acted() {
			t.Fatalf("scripted handled = %+v, want acted", got)
		}
		if seen == nil || seen.SpellID != spellID || seen.NpcID != 100 || seen.NpcIndex != 3 {
			t.Fatalf("event = %+v, want the cast identity", seen)
		}
		if spell.startCasts != 1 {
			t.Fatalf("startCasts = %d, want 1", spell.startCasts)
		}
		if got := p.Skills().Experience(game.SkillMagic); got != 20 {
			t.Fatalf("magic XP = %d, want 20", got)
		}
	})
	t.Run("out of approach range walks in", func(t *testing.T) {
		p := testPlayer()
		npc := game.NewNPC(100, game.Location{X: 3105, Y: 3524, Z: 0})
		npc.Hitpoints = 10
		// WalkToEntity refuses unregistered targets
		// (interactionEntityValid).
		npc.SetRegistered(true)
		deps := npcTestDeps(npc, talkToDef(), NpcHooks{})
		if got := routeSpellOnNpc(p, msg, deps, drop, acted, noAction); got != acted() {
			t.Fatalf("scripted walk = %+v, want acted", got)
		}
		if !p.MovementQueue().WalkToPending() {
			t.Fatal("scripted walk must queue a walk-to")
		}
	})
	t.Run("resolve-spell supplies a utility spell", func(t *testing.T) {
		utility := registerTestUtilitySpell(t, 42404)
		p, session := spellTestPlayer(50)
		npc := game.NewNPC(100, game.Location{X: 3090, Y: 3524, Z: 0})
		npc.Hitpoints = 10
		deps := npcTestDeps(npc, talkToDef(), NpcHooks{})
		deps.SpellHooks.SpellName = func(widgetID, itemID int) (string, bool) {
			return "Mage Arena Spell", true
		}
		p.EmitCustomEvent = func(name string, payload any) {
			if ev, ok := payload.(*game.SpellResolveEvent); ok && name == "magic:resolve-spell" {
				ev.Spell = utility
			}
		}
		unknown := protocol.SpellOnNpc{TargetIndex: 3, SpellWidget: 1151 << 16, SpellChild: 999999, SpellItemID: -1}
		if got := routeSpellOnNpc(p, unknown, deps, drop, acted, noAction); got != noAction() {
			t.Fatalf("resolved utility = %+v, want noAction (no hook claims it)", got)
		}
		if len(session.gameMessages) != 1 || session.gameMessages[0] != "You can't attack this npc." {
			t.Fatalf("messages = %v, want the refusal line", session.gameMessages)
		}
	})
}

func TestSpellOnPlayer(t *testing.T) {
	const spellID = 42405
	registerTestCombatSpell(t, spellID, "Wind Strike")
	msg := protocol.SpellOnPlayer{TargetIndex: 7, SpellWidget: 1151 << 16, SpellChild: spellID, SpellItemID: -1}
	drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
	acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
	noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }

	t.Run("cast through the seam", func(t *testing.T) {
		p := testPlayer()
		target := otherPlayer()
		var castTarget *game.Mobile
		deps := playerTestDeps(target, PlayerHooks{}, nil)
		deps.SpellHooks.CastSpell = func(attacker *game.Player, m *game.Mobile, spell game.CombatSpell) {
			castTarget = m
		}
		// Decode-level: a real opcode-32 payload resolving by
		// spellChild.
		payload := spellOnPlayerPayload(7, (1151<<16)|1152, spellID, -1, 0)
		got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpPlayerT, Payload: payload}, 1, deps)
		if got != (Outcome{Recognized: true, Implemented: true}) {
			t.Fatalf("spell on player = %+v, want implemented", got)
		}
		if castTarget != &target.Mobile {
			t.Fatalf("cast target = %v, want the target mobile", castTarget)
		}
	})
	t.Run("self target resets and refuses", func(t *testing.T) {
		p := testPlayer()
		deps := playerTestDeps(p, PlayerHooks{}, nil)
		self := protocol.SpellOnPlayer{TargetIndex: 7, SpellWidget: 1151 << 16, SpellChild: spellID, SpellItemID: -1}
		if got := routeSpellOnPlayer(p, self, deps, drop, acted, noAction); got != noAction() {
			t.Fatalf("self target = %+v, want noAction", got)
		}
	})
	t.Run("unknown spell refuses", func(t *testing.T) {
		p := testPlayer()
		deps := playerTestDeps(otherPlayer(), PlayerHooks{}, nil)
		unknown := protocol.SpellOnPlayer{TargetIndex: 7, SpellWidget: 1151 << 16, SpellChild: 999999, SpellItemID: -1}
		if got := routeSpellOnPlayer(p, unknown, deps, drop, acted, noAction); got != noAction() {
			t.Fatalf("unknown spell = %+v, want noAction", got)
		}
	})
	t.Run("spell-on-player event claims the click", func(t *testing.T) {
		p := testPlayer()
		deps := playerTestDeps(otherPlayer(), PlayerHooks{}, nil)
		deps.SpellHooks.CastSpell = func(attacker *game.Player, m *game.Mobile, spell game.CombatSpell) {
			t.Fatal("handled event must skip the cast")
		}
		p.EmitCustomEvent = func(name string, payload any) {
			if ev, ok := payload.(*SpellOnPlayerEvent); ok && name == "magic:spell-on-player" {
				ev.Handled = true
			}
		}
		if got := routeSpellOnPlayer(p, msg, deps, drop, acted, noAction); got != acted() {
			t.Fatalf("handled event = %+v, want acted", got)
		}
	})
	t.Run("unwired cast seam drops", func(t *testing.T) {
		p := testPlayer()
		deps := playerTestDeps(otherPlayer(), PlayerHooks{}, nil)
		if got := routeSpellOnPlayer(p, msg, deps, drop, acted, noAction); got.Reason != ReasonMagicOnPlayer {
			t.Fatalf("unwired seam = %+v, want ReasonMagicOnPlayer", got)
		}
	})
}

func TestTelegrab(t *testing.T) {
	const (
		groundItemID = 4151 // an abyssal whip on the floor
		x            = 3090
		y            = 3524
	)
	// The name seam returns the telegrab name: 1168 is an
	// effect spell, so only the cache-name fallback resolves
	// it — exactly as production resolves it.
	nameSeam := func(widgetID, itemID int) (string, bool) { return "Telekinetic Grab", true }
	schedule := &fakeSchedule{}

	setup := func(magicLevel int, runes bool) (*game.Player, *capturingSession, *managerStore) {
		p, session := spellTestPlayer(magicLevel)
		p.SetRegistered(true)
		if runes {
			p.Inventory().Adds(runeLawID, 1)
			p.Inventory().Adds(runeAirID, 1)
		}
		store := newManagerStore()
		store.Register(game.NewItemOnGround(game.GroundItemSeenByEveryone, "", game.Location{X: x, Y: y, Z: 0}, groundItemID, 1, nil, true, -1, false, nil))
		return p, session, store
	}
	depsFor := func(store *managerStore) Deps {
		deps := Deps{GroundItems: store}
		deps.SpellHooks.SpellName = nameSeam
		deps.SpellHooks.ScheduleTask = schedule.schedule
		return deps
	}
	msg := protocol.SpellOnGround{GroundItemID: groundItemID, X: x, Y: y, SpellWidget: 1151 << 16, SpellChild: 1168, SpellItemID: -1}
	drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
	acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
	noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }

	t.Run("cast consumes runes and lands the item", func(t *testing.T) {
		p, _, store := setup(33, true)
		deps := depsFor(store)
		// Decode-level: a real opcode-94 payload.
		payload := spellOnGroundPayload(groundItemID, x, y, (1151<<16)|1168, 1168, -1, 0)
		got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpLocTAlt, Payload: payload}, 7, deps)
		if got != (Outcome{Recognized: true, Implemented: true}) {
			t.Fatalf("telegrab = %+v, want implemented", got)
		}
		if got := p.Inventory().GetAmount(runeLawID); got != 0 {
			t.Fatalf("law runes = %d, want 0 (spent)", got)
		}
		if got := p.Inventory().GetAmount(runeAirID); got != 0 {
			t.Fatalf("air runes = %d, want 0 (spent)", got)
		}
		if len(schedule.tasks) != 1 || schedule.tasks[0].Delay != telegrabLandTicks {
			t.Fatalf("scheduled = %d tasks, want 1 landing in %d ticks", len(schedule.tasks), telegrabLandTicks)
		}
		// The item is still on the floor until the projectile
		// lands.
		if got := p.Inventory().GetAmount(groundItemID); got != 0 {
			t.Fatalf("item before landing = %d, want 0", got)
		}
		schedule.run(t, 0, 10)
		if got := p.Inventory().GetAmount(groundItemID); got != 1 {
			t.Fatalf("item after landing = %d, want 1", got)
		}
		if got := p.Skills().Experience(game.SkillMagic); got != telegrabXP {
			t.Fatalf("magic XP = %d, want %d", got, telegrabXP)
		}
	})
	t.Run("no runes refuses with the missing-rune line", func(t *testing.T) {
		p, session, store := setup(33, false)
		deps := depsFor(store)
		if got := routeSpellOnGround(p, msg, 7, deps, drop, acted, noAction); got != noAction() {
			t.Fatalf("no runes = %+v, want noAction", got)
		}
		if len(session.gameMessages) != 1 || session.gameMessages[0] != "You do not have enough Law Runes to cast this spell." {
			t.Fatalf("messages = %v, want the missing-rune line", session.gameMessages)
		}
	})
	t.Run("low Magic refuses with the level line", func(t *testing.T) {
		p, session, store := setup(32, true)
		deps := depsFor(store)
		if got := routeSpellOnGround(p, msg, 7, deps, drop, acted, noAction); got != noAction() {
			t.Fatalf("low magic = %+v, want noAction", got)
		}
		if len(session.gameMessages) != 1 || session.gameMessages[0] != "You need a Magic level of 33 to cast this spell." {
			t.Fatalf("messages = %v, want the level line", session.gameMessages)
		}
	})
	t.Run("out of range walks in", func(t *testing.T) {
		p, _, store := setup(33, true)
		p.SetLocation(game.Location{X: 3120, Y: 3524, Z: 0})
		deps := depsFor(store)
		if got := routeSpellOnGround(p, msg, 7, deps, drop, acted, noAction); got != acted() {
			t.Fatalf("far telegrab = %+v, want acted (walk started)", got)
		}
		if !p.MovementQueue().WalkToPending() {
			t.Fatal("far telegrab must queue a walk-to")
		}
	})
	t.Run("unwired ground store drops", func(t *testing.T) {
		p, _, _ := setup(33, true)
		if got := routeSpellOnGround(p, msg, 7, Deps{}, drop, acted, noAction); got.Reason != ReasonMagicOnItem {
			t.Fatalf("unwired store = %+v, want ReasonMagicOnItem", got)
		}
	})
	t.Run("non-telegrab spell refuses", func(t *testing.T) {
		p, _, store := setup(33, true)
		deps := depsFor(store)
		deps.SpellHooks.SpellName = func(widgetID, itemID int) (string, bool) {
			return "Low Level Alchemy", true
		}
		if got := routeSpellOnGround(p, msg, 7, deps, drop, acted, noAction); got != noAction() {
			t.Fatalf("alchemy on ground = %+v, want noAction", got)
		}
	})
}

func TestAlchemy(t *testing.T) {
	setup := func(magicLevel int) (*game.Player, *capturingSession, *fakeSchedule) {
		p, session := spellTestPlayer(magicLevel)
		p.SetItemDefinitions(alchTestDefs())
		// Queued casts (and the telegrab landing) require
		// a registered player, as in production.
		p.SetRegistered(true)
		p.Inventory().Adds(itemPlain, 2)
		p.Inventory().Adds(runeNatureID, 2)
		p.Inventory().Adds(runeFireID, 10)
		schedule := &fakeSchedule{}
		return p, session, schedule
	}
	depsFor := func(schedule *fakeSchedule) Deps {
		deps := Deps{}
		deps.SpellHooks.ScheduleTask = schedule.schedule
		return deps
	}

	t.Run("high alchemy pays coins and XP", func(t *testing.T) {
		p, _, schedule := setup(55)
		slot := findSlot(p, itemPlain)
		if slot < 0 {
			t.Fatal("plain item missing from inventory")
		}
		if !castOnItem(p, spellHighAlchemyID, itemPlain, slot, 7, depsFor(schedule)) {
			t.Fatal("high alchemy must be taken")
		}
		if got := p.Inventory().GetAmount(itemPlain); got != 1 {
			t.Fatalf("plain items = %d, want 1 (one alched)", got)
		}
		if got := p.Inventory().GetAmount(itemCoinsID); got != 100 {
			t.Fatalf("coins = %d, want 100", got)
		}
		if got := p.Inventory().GetAmount(runeFireID); got != 5 {
			t.Fatalf("fire runes = %d, want 5 (5 spent)", got)
		}
		if got := p.Inventory().GetAmount(runeNatureID); got != 1 {
			t.Fatalf("nature runes = %d, want 1 (1 spent)", got)
		}
		if got := p.Skills().Experience(game.SkillMagic); got != alchHighXP {
			t.Fatalf("magic XP = %d, want %d", got, alchHighXP)
		}
	})
	t.Run("second cast in the window queues", func(t *testing.T) {
		p, _, schedule := setup(55)
		slot := findSlot(p, itemPlain)
		deps := depsFor(schedule)
		if !castOnItem(p, spellLowAlchemyID, itemPlain, slot, 7, deps) {
			t.Fatal("first low alchemy must be taken")
		}
		if got := p.Inventory().GetAmount(itemCoinsID); got != 60 {
			t.Fatalf("coins after first = %d, want 60", got)
		}
		slot = findSlot(p, itemPlain)
		if !castOnItem(p, spellLowAlchemyID, itemPlain, slot, 7, deps) {
			t.Fatal("second low alchemy must be taken (queued)")
		}
		// Still one plain item: the queued cast has not fired.
		if got := p.Inventory().GetAmount(itemPlain); got != 1 {
			t.Fatalf("plain items = %d, want 1 (queued, not cast)", got)
		}
		if len(schedule.tasks) != 1 || schedule.tasks[0].Delay != alchLowCastTicks {
			t.Fatalf("scheduled = %d tasks, want 1 queued in %d ticks", len(schedule.tasks), alchLowCastTicks)
		}
		schedule.run(t, 0, 10)
		if got := p.Inventory().GetAmount(itemPlain); got != 0 {
			t.Fatalf("plain items after queue fires = %d, want 0", got)
		}
		if got := p.Inventory().GetAmount(itemCoinsID); got != 120 {
			t.Fatalf("coins after queue fires = %d, want 120", got)
		}
	})
	t.Run("low Magic refuses with the level line", func(t *testing.T) {
		p, session, schedule := setup(20)
		slot := findSlot(p, itemPlain)
		if !castOnItem(p, spellLowAlchemyID, itemPlain, slot, 7, depsFor(schedule)) {
			t.Fatal("low-magic alchemy is still taken (refused with a message)")
		}
		if len(session.gameMessages) != 1 || session.gameMessages[0] != "You need a Magic level of 21 to cast this spell." {
			t.Fatalf("messages = %v, want the level line", session.gameMessages)
		}
		if got := p.Inventory().GetAmount(itemPlain); got != 2 {
			t.Fatalf("plain items = %d, want 2 (nothing alched)", got)
		}
	})
	t.Run("untradeable item warns and does not cast", func(t *testing.T) {
		p, session, schedule := setup(55)
		defs := alchTestDefs()
		defs.cfgs[itemPlain] = defConfig{slot: -1, dropable: true, tradeable: false}
		p.SetItemDefinitions(defs)
		slot := findSlot(p, itemPlain)
		if !castOnItem(p, spellHighAlchemyID, itemPlain, slot, 7, depsFor(schedule)) {
			t.Fatal("untradeable alchemy is still taken (refused with a message)")
		}
		if len(session.gameMessages) != 1 || session.gameMessages[0] != "That item cannot be traded. Really cast High Level Alchemy on it?" {
			t.Fatalf("messages = %v, want the warning line", session.gameMessages)
		}
		if got := p.Inventory().GetAmount(itemPlain); got != 2 {
			t.Fatalf("plain items = %d, want 2 (nothing alched)", got)
		}
		if got := p.Inventory().GetAmount(runeFireID); got != 10 {
			t.Fatalf("fire runes = %d, want 10 (none spent)", got)
		}
	})
	t.Run("coins cannot be alched", func(t *testing.T) {
		p, session, schedule := setup(55)
		p.Inventory().Adds(itemCoinsID, 10)
		slot := findSlot(p, itemCoinsID)
		if !castOnItem(p, spellHighAlchemyID, itemCoinsID, slot, 7, depsFor(schedule)) {
			t.Fatal("coins alchemy is still taken (refused with a message)")
		}
		if len(session.gameMessages) != 1 || session.gameMessages[0] != "This spell can not be cast on this item." {
			t.Fatalf("messages = %v, want the refusal line", session.gameMessages)
		}
	})
	t.Run("reanimation is not taken", func(t *testing.T) {
		p, _, schedule := setup(55)
		slot := findSlot(p, itemPlain)
		if castOnItem(p, spellBasicReanimationID, itemPlain, slot, 7, depsFor(schedule)) {
			t.Fatal("reanimation must not be taken (Arceuus NOT STARTED)")
		}
	})
	t.Run("widget-target route alchs through the handle", func(t *testing.T) {
		p, _, schedule := setup(55)
		slot := findSlot(p, itemPlain)
		deps := depsFor(schedule)
		deps.SpellHooks.SpellName = func(widgetID, itemID int) (string, bool) {
			return "High Level Alchemy", true
		}
		// Decode-level: a real opcode-90 payload. The target
		// is the main inventory (group 149); the source is the
		// spellbook widget naming the alchemy spell.
		payload := widgetTargetPayload((149<<16)|3214, slot, (1151<<16)|1178, 0, -1, itemPlain)
		got := Handle(p, game.TurnPacket{Opcode: protocol.NativeIfButtont, Payload: payload}, 7, deps)
		if got != (Outcome{Recognized: true, Implemented: true}) {
			t.Fatalf("widget-target alchemy = %+v, want implemented", got)
		}
		if got := p.Inventory().GetAmount(itemCoinsID); got != 100 {
			t.Fatalf("coins = %d, want 100", got)
		}
	})
}

// TestCastSpellOnItemHookDispatch covers the phase-43
// onSpellOnItem dispatch in castSpellOnItem
// (NetworkBuilder.ts:1182-1204): a hook claiming the cast
// consumes the click before the listener's own cast route
// runs, a non-matching cast falls through to alchemy
// unchanged, and a nil seam reads as no hooks wired.
// Fixtures only.
func TestCastSpellOnItemHookDispatch(t *testing.T) {
	const ironOre = 440
	msg := func(spellName string, targetID, slot int) protocol.WidgetTarget {
		return protocol.WidgetTarget{
			SourceWidgetID: int32(192<<16 | 30),
			SourceItemID:   0,
			TargetItemID:   targetID,
			TargetSlot:     slot,
		}
	}
	depsWithHook := func(claim func(ev *SpellOnItemEvent) bool) Deps {
		deps := Deps{}
		deps.SpellHooks.SpellName = func(widgetID, itemID int) (string, bool) {
			return "Superheat Item", true
		}
		deps.SpellHooks.EmitSpellOnItem = func(ev *SpellOnItemEvent) bool {
			return claim(ev)
		}
		return deps
	}

	t.Run("hook claim consumes the click before alchemy", func(t *testing.T) {
		p, _ := spellTestPlayer(43)
		p.SetRegistered(true)
		p.Inventory().Adds(ironOre, 1)
		p.Inventory().Adds(runeFireID, 4)
		p.Inventory().Adds(runeNatureID, 1)
		slot := findSlot(p, ironOre)
		seen := 0
		deps := depsWithHook(func(ev *SpellOnItemEvent) bool {
			seen++
			if ev.Player != p || ev.SpellName != "Superheat Item" || ev.ItemID != ironOre || ev.Slot != slot || ev.Cycle != 9 {
				t.Errorf("event = %+v, want player + Superheat Item + iron ore slot %d cycle 9", ev, slot)
			}
			ev.Handled = true
			return true
		})
		if !castSpellOnItem(p, msg("Superheat Item", ironOre, slot), 9, deps) {
			t.Fatal("castSpellOnItem = false, want true (hook claimed the cast)")
		}
		if seen != 1 {
			t.Fatalf("hook calls = %d, want 1", seen)
		}
		// The runes are untouched: the hook owns the cast, and
		// the listener's alchemy route never ran for it.
		if got := p.Inventory().GetAmount(runeFireID); got != 4 {
			t.Fatalf("fire runes = %d, want 4 (hook's cast, not alchemy's)", got)
		}
	})

	t.Run("non-matching cast falls through to alchemy", func(t *testing.T) {
		p, _, schedule := func() (*game.Player, *capturingSession, *fakeSchedule) {
			pp, s := spellTestPlayer(55)
			pp.SetRegistered(true)
			pp.Inventory().Adds(itemPlain, 2)
			pp.Inventory().Adds(runeNatureID, 2)
			pp.Inventory().Adds(runeFireID, 10)
			return pp, s, &fakeSchedule{}
		}()
		slot := findSlot(p, itemPlain)
		deps := depsWithHook(func(ev *SpellOnItemEvent) bool { return false })
		deps.SpellHooks.SpellName = func(widgetID, itemID int) (string, bool) {
			return "Low Level Alchemy", true
		}
		deps.SpellHooks.ScheduleTask = schedule.schedule
		if !castSpellOnItem(p, msg("Low Level Alchemy", itemPlain, slot), 7, deps) {
			t.Fatal("castSpellOnItem = false, want true (alchemy took the unclaimed cast)")
		}
		if got := p.Inventory().GetAmount(itemCoinsID); got != 60 {
			t.Fatalf("coins = %d, want 60 (low alchemy paid)", got)
		}
	})

	t.Run("nil seam reads as no hooks wired", func(t *testing.T) {
		p, _ := spellTestPlayer(43)
		p.SetRegistered(true)
		p.Inventory().Adds(ironOre, 1)
		// No SpellName seam and no EmitSpellOnItem: the spell
		// resolves to nothing, so the click falls through.
		if castSpellOnItem(p, msg("Superheat Item", ironOre, 0), 7, Deps{}) {
			t.Fatal("castSpellOnItem = true, want false (no hooks, no resolvable spell)")
		}
	})
}

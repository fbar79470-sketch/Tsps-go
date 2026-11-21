package main

import (
	"testing"

	"tsps-go/internal/cache"
	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/persist"
	"tsps-go/internal/plugins"
)

// Combat-wiring tests: InstallCombatListeners (lookup, hook
// adapters, the no-clobber rule for NpcHooks, the combat seam),
// InstallCombatHooks (seam population, event translation,
// read-backs, victim resolution, the provider-registry install),
// InstallDeathWiring (save + ground-item seams, respawn
// location), and the adapter nil-degradations. Fixtures only: a hub with
// test plugins, a world with added players/NPCs, and hand-built
// game events. The game.Hooks global is saved and restored
// around every test that installs hooks.

// combatTestHub builds a hub whose single plugin registers the
// given combat hooks, then loads it.
func combatTestHub(t *testing.T, register func(*plugins.Api)) *plugins.Hub {
	t.Helper()
	hub := plugins.NewHub([]plugins.Plugin{{
		Name:     "CombatProbe",
		Register: register,
	}}, plugins.HubOptions{Logger: discardLogger()})
	hub.Load()
	return hub
}

// saveCombatHooks snapshots the game.Hooks seam and restores it
// when the test ends.
func saveCombatHooks(t *testing.T) {
	t.Helper()
	saved := game.Hooks
	t.Cleanup(func() { game.Hooks = saved })
}

func TestInstallDeathWiring(t *testing.T) {
	saveCombatHooks(t)
	prevLocation := game.DefaultRespawnLocation
	t.Cleanup(func() { game.SetDefaultRespawnLocation(prevLocation) })
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	groundItems := game.NewGroundItemManager(game.GroundItemManagerConfig{})
	saves := newSaveCoordinator(persist.NewMemoryStore(), world, discardLogger(), 600, 100, 10)
	InstallDeathWiring(saves, groundItems, world, discardLogger())
	if game.Hooks.DropGroundItem == nil || game.Hooks.RequestSave == nil {
		t.Fatal("death seams must be populated (ground items + save)")
	}
	if game.DefaultRespawnLocation != world.Spawn() {
		t.Fatalf("respawn location = %v, want world spawn %v", game.DefaultRespawnLocation, world.Spawn())
	}
	// The ground seam registers a visible stack for the owner.
	owner := game.NewPlayer("owner", game.Location{X: 3000, Y: 3000})
	game.Hooks.DropGroundItem(game.NewItem(526, 1), game.Location{X: 3000, Y: 3000}, owner)
	items := groundItems.Items()
	if len(items) != 1 || items[0].ItemID != 526 || items[0].Position != (game.Location{X: 3000, Y: 3000}) {
		t.Fatalf("ground items = %+v, want one bones stack at (3000, 3000)", items)
	}
	// Nil arguments leave the seams unset (documented gap, not a panic).
	game.Hooks = game.CombatHooks{}
	game.SetDefaultRespawnLocation(prevLocation)
	InstallDeathWiring(nil, nil, nil, discardLogger())
	if game.Hooks.DropGroundItem != nil || game.Hooks.RequestSave != nil {
		t.Fatal("nil death wiring must leave the seams unset")
	}
	if game.DefaultRespawnLocation != prevLocation {
		t.Fatalf("nil world must leave the respawn location at %v", prevLocation)
	}
}

// TestDeathAdapters checks the Phase 28 death adapters: the
// handled read-back on the player-death event, the per-item
// read-back (SuppressDefaultDrop + ExtraDrops), the drops
// read-back (Items reassignment), and the first-answer gate
// passthroughs.
func TestDeathAdapters(t *testing.T) {
	saveCombatHooks(t)
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	victim := game.NewPlayer("victim", game.Location{X: 3000, Y: 3000})
	if _, ok := world.Players().Add(victim); !ok {
		t.Fatal("player add failed")
	}
	killer := game.NewPlayer("killer", game.Location{X: 3001, Y: 3000})
	if _, ok := world.Players().Add(killer); !ok {
		t.Fatal("killer add failed")
	}
	defeated := 0
	hub := combatTestHub(t, func(api *plugins.Api) {
		api.OnPlayerDeath(func(ev *plugins.PlayerDeathEvent) { ev.Handled = true })
		api.OnShouldDropItemsOnDeath(func(ev *plugins.ShouldDropItemsOnDeathEvent) {
			ev.ShouldDrop = plugins.BoolPtr(false)
		})
		api.OnShouldKeepItemOnDeath(func(ev *plugins.ShouldKeepItemOnDeathEvent) {
			ev.Keep = plugins.BoolPtr(true)
		})
		api.OnPlayerDeathItemDrop(func(ev *plugins.PlayerDeathItemDropEvent) {
			ev.SuppressDefaultDrop = true
			ev.ExtraDrops = append(ev.ExtraDrops, game.NewItem(526, 1))
		})
		api.OnPlayerDeathDrops(func(ev *plugins.PlayerDeathDropsEvent) {
			ev.Items = ev.Items[1:]
		})
		api.OnPlayerDefeated(func(*plugins.PlayerDefeatedEvent) { defeated++ })
	})
	InstallCombatHooks(hub, world)
	// Handled read-back.
	ev := &game.CombatDeathEvent{Victim: &victim.Mobile, Killer: killer, ItemsLost: true, HasItemsLost: true}
	if !game.Hooks.OnPlayerDeath(ev) || !ev.Handled {
		t.Fatal("handled death must read back true")
	}
	// First-answer gates pass through.
	if got := game.Hooks.ShouldDropItemsOnDeath(victim, killer); got == nil || *got {
		t.Fatalf("should-drop answer = %v, want false", got)
	}
	if got := game.Hooks.ShouldKeepItemOnDeath(victim, game.NewItem(995, 1)); got == nil || !*got {
		t.Fatalf("should-keep answer = %v, want true", got)
	}
	// Per-item read-back.
	itemEv := &game.DeathItemDropEvent{Player: victim, Killer: killer, Item: game.NewItem(995, 100), DropEligible: true, ShouldDropItems: true}
	if game.Hooks.OnPlayerDeathItemDrop(itemEv) {
		t.Fatal("unhandled item drop must return false")
	}
	if !itemEv.SuppressDefaultDrop || len(itemEv.ExtraDrops) != 1 || itemEv.ExtraDrops[0].ID() != 526 {
		t.Fatalf("item read-back = suppress %v extras %+v, want suppress + 1 bones", itemEv.SuppressDefaultDrop, itemEv.ExtraDrops)
	}
	// Drops read-back: the grave took the first item.
	dropsEv := &game.DeathDropsEvent{
		Player: victim, Killer: killer,
		Items: []game.Item{game.NewItem(995, 100), game.NewItem(526, 1)},
	}
	game.Hooks.OnPlayerDeathDrops(dropsEv)
	if len(dropsEv.Items) != 1 || dropsEv.Items[0].ID() != 526 {
		t.Fatalf("drops Items = %+v, want only bones", dropsEv.Items)
	}
	// Defeated fires (killer and killer-less alike).
	game.Hooks.OnPlayerDefeated(killer, victim)
	game.Hooks.OnPlayerDefeated(nil, victim)
	if defeated != 2 {
		t.Fatalf("defeated calls = %d, want 2", defeated)
	}
}

func TestPlayerListLookup(t *testing.T) {
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	player := game.NewPlayer("testplayer", game.Location{X: 3000, Y: 3000})
	index, ok := world.Players().Add(player)
	if !ok {
		t.Fatal("player add failed")
	}
	lookup := playerListLookup{list: world.Players()}
	if got := lookup.Find(index); got != player {
		t.Fatalf("Find(%d) = %v, want the player", index, got)
	}
	if got := lookup.Find(index + 1); got != nil {
		t.Fatalf("Find(empty slot) = %v, want nil", got)
	}
	if got := (playerListLookup{}).Find(index); got != nil {
		t.Fatalf("nil list Find = %v, want nil", got)
	}
}

func TestInstallCombatListeners(t *testing.T) {
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	player := game.NewPlayer("testplayer", game.Location{X: 3000, Y: 3000})
	index, ok := world.Players().Add(player)
	if !ok {
		t.Fatal("player add failed")
	}

	var routeSeen *plugins.PlayerOptionRouteEvent
	var attackSeen *plugins.PlayerAttackEvent
	var optionSeen *plugins.NpcAttackOptionEvent
	hub := combatTestHub(t, func(api *plugins.Api) {
		api.OnCustomEvent(plugins.PlayerOptionRouteEventName, func(payload any) {
			if ev, ok := payload.(*plugins.PlayerOptionRouteEvent); ok {
				routeSeen = ev
				ev.Handled = true
			}
		})
		api.OnPlayerAttack(func(ev *plugins.PlayerAttackEvent) { attackSeen = ev })
		api.OnNpcAttackOption(func(ev *plugins.NpcAttackOptionEvent) {
			optionSeen = ev
			ev.Handled = true
		})
	})

	// The no-clobber rule: StartDialogue and EmitOperated are
	// already installed (by InstallDialogueListeners) and must
	// survive InstallCombatListeners, which sets only the
	// EmitNpcAttackOption field.
	deps := listeners.Deps{}
	started := false
	operated := false
	deps.NpcHooks.StartDialogue = func(*listeners.NpcDialogueRequest) bool { started = true; return true }
	deps.NpcHooks.EmitOperated = func(*listeners.NpcOperatedEvent) { operated = true }
	InstallCombatListeners(&deps, hub, world.Players())

	if deps.Players == nil || deps.Players.Find(index) != player {
		t.Fatal("player lookup not wired")
	}
	if deps.CombatAttack == nil {
		t.Fatal("combat seam not wired")
	}
	if deps.PlayerHooks.EmitOptionRoute == nil || deps.PlayerHooks.EmitPlayerAttack == nil {
		t.Fatal("player hooks not wired")
	}
	if deps.NpcHooks.EmitNpcAttackOption == nil {
		t.Fatal("NPC attack-option gate not wired")
	}
	deps.NpcHooks.StartDialogue(nil)
	deps.NpcHooks.EmitOperated(nil)
	if !started || !operated {
		t.Fatal("InstallCombatListeners clobbered the dialogue-installed NpcHooks fields")
	}

	// The route adapter claims through the custom event and
	// mirrors Handled onto the listener event.
	target := game.NewPlayer("otherplayer", game.Location{X: 3001, Y: 3000})
	route := &listeners.PlayerOptionRouteEvent{Player: player, Target: target, Option: 1}
	if !deps.PlayerHooks.EmitOptionRoute(route) || !route.Handled {
		t.Fatalf("route = %+v, want claimed", route)
	}
	if routeSeen == nil || routeSeen.Player != player || routeSeen.Target != target || routeSeen.Option != 1 {
		t.Fatalf("route event = %+v, want player/target/option 1", routeSeen)
	}

	// The player-attack adapter delivers the event.
	deps.PlayerHooks.EmitPlayerAttack(&listeners.PlayerAttackEvent{Player: player, Target: target})
	if attackSeen == nil || attackSeen.Player != player || attackSeen.Target != target {
		t.Fatalf("attack event = %+v, want player/target", attackSeen)
	}

	// The NPC attack-option adapter claims and mirrors Handled.
	npc := game.NewNPC(200, game.Location{X: 3001, Y: 3000})
	option := &listeners.NpcAttackOptionEvent{Player: player, NPC: npc, NpcID: 200, ClickType: 1}
	if !deps.NpcHooks.EmitNpcAttackOption(option) || !option.Handled {
		t.Fatalf("option = %+v, want claimed", option)
	}
	if optionSeen == nil || optionSeen.Player != player || optionSeen.NPC != npc || optionSeen.NpcID != 200 || optionSeen.ClickType != 1 {
		t.Fatalf("option event = %+v, want player/NPC/id 200/clickType 1", optionSeen)
	}

	// The combat seam calls through to the combat contract:
	// the attack targets the given Mobile.
	deps.CombatAttack(player, &npc.Mobile)
	if player.GetCombat().Target() != &npc.Mobile {
		t.Fatalf("combat target = %v, want the NPC Mobile", player.GetCombat().Target())
	}
}

func TestInstallCombatListenersDegradations(t *testing.T) {
	// Nil deps: no panic.
	InstallCombatListeners(nil, nil, nil)

	// Nil hub: the lookup and combat seam still wire; the hook
	// adapters stay unset (the no-hooks defaults).
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	deps := listeners.Deps{}
	InstallCombatListeners(&deps, nil, world.Players())
	if deps.Players == nil {
		t.Fatal("nil hub must still wire the lookup")
	}
	if deps.CombatAttack == nil {
		t.Fatal("nil hub must still wire the combat seam")
	}
	if deps.PlayerHooks.EmitOptionRoute != nil || deps.PlayerHooks.EmitPlayerAttack != nil || deps.NpcHooks.EmitNpcAttackOption != nil {
		t.Fatal("nil hub must leave the hook adapters unset")
	}

	// Nil player list: the lookup stays unset (the unwired drop).
	deps = listeners.Deps{}
	InstallCombatListeners(&deps, combatTestHub(t, func(*plugins.Api) {}), nil)
	if deps.Players != nil {
		t.Fatal("nil list must leave the lookup unset")
	}
	if deps.CombatAttack == nil {
		t.Fatal("nil list must still wire the combat seam")
	}
}

func TestInstallCombatHooksPopulatesSeam(t *testing.T) {
	saveCombatHooks(t)
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	hub := combatTestHub(t, func(*plugins.Api) {})
	InstallCombatHooks(hub, world)

	if game.Hooks.OnNpcDeath == nil || game.Hooks.OnPlayerDeath == nil ||
		game.Hooks.OnPlayerDefeated == nil || game.Hooks.ShouldDropItemsOnDeath == nil ||
		game.Hooks.ShouldKeepItemOnDeath == nil || game.Hooks.OnPlayerDeathItemDrop == nil ||
		game.Hooks.OnPlayerDeathDrops == nil ||
		game.Hooks.OnCombatHitPrepare == nil || game.Hooks.OnCombatHitRoll == nil ||
		game.Hooks.OnCombatHitDamage == nil || game.Hooks.OnCombatHitResolved == nil ||
		game.Hooks.OnCombatAttackDistance == nil || game.Hooks.OnCanAttack == nil ||
		game.Hooks.SpellDisabled == nil || game.Hooks.SpellRuneBypass == nil ||
		game.Hooks.CheckSpellRuneSource == nil || game.Hooks.ConsumeSpellRuneSource == nil ||
		game.Hooks.ApplySkull == nil || game.Hooks.IsSkulled == nil || game.Hooks.ExtendSkull == nil {
		t.Fatal("the twenty ported seam fields must be populated")
	}
	// The table-only families stay unset: the zero seam is inert.
	if game.Hooks.OnNpcBeforeDeath != nil || game.Hooks.OnPlayerBeforeDeath != nil ||
		game.Hooks.OnCombatHitRolled != nil ||
		game.Hooks.OnAttackTiming != nil || game.Hooks.OnAttackSpeed != nil ||
		game.Hooks.ManipulateHit != nil {
		t.Fatal("table-only seam fields must stay unset")
	}
	if hub.NpcCombatMethodProviderRegistry() == nil {
		t.Fatal("the provider registry must be installed on the hub")
	}

	// A nil hub leaves the seam untouched.
	game.Hooks = game.CombatHooks{}
	InstallCombatHooks(nil, world)
	if game.Hooks.OnNpcDeath != nil || game.Hooks.OnCombatHitPrepare != nil {
		t.Fatal("nil hub must leave the seam zero")
	}
}

func TestInstallRangedData(t *testing.T) {
	saveCombatHooks(t)
	t.Cleanup(func() { game.RegisterRangedDataResolver(nil) })
	InstallRangedData()

	// The production resolver is installed: a bow resolves
	// RANGED through GetMethod, and the ammunition data is
	// registered (a poisoned rune arrow is the rune arrow).
	archer := game.NewPlayer("archer", game.Location{X: 3000, Y: 3000})
	archer.GetCombat()
	archer.Equipment().Set(game.WeaponSlot, game.NewItem(841, 1))
	archer.Equipment().Set(game.AmmunitionSlot, game.NewItem(893, 10))
	if got := game.GetMethod(&archer.Mobile); got.Type() != game.CombatTypeRanged {
		t.Fatalf("bow method = %v, want RANGED", got.Type())
	}
	if got := archer.GetCombat().Ammunition(); got == nil || got.ItemID != 892 {
		t.Fatalf("bow ammunition = %v, want item 892", got)
	}

	// The charge-bow decrement skip is chained onto the seam:
	// a crystal bow spends nothing, a thrown dart still pays.
	if game.Hooks.DecrementAmmo == nil {
		t.Fatal("DecrementAmmo seam must be installed")
	}
	crystal := game.NewItem(4214, 1)
	if !game.Hooks.DecrementAmmo(archer, crystal, game.NewItem(-1, 0), 1) {
		t.Errorf("crystal bow decrement should be skipped")
	}
	dart := game.NewItem(806, 5)
	if game.Hooks.DecrementAmmo(archer, dart, game.NewItem(-1, 0), 1) {
		t.Errorf("thrown dart decrement should not be skipped")
	}
	// A previously installed hook runs first and wins when it
	// claims the shot.
	game.Hooks.DecrementAmmo = func(player *game.Player, weapon, ammo game.Item, amount int) bool { return true }
	InstallRangedData()
	if !game.Hooks.DecrementAmmo(archer, dart, game.NewItem(-1, 0), 1) {
		t.Errorf("a claiming previous hook must win after chaining")
	}
	// Installing twice is idempotent: the resolver still
	// resolves a bow.
	if got := game.GetMethod(&archer.Mobile); got.Type() != game.CombatTypeRanged {
		t.Fatalf("bow method after reinstall = %v, want RANGED", got.Type())
	}
}

func TestCombatDeathAdapters(t *testing.T) {
	saveCombatHooks(t)
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	killer := game.NewPlayer("killer", game.Location{X: 3000, Y: 3000})
	if _, ok := world.Players().Add(killer); !ok {
		t.Fatal("killer add failed")
	}
	victim := game.NewPlayer("victim", game.Location{X: 3001, Y: 3000})
	if _, ok := world.Players().Add(victim); !ok {
		t.Fatal("victim add failed")
	}
	npc := game.NewNPC(200, game.Location{X: 3002, Y: 3000})
	if _, ok := world.NPCs().Add(npc); !ok {
		t.Fatal("NPC add failed")
	}

	var npcDeathSeen *plugins.NpcDeathEvent
	var playerDeathSeen *plugins.PlayerDeathEvent
	hub := combatTestHub(t, func(api *plugins.Api) {
		api.OnNpcDeath(func(ev *plugins.NpcDeathEvent) {
			npcDeathSeen = ev
			ev.Remains = &plugins.NpcDeathRemains{Ticks: 5, RespawnTicks: 50, HasRespawnTicks: true}
		})
		api.OnPlayerDeath(func(ev *plugins.PlayerDeathEvent) { playerDeathSeen = ev })
	})
	InstallCombatHooks(hub, world)

	// NPC death: the victim resolves to the NPC, the event
	// carries the killer/damagers/tile, and the remains read
	// back onto the game event.
	death := &game.CombatDeathEvent{
		Victim:   &npc.Mobile,
		Killer:   killer,
		Damagers: []*game.Player{killer},
		Location: game.Location{X: 3002, Y: 3000, Z: 0},
	}
	game.Hooks.OnNpcDeath(death)
	if npcDeathSeen == nil || npcDeathSeen.NPC != npc || npcDeathSeen.NpcID != 200 ||
		npcDeathSeen.Killer != killer || len(npcDeathSeen.Damagers) != 1 || npcDeathSeen.Location != death.Location {
		t.Fatalf("NPC death event = %+v, want NPC/id 200/killer/damagers/tile", npcDeathSeen)
	}
	if death.RemainsTicks != 5 || death.RespawnTicks != 50 {
		t.Fatalf("remains read-back = (%d, %d), want (5, 50)", death.RemainsTicks, death.RespawnTicks)
	}

	// Player death: the victim resolves to the player, and
	// ItemsLost stays unset (no item-loss pipeline).
	playerDeath := &game.CombatDeathEvent{Victim: &victim.Mobile, Killer: killer, Location: victim.Location()}
	game.Hooks.OnPlayerDeath(playerDeath)
	if playerDeathSeen == nil || playerDeathSeen.Player != victim || playerDeathSeen.Killer != killer {
		t.Fatalf("player death event = %+v, want victim/killer", playerDeathSeen)
	}
	if playerDeathSeen.HasItemsLost {
		t.Fatal("ItemsLost must stay unset: the item-loss pipeline is not ported")
	}

	// An unresolvable victim (a Mobile that never joined the
	// world) skips the emit: no hook runs, no panic.
	npcDeathSeen = nil
	playerDeathSeen = nil
	stray := game.NewNPC(201, game.Location{X: 3010, Y: 3000})
	game.Hooks.OnNpcDeath(&game.CombatDeathEvent{Victim: &stray.Mobile})
	game.Hooks.OnPlayerDeath(&game.CombatDeathEvent{Victim: &stray.Mobile})
	if npcDeathSeen != nil || playerDeathSeen != nil {
		t.Fatal("an unresolvable victim must skip the death emits")
	}

	// A nil event is a no-op.
	game.Hooks.OnNpcDeath(nil)
	game.Hooks.OnPlayerDeath(nil)
}

func TestCombatDeathRemainsZeroTicksNoReadback(t *testing.T) {
	saveCombatHooks(t)
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	npc := game.NewNPC(200, game.Location{X: 3002, Y: 3000})
	if _, ok := world.NPCs().Add(npc); !ok {
		t.Fatal("NPC add failed")
	}
	hub := combatTestHub(t, func(api *plugins.Api) {
		api.OnNpcDeath(func(ev *plugins.NpcDeathEvent) {
			ev.Remains = &plugins.NpcDeathRemains{Ticks: 0, RespawnTicks: 50, HasRespawnTicks: true}
		})
	})
	InstallCombatHooks(hub, world)
	death := &game.CombatDeathEvent{Victim: &npc.Mobile, Location: npc.Location()}
	game.Hooks.OnNpcDeath(death)
	if death.RemainsTicks != 0 || death.RespawnTicks != 0 {
		t.Fatalf("zero-tick remains read back (%d, %d), want zeros", death.RemainsTicks, death.RespawnTicks)
	}
}

func TestCombatHitAdapters(t *testing.T) {
	saveCombatHooks(t)
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	attacker := game.NewPlayer("attacker", game.Location{X: 3000, Y: 3000})
	target := game.NewNPC(200, game.Location{X: 3001, Y: 3000})

	var prepareSeen *plugins.CombatHitPrepareEvent
	var rollSeen *plugins.CombatHitRollEvent
	var damageSeen *plugins.CombatHitDamageEvent
	var resolvedSeen *plugins.CombatHitResolvedEvent
	var distanceSeen *plugins.CombatAttackDistanceEvent
	hub := combatTestHub(t, func(api *plugins.Api) {
		api.OnCombatHitPrepare(func(ev *plugins.CombatHitPrepareEvent) { prepareSeen = ev })
		api.OnCombatHitRoll(func(ev *plugins.CombatHitRollEvent) {
			rollSeen = ev
			ev.ForceAccurate = true
			ev.ForceMaxHit = true
			ev.BypassProtectionPrayer = true
		})
		api.OnCombatHitDamage(func(ev *plugins.CombatHitDamageEvent) {
			damageSeen = ev
			ev.Accurate = true
		})
		api.OnCombatHitResolved(func(ev *plugins.CombatHitResolvedEvent) { resolvedSeen = ev })
		api.OnCombatAttackDistance(func(ev *plugins.CombatAttackDistanceEvent) {
			distanceSeen = ev
			ev.Distance = 7
		})
	})
	InstallCombatHooks(hub, world)

	// Prepare: attacker/target/type translate (RANGED maps to
	// RANGED — the enum orders match).
	game.Hooks.OnCombatHitPrepare(&game.CombatHitPrepareEvent{
		Attacker: &attacker.Mobile, Target: &target.Mobile, CombatType: game.CombatTypeRanged,
	})
	if prepareSeen == nil || prepareSeen.Attacker != &attacker.Mobile || prepareSeen.Target != &target.Mobile ||
		prepareSeen.CombatType != plugins.CombatTypeRanged {
		t.Fatalf("prepare event = %+v, want attacker/target/RANGED", prepareSeen)
	}

	// Roll: the three flags read back.
	roll := &game.CombatHitRollEvent{Attacker: &attacker.Mobile, Target: &target.Mobile, CombatType: game.CombatTypeMelee}
	game.Hooks.OnCombatHitRoll(roll)
	if rollSeen == nil || rollSeen.CombatType != plugins.CombatTypeMelee {
		t.Fatalf("roll event = %+v, want MELEE", rollSeen)
	}
	if !roll.ForceAccurate || !roll.ForceMaxHit || !roll.BypassProtectionPrayer {
		t.Fatalf("roll read-back = %+v, want all three flags", roll)
	}

	// Damage: Accurate reads back; the Hit (PendingHit) and
	// Damage (HitDamage) pointers pass through shared, and a
	// hook replacing Damage has the replacement written back.
	hit := &game.PendingHit{}
	damageValue := &game.HitDamage{}
	replacement := &game.HitDamage{}
	damage := &game.CombatHitDamageEvent{
		Attacker: &attacker.Mobile, Target: &target.Mobile, CombatType: game.CombatTypeMagic,
		Hit: hit, Damage: damageValue, Accurate: false,
	}
	game.Hooks.OnCombatHitDamage(damage)
	if damageSeen == nil || damageSeen.Hit != any(hit) || damageSeen.Damage != any(damageValue) || damageSeen.CombatType != plugins.CombatTypeMagic {
		t.Fatalf("damage event = %+v, want shared hit/damage/MAGIC", damageSeen)
	}
	if !damage.Accurate {
		t.Fatalf("damage read-back = %+v, want accurate", damage)
	}
	// A hook replacing Damage with a different *HitDamage has
	// the replacement written back; a non-*HitDamage
	// replacement leaves the rolled damage unchanged.
	hub2 := combatTestHub(t, func(api *plugins.Api) {
		api.OnCombatHitDamage(func(ev *plugins.CombatHitDamageEvent) { ev.Damage = replacement })
	})
	game.Hooks.OnCombatHitDamage = combatHitDamageAdapter(hub2)
	damage2 := &game.CombatHitDamageEvent{Attacker: &attacker.Mobile, Target: &target.Mobile, Hit: hit, Damage: damageValue}
	game.Hooks.OnCombatHitDamage(damage2)
	if damage2.Damage != replacement {
		t.Fatalf("replaced damage = %v, want the replacement", damage2.Damage)
	}
	hub2b := combatTestHub(t, func(api *plugins.Api) {
		api.OnCombatHitDamage(func(ev *plugins.CombatHitDamageEvent) { ev.Damage = "not-a-hit-damage" })
	})
	game.Hooks.OnCombatHitDamage = combatHitDamageAdapter(hub2b)
	damage3 := &game.CombatHitDamageEvent{Attacker: &attacker.Mobile, Target: &target.Mobile, Hit: hit, Damage: damageValue}
	game.Hooks.OnCombatHitDamage(damage3)
	if damage3.Damage != damageValue {
		t.Fatalf("non-HitDamage damage read back as %v, want the original", damage3.Damage)
	}
	// A nil Damage runs no hooks (the emit's nil-Damage guard,
	// matching TS, which checks damage only).
	damageSeen = nil
	game.Hooks.OnCombatHitDamage = combatHitDamageAdapter(hub)
	game.Hooks.OnCombatHitDamage(&game.CombatHitDamageEvent{Attacker: &attacker.Mobile, Target: &target.Mobile, Hit: hit})
	if damageSeen != nil {
		t.Fatal("a nil damage must run no hit-damage hooks")
	}

	// Resolved: the shared hit passes through; a nil hit runs
	// no hooks (the emit's hit guard).
	resolved := &game.CombatHitResolvedEvent{Attacker: &attacker.Mobile, Target: &target.Mobile, Hit: hit}
	game.Hooks.OnCombatHitResolved(resolved)
	if resolvedSeen == nil || resolvedSeen.Hit != any(hit) {
		t.Fatalf("resolved event = %+v, want shared hit", resolvedSeen)
	}
	resolvedSeen = nil
	game.Hooks.OnCombatHitResolved(&game.CombatHitResolvedEvent{Attacker: &attacker.Mobile, Target: &target.Mobile})
	if resolvedSeen != nil {
		t.Fatal("a nil hit must run no resolved hooks")
	}

	// Attack distance: the hook's distance writes back, and
	// the floor applies.
	distance := &game.CombatAttackDistanceEvent{
		Attacker: &attacker.Mobile, Target: &target.Mobile,
		CombatType: game.CombatTypeRanged, ManualCast: true, Distance: 4,
	}
	game.Hooks.OnCombatAttackDistance(distance)
	if distanceSeen == nil || !distanceSeen.ManualCast || distanceSeen.CombatType != plugins.CombatTypeRanged {
		t.Fatalf("distance event = %+v, want manual-cast/RANGED", distanceSeen)
	}
	if distance.Distance != 7 {
		t.Fatalf("distance = %d, want 7", distance.Distance)
	}
	hub3 := combatTestHub(t, func(api *plugins.Api) {
		api.OnCombatAttackDistance(func(ev *plugins.CombatAttackDistanceEvent) { ev.Distance = 0 })
	})
	game.Hooks.OnCombatAttackDistance = combatAttackDistanceAdapter(hub3)
	floored := &game.CombatAttackDistanceEvent{Attacker: &attacker.Mobile, Target: &target.Mobile, Distance: 4}
	game.Hooks.OnCombatAttackDistance(floored)
	if floored.Distance != 1 {
		t.Fatalf("floored distance = %d, want 1", floored.Distance)
	}

	// Nil events are no-ops.
	game.Hooks.OnCombatHitPrepare(nil)
	game.Hooks.OnCombatHitRoll(nil)
	game.Hooks.OnCombatHitDamage(nil)
	game.Hooks.OnCombatHitResolved(nil)
	game.Hooks.OnCombatAttackDistance(nil)
}

func TestInstallCombatHooksNilWorld(t *testing.T) {
	saveCombatHooks(t)
	// A nil world still installs the hit/distance adapters;
	// the death adapters skip their emits (no victim can
	// resolve).
	called := false
	hub := combatTestHub(t, func(api *plugins.Api) {
		api.OnNpcDeath(func(*plugins.NpcDeathEvent) { called = true })
		api.OnPlayerDeath(func(*plugins.PlayerDeathEvent) { called = true })
		api.OnCombatHitPrepare(func(*plugins.CombatHitPrepareEvent) { called = true })
	})
	InstallCombatHooks(hub, nil)
	if game.Hooks.OnCombatHitPrepare == nil {
		t.Fatal("nil world must still install the hit adapters")
	}
	npc := game.NewNPC(200, game.Location{X: 3002, Y: 3000})
	game.Hooks.OnNpcDeath(&game.CombatDeathEvent{Victim: &npc.Mobile})
	game.Hooks.OnPlayerDeath(&game.CombatDeathEvent{Victim: &npc.Mobile})
	if called {
		t.Fatal("death adapters with a nil world must skip their emits")
	}
	attacker := game.NewPlayer("attacker", game.Location{X: 3000, Y: 3000})
	game.Hooks.OnCombatHitPrepare(&game.CombatHitPrepareEvent{Attacker: &attacker.Mobile, Target: &npc.Mobile})
	if !called {
		t.Fatal("hit adapters with a nil world must still fire")
	}
}

// fakeCombatSpell is a minimal game.CombatSpell for the
// autocast-clear test: only its presence as a non-nil spell
// matters, never its behavior.
type fakeCombatSpell struct{}

func (fakeCombatSpell) MaximumHit() int                                  { return 0 }
func (fakeCombatSpell) BaseExperience() int                              { return 0 }
func (fakeCombatSpell) SplashExperience() int                            { return 0 }
func (fakeCombatSpell) AttackSpeed() int                                 { return 0 }
func (fakeCombatSpell) HitDelay() int                                    { return 0 }
func (fakeCombatSpell) CanCast(*game.Player, bool) bool                  { return false }
func (fakeCombatSpell) CanCastOnTarget(*game.Player, *game.Mobile) bool  { return false }
func (fakeCombatSpell) StartCast(*game.Mobile, *game.Mobile)             {}
func (fakeCombatSpell) FinishCast(*game.Mobile, *game.Mobile, bool, int) {}
func (fakeCombatSpell) SpellRadius() int                                 { return 0 }

// SpellID is the contract-C1 spell id (Worker A adds it to the
// CombatSpell interface): 0 here, so this fake stays a valid
// CombatSpell once the interface grows, and the spell-disables
// tests can pass it as the spell argument.
func (fakeCombatSpell) SpellID() int { return 0 }

func TestCombatAttackAutocastClear(t *testing.T) {
	// The seam's autocast-conditional clear
	// (NPCOptionPacketListener.ts:62,
	// PlayerOptionPacketListener.ts:25): with an autocast
	// spell set, the single-cast spell is cleared before the
	// attack; without one, the single-cast spell survives.
	// The autocast spell itself is never cleared.
	target := game.NewNPC(200, game.Location{X: 3001, Y: 3000})

	autocasting := game.NewPlayer("caster", game.Location{X: 3000, Y: 3000})
	combat := autocasting.GetCombat()
	combat.SetAutoCastSpell(fakeCombatSpell{})
	combat.SetCastSpell(fakeCombatSpell{})
	combatAttack(autocasting, &target.Mobile)
	if combat.CastSpell() != nil {
		t.Fatal("an autocasting attack must clear the single-cast spell")
	}
	if combat.AutoCastSpell() == nil {
		t.Fatal("the attack must not clear the autocast spell")
	}
	if combat.Target() != &target.Mobile {
		t.Fatal("the attack must still target the Mobile")
	}

	manual := game.NewPlayer("manual", game.Location{X: 3000, Y: 3000})
	combat = manual.GetCombat()
	combat.SetCastSpell(fakeCombatSpell{})
	combatAttack(manual, &target.Mobile)
	if combat.CastSpell() == nil {
		t.Fatal("without autocast, the single-cast spell must survive the attack")
	}

	// A nil attacker is a defensive no-op.
	combatAttack(nil, &target.Mobile)
}

func TestCombatMethodProviderRegistryAdapter(t *testing.T) {
	// The adapter forwards to the game registry without
	// panicking on the degenerate provide shapes. The game
	// registry itself is unexported, so the success path (a
	// real method reaching CombatFactory) is Worker 2's
	// territory plus the coordinator's smoke test; here the
	// adapter's nil/assertion branches are pinned. The ids
	// are distinctive so the appended entries can never match
	// a real NPC in this package's tests.
	reg := combatMethodProviderRegistry{}
	var _ plugins.NpcCombatMethodProviderRegistry = reg
	reg.RegisterProvider("Probe", []int{424242}, nil)
	reg.RegisterProvider("Probe", []int{424243}, func(*game.NPC) any { return nil })
	reg.RegisterProvider("Probe", []int{424244}, func(*game.NPC) any { return "not-a-method" })
}

// TestInstallCombatSources installs the world-backed combat
// seams: the combat_hits world phase, and (with no areas,
// regions, or wilderness on a bare world) no panics from the
// nil-degenerate sources. The seam sources themselves are
// behavior-verified by the live smoke test; this pins the
// hook installation and the nil-world no-op, and restores
// the global seams so later tests see the unset defaults.
func TestInstallCombatSources(t *testing.T) {
	// A nil world installs nothing and must not panic.
	InstallCombatSources(nil, nil)

	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	InstallCombatSources(world, nil)
	t.Cleanup(func() {
		game.SetCombatCycleSource(nil)
		game.SetCombatClippingSource(nil)
		game.SetCombatMultiSource(nil)
		game.SetCombatWildernessSource(nil)
		game.SetNpcRespawnScheduler(nil)
		game.SetSpecialRestoreTasks(nil)
		game.SetAggressionCombatBridge(nil)
	})
	if world.Hooks.CombatHits == nil {
		t.Fatal("InstallCombatSources must install the combat_hits world phase")
	}
	// Idempotent re-install: no panic, hook still set.
	InstallCombatSources(world, nil)
	if world.Hooks.CombatHits == nil {
		t.Fatal("re-install must keep the combat_hits world phase")
	}
}

// TestInstallSequenceAnimationsRealCache proves the stall
// source install end to end with real cache data: the
// scanner built over the rev-242 seed cache measures seq
// 422 (unarmed punch, 38 client cycles) as a 2-tick stall,
// and an armed swing playing it through the installed
// source stalls exactly that long. It also proves the
// delayed-animation submitter runs on the world's
// TaskManager. It skips when the seed cache is absent.
func TestInstallSequenceAnimationsRealCache(t *testing.T) {
	const seedCache = "/home/hatch/smoke27/seed-root/caches/osrs-242_2026-11-03"
	store, err := cache.OpenStore(seedCache)
	if err != nil {
		t.Skipf("seed cache absent (%v)", err)
	}
	defer store.Close()
	defs, err := cache.NewCacheDefinitionsFromStore(cache.CacheInfo{
		Name: "osrs-242_2026-11-03", Game: cache.GameOldschool,
		Environment: "live", Revision: 242, Timestamp: "2026-11-03T00:00:00Z",
	}, store)
	if err != nil {
		t.Fatal(err)
	}
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	InstallSequenceAnimations(defs, world)
	t.Cleanup(func() {
		game.SetSequenceStallSource(nil)
		game.SetSequenceForcedPrioritySource(nil)
		game.SetSequencePrioritySource(nil)
		game.SetDelayedAnimationSubmit(nil)
		game.SetCombatCycleSource(nil)
	})
	game.SetCombatCycleSource(world.ProcessCycle)
	p := game.NewPlayer("alice", game.Location{X: 3000, Y: 3000})
	p.GetCombat() // bind identity
	cycle := world.ProcessCycle()
	p.GetCombat().ExpectAttackAnimation()
	p.PerformAnimation(422, 0)
	if got := p.GetCombat().GetIncomingHitCycle(); got != cycle+2 {
		t.Errorf("incoming-hit cycle = %d, want %d (real seq 422 stalls 2 ticks)", got, cycle+2)
	}
	// The delayed-animation submitter runs on the world's
	// TaskManager: a 1-tick delayed animation plays after
	// one task pass. (The swing is cleared first: its
	// forced priority 6 would otherwise refuse the
	// delayed 999, forced 5, under the equal-priority
	// rule — the same arbitration as in the live game.)
	p.ResetUpdating()
	p.DelayedAnimation(999, 0, 1)
	world.Tasks().Process(cycle + 1)
	if p.Animation() == nil || p.Animation().ID != 999 {
		t.Errorf("delayed Animation = %+v, want id 999 after one task tick", p.Animation())
	}
	// A nil facade installs the no-cache scanner (every id
	// reads the defaults) without panicking.
	InstallSequenceAnimations(nil, world)
	p2 := game.NewPlayer("bob", game.Location{X: 3000, Y: 3000})
	p2.GetCombat()
	p2.GetCombat().ExpectAttackAnimation()
	p2.PerformAnimation(422, 0)
	if got := p2.GetCombat().GetIncomingHitCycle(); got != -1 {
		t.Errorf("no-cache incoming-hit cycle = %d, want -1 (no stall without a cache)", got)
	}
}

func TestInstallCombatHooksSpellSeams(t *testing.T) {
	saveCombatHooks(t)
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	holdings := map[int]int{556: 3}
	hub := combatTestHub(t, func(api *plugins.Api) {
		api.OnSpellDisabled(func(ev *plugins.SpellDisabledEvent) {
			if ev.SpellID == 1152 {
				ev.Disabled = plugins.BoolPtr(true)
			}
		})
		api.OnSpellRuneBypass(func(ev *plugins.SpellRuneBypassEvent) {
			if ev.RuneID == 556 {
				ev.Bypass = plugins.BoolPtr(true)
			}
		})
		api.RegisterSpellRuneSource(testSpellRuneSource{holdings: holdings})
	})
	InstallCombatHooks(hub, world)

	player := game.NewPlayer("caster", game.Location{X: 3000, Y: 3000})
	// SpellDisabled: the 1152 hook disables, anything else passes.
	if !game.Hooks.SpellDisabled(player, game.SpellbookNormal, 1152, fakeCombatSpell{}) {
		t.Fatal("SpellDisabled(1152) = false, want true")
	}
	if game.Hooks.SpellDisabled(player, game.SpellbookNormal, 1154, fakeCombatSpell{}) {
		t.Fatal("SpellDisabled(1154) = true, want false")
	}
	// SpellRuneBypass: the air-rune hook bypasses, the
	// whole-cost query (rune 0) does not.
	if !game.Hooks.SpellRuneBypass(player, game.SpellbookNormal, 1152, 556) {
		t.Fatal("SpellRuneBypass(556) = false, want true")
	}
	if game.Hooks.SpellRuneBypass(player, game.SpellbookNormal, 1152, 0) {
		t.Fatal("SpellRuneBypass(0) = true, want false")
	}
	// Rune source: the pouch holds 3 air runes.
	items := []game.Item{game.NewItem(556, 3)}
	if !game.Hooks.CheckSpellRuneSource(player, items) {
		t.Fatal("CheckSpellRuneSource = false, want true")
	}
	game.Hooks.ConsumeSpellRuneSource(player, items)
	if holdings[556] != 0 {
		t.Fatalf("pouch holdings = %v, want 0 air runes after consumption", holdings)
	}
	if game.Hooks.CheckSpellRuneSource(player, items) {
		t.Fatal("CheckSpellRuneSource after consumption = true, want false")
	}
}

// testSpellRuneSource is a minimal plugins.SpellRuneSource for
// the seam test: holdings maps item id -> amount held.
type testSpellRuneSource struct {
	holdings map[int]int
}

func (s testSpellRuneSource) Check(player *game.Player, items []game.Item) bool {
	for _, item := range items {
		if s.holdings[item.ID()] < item.Amount() {
			return false
		}
	}
	return true
}

func (s testSpellRuneSource) Consume(player *game.Player, items []game.Item) {
	for _, item := range items {
		s.holdings[item.ID()] -= item.Amount()
	}
}

func TestInstallCombatHooksSpellSeamsNilHub(t *testing.T) {
	saveCombatHooks(t)
	world := game.NewWorld(game.Location{X: 3000, Y: 3000})
	game.Hooks = game.CombatHooks{}
	InstallCombatHooks(nil, world)
	if game.Hooks.SpellDisabled != nil || game.Hooks.SpellRuneBypass != nil ||
		game.Hooks.CheckSpellRuneSource != nil || game.Hooks.ConsumeSpellRuneSource != nil {
		t.Fatal("nil hub must leave the spell seam fields unset")
	}
}

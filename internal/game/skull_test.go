package game

import (
	"testing"
)

// Phase 28 (Worker B): live skull state (skull.go) —
// Player.ts:1377-1401, 1550-1580 and
// CombatFactory.ts:1388-1420, 1473-1494 at bfd81e3.

func TestSkullDefaults(t *testing.T) {
	p, _ := combatTestPlayer(t, "skullless", 3000, 3000)
	if p.SkullTimer() != 0 || p.SkullType() != SkullTypeWhite || p.IsSkulled() {
		t.Fatalf("fresh player skull = timer %d type %v skulled %v, want 0/white/false",
			p.SkullTimer(), p.SkullType(), p.IsSkulled())
	}
	if got := p.HeadSkullIcon(); got != -1 {
		t.Fatalf("fresh player head skull icon = %d, want -1", got)
	}
	if got := p.SkullIconID(); got != 0 {
		t.Fatalf("fresh player skull icon id = %d, want 0 (white)", got)
	}
}

func TestSkullTicksConversion(t *testing.T) {
	if got := skullTicks(PVPSkullSeconds); got != 3000 {
		t.Fatalf("skullTicks(1800) = %d, want 3000", got)
	}
	if got := skullTicks(ItemSkullSeconds); got != 2000 {
		t.Fatalf("skullTicks(1200) = %d, want 2000", got)
	}
	if got := skullTicks(0); got != 0 {
		t.Fatalf("skullTicks(0) = %d, want 0", got)
	}
}

func TestApplySkullWhite(t *testing.T) {
	p, fs := combatTestPlayer(t, "skulled", 3000, 3000)
	p.ApplySkull(SkullTypeWhite, PVPSkullSeconds)
	if p.SkullTimer() != 3000 || p.SkullType() != SkullTypeWhite || !p.IsSkulled() {
		t.Fatalf("white skull = timer %d type %v skulled %v, want 3000/white/true",
			p.SkullTimer(), p.SkullType(), p.IsSkulled())
	}
	if got := p.HeadSkullIcon(); got != 0 {
		t.Fatalf("head skull icon = %d, want 0 (white)", got)
	}
	if len(fs.messages) != 1 || fs.messages[0] != "You've been skulled!" {
		t.Fatalf("skull messages = %v, want [\"You've been skulled!\"]", fs.messages)
	}
	if !p.Flags.Has(FlagAppearance) {
		t.Fatal("applying a skull must flag APPEARANCE")
	}
}

func TestApplySkullRed(t *testing.T) {
	p, fs := combatTestPlayer(t, "redskull", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillPrayer: 50})
	if !ActivatePrayer(p, PrayerProtectItem) {
		t.Fatal("could not activate Protect Item for the red-skull test")
	}
	if !PrayerActive(&p.Mobile, PrayerProtectItem) {
		t.Fatal("Protect Item should be active before the red skull")
	}
	p.ApplySkull(SkullTypeRed, PVPSkullSeconds)
	if p.SkullTimer() != 3000 || p.SkullType() != SkullTypeRed {
		t.Fatalf("red skull = timer %d type %v, want 3000/red", p.SkullTimer(), p.SkullType())
	}
	if got := p.HeadSkullIcon(); got != 1 {
		t.Fatalf("head skull icon = %d, want 1 (red)", got)
	}
	want := "@bla@You have received a @red@red skull@bla@! You can no longer use the Protect item prayer!"
	if len(fs.messages) != 1 || fs.messages[0] != want {
		t.Fatalf("red skull messages = %v, want [%q]", fs.messages, want)
	}
	if PrayerActive(&p.Mobile, PrayerProtectItem) {
		t.Fatal("a red skull must deactivate the Protect Item prayer")
	}
}

func TestExtendSkullKeepsColourAndMax(t *testing.T) {
	p, fs := combatTestPlayer(t, "extender", 3000, 3000)
	p.ApplySkull(SkullTypeWhite, PVPSkullSeconds)
	p.SetSkullTimer(100)
	fs.messages = nil
	p.Flags = Flags(0)
	p.ExtendSkull(PVPSkullSeconds)
	if p.SkullTimer() != 3000 || p.SkullType() != SkullTypeWhite {
		t.Fatalf("extended skull = timer %d type %v, want 3000/white", p.SkullTimer(), p.SkullType())
	}
	if len(fs.messages) != 0 {
		t.Fatalf("extending a skull must send no message, got %v", fs.messages)
	}
	if p.Flags.Has(FlagAppearance) {
		t.Fatal("extending a skull must not flag APPEARANCE")
	}
	// A shorter extension never shortens the timer.
	p.ExtendSkull(60)
	if p.SkullTimer() != 3000 {
		t.Fatalf("short extension shortened the timer to %d, want 3000", p.SkullTimer())
	}
}

func TestClearSkull(t *testing.T) {
	p, _ := combatTestPlayer(t, "clearer", 3000, 3000)
	p.ApplySkull(SkullTypeRed, PVPSkullSeconds)
	p.Flags = Flags(0)
	p.ClearSkull()
	if p.SkullTimer() != 0 || p.SkullType() != SkullTypeWhite || p.IsSkulled() {
		t.Fatalf("cleared skull = timer %d type %v skulled %v, want 0/white/false",
			p.SkullTimer(), p.SkullType(), p.IsSkulled())
	}
	if got := p.HeadSkullIcon(); got != -1 {
		t.Fatalf("cleared head skull icon = %d, want -1", got)
	}
	if !p.Flags.Has(FlagAppearance) {
		t.Fatal("clearing a skull must flag APPEARANCE so the icon drops")
	}
}

func TestSkullIconOverride(t *testing.T) {
	p, _ := combatTestPlayer(t, "override", 3000, 3000)
	icon := 2
	p.SetSkullIconOverride(&icon, true)
	if got := p.HeadSkullIcon(); got != 2 {
		t.Fatalf("always-override head icon = %d, want 2", got)
	}
	// Always-override shows even without a skull timer.
	if p.IsSkulled() {
		t.Fatal("override must not skull the player")
	}
	// A non-always override hides while unskulled, shows while skulled.
	p.SetSkullIconOverride(&icon, false)
	if got := p.HeadSkullIcon(); got != -1 {
		t.Fatalf("non-always override while unskulled = %d, want -1", got)
	}
	p.ApplySkull(SkullTypeWhite, PVPSkullSeconds)
	if got := p.HeadSkullIcon(); got != 2 {
		t.Fatalf("non-always override while skulled = %d, want 2", got)
	}
	if got := p.SkullIconID(); got != 2 {
		t.Fatalf("skull icon id = %d, want 2 (override)", got)
	}
	// Clearing the override falls back to the skull type's icon.
	p.SetSkullIconOverride(nil, false)
	if got := p.HeadSkullIcon(); got != 0 {
		t.Fatalf("head icon after clearing override = %d, want 0 (white)", got)
	}
	// A negative icon normalizes to nil (TS: non-integer or
	// negative id -> null).
	negative := -3
	p.SetSkullIconOverride(&negative, true)
	if got := p.HeadSkullIcon(); got != 0 {
		t.Fatalf("negative override head icon = %d, want 0 (white, override cleared)", got)
	}
	// No-change override does not re-flag APPEARANCE.
	p.Flags = Flags(0)
	p.SetSkullIconOverride(nil, false)
	if p.Flags.Has(FlagAppearance) {
		t.Fatal("an unchanged override must not flag APPEARANCE")
	}
}

func TestSkullTypeIconMapping(t *testing.T) {
	if SkullTypeWhite.IconID() != 0 || SkullTypeRed.IconID() != 1 {
		t.Fatalf("icon ids = %d/%d, want 0/1", SkullTypeWhite.IconID(), SkullTypeRed.IconID())
	}
	if SkullTypeFromIconID(1) != SkullTypeRed || SkullTypeFromIconID(0) != SkullTypeWhite ||
		SkullTypeFromIconID(7) != SkullTypeWhite {
		t.Fatal("SkullTypeFromIconID must map 1 -> red, anything else -> white")
	}
}

func TestProcessSkullTimerCountsDownAndExpires(t *testing.T) {
	p, _ := combatTestPlayer(t, "ticking", 3000, 3000)
	p.ApplySkull(SkullTypeWhite, PVPSkullSeconds)
	p.SetSkullTimer(3)
	p.Flags = Flags(0)
	p.ProcessSkullTimer()
	if p.SkullTimer() != 2 {
		t.Fatalf("timer after one tick = %d, want 2", p.SkullTimer())
	}
	if p.Flags.Has(FlagAppearance) {
		t.Fatal("a non-expiring tick must not flag APPEARANCE")
	}
	p.ProcessSkullTimer()
	p.ProcessSkullTimer()
	if p.SkullTimer() != 0 || p.IsSkulled() {
		t.Fatalf("timer after expiry = %d skulled %v, want 0/false", p.SkullTimer(), p.IsSkulled())
	}
	if !p.Flags.Has(FlagAppearance) {
		t.Fatal("the expiry tick must flag APPEARANCE")
	}
	if got := p.HeadSkullIcon(); got != -1 {
		t.Fatalf("head icon after expiry = %d, want -1", got)
	}
	// An unskulled player is a no-op.
	p.Flags = Flags(0)
	p.ProcessSkullTimer()
	if p.Flags.Has(FlagAppearance) {
		t.Fatal("an unskulled tick must not flag APPEARANCE")
	}
}

func TestProcessSkullTimerSkippedWhileDying(t *testing.T) {
	p, _ := combatTestPlayer(t, "dying", 3000, 3000)
	p.ApplySkull(SkullTypeWhite, PVPSkullSeconds)
	p.SetSkullTimer(5)
	combatantFor(&p.Mobile).SetDying(true)
	t.Cleanup(func() { combatantFor(&p.Mobile).SetDying(false) })
	p.ProcessSkullTimer()
	if p.SkullTimer() != 5 {
		t.Fatalf("dying timer = %d, want 5 (frozen)", p.SkullTimer())
	}
}

func TestProcessSkullTimerHoldsLastTickInPvpCombat(t *testing.T) {
	other, _ := combatTestPlayer(t, "other", 3001, 3000)
	npc := combatTestNPC(t, 1, 3002, 3000, nil)

	// Player target holds the last tick.
	p, _ := combatTestPlayer(t, "held-target", 3000, 3000)
	p.ApplySkull(SkullTypeWhite, PVPSkullSeconds)
	p.SetSkullTimer(1)
	p.GetCombat().target = &other.Mobile
	p.ProcessSkullTimer()
	if p.SkullTimer() != 1 {
		t.Fatalf("target-held timer = %d, want 1", p.SkullTimer())
	}

	// Player attacker holds the last tick.
	p.GetCombat().target = nil
	p.GetCombat().attacker = &other.Mobile
	p.ProcessSkullTimer()
	if p.SkullTimer() != 1 {
		t.Fatalf("attacker-held timer = %d, want 1", p.SkullTimer())
	}

	// An incoming hit from a player holds the last tick.
	p.GetCombat().attacker = nil
	queue := p.GetCombat().HitQueue()
	queue.AddPendingHit(&PendingHit{attacker: &other.Mobile, target: &p.Mobile}, 0)
	t.Cleanup(queue.Clear)
	p.ProcessSkullTimer()
	if p.SkullTimer() != 1 {
		t.Fatalf("incoming-hit-held timer = %d, want 1", p.SkullTimer())
	}
	queue.Clear()

	// The hold applies only to the last tick: at 2 ticks the
	// timer counts down even mid-fight.
	p.SetSkullTimer(2)
	p.GetCombat().target = &other.Mobile
	p.ProcessSkullTimer()
	if p.SkullTimer() != 1 {
		t.Fatalf("mid-fight timer at 2 = %d, want 1 (counts down)", p.SkullTimer())
	}

	// An NPC target is not PvP: the last tick expires.
	p.GetCombat().target = &npc.Mobile
	p.ProcessSkullTimer()
	if p.SkullTimer() != 0 {
		t.Fatalf("npc-target timer = %d, want 0 (no hold)", p.SkullTimer())
	}
}

// wireLiveSkullSeam installs the live skull system behind the
// Hooks seam, exactly as cmd/server InstallCombatHooks does.
func wireLiveSkullSeam() {
	Hooks.ApplySkull = func(player *Player, seconds int) {
		player.ApplySkull(SkullTypeWhite, seconds)
	}
	Hooks.IsSkulled = func(player *Player) bool { return player.IsSkulled() }
	Hooks.ExtendSkull = func(player *Player, seconds int) { player.ExtendSkull(seconds) }
}

func TestHandleSkullWithLiveSeam(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	wireLiveSkullSeam()
	SetCombatWildernessSource(func(loc Location) (bool, bool) { return true, false })
	t.Cleanup(func() { SetCombatWildernessSource(nil) })

	attacker, afs := combatTestPlayer(t, "attacker", 3000, 3000)
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	HandleSkull(attacker, target)
	if !attacker.IsSkulled() || attacker.SkullTimer() != 3000 || attacker.SkullType() != SkullTypeWhite {
		t.Fatalf("wilderness attack skull = timer %d type %v skulled %v, want 3000/white/true",
			attacker.SkullTimer(), attacker.SkullType(), attacker.IsSkulled())
	}
	if len(afs.messages) != 1 || afs.messages[0] != "You've been skulled!" {
		t.Fatalf("wilderness attack messages = %v, want the skulled line", afs.messages)
	}
	if !WasAttackedBy(target, attacker) {
		t.Fatal("the target must record the attack (retaliation without a skull)")
	}
	if got := target.HeadSkullIcon(); got != -1 {
		t.Fatalf("target head icon = %d, want -1 (attacked, not skulled)", got)
	}

	// Retaliation does not skull: the target was attacked by
	// the attacker, so WouldSkull is false and HandleSkull
	// leaves the target unskulled.
	targetFs := target.Session.(*fakeSession)
	HandleSkull(target, attacker)
	if target.IsSkulled() {
		t.Fatal("retaliation must not skull the target")
	}
	if len(targetFs.messages) != 0 {
		t.Fatalf("retaliation messages = %v, want none", targetFs.messages)
	}
}

func TestHandleSkullExtendsSkulledAttacker(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	wireLiveSkullSeam()
	SetCombatWildernessSource(func(loc Location) (bool, bool) { return true, false })
	t.Cleanup(func() { SetCombatWildernessSource(nil) })

	attacker, afs := combatTestPlayer(t, "attacker", 3000, 3000)
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	other, _ := combatTestPlayer(t, "other", 3002, 3000)
	attacker.ApplySkull(SkullTypeWhite, PVPSkullSeconds)
	attacker.SetSkullTimer(100)
	afs.messages = nil
	// A fresh victim: initiating extends the skull to full.
	HandleSkull(attacker, other)
	if attacker.SkullTimer() != 3000 {
		t.Fatalf("extended timer = %d, want 3000", attacker.SkullTimer())
	}
	if len(afs.messages) != 0 {
		t.Fatalf("extending must send no message, got %v", afs.messages)
	}
	// Retaliation against the player who attacked first does
	// NOT extend: the timer keeps its value.
	RecordAttackedBy(attacker, target)
	attacker.SetSkullTimer(100)
	HandleSkull(attacker, target)
	if attacker.SkullTimer() != 100 {
		t.Fatalf("retaliation-extended timer = %d, want 100 (no extension)", attacker.SkullTimer())
	}
}

func TestHandleSkullNoWildernessNoSkull(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	wireLiveSkullSeam()
	SetCombatWildernessSource(func(loc Location) (bool, bool) { return false, false })
	t.Cleanup(func() { SetCombatWildernessSource(nil) })

	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	HandleSkull(attacker, target)
	if attacker.IsSkulled() {
		t.Fatal("an attack outside the Wilderness must not skull")
	}
}

func TestProcessSlotRunsSkullTimer(t *testing.T) {
	p, _ := combatTestPlayer(t, "slot", 3000, 3000)
	p.ApplySkull(SkullTypeWhite, PVPSkullSeconds)
	p.SetSkullTimer(2)
	// Player.Process runs the skull step after area
	// processing, before run-energy restore (Player.ts:556).
	p.Process(1)
	if p.SkullTimer() != 1 {
		t.Fatalf("timer after one Process tick = %d, want 1", p.SkullTimer())
	}
	p.Process(2)
	if p.SkullTimer() != 0 || p.IsSkulled() {
		t.Fatalf("timer after two Process ticks = %d skulled %v, want 0/false",
			p.SkullTimer(), p.IsSkulled())
	}
}

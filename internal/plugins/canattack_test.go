package plugins

import (
	"testing"

	"tsps-go/internal/game"
)

// onCanAttack tests: the registration method, the allow-veto emit,
// and the registration-wrapper guards in combat.go, grounded in
// PluginManager.emitCanAttack (PluginManager.ts:1360-1377),
// the onCanAttack registration (PluginManager.ts:3581-3593), and
// PluginCanAttackEvent (PluginTypes.ts:315-321). Fixtures only:
// newTestHub and testPlayer come from api_test.go / hooks_test.go.

func TestOnCanAttackAllowVetoGate(t *testing.T) {
	h, _ := newTestHub()
	attacker := testPlayer("attacker")
	target := testPlayer("target")

	// No hooks: nil (TS returns null without building an event).
	if got := h.EmitCanAttack(&attacker.Mobile, &target.Mobile, game.MeleeMethod); got != nil {
		t.Fatalf("no hooks = %v, want nil", got)
	}

	// A deny short-circuits: the hook after it never runs.
	secondRan := false
	newApi(h, "Veto").OnCanAttack(func(ev *CanAttackEvent) { ev.Allow = BoolPtr(false) })
	newApi(h, "After").OnCanAttack(func(ev *CanAttackEvent) { secondRan = true })
	if got := h.EmitCanAttack(&attacker.Mobile, &target.Mobile, game.MeleeMethod); got == nil || *got {
		t.Fatalf("veto = %v, want false", got)
	}
	if secondRan {
		t.Fatal("hook after a deny must not run")
	}

	// An allow sticks across a silent hook, and a later deny
	// still vetoes after an allow.
	h2, _ := newTestHub()
	newApi(h2, "Allow").OnCanAttack(func(ev *CanAttackEvent) { ev.Allow = BoolPtr(true) })
	newApi(h2, "Silent").OnCanAttack(func(ev *CanAttackEvent) {})
	if got := h2.EmitCanAttack(&attacker.Mobile, &target.Mobile, game.MeleeMethod); got == nil || !*got {
		t.Fatalf("allow + silence = %v, want true", got)
	}
	h3, _ := newTestHub()
	newApi(h3, "Allow").OnCanAttack(func(ev *CanAttackEvent) { ev.Allow = BoolPtr(true) })
	newApi(h3, "Veto").OnCanAttack(func(ev *CanAttackEvent) { ev.Allow = BoolPtr(false) })
	if got := h3.EmitCanAttack(&attacker.Mobile, &target.Mobile, game.MeleeMethod); got == nil || *got {
		t.Fatalf("allow then veto = %v, want false", got)
	}

	// Silence from every hook is undecided.
	h4, _ := newTestHub()
	newApi(h4, "Silent").OnCanAttack(func(ev *CanAttackEvent) {})
	if got := h4.EmitCanAttack(&attacker.Mobile, &target.Mobile, game.MeleeMethod); got != nil {
		t.Fatalf("all silent = %v, want nil", got)
	}

	// A nil hub yields nil without panicking.
	var nilHub *Hub
	if got := nilHub.EmitCanAttack(&attacker.Mobile, &target.Mobile, game.MeleeMethod); got != nil {
		t.Fatalf("nil hub = %v, want nil", got)
	}
}

func TestOnCanAttackAllowResetPerHook(t *testing.T) {
	h, _ := newTestHub()
	attacker := testPlayer("attacker")
	target := testPlayer("target")

	// TS resets event.allow to null before EACH hook
	// (emitCanAttack): the second hook must see a nil Allow
	// even though the first hook allowed.
	seen := []*bool{nil}
	newApi(h, "First").OnCanAttack(func(ev *CanAttackEvent) { ev.Allow = BoolPtr(true) })
	newApi(h, "Second").OnCanAttack(func(ev *CanAttackEvent) { seen[0] = ev.Allow })
	if got := h.EmitCanAttack(&attacker.Mobile, &target.Mobile, game.MeleeMethod); got == nil || !*got {
		t.Fatalf("emit = %v, want true", got)
	}
	if seen[0] != nil {
		t.Fatalf("second hook saw Allow = %v, want nil (reset before each hook)", *seen[0])
	}
}

func TestOnCanAttackWrapperGuardsAndMethod(t *testing.T) {
	h, _ := newTestHub()
	attacker := testPlayer("attacker")
	target := testPlayer("target")

	// The registration wrapper skips fighter-less events, and a
	// nil method passes through (TS undefined).
	calls := 0
	var seenMethod game.CombatMethod = game.RangedMethod
	newApi(h, "Watch").OnCanAttack(func(ev *CanAttackEvent) {
		calls++
		seenMethod = ev.Method
	})
	if got := h.EmitCanAttack(nil, &target.Mobile, game.MeleeMethod); got != nil {
		t.Fatalf("attacker-less emit = %v, want nil", got)
	}
	if got := h.EmitCanAttack(&attacker.Mobile, nil, game.MeleeMethod); got != nil {
		t.Fatalf("target-less emit = %v, want nil", got)
	}
	if calls != 0 {
		t.Fatalf("calls after fighter-less emits = %d, want 0", calls)
	}
	if got := h.EmitCanAttack(&attacker.Mobile, &target.Mobile, game.MeleeMethod); got != nil {
		t.Fatalf("silent emit = %v, want nil", got)
	}
	if calls != 1 || seenMethod != game.MeleeMethod {
		t.Fatalf("calls = %d method = %v, want 1 call with MeleeMethod", calls, seenMethod)
	}
	if got := h.EmitCanAttack(&attacker.Mobile, &target.Mobile, nil); got != nil {
		t.Fatalf("nil-method emit = %v, want nil", got)
	}
	if seenMethod != nil {
		t.Fatalf("nil-method call saw method %v, want nil", seenMethod)
	}
}

func TestOnCanAttackNilHandlerIgnored(t *testing.T) {
	h, _ := newTestHub()
	newApi(h, "Nil").OnCanAttack(nil)
	attacker := testPlayer("attacker")
	target := testPlayer("target")
	if got := h.EmitCanAttack(&attacker.Mobile, &target.Mobile, game.MeleeMethod); got != nil {
		t.Fatalf("nil-handler emit = %v, want nil", got)
	}
}

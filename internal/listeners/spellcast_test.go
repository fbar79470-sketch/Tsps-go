package listeners

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Spell-cast emit + spell-on-object routing tests (phase 45):
// the onSpellCast emit site in routeWidgetAction
// (NetworkBuilder.ts:1060, 1804-1812) and the opcode-2 route to
// routeSpellOnObject (UseItemPacketListener.spellOnObject,
// UseItemPacketListener.ts:202-241). Both helpers existed
// unwired before this phase — emitSpellCast had no call site
// and the NativeOpLocT dispatch dropped the packet — and the
// live legs caught it (smoke45 leg (a)).

func routeSpellOnObjectOutcome(p *game.Player, msg protocol.SpellOnObject, cycle int64, deps Deps) Outcome {
	drop, acted, noAction := objectOutcomeFuncs()
	return routeSpellOnObject(p, msg, cycle, deps, drop, acted, noAction)
}

func spellOnObjectClick() protocol.SpellOnObject {
	return protocol.SpellOnObject{
		ObjectID: 100, X: 3090, Y: 3524,
		SpellWidget: 14286868, SpellChild: 0, SpellItemID: 0,
	}
}

func TestSpellOnObjectRoute(t *testing.T) {
	p := testPlayer()
	p.SetRegistered(true)
	msg := spellOnObjectClick()
	if got := routeSpellOnObjectOutcome(p, msg, 1, Deps{}); got.Reason != ReasonUseItem {
		t.Fatalf("unwired = %+v, want ReasonUseItem", got)
	}
	if got := routeSpellOnObjectOutcome(nil, msg, 1, Deps{}); got.Reason != ReasonNoPlayer {
		t.Fatalf("nil player = %+v, want ReasonNoPlayer", got)
	}
	// Wired object lookup: the walk to the object starts, so
	// the route acts (the emit itself fires on arrival).
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	deps, _, _ := objectTestDeps(entry, ObjectHooks{})
	if got := routeSpellOnObjectOutcome(p, msg, 1, deps); !got.Implemented {
		t.Fatalf("wired = %+v, want Implemented (walk to object started)", got)
	}
}

func routeWidgetOutcome(p *game.Player, msg protocol.WidgetAction, deps Deps) Outcome {
	drop, acted, noAction := objectOutcomeFuncs()
	return routeWidgetAction(p, msg, deps, drop, acted, noAction)
}

func spellWidgetClick() protocol.WidgetAction {
	return protocol.WidgetAction{
		WidgetID: 14286859, GroupID: 1151, ChildID: 0,
		ButtonNum: 1, HasButtonNum: true, Slot: -1, ItemID: -1,
	}
}

func spellCastDeps(spellName string, claim bool, seen *SpellCastEvent) Deps {
	deps := Deps{}
	deps.SpellHooks.SpellName = func(widgetID, itemID int) (string, bool) {
		return spellName, true
	}
	deps.SpellHooks.IsSpellWidgetGroup = func(groupID int) bool {
		return groupID == 1151
	}
	deps.SpellHooks.EmitSpellCast = func(ev *SpellCastEvent) bool {
		if claim {
			ev.Handled = true
			if seen != nil {
				*seen = *ev
			}
			return true
		}
		if seen != nil {
			*seen = *ev
		}
		return false
	}
	return deps
}

func TestWidgetActionSpellCastEmit(t *testing.T) {
	t.Run("hook claim consumes the click", func(t *testing.T) {
		p, _ := spellTestPlayer(43)
		p.SetRegistered(true)
		var seen SpellCastEvent
		deps := spellCastDeps("Wind Strike", true, &seen)
		if got := routeWidgetOutcome(p, spellWidgetClick(), deps); !got.Implemented {
			t.Fatalf("claimed = %+v, want Implemented (hook claimed the cast)", got)
		}
		if seen.SpellName != "Wind Strike" || seen.SpellWidget != 14286859 || seen.OpID != 1 {
			t.Fatalf("event = %+v, want Wind Strike / widget 14286859 / op 1", seen)
		}
		if !seen.Handled {
			t.Fatal("event.Handled = false, want true (hook claimed it)")
		}
	})

	t.Run("unclaimed falls through to the self-cast route", func(t *testing.T) {
		p, session := spellTestPlayer(43)
		p.SetRegistered(true)
		deps := spellCastDeps("Charge", false, nil)
		if got := routeWidgetOutcome(p, spellWidgetClick(), deps); !got.Implemented {
			t.Fatalf("unclaimed Charge = %+v, want Implemented (Charge self-cast ran)", got)
		}
		// Charge refuses before the runes: no Mage Arena cape
		// answerer, no spell-unlock answerer.
		want := "You need to complete the Mage Arena to cast this spell."
		found := false
		for _, m := range session.gameMessages {
			if m == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("messages = %v, want the Mage Arena refusal", session.gameMessages)
		}
	})

	t.Run("unclaimed non-self-cast falls through to no action", func(t *testing.T) {
		p, _ := spellTestPlayer(43)
		p.SetRegistered(true)
		deps := spellCastDeps("Wind Strike", false, nil)
		if got := routeWidgetOutcome(p, spellWidgetClick(), deps); got.Reason != ReasonNoAction {
			t.Fatalf("unclaimed Wind Strike = %+v, want ReasonNoAction", got)
		}
	})

	t.Run("unresolved spell name skips the emit", func(t *testing.T) {
		p, _ := spellTestPlayer(43)
		p.SetRegistered(true)
		deps := Deps{}
		deps.SpellHooks.SpellName = func(widgetID, itemID int) (string, bool) {
			return "", false
		}
		deps.SpellHooks.IsSpellWidgetGroup = func(groupID int) bool { return true }
		deps.SpellHooks.EmitSpellCast = func(ev *SpellCastEvent) bool {
			t.Error("EmitSpellCast called with no resolvable spell name")
			return true
		}
		if got := routeWidgetOutcome(p, spellWidgetClick(), deps); got.Reason != ReasonNoAction {
			t.Fatalf("unresolved = %+v, want ReasonNoAction", got)
		}
	})
}

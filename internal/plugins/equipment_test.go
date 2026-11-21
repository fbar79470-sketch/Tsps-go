package plugins

import (
	"testing"

	"tsps-go/internal/game"
)

// recordingModifierRegistry records the modifiers pushed onto it,
// in push order per family.
type recordingModifierRegistry struct {
	restore []game.RunEnergyRestoreModifier
	drain   []game.RunEnergyDrainModifier
}

func (r *recordingModifierRegistry) RegisterRestoreModifier(modifier game.RunEnergyRestoreModifier) {
	r.restore = append(r.restore, modifier)
}

func (r *recordingModifierRegistry) RegisterDrainModifier(modifier game.RunEnergyDrainModifier) {
	r.drain = append(r.drain, modifier)
}

func TestRegisterRunEnergyModifiersQueueUntilInstalled(t *testing.T) {
	h, _ := newTestHub()
	restore := func(_ *game.Player, units float64) float64 { return units * 2 }
	drain := func(_ *game.Player, amount float64) float64 { return amount / 2 }
	// Registration before installation queues: nothing is
	// pushed, nothing is lost.
	newApi(h, "GracefulOutfit").RegisterRunEnergyRestoreModifier(restore)
	newApi(h, "Endurance").RegisterRunEnergyDrainModifier(drain)
	if got := h.RunEnergyModifierRegistry(); got != nil {
		t.Fatalf("registry before install = %v, want nil", got)
	}
	reg := &recordingModifierRegistry{}
	h.SetRunEnergyModifierRegistry(reg)
	if len(reg.restore) != 1 || len(reg.drain) != 1 {
		t.Fatalf("flushed = %d restore, %d drain, want 1/1", len(reg.restore), len(reg.drain))
	}
	if h.RunEnergyModifierRegistry() != RunEnergyModifierRegistry(reg) {
		t.Fatal("RunEnergyModifierRegistry must return the installed registry")
	}
	// Registration after installation pushes directly.
	restore2 := func(_ *game.Player, units float64) float64 { return units + 1 }
	drain2 := func(_ *game.Player, amount float64) float64 { return amount - 1 }
	newApi(h, "Late").RegisterRunEnergyRestoreModifier(restore2)
	newApi(h, "Late").RegisterRunEnergyDrainModifier(drain2)
	if len(reg.restore) != 2 || len(reg.drain) != 2 {
		t.Fatalf("after direct registration = %d restore, %d drain, want 2/2", len(reg.restore), len(reg.drain))
	}
}

func TestRegisterRunEnergyModifiersPreserveOrder(t *testing.T) {
	h, _ := newTestHub()
	// Three modifiers per family, registered interleaved: each
	// family's flush order is its own registration order.
	var order []string
	restore := func(tag string) game.RunEnergyRestoreModifier {
		return func(_ *game.Player, units float64) float64 {
			order = append(order, tag)
			return units
		}
	}
	newApi(h, "P").RegisterRunEnergyRestoreModifier(restore("first"))
	newApi(h, "P").RegisterRunEnergyDrainModifier(func(_ *game.Player, a float64) float64 { return a })
	newApi(h, "P").RegisterRunEnergyRestoreModifier(restore("second"))
	newApi(h, "P").RegisterRunEnergyRestoreModifier(restore("third"))
	reg := &recordingModifierRegistry{}
	h.SetRunEnergyModifierRegistry(reg)
	if len(reg.restore) != 3 {
		t.Fatalf("flushed restore = %d, want 3", len(reg.restore))
	}
	// Apply through the recorded order: the tags must come out
	// in registration order.
	effects := game.NewEquipmentEffects()
	for _, m := range reg.restore {
		effects.RegisterRestoreModifier(m)
	}
	effects.ApplyRestoreModifiers(nil, 10)
	if len(order) != 3 || order[0] != "first" || order[1] != "second" || order[2] != "third" {
		t.Fatalf("apply order = %v, want [first second third]", order)
	}
}

func TestRegisterRunEnergyModifiersNilHandling(t *testing.T) {
	h, _ := newTestHub()
	// Nil modifiers are ignored, never queued: the flush sees
	// nothing.
	newApi(h, "P").RegisterRunEnergyRestoreModifier(nil)
	newApi(h, "P").RegisterRunEnergyDrainModifier(nil)
	reg := &recordingModifierRegistry{}
	h.SetRunEnergyModifierRegistry(reg)
	if len(reg.restore) != 0 || len(reg.drain) != 0 {
		t.Fatalf("flushed nils = %d restore, %d drain, want 0/0", len(reg.restore), len(reg.drain))
	}
	// Nil modifiers after installation are ignored too.
	newApi(h, "P").RegisterRunEnergyRestoreModifier(nil)
	newApi(h, "P").RegisterRunEnergyDrainModifier(nil)
	if len(reg.restore) != 0 || len(reg.drain) != 0 {
		t.Fatalf("direct nils = %d restore, %d drain, want 0/0", len(reg.restore), len(reg.drain))
	}
	// A nil Api, or an Api with a nil hub, is a no-op.
	var nilApi *Api
	nilApi.RegisterRunEnergyRestoreModifier(func(_ *game.Player, u float64) float64 { return u })
	nilApi.RegisterRunEnergyDrainModifier(func(_ *game.Player, u float64) float64 { return u })
	newApi(nil, "NoHub").RegisterRunEnergyRestoreModifier(func(_ *game.Player, u float64) float64 { return u })
	newApi(nil, "NoHub").RegisterRunEnergyDrainModifier(func(_ *game.Player, u float64) float64 { return u })
	// A nil Hub tolerates the install seam.
	var nilHub *Hub
	nilHub.SetRunEnergyModifierRegistry(reg)
	if got := nilHub.RunEnergyModifierRegistry(); got != nil {
		t.Fatalf("nil hub registry = %v, want nil", got)
	}
}

func TestSetRunEnergyModifierRegistryNilKeepsQueue(t *testing.T) {
	h, _ := newTestHub()
	newApi(h, "P").RegisterRunEnergyRestoreModifier(func(_ *game.Player, u float64) float64 { return u })
	// Installing nil clears the registry but keeps the queue.
	h.SetRunEnergyModifierRegistry(nil)
	if got := h.RunEnergyModifierRegistry(); got != nil {
		t.Fatalf("registry after nil install = %v, want nil", got)
	}
	// A later install flushes the queued modifier.
	reg := &recordingModifierRegistry{}
	h.SetRunEnergyModifierRegistry(reg)
	if len(reg.restore) != 1 {
		t.Fatalf("flushed after nil install = %d, want 1", len(reg.restore))
	}
	// Reinstalling does not re-flush: the queue was drained.
	reg2 := &recordingModifierRegistry{}
	h.SetRunEnergyModifierRegistry(reg2)
	if len(reg2.restore) != 0 {
		t.Fatalf("second install flushed %d, want 0", len(reg2.restore))
	}
}

func TestRegisterRunEnergyModifiersHaveNoLoadGate(t *testing.T) {
	// The TS closures have no phase check (PluginManager.ts:
	// 4761-4768): registration after Load seals the hub still
	// queues/pushes, like RegisterArea. Load a hub with no
	// plugins, then register through a fresh Api.
	h, _ := newTestHub()
	h.Load()
	newApi(h, "Late").RegisterRunEnergyRestoreModifier(func(_ *game.Player, u float64) float64 { return u })
	reg := &recordingModifierRegistry{}
	h.SetRunEnergyModifierRegistry(reg)
	if len(reg.restore) != 1 {
		t.Fatalf("post-load registration flushed %d, want 1", len(reg.restore))
	}
}

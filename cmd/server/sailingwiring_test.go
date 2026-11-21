package main

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/persist"
	"tsps-go/internal/plugins"
	"tsps-go/internal/protocol"
	"tsps-go/internal/session"
)

// Sailing-wiring tests: InstallSailing (hook install, runtime
// publication, nil degradations), installWorldEntityListenerDeps
// (gate/emit/helm seams), applyEntrySailing /
// snapshotSailingJSON (owned save state), and the
// WorldEntityPackets flush adapter (framing, XTEA, deck locs).
// Fixtures only. The teleport listener InstallSailing
// registers is inert (nothing in this package fires teleport
// listeners).

func sailingTestSpec() game.BoatSpec {
	return game.BoatSpec{
		Type:           "sloop",
		ConfigID:       7,
		TemplateChunkX: 100,
		TemplateChunkY: 100,
		SizeX:          8,
		SizeZ:          16,
		Hull:           game.BoatHull{OffsetX: -64, OffsetY: -256, Width: 128, Length: 512},
		DeckCentreX:    512,
		DeckCentreY:    1024,
		DeckLevel:      1,
		WalkableDeck: []game.DeckTile{
			{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1},
			{X: 4, Y: 8}, {X: 5, Y: 8}, {X: 4, Y: 9},
		},
		BoardingTile: game.DeckTile{X: 4, Y: 8},
		Locs: []game.BoatDeckLoc{
			{ID: 100, X: 0, Y: 0, Level: 1, Shape: 10, Rotation: 0, Blocks: true},
		},
	}
}

func sailingTestPlacement() game.BoatPlacement {
	return game.BoatPlacement{FineX: 3000 * game.FineUnitsPerTile, FineY: 3000 * game.FineUnitsPerTile, Level: 0, Angle: 0}
}

func sailingTestHub(t *testing.T, register func(*plugins.Api)) *plugins.Hub {
	t.Helper()
	hub := plugins.NewHub([]plugins.Plugin{{
		Name:     "SailingProbe",
		Register: register,
	}}, plugins.HubOptions{Logger: log.New(io.Discard, "", 0)})
	hub.Load()
	return hub
}

func TestInstallSailingPublishesAndHooks(t *testing.T) {
	world := game.NewWorld(game.Location{X: 3089, Y: 3524, Z: 0})
	hub := sailingTestHub(t, nil)
	sailing := InstallSailing(hub, world)
	if sailing == nil {
		t.Fatal("InstallSailing returned nil")
	}
	if hub.Sailing() != sailing {
		t.Error("the Sailing runtime must be installed on the hub")
	}
	manager := world.BoatManager()
	if manager == nil {
		t.Fatal("the BoatManager must be installed on the world")
	}
	if world.Hooks.Boats == nil {
		t.Fatal("the move_boats hook must be installed")
	}
	// The hook ticks the manager: an after-tick listener
	// fires once per hook call. (The boat itself cannot
	// sail: the test world's regions hold no water.)
	boat := manager.Spawn(1, sailingTestSpec(), sailingTestPlacement())
	boat.MoveMode = game.BoatMoveModeFull
	ticks := 0
	manager.OnAfterTick(func() { ticks++ })
	world.Hooks.Boats(1)
	if ticks != 1 {
		t.Errorf("after-tick listeners fired %d times, want 1", ticks)
	}
	// Re-installing is idempotent for the teleport hook:
	// Initialize's guard means no duplicate registration.
	again := InstallSailing(hub, world)
	if again == sailing {
		t.Error("a second install creates a fresh runtime")
	}
}

func TestInstallSailingNilWorldAndHub(t *testing.T) {
	sailing := InstallSailing(nil, nil)
	if sailing == nil {
		t.Fatal("InstallSailing(nil, nil) must still create the runtime")
	}
	if sailing.BoatManager() == nil {
		t.Error("nil inputs must still create the boat manager")
	}
}

func TestInstallWorldEntityListenerDeps(t *testing.T) {
	world := game.NewWorld(game.Location{X: 3089, Y: 3524, Z: 0})
	var emitted *plugins.WorldEntityInteractionEvent
	hub := sailingTestHub(t, func(api *plugins.Api) {
		api.OnWorldEntityInteraction(func(ev *plugins.WorldEntityInteractionEvent) {
			emitted = ev
			ev.Handled = true
		})
	})
	InstallSailing(hub, world)
	manager := world.BoatManager()
	boat := manager.Spawn(1, sailingTestSpec(), sailingTestPlacement())
	var deps listeners.Deps
	installWorldEntityListenerDeps(&deps, hub, manager)
	if deps.WorldEntityInteractable == nil || deps.WorldEntityEmit == nil || deps.SetHelmHeading == nil {
		t.Fatal("all three seams must be installed")
	}
	viewer := game.NewPlayer("viewer", game.Location{X: 3000, Y: 3000, Z: 0})
	viewer.SetIndex(1)
	if !deps.WorldEntityInteractable(viewer, boat.EntityIndex) {
		t.Error("the gate must pass for a boat in view")
	}
	if deps.WorldEntityInteractable(viewer, 3999) {
		t.Error("the gate must fail for a missing boat")
	}
	handled := deps.WorldEntityEmit(&listeners.WorldEntityInteractionEvent{Player: viewer, EntityIndex: boat.EntityIndex, ClickType: 2})
	if !handled || emitted == nil || emitted.ClickType != 2 || emitted.EntityIndex != boat.EntityIndex {
		t.Fatalf("handled=%v emitted=%+v", handled, emitted)
	}
	// The helm seam reaches the manager: an aboard helm
	// holder's heading is set. (SetHelmHeading is a no-op
	// for a player who does not steer; assert no panic and
	// no state change for a stranger.)
	deps.SetHelmHeading(viewer, 8)
	if boat.Heading != 0 {
		t.Errorf("a stranger's heading must not steer the boat, got %d", boat.Heading)
	}
}

func TestInstallWorldEntityListenerDepsNil(t *testing.T) {
	var deps listeners.Deps
	installWorldEntityListenerDeps(&deps, nil, nil)
	if deps.WorldEntityInteractable != nil || deps.WorldEntityEmit != nil || deps.SetHelmHeading != nil {
		t.Error("nil hub/manager must leave every seam nil")
	}
}

func TestApplyEntrySailing(t *testing.T) {
	p := game.NewPlayer("sailor", game.Location{X: 3000, Y: 3000, Z: 0})
	carried := &persist.PlayerSave{Sailing: json.RawMessage(`{"boats":[{"slot":0,"type":"sloop","name":[1,2,3],"hitpoints":0,"damage":0,"facilities":[],"location":{"kind":"docked","dock":"catherby"},"cargo":[],"parts":{"hull":0,"keel":0,"sails":0,"helm":0}}],"activeBoatSlot":0,"returnPoint":null,"tools":[],"lastDock":"catherby","lastStandardDock":"catherby"}`)}
	applyEntrySailing(p, carried)
	state := p.GetSailing()
	if len(state.Boats) != 1 || state.Boats[0].Type != "sloop" || state.ActiveBoatSlot == nil || *state.ActiveBoatSlot != 0 {
		t.Errorf("loaded state = %+v", state)
	}
	// Malformed JSON loads the empty state.
	bad := &persist.PlayerSave{Sailing: json.RawMessage(`{"boats": "nope"`)}
	applyEntrySailing(p, bad)
	if len(p.GetSailing().Boats) != 0 {
		t.Errorf("malformed state = %+v, want empty", p.GetSailing())
	}
}

func TestSnapshotSailingJSONRoundTrip(t *testing.T) {
	p := game.NewPlayer("sailor", game.Location{X: 3000, Y: 3000, Z: 0})
	p.GetSailing().Boats = append(p.GetSailing().Boats, game.OwnedBoat{
		Slot: 2, Type: "sloop", Name: game.BoatName{1, 2, 3},
		Location: game.DockedBoatLocation("catherby"),
		Cargo:    []*game.CargoSlot{},
		Parts:    game.BaseParts(),
	})
	raw, err := snapshotSailingJSON(p)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	// The snapshot is normalized: the boat survives, and a
	// re-parse yields the same state.
	restored := game.ParseSailingStateJSON(raw)
	if len(restored.Boats) != 1 || restored.Boats[0].Slot != 2 {
		t.Errorf("restored = %+v", restored)
	}
	// WithOwned overwrite/preserve for the sailing field.
	empty := persist.PlayerSave{Sailing: json.RawMessage(`{"boats":[],"activeBoatSlot":null,"returnPoint":null,"tools":[],"lastDock":null,"lastStandardDock":null}`)}
	owned := persist.OwnedState{Sailing: raw}
	with := empty.WithOwned(owned)
	if string(with.Sailing) != string(raw) {
		t.Errorf("WithOwned sailing = %s, want %s", with.Sailing, raw)
	}
	preserved := empty.WithOwned(persist.OwnedState{})
	if string(preserved.Sailing) != string(empty.Sailing) {
		t.Errorf("a nil sailing field must preserve, got %s", preserved.Sailing)
	}
}

func TestWorldEntityPackets(t *testing.T) {
	world := game.NewWorld(game.Location{X: 3089, Y: 3524, Z: 0})
	InstallSailing(nil, world)
	manager := world.BoatManager()
	manager.Spawn(1, sailingTestSpec(), sailingTestPlacement())
	viewer := game.NewPlayer("viewer", game.Location{X: 3000, Y: 3000, Z: 0})
	viewer.SetIndex(1)
	src := &playerFlushSource{owner: viewer, worldEntities: world.WorldEntities()}
	provider := func(regionID int) [4]int32 { return [4]int32{1, 2, 3, 4} }
	packets := src.WorldEntityPackets(provider)
	// Scene, info, spawn loc: three packets, in that order.
	if len(packets) != 3 {
		t.Fatalf("packets = %d, want 3 (scene, info, spawn loc)", len(packets))
	}
	scene := packets[0]
	if len(scene) < 3 || scene[0] != protocol.ServerRebuildWorldEntity {
		t.Fatalf("scene packet head = %v", scene[:min(3, len(scene))])
	}
	if got := binary.BigEndian.Uint16(scene[1:3]); int(got) != len(scene)-3 {
		t.Errorf("scene length prefix = %d, want %d", got, len(scene)-3)
	}
	// The XTEA key is the packet's last 16 bytes.
	tail := scene[len(scene)-16:]
	for i, want := range []int32{1, 2, 3, 4} {
		if got := int32(binary.BigEndian.Uint32(tail[i*4:])); got != want {
			t.Errorf("xtea[%d] = %d, want %d", i, got, want)
		}
	}
	info := packets[1]
	if len(info) < 2 || info[0] != protocol.ServerWorldEntityInfo || int(info[1]) != len(info)-2 {
		t.Fatalf("info packet head = %v", info[:min(2, len(info))])
	}
	// The spawn loc: opcode 134, 1-byte length, 8-byte payload.
	loc := packets[2]
	if len(loc) != 10 || loc[0] != protocol.ServerLocAddChange || loc[1] != 8 {
		t.Fatalf("spawn loc packet = %v", loc)
	}
	// A second flush sends nothing (nothing changed).
	if again := src.WorldEntityPackets(provider); len(again) != 0 {
		t.Errorf("second flush = %d packets, want 0", len(again))
	}
	// A nil provider yields the zero key.
	world.ForgetWorldEntityViewer(viewer)
	packets = src.WorldEntityPackets(nil)
	if len(packets) != 3 {
		t.Fatalf("nil-provider packets = %d, want 3", len(packets))
	}
	for _, b := range packets[0][len(packets[0])-16:] {
		if b != 0 {
			t.Error("a nil provider must yield the zero XTEA key")
			break
		}
	}
}

func TestWorldEntityPacketsDeckLocForms(t *testing.T) {
	world := game.NewWorld(game.Location{X: 3089, Y: 3524, Z: 0})
	InstallSailing(nil, world)
	manager := world.BoatManager()
	boat := manager.Spawn(1, sailingTestSpec(), sailingTestPlacement())
	viewer := game.NewPlayer("viewer", game.Location{X: 3000, Y: 3000, Z: 0})
	viewer.SetIndex(1)
	src := &playerFlushSource{owner: viewer, worldEntities: world.WorldEntities()}
	src.WorldEntityPackets(nil) // consume the spawn
	// An opFlags loc sends the 9-byte form (11 framed).
	opFlags := 3
	manager.SetDeckLoc(boat, game.BoatDeckLoc{ID: 200, X: 1, Y: 1, Level: 1, Shape: 10, Rotation: 1, OpFlags: &opFlags})
	packets := src.WorldEntityPackets(nil)
	if len(packets) != 1 || len(packets[0]) != 11 || packets[0][0] != protocol.ServerLocAddChange || packets[0][1] != 9 || packets[0][10] != 3 {
		t.Fatalf("opFlags loc packet = %v", packets)
	}
	// A removal sends LOC_DEL (8 framed bytes).
	manager.RemoveDeckLoc(boat, game.BoatDeckLoc{ID: 200, X: 1, Y: 1, Level: 1, Shape: 10, Rotation: 1})
	packets = src.WorldEntityPackets(nil)
	if len(packets) != 1 || len(packets[0]) != 8 || packets[0][0] != protocol.ServerLocDel || packets[0][1] != 6 {
		t.Fatalf("del loc packet = %v", packets)
	}
}

func TestWorldEntityPacketsNilOwner(t *testing.T) {
	var src *playerFlushSource
	if got := src.WorldEntityPackets(nil); got != nil {
		t.Errorf("a nil source must send nothing, got %d packets", len(got))
	}
	if got := (&playerFlushSource{}).WorldEntityPackets(nil); got != nil {
		t.Errorf("a nil owner must send nothing, got %d packets", len(got))
	}
	_ = session.XteaProvider(nil)
}

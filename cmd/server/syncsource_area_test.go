package main

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
	"tsps-go/internal/session"
)

// The SyncArea/MusicTile adapter (syncsource.go): the owner
// snapshot carries the private-area scene identity (the syncArea
// rule) and the music tile (the templated source tile, else the
// root-location tile). These tests pin the adapter; the flush
// decision table is pinned in flush_templated_test.go.

func areaTestPlayer(t *testing.T, name string, loc game.Location) *game.Player {
	t.Helper()
	p := game.NewPlayer(name, loc)
	p.Session = session.NewSession(session.NewHeadlessChannel(name))
	return p
}

func TestSyncViewPlainPrivateArea(t *testing.T) {
	spawn := game.Location{X: 3200, Y: 3200, Z: 0}
	owner := areaTestPlayer(t, "alice", spawn)
	src := &playerFlushSource{owner: owner}

	// No area: no SyncArea.
	ownerSnap, _, _ := src.SyncView()
	if ownerSnap.SyncArea != nil {
		t.Fatalf("no area: SyncArea = %+v, want nil", ownerSnap.SyncArea)
	}

	// In a plain private area: SyncArea set, no template.
	pa := game.NewPrivateArea([]game.Boundary{game.NewBoundary(3200, 3263, 3200, 3263, 0)})
	pa.Add(owner)
	ownerSnap, _, _ = src.SyncView()
	if ownerSnap.SyncArea == nil {
		t.Fatalf("in area: SyncArea = nil, want a snapshot")
	}
	if ownerSnap.SyncArea.Identity != any(pa) {
		t.Fatalf("SyncArea.Identity = %v, want the private area", ownerSnap.SyncArea.Identity)
	}
	if ownerSnap.SyncArea.Templated != nil {
		t.Fatalf("SyncArea.Templated = %+v, want nil for a plain area", ownerSnap.SyncArea.Templated)
	}

	// Leaving clears it again.
	pa.Remove(owner)
	ownerSnap, _, _ = src.SyncView()
	if ownerSnap.SyncArea != nil {
		t.Fatalf("after leaving: SyncArea = %+v, want nil", ownerSnap.SyncArea)
	}
}

func TestSyncViewTemplatedInstance(t *testing.T) {
	tia := game.NewTemplatedInstanceArea(1, 1)
	chunk := &game.TemplateChunk{SourceChunkX: 400, SourceChunkY: 400, SourcePlane: 0, Rotation: 0}
	tia.SetChunk(0, 0, 0, chunk)

	owner := areaTestPlayer(t, "alice", tia.Tile(2, 3, 0))
	tia.Add(owner)
	defer func() {
		tia.Remove(owner)
		tia.Destroy()
	}()
	src := &playerFlushSource{owner: owner}

	ownerSnap, _, _ := src.SyncView()
	if ownerSnap.SyncArea == nil || ownerSnap.SyncArea.Templated == nil {
		t.Fatalf("SyncArea = %+v, want a templated snapshot", ownerSnap.SyncArea)
	}
	if ownerSnap.SyncArea.Templated.SceneVersion != tia.GetSceneVersion() {
		t.Fatalf("SceneVersion = %d, want %d", ownerSnap.SyncArea.Templated.SceneVersion, tia.GetSceneVersion())
	}
	palette, _ := ownerSnap.SyncArea.Templated.Build(tia.BaseChunkX(), tia.BaseChunkY())
	if got := palette[0][6][6]; got != game.PackTemplateChunk(*chunk) {
		t.Fatalf("palette[0][6][6] = %d, want the packed chunk %d", got, game.PackTemplateChunk(*chunk))
	}

	// The music tile is the instance's source tile: chunk
	// (400,400) base (3200,3200) plus the local (2,3) offset.
	if ownerSnap.MusicTile == nil {
		t.Fatalf("MusicTile = nil, want the source tile")
	}
	want := protocol.Tile{X: 3202, Y: 3203, Level: 0}
	if *ownerSnap.MusicTile != want {
		t.Fatalf("MusicTile = %+v, want %+v", *ownerSnap.MusicTile, want)
	}
}

func TestSyncViewBoatDeck(t *testing.T) {
	m := game.NewBoatManager(nil)
	spec := game.BoatSpec{
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
			{X: 4, Y: 8},
		},
		BoardingTile: game.DeckTile{X: 4, Y: 8},
	}
	boat := m.Spawn(1, spec, game.BoatPlacement{FineX: 3000 * game.FineUnitsPerTile, FineY: 3000 * game.FineUnitsPerTile, Level: 0, Angle: 0})
	if boat == nil {
		t.Fatal("Spawn returned nil")
	}
	deckTile := game.Location{X: boat.DeckBaseX() + 4, Y: boat.DeckBaseY() + 8, Z: 1}
	owner := areaTestPlayer(t, "alice", deckTile)
	m.GetDeck(boat).Enter(owner)
	src := &playerFlushSource{owner: owner, boats: m}

	ownerSnap, _, _ := src.SyncView()
	// A deck counts as the main world: no SyncArea.
	if ownerSnap.SyncArea != nil {
		t.Fatalf("aboard: SyncArea = %+v, want nil (deck counts as main world)", ownerSnap.SyncArea)
	}
	// The music tile is the root-location (sea) tile, not the
	// deck tile.
	if ownerSnap.MusicTile == nil {
		t.Fatalf("MusicTile = nil, want the root-location tile")
	}
	root := m.RootLocation(&owner.Mobile)
	want := protocol.Tile{X: root.X, Y: root.Y, Level: root.Z}
	if *ownerSnap.MusicTile != want {
		t.Fatalf("MusicTile = %+v, want root tile %+v", *ownerSnap.MusicTile, want)
	}
	if ownerSnap.MusicTile.X == deckTile.X && ownerSnap.MusicTile.Y == deckTile.Y {
		t.Fatalf("MusicTile %+v equals the deck tile; want the sea tile under the boat", *ownerSnap.MusicTile)
	}
}

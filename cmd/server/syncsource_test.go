package main

import (
	"testing"
	"time"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
	"tsps-go/internal/session"
)

func sentHasOpcode(t *testing.T, ch *recordingChannel, opcode byte) bool {
	t.Helper()
	for _, frame := range ch.sent() {
		for _, p := range decodeServerFrame(t, frame) {
			if p.Opcode == opcode {
				return true
			}
		}
	}
	return false
}

func TestPlayerFlushSourceSyncView(t *testing.T) {
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	world := game.NewWorld(spawn)

	owner := game.NewPlayer("alice", spawn)
	owner.DisplayName = "Alice"
	ownerSession := session.NewSession(session.NewHeadlessChannel("owner"))
	owner.Session = ownerSession
	ownerBlock := []byte{0xde, 0xad, 0xbe, 0xef}
	ownerSession.SetSyncAppearance(ownerBlock)

	near := game.NewPlayer("bob", game.Location{X: 3094, Y: 3524, Z: 0})
	near.DisplayName = "Bob"
	near.Session = session.NewSession(session.NewHeadlessChannel("near"))
	// No appearance block stored: the snapshot must carry nil and
	// report AppearanceFlag (the bootstrap-race guard).

	far := game.NewPlayer("carol", game.Location{X: 3200, Y: 3524, Z: 0})
	far.Session = session.NewSession(session.NewHeadlessChannel("far"))

	npc := game.NewNPC(1, game.Location{X: 3092, Y: 3524, Z: 0})
	npc.Visible = true
	hidden := game.NewNPC(2, game.Location{X: 3092, Y: 3524, Z: 0})
	// hidden.Visible stays false: never in a local list.

	world.QueueAddPlayer(owner)
	world.QueueAddPlayer(near)
	world.QueueAddPlayer(far)
	world.QueueAddNPC(npc)
	world.QueueAddNPC(hidden)
	world.Process()

	// Set per-tick state AFTER Process: the reset phases clear
	// flags and step directions.
	near.Flags = near.Flags.Set(game.FlagAppearance)
	near.NeedsPlacement = true
	near.WalkingDirection = game.DirNorth
	npc.Facing = game.DirEast
	npc.WalkingDirection = game.DirNorth
	npc.RunningDirection = game.DirNorthEast

	src := &playerFlushSource{owner: owner}
	gotOwner, locals, npcs := src.SyncView()

	if gotOwner.Identity != any(owner) || gotOwner.Index != owner.Index {
		t.Errorf("owner identity/index = %v/%d, want %p/%d", gotOwner.Identity, gotOwner.Index, owner, owner.Index)
	}
	if gotOwner.X != 3089 || gotOwner.Y != 3524 || gotOwner.Z != 0 {
		t.Errorf("owner tile = (%d,%d,%d), want (3089,3524,0)", gotOwner.X, gotOwner.Y, gotOwner.Z)
	}
	if gotOwner.DisplayName != "Alice" {
		t.Errorf("owner display name = %q, want Alice", gotOwner.DisplayName)
	}
	if string(gotOwner.AppearanceBlock) != string(ownerBlock) {
		t.Errorf("owner appearance block = %x, want %x", gotOwner.AppearanceBlock, ownerBlock)
	}
	if gotOwner.AppearanceFlag {
		t.Error("owner AppearanceFlag = true, want false (block stored, no live flag)")
	}

	if len(locals) != 1 {
		t.Fatalf("locals = %d players, want 1 (near only)", len(locals))
	}
	local := locals[0]
	if local.Identity != any(near) || local.Index != near.Index {
		t.Errorf("local identity/index = %v/%d, want %p/%d", local.Identity, local.Index, near, near.Index)
	}
	if local.DisplayName != "Bob" || local.X != 3094 || local.Y != 3524 {
		t.Errorf("local = %+v, want Bob at (3094,3524,0)", local)
	}
	if local.AppearanceBlock != nil {
		t.Errorf("local appearance block = %x, want nil (no block stored)", local.AppearanceBlock)
	}
	if !local.AppearanceFlag {
		t.Error("local AppearanceFlag = false, want true (live flag + no stored block)")
	}
	if !local.NeedsPlacement {
		t.Error("local NeedsPlacement = false, want true")
	}
	if local.WalkingDx != 0 || local.WalkingDy != 1 {
		t.Errorf("local walking delta = (%d,%d), want (0,1) north", local.WalkingDx, local.WalkingDy)
	}
	if local.RunningDx != 0 || local.RunningDy != 0 {
		t.Errorf("local running delta = (%d,%d), want (0,0)", local.RunningDx, local.RunningDy)
	}

	if len(npcs) != 1 {
		t.Fatalf("npcs = %d, want 1 (visible only)", len(npcs))
	}
	snap := npcs[0]
	if snap.Identity != any(npc) || snap.Index != npc.Index || snap.TypeID != 1 {
		t.Errorf("npc snapshot = %+v, want identity %p index %d type 1", snap, npc, npc.Index)
	}
	if snap.FacingDx != 1 || snap.FacingDy != 0 {
		t.Errorf("npc facing delta = (%d,%d), want (1,0) east", snap.FacingDx, snap.FacingDy)
	}
	if snap.WalkingDx != 0 || snap.WalkingDy != 1 {
		t.Errorf("npc walking delta = (%d,%d), want (0,1)", snap.WalkingDx, snap.WalkingDy)
	}
	if snap.RunningDx != 1 || snap.RunningDy != 1 {
		t.Errorf("npc running delta = (%d,%d), want (1,1) north-east", snap.RunningDx, snap.RunningDy)
	}

	// Re-fetch semantics: moving the far player into view and
	// ticking again must surface it in a fresh SyncView.
	far.Loc = game.Location{X: 3090, Y: 3524, Z: 0}
	world.Process()
	_, locals2, _ := src.SyncView()
	if len(locals2) != 2 {
		t.Fatalf("locals after move = %d, want 2", len(locals2))
	}

	// A stored block clears the bootstrap-race guard flag when no
	// live flag is set.
	near.Flags = near.Flags.Reset()
	nearSession := near.Session.(*session.Session)
	nearSession.SetSyncAppearance([]byte{0x01})
	_, locals3, _ := src.SyncView()
	for _, l := range locals3 {
		if l.Identity == any(near) && l.AppearanceFlag {
			t.Error("local AppearanceFlag stayed true after a block was stored with no live flag")
		}
	}
}

func TestBootstrapStoresSyncAppearance(t *testing.T) {
	srv := testServer(t, 10)
	srv.Start()
	_, conn := enterTestPlayer(t, srv, "alice", "secret")
	defer conn.Cleanup()
	if conn.Session() == nil {
		t.Fatal("no session after entering")
	}
	if block := conn.Session().SyncAppearance(); len(block) == 0 {
		t.Error("SyncAppearance empty after bootstrap: the hookup did not store a block")
	}
}

func TestAddPlayerAttachesSyncAndTicks(t *testing.T) {
	srv := testServer(t, 10)
	srv.Start()
	ch, conn := enterTestPlayer(t, srv, "alice", "secret")
	defer conn.Cleanup()

	// The per-tick flush must send a rebuild on the first tick and
	// a TICK + player sync + NPC sync frame every tick after login.
	// (Phase 2's smoke test found no such frames arrive.)
	waitFor(t, "rebuild frame", 2*time.Second, func() bool {
		return sentHasOpcode(t, ch, protocol.ServerRebuildNormal)
	})
	waitFor(t, "tick frame", 2*time.Second, func() bool {
		return sentHasOpcode(t, ch, protocol.ServerTick)
	})
	waitFor(t, "player sync frame", 2*time.Second, func() bool {
		return sentHasOpcode(t, ch, protocol.ServerPlayerSync)
	})
	waitFor(t, "npc sync frame", 2*time.Second, func() bool {
		return sentHasOpcode(t, ch, protocol.ServerNpcInfo)
	})

	// Ticks keep coming: at least three distinct TICK packets over
	// the wait window (10 ms ticks, generous bound).
	deadline := time.Now().Add(2 * time.Second)
	ticks := 0
	for time.Now().Before(deadline) && ticks < 3 {
		ticks = 0
		for _, frame := range ch.sent() {
			for _, p := range decodeServerFrame(t, frame) {
				if p.Opcode == protocol.ServerTick {
					ticks++
				}
			}
		}
		if ticks < 3 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if ticks < 3 {
		t.Fatalf("saw %d TICK packets, want >= 3", ticks)
	}
}

// TestSnapshotForcedChat pins the flush-source gate: the snapshot
// carries overhead text only when the FORCED_CHAT flag is set and
// the text is non-nil (the createActorUpdates gate), for players
// and NPCs alike.
func TestSnapshotForcedChat(t *testing.T) {
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	p := game.NewPlayer("alice", spawn)
	if got := playerSnapshot(p, p); got.ForcedChat != nil {
		t.Errorf("plain player snapshot ForcedChat = %q, want nil", *got.ForcedChat)
	}
	p.ForceChat("over head")
	if got := playerSnapshot(p, p); got.ForcedChat == nil || *got.ForcedChat != "over head" {
		t.Errorf("player snapshot ForcedChat = %v, want \"over head\"", got.ForcedChat)
	}
	p.ResetUpdating()
	if got := playerSnapshot(p, p); got.ForcedChat != nil {
		t.Errorf("reset player snapshot ForcedChat = %q, want nil", *got.ForcedChat)
	}

	n := game.NewNPC(1, spawn)
	if got := npcSnapshot(p, n); got.ForcedChat != nil {
		t.Errorf("plain npc snapshot ForcedChat = %q, want nil", *got.ForcedChat)
	}
	n.ForceChat("npc says hi")
	if got := npcSnapshot(p, n); got.ForcedChat == nil || *got.ForcedChat != "npc says hi" {
		t.Errorf("npc snapshot ForcedChat = %v, want \"npc says hi\"", got.ForcedChat)
	}
}

// TestSnapshotAnimationGraphic covers the Phase 29 snapshot
// population: a performed animation/graphic lands on the
// snapshot only while its flag is set (createActorUpdates),
// slot 0 first, then the other slots in ascending order,
// a cleared slot as id -1 — and ResetUpdating clears it
// all again.
func TestSnapshotAnimationGraphic(t *testing.T) {
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	p := game.NewPlayer("alice", spawn)
	if got := playerSnapshot(p, p); got.Animation != nil || len(got.Graphics) != 0 {
		t.Errorf("plain player snapshot Animation/Graphics = %+v/%v, want nil/empty", got.Animation, got.Graphics)
	}
	p.PerformAnimation(422, 1)
	p.PerformGraphic(100, 50, 0)
	p.PerformGraphicInSlot(2, 200, 0, 0)
	p.ClearGraphicInSlot(3)
	snap := playerSnapshot(p, p)
	if snap.Animation == nil || snap.Animation.ID != 422 || snap.Animation.Delay != 1 {
		t.Errorf("player snapshot Animation = %+v, want id 422 delay 1", snap.Animation)
	}
	if len(snap.Graphics) != 3 ||
		snap.Graphics[0] != (protocol.GraphicView{Slot: 0, ID: 100, Height: 50, Delay: 0}) ||
		snap.Graphics[1] != (protocol.GraphicView{Slot: 2, ID: 200, Height: 0, Delay: 0}) ||
		snap.Graphics[2] != (protocol.GraphicView{Slot: 3, ID: -1, Height: 0, Delay: 0}) {
		t.Errorf("player snapshot Graphics = %+v, want slot-0 100, slot-2 200, cleared slot-3", snap.Graphics)
	}
	p.ResetUpdating()
	if got := playerSnapshot(p, p); got.Animation != nil || len(got.Graphics) != 0 {
		t.Errorf("reset player snapshot Animation/Graphics = %+v/%v, want nil/empty", got.Animation, got.Graphics)
	}

	n := game.NewNPC(1, spawn)
	n.PerformAnimation(836, 0)
	n.PerformGraphicInSlot(1, 300, 100, 2)
	nsnap := npcSnapshot(p, n)
	if nsnap.Animation == nil || nsnap.Animation.ID != 836 {
		t.Errorf("npc snapshot Animation = %+v, want id 836", nsnap.Animation)
	}
	if len(nsnap.Graphics) != 1 || nsnap.Graphics[0] != (protocol.GraphicView{Slot: 1, ID: 300, Height: 100, Delay: 2}) {
		t.Errorf("npc snapshot Graphics = %+v, want slot-1 id 300", nsnap.Graphics)
	}
}

func TestHitSnapshotsMapping(t *testing.T) {
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	viewer := game.NewPlayer("alice", spawn)
	viewer.GetCombat()
	n := game.NewNPC(1, spawn)
	n.GetCombat()
	n.Hitpoints = 8

	// No hits: no splats and no health bar.
	if hits, health := hitSnapshots(viewer, &n.Mobile, false); hits != nil || health != nil {
		t.Fatalf("no hits: hits=%v health=%v, want nil/nil", hits, health)
	}

	// A max hit from the viewer: the splat carries the mask,
	// the viewer-source flag, the max-hit flag, and the
	// splat overrides; the health bar reads the NPC's
	// current/max hitpoints.
	hit := game.NewHitDamage(5, game.HitMaskRed).
		SetRolledMaximum(5).
		SetSource(&viewer.Mobile).
		SetSplatTypes(1, 2)
	game.AddTickHit(&n.Mobile, hit)
	hits, health := hitSnapshots(viewer, &n.Mobile, false)
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	got := hits[0]
	if got.Mask != int(game.HitMaskRed) || got.Damage != 5 || got.OnMe ||
		!got.SourceIsViewer || !got.IsMaxHit || got.MaxHitThreshold != 0 ||
		got.SplatMine != 1 || got.SplatOthers != 2 {
		t.Fatalf("hit snapshot = %+v, want red/5/viewer/max/threshold-0/splats-1-2", got)
	}
	if health == nil || health.Current != 8 || health.Max != 10 {
		t.Fatalf("health = %+v, want {8 10}", health)
	}

	// A non-max hit from someone else, viewed on the viewer
	// themselves: OnMe set, SourceIsViewer and IsMaxHit clear.
	other := game.NewPlayer("bob", spawn)
	other.GetCombat()
	viewer.Skills().SetCurrentLevel(game.SkillHitpoints, 7, true)
	game.AddTickHit(&viewer.Mobile, game.NewHitDamage(3, game.HitMaskBlue).
		SetRolledMaximum(9).SetSource(&other.Mobile))
	hits, health = hitSnapshots(viewer, &viewer.Mobile, true)
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	got = hits[0]
	if !got.OnMe || got.SourceIsViewer || got.IsMaxHit {
		t.Fatalf("self hit snapshot = %+v, want OnMe, not viewer, not max", got)
	}
	if health == nil || health.Current != 7 || health.Max != 10 {
		t.Fatalf("health = %+v, want {7 10}", health)
	}
}

package main

import (
	"tsps-go/internal/game"
	"tsps-go/internal/session"
)

// This file installs the phase 9 live skills/appearance wiring for an
// entering player: the SkillManager update hook (skill deltas to the
// client) and the OnEquipmentChanged hook (appearance-block rebuilds
// from live state). It is the adapter half of the seam contracts in
// PHASE9_PLAN.md: container refreshes and skill updates reach the
// client through coordinator-installed hooks, never through new
// SessionPort methods, and appearance rebuilds go through
// OnEquipmentChanged into the session package's plain-data builder.
//
// CALL SITE (coordinator): worldAdapter.AddPlayer in server.go,
// inside the `if ep.Session != nil` per-tick sync wiring block,
// after installGroundItemSource and before the player is queued:
//
//	installSkillsAppearanceWiring(p, ep.Session)
//
// The skull icon in each rebuild comes from the player's live
// skull state (HeadSkullIcon, internal/game skull.go), already
// loaded from the carried save by prepareCarriedSave — no
// entry-time value is closed over. A skull change triggers a
// rebuild itself (ApplySkull / ClearSkull / timer expiry run
// the same onEquipmentChanged path an equipment refresh does),
// so the icon reaches observers on the tick it changes.
//
// The hooks run on the tick goroutine (skill updates fire from
// setters/AddExperience during turn processing; equipment refresh
// fires OnEquipmentChanged during turn processing), before the
// flush phase reads the stored block — so a rebuild lands in the
// same tick's sync frame. Session.SendPacket and
// Session.SetSyncAppearance are mutex-guarded, so the hook bodies
// are safe as written. With a nil session (tests only) the
// coordinator installs nothing and the manager/flag machinery
// degrades to no-ops, as designed.
//
// OnEquipmentChanged has no chaining: this install overwrites any
// previous hook. It is the sole installer; the coordinator must not
// install a second one.
//
// NOT handled here: the worn-inventory-94 send for transformed
// players is a gap (no transformation state), and container
// refresh sends (inventory/equipment snapshots) belong to the
// containers worker's adapter, not to this hook — this hook
// rebuilds ONLY the appearance block. The bootstrap-time block
// construction stays as-is; this hook only refreshes AFTER entry.
//
// FLAG SUFFICIENCY (verified by reading syncsource.go and
// flush.go; no edit needed there): playerSnapshot reports
// AppearanceFlag from the live Flag.APPEARANCE, and
// createPlayerView treats a flagged snapshot as dirty and re-reads
// the stored block into the observer's appearance cache. Equipment
// refresh flags APPEARANCE through onEquipmentChanged (player.go)
// on the same tick this hook rebuilds the block, and flags reset
// only in reset_player_updates, AFTER the flush phase (world.go) —
// so observers pick the rebuilt block up on the same tick. The
// one ordering requirement is the one this call site already
// satisfies: the hook must be installed before the player is
// queued, so no equipment change can fire before the rebuild hook
// exists.

// installSkillsAppearanceWiring installs the skill-update hook and
// the equipment-changed appearance rebuild for one entering player.
// A nil player or session installs nothing.
func installSkillsAppearanceWiring(p *game.Player, sess *session.Session) {
	if p == nil || sess == nil {
		return
	}
	// Skill deltas: SkillManager.updateSkill (via the hook) ->
	// Session.SendSkillDelta -> EncodeSkillsDelta, mirroring
	// PacketSender.sendSkill (PacketSender.ts:1480). The hook
	// carries the array index and the computed values; the
	// session sender translates the index to the client id.
	p.Skills().SetUpdateHook(func(update game.SkillUpdate) {
		sess.SendSkillDelta(update.Skill, update.CurrentLevel, update.MaxLevel, update.Experience, update.TotalLevel, update.CombatLevel)
	})
	// Appearance rebuilds: the scaffold's onEquipmentChanged
	// (player.go) flags Flag.APPEARANCE and invokes this hook when
	// equipment refreshes — and a skull change runs the same path
	// (see skull.go), so the block is rebuilt whenever the drawn
	// icon can change. (TS Player.onEquipmentChanged,
	// Player.ts:662, only resets the hitpoints-regen state; the
	// rebuild hook is the Go wiring that turns the refresh into a
	// new stored block.) Rebuild the block from LIVE state (look
	// + equipment + skills + display name + sync animations + the
	// live head skull icon) and store it with SetSyncAppearance,
	// mirroring createPlayerView's inputs (PlayerSession.ts:662,
	// 700-733). The Flag.APPEARANCE set by onEquipmentChanged
	// makes observers re-read the block.
	p.OnEquipmentChanged = func(pl *game.Player) {
		sess.SetSyncAppearance(session.BuildAppearanceBlock(liveAppearanceInput(pl)))
	}
}

// liveAppearanceInput renders one player's live state as the
// plain-data input BuildAppearanceBlock needs. Equipment comes
// from the live container in slot order (game.Item ID()/Amount());
// amounts of empty slots read as the container reports them (id
// -1), matching the bootstrap's empty-slot handling. Combat and
// total levels are the live computed values (CombatLevel over the
// raw base array, TotalLevel as the raw sum), as createPlayerView
// passes them. The skull icon is the live head skull icon
// (HeadSkullIcon: the always-override, else the skull type's
// icon while skulled, else -1 — Player.ts:1571-1576).
func liveAppearanceInput(p *game.Player) session.AppearanceInput {
	items := p.Equipment().Items()
	equipment := make([]session.EquippedItem, len(items))
	for i, item := range items {
		equipment[i] = session.EquippedItem{ID: item.ID(), Amount: item.Amount()}
	}
	return session.AppearanceInput{
		Look:        p.Appearance().Look(),
		Equipment:   equipment,
		DisplayName: p.GetDisplayName(),
		CombatLevel: p.Skills().CombatLevel(),
		TotalLevel:  p.Skills().TotalLevel(),
		Animations:  p.SyncAnimations(),
		SkullIcon:   p.HeadSkullIcon(),
		Pronouns:    p.Appearance().Pronouns(),
		HeadHint:    p.Appearance().HeadHint(),
		Hidden:      false,
	}
}

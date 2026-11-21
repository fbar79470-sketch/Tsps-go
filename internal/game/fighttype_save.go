package game

import "encoding/json"

// Fight-type save wiring (phase 30, item 9): the save's
// fightType field is a LIVE field in TS, not dead data.
//
// Load (PlayerSave.applyToPlayer, PlayerSave.ts:642):
// player.setFightType(this.fightType) — the saved fight type
// becomes the player's current fight type. Player.setFightType
// (Player.ts:1678-1683) resolves it with FightType.resolve
// (FightType.ts:81-111) against all fight types and ignores a
// value that resolves to nothing, leaving the UNARMED_PUNCH
// field default (Player.ts:230); Player.getFightType
// (Player.ts:1654-1676) then repairs an unresolvable value to
// the equipped weapon's first offered style. The login assign
// (WeaponInterfaceManager.assign, WeaponInterfaceManager.ts)
// finally re-picks from the offered styles — the remembered
// per-category slot first, else the current fight type when it
// is one of the offered styles, else the lowest child — so the
// saved value is observable exactly when no style was ever
// remembered for the equipped weapon's category and the saved
// value is one of its offered styles.
//
// PlayerSave.resolveSavedFightType (PlayerSave.ts:255-260)
// spells out the same resolution against the equipped
// weapon's offered styles — it has no callers in TS, but its
// available-types resolution is what the login assign
// effectively enforces, so restoring here against the offered
// styles yields the same post-assign state as TS.
//
// Save (PlayerSave.fromPlayer, PlayerSave.ts:754):
// playerSave.fightType = player.getFightType() — the live
// fight type is written on every snapshot. Go previously
// preserved the carried value verbatim (persist.WithOwned),
// so a style change mid-session never reached the save; the
// snapshot builder now writes the live value through
// FightTypeSaveJSON (see cmd/server/savewiring.go).

// savedFightType is the JSON shape FightType serializes to
// (FightType.ts:49-59, 60-77; defaultFightTypeJSON in
// persist/apply.go): animation/parentId/childId/bonusType,
// the style as {} (a FightStyle subclass instance with no own
// properties), the attack sound, rapid, npcAnimation. The
// fields are pointers so a missing field reads as "ignore
// this field", matching FightType.resolve's null-field rule
// (FightType.ts:91-104).
type savedFightType struct {
	Animation    *int  `json:"animation"`
	ParentID     *int  `json:"parentId"`
	ChildID      *int  `json:"childId"`
	BonusType    *int  `json:"bonusType"`
	Style        any   `json:"style"`
	AttackSound  any   `json:"attackSound"`
	Rapid        *bool `json:"rapid"`
	NPCAnimation *int  `json:"npcAnimation"`
}

// savedFightTypeJSON is the encoder half of savedFightType:
// the TS field order above, for FightTypeSaveJSON.
type savedFightTypeJSON struct {
	Animation    int            `json:"animation"`
	ParentID     int            `json:"parentId"`
	ChildID      int            `json:"childId"`
	BonusType    int            `json:"bonusType"`
	Style        struct{}       `json:"style"`
	AttackSound  savedSoundJSON `json:"attackSound"`
	Rapid        bool           `json:"rapid"`
	NPCAnimation int            `json:"npcAnimation"`
}

// savedSoundJSON is Sound's serialized shape (Sound.ts:90,
// 248-259): id/volume/delay/loopType.
type savedSoundJSON struct {
	ID       int `json:"id"`
	Volume   int `json:"volume"`
	Delay    int `json:"delay"`
	LoopType int `json:"loopType"`
}

// ResolveSavedFightType resolves a saved fightType object
// against the offered styles (PlayerSave.resolveSavedFightType,
// PlayerSave.ts:255-260, over FightType.resolve,
// FightType.ts:81-111): the first offered style whose
// animation, parent, child and bonus-type fields all match
// the saved one's, with a field ignored when the saved value
// does not carry it. It returns nil when the saved value is
// empty, malformed, carries none of the four fields, or
// matches no offered style — the caller leaves the current
// fight type alone, exactly as TS setFightType ignores an
// unresolvable value (Player.ts:1678-1683) and getFightType
// repairs it on first read.
func ResolveSavedFightType(raw json.RawMessage, available []*FightType) *FightType {
	if len(raw) == 0 {
		return nil
	}
	var saved savedFightType
	if err := json.Unmarshal(raw, &saved); err != nil {
		return nil
	}
	if saved.Animation == nil && saved.ParentID == nil &&
		saved.ChildID == nil && saved.BonusType == nil {
		return nil
	}
	for _, offered := range available {
		if offered == nil {
			continue
		}
		if saved.Animation != nil && offered.Animation != *saved.Animation {
			continue
		}
		if saved.ParentID != nil && offered.ParentID != *saved.ParentID {
			continue
		}
		if saved.ChildID != nil && offered.ChildID != *saved.ChildID {
			continue
		}
		if saved.BonusType != nil && offered.BonusType != *saved.BonusType {
			continue
		}
		return offered
	}
	return nil
}

// RestoreSavedFightType restores a carried save's fightType
// onto the live player (PlayerSave.applyToPlayer,
// PlayerSave.ts:642): the saved value resolved against the
// equipped weapon's offered styles (CurrentWeaponInterface,
// the same resolution resolveSavedFightType uses), set when
// it resolves, left alone otherwise. A nil player or an empty
// save value is a no-op. Called at login before
// AssignWeaponInterface, so the assign's "current fight type
// when offered" branch can honor the saved value when no
// per-category style was ever remembered.
func RestoreSavedFightType(p *Player, raw json.RawMessage) {
	if p == nil || len(raw) == 0 {
		return
	}
	iface := CurrentWeaponInterface(p)
	if iface == nil {
		return
	}
	if ft := ResolveSavedFightType(raw, iface.FightTypes); ft != nil {
		p.GetCombat().SetFightType(ft)
	}
}

// FightTypeSaveJSON serializes a live fight type to its save
// shape (PlayerSave.fromPlayer, PlayerSave.ts:754), in TS
// field order. A nil fight type encodes as UNARMED_PUNCH, the
// Player field default (Player.ts:230); Combat.FightType
// already defaults the same way, so nil only arises from a
// direct call.
func FightTypeSaveJSON(ft *FightType) json.RawMessage {
	if ft == nil {
		ft = UnarmedPunch
	}
	encoded, err := json.Marshal(savedFightTypeJSON{
		Animation:    ft.Animation,
		ParentID:     ft.ParentID,
		ChildID:      ft.ChildID,
		BonusType:    ft.BonusType,
		Style:        struct{}{},
		AttackSound:  savedSoundJSON{ID: ft.Sound.ID(), Volume: ft.Sound.Volume(), Delay: ft.Sound.Delay(), LoopType: ft.Sound.LoopType()},
		Rapid:        ft.Rapid,
		NPCAnimation: ft.NPCAnimation,
	})
	if err != nil {
		return nil
	}
	return json.RawMessage(encoded)
}

package game

// Live skull state: the port of Player.skullTimer / skullType /
// skullIconOverride (Player.ts:216-223, 278-279, 1377-1401,
// 1550-1580) and CombatFactory.skull / extendSkull
// (CombatFactory.ts:1473-1494), at commit bfd81e3.
//
// A skull is WHITE (icon 0) or RED (icon 1)
// (model/SkullType.ts). Core applies only WHITE, through
// HandleSkull (CombatFactory.ts:1418); RED is a supported type
// core never applies — no core call site passes RED_SKULL (the
// only CombatFactory.skull caller is handleSkull with
// WHITE_SKULL; PrayerHandler.ts:371 and PlayerDeath.ts:313
// only READ the red state). Item skulls (ITEM_SKULL_SECONDS,
// 20 minutes) are likewise plugin content, not core: the
// amulet of avarice keeps its wearer skulled from an
// onPlayerProcess hook (server/plugins/npcs/revenants/
// Avarice.Revenants.js keepSkulled) and the cape of skulls
// skulls on wear / login (server/plugins/items/
// CapeOfSkulls.plugin.js); both call CombatFactory.skull /
// extendSkull with WHITE and ITEM_SKULL_SECONDS. The Go core
// therefore provides the timer/type/override machinery plus
// ApplySkull/ExtendSkull for those callers, and records
// ItemSkullSeconds (combat_factory.go) for them; it invents
// no red-skull or item-skull application rule of its own.
//
// The timer is in TICKS, as TS stores it (setSkullTimer takes
// the saved value directly, PlayerSave.ts:667; fromPlayer
// writes getSkullTimer(), :777). ApplySkull/ExtendSkull take
// SECONDS, as the TS signatures do, and convert with
// skullTicks, the integer form of Misc.getTicks
// (seconds / 0.6, Misc.ts:14-16): exact for the 1800s and
// 1200s constants (3000 / 2000 ticks).
//
// SAVE SHAPE: the timer/type are owned save state
// (persist.PlayerSave SkullTimer/SkullType, PlayerSave.ts:
// 777-778). Entry loads them (cmd/server applyEntryLiveState)
// and the snapshot builder writes the live values
// (cmd/server buildSnapshot); a skulled player therefore
// keeps timer + type across relogin. The icon override is
// session-only: TS never persists it (PlayerSave carries no
// override field), so a relogin clears it, exactly as TS
// constructs a fresh Player with a null override.
//
// APPEARANCE SYNC: TS flags Flag.APPEARANCE on skull changes
// and createPlayerView re-encodes the block from live state
// every flush. Go caches the encoded block (SetSyncAppearance),
// so flagging alone would leave observers staring at a stale
// block: every skull mutation that changes the drawn icon
// therefore also runs onEquipmentChanged, the existing
// rebuild trigger, which re-encodes the block from live state
// (the coordinator-installed hook reads HeadSkullIcon()).
// ExtendSkull changes no icon, so like TS it flags nothing.
//
// TICK-GOROUTINE-ONLY: like the rest of Player.

// SkullType is a skull colour (model/SkullType.ts): WHITE
// icon 0, RED icon 1.
type SkullType int

const (
	// SkullTypeWhite is SkullType.WHITE_SKULL (icon 0).
	SkullTypeWhite SkullType = 0
	// SkullTypeRed is SkullType.RED_SKULL (icon 1).
	SkullTypeRed SkullType = 1
)

// IconID returns the skull's head icon id (SkullType.ts
// getIconId): 0 white, 1 red. Any other value reads as
// white: the save hydrator defaults unknown icons to 0
// (persist apply.go, NewCharacterSave), and TS only ever
// constructs the two singletons.
func (t SkullType) IconID() int {
	if t == SkullTypeRed {
		return 1
	}
	return 0
}

// SkullTypeFromIconID maps a saved icon id back to a skull
// type: 1 is RED, anything else is WHITE (see IconID).
func SkullTypeFromIconID(iconID int) SkullType {
	if iconID == 1 {
		return SkullTypeRed
	}
	return SkullTypeWhite
}

// skullTicks converts seconds to ticks (Misc.getTicks,
// seconds / 0.6): seconds*5/3 in integer arithmetic, exact
// for the 1800s PvP and 1200s item constants. A non-multiple
// of 3 truncates, as the int tick field the result lands in
// would; TS keeps the float, but no Go caller passes such a
// value (the save stores ticks, the seam passes seconds).
func skullTicks(seconds int) int {
	if seconds <= 0 {
		return 0
	}
	return seconds * 5 / 3
}

// SkullTimer returns the live skull timer in ticks
// (Player.ts:1403-1405, getSkullTimer). A nil player reads 0.
func (p *Player) SkullTimer() int {
	if p == nil {
		return 0
	}
	return p.skullTimer
}

// SetSkullTimer writes the live skull timer in ticks
// (Player.ts:1407-1409, setSkullTimer). It flags nothing and
// sends nothing: TS's setter is likewise bare, and callers
// that change the drawn icon (ApplySkull, ClearSkull, the
// timer expiry) flag it themselves. A nil player is a no-op.
func (p *Player) SetSkullTimer(ticks int) {
	if p == nil {
		return
	}
	p.skullTimer = ticks
}

// SkullType returns the live skull type (Player.ts:1550-1552,
// getSkullType). A nil player reads WHITE.
func (p *Player) SkullType() SkullType {
	if p == nil {
		return SkullTypeWhite
	}
	return p.skullType
}

// SetSkullType writes the live skull type (Player.ts:1578-1580,
// setSkullType). Bare, like SetSkullTimer: the icon-changing
// callers flag it themselves. A nil player is a no-op.
func (p *Player) SetSkullType(t SkullType) {
	if p == nil {
		return
	}
	p.skullType = t
}

// IsSkulled reports whether the player is skulled
// (Player.ts:1377-1379): the skull timer is above 0. A nil
// player is not skulled. This is the method form of the
// package-level IsSkulled (combat_factory.go), which reads
// the same state through the Hooks seam; the seam's
// IsSkulled field is wired to this method in cmd/server.
func (p *Player) IsSkulled() bool {
	return p.SkullTimer() > 0
}

// SkullIconID returns the skull icon id ignoring the skulled
// state (Player.ts:1554-1556, getSkullIconId): the override
// when one is set, else the skull type's icon.
func (p *Player) SkullIconID() int {
	if p == nil {
		return 0
	}
	if p.skullIconOverride != nil {
		return *p.skullIconOverride
	}
	return p.skullType.IconID()
}

// SetSkullIconOverride sets the skull icon override
// (Player.ts:1558-1568, setSkullIconOverride): icon nil clears
// it; a negative icon is normalized to nil, exactly as TS
// normalizes a non-integer or negative id to null; `always`
// only takes effect with a non-nil icon. A change flags
// APPEARANCE (and rebuilds the cached appearance block, see
// the file header); an unchanged override is a no-op, as TS
// returns early. A nil player is a no-op.
func (p *Player) SetSkullIconOverride(icon *int, always bool) {
	if p == nil {
		return
	}
	var normalized *int
	normalizedAlways := false
	if icon != nil && *icon >= 0 {
		v := *icon
		normalized = &v
		normalizedAlways = always
	}
	sameIcon := (p.skullIconOverride == nil && normalized == nil) ||
		(p.skullIconOverride != nil && normalized != nil && *p.skullIconOverride == *normalized)
	if sameIcon && p.skullIconOverrideAlways == normalizedAlways {
		return
	}
	p.skullIconOverride = normalized
	p.skullIconOverrideAlways = normalizedAlways
	p.onEquipmentChanged()
}

// HeadSkullIcon returns the skull drawn over the player's
// head, or -1 for none (Player.ts:1571-1576,
// getHeadSkullIcon): the override when it is marked always,
// else the skull icon id while skulled, else -1. A nil
// player draws none.
func (p *Player) HeadSkullIcon() int {
	if p == nil {
		return -1
	}
	if p.skullIconOverrideAlways {
		if p.skullIconOverride == nil {
			return -1
		}
		return *p.skullIconOverride
	}
	if !p.IsSkulled() {
		return -1
	}
	return p.SkullIconID()
}

// ApplySkull is CombatFactory.skull (CombatFactory.ts:1481-1494):
// set the type and the timer (seconds -> ticks), flag
// APPEARANCE (and rebuild the cached block), and send the
// skull message — "You've been skulled!" for WHITE,
// "@bla@You have received a @red@red skull@bla@! You can no
// longer use the Protect item prayer!" for RED, verbatim. A
// RED skull also deactivates the Protect Item prayer
// (PrayerHandler.deactivatePrayer, PrayerHandler.ts). A nil
// player is a no-op. This is what the Hooks.ApplySkull seam
// field is wired to in cmd/server (WHITE only: HandleSkull
// never applies RED, and the seam carries no type).
func (p *Player) ApplySkull(skullType SkullType, seconds int) {
	if p == nil {
		return
	}
	p.skullType = skullType
	p.skullTimer = skullTicks(seconds)
	p.onEquipmentChanged()
	if skullType == SkullTypeRed {
		p.SendMessage("@bla@You have received a @red@red skull@bla@! You can no longer use the Protect item prayer!")
		DeactivatePrayer(p, PrayerProtectItem)
	} else {
		p.SendMessage("You've been skulled!")
	}
}

// ExtendSkull is CombatFactory.extendSkull
// (CombatFactory.ts:1478-1480): keep the skull (and its
// colour) for at least `seconds` more — the timer becomes
// the max of the current timer and the converted seconds —
// without the skulled message and without flagging
// APPEARANCE, exactly as TS. A nil player is a no-op. This
// is what the Hooks.ExtendSkull seam field is wired to in
// cmd/server.
func (p *Player) ExtendSkull(seconds int) {
	if p == nil {
		return
	}
	if ticks := skullTicks(seconds); ticks > p.skullTimer {
		p.skullTimer = ticks
	}
}

// ClearSkull clears the skull: timer 0, type WHITE — the
// skull lines of resetAttributes (Player.ts:285-286, called
// from the PlayerDeath respawn tick). It flags APPEARANCE
// (and rebuilds the cached block) so the icon drops live:
// TS's resetAttributes relies on the death task's own
// appearance flagging, which a standalone clear has no
// equivalent of. A nil player is a no-op. Worker C (player
// death) calls this for the death reset; it never touches
// the Hooks seam directly.
func (p *Player) ClearSkull() {
	if p == nil {
		return
	}
	p.skullTimer = 0
	p.skullType = SkullTypeWhite
	p.onEquipmentChanged()
}

// ProcessSkullTimer counts the skull down a tick
// (Player.ts:1385-1401, processSkullTimer), called from
// Player.Process after area processing, before run-energy
// restore (Player.ts:556). Skipped when not skulled or
// dying: the items a death keeps follow the skull at the
// killing blow, not four ticks on. A skull "cannot disappear
// during PvP combat": its last tick waits while a player is
// this player's target or attacker (the attacker clears six
// seconds after the last hit), or a player's first hit is
// still on its way (the attacker is only set when it lands).
// All three terms have exact Go forms — Combat.Target() /
// Attacker() with Mobile.IsPlayer, HitQueue.
// HasIncomingPlayerHit — so no deviation is needed. (The
// "six seconds" is the attacker's own expiry in Combat, not
// a term this function re-checks.) Otherwise the timer
// decrements; at 0 the appearance is flagged (and the cached
// block rebuilt) so the icon drops on that tick. A nil
// player is a no-op.
func (p *Player) ProcessSkullTimer() {
	if p == nil || !p.IsSkulled() || combatantFor(&p.Mobile).Dying() {
		return
	}
	combat := p.GetCombat()
	inPvpCombat := false
	if combat != nil {
		if target := combat.Target(); target != nil && target.IsPlayer() {
			inPvpCombat = true
		}
		if attacker := combat.Attacker(); attacker != nil && attacker.IsPlayer() {
			inPvpCombat = true
		}
		if queue := combat.HitQueue(); queue != nil && queue.HasIncomingPlayerHit() {
			inPvpCombat = true
		}
	}
	if p.skullTimer <= 1 && inPvpCombat {
		return
	}
	p.skullTimer--
	if p.skullTimer == 0 {
		p.onEquipmentChanged()
	}
}

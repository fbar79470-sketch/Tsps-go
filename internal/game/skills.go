package game

import "math"

// SkillManager ports game/content/skill/SkillManager.ts (723 lines) and
// the skill identity/order in game/model/Skill.ts (187 lines) at commit
// bfd81e3. It REPLACES the phase 7 minimal Agility read (the static save
// level in runenergy.go): the coordinator rewires GetCurrentAgilityLevel
// to CurrentLevel(SkillAgility) during integration, which preserves the
// negative-level sanitize exactly (SkillManager.ts:425-433).
//
// IDENTITY: a skill is a plain int index everywhere (Skill.ts
// declaration order via the nextIndex counter), never a Skill enum
// type, so containers and listeners stay decoupled from this package's
// skill types. The client-id translation (Construction 21 -> 22,
// Hunter 22 -> 21, Skill.getClientId, Skill.ts) lives in the session
// package (skillClientIDs, bootstrap.go), which is the only place
// skill ids go on the wire; the game-side reading of the same table,
// with names, tab buttons, and chatbox interfaces, is skillmeta.go
// (SkillName/SkillButton/SkillChatboxInterface/SkillClientID).
//
// EXPERIENCE is int, matching the persist.Skills shape (persist
// stores []int). TS stores tenths as floats (sanitizeExperienceValue
// rounds to tenths, SkillManager.ts:111-119); the tenth-rounding step
// is a no-op for integers, and gains arrive as whole XP, so no
// precision is lost that the save could have kept. GetExperience
// returns the floored value TS sends (SkillManager.ts:508-517).
//
// GAPS, all deliberate (nothing below is faked):
//   - XP gain SOURCES: combat grants XP through SkillManager
//     (combat_factory.go, combat_magic.go); no skilling action
//     or content calls AddExperience in production yet. The
//     gate, rate, and emits below are driven by combat gains
//     and by tests until skilling content ports.
//   - The XP gate and world rate ARE ported: AddExperience
//     consults SkillHooks.CanGainExperience first (deny =>
//     return before any send) and applies rate = worldRate *
//     gateMultiplier when multipliers is true, else 1
//     (SkillManager.ts:136-143; PluginManager.ts:1665-1674;
//     GameConstants.ts:41, 119-124). The world rate defaults
//     to 1.0 and is set at boot through SetExperienceRate
//     (skill_hooks.go); the gate multiplier is sanitized
//     finite-and-positive else 1 at the seam boundary. With
//     no hub wiring the seam is inert and the rate is 1.
//   - The XP lock (XP_LOCKED_ATTRIBUTE, "skills:xp-locked") IS
//     ported: AddExperienceWithMultipliers returns early when
//     the player's attribute is strictly true, at the TS position
//     (after the exp-drop send, before the max-exp check;
//     SkillManager.ts:156).
//   - The exp-drop packet (PacketSender.sendExpDrop,
//     PacketSender.ts:1759-1767) is a NO-OP in TS at bfd81e3:
//     it builds a PacketBuilder(116) and hands it to the empty
//     PlayerSession.write() (PlayerSession.ts:228-230); no
//     native exp-drop encoder exists, and legacy opcode 116
//     collides with WIDGET_SET_ROTATE_SPEED. The hook still
//     fires at the TS position: the per-manager exp-drop hook
//     (SetExpDropHook) is invoked with the post-rate tenths
//     value as an int (the TS putInt truncation), and the
//     downstream send is a deliberate no-op mirroring TS —
//     EncodeExpDrop is layout-only (never sent) and
//     Session.SendExpDrop builds then discards (see
//     internal/protocol/expdrop.go and
//     internal/session/skillsends.go). With no hook installed
//     AddExperience invokes nothing for the drop.
//   - Level-up feedback (message, graphic, level-up interface)
//     belongs to the SkillLevelUp plugin (content, not ported),
//     but the events ARE emitted: emitPlayerLevelUp /
//     emitExperienceGained through SkillHooks, and the
//     "skills:experience-gained" custom event through
//     Player.EmitCustomEvent, with the two different
//     experience values TS sends (landed post-cap delta vs
//     post-rate tenths amount; SkillManager.ts:185-202). The
//     level-up DOES flag Flag.APPEARANCE, as TS does
//     (SkillManager.ts:193), and DOES publish the new levels
//     through the update hook before any feedback would run.
//   - pressedSkill (SkillManager.ts:205-234) is NOT ported: it
//     is DEAD CODE in TS — a grep over the whole bfd81e3 tree
//     (server/src and server/plugins, *.ts and *.js) finds
//     zero callers; only its own definition. The skills-tab
//     click is owned by content (SkillGuide.plugin.js offers
//     "skills:stat-clicked", SetSkillLevel.plugin.js claims
//     it). Porting pressedSkill would port behavior TS never
//     runs. The skill metadata it would have read (names,
//     buttons, chatbox interfaces, client ids, Skill.ts) IS
//     ported (skillmeta.go) for future content.
//   - startSkillable / stopSkillable (SkillManager.ts:626-649)
//     ARE ported, over the player skill slot (Player.GetSkill
//     / SetSkill, Player.ts:1590-1597) and the creation-menu
//     slot (Player.GetCreationMenu / SetCreationMenu,
//     Player.ts:1598-1603, creationmenu.go). startSkillable's
//     interface-removal step goes through the DialogueSender
//     seam (dialogue_entries.go), whose SendInterfaceRemoval
//     has minimal faithful scope (the mounted chatbox close
//     only); stopSkillable clears the creation-menu slot but
//     sends no close frames, exactly as TS does
//     (SkillManager.ts:648 sets the field only —
//     closeCreationMenu is a separate call). No skillable
//     content exists yet; the slot is driven by tests and
//     future content. Of the TS core callers of stopSkillable,
//     walkToReset (movement_queue.go) and the Drop/Equip
//     listeners (internal/listeners/drop.go, equip.go) call
//     StopSkillable and are live now that the slot exists
//     (their comments were updated at phase 20 integration);
//     Emotes, TeleportHandler, MagicOnItem, and
//     SecondGroundItem listeners have no Go equivalent call
//     site and stay gaps for their owning ports — do not
//     invent them.
//   - setLevel's guards are ported in part (SkillManager.ts:
//     236-282): the Wilderness guard and the
//     equipment-requirements guard run, with the TS refusal
//     texts, and PrayerHandler.deactivatePrayers runs through
//     DeactivatePrayers. PARTIAL: the rights bypasses are NOT
//     applied — the Go Player carries no rights field (see
//     player.go; commands.go preserves Rights verbatim in the
//     save and gates commands through a per-registry
//     RightsSource seam the SkillManager cannot reach), so
//     every player reads as RightsNone (the commands.go
//     degenerate reading) and the guards apply to everyone.
//     The equipment guard's item name reads through the
//     optional ItemNameSource; a source without it yields ""
//     (the DEFAULT definition's empty name), degrading only
//     the refusal text. Vengeance / BonusManager /
//     WeaponInterfaceManager stay gaps (systems NOT STARTED).
//     The Hitpoints floor's refusal ("Hitpoints must be set
//     to at least level 10.") IS sent. SetLevel applies apart
//     from these guards, which is safe because no production
//     caller exists (pressedSkill is dead code, above).
//   - The resting-level modifier registry is ported with ZERO
//     registered modifiers (plugin-owned); RestingLevel therefore
//     returns the base level until a modifier is registered.
//   - The "skills:current-level-changed", "skills:restore", and
//     "skills:experience-gained" custom events ARE emitted
//     through Player.EmitCustomEvent (nil-safe); restore
//     amounts ARE adjusted by the TS read-back rule
//     (SkillManager.ts:589-594). The restore payload is
//     emitted as a pointer so handlers can mutate it (see
//     skill_hooks.go).
//   - BonusManager.update in ensureCombatBaseline is a gap (no
//     BonusManager); the normalization and updateSkill sends are
//     ported.
//   - TS setters return `this` for chaining; Go setters return
//     nothing. Call sites sequence the calls.
//
// OUT-OF-RANGE INDICES: TS throws on a null skill. Go methods ignore
// an out-of-range index (getters return 0, setters return without
// effect, UpdateSkill sends nothing) rather than panicking on the
// tick goroutine, which would log the player out. No Go caller
// passes an invalid index; the guard only keeps corrupt input from
// becoming a crash.
//
// TICK-GOROUTINE-ONLY: like the rest of Player, the manager is
// mutated only on the tick goroutine (and at entry, before queueing).
// The update hook is installed once at entry and invoked
// synchronously from the tick goroutine.

// Skill indices in Skill.ts declaration order (the nextIndex counter
// assigns them as the static Skill instances are constructed,
// Skill.ts:7-31). SkillHitpoints == persist.HitpointsIndex (3) and
// SkillAgility == persist.AgilityIndex (16): the game package must
// not import persist, so the equality is pinned by TestSkillIndices
// with a comment citing internal/persist/playersave.go. The VALUES
// list order (Skill.ts:33-58) is identical to declaration order.
//
// Construction/Hunter swap on the wire only: Construction is index
// 21 with client id 22, Hunter is index 22 with client id 21
// (Skill.ts:28-29, getClientId). Array indices here are NEVER client
// ids.
const (
	SkillAttack       = iota // 0
	SkillDefence             // 1
	SkillStrength            // 2
	SkillHitpoints           // 3
	SkillRanged              // 4
	SkillPrayer              // 5
	SkillMagic               // 6
	SkillCooking             // 7
	SkillWoodcutting         // 8
	SkillFletching           // 9
	SkillFishing             // 10
	SkillFiremaking          // 11
	SkillCrafting            // 12
	SkillSmithing            // 13
	SkillMining              // 14
	SkillHerblore            // 15
	SkillAgility             // 16
	SkillThieving            // 17
	SkillSlayer              // 18
	SkillFarming             // 19
	SkillRunecrafting        // 20
	SkillConstruction        // 21 (client id 22)
	SkillHunter              // 22 (client id 21)
	SkillSailing             // 23
)

// SkillCount is SkillManager.AMOUNT_OF_SKILLS
// (SkillManager.ts:24): Skill.values().length, 24 including Sailing.
const SkillCount = 24

// MaxExperience is SkillManager.MAX_EXPERIENCE (SkillManager.ts:26):
// a skill's experience is capped at 200,000,000 (Wiki, Experience).
const MaxExperience = 200000000

// ExperienceFor99 is SkillManager.EXPERIENCE_FOR_99
// (SkillManager.ts:28): the experience threshold for level 99.
const ExperienceFor99 = 13034431

// XPLockedAttribute is SkillManager.XP_LOCKED_ATTRIBUTE
// (SkillManager.ts:27): "skills:xp-locked".
// AddExperienceWithMultipliers consults it: a gain is dropped
// when the player's attribute is strictly true
// (SkillManager.ts:156).
const XPLockedAttribute = "skills:xp-locked"

// ExpArray is SkillManager.EXP_ARRAY (SkillManager.ts:29-36): the
// experience threshold for each level 1..99, at index level-1
// (index 0 is level 1, threshold 0; index 98 is level 99,
// 13,034,431). It is identical to the session package's copy in
// bootstrap.go, which must not be refactored to share it (the
// session package duplicates the bootstrap-time normalization
// deliberately; see SetSkills).
var ExpArray = [99]int{
	0, 83, 174, 276, 388, 512, 650, 801, 969, 1154, 1358, 1584, 1833, 2107,
	2411, 2746, 3115, 3523, 3973, 4470, 5018, 5624, 6291, 7028, 7842, 8740, 9730, 10824, 12031, 13363, 14833,
	16456, 18247, 20224, 22406, 24815, 27473, 30408, 33648, 37224, 41171, 45529, 50339, 55649, 61512, 67983,
	75127, 83014, 91721, 101333, 111945, 123660, 136594, 150872, 166636, 184040, 203254, 224466, 247886, 273742,
	302288, 333804, 368599, 407015, 449428, 496254, 547953, 605032, 668051, 737627, 814445, 899257, 992895,
	1096278, 1210421, 1336443, 1475581, 1629200, 1798808, 1986068, 2192818, 2421087, 2673114, 2951373, 3258594,
	3597792, 3972294, 4385776, 4842295, 5346332, 5902831, 6517253, 7195629, 7944614, 8771558, 9684577, 10692629,
	11805606, 13034431,
}

// baseCombatSkills is SkillManager.BASE_COMBAT_SKILLS
// (SkillManager.ts:37-45), used by ensureCombatBaseline.
var baseCombatSkills = []int{
	SkillAttack, SkillDefence, SkillStrength, SkillHitpoints,
	SkillRanged, SkillPrayer, SkillMagic,
}

// SkillUpdate is one skill-delta send: the values PacketSender
// skillView (PacketSender.ts:1509) plus the totals sendSkill
// passes to encodeSkillsDelta (PacketSender.ts:1480). The
// Skill field is the ARRAY index; the session sender translates it
// to the client id. CurrentLevel is the sanitized current level,
// MaxLevel the computed base level (XP floor + cap), Experience the
// floored sanitized XP, and TotalLevel/CombatLevel the totals TS
// sends with every delta (raw-sum total, computed combat).
type SkillUpdate struct {
	Skill        int
	CurrentLevel int
	MaxLevel     int
	Experience   int
	TotalLevel   int
	CombatLevel  int
}

// SkillUpdateHook receives skill updates the coordinator must send
// to the client. It is installed once at entry by the coordinator
// (cmd/server adapter) and invoked synchronously by UpdateSkill on
// the tick goroutine; a nil hook is a no-op, so a bare NewPlayer in
// tests sends nothing. The hook must not mutate skill state: it
// runs inside setters that may still be updating arrays.
type SkillUpdateHook func(update SkillUpdate)

// RestingLevelModifier is a plugin's permanent boost: the level
// decay and natural restoration settle a skill at (SkillManager.ts:
// restingLevelModifiers, registerRestingLevelModifier). The
// signature mirrors TS (player, skill, level) with the skill as a
// plain index. Zero modifiers are registered; see
// RegisterRestingLevelModifier.
type RestingLevelModifier func(player *Player, skill, level int) int

// restingLevelModifiers is SkillManager.restingLevelModifiers
// (SkillManager.ts:454): the plugin registry, empty in Go (no
// plugin system). Registration happens before serving; reads are
// tick-goroutine-only, like the rest of the manager.
var restingLevelModifiers []RestingLevelModifier

// RegisterRestingLevelModifier ports
// SkillManager.registerRestingLevelModifier (SkillManager.ts:457-459):
// a plugin's permanent boost is appended to the registry. A nil
// modifier is ignored, as TS ignores a non-function.
func RegisterRestingLevelModifier(modifier RestingLevelModifier) {
	if modifier != nil {
		restingLevelModifiers = append(restingLevelModifiers, modifier)
	}
}

// SkillManager is the live skill state for one player: the Skills
// arrays (level/maxLevel/experience, SkillManager.ts:50-51, 687-723)
// plus the temporary level caps. The arrays are always length
// SkillCount; level holds current (boosted/drained) levels, maxLevel
// holds base levels, experience holds sanitized XP.
type SkillManager struct {
	player     *Player
	level      []int
	maxLevel   []int
	experience []int
	// levelCaps holds temporary, unsaved caps on base levels by
	// skill index (SkillManager.ts:476). A cap is gone on the next
	// login: nothing here is persisted.
	levelCaps  map[int]int
	updateHook SkillUpdateHook
	// expDropHook receives the exp-drop send at the TS position
	// in AddExperienceWithMultipliers (SkillManager.ts:151,
	// PacketSender.sendExpDrop). It is installed once at entry
	// by the coordinator (see SetExpDropHook); a nil hook sends
	// nothing. The exp argument is the post-rate tenths value
	// as an int (the TS putInt truncation).
	expDropHook func(skill, exp int)
}

// NewSkillManager returns a SkillManager with the constructor
// defaults (SkillManager.ts:53-63): all levels 1, Hitpoints 10 with
// 1184 XP. Note the 1184: the constructor hardcodes it, while the
// Skills constructor and setSkills use getExperienceForLevel(10) =
// 1154 (see SetSkills). A new character gets 1184; a loaded save
// whose Hitpoints XP is below 1154 is floored to 1154 by SetSkills.
func NewSkillManager(p *Player) *SkillManager {
	s := &SkillManager{
		player:     p,
		level:      make([]int, SkillCount),
		maxLevel:   make([]int, SkillCount),
		experience: make([]int, SkillCount),
		levelCaps:  make(map[int]int),
	}
	for i := 0; i < SkillCount; i++ {
		s.level[i] = 1
		s.maxLevel[i] = 1
		s.experience[i] = 0
	}
	s.level[SkillHitpoints] = 10
	s.maxLevel[SkillHitpoints] = 10
	s.experience[SkillHitpoints] = 1184
	return s
}

// validSkill reports whether skill is a usable array index. See the
// file header for the out-of-range policy.
func validSkill(skill int) bool { return skill >= 0 && skill < SkillCount }

// SetUpdateHook installs the skill-update hook (see SkillUpdateHook).
// The coordinator calls it once at entry; calling it with nil
// uninstalls the hook.
func (s *SkillManager) SetUpdateHook(hook SkillUpdateHook) {
	s.updateHook = hook
}

// SetExpDropHook installs the exp-drop hook: the coordinator's
// handling of the exp-drop packet (PacketSender.sendExpDrop,
// PacketSender.ts:1759-1767, opcode 116). The wired handler
// builds the legacy layout and discards it, mirroring the TS
// no-op (see the file header); the hook itself still fires at
// the TS position, so the call order stays observable. It is
// invoked from AddExperienceWithMultipliers at the TS position —
// after the tenths step, before the XP-lock check — with the
// skill index and the post-rate tenths gain as an int, and fires
// even for locked or capped gains, as TS sends the drop before
// those checks. Calling it with nil uninstalls the hook.
func (s *SkillManager) SetExpDropHook(hook func(skill, exp int)) {
	s.expDropHook = hook
}

// ExperienceForLevel ports SkillManager.getExperienceForLevel
// (SkillManager.ts:65-80): the XP threshold for a level. Levels 1..99
// read ExpArray at index level-1 (the TS `--level > 98 ? 98 : level`
// index can never exceed 98 for level <= 99). Levels above 99 run
// the TS summation loop, which returns the output accumulated
// through the level-1 terms — the standard formula, so level 100
// returns 14,391,160 (the virtual-level threshold), not the
// level-99 value. Levels <= 0 return 0: TS returns undefined
// (EXP_ARRAY[-1]), and the only consumer, setExperience, sanitizes
// undefined to 0, so 0 is the system-level equivalent. Absurdly
// large levels saturate at MaxExperience: TS returns values no
// consumer can store (setExperience caps at 200M), and the float
// sum overflows Go int range.
func ExperienceForLevel(level int) int {
	if level <= 99 {
		if level <= 0 {
			return 0
		}
		return ExpArray[level-1]
	}
	points := 0.0
	output := 0.0
	for lvl := 1; lvl <= level; lvl++ {
		points += math.Floor(float64(lvl) + 300.0*math.Pow(2.0, float64(lvl)/7.0))
		if lvl >= level {
			if !(output < 9.0e18) {
				return MaxExperience
			}
			return int(output)
		}
		output = math.Floor(points / 4)
	}
	return 0
}

// LevelForExperience ports SkillManager.getLevelForExperience
// (SkillManager.ts:82-100), mirroring the session package's
// levelForExperience (bootstrap.go) exactly, including the
// fall-through: negative XP returns 99, because the <= 99 scan finds
// no threshold <= a negative value and control reaches the final
// `return 99`. That quirk is unreachable through sanitized callers
// (every internal caller passes sanitized XP >= 0); callers must
// sanitize first, as getMaxLevel does via getExperience.
func LevelForExperience(xp int) int {
	if xp <= ExperienceFor99 {
		for j := len(ExpArray) - 1; j >= 0; j-- {
			if ExpArray[j] <= xp {
				return j + 1
			}
		}
	} else {
		points := 0
		output := 0
		for lvl := 1; lvl <= 99; lvl++ {
			points += int(math.Floor(float64(lvl) + 300.0*math.Pow(2.0, float64(lvl)/7.0)))
			output = points / 4
			if output >= xp {
				return lvl
			}
		}
	}
	return 99
}

// MaxAchievingLevel ports SkillManager.getMaxAchievingLevel
// (SkillManager.ts:102-104): the maximum achievable level for a
// skill, always 99. The skill argument is unused in TS and unused
// here; it is kept so the signature mirrors the TS static.
func MaxAchievingLevel(skill int) int {
	return 99
}

// sanitizeExperienceValue ports
// SkillManager.sanitizeExperienceValue (SkillManager.ts:111-119) for
// integer experience: non-positive values become 0, values at or
// above the cap become the cap. The TS tenth-rounding
// (Math.round(value*10)/10) is a no-op for integers, as in the
// session package's sanitizeExperience (bootstrap.go).
func sanitizeExperienceValue(value int) int {
	if value <= 0 {
		return 0
	}
	if value >= MaxExperience {
		return MaxExperience
	}
	return value
}

// AddExperiences ports SkillManager.addExperiences
// (SkillManager.ts:121-123): addExperience with multipliers on. The
// name is TS's (despite taking one skill); it exists so callers
// ported from TS keep their call shape.
func (s *SkillManager) AddExperiences(skill, xp int) {
	s.AddExperienceWithMultipliers(skill, xp, true)
}

// AddExperience ports SkillManager.addExperience with the default
// multipliers=true (SkillManager.ts:130-203): every gain takes the
// world's rate and the plugin gate's multiplier, through
// AddExperienceWithMultipliers.
func (s *SkillManager) AddExperience(skill, xp int) {
	s.AddExperienceWithMultipliers(skill, xp, true)
}

// AddExperienceWithMultipliers ports SkillManager.addExperience
// (SkillManager.ts:130-203) with an explicit multipliers flag: only
// content whose amount must not be multiplied (a league relic's
// bonus) passes false. Order is TS order: the plugin gate
// (SkillHooks.CanGainExperience, PluginManager.ts:1665-1674) is
// consulted first and a deny returns before any send; the rate is
// worldRate * gateMultiplier when multipliers is true, else 1;
// the tenths step keeps tenths and drops the rest; the exp-drop
// hook fires at the TS position (before the lock and cap checks,
// so locked and capped gains still send the drop); the XP lock
// returns early when the player's skills:xp-locked attribute is
// strictly true; the cap check returns at max experience. On a
// level-up the new levels are published through the update hook,
// SkillHooks.EmitPlayerLevelUp fires with the old and new base
// levels, and Flag.APPEARANCE is set. On a non-level-up gain,
// UpdateSkill sends the delta. Every landed gain (never a
// refused, locked, or capped one) fires
// SkillHooks.EmitExperienceGained with the LANDED post-cap delta
// and the "skills:experience-gained" custom event with the
// post-rate tenths amount, the multiplied flag, and the rate —
// the two different experience values TS sends, preserved.
func (s *SkillManager) AddExperienceWithMultipliers(skill, xp int, multipliers bool) {
	if !validSkill(skill) {
		return
	}
	if xp <= 0 {
		return
	}
	// The plugin gate (emitCanGainExperience, PluginManager.ts:
	// 1665-1674) runs first: a deny returns before any send
	// (SkillManager.ts:136-139). A nil seam is the no-hooks
	// answer: allow with multiplier 1. The multiplier is
	// sanitized finite-and-positive else 1 (PluginManager.ts:
	// 1673), re-applied here at the seam boundary so a
	// misbehaving seam cannot poison the rate; the hub applies
	// the same sanitize.
	gateMultiplier := 1.0
	if SkillHooks.CanGainExperience != nil {
		allow, multiplier := SkillHooks.CanGainExperience(s.player, skill, xp)
		if allow != nil && !*allow {
			return
		}
		if !math.IsNaN(multiplier) && !math.IsInf(multiplier, 0) && multiplier > 0 {
			gateMultiplier = multiplier
		}
	}
	// Multipliers: the world's rate, then the player's own (a
	// league profile's rate, carried by the gate multiplier).
	// multipliers=false yields rate 1, as in TS (rate =
	// multipliers ? world*gate : 1; SkillManager.ts:142).
	rate := 1.0
	if multipliers {
		rate = ExperienceRate() * gateMultiplier
	}
	// OSRS counts experience in tenths: a gain keeps its tenths
	// and drops the rest (the epsilon keeps 3.3 from reading as
	// 3.29999). Go stores integer experience, so the tenths
	// value is truncated to int here — the same truncation TS's
	// putInt applies to the exp drop, and the save shape
	// (persist stores []int) cannot keep tenths anyway.
	gained := int(math.Floor(float64(xp)*rate*10+1e-6) / 10)
	if gained <= 0 {
		return
	}
	// Send exp drop (SkillManager.ts:151, PacketSender.ts:1759):
	// after the tenths step, before the lock and cap checks.
	if s.expDropHook != nil {
		s.expDropHook(skill, gained)
	}
	// Don't add the experience if it has been locked
	// (SkillManager.ts:156): the player's XP_LOCKED_ATTRIBUTE is
	// strictly true. The check sits after the exp-drop send and
	// before the max-exp sanitize write-back, as in TS, so a
	// locked gain leaves stored experience untouched. A nil
	// player (a bare manager in a test) is never locked; the
	// manager reaches the player through its own player field,
	// exactly as TS SkillManager does (SkillManager.ts:49-51).
	if s.player != nil && s.player.GetAttribute(XPLockedAttribute) == true {
		return
	}
	//
	// If we already have max exp, don't add any more.
	currentExperience := sanitizeExperienceValue(s.experience[skill])
	s.experience[skill] = currentExperience
	if currentExperience >= MaxExperience {
		return
	}
	// The skill's level before any experience is added. Go ints
	// are always finite, so the TS non-finite branch has no form
	// here; the max(1, ...) write-back is ported.
	startingLevel := s.maxLevel[skill]
	if startingLevel < 1 {
		startingLevel = 1
	}
	s.maxLevel[skill] = startingLevel
	// Add experience to the selected skill.
	s.experience[skill] = sanitizeExperienceValue(s.experience[skill] + gained)
	// Get the skill's new level after experience has been added.
	newLevel := LevelForExperience(s.experience[skill])
	// Handle level up.
	if newLevel > startingLevel {
		levelsGained := newLevel - startingLevel
		s.maxLevel[skill] += levelsGained
		// The current level rises by the levels gained, as OSRS
		// does: a level-up restores nothing else and keeps any
		// boost or drain. setCurrentLevels refreshes, publishing
		// the new levels (and the native stats) through the
		// update hook before feedback hooks would run.
		s.SetCurrentLevels(skill, s.CurrentLevel(skill)+levelsGained)
		// emitPlayerLevelUp (SkillManager.ts:185-191): after
		// publishing, before the APPEARANCE flag. newLevel is
		// the maxLevel array value after the increment, as TS
		// reads it. The message, graphic, and pop-up belong to
		// the SkillLevelUp plugin, which listens to this event.
		if SkillHooks.EmitPlayerLevelUp != nil {
			SkillHooks.EmitPlayerLevelUp(&PlayerLevelUpEvent{
				Player:   s.player,
				Skill:    skill,
				OldLevel: startingLevel,
				NewLevel: s.maxLevel[skill],
			})
		}
		if s.player != nil {
			s.player.Flags = s.player.Flags.Set(FlagAppearance)
		}
	}
	if newLevel <= startingLevel {
		s.UpdateSkill(skill)
	}
	// What landed: the gain less anything over the experience
	// cap (SkillManager.ts:196). Only landed gains reach here:
	// refused, locked, and capped gains returned above.
	if SkillHooks.EmitExperienceGained != nil {
		SkillHooks.EmitExperienceGained(&ExperienceGainedEvent{
			Player:     s.player,
			Skill:      skill,
			Experience: s.experience[skill] - currentExperience,
		})
	}
	// The gain as it was added (after the rates and the tenths),
	// for content that reacts to experience itself (a league
	// relic's bonus XP). `multiplied` says whether the rates
	// applied, and `rate` is what they came to (1 when they
	// didn't). Nil-safe: no EmitCustomEvent means no
	// subscribers.
	if s.player != nil && s.player.EmitCustomEvent != nil {
		s.player.EmitCustomEvent("skills:experience-gained", SkillsExperienceGainedPayload{
			Player:     s.player,
			Skill:      skill,
			Experience: gained,
			Multiplied: multipliers,
			Rate:       rate,
		})
	}
}

// SetLevel ports SkillManager.setLevel (SkillManager.ts:236-282):
// the Wilderness guard, the equipment-requirements guard, the
// Hitpoints floor (each with its TS refusal message), the level/XP
// assignment plus the APPEARANCE flag, and
// PrayerHandler.deactivatePrayers through DeactivatePrayers.
// PARTIAL, see the file header: the rights bypasses are not
// applied (the Go Player carries no rights state, so every
// player reads as RightsNone and the guards apply to everyone),
// and setHasVengeance(false), BonusManager.update, and
// WeaponInterfaceManager.assign stay gaps (systems NOT STARTED).
// Note the TS double send: setExperience refreshes (updateSkill),
// then setLevel calls updateSkill again. Both sends are ported, so
// the hook fires twice, as PacketSender.sendSkill fires twice in
// TS. The equipment guard reads requirements through the player's
// ItemDefinitionSource (the equipment.go pattern: a nil source
// means no requirements; a missing entry counts as 0) and the
// item name through the optional ItemNameSource; the game package
// already imports internal/definition elsewhere, but the manager
// has no definition store to read, so the seam is the only
// faithful route.
func (s *SkillManager) SetLevel(skill, level int) {
	if !validSkill(skill) {
		return
	}
	// Make sure they aren't in wild (SkillManager.ts:239-245).
	// GAP (partial): the ADMINISTRATOR/DEVELOPER/OWNER bypass
	// needs rights state the Go Player does not carry; see the
	// file header. A player with no AreaManager (a standalone
	// player, or a world without Wilderness wired) is never in
	// the Wilderness, matching the AggressionEnv zero value.
	if s.player != nil && s.player.inWilderness() {
		s.player.SendMessage("You cannot do this in the Wilderness!")
		return
	}
	// Make sure they aren't wearing any items which aren't
	// allowed to be worn at that level (SkillManager.ts:248-259).
	// GAP (partial): the DEVELOPER bypass needs rights state;
	// see the file header. The iteration is slot order, as TS
	// iterates getEquipment().getItems(); empty slots (id -1,
	// the Go form of TS nulls) are skipped, and a nil
	// requirements array means no gate for that item.
	if s.player != nil {
		if src := s.player.ItemDefinitions(); src != nil {
			if equipment := s.player.Equipment(); equipment != nil {
				for _, item := range equipment.Items() {
					if item.ID() < 0 {
						continue
					}
					requirements := src.Requirements(item.ID())
					if requirements == nil {
						continue
					}
					required := 0
					if skill < len(requirements) {
						required = requirements[skill]
					}
					if required > level {
						s.player.SendMessage("Please unequip your " + itemNameOf(src, item.ID()) + " before doing that.")
						return
					}
				}
			}
		}
	}
	if skill == SkillHitpoints && level < 10 {
		// SkillManager.ts:266: the refusal text is sent and
		// setLevel returns without assigning.
		s.player.SendMessage("Hitpoints must be set to at least level 10.")
		return
	}
	s.SetCurrentLevel(skill, level, false)
	s.SetMaxLevels(skill, level, false)
	s.SetExperience(skill, ExperienceForLevel(level))
	s.UpdateSkill(skill)
	// GAP: setHasVengeance(false), BonusManager.update,
	// WeaponInterfaceManager.assign (systems NOT STARTED).
	if s.player != nil {
		// PrayerHandler.deactivatePrayers (SkillManager.ts:280).
		DeactivatePrayers(&s.player.Mobile)
		s.player.Flags = s.player.Flags.Set(FlagAppearance)
	}
}

// itemNameOf reads an item's display name through the optional
// ItemNameSource capability of src (ItemDefinition.getName,
// ItemDefinition.ts:177-179). A source without the capability
// yields "", matching the DEFAULT definition's empty name
// (ItemDefinition.ts:75); see ItemNameSource in trade_hooks.go.
func itemNameOf(src ItemDefinitionSource, id int) string {
	if src == nil {
		return ""
	}
	if names, ok := src.(ItemNameSource); ok {
		return names.ItemName(id)
	}
	return ""
}

// UpdateSkill ports SkillManager.updateSkill (SkillManager.ts:284-294):
// it sends the skill delta (PacketSender.sendSkill) through the
// coordinator-installed hook, with the skillView values and the
// totals. With no hook installed it is a no-op. TS returns `this`;
// the dictated Go signature returns nothing.
func (s *SkillManager) UpdateSkill(skill int) {
	if s.updateHook == nil || !validSkill(skill) {
		return
	}
	s.updateHook(SkillUpdate{
		Skill:        skill,
		CurrentLevel: s.CurrentLevel(skill),
		MaxLevel:     s.MaxLevel(skill),
		Experience:   s.Experience(skill),
		TotalLevel:   s.TotalLevel(),
		CombatLevel:  s.CombatLevel(),
	})
}

// CombatLevel ports SkillManager.getCombatLevel
// (SkillManager.ts:301-303): calculateCombatLevel over the RAW
// maxLevel array (not the computed MaxLevel values).
func (s *SkillManager) CombatLevel() int {
	return CalculateCombatLevel(s.maxLevel)
}

// CalculateCombatLevel ports SkillManager.calculateCombatLevel
// (SkillManager.ts:313-326): the combat level for a set of base
// levels indexed by skill ordinal, clamped to 3..126. The formula,
// in TS float order: base = (defence + hp + floor(prayer/2)) * 0.25;
// melee = (attack + strength) * 0.325; ranger =
// floor(ranged*1.5) * 0.325; mage = floor(magic*1.5) * 0.325;
// floor(base + max(melee, ranger, mage)), clamped. Missing entries
// in a short slice read as 0: TS would compute NaN there, and no
// Go caller passes a short array.
func CalculateCombatLevel(levels []int) int {
	at := func(skill int) int {
		if skill < len(levels) {
			return levels[skill]
		}
		return 0
	}
	attack := at(SkillAttack)
	defence := at(SkillDefence)
	strength := at(SkillStrength)
	hp := at(SkillHitpoints)
	prayer := at(SkillPrayer)
	ranged := at(SkillRanged)
	magic := at(SkillMagic)
	base := (float64(defence) + float64(hp) + math.Floor(float64(prayer)/2)) * 0.25
	melee := float64(attack+strength) * 0.325
	ranger := math.Floor(float64(ranged)*1.5) * 0.325
	mage := math.Floor(float64(magic)*1.5) * 0.325
	level := int(math.Floor(base + math.Max(melee, math.Max(ranger, mage))))
	if level < 3 {
		return 3
	}
	if level > 126 {
		return 126
	}
	return level
}

// applyMinimumSkill ports SkillManager.applyMinimumSkill
// (SkillManager.ts:328-348): raise a skill's base level, current
// level, and (when given) experience to minimums, sending an update
// when anything changed. The variadic minExperience mirrors the TS
// optional parameter: absent means "no experience minimum".
func (s *SkillManager) applyMinimumSkill(skill, minLevel int, minExperience ...int) bool {
	updated := false
	if s.maxLevel[skill] < minLevel {
		s.maxLevel[skill] = minLevel
		updated = true
	}
	if s.level[skill] < minLevel {
		s.level[skill] = minLevel
		updated = true
	}
	if len(minExperience) > 0 && s.experience[skill] < minExperience[0] {
		s.experience[skill] = minExperience[0]
		updated = true
	}
	if updated {
		s.UpdateSkill(skill)
	}
	return updated
}

// EnsureCombatBaseline ports SkillManager.ensureCombatBaseline
// (SkillManager.ts:350-396): normalize every skill (sanitized XP,
// base level at least the XP-implied level, current level at least
// 0), then enforce the combat baseline — Hitpoints at least 10 with
// at least 1184 XP, every other base combat skill at least 1. The
// Hitpoints applyMinimumSkill return value is discarded in TS and
// is discarded here too; only the other combat skills feed the
// `updated` flag. GAP: BonusManager.update (no BonusManager); the
// normalization and per-skill sends are ported.
func (s *SkillManager) EnsureCombatBaseline() {
	updated := false
	for i := 0; i < SkillCount; i++ {
		rawExp := s.experience[i]
		rawMax := s.maxLevel[i]
		rawLevel := s.level[i]
		exp := sanitizeExperienceValue(rawExp)
		levelFromExp := LevelForExperience(exp)
		if levelFromExp < 1 {
			levelFromExp = 1
		}
		// Go ints are always finite, so the TS non-finite
		// branches have no form here; floor is the identity.
		max := levelFromExp
		if rawMax > max {
			max = rawMax
		}
		level := rawLevel
		if level < 0 {
			level = 0
		}
		if exp != rawExp {
			s.experience[i] = exp
			updated = true
		}
		if max != rawMax {
			s.maxLevel[i] = max
			updated = true
		}
		if level != rawLevel {
			s.level[i] = level
			updated = true
		}
	}
	baselineXp := ExperienceForLevel(10)
	if baselineXp < 1184 {
		baselineXp = 1184
	}
	s.applyMinimumSkill(SkillHitpoints, 10, baselineXp)
	for _, skill := range baseCombatSkills {
		if skill == SkillHitpoints {
			continue
		}
		updated = s.applyMinimumSkill(skill, 1) || updated
	}
	// GAP: BonusManager.update(this.player) when updated — no
	// BonusManager exists in Go.
	_ = updated
}

// TotalLevel ports SkillManager.getTotalLevel
// (SkillManager.ts:398-404): the sum of the RAW maxLevel array —
// not the computed MaxLevel values, so neither the XP floor nor
// level caps apply here. (After SetSkills normalization the raw
// values already include the XP floor; the distinction only shows
// for state written through the raw setters.)
func (s *SkillManager) TotalLevel() int {
	total := 0
	for i := 0; i < SkillCount; i++ {
		total += s.maxLevel[i]
	}
	return total
}

// TotalExperience ports SkillManager.getTotalExp
// (SkillManager.ts:411-418): the sum of getExperience over all
// skills (sanitized, floored XP).
func (s *SkillManager) TotalExperience() int {
	total := 0
	for i := 0; i < SkillCount; i++ {
		total += s.Experience(i)
	}
	return total
}

// CurrentLevel ports SkillManager.getCurrentLevel
// (SkillManager.ts:425-433) INCLUDING the sanitize: a negative
// level is written back as 0 and 0 is returned. Go ints cannot be
// non-finite, so the TS non-finite branch has no Go form. The
// run-energy drain depends on this exact behavior (runenergy.go);
// the phase 7 minimal read is preserved verbatim here.
func (s *SkillManager) CurrentLevel(skill int) int {
	if !validSkill(skill) {
		return 0
	}
	if s.level[skill] < 0 {
		s.level[skill] = 0
		return 0
	}
	return s.level[skill]
}

// MaxLevel ports SkillManager.getMaxLevel (SkillManager.ts:441-451):
// the base level is at least the level the XP implies (the XP
// floor), capped by any temporary level cap — and it is NEVER
// written back, because mid-way through
// setMaxLevel(...).setExperience(...) the old experience would pin
// the max level at its old value (TS issue #101).
func (s *SkillManager) MaxLevel(skill int) int {
	if !validSkill(skill) {
		return 0
	}
	fromArray := s.maxLevel[skill]
	if fromArray < 1 {
		fromArray = 1
	}
	fromExp := LevelForExperience(s.Experience(skill))
	if fromExp < 1 {
		fromExp = 1
	}
	level := fromArray
	if fromExp > level {
		level = fromExp
	}
	if cap, ok := s.levelCaps[skill]; ok {
		if level > cap {
			level = cap
		}
		if level < 1 {
			level = 1
		}
	}
	return level
}

// RestingLevel ports SkillManager.getRestingLevel
// (SkillManager.ts:465-474): the base level, raised by any
// registered resting-level modifier (a plugin's permanent boost).
// Boosts decay down to it and drained levels restore up to it. With
// zero modifiers registered it returns the base level; a modifier
// may never lower the result below the base.
func (s *SkillManager) RestingLevel(skill int) int {
	if !validSkill(skill) {
		return 0
	}
	base := s.MaxLevel(skill)
	level := base
	for _, modifier := range restingLevelModifiers {
		// TS keeps the modifier's value only when finite and
		// floors it; Go ints are always finite and integral.
		level = modifier(s.player, skill, level)
	}
	if level < base {
		return base
	}
	return level
}

// SetMaxLevelCap ports SkillManager.setMaxLevelCap
// (SkillManager.ts:482-492): cap a skill's base level until cleared
// with a nil cap. Nothing is saved: the cap is gone on the next
// login. A non-nil cap is floored at 1 (Go ints need no floor
// step). When the cap lowers the base level below the current
// level, the current level is lowered to the cap without a
// refresh; the cap change itself always sends an update.
func (s *SkillManager) SetMaxLevelCap(skill int, cap *int) {
	if !validSkill(skill) {
		return
	}
	if cap == nil {
		delete(s.levelCaps, skill)
	} else {
		capped := *cap
		if capped < 1 {
			capped = 1
		}
		s.levelCaps[skill] = capped
	}
	if cap != nil && s.CurrentLevel(skill) > s.MaxLevel(skill) {
		s.SetCurrentLevel(skill, s.MaxLevel(skill), false)
	}
	s.UpdateSkill(skill)
}

// GetMaxLevels ports SkillManager.getMaxLevels
// (SkillManager.ts:498-500): the RAW maxLevel array value at the
// given index — note the TS parameter is a number index, not a
// Skill. Unlike MaxLevel, it applies neither the XP floor nor any
// cap, and never sanitizes.
func (s *SkillManager) GetMaxLevels(skill int) int {
	if !validSkill(skill) {
		return 0
	}
	return s.maxLevel[skill]
}

// Experience ports SkillManager.getExperience
// (SkillManager.ts:508-517): the sanitized XP, written back when
// the stored value needed sanitizing, returned floored (the
// tenths stay stored in TS; Go ints have no tenths, so the stored
// and returned values coincide).
func (s *SkillManager) Experience(skill int) int {
	if !validSkill(skill) {
		return 0
	}
	sanitized := sanitizeExperienceValue(s.experience[skill])
	if s.experience[skill] != sanitized {
		s.experience[skill] = sanitized
	}
	return sanitized
}

// SetCurrentLevel ports SkillManager.setCurrentLevel
// (SkillManager.ts:527-541): set the current (temporary) level,
// clamping negatives to 0, emitting the
// "skills:current-level-changed" custom event when the stored
// level actually changed (before the refresh), and refreshing
// (sending the delta) when asked. The emit is nil-safe: no
// player, or no EmitCustomEvent installed, emits nothing.
func (s *SkillManager) SetCurrentLevel(skill, level int, refresh bool) {
	if !validSkill(skill) {
		return
	}
	previous := s.level[skill]
	current := level
	if current < 0 {
		current = 0
	}
	s.level[skill] = current
	if previous != current && s.player != nil && s.player.EmitCustomEvent != nil {
		s.player.EmitCustomEvent("skills:current-level-changed", SkillsCurrentLevelChangedPayload{
			Player:   s.player,
			Skill:    skill,
			Previous: previous,
			Current:  current,
		})
	}
	if refresh {
		s.UpdateSkill(skill)
	}
}

// SetMaxLevels ports SkillManager.setMaxLevels
// (SkillManager.ts:544-551): write the raw base level, refreshing
// when asked. The value is stored as given (no XP floor, no clamp):
// the floor lives in MaxLevel, not in storage.
func (s *SkillManager) SetMaxLevels(skill, level int, refresh bool) {
	if !validSkill(skill) {
		return
	}
	s.maxLevel[skill] = level
	if refresh {
		s.UpdateSkill(skill)
	}
}

// SetExperiences ports SkillManager.setExperiences
// (SkillManager.ts:553-559): write sanitized XP, refreshing when
// asked.
func (s *SkillManager) SetExperiences(skill, xp int, refresh bool) {
	if !validSkill(skill) {
		return
	}
	s.experience[skill] = sanitizeExperienceValue(xp)
	if refresh {
		s.UpdateSkill(skill)
	}
}

// SetCurrentLevels ports SkillManager.setCurrentLevels
// (SkillManager.ts:561-565): the current level only (Java parity —
// the max/real level is managed through SetMaxLevels/SetMaxLevel),
// refreshing. TS takes an optional refresh defaulting to true; the
// Go form always refreshes, and callers needing no refresh use
// SetCurrentLevel with refresh=false.
func (s *SkillManager) SetCurrentLevels(skill, level int) {
	s.SetCurrentLevel(skill, level, true)
}

// SetCurrentLevelCombat ports SkillManager.setCurrentLevelCombat
// (SkillManager.ts:567-570): set the current level with a refresh.
func (s *SkillManager) SetCurrentLevelCombat(skill, level int) {
	s.SetCurrentLevel(skill, level, true)
}

// SetMaxLevel ports SkillManager.setMaxLevel
// (SkillManager.ts:572-574): SetMaxLevels with a refresh.
func (s *SkillManager) SetMaxLevel(skill, level int) {
	s.SetMaxLevels(skill, level, true)
}

// SetExperience ports SkillManager.setExperience
// (SkillManager.ts:576-578): SetExperiences with a refresh.
func (s *SkillManager) SetExperience(skill, xp int) {
	s.SetExperiences(skill, xp, true)
}

// IncreaseCurrentLevelMax ports
// SkillManager.increaseCurrentLevelMax (SkillManager.ts:580-582):
// raise the current level by amount, capped at the base level plus
// amount.
func (s *SkillManager) IncreaseCurrentLevelMax(skill, amount int) {
	s.IncreaseCurrentLevel(skill, amount, s.MaxLevel(skill)+amount)
}

// IncreaseCurrentLevel ports SkillManager.increaseCurrentLevel
// (SkillManager.ts:584-603): a restore or boost, capped at max. An
// increase never lowers a level: one already at or past the cap
// stays where it is. When amount > 0 the "skills:restore" custom
// event fires FIRST (before the cap checks, as in TS), and the
// amount is read back under the TS rule (SkillManager.ts:589-594):
// only when a handler changed it, and only when the changed value
// is finite and >= 0, floored, is it used. Go ints are always
// finite and integral, so the rule collapses to "changed and >=
// 0"; a handler that sets a negative amount is ignored. The
// payload is emitted as a pointer so the mutation is visible
// (see SkillsRestorePayload). With no EmitCustomEvent installed
// there is no handler to change the amount, so nothing is
// emitted and the amount stands.
func (s *SkillManager) IncreaseCurrentLevel(skill, amount, max int) {
	if !validSkill(skill) {
		return
	}
	if amount > 0 && s.player != nil && s.player.EmitCustomEvent != nil {
		// A restore or boost, whatever its source: plugins may
		// change `amount` (a league passive's Prayer gain);
		// `max` stays the caller's cap.
		restore := &SkillsRestorePayload{Player: s.player, Skill: skill, Amount: amount, Max: max}
		s.player.EmitCustomEvent("skills:restore", restore)
		if restore.Amount != amount && restore.Amount >= 0 {
			amount = restore.Amount
		}
	}
	curr := s.CurrentLevel(skill)
	if curr >= max {
		return
	}
	if curr+amount > max {
		s.SetCurrentLevels(skill, max)
		return
	}
	s.SetCurrentLevels(skill, curr+amount)
}

// DecreaseCurrentLevel ports SkillManager.decreaseCurrentLevel
// (SkillManager.ts:605-612): lower the current level by amount,
// floored at minimum.
func (s *SkillManager) DecreaseCurrentLevel(skill, amount, minimum int) {
	if !validSkill(skill) {
		return
	}
	curr := s.CurrentLevel(skill)
	if curr-amount < minimum {
		s.SetCurrentLevels(skill, minimum)
		return
	}
	s.SetCurrentLevels(skill, curr-amount)
}

// DecreaseLevelMax ports SkillManager.decreaseLevelMax
// (SkillManager.ts:614-616): lower the current level by amount,
// floored at the base level minus amount.
func (s *SkillManager) DecreaseLevelMax(skill, amount int) {
	s.DecreaseCurrentLevel(skill, amount, s.MaxLevel(skill)-amount)
}

// IsBoosted ports SkillManager.isBoosted (SkillManager.ts:618-620):
// the current level is above the base level.
func (s *SkillManager) IsBoosted(skill int) bool {
	return s.CurrentLevel(skill) > s.MaxLevel(skill)
}

// Skillable is the Go form of the player's current skill action
// (Player.ts:204, `private skill: any`, read and written through
// getSkill/setSkill, Player.ts:1590-1597). TS types the slot as
// `any`: content sets it both with full startable skills and
// with bare `{cancel}` objects (Prayer.plugin.js burying task),
// so the slot interface carries only what stopSkillable needs —
// Cancel. Startable skills implement StartableSkill.
type Skillable interface {
	// Cancel stops the action for player (SkillManager.ts:645:
	// `this.player.getSkill().cancel(this.player)`).
	Cancel(player *Player)
}

// StartableSkill is a skill action startSkillable can start
// (SkillManager.ts:626-641): a Skillable with requirements to
// check and a start to run. Content's Task-shaped skills
// (FurnitureBuild in DaddysHome.Quest.js: hasRequirements /
// start / cancel) satisfy it directly.
type StartableSkill interface {
	Skillable
	// HasRequirements reports whether player may start the
	// action (SkillManager.ts:636).
	HasRequirements(player *Player) bool
	// Start begins the action for player (SkillManager.ts:640).
	Start(player *Player)
}

// GetSkill returns the player's current skill action, or nil when
// none is running (Player.getSkill, Player.ts:1590-1592). A nil
// player has no slot. Tick-goroutine-only, like the rest of
// Player.
func (p *Player) GetSkill() Skillable {
	if p == nil {
		return nil
	}
	return p.skill
}

// SetSkill installs skill as the player's current skill action,
// or clears the slot when skill is nil (Player.setSkill,
// Player.ts:1594-1596). Callers pass an untyped nil to clear: a
// typed-nil pointer in the interface would read non-nil and
// panic on Cancel. A nil player records nothing.
// Tick-goroutine-only, like the rest of Player.
func (p *Player) SetSkill(skill Skillable) {
	if p == nil {
		return
	}
	p.skill = skill
}

// GetCreationMenu returns the player's open creation menu, or nil
// when none is open (Player.getCreationMenu, Player.ts:1598-1600).
// A nil player has no menu. Tick-goroutine-only, like the rest of
// Player.
func (p *Player) GetCreationMenu() *CreationMenu {
	if p == nil {
		return nil
	}
	return p.creationMenu
}

// SetCreationMenu installs menu as the player's open creation
// menu, or clears the slot when menu is nil
// (Player.setCreationMenu, Player.ts:1602-1603). A nil player
// records nothing. Tick-goroutine-only, like the rest of Player.
func (p *Player) SetCreationMenu(menu *CreationMenu) {
	if p == nil {
		return
	}
	p.creationMenu = menu
}

// StartSkillables ports SkillManager.startSkillables
// (SkillManager.ts:622-624): it always returns false in TS. The TS
// parameter is a GameObject; no GameObject type exists in Go, so
// the Go form takes no arguments. No skillable content exists;
// nothing is started.
func (s *SkillManager) StartSkillables() bool {
	return false
}

// StartSkillable ports SkillManager.startSkillable
// (SkillManager.ts:626-641): stop the previous action, close the
// player's interfaces, check the requirements, set the slot, and
// start the skill. The interface-removal step goes through the
// DialogueSender seam (dialogue_entries.go), following the
// trading/dialogue precedent: a session without a DialogueSender
// (a session-less player) closes nothing. A nil skill stops the
// previous action and closes interfaces, then returns: TS would
// throw calling hasRequirements on null, and Go must not panic
// the tick goroutine. A manager with no player does nothing.
func (s *SkillManager) StartSkillable(skill StartableSkill) {
	if s == nil || s.player == nil {
		return
	}
	// Stop previous skills..
	s.StopSkillable()
	// Close interfaces..
	if sender := s.player.dialogueSender(); sender != nil {
		sender.SendInterfaceRemoval()
	}
	if skill == nil {
		return
	}
	// Check if we have the requirements to start this skill..
	if !skill.HasRequirements(s.player) {
		return
	}
	// Start the skill..
	s.player.SetSkill(skill)
	skill.Start(s.player)
}

// StopSkillable ports SkillManager.stopSkillable
// (SkillManager.ts:643-649): cancel the player's current skill, if
// any, and clear the skill and creation menu. The cancel runs
// before the clear, as in TS, so a Cancel that installs a new
// action still has it cleared. A manager with no player, or an
// empty slot, does nothing.
func (s *SkillManager) StopSkillable() {
	if s == nil || s.player == nil {
		return
	}
	if current := s.player.GetSkill(); current != nil {
		current.Cancel(s.player)
	}
	s.player.SetSkill(nil)
	s.player.SetCreationMenu(nil)
}

// Snapshot returns defensive copies of the RAW stored arrays, in
// (level, maxLevel, experience) order, for the save. It is the Go
// form of the fromPlayer clone (PlayerSave.ts:802-806), which
// copies the live Skills arrays as stored — no normalization, no
// XP floor, no sanitize write-back beyond what the setters already
// applied on write. Callers may mutate the returned slices freely.
// (TS getSkills, SkillManager.ts:651-653, returns the live Skills
// object; Snapshot's copies are the ownership-safe Go equivalent,
// and the save path is its only consumer.)
func (s *SkillManager) Snapshot() (level, maxLevel, xp []int) {
	level = append([]int(nil), s.level...)
	maxLevel = append([]int(nil), s.maxLevel...)
	xp = append([]int(nil), s.experience...)
	return level, maxLevel, xp
}

// SetSkills ports SkillManager.setSkills (SkillManager.ts:655-684)
// exactly, including its normalization. It is AUTHORITATIVE for
// live state: the session package's bootstrapSkills duplicates
// this normalization for the bootstrap snapshot (bootstrap.go),
// and must not be refactored to call this (the session package
// never imports game; the bootstrap reads the save, not live
// state). SetSkills sends no updates, as TS does not.
//
// Normalization, per skill: experience is sanitized (<= 0 -> 0,
// >= 200M -> 200M); the base level is at least the level the XP
// implies; a missing current level rests at the base level; a
// missing base level is the XP-implied level; a missing experience
// is 0. Hitpoints keeps at least level 10 and at least 1154 XP
// (getExperienceForLevel(10), EXP_ARRAY[9]) — the 1154 floor, NOT
// the constructor's 1184. Entries past SkillCount are ignored; a
// nil or short slice reads as missing entries (the TS NaN path:
// a non-array or missing element normalizes to the defaults
// above). A nil Skills object in TS returns early with no change;
// Go has no null-object form — nil slices mean "all entries
// missing", which normalizes to the fresh-Skills shape.
func (s *SkillManager) SetSkills(level, maxLevel, xp []int) {
	nextLevel := make([]int, SkillCount)
	nextMax := make([]int, SkillCount)
	nextExp := make([]int, SkillCount)
	hpMinXP := ExperienceForLevel(10)
	if hpMinXP < 0 {
		hpMinXP = 0
	}
	for i := 0; i < SkillCount; i++ {
		// The TS NaN path: a missing element (short array) reads
		// as NaN, which sanitizes to 0 XP, implies the base
		// level from XP alone, and rests the current level at
		// the base level.
		exp := 0
		if i < len(xp) {
			exp = sanitizeExperienceValue(xp[i])
		}
		maxFromExp := LevelForExperience(exp)
		if maxFromExp < 1 {
			maxFromExp = 1
		}
		max := maxFromExp
		if i < len(maxLevel) && maxLevel[i] > max {
			max = maxLevel[i]
		}
		current := max
		if i < len(level) {
			current = level[i]
			if current < 0 {
				current = 0
			}
		}
		if i == SkillHitpoints {
			if exp < hpMinXP {
				exp = hpMinXP
			}
			if max < 10 {
				max = 10
			}
			if current < 10 {
				current = 10
			}
		}
		nextExp[i] = exp
		nextMax[i] = max
		nextLevel[i] = current
	}
	s.level = nextLevel
	s.maxLevel = nextMax
	s.experience = nextExp
}

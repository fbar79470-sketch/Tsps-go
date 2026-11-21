package game

// Prayer: the combat-facing surface of PrayerHandler.ts (739
// lines). Ported: the 30-prayer state array, activation with
// conflict groups, protection-prayer reads, the head hint, the
// drain loop, and the Deadeye / Mystic Vigour upgrades. NOT
// ported: the prayer-book interface (tab clicks, quick-prayer
// selection UI, varbit glow sends — milestone 8, interface
// runtime), the prayer:activated plugin event, and sounds.
// Activation is driven by content/listeners calling
// ActivatePrayer; nothing in core activates a prayer on its
// own. Prayer state is SESSION-ONLY: active prayers are not
// saved (only the quick-prayer selection persists, already
// handled in bootstrap); see the save finding in combat.go.
//
// TS stores the prayer array on Mobile (Mobile.ts
// getPrayerActive, lazy). Go stores it on Combat — the only
// per-Mobile holder available without editing entity.go —
// reached through prayerStateFor. The deviation is invisible
// to callers: every read/write goes through the helpers here.

// Prayer ids (PrayerHandler.ts, Prayer enum order).
const (
	PrayerThickSkin           = 0
	PrayerBurstOfStrength     = 1
	PrayerClarityOfThought    = 2
	PrayerSharpEye            = 3
	PrayerMysticWill          = 4
	PrayerRockSkin            = 5
	PrayerSuperhumanStrength  = 6
	PrayerImprovedReflexes    = 7
	PrayerRapidRestore        = 8
	PrayerRapidHeal           = 9
	PrayerProtectItem         = 10
	PrayerHawkEye             = 11
	PrayerMysticLore          = 12
	PrayerSteelSkin           = 13
	PrayerUltimateStrength    = 14
	PrayerIncredibleReflexes  = 15
	PrayerProtectFromMagic    = 16
	PrayerProtectFromMissiles = 17
	PrayerProtectFromMelee    = 18
	PrayerEagleEye            = 19
	PrayerMysticMight         = 20
	PrayerRetribution         = 21
	PrayerRedemption          = 22
	PrayerSmite               = 23
	PrayerPreserve            = 24
	PrayerChivalry            = 25
	PrayerPiety               = 26
	PrayerRigour              = 27
	PrayerAugury              = 28
	PrayerZeal                = 29
)

// PrayerCount is the size of the prayer array (Prayer.values()
// length, 30).
const PrayerCount = 30

// prayerData is one row of PrayerData (PrayerHandler.ts): the
// level requirement, the drain effect, the button id, and the
// config id. Button/config ids are carried for grounding; Go
// sends no prayer interface packets (milestone 8).
type prayerData struct {
	requirement int
	drainEffect int
	buttonID    int
	configID    int
	hint        int // head-hint value, -1 when none
}

var prayerTable = [PrayerCount]prayerData{
	PrayerThickSkin:           {1, 1, 5609, 4104, -1},
	PrayerBurstOfStrength:     {4, 1, 5610, 4105, -1},
	PrayerClarityOfThought:    {7, 1, 5611, 4106, -1},
	PrayerSharpEye:            {8, 1, 19812, 4122, -1},
	PrayerMysticWill:          {9, 1, 19814, 4123, -1},
	PrayerRockSkin:            {10, 6, 5612, 4107, -1},
	PrayerSuperhumanStrength:  {13, 6, 5613, 4108, -1},
	PrayerImprovedReflexes:    {16, 6, 5614, 4109, -1},
	PrayerRapidRestore:        {19, 1, 5615, 4110, -1},
	PrayerRapidHeal:           {22, 2, 5616, 4111, -1},
	PrayerProtectItem:         {25, 2, 5617, 4112, -1},
	PrayerHawkEye:             {26, 6, 19816, 4124, -1},
	PrayerMysticLore:          {27, 6, 19818, 4125, -1},
	PrayerSteelSkin:           {28, 12, 5618, 4113, -1},
	PrayerUltimateStrength:    {31, 12, 5619, 4114, -1},
	PrayerIncredibleReflexes:  {34, 12, 5620, 4115, -1},
	PrayerProtectFromMagic:    {37, 12, 5621, 4116, 2},
	PrayerProtectFromMissiles: {40, 12, 5622, 4117, 1},
	PrayerProtectFromMelee:    {43, 12, 5623, 4118, 0},
	PrayerEagleEye:            {44, 12, 19821, 4126, -1},
	PrayerMysticMight:         {45, 12, 19823, 4127, -1},
	PrayerRetribution:         {46, 3, 683, 4119, 4},
	PrayerRedemption:          {49, 6, 684, 4120, 5},
	PrayerSmite:               {52, 18, 685, 4121, 6},
	PrayerPreserve:            {55, 2, 28001, 5466, -1},
	PrayerChivalry:            {60, 12, 19825, 4128, -1},
	PrayerPiety:               {70, 24, 19827, 4129, -1},
	PrayerRigour:              {74, 24, 28004, 5464, -1},
	PrayerAugury:              {77, 24, 28007, 5465, -1},
	PrayerZeal:                {84, 24, -1, 16577, -1},
}

// Prayer conflict groups (PrayerHandler.ts): activating a
// prayer resets the other members of its groups. Overhead
// prayers conflict with each other; the stat groups conflict
// within themselves.
var (
	prayerDefenceGroup    = []int{PrayerThickSkin, PrayerRockSkin, PrayerSteelSkin, PrayerChivalry, PrayerPiety, PrayerRigour, PrayerAugury, PrayerZeal}
	prayerStrengthGroup   = []int{PrayerBurstOfStrength, PrayerSuperhumanStrength, PrayerUltimateStrength, PrayerChivalry, PrayerPiety, PrayerZeal}
	prayerAttackGroup     = []int{PrayerClarityOfThought, PrayerImprovedReflexes, PrayerIncredibleReflexes, PrayerChivalry, PrayerPiety, PrayerZeal}
	prayerRangedGroup     = []int{PrayerSharpEye, PrayerHawkEye, PrayerEagleEye, PrayerRigour}
	prayerMagicGroup      = []int{PrayerMysticWill, PrayerMysticLore, PrayerMysticMight, PrayerAugury}
	prayerOverheadGroup   = []int{PrayerProtectFromMagic, PrayerProtectFromMissiles, PrayerProtectFromMelee, PrayerRetribution, PrayerRedemption, PrayerSmite}
	prayerProtectionGroup = []int{PrayerProtectFromMagic, PrayerProtectFromMissiles, PrayerProtectFromMelee}
)

// prayerGroupsFor returns the conflict groups a prayer belongs
// to (PrayerHandler.ts activatePrayer switch). Chivalry/Piety/
// Zeal sit in defence, strength, and attack groups; Rigour in
// defence and ranged; Augury in defence and magic.
func prayerGroupsFor(prayer int) [][]int {
	switch prayer {
	case PrayerThickSkin, PrayerRockSkin, PrayerSteelSkin:
		return [][]int{prayerDefenceGroup}
	case PrayerBurstOfStrength, PrayerSuperhumanStrength, PrayerUltimateStrength:
		return [][]int{prayerStrengthGroup}
	case PrayerClarityOfThought, PrayerImprovedReflexes, PrayerIncredibleReflexes:
		return [][]int{prayerAttackGroup}
	case PrayerSharpEye, PrayerHawkEye, PrayerEagleEye:
		return [][]int{prayerRangedGroup}
	case PrayerMysticWill, PrayerMysticLore, PrayerMysticMight:
		return [][]int{prayerMagicGroup}
	case PrayerChivalry, PrayerPiety, PrayerZeal:
		return [][]int{prayerDefenceGroup, prayerStrengthGroup, prayerAttackGroup}
	case PrayerRigour:
		return [][]int{prayerDefenceGroup, prayerRangedGroup}
	case PrayerAugury:
		return [][]int{prayerDefenceGroup, prayerMagicGroup}
	case PrayerProtectFromMagic, PrayerProtectFromMissiles, PrayerProtectFromMelee,
		PrayerRetribution, PrayerRedemption, PrayerSmite:
		return [][]int{prayerOverheadGroup}
	default:
		return nil
	}
}

// PrayerRunOutMessage is RUN_OUT_MESSAGE (PrayerHandler.ts):
// sent when the drain empties the Prayer level.
const PrayerRunOutMessage = "You have run out of prayer points, you can recharge at an altar."

// PrayerState is one fighter's prayer state: the active array,
// the drain accumulator, and the drain flags (PrayerHandler.ts
// fields on Mobile/Player: prayerActive, pointDrain, draining,
// activatedThisTick).
type PrayerState struct {
	active            [PrayerCount]bool
	pointDrain        float64
	draining          bool
	activatedThisTick bool
}

// prayerStateFor returns the PrayerState for m, allocating it
// on first use (Mobile.ts getPrayerActive lazy shape). Nil
// Mobile yields a throwaway state (reads false, writes lost);
// callers that need persistence pass a real Mobile.
func prayerStateFor(m *Mobile) *PrayerState {
	if m == nil {
		return &PrayerState{}
	}
	c := m.GetCombat()
	if c.prayer == nil {
		c.prayer = &PrayerState{}
	}
	return c.prayer
}

// PrayerActive reports whether prayer is active for m
// (PrayerHandler.isActivated). Out-of-range ids answer false.
func PrayerActive(m *Mobile, prayer int) bool {
	if prayer < 0 || prayer >= PrayerCount {
		return false
	}
	return prayerStateFor(m).active[prayer]
}

// ProtectingPrayer returns the protection prayer id for a
// combat type (PrayerHandler.getProtectingPrayer): MELEE -> 18,
// MAGIC -> 16, RANGED -> 17. TS throws for other types; Go has
// only the three.
func ProtectingPrayer(t CombatType) int {
	switch t {
	case CombatTypeMagic:
		return PrayerProtectFromMagic
	case CombatTypeRanged:
		return PrayerProtectFromMissiles
	default:
		return PrayerProtectFromMelee
	}
}

// IsProtectingAgainst reports whether m has the protection
// prayer for combat type t active.
func IsProtectingAgainst(m *Mobile, t CombatType) bool {
	return PrayerActive(m, ProtectingPrayer(t))
}

// HeadHint returns the overhead-prayer head hint for m
// (PrayerHandler.getHeadHint): melee 0, missiles 1, magic 2,
// retribution 3, smite 4, redemption 5, else -1. The check
// order is the TS order; conflicts keep at most one overhead
// prayer active.
func HeadHint(m *Mobile) int {
	s := prayerStateFor(m)
	switch {
	case s.active[PrayerProtectFromMelee]:
		return 0
	case s.active[PrayerProtectFromMissiles]:
		return 1
	case s.active[PrayerProtectFromMagic]:
		return 2
	case s.active[PrayerRetribution]:
		return 3
	case s.active[PrayerSmite]:
		return 4
	case s.active[PrayerRedemption]:
		return 5
	default:
		return -1
	}
}

// IsDeadeye reports whether m has the Deadeye upgrade: Eagle
// Eye active and the prayer:deadeye-unlocked attribute strictly
// true (PrayerHandler.ts Deadeye handling; varbit 16090, level
// 62 — the varbit send is milestone 8).
func IsDeadeye(m *Mobile) bool {
	return PrayerActive(m, PrayerEagleEye) && m != nil && m.GetAttribute("prayer:deadeye-unlocked") == true
}

// IsMysticVigour reports whether m has the Mystic Vigour
// upgrade: Mystic Might active and the
// prayer:mystic-vigour-unlocked attribute strictly true
// (varbit 16091, level 63).
func IsMysticVigour(m *Mobile) bool {
	return PrayerActive(m, PrayerMysticMight) && m != nil && m.GetAttribute("prayer:mystic-vigour-unlocked") == true
}

// isProtectionPrayer reports whether prayer is one of the
// three protection prayers (16/17/18).
func isProtectionPrayer(prayer int) bool {
	return prayer == PrayerProtectFromMagic || prayer == PrayerProtectFromMissiles || prayer == PrayerProtectFromMelee
}

// ActivatePrayer activates prayer for player
// (PrayerHandler.activatePrayer): the conflict groups reset,
// the drain starts, and the head hint updates. It returns
// false (with a message) when the prayer is disabled by
// content, when a protection prayer is blocked by the prayer
// block, or when the player has no Prayer points. The
// per-prayer LEVEL requirements in TS canUse (Prayer level,
// and Defence 70 for some prayers) are NOT checked here —
// they are enforced by the interface layer (milestone 8),
// which does not exist yet. Sounds, varbit glow, and the
// prayer:activated event are not ported.
func ActivatePrayer(player *Player, prayer int) bool {
	if player == nil || prayer < 0 || prayer >= PrayerCount {
		return false
	}
	if Hooks.PrayerDisabled != nil && Hooks.PrayerDisabled(player, prayer) {
		return false
	}
	// PrayerHandler.canUse: while the prayer block (from
	// Sever / disableProtectionPrayers) is active, protection
	// prayers cannot activate.
	if isProtectionPrayer(prayer) && !player.GetCombat().PrayerBlockFinished() {
		player.SendMessage("You have been disabled and can no longer use protection prayers.")
		return false
	}
	if player.Skills().CurrentLevel(SkillPrayer) <= 0 {
		player.SendMessage("You do not have enough Prayer points.")
		return false
	}
	// PrayerHandler.canUse, last check (PrayerHandler.ts:391-397):
	// in a duel with the NO_PRAYER rule, no prayer activates.
	// TS sends only the varbit glow (interface layer, not
	// ported) and a commented-out statement dialogue (not
	// ported), so the refusal is silent here.
	if dueling := player.Dueling(); dueling.InDuel() && dueling.RuleSet(DuelRuleNoPrayer) {
		return false
	}
	s := prayerStateFor(&player.Mobile)
	for _, group := range prayerGroupsFor(prayer) {
		for _, other := range group {
			if other != prayer {
				s.active[other] = false
			}
		}
	}
	s.active[prayer] = true
	s.activatedThisTick = true
	s.draining = true
	syncPrayerHeadHint(player)
	return true
}

// TogglePrayer toggles one prayer for player
// (PrayerHandler.togglePrayer, PrayerHandler.ts:31-36): an
// active prayer deactivates, an inactive one activates. The
// TS form takes a button child id and maps it to a prayer id
// through the prayer-book interface data; this port takes the
// prayer id directly (the button mapping is interface
// runtime, milestone 8).
func TogglePrayer(player *Player, prayer int) bool {
	if player == nil || prayer < 0 || prayer >= PrayerCount {
		return false
	}
	if PrayerActive(&player.Mobile, prayer) {
		DeactivatePrayer(player, prayer)
		return true
	}
	return ActivatePrayer(player, prayer)
}

// DeactivatePrayer deactivates one prayer for player
// (PrayerHandler.deactivatePrayer), updating the head hint.
// When no prayer remains active the drain stops.
func DeactivatePrayer(player *Player, prayer int) {
	if player == nil || prayer < 0 || prayer >= PrayerCount {
		return
	}
	s := prayerStateFor(&player.Mobile)
	s.active[prayer] = false
	if !s.anyActive() {
		s.stopDrain()
	}
	syncPrayerHeadHint(player)
}

// DeactivatePrayers deactivates every prayer for m
// (PrayerHandler.deactivatePrayers / resetPrayer), stopping
// the drain. Used by death, disableProtectionPrayers, and the
// drain-empty path.
func DeactivatePrayers(m *Mobile) {
	if m == nil {
		return
	}
	s := prayerStateFor(m)
	for i := range s.active {
		s.active[i] = false
	}
	s.stopDrain()
	if p := combatantFor(m).Player(); p != nil {
		syncPrayerHeadHint(p)
	}
}

// DeactivateProtectionPrayers resets only the three protection
// prayers (PrayerHandler.resetProtectionPrayers, used by
// disableProtectionPrayers).
func DeactivateProtectionPrayers(m *Mobile) {
	if m == nil {
		return
	}
	s := prayerStateFor(m)
	for _, p := range prayerProtectionGroup {
		s.active[p] = false
	}
	if !s.anyActive() {
		s.stopDrain()
	}
	if pl := combatantFor(m).Player(); pl != nil {
		syncPrayerHeadHint(pl)
	}
}

func (s *PrayerState) anyActive() bool {
	for _, a := range s.active {
		if a {
			return true
		}
	}
	return false
}

func (s *PrayerState) stopDrain() {
	s.draining = false
	s.pointDrain = 0
}

// StartDrain starts the drain loop (PrayerHandler.startDrain).
// ActivatePrayer calls it implicitly; content that sets prayer
// state directly calls it explicitly.
func StartDrain(m *Mobile) {
	if m == nil {
		return
	}
	prayerStateFor(m).draining = true
}

// ProcessDrain runs one drain tick for player
// (PrayerHandler.processDrain, called from the player turn):
// the activation tick is free; an empty Prayer level
// deactivates everything with the run-out message; otherwise
// the active drain effects accumulate against the resistance
// (60 + 2 x prayer bonus, minimum 1) and whole points are
// subtracted. The coordinator wires this into Player.Process
// (player.go is owned by other work — see the report). NPCs
// never drain: TS drains players only.
func ProcessDrain(player *Player) {
	if player == nil {
		return
	}
	s := prayerStateFor(&player.Mobile)
	if !s.draining {
		return
	}
	if s.activatedThisTick {
		// The tick a prayer is activated is free
		// (PrayerHandler.ts: the activated-this-tick consume).
		s.activatedThisTick = false
		return
	}
	skills := player.Skills()
	if skills.CurrentLevel(SkillPrayer) <= 0 {
		DeactivatePrayers(&player.Mobile)
		player.SendMessage(PrayerRunOutMessage)
		return
	}
	drainEffect := 0
	for i, active := range s.active {
		if active {
			drainEffect += prayerTable[i].drainEffect
		}
	}
	if combatantFor(&player.Mobile).Hitpoints() <= 0 || drainEffect <= 0 {
		s.stopDrain()
		return
	}
	// Resistance: max(1, 60 + 2 x prayer bonus)
	// (PrayerHandler.ts processDrain).
	resistance := 60 + 2*combatantFor(&player.Mobile).OtherBonus(OtherBonusPrayer)
	if resistance < 1 {
		resistance = 1
	}
	// Plugin drain-rate modifiers are identity in Go (no
	// hook field: the rate event is content-only).
	s.pointDrain += float64(drainEffect)
	threshold := int(s.pointDrain/float64(resistance) + 1e-9)
	if threshold >= 1 {
		skills.DecreaseCurrentLevel(SkillPrayer, threshold, 0)
		s.pointDrain -= float64(threshold * resistance)
		if s.pointDrain < 0 {
			s.pointDrain = 0
		}
	}
}

// DrainPrayer subtracts amount Prayer levels directly
// (PrayerHandler.drainPrayer, used by Smite): floored at 0,
// no drain-loop interaction.
func DrainPrayer(player *Player, amount int) {
	if player == nil || amount <= 0 {
		return
	}
	player.Skills().DecreaseCurrentLevel(SkillPrayer, amount, 0)
}

// syncPrayerHeadHint pushes the head hint into the player's
// appearance (PrayerHandler.ts: the appearance head hint is
// how overhead prayers reach the client). Appearance exists
// for every player; a nil appearance is tolerated.
func syncPrayerHeadHint(player *Player) {
	if player == nil {
		return
	}
	if app := player.Appearance(); app != nil {
		app.SetHeadHint(HeadHint(&player.Mobile))
	}
}

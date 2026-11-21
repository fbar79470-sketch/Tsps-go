package game

// Dueling: the port of game/content/Duelling.ts (57 lines) —
// the duel state class, the duel rule constants, and the duel
// state enum.
//
// Only the state is ported. The Duel Arena's gameplay (the
// duel window, stakes, the arena match, forfeit) is plugin
// content and is NOT STARTED. What core consumes is the
// state: Dueling.InDuel() and the rules drive the movement
// gate (NO_MOVEMENT), the combat rule checks (NO_MELEE,
// NO_RANGED, NO_MAGIC, the opponent checks), and the prayer
// gate (NO_PRAYER). Those consumers are wired in
// movement_queue.go, combat_factory.go, and combat_prayer.go.
//
// STORAGE. One Dueling per player, reached through
// Player.Dueling(). TS constructs it eagerly as a Player field
// (Player.ts:142). The Player struct lives outside this phase's
// ownership, so — as with Trading — the instance is held in
// the player's attribute store under a NUL-prefixed key that
// content can never address, allocated lazily on first access
// under the store's own mutex. The key is never registered
// persistent, so the save snapshot can never see it.

// DuelState is Duelling.ts's DuelState enum. The order matters:
// InDuel is a range check (STARTING_DUEL through IN_DUEL), and
// the combat permission check requires IN_DUEL exactly.
type DuelState int

const (
	// DuelStateNone: no duel (NONE).
	DuelStateNone DuelState = iota
	// DuelStateRequestedDuel: a duel request is outstanding
	// (REQUESTED_DUEL).
	DuelStateRequestedDuel
	// DuelStateDuelScreen: the duel setup screen (DUEL_SCREEN).
	DuelStateDuelScreen
	// DuelStateAcceptedDuelScreen: the duel setup screen,
	// accepted (ACCEPTED_DUEL_SCREEN).
	DuelStateAcceptedDuelScreen
	// DuelStateConfirmScreen: the confirm screen
	// (CONFIRM_SCREEN).
	DuelStateConfirmScreen
	// DuelStateAcceptedConfirmScreen: the confirm screen,
	// accepted (ACCEPTED_CONFIRM_SCREEN).
	DuelStateAcceptedConfirmScreen
	// DuelStateStartingDuel: the duel is starting
	// (STARTING_DUEL).
	DuelStateStartingDuel
	// DuelStateInDuel: the duel is in progress (IN_DUEL).
	DuelStateInDuel
)

// DuelRule is one Duel Arena rule (DuelRule, Duelling.ts:30-56).
// The bit is the rule's index in the rules array and its
// config bit; the equipment slot is the slot the rule
// restricts, or -1 for a rule with no equipment slot.
type DuelRule struct {
	bit           int
	equipmentSlot int
}

// Duel rules (Duelling.ts:7-30). The bit doubles as the
// button id (getButtonId returns the bit) and the config id
// is 1<<bit (getConfigId). Equipment slots are the equipment
// slot constants in equipment.go.
var (
	// DuelRuleNoRanged disables ranged attacks (bit 4).
	DuelRuleNoRanged = DuelRule{bit: 4, equipmentSlot: -1}
	// DuelRuleNoMelee disables melee attacks (bit 5).
	DuelRuleNoMelee = DuelRule{bit: 5, equipmentSlot: -1}
	// DuelRuleNoMagic disables magic attacks (bit 6).
	DuelRuleNoMagic = DuelRule{bit: 6, equipmentSlot: -1}
	// DuelRuleNoSpecialAttacks disables special attacks
	// (bit 13).
	DuelRuleNoSpecialAttacks = DuelRule{bit: 13, equipmentSlot: -1}
	// DuelRuleLockWeapon locks the weapon slot (bit 2).
	DuelRuleLockWeapon = DuelRule{bit: 2, equipmentSlot: -1}
	// DuelRuleNoForfeit disables forfeiting (bit 0).
	DuelRuleNoForfeit = DuelRule{bit: 0, equipmentSlot: -1}
	// DuelRuleNoPotions disables potions (bit 7).
	DuelRuleNoPotions = DuelRule{bit: 7, equipmentSlot: -1}
	// DuelRuleNoFood disables food (bit 8).
	DuelRuleNoFood = DuelRule{bit: 8, equipmentSlot: -1}
	// DuelRuleNoPrayer disables prayer (bit 9).
	DuelRuleNoPrayer = DuelRule{bit: 9, equipmentSlot: -1}
	// DuelRuleNoMovement disables movement (bit 1).
	DuelRuleNoMovement = DuelRule{bit: 1, equipmentSlot: -1}
	// DuelRuleFunWeapons allows only fun weapons (bit 12).
	DuelRuleFunWeapons = DuelRule{bit: 12, equipmentSlot: -1}
	// DuelRuleShowInventories shows both inventories (bit 3).
	DuelRuleShowInventories = DuelRule{bit: 3, equipmentSlot: -1}
	// DuelRuleNoHelm disables the helm slot (bit 14).
	DuelRuleNoHelm = DuelRule{bit: 14, equipmentSlot: HeadSlot}
	// DuelRuleNoCape disables the cape slot (bit 15).
	DuelRuleNoCape = DuelRule{bit: 15, equipmentSlot: CapeSlot}
	// DuelRuleNoAmulet disables the amulet slot (bit 16).
	DuelRuleNoAmulet = DuelRule{bit: 16, equipmentSlot: AmuletSlot}
	// DuelRuleNoAmmunition disables the ammunition slot
	// (bit 27).
	DuelRuleNoAmmunition = DuelRule{bit: 27, equipmentSlot: AmmunitionSlot}
	// DuelRuleNoWeapon disables the weapon slot (bit 17).
	DuelRuleNoWeapon = DuelRule{bit: 17, equipmentSlot: WeaponSlot}
	// DuelRuleNoBody disables the body slot (bit 18).
	DuelRuleNoBody = DuelRule{bit: 18, equipmentSlot: BodySlot}
	// DuelRuleNoShield disables the shield slot (bit 19).
	DuelRuleNoShield = DuelRule{bit: 19, equipmentSlot: ShieldSlot}
	// DuelRuleNoLegs disables the leg slot (bit 21).
	DuelRuleNoLegs = DuelRule{bit: 21, equipmentSlot: LegSlot}
	// DuelRuleNoRing disables the ring slot (bit 26).
	DuelRuleNoRing = DuelRule{bit: 26, equipmentSlot: RingSlot}
	// DuelRuleNoBoots disables the boots slot (bit 24).
	DuelRuleNoBoots = DuelRule{bit: 24, equipmentSlot: FeetSlot}
	// DuelRuleNoGloves disables the gloves slot (bit 23).
	DuelRuleNoGloves = DuelRule{bit: 23, equipmentSlot: HandsSlot}
)

// ButtonID returns the rule's button id: its bit
// (getButtonId, Duelling.ts:37-39).
func (r DuelRule) ButtonID() int { return r.bit }

// ConfigID returns the rule's config id: 1<<bit (getConfigId,
// Duelling.ts:41-43).
func (r DuelRule) ConfigID() int { return 1 << uint(r.bit) }

// EquipmentSlot returns the equipment slot the rule restricts,
// or -1 for a rule with no equipment slot (getEquipmentSlot,
// Duelling.ts:45-47).
func (r DuelRule) EquipmentSlot() int { return r.equipmentSlot }

// duellingRulesLen is the rules array's length (Duelling.ts:17):
// 28 bits, one per rule position.
const duellingRulesLen = 28

// duellingAttributeKey is the attribute-store key holding a
// player's Dueling (see the file comment).
const duellingAttributeKey = "\x00duelling-state"

// Dueling is one player's duel state (Dueling,
// Duelling.ts:10-24).
type Dueling struct {
	state       DuelState
	interact    *Player
	rules       []bool
	buttonDelay *SecondsTimer
}

// NewDueling returns a Dueling in state NONE with no interact
// and every rule off (Dueling, Duelling.ts:16-20).
func NewDueling() *Dueling {
	return &Dueling{
		state:       DuelStateNone,
		rules:       make([]bool, duellingRulesLen),
		buttonDelay: newSecondsTimer(),
	}
}

// Dueling returns the player's Dueling, allocating it on first
// access (see the file comment for the storage decision). A nil
// player yields nil. The get-or-create runs under the attribute
// store's own mutex, so concurrent first access cannot fork the
// instance.
func (p *Player) Dueling() *Dueling {
	if p == nil {
		return nil
	}
	s := p.store(true)
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.values[duellingAttributeKey]; ok {
		if d, ok := v.(*Dueling); ok && d != nil {
			return d
		}
	}
	d := NewDueling()
	s.values[duellingAttributeKey] = d
	return d
}

// State returns the duel state (getState, Duelling.ts:26-28).
func (d *Dueling) State() DuelState { return d.state }

// SetState sets the duel state (setState, Duelling.ts:32-34).
func (d *Dueling) SetState(state DuelState) { d.state = state }

// Interact returns the duel opponent, or nil (getInteract,
// Duelling.ts:38-40).
func (d *Dueling) Interact() *Player { return d.interact }

// SetInteract sets the duel opponent (setInteract,
// Duelling.ts:44-46).
func (d *Dueling) SetInteract(interact *Player) { d.interact = interact }

// Rules returns the LIVE rules array (getRules,
// Duelling.ts:50-52): callers mutate the array in place
// (rules[bit] = ..., rules.fill(false)) and the mutation is
// the duel's state. The slice must never be copied on return.
func (d *Dueling) Rules() []bool { return d.rules }

// RuleSet reports whether rule is enabled, reading the live
// array at the rule's bit. TS has no such accessor — callers
// index getRules() directly — but the movement and combat
// gates read rules through this single site so the bit
// indexing is stated once.
func (d *Dueling) RuleSet(rule DuelRule) bool {
	return d.rules[rule.bit]
}

// ButtonDelay returns the duel-button delay timer
// (getButtonDelay, Duelling.ts:56-58). The movement mobility
// gate reads it.
func (d *Dueling) ButtonDelay() *SecondsTimer { return d.buttonDelay }

// InDuel reports whether the player is in a duel
// (inDuel, Duelling.ts:22-24): state STARTING_DUEL or IN_DUEL.
func (d *Dueling) InDuel() bool {
	return d.state == DuelStateStartingDuel || d.state == DuelStateInDuel
}

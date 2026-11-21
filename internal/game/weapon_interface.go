package game

// Weapon interface manager: the port of
// game/content/combat/WeaponInterfaces.ts (87 lines) and
// game/content/combat/WeaponInterfaceManager.ts (98 lines).
// WeaponInterfaces.ts itself is only the registry half — the
// registry lives in combat_styles.go (CombatStyleRegistry);
// this file is the manager: weapon-category interface
// selection, the equip/change sends, style-button handling,
// and the auto-retaliate button + varp.
//
// The manager is stateless: the player's current weapon
// interface is recomputed from the equipped item on every
// call (TS keeps it on Player.weapon via setWeapon, but Go's
// Player carries no weapon-interface field, and every use in
// TS re-derives it from the equipment except the two reads
// below, which resolve it the same way here). The only
// persistent state is the fight type on Combat (SetFightType)
// and the per-category style memory attribute
// (combat:style-by-category).
//
// What is NOT here (named gaps, all Worker C or later
// phases):
//   - CombatSpecial.updateBar's weapon-meter tail
//     (WeaponInterfaceManager.assign): in TS it goes through
//     the no-op PlayerSession.write (see special_restore.go),
//     so nothing is sent. The orb half (varps 300/301) IS
//     sent from AssignWeaponInterface, below.
//   - Autocasting.setAutocast (resetWeapon's wilderness /
//     staff re-cast halves, Autocasting.ts): the set/clear/
//     selector paths live in autocasting.go (Phase 27);
//     clearForStyle / refreshIndicators live here because
//     they need only the autocast spell slot
//     (Combat.AutoCastSpell) and the staff check, and
//     SetAutocast calls RefreshAutocastIndicators.
//   - The equip listener's resetWeapon special-deactivate half
//     (setSpecialActivated(false) on equip) is NOT ported:
//     it belongs to the special-attack activation system
//     (Worker C).

// CombatStyleSender is the outbound sender surface
// WeaponInterfaceManager.assign/changeCombatStyle use, after
// Player.SendVarp (varp 43/172, PacketSender.sendConfig).
// It follows the DialogueSender/InterfaceSender precedent in
// this package: small interfaces over p.Session, so a
// session-less player drops the sends. *Session implements
// all four methods.
type CombatStyleSender interface {
	// SendVarbit queues a varbit packet (PacketSender.sendVarbit).
	SendVarbit(id, value int)
	// SendTabInterface mounts an interface in a gameframe tab
	// (PacketSender.sendTabInterface).
	SendTabInterface(tabID, interfaceID int)
	// SendString writes text into a component
	// (PacketSender.sendString).
	SendString(text string, uid int)
	// SendInterfaceDisplayState hides or shows a component
	// (PacketSender.sendInterfaceDisplayState).
	SendInterfaceDisplayState(uid int, hide bool)
}

// combatStyleSender returns the player's CombatStyleSender, or
// nil when the player is nil or the session does not
// implement it (the dialogueSender assertion pattern).
func (p *Player) combatStyleSender() CombatStyleSender {
	if p == nil {
		return nil
	}
	sender, _ := p.Session.(CombatStyleSender)
	return sender
}

// combatStyleButtons are the combat tab's four style buttons
// (593:6, 10, 14, 18), the styles' slots 0-3
// (WeaponInterfaceManager.ts STYLE_BUTTONS).
var combatStyleButtons = [4]int{6, 10, 14, 18}

// CurrentWeaponInterface resolves the equipped weapon's
// interface (WeaponInterfaceManager.assign's resolution):
// the item-definition interface for the wielded item, or
// UNARMED when nothing is wielded, the item has no interface
// name, or the name is unknown. TS reads
// equippedWeapon.getDefinition().getWeaponInterface(); Go
// reads the same name through WeaponInfoSource (definition
// store, from item-gameplay.json).
func CurrentWeaponInterface(p *Player) *WeaponInterface {
	reg := CombatStyles()
	if p == nil {
		return reg.WeaponInterface("UNARMED")
	}
	var src WeaponInfoSource
	if defs := p.ItemDefinitions(); defs != nil {
		src, _ = defs.(WeaponInfoSource)
	}
	if eq := p.Equipment(); eq != nil {
		if weapon := eq.GetWeapon(); weapon.IsValid() {
			if iface := reg.InterfaceForItem(weapon.ID(), src); iface != nil {
				return iface
			}
		}
	}
	return reg.WeaponInterface("UNARMED")
}

// AssignWeaponInterface assigns the combat-tab interface for
// the equipped weapon (WeaponInterfaceManager.assign). The
// send order is TS order:
// varbit 357 (category) → "Weapon: " line for CROSSBOW/WHIP →
// tab 0 mount (which always mounts group 593, ignoring the
// interface id, in both TS PacketSender.sendTabInterface and
// Go SendTabInterface) → varp 172 echo → weapon-name line →
// special-bar show/hide (CombatSpecial.assign) → fight-type
// selection + varp 43 → autocast indicators.
// The fight-type rule is TS verbatim: the remembered slot for
// the category, else the current fight type when no slot was
// ever remembered for the category and it is one of the
// offered styles, else the lowest child id. A nil player is a
// no-op.
func AssignWeaponInterface(p *Player) {
	if p == nil {
		return
	}
	sender := p.combatStyleSender()
	weapon := CurrentWeaponInterface(p)
	if sender != nil {
		sender.SendVarbit(WeaponCategoryVarbit, weapon.Category)
		if weapon.Name == "CROSSBOW" || weapon.Name == "WHIP" {
			sender.SendString("Weapon: ", weapon.NameLineID-1)
		}
		sender.SendTabInterface(0, weapon.InterfaceID)
		p.SendVarp(AutoRetaliateVarp, autoRetaliateVarpValue(p))
		sender.SendString(weaponDisplayName(p, weapon), weapon.NameLineID)
		assignSpecialBar(p, sender, weapon)
	}
	// The spec-orb send (CombatSpecial.updateBar, called by
	// WeaponInterfaceManager.assign right after
	// CombatSpecial.assign, WeaponInterfaceManager.ts:79-80):
	// varp 300 then varp 301, in TS order. This is the login
	// send too: onLogin calls assign (Player.ts:794), and the
	// equip path calls it on every weapon change. A
	// session-less player drops both sends (Player.SendVarp
	// is a nil-session no-op). The weapon-meter tail of
	// updateBar is NOT sent: in TS it goes through the no-op
	// PlayerSession.write (see special_restore.go).
	updateSpecialAttackOrb(&p.Mobile)
	remembered, hasRemembered := RememberedCombatStyleSlot(p, weapon.Category)
	current := p.GetCombat().FightType()
	var fightType *FightType
	switch {
	case hasRemembered:
		fightType = weapon.FightTypeForChild(remembered)
	case !hasRemembered && weapon.HasFightType(current):
		fightType = current
	}
	if fightType == nil {
		fightType = weapon.LowestFightType()
	}
	if fightType == nil {
		fightType = UnarmedPunch
	}
	p.GetCombat().SetFightType(fightType)
	RememberCombatStyle(p, weapon.Category, fightType.ChildID)
	p.SendVarp(CombatStyleVarp, fightType.ChildID)
	RefreshAutocastIndicators(p)
}

// weaponDisplayName is the name assign writes on the weapon
// interface's name line: "Unarmed" with no weapon, else the
// equipped item's definition name (ItemDefinition.getName).
func weaponDisplayName(p *Player, weapon *WeaponInterface) string {
	if weapon != nil && weapon.Name == "UNARMED" {
		return "Unarmed"
	}
	if eq := p.Equipment(); eq != nil {
		if id := eq.GetWeapon().ID(); id > 0 {
			return p.ItemName(id)
		}
	}
	return "Unarmed"
}

// assignSpecialBar is CombatSpecial.assign's show/hide half
// (CombatSpecial.ts): a registered special for the weapon (or
// the shield, which can carry one — Rondache) shows the
// weapon's special bar; with no special the bar hides and the
// armed special clears. A weapon interface with no bar
// (specialBar -1) sends nothing. The orb half of updateBar
// (varps 300/301) is sent by AssignWeaponInterface, after
// this call, in TS order (assign, then updateBar).
func assignSpecialBar(p *Player, sender CombatStyleSender, weapon *WeaponInterface) {
	if weapon == nil || weapon.SpecialBar == -1 {
		return
	}
	eq := p.Equipment()
	if eq == nil {
		return
	}
	special := CombatSpecialForItem(eq.GetWeapon().ID())
	if special == nil {
		special = CombatSpecialForItem(eq.ItemAt(ShieldSlot).ID())
	}
	if special != nil {
		sender.SendInterfaceDisplayState(weapon.SpecialBar, false)
		return
	}
	sender.SendInterfaceDisplayState(weapon.SpecialBar, true)
	p.GetCombat().SetSpecialActivated(false)
}

// autoRetaliateVarpValue is the varp 172 value for the
// player's auto-retaliate setting: %option_nodef is inverted,
// 0 enables auto-retaliate (WeaponInterfaceManager.assign).
func autoRetaliateVarpValue(p *Player) int {
	if p.GetCombat().AutoRetaliate() {
		return 0
	}
	return 1
}

// ChangeCombatStyle changes the fight type to the given slot
// (WeaponInterfaceManager.changeCombatStyle): the slot must be
// an integer 0..3 and name an offered style of the current
// weapon, else false with nothing changed. On success it
// clears autocast for the style, sets the fight type,
// remembers it per category, sends varp 43, and refreshes the
// autocast indicators.
func ChangeCombatStyle(p *Player, slot int) bool {
	if p == nil || slot < 0 || slot > 3 {
		return false
	}
	weapon := CurrentWeaponInterface(p)
	fightType := weapon.FightTypeForChild(slot)
	if fightType == nil {
		return false
	}
	ClearAutocastForStyle(p)
	p.GetCombat().SetFightType(fightType)
	RememberCombatStyle(p, weapon.Category, fightType.ChildID)
	p.SendVarp(CombatStyleVarp, fightType.ChildID)
	RefreshAutocastIndicators(p)
	return true
}

// HandleStyleButton answers a style-button click on the
// combat tab (WeaponInterfaceManager.handleStyleButton): group
// 593, child 6/10/14/18 → slots 0-3. Anything else returns
// false so the click falls through to the next handler.
func HandleStyleButton(p *Player, groupID, childID int) bool {
	if groupID != CombatInterfaceGroup {
		return false
	}
	slot := -1
	for i, child := range combatStyleButtons {
		if child == childID {
			slot = i
			break
		}
	}
	if slot < 0 {
		return false
	}
	return ChangeCombatStyle(p, slot)
}

// HandleAutoRetaliateButton answers the combat tab's
// auto-retaliate button (NetworkBuilder.ts:1035, group 593
// child 32, COMBAT_AUTO_RETALIATE_CHILD): it toggles the
// setting — the button is authoritative, the client does not
// emit a varp change in every cache/client path — stops
// retaliation when turned off, and echoes varp 172. Any
// other click returns false.
func HandleAutoRetaliateButton(p *Player, groupID, childID int) bool {
	if groupID != CombatInterfaceGroup || childID != 32 {
		return false
	}
	ApplyAutoRetaliate(p, autoRetaliateToggleValue(p))
	return true
}

// autoRetaliateToggleValue is the varp 172 transmit value
// that flips the current setting: the setting is enabled when
// the varp is 0, so turning it off sends 1 and turning it on
// sends 0.
func autoRetaliateToggleValue(p *Player) int {
	if p.GetCombat().AutoRetaliate() {
		return 1
	}
	return 0
}

// ApplyAutoRetaliate applies a varp 172 transmit
// (NetworkBuilder.ts:1155-1160): value 0 enables
// auto-retaliate, anything else disables it; disabling stops
// retaliation, and the (normalized) value is echoed back as
// varp 172.
func ApplyAutoRetaliate(p *Player, value int) {
	if p == nil {
		return
	}
	p.GetCombat().SetAutoRetaliate(value == 0)
	if !p.GetCombat().AutoRetaliate() {
		p.GetCombat().StopAutoRetaliation()
	}
	p.SendVarp(AutoRetaliateVarp, autoRetaliateVarpValue(p))
}

// ClearAutocastForStyle clears the autocast spell when a
// weapon style is chosen (Autocasting.clearForStyle): the
// default spell is not kept. With no autocast spell set it
// sends nothing.
func ClearAutocastForStyle(p *Player) {
	if p == nil {
		return
	}
	if p.GetCombat().AutoCastSpell() == nil {
		return
	}
	p.GetCombat().SetAutoCastSpell(nil)
	RefreshAutocastIndicators(p)
}

// RefreshAutocastIndicators refreshes the autocast indicators
// (Autocasting.refreshIndicators): varbit 275 (autocast set),
// 276 (autocast spell index), 2668 (defensive autocast), and
// varp 43 = 4 (COM_MODE_AUTOCAST) while a spell is set. The
// spell counts only with a staff equipped
// (Equipment.hasStaffEquipped: STAFF, ANCIENT_STAFF or
// BLADED_STAFF); defensive reads the current fight type's
// style. The spell index resolves against the autocast spell
// table (AUTOCAST_SPELLS, autocasting.go); a spell outside
// the table reads index 0, the TS fallback.
func RefreshAutocastIndicators(p *Player) {
	if p == nil {
		return
	}
	sender := p.combatStyleSender()
	var spell CombatSpell
	if hasStaffEquipped(p) {
		spell = p.GetCombat().AutoCastSpell()
	}
	defensive := p.GetCombat().FightType().Style == FightStyleDefensive
	if sender != nil {
		if spell == nil {
			sender.SendVarbit(275, 0)
			sender.SendVarbit(276, 0)
		} else {
			sender.SendVarbit(275, 1)
			sender.SendVarbit(276, autocastSpellIndex(spell))
		}
		if spell != nil && defensive {
			sender.SendVarbit(2668, 1)
		} else {
			sender.SendVarbit(2668, 0)
		}
	}
	if spell != nil {
		p.SendVarp(CombatStyleVarp, 4)
	}
}

// hasStaffEquipped is Equipment.hasStaffEquipped
// (Equipment.ts:118-139) for the autocast-indicator path:
// the wielded weapon's interface is STAFF, ANCIENT_STAFF or
// BLADED_STAFF. (Equipment itself does not port
// hasStaffEquipped/canAutocastWith yet; this helper covers
// the one call site that needs it. Powered staves and
// polestaves do not autocast.)
func hasStaffEquipped(p *Player) bool {
	if p == nil {
		return false
	}
	switch CurrentWeaponInterface(p).Name {
	case "STAFF", "ANCIENT_STAFF", "BLADED_STAFF":
		return true
	default:
		return false
	}
}

package game

// Magic spellbooks: the port of game/model/MagicSpellbook.ts
// (110 lines) — the four books, their tab-6 interface ids, and
// changeSpellbook. The teleport type each book carries is NOT
// ported: there is no teleport system in Go, and the save
// preserves the teleportType JSON verbatim (persist SpellBook)
// without the live code reading it.
//
// The live book is session-only state on Player (Player.ts:201,
// `private spellbook = MagicSpellbook.NORMAL`): the save holds
// the book's interface id (persist SpellBook.InterfaceID), entry
// restores it through applyEntrySpellbook (cmd/server), and
// snapshots keep the interface id. The zero Spellbook is NORMAL,
// so a fresh Player reads the TS field default with no
// initialization.

// Spellbook is one of the four magic spellbooks
// (MagicSpellbook.ts:12-15).
type Spellbook int

const (
	// SpellbookNormal is MagicSpellbook.NORMAL (interface 1151).
	SpellbookNormal Spellbook = iota
	// SpellbookAncient is MagicSpellbook.ANCIENT (interface 12855).
	SpellbookAncient
	// SpellbookLunar is MagicSpellbook.LUNAR (interface 29999).
	SpellbookLunar
	// SpellbookArceuus is MagicSpellbook.ARCEUUS (interface 39999).
	SpellbookArceuus
)

// SpellbookInterfaceIDs are the tab-6 interface ids
// (MagicSpellbook.ts:12-15), keyed by book. The persist package
// carries the same four ids (Spellbook*InterfaceID); they are
// repeated here so game code never imports persist.
var spellbookInterfaceIDs = [4]int{1151, 12855, 29999, 39999}

// InterfaceID returns the book's tab-6 interface id
// (MagicSpellbook.getInterfaceId).
func (b Spellbook) InterfaceID() int {
	if b < SpellbookNormal || b > SpellbookArceuus {
		return 1151
	}
	return spellbookInterfaceIDs[b]
}

// Key returns the book's event key: the static's name lowercased
// ("normal", "ancient", "lunar", "arceuus"). TS derives it as
// Object.keys(MagicSpellbook).find(name => MagicSpellbook[name]
// === book)?.toLowerCase() (MagicSpellbook.ts changeSpellbook);
// the keys are the four static names, so the mapping is exact.
func (b Spellbook) Key() string {
	switch b {
	case SpellbookAncient:
		return "ancient"
	case SpellbookLunar:
		return "lunar"
	case SpellbookArceuus:
		return "arceuus"
	default:
		return "normal"
	}
}

// SpellbookForInterfaceID returns the book for a saved interface
// id, or NORMAL for anything else. TS's forId
// (MagicSpellbook.ts:27-34) compares a field the class never
// defines (`.ordinal`), so it has no working callers at
// bfd81e3 and is not ported; the interface-id match is the
// behavior the save hydration (PlayerSave.ts:641,
// player.setSpellbook) and the bootstrap varbit mapping
// (PacketSender.ts:549: Ancient 1, Lunar 2, Arceuus 3, else 0)
// both implement.
func SpellbookForInterfaceID(interfaceID int) Spellbook {
	switch interfaceID {
	case 12855:
		return SpellbookAncient
	case 29999:
		return SpellbookLunar
	case 39999:
		return SpellbookArceuus
	default:
		return SpellbookNormal
	}
}

// Spellbook returns the player's current spellbook
// (Player.getSpellbook). A nil player reads NORMAL.
func (p *Player) Spellbook() Spellbook {
	if p == nil {
		return SpellbookNormal
	}
	return p.spellbook
}

// SetSpellbook sets the player's current spellbook
// (Player.setSpellbook). A nil player is a no-op. This is the
// plain setter: content that switches books calls
// ChangeSpellbook, which applies the Lunar gate, resets
// autocast, and emits the event.
func (p *Player) SetSpellbook(book Spellbook) {
	if p == nil {
		return
	}
	p.spellbook = book
}

// SpellbookChangedEventName is the custom event changeSpellbook
// emits ("magic:spellbook-changed", MagicSpellbook.ts): the book
// switched to, by its key.
const SpellbookChangedEventName = "magic:spellbook-changed"

// SpellbookChangedPayload is the magic:spellbook-changed payload
// (MagicSpellbook.ts: `{ player, spellbook }`): the player and
// the book key ("normal", "ancient", "lunar", "arceuus"). It
// follows the PrivateAreaEnteredPayload precedent: a named game
// payload type, emitted through Player.EmitCustomEvent.
type SpellbookChangedPayload struct {
	Player *Player
	// Spellbook is the book key (Spellbook.Key).
	Spellbook string
}

// defaultSpellbookChangeMessage is the message changeSpellbook
// sends on a switch when the caller supplies no message of its
// own (MagicSpellbook.ts: message default).
const defaultSpellbookChangeMessage = "You have changed your magic spellbook."

// ChangeSpellbook switches p to book
// (MagicSpellbook.changeSpellbook): false when refused (Lunar's
// 40 Defence), true otherwise.
//
//   - Same book: refreshes tab 6 only (the spell-unlock states
//     went out with the login mount and are not re-sent) — no
//     autocast reset, no message, no event.
//   - Lunar below 40 max Defence (unless bypassRequirements):
//     sends "You need at least level 40 Defence to use the Lunar
//     spellbook." and refuses.
//   - Otherwise: sets the book, resets autocast (setAutocast
//     with nil), sends the message, mounts the book on tab 6,
//     and emits magic:spellbook-changed with the book key.
//
// message selects the switch line: nil sends the default line,
// a non-nil pointer sends its contents, and a pointer to ""
// sends nothing (the Go form of TS null, which sends none).
// A nil player returns false.
func ChangeSpellbook(p *Player, book Spellbook, bypassRequirements bool, message *string) bool {
	if p == nil {
		return false
	}
	if book == p.Spellbook() {
		// Refresh the client spellbook.
		if sender := p.combatStyleSender(); sender != nil {
			sender.SendTabInterface(6, book.InterfaceID())
		}
		return true
	}
	if !bypassRequirements && book == SpellbookLunar {
		defence := 0
		if skills := p.Skills(); skills != nil {
			defence = skills.MaxLevel(SkillDefence)
		}
		if defence < 40 {
			p.SendMessage("You need at least level 40 Defence to use the Lunar spellbook.")
			return false
		}
	}

	// Update spellbook.
	p.SetSpellbook(book)

	// Reset autocast.
	SetAutocast(p, nil)

	// Send notification message.
	switch {
	case message == nil:
		p.SendMessage(defaultSpellbookChangeMessage)
	case *message != "":
		p.SendMessage(*message)
	}

	// Send the new spellbook interface to the client side tabs.
	if sender := p.combatStyleSender(); sender != nil {
		sender.SendTabInterface(6, p.Spellbook().InterfaceID())
	}

	// "magic:spellbook-changed" { player, spellbook }: the book
	// switched to, by its key ("lunar").
	if p.EmitCustomEvent != nil {
		p.EmitCustomEvent(SpellbookChangedEventName, SpellbookChangedPayload{
			Player:    p,
			Spellbook: book.Key(),
		})
	}
	return true
}

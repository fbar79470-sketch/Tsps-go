// Helpers shared by the implemented packet listeners: definition
// accessors, item-stack identity, and meta conversion. The meta
// conversion pair exists because game.Item meta is
// map[string]json.RawMessage while game.ItemOnGround meta is
// map[string]any; both are JSON values, so conversion is a
// re-encode, not a reinterpretation.

package listeners

import (
	"bytes"
	"encoding/json"
	"strings"

	"tsps-go/internal/game"
)

// playerReady reports whether p carries the systems the implemented
// listeners touch. NewPlayer always installs them; the guard only
// keeps a zero-value Player from panicking the tick goroutine. The
// hitpoints gate and the container reads all assume a ready player.
func playerReady(p *game.Player) bool {
	return p != nil &&
		p.Skills() != nil &&
		p.Inventory() != nil &&
		p.Equipment() != nil &&
		p.MovementQueue() != nil
}

// hitpoints reads the player's current Hitpoints level: TS gates
// listeners on player.getHitpoints(), which is the SkillManager's
// current Hitpoints level (Player.ts:329-331). A dead player (<= 0)
// never equips, drops, rearranges, or picks up.
func hitpoints(p *game.Player) int {
	return p.Skills().CurrentLevel(game.SkillHitpoints)
}

// defs returns the player's item-definition source, which may be
// nil for a player built without definitions. Callers treat a nil
// source as "every id resolves to the DEFAULT definition": no
// equipment slot, not stackable, not double-handed, not
// tradeable/sellable/dropable — the same answers the game layer's
// nil-source defaults give (see internal/game/item.go: a nil
// source answers false, matching the TS DEFAULT definition whose
// fields are unset/falsy).
func defs(p *game.Player) game.ItemDefinitionSource {
	return p.ItemDefinitions()
}

// equipmentSlotOf returns the equipment slot id occupies, or -1
// when the id is not wearable or no definition source is installed.
// TS: item.getDefinition().getEquipmentType().getSlot()
// (EquipPacketListener.ts:153-157); the DEFAULT definition has no
// equipment type, so unknown ids read -1.
func equipmentSlotOf(p *game.Player, id int) int {
	if src := defs(p); src != nil {
		return src.EquipmentSlot(id)
	}
	return -1
}

// stackableOf reports whether id stacks, reading the definition
// source or false when none is installed.
func stackableOf(p *game.Player, id int) bool {
	if src := defs(p); src != nil {
		return src.IsStackable(id)
	}
	return false
}

// doubleHandedOf reports whether id is a two-handed weapon, reading
// the definition source or false when none is installed. TS reads
// definition.isDoubleHanded() (EquipPacketListener.ts:183-184);
// the DEFAULT definition is not double-handed.
func doubleHandedOf(p *game.Player, id int) bool {
	if src := defs(p); src != nil {
		return src.IsDoubleHanded(id)
	}
	return false
}

// actionSource returns the player's item-action source when the
// installed definition source implements game.ItemActionSource, or
// nil when it does not. The action source is OPTIONAL (see
// internal/game/itemdef.go): without it, listeners fall back to the
// ObjType defaults, which give correct Drop/Take routing but cannot
// resolve custom option names or shift-click indices. See the
// package doc for the coordinator wiring that installs the real
// source.
func actionSource(p *game.Player) game.ItemActionSource {
	src, _ := defs(p).(game.ItemActionSource)
	return src
}

// inventoryActions returns the five inventory option strings for
// id, from the action source when installed, else the ObjType
// default (["", "", "", "", "Drop"]). The result always has length
// 5: a source returning a shorter list is padded with empty
// options, a longer one truncated. TS reads
// definition.inventoryActions (ObjType.ts) with null entries for
// unset options; Go represents unset as "".
func inventoryActions(src game.ItemActionSource, id int) []string {
	actions := game.DefaultInventoryActions()
	if src != nil {
		actions = src.InventoryActions(id)
	}
	return normalizeActions(actions)
}

// groundActions returns the five ground option strings for id, from
// the action source when installed, else the ObjType default
// (["", "", "Take", "", ""]). See inventoryActions.
func groundActions(src game.ItemActionSource, id int) []string {
	actions := game.DefaultGroundActions()
	if src != nil {
		actions = src.GroundActions(id)
	}
	return normalizeActions(actions)
}

// normalizeActions pads or truncates an action list to the five
// option slots TS ObjType carries (indices 0-4, option numbers
// 1-5). A nil list becomes five empty options.
func normalizeActions(actions []string) []string {
	out := make([]string, 5)
	copy(out, actions)
	return out
}

// shiftClickIndex returns the shift-click option index (0-based) for
// id: the action source's value when installed, else the default
// computed from the default inventory actions. TS:
// ObjType.getShiftClickIndex (ObjType.ts:620-632) — the cache
// shiftClickIndex field (-1 means "not set", -2 means "use the
// default rule"), and the default rule is "option 5 when it reads
// Drop, else unset". With the default actions the fifth option is
// "Drop", so the default shift-click index is 4. A source returning
// -1 means "no shift-click option".
func shiftClickIndex(src game.ItemActionSource, id int) int {
	if src != nil {
		return src.ShiftClickIndex(id)
	}
	if strings.EqualFold(game.DefaultInventoryActions()[4], "drop") {
		return 4
	}
	return -1
}

// findActionIndex returns the 0-based index of the first action
// equal to option (case-insensitive), or -1. TS:
// actions.findIndex(a => a?.toLowerCase() === option) — empty
// options never match a non-empty option.
func findActionIndex(actions []string, option string) int {
	for i, action := range actions {
		if action != "" && strings.EqualFold(action, option) {
			return i
		}
	}
	return -1
}

// sameStack reports whether two items are the same stack: same id
// and equal meta. TS compares id equality plus
// JSON.stringify(meta) equality (EquipPacketListener.ts:169-173,
// "sameStack"). The fingerprint normalizes key order through a
// JSON round-trip — the same canonicalization as
// game.Item.metaFingerprint, so stack identity here agrees with
// the container layer's merge identity. Number literals are
// preserved verbatim (1 vs 1.0 differ), exactly as in
// metaFingerprint: TS JSON.stringify would fold them, but
// diverging from the container layer would be worse than the
// residual difference.
func sameStack(a, b game.Item) bool {
	return a.ID() == b.ID() && metaFingerprint(a.Meta()) == metaFingerprint(b.Meta())
}

// metaFingerprint canonicalizes an item meta map for equality
// comparison: nil meta fingerprints as "null", and every value is
// decoded and re-encoded so key order does not affect the result.
// Values that fail to decode fall back to their raw bytes. Number
// literals are preserved verbatim (see sameStack). Mirrors
// game.Item.metaFingerprint, which is unexported.
func metaFingerprint(meta map[string]json.RawMessage) string {
	if meta == nil {
		return "null"
	}
	normalized := make(map[string]any, len(meta))
	for key, raw := range meta {
		var value any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&value); err != nil {
			normalized[key] = string(raw)
			continue
		}
		normalized[key] = value
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "null"
	}
	return string(encoded)
}

// metaMatchesGround reports whether an item's meta and a
// ground-item stack's meta describe the same JSON object. The
// ground meta is converted to item meta first so both sides go
// through the same canonicalization (numbers decode to
// json.Number on both sides). TS compares
// JSON.stringify(item.meta) with JSON.stringify(groundItem meta)
// (ItemOnGroundManager.ts:388-389, matchingAmount).
func metaMatchesGround(itemMeta map[string]json.RawMessage, groundMeta map[string]any) bool {
	return metaFingerprint(itemMeta) == metaFingerprint(groundMetaToItem(groundMeta))
}

// itemMetaToGround converts item meta (map[string]json.RawMessage)
// to ground-item meta (map[string]any) by decoding each value.
// A nil map stays nil (TS: an item with no meta registers a stack
// with no meta). A value that fails to decode is kept as its raw
// string rather than dropped.
func itemMetaToGround(meta map[string]json.RawMessage) map[string]any {
	if meta == nil {
		return nil
	}
	out := make(map[string]any, len(meta))
	for key, raw := range meta {
		var value any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&value); err != nil {
			out[key] = string(raw)
			continue
		}
		out[key] = value
	}
	return out
}

// groundMetaToItem converts ground-item meta (map[string]any) to
// item meta (map[string]json.RawMessage) by encoding each value.
// A nil map stays nil. A value that fails to encode is stored as
// JSON null rather than dropped.
func groundMetaToItem(meta map[string]any) map[string]json.RawMessage {
	if meta == nil {
		return nil
	}
	out := make(map[string]json.RawMessage, len(meta))
	for key, value := range meta {
		encoded, err := json.Marshal(value)
		if err != nil {
			out[key] = json.RawMessage("null")
			continue
		}
		out[key] = encoded
	}
	return out
}

// isOwnerName reports whether username owns a ground-item stack
// whose owner is recorded as owner. TS compares
// Misc.formatNameForProtocol (Misc.ts:801-803: lowercase, spaces
// to underscores) on both sides
// (ItemOnGroundManager.ts:102-104, isOwner). An empty owner never
// matches: unowned stacks are not "owned" by anyone.
func isOwnerName(owner, username string) bool {
	if owner == "" {
		return false
	}
	return formatNameForProtocol(owner) == formatNameForProtocol(username)
}

// formatNameForProtocol is Misc.formatNameForProtocol
// (Misc.ts:801-803): lowercase with spaces replaced by
// underscores. Player usernames are stored in this form in Go
// (Player.Username), but a stack owner recorded from another
// source may not be, so both sides are normalized.
func formatNameForProtocol(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), " ", "_")
}

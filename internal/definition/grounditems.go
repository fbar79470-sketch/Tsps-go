package definition

import (
	"fmt"
	"path/filepath"
)

// GroundItemSpawn is one loaded ground-item spawn in the contract
// shape: the static world ground spawn
// (GroundItemSpawnDefinitionLoader.ts) as plain data.
type GroundItemSpawn struct {
	ID      int
	X, Y, Z int
	Amount  int
	// Respawn is the entry's respawn value as loaded: ticks until
	// the item comes back after a pickup, or 0 for a permanent
	// fixture that never respawns or despawns (see the loader's
	// doc comment, GroundItemSpawnDefinitionLoader.ts:20-25). The
	// TS ItemOnGround stores respawn > 0 ? respawn : -1 as its
	// respawn timer (line 124, respawns() is timer > 0); the
	// conversion belongs to the ground-item system that consumes
	// this data, not to the loader.
	Respawn int
	// Description is the entry's "description" string (a location
	// note such as "Port Sarim" in the shipped file). The TS
	// loader ignores it — it is not part of RawGroundItemSpawn —
	// but the contract carries it, so it is preserved verbatim
	// ("" when absent or not a string).
	Description string
}

// PickupReplacement is a ground spawn's pickupReplacement: the
// item left behind when the spawn is picked up, with an optional
// telegrab denial message (ItemOnGround.GroundPickupReplacement,
// as validated by GroundItemSpawnDefinitionLoader.ts:112-114).
type PickupReplacement struct {
	ID int
	// TelegrabDenial is the denial message, or "" when the entry
	// carries none. TS requires a present denial to be a
	// non-empty string, so "" unambiguously means absent.
	TelegrabDenial string
}

// GroundItem is one loaded ground-item spawn with the fields the
// contract shape omits: the members flag and the pickup
// replacement. Spawn holds the contract view.
type GroundItem struct {
	Spawn GroundItemSpawn
	// Members marks a members-only spawn, skipped on free-to-play
	// worlds (GroundItemSpawnDefinitionLoader.ts:62).
	Members bool
	// PickupReplacement is nil when the entry carries none.
	PickupReplacement *PickupReplacement
}

// loadGroundItems ports GroundItemSpawnDefinitionLoader.load()
// (GroundItemSpawnDefinitionLoader.ts:30-81): the core file is
// REQUIRED (readCoreDefinitions throws on a missing, malformed,
// or non-array file) and is prepended as the "ground-items"
// source at priority 0, then all sources are sorted by priority
// then name. Each entry is validated; invalid entries are
// skipped, members-only entries are skipped on F2P worlds, and
// entries whose item the cache does not support are skipped.
// Registration into ItemOnGroundManager is the ground-item
// system's job, not this package's.
func (s *Store) loadGroundItems() error {
	contributed, failures := s.registry.loadSources(groundItemsDefinitionType, s.logf)
	core, err := s.readCoreGroundItems()
	if err != nil {
		return err
	}
	sources := append([]LoadedSource{{
		Name:        "ground-items",
		Owner:       "ground-items",
		Priority:    0,
		Definitions: core,
	}}, contributed...)
	sortLoadedSources(sources)

	candidates, invalid, unsupported, membersSkipped := 0, 0, 0, 0
	for _, source := range sources {
		candidates += len(source.Definitions)
		for _, raw := range source.Definitions {
			item, ok := s.toGroundItem(raw)
			if !ok {
				invalid++
				continue
			}
			if item.Members && !s.membersWorld {
				membersSkipped++
				continue
			}
			if !s.hasItem(item.Spawn.ID) {
				unsupported++
				continue
			}
			s.groundItems = append(s.groundItems, item)
		}
	}
	// TS logs warn when invalid > 0 or unsupported > 0, debug
	// otherwise (line 75); Go has one log channel, so the message
	// is the same either way.
	s.logf("[ground-items] Loaded %d definitions from %s (candidates=%d, invalid=%d, unsupported=%d, membersSkipped=%d)",
		len(s.groundItems), joinSourceNames(sources), candidates, invalid, unsupported, membersSkipped)
	_ = failures // TS returns contributed.failures == 0; GameBuilder ignores it.
	return nil
}

// readCoreGroundItems is readCoreDefinitions
// (GroundItemSpawnDefinitionLoader.ts:87-94).
func (s *Store) readCoreGroundItems() ([]any, error) {
	file := filepath.Join(s.dir, "ground-items.json")
	parsed, err := readJSONValue(file)
	if err != nil {
		return nil, err
	}
	defs, ok := parsed.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must contain an array", file)
	}
	return defs, nil
}

// toGroundItem is toItemOnGround
// (GroundItemSpawnDefinitionLoader.ts:96-129), returning the
// loaded record instead of an ItemOnGround: integer id >= 0,
// integer x/y, integer z in 0..3, integer amount > 0 (default 1),
// integer respawn >= 0 (default 0), a boolean-or-absent members
// flag, and a valid pickup replacement when present (integer id,
// cache-supported, optional non-empty telegrab denial).
func (s *Store) toGroundItem(raw any) (GroundItem, bool) {
	idNum := numField(raw, "id")
	location := objField(raw, "location")
	xNum := numField(location, "x")
	yNum := numField(location, "y")
	zNum := numField(location, "z")
	amount := 1.0
	if v, present := field(raw, "amt"); present {
		amount = jsNumber(v, true)
	}
	respawn := 0.0
	if v, present := field(raw, "respawn"); present {
		respawn = jsNumber(v, true)
	}
	var replacement *PickupReplacement
	if v, present := field(raw, "pickupReplacement"); present {
		rep, ok := s.parsePickupReplacement(v)
		if !ok {
			return GroundItem{}, false
		}
		replacement = rep
	}
	members := false
	if v, present := field(raw, "members"); present {
		b, ok := v.(bool)
		if !ok {
			return GroundItem{}, false
		}
		members = b
	}
	if !isJSInteger(idNum) || jsTrunc(idNum) < 0 ||
		!isJSInteger(xNum) || !isJSInteger(yNum) ||
		!isJSInteger(zNum) || jsTrunc(zNum) < 0 || jsTrunc(zNum) > 3 ||
		!isJSInteger(amount) || jsTrunc(amount) <= 0 ||
		!isJSInteger(respawn) || jsTrunc(respawn) < 0 {
		return GroundItem{}, false
	}
	description := ""
	if v, present := field(raw, "description"); present {
		description, _ = v.(string)
	}
	return GroundItem{
		Spawn: GroundItemSpawn{
			ID:          jsTrunc(idNum),
			X:           jsTrunc(xNum),
			Y:           jsTrunc(yNum),
			Z:           jsTrunc(zNum),
			Amount:      jsTrunc(amount),
			Respawn:     jsTrunc(respawn),
			Description: description,
		},
		Members:           members,
		PickupReplacement: replacement,
	}, true
}

// parsePickupReplacement validates one pickupReplacement value.
// A JSON null counts as present-but-invalid in TS (the check is
// `replacement !== undefined && (!replacement || ...)`), so it is
// rejected here too.
func (s *Store) parsePickupReplacement(v any) (*PickupReplacement, bool) {
	rep, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	// Number.isInteger(replacement.id): strict, no Number()
	// coercion, unlike the entry's own id above.
	idNum, ok := numberValue(mustNum(rep, "id"))
	if !ok || !isJSInteger(idNum) || !s.hasItem(jsTrunc(idNum)) {
		return nil, false
	}
	denial := ""
	if d, present := field(rep, "telegrabDenial"); present {
		text, ok := d.(string)
		if !ok || text == "" {
			return nil, false
		}
		denial = text
	}
	return &PickupReplacement{ID: jsTrunc(idNum), TelegrabDenial: denial}, true
}

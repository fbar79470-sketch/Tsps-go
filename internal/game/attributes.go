package game

import "sync"

// Player/NPC attributes: the port of the attribute map on
// game/entity/impl/Mobile.ts:102-110. TS holds a lazily allocated
// `Map<Object, Object> | null`; getAttribute returns
// `attributes?.get(name)` (undefined when absent or never
// allocated) and setAttribute allocates the map on first use
// (`??= new Map()`) and stores the value. Player and NPC inherit
// the map through the embedded Mobile, as they do here.
//
// NO ENUMERATION METHOD: TS core never enumerates a player's
// attribute map. The only code that reads the map as a whole is
// the save snapshot, and it iterates the persistent-key registry
// (PlayerSave.persistentAttributeKeys), not the map
// (PlayerSave.ts:811-816, fromPlayer). A grep over the TS core at
// commit bfd81e3 finds no access to the `attributes` field outside
// Mobile.ts itself (the other `.attributes` hits are the save
// object's attributes field and NpcDefinition's unrelated
// string list). Enumerating live attributes would therefore have
// no TS behavior to match; the persist layer rebuilds saves from
// its own registry, as TS does.
//
// NO REMOVE METHOD: no removeAttribute/clearAttributes/
// deleteAttribute exists in TS core or in the plugin sources (a
// grep over game/ and plugins/ at commit bfd81e3 finds zero
// matches; the single removeAttribute hit in the tree is the DOM
// API inside a minified admin-panel bundle). Clearing a key is
// done by storing an absent value; see SetAttribute.
//
// VALUES ARE STORED BY REFERENCE, exactly as TS stores the
// reference in its Map. Mutating a stored map or slice through
// the value GetAttribute returns mutates the stored value, as in
// TS. Cloning happens only at the save/load boundary, in the
// persist layer (PlayerSave.cloneAttribute) — never here.
//
// THREAD-SAFETY: the store is a lazily allocated POINTER to a
// struct holding the map plus a sync.RWMutex. The pointer form
// mirrors the TS lazy allocation and keeps Mobile copyable by
// value (NewMobile returns a Mobile; Player/NPC embed it): the
// mutex itself is never copied. The mutex is required because
// login emits run on the login goroutine while tick work runs on
// the tick goroutine (see internal/content/dateminutes), so
// attribute reads and writes from content can race the tick. Get
// takes the read lock, Set the write lock. Lazy allocation itself
// is serialized by attributeAllocMu: every load of the store
// pointer goes through it, so a first Set racing a Get cannot
// tear the pointer. A Mobile value copied after attributes are
// set shares the store with the copy (the pointer is copied);
// Players and NPCs are always handled by pointer, so no live
// entity is ever copied after use, as with Player's own mutex.
//
// NIL RECEIVERS: Get/Has on a nil *Mobile report absent and Set
// on a nil *Mobile is a no-op. TS would throw on a null entity;
// the Go form stays silent so a bare fixture cannot panic the
// tick goroutine, following the ItemPickupThrottled precedent in
// player.go. This guards a nil *Mobile only: a call promoted
// through a nil *Player or *NPC panics at the promotion site
// before the method body runs (Go evaluates the embedded field's
// address), so callers must nil-check the entity itself.

// attributeStore is one Mobile's attribute map plus its guarding
// mutex. It is allocated lazily, on the first SetAttribute that
// stores a value (Mobile.ts:102, 107-109).
type attributeStore struct {
	mu     sync.RWMutex
	values map[string]any
}

// attributeAllocMu serializes lazy allocation of attribute
// stores and every load of a Mobile's store pointer. See the
// file header.
var attributeAllocMu sync.Mutex

// store returns m's attribute store, allocating it when create
// is true and none exists yet. It returns nil when no store
// exists and create is false. A nil receiver has no store.
func (m *Mobile) store(create bool) *attributeStore {
	if m == nil {
		return nil
	}
	attributeAllocMu.Lock()
	defer attributeAllocMu.Unlock()
	if m.attributes == nil && create {
		m.attributes = &attributeStore{values: make(map[string]any)}
	}
	return m.attributes
}

// GetAttribute returns the value stored under key, or nil when
// absent. Mobile.ts:104-106 (getAttribute): TS returns undefined
// when the map is unallocated or the key is missing; nil is the
// Go form of undefined here. A stored falsy value (false, 0, "")
// is returned as stored and is distinct from absent — use
// HasAttribute to tell the two apart.
func (m *Mobile) GetAttribute(key string) any {
	s := m.store(false)
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.values[key]
}

// SetAttribute stores value under key, allocating the map on
// first use. Mobile.ts:107-109 (setAttribute).
//
// A NIL VALUE DELETES THE KEY. This is the deliberate Go form of
// TS `setAttribute(key, undefined)`: TS stores undefined in the
// Map, getAttribute returns undefined, and fromPlayer skips
// undefined values when rebuilding the save (PlayerSave.ts:811-816,
// the `!== undefined` filter) — so the observable effect of
// storing undefined is absence, in memory and in the save, and
// deletion reproduces it exactly. TS content also stores explicit
// null to clear keys (346 setAttribute(key, null) sites at commit
// bfd81e3: 331 in plugin JS, 15 in TS core); Go has no null
// distinct from nil, so those ports pass nil and delete. The
// collapse is safe: no code checks a getAttribute result with
// strict `=== null` / `!== null` (zero such checks in plugin JS;
// the four core null checks use loose `==` / `!= null`, under
// which null and undefined are the same), and every
// `=== true` / `?? default` consumer treats a deleted key
// exactly as it treats a null-valued one. The one unobservable
// difference is a save that would have carried an explicit JSON
// null for a persistent key: Go writes no entry for it, which is
// what a fresh save holds for that key anyway.
func (m *Mobile) SetAttribute(key string, value any) {
	if value == nil {
		s := m.store(false)
		if s == nil {
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.values, key)
		return
	}
	s := m.store(true)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
}

// HasAttribute reports whether key is present in the store,
// including keys holding falsy values (false, 0, ""). It is a
// GO-ONLY HELPER: TS distinguishes presence via undefined
// (getAttribute returns undefined only when absent), but Go
// cannot tell a stored nil from an absent key through
// GetAttribute alone — and SetAttribute(nil) deletes, so a nil
// is never stored. Callers that need "was this key ever set"
// use this instead of comparing GetAttribute against nil.
func (m *Mobile) HasAttribute(key string) bool {
	s := m.store(false)
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.values[key]
	return ok
}

package definition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InterfaceSurface is the surface an interface layout opens on
// (InterfaceSurface, InterfaceLayoutDefinition.ts:1-8).
type InterfaceSurface string

const (
	SurfaceMain            InterfaceSurface = "main"
	SurfaceFullscreen      InterfaceSurface = "fullscreen"
	SurfaceChatbox         InterfaceSurface = "chatbox"
	SurfaceWalkableOverlay InterfaceSurface = "walkable-overlay"
	SurfaceSidebar         InterfaceSurface = "sidebar"
	SurfaceSidebarOverlay  InterfaceSurface = "sidebar-overlay"
	SurfaceMainWithSidebar InterfaceSurface = "main-with-sidebar"
)

// InterfaceLayoutDefinition is one interface layout: the key it is
// looked up by, the interface id, its display name, the surface it
// opens on, and the surface-specific ids
// (InterfaceLayoutDefinition.ts:11-18). TabID and
// SidebarInterfaceID are nil when the JSON entry omits them (TS
// omits the property entirely).
type InterfaceLayoutDefinition struct {
	Key                string
	ID                 int
	Name               string
	Surface            InterfaceSurface
	TabID              *int
	SidebarInterfaceID *int
}

// Layout returns the layout for a key, or false
// (InterfaceLayoutRegistry.get with a string,
// InterfaceLayoutDefinition.ts:51-58).
func (s *Store) Layout(key string) (*InterfaceLayoutDefinition, bool) {
	def, ok := s.layouts[key]
	return def, ok
}

// LayoutByID returns the first layout with an interface id, or
// false (InterfaceLayoutRegistry.get with a number: the first of
// the definitions sharing the id).
func (s *Store) LayoutByID(id int) (*InterfaceLayoutDefinition, bool) {
	defs := s.layoutsByID[id]
	if len(defs) == 0 {
		return nil, false
	}
	return defs[0], true
}

// Layouts returns all layouts in file order
// (InterfaceLayoutRegistry.all).
func (s *Store) Layouts() []*InterfaceLayoutDefinition {
	return append([]*InterfaceLayoutDefinition(nil), s.layoutList...)
}

// loadLayouts ports InterfaceLayoutDefinitionLoader.load()
// (InterfaceLayoutDefinitionLoader.ts:13-29): the file is REQUIRED
// (readFileSync/JSON.parse throw), must hold a JSON object keyed
// by name, and every entry is validated — the first invalid entry
// fails the load. InterfaceLayoutRegistry.open (the packet-sender
// dispatch) is not ported: there is no packet sender in this
// package, and the layouts are data for the consumer.
func (s *Store) loadLayouts() error {
	filePath := filepath.Join(s.dir, "interface-layouts.json")
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}
	entries, err := parseOrderedJSONObject(content)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", filePath, err)
	}
	if entries == nil {
		return fmt.Errorf("Interface layouts must be a JSON object keyed by name")
	}
	var definitions []*InterfaceLayoutDefinition
	seen := make(map[string]bool)
	for _, entry := range entries {
		def, err := validateLayout(entry.key, entry.value, seen)
		if err != nil {
			return err
		}
		definitions = append(definitions, def)
	}
	for _, def := range definitions {
		s.layouts[def.Key] = def
		s.layoutsByID[def.ID] = append(s.layoutsByID[def.ID], def)
	}
	s.layoutList = definitions
	s.logf("[interfaces] Loaded %d interface layouts", len(definitions))
	return nil
}

// validateLayout is validate (InterfaceLayoutDefinitionLoader.ts:35-84).
func validateLayout(key string, raw any, seen map[string]bool) (*InterfaceLayoutDefinition, error) {
	if key == "" || seen[key] {
		return nil, fmt.Errorf("Invalid or duplicate interface layout key: %s", key)
	}
	seen[key] = true
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Interface layout %s must be an object", key)
	}
	idNum, ok := numberValue(mustNum(obj, "id"))
	if !ok || !isJSInteger(idNum) || idNum < 0 {
		return nil, fmt.Errorf("Interface layout %s has an invalid id", key)
	}
	name, _ := mustNum(obj, "name").(string)
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("Interface layout %s requires a name", key)
	}
	surface, _ := mustNum(obj, "surface").(string)
	switch InterfaceSurface(surface) {
	case SurfaceMain, SurfaceFullscreen, SurfaceChatbox, SurfaceWalkableOverlay,
		SurfaceSidebar, SurfaceSidebarOverlay, SurfaceMainWithSidebar:
	default:
		return nil, fmt.Errorf("Interface layout %s has an invalid surface", key)
	}
	def := &InterfaceLayoutDefinition{
		Key:     key,
		ID:      jsTrunc(idNum),
		Name:    strings.TrimSpace(name),
		Surface: InterfaceSurface(surface),
	}
	// TS copies tabId/sidebarInterfaceId through verbatim when
	// present (non-null), and validates them only for the
	// surfaces that need them. Go's *int fields cannot hold a
	// non-number, so a non-numeric value on a surface that does
	// not validate it is dropped — unobservable, since only
	// open() reads these fields and it re-validates them.
	if v, present := field(obj, "tabId"); present && v != nil {
		if n, ok := numberValue(v); ok {
			tab := jsTrunc(n)
			def.TabID = &tab
		}
	}
	if v, present := field(obj, "sidebarInterfaceId"); present && v != nil {
		if n, ok := numberValue(v); ok {
			sid := jsTrunc(n)
			def.SidebarInterfaceID = &sid
		}
	}
	if def.Surface == SurfaceSidebar {
		tab, present := field(obj, "tabId")
		n, ok := numberValue(tab)
		if !present || tab == nil || !ok || !isJSInteger(n) || n < 0 {
			return nil, fmt.Errorf("Sidebar interface %s requires a tabId", key)
		}
	}
	if def.Surface == SurfaceMainWithSidebar {
		sid, present := field(obj, "sidebarInterfaceId")
		n, ok := numberValue(sid)
		if !present || sid == nil || !ok || !isJSInteger(n) || n < 0 {
			return nil, fmt.Errorf("Main-with-sidebar interface %s requires sidebarInterfaceId", key)
		}
	}
	return def, nil
}

// orderedEntry is one key/value pair of a JSON object in file
// order.
type orderedEntry struct {
	key   string
	value any
}

// parseOrderedJSONObject parses a JSON document that must be an
// object, preserving key order (encoding/json into a map loses
// it, and the TS Object.entries order is the file's key order for
// the non-numeric keys these files use). It returns (nil, nil)
// when the document is valid JSON but not an object, mirroring
// the loader's object check. Duplicate keys keep the last value
// in their first position, as JSON.parse does; the layout
// validator's duplicate-key check therefore never fires on
// parsed JSON, exactly as in TS.
func parseOrderedJSONObject(data []byte) ([]orderedEntry, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, nil
	}
	// Non-nil so an empty object {} is distinguishable from a
	// non-object document (which returns nil above).
	entries := []orderedEntry{}
	index := make(map[string]int)
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("object key is not a string")
		}
		var value any
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		if i, exists := index[key]; exists {
			entries[i].value = value
			continue
		}
		index[key] = len(entries)
		entries = append(entries, orderedEntry{key: key, value: value})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	// Trailing garbage after the object is a JSON.parse error in
	// TS; encoding/json's Decoder would ignore it without this
	// check.
	if dec.More() {
		return nil, fmt.Errorf("unexpected data after JSON object")
	}
	return entries, nil
}

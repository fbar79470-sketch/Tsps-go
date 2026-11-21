package cache

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// This file ports the custom-definition overlays: CacheDefinitions
// .registerCustomDefinitions and .registerCustomItems
// (CacheDefinitions.ts:597-731), the JSON pipelines that lay
// project-authored definitions over the cache without repacking
// it, plus the file readers that feed them.
//
// Call sites in TypeScript (verified, not guessed):
//   - registerCustomDefinitions is called by the cache editor's
//     Document.apply (plugins/tools/cache-editor/Document
//     .CacheEditor.cjs:195-204), invoked from the editor plugin's
//     register() (Catalog.CacheEditor.plugin.js:36-44), which
//     runs during PluginManager.loadFromDirectory — before
//     GameBuilder.loadStartupData, so spawn loaders see the
//     overlay ids (Server.ts:282-290). The document is
//     data/definitions/custom-definitions.json, shaped
//     {"version":1,"rows":[{kind,id,baseId,fields}]}; a missing
//     file reads as the empty document (Document.read), a
//     present-but-malformed one throws.
//   - registerCustomItems is called by the ItemDefinitionLoader
//     plugin (plugins/items/ItemDefinitionLoader.plugin.js,
//     loadItemDefinitions), also at plugin-register time. Its
//     file is data/definitions/custom-items.json, an array (or
//     object, read as its values) of
//     {"id","baseItemId","objType":{...},"itemDef","models",
//     "specialAttack"} rows; a missing file reads as zero rows
//     (existsSync guard), a present-but-malformed one throws.
//   - custom-shops.json does NOT feed this facade: it is read by
//     ShopDefinitionLoader.readCoreDefinitions
//     (ShopDefinitionLoader.ts:103-115) alongside shops.json into
//     ShopDefinition loading, which belongs to the JSON loader
//     package, not here. ApplyCustomOverlays deliberately never
//     reads it.
//
// OVERLAY PRECEDENCE, as implemented in the TypeScript getters
// (CacheDefinitions.ts getNpc/getItem/getObject) and reproduced
// here through ArchivedTypeLoader.SetOverlay (see archived.go):
// custom items first, then custom definitions, then archived
// definitions, then native cache definitions. In practice the
// layers cannot collide on an id: registerCustomItems requires
// 50000 <= id < items count... 65022 and registerCustomDefinitions
// requires max(50000, base count) <= id < 65535, so overlay ids
// always sit above the cache's own range, and each registration
// refuses an id the other layer already owns. The getter order
// still matters on the one overlap the validators do not exclude:
// an archived bundle id in the same appended range, where the
// custom layer wins here exactly as the TypeScript getter order
// dictates. (TypeScript's registerDefinitionArchives additionally
// refuses archived ids that collide with custom content; that
// guard lives in facade.go's RegisterDefinitionArchives, which
// this port does not change, so the guard is absent here — the Go
// server never registers archived bundles at runtime, so the gap
// is latent, not live.)
//
// Counts: TypeScript getCounts extends the base counts by custom
// DEFINITION ids only (CacheDefinitions.ts:811-819); custom
// items do not extend the counts (hasItem checks customItems
// separately). The overlay counts installed on the loaders follow
// the same rule: only custom-definition ids extend Count.
//
// Deliberate deviations and gaps, all at the site or here:
//   - Field application is stricter than TypeScript's Object
//     .assign: unknown field names and mistyped values are
//     errors, because Go cannot attach arbitrary keys to a
//     struct. This matches the cache editor's save-time audit
//     (Document.auditRow), which rejects unknown fields and
//     mistyped values before a row ever reaches a loader.
//   - The encodeContentData check in registerCustomDefinitions
//     (CacheDefinitions.ts:722-728) is approximated by a JSON
//     encodability check: Go has no content-data encoder, and
//     the transport budget (64 KiB) is not enforced, because
//     nothing in the Go server sends overlay datasets to the
//     client (no sendContentData encoder exists yet). Custom
//     definitions therefore affect the server's definition
//     lookups only; the client never learns of them.
//   - Custom models are validated (id range and uniqueness, as
//     in registerCustomItems) but their .dat files under
//     data/models are never read and never sent: there is no
//     model pipeline in the Go server. A custom item whose
//     models a client needs will not render its custom models.
//   - Custom items' itemDef and specialAttack rows are stored
//     and returned by GetCustomItems, but not applied: there is
//     no ItemDefinition or CombatSpecial in Go yet. specialAttack
//     shape validation follows the plugin's validateSpecialAttack.
//   - HasDefinition/HasItem in facade.go do not consult the
//     overlay (that file is outside this change): HasItem
//     answers false for a custom-item id beyond the counts even
//     though GetItem returns it, and HasDefinition answers false
//     for overlay ids. GetNpc/GetItem/GetObject — the lookups
//     the server reads through — do see the overlay.
//   - NoCache (a nil *CacheDefinitions): ApplyCustomOverlays is
//     a no-op that logs a note and returns zero counts. This is
//     a judgment call, grounded in two facts: the cache editor's
//     apply is non-fatal in TypeScript (Catalog.CacheEditor
//     .plugin.js:42-44 catches and logs), and the Go NoCache
//     precedent degrades gracefully (nil facades, boot
//     continues). Overlay validation needs base counts that do
//     not exist without a cache, so the files are not even read.

// CustomDefinitionKind is the kind of a custom-definition row:
// "loc", "npc" or "item", exactly as in TypeScript
// (CacheDefinitions.ts:705, registerCustomDefinitions).
type CustomDefinitionKind string

const (
	// CustomKindLoc overlays object (loc) definitions.
	CustomKindLoc CustomDefinitionKind = "loc"
	// CustomKindNPC overlays NPC definitions.
	CustomKindNPC CustomDefinitionKind = "npc"
	// CustomKindItem overlays item (obj) definitions.
	CustomKindItem CustomDefinitionKind = "item"
)

// customDefinitionSource is the single overlay source in Go: the
// cache editor's document (CACHE_EDITOR_SOURCE,
// CacheDefinitions.ts:33). TypeScript supports further sources
// (e.g. generated custom-pet rows) side by side; with no plugin
// system, Go has only this one, so the per-source replacement in
// registerCustomDefinitions simplifies to whole-overlay
// replacement per registration call.
const customDefinitionSource = "cache-editor"

// customOverlayIDStart is OVERLAY_ID_START
// (Document.CacheEditor.cjs): overlay ids start above the cache's
// own count per kind, at 50000 at the lowest.
const customOverlayIDStart = 50000

// customItemMaxID is the top of the custom-item range
// (CacheDefinitions.ts:606: id > 65022 is rejected).
const customItemMaxID = 65022

// customDefinitionMaxID is the top of the custom-definition range
// (CacheDefinitions.ts:711: id >= 65535 is rejected).
const customDefinitionMaxID = 65535

// CustomDefinitionRow is one overlay row: {kind, id, baseId,
// fields} (CacheDefinitions.ts:703-705). BaseID nil means the
// TypeScript baseId null: a blank definition built from defaults
// instead of cloned from a cache definition. Fields maps
// TypeScript field names (camelCase, e.g. "name", "hitpoints",
// "modelIds") to JSON values.
type CustomDefinitionRow struct {
	Kind   CustomDefinitionKind
	ID     int
	BaseID *int
	Fields map[string]any
}

// CustomItemSpecialAttack is a custom item's special-attack bar
// (ServerCustomItem.specialAttack, CacheDefinitions.ts:53-56):
// `from`, another item whose special it uses, and `energy`, its
// cost in percent. At least one must be present.
type CustomItemSpecialAttack struct {
	From   *int
	Energy *int
}

// CustomItem is one custom-items.json row: ServerCustomItem
// (CacheDefinitions.ts:48-57). ObjType maps TypeScript ObjType
// field names to JSON values; ItemDef holds the gameplay
// overrides (stored, not applied — see the file header); Models
// maps a decimal model id to its data/models path (validated,
// never read — see the file header).
type CustomItem struct {
	ID            int
	BaseItemID    *int
	ObjType       map[string]any
	ItemDef       map[string]any
	Models        map[string]string
	SpecialAttack *CustomItemSpecialAttack
}

// CustomOverlayCounts are the counts ApplyCustomOverlays reports:
// how many custom definitions and custom items were registered.
type CustomOverlayCounts struct {
	Definitions int
	Items       int
}

// customOverlay is the registered overlay state for one facade:
// the rows, the eagerly resolved definitions, and the per-kind
// highest custom-definition id (for Count extension — custom
// items never extend counts, see the file header). Resolution is
// eager at registration so a bad field fails the boot instead of
// the first lookup. All access goes through mu.
type customOverlay struct {
	mu       sync.Mutex
	defs     map[string]CustomDefinitionRow
	items    map[int]CustomItem
	npcs     map[int]*NpcType
	locs     map[int]*LocType
	defObjs  map[int]*ObjType // custom-definition items
	itemObjs map[int]*ObjType // custom items (checked before defObjs)
	maxDefID map[CustomDefinitionKind]int
}

func newCustomOverlay() *customOverlay {
	return &customOverlay{
		defs:     make(map[string]CustomDefinitionRow),
		items:    make(map[int]CustomItem),
		npcs:     make(map[int]*NpcType),
		locs:     make(map[int]*LocType),
		defObjs:  make(map[int]*ObjType),
		itemObjs: make(map[int]*ObjType),
		maxDefID: make(map[CustomDefinitionKind]int),
	}
}

func customOverlayKey(kind CustomDefinitionKind, id int) string {
	return string(kind) + ":" + strconv.Itoa(id)
}

// clone returns a deep copy of the overlay's row state with the
// resolved definitions shared (resolved types are never mutated
// after publication). Registration builds on a clone and
// publishes it whole, so a failed registration leaves the
// previous overlay untouched, as in TypeScript (both register
// functions build local maps and assign only on success).
func (o *customOverlay) clone() *customOverlay {
	if o == nil {
		return newCustomOverlay()
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	out := newCustomOverlay()
	for k, row := range o.defs {
		out.defs[k] = CustomDefinitionRow{Kind: row.Kind, ID: row.ID, BaseID: cloneIntPtr(row.BaseID), Fields: cloneFields(row.Fields)}
	}
	for id, item := range o.items {
		out.items[id] = cloneCustomItem(item)
	}
	for id, v := range o.npcs {
		out.npcs[id] = v
	}
	for id, v := range o.locs {
		out.locs[id] = v
	}
	for id, v := range o.defObjs {
		out.defObjs[id] = v
	}
	for id, v := range o.itemObjs {
		out.itemObjs[id] = v
	}
	for k, v := range o.maxDefID {
		out.maxDefID[k] = v
	}
	return out
}

func cloneIntPtr(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// cloneFields deep-copies a JSON field bag (maps, slices, scalars)
// the way structuredClone does in TypeScript
// (CacheDefinitions.ts:718).
func cloneFields(fields map[string]any) map[string]any {
	if fields == nil {
		return nil
	}
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		out[k] = cloneJSONValue(v)
	}
	return out
}

func cloneJSONValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = cloneJSONValue(e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = cloneJSONValue(e)
		}
		return out
	default:
		return v
	}
}

func cloneCustomItem(item CustomItem) CustomItem {
	out := CustomItem{
		ID:         item.ID,
		BaseItemID: cloneIntPtr(item.BaseItemID),
		ObjType:    cloneFields(item.ObjType),
		ItemDef:    cloneFields(item.ItemDef),
	}
	if item.Models != nil {
		out.Models = make(map[string]string, len(item.Models))
		for k, v := range item.Models {
			out.Models[k] = v
		}
	}
	if item.SpecialAttack != nil {
		out.SpecialAttack = &CustomItemSpecialAttack{From: cloneIntPtr(item.SpecialAttack.From), Energy: cloneIntPtr(item.SpecialAttack.Energy)}
	}
	return out
}

func (o *customOverlay) lookupNPC(id int) (*NpcType, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	v, ok := o.npcs[id]
	return v, ok
}

func (o *customOverlay) lookupLoc(id int) (*LocType, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	v, ok := o.locs[id]
	return v, ok
}

// lookupObj checks custom items before custom definitions, the
// getItem order (CacheDefinitions.ts:588-593).
func (o *customOverlay) lookupObj(id int) (*ObjType, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if v, ok := o.itemObjs[id]; ok {
		return v, true
	}
	v, ok := o.defObjs[id]
	return v, ok
}

func (o *customOverlay) countFor(kind CustomDefinitionKind) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	if max, ok := o.maxDefID[kind]; ok {
		return max + 1
	}
	return 0
}

// baseCounts returns the native-plus-archived counts per kind,
// the getBaseCounts values registerCustomDefinitions validates
// against (CacheDefinitions.ts:707). They come from the registry
// rather than the loaders so a previously registered overlay
// (which extends loader Counts) cannot move the goalposts.
func (d *CacheDefinitions) baseCounts() DefinitionCounts {
	return DefinitionCounts{
		NPCs:    d.archives.GetCount(ArchivedKindNPC),
		Items:   d.archives.GetCount(ArchivedKindObj),
		Objects: d.archives.GetCount(ArchivedKindLoc),
		Seqs:    d.archives.GetCount(ArchivedKindSeq),
	}
}

func baseCountFor(counts DefinitionCounts, kind CustomDefinitionKind) int {
	switch kind {
	case CustomKindLoc:
		return counts.Objects
	case CustomKindNPC:
		return counts.NPCs
	default:
		return counts.Items
	}
}

// installCustomOverlayLookups wires the overlay into the facade's
// loaders: each lookup dereferences the registry's current
// overlay, so re-registration publishes atomically without
// reinstalling closures. It is called on every registration and
// is idempotent.
func (d *CacheDefinitions) installCustomOverlayLookups() {
	current := func() *customOverlay { return d.archives.getCustomOverlay() }
	d.npcs.SetOverlay(
		func(id int) (*NpcType, bool) {
			if o := current(); o != nil {
				return o.lookupNPC(id)
			}
			return nil, false
		},
		func() int {
			if o := current(); o != nil {
				return o.countFor(CustomKindNPC)
			}
			return 0
		})
	d.objects.SetOverlay(
		func(id int) (*LocType, bool) {
			if o := current(); o != nil {
				return o.lookupLoc(id)
			}
			return nil, false
		},
		func() int {
			if o := current(); o != nil {
				return o.countFor(CustomKindLoc)
			}
			return 0
		})
	// Items consult the overlay under the post-processed loader:
	// overlay ObjTypes are marked post-processed at resolution
	// (see resolveCustomObj), so the wrapper returns them
	// untouched, as TypeScript's getItem returns custom types
	// without post-processing.
	d.objArchives.SetOverlay(
		func(id int) (*ObjType, bool) {
			if o := current(); o != nil {
				return o.lookupObj(id)
			}
			return nil, false
		},
		func() int {
			if o := current(); o != nil {
				return o.countFor(CustomKindItem)
			}
			return 0
		})
}

// RegisterCustomDefinitions registers overlay rows for the single
// Go source (the cache editor's document), replacing any rows a
// previous call registered, and returns how many rows were
// registered. It mirrors registerCustomDefinitions
// (CacheDefinitions.ts:703-731): identity validation (kind, id
// range, base range, duplicates, custom-item collision), the
// forbidden-field check, and the encodability check, with
// publication only on full success — a failed call leaves the
// previous overlay untouched. Custom items registered by
// RegisterCustomItems are preserved.
//
// A nil receiver is an error: without a facade there are no base
// counts to validate against. Use ApplyCustomOverlays for the
// NoCache-tolerant boot path.
func (d *CacheDefinitions) RegisterCustomDefinitions(rows []CustomDefinitionRow) (int, error) {
	if d == nil || d.archives == nil {
		return 0, errors.New("RegisterCustomDefinitions: no cache definitions loaded")
	}
	counts := d.baseCounts()
	next := d.archives.getCustomOverlay().clone()
	// This source's rows are replaced wholesale (the single-source
	// simplification of the per-source replacement in TypeScript).
	next.defs = make(map[string]CustomDefinitionRow)
	next.npcs = make(map[int]*NpcType)
	next.locs = make(map[int]*LocType)
	next.defObjs = make(map[int]*ObjType)
	next.maxDefID = make(map[CustomDefinitionKind]int)

	for _, row := range rows {
		count := baseCountFor(counts, row.Kind)
		validBase := row.BaseID == nil || (*row.BaseID >= 0 && *row.BaseID < count)
		if row.Kind != CustomKindLoc && row.Kind != CustomKindNPC && row.Kind != CustomKindItem ||
			row.ID < maxInt(customOverlayIDStart, count) || row.ID >= customDefinitionMaxID ||
			!validBase || next.hasDef(row.Kind, row.ID) {
			return 0, fmt.Errorf("Invalid custom definition identity: kind=%q id=%d baseId=%s", row.Kind, row.ID, formatBaseID(row.BaseID))
		}
		if row.Kind == CustomKindItem {
			if _, ok := next.items[row.ID]; ok {
				return 0, fmt.Errorf("Item ID already belongs to custom-items: %d", row.ID)
			}
		}
		for key := range row.Fields {
			if isForbiddenCustomField(key) {
				return 0, fmt.Errorf("Invalid custom definition field: %q for %s %d", key, row.Kind, row.ID)
			}
		}
		next.defs[customOverlayKey(row.Kind, row.ID)] = CustomDefinitionRow{
			Kind: row.Kind, ID: row.ID, BaseID: cloneIntPtr(row.BaseID), Fields: cloneFields(row.Fields),
		}
	}
	// The encodeContentData check, approximated: the dataset must
	// be JSON-encodable (see the file header). Only non-empty
	// registrations are checked, as in TypeScript.
	if len(next.defs) > 0 {
		dataset := customDefinitionDataset(next)
		if _, err := json.Marshal(dataset); err != nil {
			return 0, fmt.Errorf("Cannot encode custom definitions from %s: %v", customDefinitionSource, err)
		}
	}
	// Eager resolution: a mistyped field fails registration here,
	// not the first lookup.
	for _, row := range rows {
		if err := next.resolveDefinition(d, row); err != nil {
			return 0, err
		}
		if row.ID >= next.maxDefID[row.Kind] {
			next.maxDefID[row.Kind] = row.ID
		}
	}
	d.archives.setCustomOverlay(next)
	d.installCustomOverlayLookups()
	return len(rows), nil
}

func (o *customOverlay) hasDef(kind CustomDefinitionKind, id int) bool {
	_, ok := o.defs[customOverlayKey(kind, id)]
	return ok
}

func formatBaseID(baseID *int) string {
	if baseID == nil {
		return "null"
	}
	return strconv.Itoa(*baseID)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// customDefinitionDataset renders the overlay rows the way
// customDefinitionDataset does (CacheDefinitions.ts:43-49), for
// the encodability check.
func customDefinitionDataset(o *customOverlay) map[string]any {
	rows := make([]any, 0, len(o.defs))
	for _, row := range o.defs {
		rows = append(rows, map[string]any{
			"kind": row.Kind, "id": row.ID, "baseId": row.BaseID, "fields": row.Fields,
		})
	}
	return map[string]any{"key": "customDefinitions", "rows": rows}
}

// resolveDefinition builds one overlay definition: the base
// definition cloned with its overridden fields, or a blank
// definition finalized with Post when baseId is null. It mirrors
// applyCustomDefinition (CacheDefinitions.ts:774-807), including
// the loc isInteractive derivation when fields override actions
// without isInteractive.
func (o *customOverlay) resolveDefinition(d *CacheDefinitions, row CustomDefinitionRow) error {
	switch row.Kind {
	case CustomKindNPC:
		var base *NpcType
		if row.BaseID == nil {
			base = NewNpcType(row.ID, d.info)
		} else {
			base = cloneNpcType(d.GetNpc(*row.BaseID), row.ID)
		}
		if err := applyNpcFields(base, row.Fields, row.ID); err != nil {
			return err
		}
		if row.BaseID == nil {
			base.Post()
		}
		o.npcs[row.ID] = base
	case CustomKindLoc:
		var base *LocType
		if row.BaseID == nil {
			base = NewLocType(row.ID, d.info)
		} else {
			base = cloneLocType(d.GetObject(*row.BaseID), row.ID)
		}
		if err := applyLocFields(base, row.Fields, row.ID); err != nil {
			return err
		}
		if _, hasActions := row.Fields["actions"]; hasActions {
			if _, hasInteractive := row.Fields["isInteractive"]; !hasInteractive {
				base.IsInteractive = 0
				for _, action := range base.Actions {
					if action != "" {
						base.IsInteractive = 1
						break
					}
				}
			}
		}
		if row.BaseID == nil {
			base.Post()
		}
		o.locs[row.ID] = base
	case CustomKindItem:
		obj, err := resolveCustomObj(d, row.ID, row.BaseID, row.Fields)
		if err != nil {
			return err
		}
		o.defObjs[row.ID] = obj
	default:
		return fmt.Errorf("Invalid custom definition identity: kind=%q id=%d", row.Kind, row.ID)
	}
	return nil
}

// resolveCustomObj builds an overlay item from a base item clone
// (or a blank ObjType when baseID is nil) with fields applied. The
// result is marked post-processed: TypeScript's getItem returns
// custom types directly, without the note/bought/placeholder
// derivations the post-processed loader applies, and the marker
// makes the Go wrapper return overlay items untouched too.
func resolveCustomObj(d *CacheDefinitions, id int, baseID *int, fields map[string]any) (*ObjType, error) {
	var base *ObjType
	if baseID == nil {
		base = NewObjType(id, d.info)
	} else {
		base = cloneObjType(d.GetItem(*baseID), id)
	}
	if err := applyObjFields(base, fields, id); err != nil {
		return nil, err
	}
	base.postProcessed = true
	return base, nil
}

// RegisterCustomItems registers custom items, replacing any items
// a previous call registered. It mirrors registerCustomItems
// (CacheDefinitions.ts:597-683): id-range, duplicate,
// custom-definition-collision, objType/name, base-item, and model
// validation, with publication only on full success. Custom
// definitions registered by RegisterCustomDefinitions are
// preserved.
//
// A nil receiver is an error; use ApplyCustomOverlays for the
// NoCache-tolerant boot path.
func (d *CacheDefinitions) RegisterCustomItems(rows []CustomItem) error {
	if d == nil || d.archives == nil {
		return errors.New("RegisterCustomItems: no cache definitions loaded")
	}
	counts := d.baseCounts()
	next := d.archives.getCustomOverlay().clone()
	next.items = make(map[int]CustomItem)
	next.itemObjs = make(map[int]*ObjType)
	modelIDs := make(map[int]bool)

	for _, row := range rows {
		if row.ID < customOverlayIDStart || row.ID < counts.Items || row.ID > customItemMaxID {
			return fmt.Errorf("[custom-items] id must be in 50000..65022: %d", row.ID)
		}
		if next.hasDef(CustomKindItem, row.ID) {
			return fmt.Errorf("[custom-items] item id %d already belongs to a custom definition", row.ID)
		}
		if _, ok := next.items[row.ID]; ok {
			return fmt.Errorf("[custom-items] duplicate item id: %d", row.ID)
		}
		if row.ObjType == nil {
			return fmt.Errorf("[custom-items] %d is missing objType", row.ID)
		}
		name, ok := row.ObjType["name"]
		if !ok {
			return fmt.Errorf("[custom-items] %d is missing objType.name", row.ID)
		}
		nameStr, ok := name.(string)
		if !ok || strings.TrimSpace(nameStr) == "" {
			return fmt.Errorf("[custom-items] %d is missing objType.name", row.ID)
		}
		if row.BaseItemID != nil {
			base := *row.BaseItemID
			if base < 0 || base >= counts.Items || d.GetItem(base).Name == "null" {
				return fmt.Errorf("[custom-items] %d has invalid baseItemId: %d", row.ID, base)
			}
		}
		if err := validateCustomItemSpecialAttack(row); err != nil {
			return err
		}
		for rawModelID := range row.Models {
			modelID, err := strconv.Atoi(rawModelID)
			if err != nil || modelID < 1000000 || modelID > math.MaxInt32 || modelIDs[modelID] {
				return fmt.Errorf("[custom-items] invalid or duplicate model id: %s", rawModelID)
			}
			modelIDs[modelID] = true
		}
		if err := validateCustomItemObjType(row.ID, row.ObjType); err != nil {
			return err
		}
		obj, err := resolveCustomObj(d, row.ID, row.BaseItemID, row.ObjType)
		if err != nil {
			return err
		}
		if len(obj.RecolorFrom) != len(obj.RecolorTo) || len(obj.RetextureFrom) != len(obj.RetextureTo) {
			return fmt.Errorf("[custom-items] %d has mismatched recolour/retexture arrays", row.ID)
		}
		if obj.InventoryActions == nil {
			obj.InventoryActions = []string{"", "", "", "", "Drop"}
		}
		next.items[row.ID] = cloneCustomItem(row)
		next.itemObjs[row.ID] = obj
	}
	d.archives.setCustomOverlay(next)
	d.installCustomOverlayLookups()
	return nil
}

// validateCustomItemSpecialAttack mirrors validateSpecialAttack
// (ItemDefinitionLoader.plugin.js): an object with only from/energy
// keys (enforced at file-parse time for JSON rows), at least one
// present, from a positive id other than the item itself, energy
// 1..100.
func validateCustomItemSpecialAttack(row CustomItem) error {
	special := row.SpecialAttack
	if special == nil {
		return nil
	}
	valid := (special.From != nil || special.Energy != nil) &&
		(special.From == nil || (*special.From > 0 && *special.From != row.ID)) &&
		(special.Energy == nil || (*special.Energy >= 1 && *special.Energy <= 100))
	if !valid {
		return fmt.Errorf("[custom-items] invalid specialAttack for item %d: { \"from\"?: <another item id>, \"energy\"?: 1-100 }, at least one", row.ID)
	}
	return nil
}

// validateCustomItemObjType mirrors the per-key validation loop of
// registerCustomItems (CacheDefinitions.ts:645-676) over the raw
// objType bag: forbidden keys, the five-entry action arrays, and
// integer recolour/retexture arrays. Per-field type checks
// against the target struct happen in applyObjFields.
func validateCustomItemObjType(id int, objType map[string]any) error {
	for key, value := range objType {
		if isForbiddenCustomField(key) {
			return fmt.Errorf("[custom-items] %d has invalid objType property: %s", id, key)
		}
		switch key {
		case "groundActions", "inventoryActions":
			actions, ok := value.([]any)
			if !ok || len(actions) != 5 {
				return fmt.Errorf("[custom-items] %d requires five %s entries", id, key)
			}
			for _, action := range actions {
				if action != nil {
					if _, ok := action.(string); !ok {
						return fmt.Errorf("[custom-items] %d requires five %s entries", id, key)
					}
				}
			}
		case "recolorFrom", "recolorTo", "retextureFrom", "retextureTo":
			arr, ok := value.([]any)
			if !ok {
				return fmt.Errorf("[custom-items] %d has invalid %s", id, key)
			}
			for _, entry := range arr {
				if _, ok := customInt(entry); !ok {
					return fmt.Errorf("[custom-items] %d has invalid %s", id, key)
				}
			}
		}
	}
	return nil
}

// isForbiddenCustomField reports whether a field key is one neither
// registration path allows (CacheDefinitions.ts:649,720): the
// identity/cache fields and the prototype-pollution keys.
func isForbiddenCustomField(key string) bool {
	switch key {
	case "id", "cacheInfo", "cacheType", "__proto__", "constructor", "prototype":
		return true
	default:
		return false
	}
}

// GetCustomDefinitions returns the registered overlay rows, with
// cloned field bags (TypeScript returns structuredClone rows,
// CacheDefinitions.ts:741-743).
func (d *CacheDefinitions) GetCustomDefinitions() []CustomDefinitionRow {
	if d == nil || d.archives == nil {
		return nil
	}
	o := d.archives.getCustomOverlay()
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	rows := make([]CustomDefinitionRow, 0, len(o.defs))
	for _, row := range o.defs {
		rows = append(rows, CustomDefinitionRow{Kind: row.Kind, ID: row.ID, BaseID: cloneIntPtr(row.BaseID), Fields: cloneFields(row.Fields)})
	}
	return rows
}

// GetCustomItems returns the registered custom items, with cloned
// field bags. It mirrors getCustomItems
// (CacheDefinitions.ts:685-687).
func (d *CacheDefinitions) GetCustomItems() []CustomItem {
	if d == nil || d.archives == nil {
		return nil
	}
	o := d.archives.getCustomOverlay()
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	items := make([]CustomItem, 0, len(o.items))
	for _, item := range o.items {
		items = append(items, cloneCustomItem(item))
	}
	return items
}

// HasCustomDefinition reports whether id is authored as a custom
// definition of kind rather than read from the cache. It mirrors
// hasCustomDefinition (CacheDefinitions.ts:759-761). Custom items
// are not custom definitions: use GetCustomItems for those.
func (d *CacheDefinitions) HasCustomDefinition(kind CustomDefinitionKind, id int) bool {
	if d == nil || d.archives == nil {
		return false
	}
	o := d.archives.getCustomOverlay()
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.defs[customOverlayKey(kind, id)]
	return ok
}

// cloneNpcType clones base with the overlay id, deep-copying every
// slice and the params map the way applyCustomDefinition's
// structuredClone does (CacheDefinitions.ts:781-785), so applying
// fields can never mutate the loader-cached base.
func cloneNpcType(base *NpcType, id int) *NpcType {
	if base == nil {
		return NewNpcType(id, CacheInfo{})
	}
	clone := *base
	clone.id = id
	clone.ModelIds = append([]int(nil), base.ModelIds...)
	clone.ChatheadModelIds = append([]int(nil), base.ChatheadModelIds...)
	clone.RecolorFrom = append([]int(nil), base.RecolorFrom...)
	clone.RecolorTo = append([]int(nil), base.RecolorTo...)
	clone.RetextureFrom = append([]int(nil), base.RetextureFrom...)
	clone.RetextureTo = append([]int(nil), base.RetextureTo...)
	clone.Actions = append([]string(nil), base.Actions...)
	clone.HeadIconSpriteIds = append([]int(nil), base.HeadIconSpriteIds...)
	clone.HeadIconSpriteIndices = append([]int(nil), base.HeadIconSpriteIndices...)
	clone.Transforms = append([]int(nil), base.Transforms...)
	clone.Params = cloneParams(base.Params)
	clone.Desc = cloneStringPtr(base.Desc)
	return &clone
}

// cloneLocType clones base with the overlay id; see cloneNpcType.
func cloneLocType(base *LocType, id int) *LocType {
	if base == nil {
		return NewLocType(id, CacheInfo{})
	}
	clone := *base
	clone.id = id
	if base.Models != nil {
		clone.Models = make([][]int, len(base.Models))
		for i, group := range base.Models {
			clone.Models[i] = append([]int(nil), group...)
		}
	}
	clone.Types = append([]int(nil), base.Types...)
	clone.RecolorFrom = append([]int(nil), base.RecolorFrom...)
	clone.RecolorTo = append([]int(nil), base.RecolorTo...)
	clone.RetextureFrom = append([]int(nil), base.RetextureFrom...)
	clone.RetextureTo = append([]int(nil), base.RetextureTo...)
	clone.Actions = append([]string(nil), base.Actions...)
	clone.Transforms = append([]int(nil), base.Transforms...)
	clone.AmbientSoundIds = append([]int(nil), base.AmbientSoundIds...)
	clone.RandomSeqIds = append([]int(nil), base.RandomSeqIds...)
	clone.RandomSeqDelays = append([]int(nil), base.RandomSeqDelays...)
	clone.Params = cloneParams(base.Params)
	clone.Desc = cloneStringPtr(base.Desc)
	return &clone
}

// cloneObjType clones base with the overlay id; see cloneNpcType.
func cloneObjType(base *ObjType, id int) *ObjType {
	if base == nil {
		return NewObjType(id, CacheInfo{})
	}
	clone := *base
	clone.id = id
	clone.RecolorFrom = append([]int(nil), base.RecolorFrom...)
	clone.RecolorTo = append([]int(nil), base.RecolorTo...)
	clone.RetextureFrom = append([]int(nil), base.RetextureFrom...)
	clone.RetextureTo = append([]int(nil), base.RetextureTo...)
	clone.GroundActions = append([]string(nil), base.GroundActions...)
	clone.InventoryActions = append([]string(nil), base.InventoryActions...)
	if base.Subops != nil {
		clone.Subops = make([][]*string, len(base.Subops))
		for i, row := range base.Subops {
			if row == nil {
				continue
			}
			clone.Subops[i] = make([]*string, len(row))
			for j, cell := range row {
				clone.Subops[i][j] = cloneStringPtr(cell)
			}
		}
	}
	clone.CountObj = append([]int(nil), base.CountObj...)
	clone.CountCo = append([]int(nil), base.CountCo...)
	clone.Params = cloneParams(base.Params)
	clone.Examine = cloneStringPtr(base.Examine)
	return &clone
}

func cloneParams(params ParamsMap) ParamsMap {
	if params == nil {
		return nil
	}
	out := make(ParamsMap, len(params))
	for k, v := range params {
		out[k] = v
	}
	return out
}

func cloneStringPtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := *s
	return &v
}

// --- JSON value helpers ---
//
// Field bags arrive from encoding/json with UseNumber decoding
// (see the file readers below), so numbers are json.Number; the
// helpers also accept the plain Go values a programmatic caller
// supplies. A non-integral number is never a valid int field:
// TypeScript stores 1.5 in a numeric field without complaint, but
// every Go target field is an int, so fractional values are
// rejected rather than truncated.

// customInt converts a JSON value to an int, reporting false for
// non-numbers, fractional numbers, and out-of-range numbers.
func customInt(v any) (int, bool) {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		// Int64 rejects "1.5" but accepts "1e3"; require the
		// literal to be integral by round-tripping through float
		// comparison only when it parses as a float.
		if f, err := n.Float64(); err == nil && f != math.Trunc(f) {
			return 0, false
		}
		if i > math.MaxInt || i < math.MinInt {
			return 0, false
		}
		return int(i), true
	case float64:
		if n != math.Trunc(n) || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		if n > float64(math.MaxInt) || n < float64(math.MinInt) {
			return 0, false
		}
		return int(n), true
	case float32:
		return customInt(float64(n))
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		if n > math.MaxInt || n < math.MinInt {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}

func customString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func customBool(v any) (bool, bool) {
	b, ok := v.(bool)
	return b, ok
}

// customIntSlice converts a JSON array to []int; a JSON null
// becomes a nil slice (the TypeScript field set to null).
func customIntSlice(v any) ([]int, bool) {
	if v == nil {
		return nil, true
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]int, len(arr))
	for i, entry := range arr {
		n, ok := customInt(entry)
		if !ok {
			return nil, false
		}
		out[i] = n
	}
	return out, true
}

// customStringSlice converts a JSON array to []string, with JSON
// null entries becoming "" (the Go absent-slot convention); a JSON
// null becomes a nil slice.
func customStringSlice(v any) ([]string, bool) {
	if v == nil {
		return nil, true
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, len(arr))
	for i, entry := range arr {
		if entry == nil {
			continue
		}
		s, ok := entry.(string)
		if !ok {
			return nil, false
		}
		out[i] = s
	}
	return out, true
}

// customParams converts a JSON object to a ParamsMap: keys are
// decimal param ids, values are ints (stored int32, as
// ReadParamsMap stores them) or strings. It mirrors the params
// special case in applyCustomDefinition
// (CacheDefinitions.ts:786-788), with the editor audit's key/value
// validation (Document.auditRow: keys /^\d+$/ <= 0xffffff, values
// string or integer).
func customParams(v any) (ParamsMap, bool) {
	if v == nil {
		return nil, true
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	out := make(ParamsMap, len(obj))
	for key, entry := range obj {
		if key == "" {
			return nil, false
		}
		for _, r := range key {
			if r < '0' || r > '9' {
				return nil, false
			}
		}
		id, err := strconv.Atoi(key)
		if err != nil || id > 0xffffff {
			return nil, false
		}
		switch e := entry.(type) {
		case string:
			out[id] = e
		default:
			n, ok := customInt(entry)
			if !ok || n > math.MaxInt32 || n < math.MinInt32 {
				return nil, false
			}
			out[id] = int32(n)
		}
	}
	return out, true
}

// customIntMatrix converts a JSON array of arrays to [][]int (the
// loc models field); a JSON null becomes a nil matrix.
func customIntMatrix(v any) ([][]int, bool) {
	if v == nil {
		return nil, true
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([][]int, len(arr))
	for i, entry := range arr {
		group, ok := customIntSlice(entry)
		if !ok {
			return nil, false
		}
		out[i] = group
	}
	return out, true
}

// customSubops converts the nested subops JSON: an array whose
// entries are null or arrays of null|string; a JSON null becomes
// a nil subops table.
func customSubops(v any) ([][]*string, bool) {
	if v == nil {
		return nil, true
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([][]*string, len(arr))
	for i, entry := range arr {
		if entry == nil {
			continue
		}
		row, ok := entry.([]any)
		if !ok {
			return nil, false
		}
		out[i] = make([]*string, len(row))
		for j, cell := range row {
			if cell == nil {
				continue
			}
			s, ok := cell.(string)
			if !ok {
				return nil, false
			}
			out[i][j] = &s
		}
	}
	return out, true
}

// customFieldError renders a mistyped-field error in the
// registerCustomItems style, naming the definition and field.
func customFieldError(kind CustomDefinitionKind, id int, field, want string) error {
	return fmt.Errorf("invalid custom %s %d field %q: want %s", kind, id, field, want)
}

// --- Field appliers ---
//
// Each applier assigns one JSON field bag onto a cloned
// definition, mapping TypeScript field names (ObjType.ts,
// NpcType.ts, LocType.ts) to Go struct fields. Unknown field
// names are errors: TypeScript's Object.assign would silently
// attach them, but the cache editor's save-time audit rejects
// unknown fields (Document.auditRow), and a Go struct cannot hold
// them. Forbidden keys are rejected by the callers before the
// applier runs; the applier rejects them again if reached
// directly.

// applyObjFields applies fields to an item definition. The field
// set is every ObjType field (ObjType.ts:8-101); the editor's
// item field table (Fields.CacheEditor.js) is a subset used for
// forms, while registerCustomItems allows any ObjType property,
// so the full set is supported here.
func applyObjFields(dst *ObjType, fields map[string]any, id int) error {
	setInt := func(field string, target *int) error {
		n, ok := customInt(fields[field])
		if !ok {
			return customFieldError(CustomKindItem, id, field, "an integer")
		}
		*target = n
		return nil
	}
	setString := func(field string, target *string) error {
		s, ok := customString(fields[field])
		if !ok {
			return customFieldError(CustomKindItem, id, field, "a string")
		}
		*target = s
		return nil
	}
	setBool := func(field string, target *bool) error {
		b, ok := customBool(fields[field])
		if !ok {
			return customFieldError(CustomKindItem, id, field, "a boolean")
		}
		*target = b
		return nil
	}
	setIntSlice := func(field string, target *[]int) error {
		s, ok := customIntSlice(fields[field])
		if !ok {
			return customFieldError(CustomKindItem, id, field, "an array of integers")
		}
		*target = s
		return nil
	}
	setStringSlice := func(field string, target *[]string) error {
		s, ok := customStringSlice(fields[field])
		if !ok {
			return customFieldError(CustomKindItem, id, field, "an array of strings or nulls")
		}
		*target = s
		return nil
	}
	for field, value := range fields {
		_ = value
		var err error
		switch field {
		case "model":
			err = setInt(field, &dst.Model)
		case "name":
			err = setString(field, &dst.Name)
		case "examine":
			s, ok := customString(fields[field])
			if !ok {
				err = customFieldError(CustomKindItem, id, field, "a string")
			} else {
				dst.Examine = &s
			}
		case "recolorFrom":
			err = setIntSlice(field, &dst.RecolorFrom)
		case "recolorTo":
			err = setIntSlice(field, &dst.RecolorTo)
		case "retextureFrom":
			err = setIntSlice(field, &dst.RetextureFrom)
		case "retextureTo":
			err = setIntSlice(field, &dst.RetextureTo)
		case "zoom2d":
			err = setInt(field, &dst.Zoom2d)
		case "xan2d":
			err = setInt(field, &dst.Xan2d)
		case "yan2d":
			err = setInt(field, &dst.Yan2d)
		case "zan2d":
			err = setInt(field, &dst.Zan2d)
		case "offsetX2d":
			err = setInt(field, &dst.OffsetX2d)
		case "offsetY2d":
			err = setInt(field, &dst.OffsetY2d)
		case "op9":
			err = setString(field, &dst.Op9)
		case "stackability":
			n, ok := customInt(fields[field])
			if !ok {
				err = customFieldError(CustomKindItem, id, field, "an integer")
			} else {
				dst.Stackability = ObjStackability(n)
			}
		case "price":
			err = setInt(field, &dst.Price)
		case "wearPos":
			err = setInt(field, &dst.WearPos)
		case "wearPos2":
			err = setInt(field, &dst.WearPos2)
		case "wearPos3":
			err = setInt(field, &dst.WearPos3)
		case "isMembers":
			err = setBool(field, &dst.IsMembers)
		case "groundActions":
			err = setStringSlice(field, &dst.GroundActions)
		case "inventoryActions":
			err = setStringSlice(field, &dst.InventoryActions)
		case "subops":
			s, ok := customSubops(fields[field])
			if !ok {
				err = customFieldError(CustomKindItem, id, field, "an array of arrays of strings or nulls")
			} else {
				dst.Subops = s
			}
		case "shiftClickIndex":
			err = setInt(field, &dst.ShiftClickIndex)
		case "maleModel":
			err = setInt(field, &dst.MaleModel)
		case "maleModel1":
			err = setInt(field, &dst.MaleModel1)
		case "maleOffset":
			err = setInt(field, &dst.MaleOffset)
		case "femaleModel":
			err = setInt(field, &dst.FemaleModel)
		case "femaleModel1":
			err = setInt(field, &dst.FemaleModel1)
		case "femaleOffset":
			err = setInt(field, &dst.FemaleOffset)
		case "maleModel2":
			err = setInt(field, &dst.MaleModel2)
		case "femaleModel2":
			err = setInt(field, &dst.FemaleModel2)
		case "maleHeadModel":
			err = setInt(field, &dst.MaleHeadModel)
		case "maleHeadModel2":
			err = setInt(field, &dst.MaleHeadModel2)
		case "femaleHeadModel":
			err = setInt(field, &dst.FemaleHeadModel)
		case "femaleHeadModel2":
			err = setInt(field, &dst.FemaleHeadModel2)
		case "countObj":
			err = setIntSlice(field, &dst.CountObj)
		case "countCo":
			err = setIntSlice(field, &dst.CountCo)
		case "note":
			err = setInt(field, &dst.Note)
		case "noteTemplate":
			err = setInt(field, &dst.NoteTemplate)
		case "resizeX":
			err = setInt(field, &dst.ResizeX)
		case "resizeY":
			err = setInt(field, &dst.ResizeY)
		case "resizeZ":
			err = setInt(field, &dst.ResizeZ)
		case "ambient":
			err = setInt(field, &dst.Ambient)
		case "contrast":
			err = setInt(field, &dst.Contrast)
		case "team":
			err = setInt(field, &dst.Team)
		case "isTradable":
			err = setBool(field, &dst.IsTradable)
		case "isUnlockable":
			err = setBool(field, &dst.IsUnlockable)
		case "weight":
			err = setInt(field, &dst.Weight)
		case "category":
			err = setInt(field, &dst.Category)
		case "unnotedId":
			err = setInt(field, &dst.UnnotedID)
		case "notedId":
			err = setInt(field, &dst.NotedID)
		case "placeholder":
			err = setInt(field, &dst.Placeholder)
		case "placeholderTemplate":
			err = setInt(field, &dst.PlaceholderTemplate)
		case "params":
			p, ok := customParams(fields[field])
			if !ok {
				err = customFieldError(CustomKindItem, id, field, "an object of param id to integer or string")
			} else {
				dst.Params = p
			}
		case "manwearXOff":
			err = setInt(field, &dst.ManwearXOff)
		case "manwearYOff":
			err = setInt(field, &dst.ManwearYOff)
		case "manwearZOff":
			err = setInt(field, &dst.ManwearZOff)
		case "womanwearXOff":
			err = setInt(field, &dst.WomanwearXOff)
		case "womanwearYOff":
			err = setInt(field, &dst.WomanwearYOff)
		case "womanwearZOff":
			err = setInt(field, &dst.WomanwearZOff)
		default:
			if isForbiddenCustomField(field) {
				return fmt.Errorf("Invalid custom definition field: %q for item %d", field, id)
			}
			return fmt.Errorf("unknown custom item field %q for item %d", field, id)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// applyNpcFields applies fields to an NPC definition. The field
// set is every NpcType field (NpcType.ts:15-101).
func applyNpcFields(dst *NpcType, fields map[string]any, id int) error {
	setInt := func(field string, target *int) error {
		n, ok := customInt(fields[field])
		if !ok {
			return customFieldError(CustomKindNPC, id, field, "an integer")
		}
		*target = n
		return nil
	}
	setBool := func(field string, target *bool) error {
		b, ok := customBool(fields[field])
		if !ok {
			return customFieldError(CustomKindNPC, id, field, "a boolean")
		}
		*target = b
		return nil
	}
	setIntSlice := func(field string, target *[]int) error {
		s, ok := customIntSlice(fields[field])
		if !ok {
			return customFieldError(CustomKindNPC, id, field, "an array of integers")
		}
		*target = s
		return nil
	}
	for field := range fields {
		var err error
		switch field {
		case "name":
			s, ok := customString(fields[field])
			if !ok {
				err = customFieldError(CustomKindNPC, id, field, "a string")
			} else {
				dst.Name = s
			}
		case "desc":
			s, ok := customString(fields[field])
			if !ok {
				err = customFieldError(CustomKindNPC, id, field, "a string")
			} else {
				dst.Desc = &s
			}
		case "size":
			err = setInt(field, &dst.Size)
		case "modelIds":
			err = setIntSlice(field, &dst.ModelIds)
		case "chatheadModelIds":
			err = setIntSlice(field, &dst.ChatheadModelIds)
		case "idleSeqId":
			err = setInt(field, &dst.IdleSeqId)
		case "turnLeftSeqId":
			err = setInt(field, &dst.TurnLeftSeqId)
		case "turnRightSeqId":
			err = setInt(field, &dst.TurnRightSeqId)
		case "walkSeqId":
			err = setInt(field, &dst.WalkSeqId)
		case "walkBackSeqId":
			err = setInt(field, &dst.WalkBackSeqId)
		case "walkLeftSeqId":
			err = setInt(field, &dst.WalkLeftSeqId)
		case "walkRightSeqId":
			err = setInt(field, &dst.WalkRightSeqId)
		case "runSeqId":
			err = setInt(field, &dst.RunSeqId)
		case "runBackSeqId":
			err = setInt(field, &dst.RunBackSeqId)
		case "runLeftSeqId":
			err = setInt(field, &dst.RunLeftSeqId)
		case "runRightSeqId":
			err = setInt(field, &dst.RunRightSeqId)
		case "crawlSeqId":
			err = setInt(field, &dst.CrawlSeqId)
		case "crawlBackSeqId":
			err = setInt(field, &dst.CrawlBackSeqId)
		case "crawlLeftSeqId":
			err = setInt(field, &dst.CrawlLeftSeqId)
		case "crawlRightSeqId":
			err = setInt(field, &dst.CrawlRightSeqId)
		case "recolorFrom":
			err = setIntSlice(field, &dst.RecolorFrom)
		case "recolorTo":
			err = setIntSlice(field, &dst.RecolorTo)
		case "retextureFrom":
			err = setIntSlice(field, &dst.RetextureFrom)
		case "retextureTo":
			err = setIntSlice(field, &dst.RetextureTo)
		case "actions":
			s, ok := customStringSlice(fields[field])
			if !ok {
				err = customFieldError(CustomKindNPC, id, field, "an array of strings or nulls")
			} else {
				dst.Actions = s
			}
		case "drawMapDot":
			err = setBool(field, &dst.DrawMapDot)
		case "combatLevel":
			err = setInt(field, &dst.CombatLevel)
		case "attackLevel":
			err = setInt(field, &dst.AttackLevel)
		case "defenceLevel":
			err = setInt(field, &dst.DefenceLevel)
		case "strengthLevel":
			err = setInt(field, &dst.StrengthLevel)
		case "hitpoints":
			err = setInt(field, &dst.Hitpoints)
		case "rangedLevel":
			err = setInt(field, &dst.RangedLevel)
		case "magicLevel":
			err = setInt(field, &dst.MagicLevel)
		case "attackSpeed":
			err = setInt(field, &dst.AttackSpeed)
		case "widthScale":
			err = setInt(field, &dst.WidthScale)
		case "heightScale":
			err = setInt(field, &dst.HeightScale)
		case "isVisible":
			err = setBool(field, &dst.IsVisible)
		case "drawPriority":
			n, ok := customInt(fields[field])
			if !ok {
				err = customFieldError(CustomKindNPC, id, field, "an integer")
			} else {
				dst.DrawPriority = NpcDrawPriority(n)
			}
		case "ambient":
			err = setInt(field, &dst.Ambient)
		case "contrast":
			err = setInt(field, &dst.Contrast)
		case "headIconPrayer":
			err = setInt(field, &dst.HeadIconPrayer)
		case "headIconSpriteIds":
			err = setIntSlice(field, &dst.HeadIconSpriteIds)
		case "headIconSpriteIndices":
			err = setIntSlice(field, &dst.HeadIconSpriteIndices)
		case "rotationSpeed":
			err = setInt(field, &dst.RotationSpeed)
		case "canHideForOverlap":
			err = setBool(field, &dst.CanHideForOverlap)
		case "overlapTintHsl":
			err = setInt(field, &dst.OverlapTintHsl)
		case "transforms":
			err = setIntSlice(field, &dst.Transforms)
		case "transformVarbit":
			err = setInt(field, &dst.TransformVarbit)
		case "transformVarp":
			err = setInt(field, &dst.TransformVarp)
		case "isInteractable":
			err = setBool(field, &dst.IsInteractable)
		case "isClickable":
			err = setBool(field, &dst.IsClickable)
		case "isClipped":
			err = setBool(field, &dst.IsClipped)
		case "isFollower":
			err = setBool(field, &dst.IsFollower)
		case "category":
			err = setInt(field, &dst.Category)
		case "loginScreenProps":
			err = setInt(field, &dst.LoginScreenProps)
		case "spawnDirection":
			err = setInt(field, &dst.SpawnDirection)
		case "heightOffset":
			err = setInt(field, &dst.HeightOffset)
		case "footprintSize":
			err = setInt(field, &dst.FootprintSize)
		case "basTypeId":
			err = setInt(field, &dst.BasTypeId)
		case "params":
			p, ok := customParams(fields[field])
			if !ok {
				err = customFieldError(CustomKindNPC, id, field, "an object of param id to integer or string")
			} else {
				dst.Params = p
			}
		case "isPet":
			err = setBool(field, &dst.IsPet)
		default:
			if isForbiddenCustomField(field) {
				return fmt.Errorf("Invalid custom definition field: %q for npc %d", field, id)
			}
			return fmt.Errorf("unknown custom npc field %q for npc %d", field, id)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// applyLocFields applies fields to an object definition. The field
// set is every LocType field (LocType.ts:12-104).
func applyLocFields(dst *LocType, fields map[string]any, id int) error {
	setInt := func(field string, target *int) error {
		n, ok := customInt(fields[field])
		if !ok {
			return customFieldError(CustomKindLoc, id, field, "an integer")
		}
		*target = n
		return nil
	}
	setBool := func(field string, target *bool) error {
		b, ok := customBool(fields[field])
		if !ok {
			return customFieldError(CustomKindLoc, id, field, "a boolean")
		}
		*target = b
		return nil
	}
	setIntSlice := func(field string, target *[]int) error {
		s, ok := customIntSlice(fields[field])
		if !ok {
			return customFieldError(CustomKindLoc, id, field, "an array of integers")
		}
		*target = s
		return nil
	}
	for field := range fields {
		var err error
		switch field {
		case "lowDetail":
			err = setBool(field, &dst.LowDetail)
		case "models":
			m, ok := customIntMatrix(fields[field])
			if !ok {
				err = customFieldError(CustomKindLoc, id, field, "an array of arrays of integers")
			} else {
				dst.Models = m
			}
		case "types":
			err = setIntSlice(field, &dst.Types)
		case "name":
			s, ok := customString(fields[field])
			if !ok {
				err = customFieldError(CustomKindLoc, id, field, "a string")
			} else {
				dst.Name = s
			}
		case "category":
			err = setInt(field, &dst.Category)
		case "desc":
			s, ok := customString(fields[field])
			if !ok {
				err = customFieldError(CustomKindLoc, id, field, "a string")
			} else {
				dst.Desc = &s
			}
		case "recolorFrom":
			err = setIntSlice(field, &dst.RecolorFrom)
		case "recolorTo":
			err = setIntSlice(field, &dst.RecolorTo)
		case "retextureFrom":
			err = setIntSlice(field, &dst.RetextureFrom)
		case "retextureTo":
			err = setIntSlice(field, &dst.RetextureTo)
		case "sizeX":
			err = setInt(field, &dst.SizeX)
		case "sizeY":
			err = setInt(field, &dst.SizeY)
		case "clipType":
			err = setInt(field, &dst.ClipType)
		case "clipMask":
			err = setInt(field, &dst.ClipMask)
		case "blocksProjectile":
			err = setBool(field, &dst.BlocksProjectile)
		case "isInteractive":
			err = setInt(field, &dst.IsInteractive)
		case "contouredGround":
			err = setInt(field, &dst.ContouredGround)
		case "contourGroundType":
			err = setInt(field, &dst.ContourGroundType)
		case "contourGroundParam":
			err = setInt(field, &dst.ContourGroundParam)
		case "mergeNormals":
			err = setBool(field, &dst.MergeNormals)
		case "modelClipped":
			err = setBool(field, &dst.ModelClipped)
		case "seqId":
			err = setInt(field, &dst.SeqId)
		case "decorDisplacement":
			err = setInt(field, &dst.DecorDisplacement)
		case "ambient":
			err = setInt(field, &dst.Ambient)
		case "contrast":
			err = setInt(field, &dst.Contrast)
		case "actions":
			s, ok := customStringSlice(fields[field])
			if !ok {
				err = customFieldError(CustomKindLoc, id, field, "an array of strings or nulls")
			} else {
				dst.Actions = s
			}
		case "mapFunctionId":
			err = setInt(field, &dst.MapFunctionId)
		case "mapSceneId":
			err = setInt(field, &dst.MapSceneId)
		case "flipMapSceneSprite":
			err = setBool(field, &dst.FlipMapSceneSprite)
		case "isRotated":
			err = setBool(field, &dst.IsRotated)
		case "clipped":
			err = setBool(field, &dst.Clipped)
		case "modelSizeX":
			err = setInt(field, &dst.ModelSizeX)
		case "modelSizeHeight":
			err = setInt(field, &dst.ModelSizeHeight)
		case "modelSizeY":
			err = setInt(field, &dst.ModelSizeY)
		case "offsetX":
			err = setInt(field, &dst.OffsetX)
		case "offsetHeight":
			err = setInt(field, &dst.OffsetHeight)
		case "offsetY":
			err = setInt(field, &dst.OffsetY)
		case "obstructsGround":
			err = setBool(field, &dst.ObstructsGround)
		case "isHollow":
			err = setBool(field, &dst.IsHollow)
		case "supportItems":
			err = setInt(field, &dst.SupportItems)
		case "transforms":
			err = setIntSlice(field, &dst.Transforms)
		case "transformVarbit":
			err = setInt(field, &dst.TransformVarbit)
		case "transformVarp":
			err = setInt(field, &dst.TransformVarp)
		case "ambientSoundId":
			err = setInt(field, &dst.AmbientSoundId)
		case "soundMaxDistance":
			err = setInt(field, &dst.SoundMaxDistance)
		case "ambientSoundChangeTicksMin":
			err = setInt(field, &dst.AmbientSoundChangeTicksMin)
		case "ambientSoundChangeTicksMax":
			err = setInt(field, &dst.AmbientSoundChangeTicksMax)
		case "soundMinDistance":
			err = setInt(field, &dst.SoundMinDistance)
		case "soundDistanceFadeCurve":
			err = setInt(field, &dst.SoundDistanceFadeCurve)
		case "soundFadeInDuration":
			err = setInt(field, &dst.SoundFadeInDuration)
		case "soundFadeOutDuration":
			err = setInt(field, &dst.SoundFadeOutDuration)
		case "soundFadeInCurve":
			err = setInt(field, &dst.SoundFadeInCurve)
		case "soundFadeOutCurve":
			err = setInt(field, &dst.SoundFadeOutCurve)
		case "soundAreaRadiusOverride":
			err = setInt(field, &dst.SoundAreaRadiusOverride)
		case "hasSpatialAmbient":
			err = setBool(field, &dst.HasSpatialAmbient)
		case "loopMultiSoundSequentially":
			err = setBool(field, &dst.LoopMultiSoundSequentially)
		case "useExactSoundPosition":
			err = setBool(field, &dst.UseExactSoundPosition)
		case "deferredAmbientSwap":
			err = setBool(field, &dst.DeferredAmbientSwap)
		case "resetAmbientOnLoopRestart":
			err = setBool(field, &dst.ResetAmbientOnLoopRestart)
		case "ambientSoundIds":
			err = setIntSlice(field, &dst.AmbientSoundIds)
		case "seqRandomStart":
			err = setBool(field, &dst.SeqRandomStart)
		case "randomSeqIds":
			err = setIntSlice(field, &dst.RandomSeqIds)
		case "randomSeqDelays":
			err = setIntSlice(field, &dst.RandomSeqDelays)
		case "params":
			p, ok := customParams(fields[field])
			if !ok {
				err = customFieldError(CustomKindLoc, id, field, "an object of param id to integer or string")
			} else {
				dst.Params = p
			}
		default:
			if isForbiddenCustomField(field) {
				return fmt.Errorf("Invalid custom definition field: %q for loc %d", field, id)
			}
			return fmt.Errorf("unknown custom loc field %q for loc %d", field, id)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// --- File readers and the boot entry point ---

// ApplyCustomOverlays reads custom-definitions.json and
// custom-items.json from the definitions directory dir and
// registers them on defs, returning how many of each were
// registered. Definitions register before items, so an item id
// that collides with a custom definition is reported by the item
// registration, as in TypeScript.
//
// Missing files behave as in TypeScript: zero rows (the cache
// editor's Document.read returns the empty document, and the
// ItemDefinitionLoader plugin skips a missing custom-items.json).
// A present file that is malformed JSON or the wrong shape is an
// error, as the TypeScript throws are. custom-shops.json is never
// read here (see the file header).
//
// When defs is nil (NoCache mode), ApplyCustomOverlays is a
// no-op: it calls logf with a note and returns zero counts and a
// nil error, without reading the files (see the file header for
// the grounding). A nil logf suppresses the note.
func ApplyCustomOverlays(dir string, defs *CacheDefinitions, logf func(format string, args ...any)) (CustomOverlayCounts, error) {
	if defs == nil {
		if logf != nil {
			logf("[definitions] custom definitions skipped: no cache loaded")
		}
		return CustomOverlayCounts{}, nil
	}
	defRows, err := readCustomDefinitionsFile(filepath.Join(dir, "custom-definitions.json"))
	if err != nil {
		return CustomOverlayCounts{}, err
	}
	itemRows, err := readCustomItemsFile(filepath.Join(dir, "custom-items.json"))
	if err != nil {
		return CustomOverlayCounts{}, err
	}
	n, err := defs.RegisterCustomDefinitions(defRows)
	if err != nil {
		return CustomOverlayCounts{}, fmt.Errorf("custom-definitions.json: %w", err)
	}
	if err := defs.RegisterCustomItems(itemRows); err != nil {
		return CustomOverlayCounts{}, fmt.Errorf("custom-items.json: %w", err)
	}
	return CustomOverlayCounts{Definitions: n, Items: len(itemRows)}, nil
}

// decodeJSONNumber decodes data as JSON with numbers preserved as
// json.Number, so integer validation sees the literal.
func decodeJSONNumber(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

// readCustomDefinitionsFile reads the cache editor's document.
// A missing file is the empty document (Document.read,
// Document.CacheEditor.cjs); a present file must be an object
// with a rows array, or it is an error ("custom-definitions.json
// is not { version, rows }"). The version field is not checked:
// neither Document.read nor registerCustomDefinitions reads it.
func readCustomDefinitionsFile(path string) ([]CustomDefinitionRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading custom-definitions.json: %w", err)
	}
	var doc map[string]json.RawMessage
	if err := decodeJSONNumber(data, &doc); err != nil {
		return nil, fmt.Errorf("custom-definitions.json: %w", err)
	}
	if doc == nil {
		return nil, errors.New("custom-definitions.json is not { version, rows }")
	}
	rawRows, ok := doc["rows"]
	if !ok {
		return nil, errors.New("custom-definitions.json is not { version, rows }")
	}
	var rows []customDefinitionFileRow
	if err := decodeJSONNumber(rawRows, &rows); err != nil {
		return nil, fmt.Errorf("custom-definitions.json: %w", err)
	}
	if rows == nil {
		return nil, errors.New("custom-definitions.json is not { version, rows }")
	}
	out := make([]CustomDefinitionRow, 0, len(rows))
	for i, row := range rows {
		parsed, err := row.toRow()
		if err != nil {
			return nil, fmt.Errorf("custom-definitions.json row %d: %w", i, err)
		}
		out = append(out, parsed)
	}
	return out, nil
}

// customDefinitionFileRow is one document row as JSON. BaseID is a
// RawMessage so an absent baseId (invalid in TypeScript:
// registerCustomDefinitions requires null or an in-range integer)
// is distinguishable from an explicit null (a blank definition).
type customDefinitionFileRow struct {
	Kind   string          `json:"kind"`
	ID     json.Number     `json:"id"`
	BaseID json.RawMessage `json:"baseId"`
	Fields map[string]any  `json:"fields"`
}

func (r customDefinitionFileRow) toRow() (CustomDefinitionRow, error) {
	id, err := r.ID.Int64()
	if err != nil {
		return CustomDefinitionRow{}, fmt.Errorf("id must be an integer: %s", r.ID.String())
	}
	if r.BaseID == nil {
		return CustomDefinitionRow{}, errors.New("baseId is required (null for a blank definition)")
	}
	var baseID *int
	trimmed := strings.TrimSpace(string(r.BaseID))
	if trimmed != "null" {
		var n json.Number
		if err := json.Unmarshal(r.BaseID, &n); err != nil {
			return CustomDefinitionRow{}, fmt.Errorf("baseId must be null or an integer: %s", trimmed)
		}
		v, err := n.Int64()
		if err != nil {
			return CustomDefinitionRow{}, fmt.Errorf("baseId must be null or an integer: %s", trimmed)
		}
		b := int(v)
		baseID = &b
	}
	fields := r.Fields
	if fields == nil {
		fields = map[string]any{}
	}
	return CustomDefinitionRow{Kind: CustomDefinitionKind(r.Kind), ID: int(id), BaseID: baseID, Fields: fields}, nil
}

// readCustomItemsFile reads custom-items.json. A missing file is
// zero rows (the plugin's existsSync guard); a present file is an
// array, or an object read as its values (the plugin's
// Array.isArray ? parsed : Object.values(parsed ?? {})).
func readCustomItemsFile(path string) ([]CustomItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading custom-items.json: %w", err)
	}
	var parsed any
	if err := decodeJSONNumber(data, &parsed); err != nil {
		return nil, fmt.Errorf("custom-items.json: %w", err)
	}
	var rawRows []any
	switch v := parsed.(type) {
	case nil:
		rawRows = nil
	case []any:
		rawRows = v
	case map[string]any:
		// Object values, in key order for determinism (TypeScript
		// Object.values follows insertion order; Go maps do not,
		// so sorted keys are the deterministic choice).
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			rawRows = append(rawRows, v[k])
		}
	default:
		return nil, errors.New("custom-items.json must contain an array or an object")
	}
	out := make([]CustomItem, 0, len(rawRows))
	for i, raw := range rawRows {
		row, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("custom-items.json row %d: each row must be an object", i)
		}
		parsed, err := customItemFromMap(row)
		if err != nil {
			return nil, fmt.Errorf("custom-items.json row %d: %w", i, err)
		}
		out = append(out, parsed)
	}
	return out, nil
}

// customItemFromMap converts one decoded row object to a
// CustomItem. Unknown top-level keys are ignored, as in
// TypeScript (registerCustomItems reads only the keys it knows);
// the objType bag itself is validated at registration.
func customItemFromMap(row map[string]any) (CustomItem, error) {
	var item CustomItem
	id, ok := customInt(row["id"])
	if !ok {
		return CustomItem{}, errors.New("id must be an integer")
	}
	item.ID = id
	if raw, present := row["baseItemId"]; present && raw != nil {
		base, ok := customInt(raw)
		if !ok {
			return CustomItem{}, fmt.Errorf("[custom-items] %d has invalid baseItemId", id)
		}
		item.BaseItemID = &base
	}
	if raw, present := row["objType"]; present && raw != nil {
		objType, ok := raw.(map[string]any)
		if !ok {
			return CustomItem{}, fmt.Errorf("[custom-items] %d is missing objType", id)
		}
		item.ObjType = objType
	}
	if raw, present := row["itemDef"]; present && raw != nil {
		itemDef, ok := raw.(map[string]any)
		if !ok {
			return CustomItem{}, fmt.Errorf("[custom-items] %d has invalid itemDef", id)
		}
		item.ItemDef = itemDef
	}
	if raw, present := row["models"]; present && raw != nil {
		models, ok := raw.(map[string]any)
		if !ok {
			return CustomItem{}, fmt.Errorf("[custom-items] %d has invalid models", id)
		}
		item.Models = make(map[string]string, len(models))
		for k, v := range models {
			path, ok := v.(string)
			if !ok {
				return CustomItem{}, fmt.Errorf("[custom-items] invalid model %s for item %d", k, id)
			}
			item.Models[k] = path
		}
	}
	if raw, present := row["specialAttack"]; present && raw != nil {
		special, ok := raw.(map[string]any)
		if !ok {
			return CustomItem{}, fmt.Errorf("[custom-items] invalid specialAttack for item %d", id)
		}
		attack := &CustomItemSpecialAttack{}
		for k, v := range special {
			switch k {
			case "from":
				n, ok := customInt(v)
				if !ok {
					return CustomItem{}, fmt.Errorf("[custom-items] invalid specialAttack for item %d", id)
				}
				attack.From = &n
			case "energy":
				n, ok := customInt(v)
				if !ok {
					return CustomItem{}, fmt.Errorf("[custom-items] invalid specialAttack for item %d", id)
				}
				attack.Energy = &n
			default:
				return CustomItem{}, fmt.Errorf("[custom-items] invalid specialAttack for item %d", id)
			}
		}
		item.SpecialAttack = attack
	}
	return item, nil
}

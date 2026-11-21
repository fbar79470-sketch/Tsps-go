package cache

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// This file ports the archived-definition overlay: definitions from
// an older cache, validated as complete bundles and layered over
// the native loaders without mutating them.
//
// Upstream sources:
//   - codec/rs/config/ArchivedDefinitionRegistry.ts
//   - codec/rs/config/ArchivedTypeLoader.ts

// ArchivedDefinitionKind is one of the four kinds the registry
// overlays. It is ArchivedDefinitionKind
// (ArchivedDefinitionRegistry.ts:345).
type ArchivedDefinitionKind string

const (
	// ArchivedKindNPC overlays NPC definitions.
	ArchivedKindNPC ArchivedDefinitionKind = "npc"
	// ArchivedKindLoc overlays object definitions.
	ArchivedKindLoc ArchivedDefinitionKind = "loc"
	// ArchivedKindObj overlays item definitions.
	ArchivedKindObj ArchivedDefinitionKind = "obj"
	// ArchivedKindSeq overlays animation definitions.
	ArchivedKindSeq ArchivedDefinitionKind = "seq"
)

var archivedKinds = []ArchivedDefinitionKind{ArchivedKindNPC, ArchivedKindLoc, ArchivedKindObj, ArchivedKindSeq}

func validArchivedKind(kind ArchivedDefinitionKind) bool {
	switch kind {
	case ArchivedKindNPC, ArchivedKindLoc, ArchivedKindObj, ArchivedKindSeq:
		return true
	default:
		return false
	}
}

// ArchivedDefinitionRow is one archived definition in a bundle
// (ArchivedDefinitionRegistry.ts:346-354). ExpectedCurrentCRC is
// present only for an in-place replacement of a retired native
// item.
type ArchivedDefinitionRow struct {
	Kind               ArchivedDefinitionKind `json:"kind"`
	Symbol             string                 `json:"symbol"`
	SourceID           int                    `json:"sourceId"`
	ID                 int                    `json:"id"`
	DataBase64         string                 `json:"dataBase64"`
	CRC                uint32                 `json:"crc"`
	ExpectedCurrentCRC *uint32                `json:"expectedCurrentCrc,omitempty"`
}

// ArchivedTargetCache identifies the cache a bundle targets: the
// name, game and revision subset of CacheInfo
// (ArchivedDefinitionRegistry.ts:357).
type ArchivedTargetCache struct {
	Name     string   `json:"name"`
	Game     GameType `json:"game"`
	Revision int      `json:"revision"`
}

// ArchivedDefinitionBundle is a validated set of archived
// definitions from one source cache
// (ArchivedDefinitionRegistry.ts:355-360).
type ArchivedDefinitionBundle struct {
	ID          string                  `json:"id"`
	TargetCache ArchivedTargetCache     `json:"targetCache"`
	SourceCache CacheInfo               `json:"sourceCache"`
	Definitions []ArchivedDefinitionRow `json:"definitions"`
}

// ArchivedNativeCounts are the native definition counts the
// registry layers over. Seq is meaningful only when SeqSupported
// is true: a context without the native sequence count rejects
// archived sequences.
type ArchivedNativeCounts struct {
	NPC          int
	Loc          int
	Obj          int
	Seq          int
	SeqSupported bool
}

// ArchivedDefinitionContext supplies the registry with the target
// cache, the native counts, and access to current native
// definitions for replacement validation. It mirrors
// ArchivedDefinitionContext
// (ArchivedDefinitionRegistry.ts:361-369). CurrentDefinition
// returns the native definition's name and raw file bytes; ok is
// false when the native archive has no such file.
type ArchivedDefinitionContext struct {
	CacheInfo CacheInfo
	Counts    ArchivedNativeCounts
	// CurrentDefinition reports the native definition for kind/id.
	// Sequences have no name; only retired items are ever replaced
	// in place.
	CurrentDefinition func(kind ArchivedDefinitionKind, id int) (name string, data []byte, ok bool)
}

// ArchivedReferences are the rebound cross-references of one
// archived definition: target ids in the appended range that
// replace the source-cache ids the payload was decoded with. They
// are the references record of ArchivedTypeSource
// (ArchivedTypeLoader.ts:570-574), in a typed form: scalar fields
// are non-nil pointers when rebound, list fields carry a presence
// flag, and SeqFields maps a TypeScript animation field name to its
// rebound sequence id.
type ArchivedReferences struct {
	Transforms          []int
	HasTransforms       bool
	CountObj            []int
	HasCountObj         bool
	Note                *int
	NoteTemplate        *int
	NotedID             *int
	UnnotedID           *int
	Placeholder         *int
	PlaceholderTemplate *int
	SeqFields           map[string]int
}

func (r *ArchivedReferences) empty() bool {
	return r == nil || (!r.HasTransforms && !r.HasCountObj &&
		r.Note == nil && r.NoteTemplate == nil && r.NotedID == nil &&
		r.UnnotedID == nil && r.Placeholder == nil && r.PlaceholderTemplate == nil &&
		len(r.SeqFields) == 0)
}

// ArchivedTypeSource is one registered archived definition: its
// source cache's provenance, its complete native bytes, and its
// rebound references. It mirrors ArchivedTypeSource
// (ArchivedTypeLoader.ts:570-574). Values must be treated as
// read-only once published by the registry.
type ArchivedTypeSource struct {
	CacheInfo  CacheInfo           `json:"cacheInfo"`
	DataBase64 string              `json:"dataBase64"`
	References *ArchivedReferences `json:"references,omitempty"`
}

// npcSeqFields are the NpcType animation fields that may name a
// bundled sequence (ArchivedTypeLoader.ts:554-556).
var npcSeqFields = []string{
	"idleSeqId", "turnLeftSeqId", "turnRightSeqId",
	"walkSeqId", "walkBackSeqId", "walkLeftSeqId", "walkRightSeqId",
	"runSeqId", "runBackSeqId", "runLeftSeqId", "runRightSeqId",
	"crawlSeqId", "crawlBackSeqId", "crawlLeftSeqId", "crawlRightSeqId",
}

// locSeqFields are the LocType animation fields that may name a
// bundled sequence (ArchivedTypeLoader.ts:557).
var locSeqFields = []string{"seqId"}

// archivedSeqFields returns the animation fields of an archived
// NPC or loc that may name a bundled sequence. It is
// archivedSeqFields (ArchivedTypeLoader.ts:565-567).
func archivedSeqFields(kind ArchivedDefinitionKind) []string {
	switch kind {
	case ArchivedKindNPC:
		return npcSeqFields
	case ArchivedKindLoc:
		return locSeqFields
	default:
		return nil
	}
}

var (
	archivedBundleIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	archivedSymbolPattern   = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	archivedBase64Pattern   = regexp.MustCompile(`^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$`)
)

// validArchivedID mirrors validId
// (ArchivedDefinitionRegistry.ts:379): an integer in [0, 0xffff).
func validArchivedID(id int) bool { return id >= 0 && id < 0xffff }

// validArchivedTimestamp approximates Number.isFinite(Date.parse())
// (ArchivedDefinitionRegistry.ts:429) over the timestamp shapes
// OpenRS2 info.json files use: RFC 3339 and bare dates, plus the
// common RFC 1123 forms. JavaScript Date.parse accepts a wider
// range of strings than this; a timestamp in an exotic-but-valid
// JS format would be rejected here, which can only make Replace
// stricter, never looser.
func validArchivedTimestamp(s string) bool {
	layouts := []string{
		time.RFC3339Nano, time.RFC3339,
		"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05",
		"2006-01-02",
		time.RFC1123, time.RFC1123Z, time.RFC822, time.RFC822Z,
	}
	for _, layout := range layouts {
		if _, err := time.Parse(layout, s); err == nil {
			return true
		}
	}
	return false
}

func archivedTargetKey(kind ArchivedDefinitionKind, id int) string {
	return string(kind) + ":" + fmt.Sprint(id)
}

func archivedSourceKey(bundle string, kind ArchivedDefinitionKind, id int) string {
	return bundle + ":" + string(kind) + ":" + fmt.Sprint(id)
}

func archivedNameKey(bundle string, kind ArchivedDefinitionKind, symbol string) string {
	return bundle + ":" + string(kind) + ":" + symbol
}

// ArchivedDefinitionRegistry validates complete bundles before
// replacing a cache-bound view. It never changes a native type or
// publishes part of a rejected update. It mirrors
// ArchivedDefinitionRegistry
// (ArchivedDefinitionRegistry.ts:385-548). Replace and the readers
// are safe for concurrent use; the TypeScript registry is
// single-threaded, so the mutex is a Go-necessary addition.
//
// The registry also holds the custom-definition overlay (see
// custom.go): the one object shared by every loader of a facade,
// so it is the natural home for the overlay the loaders consult.
// The overlay is a separate layer from archived bundles —
// TypeScript keeps customDefinitions apart from the registry too
// (CacheDefinitions.ts:88) — and Replace never touches it.
type ArchivedDefinitionRegistry struct {
	context            ArchivedDefinitionContext
	nativeCounts       map[ArchivedDefinitionKind]int
	sequencesSupported bool
	target             ArchivedTargetCache

	mu      sync.RWMutex
	sources map[string]ArchivedTypeSource
	names   map[string]int
	bundles []ArchivedDefinitionBundle
	counts  map[ArchivedDefinitionKind]int
	order   []string // source keys in publication order, for IDs()

	// custom is the custom-definition overlay registered through
	// CacheDefinitions (see custom.go), or nil. It is set only at
	// boot, before the engine starts; readers take mu.
	custom *customOverlay
}

// getCustomOverlay returns the registered custom-definition
// overlay, or nil when none is registered.
func (r *ArchivedDefinitionRegistry) getCustomOverlay() *customOverlay {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.custom
}

// setCustomOverlay publishes a custom-definition overlay,
// replacing any previous one. Callers build the overlay fully
// before publishing, so a failed registration leaves the previous
// overlay untouched (see custom.go).
func (r *ArchivedDefinitionRegistry) setCustomOverlay(o *customOverlay) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.custom = o
}

// NewArchivedDefinitionRegistry builds an empty registry over
// context, mirroring the constructor
// (ArchivedDefinitionRegistry.ts:394-404). Invalid native counts
// are an error, as the TypeScript throw is.
func NewArchivedDefinitionRegistry(context ArchivedDefinitionContext) (*ArchivedDefinitionRegistry, error) {
	sequencesSupported := context.Counts.SeqSupported
	nativeCounts := map[ArchivedDefinitionKind]int{
		ArchivedKindNPC: context.Counts.NPC,
		ArchivedKindLoc: context.Counts.Loc,
		ArchivedKindObj: context.Counts.Obj,
		ArchivedKindSeq: context.Counts.Seq,
	}
	if !sequencesSupported {
		nativeCounts[ArchivedKindSeq] = 0
	}
	for _, kind := range archivedKinds {
		if nativeCounts[kind] < 0 {
			return nil, errors.New("Invalid native definition count")
		}
	}
	counts := make(map[ArchivedDefinitionKind]int, len(nativeCounts))
	for k, v := range nativeCounts {
		counts[k] = v
	}
	return &ArchivedDefinitionRegistry{
		context:            context,
		nativeCounts:       nativeCounts,
		sequencesSupported: sequencesSupported,
		target: ArchivedTargetCache{
			Name:     context.CacheInfo.Name,
			Game:     context.CacheInfo.Game,
			Revision: context.CacheInfo.Revision,
		},
		sources: make(map[string]ArchivedTypeSource),
		names:   make(map[string]int),
		counts:  counts,
	}, nil
}

// Replace validates bundles completely and, only on success,
// replaces the registry's published view. It mirrors
// ArchivedDefinitionRegistry.replace
// (ArchivedDefinitionRegistry.ts:406-526), including owning the
// input (a deep copy is kept, so callers mutating their bundles
// afterwards cannot affect the published view) and atomicity: a
// rejected update leaves the previous view untouched.
func (r *ArchivedDefinitionRegistry) Replace(input []ArchivedDefinitionBundle) error {
	// Own the input: packet/store callers may reuse or mutate
	// their objects. (TypeScript round-trips through JSON.)
	bundles := copyArchivedBundles(input)
	sources := make(map[string]ArchivedTypeSource)
	names := make(map[string]int)
	sourceIDs := make(map[string]int)
	counts := make(map[ArchivedDefinitionKind]int, len(r.nativeCounts))
	for k, v := range r.nativeCounts {
		counts[k] = v
	}
	bundleIDs := make(map[string]bool)
	type decodedEntry struct {
		value  any // *NpcType, *LocType, *ObjType or *SeqType
		bundle string
		row    ArchivedDefinitionRow
	}
	decoded := make(map[string]*decodedEntry)
	var order []string

	for _, bundle := range bundles {
		if bundle.ID == "" || !archivedBundleIDPattern.MatchString(bundle.ID) || bundleIDs[bundle.ID] {
			return errors.New("Invalid or duplicate archive bundle id")
		}
		bundleIDs[bundle.ID] = true
		target := bundle.TargetCache
		if target.Name != r.target.Name || target.Game != r.target.Game ||
			target.Revision != r.target.Revision {
			return errors.New("Archived bundle targets a different cache")
		}
		info := bundle.SourceCache
		if info.Name == "" || info.Game != r.target.Game ||
			info.Revision <= 0 || info.Revision > r.target.Revision ||
			!validArchivedTimestamp(info.Timestamp) || info.Size < 0 {
			return errors.New("Invalid archived source cache provenance")
		}
		if len(bundle.Definitions) == 0 {
			return errors.New("Archive bundle has no definitions")
		}
		sourceInfo := info
		for _, row := range bundle.Definitions {
			if !validArchivedKind(row.Kind) || !validArchivedID(row.ID) || row.ID == 0 || !validArchivedID(row.SourceID) {
				return errors.New("Invalid archived definition kind or wire id range")
			}
			if row.Kind == ArchivedKindSeq && !r.sequencesSupported {
				return errors.New("Archived sequences need the native sequence count")
			}
			if !archivedSymbolPattern.MatchString(row.Symbol) {
				return errors.New("Invalid archived definition symbol")
			}
			key := archivedTargetKey(row.Kind, row.ID)
			original := archivedSourceKey(bundle.ID, row.Kind, row.SourceID)
			name := archivedNameKey(bundle.ID, row.Kind, row.Symbol)
			if _, ok := sources[key]; ok {
				return errors.New("Duplicate archived definition target, source or symbol")
			}
			if _, ok := sourceIDs[original]; ok {
				return errors.New("Duplicate archived definition target, source or symbol")
			}
			if _, ok := names[name]; ok {
				return errors.New("Duplicate archived definition target, source or symbol")
			}
			if len(row.DataBase64) > 87384 || !archivedBase64Pattern.MatchString(row.DataBase64) {
				return errors.New("Invalid archived definition payload")
			}
			data, err := base64.StdEncoding.DecodeString(row.DataBase64)
			if err != nil {
				return errors.New("Invalid archived definition payload")
			}
			if CRC32(data) != row.CRC {
				return errors.New("Archived source CRC checksum mismatch")
			}
			if row.ID < r.nativeCounts[row.Kind] {
				if row.Kind != ArchivedKindObj {
					return errors.New("Archived definition collides with an existing native type")
				}
				currentName, currentData, currentOK := r.context.CurrentDefinition(row.Kind, row.ID)
				if strings.ToLower(currentName) != "null" || !currentOK ||
					row.ExpectedCurrentCRC == nil || CRC32(currentData) != *row.ExpectedCurrentCRC {
					return errors.New("Archived replacement does not match the retired current definition checksum")
				}
			} else if row.ExpectedCurrentCRC != nil {
				return errors.New("Appended archived type cannot replace a current definition")
			}
			source := ArchivedTypeSource{CacheInfo: sourceInfo, DataBase64: row.DataBase64}
			value, err := decodeArchivedValue(row.Kind, row.ID, source)
			if err != nil {
				return err
			}
			sources[key] = source
			names[name] = row.ID
			sourceIDs[original] = row.ID
			if row.ID+1 > counts[row.Kind] {
				counts[row.Kind] = row.ID + 1
			}
			decoded[key] = &decodedEntry{value: value, bundle: bundle.ID, row: row}
			order = append(order, key)
		}
	}

	// All definition references are bound inside their source
	// bundle. This includes item/placeholder/template cycles, not
	// only NPC/loc morphs.
	reference := func(bundle string, kind ArchivedDefinitionKind, id int) (int, error) {
		if id == -1 {
			return -1, nil
		}
		target, ok := sourceIDs[archivedSourceKey(bundle, kind, id)]
		if !ok {
			return 0, fmt.Errorf("Unresolved archived definition reference %d", id)
		}
		return target, nil
	}
	for _, key := range order {
		entry := decoded[key]
		refs := &ArchivedReferences{}
		switch entry.row.Kind {
		case ArchivedKindNPC, ArchivedKindLoc:
			var transforms []int
			switch v := entry.value.(type) {
			case *NpcType:
				transforms = v.Transforms
			case *LocType:
				transforms = v.Transforms
			}
			if transforms != nil {
				mapped := make([]int, len(transforms))
				for i, id := range transforms {
					target, err := reference(entry.bundle, entry.row.Kind, id)
					if err != nil {
						return err
					}
					mapped[i] = target
				}
				refs.Transforms = mapped
				refs.HasTransforms = true
			}
			// Animations are optional bundle members: an id the
			// bundle does not archive is a current native
			// sequence, while a recycled one must be carried by
			// the same bundle and is rebound to its appended id.
			for _, field := range archivedSeqFields(entry.row.Kind) {
				id, ok := archivedSeqFieldValue(entry.value, field)
				if !ok || id < 0 {
					continue
				}
				target, ok := sourceIDs[archivedSourceKey(entry.bundle, ArchivedKindSeq, id)]
				if ok && target != id {
					if refs.SeqFields == nil {
						refs.SeqFields = make(map[string]int)
					}
					refs.SeqFields[field] = target
				}
			}
		case ArchivedKindObj:
			item := entry.value.(*ObjType)
			bindScalar := func(dst **int, value int) error {
				if value < 0 {
					return nil
				}
				target, err := reference(entry.bundle, ArchivedKindObj, value)
				if err != nil {
					return err
				}
				*dst = &target
				return nil
			}
			for _, bind := range []struct {
				dst   **int
				value int
			}{
				{&refs.Note, item.Note},
				{&refs.NoteTemplate, item.NoteTemplate},
				{&refs.NotedID, item.NotedID},
				{&refs.UnnotedID, item.UnnotedID},
				{&refs.Placeholder, item.Placeholder},
				{&refs.PlaceholderTemplate, item.PlaceholderTemplate},
			} {
				if err := bindScalar(bind.dst, bind.value); err != nil {
					return err
				}
			}
			if item.CountObj != nil {
				mapped := make([]int, len(item.CountObj))
				for i, id := range item.CountObj {
					if i < len(item.CountCo) && item.CountCo[i] > 0 {
						target, err := reference(entry.bundle, ArchivedKindObj, id)
						if err != nil {
							return err
						}
						mapped[i] = target
					} else {
						mapped[i] = id
					}
				}
				refs.CountObj = mapped
				refs.HasCountObj = true
			}
		}
		if !refs.empty() {
			source := sources[key]
			source.References = refs
			sources[key] = source
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.bundles = bundles
	r.sources = sources
	r.names = names
	r.counts = counts
	r.order = order
	return nil
}

// GetSource returns the registered source for kind/id, or false
// when none is registered.
func (r *ArchivedDefinitionRegistry) GetSource(kind ArchivedDefinitionKind, id int) (ArchivedTypeSource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	source, ok := r.sources[archivedTargetKey(kind, id)]
	return source, ok
}

// GetCount returns the native count extended by appended archived
// definitions of the kind.
func (r *ArchivedDefinitionRegistry) GetCount(kind ArchivedDefinitionKind) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.counts[kind]
}

// Resolve returns the appended id for a bundle's symbol, or false
// when the bundle has no such definition.
func (r *ArchivedDefinitionRegistry) Resolve(bundle string, kind ArchivedDefinitionKind, symbol string) (int, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.names[archivedNameKey(bundle, kind, symbol)]
	return id, ok
}

// IDs returns the registered ids of a kind, in publication order.
func (r *ArchivedDefinitionRegistry) IDs(kind ArchivedDefinitionKind) []int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	prefix := string(kind) + ":"
	var ids []int
	for _, key := range r.order {
		if strings.HasPrefix(key, prefix) {
			var id int
			fmt.Sscanf(key[len(prefix):], "%d", &id)
			ids = append(ids, id)
		}
	}
	return ids
}

// GetBundles returns a deep copy of the published bundles.
func (r *ArchivedDefinitionRegistry) GetBundles() []ArchivedDefinitionBundle {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return copyArchivedBundles(r.bundles)
}

func copyArchivedBundles(in []ArchivedDefinitionBundle) []ArchivedDefinitionBundle {
	if in == nil {
		return nil
	}
	out := make([]ArchivedDefinitionBundle, len(in))
	for i, bundle := range in {
		out[i] = bundle
		defs := make([]ArchivedDefinitionRow, len(bundle.Definitions))
		for j, row := range bundle.Definitions {
			defs[j] = row
			if row.ExpectedCurrentCRC != nil {
				crc := *row.ExpectedCurrentCRC
				defs[j].ExpectedCurrentCRC = &crc
			}
		}
		out[i].Definitions = defs
	}
	return out
}

// decodeArchivedValue decodes one archived definition of the given
// kind without references (the registry's validation pass).
func decodeArchivedValue(kind ArchivedDefinitionKind, id int, source ArchivedTypeSource) (any, error) {
	switch kind {
	case ArchivedKindNPC:
		return DecodeArchivedType(NewNpcType, id, source)
	case ArchivedKindLoc:
		return DecodeArchivedType(NewLocType, id, source)
	case ArchivedKindObj:
		return DecodeArchivedType(NewObjType, id, source)
	case ArchivedKindSeq:
		return DecodeArchivedType(NewSeqType, id, source)
	default:
		return nil, fmt.Errorf("Invalid archived definition kind: %s", kind)
	}
}

// archivedSeqFieldValue reads one animation field of an archived
// NPC or loc by its TypeScript field name.
func archivedSeqFieldValue(value any, field string) (int, bool) {
	switch v := value.(type) {
	case *NpcType:
		return v.archivedSeqField(field)
	case *LocType:
		if field == "seqId" {
			return v.SeqId, true
		}
	}
	return 0, false
}

// archivableType is the constraint on the strict archived decoder:
// a definition type whose opcodes can be driven one at a time.
type archivableType interface {
	DefinitionType
	DecodeOpcode(opcode int, buf *ByteBuffer) error
	CacheType() CacheType
}

// DecodeArchivedType is the strict decoder shared by registry
// validation and the overlay loader. It mirrors decodeArchivedType
// (ArchivedTypeLoader.ts:577-605): the payload must terminate with
// opcode 0 and carry no trailing bytes (no dat2 EOF tolerance),
// references are applied only when they match a native field of
// the same shape, and Post runs last.
func DecodeArchivedType[T archivableType](newFunc func(id int, info CacheInfo) T, id int, source ArchivedTypeSource) (T, error) {
	var zero T
	bytes, err := base64.StdEncoding.DecodeString(source.DataBase64)
	if err != nil {
		return zero, fmt.Errorf("Invalid archived definition payload: %v", err)
	}
	buf := NewByteBuffer(bytes)
	value := newFunc(id, source.CacheInfo)
	terminated := false
	for buf.Remaining() > 0 {
		opcode, err := buf.ReadUnsignedByte()
		if err != nil {
			return zero, err
		}
		if opcode == 0 {
			terminated = true
			break
		}
		if err := value.DecodeOpcode(opcode, buf); err != nil {
			return zero, err
		}
	}
	if !terminated {
		return zero, fmt.Errorf("Archived definition %d is missing its terminator", id)
	}
	if buf.Remaining() != 0 {
		return zero, fmt.Errorf("Archived definition %d has trailing data", id)
	}
	if err := applyArchivedReferences(value, source.References); err != nil {
		return zero, err
	}
	value.Post()
	return value, nil
}

// applyArchivedReferences applies rebound references to a decoded
// archived definition, mirroring the reference loop of
// decodeArchivedType (ArchivedTypeLoader.ts:592-602): a list
// reference requires a native list of the same length, a scalar
// reference requires the native field, and anything else is an
// error.
func applyArchivedReferences(value any, refs *ArchivedReferences) error {
	if refs.empty() {
		return nil
	}
	switch v := value.(type) {
	case *NpcType:
		if refs.HasTransforms {
			if v.Transforms == nil || len(refs.Transforms) != len(v.Transforms) {
				return errors.New("Archived reference mapping has no matching native field")
			}
			v.Transforms = append([]int(nil), refs.Transforms...)
		}
		if refs.HasCountObj || refs.Note != nil || refs.NoteTemplate != nil ||
			refs.NotedID != nil || refs.UnnotedID != nil ||
			refs.Placeholder != nil || refs.PlaceholderTemplate != nil {
			return errors.New("Archived reference mapping has no matching native field")
		}
		for field, target := range refs.SeqFields {
			if !v.setArchivedSeqField(field, target) {
				return errors.New("Unsupported archived reference field")
			}
		}
	case *LocType:
		if refs.HasTransforms {
			if v.Transforms == nil || len(refs.Transforms) != len(v.Transforms) {
				return errors.New("Archived reference mapping has no matching native field")
			}
			v.Transforms = append([]int(nil), refs.Transforms...)
		}
		if refs.HasCountObj || refs.Note != nil || refs.NoteTemplate != nil ||
			refs.NotedID != nil || refs.UnnotedID != nil ||
			refs.Placeholder != nil || refs.PlaceholderTemplate != nil {
			return errors.New("Archived reference mapping has no matching native field")
		}
		for field, target := range refs.SeqFields {
			if field != "seqId" {
				return errors.New("Unsupported archived reference field")
			}
			v.SeqId = target
		}
	case *ObjType:
		if refs.HasTransforms || len(refs.SeqFields) > 0 {
			return errors.New("Archived reference mapping has no matching native field")
		}
		if refs.HasCountObj {
			if v.CountObj == nil || len(refs.CountObj) != len(v.CountObj) {
				return errors.New("Archived reference mapping has no matching native field")
			}
			v.CountObj = append([]int(nil), refs.CountObj...)
		}
		if refs.Note != nil {
			v.Note = *refs.Note
		}
		if refs.NoteTemplate != nil {
			v.NoteTemplate = *refs.NoteTemplate
		}
		if refs.NotedID != nil {
			v.NotedID = *refs.NotedID
		}
		if refs.UnnotedID != nil {
			v.UnnotedID = *refs.UnnotedID
		}
		if refs.Placeholder != nil {
			v.Placeholder = *refs.Placeholder
		}
		if refs.PlaceholderTemplate != nil {
			v.PlaceholderTemplate = *refs.PlaceholderTemplate
		}
	default:
		return errors.New("Archived reference mapping has no matching native field")
	}
	return nil
}

// ArchivedTypeLoader decodes archived definitions using their own
// cache revision without mutating the ordinary loader. ID
// allocation, provenance and reference mappings belong to the
// registry; this adapter only decodes the exact registered
// payload. It mirrors ArchivedTypeLoader
// (ArchivedTypeLoader.ts:612-655).
type ArchivedTypeLoader[T archivableType] struct {
	base        TypeLoader[T]
	newFunc     func(id int, info CacheInfo) T
	sourceForID func(id int) (ArchivedTypeSource, bool)
	sourceCount func() int

	// overlay is the custom-definition overlay (see custom.go),
	// consulted before the archived and native loaders, and
	// overlayCount extends Count past the overlay's highest id.
	// Both are nil when no overlay is registered, in which case
	// Load and Count behave exactly as before. They are installed
	// by SetOverlay at boot, before the engine starts; the overlay
	// itself is safe for concurrent use.
	overlay      func(id int) (T, bool)
	overlayCount func() int

	mu       sync.Mutex
	archived map[int]archivedEntry[T]
}

type archivedEntry[T any] struct {
	data  string
	info  string
	value T
}

// NewArchivedTypeLoader builds an overlay loader over base.
// sourceForID returns the registered source for an id, and
// sourceCount returns the registry's extended count for the kind.
func NewArchivedTypeLoader[T archivableType](base TypeLoader[T], newFunc func(id int, info CacheInfo) T, sourceForID func(id int) (ArchivedTypeSource, bool), sourceCount func() int) *ArchivedTypeLoader[T] {
	return &ArchivedTypeLoader[T]{
		base: base, newFunc: newFunc, sourceForID: sourceForID, sourceCount: sourceCount,
		archived: make(map[int]archivedEntry[T]),
	}
}

// SetOverlay installs a definition overlay consulted before the
// archived and native loaders: the Go form of the
// applyCustomDefinition-first getter order in TypeScript
// (CacheDefinitions.ts getNpc/getItem/getObject), where a custom
// definition wins over the archived/native definition of the same
// id. lookup returns the overlay definition for id, or false to
// fall through to the archived/native loaders; count returns one
// past the overlay's highest id (0 when the overlay is empty) and
// extends Count. Passing nils removes the overlay. Call it only
// before concurrent loads begin (at boot); the installed functions
// must be safe for concurrent use.
func (l *ArchivedTypeLoader[T]) SetOverlay(lookup func(id int) (T, bool), count func() int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.overlay = lookup
	l.overlayCount = count
}

// Load returns the overlay definition for id when one is
// registered, else the archived definition when one is
// registered, else the base loader's definition. A registered
// source that fails to decode panics: the TypeScript loader
// throws, and registry validation makes this unreachable for
// published sources.
func (l *ArchivedTypeLoader[T]) Load(id int) T {
	l.mu.Lock()
	overlay := l.overlay
	l.mu.Unlock()
	if overlay != nil {
		if value, ok := overlay(id); ok {
			return value
		}
	}
	source, ok := l.sourceForID(id)
	if !ok {
		l.mu.Lock()
		delete(l.archived, id)
		l.mu.Unlock()
		return l.base.Load(id)
	}
	infoKey := archivedSourceInfoKey(source)
	l.mu.Lock()
	defer l.mu.Unlock()
	if cached, ok := l.archived[id]; ok && cached.data == source.DataBase64 && cached.info == infoKey {
		return cached.value
	}
	value, err := DecodeArchivedType(l.newFunc, id, source)
	if err != nil {
		panic(err)
	}
	l.archived[id] = archivedEntry[T]{data: source.DataBase64, info: infoKey, value: value}
	return value
}

// archivedSourceInfoKey is the JSON.stringify([cacheInfo,
// references]) cache key (ArchivedTypeLoader.ts:632).
func archivedSourceInfoKey(source ArchivedTypeSource) string {
	key, err := json.Marshal([]any{source.CacheInfo, source.References})
	if err != nil {
		return fmt.Sprintf("%#v", source)
	}
	return string(key)
}

// Count returns the greatest of the base count, the registry
// count, and the overlay count. A negative registry count is clamped
// to 0; TypeScript throws, which cannot happen for a registry-built
// count.
func (l *ArchivedTypeLoader[T]) Count() int {
	count := l.sourceCount()
	if count < 0 {
		count = 0
	}
	l.mu.Lock()
	overlayCount := l.overlayCount
	l.mu.Unlock()
	if overlayCount != nil {
		if oc := overlayCount(); oc > count {
			count = oc
		}
	}
	if base := l.base.Count(); base > count {
		return base
	}
	return count
}

// ClearCache drops archived and base caches.
func (l *ArchivedTypeLoader[T]) ClearCache() {
	l.ClearArchivedCache()
	l.base.ClearCache()
}

// ClearArchivedCache drops only the archived cache, so a replaced
// registry is picked up without dropping native definitions.
func (l *ArchivedTypeLoader[T]) ClearArchivedCache() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.archived = make(map[int]archivedEntry[T])
}

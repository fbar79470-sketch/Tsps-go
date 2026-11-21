package game

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// MapRegionReplacementManager ports
// game/collision/MapRegionReplacementManager.ts (445 lines):
// runtime region replacements — map data streamed to clients
// in place of the cache's map for a region — plus the .pack
// file loading the map editor exports and the region-load
// scan consumes.
//
// LIVENESS (grounded before porting, per the Phase 21 brief).
// The replacement packet path is LIVE, not a write() no-op:
// sendRegionPacketToPlayer and sendPayloadToPlayer call
// session.sendClientPacket(encodeCachedRegionReplacement(...))
// (MapRegionReplacementManager.ts:143-147, 229-238), and
// sendClientPacket is the live queued flush path
// (PlayerSession.ts:287). flushClient calls
// sendVisibleReplacementsToPlayer on every scene-window change
// (PlayerSession.ts:497-505). This port therefore sends for
// real through RegionReplacementSink, following the Phases
// 19/20 precedent in reverse: where those phases mirrored
// no-ops, this one ports the live path.
//
// FRAMING. TS builds the wire frame with encodeServerPacket
// inside encodeCachedRegionReplacement
// (ClientProtocol.ts:2063-2071). Go cannot frame packets from
// this package (game must never import protocol), so the
// manager builds prepared RegionReplacementPackets (opcode +
// payload, byte 2 the allowReload flag, exactly the TS
// RegionReplacementPacket type) and the sink frames them with
// protocol.EncodePacket. The payload bytes — including the
// deflate fallback for dense regions — are built here, byte
// for byte per regionReplacementPacket
// (ClientProtocol.ts:2020-2061).
//
// STATE. TS keeps replacements in a static Map. Go keeps them
// in a manager value, constructed with
// NewMapRegionReplacementManager and owned by the coordinator
// (one per world, like RegionManager). The manager is safe for
// concurrent use: replacements install from content/plugin
// calls while the flush path reads them, and Go maps panic on
// concurrent access where the TS single-threaded runtime does
// not.
//
// OUT OF SCOPE. The HTTP/ContentApi side (NetworkBuilder's
// /regions endpoints, NetworkBuilder.ts:379-399) stays out:
// Go transport is WebSocket-only. The data/regions directory
// scan that RegionManager.init performs is ported as
// LoadRegionReplacementDir, called by boot wiring. Region
// loading consults replacements through ReplacementRegionSource
// (below), which wraps the cache source — TS loadMapFiles
// reads getReplacementMapData inline (RegionManager.ts:900),
// and the wrapper is the Go form of that read without touching
// the region decoder.
//
// PERSISTENCE. Replacements are runtime-only: nothing persists
// (they re-derive from data/regions files at boot, or are
// re-installed by plugins). No save fields exist or are added.

// MaxRegionReplacementPayload is MAX_REGION_REPLACEMENT_PAYLOAD
// (ClientProtocol.ts:2007): a REGION_REPLACEMENT travels in a
// short packet inside a 64 KiB frame, so its payload stops at
// 0xfffd. Anything larger is deflated instead of refused.
const MaxRegionReplacementPayload = 0xfffd

// Region replacement opcodes: ServerPacketId.REGION_REPLACEMENT
// and REGION_REPLACEMENT_COMPRESSED (ClientProtocol.ts), the
// values Go's protocol table carries as
// ServerRegionReplacement (144) and
// ServerRegionReplacementCompressed (147). This package must
// not import protocol, so the values are restated here with
// the citation; the sink maps them to the protocol constants
// when framing.
const (
	RegionReplacementOpcode           = 144
	RegionReplacementCompressedOpcode = 147
)

// compressedReplacementHeader is
// COMPRESSED_REGION_REPLACEMENT_HEADER (ClientProtocol.ts:2010):
// regionId(2) + flags(1) + rawTerrainLength(4) +
// rawObjectLength(4).
const compressedReplacementHeader = 11

// maxReplacementRawBytes bounds the raw terrain+objects a
// replacement may carry (ClientProtocol.ts:2029: 16 MiB, the
// decoder's bounded output buffer).
const maxReplacementRawBytes = 16 * 1024 * 1024

// RegionReplacementPacket is a prepared replacement payload
// (ClientProtocol.ts:2013, RegionReplacementPacket): the opcode
// plus the payload whose byte 2 is the allowReload flag, so it
// can be flipped without re-encoding. Payload is owned by the
// packet; callers must not mutate it — Frame copies when the
// flag must change.
type RegionReplacementPacket struct {
	Opcode  int
	Payload []byte
}

// Frame returns the opcode and payload to send with the given
// allowReload flag (ClientProtocol.ts:2063-2071,
// encodeCachedRegionReplacement): when the stored flag already
// matches, the stored payload is returned as-is; otherwise a
// copy with the flag flipped is returned. The caller frames
// the pair with protocol.EncodePacket.
func (p RegionReplacementPacket) Frame(allowReload bool) (opcode int, payload []byte) {
	flag := byte(0)
	if allowReload {
		flag = 1
	}
	if len(p.Payload) > 2 && p.Payload[2] == flag {
		return p.Opcode, p.Payload
	}
	copied := append([]byte(nil), p.Payload...)
	if len(copied) > 2 {
		copied[2] = flag
	}
	return p.Opcode, copied
}

// BuildRegionReplacementPacket builds a prepared replacement
// payload once (ClientProtocol.ts:2020-2061,
// regionReplacementPacket). A dense region costs a deflate, so
// callers that send the same replacement to many players keep
// the result and re-frame it instead of re-encoding. objectData
// nil means terrain-only (the client keeps the cache's objects
// for the region). It returns an error when the terrain is
// empty, the raw sections exceed 16 MiB, or even the deflated
// form exceeds MaxRegionReplacementPayload — the TS throws,
// with the same messages.
func BuildRegionReplacementPacket(regionID int, allowReload bool, terrainData, objectData []byte) (RegionReplacementPacket, error) {
	objects := objectData
	if objects == nil {
		objects = []byte{}
	}
	if len(terrainData) == 0 || len(terrainData)+len(objects) > maxReplacementRawBytes {
		return RegionReplacementPacket{}, fmt.Errorf("Region replacement %d has invalid section lengths", regionID)
	}
	payloadLength := 7 + len(terrainData) + len(objects)
	flag := byte(0)
	if allowReload {
		flag = 1
	}
	if payloadLength <= MaxRegionReplacementPayload {
		payload := make([]byte, payloadLength)
		binary.BigEndian.PutUint16(payload[0:2], uint16(regionID&0xffff))
		payload[2] = flag
		binary.BigEndian.PutUint16(payload[3:5], uint16(len(terrainData)))
		binary.BigEndian.PutUint16(payload[5:7], uint16(len(objects)))
		copy(payload[7:], terrainData)
		copy(payload[7+len(terrainData):], objects)
		return RegionReplacementPacket{Opcode: RegionReplacementOpcode, Payload: payload}, nil
	}
	var deflated bytes.Buffer
	writer, err := zlib.NewWriterLevel(&deflated, zlib.BestCompression)
	if err != nil {
		return RegionReplacementPacket{}, err
	}
	if _, err := writer.Write(terrainData); err != nil {
		return RegionReplacementPacket{}, err
	}
	if _, err := writer.Write(objects); err != nil {
		return RegionReplacementPacket{}, err
	}
	if err := writer.Close(); err != nil {
		return RegionReplacementPacket{}, err
	}
	compressedLength := compressedReplacementHeader + deflated.Len()
	if compressedLength > MaxRegionReplacementPayload {
		return RegionReplacementPacket{}, fmt.Errorf("Region replacement %d is too large to stream: %d bytes, %d compressed, max %d", regionID, payloadLength, compressedLength, MaxRegionReplacementPayload)
	}
	payload := make([]byte, compressedLength)
	binary.BigEndian.PutUint16(payload[0:2], uint16(regionID&0xffff))
	payload[2] = flag
	binary.BigEndian.PutUint32(payload[3:7], uint32(len(terrainData)))
	binary.BigEndian.PutUint32(payload[7:11], uint32(len(objects)))
	copy(payload[compressedReplacementHeader:], deflated.Bytes())
	return RegionReplacementPacket{Opcode: RegionReplacementCompressedOpcode, Payload: payload}, nil
}

// EncodeRegionReplacement builds and returns a one-shot
// replacement packet (ClientProtocol.ts:2073-2082,
// encodeRegionReplacement): BuildRegionReplacementPacket plus
// the flag the frame carries. Prefer BuildRegionReplacementPacket
// plus Frame for replacements sent more than once.
func EncodeRegionReplacement(regionID int, allowReload bool, terrainData, objectData []byte) (RegionReplacementPacket, error) {
	return BuildRegionReplacementPacket(regionID, allowReload, terrainData, objectData)
}

// ReplaceMapRegionResult reports a successful replacement
// install (MapRegionReplacementManager.ts:20-26).
type ReplaceMapRegionResult struct {
	RegionID     int
	Source       string
	TerrainBytes int
	ObjectBytes  int
	ObjectCount  int
}

// regionReplacement is one installed replacement: the stored
// map bytes plus the packet encoded once at install time.
type regionReplacement struct {
	regionID    int
	source      string
	terrainData []byte
	objectData  []byte // nil = terrain-only
	packet      RegionReplacementPacket
}

// MapRegionReplacementManager holds the installed region
// replacements. See the file comment.
type MapRegionReplacementManager struct {
	mu           sync.RWMutex
	replacements map[int]*regionReplacement

	// nativeSource answers cache map data for NativeRegionPacket
	// (CacheMaps.getRegion). Nil means no native maps exist.
	nativeSource RegionDataSource
	// archiveIDs answers the cache archive ids for a region
	// (CacheMaps.getArchiveIds), for .pack validation and
	// GetRegionPack. Nil means archive ids are unavailable and
	// .pack loads are refused.
	archiveIDs func(regionID int) (terrainFile, objectFile int, ok bool)
}

// NewMapRegionReplacementManager returns an empty manager: no
// replacements, no native source, no archive-id lookup. The
// coordinator installs the seams with SetNativeSource and
// SetArchiveIDs.
func NewMapRegionReplacementManager() *MapRegionReplacementManager {
	return &MapRegionReplacementManager{replacements: make(map[int]*regionReplacement)}
}

// SetNativeSource installs the cache source NativeRegionPacket
// reads native maps through. A nil source means no native map
// ever exists (NativeRegionPacket always answers absent).
func (m *MapRegionReplacementManager) SetNativeSource(src RegionDataSource) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nativeSource = src
}

// SetArchiveIDs installs the cache archive-id lookup .pack
// loading validates against and GetRegionPack writes into the
// header. A nil lookup means .pack loads are refused and
// GetRegionPack always answers absent.
func (m *MapRegionReplacementManager) SetArchiveIDs(fn func(regionID int) (terrainFile, objectFile int, ok bool)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.archiveIDs = fn
}

func validRegionID(regionID int) bool { return regionID >= 0 && regionID <= 0xffff }

// ReplaceMapRegion installs a replacement from a single-file
// source: a .pack path (MapRegionReplacementManager.ts:33-44,
// loadSource). The path may be absolute or relative to the
// working directory. A non-.pack single file is refused, as
// TS refuses it — use ReplaceMapRegionFromFiles for a
// terrain/object pair. It returns an error for an invalid
// region id, an unreadable file, or a region that cannot be
// streamed even deflated.
func (m *MapRegionReplacementManager) ReplaceMapRegion(regionID int, source string) (ReplaceMapRegionResult, error) {
	if !validRegionID(regionID) {
		return ReplaceMapRegionResult{}, fmt.Errorf("invalid regionId: %d", regionID)
	}
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return ReplaceMapRegionResult{}, fmt.Errorf("source must be a non-empty string or [terrainPath, objectPath]")
	}
	sourcePath := resolveReplacementPath(trimmed)
	if !strings.HasSuffix(strings.ToLower(sourcePath), ".pack") {
		return ReplaceMapRegionResult{}, fmt.Errorf("single-file source '%s' is unsupported; use .pack or [terrainPath, objectPath]", sourcePath)
	}
	terrainData, objectData, err := m.readPackSource(regionID, sourcePath)
	if err != nil {
		return ReplaceMapRegionResult{}, err
	}
	return m.register(regionID, terrainData, objectData, sourcePath)
}

// ReplaceMapRegionFromFiles installs a replacement from a
// [terrainPath, objectPath] pair
// (MapRegionReplacementManager.ts:285-296, loadSource). Both
// paths may be absolute or relative to the working directory,
// and both files may be gzip-compressed (decoded on read). The
// recorded source is "[terrainPath, objectPath]" with the
// resolved paths, as in TS.
func (m *MapRegionReplacementManager) ReplaceMapRegionFromFiles(regionID int, terrainPath, objectPath string) (ReplaceMapRegionResult, error) {
	if !validRegionID(regionID) {
		return ReplaceMapRegionResult{}, fmt.Errorf("invalid regionId: %d", regionID)
	}
	terrainResolved := resolveReplacementPath(strings.TrimSpace(terrainPath))
	objectResolved := resolveReplacementPath(strings.TrimSpace(objectPath))
	terrainData, err := readReplacementMapFile(terrainResolved)
	if err != nil {
		return ReplaceMapRegionResult{}, err
	}
	objectData, err := readReplacementMapFile(objectResolved)
	if err != nil {
		return ReplaceMapRegionResult{}, err
	}
	return m.register(regionID, terrainData, objectData, "["+terrainResolved+", "+objectResolved+"]")
}

// ReplaceMapRegionData installs a replacement from in-memory
// map data (MapRegionReplacementManager.ts:46-58). objectData
// nil means terrain-only. source names the installer (a plugin
// name, "runtime"); the ownership checks in the hub read it
// back through GetSource.
func (m *MapRegionReplacementManager) ReplaceMapRegionData(regionID int, terrainData, objectData []byte, source string) (ReplaceMapRegionResult, error) {
	if !validRegionID(regionID) {
		return ReplaceMapRegionResult{}, fmt.Errorf("invalid regionId: %d", regionID)
	}
	return m.register(regionID, terrainData, objectData, source)
}

// ReplaceMapRegionTerrain installs a terrain-only replacement:
// clients keep the cache's objects for the region
// (MapRegionReplacementManager.ts:80-90).
func (m *MapRegionReplacementManager) ReplaceMapRegionTerrain(regionID int, terrainData []byte, source string) (ReplaceMapRegionResult, error) {
	if !validRegionID(regionID) {
		return ReplaceMapRegionResult{}, fmt.Errorf("invalid regionId: %d", regionID)
	}
	return m.register(regionID, terrainData, nil, source)
}

// register encodes the packet once — so a region that could
// never be streamed is rejected at load time instead of
// failing while a player's scene goes out — counts the object
// placements, and stores copies of the data
// (MapRegionReplacementManager.ts:60-78).
func (m *MapRegionReplacementManager) register(regionID int, terrainData, objectData []byte, source string) (ReplaceMapRegionResult, error) {
	packet, err := BuildRegionReplacementPacket(regionID, true, terrainData, objectData)
	if err != nil {
		return ReplaceMapRegionResult{}, err
	}
	objectCount := countObjectPlacements(objectData)
	storedTerrain := append([]byte(nil), terrainData...)
	var storedObjects []byte
	if objectData != nil {
		storedObjects = append([]byte(nil), objectData...)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.replacements[regionID] = &regionReplacement{
		regionID:    regionID,
		source:      source,
		terrainData: storedTerrain,
		objectData:  storedObjects,
		packet:      packet,
	}
	return ReplaceMapRegionResult{
		RegionID:     regionID,
		Source:       source,
		TerrainBytes: len(terrainData),
		ObjectBytes:  len(objectData),
		ObjectCount:  objectCount,
	}, nil
}

// GetReplacementMapData returns the stored map data for a
// region (MapRegionReplacementManager.ts:92-105), or ok=false
// when no replacement is installed. The returned slices are
// copies; mutating them does not affect the stored data. (TS
// returns the stored arrays; copying is a safe deviation —
// region loading only decodes them.)
func (m *MapRegionReplacementManager) GetReplacementMapData(regionID int) (terrainData, objectData []byte, ok bool) {
	if m == nil {
		return nil, nil, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	replacement, exists := m.replacements[regionID]
	if !exists {
		return nil, nil, false
	}
	terrainData = append([]byte(nil), replacement.terrainData...)
	if replacement.objectData != nil {
		objectData = append([]byte(nil), replacement.objectData...)
	}
	return terrainData, objectData, true
}

// GetSource reports who installed a region's replacement (a
// .pack path, "agent-mcp", a plugin name), or ok=false when no
// replacement is installed
// (MapRegionReplacementManager.ts:108-110).
func (m *MapRegionReplacementManager) GetSource(regionID int) (string, bool) {
	if m == nil {
		return "", false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	replacement, ok := m.replacements[regionID]
	if !ok {
		return "", false
	}
	return replacement.source, true
}

// RemoveMapRegion forgets a runtime replacement; the caller
// reloads clipping and re-streams the native map
// (MapRegionReplacementManager.ts:113-115). It reports whether
// a replacement was installed.
func (m *MapRegionReplacementManager) RemoveMapRegion(regionID int) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.replacements[regionID]; !ok {
		return false
	}
	delete(m.replacements, regionID)
	return true
}

// NativeRegionPacket builds the native-map packet for a
// region, for restoring it: clients keep a streamed
// replacement for the session, so restoring sends them the
// native cache map the same way — there is no separate
// "forget" packet (MapRegionReplacementManager.ts:117-127).
// It returns (nil, nil) when the cache has no native map for
// the region, and an error when the native map cannot be
// encoded.
func (m *MapRegionReplacementManager) NativeRegionPacket(regionID int) (*RegionReplacementPacket, error) {
	if m == nil {
		return nil, nil
	}
	m.mu.RLock()
	src := m.nativeSource
	m.mu.RUnlock()
	if src == nil {
		return nil, nil
	}
	terrain, objects, ok := src.RegionData(regionID)
	if !ok || terrain == nil {
		return nil, nil
	}
	packet, err := BuildRegionReplacementPacket(regionID, true, terrain, objects)
	if err != nil {
		return nil, err
	}
	return &packet, nil
}

// GetRegionIDs returns the installed replacement region ids,
// sorted ascending (MapRegionReplacementManager.ts:149-151).
func (m *MapRegionReplacementManager) GetRegionIDs() []int {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]int, 0, len(m.replacements))
	for id := range m.replacements {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

// GetRegionPack builds the .pack the map editor exports and
// the server loads from data/regions: header (28 bytes) +
// object archive + terrain archive
// (MapRegionReplacementManager.ts:153-183). objectDataOverride
// lets a caller build a pack for a region whose live
// replacement is terrain-only — the pack still has to carry
// the region's objects, because a .pack is a complete region,
// not a patch. A nil override uses the replacement's objects.
// It returns ok=false when no replacement is installed, the
// cache has no archive ids for the region, or no objects are
// available.
func (m *MapRegionReplacementManager) GetRegionPack(regionID int, objectDataOverride []byte) ([]byte, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.RLock()
	replacement, exists := m.replacements[regionID]
	archiveIDs := m.archiveIDs
	m.mu.RUnlock()
	if !exists || archiveIDs == nil {
		return nil, false
	}
	terrainFile, objectFile, ok := archiveIDs(regionID)
	if !ok {
		return nil, false
	}
	objects := objectDataOverride
	if objects == nil {
		objects = replacement.objectData
	}
	if objects == nil {
		return nil, false
	}
	terrain := replacement.terrainData
	pack := make([]byte, 28+len(objects)+len(terrain))
	binary.BigEndian.PutUint32(pack[0:4], 1)
	binary.BigEndian.PutUint32(pack[4:8], uint32(int32(objectFile)))
	binary.BigEndian.PutUint32(pack[8:12], uint32(int32(terrainFile)))
	binary.BigEndian.PutUint32(pack[12:16], uint32(regionID>>8))
	binary.BigEndian.PutUint32(pack[16:20], uint32(regionID&0xff))
	binary.BigEndian.PutUint32(pack[20:24], uint32(len(objects)))
	copy(pack[24:], objects)
	binary.BigEndian.PutUint32(pack[24+len(objects):], uint32(len(terrain)))
	copy(pack[28+len(objects):], terrain)
	return pack, true
}

// RegionReplacementSink frames and sends one prepared
// replacement packet to one player: it receives the opcode and
// the payload (with the allowReload flag already applied),
// frames them with protocol.EncodePacket, and queues the frame
// on the player's session. It returns false when the player
// has no live session to send to, mirroring sendClientPacket's
// false. A nil sink sends nothing and answers false.
type RegionReplacementSink func(opcode int, payload []byte) bool

// SendRegionPacket sends one prepared packet to a player
// (MapRegionReplacementManager.ts:141-147,
// sendRegionPacketToPlayer): the packet framed with
// allowReload=true. It returns false when the sink is nil or
// the send fails.
func (m *MapRegionReplacementManager) SendRegionPacket(sink RegionReplacementSink, packet RegionReplacementPacket) bool {
	return sendReplacementPacket(sink, packet, true)
}

func sendReplacementPacket(sink RegionReplacementSink, packet RegionReplacementPacket, allowReload bool) bool {
	if sink == nil {
		return false
	}
	opcode, payload := packet.Frame(allowReload)
	return sink(opcode, payload)
}

// SendAllReplacements sends every installed replacement to a
// player, in region-id order
// (MapRegionReplacementManager.ts:185-197), and returns how
// many sends succeeded.
func (m *MapRegionReplacementManager) SendAllReplacements(sink RegionReplacementSink) int {
	ordered := m.orderedReplacements()
	sent := 0
	for _, replacement := range ordered {
		if sendReplacementPacket(sink, replacement.packet, true) {
			sent++
		}
	}
	return sent
}

// SendVisibleReplacements sends the installed replacements
// whose regions are visible from the tile (tileX, tileY) —
// the regions intersecting the chunk window of the given
// radius around it — in region-id order
// (MapRegionReplacementManager.ts:199-232). excludeRegionIds
// are skipped; allowReload is the frame flag. It returns how
// many sends succeeded. The flush path calls it with radius 6
// on every scene-window change (PlayerSession.ts:497-505).
func (m *MapRegionReplacementManager) SendVisibleReplacements(sink RegionReplacementSink, tileX, tileY, chunkRadius int, excludeRegionIDs []int, allowReload bool) int {
	visible := computeVisibleRegionIDs(tileX, tileY, chunkRadius)
	if len(visible) == 0 {
		return 0
	}
	excluded := make(map[int]struct{}, len(excludeRegionIDs))
	for _, id := range excludeRegionIDs {
		excluded[id] = struct{}{}
	}
	ordered := make([]int, 0, len(visible))
	for id := range visible {
		ordered = append(ordered, id)
	}
	sort.Ints(ordered)
	sent := 0
	for _, regionID := range ordered {
		if _, skip := excluded[regionID]; skip {
			continue
		}
		replacement, ok := m.replacement(regionID)
		if !ok {
			continue
		}
		if sendReplacementPacket(sink, replacement.packet, allowReload) {
			sent++
		}
	}
	return sent
}

// SendNonVisibleReplacements sends every installed
// replacement whose region is NOT visible from the tile, in
// region-id order (MapRegionReplacementManager.ts:234-256).
// TS falls back to sending all replacements when the
// coordinates are not integers; Go ints are always integers,
// so that branch has no form here.
func (m *MapRegionReplacementManager) SendNonVisibleReplacements(sink RegionReplacementSink, tileX, tileY, chunkRadius int) int {
	visible := computeVisibleRegionIDs(tileX, tileY, chunkRadius)
	sent := 0
	for _, replacement := range m.orderedReplacements() {
		if _, isVisible := visible[replacement.regionID]; isVisible {
			continue
		}
		if sendReplacementPacket(sink, replacement.packet, true) {
			sent++
		}
	}
	return sent
}

// SendReplacement sends one installed replacement to a player
// (MapRegionReplacementManager.ts:258-268). It returns false
// when no replacement is installed for the region, the sink is
// nil, or the send fails.
func (m *MapRegionReplacementManager) SendReplacement(sink RegionReplacementSink, regionID int, allowReload bool) bool {
	replacement, ok := m.replacement(regionID)
	if !ok {
		return false
	}
	return sendReplacementPacket(sink, replacement.packet, allowReload)
}

func (m *MapRegionReplacementManager) replacement(regionID int) (*regionReplacement, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	replacement, ok := m.replacements[regionID]
	return replacement, ok
}

func (m *MapRegionReplacementManager) orderedReplacements() []*regionReplacement {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	ordered := make([]*regionReplacement, 0, len(m.replacements))
	for _, replacement := range m.replacements {
		ordered = append(ordered, replacement)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].regionID < ordered[j].regionID })
	return ordered
}

// computeVisibleRegionIDs answers the region ids intersecting
// the chunk window of the given radius around a tile
// (MapRegionReplacementManager.ts:269-293). Region coordinates
// outside 0..255 are skipped.
func computeVisibleRegionIDs(tileX, tileY, chunkRadius int) map[int]struct{} {
	chunkX := tileX >> 3
	chunkY := tileY >> 3
	minRegionX := (chunkX - chunkRadius) >> 3
	maxRegionX := (chunkX + chunkRadius) >> 3
	minRegionY := (chunkY - chunkRadius) >> 3
	maxRegionY := (chunkY + chunkRadius) >> 3
	ids := make(map[int]struct{})
	for regionX := minRegionX; regionX <= maxRegionX; regionX++ {
		if regionX < 0 || regionX > 0xff {
			continue
		}
		for regionY := minRegionY; regionY <= maxRegionY; regionY++ {
			if regionY < 0 || regionY > 0xff {
				continue
			}
			ids[((regionX&0xff)<<8)|(regionY&0xff)] = struct{}{}
		}
	}
	return ids
}

// countObjectPlacements counts the object placements in an
// object stream (MapRegionReplacementManager.ts:295-322): the
// id-delta/location-delta smart encoding, one count per
// placement attribute byte. A nil or empty stream counts 0.
func countObjectPlacements(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	stream := &mapReader{buf: data}
	count := 0
	for stream.off < len(stream.buf) {
		if stream.getUSmart() == 0 {
			break
		}
		for stream.off < len(stream.buf) {
			if stream.getUSmart() == 0 {
				break
			}
			if stream.off >= len(stream.buf) {
				break
			}
			stream.readUnsignedByte()
			count++
		}
	}
	return count
}

// resolveReplacementPath resolves a replacement source path:
// absolute paths as-is, relative paths against the working
// directory (MapRegionReplacementManager.ts:370-379,
// resolveSourcePath). An empty path resolves to the working
// directory itself; callers validate emptiness first.
func resolveReplacementPath(rawPath string) string {
	trimmed := strings.TrimSpace(rawPath)
	if filepath.IsAbs(trimmed) {
		return trimmed
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return trimmed
	}
	return abs
}

// readPackSource reads a .pack file: the 28-byte header plus
// the object and terrain archives
// (MapRegionReplacementManager.ts:327-368). It validates the
// version, the region, and the cache archive ids, and gunzip-
// decodes both sections. Archive-id validation requires the
// archive-id seam: without it, .pack loads are refused, since
// TS always validates against the active cache.
func (m *MapRegionReplacementManager) readPackSource(regionID int, packPath string) (terrainData, objectData []byte, err error) {
	data, err := os.ReadFile(packPath)
	if err != nil {
		return nil, nil, err
	}
	if len(data) < 28 {
		return nil, nil, fmt.Errorf("pack file too short: %s", packPath)
	}
	readInt32 := func(off int) int32 { return int32(binary.BigEndian.Uint32(data[off:])) }
	if readInt32(0) != 1 || readInt32(12) != int32(regionID>>8) || readInt32(16) != int32(regionID&0xff) {
		return nil, nil, fmt.Errorf("pack region does not match %d: %s", regionID, packPath)
	}
	m.mu.RLock()
	archiveIDs := m.archiveIDs
	m.mu.RUnlock()
	if archiveIDs == nil {
		return nil, nil, fmt.Errorf("pack archive IDs cannot be checked without cache archive IDs: %s", packPath)
	}
	terrainFile, objectFile, ok := archiveIDs(regionID)
	if !ok || readInt32(4) != int32(objectFile) || readInt32(8) != int32(terrainFile) {
		return nil, nil, fmt.Errorf("pack archive IDs do not match the active cache: %s", packPath)
	}
	firstLength := int(readInt32(20))
	if firstLength <= 0 {
		return nil, nil, fmt.Errorf("invalid first entry length in pack: %s", packPath)
	}
	firstDataStart := 24
	firstDataEnd := firstDataStart + firstLength
	if firstDataEnd+4 > len(data) {
		return nil, nil, fmt.Errorf("truncated first entry in pack: %s", packPath)
	}
	secondLength := int(readInt32(firstDataEnd))
	if secondLength < 0 {
		return nil, nil, fmt.Errorf("invalid second entry length in pack: %s", packPath)
	}
	secondDataStart := firstDataEnd + 4
	secondDataEnd := secondDataStart + secondLength
	if secondDataEnd > len(data) {
		return nil, nil, fmt.Errorf("truncated second entry in pack: %s", packPath)
	}
	objectData, err = decodeReplacementMapFileBytes(data[firstDataStart:firstDataEnd], packPath+"#object")
	if err != nil {
		return nil, nil, err
	}
	terrainData, err = decodeReplacementMapFileBytes(data[secondDataStart:secondDataEnd], packPath+"#terrain")
	if err != nil {
		return nil, nil, err
	}
	return terrainData, objectData, nil
}

// readReplacementMapFile reads one map file, gunzip-decoding
// it when it carries the gzip magic
// (MapRegionReplacementManager.ts:381-384).
func readReplacementMapFile(filePath string) ([]byte, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return decodeReplacementMapFileBytes(data, filePath)
}

// decodeReplacementMapFileBytes gunzip-decodes gzip-compressed
// map bytes and passes raw bytes through
// (MapRegionReplacementManager.ts:386-402). Empty input is an
// error, as in TS.
func decodeReplacementMapFileBytes(data []byte, sourceLabel string) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty map data: %s", sourceLabel)
	}
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("failed to gunzip '%s': %v", sourceLabel, err)
		}
		decoded, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to gunzip '%s': %v", sourceLabel, err)
		}
		if err := reader.Close(); err != nil {
			return nil, fmt.Errorf("failed to gunzip '%s': %v", sourceLabel, err)
		}
		return decoded, nil
	}
	return append([]byte(nil), data...), nil
}

var (
	replacementPackName    = regexp.MustCompile(`^(\d+)\.pack$`)
	replacementTerrainName = regexp.MustCompile(`^(\d+)\.terrain\.dat$`)
)

// LoadRegionReplacementDir loads the region replacements in a
// directory, as RegionManager.init does for data/regions
// (RegionManager.ts:52-84): `<regionId>.pack` files and
// `<regionId>.terrain.dat` files, sorted by name. A region may
// only carry one replacement file: a .pack wins over a
// .terrain.dat for the same region, matching how the editor
// writes them, and the .terrain.dat is skipped with a warning.
// A file that fails to load is skipped with a warning and the
// cache region stands. A missing directory is a no-op. warn
// receives the warning lines (nil discards them). Terrain .dat
// files load raw, without gunzip decoding, exactly as TS reads
// them.
func LoadRegionReplacementDir(dir string, m *MapRegionReplacementManager, warn func(msg string)) {
	if m == nil {
		return
	}
	if warn == nil {
		warn = func(string) {}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	packRegions := make(map[int]struct{})
	for _, name := range names {
		if match := replacementPackName.FindStringSubmatch(name); match != nil {
			var id int
			fmt.Sscanf(match[1], "%d", &id)
			packRegions[id] = struct{}{}
		}
	}
	for _, name := range names {
		var regionID int
		if match := replacementPackName.FindStringSubmatch(name); match != nil {
			fmt.Sscanf(match[1], "%d", &regionID)
			filePath := filepath.Join(dir, name)
			if _, err := m.ReplaceMapRegion(regionID, filePath); err != nil {
				warn(fmt.Sprintf("[collision] skipping region replacement (falling back to cache region): %s %v", filePath, err))
			}
			continue
		}
		match := replacementTerrainName.FindStringSubmatch(name)
		if match == nil {
			continue
		}
		fmt.Sscanf(match[1], "%d", &regionID)
		if _, hasPack := packRegions[regionID]; hasPack {
			warn(fmt.Sprintf("[collision] ignoring %s: region %d already has a .pack", name, regionID))
			continue
		}
		filePath := filepath.Join(dir, name)
		raw, err := os.ReadFile(filePath)
		if err != nil {
			warn(fmt.Sprintf("[collision] skipping region replacement (falling back to cache region): %s %v", filePath, err))
			continue
		}
		if _, err := m.ReplaceMapRegionTerrain(regionID, raw, filePath); err != nil {
			warn(fmt.Sprintf("[collision] skipping region replacement (falling back to cache region): %s %v", filePath, err))
		}
	}
}

// ReplacementRegionSource wraps a RegionDataSource so region
// loads consult installed replacements first: the Go form of
// the replacement lookup in loadMapFiles
// (RegionManager.ts:900-906). A replacement's terrain always
// wins when one is installed; its objects win when present,
// else the cache's objects stand (a terrain-only replacement
// keeps the cache's objects). With no replacement installed,
// the base source answers. A nil Base answers only
// replacements; a nil Replacements answers only the base. Wire
// it between the cache adapter and NewRegionManager at boot so
// collision loads see replaced maps without touching the
// region decoder.
type ReplacementRegionSource struct {
	Base         RegionDataSource
	Replacements *MapRegionReplacementManager
}

// RegionIDs delegates to the base source: replacements never
// add region ids to the index (TS indexes the regions map from
// the cache alone, RegionManager.ts:46-52).
func (s *ReplacementRegionSource) RegionIDs() []int {
	if s == nil || s.Base == nil {
		return nil
	}
	return s.Base.RegionIDs()
}

// RegionData answers the replacement-aware map data for a
// region. See the type comment.
func (s *ReplacementRegionSource) RegionData(regionID int) (terrain, objects []byte, ok bool) {
	if s != nil && s.Replacements != nil {
		if terrainData, objectData, replaced := s.Replacements.GetReplacementMapData(regionID); replaced {
			if objectData == nil && s.Base != nil {
				if _, cachedObjects, cachedOK := s.Base.RegionData(regionID); cachedOK {
					objectData = cachedObjects
				}
			}
			return terrainData, objectData, true
		}
	}
	if s == nil || s.Base == nil {
		return nil, nil, false
	}
	return s.Base.RegionData(regionID)
}

var _ RegionDataSource = (*ReplacementRegionSource)(nil)

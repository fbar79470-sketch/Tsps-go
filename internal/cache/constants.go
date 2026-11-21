package cache

// Cache file layout constants.
//
// Sector geometry: Sector.ts. A sector is 520 bytes: an 8-byte header
// (10 for extended archives) plus 512 bytes of data (510 extended).
const (
	// SectorSize is the total size of one sector in main_file_cache.dat2.
	SectorSize = 520
	// SectorHeaderSize is the header size for archives with id <= 65535.
	SectorHeaderSize = 8
	// SectorDataSize is the payload size for archives with id <= 65535.
	SectorDataSize = 512
	// SectorExtendedHeaderSize is the header size for archives with id > 65535.
	SectorExtendedHeaderSize = 10
	// SectorExtendedDataSize is the payload size for archives with id > 65535.
	SectorExtendedDataSize = 510
	// SectorClusterSize is the size of one index entry: a 3-byte size
	// followed by a 3-byte first-sector number. SectorCluster.SIZE
	// (store/SectorCluster.ts:4).
	SectorClusterSize = 6
	// ExtendedArchiveIDThreshold is the archive id above which the
	// extended sector format is used (FileStore.ts:44).
	ExtendedArchiveIDThreshold = 65535
)

// Cache file names. CacheFiles.ts.
const (
	// Dat2FileName is the main data file.
	Dat2FileName = "main_file_cache.dat2"
	// IndexFilePrefix prefixes every index file; the index id follows it.
	IndexFilePrefix = "main_file_cache.idx"
	// MetaFileName is the meta index (index 255), which holds the
	// reference table of every other index.
	MetaFileName = "main_file_cache.idx255"
)

// MetaIndexID is the index id whose archives are the other indexes'
// reference tables. CacheIndex.META_INDEX_ID (CacheIndex.ts:14).
const MetaIndexID = 255

// GamevalIndex is the index holding gameval name tables.
// GAMEVAL_INDEX (Gamevals.ts:9). Cache validation requires idx255 to
// reach past this index (CachePipeline.isValid).
const GamevalIndex = 24

// Index ids (DAT2 layout), from IndexType.DAT2 and IndexType.OSRS
// (cache/IndexType.ts:18-45).
const (
	IndexAnimations        = 0
	IndexSkeletons         = 1
	IndexConfigs           = 2
	IndexInterfaces        = 3
	IndexSoundEffects      = 4
	IndexMaps              = 5
	IndexMusicTracks       = 6
	IndexModels            = 7
	IndexSprites           = 8
	IndexTextures          = 9
	IndexBinary            = 10
	IndexMusicJingles      = 11
	IndexClientScript      = 12
	IndexFonts             = 13
	IndexMusicSamples      = 14
	IndexMusicPatches      = 15
	IndexWorldMapOld       = 16
	IndexGraphicDefaults   = 17
	IndexWorldMapGeography = 18
	IndexWorldMap          = 19
	IndexWorldMapGround    = 20
	IndexDBTable           = 21
	IndexAnimKeyFrames     = 22
)

// Config archive ids within the configs index (index 2), from
// ConfigType.DAT2 and ConfigType.OSRS (cache/ConfigType.ts:70-97).
const (
	ConfigUnderlays       = 1
	ConfigIdentkits       = 3
	ConfigOverlays        = 4
	ConfigInv             = 5
	ConfigLocs            = 6
	ConfigEnums           = 8
	ConfigNPCs            = 9
	ConfigObjs            = 10
	ConfigParams          = 11
	ConfigSeqs            = 12
	ConfigSpotAnims       = 13
	ConfigVarbits         = 14
	ConfigVarClientString = 15
	ConfigVarps           = 16
	ConfigVarPlayer       = 16
	ConfigVarClient       = 19
	ConfigHitSplat        = 32
	ConfigHealthBar       = 33
	ConfigStruct          = 34
	ConfigMapFunctions    = 35
	ConfigDBRow           = 38
	ConfigDBTable         = 39
	ConfigWorldEntity     = 72
)

// Compression identifies a container's compression scheme: the first
// byte of a container. CompressionType (compression/CompressionType.ts).
type Compression int

const (
	// CompressionNone: the container holds raw bytes.
	CompressionNone Compression = 0
	// CompressionBzip2: the payload is bzip2 data with its 4-byte
	// "BZh?" header stripped (restored before decompression).
	CompressionBzip2 Compression = 1
	// CompressionGzip: the payload is a gzip stream.
	CompressionGzip Compression = 2
)

// String returns the TypeScript enum member name, as used in the
// container size-mismatch error (Container.ts:42).
func (c Compression) String() string {
	switch c {
	case CompressionNone:
		return "None"
	case CompressionBzip2:
		return "Bzip2"
	case CompressionGzip:
		return "Gzip"
	default:
		return "Unknown"
	}
}

// Package cache reads the Old School RuneScape game cache from local
// disk, the way TSPS does: a FileStore over main_file_cache.dat2 /
// main_file_cache.idx*, container decryption/decompression, and
// reference-table decoding.
//
// There is deliberately no JS5/update server here: TSPS has none (no
// file under server/src mentions JS5), and the client obtains the
// cache independently. The server only ever reads its local copy,
// which EnsureCache downloads at startup from the OpenRS2 archive
// when it is missing. Cache data is never bundled with this project.
//
// Upstream sources (paths relative to server/ in RSPSApp/tsps, under
// src/main/typescript/elvarg/game/cache unless noted):
//   - CachePipeline.ts (target parsing, validation, OpenRS2 download)
//   - codec/rs/cache/store/FileStore.ts, Sector.ts, SectorCluster.ts,
//     CacheStore.ts
//   - codec/rs/cache/CacheIndex.ts, Container.ts, Archive.ts,
//     ArchiveFile.ts, ref/ReferenceTable.ts, ref/ArchiveReference.ts,
//     ref/ArchiveFileReference.ts
//   - codec/rs/cache/IndexType.ts, ConfigType.ts, CacheFiles.ts,
//     CacheInfo.ts, CacheType.ts
//   - codec/rs/io/ByteBuffer.ts
//   - codec/rs/crypto/Xtea.ts
//   - codec/rs/compression/CompressionType.ts, Bzip2.ts, Gzip.ts
//   - codec/rs/util/Crc32.ts, StringUtil.ts
//   - codec/rs/MathConstants.ts (BIT_MASKS)
//   - codec/rs/config/Type.ts, TypeLoader.ts,
//     ArchivedDefinitionRegistry.ts, ArchivedTypeLoader.ts, and the
//     objtype, npctype, loctype, seqtype, spotanimtype, vartype,
//     idktype, bastype and dbrow definition decoders
//   - CacheDefinitions.ts, CacheMaps.ts, Gamevals.ts
//   - scripts/ensure-cache.ts (pre-start download variant)
//
// The definition decoders live in definitions.go (generic loader
// machinery), obj.go, npc.go, loc.go, seq.go, spotanim.go,
// varbit.go, idk.go, bas.go and dbrow.go; the archived overlay in
// archived.go; the CacheIndexDat2 facade in index.go; the
// CacheDefinitions facade in facade.go; CacheMaps in maps.go; and
// Gamevals in gamevals.go.
//
// Behavior that TypeScript leaves unchecked (truncated buffers, corrupt
// sector data) returns an explicit error here instead of silently
// producing short or garbage data; each such hardening is noted in a
// comment at the site.
package cache

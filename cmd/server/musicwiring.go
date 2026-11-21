package main

import (
	"tsps-go/internal/cache"
	"tsps-go/internal/music"
)

// Music wiring: boot-time loading of the music-area data and the
// track lengths from DbTable 44 (Music.cachedTracks, Music.ts).
// The returned Data is installed on every entering session (see
// AddPlayer); sessions read areas and lengths from it on the tick
// goroutine.

// musicDbTableID is the DbTable id Music.cachedTracks reads
// (CacheDefinitions.getDbTableRows(44), Music.ts).
const musicDbTableID = 44

// loadMusicData loads the music areas from the definitions
// directory dir and, when the cache facade is present, the track
// lengths from DbTable 44. Area loading never fails the boot
// (music.LoadAreas warns and loads empty, as TS loadMusicAreas
// does); a lengths failure likewise logs and continues without
// lengths, so area tracks never end on their own — the TS
// no-lengths behavior when getDbTableRows returns no rows. In
// NoCache mode defs is nil and only the areas load. The returned
// Data is never nil.
func loadMusicData(dir string, defs *cache.CacheDefinitions, logf func(format string, args ...any)) *music.Data {
	data := music.LoadAreas(dir)
	if defs == nil {
		return data
	}
	rows, err := defs.GetDbTableRows(musicDbTableID)
	if err != nil {
		if logf != nil {
			logf("[music] failed to load track lengths from DbTable %d: %v (tracks will not end on their own)", musicDbTableID, err)
		}
		return data
	}
	converted := make([]music.Row, len(rows))
	for i, row := range rows {
		converted[i] = row
	}
	data.SetLengths(converted)
	return data
}

package persist

// Profiles: port of game/entity/impl/player/persistence/PlayerProfile.ts.
//
// Which of an account's characters a save belongs to. A world running a
// Leagues or Hardcore profile logs every account into that character,
// which lives in its own save, never as a flag on the main one. They all
// share the account's name, so only one of them can be online at a time.

// Profile is one of an account's characters (PlayerProfile.ts:8).
type Profile string

const (
	ProfileMain     Profile = "main"
	ProfileLeague   Profile = "league"
	ProfileHardcore Profile = "hardcore"
)

// Profiles lists every profile an account can have a character in, the
// main one first (PlayerProfile.ts:14, PLAYER_PROFILES).
var Profiles = []Profile{ProfileMain, ProfileLeague, ProfileHardcore}

// Valid reports whether p is one of the three known profiles.
func (p Profile) Valid() bool {
	switch p {
	case ProfileMain, ProfileLeague, ProfileHardcore:
		return true
	default:
		return false
	}
}

// OtherProfiles returns the account's other characters, in Profiles
// order: the saves that vouch for a login of a character not made yet
// (PlayerProfile.ts:18-20).
func OtherProfiles(profile Profile) []Profile {
	out := make([]Profile, 0, 2)
	for _, p := range Profiles {
		if p != profile {
			out = append(out, p)
		}
	}
	return out
}

// ProfileStorageKey is a backend's key for a profile's save, exactly as
// in TS (PlayerProfile.ts:26-29): the main profile keeps the plain
// (normalised) name; any other is prefixed after normalisation, with a
// character normalisation never emits (":"), so no real account name
// can collide with it. normalizedUsername must ALREADY be normalised
// (see NormalizeUsername); this function does not normalise, matching
// TS, whose callers normalise first (e.g. SqlitePlayerPersistence
// saveKey).
func ProfileStorageKey(normalizedUsername string, profile Profile) string {
	if profile == ProfileMain {
		return normalizedUsername
	}
	return string(profile) + ":" + normalizedUsername
}

// StorageKey derives the full storage key for a username and profile:
// the username is normalised (NormalizeUsername, as the JSON and SQLite
// backends do before keying), then ProfileStorageKey is applied. This
// is the key the SQLite backend stores as its row key
// (SqlitePlayerPersistence.plugin.js saveKey) and the key MemoryStore
// indexes by. The JSON file backend instead uses subdirectories per
// profile (see JSONFileStore); StorageKey is still the right key for
// comparing "same character" across backends.
func StorageKey(username string, profile Profile) string {
	return ProfileStorageKey(NormalizeUsername(username), profile)
}

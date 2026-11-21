package persist

import (
	"regexp"
	"strings"
	"unicode"
)

// Name handling: ports of DisplayNames.key, Misc.formatPlayerName /
// formatText / formatDisplayName, and the persistence plugins'
// normalizeUsername.

// Key returns the key two names share when they are the same name: case,
// underscores and extra spaces don't count. Port of DisplayNames.key
// (DisplayNames.ts:23-25), which Accounts.key delegates to
// (Accounts.ts:57-59).
func Key(name string) string {
	s := strings.ReplaceAll(name, "_", " ")
	s = strings.TrimSpace(s)
	s = whitespaceRuns.ReplaceAllString(s, " ")
	return strings.ToLower(s)
}

var whitespaceRuns = regexp.MustCompile(`\s+`)

// FormatPlayerName formats a typed name the way punishments (and save
// file names) key it: port of Misc.formatPlayerName, which delegates to
// Misc.formatText (Misc.ts:331-333, 305-322). The first character and the
// character after any non-alphanumeric character are uppercased, and
// only the FIRST underscore is replaced with a space (JS String.replace
// with a string argument replaces one occurrence). Callers in TS
// lowercase the input first (PlayerPunishment.banned/muted); this
// function does not lowercase, matching formatText itself.
func FormatPlayerName(s string) string {
	runes := []rune(s)
	for i, r := range runes {
		if i == 0 {
			runes[i] = unicode.ToUpper(r)
			continue
		}
		if !isAlphanumericASCII(runes[i-1]) {
			runes[i] = unicode.ToUpper(r)
		}
	}
	out := string(runes)
	// Replace only the first underscore, matching JS replace("_", " ").
	if idx := strings.Index(out, "_"); idx >= 0 {
		out = out[:idx] + " " + out[idx+1:]
	}
	return out
}

func isAlphanumericASCII(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// FormatDisplayName renders a typed login name as shown in game:
// capitals kept, underscores as spaces. Port of Misc.formatDisplayName
// (Misc.ts:806-808).
func FormatDisplayName(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "_", " ")
}

var nonAlphanumericRuns = regexp.MustCompile(`(?i)[^a-z0-9]+`)

// NormalizeUsername derives the file/database name for a username. Port
// of JsonPlayerPersistence.normalizeUsername
// (JsonPlayerPersistence.plugin.js, resolveFilePath/normalizeUsername;
// SqlitePlayerPersistence.plugin.js normalizeUsername is identical):
// trim, lowercase, formatPlayerName, replace every run of non-alphanumeric
// characters with a single underscore, strip leading/trailing
// underscores, lowercase again; an empty result becomes "player".
func NormalizeUsername(username string) string {
	formatted := FormatPlayerName(strings.TrimSpace(strings.ToLower(username)))
	safe := nonAlphanumericRuns.ReplaceAllString(formatted, "_")
	safe = strings.Trim(safe, "_")
	safe = strings.ToLower(safe)
	if safe == "" {
		return "player"
	}
	return safe
}

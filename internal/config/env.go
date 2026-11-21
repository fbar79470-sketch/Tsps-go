package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Env is the process-environment configuration for the server. Every
// field documents its variable and default; defaults follow the
// TypeScript sources cited, not server/.env.example's illustrative
// values (notably WEBSOCKET_PORT: 43594 in code, 49598 in the
// example file).
//
// LoadEnv reads the process environment only. It does not read .env
// files: in the TypeScript setup the start scripts layer .env files
// before launching (via run-with-env.mjs, outside the server package),
// with explicitly exported variables winning.
type Env struct {
	// WebsocketPort is WEBSOCKET_PORT, the port for the WebSocket
	// listener and HTTP content endpoints. Default 43594
	// (NetworkConstants.WEBSOCKET_PORT, net/NetworkConstants.ts:3).
	// An unset or empty value means the default; a value with no
	// leading digits (as in JavaScript parseInt with radix 10)
	// means 0, as in the TypeScript code.
	WebsocketPort int

	// PublicHost is PUBLIC_HOST, the host advertised in the gateway
	// jav_config/world list. Default "127.0.0.1". A value that is
	// not a bare host name or IP address is ignored and the default
	// is used instead (GatewayTarget.host, net/http/GatewayTarget.ts).
	PublicHost string

	// PlayerSaveDatabasePath is PLAYER_SAVE_DATABASE_PATH, the SQLite
	// player-save database location. Default
	// <cwd>/data/saves/players.sqlite. A set value is resolved to an
	// absolute path (SqlitePlayerPersistence.plugin.js).
	PlayerSaveDatabasePath string

	// AccountsFile is TSPS_ACCOUNTS_FILE, the accounts file override.
	// Default <cwd>/data/saves/accounts.json. Unlike the save paths,
	// a set value is used verbatim (Accounts.file, game/entity/impl/
	// player/Accounts.ts).
	AccountsFile string

	// LegacySaveDir is LEGACY_PLAYER_SAVE_DIRECTORY, the legacy JSON
	// save directory imported from on startup. Default
	// <cwd>/data/saves/characters. A set value is resolved to an
	// absolute path (SqlitePlayerPersistence.plugin.js).
	LegacySaveDir string

	// DisablePlayerBots is DISABLE_PLAYER_BOTS == "1": the player-bot
	// plugin/runtime is disabled entirely (PluginManager.ts, Server.ts).
	DisablePlayerBots bool

	// DevUsernames is DEV_USERNAME split on commas, with whitespace
	// trimmed and empty entries dropped: usernames granted DEVELOPER
	// rights on login (NetworkBuilder.isConfiguredDeveloperUsername).
	// Nil when the variable is unset or blank. Case is preserved;
	// comparison lowercases both sides in the TypeScript code.
	DevUsernames []string

	// BrowserHost is BROWSER_HOST == "1": browser-host mode, in which
	// the cache must already exist (CachePipeline refuses to download
	// it) and game frames travel over the HTTP bridge instead of a
	// WebSocket (NetworkBuilder.ts, CachePipeline.ts).
	BrowserHost bool
}

// hostPattern is the bare-host check from GatewayTarget.host: no
// scheme, port, path, or spaces.
var hostPattern = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

// LoadEnv reads the process environment into an Env. See the Env
// field documentation for each variable's meaning and default.
func LoadEnv() Env {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	env := Env{
		WebsocketPort:          defaultPort("WEBSOCKET_PORT", 43594),
		PublicHost:             publicHost(),
		PlayerSaveDatabasePath: resolvedPath("PLAYER_SAVE_DATABASE_PATH", filepath.Join(cwd, "data", "saves", "players.sqlite")),
		AccountsFile:           accountsFile(cwd),
		LegacySaveDir:          resolvedPath("LEGACY_PLAYER_SAVE_DIRECTORY", filepath.Join(cwd, "data", "saves", "characters")),
		DisablePlayerBots:      os.Getenv("DISABLE_PLAYER_BOTS") == "1",
		DevUsernames:           devUsernames(),
		BrowserHost:            os.Getenv("BROWSER_HOST") == "1",
	}
	return env
}

// defaultPort mirrors `parseInt(process.env.X || "<def>", 10) || 0`
// from NetworkConstants.ts: unset/empty means the default; otherwise
// a JavaScript parseInt (radix 10) of the value, or 0 when the value
// has no leading digits.
func defaultPort(name string, def int) int {
	value := os.Getenv(name)
	if value == "" {
		return def
	}
	return parseJSInt(value)
}

// parseJSInt parses like JavaScript parseInt(s, 10): leading
// whitespace skipped, an optional sign, then decimal digits; the
// parse stops at the first non-digit, and a value with no digits
// yields 0.
func parseJSInt(s string) int {
	s = strings.TrimLeft(s, jsTrimSpace)
	neg := false
	if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
		neg = s[0] == '-'
		s = s[1:]
	}
	digits := 0
	for digits < len(s) && s[digits] >= '0' && s[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return 0
	}
	n, err := strconv.ParseInt(s[:digits], 10, 64)
	if err != nil {
		// ParseInt returns the clamped extreme on range errors;
		// keep it, as JavaScript would keep the large number.
		if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange {
			if neg {
				return int(-n)
			}
			return int(n)
		}
		return 0
	}
	if neg {
		n = -n
	}
	return int(n)
}

func publicHost() string {
	configured := strings.TrimSpace(os.Getenv("PUBLIC_HOST"))
	if configured == "" || !hostPattern.MatchString(configured) {
		return "127.0.0.1"
	}
	return configured
}

// resolvedPath mirrors the `process.env.X ? path.resolve(X) : default`
// pattern of SqlitePlayerPersistence.plugin.js: an empty value means
// the default; a set value is resolved against the working directory.
func resolvedPath(name, def string) string {
	value := os.Getenv(name)
	if value == "" {
		return def
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return value
	}
	return abs
}

func accountsFile(cwd string) string {
	if value := os.Getenv("TSPS_ACCOUNTS_FILE"); value != "" {
		return value
	}
	return filepath.Join(cwd, "data", "saves", "accounts.json")
}

func devUsernames() []string {
	raw := os.Getenv("DEV_USERNAME")
	if raw == "" {
		return nil
	}
	var names []string
	for _, name := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}

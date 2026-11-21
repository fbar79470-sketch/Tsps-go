package config

import (
	"os"
	"path/filepath"
	"testing"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"WEBSOCKET_PORT", "PUBLIC_HOST", "PLAYER_SAVE_DATABASE_PATH",
		"TSPS_ACCOUNTS_FILE", "LEGACY_PLAYER_SAVE_DIRECTORY",
		"DISABLE_PLAYER_BOTS", "DEV_USERNAME", "BROWSER_HOST",
	} {
		t.Setenv(name, "")
	}
}

func TestLoadEnvDefaults(t *testing.T) {
	clearEnv(t)
	// Unset (not merely empty) must behave the same; t.Setenv cannot
	// unset, so verify the empty-value path and the unset path for one
	// variable via os.Unsetenv with restore.
	t.Setenv("WEBSOCKET_PORT", "")
	env := LoadEnv()
	if env.WebsocketPort != 43594 {
		t.Errorf("WebsocketPort = %d, want 43594 (code default, not .env.example's 49598)", env.WebsocketPort)
	}
	if env.PublicHost != "127.0.0.1" {
		t.Errorf("PublicHost = %q", env.PublicHost)
	}
	cwd, _ := os.Getwd()
	if want := filepath.Join(cwd, "data", "saves", "players.sqlite"); env.PlayerSaveDatabasePath != want {
		t.Errorf("PlayerSaveDatabasePath = %q, want %q", env.PlayerSaveDatabasePath, want)
	}
	if want := filepath.Join(cwd, "data", "saves", "accounts.json"); env.AccountsFile != want {
		t.Errorf("AccountsFile = %q, want %q", env.AccountsFile, want)
	}
	if want := filepath.Join(cwd, "data", "saves", "characters"); env.LegacySaveDir != want {
		t.Errorf("LegacySaveDir = %q, want %q", env.LegacySaveDir, want)
	}
	if env.DisablePlayerBots || env.BrowserHost {
		t.Errorf("DisablePlayerBots = %v, BrowserHost = %v, want both false", env.DisablePlayerBots, env.BrowserHost)
	}
	if len(env.DevUsernames) != 0 {
		t.Errorf("DevUsernames = %v, want empty", env.DevUsernames)
	}
}

func TestLoadEnvUnset(t *testing.T) {
	// os.Unsetenv path: LoadEnv must not depend on variables being set
	// to empty strings.
	old, hadOld := os.LookupEnv("WEBSOCKET_PORT")
	t.Cleanup(func() {
		if hadOld {
			os.Setenv("WEBSOCKET_PORT", old)
		} else {
			os.Unsetenv("WEBSOCKET_PORT")
		}
	})
	os.Unsetenv("WEBSOCKET_PORT")
	if got := LoadEnv().WebsocketPort; got != 43594 {
		t.Errorf("unset WEBSOCKET_PORT -> %d, want 43594", got)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("WEBSOCKET_PORT", "49598")
	t.Setenv("PUBLIC_HOST", "play.example.com")
	t.Setenv("PLAYER_SAVE_DATABASE_PATH", "/tmp/saves/db.sqlite")
	t.Setenv("TSPS_ACCOUNTS_FILE", "relative/accounts.json")
	t.Setenv("LEGACY_PLAYER_SAVE_DIRECTORY", "/tmp/legacy")
	t.Setenv("DISABLE_PLAYER_BOTS", "1")
	t.Setenv("DEV_USERNAME", "Alice, bob ,,  Carol ")
	t.Setenv("BROWSER_HOST", "1")
	env := LoadEnv()
	if env.WebsocketPort != 49598 {
		t.Errorf("WebsocketPort = %d", env.WebsocketPort)
	}
	if env.PublicHost != "play.example.com" {
		t.Errorf("PublicHost = %q", env.PublicHost)
	}
	if env.PlayerSaveDatabasePath != "/tmp/saves/db.sqlite" {
		t.Errorf("PlayerSaveDatabasePath = %q", env.PlayerSaveDatabasePath)
	}
	// AccountsFile keeps a set value verbatim (Accounts.file).
	if env.AccountsFile != "relative/accounts.json" {
		t.Errorf("AccountsFile = %q, want verbatim relative value", env.AccountsFile)
	}
	if env.LegacySaveDir != "/tmp/legacy" {
		t.Errorf("LegacySaveDir = %q", env.LegacySaveDir)
	}
	if !env.DisablePlayerBots || !env.BrowserHost {
		t.Error("DisablePlayerBots/BrowserHost should be true for \"1\"")
	}
	if got := env.DevUsernames; len(got) != 3 || got[0] != "Alice" || got[1] != "bob" || got[2] != "Carol" {
		t.Errorf("DevUsernames = %v", got)
	}
}

func TestLoadEnvEdgeCases(t *testing.T) {
	clearEnv(t)
	// parseInt-like port parsing: trailing junk is ignored, a value
	// with no leading digits means 0 (TypeScript `|| 0`).
	t.Setenv("WEBSOCKET_PORT", "1234junk")
	if got := LoadEnv().WebsocketPort; got != 1234 {
		t.Errorf("port '1234junk' = %d, want 1234", got)
	}
	t.Setenv("WEBSOCKET_PORT", "notaport")
	if got := LoadEnv().WebsocketPort; got != 0 {
		t.Errorf("port 'notaport' = %d, want 0", got)
	}
	t.Setenv("WEBSOCKET_PORT", "  8080  ")
	if got := LoadEnv().WebsocketPort; got != 8080 {
		t.Errorf("port '  8080  ' = %d, want 8080", got)
	}
	// Only exactly "1" enables boolean flags.
	t.Setenv("DISABLE_PLAYER_BOTS", "true")
	t.Setenv("BROWSER_HOST", "yes")
	env := LoadEnv()
	if env.DisablePlayerBots || env.BrowserHost {
		t.Errorf("flags for 'true'/'yes': DisablePlayerBots=%v BrowserHost=%v, want false/false", env.DisablePlayerBots, env.BrowserHost)
	}
	// An invalid PUBLIC_HOST falls back to the default.
	t.Setenv("PUBLIC_HOST", "https://example.com:8080/x")
	if got := LoadEnv().PublicHost; got != "127.0.0.1" {
		t.Errorf("PublicHost for URL value = %q, want default", got)
	}
	t.Setenv("PUBLIC_HOST", "  host.local  ")
	if got := LoadEnv().PublicHost; got != "host.local" {
		t.Errorf("PublicHost with whitespace = %q, want trimmed", got)
	}
	// A relative save path is resolved against the working directory.
	t.Setenv("PLAYER_SAVE_DATABASE_PATH", "saves/db.sqlite")
	cwd, _ := os.Getwd()
	if want := filepath.Join(cwd, "saves", "db.sqlite"); LoadEnv().PlayerSaveDatabasePath != want {
		t.Errorf("PlayerSaveDatabasePath = %q, want %q", LoadEnv().PlayerSaveDatabasePath, want)
	}
}

func TestConstants(t *testing.T) {
	if TickRateMs != 600 || MaxPlayers != 2048 || MaxNPCs != 65534 ||
		MaxLocalPlayers != 255 || MaxLocalNPCs != 255 ||
		MaxNewLocalPlayersPerCycle != 25 || QueueDrainCap != 150 ||
		MaxFrameBytes != 65536 || PacketQueueLimit != 240 ||
		RunEnergyMaxUnits != 10000 {
		t.Errorf("constants drifted: tick=%d players=%d npcs=%d localP=%d localN=%d new=%d drain=%d frame=%d queue=%d energy=%d",
			TickRateMs, MaxPlayers, MaxNPCs, MaxLocalPlayers, MaxLocalNPCs,
			MaxNewLocalPlayersPerCycle, QueueDrainCap, MaxFrameBytes,
			PacketQueueLimit, RunEnergyMaxUnits)
	}
}

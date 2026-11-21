package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeKeysFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "keys.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadKeys(t *testing.T) {
	path := writeKeysFile(t, `{"1234":[1,2,3,4],"5678":[0,0,0,0],"90":[-1,-2147483648,2147483647,42]}`)
	keys, err := LoadKeys(path)
	if err != nil {
		t.Fatalf("LoadKeys: %v", err)
	}
	if got := keys.Get(1234); got != [4]int32{1, 2, 3, 4} {
		t.Errorf("Get(1234) = %v", got)
	}
	if got := keys.Get(5678); got != [4]int32{} {
		t.Errorf("Get(5678) = %v", got)
	}
	if got := keys.Get(90); got != [4]int32{-1, -2147483648, 2147483647, 42} {
		t.Errorf("Get(90) = %v", got)
	}
	// Absent regions return the zero key.
	if got := keys.Get(9999); got != [4]int32{} {
		t.Errorf("Get(absent) = %v", got)
	}
}

func TestLoadKeysUnsignedRange(t *testing.T) {
	// 4294967295 is 2^32-1: it wraps to -1, as JavaScript bitwise
	// conversion would.
	path := writeKeysFile(t, `{"1":[4294967295,4294967294,0,1]}`)
	keys, err := LoadKeys(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := keys.Get(1); got != [4]int32{-1, -2, 0, 1} {
		t.Errorf("Get(1) = %v", got)
	}
}

func TestLoadKeysErrors(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"not an object", `[1,2,3]`, "must be an object"},
		{"bad region id", `{"abc":[1,2,3,4]}`, "invalid region id"},
		{"short key", `{"1":[1,2,3]}`, "must be an array of 4 integers"},
		{"long key", `{"1":[1,2,3,4,5]}`, "must be an array of 4 integers"},
		{"non-number word", `{"1":[1,2,"x",4]}`, "is not a number"},
		{"fractional word", `{"1":[1,2,3.5,4]}`, "is not an integer"},
		{"out of range word", `{"1":[1,2,3,4294967296]}`, "outside the 32-bit range"},
		{"malformed json", `{oops`, "parsing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadKeys(writeKeysFile(t, tc.content))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want substring %q", err, tc.want)
			}
		})
	}
	if _, err := LoadKeys(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("missing file loaded, want error")
	}
	// Empty object is valid.
	keys, err := LoadKeys(writeKeysFile(t, `{}`))
	if err != nil || len(keys) != 0 {
		t.Errorf("empty keys: %v, %v", keys, err)
	}
}

func TestKeysGetNil(t *testing.T) {
	var keys Keys
	if got := keys.Get(1); got != [4]int32{} {
		t.Errorf("nil Keys Get = %v", got)
	}
}

func TestParseTarget(t *testing.T) {
	target, err := ParseTarget("osrs-242_2026-11-03")
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "osrs-242_2026-11-03" || target.Revision != 242 || target.Date != "2026-11-03" {
		t.Errorf("target = %+v", target)
	}
	// Surrounding whitespace (including the trailing newline in
	// target.txt) is trimmed.
	target, err = ParseTarget("  osrs-241_2025-01-15\n")
	if err != nil || target.Name != "osrs-241_2025-01-15" || target.Revision != 241 {
		t.Errorf("trimmed target = %+v, %v", target, err)
	}
	bad := []string{
		"",
		"osrs-242",
		"osrs_2026-11-03",
		"rs2-242_2026-11-03",
		"osrs-242_2026-11-3",
		"osrs-242_2026/11/03",
		"osrs-242_2026-13-01", // impossible month
		"osrs-242_2026-02-30", // impossible day
		"osrs-abc_2026-11-03",
		"osrs-242_2026-11-03-extra",
		"osrs-99999999999999999999999_2026-11-03",
	}
	for _, s := range bad {
		if _, err := ParseTarget(s); err == nil {
			t.Errorf("ParseTarget(%q) succeeded, want error", s)
		}
	}
}

func TestValidateCacheDir(t *testing.T) {
	valid := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		for _, name := range requiredCacheFiles {
			size := 10
			if name == MetaFileName {
				size = 150 // must exceed 24*6 = 144
			}
			if err := os.WriteFile(filepath.Join(dir, name), make([]byte, size), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	if err := ValidateCacheDir(valid(t)); err != nil {
		t.Errorf("valid dir: %v", err)
	}
	// Each missing file fails, naming the file.
	for _, missing := range requiredCacheFiles {
		dir := valid(t)
		if err := os.Remove(filepath.Join(dir, missing)); err != nil {
			t.Fatal(err)
		}
		err := ValidateCacheDir(dir)
		if err == nil || !strings.Contains(err.Error(), missing) {
			t.Errorf("missing %s: err = %v", missing, err)
		}
	}
	// idx255 at exactly the threshold fails; one byte over passes.
	dir := valid(t)
	if err := os.WriteFile(filepath.Join(dir, MetaFileName), make([]byte, 144), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCacheDir(dir); err == nil {
		t.Error("idx255 of exactly 144 bytes validated, want error")
	}
	if err := os.WriteFile(filepath.Join(dir, MetaFileName), make([]byte, 145), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCacheDir(dir); err != nil {
		t.Errorf("idx255 of 145 bytes: %v", err)
	}
	// A directory in place of a required file fails.
	dir = valid(t)
	if err := os.Remove(filepath.Join(dir, "keys.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "keys.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCacheDir(dir); err == nil {
		t.Error("directory as keys.json validated, want error")
	}
	// Nonexistent directory fails.
	if err := ValidateCacheDir(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("missing directory validated, want error")
	}
}

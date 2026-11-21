package cache

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupCacheRoot creates a root with target.txt and a valid cache
// directory, and returns the root and cache directory.
func setupCacheRoot(t *testing.T, targetName, infoJSON, keysJSON string) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte(targetName+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir = filepath.Join(root, "caches", targetName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, size := range map[string]int{Dat2FileName: 10, MetaFileName: 150} {
		if err := os.WriteFile(filepath.Join(dir, name), make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "info.json"), []byte(infoJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "keys.json"), []byte(keysJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, dir
}

func TestEnsureCacheValidPresent(t *testing.T) {
	root, dir := setupCacheRoot(t, "osrs-242_2026-11-03",
		`{"game":"oldschool","environment":"live","timestamp":"2026-11-03T00:00:00.000Z","size":123}`,
		`{"1234":[1,2,3,4]}`)
	active, err := EnsureCache(root)
	if err != nil {
		t.Fatalf("EnsureCache: %v", err)
	}
	if active.Name != "osrs-242_2026-11-03" || active.Revision != 242 || active.Directory != dir ||
		active.Timestamp != "2026-11-03T00:00:00.000Z" {
		t.Errorf("active = %+v", active)
	}
	// The manifest is written with the CachePipeline field order and
	// no extra whitespace.
	manifest, err := os.ReadFile(filepath.Join(root, "caches", "caches.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"name":"osrs-242_2026-11-03","game":"oldschool","environment":"live","revision":242,"timestamp":"2026-11-03T00:00:00.000Z","size":123}]`
	if string(manifest) != want {
		t.Errorf("manifest = %s\nwant    %s", manifest, want)
	}
	// A second run leaves the manifest untouched (no download, no rewrite).
	info1, _ := os.Stat(filepath.Join(root, "caches", "caches.json"))
	if _, err := EnsureCache(root); err != nil {
		t.Fatal(err)
	}
	info2, _ := os.Stat(filepath.Join(root, "caches", "caches.json"))
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Error("second EnsureCache rewrote an identical manifest")
	}
	// A stale manifest is replaced.
	if err := os.WriteFile(filepath.Join(root, "caches", "caches.json"), []byte(`[]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureCache(root); err != nil {
		t.Fatal(err)
	}
	manifest, _ = os.ReadFile(filepath.Join(root, "caches", "caches.json"))
	if string(manifest) != want {
		t.Errorf("replaced manifest = %s", manifest)
	}
}

func TestEnsureCacheInfoDefaults(t *testing.T) {
	// info.json with no usable fields: game/environment/size default,
	// and the timestamp falls back to the target date.
	root, _ := setupCacheRoot(t, "osrs-242_2026-11-03", `{}`, `{}`)
	active, err := EnsureCache(root)
	if err != nil {
		t.Fatal(err)
	}
	if active.Timestamp != "2026-11-03" {
		t.Errorf("Timestamp = %q, want target date", active.Timestamp)
	}
	manifest, _ := os.ReadFile(filepath.Join(root, "caches", "caches.json"))
	want := `[{"name":"osrs-242_2026-11-03","game":"oldschool","environment":"live","revision":242,"timestamp":"2026-11-03","size":0}]`
	if string(manifest) != want {
		t.Errorf("manifest = %s", manifest)
	}
}

func TestEnsureCacheErrors(t *testing.T) {
	// Missing target.txt.
	if _, err := EnsureCache(t.TempDir()); err == nil || !strings.Contains(err.Error(), "target.txt") {
		t.Errorf("missing target.txt: err = %v", err)
	}
	// Invalid target.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte("bogus"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureCache(root); err == nil || !strings.Contains(err.Error(), "invalid cache target") {
		t.Errorf("invalid target: err = %v", err)
	}
	// Malformed keys.json in an otherwise valid cache is fatal
	// (CachePipeline.initialize parses it eagerly).
	root, _ = setupCacheRoot(t, "osrs-242_2026-11-03", `{}`, `{"1":[1,2]}`)
	if _, err := EnsureCache(root); err == nil {
		t.Error("malformed keys.json accepted, want error")
	}
	// Malformed info.json is fatal.
	root, _ = setupCacheRoot(t, "osrs-242_2026-11-03", `{bad`, `{}`)
	if _, err := EnsureCache(root); err == nil {
		t.Error("malformed info.json accepted, want error")
	}
}

func TestEnsureCacheBrowserHost(t *testing.T) {
	t.Setenv("BROWSER_HOST", "1")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte("osrs-242_2026-11-03"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := EnsureCache(root)
	if err == nil || err.Error() != "Browser-host cache is missing or incomplete; restart the server to reload it" {
		t.Errorf("err = %v", err)
	}
}

func TestSelectOpenRS2Entry(t *testing.T) {
	entries := []openRS2Entry{
		{ID: 1, Scope: "runescape", Game: "oldschool", Language: "en", Builds: []openRS2Build{{Major: 242}}, Timestamp: "2026-11-03T10:00:00.000Z"},
		{ID: 2, Scope: "runescape", Game: "oldschool", Language: "en", Builds: []openRS2Build{{Major: 242}}, Timestamp: "2026-11-04T10:00:00.000Z"},
		{ID: 3, Scope: "runescape", Game: "oldschool", Language: "en", Builds: []openRS2Build{{Major: 241}}, Timestamp: "2026-11-05T10:00:00.000Z"},
		{ID: 4, Scope: "runescape", Game: "oldschool", Language: "de", Builds: []openRS2Build{{Major: 242}}, Timestamp: "2026-11-06T10:00:00.000Z"},
		{ID: 5, Scope: "other", Game: "oldschool", Language: "en", Builds: []openRS2Build{{Major: 242}}, Timestamp: "2026-11-06T10:00:00.000Z"},
		{ID: 6, Scope: "runescape", Game: "oldschool", Language: "en", Builds: nil, Timestamp: "2026-11-06T10:00:00.000Z"},
	}
	// Prefer the entry matching the target date, even though a newer
	// one exists.
	entry, ok := selectOpenRS2Entry(entries, 242, "2026-11-03")
	if !ok || entry.ID != 1 {
		t.Errorf("date match: entry = %+v, %v", entry, ok)
	}
	// No date match: newest for the revision.
	entry, ok = selectOpenRS2Entry(entries, 242, "2026-01-01")
	if !ok || entry.ID != 2 {
		t.Errorf("newest fallback: entry = %+v, %v", entry, ok)
	}
	// No candidates for the revision.
	if _, ok := selectOpenRS2Entry(entries, 999, "2026-11-03"); ok {
		t.Error("unexpected entry for unknown revision")
	}
}

func TestRewriteOpenRS2Keys(t *testing.T) {
	keys, err := rewriteOpenRS2Keys([]byte(`[{"group":1234,"key":[1,2,3,4]},{"group":5,"key":[0,0,0,0]}]`))
	if err != nil {
		t.Fatal(err)
	}
	if keys.Get(1234) != [4]int32{1, 2, 3, 4} || len(keys) != 2 {
		t.Errorf("keys = %v", keys)
	}
	// Entries that cannot be valid keys are dropped.
	keys, err = rewriteOpenRS2Keys([]byte(`[{"group":1,"key":[1,2,3]},{"group":2,"key":[1,2,3,4]},{"group":3,"key":[1,2,3,4294967296]}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys.Get(2) != [4]int32{1, 2, 3, 4} {
		t.Errorf("keys = %v", keys)
	}
	// A nil body (failed keys request) yields an empty map.
	keys, err = rewriteOpenRS2Keys(nil)
	if err != nil || len(keys) != 0 {
		t.Errorf("nil body: %v, %v", keys, err)
	}
	if _, err := rewriteOpenRS2Keys([]byte(`{bad`)); err == nil {
		t.Error("malformed keys body parsed, want error")
	}
}

func TestExtractCacheZip(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "disk.zip")
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"cache/main_file_cache.dat2": "dat2",
		"cache/nested/file.txt":      "nested",
		"cache/":                     "",
		"other/skip.txt":             "skip",
		"top.txt":                    "skip",
	} {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := extractCacheZip(zipPath, dest); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dest, "main_file_cache.dat2")); err != nil || string(data) != "dat2" {
		t.Errorf("dat2 = %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(dest, "nested", "file.txt")); err != nil || string(data) != "nested" {
		t.Errorf("nested = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "other")); !os.IsNotExist(err) {
		t.Error("non-cache entry was extracted")
	}
	if _, err := os.Stat(filepath.Join(dest, "top.txt")); !os.IsNotExist(err) {
		t.Error("top-level entry was extracted")
	}
}

func TestExtractCacheZipUnsafePath(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "evil.zip")
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("cache/../../evil.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("evil"))
	w.Close()
	os.WriteFile(zipPath, buf.Bytes(), 0o644)
	if err := extractCacheZip(zipPath, t.TempDir()); err == nil || !strings.Contains(err.Error(), "unsafe entry path") {
		t.Errorf("err = %v", err)
	}
}

func TestAcquireDownloadLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".cache-download.lock")
	// Free lock: acquire succeeds and writes the pid.
	owns, err := acquireDownloadLock(lockPath, dir)
	if err != nil || !owns {
		t.Fatalf("owns = %v, err = %v", owns, err)
	}
	content, _ := os.ReadFile(lockPath)
	if string(content) == "" {
		t.Error("lock file is empty, want pid")
	}
	os.Remove(lockPath)
	// Stale lock (older than 10 minutes) is removed and replaced.
	if err := os.WriteFile(lockPath, []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-11 * time.Minute)
	if err := os.Chtimes(lockPath, stale, stale); err != nil {
		t.Fatal(err)
	}
	owns, err = acquireDownloadLock(lockPath, dir)
	if err != nil || !owns {
		t.Fatalf("stale lock: owns = %v, err = %v", owns, err)
	}
	os.Remove(lockPath)
}

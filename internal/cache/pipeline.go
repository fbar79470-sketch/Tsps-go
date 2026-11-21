package cache

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// OpenRS2 archive base URL (OPENRS2 in CachePipeline.ts:8).
const openRS2BaseURL = "https://archive.openrs2.org"

// requiredCacheFiles must all exist in a valid cache directory
// (REQUIRED_FILES in CachePipeline.ts:9-14).
var requiredCacheFiles = []string{Dat2FileName, MetaFileName, "info.json", "keys.json"}

// ValidateCacheDir checks that dir holds a usable cache: every
// required file exists, and the meta index reaches past the gamevals
// index, which guards against truncated or stale caches. It mirrors
// CachePipeline.isValid (CachePipeline.ts:104-108): idx255's size
// must exceed GAMEVAL_INDEX * SectorCluster.SIZE (24 * 6 = 144
// bytes). Unlike the boolean isValid, it reports which check failed.
func ValidateCacheDir(dir string) error {
	for _, file := range requiredCacheFiles {
		info, err := os.Stat(filepath.Join(dir, file))
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("cache directory %s is missing required file %s", dir, file)
			}
			return fmt.Errorf("cache directory %s: checking %s: %w", dir, file, err)
		}
		if info.IsDir() {
			return fmt.Errorf("cache directory %s: required file %s is a directory", dir, file)
		}
	}
	info, err := os.Stat(filepath.Join(dir, MetaFileName))
	if err != nil {
		return fmt.Errorf("cache directory %s: checking %s: %w", dir, MetaFileName, err)
	}
	if min := int64(GamevalIndex * SectorClusterSize); info.Size() <= min {
		return fmt.Errorf("cache directory %s: %s size %d does not exceed %d bytes (truncated or stale cache)",
			dir, MetaFileName, info.Size(), min)
	}
	return nil
}

// ActiveCache describes the cache a server run uses. It mirrors the
// ActiveCache type in CachePipeline.ts:27-32.
type ActiveCache struct {
	Name      string
	Revision  int
	Directory string
	Timestamp string
}

// EnsureCache makes the target cache from <root>/target.txt available
// and returns it. It mirrors CachePipeline.initialize
// (CachePipeline.ts:66-102):
//
//  1. Read and parse target.txt; the cache lives at
//     <root>/caches/<target-name>.
//  2. If that directory is not a valid cache (see ValidateCacheDir)
//     and BROWSER_HOST=1, return an error: a browser-hosted server
//     cannot download the cache. Otherwise download it from the
//     OpenRS2 archive (see downloadCache).
//  3. Load info.json and keys.json (validating the latter), and
//     write the <root>/caches/caches.json manifest when it is
//     missing or differs.
//
// Callers that need XTEA keys load them separately with LoadKeys on
// ActiveCache.Directory; the TypeScript version keeps them in a
// module-level map, which this package deliberately does not
// replicate (no global mutable state).
//
// The download half is factored into downloadCache and the
// openRS2Downloader type so it can be tested without the network;
// unit tests in this package never touch the network.
func EnsureCache(root string) (*ActiveCache, error) {
	return ensureCache(root, newOpenRS2Downloader(openRS2BaseURL))
}

func ensureCache(root string, downloader *openRS2Downloader) (*ActiveCache, error) {
	rawTarget, err := os.ReadFile(filepath.Join(root, "target.txt"))
	if err != nil {
		return nil, fmt.Errorf("reading target.txt in %s: %w", root, err)
	}
	target, err := ParseTarget(string(rawTarget))
	if err != nil {
		return nil, fmt.Errorf("target.txt: %w", err)
	}
	cachesDir := filepath.Join(root, "caches")
	if err := os.MkdirAll(cachesDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating caches directory %s: %w", cachesDir, err)
	}
	directory := filepath.Join(cachesDir, target.Name)
	if err := ValidateCacheDir(directory); err != nil {
		if os.Getenv("BROWSER_HOST") == "1" {
			return nil, errors.New("Browser-host cache is missing or incomplete; restart the server to reload it")
		}
		if err := downloadCache(cachesDir, directory, target, downloader); err != nil {
			return nil, err
		}
	}

	info, err := readCacheInfo(filepath.Join(directory, "info.json"))
	if err != nil {
		return nil, err
	}
	// A missing (or empty) timestamp falls back to the target date,
	// as `info.timestamp ?? date` does in CachePipeline.initialize.
	if info.timestamp == "" {
		info.timestamp = target.Date
	}
	if _, err := LoadKeys(filepath.Join(directory, "keys.json")); err != nil {
		return nil, err
	}
	if err := writeCacheManifest(cachesDir, target, info); err != nil {
		return nil, err
	}
	return &ActiveCache{
		Name:      target.Name,
		Revision:  target.Revision,
		Directory: directory,
		Timestamp: info.timestamp,
	}, nil
}

// cacheInfo is the subset of an OpenRS2 entry's info.json that the
// manifest and ActiveCache need. Missing or null fields fall back to
// the CachePipeline.initialize defaults (CachePipeline.ts:87-100).
type cacheInfo struct {
	game        string
	environment string
	timestamp   string
	size        int64
}

func readCacheInfo(path string) (cacheInfo, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return cacheInfo{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var info map[string]any
	if err := json.Unmarshal(raw, &info); err != nil {
		return cacheInfo{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	out := cacheInfo{game: "oldschool", environment: "live", size: 0}
	if s, ok := info["game"].(string); ok {
		out.game = s
	}
	if s, ok := info["environment"].(string); ok {
		out.environment = s
	}
	if s, ok := info["timestamp"].(string); ok {
		out.timestamp = s
	}
	if f, ok := info["size"].(float64); ok {
		out.size = int64(f)
	}
	return out, nil
}

// cacheManifestEntry is one entry of caches/caches.json, with fields
// in the order CachePipeline.initialize writes them.
type cacheManifestEntry struct {
	Name        string `json:"name"`
	Game        string `json:"game"`
	Environment string `json:"environment"`
	Revision    int    `json:"revision"`
	Timestamp   string `json:"timestamp"`
	Size        int64  `json:"size"`
}

func writeCacheManifest(cachesDir string, target Target, info cacheInfo) error {
	timestamp := info.timestamp
	if timestamp == "" {
		timestamp = target.Date
	}
	manifest, err := json.Marshal([]cacheManifestEntry{{
		Name:        target.Name,
		Game:        info.game,
		Environment: info.environment,
		Revision:    target.Revision,
		Timestamp:   timestamp,
		Size:        info.size,
	}})
	if err != nil {
		return fmt.Errorf("writing cache manifest: %w", err)
	}
	manifestPath := filepath.Join(cachesDir, "caches.json")
	existing, err := os.ReadFile(manifestPath)
	if err == nil && bytes.Equal(existing, manifest) {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", manifestPath, err)
	}
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", manifestPath, err)
	}
	return nil
}

// --- OpenRS2 download (CachePipeline.download, CachePipeline.ts:110-188) ---

// lockStaleAfter is how old a download lock may be before another
// process may remove it: 10 minutes (CachePipeline.ts:121,128).
const lockStaleAfter = 10 * time.Minute

// lockPollInterval is how often a process waiting on the download
// lock re-checks it: 1 second (CachePipeline.ts:132).
const lockPollInterval = time.Second

// openRS2Downloader fetches from an OpenRS2-compatible archive. The
// base URL is injectable so the download flow can be exercised
// against a local server; production uses openRS2BaseURL. The HTTP
// client has no timeout, matching fetch's behavior in the TypeScript
// version.
type openRS2Downloader struct {
	client  *http.Client
	baseURL string
}

func newOpenRS2Downloader(baseURL string) *openRS2Downloader {
	return &openRS2Downloader{client: &http.Client{}, baseURL: strings.TrimRight(baseURL, "/")}
}

// openRS2Entry is one entry of the archive's caches.json, with the
// raw JSON preserved so info.json can be written verbatim. It mirrors
// the OpenRs2Cache type in CachePipeline.ts:16-25.
type openRS2Entry struct {
	ID          int
	Scope       string
	Game        string
	Environment string
	Language    string
	Builds      []openRS2Build
	Timestamp   string
	Size        int64
	raw         json.RawMessage
}

type openRS2Build struct {
	Major int `json:"major"`
}

// selectOpenRS2Entry picks the cache entry for a target revision and
// date from the archive index: scope=runescape, game=oldschool,
// language=en, builds[0].major == revision, newest first, preferring
// the entry whose timestamp starts with the target date and otherwise
// the newest (CachePipeline.ts:142-152). The pre-start script
// (scripts/ensure-cache.ts) differs here: it requires an exact date
// match. This follows the runtime CachePipeline behavior. Pure and
// network-free, for testing.
func selectOpenRS2Entry(entries []openRS2Entry, revision int, date string) (openRS2Entry, bool) {
	var candidates []openRS2Entry
	for _, entry := range entries {
		if entry.Scope != "runescape" || entry.Game != "oldschool" || entry.Language != "en" {
			continue
		}
		if len(entry.Builds) == 0 || entry.Builds[0].Major != revision {
			continue
		}
		candidates = append(candidates, entry)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return parseCacheTimestamp(candidates[i].Timestamp).After(parseCacheTimestamp(candidates[j].Timestamp))
	})
	for _, entry := range candidates {
		if strings.HasPrefix(entry.Timestamp, date) {
			return entry, true
		}
	}
	if len(candidates) > 0 {
		return candidates[0], true
	}
	return openRS2Entry{}, false
}

func parseCacheTimestamp(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// fetchEntries downloads and parses the archive's caches.json.
func (d *openRS2Downloader) fetchEntries() ([]openRS2Entry, error) {
	resp, err := d.client.Get(d.baseURL + "/caches.json")
	if err != nil {
		return nil, fmt.Errorf("fetching OpenRS2 cache index: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("OpenRS2 cache index returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fetching OpenRS2 cache index: %w", err)
	}
	var rawEntries []json.RawMessage
	if err := json.Unmarshal(body, &rawEntries); err != nil {
		return nil, fmt.Errorf("parsing OpenRS2 cache index: %w", err)
	}
	entries := make([]openRS2Entry, 0, len(rawEntries))
	for _, raw := range rawEntries {
		var decoded struct {
			ID          int            `json:"id"`
			Scope       string         `json:"scope"`
			Game        string         `json:"game"`
			Environment string         `json:"environment"`
			Language    string         `json:"language"`
			Builds      []openRS2Build `json:"builds"`
			Timestamp   string         `json:"timestamp"`
			Size        int64          `json:"size"`
		}
		// A malformed entry is skipped, as the TypeScript filter
		// would skip an entry whose fields are undefined.
		if err := json.Unmarshal(raw, &decoded); err != nil {
			continue
		}
		entries = append(entries, openRS2Entry{
			ID:          decoded.ID,
			Scope:       decoded.Scope,
			Game:        decoded.Game,
			Environment: decoded.Environment,
			Language:    decoded.Language,
			Builds:      decoded.Builds,
			Timestamp:   decoded.Timestamp,
			Size:        decoded.Size,
			raw:         raw,
		})
	}
	return entries, nil
}

// downloadCache downloads one cache into directory, following
// CachePipeline.download: an exclusive lock file with 10-minute
// staleness, parallel disk.zip + keys.json fetches, extraction of the
// zip's cache/ subtree into a temporary directory, keys.json
// rewriting, validation, and an atomic rename over the target
// directory.
func downloadCache(cachesDir, directory string, target Target, downloader *openRS2Downloader) error {
	lockPath := filepath.Join(cachesDir, ".cache-download.lock")
	ownsLock, err := acquireDownloadLock(lockPath, directory)
	if err != nil {
		return err
	}
	if !ownsLock {
		// Another process finished the download while we waited.
		return nil
	}
	defer os.Remove(lockPath)

	entries, err := downloader.fetchEntries()
	if err != nil {
		return err
	}
	entry, ok := selectOpenRS2Entry(entries, target.Revision, target.Date)
	if !ok {
		return fmt.Errorf("OpenRS2 has no oldschool cache for revision %d", target.Revision)
	}

	pid := os.Getpid()
	temporary := filepath.Join(cachesDir, fmt.Sprintf(".%s-%d", target.Name, pid))
	if err := os.RemoveAll(temporary); err != nil {
		return fmt.Errorf("preparing cache staging directory: %w", err)
	}
	if err := os.MkdirAll(temporary, 0o755); err != nil {
		return fmt.Errorf("preparing cache staging directory: %w", err)
	}
	defer os.RemoveAll(temporary)

	zipPath := filepath.Join(cachesDir, fmt.Sprintf(".%s-%d.zip", target.Name, pid))
	defer os.Remove(zipPath)
	keysBody, err := downloadCacheFiles(downloader, entry, zipPath)
	if err != nil {
		return err
	}
	if err := extractCacheZip(zipPath, temporary); err != nil {
		return err
	}
	keys, err := rewriteOpenRS2Keys(keysBody)
	if err != nil {
		return err
	}
	keysJSON, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("rewriting XTEA keys: %w", err)
	}
	if err := os.WriteFile(filepath.Join(temporary, "keys.json"), keysJSON, 0o644); err != nil {
		return fmt.Errorf("writing staged keys.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(temporary, "info.json"), entry.raw, 0o644); err != nil {
		return fmt.Errorf("writing staged info.json: %w", err)
	}
	if err := ValidateCacheDir(temporary); err != nil {
		return fmt.Errorf("downloaded cache failed validation: %w", err)
	}
	if err := os.RemoveAll(directory); err != nil {
		return fmt.Errorf("replacing cache directory: %w", err)
	}
	if err := os.Rename(temporary, directory); err != nil {
		return fmt.Errorf("replacing cache directory: %w", err)
	}
	return nil
}

// acquireDownloadLock implements the CachePipeline.download lock
// protocol. It returns (true, nil) when this process owns the lock,
// (false, nil) when another process completed the download while this
// one waited, or an error. A lock older than 10 minutes is treated as
// stale and removed. One deliberate deviation: if the lock file
// vanishes between the existence check and the stat (a race the
// TypeScript version would throw on), it is treated as released
// instead of failing the download.
func acquireDownloadLock(lockPath, directory string) (bool, error) {
	tryAcquire := func() bool {
		if info, err := os.Stat(lockPath); err == nil && time.Since(info.ModTime()) > lockStaleAfter {
			os.Remove(lockPath)
		}
		f, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return false
		}
		fmt.Fprint(f, os.Getpid())
		f.Close()
		return true
	}
	if tryAcquire() {
		return true, nil
	}
	for {
		info, err := os.Stat(lockPath)
		if err != nil {
			if os.IsNotExist(err) {
				break
			}
			return false, fmt.Errorf("waiting for cache download lock: %w", err)
		}
		if time.Since(info.ModTime()) > lockStaleAfter {
			os.Remove(lockPath)
			break
		}
		time.Sleep(lockPollInterval)
	}
	if err := ValidateCacheDir(directory); err == nil {
		return false, nil
	}
	if !tryAcquire() {
		return false, fmt.Errorf("failed to acquire cache download lock %s after waiting", lockPath)
	}
	return true, nil
}

// downloadCacheFiles fetches disk.zip and keys.json in parallel (as
// CachePipeline.download's Promise.all does). The zip is streamed to
// zipPath; the keys body is returned, or nil when the keys request
// did not succeed (the TypeScript version writes an empty key map in
// that case). A failed zip download, or a keys fetch that fails at
// the transport level, is an error.
func downloadCacheFiles(downloader *openRS2Downloader, entry openRS2Entry, zipPath string) ([]byte, error) {
	var wg sync.WaitGroup
	var zipErr, keysErr error
	var keysBody []byte
	var keysOK bool
	wg.Add(2)
	go func() {
		defer wg.Done()
		zipErr = downloadToFile(downloader, fmt.Sprintf("%s/caches/%s/%d/disk.zip", downloader.baseURL, entry.Scope, entry.ID), zipPath, "OpenRS2 disk archive")
	}()
	go func() {
		defer wg.Done()
		keysBody, keysOK, keysErr = downloadKeys(downloader, fmt.Sprintf("%s/caches/%s/%d/keys.json", downloader.baseURL, entry.Scope, entry.ID))
	}()
	wg.Wait()
	if zipErr != nil {
		return nil, zipErr
	}
	if keysErr != nil {
		return nil, keysErr
	}
	if !keysOK {
		return nil, nil
	}
	return keysBody, nil
}

func downloadToFile(downloader *openRS2Downloader, url, destPath, label string) error {
	resp, err := downloader.client.Get(url)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", label, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s returned HTTP %d", label, resp.StatusCode)
	}
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", label, err)
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(destPath)
		return fmt.Errorf("downloading %s: %w", label, copyErr)
	}
	if closeErr != nil {
		os.Remove(destPath)
		return fmt.Errorf("downloading %s: %w", label, closeErr)
	}
	return nil
}

func downloadKeys(downloader *openRS2Downloader, url string) ([]byte, bool, error) {
	resp, err := downloader.client.Get(url)
	if err != nil {
		return nil, false, fmt.Errorf("fetching OpenRS2 XTEA keys: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, false, nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("fetching OpenRS2 XTEA keys: %w", err)
	}
	return body, true, nil
}

// rewriteOpenRS2Keys converts the OpenRS2 keys.json array form
// ([{group, key}]) into the cache's map form ({"<regionId>": [k0..k3]}),
// as CachePipeline.download does (CachePipeline.ts:170-176). Entries
// whose key is not exactly four 32-bit integers are dropped: they
// could never form a valid XTEA key (see XTEAValidKey), and keeping
// them verbatim, as the TypeScript version does, would make the
// rewritten file fail LoadKeys validation. A nil body (keys request
// failed) yields an empty map.
func rewriteOpenRS2Keys(body []byte) (Keys, error) {
	keys := Keys{}
	if body == nil {
		return keys, nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var entries []struct {
		Group int           `json:"group"`
		Key   []json.Number `json:"key"`
	}
	if err := dec.Decode(&entries); err != nil {
		return nil, fmt.Errorf("parsing OpenRS2 XTEA keys: %w", err)
	}
	for _, entry := range entries {
		if len(entry.Key) != 4 {
			continue
		}
		var key [4]int32
		valid := true
		for i, word := range entry.Key {
			w, err := jsonNumberToInt32(word)
			if err != nil {
				valid = false
				break
			}
			key[i] = w
		}
		if valid {
			keys[entry.Group] = key
		}
	}
	return keys, nil
}

// extractCacheZip extracts the "cache/" subtree of a zip archive into
// dest, stripping the prefix — the equivalent of adm-zip's
// extractEntryTo("cache/", temporary, false, true) in
// CachePipeline.download. Entries outside cache/ are skipped. Entry
// paths that would escape dest (absolute paths or ".." elements) are
// rejected: adm-zip sanitizes these itself, and Go's archive/zip does
// not.
func extractCacheZip(zipPath, dest string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("opening cache archive: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, "cache/") {
			continue
		}
		rest := strings.TrimPrefix(file.Name, "cache/")
		if rest == "" || strings.HasSuffix(rest, "/") {
			continue
		}
		if !isSafeZipPath(rest) {
			return fmt.Errorf("opening cache archive: unsafe entry path %q", file.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(rest))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("extracting cache archive: %w", err)
		}
		if err := extractZipEntry(file, target); err != nil {
			return err
		}
	}
	return nil
}

func isSafeZipPath(rest string) bool {
	if filepath.IsAbs(rest) || strings.HasPrefix(rest, "/") {
		return false
	}
	for _, elem := range strings.Split(rest, "/") {
		if elem == ".." {
			return false
		}
	}
	// filepath.Join also cleans the result; confirm the cleaned path
	// stays inside the destination.
	rel, err := filepath.Rel(".", filepath.Join(".", filepath.FromSlash(rest)))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func extractZipEntry(file *zip.File, target string) error {
	src, err := file.Open()
	if err != nil {
		return fmt.Errorf("extracting cache archive entry %q: %w", file.Name, err)
	}
	defer src.Close()
	dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("extracting cache archive entry %q: %w", file.Name, err)
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return fmt.Errorf("extracting cache archive entry %q: %w", file.Name, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("extracting cache archive entry %q: %w", file.Name, closeErr)
	}
	return nil
}

package definition

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureDir writes files into a fresh temp directory and returns
// its path.
func fixtureDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// minimalFiles is the smallest fixture set Load accepts: every
// required file present and valid, every optional file absent.
func minimalFiles() map[string]string {
	return map[string]string{
		"interface-layouts.json": `{}`,
		"object-spawns.json":     `[]`,
		"ground-items.json":      `[]`,
		"npc-combat-defs.json":   `{}`,
		"npc-animations.json":    `{}`,
		"shops.json":             `[]`,
		"custom-shops.json":      `[]`,
		"npc-spawns.json":        `[]`,
		"npc-drops.json":         `{"tables": {}, "npcs": {}}`,
	}
}

// withFiles overlays extra files onto the minimal set.
func withFiles(extra map[string]string) map[string]string {
	files := minimalFiles()
	for name, content := range extra {
		files[name] = content
	}
	return files
}

// withoutFiles removes files from the minimal set.
func withoutFiles(names ...string) map[string]string {
	files := minimalFiles()
	for _, name := range names {
		delete(files, name)
	}
	return files
}

// logCapture collects Logf output for assertions.
type logCapture struct {
	lines []string
}

func (c *logCapture) logf(format string, args ...any) {
	c.lines = append(c.lines, fmt.Sprintf(format, args...))
}

func (c *logCapture) joined() string { return strings.Join(c.lines, "\n") }

func (c *logCapture) contains(sub string) bool { return strings.Contains(c.joined(), sub) }

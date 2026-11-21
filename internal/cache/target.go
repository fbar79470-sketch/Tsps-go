package cache

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Target is a cache target from target.txt: a name of the form
// osrs-{revision}_{date}, split into its parts. It mirrors the
// {revision, date} parseCacheTarget returns in CachePipeline.ts:34-40,
// plus the target name itself (used as the cache directory name).
type Target struct {
	Name     string
	Date     string // YYYY-MM-DD, a real calendar date
	Revision int
}

var targetPattern = regexp.MustCompile(`^osrs-(\d+)_(\d{4}-\d{2}-\d{2})$`)

// ParseTarget parses a cache target string such as
// "osrs-242_2026-11-03". Surrounding whitespace is trimmed, as
// CachePipeline.initialize trims target.txt. The format must match
// osrs-{revision}_{YYYY-MM-DD} exactly, and the date must be a real
// calendar date. (The TypeScript regex only checks the digit pattern;
// rejecting impossible dates like 2026-02-30 is a deliberate
// tightening, since no OpenRS2 entry could ever match them.)
func ParseTarget(s string) (Target, error) {
	trimmed := strings.TrimSpace(s)
	match := targetPattern.FindStringSubmatch(trimmed)
	if match == nil {
		return Target{}, fmt.Errorf("invalid cache target %q; expected osrs-{revision}_{date}", s)
	}
	revision, err := strconv.Atoi(match[1])
	if err != nil {
		return Target{}, fmt.Errorf("invalid cache target %q: revision %q is not a number", s, match[1])
	}
	if _, err := time.Parse("2006-01-02", match[2]); err != nil {
		return Target{}, fmt.Errorf("invalid cache target %q: date %q is not a real calendar date", s, match[2])
	}
	return Target{Name: trimmed, Date: match[2], Revision: revision}, nil
}

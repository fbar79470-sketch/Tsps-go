package cache

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
)

// Keys maps cache region ids to their 128-bit XTEA keys. It mirrors
// the Map<number, number[]> CachePipeline builds from keys.json
// (CachePipeline.ts:83-85).
type Keys map[int][4]int32

// LoadKeys reads a keys.json file: a JSON object mapping region ids
// (as strings) to arrays of exactly four integers. Every entry is
// validated; a malformed entry returns an error naming the region id.
// Key words are signed 32-bit values; a word given in the unsigned
// range (up to 2^32-1) is converted the way JavaScript's bitwise
// operations would convert it, since XTEA arithmetic is mod 2^32.
func LoadKeys(path string) (Keys, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parsing %s: keys.json must be an object mapping region ids to keys", path)
	}
	keys := make(Keys, len(obj))
	for region, entry := range obj {
		regionID, err := strconv.Atoi(region)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: invalid region id %q", path, region)
		}
		words, ok := entry.([]any)
		if !ok || len(words) != 4 {
			return nil, fmt.Errorf("parsing %s: key for region %d must be an array of 4 integers", path, regionID)
		}
		var key [4]int32
		for i, word := range words {
			num, ok := word.(json.Number)
			if !ok {
				return nil, fmt.Errorf("parsing %s: key for region %d: element %d is not a number", path, regionID, i)
			}
			w, err := jsonNumberToInt32(num)
			if err != nil {
				return nil, fmt.Errorf("parsing %s: key for region %d: element %d: %w", path, regionID, i, err)
			}
			key[i] = w
		}
		keys[regionID] = key
	}
	return keys, nil
}

// jsonNumberToInt32 converts a JSON number to an int32 word, accepting
// integral values in [-2^31, 2^32) and wrapping the unsigned half,
// matching JavaScript ToInt32 semantics for XTEA key words.
func jsonNumberToInt32(num json.Number) (int32, error) {
	if i, err := num.Int64(); err == nil {
		if i < math.MinInt32 || i > math.MaxUint32 {
			return 0, fmt.Errorf("value %d is outside the 32-bit range", i)
		}
		return int32(i), nil
	}
	f, err := num.Float64()
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || math.Trunc(f) != f {
		return 0, fmt.Errorf("value %q is not an integer", num.String())
	}
	if f < math.MinInt32 || f > math.MaxUint32 {
		return 0, fmt.Errorf("value %v is outside the 32-bit range", f)
	}
	return int32(int64(f)), nil
}

// Get returns the key for a region, or the all-zero key when the
// region has none. It mirrors CachePipeline.getXtea
// (CachePipeline.ts:62-64). Get is safe on a nil Keys map.
func (k Keys) Get(regionID int) [4]int32 {
	if k == nil {
		return [4]int32{}
	}
	return k[regionID]
}

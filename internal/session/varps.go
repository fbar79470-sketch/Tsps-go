package session

import "sync"

// The live per-session varp/varbit store, porting the varp half of
// PacketSender (net/packet/PacketSender.ts): the varps map (:221),
// getVarp (:340-342), getVarbit (:344-348), sendConfig (:355-359),
// sendVarbit (:1523-1531), and replayVarps (:336-338). The store is
// session-only: PlayerSave.ts has no varp/varc fields (its only
// varp-keyed record is audioSettings, the client-settings map the
// game layer persists separately), so nothing here is ever saved.
//
// Concurrency: TS is single-threaded, but Go calls the sender from
// the login goroutine (hooks) and the tick goroutine, so the store
// guards itself with its own mutex and never relies on Session.mu.
// A resolver must be a pure lookup: it runs under the store mutex,
// so calling back into the store from one would deadlock. The
// coordinator's resolver over CacheDefinitions.GetVarbit satisfies
// that (the cache loader takes only its own lock).
//
// Construction: Worker 1 wires `&varpStore{values:
// make(map[int]int)}` into NewSession (see session.go). The type
// name and that construction form are pinned by the Phase 18 plan.
// The zero value is also usable: a nil map reads 0 and is allocated
// on the first set. A varpStore must not be copied after use (it
// holds a mutex); sessions always hold it by pointer.

// VarbitResolver resolves a varbit id to its cache definition: the
// backing varp, the first bit, and the last bit, as VarBitType
// decodes them (opcode 1: baseVar u16, startBit u8, endBit u8).
// ok is false when no definition exists for the id. The coordinator
// installs a resolver over CacheDefinitions.GetVarbit (see
// SetVarbitResolver); with no resolver installed, every varbit
// reads as undefined, matching the unknown-definition behavior
// below. The bit range follows the TS convention: endBit is the
// last bit INCLUDED, so the field is endBit-startBit+1 bits wide
// (BIT_MASKS[endBit-startBit] is the mask for that many bits).
type VarbitResolver func(id int) (baseVar, startBit, endBit int, ok bool)

// varpStore is the per-session live varp cache, porting the
// PacketSender.varps map (PacketSender.ts:221). values holds the
// last value sent for each varp, truncated to int32 exactly as
// sendConfig's `state | 0` truncates; order records first-insertion
// order so replayVarps re-sends entries in the order TS's Map
// iteration would (a re-set key keeps its original position, as a
// JS Map does). resolve supplies varbit definitions; nil means
// none are known.
type varpStore struct {
	mu      sync.Mutex
	values  map[int]int
	order   []int
	resolve VarbitResolver
}

// bitMask returns BIT_MASKS[width] (MathConstants.ts:8-19): the mask
// for width+1 bits, where width is endBit-startBit. Index 31 is -1
// (all 32 bits); any width outside 0..31 has no entry, and TS reads
// `undefined` there, which makes both getVarbit and sendVarbit
// degrade to a no-op on the stored value (see getVarbitLocked /
// applyVarbitLocked).
func bitMask(width int) (int32, bool) {
	if width < 0 || width > 31 {
		return 0, false
	}
	if width == 31 {
		return -1, true
	}
	return (1 << uint(width+1)) - 1, true
}

// get returns the last value recorded for id, or 0 when the varp
// was never set (getVarp, PacketSender.ts:340-342: `?? 0`).
func (st *varpStore) get(id int) int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.getLocked(id)
}

func (st *varpStore) getLocked(id int) int {
	if st.values == nil {
		return 0
	}
	return st.values[id]
}

// set records value for id, truncating to int32 as sendConfig's
// `state | 0` does (PacketSender.ts:356). A first-time id joins the
// replay order; re-setting keeps its position.
func (st *varpStore) set(id, value int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.setLocked(id, value)
}

func (st *varpStore) setLocked(id, value int) {
	if st.values == nil {
		st.values = make(map[int]int)
	}
	if _, ok := st.values[id]; !ok {
		st.order = append(st.order, id)
	}
	st.values[id] = int(int32(value))
}

// setResolver installs the varbit-definition source. It runs at
// AddPlayer time, before the session is shared with the tick
// goroutine (the SetMusicData precedent); the store lock makes a
// later install safe, but installing mid-session would change what
// in-flight getVarbit calls resolve.
func (st *varpStore) setResolver(r VarbitResolver) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.resolve = r
}

// getVarbit returns the varbit's field of its backing varp
// (getVarbit, PacketSender.ts:344-348): the backing varp shifted
// right by startBit (an arithmetic shift, as JS `>>` is) and
// masked to the field width.
//
// Unknown definitions: TS destructures CacheDefinitions.getVarbit
// (id), whose loader ALWAYS returns a VarBitType object — an
// unknown id yields a default-constructed one whose baseVar,
// startBit, and endBit are all `undefined` (VarBitType.ts: the
// definite-assignment fields are set only by opcode 1; TypeLoader
// load() returns the default object when no file exists). The
// destructure therefore never throws: getVarp(undefined) reads 0,
// `0 >> undefined` is 0, BIT_MASKS[NaN] is undefined, and `0 &
// undefined` is 0. So an unknown varbit id reads 0 in TS, and it
// reads 0 here (ok false, or a field width outside BIT_MASKS).
// The Go facade mirrors the loader shape: GetVarbit returns a
// *VarBitType that is never nil, and Defined() reports whether
// opcode 1 set its fields — the coordinator's resolver maps
// !Defined() to ok=false.
func (st *varpStore) getVarbit(id int) int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.getVarbitLocked(id)
}

func (st *varpStore) getVarbitLocked(id int) int {
	if st.resolve == nil {
		return 0
	}
	baseVar, startBit, endBit, ok := st.resolve(id)
	if !ok {
		return 0
	}
	mask, ok := bitMask(endBit - startBit)
	if !ok {
		return 0
	}
	// JS `>>` masks its shift count to 5 bits; mirror that for a
	// pathological definition rather than shifting by 32+.
	return int((int32(st.getLocked(baseVar)) >> uint(startBit&31)) & mask)
}

// applyVarbit records a sendVarbit into the backing varp
// (PacketSender.ts:1523-1531): read-modify-write packing. The
// value is clamped to 0 when negative or wider than the field
// (unless the field is the full 32 bits, mask -1, which takes any
// non-negative value); the field's bits in the backing varp are
// replaced and every other bit is preserved. The caller always
// sends the encodeVarbit frame with the ORIGINAL value, as TS
// does — this method only updates the record.
//
// Unknown definitions: TS sets varps[undefined] (the destructured
// baseVar), a key no numeric getVarp can ever observe, and still
// sends the frame. The observable record is therefore unchanged,
// and this method records nothing for an unknown id or a field
// width outside BIT_MASKS (where TS's shifted mask is 0 and the
// read-modify-write is an identity). The frame itself is the
// caller's job and is unaffected.
func (st *varpStore) applyVarbit(id, value int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.applyVarbitLocked(id, value)
}

func (st *varpStore) applyVarbitLocked(id, value int) {
	if st.resolve == nil {
		return
	}
	baseVar, startBit, endBit, ok := st.resolve(id)
	if !ok {
		return
	}
	mask, ok := bitMask(endBit - startBit)
	if !ok {
		return
	}
	// Match the client's varbit range handling and shared backing
	// varp (the TS comment at PacketSender.ts:1526). The range
	// comparison is numeric, as in JS: value is compared before
	// any int32 truncation.
	stored := int32(value)
	if value < 0 || (mask != -1 && int64(value) > int64(mask)) {
		stored = 0
	}
	shift := uint(startBit & 31)
	field := mask << shift
	current := int32(st.getLocked(baseVar))
	st.setLocked(baseVar, int((current&^field)|((stored<<shift)&field)))
}

// entries returns every recorded varp in first-insertion order,
// for replayVarps. The snapshot is taken under the lock; the
// caller sends without holding it.
func (st *varpStore) entries() [][2]int {
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.order) == 0 {
		return nil
	}
	out := make([][2]int, 0, len(st.order))
	for _, id := range st.order {
		out = append(out, [2]int{id, st.values[id]})
	}
	return out
}

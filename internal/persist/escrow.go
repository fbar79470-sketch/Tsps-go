package persist

import (
	"encoding/json"
	"errors"
)

// Trade escrow in snapshots (PlayerSave.ts:782-791, fromPlayer):
// uncompleted trades are cancelled on login, so a snapshot
// includes the player's own offered items in the inventory —
// without moving items in the running world. The fold runs over
// a COPY of the inventory: offered stacks are added to the copy
// (stackable stacks merge, unstackable stacks fan one unit per
// slot), and the copy becomes the snapshot's inventory. When the
// copy cannot hold an offered stack, TS throws and the save
// fails; FoldTradeEscrow returns an error for the caller to
// treat the same way (skip the save).
//
// Nothing else about a trade persists: applyToPlayer has no
// trade handling (no offered container is ever restored), and
// no save field carries trade state. The post-trade save is a
// plain snapshot taken after the transfer (see the save seam
// in internal/game). The coordinator calls FoldTradeEscrow from
// the snapshot builder with the player's offered items and the
// definition stackability read; this package imports no other
// tsps-go package, so both arrive as values.

// maxStackAmount is MAX_STACK_AMOUNT (ItemContainer.ts): the
// 32-bit signed stack cap. Restated here: persist imports no
// other tsps-go package.
const maxStackAmount = 0x7fffffff

// ErrTradeEscrowFull is the fold's failure: the inventory copy
// cannot hold an offered stack (PlayerSave.ts:789 throws
// "Cannot snapshot trade escrow: inventory is full"). The
// caller skips the save, as TS's throw skips it.
var ErrTradeEscrowFull = errors.New("Cannot snapshot trade escrow: inventory is full")

// FoldTradeEscrow folds offered into a copy of inventory and
// returns the copy (PlayerSave.ts:782-791). inventory is the
// snapshot's inventory items (28 slots, id -1 for empty);
// offered are the valid offered stacks (id >= 0, amount > 0),
// in offer-container order; stackable answers definition
// stackability per id (nil reads every id unstackable, the
// DEFAULT definition's answer). The inputs are never mutated:
// the returned slice and every meta map in it are fresh
// copies. An offered stack the copy cannot hold — the
// slot-count pre-check fails — returns ErrTradeEscrowFull and
// no slice.
//
// The per-item rule, exactly as TS: a stackable item joining a
// held stack of the same id and metadata needs 0 slots; a
// stackable item starting a new stack needs 1; an unstackable
// item needs one slot per unit. The pre-check runs before
// each add, against the copy as it stands after the previous
// folds. The add itself mirrors ItemContainer.add: a stackable
// stack merges into the first same-id, same-meta slot (clamped
// to the stack cap, the excess dropped) or fills the first
// empty slot; an unstackable stack fans one-unit clones,
// carrying the offered metadata, into empty slots in order.
// Invalid offered stacks (id < 0 or amount <= 0) are skipped,
// as add's guard skips them.
func FoldTradeEscrow(inventory, offered []Item, stackable func(id int) bool) ([]Item, error) {
	folded := cloneItems(inventory)
	for _, item := range offered {
		if item.ID < 0 || item.Amount <= 0 {
			continue
		}
		stacks := stackable != nil && stackable(item.ID)
		sameStack := stacks && hasSameStack(folded, item)
		var slots int
		switch {
		case sameStack:
			slots = 0
		case stacks:
			slots = 1
		default:
			slots = item.Amount
		}
		if escrowFreeSlots(folded) < slots {
			return nil, ErrTradeEscrowFull
		}
		if err := escrowAdd(folded, item, stacks); err != nil {
			return nil, err
		}
	}
	return folded, nil
}

// escrowFreeSlots counts the copy's free slots: id == -1
// (ItemContainer.getFreeSlots, ItemContainer.ts:173-181).
func escrowFreeSlots(items []Item) int {
	free := 0
	for _, item := range items {
		if item.ID == -1 {
			free++
		}
	}
	return free
}

// escrowEmptySlot returns the first empty slot: id < 0, or
// amount <= 0 in a non-bank container (ItemContainer.
// getEmptySlot, ItemContainer.ts:238-249). -1 when full.
func escrowEmptySlot(items []Item) int {
	for i, item := range items {
		if item.ID < 0 || item.Amount <= 0 {
			return i
		}
	}
	return -1
}

// hasSameStack reports whether items holds a stack of item's
// id with equal metadata (PlayerSave.ts:784-785).
func hasSameStack(items []Item, item Item) bool {
	for _, held := range items {
		if held.ID == item.ID && escrowMetaEqual(held.Meta, item.Meta) {
			return true
		}
	}
	return false
}

// escrowAdd adds item to the copy, mirroring ItemContainer.add
// (ItemContainer.ts:497-581) for a non-bank container: the
// stackable merge-or-fill, or the unstackable fan-out. The
// pre-check in FoldTradeEscrow guarantees room; a missing slot
// here is unreachable, and reports ErrTradeEscrowFull rather
// than panicking.
func escrowAdd(items []Item, item Item, stacks bool) error {
	if stacks {
		for i, held := range items {
			if held.ID == item.ID && escrowMetaEqual(held.Meta, item.Meta) {
				total := held.Amount + item.Amount
				if total > maxStackAmount {
					total = maxStackAmount
				}
				items[i].Amount = total
				items[i].Meta = cloneRawMessageMap(item.Meta)
				return nil
			}
		}
		slot := escrowEmptySlot(items)
		if slot == -1 {
			return ErrTradeEscrowFull
		}
		amount := item.Amount
		if amount > maxStackAmount {
			amount = maxStackAmount
		}
		items[slot] = Item{ID: item.ID, Amount: amount, Meta: cloneRawMessageMap(item.Meta)}
		return nil
	}
	for n := 0; n < item.Amount; n++ {
		slot := escrowEmptySlot(items)
		if slot == -1 {
			return ErrTradeEscrowFull
		}
		items[slot] = Item{ID: item.ID, Amount: 1, Meta: cloneRawMessageMap(item.Meta)}
	}
	return nil
}

// escrowMetaEqual compares metadata as TS does
// (JSON.stringify(meta ?? null) equality, PlayerSave.ts:785),
// with one necessary deviation: TS stringifies in key-insertion
// order, which a Go map does not preserve, so the comparison
// canonicalizes (encoding/json sorts map keys). A nil map
// stringifies as "null", distinct from an empty map ("{}").
// Unmarshalable raw values compare unequal.
func escrowMetaEqual(a, b map[string]json.RawMessage) bool {
	fa, okA := escrowMetaFingerprint(a)
	fb, okB := escrowMetaFingerprint(b)
	return okA && okB && fa == fb
}

// escrowMetaFingerprint renders meta's canonical JSON form: a
// nil map renders as "null".
func escrowMetaFingerprint(meta map[string]json.RawMessage) (string, bool) {
	if meta == nil {
		return "null", true
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return "", false
	}
	return string(raw), true
}

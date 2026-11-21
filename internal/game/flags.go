package game

import (
	"fmt"
	"strings"
)

// Flag names the per-tick update-block flags. Order and names are quoted
// verbatim from game/model/Flag.ts:
//
//	CHAT, FORCED_CHAT, FORCED_MOVEMENT, ENTITY_INTERACTION, FACE_POSITION,
//	APPEARANCE, ANIMATION, GRAPHIC, HIT
//
// TS Flag is an enum starting at 0; the Go constants keep the same numeric
// values so bit positions match the TS declaration order.
type Flag uint8

const (
	FlagChat Flag = iota
	FlagForcedChat
	FlagForcedMovement
	FlagEntityInteraction
	FlagFacePosition
	FlagAppearance
	FlagAnimation
	FlagGraphic
	// FlagHit marks one or more hitsplats this tick; the list itself is
	// the Combat tick hits (combat_death.go AddTickHit/TickHits),
	// cleared by the next cycle's ProcessHitQueue.
	FlagHit
)

// flagNames mirrors the Flag enum declaration order in Flag.ts.
var flagNames = [...]string{
	"CHAT",
	"FORCED_CHAT",
	"FORCED_MOVEMENT",
	"ENTITY_INTERACTION",
	"FACE_POSITION",
	"APPEARANCE",
	"ANIMATION",
	"GRAPHIC",
	"HIT",
}

// String returns the TS enum name, or "FLAG(n)" for out-of-range values.
func (f Flag) String() string {
	if int(f) < len(flagNames) {
		return flagNames[f]
	}
	return fmt.Sprintf("FLAG(%d)", int(f))
}

// Flags is a set of update flags, stored as a bitset. It ports
// game/model/UpdateFlag.ts, which wraps Set<Flag> with flagged/flag/reset/
// isUpdateRequired. The bitset keeps the same semantics with no allocation.
type Flags uint32

// Has reports whether flag is set. UpdateFlag.ts: flagged().
func (f Flags) Has(flag Flag) bool { return f&(1<<flag) != 0 }

// Set returns f with flag added. UpdateFlag.ts: flag().
func (f Flags) Set(flag Flag) Flags { return f | (1 << flag) }

// Clear returns f with flag removed.
func (f Flags) Clear(flag Flag) Flags { return f &^ (1 << flag) }

// Reset returns the empty set. UpdateFlag.ts: reset().
func (f Flags) Reset() Flags { return 0 }

// IsUpdateRequired reports whether any flag is set.
// UpdateFlag.ts: isUpdateRequired().
func (f Flags) IsUpdateRequired() bool { return f != 0 }

// String lists the set flags in declaration order, comma-separated, for
// debugging; the empty set renders as "{}".
func (f Flags) String() string {
	if f == 0 {
		return "{}"
	}
	var names []string
	for i := Flag(0); int(i) < len(flagNames); i++ {
		if f.Has(i) {
			names = append(names, flagNames[i])
		}
	}
	return "{" + strings.Join(names, ",") + "}"
}

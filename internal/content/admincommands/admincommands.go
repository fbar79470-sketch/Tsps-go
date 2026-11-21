// Package admincommands is the Go port of the AdminCommands plugin
// (server/plugins/commands/AdminCommands.plugin.js), the staff-command
// plugin. This package starts with the first command the Go port
// registers — ::item — and is the home later admin commands
// (::give, ::clearinv, ...) port into; the registration pattern
// established here (one Plugin value, one Register function calling
// api.RegisterCommand per command) is the one they follow.
//
// The ::item port is faithful to AdminCommands.plugin.js:1623-1635:
// default amount 1, amount capped at Number.MAX_SAFE_INTEGER, invalid
// args print "Usage: ::item id [amount]", success prints "Spawned
// item {id} x{amount}.", rights gate ADMINISTRATOR. The id is NOT
// validated against item definitions in this command (unlike the
// ::items search UI, which checks CacheDefinitions.hasItem): an
// unknown id goes through the inventory's add exactly like any other
// id — non-stackable per the definition source's DEFAULT shape, one
// item per slot, at most 28, then "You couldn't hold all those
// items." — and the success message still prints, because TS prints
// it unconditionally after adds.
//
// One deliberate deviation, forced by the language change: JS
// parseInt returns a float, so ::item 99999999999999999999 gives TS
// an id of 1e20 and prints it back as 100000000000000000000. Go has
// no float ids, so parseIntArg saturates at math.MaxInt64 and the
// same input prints 9223372036854775807. The divergence is observable
// only for ids >= 2^63 — no real item id is near that range — and
// the inventory behaviour (non-stackable, one per slot) is identical
// either way. Pinned by TestParseIntArgSaturates.
package admincommands

import (
	"fmt"
	"math"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// maxSafeInteger is Number.MAX_SAFE_INTEGER (2^53 - 1): the amount
// cap in the upstream command (AdminCommands.plugin.js:1631).
const maxSafeInteger = 9007199254740991

// parseIntWhitespace is the leading-whitespace set JS parseInt skips
// (String trim's WhiteSpace + LineTerminator, matching the cutset the
// command parser uses in internal/game/commands.go).
const parseIntWhitespace = " \t\n\v\f\r\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"

// parseIntArg ports parseIntArg (AdminCommands.plugin.js:172-175):
// Number.parseInt(value, 10), NaN → invalid. Leading whitespace is
// skipped, then an optional sign, then leading ASCII decimal digits
// ("12abc" → 12, "0x10" → 0, "1.5" → 1, "1e3" → 1, "+7" → 7, "-5" →
// -5); no digits at all → invalid. The value accumulates into int64
// and saturates at math.MaxInt64 / math.MinInt64 on overflow — JS
// keeps a float here (see the package comment for the one observable
// consequence, ids >= 2^63).
func parseIntArg(value string) (int64, bool) {
	s := strings.TrimLeft(value, parseIntWhitespace)
	if s == "" {
		return 0, false
	}
	negative := false
	if s[0] == '+' || s[0] == '-' {
		negative = s[0] == '-'
		s = s[1:]
	}
	if s == "" || s[0] < '0' || s[0] > '9' {
		return 0, false
	}
	var magnitude uint64
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		digit := uint64(s[i] - '0')
		if magnitude > (math.MaxUint64-digit)/10 {
			// Further digits cannot change the saturated result.
			if negative {
				return math.MinInt64, true
			}
			return math.MaxInt64, true
		}
		magnitude = magnitude*10 + digit
	}
	if negative {
		if magnitude > uint64(math.MaxInt64)+1 {
			return math.MinInt64, true
		}
		if magnitude == uint64(math.MaxInt64)+1 {
			return math.MinInt64, true
		}
		return -int64(magnitude), true
	}
	if magnitude > uint64(math.MaxInt64) {
		return math.MaxInt64, true
	}
	return int64(magnitude), true
}

// itemCommand is the ::item handler
// (AdminCommands.plugin.js:1623-1635). Parts is the parsed command:
// Parts[0] is the base name, Parts[1] the id, Parts[2] the optional
// amount; extra parts are ignored, as in TS.
func itemCommand(e *game.CommandEvent) bool {
	if e == nil || e.Player == nil {
		return false
	}
	var id int64
	var ok bool
	if len(e.Parts) > 1 {
		id, ok = parseIntArg(e.Parts[1])
	}
	if !ok {
		e.Player.SendMessage("Usage: ::item id [amount]")
		return true
	}
	amount := int64(1)
	if len(e.Parts) > 2 {
		var amountOk bool
		amount, amountOk = parseIntArg(e.Parts[2])
		if !amountOk {
			e.Player.SendMessage("Usage: ::item id [amount]")
			return true
		}
	}
	if id < 0 || amount <= 0 {
		e.Player.SendMessage("Usage: ::item id [amount]")
		return true
	}
	cappedAmount := amount
	if cappedAmount > maxSafeInteger {
		cappedAmount = maxSafeInteger
	}
	e.Player.Inventory().Adds(int(id), int(cappedAmount))
	e.Player.SendMessage(fmt.Sprintf("Spawned item %d x%d.", id, cappedAmount))
	return true
}

// Plugin is the registered AdminCommands plugin value: name and
// description from the plugin file (AdminCommands.plugin.js:969-970).
// Only ::item is registered so far; later admin commands register
// in this same Register function.
var Plugin = plugins.Plugin{
	Name:        "AdminCommands",
	Description: "Staff commands: teleporting, items and spawns, levels, kicks, mutes and bans, reloading data, and logging and debug tools.",
	Register: func(api *plugins.Api) {
		api.RegisterCommand("item", itemCommand, game.RightsAdministrator, "Spawn an item")
	},
}

func init() {
	plugins.Register(Plugin)
}

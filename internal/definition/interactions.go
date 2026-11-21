package definition

import (
	"regexp"
	"strings"
)

// NpcInteractionAction is one click action: the method to invoke,
// its arguments, and the source that supplied it
// (NpcInteractionActionDefinition, NpcInteractionDefinition.ts:1-4).
// Args holds decoded JSON values (numbers are float64) except for
// the synthesized shorthand actions, whose ids and coordinates are
// ints.
type NpcInteractionAction struct {
	Method string
	Args   map[string]any
	Source string
}

// NpcInteractionDefinition is the five click actions for one NPC
// (NpcInteractionDefinition.ts:5-76). A nil action means the NPC
// has no defined action for that click.
type NpcInteractionDefinition struct {
	NpcID       int
	FirstClick  *NpcInteractionAction
	SecondClick *NpcInteractionAction
	ThirdClick  *NpcInteractionAction
	FourthClick *NpcInteractionAction
	FifthClick  *NpcInteractionAction
}

// Action returns the action for click type 1..5, or nil for any
// other click type or an undefined action (getAction,
// NpcInteractionDefinition.ts:136-153).
func (d *NpcInteractionDefinition) Action(clickType int) *NpcInteractionAction {
	switch clickType {
	case 1:
		return d.FirstClick
	case 2:
		return d.SecondClick
	case 3:
		return d.ThirdClick
	case 4:
		return d.FourthClick
	case 5:
		return d.FifthClick
	default:
		return nil
	}
}

// Interaction returns the interaction definition for an NPC id,
// or false when none was loaded (forNpcId,
// NpcInteractionDefinition.ts:124-126).
func (s *Store) Interaction(npcID int) (*NpcInteractionDefinition, bool) {
	def, ok := s.interactions[npcID]
	return def, ok
}

// Interactions returns all loaded interaction definitions in
// first-seen order (the TS Map's insertion order).
func (s *Store) Interactions() []*NpcInteractionDefinition {
	return append([]*NpcInteractionDefinition(nil), s.interactionList...)
}

// loadInteractions ports NpcInteractionDefinitionLoader.load()
// (NpcInteractionDefinitionLoader.ts:32-86). Plugins supply every
// definition, so there is no file: the loader merges contributed
// sources only, per NPC id, per click slot. Later sources
// overwrite earlier ones per slot, but only when the later action
// normalizes successfully — a bad action never clears a good one.
func (s *Store) loadInteractions() error {
	sources, failures := s.registry.loadSources(npcInteractionsType, s.logf)
	merged := make(map[int]*NpcInteractionDefinition)
	var order []int
	candidates, invalid := 0, 0
	for _, source := range sources {
		candidates += len(source.Definitions)
		for _, raw := range source.Definitions {
			npcIDNum := numField(raw, "npcId")
			if !isJSInteger(npcIDNum) || jsTrunc(npcIDNum) < 0 {
				invalid++
				continue
			}
			npcID := jsTrunc(npcIDNum)
			def, exists := merged[npcID]
			if !exists {
				def = &NpcInteractionDefinition{NpcID: npcID}
			}
			mergeInteractionAction(raw, "firstClick", "first_click", source.Owner, def, 1)
			mergeInteractionAction(raw, "secondClick", "second_click", source.Owner, def, 2)
			mergeInteractionAction(raw, "thirdClick", "third_click", source.Owner, def, 3)
			mergeInteractionAction(raw, "fourthClick", "fourth_click", source.Owner, def, 4)
			mergeInteractionAction(raw, "fifthClick", "fifth_click", source.Owner, def, 5)
			if def.FirstClick != nil || def.SecondClick != nil || def.ThirdClick != nil || def.FourthClick != nil || def.FifthClick != nil {
				if !exists {
					merged[npcID] = def
					order = append(order, npcID)
				}
			} else {
				// A fresh entry that yields no action is
				// invalid and discarded; a pre-existing
				// entry always has an action, so this
				// branch only fires for fresh entries —
				// exactly the TS invalid++ path (line 63).
				invalid++
			}
		}
	}
	s.interactions = merged
	for _, npcID := range order {
		s.interactionList = append(s.interactionList, merged[npcID])
	}
	s.logf("[npc-interactions] Loaded %d definitions from %s (candidates=%d, invalid=%d)",
		len(order), joinSourceNames(sources), candidates, invalid)
	_ = failures // TS returns failures == 0; GameBuilder ignores it.
	return nil
}

// mergeInteractionAction is mergeAction
// (NpcInteractionDefinitionLoader.ts:93-118): when neither the
// camelCase nor the snake_case property is present, the slot is
// untouched; when both are present the camelCase value wins; a
// value that fails to normalize leaves the slot untouched.
func mergeInteractionAction(raw any, camel, snake, source string, def *NpcInteractionDefinition, clickType int) {
	_, hasCamel := field(raw, camel)
	_, hasSnake := field(raw, snake)
	if !hasCamel && !hasSnake {
		return
	}
	var value any
	if hasCamel {
		value, _ = field(raw, camel)
	} else {
		value, _ = field(raw, snake)
	}
	action := normalizeInteractionAction(value, source)
	if action != nil {
		setInteractionAction(def, clickType, action)
	}
}

func setInteractionAction(def *NpcInteractionDefinition, clickType int, action *NpcInteractionAction) {
	switch clickType {
	case 1:
		def.FirstClick = action
	case 2:
		def.SecondClick = action
	case 3:
		def.ThirdClick = action
	case 4:
		def.FourthClick = action
	case 5:
		def.FifthClick = action
	}
}

// interactionMethodPattern is the explicit-method validation
// (NpcInteractionDefinitionLoader.ts:132): a lowercase
// namespace, then one or more dotted identifiers.
var interactionMethodPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(?:\.[A-Za-z_$][A-Za-z0-9_$]*)+$`)

// normalizeInteractionAction is normalizeAction
// (NpcInteractionDefinitionLoader.ts:120-174). An explicit method
// wins when present and well-formed (its args must be an object
// or absent); otherwise the shop/teleport/dialogue shorthands
// apply in that priority order. The dialogue shorthand produces a
// core.dialogues.start action reference, as TS does — it is not
// dialogue data loading (see the package comment).
func normalizeInteractionAction(value any, source string) *NpcInteractionAction {
	raw, ok := value.(map[string]any)
	if !ok || value == nil {
		return nil
	}
	method := ""
	if m, present := field(raw, "method"); present {
		if text, ok := m.(string); ok {
			method = strings.TrimSpace(text)
		}
	}
	if method != "" {
		if !interactionMethodPattern.MatchString(method) {
			return nil
		}
		argsValue, present := field(raw, "args")
		if present && argsValue != nil {
			argsObj, ok := argsValue.(map[string]any)
			if !ok {
				return nil
			}
			args := make(map[string]any, len(argsObj))
			for k, v := range argsObj {
				args[k] = v
			}
			return &NpcInteractionAction{Method: method, Args: args, Source: source}
		}
		return &NpcInteractionAction{Method: method, Args: make(map[string]any), Source: source}
	}
	shopID := optionalInteractionID(raw, "shopId", "shop_id")
	teleport := interactionLocation(raw)
	dialogueID := optionalInteractionID(raw, "dialogueId", "dialogue_id")
	// Priority in TS is shop, then teleport, then dialogue; the
	// dialogue id is read before the teleport location, but the
	// checks run shop/teleport/dialogue (lines 150-172).
	if shopID != nil {
		return &NpcInteractionAction{
			Method: "core.shops.open",
			Args:   map[string]any{"shopId": *shopID},
			Source: source,
		}
	}
	if teleport != nil {
		return &NpcInteractionAction{
			Method: "core.player.teleport",
			Args: map[string]any{"destination": map[string]any{
				"x": teleport[0], "y": teleport[1], "z": teleport[2],
			}},
			Source: source,
		}
	}
	if dialogueID != nil {
		return &NpcInteractionAction{
			Method: "core.dialogues.start",
			Args:   map[string]any{"dialogueId": *dialogueID},
			Source: source,
		}
	}
	return nil
}

// optionalInteractionID is optionalId
// (NpcInteractionDefinitionLoader.ts:176-180): Number.isInteger
// and >= 0, with NO coercion — a string id does not count. The
// first of the two spellings that is not null/undefined wins
// (the ?? chain).
func optionalInteractionID(raw map[string]any, camel, snake string) *int {
	v, present := field(raw, camel)
	if !present || v == nil {
		v, present = field(raw, snake)
		if !present || v == nil {
			return nil
		}
	}
	n, ok := numberValue(v)
	if !ok || !isJSInteger(n) || n < 0 {
		return nil
	}
	id := jsTrunc(n)
	return &id
}

// interactionLocation is location
// (NpcInteractionDefinitionLoader.ts:182-194): the
// teleportLocation/teleport_location object, with x/y/z coerced
// by Number() (z defaulting to 0 when null/undefined) and
// truncated; all three must be finite.
func interactionLocation(raw map[string]any) *[3]int {
	v, present := field(raw, "teleportLocation")
	if !present || v == nil {
		v, present = field(raw, "teleport_location")
		if !present || v == nil {
			return nil
		}
	}
	loc, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	x := numField(loc, "x")
	y := numField(loc, "y")
	z := 0.0
	if zv, present := field(loc, "z"); present && zv != nil {
		z = jsNumber(zv, true)
	}
	if !isFiniteNumber(x) || !isFiniteNumber(y) || !isFiniteNumber(z) {
		return nil
	}
	return &[3]int{jsTrunc(x), jsTrunc(y), jsTrunc(z)}
}

// Dialogue builders: DialogueBuilder, DialogueChainBuilder, and
// DynamicDialogueBuilder
// (dialogues/builders/DialogueBuilder.ts, DialogueChainBuilder.ts,
// DynamicDialogueBuilder.ts), plus the port of the only static dialogue,
// TestStaticDialogue (builders/impl/TestStaticDialogue.ts).
//
// TS DialogueBuilder is an abstract class holding a Map<number,
// Dialogue> keyed by each entry's own index (DialogueBuilder.ts:9-14:
// add() sets dialogues[dialogue.getIndex()] = dialogue). Indices are
// NOT dense or ordered: the map keys are whatever indices the entries
// carry, duplicates overwrite (last add wins), and the manager walks
// index+1, ending the chain at the first missing key. Go mirrors this
// with DialogueBuilder as an interface over GetDialogues: the map is
// the contract, and any builder shape (chain, dynamic, hand-built)
// satisfies it. GetDialogues returns the live map, as TS does — the
// manager copies entries out of it at start time (see
// DialogueManager.startDialogueMap), so later builder mutation never
// affects a running chain.
//
// DEVIATION (Go has no optional constructor parameters, and no class
// inheritance): TS DialogueChainBuilder is an empty subclass used only
// for instantiation, inheriting the variadic add(...dialogues). The Go
// DialogueChainBuilder is a concrete struct whose Add takes ONE
// dialogue and returns the builder for chaining; multi-entry chains
// chain Add calls. DynamicDialogueBuilder is an interface (DialogueBuilder
// + Build) rather than an abstract class; a dynamic builder embeds or
// wraps a DialogueChainBuilder for storage. The manager detects dynamic
// builders by interface assertion, the Go form of the TS
// `instanceof DynamicDialogueBuilder` check (DialogueManager.ts:120).

package game

// DialogueBuilder yields the indexed dialogue map a chain starts from
// (DialogueBuilder.getDialogues, DialogueBuilder.ts:16-18).
type DialogueBuilder interface {
	// GetDialogues returns the live index -> entry map. The manager
	// copies it at start time; mutating it afterwards does not
	// affect a running chain.
	GetDialogues() map[int]Dialogue
}

// DialogueChainBuilder is the concrete ordered builder
// (DialogueChainBuilder.ts:3-5: an empty DialogueBuilder subclass,
// "used for instantiating"). Entries are keyed by their own index,
// exactly as TS add() keys them; adding two entries with the same
// index keeps the last, as a Map.set does.
type DialogueChainBuilder struct {
	dialogues map[int]Dialogue
}

// NewDialogueChainBuilder returns an empty chain builder.
func NewDialogueChainBuilder() *DialogueChainBuilder {
	return &DialogueChainBuilder{dialogues: make(map[int]Dialogue)}
}

// Add appends dialogue to the chain, keyed by its index
// (DialogueBuilder.add, DialogueBuilder.ts:9-14), and returns the
// builder so calls chain. A nil dialogue is stored like any other
// entry: TS add() would throw on dialogue.getIndex() for null, but a
// nil Dialogue here panics on GetIndex the same way — callers must
// not add nil entries.
func (b *DialogueChainBuilder) Add(d Dialogue) *DialogueChainBuilder {
	b.dialogues[d.GetIndex()] = d
	return b
}

// GetDialogues returns the live index -> entry map
// (DialogueBuilder.getDialogues, DialogueBuilder.ts:16-18).
func (b *DialogueChainBuilder) GetDialogues() map[int]Dialogue {
	return b.dialogues
}

// DynamicDialogueBuilder is a builder built per player at start time
// (DynamicDialogueBuilder.ts:4-6: abstract build(player)). The manager
// calls Build with the player before reading the map, so Build is
// where per-player entries are added.
type DynamicDialogueBuilder interface {
	DialogueBuilder
	// Build populates the builder for player. It runs once, at
	// start time, before the manager reads the map.
	Build(p *Player)
}

// NewTestStaticDialogue builds the test static dialogue
// (TestStaticDialogue.ts:5-14): two player lines, then an NPC line
// from NPC 6797. It is the only STATIC_DIALOGUES entry
// (DialogueManager.ts:17-19) and exists in TS as a smoke check that
// the static-dialogue path works; the Go port keeps it for the same
// purpose.
func NewTestStaticDialogue() *DialogueChainBuilder {
	return NewDialogueChainBuilder().
		Add(NewPlayerDialogue(0, "Well this works just fine.")).
		Add(NewPlayerDialogue(1, "Second test")).
		Add(NewNpcDialogue(2, 6797, "okay great."))
}

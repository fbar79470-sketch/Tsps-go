package game

import (
	"reflect"
	"testing"
)

func TestDialogueChainBuilderKeying(t *testing.T) {
	b := NewDialogueChainBuilder()
	if len(b.GetDialogues()) != 0 {
		t.Error("fresh builder is not empty")
	}
	first := NewPlayerDialogue(0, "First.")
	second := NewPlayerDialogue(5, "Fifth.")
	// Add chains and keys by each entry's own index (sparse is fine).
	got := b.Add(first).Add(second)
	if got != b {
		t.Error("Add did not return the builder")
	}
	dialogues := b.GetDialogues()
	if len(dialogues) != 2 {
		t.Fatalf("builder holds %d entries, want 2", len(dialogues))
	}
	if dialogues[0] != Dialogue(first) || dialogues[5] != Dialogue(second) {
		t.Error("entries not keyed by their indices")
	}
	// A duplicate index overwrites, as Map.set does.
	replacement := NewStatementDialogue(0, "Replacement.")
	b.Add(replacement)
	if b.GetDialogues()[0] != Dialogue(replacement) {
		t.Error("duplicate index did not overwrite")
	}
	// GetDialogues returns the live map, as TS does.
	dialogues[9] = NewEndDialogue(9)
	if b.GetDialogues()[9] == nil {
		t.Error("GetDialogues did not return the live map")
	}
}

func TestDialogueChainBuilderSatisfiesDialogueBuilder(t *testing.T) {
	var _ DialogueBuilder = NewDialogueChainBuilder()
	var _ DynamicDialogueBuilder = &dynamicTestBuilder{}
}

func TestTestStaticDialogue(t *testing.T) {
	b := NewTestStaticDialogue()
	dialogues := b.GetDialogues()
	if len(dialogues) != 3 {
		t.Fatalf("static dialogue has %d entries, want 3", len(dialogues))
	}
	first, ok := dialogues[0].(*PlayerDialogue)
	if !ok || first.GetText() != "Well this works just fine." {
		t.Errorf("entry 0 = %T %v", dialogues[0], dialogues[0])
	}
	second, ok := dialogues[1].(*PlayerDialogue)
	if !ok || second.GetText() != "Second test" {
		t.Errorf("entry 1 = %T %v", dialogues[1], dialogues[1])
	}
	third, ok := dialogues[2].(*NpcDialogue)
	if !ok || third.GetNpcID() != 6797 || third.GetText() != "okay great." {
		t.Errorf("entry 2 = %T %v", dialogues[2], dialogues[2])
	}
	// Each build is fresh: mutating one builder's map must not affect
	// the next (the registry factory calls Create on every start).
	b.GetDialogues()[0] = NewEndDialogue(0)
	if _, ok := NewTestStaticDialogue().GetDialogues()[0].(*PlayerDialogue); !ok {
		t.Error("a fresh static build inherited a mutation")
	}
}

func TestStaticDialoguesRegistry(t *testing.T) {
	def, ok := StaticDialogues[DialogueIdentifierTest]
	if !ok {
		t.Fatal("TEST static dialogue missing from the registry")
	}
	if def.StartIndex != 0 {
		t.Errorf("TEST start index = %d, want 0", def.StartIndex)
	}
	if DialogueIdentifierTest != 0 {
		t.Errorf("DialogueIdentifierTest = %d, want 0", DialogueIdentifierTest)
	}
	built := def.Create().GetDialogues()
	if len(built) != 3 {
		t.Fatalf("registry factory built %d entries, want 3", len(built))
	}
	if !reflect.DeepEqual(built[1].(*PlayerDialogue).GetText(), "Second test") {
		t.Error("registry factory built the wrong dialogue")
	}
}

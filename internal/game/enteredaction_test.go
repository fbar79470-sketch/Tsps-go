package game

import "testing"

func TestEnteredAmountActionGetSet(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3200, Y: 3200})
	if got := p.GetEnteredAmountAction(); got != nil {
		t.Fatalf("GetEnteredAmountAction() = %v, want nil", got)
	}
	action := &EnteredAmountAction{
		AcceptsZero: true,
		Execute:     func(amount int) {},
		Cancel:      func() {},
	}
	p.SetEnteredAmountAction(action)
	if got := p.GetEnteredAmountAction(); got != action {
		t.Fatalf("GetEnteredAmountAction() = %v, want the armed action", got)
	}
	p.SetEnteredAmountAction(nil)
	if got := p.GetEnteredAmountAction(); got != nil {
		t.Fatalf("GetEnteredAmountAction() after clear = %v, want nil", got)
	}
}

func TestEnteredSyntaxActionGetSet(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3200, Y: 3200})
	if got := p.GetEnteredSyntaxAction(); got != nil {
		t.Fatalf("GetEnteredSyntaxAction() = %v, want nil", got)
	}
	action := &EnteredSyntaxAction{Execute: func(syntax string) {}}
	p.SetEnteredSyntaxAction(action)
	if got := p.GetEnteredSyntaxAction(); got != action {
		t.Fatalf("GetEnteredSyntaxAction() = %v, want the armed action", got)
	}
	p.SetEnteredSyntaxAction(nil)
	if got := p.GetEnteredSyntaxAction(); got != nil {
		t.Fatalf("GetEnteredSyntaxAction() after clear = %v, want nil", got)
	}
}

func TestHasInputPrompt(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3200, Y: 3200})
	if p.HasInputPrompt() {
		t.Fatal("HasInputPrompt() = true with nothing armed")
	}
	p.SetEnteredAmountAction(&EnteredAmountAction{Execute: func(int) {}})
	if !p.HasInputPrompt() {
		t.Fatal("HasInputPrompt() = false with an amount action armed")
	}
	p.SetEnteredAmountAction(nil)
	p.SetEnteredSyntaxAction(&EnteredSyntaxAction{Execute: func(string) {}})
	if !p.HasInputPrompt() {
		t.Fatal("HasInputPrompt() = false with a syntax action armed")
	}
	var nilPlayer *Player
	if nilPlayer.HasInputPrompt() {
		t.Fatal("nil HasInputPrompt() = true")
	}
}

func TestAbortEnteredActionsClearsBothAndCancels(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3200, Y: 3200})
	cancelled := false
	p.SetEnteredAmountAction(&EnteredAmountAction{
		Execute: func(int) {},
		Cancel:  func() { cancelled = true },
	})
	p.SetEnteredSyntaxAction(&EnteredSyntaxAction{Execute: func(string) {}})
	p.AbortEnteredActions()
	if p.GetEnteredAmountAction() != nil || p.GetEnteredSyntaxAction() != nil {
		t.Fatal("holders not cleared by AbortEnteredActions")
	}
	if !cancelled {
		t.Fatal("amount action Cancel did not run")
	}
	// Aborting with nothing armed, and a nil Cancel, is safe.
	p.AbortEnteredActions()
	p.SetEnteredAmountAction(&EnteredAmountAction{Execute: func(int) {}})
	p.AbortEnteredActions()
	var nilPlayer *Player
	nilPlayer.AbortEnteredActions()
}

func TestEnteredActionAccessorsNilPlayerSafe(t *testing.T) {
	var nilPlayer *Player
	if nilPlayer.GetEnteredAmountAction() != nil || nilPlayer.GetEnteredSyntaxAction() != nil {
		t.Fatal("nil getters returned non-nil")
	}
	nilPlayer.SetEnteredAmountAction(&EnteredAmountAction{})
	nilPlayer.SetEnteredSyntaxAction(&EnteredSyntaxAction{})
}

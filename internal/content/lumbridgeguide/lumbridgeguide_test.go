package lumbridgeguide

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

func testContext(pages ...plugins.NpcDialoguePage) *plugins.NpcDialogueContext {
	return &plugins.NpcDialogueContext{
		Player: game.NewPlayer("testplayer", game.Location{X: 3090, Y: 3524, Z: 0}),
		NpcID:  306,
		Pages:  pages,
	}
}

func guidePage() plugins.NpcDialoguePage {
	return plugins.NpcDialoguePage{Page: Page, Variants: []string{LostVariant, NormalVariant}}
}

func TestChooseVariantNilContext(t *testing.T) {
	if got := newGuide(nil).chooseVariant(nil); got != nil {
		t.Fatalf("chooseVariant(nil) = %+v, want nil", got)
	}
}

func TestChooseVariantNoPages(t *testing.T) {
	g := newGuide(nil)
	for name, ctx := range map[string]*plugins.NpcDialogueContext{
		"nil pages":   testContext(),
		"empty pages": {Player: game.NewPlayer("p", game.Location{}), Pages: []plugins.NpcDialoguePage{}},
		"other page":  testContext(plugins.NpcDialoguePage{Page: "Bob", Variants: []string{"standard-dialogue"}}),
	} {
		if got := g.chooseVariant(ctx); got != nil {
			t.Errorf("%s: chooseVariant = %+v, want nil", name, got)
		}
	}
}

func TestChooseVariantNormalWithoutEmit(t *testing.T) {
	// No emit func installed (before Register): the request is
	// never answered, so the standard dialogue plays — the
	// no-hooks default.
	got := newGuide(nil).chooseVariant(testContext(guidePage()))
	if got == nil {
		t.Fatal("chooseVariant = nil, want the NORMAL choice")
	}
	if !got.HasPage || got.Page != Page || got.Variant != NormalVariant {
		t.Fatalf("chooseVariant = %+v, want page-qualified %q", got, NormalVariant)
	}
}

func TestChooseVariantNormalWhenUnanswered(t *testing.T) {
	var emitted string
	var seen *game.LumbridgeGuideAmuletStatusRequest
	emit := func(name string, payload any) {
		emitted = name
		seen = payload.(*game.LumbridgeGuideAmuletStatusRequest)
	}
	ctx := testContext(guidePage())
	got := newGuide(emit).chooseVariant(ctx)
	if got == nil || got.Variant != NormalVariant {
		t.Fatalf("chooseVariant = %+v, want NORMAL", got)
	}
	if emitted != game.LumbridgeGuideAmuletStatusEventName {
		t.Fatalf("emitted event = %q, want %q", emitted, game.LumbridgeGuideAmuletStatusEventName)
	}
	if seen == nil || seen.Player != ctx.Player || seen.Reclaim {
		t.Fatalf("request = %+v, want player set and reclaim false", seen)
	}
}

func TestChooseVariantLostWhenReclaim(t *testing.T) {
	emit := func(name string, payload any) {
		payload.(*game.LumbridgeGuideAmuletStatusRequest).Reclaim = true
	}
	got := newGuide(emit).chooseVariant(testContext(guidePage()))
	if got == nil {
		t.Fatal("chooseVariant = nil, want the LOST choice")
	}
	if !got.HasPage || got.Page != Page || got.Variant != LostVariant {
		t.Fatalf("chooseVariant = %+v, want page-qualified %q", got, LostVariant)
	}
}

func TestChooseVariantIgnoresOtherPagesFirst(t *testing.T) {
	// The guide's page need not be first in the list.
	ctx := testContext(
		plugins.NpcDialoguePage{Page: "Bob", Variants: []string{"standard-dialogue"}},
		guidePage(),
	)
	got := newGuide(nil).chooseVariant(ctx)
	if got == nil || got.Variant != NormalVariant {
		t.Fatalf("chooseVariant = %+v, want NORMAL", got)
	}
}

func TestPluginIdentity(t *testing.T) {
	if Plugin.Name != "LumbridgeGuide" {
		t.Fatalf("Plugin.Name = %q, want LumbridgeGuide", Plugin.Name)
	}
	want := "Picks the Lumbridge Guide's dialogue depending on whether the player has lost their Amulet of the Eye."
	if Plugin.Description != want {
		t.Fatalf("Plugin.Description = %q, want %q", Plugin.Description, want)
	}
	if Plugin.Members {
		t.Fatal("Plugin.Members = true, want false: the entry declares no members flag")
	}
	if Plugin.Register == nil {
		t.Fatal("Plugin.Register is nil")
	}
}

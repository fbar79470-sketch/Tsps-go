package game

import "testing"

// Dialogue shop-step seam tests: ShopDialogueSeams binds a fake
// ShopOpener to the TranscriptDeps function shapes, and the
// nil-opener degradation matches the nil-seam outcome (every
// open_shop step falls through). The name-match and core-filter
// behavior lives in the runner and the adapter, not here; these
// tests pin the passthrough contract only.

type fakeShopOpener struct {
	shopsByName map[string][]TranscriptShop
	opened      []int
	openValue   bool
}

func (f *fakeShopOpener) FindShops(name string) []TranscriptShop {
	return f.shopsByName[name]
}

func (f *fakeShopOpener) OpenShop(player *Player, shopID int) bool {
	f.opened = append(f.opened, shopID)
	return f.openValue
}

func TestShopDialogueSeamsPassthrough(t *testing.T) {
	opener := &fakeShopOpener{
		shopsByName: map[string][]TranscriptShop{
			"Bob's Shop": {
				{ID: 7, Source: "core"},
				{ID: 900, Source: "skillcapes"},
			},
		},
		openValue: true,
	}
	findShops, openShop := ShopDialogueSeams(opener)

	// All name matches, unfiltered: the runner owns the >1
	// core-source filter, so both the core shop and the
	// skillcape copy come back, in load order.
	got := findShops("Bob's Shop")
	if len(got) != 2 || got[0].ID != 7 || got[1].ID != 900 {
		t.Fatalf("FindShops = %+v, want IDs [7 900] in order", got)
	}
	// Unknown names yield nothing.
	if got := findShops("No Such Shop"); len(got) != 0 {
		t.Fatalf("FindShops(unknown) = %+v, want empty", got)
	}
	// The empty name matches nothing (the undefined-target form).
	if got := findShops(""); len(got) != 0 {
		t.Fatalf("FindShops(\"\") = %+v, want empty", got)
	}

	// OpenShop forwards the id and the result.
	player := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	if !openShop(player, 7) {
		t.Fatal("OpenShop must forward the opener's true")
	}
	if len(opener.opened) != 1 || opener.opened[0] != 7 {
		t.Fatalf("opened = %v, want [7]", opener.opened)
	}

	// A failed open forwards false (the step falls through).
	opener.openValue = false
	if openShop(player, 7) {
		t.Fatal("OpenShop must forward the opener's false")
	}
}

func TestShopDialogueSeamsNilOpener(t *testing.T) {
	findShops, openShop := ShopDialogueSeams(nil)
	if got := findShops("Bob's Shop"); len(got) != 0 {
		t.Fatalf("nil-opener FindShops = %+v, want empty", got)
	}
	player := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	if openShop(player, 7) {
		t.Fatal("nil-opener OpenShop must return false")
	}
}

func TestShopDialogueSeamsFillTranscriptDeps(t *testing.T) {
	// The returned functions must be directly assignable to
	// the TranscriptDeps fields (the coordinator's wiring
	// shape), and the assigned deps must answer through them.
	opener := &fakeShopOpener{
		shopsByName: map[string][]TranscriptShop{
			"Bob's Shop": {{ID: 7, Source: "core"}},
		},
		openValue: true,
	}
	var deps TranscriptDeps
	deps.FindShops, deps.OpenShop = ShopDialogueSeams(opener)
	if deps.FindShops == nil || deps.OpenShop == nil {
		t.Fatal("both deps fields must be filled")
	}
	got := deps.FindShops("Bob's Shop")
	if len(got) != 1 || got[0].ID != 7 {
		t.Fatalf("deps.FindShops = %+v, want one shop with ID 7", got)
	}
	player := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	if !deps.OpenShop(player, 7) {
		t.Fatal("deps.OpenShop must forward the opener's true")
	}
}

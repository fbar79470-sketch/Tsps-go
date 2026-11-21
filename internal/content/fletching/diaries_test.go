package fletching

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// diaryFixture builds a Diaries unit over the fixture tables
// (one western task: an oak shortbow in the Gnome Stronghold
// rect) with a capturing emit func.
func diaryFixture(t *testing.T) (*Diaries, *[]*DiaryTaskEvent) {
	t.Helper()
	tables := loadFixture(t)
	d := NewDiaries(tables)
	var emitted []*DiaryTaskEvent
	d.emit = func(name string, payload any) {
		if name != DiaryTaskEventName {
			t.Errorf("emit name = %q, want %q", name, DiaryTaskEventName)
			return
		}
		event, ok := payload.(*DiaryTaskEvent)
		if !ok {
			t.Errorf("emit payload = %T, want *DiaryTaskEvent", payload)
			return
		}
		emitted = append(emitted, event)
	}
	return d, &emitted
}

func TestDiariesMadeInsideArea(t *testing.T) {
	d, emitted := diaryFixture(t)
	player := game.NewPlayer("alice", game.Location{X: 2400, Y: 3400, Z: 0})
	oakShortbow, _ := loadFixture(t).ItemID("OAK_SHORTBOW")
	d.made(&FletchingMadeEvent{Player: player, ItemID: oakShortbow, Amount: 1})
	if len(*emitted) != 1 {
		t.Fatalf("emitted = %d diary:task events, want 1", len(*emitted))
	}
	event := (*emitted)[0]
	if event.Player != player || event.Diary != "western" ||
		event.Task != "fletch-an-oak-shortbow-in-the-gnome-stronghold" {
		t.Errorf("diary:task = %+v, want alice/western/fletch-an-oak-shortbow-in-the-gnome-stronghold", event)
	}
}

func TestDiariesMadeOutsideArea(t *testing.T) {
	d, emitted := diaryFixture(t)
	// Lumbridge is nowhere near the Gnome Stronghold rect.
	player := game.NewPlayer("alice", game.Location{X: 3222, Y: 3218, Z: 0})
	oakShortbow, _ := loadFixture(t).ItemID("OAK_SHORTBOW")
	d.made(&FletchingMadeEvent{Player: player, ItemID: oakShortbow, Amount: 1})
	if len(*emitted) != 0 {
		t.Errorf("emitted = %d diary:task events, want 0 (outside the rect)", len(*emitted))
	}
}

func TestDiariesMadeWrongItem(t *testing.T) {
	d, emitted := diaryFixture(t)
	player := game.NewPlayer("alice", game.Location{X: 2400, Y: 3400, Z: 0})
	shafts, _ := loadFixture(t).ItemID("ARROW_SHAFT")
	d.made(&FletchingMadeEvent{Player: player, ItemID: shafts, Amount: 15})
	if len(*emitted) != 0 {
		t.Errorf("emitted = %d diary:task events, want 0 (wrong item)", len(*emitted))
	}
}

func TestDiariesMadeIgnoresBadPayloads(t *testing.T) {
	d, emitted := diaryFixture(t)
	d.made(nil)
	d.made("not an event")
	d.made(&FletchingMadeEvent{Player: nil, ItemID: 1, Amount: 1})
	d.made(FletchingMadeEvent{Player: nil, ItemID: 1, Amount: 1})
	if len(*emitted) != 0 {
		t.Errorf("emitted = %d diary:task events, want 0", len(*emitted))
	}
	// A value (non-pointer) FletchingMadeEvent is accepted too.
	player := game.NewPlayer("alice", game.Location{X: 2400, Y: 3400, Z: 0})
	oakShortbow, _ := loadFixture(t).ItemID("OAK_SHORTBOW")
	d.made(FletchingMadeEvent{Player: player, ItemID: oakShortbow, Amount: 1})
	if len(*emitted) != 1 {
		t.Errorf("emitted = %d diary:task events, want 1 (value payload)", len(*emitted))
	}
}

func TestDiariesRectEdgesInclusive(t *testing.T) {
	d, emitted := diaryFixture(t)
	oakShortbow, _ := loadFixture(t).ItemID("OAK_SHORTBOW")
	for _, loc := range []game.Location{
		{X: 2370, Y: 3380, Z: 0}, // min corner
		{X: 2500, Y: 3520, Z: 0}, // max corner
		{X: 2500, Y: 3520, Z: 1}, // height is not checked
	} {
		player := game.NewPlayer("alice", loc)
		d.made(&FletchingMadeEvent{Player: player, ItemID: oakShortbow, Amount: 1})
	}
	if len(*emitted) != 3 {
		t.Errorf("emitted = %d diary:task events, want 3 (rect edges are inclusive, Z ignored)", len(*emitted))
	}
}

func TestDiariesNoTasks(t *testing.T) {
	d := NewDiaries(nil)
	if len(d.Tasks()) != 0 {
		t.Errorf("nil-tables diaries tasks = %d, want 0", len(d.Tasks()))
	}
	called := false
	d.emit = func(string, any) { called = true }
	player := game.NewPlayer("alice", game.Location{X: 2400, Y: 3400, Z: 0})
	d.made(&FletchingMadeEvent{Player: player, ItemID: 1, Amount: 1})
	if called {
		t.Error("nil-tables diaries should emit nothing")
	}
	// Register with a nil api is a no-op, not a panic.
	d.Register(nil)
}

func TestDiariesRegisterSubscribes(t *testing.T) {
	// The hub round-trip: Register subscribes to
	// "fletching:made", and a hub emit reaches the handler,
	// which emits "diary:task" back through the hub.
	tables := loadFixture(t)
	d := NewDiaries(tables)
	var emitted []*DiaryTaskEvent
	hub := plugins.NewHub([]plugins.Plugin{
		{
			Name:        "FletchingDiaries",
			Description: "Diary tasks done by fletching something in a place.",
			Members:     true,
			Register:    func(api *plugins.Api) { d.Register(api) },
		},
		{
			Name:        "DiaryListener",
			Description: "Test listener for diary:task.",
			Register: func(api *plugins.Api) {
				api.OnCustomEvent(DiaryTaskEventName, func(payload any) {
					if event, ok := payload.(*DiaryTaskEvent); ok {
						emitted = append(emitted, event)
					}
				})
			},
		},
	}, plugins.HubOptions{MembersWorld: true})
	hub.Load()
	player := game.NewPlayer("alice", game.Location{X: 2400, Y: 3400, Z: 0})
	oakShortbow, _ := tables.ItemID("OAK_SHORTBOW")
	hub.EmitCustomEvent(MadeEventName, &FletchingMadeEvent{Player: player, ItemID: oakShortbow, Amount: 1})
	if len(emitted) != 1 {
		t.Fatalf("hub round-trip emitted = %d diary:task events, want 1", len(emitted))
	}
	if emitted[0].Diary != "western" || emitted[0].Task != "fletch-an-oak-shortbow-in-the-gnome-stronghold" {
		t.Errorf("hub round-trip diary:task = %+v, want the western oak-shortbow task", emitted[0])
	}
}

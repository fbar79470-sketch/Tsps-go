// Diaries is the Go port of the Diaries sub-unit
// (server/plugins/skills/fletching/Diaries.Fletching.js, 35
// lines, commit bfd81e3): achievement diary tasks done by
// fletching something in a place — an oak shortbow in the
// Gnome Stronghold and a magic longbow in Tirannwn (Western
// Provinces), dragon darts at the Bedabin Camp (Desert), a
// maple shortbow in Seers' Village bank (Kandarin).
//
// The tasks come from the loaded Tables (the JSON diaryTasks,
// items resolved to ids, rects as-is). The unit listens to
// "fletching:made" (the FletchingMadeEvent the Shared loop
// emits after every batch) and emits "diary:task" for every
// task whose item matches and whose area contains the
// player's location. The "diary:task" emit has no consumer in
// Go yet — harmless, as in TS until a diary system answers
// it. The diary keys (western/desert/kandarin) are as the
// JSON has them.
//
// Line numbers in comments cite Diaries.Fletching.js.
package fletching

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// MadeEventName is the "fletching:made" custom event the
// Shared loop emits after every batch (shared.go
// FletchingMadeEvent).
const MadeEventName = "fletching:made"

// DiaryTaskEventName is the "diary:task" custom event the
// Diaries unit emits when a made item satisfies a diary task
// (Diaries.Fletching.js:17). No consumer exists in Go yet.
const DiaryTaskEventName = "diary:task"

// DiaryArea is one [minX, maxX, minY, maxY] diary rect
// (diaryTasks areas, Diaries.Fletching.js inside()).
type DiaryArea struct {
	MinX, MaxX, MinY, MaxY int
}

// Contains reports whether loc lies inside the rect
// (inclusive on both axes; the height level is not checked,
// as in TS).
func (a DiaryArea) Contains(loc game.Location) bool {
	return loc.X >= a.MinX && loc.X <= a.MaxX && loc.Y >= a.MinY && loc.Y <= a.MaxY
}

// DiaryTask is one resolved diaryTasks entry: the diary and
// task keys, the item ids that satisfy it, and its areas.
type DiaryTask struct {
	Diary string
	Task  string
	// ItemNames are the raw item names; ItemIDs are their
	// resolved ids, in the same order.
	ItemNames []string
	ItemIDs   []int
	Areas     []DiaryArea
}

// DiaryTaskEvent is the "diary:task" payload: the player, the
// diary key and the task key (Diaries.Fletching.js:17).
type DiaryTaskEvent struct {
	Player *game.Player
	Diary  string
	Task   string
}

// Diaries holds the diary tasks and the emit func: the Go form
// of the JS module-level `api` and `tasks`
// (Diaries.Fletching.js:8-9), captured at Register. One
// instance serves the Fletching plugin; tests build their own
// with a fake emit func.
type Diaries struct {
	tasks []*DiaryTask
	emit  func(name string, payload any)
}

// NewDiaries builds the Diaries unit from loaded tables. A nil
// tables value yields a unit with no tasks, which listens and
// never fires.
func NewDiaries(tables *Tables) *Diaries {
	d := &Diaries{}
	if tables != nil {
		d.tasks = tables.DiaryTasks
	}
	return d
}

// Tasks returns the diary tasks this unit watches, in JSON
// order.
func (d *Diaries) Tasks() []*DiaryTask {
	return d.tasks
}

// made handles one "fletching:made" emit (made,
// Diaries.Fletching.js:12-18): for every task whose item set
// holds the made item and whose areas contain the player's
// location, emit "diary:task". A nil player, a payload of any
// other type, or a task with no matching item or area emits
// nothing. Every matching task fires — there is no break in
// the TS loop.
func (d *Diaries) made(payload any) {
	var event *FletchingMadeEvent
	switch ev := payload.(type) {
	case *FletchingMadeEvent:
		event = ev
	case FletchingMadeEvent:
		event = &ev
	default:
		return
	}
	if event == nil || event.Player == nil {
		return
	}
	location := event.Player.Location()
	for _, task := range d.tasks {
		if !taskHasItem(task, event.ItemID) {
			continue
		}
		if !taskInArea(task, location) {
			continue
		}
		if d.emit != nil {
			d.emit(DiaryTaskEventName, &DiaryTaskEvent{
				Player: event.Player,
				Diary:  task.Diary,
				Task:   task.Task,
			})
		}
	}
}

// taskHasItem reports whether the task's item ids hold id
// (task.itemIds.has(itemId), Diaries.Fletching.js:15).
func taskHasItem(task *DiaryTask, id int) bool {
	if task == nil {
		return false
	}
	for _, itemID := range task.ItemIDs {
		if itemID == id {
			return true
		}
	}
	return false
}

// taskInArea reports whether any of the task's areas contains
// the location (task.areas.some(area => inside(location,
// area)), Diaries.Fletching.js:15; inside() checks x/y only).
func taskInArea(task *DiaryTask, location game.Location) bool {
	if task == nil {
		return false
	}
	for _, area := range task.Areas {
		if area.Contains(location) {
			return true
		}
	}
	return false
}

// registerDiaries attaches the Diaries unit to the shared
// instance's tables (the SEAM in fletching.go's Register,
// after registerSpool, in TS UNITS order). A nil tables
// value yields a unit with no tasks, which listens and never
// fires.
func (f *fletching) registerDiaries(api *plugins.Api) {
	NewDiaries(f.tables).Register(api)
}

// Register captures the emit func and subscribes the made
// handler (attach, Diaries.Fletching.js:24-26), following the
// gracefuloutfit/lumbridgeguide precedent: the emit func is
// captured at Register, not in a module-level global.
func (d *Diaries) Register(api *plugins.Api) {
	if api == nil {
		return
	}
	d.emit = api.EmitCustomEvent
	api.OnCustomEvent(MadeEventName, d.made)
}

// Worker C test helpers (wine / churn / sacred eel / leaping
// fish / gnome): the map-backed gameval resolver and the
// session fake with the interface-mount + sound surface the
// Gnome unit needs. Hand-built fixtures only; no live cache.
// All identifiers are wc-prefixed so they cannot collide with
// Workers A/B helpers (helpers_test.go).
package cooking

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/session"
)

// wcMapResolver is a gamevalResolver backed by maps: every
// name the Worker C units resolve outside cooking.json, plus
// the Gnome interface ids.
type wcMapResolver struct {
	objs       map[string]int
	seqs       map[string]int
	spotanims  map[string]int
	varbits    map[string]int
	interfaces map[string]int
	components map[string]int
	members    map[int]bool
}

func (r wcMapResolver) ObjID(name string) (int, bool, error) {
	id, ok := r.objs[name]
	return id, ok, nil
}

func (r wcMapResolver) SeqID(name string) (int, bool, error) {
	id, ok := r.seqs[name]
	return id, ok, nil
}

func (r wcMapResolver) VarbitID(name string) (int, bool, error) {
	id, ok := r.varbits[name]
	return id, ok, nil
}

func (r wcMapResolver) IsMembersItem(id int) bool { return r.members[id] }

func (r wcMapResolver) SpotanimID(name string) (int, bool, error) {
	id, ok := r.spotanims[name]
	return id, ok, nil
}

func (r wcMapResolver) InterfaceID(name string) (int, bool, error) {
	id, ok := r.interfaces[name]
	return id, ok, nil
}

func (r wcMapResolver) ComponentID(name string) (int, bool, error) {
	id, ok := r.components[name]
	return id, ok, nil
}

// wcTestSession is a fakeSession with the extra sender
// surface the Gnome unit asserts (interface mounts, flags,
// sounds) and the sound capture the Wine unit needs. It
// embeds *fakeSession, so messages/spam/varbits/menus behave
// exactly as in the Workers A/B tests.
type wcTestSession struct {
	*fakeSession
	mounts map[int]*session.SubInterfaceMount
	flags  []wcFlagsCall
	sounds []int
}

// wcFlagsCall is one SendInterfaceFlagsRange call.
type wcFlagsCall struct {
	uid      int
	fromSlot int
	toSlot   int
	flags    int
}

func newWCTestSession() *wcTestSession {
	return &wcTestSession{
		fakeSession: &fakeSession{},
		mounts:      make(map[int]*session.SubInterfaceMount),
	}
}

func (s *wcTestSession) SendSoundEffect(soundID, loopType, delay, volume int) {
	s.sounds = append(s.sounds, soundID)
}

func (s *wcTestSession) SendAreaSound(soundID, x, y, level, loops, delay, radius, attenuation int) {
}

func (s *wcTestSession) SendSound(soundID, volume, delay int) {
	s.sounds = append(s.sounds, soundID)
}

func (s *wcTestSession) SendInterfaceFlagsRange(uid, fromSlot, toSlot, flags int) {
	s.flags = append(s.flags, wcFlagsCall{uid: uid, fromSlot: fromSlot, toSlot: toSlot, flags: flags})
}

func (s *wcTestSession) MountSubInterface(targetUID, groupID, typ int) *session.SubInterfaceMount {
	mount := &session.SubInterfaceMount{TargetUID: targetUID, GroupID: groupID, Type: typ}
	s.mounts[targetUID] = mount
	return mount
}

func (s *wcTestSession) GetSubInterfaceAt(targetUID int) (*session.SubInterfaceMount, bool) {
	mount, ok := s.mounts[targetUID]
	return mount, ok
}

func (s *wcTestSession) CloseSubInterfaceMount(mount *session.SubInterfaceMount) bool {
	if mount == nil {
		return false
	}
	if current, ok := s.mounts[mount.TargetUID]; !ok || current != mount {
		return false
	}
	delete(s.mounts, mount.TargetUID)
	return true
}

var (
	_ game.SessionPort        = (*wcTestSession)(nil)
	_ game.SoundSender        = (*wcTestSession)(nil)
	_ gnomePacketSender       = (*wcTestSession)(nil)
	_ game.CreationMenuSender = (*wcTestSession)(nil)
)

// wcProduced returns the skills:produced payloads in emitted,
// in order.
func wcProduced(emitted *[]emittedEvent) []*ProducedEvent {
	var out []*ProducedEvent
	for _, e := range *emitted {
		if e.name == SkillsProducedEventName {
			if p, ok := e.payload.(*ProducedEvent); ok {
				out = append(out, p)
			}
		}
	}
	return out
}

// wcXP returns the player's Cooking experience.
func wcXP(p *game.Player) int {
	return p.Skills().Experience(game.SkillCooking)
}

// wcChurnTables builds fixture tables with the FULL churn
// data (all 4 inputs, all 7 products, all 3 labels) over
// fixture ids, extending fixtureTables() (which carries only
// the milk→cream row). Ids: milk 322, butter 323, cheese 324,
// not-garlic butter 325, cream input 326, cream product 244,
// bucket empty 211 — the loader's ids for the same names.
func wcChurnTables() *Tables {
	t := fixtureTables()
	t.Churn = &ChurnData{
		Labels: map[string]string{
			"pot_of_cream":  "Cream",
			"pot_of_butter": "Butter",
			"cheese":        "Cheese",
		},
		LabelsByProductID: map[int]string{244: "Cream", 323: "Butter", 324: "Cheese"},
		Inputs: []ChurnInput{
			{InputID: 325, Name: "butter"},
			{InputID: 323, Name: "butter"},
			{InputID: 326, Name: "cream"},
			{InputID: 322, Name: "milk", ReturnID: fxBucketEmpty, HasReturn: true},
		},
		Products: []ChurnProduct{
			{InputID: 322, ProductID: 244, Name: "cream", Level: 21, XP: 18, Ticks: 10, SeqID: 9001},
			{InputID: 322, ProductID: 323, Name: "butter", Level: 38, XP: 40.5, Ticks: 19, SeqID: 9002},
			{InputID: 322, ProductID: 324, Name: "cheese", Level: 48, XP: 64, Ticks: 26, SeqID: 9003},
			{InputID: 326, ProductID: 323, Name: "butter", Level: 38, XP: 22.5, Ticks: 10, SeqID: 9001},
			{InputID: 326, ProductID: 324, Name: "cheese", Level: 48, XP: 46, Ticks: 19, SeqID: 9002},
			{InputID: 323, ProductID: 324, Name: "cheese", Level: 48, XP: 23.5, Ticks: 10, SeqID: 9001},
			{InputID: 325, ProductID: 324, Name: "cheese", Level: 48, XP: 23.5, Ticks: 10, SeqID: 9001},
		},
	}
	if t.ItemIDs == nil {
		t.ItemIDs = make(map[string]int)
	}
	t.ItemIDs["pot_of_cream"] = 244
	t.ItemIDs["pot_of_butter"] = 323
	t.ItemIDs["cheese"] = 324
	t.ItemIDs["pot_of_not_garlic_butter"] = 325
	t.ItemIDs["bucket_milk"] = 322
	return t
}

// wcTestCooking builds a cooking instance over tables with a
// capturing emit func and a fresh TaskManager (the
// newTestCooking shape, for tables newTestCooking does not
// build).
func wcTestCooking(tables *Tables) (*cooking, *[]emittedEvent, *game.TaskManager) {
	var emitted []emittedEvent
	tm := game.NewTaskManager()
	c := newCooking(tables)
	c.emit = func(name string, payload any) {
		emitted = append(emitted, emittedEvent{name: name, payload: payload})
	}
	c.tasks = tm
	return c, &emitted, tm
}

// wcItemOnItem builds an item-on-item event for a pair use.
func wcItemOnItem(p *game.Player, usedID, withID int) *plugins.PluginItemOnItemEvent {
	return wcItemOnItemSlots(p, usedID, 0, withID, 1)
}

// wcItemOnItemSlots builds an item-on-item event for a pair
// use at explicit slots (some tests need items past slot 1).
func wcItemOnItemSlots(p *game.Player, usedID, usedSlot, withID, withSlot int) *plugins.PluginItemOnItemEvent {
	return &plugins.PluginItemOnItemEvent{
		Player:           p,
		UsedItem:         game.NewItem(usedID, 1),
		UsedItemID:       usedID,
		UsedItemSlot:     usedSlot,
		UsedWithItem:     game.NewItem(withID, 1),
		UsedWithItemID:   withID,
		UsedWithItemSlot: withSlot,
	}
}

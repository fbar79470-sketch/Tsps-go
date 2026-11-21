package cooking

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Churn unit tests (Churn.Cooking.js): hand-built fixtures
// over wcChurnTables() (the full churn data: 4 inputs, 7
// products, 3 labels).

func TestChurnChoicesOrder(t *testing.T) {
	c, _, _ := wcTestCooking(wcChurnTables())
	u := newChurnUnit(c)
	p := newTestPlayer("alice")
	setCooking(p, 99)
	give(p, 322, 1) // milk
	give(p, 326, 1) // cream
	got := u.choices(p, 0)
	// Products in product order (cream, butter, cheese);
	// within a product, the carried inputs in data order
	// (cream before milk).
	order := u.productOrder()
	if len(order) != 3 || order[0] != 244 || order[1] != 323 || order[2] != 324 {
		t.Fatalf("productOrder = %v, want [244 323 324]", order)
	}
	if len(got) != 3 {
		t.Fatalf("choices = %d products, want 3", len(got))
	}
	if rows := got[244]; len(rows) != 1 || rows[0].InputID != 322 {
		t.Fatalf("choices[244] = %+v, want cream from milk", rows)
	}
	if rows := got[323]; len(rows) != 2 ||
		rows[0].InputID != 326 || rows[1].InputID != 322 {
		t.Fatalf("choices[323] = %+v, want butter from cream then milk", rows)
	}
	if rows := got[324]; len(rows) != 2 ||
		rows[0].InputID != 326 || rows[1].InputID != 322 {
		t.Fatalf("choices[324] = %+v, want cheese from cream then milk", rows)
	}
}

func TestChurnOneBucketAndProduct(t *testing.T) {
	c, emitted, _ := wcTestCooking(wcChurnTables())
	u := newChurnUnit(c)
	p := newTestPlayer("alice")
	setCooking(p, 99)
	give(p, 322, 1) // milk in slot 0
	u.churnOne(p, ChurnProduct{InputID: 322, ProductID: 244, Name: "cream", Level: 21, XP: 18, Ticks: 10, SeqID: 9001})
	if got := countOf(p, 244); got != 1 {
		t.Fatalf("cream = %d, want 1 (in the milk's slot)", got)
	}
	if got := countOf(p, fxBucketEmpty); got != 1 {
		t.Fatalf("empty bucket = %d, want 1", got)
	}
	if got := countOf(p, 322); got != 0 {
		t.Fatalf("milk = %d, want 0", got)
	}
	if got := wcXP(p); got != 18 {
		t.Fatalf("xp = %d, want 18", got)
	}
	produced := wcProduced(emitted)
	if len(produced) != 1 || produced[0].Kind != "churn" || produced[0].ItemID != 244 || produced[0].XP != 18 {
		t.Fatalf("produced = %+v, want one churn event for cream / 18 xp", produced)
	}
}

func TestChurnOneGroundDropWhenFull(t *testing.T) {
	c, _, _ := wcTestCooking(wcChurnTables())
	u := newChurnUnit(c)
	p := newTestPlayer("alice")
	setCooking(p, 99)
	// Fill the inventory: 26 junk + milk + a second item the
	// churn cannot spend, so the bucket takes the milk's slot
	// and the product has nowhere to go.
	for i := 0; i < 26; i++ {
		give(p, 1000+i, 1)
	}
	give(p, 322, 1)
	give(p, 999, 1)
	var dropped []game.Item
	old := game.Hooks.DropGroundItem
	game.Hooks.DropGroundItem = func(item game.Item, loc game.Location, owner *game.Player) {
		dropped = append(dropped, item)
	}
	defer func() { game.Hooks.DropGroundItem = old }()
	u.churnOne(p, ChurnProduct{InputID: 322, ProductID: 244, Name: "cream", Level: 21, XP: 18, Ticks: 10, SeqID: 9001})
	if len(dropped) != 1 || dropped[0].ID() != 244 {
		t.Fatalf("dropped = %v, want [cream]", dropped)
	}
	if got := countOf(p, fxBucketEmpty); got != 1 {
		t.Fatalf("empty bucket = %d, want 1 (took the milk's slot)", got)
	}
}

func TestChurnStartTiming(t *testing.T) {
	c, _, tm := wcTestCooking(wcChurnTables())
	u := newChurnUnit(c)
	p := newTestPlayer("alice")
	sess := newWCTestSession()
	p.Session = sess
	setCooking(p, 99)
	give(p, 322, 2)
	u.offer(p, 0)
	if len(sess.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(sess.menus))
	}
	menu := sess.menus[0]
	if menu.Title() != churnMenuTitle || len(menu.Items()) != 3 {
		t.Fatalf("menu = %q %v, want title %q and 3 products", menu.Title(), menu.Items(), churnMenuTitle)
	}
	menu.Execute(244, 2)
	// First item after ticks-1 = 9 ticks.
	driveTicks(tm, 9)
	if got := countOf(p, 244); got != 1 {
		t.Fatalf("cream after 9 ticks = %d, want 1", got)
	}
	if anim := p.Animation(); anim == nil || anim.ID != 9001 {
		t.Fatalf("animation = %v, want the churn seq 9001", anim)
	}
	// Second item one full cycle (10 ticks) later.
	driveTicks(tm, 10)
	if got := countOf(p, 244); got != 2 {
		t.Fatalf("cream after 19 ticks = %d, want 2", got)
	}
	if got := countOf(p, 322); got != 0 {
		t.Fatalf("milk = %d, want 0", got)
	}
}

func TestChurnOfferNothing(t *testing.T) {
	c, _, _ := wcTestCooking(wcChurnTables())
	u := newChurnUnit(c)
	p := newTestPlayer("alice")
	sess := newWCTestSession()
	p.Session = sess
	setCooking(p, 99)
	u.offer(p, 0)
	if len(sess.menus) != 0 {
		t.Fatalf("menus = %d, want 0", len(sess.menus))
	}
	if len(sess.messages) != 1 || sess.messages[0] != nothingToChurnMessage {
		t.Fatalf("messages = %v, want [%q]", sess.messages, nothingToChurnMessage)
	}
}

func TestChurnHasLevel(t *testing.T) {
	c, _, _ := wcTestCooking(wcChurnTables())
	u := newChurnUnit(c)
	p := newTestPlayer("alice")
	sess := newWCTestSession()
	p.Session = sess
	setCooking(p, 20)
	row := ChurnProduct{InputID: 322, ProductID: 324, Name: "cheese", Level: 48, XP: 64, Ticks: 26, SeqID: 9003}
	if u.hasLevel(p, row) {
		t.Fatal("hasLevel(cheese at level 20) = true, want false")
	}
	setCooking(p, 48)
	if !u.hasLevel(p, row) {
		t.Fatal("hasLevel(cheese at level 48) = false, want true")
	}
}

func TestChurnClickFreeWorld(t *testing.T) {
	c, _, _ := wcTestCooking(wcChurnTables())
	u := newChurnUnit(c)
	p := newTestPlayer("alice")
	sess := newWCTestSession()
	p.Session = sess
	c.membersWorld = false
	if !u.churnClick(&plugins.ObjectInteractionEvent{Player: p}) {
		t.Fatal("churnClick on a free world = false, want true (the click is claimed with the message)")
	}
	if len(sess.messages) != 1 || sess.messages[0] != FreeToPlayItemMessage {
		t.Fatalf("messages = %v, want [%q]", sess.messages, FreeToPlayItemMessage)
	}
}

func TestChurnUseOnChurn(t *testing.T) {
	c, _, _ := wcTestCooking(wcChurnTables())
	u := newChurnUnit(c)
	p := newTestPlayer("alice")
	sess := newWCTestSession()
	p.Session = sess
	setCooking(p, 99)
	give(p, 322, 1)
	c.objectInfo = func(id int) (ObjectInfo, bool) {
		if id == 1 {
			return ObjectInfo{Name: "Dairy churn"}, true
		}
		return ObjectInfo{}, false
	}
	u.useOnChurn(&plugins.ItemOnObjectEvent{Player: p, ObjectID: 1, ItemID: 322})
	if len(sess.menus) != 1 {
		t.Fatalf("menus = %d, want 1 (milk on the churn offers)", len(sess.menus))
	}
	// A non-input item on the churn does not offer.
	u.useOnChurn(&plugins.ItemOnObjectEvent{Player: p, ObjectID: 1, ItemID: fxKnife})
	if len(sess.menus) != 1 {
		t.Fatalf("menus = %d, want 1 (knife on the churn offers nothing)", len(sess.menus))
	}
	// Milk on a non-churn object does not offer.
	u.useOnChurn(&plugins.ItemOnObjectEvent{Player: p, ObjectID: 2, ItemID: 322})
	if len(sess.menus) != 1 {
		t.Fatalf("menus = %d, want 1 (milk on a non-churn offers nothing)", len(sess.menus))
	}
}

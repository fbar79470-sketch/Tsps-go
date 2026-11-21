package listeners

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// pricedExamineSource is a fakeExamineSource that also supplies
// examine prices (ExaminePriceSource).
type pricedExamineSource struct {
	*fakeExamineSource
	name   string
	prices game.ExaminePrices
	ok     bool
	ids    []int
}

func (s *pricedExamineSource) ExaminePriceValues(itemID int) (string, game.ExaminePrices, bool) {
	s.ids = append(s.ids, itemID)
	return s.name, s.prices, s.ok
}

var _ ExaminePriceSource = (*pricedExamineSource)(nil)

// varbitMessageSession is a messageRecordingSession with a
// varbit store (game.VarbitReader), for the price-line settings
// gate.
type varbitMessageSession struct {
	*messageRecordingSession
	varbits map[int]int
}

func (s *varbitMessageSession) GetVarbit(id int) int { return s.varbits[id] }

var _ game.VarbitReader = (*varbitMessageSession)(nil)

// TestExaminePriceLine pins the price line on the inventory
// examine route (examineItem, ExaminePrice.ts:71-75): text, then
// the line when the source supplies prices and the settings
// varbits allow it; text only otherwise.
func TestExaminePriceLine(t *testing.T) {
	route := func(p *game.Player, src ExamineSource) {
		setInv(p, 0, itemBronzeSword, 3)
		got := Handle(p, game.TurnPacket{Opcode: protocol.ClientInventoryUse, Payload: inventoryUsePayload(0, itemBronzeSword, "Examine")}, 7, Deps{GroundItems: newManagerStore(), Examine: src})
		if got != (Outcome{Recognized: true, Implemented: true}) {
			t.Fatalf("Handle = %+v", got)
		}
	}

	t.Run("price line follows the text when settings allow", func(t *testing.T) {
		p, _ := messageTestPlayer()
		sess := &varbitMessageSession{messageRecordingSession: &messageRecordingSession{}, varbits: map[int]int{
			game.ExaminePriceGEVarbit: 1, game.ExaminePriceAlchVarbit: 1,
		}}
		p.Session = sess
		src := &pricedExamineSource{
			fakeExamineSource: &fakeExamineSource{itemText: "A bronze sword."},
			name:              "Bronze sword",
			prices:            game.ExaminePrices{GE: 67, HA: 240, LA: 160},
			ok:                true,
		}
		route(p, src)
		want := []string{
			"A bronze sword.",
			"Price of 3 x Bronze sword: GE <col=ff0000>201</col> (<col=ff0000>67</col>ea)  HA <col=ff0000>720</col> (<col=ff0000>240</col>ea)  LA <col=ff0000>480</col> (<col=ff0000>160</col>ea)",
		}
		if len(sess.messages) != len(want) {
			t.Fatalf("messages = %q, want %q", sess.messages, want)
		}
		for i := range want {
			if sess.messages[i] != want[i] {
				t.Fatalf("messages[%d] = %q, want %q", i, sess.messages[i], want[i])
			}
		}
		if len(src.ids) != 1 || src.ids[0] != itemBronzeSword {
			t.Fatalf("price ids = %v, want [%d]", src.ids, itemBronzeSword)
		}
	})

	t.Run("settings off sends text only", func(t *testing.T) {
		p, _ := messageTestPlayer()
		sess := &varbitMessageSession{messageRecordingSession: &messageRecordingSession{}, varbits: map[int]int{}}
		p.Session = sess
		src := &pricedExamineSource{
			fakeExamineSource: &fakeExamineSource{itemText: "A bronze sword."},
			name:              "Bronze sword", prices: game.ExaminePrices{GE: 67}, ok: true,
		}
		route(p, src)
		if len(sess.messages) != 1 || sess.messages[0] != "A bronze sword." {
			t.Fatalf("messages = %q", sess.messages)
		}
	})

	t.Run("no varbit reader sends text only", func(t *testing.T) {
		p, sess := messageTestPlayer()
		src := &pricedExamineSource{
			fakeExamineSource: &fakeExamineSource{itemText: "A bronze sword."},
			name:              "Bronze sword", prices: game.ExaminePrices{GE: 67}, ok: true,
		}
		route(p, src)
		if len(sess.messages) != 1 {
			t.Fatalf("messages = %q, want text only", sess.messages)
		}
	})

	t.Run("untradeable sends text only", func(t *testing.T) {
		p, _ := messageTestPlayer()
		sess := &varbitMessageSession{messageRecordingSession: &messageRecordingSession{}, varbits: map[int]int{
			game.ExaminePriceGEVarbit: 1, game.ExaminePriceAlchVarbit: 1,
		}}
		p.Session = sess
		src := &pricedExamineSource{
			fakeExamineSource: &fakeExamineSource{itemText: "Bound."},
			ok:                false,
		}
		route(p, src)
		if len(sess.messages) != 1 || sess.messages[0] != "Bound." {
			t.Fatalf("messages = %q", sess.messages)
		}
	})
}

package session

import (
	"bytes"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

func TestSendGraphicQueuesTileSpotAnim(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendGraphic(100, 50, 5, 3200, 3201, 1)
	packets := queuedPackets(t, s)
	want := protocol.EncodeTileSpotAnim(100, 3200, 3201, 1, 50, 5)
	if len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Errorf("packet = %x, want %x", packets, want)
	}
}

func TestSendNpcSpotAnimationSpecificQueuesEffect(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	npc := game.NewNPC(1, game.Location{X: 3200, Y: 3200})
	npc.SetIndex(12)
	s.SendNpcSpotAnimationSpecific(npc, 200, 1, 100, 30)
	// Nothing queued directly: the frame waits in the effect queue.
	if packets := queuedPackets(t, s); len(packets) != 0 {
		t.Fatalf("direct queue = %d packets, want 0 (effect queue path)", len(packets))
	}
	effects := s.takeNpcSpecificEffects()
	if len(effects) != 1 {
		t.Fatalf("effects = %d, want 1", len(effects))
	}
	if effects[0].Identity != any(npc) || effects[0].Index != 12 {
		t.Errorf("effect identity/index = %v/%d, want npc/12", effects[0].Identity, effects[0].Index)
	}
	want, _ := protocol.EncodeNpcSpotAnimationSpecific(12, 200, 1, 100, 30)
	if !bytes.Equal(effects[0].Packet, want) {
		t.Errorf("effect packet = %x, want %x", effects[0].Packet, want)
	}
	// Nil NPC and invalid values queue nothing.
	s.SendNpcSpotAnimationSpecific(nil, 200, 0, 0, 0)
	s.SendNpcSpotAnimationSpecific(npc, -2, 0, 0, 0)
	s.SendNpcSpotAnimationSpecific(npc, 200, 256, 0, 0)
	if effects := s.takeNpcSpecificEffects(); len(effects) != 0 {
		t.Errorf("invalid sends queued %d effects, want 0", len(effects))
	}
	// -1 clears the slot and queues.
	s.SendNpcSpotAnimationSpecific(npc, -1, 0, 0, 0)
	if effects := s.takeNpcSpecificEffects(); len(effects) != 1 {
		t.Errorf("clear-slot effects = %d, want 1", len(effects))
	}
}

func graphicViewerPlayer(t *testing.T, name string, loc game.Location) (*game.Player, *Session) {
	t.Helper()
	p := game.NewPlayer(name, loc)
	sess := NewSession(NewHeadlessChannel(""))
	p.Session = sess
	return p, sess
}

func TestSendGlobalGraphicGates(t *testing.T) {
	area := game.NewArea("a", nil)
	loc := game.Location{X: 3200, Y: 3200}
	near, nearSess := graphicViewerPlayer(t, "near", game.Location{X: 3205, Y: 3200})
	near.SetArea(area)
	far, farSess := graphicViewerPlayer(t, "far", game.Location{X: 3220, Y: 3200})
	far.SetArea(area)
	otherArea, otherSess := graphicViewerPlayer(t, "other", game.Location{X: 3205, Y: 3200})
	otherArea.SetArea(game.NewArea("b", nil))
	otherLevel, levelSess := graphicViewerPlayer(t, "up", game.Location{X: 3205, Y: 3200, Z: 1})
	otherLevel.SetArea(area)
	bot, botSess := graphicViewerPlayer(t, "bot", game.Location{X: 3205, Y: 3200})
	bot.SetArea(area)
	bot.IsBot = true

	SendGlobalGraphic(100, 50, 0, loc, area, []*game.Player{near, far, otherArea, otherLevel, bot})
	want := protocol.EncodeTileSpotAnim(100, 3200, 3200, 0, 50, 0)
	if packets := queuedPackets(t, nearSess); len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Errorf("near packets = %x, want [%x]", packets, want)
	}
	for name, sess := range map[string]*Session{"far": farSess, "otherArea": otherSess, "otherLevel": levelSess, "bot": botSess} {
		if packets := queuedPackets(t, sess); len(packets) != 0 {
			t.Errorf("%s packets = %d, want 0", name, len(packets))
		}
	}
	// Nil source area (main world): only area-less viewers hear it.
	mainWorld, mainSess := graphicViewerPlayer(t, "main", game.Location{X: 3205, Y: 3200})
	SendGlobalGraphic(100, 50, 0, loc, nil, []*game.Player{near, mainWorld})
	if packets := queuedPackets(t, nearSess); len(packets) != 1 {
		t.Errorf("near after main-world send = %d, want 1 (no new frame)", len(packets))
	}
	if packets := queuedPackets(t, mainSess); len(packets) != 1 {
		t.Errorf("main-world packets = %d, want 1", len(packets))
	}
	// Nil viewers and session-less viewers are skipped safely.
	nosess := game.NewPlayer("nosess", game.Location{X: 3205, Y: 3200})
	SendGlobalGraphic(100, 50, 0, loc, nil, []*game.Player{nil, nosess})
}

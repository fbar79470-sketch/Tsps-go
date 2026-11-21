package game

// fakeSession is a SessionPort test double recording flushes and sends.
type fakeSession struct {
	open     bool
	flushes  []int64
	flushErr error
	sent     [][]byte
	closes   [][2]any // [code, reason]
	// runEnergy records SendRunEnergy calls as [units, running, weightKg,
	// staminaTicks]; destinations records SendDestination calls as [x, y];
	// varps records SendVarp calls as [id, value]; messages records
	// SendGameMessage calls (plain text); typedMessages records
	// SendTypedGameMessage calls as [chatType, message, from].
	// varbits records SendVarbit calls as [id, value]; tabInterfaces
	// records SendTabInterface calls as [tabID, interfaceID]; strings
	// records SendString calls as [text, uid]; displayStates records
	// SendInterfaceDisplayState calls as [uid, hide]; jingles records
	// SendJingle calls as [id, delay]; varbitValues is the backing
	// store GetVarbit reads (0 when unset, the fresh-store default).
	runEnergy     [][4]any
	destinations  [][2]int
	varps         [][2]int
	messages      []string
	typedMessages [][3]any
	varbits       [][2]int
	tabInterfaces [][2]int
	strings       [][2]any
	displayStates [][2]any
	jingles       [][2]int
	varbitValues  map[int]int
}

func newFakeSession() *fakeSession { return &fakeSession{open: true} }

func (f *fakeSession) SendPacket(packet []byte) { f.sent = append(f.sent, packet) }

func (f *fakeSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
	f.runEnergy = append(f.runEnergy, [4]any{units, running, weightKg, staminaTicks})
}

func (f *fakeSession) SendDestination(x, y int) {
	f.destinations = append(f.destinations, [2]int{x, y})
}

func (f *fakeSession) SendVarp(id, value int) {
	f.varps = append(f.varps, [2]int{id, value})
}

func (f *fakeSession) SendVarbit(id, value int) {
	f.varbits = append(f.varbits, [2]int{id, value})
	if f.varbitValues == nil {
		f.varbitValues = make(map[int]int)
	}
	f.varbitValues[id] = value
}

// GetVarbit returns the last value recorded for a varbit, or 0
// when it was never sent. It activates the game's VarbitReader
// extension on this fake (examine_price.go).
func (f *fakeSession) GetVarbit(id int) int {
	return f.varbitValues[id]
}

// SendJingle records a jingle send. It activates the game's
// JingleSender extension on this fake (playerdeath.go).
func (f *fakeSession) SendJingle(id, delay int) {
	f.jingles = append(f.jingles, [2]int{id, delay})
}

func (f *fakeSession) SendTabInterface(tabID, interfaceID int) {
	f.tabInterfaces = append(f.tabInterfaces, [2]int{tabID, interfaceID})
}

func (f *fakeSession) SendString(text string, uid int) {
	f.strings = append(f.strings, [2]any{text, uid})
}

func (f *fakeSession) SendInterfaceDisplayState(uid int, hide bool) {
	f.displayStates = append(f.displayStates, [2]any{uid, hide})
}

func (f *fakeSession) SendGameMessage(message string) {
	f.messages = append(f.messages, message)
}

func (f *fakeSession) SendTypedGameMessage(chatType int, message, from string) {
	f.typedMessages = append(f.typedMessages, [3]any{chatType, message, from})
}

func (f *fakeSession) FlushTick(cycle int64) error {
	f.flushes = append(f.flushes, cycle)
	return f.flushErr
}

func (f *fakeSession) Close(code int, reason string) {
	f.open = false
	f.closes = append(f.closes, [2]any{code, reason})
}

func (f *fakeSession) IsOpen() bool { return f.open }

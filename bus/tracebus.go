package bus

import "time"

// TraceBus wraps a Bus and records the most-recent generation in
// which each address was read or written. UI can query whether a
// cell has been accessed recently for visualization.
//
// Memory cost is fixed: two uint32 arrays of length 0x10000 = 512 KB.
// That's a deliberate space-for-time trade — O(1) trace marking and
// O(1) lookup, no decay loop. The "generation" counter advances
// whenever the caller calls Tick (typically once per UI frame).
//
// gen=0 is reserved for "never accessed", so stamps are stored as
// (gen+1) and compared accordingly.
type TraceBus struct {
	inner Bus
	// readGen[addr]   = gen+1 stamp of last read at addr (0 = never)
	// writeGen[addr]  = gen+1 stamp of last write at addr (0 = never)
	// writeChGen[addr] = gen+1 stamp of last value-changing write
	//
	// A write that stores the same byte already at the address marks
	// writeGen but not writeChGen, so the UI can colour "touched but
	// unchanged" cells differently from "actually mutated" cells.
	readGen    [0x10000]uint32
	writeGen   [0x10000]uint32
	writeChGen [0x10000]uint32
	gen        uint32
}

func NewTraceBus(inner Bus) *TraceBus { return &TraceBus{inner: inner} }

func (t *TraceBus) Read(addr uint16) uint8 {
	t.readGen[addr] = t.gen + 1
	return t.inner.Read(addr)
}

func (t *TraceBus) Write(addr uint16, v uint8) {
	// Peek the prior value so we can distinguish value-changing
	// writes from idempotent ones. All bus components in this project
	// are pure on read, so the extra read is side-effect-free.
	prev := t.inner.Read(addr)
	t.inner.Write(addr, v)
	t.writeGen[addr] = t.gen + 1
	if prev != v {
		t.writeChGen[addr] = t.gen + 1
	}
}

func (t *TraceBus) Register(c Component) error { return t.inner.Register(c) }
func (t *TraceBus) Components() []Component    { return t.inner.Components() }

// Inner returns the underlying bus. Useful for the memory inspector
// which wants to read cells without polluting the trace with its own
// display reads.
func (t *TraceBus) Inner() Bus { return t.inner }

// Tick advances the generation counter and drives any Ticker
// components attached to the underlying bus. Call once per UI frame
// so access freshness ages out over time and any time-driven
// peripherals (e.g. a 6522 VIA timer) advance on the host's
// wall-clock — independent of CPU stepping or pause state.
func (t *TraceBus) Tick(dt time.Duration) {
	t.gen++
	for _, c := range t.inner.Components() {
		if tk, ok := c.(Ticker); ok {
			tk.Tick(dt)
		}
	}
}

// RecentRead reports whether addr was read within `freshness`
// generations of now. False if never accessed.
func (t *TraceBus) RecentRead(addr uint16, freshness uint32) bool {
	g := t.readGen[addr]
	if g == 0 {
		return false
	}
	return t.gen-(g-1) < freshness
}

// RecentWrite reports whether addr was written within `freshness`
// generations — regardless of whether the write changed the value.
func (t *TraceBus) RecentWrite(addr uint16, freshness uint32) bool {
	g := t.writeGen[addr]
	if g == 0 {
		return false
	}
	return t.gen-(g-1) < freshness
}

// RecentWriteChanged reports whether addr was written with a value
// different from its prior contents within `freshness` generations.
// Always implies RecentWrite, but the reverse is not true.
func (t *TraceBus) RecentWriteChanged(addr uint16, freshness uint32) bool {
	g := t.writeChGen[addr]
	if g == 0 {
		return false
	}
	return t.gen-(g-1) < freshness
}

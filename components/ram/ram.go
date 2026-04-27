// Package ram is a writable memory component for the bus.
package ram

type RAM struct {
	name  string
	base  uint16
	bytes []uint8
}

// New creates a RAM component of the given size at the given base.
func New(name string, base uint16, size int) *RAM {
	return &RAM{name: name, base: base, bytes: make([]uint8, size)}
}

func (r *RAM) Name() string              { return r.name }
func (r *RAM) Base() uint16              { return r.base }
func (r *RAM) Size() int                 { return len(r.bytes) }
func (r *RAM) Read(off uint16) uint8     { return r.bytes[off] }
func (r *RAM) Write(off uint16, v uint8) { r.bytes[off] = v }

// Reset zeroes all memory.
func (r *RAM) Reset() {
	for i := range r.bytes {
		r.bytes[i] = 0
	}
}

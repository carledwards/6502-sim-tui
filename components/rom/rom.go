// Package rom is a read-only memory component for the bus.
package rom

import "fmt"

type ROM struct {
	name  string
	base  uint16
	bytes []uint8
}

// New creates a ROM component of the given size at the given base.
// Contents start zeroed; populate with Load.
func New(name string, base uint16, size int) *ROM {
	return &ROM{name: name, base: base, bytes: make([]uint8, size)}
}

func (r *ROM) Name() string          { return r.name }
func (r *ROM) Base() uint16          { return r.base }
func (r *ROM) Size() int             { return len(r.bytes) }
func (r *ROM) Read(off uint16) uint8 { return r.bytes[off] }

// Write is a no-op — ROM is read-only on the bus.
func (r *ROM) Write(off uint16, v uint8) {}

// Clear zeroes every byte of ROM. Useful before loading a different
// program so leftover bytes from the previous payload don't bleed in.
func (r *ROM) Clear() {
	for i := range r.bytes {
		r.bytes[i] = 0
	}
}

// Load copies data into ROM starting at the given offset. Returns an
// error if data wouldn't fit.
func (r *ROM) Load(offset uint16, data []uint8) error {
	end := int(offset) + len(data)
	if end > len(r.bytes) {
		return fmt.Errorf("rom %q: load of %d bytes at $%04X overflows size %d",
			r.name, len(data), offset, len(r.bytes))
	}
	copy(r.bytes[offset:], data)
	return nil
}

// SetResetVector writes the standard 6502 reset vector ($FFFC/$FFFD)
// relative to the ROM. Useful for ROMs mapped at $E000-$FFFF where
// the reset vector lives at offset (0xFFFC - base).
func (r *ROM) SetResetVector(addr uint16) error {
	if r.base > 0xFFFC || int(r.base)+r.Size() < 0x10000 {
		return fmt.Errorf("rom %q does not cover reset vector $FFFC/$FFFD", r.name)
	}
	off := uint16(0xFFFC - r.base)
	r.bytes[off] = uint8(addr & 0xFF)
	r.bytes[off+1] = uint8(addr >> 8)
	return nil
}

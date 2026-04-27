package bus

import "testing"

type stub struct {
	name  string
	base  uint16
	size  int
	bytes []uint8
}

func newStub(name string, base uint16, size int) *stub {
	return &stub{name: name, base: base, size: size, bytes: make([]uint8, size)}
}

func (s *stub) Name() string                    { return s.name }
func (s *stub) Base() uint16                    { return s.base }
func (s *stub) Size() int                       { return s.size }
func (s *stub) Read(off uint16) uint8           { return s.bytes[off] }
func (s *stub) Write(off uint16, v uint8)       { s.bytes[off] = v }

func TestRegisterAndDispatch(t *testing.T) {
	b := New()
	ram := newStub("ram", 0x0000, 0x2000)
	rom := newStub("rom", 0xE000, 0x2000)
	if err := b.Register(ram); err != nil {
		t.Fatalf("register ram: %v", err)
	}
	if err := b.Register(rom); err != nil {
		t.Fatalf("register rom: %v", err)
	}

	b.Write(0x0010, 0x42)
	if got := b.Read(0x0010); got != 0x42 {
		t.Errorf("ram read: got %02X, want 42", got)
	}
	if ram.bytes[0x10] != 0x42 {
		t.Errorf("ram backing not updated: got %02X", ram.bytes[0x10])
	}

	rom.bytes[0x100] = 0xAA
	if got := b.Read(0xE100); got != 0xAA {
		t.Errorf("rom read: got %02X, want AA", got)
	}
}

func TestUnmappedReadReturnsZero(t *testing.T) {
	b := New()
	if got := b.Read(0x4000); got != 0x00 {
		t.Errorf("unmapped read: got %02X, want 00", got)
	}
}

func TestUnmappedWriteIsDropped(t *testing.T) {
	b := New()
	b.Write(0x4000, 0xFF) // must not panic
}

func TestRegisterOverlapRejected(t *testing.T) {
	b := New()
	if err := b.Register(newStub("a", 0x1000, 0x1000)); err != nil {
		t.Fatalf("first register: %v", err)
	}
	cases := []struct {
		name string
		c    Component
	}{
		{"exact overlap", newStub("b", 0x1000, 0x1000)},
		{"left tail", newStub("b", 0x0F00, 0x0200)},
		{"right tail", newStub("b", 0x1F00, 0x0200)},
		{"contained", newStub("b", 0x1100, 0x0100)},
		{"contains", newStub("b", 0x0800, 0x2000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := b.Register(tc.c); err == nil {
				t.Errorf("expected overlap error, got nil")
			}
		})
	}
}

func TestRegisterAdjacentAccepted(t *testing.T) {
	b := New()
	if err := b.Register(newStub("a", 0x0000, 0x1000)); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if err := b.Register(newStub("b", 0x1000, 0x1000)); err != nil {
		t.Errorf("adjacent register should succeed: %v", err)
	}
}

func TestRegisterOutOfRangeRejected(t *testing.T) {
	b := New()
	// 0xFFFF base + size 2 spills past 0xFFFF.
	if err := b.Register(newStub("oversize", 0xFFFF, 2)); err == nil {
		t.Errorf("expected out-of-range error, got nil")
	}
}

func TestRegisterFull64KAccepted(t *testing.T) {
	b := New()
	if err := b.Register(newStub("everything", 0x0000, 0x10000)); err != nil {
		t.Errorf("full 64K register should succeed: %v", err)
	}
}

func TestComponentsSortedByBase(t *testing.T) {
	b := New()
	_ = b.Register(newStub("rom", 0xE000, 0x2000))
	_ = b.Register(newStub("ram", 0x0000, 0x2000))
	_ = b.Register(newStub("io", 0x6000, 0x0100))
	got := b.Components()
	if len(got) != 3 || got[0].Name() != "ram" || got[1].Name() != "io" || got[2].Name() != "rom" {
		t.Errorf("components not sorted by base: %v", names(got))
	}
}

func names(cs []Component) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name()
	}
	return out
}

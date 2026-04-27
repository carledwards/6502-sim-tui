package rom

import "testing"

func TestWriteIgnored(t *testing.T) {
	r := New("rom", 0xE000, 0x2000)
	if err := r.Load(0, []uint8{0xAA}); err != nil {
		t.Fatalf("load: %v", err)
	}
	r.Write(0, 0xFF)
	if got := r.Read(0); got != 0xAA {
		t.Errorf("rom write should not stick: got %02X, want AA", got)
	}
}

func TestLoadAndRead(t *testing.T) {
	r := New("rom", 0xE000, 0x10)
	prog := []uint8{0xA9, 0x50, 0x8D, 0x00, 0x10}
	if err := r.Load(0, prog); err != nil {
		t.Fatalf("load: %v", err)
	}
	for i, want := range prog {
		if got := r.Read(uint16(i)); got != want {
			t.Errorf("offset %d: got %02X, want %02X", i, got, want)
		}
	}
}

func TestLoadOverflowRejected(t *testing.T) {
	r := New("rom", 0, 4)
	if err := r.Load(2, []uint8{1, 2, 3}); err == nil {
		t.Errorf("expected overflow error")
	}
}

func TestSetResetVector(t *testing.T) {
	r := New("rom", 0xE000, 0x2000)
	if err := r.SetResetVector(0xE000); err != nil {
		t.Fatalf("set reset vector: %v", err)
	}
	if got := r.Read(0x1FFC); got != 0x00 {
		t.Errorf("low byte: got %02X, want 00", got)
	}
	if got := r.Read(0x1FFD); got != 0xE0 {
		t.Errorf("high byte: got %02X, want E0", got)
	}
}

func TestSetResetVectorRejectsROMNotCoveringFFFC(t *testing.T) {
	r := New("rom", 0x8000, 0x100) // doesn't reach $FFFC
	if err := r.SetResetVector(0x8000); err == nil {
		t.Errorf("expected error for ROM not covering reset vector")
	}
}

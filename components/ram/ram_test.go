package ram

import "testing"

func TestReadWrite(t *testing.T) {
	r := New("ram", 0x0000, 0x100)
	r.Write(0x10, 0x42)
	if got := r.Read(0x10); got != 0x42 {
		t.Errorf("got %02X, want 42", got)
	}
}

func TestReset(t *testing.T) {
	r := New("ram", 0, 4)
	r.Write(0, 1)
	r.Write(3, 9)
	r.Reset()
	for i := uint16(0); i < 4; i++ {
		if got := r.Read(i); got != 0 {
			t.Errorf("offset %d: got %02X, want 0", i, got)
		}
	}
}

func TestMetadata(t *testing.T) {
	r := New("main", 0x1234, 0x80)
	if r.Name() != "main" {
		t.Errorf("name: got %q", r.Name())
	}
	if r.Base() != 0x1234 {
		t.Errorf("base: got %04X", r.Base())
	}
	if r.Size() != 0x80 {
		t.Errorf("size: got %d", r.Size())
	}
}

package disasm

import "testing"

func TestDecodeDemoProgram(t *testing.T) {
	// $E000 LDX #$00
	// $E002 TXA
	// $E003 STA $00,X
	// $E005 INX
	// $E006 JMP $E002
	prog := []uint8{0xA2, 0x00, 0x8A, 0x95, 0x00, 0xE8, 0x4C, 0x02, 0xE0}
	read := func(addr uint16) uint8 { return prog[addr-0xE000] }

	cases := []struct {
		pc      uint16
		want    string
		size    int
	}{
		{0xE000, "LDX #$00", 2},
		{0xE002, "TXA", 1},
		{0xE003, "STA $00,X", 2},
		{0xE005, "INX", 1},
		{0xE006, "JMP $E002", 3},
	}
	for _, c := range cases {
		ins := Decode(c.pc, read)
		if ins.Pretty != c.want {
			t.Errorf("$%04X: got %q, want %q", c.pc, ins.Pretty, c.want)
		}
		if ins.Size() != c.size {
			t.Errorf("$%04X: size got %d, want %d", c.pc, ins.Size(), c.size)
		}
	}
}

func TestDecodeRelative(t *testing.T) {
	// BNE +4 at $E010 should target $E016
	read := func(addr uint16) uint8 {
		if addr == 0xE010 {
			return 0xD0 // BNE
		}
		if addr == 0xE011 {
			return 0x04
		}
		return 0
	}
	ins := Decode(0xE010, read)
	if ins.Pretty != "BNE $E016" {
		t.Errorf("got %q, want BNE $E016", ins.Pretty)
	}

	// Negative offset
	read2 := func(addr uint16) uint8 {
		if addr == 0xE010 {
			return 0xD0
		}
		if addr == 0xE011 {
			return 0xFC // -4
		}
		return 0
	}
	ins = Decode(0xE010, read2)
	if ins.Pretty != "BNE $E00E" {
		t.Errorf("got %q, want BNE $E00E", ins.Pretty)
	}
}

func TestDecodeAddressingModes(t *testing.T) {
	cases := []struct {
		bytes []uint8
		want  string
	}{
		{[]uint8{0xA9, 0x42}, "LDA #$42"},                  // immediate
		{[]uint8{0xA5, 0x10}, "LDA $10"},                   // zp
		{[]uint8{0xB5, 0x10}, "LDA $10,X"},                 // zp,X
		{[]uint8{0xB6, 0x10}, "LDX $10,Y"},                 // zp,Y
		{[]uint8{0xAD, 0x34, 0x12}, "LDA $1234"},           // abs
		{[]uint8{0xBD, 0x34, 0x12}, "LDA $1234,X"},         // abs,X
		{[]uint8{0xB9, 0x34, 0x12}, "LDA $1234,Y"},         // abs,Y
		{[]uint8{0x6C, 0x34, 0x12}, "JMP ($1234)"},         // indirect
		{[]uint8{0xA1, 0x10}, "LDA ($10,X)"},               // (zp,X)
		{[]uint8{0xB1, 0x10}, "LDA ($10),Y"},               // (zp),Y
		{[]uint8{0x0A}, "ASL A"},                            // accumulator
		{[]uint8{0xEA}, "NOP"},                              // implied
		{[]uint8{0xFF}, "???"},                              // illegal
	}
	for _, c := range cases {
		read := func(addr uint16) uint8 {
			i := int(addr - 0x1000)
			if i >= 0 && i < len(c.bytes) {
				return c.bytes[i]
			}
			return 0
		}
		ins := Decode(0x1000, read)
		if ins.Pretty != c.want {
			t.Errorf("bytes %v: got %q, want %q", c.bytes, ins.Pretty, c.want)
		}
	}
}

func TestHexBytes(t *testing.T) {
	cases := []struct {
		in   []uint8
		want string
	}{
		{[]uint8{0xA9}, "A9      "},
		{[]uint8{0xA9, 0x42}, "A9 42   "},
		{[]uint8{0x4C, 0x02, 0xE0}, "4C 02 E0"},
	}
	for _, c := range cases {
		got := HexBytes(c.in)
		if got != c.want {
			t.Errorf("HexBytes(%v): got %q, want %q", c.in, got, c.want)
		}
	}
}

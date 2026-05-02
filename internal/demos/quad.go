package demos

// Quadrant scroll demo — divides the framebuffer into 4 rectangular
// regions and rotates each independently in a different cardinal
// direction every frame:
//
//   ┌──── rows 0..5 ────┐
//   │  TL: rotate up    │  TR: rotate right
//   ├──── row 6 div ────┤
//   │  BL: rotate down  │  BR: rotate left
//   └──── rows 7..12 ───┘
//
// All shifting is done by the VIC controller's CmdRect* commands —
// the CPU just writes (X, Y, W, H) into RegRectX/Y/W/H ($8803..$8806)
// and fires a CmdRectRot{Up,Right,Down,Left} at $8800. Pause + Frame
// commit each iteration as one snapshot.

// Quad is built at init time by assembling the listing below.
var Quad = buildQuad()

// ----- Tiny 6502 assembler (just enough for this demo) -----
//
// Forward refs (JSR/JMP to labels defined later) and relative branches
// are back-patched at the end via the labels map. Hand-counting branch
// offsets across ~150 bytes was error-prone; this is correct by
// construction.

type asm struct {
	base   int
	code   []byte
	labels map[string]int
	fixes  []asmFix
}

type asmFix struct {
	pos    int
	target string
	rel    bool
	relPC  int
}

func newAsm(base int) *asm {
	return &asm{base: base, labels: map[string]int{}}
}

func (a *asm) pc() int               { return a.base + len(a.code) }
func (a *asm) emit(b ...byte)        { a.code = append(a.code, b...) }
func (a *asm) label(name string)     { a.labels[name] = a.pc() }
func (a *asm) addrFix(target string) { a.fixes = append(a.fixes, asmFix{pos: len(a.code), target: target}); a.emit(0, 0) }
func (a *asm) relFix(target string) {
	a.fixes = append(a.fixes, asmFix{pos: len(a.code), target: target, rel: true, relPC: a.pc() + 1})
	a.emit(0)
}

// Instruction emitters.
func (a *asm) lda_imm(v byte)       { a.emit(0xA9, v) }
func (a *asm) ldx_imm(v byte)       { a.emit(0xA2, v) }
func (a *asm) sta_abs(addr uint16)  { a.emit(0x8D, byte(addr), byte(addr>>8)) }
func (a *asm) sta_absx(addr uint16) { a.emit(0x9D, byte(addr), byte(addr>>8)) }
func (a *asm) txa()                 { a.emit(0x8A) }
func (a *asm) dex()                 { a.emit(0xCA) }
func (a *asm) clc()                 { a.emit(0x18) }
func (a *asm) adc_imm(v byte)       { a.emit(0x69, v) }
func (a *asm) rts()                 { a.emit(0x60) }
func (a *asm) jsr(target string)    { a.emit(0x20); a.addrFix(target) }
func (a *asm) jmp(target string)    { a.emit(0x4C); a.addrFix(target) }
func (a *asm) bpl(target string)    { a.emit(0x10); a.relFix(target) }

func (a *asm) build() []byte {
	for _, f := range a.fixes {
		addr, ok := a.labels[f.target]
		if !ok {
			panic("undefined label: " + f.target)
		}
		if f.rel {
			off := addr - f.relPC
			if off < -128 || off > 127 {
				panic("relative branch out of range to " + f.target)
			}
			a.code[f.pos] = byte(off)
		} else {
			a.code[f.pos] = byte(addr)
			a.code[f.pos+1] = byte(addr >> 8)
		}
	}
	return a.code
}

// ----- Demo program -----

func buildQuad() []byte {
	a := newAsm(0xE000)

	// Controller register addresses.
	const (
		regCmd   uint16 = 0x8800
		regPause uint16 = 0x8801
		regFrame uint16 = 0x8802
		regRectX uint16 = 0x8803
		regRectY uint16 = 0x8804
		regRectW uint16 = 0x8805
		regRectH uint16 = 0x8806

		cmdClear        byte = 0x01
		cmdRectRotUp    byte = 0x0F
		cmdRectRotDown  byte = 0x10
		cmdRectRotLeft  byte = 0x11
		cmdRectRotRight byte = 0x12
	)

	// fireRect emits the 5-instruction sequence that programs the
	// rect coords and fires a single CmdRect* command.
	fireRect := func(x, y, w, h, cmd byte) {
		a.lda_imm(x)
		a.sta_abs(regRectX)
		a.lda_imm(y)
		a.sta_abs(regRectY)
		a.lda_imm(w)
		a.sta_abs(regRectW)
		a.lda_imm(h)
		a.sta_abs(regRectH)
		a.lda_imm(cmd)
		a.sta_abs(regCmd)
	}

	// === ENTRY ===
	a.lda_imm(cmdClear)
	a.sta_abs(regCmd)
	a.lda_imm(0x01)
	a.sta_abs(regPause)
	a.jsr("DRAW")

	// === LOOP — issue four rect rotations, then frame-commit ===
	a.label("LOOP")
	fireRect(0, 0, 20, 6, cmdRectRotUp)     // TL
	fireRect(20, 0, 20, 6, cmdRectRotRight) // TR
	fireRect(0, 7, 20, 6, cmdRectRotDown)   // BL
	fireRect(20, 7, 20, 6, cmdRectRotLeft)  // BR
	a.lda_imm(0x01)
	a.sta_abs(regFrame)
	a.jmp("LOOP")

	// === DRAW — paint distinct color patterns into the 4 quadrants ===
	a.label("DRAW")

	// Horizontal stripes (TL / BL): each row a distinct color so
	// vertical motion is visible.
	drawHStripe := func(rowAddr uint16, colorByte byte) {
		a.lda_imm(colorByte)
		a.ldx_imm(20 - 1)
		lbl := "STR" + intToLabel(int(rowAddr))
		a.label(lbl)
		a.sta_absx(rowAddr)
		a.dex()
		a.bpl(lbl)
	}
	// TL rows 0..5
	drawHStripe(0x8200, 0x1F)
	drawHStripe(0x8228, 0x2F)
	drawHStripe(0x8250, 0x3F)
	drawHStripe(0x8278, 0x4F)
	drawHStripe(0x82A0, 0x5F)
	drawHStripe(0x82C8, 0x6F)
	// BL rows 7..12
	drawHStripe(0x8318, 0xAF)
	drawHStripe(0x8340, 0xBF)
	drawHStripe(0x8368, 0xCF)
	drawHStripe(0x8390, 0xDF)
	drawHStripe(0x83B8, 0xEF)
	drawHStripe(0x83E0, 0xFF)

	// Vertical stripes (TR / BR): each column a distinct color so
	// horizontal motion is visible. Same X loop fills 6 row addresses
	// per quadrant.
	drawVStripes := func(colorBias byte, rowAddrs ...uint16) {
		a.ldx_imm(20 - 1)
		lbl := "VS" + intToLabel(int(rowAddrs[0]))
		a.label(lbl)
		a.txa()
		a.clc()
		a.adc_imm(colorBias)
		for _, addr := range rowAddrs {
			a.sta_absx(addr)
		}
		a.dex()
		a.bpl(lbl)
	}
	drawVStripes(0x80, 0x8214, 0x823C, 0x8264, 0x828C, 0x82B4, 0x82DC) // TR
	drawVStripes(0x10, 0x832C, 0x8354, 0x837C, 0x83A4, 0x83CC, 0x83F4) // BR

	a.rts()

	return a.build()
}

func intToLabel(n int) string {
	const hex = "0123456789ABCDEF"
	if n == 0 {
		return "0"
	}
	out := []byte{}
	for n > 0 {
		out = append([]byte{hex[n&0xF]}, out...)
		n >>= 4
	}
	return string(out)
}

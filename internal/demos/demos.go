// Package demos holds the 6502 demo programs surfaced by the
// simulator's Demo menu. Both the terminal binary (cmd/6502-sim)
// and the wasm binary (cmd/6502-wasm) import this package so they
// share an identical demo lineup without copy/paste of bytecode.
package demos

// Memory map a demo author should know:
//
//	$0000-$1FFF  RAM (8 KB)
//	$8000+       VIC color plane  (40 × 13 = 520 bytes)
//	$8500+       VIC char plane   (520 bytes)
//	$8800-$8802  VIC controller   (cmd / pause / frame)
//	$E000-$FFFF  ROM (program loaded here, reset vector at $FFFC)

// Demo is one selectable ROM payload. Each demo starts at $E000;
// the reset vector is wired to point there. Switching demos at
// runtime: clear the ROM, load the new bytes, set the reset vector,
// repaint the host-side display init pattern, then Reset the CPU.
type Demo struct {
	Name  string
	Bytes []uint8
}

// Section is a labelled group of demos shown in the Demo menu.
// Sections are separated by a Separator menu item.
type Section struct {
	Demos []Demo
}

// Sections returns the menu lineup. First section is "live" (UI
// updates as memory changes), second is "framed" (UI shows snapshot,
// CPU controls when to commit).
func Sections() []Section {
	return []Section{
		{[]Demo{
			{"&Marquee (default)", Marquee},
			{"&Bouncer", Bouncer},
			{"&Scroller", Scroller},
			{"S&now (LFSR)", Snow},
		}},
		{[]Demo{
			{"Scroller (&framed)", ScrollerFramed},
			{"&Blitter (RAM→VIC)", Blitter},
			{"&Quadrants (4 scrolls)", Quad},
		}},
		{[]Demo{
			{"&Bouncing Balls (graphics mode)", BouncingBalls},
		}},
	}
}

// Marquee — "HELLO 6502 SIM" scrolling marquee.
//
// Setup phase:
//  1. Clear the display via the controller.
//  2. Copy a 14-byte message into row 6 of the char plane
//     ($05F0..$05FD) and paint that range navy-bg/yellow-fg in the
//     color plane ($02F0..$02FD).
//
// Steady state:
//
//	loop forever: poke CmdRotLeft into $0800.
//
// All rows rotate independently each iteration, so the host's
// initial diagonal-gradient pattern keeps shifting in the rows above
// and below the message.
//
//	$E000  LDA #$01         ; A9 01
//	$E002  STA $0800        ; 8D 00 08      controller: clear
//	$E005  LDX #$00         ; A2 00
//	$E007  LDA $E01F,X      ; BD 1F E0      msg[X]
//	$E00A  STA $05F0,X      ; 9D F0 05      char[row6+X] = msg[X]
//	$E00D  LDA #$1E         ; A9 1E
//	$E00F  STA $02F0,X      ; 9D F0 02      color: bg navy, fg yellow
//	$E012  INX              ; E8
//	$E013  CPX #$0E         ; E0 0E         14 chars
//	$E015  BNE $E007        ; D0 F0
//	$E017  LDA #$07         ; A9 07
//	$E019  STA $0800        ; 8D 00 08      controller: rot left
//	$E01C  JMP $E017        ; 4C 17 E0
//	$E01F  "HELLO 6502 SIM" ; 14 bytes
var Marquee = []uint8{
	// Clear screen via controller @ $8800.
	0xA9, 0x01,
	0x8D, 0x00, 0x88,

	// Copy message + paint color into row 6.
	0xA2, 0x00, // LDX #$00
	0xBD, 0x1F, 0xE0, // LDA $E01F,X      (msg+X)
	0x9D, 0xF0, 0x85, // STA $85F0,X      (char plane row 6)
	0xA9, 0x1E, //       LDA #$1E         (bg=navy, fg=yellow)
	0x9D, 0xF0, 0x82, // STA $82F0,X      (color plane row 6)
	0xE8,       // INX
	0xE0, 0x0E, // CPX #14
	0xD0, 0xF0, // BNE -16          (back to LDA msg)

	// Forever-scroll: poke CmdRotLeft (= $07) to controller @ $8800.
	0xA9, 0x07,
	0x8D, 0x00, 0x88,
	0x4C, 0x17, 0xE0, // JMP back to LDA #$07

	// Message bytes at $E01F.
	'H', 'E', 'L', 'L', 'O', ' ', '6', '5', '0', '2', ' ', 'S', 'I', 'M',
}

// Bouncer — a single colored '*' bounces left-and-right across row 6,
// erasing its trail. ZP[$00] = x, ZP[$01] = direction (+1/-1).
//
// Includes a Y-counter delay loop after each move so motion is
// visible regardless of clock speed (without it, at high batch sizes
// the cell crosses the row faster than the eye can track).
var Bouncer = []uint8{
	// Setup: clear, init x=20, dx=+1.
	0xA9, 0x01, 0x8D, 0x00, 0x88, // LDA #$01 ; STA $8800   (clear)
	0xA2, 0x14, 0x86, 0x00, // LDX #20  ; STX $00     (x=20)
	0xA9, 0x01, 0x85, 0x01, // LDA #1   ; STA $01     (dx=+1)

	// Loop @ $E00D
	0xA6, 0x00, // LDX $00
	0xA9, 0x20, 0x9D, 0xF0, 0x85, // LDA #' ' ; STA $85F0,X  (erase char)
	0xA9, 0x00, 0x9D, 0xF0, 0x82, // LDA #0   ; STA $82F0,X  (erase color)

	// Move: dx>=0 → INX, else DEX.
	0xA5, 0x01, 0x10, 0x04, // LDA $01 ; BPL +4
	0xCA, 0x4C, 0x22, 0xE0, // DEX ; JMP $E022
	0xE8,       // INX
	0x86, 0x00, // STX $00

	// Draw: '*' at row 6 col x.
	0xA9, 0x2A, 0x9D, 0xF0, 0x85, // LDA #'*' ; STA $85F0,X
	0xA9, 0x1E, 0x9D, 0xF0, 0x82, // LDA #$1E ; STA $82F0,X (navy/yellow)

	// Delay — Y counts down from $FF, ~256 iterations.
	0xA0, 0xFF, // LDY #$FF
	0x88, 0xD0, 0xFD, // DEY ; BNE -3

	// Bounds: if X==0 or X==39, jump to flip @ $E03E.
	0xE0, 0x00, 0xF0, 0x07, // CPX #0  ; BEQ +7
	0xE0, 0x27, 0xF0, 0x03, // CPX #39 ; BEQ +3
	0x4C, 0x0D, 0xE0, // JMP loop

	// Flip @ $E03E: dx = -dx.
	0xA5, 0x01, 0x49, 0xFF, // LDA $01 ; EOR #$FF
	0x18, 0x69, 0x01, // CLC ; ADC #1
	0x85, 0x01, // STA $01
	0x4C, 0x0D, 0xE0, // JMP loop
}

// Scroller — fills the bottom row (row 12, offsets $1E0..$207) with
// a varying gradient (cell value = X + counter), then pokes
// CmdShiftUp. Each iteration: counter++, fill, shift. Result is a
// continuously flowing diagonal pattern climbing up the screen.
var Scroller = []uint8{
	// Loop @ $E000
	0xE6, 0x00, // INC $00              (counter++)
	0xA2, 0x00, // LDX #0

	// Fill bottom row @ $E004
	0x8A,             // TXA
	0x18,             // CLC
	0x65, 0x00,       // ADC $00              (A = X + counter)
	0x9D, 0xE0, 0x83, // STA $83E0,X          (color[bottomrow + X])
	0x9D, 0xE0, 0x86, // STA $86E0,X          (char[bottomrow + X])
	0xE8,             // INX
	0xE0, 0x28,       // CPX #40
	0xD0, 0xF1,       // BNE fill (-15)

	// Shift up via controller.
	0xA9, 0x02, // LDA #2               (CmdShiftUp)
	0x8D, 0x00, 0x88, // STA $8800

	0x4C, 0x00, 0xE0, // JMP loop
}

// Snow — fills the framebuffer with pseudo-random colored chars
// from an 8-bit Galois LFSR (taps = $B8), pauses, clears via
// controller, repeats. Two passes (256 cells each) cover the first
// 512 cells of the display; the last 8 cells of row 12 remain blank.
var Snow = []uint8{
	// Seed init @ $E000.
	0xA9, 0x01, 0x85, 0x00, // LDA #1 ; STA $00

	// Outer loop @ $E004
	0xA2, 0x00, // LDX #0

	// Pass 1 @ $E006 — cells 0..255 (color $8200..$82FF, char $8500..$85FF).
	0xA5, 0x00, // LDA $00
	0x4A,             // LSR A
	0x90, 0x02,       // BCC +2
	0x49, 0xB8,       // EOR #$B8
	0x85, 0x00,       // STA $00
	0x9D, 0x00, 0x82, // STA $8200,X
	0x49, 0x5A,       // EOR #$5A
	0x9D, 0x00, 0x85, // STA $8500,X
	0xE8,             // INX
	0xD0, 0xEC,       // BNE pass1 (-20)

	// Pass 2 setup @ $E01A — cells 256..511 (color $8300..$83FF, char $8600..$86FF).
	0xA2, 0x00, // LDX #0
	0xA5, 0x00, // LDA $00
	0x4A,             // LSR A
	0x90, 0x02,       // BCC +2
	0x49, 0xB8,       // EOR #$B8
	0x85, 0x00,       // STA $00
	0x9D, 0x00, 0x83, // STA $8300,X
	0x49, 0x5A,       // EOR #$5A
	0x9D, 0x00, 0x86, // STA $8600,X
	0xE8,             // INX
	0xD0, 0xEC,       // BNE pass2 (-20)

	// Delay @ $E030.
	0xA0, 0x10, // LDY #$10
	0xA2, 0xFF, // LDX #$FF
	0xCA,       // DEX
	0xD0, 0xFD, // BNE d2 (-3)
	0x88,       // DEY
	0xD0, 0xF8, // BNE d1 (-8)

	// Clear via controller, then repeat.
	0xA9, 0x01, // LDA #1
	0x8D, 0x00, 0x88, // STA $8800
	0x4C, 0x04, 0xE0, // JMP outer
}

// Blitter — classic double-buffer pattern. Build a 256-byte image
// in off-screen RAM at $1000, then copy to both VIC planes, then
// fire RegFrame. The user only ever sees fully-rendered frames.
//
//	$00     = counter, increments per frame
//	$1000   = 256-byte off-screen buffer
//	$0200   = color plane (first 256 cells get the buffer; rest stays
//	          at host init)
//	$0500   = char plane  (each byte is buffer[X] mapped into '@'..'_')
//
//	$E000  LDA #1    STA $0801   ; pause
//	$E005  INC $00                       ← loop
//	$E007  LDX #0
//	$E009  TXA ; CLC ; ADC $00 ; STA $1000,X ; INX ; BNE     ← build
//	$E013  LDX #0
//	$E015  LDA $1000,X ; STA $0200,X ; INX ; BNE             ← blit color
//	$E01E  LDX #0
//	$E020  LDA $1000,X ; AND #$1F ; CLC ; ADC #'@' ; STA $0500,X
//	       INX ; BNE                                          ← blit char
//	$E02E  STA $0802                                          ← frame
//	$E031  JMP loop
var Blitter = []uint8{
	// Pause once @ $8801.
	0xA9, 0x01, 0x8D, 0x01, 0x88, // LDA #1 ; STA $8801

	// Loop: bump counter, init X.
	0xE6, 0x00, // INC $00
	0xA2, 0x00, // LDX #0

	// Build off-screen image: buffer[X] = X + counter, in RAM at $1000.
	0x8A,             // TXA
	0x18,             // CLC
	0x65, 0x00,       // ADC $00
	0x9D, 0x00, 0x10, // STA $1000,X (RAM is one big block, scratch lives at $1000)
	0xE8,             // INX
	0xD0, 0xF6,       // BNE build (-10)

	// Blit to color plane.
	0xA2, 0x00, // LDX #0
	0xBD, 0x00, 0x10, // LDA $1000,X
	0x9D, 0x00, 0x82, // STA $8200,X
	0xE8,             // INX
	0xD0, 0xF7,       // BNE blit_color (-9)

	// Blit to char plane (mapped into '@'..'_').
	0xA2, 0x00, // LDX #0
	0xBD, 0x00, 0x10, // LDA $1000,X
	0x29, 0x1F,       // AND #$1F
	0x18,             // CLC
	0x69, 0x40,       // ADC #'@'
	0x9D, 0x00, 0x85, // STA $8500,X
	0xE8,             // INX
	0xD0, 0xF2,       // BNE blit_char (-14)

	// Frame trigger — commit the snapshot.
	0x8D, 0x02, 0x88, // STA $8802

	0x4C, 0x05, 0xE0, // JMP loop
}

// ScrollerFramed — uses the new VIC pause/frame registers.
// Order: shift-up FIRST (so the bottom row gets blanked), then fill
// the bottom row, then commit the frame. Every snapshot the UI sees
// has a populated bottom row — no flicker, no black gap.
//
//	$0800 = command register   ($02 = ShiftUp)
//	$0801 = pause state        (1 = paused)
//	$0802 = frame trigger      (any write = snapshot)
var ScrollerFramed = []uint8{
	// Pause once @ $8801.
	0xA9, 0x01, 0x8D, 0x01, 0x88, // LDA #1 ; STA $8801

	// Loop @ $E005 — shift first.
	0xA9, 0x02, 0x8D, 0x00, 0x88, // LDA #$02 ; STA $8800 (CmdShiftUp)

	0xE6, 0x00, // INC $00
	0xA2, 0x00, // LDX #0

	// Fill bottom row @ $E00E.
	0x8A,             // TXA
	0x18,             // CLC
	0x65, 0x00,       // ADC $00
	0x9D, 0xE0, 0x83, // STA $83E0,X
	0x9D, 0xE0, 0x86, // STA $86E0,X
	0xE8,             // INX
	0xE0, 0x28,       // CPX #40
	0xD0, 0xF1,       // BNE fill (-15 from $E01D → $E00E)

	// Frame trigger.
	0x8D, 0x02, 0x88, // STA $8802

	0x4C, 0x05, 0xE0, // JMP loop
}

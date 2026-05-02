package demos

// Graphics demos — drive the VIC's optional graphics plane via the
// new CmdGfx* commands. These programs assume a graphics plane is
// attached to the controller (NewControllerWithGraphics); on a
// terminal build without one the commands are silently no-ops.
//
// Controller register layout:
//
//	$8800  Cmd
//	$8803  RectX
//	$8804  RectY
//	$8805  RectW    (also: line X-end, circle radius)
//	$8806  RectH    (also: line Y-end)
//	$8807  GfxColor (palette index 0..15)
//	$8808  Mode     (0 = char, 1 = graphics)
//
// Graphics commands:
//
//	$20  CmdGfxClear
//	$21  CmdGfxPlot
//	$22  CmdGfxLine
//	$23  CmdGfxRectFill
//	$24  CmdGfxRectStroke
//	$25  CmdGfxCircle
//	$26  CmdGfxFillCircle

// BouncingBalls — four colored balls bouncing around the 160×104
// graphics plane. Each ball has position, velocity, and color in
// zero page; on each wall hit, velocity reverses and color advances
// to the next palette slot. The whole frame is cleared and redrawn
// each iteration.
//
// Zero-page layout:
//
//	$10..$13   ball X positions (x0..x3)
//	$14..$17   ball Y positions (y0..y3)
//	$18..$1B   ball X velocities (signed: 0x01 = +1, 0xFF = -1)
//	$1C..$1F   ball Y velocities
//	$20..$23   ball palette indices
//
// Bounds: balls keep a 4-pixel radius clear of the edges, so X is
// constrained to [4, 155] and Y to [4, 99].
var BouncingBalls = buildBouncingBalls()

func buildBouncingBalls() []byte {
	a := newAsm(0xE000)

	const (
		regCmd      uint16 = 0x8800
		regRectX    uint16 = 0x8803
		regRectY    uint16 = 0x8804
		regRectW    uint16 = 0x8805
		regGfxColor uint16 = 0x8807
		regMode     uint16 = 0x8808

		cmdGfxClear      byte = 0x20
		cmdGfxFillCircle byte = 0x26

		ballRadius byte = 4
		numBalls   byte = 4
		minXY      byte = 4
		maxX      byte = 155 // 159 - radius
		maxY      byte = 99  // 103 - radius
	)

	// === ENTRY ===

	// Switch the VIC into graphics mode.
	a.lda_imm(0x01)
	a.sta_abs(regMode)

	// Initialise four balls. Positions chosen to avoid initial overlap;
	// velocities a mix of ±1 in each axis; colors picked to look good
	// on the dark-blue clear background.
	initX := []byte{20, 80, 130, 50}
	initY := []byte{20, 40, 70, 85}
	initVX := []byte{0x01, 0xFF, 0x01, 0xFF} // +1, -1, +1, -1
	initVY := []byte{0x01, 0x01, 0xFF, 0xFF} // +1, +1, -1, -1
	initC := []byte{0x04, 0x02, 0x0E, 0x0B}  // red, green, yellow, lt-cyan

	for i := byte(0); i < numBalls; i++ {
		a.lda_imm(initX[i])
		a.sta_zp(0x10 + i)
		a.lda_imm(initY[i])
		a.sta_zp(0x14 + i)
		a.lda_imm(initVX[i])
		a.sta_zp(0x18 + i)
		a.lda_imm(initVY[i])
		a.sta_zp(0x1C + i)
		a.lda_imm(initC[i])
		a.sta_zp(0x20 + i)
	}

	// === MAIN LOOP ===
	a.label("MAIN")

	// --- Clear the plane to dark blue. ---
	a.lda_imm(0x01) // palette: blue
	a.sta_abs(regGfxColor)
	a.lda_imm(cmdGfxClear)
	a.sta_abs(regCmd)

	// --- Draw all balls. ---
	a.ldx_imm(0)
	a.label("DRAW")
	a.lda_zpx(0x10) // x = ball.x
	a.sta_abs(regRectX)
	a.lda_zpx(0x14) // y = ball.y
	a.sta_abs(regRectY)
	a.lda_imm(ballRadius)
	a.sta_abs(regRectW)
	a.lda_zpx(0x20) // color = ball.color
	a.sta_abs(regGfxColor)
	a.lda_imm(cmdGfxFillCircle)
	a.sta_abs(regCmd)
	a.inx()
	a.cpx_imm(numBalls)
	a.bne("DRAW")

	// --- Update positions + bounce off walls. ---
	a.ldx_imm(0)
	a.label("UPDATE")

	// x += vx
	a.clc()
	a.lda_zpx(0x10)
	a.adc_zpx(0x18)
	a.sta_zpx(0x10)

	// if x < minXY: bounce off left wall
	a.cmp_imm(minXY)
	a.bcs("X_LEFT_OK")
	a.lda_zpx(0x18) // vx
	a.eor_imm(0xFE) // 0x01 <-> 0xFF
	a.sta_zpx(0x18)
	a.lda_imm(minXY)
	a.sta_zpx(0x10)
	a.inc_zpx(0x20) // bump color
	a.label("X_LEFT_OK")

	// if x > maxX: bounce off right wall
	a.lda_zpx(0x10)
	a.cmp_imm(maxX + 1)
	a.bcc("X_RIGHT_OK")
	a.lda_zpx(0x18)
	a.eor_imm(0xFE)
	a.sta_zpx(0x18)
	a.lda_imm(maxX)
	a.sta_zpx(0x10)
	a.inc_zpx(0x20)
	a.label("X_RIGHT_OK")

	// y += vy
	a.clc()
	a.lda_zpx(0x14)
	a.adc_zpx(0x1C)
	a.sta_zpx(0x14)

	// if y < minXY: bounce off top
	a.cmp_imm(minXY)
	a.bcs("Y_TOP_OK")
	a.lda_zpx(0x1C)
	a.eor_imm(0xFE)
	a.sta_zpx(0x1C)
	a.lda_imm(minXY)
	a.sta_zpx(0x14)
	a.inc_zpx(0x20)
	a.label("Y_TOP_OK")

	// if y > maxY: bounce off bottom
	a.lda_zpx(0x14)
	a.cmp_imm(maxY + 1)
	a.bcc("Y_BOT_OK")
	a.lda_zpx(0x1C)
	a.eor_imm(0xFE)
	a.sta_zpx(0x1C)
	a.lda_imm(maxY)
	a.sta_zpx(0x14)
	a.inc_zpx(0x20)
	a.label("Y_BOT_OK")

	a.inx()
	a.cpx_imm(numBalls)
	a.bne("UPDATE")

	// --- Frame delay so motion is paced. The Y init value is the
	//     primary tuning knob: 0xFF gives ~65K cycles per frame
	//     (~smooth motion at typical interp speed), 0x80 doubles
	//     speed, 0x40 quadruples. The outer counter is 1 here —
	//     bump if you want longer pauses between frames.
	a.lda_imm(0x01)
	a.sta_zp(0x00) // outer counter
	a.label("DELAY_OUTER")
	a.ldy_imm(0x10) // tune for visible-but-quick ball motion
	a.label("DELAY_Y")
	a.ldx_imm(0xFF)
	a.label("DELAY_X")
	a.dex()
	a.bne("DELAY_X")
	a.dey()
	a.bne("DELAY_Y")
	a.emit(0xC6, 0x00) // DEC $00 — no helper for zp DEC yet
	a.bne("DELAY_OUTER")

	// X register was clobbered by the delay; the redraw loop at MAIN
	// re-initialises it to 0, so we can fall straight back through.
	a.jmp("MAIN")

	return a.build()
}

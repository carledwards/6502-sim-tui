package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	foxpro "github.com/carledwards/foxpro-go"

	"github.com/carledwards/6502-sim-tui/bus"
	"github.com/carledwards/6502-sim-tui/components/display"
	"github.com/carledwards/6502-sim-tui/components/ram"
	"github.com/carledwards/6502-sim-tui/components/rom"
	"github.com/carledwards/6502-sim-tui/cpu"
	"github.com/carledwards/6502-sim-tui/cpu/interp"
	"github.com/carledwards/6502-sim-tui/cpu/netsim"
	"github.com/carledwards/6502-sim-tui/ui/clockwin"
	"github.com/carledwards/6502-sim-tui/ui/cpuwin"
	"github.com/carledwards/6502-sim-tui/ui/displaywin"
	"github.com/carledwards/6502-sim-tui/ui/ramwin"

	"github.com/gdamore/tcell/v2"
)

// Memory map. Modeled after a real 6502 machine: contiguous RAM in
// the bottom half, I/O up high, ROM at the top. The 8 KB RAM is one
// flat block — programs can use $0000-$00FF (zero page), $0100-$01FF
// (stack), and the rest as ordinary working memory.
//
// VIC bases are laid out so that each is a uniform +$8000 offset
// from the equivalent in older builds. That keeps demo addresses
// translatable by changing just the high nibble of the high byte
// ($02 → $82, $05 → $85, $08 → $88), and matches the C64-style
// "I/O lives high" convention.
const (
	ramBase   = 0x0000
	ramSize   = 0x2000 // 8 KB at $0000-$1FFF
	colorBase = 0x8200 // VIC color plane (520 bytes)
	charBase  = 0x8500 // VIC char plane  (520 bytes)
	ctrlBase  = 0x8800 // VIC controller registers (3 bytes)
	dispW     = 40
	dispH     = 13
	romBase   = 0xE000
	romSize   = 0x2000
)

// demo is one selectable ROM payload. Each demo starts at $E000;
// the reset vector is wired to point there. Switching demos at
// runtime: clear the ROM, load the new bytes, set the reset vector,
// repaint the host-side display init pattern, then Reset the CPU.
type demo struct {
	name  string
	bytes []uint8
}

// Memory map a demo author should know:
//   $0000-$1FFF  RAM (8 KB)
//   $8000+       VIC color plane  (40 × 13 = 520 bytes)
//   $8500+       VIC char plane   (520 bytes)
//   $8800-$8802  VIC controller   (cmd / pause / frame)
//   $E000-$FFFF  ROM (program loaded here, reset vector at $FFFC)

// Demo program — "HELLO 6502 SIM" marquee.
//
// Setup phase:
//   1. Clear the display via the controller.
//   2. Copy a 14-byte message into row 6 of the char plane
//      ($05F0..$05FD) and paint that range navy-bg/yellow-fg in the
//      color plane ($02F0..$02FD).
//
// Steady state:
//   loop forever: poke CmdRotLeft into $0800.
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
var demoProg = []uint8{
	// Clear screen via controller @ $8800.
	0xA9, 0x01,
	0x8D, 0x00, 0x88,

	// Copy message + paint color into row 6.
	0xA2, 0x00, // LDX #$00
	0xBD, 0x1F, 0xE0, // LDA $E01F,X      (msg+X)
	0x9D, 0xF0, 0x85, // STA $85F0,X      (char plane row 6)
	0xA9, 0x1E, //       LDA #$1E         (bg=navy, fg=yellow)
	0x9D, 0xF0, 0x82, // STA $82F0,X      (color plane row 6)
	0xE8,             // INX
	0xE0, 0x0E,       // CPX #14
	0xD0, 0xF0,       // BNE -16          (back to LDA msg)

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
var bouncerProg = []uint8{
	// Setup: clear, init x=20, dx=+1.
	0xA9, 0x01, 0x8D, 0x00, 0x88, // LDA #$01 ; STA $8800   (clear)
	0xA2, 0x14, 0x86, 0x00,       // LDX #20  ; STX $00     (x=20)
	0xA9, 0x01, 0x85, 0x01,       // LDA #1   ; STA $01     (dx=+1)

	// Loop @ $E00D
	0xA6, 0x00,                   // LDX $00
	0xA9, 0x20, 0x9D, 0xF0, 0x85, // LDA #' ' ; STA $85F0,X  (erase char)
	0xA9, 0x00, 0x9D, 0xF0, 0x82, // LDA #0   ; STA $82F0,X  (erase color)

	// Move: dx>=0 → INX, else DEX.
	0xA5, 0x01, 0x10, 0x04,       // LDA $01 ; BPL +4
	0xCA, 0x4C, 0x22, 0xE0,       // DEX ; JMP $E022
	0xE8,                         // INX
	0x86, 0x00,                   // STX $00

	// Draw: '*' at row 6 col x.
	0xA9, 0x2A, 0x9D, 0xF0, 0x85, // LDA #'*' ; STA $85F0,X
	0xA9, 0x1E, 0x9D, 0xF0, 0x82, // LDA #$1E ; STA $82F0,X (navy/yellow)

	// Delay — Y counts down from $FF, ~256 iterations.
	0xA0, 0xFF,                   // LDY #$FF
	0x88, 0xD0, 0xFD,             // DEY ; BNE -3

	// Bounds: if X==0 or X==39, jump to flip @ $E03E.
	0xE0, 0x00, 0xF0, 0x07,       // CPX #0  ; BEQ +7
	0xE0, 0x27, 0xF0, 0x03,       // CPX #39 ; BEQ +3
	0x4C, 0x0D, 0xE0,             // JMP loop

	// Flip @ $E03E: dx = -dx.
	0xA5, 0x01, 0x49, 0xFF,       // LDA $01 ; EOR #$FF
	0x18, 0x69, 0x01,             // CLC ; ADC #1
	0x85, 0x01,                   // STA $01
	0x4C, 0x0D, 0xE0,             // JMP loop
}

// Scroller — fills the bottom row (row 12, offsets $1E0..$207) with
// a varying gradient (cell value = X + counter), then pokes
// CmdShiftUp. Each iteration: counter++, fill, shift. Result is a
// continuously flowing diagonal pattern climbing up the screen.
var scrollerProg = []uint8{
	// Loop @ $E000
	0xE6, 0x00,             // INC $00              (counter++)
	0xA2, 0x00,             // LDX #0

	// Fill bottom row @ $E004
	0x8A,                   // TXA
	0x18,                   // CLC
	0x65, 0x00,             // ADC $00              (A = X + counter)
	0x9D, 0xE0, 0x83,       // STA $83E0,X          (color[bottomrow + X])
	0x9D, 0xE0, 0x86,       // STA $86E0,X          (char[bottomrow + X])
	0xE8,                   // INX
	0xE0, 0x28,             // CPX #40
	0xD0, 0xF1,             // BNE fill (-15)

	// Shift up via controller.
	0xA9, 0x02,             // LDA #2               (CmdShiftUp)
	0x8D, 0x00, 0x88,       // STA $8800

	0x4C, 0x00, 0xE0,       // JMP loop
}

// Snow — fills the framebuffer with pseudo-random colored chars
// from an 8-bit Galois LFSR (taps = $B8), pauses, clears via
// controller, repeats. Two passes (256 cells each) cover the first
// 512 cells of the display; the last 8 cells of row 12 remain blank.
var snowProg = []uint8{
	// Seed init @ $E000.
	0xA9, 0x01, 0x85, 0x00,        // LDA #1 ; STA $00

	// Outer loop @ $E004
	0xA2, 0x00,                    // LDX #0

	// Pass 1 @ $E006 — cells 0..255 (color $8200..$82FF, char $8500..$85FF).
	0xA5, 0x00,                    // LDA $00
	0x4A,                          // LSR A
	0x90, 0x02,                    // BCC +2
	0x49, 0xB8,                    // EOR #$B8
	0x85, 0x00,                    // STA $00
	0x9D, 0x00, 0x82,              // STA $8200,X
	0x49, 0x5A,                    // EOR #$5A
	0x9D, 0x00, 0x85,              // STA $8500,X
	0xE8,                          // INX
	0xD0, 0xEC,                    // BNE pass1 (-20)

	// Pass 2 setup @ $E01A — cells 256..511 (color $8300..$83FF, char $8600..$86FF).
	0xA2, 0x00,                    // LDX #0
	0xA5, 0x00,                    // LDA $00
	0x4A,                          // LSR A
	0x90, 0x02,                    // BCC +2
	0x49, 0xB8,                    // EOR #$B8
	0x85, 0x00,                    // STA $00
	0x9D, 0x00, 0x83,              // STA $8300,X
	0x49, 0x5A,                    // EOR #$5A
	0x9D, 0x00, 0x86,              // STA $8600,X
	0xE8,                          // INX
	0xD0, 0xEC,                    // BNE pass2 (-20)

	// Delay @ $E030.
	0xA0, 0x10,                    // LDY #$10
	0xA2, 0xFF,                    // LDX #$FF
	0xCA,                          // DEX
	0xD0, 0xFD,                    // BNE d2 (-3)
	0x88,                          // DEY
	0xD0, 0xF8,                    // BNE d1 (-8)

	// Clear via controller, then repeat.
	0xA9, 0x01,                    // LDA #1
	0x8D, 0x00, 0x88,              // STA $8800
	0x4C, 0x04, 0xE0,              // JMP outer
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
var blitterProg = []uint8{
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
	0xA2, 0x00,       // LDX #0
	0xBD, 0x00, 0x10, // LDA $1000,X
	0x9D, 0x00, 0x82, // STA $8200,X
	0xE8,             // INX
	0xD0, 0xF7,       // BNE blit_color (-9)

	// Blit to char plane (mapped into '@'..'_').
	0xA2, 0x00,       // LDX #0
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

// Scroller (framed) — uses the new VIC pause/frame registers.
// Order: shift-up FIRST (so the bottom row gets blanked), then fill
// the bottom row, then commit the frame. Every snapshot the UI sees
// has a populated bottom row — no flicker, no black gap.
//
//	$0800 = command register   ($02 = ShiftUp)
//	$0801 = pause state        (1 = paused)
//	$0802 = frame trigger      (any write = snapshot)
var scrollerFramedProg = []uint8{
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

// demoSection is a labelled group of demos shown in the Demo menu.
// Sections are separated by a Separator menu item.
type demoSection struct {
	demos []demo
}

// demoSections — first group is "live" (UI updates as memory changes),
// second is "framed" (UI shows snapshot, CPU controls when to commit).
var demoSections = []demoSection{
	{[]demo{
		{"&Marquee (default)", demoProg},
		{"&Bouncer", bouncerProg},
		{"&Scroller", scrollerProg},
		{"S&now (LFSR)", snowProg},
	}},
	{[]demo{
		{"Scroller (&framed)", scrollerFramedProg},
		{"&Blitter (RAM→VIC)", blitterProg},
		{"&Quadrants (4 scrolls)", quadProg},
	}},
}

// tuneCandidates are the batch sizes auto-tune tries in order. They
// are already round numbers, so the picked value is also "memorable"
// — no separate rounding step needed.
var tuneCandidates = []int{500, 1000, 1500, 2000, 2500, 3000, 4000, 5000, 7500, 10000, 20000, 50000, 100000}

// autoTune runs increasing-size batches against the backend and
// returns the largest size that fit inside `budget`. Conservative
// by design: budget < tickPeriod leaves UI headroom.
//
// Mutates backend state (advances cycles); the caller should Reset
// the CPU after.
func autoTune(backend cpu.Backend, budget time.Duration) int {
	best := tuneCandidates[0]
	for _, n := range tuneCandidates {
		start := time.Now()
		for i := 0; i < n; i++ {
			backend.HalfStep()
		}
		elapsed := time.Since(start)
		if elapsed <= budget {
			best = n
			continue
		}
		break // batches will only get slower; stop
	}
	return best
}

const tickPeriod = 50 * time.Millisecond

func main() {
	cpuFlag := flag.String("cpu", "netsim", "CPU backend: netsim or interp")
	runFlag := flag.Bool("run", false, "start the clock running immediately")
	speedFlag := flag.String("speed", "", "starting clock speed: 1, 10, 100, 1k (or 1000), max")
	batchFlag := flag.Int("batch", 0, "max HalfSteps per UI tick (0 = default 500). Raise for interp; lower if UI is janky.")
	cpuProfile := flag.String("cpuprofile", "", "write CPU profile to file (active for the lifetime of the process)")
	memProfile := flag.String("memprofile", "", "write heap profile to file at exit")
	flag.Parse()

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatalf("cpuprofile create: %v", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatalf("cpuprofile start: %v", err)
		}
		defer pprof.StopCPUProfile()
	}
	if *memProfile != "" {
		defer func() {
			f, err := os.Create(*memProfile)
			if err != nil {
				log.Printf("memprofile create: %v", err)
				return
			}
			defer f.Close()
			runtime.GC()
			if err := pprof.WriteHeapProfile(f); err != nil {
				log.Printf("memprofile write: %v", err)
			}
		}()
	}

	// Bus + memory map. The outer TraceBus stamps each read/write with
	// a generation counter so the memory viewer can tint cells that
	// have been touched recently. The inner bus is what the memory
	// viewer's display reads use, so its own polling doesn't pollute
	// the trace.
	innerBus := bus.New()
	b := bus.NewTraceBus(innerBus)
	mainRAM := ram.New("ram", ramBase, ramSize)
	colorPlane := display.New("display.color", colorBase, dispW, dispH)
	charPlane := display.New("display.char", charBase, dispW, dispH)
	mainROM := rom.New("rom", romBase, romSize)

	// paintInitialDisplay seeds the framebuffer with a diagonal-gradient
	// background so there's something to see before any program runs.
	// Also called when switching demos to give a clean canvas.
	paintInitialDisplay := func() {
		for y := 0; y < dispH; y++ {
			for x := 0; x < dispW; x++ {
				colorPlane.SetPixel(x, y, uint8(((x+y)%16)<<4))
				charPlane.SetPixel(x, y, 0x20)
			}
		}
	}
	paintInitialDisplay()

	dispCtrl := display.NewController("display.ctrl", ctrlBase, colorPlane, charPlane)

	must(mainROM.Load(0, demoProg))
	must(mainROM.SetResetVector(0xE000))
	must(b.Register(mainRAM))
	must(b.Register(colorPlane))
	must(b.Register(charPlane))
	must(b.Register(dispCtrl))
	must(b.Register(mainROM))

	// CPU backend — mutable so the CPU menu can swap it at runtime.
	buildBackend := func(name string) (cpu.Backend, error) {
		switch name {
		case "netsim":
			return netsim.New(b)
		case "interp":
			return interp.New(b), nil
		}
		return nil, fmt.Errorf("unknown cpu %q (want netsim or interp)", name)
	}

	backend, err := buildBackend(*cpuFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	backend.Reset()
	currentCPU := *cpuFlag
	cpuTitle := fmt.Sprintf("CPU (%s)", currentCPU)

	// foxpro-go app.
	app, err := foxpro.NewApp()
	if err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		os.Exit(1)
	}
	defer app.Close()

	// Track every window we create so we can toggle visibility from
	// a Window menu after the user closes one.
	var windows []*foxpro.Window
	addWindow := func(title string, bounds foxpro.Rect, content foxpro.ContentProvider, minW, minH int) *foxpro.Window {
		w := foxpro.NewWindow(title, bounds, content)
		w.MinW = minW
		w.MinH = minH
		app.Manager.Add(w)
		windows = append(windows, w)
		return w
	}

	pcHighlight := func() (uint16, bool) {
		return backend.Registers().PC, true
	}

	cpuProv := &cpuwin.Provider{Backend: backend}
	cpuWindow := addWindow(cpuTitle,
		foxpro.Rect{X: 2, Y: 1, W: 38, H: 13},
		cpuProv,
		cpuwin.MinW, cpuwin.MinH)

	ramProv := &ramwin.Provider{
		Bus:          innerBus, // read display state without tracing it
		Trace:        b,
		Backend:      backend,
		Base:         0x0000,
		Length:       0x100,
		Highlight:    pcHighlight,
		EditableBase: true,
	}
	memWin := addWindow("Memory",
		foxpro.Rect{X: 42, Y: 1, W: 76, H: 14},
		ramProv,
		ramwin.MinW, ramwin.MinH)
	ramProv.Window = memWin

	romProv := &ramwin.Provider{
		Bus:          innerBus,
		Trace:        b,
		Backend:      backend,
		Base:         romBase,
		Length:       romSize,
		Highlight:    pcHighlight,
		EditableBase: true,
		View:         ramwin.ViewDisasm,
	}
	romWin := addWindow("Memory",
		foxpro.Rect{X: 42, Y: 16, W: 76, H: 8},
		romProv,
		ramwin.MinW, ramwin.MinH)
	romProv.Window = romWin

	clockProv := clockwin.NewProvider(backend)
	clockProv.MaxBatch = *batchFlag
	addWindow("Clock",
		foxpro.Rect{X: 2, Y: 13, W: 38, H: 7},
		clockProv,
		clockwin.MinW, clockwin.MinH)

	// machineReset = full simulated-machine restart: stop clock, drop
	// VIC pause, clear RAM, repaint display, reset CPU. ROM stays
	// loaded with the current demo so reset starts that demo over.
	machineReset := func() {
		clockProv.SetRunning(false)
		b.Write(ctrlBase+display.RegPause, 0)
		mainRAM.Reset()
		paintInitialDisplay()
		clockProv.Reset()
	}
	cpuProv.OnReset = machineReset


	dispProv := &displaywin.Provider{
		// inner bus so the window's own hex-dump reads don't pollute
		// the read-trace. Component dispatch is identical — every
		// component is registered on the inner bus via TraceBus's
		// delegating Register, and button POKEs to $8800 still hit
		// the controller normally; they just aren't shown in the
		// per-cell trace tinting.
		Bus:        innerBus,
		Controller: dispCtrl,
		ColorBase:  colorBase,
		CharBase:   charBase,
		CtrlBase:   ctrlBase,
		HasChars:   true,
		HasCtrl:    true,
		Width:      dispW,
		Height:     dispH,

		// Hex strip can only scroll across the VIC's own memory:
		// color plane through the last controller register.
		MemRangeStart: colorBase,
		MemRangeEnd:   ctrlBase + 6,
	}
	// Layout: display + button column on top, hex-dump half-box below.
	// Half-box adds left │ + scrollbar on right and top/bottom rules:
	//   1 (│) + labelW (7) + hex (48) + gap (2) + ascii (16) + 1 (▲)
	//   = 75 cols inner → outer 77.
	// Heights: 17 (display incl. frames) + box top (1) + header (1)
	//   + 7 data + box bottom (1) = 27 inner → outer 29.
	dispTitle := fmt.Sprintf("VIC $%04X-$%04X", colorBase, ctrlBase+6)
	addWindow(dispTitle,
		foxpro.Rect{X: 60, Y: 1, W: 77, H: 29},
		dispProv,
		displaywin.MinW, displaywin.MinH)

	// Run loop. App.Tick fires on the UI thread, so simulator
	// advancement, register reads, and bus reads all serialize
	// naturally — no locks needed.
	app.Tick(tickPeriod, func() {
		clockProv.Advance(tickPeriod)
		b.Tick() // age the read/write trace
	})

	// Global key bindings. Active in any focused window so the user
	// can drive the simulator without first focusing the Clock window.
	app.OnKey = func(ev *tcell.EventKey) bool {
		if ev.Key() != tcell.KeyRune {
			return false
		}
		switch ev.Rune() {
		case 'r', 'R':
			clockProv.SetRunning(true)
			return true
		case '.':
			clockProv.SetRunning(false)
			return true
		case 's', 'S':
			clockProv.StepInstruction()
			return true
		case 't', 'T':
			clockProv.StepOne()
			return true
		case 'z', 'Z':
			machineReset()
			return true
		case '<', ',':
			clockProv.CycleSpeed(-1)
			return true
		case '>', '/':
			clockProv.CycleSpeed(1)
			return true
		}
		return false
	}

	// loadDemo swaps in a different ROM payload and resets the CPU.
	// Also pokes CmdResume into the display controller so a previous
	// framed demo's pause state doesn't leak into a live demo.
	loadDemo := func(d demo) {
		clockProv.SetRunning(false)
		// Resume the VIC so a previous framed demo's pause state
		// doesn't leak into a live demo.
		b.Write(ctrlBase+display.RegPause, 0)
		mainROM.Clear()
		_ = mainROM.Load(0, d.bytes)
		_ = mainROM.SetResetVector(0xE000)
		paintInitialDisplay()
		clockProv.Reset()
	}

	// switchCPU swaps the CPU backend at runtime. The bus stays the
	// same — RAM, display, and ROM contents are preserved across the
	// switch — so the freshly-Reset CPU starts from $E000 against the
	// existing memory map.
	switchCPU := func(name string) {
		if name == currentCPU {
			return
		}
		clockProv.SetRunning(false)
		newBackend, err := buildBackend(name)
		if err != nil {
			return
		}
		newBackend.Reset()
		backend = newBackend
		clockProv.Backend = newBackend
		cpuProv.Backend = newBackend
		ramProv.Backend = newBackend
		romProv.Backend = newBackend
		currentCPU = name
		cpuWindow.Title = fmt.Sprintf("CPU (%s)", name)
	}

	demoItems := []foxpro.MenuItem{}
	for sIdx, sec := range demoSections {
		if sIdx > 0 {
			demoItems = append(demoItems, foxpro.MenuItem{Separator: true})
		}
		for _, d := range sec.demos {
			d := d
			demoItems = append(demoItems, foxpro.MenuItem{
				Label:    d.name,
				OnSelect: func() { loadDemo(d) },
			})
		}
	}

	// Window menu — toggle visibility for each window we created.
	// Closing a window via the ■ glyph removes it from the manager
	// but keeps the *foxpro.Window alive (we hold a reference here),
	// so toggling adds the same instance back with its scroll
	// position and other state intact.
	windowItems := make([]foxpro.MenuItem, 0, len(windows))
	for _, w := range windows {
		w := w
		windowItems = append(windowItems, foxpro.MenuItem{
			Label: w.Title,
			OnSelect: func() {
				if app.Manager.Contains(w) {
					app.Manager.Remove(w)
				} else {
					app.Manager.Add(w)
				}
			},
		})
	}

	app.MenuBar = foxpro.NewMenuBar([]foxpro.Menu{
		{
			Label: "&File",
			Items: []foxpro.MenuItem{
				{Label: "&Reset Machine", Hotkey: "Z", OnSelect: machineReset},
				{Label: "&Command Window", Hotkey: "F2", OnSelect: app.ToggleCommandWindow},
				{Separator: true},
				{Label: "E&xit", Hotkey: "Esc", OnSelect: app.Quit},
			},
		},
		{
			Label: "&Run",
			Items: []foxpro.MenuItem{
				{Label: "R&un", Hotkey: "R", OnSelect: func() { clockProv.SetRunning(true) }},
				{Label: "S&top", Hotkey: ".", OnSelect: func() { clockProv.SetRunning(false) }},
				{Label: "&Step instruction", Hotkey: "S", OnSelect: clockProv.StepInstruction},
				{Label: "&Tick (½ cycle)", Hotkey: "T", OnSelect: clockProv.StepOne},
			},
		},
		{
			Label: "&CPU",
			Items: []foxpro.MenuItem{
				{Label: "&Interpretive", OnSelect: func() { switchCPU("interp") }},
				{Label: "&Transistor (netsim)", OnSelect: func() { switchCPU("netsim") }},
				{Separator: true},
				{Label: "Auto-&tune Batch", OnSelect: func() {
					clockProv.SetRunning(false)
					// Budget: 70% of the 50ms tick for UI headroom.
					best := autoTune(backend, 35*time.Millisecond)
					clockProv.MaxBatch = best
					clockProv.Reset()
				}},
			},
		},
		{
			Label: "&Demo",
			Items: demoItems,
		},
		{
			Label: "&Window",
			Items: windowItems,
		},
	})

	// Live tray — top-right of the menu bar. Compute fns run every
	// frame, so the rate updates as the sim runs.
	app.MenuBar.Tray = []foxpro.TrayItem{
		{Compute: func() string {
			if clockProv.Running() {
				return fmt.Sprintf("● running %s", cpuwin.FormatHz(cpuProv.Rate()))
			}
			return "○ stopped"
		}},
		{Compute: func() string {
			return fmt.Sprintf("batch: %d", clockProv.EffectiveBatch())
		}},
		{Compute: func() string {
			return fmt.Sprintf("CPU: %s", currentCPU)
		}},
	}

	if *speedFlag != "" {
		hz := -1
		switch *speedFlag {
		case "1":
			hz = 1
		case "10":
			hz = 10
		case "100":
			hz = 100
		case "1k", "1000":
			hz = 1000
		case "max", "0":
			hz = 0
		}
		if hz < 0 || !clockProv.SetSpeedHz(hz) {
			fmt.Fprintf(os.Stderr, "unknown -speed=%q (want 1, 10, 100, 1k, max)\n", *speedFlag)
			os.Exit(2)
		}
	}

	if *runFlag {
		clockProv.SetRunning(true)
	}

	app.Run()
}

func must(err error) {
	if err != nil {
		log.Fatalf("setup: %v", err)
	}
}

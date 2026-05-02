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

	"github.com/carledwards/6502-sim-tui/asm"
	"github.com/carledwards/6502-sim-tui/bus"
	"github.com/carledwards/6502-sim-tui/components/display"
	"github.com/carledwards/6502-sim-tui/components/ram"
	"github.com/carledwards/6502-sim-tui/components/rom"
	"github.com/carledwards/6502-sim-tui/components/via"
	"github.com/carledwards/6502-sim-tui/cpu"
	"github.com/carledwards/6502-sim-tui/cpu/interp"
	"github.com/carledwards/6502-sim-tui/cpu/netsim"
	"github.com/carledwards/6502-sim-tui/internal/demos"
	"github.com/carledwards/6502-sim-tui/ui/clockwin"
	"github.com/carledwards/6502-sim-tui/ui/cpuwin"
	"github.com/carledwards/6502-sim-tui/ui/displaywin"
	"github.com/carledwards/6502-sim-tui/ui/ramwin"
	"github.com/carledwards/6502-sim-tui/ui/viawin"

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
	colorBase = 0xA000 // VIC color plane  (520 bytes, in 1 KB block)
	charBase  = 0xA400 // VIC char plane   (520 bytes, in 1 KB block)
	ctrlBase  = 0xA800 // VIC controller registers (16 bytes, in 1 KB CS block)
	viaBase   = 0xB000 // 6522 VIA #1 (own 256-byte CS; mirrors every 16 bytes)
	dispW     = 40
	dispH     = 13
	romBase   = 0xE000
	romSize   = 0x2000
)

// Memory map a demo author should know:
//   $0000-$1FFF  RAM (8 KB)
//   $8000+       VIC color plane  (40 × 13 = 520 bytes)
//   $8500+       VIC char plane   (520 bytes)
//   $8800-$8802  VIC controller   (cmd / pause / frame)
//   $E000-$FFFF  ROM (program loaded here, reset vector at $FFFC)


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
	// Defaults are tuned for "open the TUI, see the demo running".
	// Interp is fast enough to make the marquee look alive without
	// the user having to tweak anything; -cpu=netsim opts into the
	// transistor-level backend for visualization.
	cpuFlag := flag.String("cpu", "interp", "CPU backend: interp or netsim")
	runFlag := flag.Bool("run", true, "start the clock running immediately (default true)")
	speedFlag := flag.String("speed", "max", "starting clock speed: 1, 10, 100, 1k (or 1000), max")
	batchFlag := flag.Int("batch", 0, "max HalfSteps per UI tick (0 = auto-tune at startup based on the chosen backend)")
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

	// 6522 VIA #1 — clocked from its own 1 MHz oscillator. Demos use
	// Timer 1 in free-running mode to pace animation. Independent of
	// CPU clock, so it keeps running while stepping or paused — same
	// as a real W65C22S board with a separate timer crystal.
	via1 := via.New("via1", viaBase, 1_000_000)

	bootDemo := demos.MarqueeDemo
	must(mainROM.Load(0, bootDemo.Bytes))
	must(mainROM.SetResetVector(0xE000))
	must(b.Register(mainRAM))
	must(b.Register(colorPlane))
	must(b.Register(charPlane))
	must(b.Register(dispCtrl))
	must(b.Register(via1))
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

	// Opt in to foxpro's standard CLEAR / HELP / QUIT / VER command-window
	// commands. As of foxpro-go's switch to opt-in builtins, this call
	// is required to keep the F2 command window populated.
	foxpro.RegisterBuiltinCommands(app)

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

	// Hardware-symbol harvest from any bus.Labeller component (VIC,
	// VIA). Merged with each demo's program-local symbols so the
	// memory window's Labels view annotates both regions.
	hwSyms := []asm.Symbol{}
	for _, c := range innerBus.Components() {
		if l, ok := c.(bus.Labeller); ok {
			hwSyms = append(hwSyms, l.Symbols()...)
		}
	}
	mergeSymbols := func(demoSyms []asm.Symbol) []asm.Symbol {
		out := make([]asm.Symbol, 0, len(demoSyms)+len(hwSyms))
		out = append(out, demoSyms...)
		out = append(out, hwSyms...)
		return out
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
		Symbols:      mergeSymbols(bootDemo.Symbols),
		Annotations:  bootDemo.Annotations,
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
		Symbols:      mergeSymbols(bootDemo.Symbols),
		Annotations:  bootDemo.Annotations,
	}
	romWin := addWindow("Memory",
		foxpro.Rect{X: 42, Y: 16, W: 76, H: 8},
		romProv,
		ramwin.MinW, ramwin.MinH)
	romProv.Window = romWin

	clockProv := clockwin.NewProvider(backend)
	if *batchFlag > 0 {
		clockProv.MaxBatch = *batchFlag
	} else {
		// Auto-tune at startup: pick a batch size that lands the
		// per-tick cost at ~70% of the 50 ms UI tick. Keeps the UI
		// responsive while letting fast backends (interp) cruise.
		clockProv.MaxBatch = autoTune(backend, 35*time.Millisecond)
	}
	addWindow("Clock",
		foxpro.Rect{X: 2, Y: 13, W: 38, H: 7},
		clockProv,
		clockwin.MinW, clockwin.MinH)

	viaProv := &viawin.Provider{VIA: via1, Base: viaBase}
	addWindow("VIA #1",
		foxpro.Rect{X: 2, Y: 21, W: 56, H: 20},
		viaProv,
		viawin.MinW, viawin.MinH)

	// machineReset = full simulated-machine restart: drop VIC pause,
	// clear RAM, reset peripherals, repaint display, reset CPU. ROM
	// stays loaded with the current demo so reset starts it over.
	//
	// Modeled on a real hardware reset button: the clock keeps
	// running. If the user had the simulator running, it stays
	// running and the demo restarts immediately. If it was stopped,
	// it stays stopped until the user hits R.
	machineReset := func() {
		b.Write(ctrlBase+display.RegPause, 0)
		mainRAM.Reset()
		via1.Reset()
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
		// Stop at the last controller register — must NOT extend
		// into VIA territory at $A810+, because reading T1C-L on
		// the bus clears IFR T1, which would race the CPU's polling
		// loop to ack the timer.
		MemRangeEnd: ctrlBase + 8,
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
	// Sub-tick: split each app.Tick into N slices, advancing CPU and
	// then the bus's Tickers in each slice. Without this, polling-
	// based demos (those that LDA / poll a peripheral flag in a tight
	// loop) only see flag transitions at app.Tick boundaries — so a
	// large CPU batch can spend the whole batch in a wait loop, never
	// observing the VIA timer underflow that's about to come.
	const subTicks = 10
	subPeriod := tickPeriod / subTicks
	app.Tick(tickPeriod, func() {
		for i := 0; i < subTicks; i++ {
			clockProv.Advance(subPeriod)
			b.Tick(subPeriod)
		}
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
	loadDemo := func(d demos.Demo) {
		clockProv.SetRunning(false)
		// Resume the VIC so a previous framed demo's pause state
		// doesn't leak into a live demo.
		b.Write(ctrlBase+display.RegPause, 0)
		via1.Reset()
		mainROM.Clear()
		_ = mainROM.Load(0, d.Bytes)
		_ = mainROM.SetResetVector(0xE000)
		merged := mergeSymbols(d.Symbols)
		ramProv.Symbols = merged
		ramProv.Annotations = d.Annotations
		romProv.Symbols = merged
		romProv.Annotations = d.Annotations
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

	// Build the Demo menu, skipping any demo whose RequiresGraphics
	// flag is set — the terminal build has no high-res pixel plane,
	// so those would load but render nothing visible. The wasm
	// build (cmd/6502-wasm) shows everything.
	demoItems := []foxpro.MenuItem{}
	first := true
	for _, sec := range demos.Sections() {
		picked := []demos.Demo{}
		for _, d := range sec.Demos {
			if d.RequiresGraphics {
				continue
			}
			picked = append(picked, d)
		}
		if len(picked) == 0 {
			continue
		}
		if !first {
			demoItems = append(demoItems, foxpro.MenuItem{Separator: true})
		}
		first = false
		for _, d := range picked {
			d := d
			demoItems = append(demoItems, foxpro.MenuItem{
				Label:    d.Name,
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

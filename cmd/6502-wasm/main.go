//go:build js && wasm

// 6502-wasm is the browser port of the 6502 simulator.
//
// Same widgets, same demos, same dual-CPU backend (interp + netsim) as
// cmd/6502-sim — only the host changes. Foxpro-go's wasm bridge does
// the heavy lifting (SimulationScreen → JS canvas, browser keys/mouse
// → tcell events), so this file is mostly the same wiring as the TUI
// main, with terminal-only bits stripped (flag parsing, profiling).
//
// Defaults differ from the terminal build because they're host-tuned:
//
//   - CPU: interp (netsim is slow in wasm; user can swap via menu)
//   - Auto-start running: yes (browser visitors expect motion)
//   - QuitKeys: cleared (Esc/Ctrl+Q would kill the wasm runtime)
//   - BackgroundDragChords: shift+click only (right-click = native menu)
package main

import (
	"fmt"
	"time"

	foxpro "github.com/carledwards/foxpro-go"
	"github.com/carledwards/foxpro-go/wasm"
	"github.com/gdamore/tcell/v2"

	"github.com/carledwards/6502-sim-tui/bus"
	"github.com/carledwards/6502-sim-tui/components/display"
	"github.com/carledwards/6502-sim-tui/components/ram"
	"github.com/carledwards/6502-sim-tui/components/rom"
	"github.com/carledwards/6502-sim-tui/cpu"
	"github.com/carledwards/6502-sim-tui/cpu/interp"
	"github.com/carledwards/6502-sim-tui/cpu/netsim"
	"github.com/carledwards/6502-sim-tui/internal/demos"
	"github.com/carledwards/6502-sim-tui/ui/clockwin"
	"github.com/carledwards/6502-sim-tui/ui/cpuwin"
	"github.com/carledwards/6502-sim-tui/ui/displaywin"
	"github.com/carledwards/6502-sim-tui/ui/ramwin"
)

// Memory map. Extends cmd/6502-sim's layout with a graphics plane —
// 160×104 at 4bpp = 8,320 bytes — placed at $A000. Below RAM and
// above the VIC controller, well clear of ROM at $E000.
const (
	ramBase   = 0x0000
	ramSize   = 0x2000 // 8 KB at $0000-$1FFF
	colorBase = 0x8200 // VIC color plane (520 bytes)
	charBase  = 0x8500 // VIC char plane  (520 bytes)
	ctrlBase  = 0x8800 // VIC controller registers (8 bytes; +7 = GfxColor)
	dispW     = 40
	dispH     = 13
	gfxBase   = 0xA000
	gfxW      = 160
	gfxH      = 104
	gfxBPP    = 4
	romBase   = 0xE000
	romSize   = 0x2000
)

// tuneCandidates and autoTune mirror cmd/6502-sim. Browser tick
// budgets are slightly tighter, but the same auto-tune loop applies —
// it just lands on a smaller batch number.
var tuneCandidates = []int{500, 1000, 1500, 2000, 2500, 3000, 4000, 5000, 7500, 10000, 20000, 50000, 100000}

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
		break
	}
	return best
}

const tickPeriod = 50 * time.Millisecond

func main() {
	// Bus + memory map. Outer TraceBus stamps reads/writes for the
	// memory viewer's "recently touched" tinting.
	innerBus := bus.New()
	b := bus.NewTraceBus(innerBus)
	mainRAM := ram.New("ram", ramBase, ramSize)
	colorPlane := display.New("display.color", colorBase, dispW, dispH)
	charPlane := display.New("display.char", charBase, dispW, dispH)
	mainROM := rom.New("rom", romBase, romSize)

	paintInitialDisplay := func() {
		for y := 0; y < dispH; y++ {
			for x := 0; x < dispW; x++ {
				colorPlane.SetPixel(x, y, uint8(((x+y)%16)<<4))
				charPlane.SetPixel(x, y, 0x20)
			}
		}
	}
	paintInitialDisplay()

	gfxPlane := display.NewGraphicsPlane(display.GraphicsConfig{
		Name:   "display.gfx",
		Base:   gfxBase,
		Width:  gfxW,
		Height: gfxH,
		BPP:    gfxBPP,
	})
	dispCtrl := display.NewControllerWithGraphics("display.ctrl", ctrlBase, colorPlane, charPlane, gfxPlane)

	// Boot directly into BouncingBalls — graphics is always wired in
	// the wasm build (gfxPlane registered above), so this is the
	// most visually interesting first impression. Other demos
	// (Marquee, Bouncer, etc.) are still selectable via the Demo
	// menu. The terminal build (cmd/6502-sim) keeps Marquee as its
	// default since it doesn't currently include a graphics plane.
	must(mainROM.Load(0, demos.BouncingBalls))
	must(mainROM.SetResetVector(0xE000))
	must(b.Register(mainRAM))
	must(b.Register(colorPlane))
	must(b.Register(charPlane))
	must(b.Register(dispCtrl))
	must(b.Register(gfxPlane))
	must(b.Register(mainROM))

	buildBackend := func(name string) (cpu.Backend, error) {
		switch name {
		case "netsim":
			return netsim.New(b)
		case "interp":
			return interp.New(b), nil
		}
		return nil, fmt.Errorf("unknown cpu %q (want netsim or interp)", name)
	}

	currentCPU := "interp" // wasm default; interp is fast enough to be lively
	backend, err := buildBackend(currentCPU)
	if err != nil {
		panic(err)
	}
	backend.Reset()
	cpuTitle := fmt.Sprintf("CPU (%s)", currentCPU)

	// SimulationScreen sized to fit every window plus menu+status rows.
	// 140×32 covers the widest layout (display window ends at col 136).
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		panic(err)
	}
	s.SetSize(140, 32)
	s.EnableMouse()

	app := foxpro.NewAppWithScreen(s)

	// Browser-appropriate settings.
	app.Settings.QuitKeys = nil
	app.Settings.BackgroundDragChords = []foxpro.BackgroundDragChord{
		{Button: tcell.Button1, Mods: tcell.ModShift},
	}
	app.Settings.StatusBarLeft = " Esc to close "

	// Track every window we create so we can toggle visibility from
	// the Window menu after a close.
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
		Bus:          innerBus,
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
	addWindow("Clock",
		foxpro.Rect{X: 2, Y: 13, W: 38, H: 7},
		clockProv,
		clockwin.MinW, clockwin.MinH)

	machineReset := func() {
		clockProv.SetRunning(false)
		b.Write(ctrlBase+display.RegPause, 0)
		b.Write(ctrlBase+display.RegMode, display.ModeChar)
		gfxPlane.Clear(0)
		mainRAM.Reset()
		paintInitialDisplay()
		clockProv.Reset()
	}
	cpuProv.OnReset = machineReset

	dispProv := &displaywin.Provider{
		Bus:           innerBus,
		Controller:    dispCtrl,
		ColorBase:     colorBase,
		CharBase:      charBase,
		CtrlBase:      ctrlBase,
		HasChars:      true,
		HasCtrl:       true,
		Width:         dispW,
		Height:        dispH,
		Graphics:      gfxPlane,
		MemRangeStart: colorBase,
		MemRangeEnd:   ctrlBase + 8, // controller now 9 bytes (0..8)
	}
	dispTitle := fmt.Sprintf("VIC $%04X-$%04X", colorBase, ctrlBase+8)
	dispWindow := addWindow(dispTitle,
		foxpro.Rect{X: 60, Y: 1, W: 77, H: 29},
		dispProv,
		displaywin.MinW, displaywin.MinH)
	dispProv.Window = dispWindow // lets Provider append [TEXT]/[GFX] to the title each Draw


	// Run loop. App.Tick fires on the UI goroutine, so simulator
	// advancement, register reads, and bus reads all serialize
	// naturally — no locks needed.
	app.Tick(tickPeriod, func() {
		clockProv.Advance(tickPeriod)
		b.Tick()
	})

	// Global key bindings — same set as the TUI.
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

	loadDemo := func(d demos.Demo) {
		clockProv.SetRunning(false)
		b.Write(ctrlBase+display.RegPause, 0)
		b.Write(ctrlBase+display.RegMode, display.ModeChar)
		gfxPlane.Clear(0)
		mainROM.Clear()
		_ = mainROM.Load(0, d.Bytes)
		_ = mainROM.SetResetVector(0xE000)
		paintInitialDisplay()
		clockProv.Reset()
		clockProv.SetRunning(true)
	}

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
	for sIdx, sec := range demos.Sections() {
		if sIdx > 0 {
			demoItems = append(demoItems, foxpro.MenuItem{Separator: true})
		}
		for _, d := range sec.Demos {
			d := d
			demoItems = append(demoItems, foxpro.MenuItem{
				Label:    d.Name,
				OnSelect: func() { loadDemo(d) },
			})
		}
	}

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

	// Auto-tune the batch size for this browser / device before
	// kicking the clock off. The same logic the CPU menu's
	// "Auto-tune Batch" item runs — happens once at boot so first-
	// frame motion lands on a sensible batch instead of the default
	// 500. autoTune advances cycles, so reset everything afterwards
	// to start the demo from a clean slate.
	clockProv.MaxBatch = autoTune(backend, 35*time.Millisecond)
	backend.Reset()
	mainRAM.Reset()
	paintInitialDisplay()
	clockProv.Reset()

	// Default to max speed — no Hz cap, run as many HalfSteps per
	// UI tick as the auto-tuned batch allows. Visitors land on a
	// page that's already at full motion; they can throttle via
	// the Clock window or '<' / '>' keys if they want a slower view.
	clockProv.SetSpeedHz(0)

	clockProv.SetRunning(true) // browser visitors expect motion right away
	wasm.Run(app, s)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}


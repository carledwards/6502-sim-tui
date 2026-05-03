// Package scopewin renders a logic-analyzer-style trace of the
// CPU's address and data buses. Each captured sample becomes a new
// column on the right edge, shifting older samples left until they
// fall off — same visual model as a DSO or Saleae logic analyzer
// in roll mode.
//
// Two render paths share the same provider:
//
//   - Cell mode (TUI default): each canvas cell is one sample. Dots
//     drawn as ● glyphs in cells. Lower density but works on any
//     terminal.
//   - Graphics mode (WASM, opt-in via the UseGraphics field):
//     stamps sentinel runes across the canvas; the wasm bridge
//     composites a pixel buffer with one pixel column per sample.
//     ~8× higher density at a given window width.
//
// Sampling cadence is one column per CPU half-cycle, fed by the
// clockwin.Provider's OnHalfStep hook. The Decimate field thins
// the stream (keep every Nth call) so high-Hz CPUs don't overwrite
// the buffer faster than the eye can track. main.go auto-tunes
// Decimate based on the active clock speed.
package scopewin

import (
	"fmt"
	"sync"

	"github.com/gdamore/tcell/v2"

	foxpro "github.com/carledwards/foxpro-go"
)

// PixelLayerID is a stable identifier the wasm bridge uses to match
// the Go-side provider with its host-side canvas overlay.
const PixelLayerID = "scope-trace"

// pixelPlaceholderRune (U+E000, Private Use Area) is the sentinel
// rune the wasm bridge looks for when compositing pixel content
// over cell-rendered windows. Same value as displaywin's sentinel,
// declared via numeric escape so the source survives clipboard
// round-trips that strip PUA codepoints.
const pixelPlaceholderRune = ''

// Layout constants.
const (
	rightStripCells = 9
	rowsClk         = 1
	rowsAddr        = 16
	rowGap          = 1
	rowsData        = 8
	totalRows       = rowsClk + rowsAddr + rowGap + rowsData
	topMargin       = 1

	// Layout offsets within the trace area (relative to the first
	// trace row, NOT including topMargin).
	clkRow0  = 0
	addrRow0 = clkRow0 + rowsClk
	dataRow0 = addrRow0 + rowsAddr + rowGap

	// pxPerCell — graphics-mode pixels per cell (= the wasm
	// bridge's per-cell subdivision: subW = pxW / cellW).
	// pxPerCell / pxPerSample = samples per cell.
	pxPerCell = 10

	// pxPerSample — pixel-column width of one captured sample.
	// Wider samples = chunkier bars, fewer visible cycles per
	// canvas. 5 gives 2 samples per cell — substantial bars
	// without giving up too much history.
	pxPerSample = 5

	// pxPerRow — vertical pixels per trace row. Bigger row
	// height makes "1" bars feel proportional to their width.
	pxPerRow = 6

	// dotH — vertical pixel-height of a "1" bar inside a row's
	// pxPerRow band. Width is determined by run-length: consecutive
	// samples with the same bit set merge into a continuous block,
	// so what shows on screen reads as "duration of high signal."
	dotH = 4

	// MinW / MinH — exposed window minimums.
	MinW = rightStripCells + 16 + 2
	MinH = topMargin + totalRows + 2

	dotRune = "●"
)

// Sample is one captured frame of bus state.
type Sample struct {
	Addr uint16
	Data uint8
	// InstrEdge is true on the half-cycle where the program counter
	// just advanced (i.e., an instruction completed and the new
	// instruction's first cycle is now in flight). Used to color
	// the CLK row's pulse so instruction boundaries are visible.
	InstrEdge bool
}

// Provider renders one logic-analyzer scope window.
type Provider struct {
	foxpro.ScrollState

	// Width is the canvas width in cells. In cell mode this is also
	// the sample-buffer length; in graphics mode the buffer holds
	// Width * pxPerCell samples (one pixel-column per sample).
	Width int

	// UseGraphics opts into the pixel-overlay render path. When
	// true, the canvas region stamps sentinel runes for the wasm
	// bridge to composite a high-resolution pixel buffer over;
	// when false, the canvas cells render ● glyphs at one sample
	// per cell. Set at construction (changing it post-hoc would
	// invalidate the buffer length).
	UseGraphics bool

	// Decimate keeps every Nth sample. 0 or 1 captures every call.
	// Useful at high CPU speeds where the ring would overwrite
	// faster than the eye can track. main.go updates this each
	// app.Tick based on the active clock speed.
	Decimate int

	mu          sync.Mutex
	buf         []Sample
	head        int  // index of next write
	filled      bool // false until ring has wrapped at least once
	skip        int  // decimation counter
	pendingEdge bool // an instr-edge that arrived during a decimated skip
}

// New returns a Provider with the given canvas cell-width. Set
// useGraphics=true for the high-density pixel-overlay path (wasm
// only); false for cell-rendered glyphs (works in any terminal).
func New(cellWidth int, useGraphics bool) *Provider {
	if cellWidth <= 0 {
		cellWidth = 128
	}
	bufN := cellWidth
	if useGraphics {
		// Each cell hosts pxPerCell pixels but each sample claims
		// pxPerSample of those, so samples-per-cell = pxPerCell /
		// pxPerSample. Total buffer is canvas-cells × that.
		bufN = cellWidth * (pxPerCell / pxPerSample)
	}
	return &Provider{
		Width:       cellWidth,
		UseGraphics: useGraphics,
		Decimate:    1,
		buf:         make([]Sample, bufN),
	}
}

// Capture pushes one sample into the ring. Honors Decimate.
// instrEdge marks the half-cycle where an instruction just
// completed (PC tick), used by the CLK row's color highlight.
func (p *Provider) Capture(addr uint16, data uint8, instrEdge bool) {
	p.mu.Lock()
	div := p.Decimate
	if div < 1 {
		div = 1
	}
	p.skip++
	if p.skip < div {
		// Even when we skip, propagate any instr-edge through to
		// the next captured sample so the highlight isn't lost
		// to decimation.
		if instrEdge {
			p.pendingEdge = true
		}
		p.mu.Unlock()
		return
	}
	p.skip = 0
	if p.pendingEdge {
		instrEdge = true
		p.pendingEdge = false
	}
	p.buf[p.head] = Sample{Addr: addr, Data: data, InstrEdge: instrEdge}
	p.head++
	if p.head >= len(p.buf) {
		p.head = 0
		p.filled = true
	}
	p.mu.Unlock()
}

// Reset clears the buffer.
func (p *Provider) Reset() {
	p.mu.Lock()
	for i := range p.buf {
		p.buf[i] = Sample{}
	}
	p.head = 0
	p.filled = false
	p.skip = 0
	p.mu.Unlock()
}

// last returns the most recently captured sample. Caller holds p.mu.
func (p *Provider) last() Sample {
	if !p.filled && p.head == 0 {
		return Sample{}
	}
	idx := p.head - 1
	if idx < 0 {
		idx = len(p.buf) - 1
	}
	return p.buf[idx]
}

// ─── ContentProvider ────────────────────────────────────────────

// Draw renders chrome (rule, labels, separator, hex readouts) and,
// in cell mode, the trace dots themselves. In graphics mode the
// canvas region is stamped with sentinels and the actual trace is
// painted by DrawPixels through the wasm bridge.
func (p *Provider) Draw(screen tcell.Screen, inner foxpro.Rect, theme foxpro.Theme, focused bool) {
	bg := theme.WindowBG
	dim := bg.Foreground(theme.Palette.DarkGray)
	hex := bg.Foreground(theme.Palette.Yellow)
	addr := bg.Foreground(theme.Palette.LightGreen)
	data := bg.Foreground(theme.Palette.LightBlue)
	c := foxpro.NewCanvas(screen, inner, &p.ScrollState)

	canvasCols := p.Width
	sepCol := canvasCols
	labelCol := canvasCols + 1
	hexCol := canvasCols + 5

	// Top margin: "Sampling: NN%" left-justified, then horizontal
	// rule out to the canvas edge.
	div := p.Decimate
	if div < 1 {
		div = 1
	}
	label := fmt.Sprintf(" Sampling: %g%% ", 100.0/float64(div))
	x := c.Put(0, 0, label, dim)
	for ; x < canvasCols; x++ {
		c.Put(x, 0, "─", dim)
	}

	// CLK label (top trace row).
	c.Put(labelCol, topMargin+clkRow0, "CLK", bg)

	// Address labels.
	for i := 0; i < rowsAddr; i++ {
		bit := rowsAddr - 1 - i
		c.Put(labelCol, topMargin+addrRow0+i, fmt.Sprintf("A%-2d", bit), bg)
	}
	// Data labels.
	for i := 0; i < rowsData; i++ {
		bit := rowsData - 1 - i
		c.Put(labelCol, topMargin+dataRow0+i, fmt.Sprintf("D%-2d", bit), bg)
	}

	// Vertical separator.
	for y := 0; y < topMargin+totalRows; y++ {
		c.Put(sepCol, y, "│", dim)
	}

	// Snapshot ring under lock for the live hex readouts and (in
	// cell mode) the trace dots.
	p.mu.Lock()
	bufN := len(p.buf)
	hasSample := p.filled || p.head > 0
	startIdx := p.head
	headIdx := p.head
	bufCopy := make([]Sample, bufN)
	copy(bufCopy, p.buf)
	latest := p.last()
	p.mu.Unlock()

	// Hex readouts of the most recent sample.
	addrHi := byte(latest.Addr >> 8)
	addrLo := byte(latest.Addr & 0xFF)
	c.Put(hexCol, topMargin+addrRow0+3, fmt.Sprintf("$%02X", addrHi), hex)
	c.Put(hexCol, topMargin+addrRow0+11, fmt.Sprintf("$%02X", addrLo), hex)
	c.Put(hexCol, topMargin+dataRow0+3, fmt.Sprintf("$%02X", latest.Data), hex)

	if p.UseGraphics {
		// Graphics mode: stamp sentinels across the canvas region.
		// The wasm bridge calls DrawPixels per frame and composites
		// the resulting RGBA buffer over these cells.
		for y := 0; y < totalRows; y++ {
			for x := 0; x < canvasCols; x++ {
				c.Set(x, topMargin+y, pixelPlaceholderRune, bg)
			}
		}
		return
	}

	// Cell mode: render trace dots directly.
	if !hasSample {
		return
	}
	clkPulse := bg.Foreground(theme.Palette.Cyan)
	clkEdge := bg.Foreground(theme.Palette.Yellow)
	for col := 0; col < canvasCols; col++ {
		var idx int
		if p.filled {
			idx = (startIdx + col) % bufN
		} else {
			if col >= headIdx {
				continue
			}
			idx = col
		}
		s := bufCopy[idx]

		// CLK row — one tick per sample, bright color on
		// instruction-edge captures. Stays put as the trace
		// scrolls (no visible-column alternation).
		st := clkPulse
		if s.InstrEdge {
			st = clkEdge
		}
		c.Put(col, topMargin+clkRow0, dotRune, st)

		for r := 0; r < rowsAddr; r++ {
			bit := uint(rowsAddr - 1 - r)
			if (s.Addr>>bit)&1 != 0 {
				c.Put(col, topMargin+addrRow0+r, dotRune, addr)
			}
		}
		for r := 0; r < rowsData; r++ {
			bit := uint(rowsData - 1 - r)
			if (s.Data>>bit)&1 != 0 {
				c.Put(col, topMargin+dataRow0+r, dotRune, data)
			}
		}
	}
}

// HandleKey — no interactive controls in Phase 1.
func (p *Provider) HandleKey(ev *tcell.EventKey) bool { return false }

// StatusHint — nothing to advertise yet.
func (p *Provider) StatusHint() string { return "" }

// ─── PixelContent + PixelRectContent ────────────────────────────

// PixelLayerID — stable identifier for the wasm canvas overlay.
func (p *Provider) PixelLayerID() string { return PixelLayerID }

// PixelSize returns the pixel-buffer dimensions. (0, 0) when
// UseGraphics is false — tells the bridge to skip this layer.
func (p *Provider) PixelSize() (int, int) {
	if !p.UseGraphics {
		return 0, 0
	}
	return p.Width * pxPerCell, totalRows * pxPerRow
}

// PixelRect places the canvas at the LEFT of the inner area; the
// label + hex strip sits to the right. Coords inner-relative.
func (p *Provider) PixelRect() (x, y, w, h int) {
	if !p.UseGraphics {
		return 0, 0, 0, 0
	}
	sx, sy := p.ScrollOffset()
	return 0 - sx, topMargin - sy, p.Width, totalRows
}

// DrawPixels renders the trace into a flat RGBA buffer (4 bytes per
// pixel, row-major). Only fires when UseGraphics is true; otherwise
// the bridge has already been told to skip this layer.
func (p *Provider) DrawPixels(buf []byte) {
	if !p.UseGraphics {
		return
	}
	pxW := p.Width * pxPerCell
	pxH := totalRows * pxPerRow

	// Background — dim navy for that DOS-scope feel.
	bgR, bgG, bgB := byte(0x00), byte(0x10), byte(0x20)
	addrR, addrG, addrB := byte(0x40), byte(0xff), byte(0x88) // green
	dataR, dataG, dataB := byte(0x60), byte(0x90), byte(0xff) // blue

	for i := 0; i < pxW*pxH; i++ {
		buf[4*i+0] = bgR
		buf[4*i+1] = bgG
		buf[4*i+2] = bgB
		buf[4*i+3] = 0xFF
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	bufN := len(p.buf)
	if !p.filled && p.head == 0 {
		return
	}

	// indexFor returns the buffer slot that contains the sample at
	// the given visible-column position s, plus a "valid" flag
	// (false when the buffer hasn't filled enough to cover that
	// column yet).
	indexFor := func(s int) (int, bool) {
		if p.filled {
			return (p.head + s) % bufN, true
		}
		if s >= p.head {
			return 0, false
		}
		return s, true
	}

	// Run-length rendering: consecutive samples with the same bit
	// set draw as one continuous bar. Transitions (high→low or
	// gap-due-to-empty-slot) end the run. This makes signal
	// duration read at a glance — a 6-cycle hold appears as a
	// 6-wide bar rather than 6 striped bars.
	drawRow := func(rowY int, bitOf func(Sample) bool, r, g, b byte) {
		runStart := -1
		flush := func(end int) {
			if runStart < 0 {
				return
			}
			fillRun(buf, pxW, runStart*pxPerSample, rowY, (end-runStart)*pxPerSample, r, g, b)
			runStart = -1
		}
		for s := 0; s < bufN; s++ {
			idx, ok := indexFor(s)
			if !ok {
				flush(s)
				continue
			}
			if bitOf(p.buf[idx]) {
				if runStart < 0 {
					runStart = s
				}
			} else {
				flush(s)
			}
		}
		flush(bufN)
	}

	// CLK row — one pulse per captured sample, with a 1-px gap
	// between pulses so adjacent samples remain distinguishable.
	// Color is dim cyan by default; bright amber for samples
	// flagged as instruction-edge (PC just changed). Pulses are
	// keyed off sample identity, NOT visible-column index, so
	// yellow markers don't drift in/out as the trace scrolls.
	clkY := clkRow0 * pxPerRow
	clkPulseR, clkPulseG, clkPulseB := byte(0x60), byte(0xc0), byte(0xc0)
	clkEdgeR, clkEdgeG, clkEdgeB := byte(0xff), byte(0xd0), byte(0x40)
	for s := 0; s < bufN; s++ {
		idx, ok := indexFor(s)
		if !ok {
			continue
		}
		smp := p.buf[idx]
		r2, g2, b2 := clkPulseR, clkPulseG, clkPulseB
		if smp.InstrEdge {
			r2, g2, b2 = clkEdgeR, clkEdgeG, clkEdgeB
		}
		// Pulse width = pxPerSample - 1 so adjacent ticks have a
		// 1-px gap; matches the visual rhythm of the address /
		// data rows under run-length rendering.
		w := pxPerSample - 1
		if w < 1 {
			w = 1
		}
		fillRun(buf, pxW, s*pxPerSample, clkY, w, r2, g2, b2)
	}

	for r := 0; r < rowsAddr; r++ {
		bit := uint(rowsAddr - 1 - r)
		drawRow((addrRow0+r)*pxPerRow, func(s Sample) bool { return (s.Addr>>bit)&1 != 0 }, addrR, addrG, addrB)
	}
	for r := 0; r < rowsData; r++ {
		bit := uint(rowsData - 1 - r)
		drawRow((dataRow0+r)*pxPerRow, func(s Sample) bool { return (s.Data>>bit)&1 != 0 }, dataR, dataG, dataB)
	}
}

// fillRun paints a w×dotH block centered vertically within the
// pxPerRow band at (px, py). Used by DrawPixels to render a
// run-length-encoded signal segment — one run per high stretch.
func fillRun(buf []byte, pxW, px, py, w int, r, g, b byte) {
	if w <= 0 {
		return
	}
	yOff := (pxPerRow - dotH) / 2
	if yOff < 0 {
		yOff = 0
	}
	for dy := 0; dy < dotH && yOff+dy < pxPerRow; dy++ {
		row := py + yOff + dy
		base := row * pxW
		for dx := 0; dx < w; dx++ {
			x := px + dx
			if x >= pxW {
				break
			}
			i := base + x
			if i*4+3 >= len(buf) {
				continue
			}
			buf[4*i+0] = r
			buf[4*i+1] = g
			buf[4*i+2] = b
			buf[4*i+3] = 0xFF
		}
	}
}

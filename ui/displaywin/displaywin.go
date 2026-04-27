// Package displaywin renders the VIC framebuffer with a single-line
// blue-on-cyan border, a vertical strip of command buttons on the
// right, and a small memory-map summary below. Button clicks POKE
// the controller via the bus — same path the CPU uses.
package displaywin

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"

	foxpro "github.com/carledwards/foxpro-go"
	"github.com/carledwards/6502-sim-tui/bus"
	"github.com/carledwards/6502-sim-tui/components/display"
)

// MinW / MinH cover the chrome border + a tiny visible interior so a
// resized display is still recognisable as one. Real natural size is
// much bigger; Canvas + ScrollState lets the user drag down freely.
const (
	MinW = 14
	MinH = 8
)

// chromeStyle returns the blue-on-cyan look shared by the display
// border and the resting state of buttons. Pulls from the theme
// palette so cursor inversion and theming work.
func chromeStyle(theme foxpro.Theme) tcell.Style {
	return tcell.StyleDefault.
		Background(theme.Palette.Cyan).
		Foreground(theme.Palette.Blue)
}

// buttonDef describes one of the right-column command buttons. Each
// fires a single bus.Write to the controller's address+reg with the
// configured value — the same operation the CPU performs.
type buttonDef struct {
	label string // padded to fixed width for column alignment
	reg   uint16 // controller register offset (0/1/2)
	val   uint8  // value to write
}

// All labels padded to 12 chars so the "< … >" brackets line up.
var buttonDefs = []buttonDef{
	{"Frame Sync  ", display.RegFrame, 0x01},
	{"Clear       ", display.RegCmd, display.CmdClear},
	{"Invert      ", display.RegCmd, display.CmdInvert},
	{"Scroll Left ", display.RegCmd, display.CmdShiftLeft},
	{"Scroll Right", display.RegCmd, display.CmdShiftRight},
	{"Scroll Up   ", display.RegCmd, display.CmdShiftUp},
	{"Scroll Down ", display.RegCmd, display.CmdShiftDown},
	{"Rotate Left ", display.RegCmd, display.CmdRotLeft},
	{"Rotate Right", display.RegCmd, display.CmdRotRight},
	{"Rotate Up   ", display.RegCmd, display.CmdRotUp},
	{"Rotate Down ", display.RegCmd, display.CmdRotDown},
}

const (
	// Right column — where Status and the button stack render.
	rightColX = 44

	// Each button row is one logical line: "○ < Label >".
	buttonW = 18 // "○ < 12chars  >" → 1+1+1+1+12+1+1 = 18

	// Indicator stays "filled" for this long after a fire so the
	// user sees a brief acknowledgment.
	flashDuration = 300 * time.Millisecond
)

// Provider renders a memory-mapped framebuffer. ColorBase points at
// the color plane (low nibble = fg, high nibble = bg). CharBase, if
// HasChars, points at a parallel char plane. CtrlBase, if HasCtrl,
// points at the controller's command register — the buttons POKE
// values there.
type Provider struct {
	Bus           bus.Bus
	Controller    *display.Controller // pause-aware framebuffer reads
	ColorBase     uint16
	CharBase      uint16
	CtrlBase      uint16
	HasChars      bool
	HasCtrl       bool
	Width         int
	Height        int
	CellsPerPixel int

	// MemRangeStart / MemRangeEnd bound the bottom hex strip's
	// scroll range (both inclusive). When unset (both zero), the
	// strip can scroll the full 64 KB address space; setting them
	// confines the view to a slice of memory — typically the VIC's
	// own region from ColorBase through the last controller register.
	MemRangeStart uint16
	MemRangeEnd   uint16

	foxpro.ScrollState

	// Button drag state. pressedIdx is the index in buttonDefs of
	// the button under the initial mouse-down (-1 if no press).
	// armed tracks whether the cursor is still over that same button.
	pressedIdx int
	armed      bool

	// Per-button "recently fired" timestamps for the indicator flash.
	lastFire []time.Time

	// memBase is the address of the first byte shown in the bottom
	// hex-dump section. Lazy-initialised from ColorBase. Mouse wheel
	// over the section and '[' / ']' / '{' / '}' nudge it.
	memBase uint16
	memInit bool

	// Thumb-drag state for the hex-strip scrollbar. Captured when a
	// left-click lands on ◆; cleared on mouse release. Track top/bot
	// snapshot lets motion handler do the math without re-deriving
	// the box geometry.
	memDragging   bool
	memDragTrackT int
	memDragTrackB int
}

// memSpan = bytes shown in the bottom hex-dump section.
const memSpan = memDataRows * memBytesPerRow

// memBounds returns the inclusive [min, max] range memBase may take.
// max is end-of-range minus one full strip, so the last visible byte
// stays inside the configured memory window.
func (p *Provider) memBounds() (minBase, maxBase int) {
	end := int(p.MemRangeEnd)
	if p.MemRangeStart == 0 && end == 0 {
		end = 0xFFFF // unset → full address space
	}
	minBase = int(p.MemRangeStart)
	maxBase = end - memSpan + 1
	if maxBase < minBase {
		maxBase = minBase // range smaller than a full strip → pinned
	}
	return
}

func (p *Provider) curMemBase() uint16 {
	if !p.memInit {
		if p.MemRangeStart != 0 || p.MemRangeEnd != 0 {
			p.memBase = p.MemRangeStart
		} else {
			p.memBase = p.ColorBase
		}
		p.memInit = true
	}
	return p.memBase
}

func (p *Provider) shiftMemBase(delta int) {
	minBase, maxBase := p.memBounds()
	base := int(p.curMemBase()) + delta
	if base < minBase {
		base = minBase
	}
	if base > maxBase {
		base = maxBase
	}
	p.memBase = uint16(base)
	p.memInit = true
}

// cellsPerPixel returns the on-screen cell width of one logical
// pixel. Default: 1 in text mode, 2 in graphics mode.
func (p *Provider) cellsPerPixel() int {
	if p.CellsPerPixel > 0 {
		return p.CellsPerPixel
	}
	if p.HasChars {
		return 1
	}
	return 2
}

// buttonRect returns the (x, y, width) of button `idx` in canvas
// logical coordinates. Single right-side column, one row per button,
// starting two rows below Status.
func (p *Provider) buttonRect(idx int) (x, y, w int) {
	return rightColX, 2 + idx, buttonW
}

// buttonHitIdx returns the button index at screen-space (mx, my),
// or -1 if no button is hit. Translates through the canvas scroll.
func (p *Provider) buttonHitIdx(mx, my int, inner foxpro.Rect) int {
	lx := (mx - inner.X) + p.X
	ly := (my - inner.Y) + p.Y
	for i := range buttonDefs {
		bx, by, bw := p.buttonRect(i)
		if lx >= bx && lx < bx+bw && ly == by {
			return i
		}
	}
	return -1
}

// fireButton writes the button's POKE pair to the controller via
// the bus, then records the timestamp for the indicator flash.
func (p *Provider) fireButton(idx int) {
	if !p.HasCtrl {
		return
	}
	def := buttonDefs[idx]
	p.Bus.Write(p.CtrlBase+def.reg, def.val)
	if p.lastFire == nil {
		p.lastFire = make([]time.Time, len(buttonDefs))
	}
	p.lastFire[idx] = time.Now()
}

func (p *Provider) Draw(screen tcell.Screen, inner foxpro.Rect, theme foxpro.Theme, focused bool) {
	c := foxpro.NewCanvas(screen, inner, &p.ScrollState)
	chrome := chromeStyle(theme)
	frame := chrome
	bg := theme.WindowBG

	cpp := p.cellsPerPixel()
	pxW := p.Width * cpp
	pxH := p.Height

	// ─── Display border + pixels ────────────────────────────
	c.Set(0, 0, '┌', frame)
	for i := 0; i < pxW; i++ {
		c.Set(1+i, 0, '─', frame)
	}
	c.Set(1+pxW, 0, '┐', frame)

	for py := 0; py < pxH; py++ {
		ly := 1 + py
		c.Set(0, ly, '│', frame)
		for px := 0; px < p.Width; px++ {
			off := py*p.Width + px
			var color uint8
			if p.Controller != nil {
				color = p.Controller.ReadColor(off)
			} else {
				color = p.Bus.Read(p.ColorBase + uint16(off))
			}
			fg := palette[color&0x0F]
			bgPx := palette[(color>>4)&0x0F]
			cellStyle := tcell.StyleDefault.Background(bgPx).Foreground(fg)

			ch := rune(' ')
			if p.HasChars {
				var raw uint8
				if p.Controller != nil {
					raw = p.Controller.ReadChar(off)
				} else {
					raw = p.Bus.Read(p.CharBase + uint16(off))
				}
				if raw >= 0x20 && raw < 0x7F {
					ch = rune(raw)
				}
			}

			lx := 1 + px*cpp
			c.Set(lx, ly, ch, cellStyle)
			for k := 1; k < cpp; k++ {
				c.Set(lx+k, ly, ' ', cellStyle)
			}
		}
		c.Set(1+pxW, ly, '│', frame)
	}

	by := 1 + pxH
	c.Set(0, by, '└', frame)
	for i := 0; i < pxW; i++ {
		c.Set(1+i, by, '─', frame)
	}
	c.Set(1+pxW, by, '┘', frame)

	// ─── Right column: Status + button stack ────────────────
	if p.HasCtrl && p.Controller != nil {
		paused := p.Controller.IsPaused()
		status := "Running"
		if paused {
			status = "Paused "
		}
		c.Put(rightColX, 0, "Status: "+status, bg)

		now := time.Now()
		for i, def := range buttonDefs {
			bxc, byc, _ := p.buttonRect(i)

			// Indicator: ● when this button's action is currently in
			// flight, regardless of source — UI armed press, recent
			// UI fire, or recent CPU write that matches this button's
			// (reg, val) pair.
			lit := false
			if p.pressedIdx == i && p.armed {
				lit = true
			}
			if p.lastFire != nil && now.Sub(p.lastFire[i]) < flashDuration {
				lit = true
			}
			switch def.reg {
			case display.RegCmd:
				if p.Controller.LastCmd() == def.val &&
					now.Sub(p.Controller.LastCmdAt()) < flashDuration {
					lit = true
				}
			case display.RegFrame:
				if now.Sub(p.Controller.LastFrameAt()) < flashDuration {
					lit = true
				}
			}
			ind := '○'
			if lit {
				ind = '●'
			}
			// Indicator stays in the chrome blue-on-cyan style — only
			// the glyph changes when active. Keeps the right column
			// visually quiet; theme.Focus on the label still flags
			// the armed state.
			c.Set(bxc, byc, ind, chrome)

			labelStyle := chrome
			if p.pressedIdx == i && p.armed {
				labelStyle = theme.Focus
			}
			c.Put(bxc+2, byc, "< "+def.label+" >", labelStyle)
		}
	}

	// ─── Bottom: scrollable hex-dump strip drawn inside a FoxPro
	// half-box (┌ + top rule, │ on left, scrollbar on right, └ +
	// bottom rule with ' '@Scrollbar on the bottom-right corner).
	// memBase is mutable (mouse wheel / '[' / ']' / '{' / '}'); rows
	// render relative to it.
	boxTopY := p.boxTopY()
	memY := boxTopY + 1                  // header row
	boxLastY := memY + memDataRows       // last data row
	boxBotY := boxLastY + 1              // bottom rule
	sbX := p.memScrollbarX()
	contentLeftX := 1 // shifted right by one to clear the │ left border
	memAsciiX := contentLeftX + memLabelW + memBytesPerRow*3 + memAsciiGap
	base := p.curMemBase()

	border := theme.Border
	sbar := theme.Scrollbar
	_, contentBG, _ := bg.Decompose()
	arrow := sbar.Foreground(contentBG)

	// Box chrome. Arrows live in the corners of the right column so
	// the track between them spans every inner row — gives the thumb
	// more travel for the same box height.
	c.Set(0, boxTopY, '┌', border)
	for x := 1; x < sbX; x++ {
		c.Set(x, boxTopY, '─', border)
	}
	c.Set(sbX, boxTopY, '▲', arrow)
	for y := memY; y <= boxLastY; y++ {
		c.Set(0, y, '│', border)
	}
	c.Set(0, boxBotY, '└', border)
	for x := 1; x < sbX; x++ {
		c.Set(x, boxBotY, '─', border)
	}
	c.Set(sbX, boxBotY, '▼', arrow)

	// Scrollbar gutter spans every content row between the arrows.
	for y := memY; y <= boxLastY; y++ {
		c.Set(sbX, y, ' ', sbar)
	}
	trackTop, trackBot := memY, boxLastY
	trackH := trackBot - trackTop + 1
	if trackH > 0 {
		minBase, maxBase := p.memBounds()
		rng := maxBase - minBase
		thumbOff := 0
		if rng > 0 && trackH > 1 {
			thumbOff = ((int(base) - minBase) * (trackH - 1)) / rng
		}
		thumbY := trackTop + thumbOff
		if thumbY < trackTop {
			thumbY = trackTop
		}
		if thumbY > trackBot {
			thumbY = trackBot
		}
		c.Set(sbX, thumbY, '◆', arrow)
	}

	// Column header.
	for col := 0; col < memBytesPerRow; col++ {
		c.Put(contentLeftX+memLabelW+col*3, memY, fmt.Sprintf(" %02X", col), bg)
	}

	// Data rows + ASCII column.
	for row := 0; row < memDataRows; row++ {
		rowAddr := uint16(int(base) + row*memBytesPerRow)
		c.Put(contentLeftX, memY+1+row, fmt.Sprintf("$%04X: ", rowAddr), bg)
		for col := 0; col < memBytesPerRow; col++ {
			addr := uint16(int(rowAddr) + col)
			b := p.Bus.Read(addr)
			c.Put(contentLeftX+memLabelW+col*3, memY+1+row, fmt.Sprintf(" %02X", b), bg)
			ch := byte('.')
			if b >= 0x20 && b < 0x7F {
				ch = b
			}
			c.Set(memAsciiX+col, memY+1+row, rune(ch), bg)
		}
	}
}

// boxTopY returns the canvas-y of the half-box's top rule. Sits one
// row below the display's bottom border so the rule itself doubles as
// the visual separator (no extra spacer needed).
func (p *Provider) boxTopY() int { return p.Height + 2 }

// memScrollbarX returns the canvas column hosting the vertical
// scrollbar for the hex strip — the rightmost column of the box,
// just past the ASCII column.
func (p *Provider) memScrollbarX() int {
	return 1 + memLabelW + memBytesPerRow*3 + memAsciiGap + memBytesPerRow
}

// Hex-dump layout constants — referenced from Draw and from the
// scrollbase math/hit-testing.
const (
	memBytesPerRow = 16
	memDataRows    = 7
	memLabelW      = 7 // "$XXXX: "
	memAsciiGap    = 2
)

// memSectionY returns the canvas-y of the header row inside the box
// (one row below the top rule).
func (p *Provider) memSectionY() int { return p.boxTopY() + 1 }

// overMemSection reports whether (mx, my) in screen space lands on
// the box interior (header row through last data row).
func (p *Provider) overMemSection(mx, my int, inner foxpro.Rect) bool {
	ly := (my - inner.Y) + p.Y
	mY := p.memSectionY()
	return ly >= mY && ly <= mY+memDataRows
}

func (p *Provider) HandleKey(ev *tcell.EventKey) bool {
	_, vh := p.LastViewport()
	switch ev.Key() {
	case tcell.KeyUp:
		p.SetScrollOffset(p.X, p.Y-1)
		return true
	case tcell.KeyDown:
		p.SetScrollOffset(p.X, p.Y+1)
		return true
	case tcell.KeyLeft:
		p.SetScrollOffset(p.X-1, p.Y)
		return true
	case tcell.KeyRight:
		p.SetScrollOffset(p.X+1, p.Y)
		return true
	case tcell.KeyPgUp:
		p.SetScrollOffset(p.X, p.Y-vh)
		return true
	case tcell.KeyPgDn:
		p.SetScrollOffset(p.X, p.Y+vh)
		return true
	case tcell.KeyHome:
		p.SetScrollOffset(0, 0)
		return true
	case tcell.KeyRune:
		// Hex-dump scroll bindings — distinct from the canvas-scroll
		// arrow keys above so the two views move independently.
		switch ev.Rune() {
		case '[':
			p.shiftMemBase(-memBytesPerRow)
			return true
		case ']':
			p.shiftMemBase(memBytesPerRow)
			return true
		case '{':
			p.shiftMemBase(-memSpan)
			return true
		case '}':
			p.shiftMemBase(memSpan)
			return true
		}
	}
	return false
}

// HandleWheel runs before the framework's canvas-scroll fallback, so
// the hex-dump strip gets first crack at wheel events when the cursor
// is over it. Outside the strip, returning false lets the canvas
// scroll take over normally.
func (p *Provider) HandleWheel(ev *tcell.EventMouse, inner foxpro.Rect) bool {
	mx, my := ev.Position()
	if !p.overMemSection(mx, my, inner) {
		return false
	}
	btns := ev.Buttons()
	switch {
	case btns&tcell.WheelUp != 0:
		p.shiftMemBase(-memBytesPerRow)
	case btns&tcell.WheelDown != 0:
		p.shiftMemBase(memBytesPerRow)
	default:
		return false
	}
	return true
}

func (p *Provider) HandleMouse(ev *tcell.EventMouse, inner foxpro.Rect) bool {
	btns := ev.Buttons()
	mx, my := ev.Position()

	if btns&tcell.Button1 == 0 {
		return false
	}

	// Scrollbar clicks: ▲/▼ nudge by a row, track above/below thumb
	// pages by 7 rows.
	if p.handleScrollbarClick(mx, my, inner) {
		return true
	}

	idx := p.buttonHitIdx(mx, my, inner)
	if idx >= 0 {
		p.pressedIdx = idx
		p.armed = true
		return true
	}
	p.pressedIdx = -1
	p.armed = false
	return false
}

// handleScrollbarClick maps a left-click in the hex-strip scrollbar
// column to a memBase shift. Returns true when the click landed on
// the scrollbar (consuming the event). Lands on the thumb starts a
// drag — subsequent motion / release events are handled by
// HandleMouseMotion / HandleMouseRelease.
func (p *Provider) handleScrollbarClick(mx, my int, inner foxpro.Rect) bool {
	lx := (mx - inner.X) + p.X
	ly := (my - inner.Y) + p.Y
	if lx != p.memScrollbarX() {
		return false
	}
	boxTopY := p.boxTopY()
	boxBotY := boxTopY + memDataRows + 2
	if ly < boxTopY || ly > boxBotY {
		return false
	}
	switch {
	case ly == boxTopY:
		p.shiftMemBase(-memBytesPerRow)
	case ly == boxBotY:
		p.shiftMemBase(memBytesPerRow)
	default:
		// Inside the gutter — page-jump unless click lands on the
		// thumb, in which case start a drag.
		trackTop, trackBot := boxTopY+1, boxBotY-1
		trackH := trackBot - trackTop + 1
		minBase, maxBase := p.memBounds()
		rng := maxBase - minBase
		thumbY := trackTop
		if rng > 0 && trackH > 1 {
			thumbY += ((int(p.curMemBase()) - minBase) * (trackH - 1)) / rng
		}
		if thumbY > trackBot {
			thumbY = trackBot
		}
		switch {
		case ly < thumbY:
			p.shiftMemBase(-memSpan)
		case ly > thumbY:
			p.shiftMemBase(memSpan)
		default:
			p.memDragging = true
			p.memDragTrackT = trackTop
			p.memDragTrackB = trackBot
		}
	}
	return true
}

// setMemBaseFromThumbY snaps memBase to the position implied by a
// thumb at canvas-y `ly`, clamped to the captured track range and
// the configured memory bounds.
func (p *Provider) setMemBaseFromThumbY(ly int) {
	trackH := p.memDragTrackB - p.memDragTrackT + 1
	if trackH <= 1 {
		return
	}
	rel := ly - p.memDragTrackT
	if rel < 0 {
		rel = 0
	}
	if rel > trackH-1 {
		rel = trackH - 1
	}
	minBase, maxBase := p.memBounds()
	rng := maxBase - minBase
	newBase := minBase + (rel*rng)/(trackH-1)
	p.memBase = uint16(newBase)
	p.memInit = true
}

func (p *Provider) HandleMouseMotion(ev *tcell.EventMouse, inner foxpro.Rect) {
	if p.memDragging {
		_, my := ev.Position()
		ly := (my - inner.Y) + p.Y
		p.setMemBaseFromThumbY(ly)
		return
	}
	if p.pressedIdx < 0 {
		return
	}
	mx, my := ev.Position()
	idx := p.buttonHitIdx(mx, my, inner)
	p.armed = idx == p.pressedIdx
}

func (p *Provider) HandleMouseRelease(ev *tcell.EventMouse, inner foxpro.Rect) {
	if p.memDragging {
		p.memDragging = false
		return
	}
	if p.armed && p.pressedIdx >= 0 {
		p.fireButton(p.pressedIdx)
	}
	p.pressedIdx = -1
	p.armed = false
}

func (p *Provider) StatusHint() string {
	return "↑/↓/←/→ scroll  PgUp/PgDn page  [/] mem ±row  {/} mem ±page  wheel mem"
}

// Palette — 16-entry CGA-ish color table indexed by the low nibble
// of each framebuffer byte.
var palette = [16]tcell.Color{
	tcell.ColorBlack,
	tcell.ColorNavy,
	tcell.ColorGreen,
	tcell.ColorTeal,
	tcell.ColorMaroon,
	tcell.ColorPurple,
	tcell.ColorOlive,
	tcell.ColorSilver,
	tcell.ColorGray,
	tcell.ColorBlue,
	tcell.ColorLime,
	tcell.ColorAqua,
	tcell.ColorRed,
	tcell.ColorFuchsia,
	tcell.ColorYellow,
	tcell.ColorWhite,
}

package display

// GraphicsPlane is a bus-attached pixel buffer for the VIC's
// graphics mode. Unlike the character/color planes (40×13 cells with
// one byte each), a graphics plane is a flat array of pixels packed
// at 1, 2, 4, or 8 bits per pixel, indexed into a 16-color CGA-style
// palette.
//
// The plane is written directly by the CPU through bus writes, OR
// indirectly via the VIC controller's CmdGfx* commands (Clear, Plot,
// Line, RectFill, etc.). Renderers (browser canvas, terminal block-
// art) read pixels back via GetPixel.
//
// This component is opt-in — controllers built via NewController do
// not have a graphics plane attached. Use NewControllerWithGraphics
// (or assign Controller.Graphics directly) to enable graphics mode.
type GraphicsPlane struct {
	name   string
	base   uint16
	w, h   int
	bpp    int
	stride int // bytes per row
	size   int // total bytes
	data   []byte
}

// GraphicsConfig parameterizes a GraphicsPlane.
//
// Resolution × bit depth determines RAM usage:
//   - 160×104 @ 4bpp = 8,320 bytes (16-color, palette indexed)
//   - 320×104 @ 4bpp = 16,640 bytes
//   - 320×200 @ 1bpp = 8,000 bytes (mono)
type GraphicsConfig struct {
	Name   string
	Base   uint16
	Width  int
	Height int
	BPP    int // 1, 2, 4, or 8
}

// NewGraphicsPlane builds a graphics plane at the given bus address.
// Panics if BPP is not 1/2/4/8 or dimensions are non-positive.
func NewGraphicsPlane(cfg GraphicsConfig) *GraphicsPlane {
	switch cfg.BPP {
	case 1, 2, 4, 8:
	default:
		panic("display.NewGraphicsPlane: BPP must be 1, 2, 4, or 8")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		panic("display.NewGraphicsPlane: width and height must be positive")
	}
	stride := (cfg.Width*cfg.BPP + 7) / 8
	return &GraphicsPlane{
		name:   cfg.Name,
		base:   cfg.Base,
		w:      cfg.Width,
		h:      cfg.Height,
		bpp:    cfg.BPP,
		stride: stride,
		size:   stride * cfg.Height,
		data:   make([]byte, stride*cfg.Height),
	}
}

// Bus.Component interface.
func (g *GraphicsPlane) Name() string { return g.name }
func (g *GraphicsPlane) Base() uint16 { return g.base }
func (g *GraphicsPlane) Size() int    { return g.size }

func (g *GraphicsPlane) Read(offset uint16) uint8 {
	if int(offset) >= g.size {
		return 0
	}
	return g.data[offset]
}

func (g *GraphicsPlane) Write(offset uint16, val uint8) {
	if int(offset) >= g.size {
		return
	}
	g.data[offset] = val
}

// Pixel-level accessors used by the Controller's graphics commands
// and by renderers.
func (g *GraphicsPlane) Width() int   { return g.w }
func (g *GraphicsPlane) Height() int  { return g.h }
func (g *GraphicsPlane) BPP() int     { return g.bpp }
func (g *GraphicsPlane) Stride() int  { return g.stride }
func (g *GraphicsPlane) Bytes() []byte { return g.data }

// pixelMask is (1 << bpp) - 1.
func (g *GraphicsPlane) pixelMask() byte { return byte((1 << g.bpp) - 1) }

// GetPixel returns the palette index at (x, y). Out-of-bounds returns 0.
//
// Pixel ordering within a byte is most-significant-bits first — i.e.
// for 4bpp, pixel 0 in a byte is the high nibble, pixel 1 the low.
// This matches how most period-accurate graphics formats stored data.
func (g *GraphicsPlane) GetPixel(x, y int) uint8 {
	if x < 0 || y < 0 || x >= g.w || y >= g.h {
		return 0
	}
	bitOffset := x * g.bpp
	off := y*g.stride + bitOffset/8
	bitWithin := bitOffset % 8
	shift := 8 - g.bpp - bitWithin
	return (g.data[off] >> shift) & g.pixelMask()
}

// SetPixel writes a palette index at (x, y). Out-of-bounds is a no-op.
// The color is masked to the plane's bit depth.
func (g *GraphicsPlane) SetPixel(x, y int, color uint8) {
	if x < 0 || y < 0 || x >= g.w || y >= g.h {
		return
	}
	mask := g.pixelMask()
	color &= mask
	bitOffset := x * g.bpp
	off := y*g.stride + bitOffset/8
	bitWithin := bitOffset % 8
	shift := 8 - g.bpp - bitWithin
	g.data[off] = (g.data[off] &^ (mask << shift)) | (color << shift)
}

// Clear fills the entire plane with the given palette index.
func (g *GraphicsPlane) Clear(color uint8) {
	mask := g.pixelMask()
	color &= mask
	pixelsPerByte := 8 / g.bpp
	var fill byte
	for i := 0; i < pixelsPerByte; i++ {
		fill |= color << (i * g.bpp)
	}
	for i := range g.data {
		g.data[i] = fill
	}
}

// FillRect fills the inclusive rect (x0, y0, w, h) with color.
// Coordinates are clamped to the plane's dimensions.
func (g *GraphicsPlane) FillRect(x0, y0, w, h int, color uint8) {
	if w <= 0 || h <= 0 {
		return
	}
	x1 := x0 + w
	y1 := y0 + h
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > g.w {
		x1 = g.w
	}
	if y1 > g.h {
		y1 = g.h
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			g.SetPixel(x, y, color)
		}
	}
}

// StrokeRect draws the outline of the rect with color (1px line width).
func (g *GraphicsPlane) StrokeRect(x0, y0, w, h int, color uint8) {
	if w <= 0 || h <= 0 {
		return
	}
	x1 := x0 + w - 1
	y1 := y0 + h - 1
	for x := x0; x <= x1; x++ {
		g.SetPixel(x, y0, color)
		g.SetPixel(x, y1, color)
	}
	for y := y0; y <= y1; y++ {
		g.SetPixel(x0, y, color)
		g.SetPixel(x1, y, color)
	}
}

// Line draws a Bresenham line from (x0, y0) to (x1, y1) inclusive.
func (g *GraphicsPlane) Line(x0, y0, x1, y1 int, color uint8) {
	dx := x1 - x0
	if dx < 0 {
		dx = -dx
	}
	dy := y1 - y0
	if dy < 0 {
		dy = -dy
	}
	sx := 1
	if x0 > x1 {
		sx = -1
	}
	sy := 1
	if y0 > y1 {
		sy = -1
	}
	err := dx - dy
	for {
		g.SetPixel(x0, y0, color)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

// Circle draws a midpoint-algorithm circle outline at (cx, cy) radius r.
func (g *GraphicsPlane) Circle(cx, cy, r int, color uint8) {
	if r < 0 {
		return
	}
	x := r
	y := 0
	err := 1 - x
	for x >= y {
		g.SetPixel(cx+x, cy+y, color)
		g.SetPixel(cx-x, cy+y, color)
		g.SetPixel(cx+x, cy-y, color)
		g.SetPixel(cx-x, cy-y, color)
		g.SetPixel(cx+y, cy+x, color)
		g.SetPixel(cx-y, cy+x, color)
		g.SetPixel(cx+y, cy-x, color)
		g.SetPixel(cx-y, cy-x, color)
		y++
		if err < 0 {
			err += 2*y + 1
		} else {
			x--
			err += 2*(y-x) + 1
		}
	}
}

// FillCircle draws a filled circle at (cx, cy) radius r.
func (g *GraphicsPlane) FillCircle(cx, cy, r int, color uint8) {
	if r < 0 {
		return
	}
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			if x*x+y*y <= r*r {
				g.SetPixel(cx+x, cy+y, color)
			}
		}
	}
}

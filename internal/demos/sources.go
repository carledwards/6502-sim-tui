package demos

// Embedded ca65 demo sources. These are the canonical demo programs,
// assembled by go6asm at package init (see demos.go). The .s files
// live in asmsrc/ — a non-package dir so the Go toolchain doesn't try
// to assemble them as Plan 9 assembly.

import _ "embed"

//go:embed asmsrc/marquee.s
var marqueeSrc []byte

//go:embed asmsrc/bouncer.s
var bouncerSrc []byte

//go:embed asmsrc/scroller.s
var scrollerSrc []byte

//go:embed asmsrc/snow.s
var snowSrc []byte

//go:embed asmsrc/blitter.s
var blitterSrc []byte

//go:embed asmsrc/scroller_framed.s
var scrollerFramedSrc []byte

//go:embed asmsrc/quad.s
var quadSrc []byte

//go:embed asmsrc/balls.s
var ballsSrc []byte

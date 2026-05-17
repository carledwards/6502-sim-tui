package demos

// Quality gate: every shipped demo, assembled by go6asm for the
// sim-tui target, must pass go6asm's static analyzer with zero
// findings. A demo edit that introduces a ROM write, an unmapped
// access, a reachable BRK, or stray bytes executed as code fails CI
// here before it ever reaches the simulator.

import (
	"testing"

	"github.com/carledwards/go6asm/analyze"
	go6 "github.com/carledwards/go6asm/asm"
	"github.com/carledwards/go6asm/target"
)

func TestDemosAnalyzeClean(t *testing.T) {
	src := map[string][]byte{
		"hello.s": helloSrc, "marquee.s": marqueeSrc, "bouncer.s": bouncerSrc,
		"scroller.s": scrollerSrc, "scroller_framed.s": scrollerFramedSrc,
		"snow.s": snowSrc, "blitter.s": blitterSrc,
		"quad.s": quadSrc, "balls.s": ballsSrc,
	}
	tgt, ok := target.Builtin("sim-tui")
	if !ok {
		t.Fatal("sim-tui target missing")
	}
	var regions []analyze.Region
	for _, r := range tgt.Regions {
		regions = append(regions, analyze.Region{
			Lo: r.Lo, Hi: r.Hi, ReadOnly: r.ReadOnly, Name: r.Name,
		})
	}

	for name, code := range src {
		t.Run(name, func(t *testing.T) {
			res := go6.Assemble(go6.Input{
				Entry:  name,
				Files:  []go6.SourceFile{{Name: name, Content: code}},
				Layer0: true,
				Target: "sim-tui",
			})
			if !res.Ok() {
				t.Fatalf("assemble: %v", res.Errors)
			}
			origin := int(res.Origin)
			end := origin + len(res.Image)
			inImage := func(a int) bool { return a >= origin && a < end }

			// Hardware vectors + program start only (not every label).
			entries := []uint16{res.Origin}
			v := int(tgt.VectorAddr)
			for _, off := range []int{0, 2, 4} {
				if inImage(v+off) && inImage(v+off+1) {
					a := uint16(res.Image[v+off-origin]) |
						uint16(res.Image[v+off+1-origin])<<8
					if a != 0 {
						entries = append(entries, a)
					}
				}
			}

			ar := analyze.Analyze(res.Image, analyze.Options{
				Origin: res.Origin, Entries: entries, Regions: regions,
			})
			if ar.HasFindings() {
				for _, d := range ar.SortedDiags() {
					t.Logf("%s[%s] %s", d.Severity, d.Code, d.Message)
				}
				t.Fatalf("%s: analyzer reported findings", name)
			}
		})
	}
}

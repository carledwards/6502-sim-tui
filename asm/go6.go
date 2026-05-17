package asm

// go6.go bridges go6asm (the canonical ca65-syntax assembler) into the
// simulator's existing Program shape. go6asm owns symbol/comment
// semantics; this adapter conforms to it, not the reverse — the
// fluent Builder in this package is being retired in favor of .s files.
//
// FromSource assembles a ca65 source file with go6asm's Layer-0
// "sim-tui" target: it infers the $E000 load address and synthesizes
// the NMI/RESET/IRQ block at $FFFA, so the returned Bytes are the full
// $E000-$FFFF ROM image ready for rom.Load(0, …).

import (
	"fmt"
	"strings"

	go6 "github.com/carledwards/go6asm/asm"
)

// FromSource assembles src (ca65 syntax) for the sim-tui target and
// returns it as a Program. name is used only in diagnostics.
func FromSource(name string, src []byte) (Program, error) {
	r := go6.Assemble(go6.Input{
		Entry:  name,
		Files:  []go6.SourceFile{{Name: name, Content: src}},
		Layer0: true,
		Target: "sim-tui",
	})
	if !r.Ok() {
		var b strings.Builder
		for _, d := range r.Errors {
			b.WriteString(d.Render())
		}
		return Program{}, fmt.Errorf("go6asm: %s failed:\n%s", name, b.String())
	}

	p := Program{Base: r.Origin, Bytes: r.Image}
	for _, s := range r.Symbols {
		p.Symbols = append(p.Symbols, Symbol{
			Name: s.Name, Addr: s.Addr, Size: 1,
		})
	}
	// go6asm keys comments by the start PC of the emitting statement;
	// Length 1 is a safe lower bound (the disasm column looks comments
	// up by PC). Refine with real instruction spans later.
	for pc, text := range r.Comments {
		p.Annotations = append(p.Annotations, Annotation{
			PC: pc, Length: 1, Comment: text,
		})
	}
	return p, nil
}

// MustFromSource is FromSource for package-init demo construction:
// a build failure is a programming error in a bundled .s file.
func MustFromSource(name string, src []byte) Program {
	p, err := FromSource(name, src)
	if err != nil {
		panic(err)
	}
	return p
}

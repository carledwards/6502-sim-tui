// Package asm is the embedded 6502 assembler used by the simulator's
// demo programs. It produces a Program — bytes plus per-instruction
// comments and named memory symbols — that downstream tools can use
// to label memory views, annotate the disassembly column, or detect
// reads/writes outside declared regions for debugging.
//
// The assembler is fluent: methods return *Builder so calls chain.
// Forward references via labels (JMP/JSR/branches to a label defined
// later) are back-patched on Build, so demo authors don't hand-count
// branch offsets.
//
//	a := asm.New(0xE000)
//	a.Symbol("TICK_LO", 0x00, 1, "system frame counter")
//	a.Comment("snapshot the tick counter").
//	  LdaZP(0x00).
//	  StaZP(0x24)
//	a.Label("WAIT")
//	a.LdaZP(0x00)
//	a.CmpZP(0x24)
//	a.BEQ("WAIT")
//	prog := a.Build()
//
// The package intentionally exposes only what the demos need —
// addressing modes can be added on demand. Two-pass assembly with
// macros / arithmetic / forward expressions is out of scope; this is
// a teaching tool and a structured byte-builder, not gas.
package asm

import "fmt"

// Annotation is a single instruction's metadata in a Program.
type Annotation struct {
	PC      uint16 // address of the first byte
	Length  int    // size of the instruction in bytes
	Comment string // human-readable description
}

// Symbol names a region of memory the program touches. Used by
// memory views (label column), the disassembler (operand → label
// substitution), and future debuggers (region-bounded read/write).
type Symbol struct {
	Name string
	Addr uint16
	Size int    // 1 byte, 2 bytes (word), or N bytes (region)
	Note string // optional human description
}

// Program is the output of a Builder. Bytes is what gets loaded into
// ROM; Annotations and Symbols are metadata the simulator consults
// for labels, disassembly comments, and debugging.
type Program struct {
	Base        uint16
	Bytes       []byte
	Annotations []Annotation
	Symbols     []Symbol
}

// Builder accumulates bytes, comments, symbols, and unresolved
// branch/jump fixes during demo construction. Use New to construct
// one and Build to finalize.
type Builder struct {
	base   uint16
	code   []byte
	labels map[string]uint16
	fixes  []fix

	// Pending comment for the NEXT emitted instruction. Set via
	// Comment(...) before the emitter, consumed on the next emit
	// and cleared. Multiple consecutive Comment calls overwrite —
	// the latest wins.
	pendingComment string

	annotations []Annotation
	symbols     []Symbol
}

type fix struct {
	pos    int    // index into code where the operand bytes go
	target string // label name to resolve
	rel    bool   // true for relative branches (1-byte signed offset)
	relPC  uint16 // PC after the branch instruction (for relative offset math)
}

// New constructs a Builder anchored at base. base is typically the
// reset vector address ($E000 in this simulator).
func New(base uint16) *Builder {
	return &Builder{
		base:   base,
		labels: map[string]uint16{},
	}
}

// PC returns the current program counter — the address the next
// emitted byte will land at.
func (a *Builder) PC() uint16 { return a.base + uint16(len(a.code)) }

// Comment attaches a comment to the NEXT instruction emitted by this
// Builder. Returns the Builder so calls chain.
func (a *Builder) Comment(text string) *Builder {
	a.pendingComment = text
	return a
}

// Symbol declares a named memory region. Reported back in the final
// Program; consumed by memory-view label rendering and (eventually)
// the debugger.
func (a *Builder) Symbol(name string, addr uint16, size int, note string) *Builder {
	a.symbols = append(a.symbols, Symbol{Name: name, Addr: addr, Size: size, Note: note})
	return a
}

// Label marks the current PC with a name so subsequent JMP / JSR /
// branch calls can refer to it. Forward references resolve on Build.
func (a *Builder) Label(name string) *Builder {
	if _, exists := a.labels[name]; exists {
		panic(fmt.Sprintf("asm: duplicate label %q", name))
	}
	a.labels[name] = a.PC()
	return a
}

// Emit writes raw bytes — escape hatch for opcodes the package's
// emitters don't cover. Consumes any pendingComment.
func (a *Builder) Emit(bytes ...byte) *Builder {
	a.flushAnnotation(len(bytes))
	a.code = append(a.code, bytes...)
	return a
}

// flushAnnotation captures the pending comment (if any) into an
// Annotation describing the instruction about to be emitted.
func (a *Builder) flushAnnotation(length int) {
	if a.pendingComment == "" {
		return
	}
	a.annotations = append(a.annotations, Annotation{
		PC:      a.PC(),
		Length:  length,
		Comment: a.pendingComment,
	})
	a.pendingComment = ""
}

// addrFix records a 16-bit absolute-address fix at the current code
// position. Two zero bytes are emitted as placeholders.
func (a *Builder) addrFix(target string) {
	a.fixes = append(a.fixes, fix{pos: len(a.code), target: target})
	a.code = append(a.code, 0, 0)
}

// relFix records a 1-byte relative-branch fix at the current code
// position. relPC is the PC after the branch instruction (operand
// is signed offset relative to that).
func (a *Builder) relFix(target string) {
	relPC := a.PC() + 1 // the operand byte is here, next instruction starts at +1
	a.fixes = append(a.fixes, fix{pos: len(a.code), target: target, rel: true, relPC: relPC})
	a.code = append(a.code, 0)
}

// Build resolves all label fixes and returns the finished Program.
// Panics if any fix references an undefined label or a relative
// branch is out of range.
func (a *Builder) Build() Program {
	for _, f := range a.fixes {
		addr, ok := a.labels[f.target]
		if !ok {
			panic(fmt.Sprintf("asm: undefined label %q", f.target))
		}
		if f.rel {
			off := int(addr) - int(f.relPC)
			if off < -128 || off > 127 {
				panic(fmt.Sprintf("asm: relative branch to %q out of range (%d)", f.target, off))
			}
			a.code[f.pos] = byte(off)
		} else {
			a.code[f.pos+0] = byte(addr & 0xFF)
			a.code[f.pos+1] = byte(addr >> 8)
		}
	}
	return Program{
		Base:        a.base,
		Bytes:       a.code,
		Annotations: a.annotations,
		Symbols:     a.symbols,
	}
}

package asm

// 6502 opcode emitters. Only the addressing modes used by demos are
// covered. Each method consumes any pending Comment, emits the
// instruction's bytes, and returns the Builder for chaining.

// ─── Loads / stores ──────────────────────────────────────────

// LDA #imm — load A from immediate.
func (a *Builder) LdaImm(v byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0xA9, v); return a }

// LDA $zp — load A from zero page.
func (a *Builder) LdaZP(zp byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0xA5, zp); return a }

// LDA $zp,X — load A from zero page indexed by X.
func (a *Builder) LdaZPX(zp byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0xB5, zp); return a }

// LDA $abs — load A from absolute address.
func (a *Builder) LdaAbs(addr uint16) *Builder {
	a.flushAnnotation(3)
	a.code = append(a.code, 0xAD, byte(addr), byte(addr>>8))
	return a
}

// LDA $abs,X — load A from absolute,X.
func (a *Builder) LdaAbsX(addr uint16) *Builder {
	a.flushAnnotation(3)
	a.code = append(a.code, 0xBD, byte(addr), byte(addr>>8))
	return a
}

// LDA $abs,X with label resolution — useful when the absolute base
// is the address of a forward-declared label (e.g. a data table
// laid out after the code that reads it).
func (a *Builder) LdaAbsXLabel(label string) *Builder {
	a.flushAnnotation(3)
	a.code = append(a.code, 0xBD)
	a.addrFix(label)
	return a
}

// LDX #imm — load X from immediate.
func (a *Builder) LdxImm(v byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0xA2, v); return a }

// LDX $zp — load X from zero page.
func (a *Builder) LdxZP(zp byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0xA6, zp); return a }

// LDY #imm — load Y from immediate.
func (a *Builder) LdyImm(v byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0xA0, v); return a }

// STA $zp — store A to zero page.
func (a *Builder) StaZP(zp byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0x85, zp); return a }

// STA $zp,X — store A to zero page,X.
func (a *Builder) StaZPX(zp byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0x95, zp); return a }

// STA $abs — store A to absolute.
func (a *Builder) StaAbs(addr uint16) *Builder {
	a.flushAnnotation(3)
	a.code = append(a.code, 0x8D, byte(addr), byte(addr>>8))
	return a
}

// STA $abs,X — store A to absolute,X.
func (a *Builder) StaAbsX(addr uint16) *Builder {
	a.flushAnnotation(3)
	a.code = append(a.code, 0x9D, byte(addr), byte(addr>>8))
	return a
}

// STX $zp — store X to zero page.
func (a *Builder) StxZP(zp byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0x86, zp); return a }

// ─── Arithmetic / logic ──────────────────────────────────────

// ADC #imm — add immediate to A with carry.
func (a *Builder) AdcImm(v byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0x69, v); return a }

// ADC $zp — add zero page value to A with carry.
func (a *Builder) AdcZP(zp byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0x65, zp); return a }

// ADC $zp,X — add zero page,X value to A with carry.
func (a *Builder) AdcZPX(zp byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0x75, zp); return a }

// AND #imm — bitwise AND immediate with A.
func (a *Builder) AndImm(v byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0x29, v); return a }

// EOR #imm — XOR immediate with A.
func (a *Builder) EorImm(v byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0x49, v); return a }

// CMP #imm — compare A to immediate.
func (a *Builder) CmpImm(v byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0xC9, v); return a }

// CMP $zp — compare A to zero page byte.
func (a *Builder) CmpZP(zp byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0xC5, zp); return a }

// CPX #imm — compare X to immediate.
func (a *Builder) CpxImm(v byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0xE0, v); return a }

// LSR A — logical shift right accumulator.
func (a *Builder) LsrA() *Builder { a.flushAnnotation(1); a.code = append(a.code, 0x4A); return a }

// CLC — clear carry.
func (a *Builder) CLC() *Builder { a.flushAnnotation(1); a.code = append(a.code, 0x18); return a }

// SEC — set carry.
func (a *Builder) SEC() *Builder { a.flushAnnotation(1); a.code = append(a.code, 0x38); return a }

// ─── Increments / decrements ─────────────────────────────────

// INC $zp — increment zero page byte.
func (a *Builder) IncZP(zp byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0xE6, zp); return a }

// INC $zp,X — increment zero page,X byte.
func (a *Builder) IncZPX(zp byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0xF6, zp); return a }

// DEC $zp — decrement zero page byte.
func (a *Builder) DecZP(zp byte) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0xC6, zp); return a }

// INX / DEX / INY / DEY — register increments/decrements.
func (a *Builder) INX() *Builder { a.flushAnnotation(1); a.code = append(a.code, 0xE8); return a }
func (a *Builder) DEX() *Builder { a.flushAnnotation(1); a.code = append(a.code, 0xCA); return a }
func (a *Builder) INY() *Builder { a.flushAnnotation(1); a.code = append(a.code, 0xC8); return a }
func (a *Builder) DEY() *Builder { a.flushAnnotation(1); a.code = append(a.code, 0x88); return a }

// ─── Transfers ───────────────────────────────────────────────

// TXA — transfer X to A.
func (a *Builder) TXA() *Builder { a.flushAnnotation(1); a.code = append(a.code, 0x8A); return a }

// ─── Branches (relative; resolve to label) ──────────────────

// BNE label — branch if Z=0.
func (a *Builder) BNE(label string) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0xD0); a.relFix(label); return a }

// BEQ label — branch if Z=1.
func (a *Builder) BEQ(label string) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0xF0); a.relFix(label); return a }

// BCS label — branch if C=1.
func (a *Builder) BCS(label string) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0xB0); a.relFix(label); return a }

// BCC label — branch if C=0.
func (a *Builder) BCC(label string) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0x90); a.relFix(label); return a }

// BPL label — branch if N=0.
func (a *Builder) BPL(label string) *Builder { a.flushAnnotation(2); a.code = append(a.code, 0x10); a.relFix(label); return a }

// ─── Jumps / subroutines ─────────────────────────────────────

// JMP $abs — unconditional jump to label (resolved on Build).
func (a *Builder) JMP(label string) *Builder { a.flushAnnotation(3); a.code = append(a.code, 0x4C); a.addrFix(label); return a }

// JMPAddr $abs — jump to a literal absolute address (no label).
func (a *Builder) JMPAddr(addr uint16) *Builder {
	a.flushAnnotation(3)
	a.code = append(a.code, 0x4C, byte(addr), byte(addr>>8))
	return a
}

// JSR label — call subroutine.
func (a *Builder) JSR(label string) *Builder { a.flushAnnotation(3); a.code = append(a.code, 0x20); a.addrFix(label); return a }

// RTS — return from subroutine.
func (a *Builder) RTS() *Builder { a.flushAnnotation(1); a.code = append(a.code, 0x60); return a }

// ─── Data ────────────────────────────────────────────────────

// Bytes inserts raw data bytes (e.g. literal strings, tables).
// Distinct from Emit in that there's no expectation of being an
// instruction — useful for data-after-code patterns.
func (a *Builder) Bytes(bs ...byte) *Builder {
	a.flushAnnotation(len(bs))
	a.code = append(a.code, bs...)
	return a
}

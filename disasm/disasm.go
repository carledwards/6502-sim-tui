// Package disasm decodes 6502 instructions to assembly mnemonics.
//
// The opcode table covers the 151 official MOS 6502 instructions.
// Illegal/undefined opcodes decode to a single-byte "???" entry.
package disasm

import (
	"fmt"
	"strings"
)

// Mode is a 6502 addressing mode.
type Mode uint8

const (
	Implied Mode = iota
	Accumulator
	Immediate
	ZeroPage
	ZeroPageX
	ZeroPageY
	Absolute
	AbsoluteX
	AbsoluteY
	Indirect
	IndirectX
	IndirectY
	Relative
)

// Size returns the number of bytes consumed (including the opcode).
func (m Mode) Size() int {
	switch m {
	case Implied, Accumulator:
		return 1
	case Absolute, AbsoluteX, AbsoluteY, Indirect:
		return 3
	default:
		return 2
	}
}

// Op is one entry in the opcode table.
type Op struct {
	Mn string
	Md Mode
}

// Instr is a single decoded instruction at a known address.
type Instr struct {
	Addr   uint16
	Op     Op
	Bytes  []uint8
	Pretty string // mnemonic + formatted operand, e.g. "LDA #$42"
}

// Size returns the number of bytes the instruction occupies.
func (i Instr) Size() int { return len(i.Bytes) }

// Decode reads one instruction from `read` starting at `pc`.
func Decode(pc uint16, read func(uint16) uint8) Instr {
	opcode := read(pc)
	op := Table[opcode]
	size := op.Md.Size()
	bytes := make([]uint8, size)
	bytes[0] = opcode
	for i := 1; i < size; i++ {
		bytes[i] = read(pc + uint16(i))
	}
	return Instr{
		Addr:   pc,
		Op:     op,
		Bytes:  bytes,
		Pretty: format(pc, op, bytes),
	}
}

func format(pc uint16, op Op, b []uint8) string {
	mn := op.Mn
	switch op.Md {
	case Implied:
		return mn
	case Accumulator:
		return mn + " A"
	case Immediate:
		return fmt.Sprintf("%s #$%02X", mn, b[1])
	case ZeroPage:
		return fmt.Sprintf("%s $%02X", mn, b[1])
	case ZeroPageX:
		return fmt.Sprintf("%s $%02X,X", mn, b[1])
	case ZeroPageY:
		return fmt.Sprintf("%s $%02X,Y", mn, b[1])
	case Absolute:
		return fmt.Sprintf("%s $%04X", mn, addr16(b[1], b[2]))
	case AbsoluteX:
		return fmt.Sprintf("%s $%04X,X", mn, addr16(b[1], b[2]))
	case AbsoluteY:
		return fmt.Sprintf("%s $%04X,Y", mn, addr16(b[1], b[2]))
	case Indirect:
		return fmt.Sprintf("%s ($%04X)", mn, addr16(b[1], b[2]))
	case IndirectX:
		return fmt.Sprintf("%s ($%02X,X)", mn, b[1])
	case IndirectY:
		return fmt.Sprintf("%s ($%02X),Y", mn, b[1])
	case Relative:
		// Branch target = (PC + 2) + signed offset.
		off := int8(b[1])
		target := uint16(int(pc) + 2 + int(off))
		return fmt.Sprintf("%s $%04X", mn, target)
	}
	return mn
}

func addr16(lo, hi uint8) uint16 { return uint16(lo) | uint16(hi)<<8 }

// HexBytes formats the raw instruction bytes as space-separated hex,
// padded to the width of a 3-byte instruction so columns align.
func HexBytes(b []uint8) string {
	var sb strings.Builder
	for i, x := range b {
		if i > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%02X", x)
	}
	for i := len(b); i < 3; i++ {
		sb.WriteString("   ")
	}
	return sb.String()
}

// Table maps opcode → mnemonic + addressing mode.
//
// Illegal/undefined opcodes get the placeholder "???" with Implied
// (one-byte) decoding so disassembly can keep walking forward.
var Table = [256]Op{
	0x00: {"BRK", Implied},
	0x01: {"ORA", IndirectX},
	0x05: {"ORA", ZeroPage},
	0x06: {"ASL", ZeroPage},
	0x08: {"PHP", Implied},
	0x09: {"ORA", Immediate},
	0x0A: {"ASL", Accumulator},
	0x0D: {"ORA", Absolute},
	0x0E: {"ASL", Absolute},
	0x10: {"BPL", Relative},
	0x11: {"ORA", IndirectY},
	0x15: {"ORA", ZeroPageX},
	0x16: {"ASL", ZeroPageX},
	0x18: {"CLC", Implied},
	0x19: {"ORA", AbsoluteY},
	0x1D: {"ORA", AbsoluteX},
	0x1E: {"ASL", AbsoluteX},
	0x20: {"JSR", Absolute},
	0x21: {"AND", IndirectX},
	0x24: {"BIT", ZeroPage},
	0x25: {"AND", ZeroPage},
	0x26: {"ROL", ZeroPage},
	0x28: {"PLP", Implied},
	0x29: {"AND", Immediate},
	0x2A: {"ROL", Accumulator},
	0x2C: {"BIT", Absolute},
	0x2D: {"AND", Absolute},
	0x2E: {"ROL", Absolute},
	0x30: {"BMI", Relative},
	0x31: {"AND", IndirectY},
	0x35: {"AND", ZeroPageX},
	0x36: {"ROL", ZeroPageX},
	0x38: {"SEC", Implied},
	0x39: {"AND", AbsoluteY},
	0x3D: {"AND", AbsoluteX},
	0x3E: {"ROL", AbsoluteX},
	0x40: {"RTI", Implied},
	0x41: {"EOR", IndirectX},
	0x45: {"EOR", ZeroPage},
	0x46: {"LSR", ZeroPage},
	0x48: {"PHA", Implied},
	0x49: {"EOR", Immediate},
	0x4A: {"LSR", Accumulator},
	0x4C: {"JMP", Absolute},
	0x4D: {"EOR", Absolute},
	0x4E: {"LSR", Absolute},
	0x50: {"BVC", Relative},
	0x51: {"EOR", IndirectY},
	0x55: {"EOR", ZeroPageX},
	0x56: {"LSR", ZeroPageX},
	0x58: {"CLI", Implied},
	0x59: {"EOR", AbsoluteY},
	0x5D: {"EOR", AbsoluteX},
	0x5E: {"LSR", AbsoluteX},
	0x60: {"RTS", Implied},
	0x61: {"ADC", IndirectX},
	0x65: {"ADC", ZeroPage},
	0x66: {"ROR", ZeroPage},
	0x68: {"PLA", Implied},
	0x69: {"ADC", Immediate},
	0x6A: {"ROR", Accumulator},
	0x6C: {"JMP", Indirect},
	0x6D: {"ADC", Absolute},
	0x6E: {"ROR", Absolute},
	0x70: {"BVS", Relative},
	0x71: {"ADC", IndirectY},
	0x75: {"ADC", ZeroPageX},
	0x76: {"ROR", ZeroPageX},
	0x78: {"SEI", Implied},
	0x79: {"ADC", AbsoluteY},
	0x7D: {"ADC", AbsoluteX},
	0x7E: {"ROR", AbsoluteX},
	0x81: {"STA", IndirectX},
	0x84: {"STY", ZeroPage},
	0x85: {"STA", ZeroPage},
	0x86: {"STX", ZeroPage},
	0x88: {"DEY", Implied},
	0x8A: {"TXA", Implied},
	0x8C: {"STY", Absolute},
	0x8D: {"STA", Absolute},
	0x8E: {"STX", Absolute},
	0x90: {"BCC", Relative},
	0x91: {"STA", IndirectY},
	0x94: {"STY", ZeroPageX},
	0x95: {"STA", ZeroPageX},
	0x96: {"STX", ZeroPageY},
	0x98: {"TYA", Implied},
	0x99: {"STA", AbsoluteY},
	0x9A: {"TXS", Implied},
	0x9D: {"STA", AbsoluteX},
	0xA0: {"LDY", Immediate},
	0xA1: {"LDA", IndirectX},
	0xA2: {"LDX", Immediate},
	0xA4: {"LDY", ZeroPage},
	0xA5: {"LDA", ZeroPage},
	0xA6: {"LDX", ZeroPage},
	0xA8: {"TAY", Implied},
	0xA9: {"LDA", Immediate},
	0xAA: {"TAX", Implied},
	0xAC: {"LDY", Absolute},
	0xAD: {"LDA", Absolute},
	0xAE: {"LDX", Absolute},
	0xB0: {"BCS", Relative},
	0xB1: {"LDA", IndirectY},
	0xB4: {"LDY", ZeroPageX},
	0xB5: {"LDA", ZeroPageX},
	0xB6: {"LDX", ZeroPageY},
	0xB8: {"CLV", Implied},
	0xB9: {"LDA", AbsoluteY},
	0xBA: {"TSX", Implied},
	0xBC: {"LDY", AbsoluteX},
	0xBD: {"LDA", AbsoluteX},
	0xBE: {"LDX", AbsoluteY},
	0xC0: {"CPY", Immediate},
	0xC1: {"CMP", IndirectX},
	0xC4: {"CPY", ZeroPage},
	0xC5: {"CMP", ZeroPage},
	0xC6: {"DEC", ZeroPage},
	0xC8: {"INY", Implied},
	0xC9: {"CMP", Immediate},
	0xCA: {"DEX", Implied},
	0xCC: {"CPY", Absolute},
	0xCD: {"CMP", Absolute},
	0xCE: {"DEC", Absolute},
	0xD0: {"BNE", Relative},
	0xD1: {"CMP", IndirectY},
	0xD5: {"CMP", ZeroPageX},
	0xD6: {"DEC", ZeroPageX},
	0xD8: {"CLD", Implied},
	0xD9: {"CMP", AbsoluteY},
	0xDD: {"CMP", AbsoluteX},
	0xDE: {"DEC", AbsoluteX},
	0xE0: {"CPX", Immediate},
	0xE1: {"SBC", IndirectX},
	0xE4: {"CPX", ZeroPage},
	0xE5: {"SBC", ZeroPage},
	0xE6: {"INC", ZeroPage},
	0xE8: {"INX", Implied},
	0xE9: {"SBC", Immediate},
	0xEA: {"NOP", Implied},
	0xEC: {"CPX", Absolute},
	0xED: {"SBC", Absolute},
	0xEE: {"INC", Absolute},
	0xF0: {"BEQ", Relative},
	0xF1: {"SBC", IndirectY},
	0xF5: {"SBC", ZeroPageX},
	0xF6: {"INC", ZeroPageX},
	0xF8: {"SED", Implied},
	0xF9: {"SBC", AbsoluteY},
	0xFD: {"SBC", AbsoluteX},
	0xFE: {"INC", AbsoluteX},
}

func init() {
	for i, op := range Table {
		if op.Mn == "" {
			Table[i] = Op{"???", Implied}
		}
	}
}

// Cycles[opcode] returns the base cycle count for the instruction
// (ignoring page-cross +1 and branch-taken +1 — close enough for an
// educational annotation). Unknown opcodes default to 2.
var Cycles = [256]int{
	0x00: 7, 0x01: 6, 0x05: 3, 0x06: 5, 0x08: 3, 0x09: 2, 0x0A: 2, 0x0D: 4, 0x0E: 6,
	0x10: 2, 0x11: 5, 0x15: 4, 0x16: 6, 0x18: 2, 0x19: 4, 0x1D: 4, 0x1E: 7,
	0x20: 6, 0x21: 6, 0x24: 3, 0x25: 3, 0x26: 5, 0x28: 4, 0x29: 2, 0x2A: 2, 0x2C: 4, 0x2D: 4, 0x2E: 6,
	0x30: 2, 0x31: 5, 0x35: 4, 0x36: 6, 0x38: 2, 0x39: 4, 0x3D: 4, 0x3E: 7,
	0x40: 6, 0x41: 6, 0x45: 3, 0x46: 5, 0x48: 3, 0x49: 2, 0x4A: 2, 0x4C: 3, 0x4D: 4, 0x4E: 6,
	0x50: 2, 0x51: 5, 0x55: 4, 0x56: 6, 0x58: 2, 0x59: 4, 0x5D: 4, 0x5E: 7,
	0x60: 6, 0x61: 6, 0x65: 3, 0x66: 5, 0x68: 4, 0x69: 2, 0x6A: 2, 0x6C: 5, 0x6D: 4, 0x6E: 6,
	0x70: 2, 0x71: 5, 0x75: 4, 0x76: 6, 0x78: 2, 0x79: 4, 0x7D: 4, 0x7E: 7,
	0x81: 6, 0x84: 3, 0x85: 3, 0x86: 3, 0x88: 2, 0x8A: 2, 0x8C: 4, 0x8D: 4, 0x8E: 4,
	0x90: 2, 0x91: 6, 0x94: 4, 0x95: 4, 0x96: 4, 0x98: 2, 0x99: 5, 0x9A: 2, 0x9D: 5,
	0xA0: 2, 0xA1: 6, 0xA2: 2, 0xA4: 3, 0xA5: 3, 0xA6: 3, 0xA8: 2, 0xA9: 2, 0xAA: 2, 0xAC: 4, 0xAD: 4, 0xAE: 4,
	0xB0: 2, 0xB1: 5, 0xB4: 4, 0xB5: 4, 0xB6: 4, 0xB8: 2, 0xB9: 4, 0xBA: 2, 0xBC: 4, 0xBD: 4, 0xBE: 4,
	0xC0: 2, 0xC1: 6, 0xC4: 3, 0xC5: 3, 0xC6: 5, 0xC8: 2, 0xC9: 2, 0xCA: 2, 0xCC: 4, 0xCD: 4, 0xCE: 6,
	0xD0: 2, 0xD1: 5, 0xD5: 4, 0xD6: 6, 0xD8: 2, 0xD9: 4, 0xDD: 4, 0xDE: 7,
	0xE0: 2, 0xE1: 6, 0xE4: 3, 0xE5: 3, 0xE6: 5, 0xE8: 2, 0xE9: 2, 0xEA: 2, 0xEC: 4, 0xED: 4, 0xEE: 6,
	0xF0: 2, 0xF1: 5, 0xF5: 4, 0xF6: 6, 0xF8: 2, 0xF9: 4, 0xFD: 4, 0xFE: 7,
}

// effects describes the action of each mnemonic in compact algebraic
// form — small enough to fit alongside the disassembly line.
var effects = map[string]string{
	"ADC": "A ← A+M+C",
	"AND": "A ← A & M",
	"ASL": "M ← M << 1",
	"BCC": "branch if C=0",
	"BCS": "branch if C=1",
	"BEQ": "branch if Z=1",
	"BIT": "A & M → flags",
	"BMI": "branch if N=1",
	"BNE": "branch if Z=0",
	"BPL": "branch if N=0",
	"BRK": "soft IRQ",
	"BVC": "branch if V=0",
	"BVS": "branch if V=1",
	"CLC": "C ← 0",
	"CLD": "D ← 0",
	"CLI": "I ← 0",
	"CLV": "V ← 0",
	"CMP": "A − M → flags",
	"CPX": "X − M → flags",
	"CPY": "Y − M → flags",
	"DEC": "M ← M-1",
	"DEX": "X ← X-1",
	"DEY": "Y ← Y-1",
	"EOR": "A ← A ^ M",
	"INC": "M ← M+1",
	"INX": "X ← X+1",
	"INY": "Y ← Y+1",
	"JMP": "PC ← target",
	"JSR": "push PC; PC ← tgt",
	"LDA": "A ← M",
	"LDX": "X ← M",
	"LDY": "Y ← M",
	"LSR": "M ← M >> 1",
	"NOP": "no op",
	"ORA": "A ← A | M",
	"PHA": "push A",
	"PHP": "push P",
	"PLA": "A ← pull",
	"PLP": "P ← pull",
	"ROL": "M ← rot-left",
	"ROR": "M ← rot-right",
	"RTI": "P,PC ← pull",
	"RTS": "PC ← pull+1",
	"SBC": "A ← A-M-(1-C)",
	"SEC": "C ← 1",
	"SED": "D ← 1",
	"SEI": "I ← 1",
	"STA": "M ← A",
	"STX": "M ← X",
	"STY": "M ← Y",
	"TAX": "X ← A",
	"TAY": "Y ← A",
	"TSX": "X ← SP",
	"TXA": "A ← X",
	"TXS": "SP ← X",
	"TYA": "A ← Y",
}

// Effect returns a short algebraic description of the mnemonic's
// action, or "" if unknown.
func Effect(mn string) string { return effects[mn] }


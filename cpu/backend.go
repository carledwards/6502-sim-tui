// Package cpu defines the simulator's CPU backend interface, plus
// types shared across implementations. Concrete backends live in
// subpackages (cpu/netsim, cpu/interp).
package cpu

// Registers is a snapshot of 6502 architectural state.
type Registers struct {
	A, X, Y, S, P uint8
	PC            uint16
}

// Status flag bits within the P register.
const (
	FlagC uint8 = 1 << 0 // Carry
	FlagZ uint8 = 1 << 1 // Zero
	FlagI uint8 = 1 << 2 // Interrupt disable
	FlagD uint8 = 1 << 3 // Decimal mode
	FlagB uint8 = 1 << 4 // Break (set when pushed by BRK/PHP)
	FlagU uint8 = 1 << 5 // Unused (conventionally 1 when pushed)
	FlagV uint8 = 1 << 6 // Overflow
	FlagN uint8 = 1 << 7 // Negative
)

// Backend is the contract every CPU implementation satisfies. The UI
// only ever talks to a Backend — never to a concrete impl.
type Backend interface {
	// Reset performs the standard 6502 power-on / reset sequence.
	Reset()

	// HalfStep advances the simulation by half a clock cycle.
	HalfStep()

	// Registers returns the current architectural state.
	Registers() Registers

	// HalfCycles returns the count of HalfStep calls since the last
	// Reset.
	HalfCycles() uint64

	// AddressBus returns the value currently on the 16-bit address
	// bus pins. For netsim this is read live from the silicon nodes;
	// for interp it's the address of the most recent bus access.
	AddressBus() uint16

	// DataBus returns the value currently on the 8-bit data bus.
	DataBus() uint8

	// ReadCycle reports whether the most recent bus access was a
	// read (R/W = high). False = write.
	ReadCycle() bool

	// IRQ / NMI report the state of the interrupt-request pins.
	// True = high = inactive (the 6502's IRQ and NMI lines are
	// active-low). For interp these are always true since the
	// interpretive core doesn't model interrupt inputs.
	IRQ() bool
	NMI() bool
}

package via

import (
	"testing"
	"time"
)

// Programs T1 in free-run with a tiny latch (1000 cycles = 1 ms at
// 1 MHz). Then drives Tick(2 ms) and expects the IFR T1 flag set.
func TestT1FreeRunPaces(t *testing.T) {
	v := New("via", 0x0000, 1_000_000)

	// Latch = 1000 = $03E8.
	v.Write(RegT1LL, 0xE8) // latch low (no start)
	v.Write(RegT1CH, 0x03) // latch high — starts T1 and clears IFR
	v.Write(RegACR, ACR_T1_FREERUN)

	// 2 ms = 2000 cycles → at least one underflow (period ~1001 cycles).
	v.Tick(2 * time.Millisecond)

	ifr := v.Read(RegIFR)
	if ifr&IFR_T1 == 0 {
		t.Fatalf("expected IFR T1 set after 2 ms, got %02X", ifr)
	}
	if ifr&IFR_IRQ != 0 {
		t.Fatalf("IRQ bit should require IER enable, got IFR=%02X (IER=0)", ifr)
	}
}

// Reading T1C-L clears the IFR T1 flag — canonical "ack the timer"
// trick used by every 65C22 polling loop.
func TestT1CL_ReadClearsIFR(t *testing.T) {
	v := New("via", 0x0000, 1_000_000)
	v.Write(RegT1LL, 0x10)
	v.Write(RegT1CH, 0x00) // latch=$0010, start
	v.Write(RegACR, ACR_T1_FREERUN)

	v.Tick(1 * time.Millisecond)
	if v.Read(RegIFR)&IFR_T1 == 0 {
		t.Fatal("precondition: T1 flag should be set")
	}
	_ = v.Read(RegT1CL) // clears IFR T1
	if v.Read(RegIFR)&IFR_T1 != 0 {
		t.Fatal("expected IFR T1 cleared after T1C-L read")
	}
}

// IFR bit 7 (IRQ) reads as 1 only when an enabled flag is set. With
// IER=0, the bit stays 0 even though the underlying flag is set.
func TestIFR_BIT7_RequiresIER(t *testing.T) {
	v := New("via", 0x0000, 1_000_000)
	v.Write(RegT1LL, 0x10)
	v.Write(RegT1CH, 0x00)
	v.Write(RegACR, ACR_T1_FREERUN)
	v.Tick(1 * time.Millisecond)

	if v.Read(RegIFR)&IFR_IRQ != 0 {
		t.Fatal("IRQ bit set without IER mask")
	}

	// Enable T1 in IER (bit 7=1 means "set these bits").
	v.Write(RegIER, 0x80|IFR_T1)
	if v.Read(RegIFR)&IFR_IRQ == 0 {
		t.Fatal("IRQ bit should reflect enabled+set flag")
	}
}

// One-shot mode underflows once, then disarms. A second Tick after
// underflow should not re-set the flag if it was already cleared.
func TestT1OneShotDisarms(t *testing.T) {
	v := New("via", 0x0000, 1_000_000)
	v.Write(RegT1LL, 0x10)
	v.Write(RegT1CH, 0x00)
	// ACR=0 → one-shot.
	v.Tick(1 * time.Millisecond)
	if v.Read(RegIFR)&IFR_T1 == 0 {
		t.Fatal("one-shot should fire once")
	}
	_ = v.Read(RegT1CL) // clear
	v.Tick(10 * time.Millisecond)
	if v.Read(RegIFR)&IFR_T1 != 0 {
		t.Fatal("one-shot should not refire without restart")
	}
}

// IER write semantics: bit 7=1 sets, bit 7=0 clears the masked bits.
func TestIER_SetClear(t *testing.T) {
	v := New("via", 0x0000, 1_000_000)
	v.Write(RegIER, 0x80|0x42) // set bits 1, 6
	if got := v.Read(RegIER); got != 0x80|0x42 {
		t.Fatalf("IER set: got %02X want %02X", got, 0x80|0x42)
	}
	v.Write(RegIER, 0x40) // bit 7=0, clear bit 6
	if got := v.Read(RegIER); got != 0x80|0x02 {
		t.Fatalf("IER clear: got %02X want %02X", got, 0x80|0x02)
	}
}

; Snow — fills both VIC planes with pseudo-random coloured characters,
; pauses so the user sees it, clears via the controller, and repeats.
;
; Randomness is an 8-bit Galois LFSR held in ZP LFSR. Each step shifts
; right; if the bit shifted out was 1, XOR the tap mask $B8. That
; visits all 255 non-zero states before repeating — cheap noise with
; no multiply/divide. The char byte is derived from the same value
; (EOR #$5A) so colour and glyph differ but share the sequence.
;
; The plane is 512 cells, so it's painted in two 256-cell passes:
; PASS1 = page 0 ($A000/$A400), PASS2 = page 1 ($A100/$A500).

LFSR = $10                     ; 8-bit Galois LFSR state (taps = $B8)
.exportzp LFSR

        LDA #$01               ; seed = 1 (any non-zero starts the LFSR)
        STA LFSR

OUTER:
        LDX #0

; PASS1 — cells 0..255 (X wraps 255->0 to end the loop).
PASS1:
        LDA LFSR               ; step LFSR: shift right, XOR taps if bit 0 was set
        LSR A
        BCC P1_NOXOR
        EOR #$B8               ; tap mask (only when carry/bit0 was set)
P1_NOXOR:
        STA LFSR               ; save new LFSR state
        STA ColorBase,X        ; paint color plane page 0
        EOR #$5A               ; derive char from same byte
        STA CharBase,X         ; paint char plane page 0
        INX
        BNE PASS1

; PASS2 — cells 256..511 (page 1).
        LDX #0
PASS2:
        LDA LFSR
        LSR A
        BCC P2_NOXOR
        EOR #$B8
P2_NOXOR:
        STA LFSR
        STA ColorBase+$100,X   ; color plane page 1
        EOR #$5A
        STA CharBase+$100,X    ; char plane page 1
        INX
        BNE PASS2

; Nested delay so the snow is visible before the clear.
        LDY #$10               ; outer delay counter
D_OUT:
        LDX #$FF               ; inner delay counter
D_IN:
        DEX
        BNE D_IN
        DEY
        BNE D_OUT

        LDA #CmdClear          ; clear via VIC, then loop
        STA RegCmd
        JMP OUTER

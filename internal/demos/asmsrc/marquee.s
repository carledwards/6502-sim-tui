; Marquee — "HELLO 6502 SIM" scrolls across row 6.
; VIA Timer 1 free-run pacing: scroll speed tracks the VIA crystal,
; independent of the CPU clock.

.export MSG                    ; surface the message table in the memory view

        LDA #CmdClear          ; clear the screen via VIC controller
        STA RegCmd

        LDA #ViaT1Bit          ; ACR bit 6 = T1 free-run (auto-reload from latch)
        STA ViaACR
        LDA #$FF               ; VIA T1 latch low: $FF
        STA ViaT1L_L
        LDA #$FF               ; VIA T1 latch high: $FF — arms T1 in free-run mode
        STA ViaT1C_H

        LDX #0                 ; loop counter for message copy
MSG_LOOP:
        LDA MSG,X              ; read message byte from MSG table (laid out after code)
        STA CharBase+240,X     ; write to char plane row 6 (offset 240 = 6*40)
        LDA #$1E               ; color: bg=navy ($1), fg=yellow ($E)
        STA ColorBase+240,X    ; write color attribute
        INX
        CPX #14                ; 13 = msg length - 1; loop while X < 14
        BNE MSG_LOOP
ROT_FOREVER:
        LDA #CmdRotLeft        ; CmdRotLeft = $07: rotate every row 1 cell left, wrapping
        STA RegCmd
WAIT_TICK:
        LDA ViaIFR             ; read IFR; T1 sets bit 6 on underflow
        AND #ViaT1Bit
        BEQ WAIT_TICK
        LDA ViaT1C_L           ; read T1C-L to clear IFR T1 — ready for next period
        JMP ROT_FOREVER

MSG:
        .byte "HELLO 6502 SIM"

; ScrollerFramed — same shape as Scroller but Pause+Frame so the
; user only sees clean snapshots.

COUNTER = $10                  ; frame counter (drives gradient phase)
.exportzp COUNTER

        LDA #$01               ; pause once
        STA RegPause

LOOP:
        LDA #CmdShiftUp        ; shift first so the bottom row is blank when we fill it
        STA RegCmd

        INC COUNTER
        LDX #0

FILL:
        TXA
        CLC
        ADC COUNTER
        STA ColorBase+480,X
        STA CharBase+480,X
        INX
        CPX #40
        BNE FILL

        STA RegFrame           ; commit the freshly-filled bottom row

        JMP LOOP

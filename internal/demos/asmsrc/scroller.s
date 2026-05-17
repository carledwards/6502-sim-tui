; Scroller — fills the bottom row with X+counter, then pokes
; CmdShiftUp. A diagonal gradient climbs the screen each iteration.

COUNTER = $10                  ; scroll iteration counter (gradient seed)
.exportzp COUNTER

LOOP:
        INC COUNTER            ; user-ZP $10 (system reserves $00..$0F)
        LDX #0                 ; X iterates 0..39 across the bottom row

FILL:
        TXA                    ; A = X
        CLC
        ADC COUNTER            ; A = X + counter
        STA ColorBase+480,X    ; color plane bottom row: $A000 + 12*40 = $A1E0
        STA CharBase+480,X     ; char plane bottom row: $A400 + 12*40 = $A5E0
        INX
        CPX #40
        BNE FILL

        LDA #CmdShiftUp        ; CmdShiftUp scrolls the entire display up one row
        STA RegCmd

        JMP LOOP

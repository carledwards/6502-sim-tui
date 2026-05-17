; Bouncer — a colored '*' bounces left/right across row 6, erasing
; its trail.
;
; Zero-page variables. Declaring them as exported constants makes
; go6asm surface them in the symbol table, so the simulator's memory
; view labels $10/$11 as X / DX instead of bare addresses.
X  = $10               ; current X position 0..39
DX = $11               ; direction byte: +1 (right) or -1 (left)
.exportzp X
.exportzp DX

        LDA #CmdClear          ; clear the screen
        STA RegCmd
        LDX #20                ; initial X = 20 (centre)
        STX X
        LDA #$01               ; initial dx = +1 (moving right)
        STA DX

LOOP:
        LDX X                  ; load X position into the X register
        LDA #' '               ; erase old char (write space)
        STA CharBase+240,X
        LDA #$00               ; erase old color (black on black)
        STA ColorBase+240,X

        LDA DX                 ; check direction sign
        BPL MOVE_RIGHT
        DEX
        JMP STORE_X
MOVE_RIGHT:
        INX
STORE_X:
        STX X

        LDA #'*'               ; '*' character
        STA CharBase+240,X
        LDA #$1E               ; color: bg=navy, fg=yellow
        STA ColorBase+240,X

        LDY #$FF               ; delay loop counter init
DELAY:
        DEY
        BNE DELAY

        CPX #0                 ; hit left wall?
        BEQ FLIP
        CPX #39                ; hit right wall?
        BEQ FLIP
        JMP LOOP

FLIP:
        LDA DX                 ; dx = -dx via two's complement
        EOR #$FF
        CLC
        ADC #$01
        STA DX
        JMP LOOP

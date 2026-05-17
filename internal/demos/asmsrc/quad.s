; Quad — split the framebuffer into four rectangles and rotate each
; a different cardinal direction, entirely with the VIC's CmdRect*
; commands (the CPU just sets RegRect{X,Y,W,H} and pokes RegCmd):
;
;   ┌──── rows 0..5 ────┐
;   │  TL: rotate up    │  TR: rotate right
;   ├──── row 6 div ────┤
;   │  BL: rotate down  │  BR: rotate left
;   └──── rows 7..12 ───┘
;
; Rect-rotation command codes (quad-specific, not in the symbol pack):
;   $0F = rot-up   $10 = rot-down   $11 = rot-left   $12 = rot-right
;
; Pause+Frame: DRAW paints the four colour patterns once, then each
; LOOP issues the four rotations and commits one snapshot.

.export DRAW                   ; surface the paint subroutine in the memory view

        LDA #CmdClear          ; clear the display
        STA RegCmd
        LDA #$01               ; pause: UI only updates on RegFrame writes
        STA RegPause
        JSR DRAW               ; paint the initial quadrant patterns

LOOP:
; Each block below programs the rect (X,Y,W,H) then fires one
; rotation command. Rects are 20 wide x 6 tall.
        LDA #0                 ; TL: rotate-up rect (0,0,20,6)
        STA RegRectX
        LDA #0
        STA RegRectY
        LDA #20
        STA RegRectW
        LDA #6
        STA RegRectH
        LDA #$0F               ; $0F = CmdRectRotUp
        STA RegCmd
        LDA #20                ; TR: rotate-right rect (20,0,20,6)
        STA RegRectX
        LDA #0
        STA RegRectY
        LDA #20
        STA RegRectW
        LDA #6
        STA RegRectH
        LDA #$12               ; $12 = CmdRectRotRight
        STA RegCmd
        LDA #0                 ; BL: rotate-down rect (0,7,20,6)
        STA RegRectX
        LDA #7
        STA RegRectY
        LDA #20
        STA RegRectW
        LDA #6
        STA RegRectH
        LDA #$10               ; $10 = CmdRectRotDown
        STA RegCmd
        LDA #20                ; BR: rotate-left rect (20,7,20,6)
        STA RegRectX
        LDA #7
        STA RegRectY
        LDA #20
        STA RegRectW
        LDA #6
        STA RegRectH
        LDA #$11               ; $11 = CmdRectRotLeft
        STA RegCmd
        LDA #$01               ; commit the rotated frame
        STA RegFrame
        JMP LOOP

; ===== DRAW: paint the four quadrant colour patterns =============
; Two helpers, unrolled:
;
;  * Horizontal stripes (STR_*): fill one 20-cell row with a single
;    colour. Used for TL/BL where the motion is vertical, so each
;    row needs a distinct colour for the up/down shift to read.
;
;  * Vertical stripes (VS_*): write A = X + bias across a column
;    range so each COLUMN gets a distinct colour. Used for TR/BR
;    where the motion is horizontal.
;
; Colour-plane layout: row N base = $A000 + 40*N (TL/TR are page 0
; rows 0..5; BL/BR are page 1 rows 7..12).
DRAW:
        LDA #$1F               ; TL row 0: colour $1F
        LDX #19                ; X counts 19..0 (20 cells)
STR_A000:
        STA $A000,X
        DEX
        BPL STR_A000
        LDA #$2F               ; TL row 1
        LDX #19
STR_A028:
        STA $A028,X
        DEX
        BPL STR_A028
        LDA #$3F               ; TL row 2
        LDX #19
STR_A050:
        STA $A050,X
        DEX
        BPL STR_A050
        LDA #$4F               ; TL row 3
        LDX #19
STR_A078:
        STA $A078,X
        DEX
        BPL STR_A078
        LDA #$5F               ; TL row 4
        LDX #19
STR_A0A0:
        STA $A0A0,X
        DEX
        BPL STR_A0A0
        LDA #$6F               ; TL row 5
        LDX #19
STR_A0C8:
        STA $A0C8,X
        DEX
        BPL STR_A0C8
        LDA #$AF               ; BL row 7
        LDX #19
STR_A118:
        STA $A118,X
        DEX
        BPL STR_A118
        LDA #$BF               ; BL row 8
        LDX #19
STR_A140:
        STA $A140,X
        DEX
        BPL STR_A140
        LDA #$CF               ; BL row 9
        LDX #19
STR_A168:
        STA $A168,X
        DEX
        BPL STR_A168
        LDA #$DF               ; BL row 10
        LDX #19
STR_A190:
        STA $A190,X
        DEX
        BPL STR_A190
        LDA #$EF               ; BL row 11
        LDX #19
STR_A1B8:
        STA $A1B8,X
        DEX
        BPL STR_A1B8
        LDA #$FF               ; BL row 12
        LDX #19
STR_A1E0:
        STA $A1E0,X
        DEX
        BPL STR_A1E0
; TR rows 0..5, columns 20..39: colour = X + $80 (varies per column).
        LDX #19
VS_A014:
        TXA
        CLC
        ADC #$80               ; column-dependent colour bias
        STA $A014,X            ; TR row 0
        STA $A03C,X            ; TR row 1
        STA $A064,X            ; TR row 2
        STA $A08C,X            ; TR row 3
        STA $A0B4,X            ; TR row 4
        STA $A0DC,X            ; TR row 5
        DEX
        BPL VS_A014
; BR rows 7..12, columns 20..39: colour = X + $10.
        LDX #19
VS_A12C:
        TXA
        CLC
        ADC #$10
        STA $A12C,X            ; BR row 7
        STA $A154,X            ; BR row 8
        STA $A17C,X            ; BR row 9
        STA $A1A4,X            ; BR row 10
        STA $A1CC,X            ; BR row 11
        STA $A1F4,X            ; BR row 12
        DEX
        BPL VS_A12C
        RTS

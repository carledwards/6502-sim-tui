; BouncingBalls — four colored balls on the 160x100 graphics plane.
;
; Pause+Frame so only finished frames show. Pacing comes from the
; 6522 VIA Timer 1 in free-run mode (~50 ms period), so motion speed
; tracks the VIA crystal, not the (variable) CPU clock — exactly like
; a real W65C22S. Halt the CPU to single-step and the timer keeps
; running, so the IFR bit flips just as it would on real silicon.
;
; Per-ball state lives in zero page as five parallel 4-byte arrays,
; indexed 0..3 by the X register. They are exported so the simulator
; labels these cells in its memory view:
;
;   BALL_X   X position (0..159)      BALL_Y   Y position (0..99)
;   BALL_VX  X velocity ($01=+1, $FF=-1, signed)
;   BALL_VY  Y velocity ($01=+1, $FF=-1, signed)
;   BALL_COL palette colour index
;
; Bounce: on hitting a wall, the velocity is reflected with EOR #$FE
; (flips $01<->$FF), the position is clamped, and the colour is
; bumped (INC) so each ball cycles colour as it ricochets.

BALL_X   = $10
BALL_Y   = $14
BALL_VX  = $18
BALL_VY  = $1C
BALL_COL = $20
.exportzp BALL_X
.exportzp BALL_Y
.exportzp BALL_VX
.exportzp BALL_VY
.exportzp BALL_COL

        LDA #$01               ; switch the VIC into graphics mode
        STA RegMode
        LDA #$01               ; pause — display only updates on RegFrame writes
        STA RegPause

; --- Program VIA Timer 1 for ~50 ms free-run pacing ---------------
; Latch = 50_000 = $C350 → 50 ms at the VIA's 1 MHz clock. Order
; matters: ACR=$40 (free-run) must precede the T1C_H write so T1 arms
; straight into free-run mode; the high-byte write transfers the
; latch into the counter and starts the timer.
        LDA #ViaT1Bit          ; ACR bit 6 = T1 free-run mode (auto-reload on underflow)
        STA ViaACR
        LDA #$50               ; T1 latch low  = $50
        STA ViaT1L_L
        LDA #$C3               ; T1 latch high = $C3 ($C350) — arms + starts T1
        STA ViaT1C_H

; --- Initialise the four balls -----------------------------------
; Positions are spread out to avoid initial overlap; velocities are
; a mix of +1/-1 per axis; colours chosen to read on the dark-blue
; background.
        LDA #20                ; ball 0: X = 20
        STA BALL_X
        LDA #20                ; ball 0: Y = 20
        STA BALL_Y
        LDA #$01               ; ball 0: VX = +1 (moving right)
        STA BALL_VX
        LDA #$01               ; ball 0: VY = +1 (moving down)
        STA BALL_VY
        LDA #$04               ; ball 0: colour = red
        STA BALL_COL
        LDA #80                ; ball 1: X = 80
        STA BALL_X+1
        LDA #40                ; ball 1: Y = 40
        STA BALL_Y+1
        LDA #$FF               ; ball 1: VX = -1 (moving left)
        STA BALL_VX+1
        LDA #$01               ; ball 1: VY = +1 (moving down)
        STA BALL_VY+1
        LDA #$02               ; ball 1: colour = green
        STA BALL_COL+1
        LDA #130               ; ball 2: X = 130
        STA BALL_X+2
        LDA #70                ; ball 2: Y = 70
        STA BALL_Y+2
        LDA #$01               ; ball 2: VX = +1 (moving right)
        STA BALL_VX+2
        LDA #$FF               ; ball 2: VY = -1 (moving up)
        STA BALL_VY+2
        LDA #$0E               ; ball 2: colour = yellow
        STA BALL_COL+2
        LDA #50                ; ball 3: X = 50
        STA BALL_X+3
        LDA #85                ; ball 3: Y = 85
        STA BALL_Y+3
        LDA #$FF               ; ball 3: VX = -1 (moving left)
        STA BALL_VX+3
        LDA #$FF               ; ball 3: VY = -1 (moving up)
        STA BALL_VY+3
        LDA #$0B               ; ball 3: colour = light cyan
        STA BALL_COL+3

MAIN:
; --- Clear the plane by filling it with the background colour -----
; Same visible result as CmdGfxClear, but it exercises the rect-fill
; primitive (a future demo could clear only part of the plane).
        LDA #$01               ; background color: dark blue
        STA RegGfxColor
        LDA #0                 ; rect origin (0,0)
        STA RegRectX
        LDA #0
        STA RegRectY
        LDA #160               ; rect size: full graphics plane (160x100)
        STA RegRectW
        LDA #100
        STA RegRectH
        LDA #CmdGfxRectFill    ; fire filled-rect — paints the background
        STA RegCmd

; --- Draw every ball as a filled circle (X = ball index) ----------
        LDX #0
DRAW:
        LDA BALL_X,X           ; circle centre X = ball X
        STA RegRectX
        LDA BALL_Y,X           ; circle centre Y = ball Y
        STA RegRectY
        LDA #4                 ; radius = 4
        STA RegRectW
        LDA BALL_COL,X         ; colour = ball's palette index
        STA RegGfxColor
        LDA #CmdGfxFillCircle
        STA RegCmd
        INX
        CPX #4
        BNE DRAW

; --- Advance every ball and bounce it off the four walls ----------
        LDX #0
UPDATE:
        CLC                    ; x += vx
        LDA BALL_X,X
        ADC BALL_VX,X
        STA BALL_X,X
        CMP #4                 ; hit left wall? (x < minXY = 4)
        BCS X_LEFT_OK
        LDA BALL_VX,X          ; reflect VX (EOR #$FE flips +1<->-1)
        EOR #$FE
        STA BALL_VX,X
        LDA #4                 ; clamp X to the left edge
        STA BALL_X,X
        INC BALL_COL,X         ; bump colour on every bounce
X_LEFT_OK:
        LDA BALL_X,X
        CMP #156               ; hit right wall? (x >= maxX+1 = 156)
        BCC X_RIGHT_OK
        LDA BALL_VX,X          ; reflect VX
        EOR #$FE
        STA BALL_VX,X
        LDA #155               ; clamp X to the right edge
        STA BALL_X,X
        INC BALL_COL,X
X_RIGHT_OK:
        CLC                    ; y += vy
        LDA BALL_Y,X
        ADC BALL_VY,X
        STA BALL_Y,X
        CMP #4                 ; hit top wall?
        BCS Y_TOP_OK
        LDA BALL_VY,X          ; reflect VY
        EOR #$FE
        STA BALL_VY,X
        LDA #4                 ; clamp Y to the top edge
        STA BALL_Y,X
        INC BALL_COL,X
Y_TOP_OK:
        LDA BALL_Y,X
        CMP #96                ; hit bottom wall? (y >= maxY+1 = 96)
        BCC Y_BOT_OK
        LDA BALL_VY,X          ; reflect VY
        EOR #$FE
        STA BALL_VY,X
        LDA #95                ; clamp Y to the bottom edge
        STA BALL_Y,X
        INC BALL_COL,X
Y_BOT_OK:
        INX
        CPX #4
        BNE UPDATE

        LDA #$01               ; RegFrame: any write captures a snapshot
        STA RegFrame

; --- Wait for the VIA T1 underflow (the 50 ms heartbeat) ----------
WAIT_TICK:
        LDA ViaIFR             ; read IFR; T1 sets bit 6 on underflow
        AND #ViaT1Bit
        BEQ WAIT_TICK
        LDA ViaT1C_L           ; read T1C-L to clear IFR T1 — ready for next period
        JMP MAIN

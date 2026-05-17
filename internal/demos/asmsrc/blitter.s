; Blitter — the classic double-buffer pattern. Each frame is fully
; rendered into off-screen RAM at SCRATCH, then copied ("blitted")
; into both VIC planes, then committed with RegFrame so the user
; never sees a half-drawn frame (no tearing).
;
; Phases per frame: BUILD (compose off-screen) → BLIT_C (copy to the
; colour plane) → BLIT_H (copy to the char plane, remapped to a
; printable glyph range) → RegFrame (commit the snapshot).

COUNTER = $10                  ; frame counter (drives gradient phase)
SCRATCH = $1000                ; off-screen image buffer (256 bytes)
.exportzp COUNTER
.export SCRATCH

        LDA #$01               ; pause: UI shows snapshot until RegFrame is written
        STA RegPause

LOOP:
        INC COUNTER            ; counter advances each frame (animates the gradient)
        LDX #0

; BUILD: off-screen image — buffer[X] = X + counter (a sliding ramp).
BUILD:
        TXA
        CLC
        ADC COUNTER
        STA SCRATCH,X          ; scratch buffer at $1000 (within RAM region)
        INX
        BNE BUILD              ; 256 bytes (X wraps 255->0)

; BLIT_C: copy the off-screen image straight into colour-plane page 0.
        LDX #0
BLIT_C:
        LDA SCRATCH,X
        STA ColorBase,X
        INX
        BNE BLIT_C

; BLIT_H: copy into char-plane page 0, folding each byte into the
; 32 glyphs '@'..'_' so the ramp shows as visible characters.
        LDX #0
BLIT_H:
        LDA SCRATCH,X
        AND #$1F               ; low 5 bits give 0..31
        CLC
        ADC #'@'               ; map 0..31 -> '@'..'_'
        STA CharBase,X
        INX
        BNE BLIT_H

        STA RegFrame           ; RegFrame: any write captures the new snapshot

        JMP LOOP

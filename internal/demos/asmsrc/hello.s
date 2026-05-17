; hello.s — the first sim-tui demo assembled by go6asm (ca65 syntax),
; replacing the fluent Go-DSL builder. It uses go6asm's Layer-0
; "sim-tui" symbol pack, so RegCmd / CharBase / ColorBase resolve with
; no hard-coded addresses. go6asm infers the $E000 load address and
; synthesizes the RESET vector automatically.

        LDA #CmdClear          ; clear the VIC screen
        STA RegCmd

        LDX #$00
print:
        LDA msg,X              ; next banner character
        BEQ done
        STA CharBase+240,X     ; char plane, row 6 (6*40)
        LDA #$1E               ; bg = navy, fg = yellow
        STA ColorBase+240,X    ; same cell, color plane
        INX
        JMP print
done:
        JMP done               ; park forever (correct top-level end)

msg:
        .byte "HELLO FROM go6asm", 0

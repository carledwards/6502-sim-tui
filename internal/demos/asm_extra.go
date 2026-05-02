package demos

// Extra assembler opcodes used by demos beyond Quad. Lives in the
// same package as quad.go's `asm` struct, so these are just
// additional methods on the same type.

func (a *asm) sta_zp(zp byte)  { a.emit(0x85, zp) }
func (a *asm) lda_zp(zp byte)  { a.emit(0xA5, zp) }
func (a *asm) lda_zpx(zp byte) { a.emit(0xB5, zp) }
func (a *asm) sta_zpx(zp byte) { a.emit(0x95, zp) }
func (a *asm) adc_zpx(zp byte) { a.emit(0x75, zp) }
func (a *asm) inc_zpx(zp byte) { a.emit(0xF6, zp) }

func (a *asm) cmp_imm(v byte) { a.emit(0xC9, v) }
func (a *asm) cpx_imm(v byte) { a.emit(0xE0, v) }
func (a *asm) eor_imm(v byte) { a.emit(0x49, v) }
func (a *asm) ldy_imm(v byte) { a.emit(0xA0, v) }

func (a *asm) inx() { a.emit(0xE8) }
func (a *asm) dey() { a.emit(0x88) }

func (a *asm) bcc(target string) { a.emit(0x90); a.relFix(target) }
func (a *asm) bcs(target string) { a.emit(0xB0); a.relFix(target) }
func (a *asm) bne(target string) { a.emit(0xD0); a.relFix(target) }

// sim.js — JS bridge for 6502-sim in the browser.
//
// Boots the wasm module, sizes a canvas to match the SimulationScreen
// grid, paints cells from snapshots taken each animation frame, and
// forwards keyboard/mouse to foxpro via the exported wasm functions.

(() => {
  // Cell metrics. Matched to a sensible monospace cap-height; the
  // canvas dimensions and font are derived from these.
  const FONT_PX = 16;
  const CELL_H = 22; // taller than FONT_PX so g/p/y/j descenders fit
  const DEFAULT_FG = '#cccccc';
  const DEFAULT_BG = '#0000aa';
  const DEFAULT_COLOR_SENTINEL = 0xff000000;

  const canvas = document.getElementById('screen');
  const ctx = canvas.getContext('2d', { alpha: false });
  const statusEl = document.getElementById('status');

  let cellW = 9;
  let buf = null;
  let view = null;
  let booted = false;
  let stopped = false;

  window.onFoxproReady = () => {
    if (booted) return;
    booted = true;
    boot();
  };

  async function start() {
    if (typeof Go === 'undefined') {
      statusEl.textContent = 'wasm_exec.js failed to load';
      return;
    }
    const go = new Go();
    statusEl.textContent = 'fetching sim.wasm…';
    try {
      const r = await WebAssembly.instantiateStreaming(fetch('sim.wasm'), go.importObject);
      statusEl.textContent = 'starting Go runtime…';
      go.run(r.instance);
    } catch (e) {
      statusEl.textContent = 'wasm load error: ' + e.message;
      console.error(e);
    }
  }

  function boot() {
    const fw = window.foxproWasm;
    if (!fw) {
      statusEl.textContent = 'foxproWasm bridge missing';
      return;
    }

    ctx.font = `${FONT_PX}px ui-monospace, "SF Mono", Menlo, Consolas, monospace`;
    ctx.textBaseline = 'top';
    cellW = Math.max(7, Math.ceil(ctx.measureText('M').width));

    const [w, h] = fw.size();
    canvas.width = w * cellW;
    canvas.height = h * CELL_H;
    ctx.font = `${FONT_PX}px ui-monospace, "SF Mono", Menlo, Consolas, monospace`;
    ctx.textBaseline = 'top';

    buf = new Uint8Array(w * h * 16);
    view = new DataView(buf.buffer);

    canvas.focus();
    setupInput();
    requestAnimationFrame(frame);
    statusEl.textContent = `${w}×${h} cells · click to focus · F10 menu · R run · . stop · S step · Z reset`;
  }

  function frame() {
    if (stopped) return;
    requestAnimationFrame(frame);
    const fw = window.foxproWasm;
    if (!fw || !buf) return;

    let sz;
    try {
      sz = fw.snapshot(buf);
    } catch (err) {
      stopped = true;
      statusEl.textContent = 'simulator exited — refresh to restart';
      console.warn(err);
      return;
    }
    const w = sz[0], h = sz[1];
    for (let y = 0; y < h; y++) {
      let off = y * w * 16;
      for (let x = 0; x < w; x++) {
        const ch = view.getUint32(off, true);
        const fg = view.getUint32(off + 4, true);
        const bg = view.getUint32(off + 8, true);
        off += 16;
        paintCell(x, y, ch, fg, bg);
      }
    }
  }

  function paintCell(x, y, ch, fg, bg) {
    const px = x * cellW;
    const py = y * CELL_H;
    ctx.fillStyle = colorToCSS(bg, DEFAULT_BG);
    ctx.fillRect(px, py, cellW, CELL_H);
    if (ch === 0 || ch === 32) return;
    const fgCss = colorToCSS(fg, DEFAULT_FG);
    if (ch >= 0x2500 && ch <= 0x258F) {
      if (drawBoxOrBlock(px, py, ch, fgCss)) return;
    }
    ctx.fillStyle = fgCss;
    ctx.fillText(String.fromCodePoint(ch), px, py + 1);
  }

  // Box-drawing (U+2500–U+257F) and block (U+2580–U+258F) chars
  // rendered as fillRect primitives so they connect cell-to-cell.
  function drawBoxOrBlock(px, py, ch, fgCss) {
    ctx.fillStyle = fgCss;
    switch (ch) {
      case 0x2580: ctx.fillRect(px, py, cellW, Math.floor(CELL_H / 2)); return true;
      case 0x2584: ctx.fillRect(px, py + Math.floor(CELL_H / 2), cellW, CELL_H - Math.floor(CELL_H / 2)); return true;
      case 0x2588: ctx.fillRect(px, py, cellW, CELL_H); return true;
      case 0x258C: ctx.fillRect(px, py, Math.floor(cellW / 2), CELL_H); return true;
      case 0x2590: ctx.fillRect(px + Math.floor(cellW / 2), py, cellW - Math.floor(cellW / 2), CELL_H); return true;
    }
    let L = false, R = false, U = false, D = false;
    switch (ch) {
      case 0x2500: L = R = true; break;
      case 0x2502: U = D = true; break;
      case 0x250C: R = D = true; break;
      case 0x2510: L = D = true; break;
      case 0x2514: R = U = true; break;
      case 0x2518: L = U = true; break;
      case 0x251C: U = D = R = true; break;
      case 0x2524: U = D = L = true; break;
      case 0x252C: L = R = D = true; break;
      case 0x2534: L = R = U = true; break;
      case 0x253C: L = R = U = D = true; break;
      default: return false;
    }
    const cx = px + Math.floor(cellW / 2);
    const cy = py + Math.floor(CELL_H / 2);
    const lw = 1;
    if (L) ctx.fillRect(px, cy, cx - px + lw, lw);
    if (R) ctx.fillRect(cx, cy, px + cellW - cx, lw);
    if (U) ctx.fillRect(cx, py, lw, cy - py + lw);
    if (D) ctx.fillRect(cx, cy, lw, py + CELL_H - cy);
    return true;
  }

  function colorToCSS(c, fallback) {
    if (c === DEFAULT_COLOR_SENTINEL) return fallback;
    const r = (c >> 16) & 0xff;
    const g = (c >> 8) & 0xff;
    const b = c & 0xff;
    return `rgb(${r},${g},${b})`;
  }

  function setupInput() {
    const fw = window.foxproWasm;
    const KEYS = fw.keys, MODS = fw.mods, BTN = fw.buttons;

    const specialKeys = {
      Enter: KEYS.Enter,
      Tab: KEYS.Tab,
      Escape: KEYS.Esc,
      Backspace: KEYS.Backspace2,
      ArrowUp: KEYS.Up,
      ArrowDown: KEYS.Down,
      ArrowLeft: KEYS.Left,
      ArrowRight: KEYS.Right,
      Home: KEYS.Home,
      End: KEYS.End,
      PageUp: KEYS.PgUp,
      PageDown: KEYS.PgDn,
      Insert: KEYS.Insert,
      Delete: KEYS.Delete,
    };
    for (let i = 1; i <= 12; i++) specialKeys['F' + i] = KEYS['F' + i];

    canvas.addEventListener('keydown', (e) => {
      if (e.metaKey) return; // let Cmd+R, Cmd+L, etc. through
      const mods =
        (e.shiftKey ? MODS.Shift : 0) |
        (e.ctrlKey ? MODS.Ctrl : 0) |
        (e.altKey ? MODS.Alt : 0) |
        (e.metaKey ? MODS.Meta : 0);
      if (e.key === 'Tab' && e.shiftKey) {
        e.preventDefault();
        fw.injectKey(KEYS.Backtab, 0, mods);
        return;
      }
      const sk = specialKeys[e.key];
      if (sk !== undefined) {
        e.preventDefault();
        fw.injectKey(sk, 0, mods);
        return;
      }
      if (e.altKey && /^Key[A-Z]$/.test(e.code)) {
        e.preventDefault();
        fw.injectKey(KEYS.Rune, e.code.charCodeAt(3) + 32, mods);
        return;
      }
      if (e.key.length === 1) {
        e.preventDefault();
        fw.injectKey(KEYS.Rune, e.key.codePointAt(0), mods);
      }
    });

    function pixelToCell(e) {
      const r = canvas.getBoundingClientRect();
      const px = (e.clientX - r.left) * (canvas.width / r.width);
      const py = (e.clientY - r.top) * (canvas.height / r.height);
      return [Math.floor(px / cellW), Math.floor(py / CELL_H)];
    }
    function buttonsMaskFromEvent(e) {
      let m = 0;
      if (e.buttons & 1) m |= BTN.Primary;
      if (e.buttons & 2) m |= BTN.Secondary;
      if (e.buttons & 4) m |= BTN.Middle;
      return m;
    }
    function modMaskFromEvent(e) {
      let m = 0;
      if (e.shiftKey) m |= MODS.Shift;
      if (e.ctrlKey) m |= MODS.Ctrl;
      if (e.altKey) m |= MODS.Alt;
      if (e.metaKey) m |= MODS.Meta;
      return m;
    }

    canvas.addEventListener('mousedown', (e) => {
      e.preventDefault();
      canvas.focus();
      const [cx, cy] = pixelToCell(e);
      fw.injectMouse(cx, cy, buttonsMaskFromEvent(e), modMaskFromEvent(e));
    });
    canvas.addEventListener('mousemove', (e) => {
      const [cx, cy] = pixelToCell(e);
      fw.injectMouse(cx, cy, buttonsMaskFromEvent(e), modMaskFromEvent(e));
    });
    canvas.addEventListener('mouseup', (e) => {
      e.preventDefault();
      const [cx, cy] = pixelToCell(e);
      fw.injectMouse(cx, cy, buttonsMaskFromEvent(e), modMaskFromEvent(e));
    });
    canvas.addEventListener('contextmenu', (e) => e.preventDefault());
    canvas.addEventListener('wheel', (e) => {
      e.preventDefault();
      const [cx, cy] = pixelToCell(e);
      let btn = 0;
      if (e.deltaY < 0) btn = BTN.WheelUp;
      else if (e.deltaY > 0) btn = BTN.WheelDown;
      else if (e.deltaX < 0) btn = BTN.WheelLeft;
      else if (e.deltaX > 0) btn = BTN.WheelRight;
      if (btn) fw.injectMouse(cx, cy, btn, modMaskFromEvent(e));
    }, { passive: false });
  }

  start();
})();

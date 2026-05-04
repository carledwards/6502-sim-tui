// sim.js — boots the 6502 simulator wasm and hands the canvas off
// to FoxproRender (shared renderer shipped by foxpro-go). All cell
// painting, pixel-layer compositing, drop-shadow tinting, and
// keyboard/mouse plumbing lives in foxpro.js.
(() => {
  const canvas = document.getElementById('screen');
  const statusEl = document.getElementById('status');

  // Attach binds the renderer to the canvas. It waits for the
  // wasm bridge to publish itself (via window.onFoxproReady) before
  // starting the per-frame loop.
  FoxproRender.attach(canvas, { statusEl });

  // Boot the wasm. The bridge calls onFoxproReady once it's up;
  // attach() picks that up and starts rendering.
  FoxproRender.bootWasm('sim.wasm', statusEl);
})();

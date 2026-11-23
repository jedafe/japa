// ghostty-web frontend for ttyd — protocol glue + OSC52→clipboard + touch shim
//
// Build (bun only — see AGENTS.md):
//   mkdir /tmp/gw && cd /tmp/gw && bun init -y && bun add ghostty-web
//   cp deploy/shell/frontend.ts .
//   bun build frontend.ts --target browser --format esm --minify --outfile bundle.js
//   # then wrap bundle.js in the index.html shell (module script, #terminal,
//   # #overlay divs) and place it at /opt/japa/shell-frontend/index.html
import { init, Terminal, FitAddon } from 'ghostty-web';

const CMD_INPUT = '0', CMD_RESIZE = '1', CMD_PAUSE = '2', CMD_RESUME = '3';
const OUT_OUTPUT = '0', OUT_TITLE = '1', OUT_PREFS = '2';

const dec = new TextDecoder();
const enc = new TextEncoder();

function b64ToUtf8(b64: string): string {
  const bin = atob(b64);
  const bytes = Uint8Array.from(bin, c => c.charCodeAt(0));
  return dec.decode(bytes);
}

// Extracts OSC 52 set-clipboard sequences from the pty stream.
// Sequences may straddle ws frames, so unmatched tails are buffered.
class Osc52 {
  private tail = '';
  feed(data: string, write: (s: string) => void) {
    let buf = this.tail + data;
    this.tail = '';
    let out = '';
    let i = 0;
    for (;;) {
      const start = buf.indexOf('\x1b]52;', i);
      if (start === -1) { out += buf.slice(i); break; }
      const end = buf.indexOf('\x07', start);
      const endEsc = buf.indexOf('\x1b\\', start);
      const stop = end === -1 ? endEsc : (endEsc === -1 ? end : Math.min(end, endEsc));
      if (stop === -1) {
        // Possibly incomplete — keep tail if it isn't absurdly large
        if (buf.length - start < 1 << 20) { this.tail = buf.slice(start); out += buf.slice(i, start); }
        else out += buf.slice(i);
        break;
      }
      const payload = buf.slice(start + 5, stop);
      const semi = payload.indexOf(';');
      const val = semi === -1 ? '' : payload.slice(semi + 1);
      if (val && val !== '?') {
        try { navigator.clipboard.writeText(b64ToUtf8(val)).catch(() => {}); } catch {}
      }
      // Forward the sequence verbatim — the emulator parses it too
      const termLen = buf[stop] === '\x1b' ? 2 : 1;
      out += buf.slice(i, stop + termLen);
      i = stop + termLen;
    }
    if (out) write(out);
  }
}

// Touch → mouse shim: single finger = press/drag (select, app mouse),
// two fingers = wheel scroll. preventDefault kills browser scroll/zoom.
function installTouchShim(el: HTMLElement) {
  el.style.touchAction = 'none';
  let lastX = 0, lastY = 0, lastPinch = 0, mode: 'none' | 'mouse' | 'scroll' = 'none';
  const fire = (type: string, x: number, y: number, dy = 0) => {
    const t = type === 'wheel'
      ? new WheelEvent('wheel', { clientX: x, clientY: y, deltaY: dy, bubbles: true })
      : new MouseEvent(type, { clientX: x, clientY: y, button: 0, buttons: type === 'mouseup' ? 0 : 1, bubbles: true });
    el.dispatchEvent(t);
  };
  el.addEventListener('touchstart', e => {
    e.preventDefault();
    if (e.touches.length === 1) {
      mode = 'mouse'; lastX = e.touches[0].clientX; lastY = e.touches[0].clientY;
      fire('mousedown', lastX, lastY);
    } else if (e.touches.length === 2) {
      mode = 'scroll'; lastPinch = e.touches[0].clientY;
      fire('mouseup', lastX, lastY);
    }
  }, { passive: false });
  el.addEventListener('touchmove', e => {
    e.preventDefault();
    if (mode === 'mouse' && e.touches.length === 1) {
      lastX = e.touches[0].clientX; lastY = e.touches[0].clientY;
      fire('mousemove', lastX, lastY);
    } else if (mode === 'scroll' && e.touches.length >= 2) {
      const y = e.touches[0].clientY;
      fire('wheel', lastX, lastY, (lastPinch - y) * 2);
      lastPinch = y;
    }
  }, { passive: false });
  el.addEventListener('touchend', e => {
    e.preventDefault();
    if (mode === 'mouse' && e.touches.length === 0) fire('mouseup', lastX, lastY);
    if (e.touches.length === 0) mode = 'none';
  }, { passive: false });
}

async function connect(overlay: HTMLElement) {
  const termEl = document.getElementById('terminal')!;
  const fit = new FitAddon();
  const term = new Terminal({
    fontSize: 13,
    fontFamily: 'ui-monospace,SFMono-Regular,Menlo,Consolas,Liberation Mono,monospace',
    scrollback: 10000,
    theme: {
      foreground: '#d6d6d6', background: '#14161a', cursor: '#adadad',
      black: '#000000', red: '#e06c75', green: '#98c379', yellow: '#e5c07b',
      blue: '#61afef', magenta: '#c678dd', cyan: '#56b6c2', white: '#dcdfe4',
      brightBlack: '#5c6370', brightRed: '#e06c75', brightGreen: '#98c379',
      brightYellow: '#e5c07b', brightBlue: '#61afef', brightMagenta: '#c678dd',
      brightCyan: '#56b6c2', brightWhite: '#f1f1f0',
    } as any,
  });
  term.open(termEl);
  term.loadAddon(fit);
  fit.fit();
  installTouchShim(termEl);
  term.focus();

  let ws: WebSocket | null = null;
  let disposed = false;
  const osc52 = new Osc52();

  const send = (cmd: string, data: string) => {
    if (ws && ws.readyState === 1) {
      const p = enc.encode(data);
      const buf = new Uint8Array(1 + p.length);
      buf[0] = cmd.charCodeAt(0); buf.set(p, 1);
      ws.send(buf);
    }
  };

  term.onData(d => send(CMD_INPUT, d));
  term.onResize(({ cols, rows }) => send(CMD_RESIZE, JSON.stringify({ columns: cols, rows })));

  const open = async () => {
    if (disposed) return;
    try {
      const tokRes = await (await fetch('token')).json();
      ws = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws`, ['tty']);
      ws.binaryType = 'arraybuffer';
      ws.onopen = () => {
        overlay.style.display = 'none';
        ws!.send(JSON.stringify({ AuthToken: tokRes.token, columns: term.cols, rows: term.rows }));
        fit.fit();
      };
      ws.onmessage = e => {
        const raw = typeof e.data === 'string' ? e.data : dec.decode(e.data);
        if (!raw.length) return;
        const c = raw[0], body = raw.slice(1);
        if (c === OUT_OUTPUT) osc52.feed(body, s => term.write(s));
        else if (c === OUT_TITLE) document.title = body || 'japa-vps';
      };
      ws.onclose = () => {
        if (disposed) return;
        overlay.textContent = 'reconnecting…';
        overlay.style.display = 'flex';
        setTimeout(open, 1000);
      };
      ws.onerror = () => ws && ws.close();
    } catch {
      overlay.textContent = 'reconnecting…';
      overlay.style.display = 'flex';
      setTimeout(open, 1000);
    }
  };

  await open();
  window.addEventListener('beforeunload', () => { disposed = true; });
}

init().then(() => connect(document.getElementById('overlay')!));

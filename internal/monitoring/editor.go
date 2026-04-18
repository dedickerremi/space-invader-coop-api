package monitoring

import (
	"fmt"
	"net/http"
)

// HandleEditor serves the level editor HTML page.
func (s *Server) HandleEditor(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, editorHTML)
}

const editorHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Level Editor - Space Invaders Coop</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { font-family: 'JetBrains Mono', ui-monospace, monospace; background: #0a0a0f; color: #e0e0e0; padding: 1.5rem; line-height: 1.5; }
    .container { max-width: 1400px; margin: 0 auto; }
    header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem; }
    h1 { color: #00ff88; text-transform: uppercase; letter-spacing: 0.25em; font-size: 1.25rem; }
    a.back { color: #00aaff; text-decoration: none; font-size: 0.875rem; }
    a.back:hover { text-decoration: underline; }

    .layout { display: grid; grid-template-columns: 240px 1fr; gap: 1.5rem; }

    .panel { background: #1a1a24; border: 2px solid #333; border-radius: 8px; padding: 1rem; }
    .panel h2 { color: #00ff88; font-size: 0.75rem; text-transform: uppercase; margin-bottom: 0.75rem; letter-spacing: 0.15em; }

    .levels-list { display: flex; flex-direction: column; gap: 0.25rem; }
    .levels-list button { background: #2a2a36; border: 1px solid #333; color: #e0e0e0; padding: 0.5rem 0.75rem; text-align: left; cursor: pointer; border-radius: 4px; font-family: inherit; font-size: 0.875rem; }
    .levels-list button:hover { background: #333; }
    .levels-list button.active { background: #00ff88; color: #000; border-color: #00ff88; font-weight: bold; }
    .levels-list .new-row { display: flex; gap: 0.25rem; margin-top: 0.5rem; }
    .levels-list input[type="text"] { flex: 1; background: #0a0a0f; border: 1px solid #333; color: #e0e0e0; padding: 0.5rem; border-radius: 4px; font-family: inherit; font-size: 0.75rem; }

    .toolbar { display: flex; gap: 0.5rem; margin-bottom: 1rem; flex-wrap: wrap; align-items: center; }
    .toolbar button, .levels-list .new-row button { background: #00ff88; color: #000; border: none; padding: 0.5rem 1rem; cursor: pointer; border-radius: 4px; font-family: inherit; font-size: 0.75rem; font-weight: bold; text-transform: uppercase; letter-spacing: 0.1em; }
    .toolbar button.danger { background: #ff4444; color: #fff; }
    .toolbar button.secondary { background: #2a2a36; color: #e0e0e0; border: 1px solid #444; }
    .toolbar button:hover { opacity: 0.85; }
    .toolbar .status { margin-left: auto; font-size: 0.75rem; color: #888; }
    .toolbar .status.success { color: #00ff88; }
    .toolbar .status.error { color: #ff4444; }

    .waves { display: flex; flex-direction: column; gap: 1rem; }
    .wave { background: #1a1a24; border: 2px solid #333; border-radius: 8px; padding: 1rem; }
    .wave-header { display: flex; gap: 0.75rem; margin-bottom: 0.75rem; align-items: center; flex-wrap: wrap; }
    .wave-header label { font-size: 0.7rem; color: #888; text-transform: uppercase; letter-spacing: 0.1em; }
    .wave-header input[type="text"], .wave-header input[type="number"] { background: #0a0a0f; border: 1px solid #333; color: #e0e0e0; padding: 0.4rem 0.6rem; border-radius: 4px; font-family: inherit; font-size: 0.875rem; }
    .wave-header input[type="text"] { flex: 1; min-width: 150px; }
    .wave-header input[type="number"] { width: 70px; }
    .wave-header .wave-actions { margin-left: auto; display: flex; gap: 0.4rem; }
    .wave-header button { background: #2a2a36; color: #e0e0e0; border: 1px solid #444; padding: 0.4rem 0.75rem; cursor: pointer; border-radius: 4px; font-family: inherit; font-size: 0.7rem; }
    .wave-header button.danger { background: #ff4444; color: #fff; border: none; }
    .wave-header button:hover { opacity: 0.85; }

    .grid { display: flex; flex-direction: column; gap: 2px; padding: 0.5rem; background: #0a0a0f; border-radius: 4px; }
    .grid-row { display: flex; gap: 2px; align-items: center; }
    .grid-row .row-actions { display: flex; gap: 2px; margin-left: 0.5rem; }
    .grid-row .row-actions button { background: #2a2a36; color: #999; border: 1px solid #333; width: 24px; height: 24px; cursor: pointer; border-radius: 3px; font-family: inherit; font-size: 0.75rem; padding: 0; }
    .grid-row .row-actions button:hover { color: #fff; background: #444; }
    .cell { width: 28px; height: 28px; border: 1px solid #333; border-radius: 3px; cursor: pointer; display: flex; align-items: center; justify-content: center; font-size: 0.75rem; font-weight: bold; user-select: none; }
    .cell.empty { background: #15151c; }
    .cell.empty:hover { background: #222; }
    .cell.S { background: #00ff88; color: #000; }
    .cell.P { background: #ff4488; color: #fff; }

    .legend { display: flex; gap: 1rem; margin-top: 0.5rem; font-size: 0.75rem; color: #888; }
    .legend span { display: inline-flex; align-items: center; gap: 0.25rem; }
    .legend .swatch { width: 14px; height: 14px; border-radius: 2px; display: inline-block; }

    .empty-state { text-align: center; color: #666; padding: 3rem; font-size: 0.875rem; }
  </style>
</head>
<body>
  <div class="container">
    <header>
      <h1>Level Editor</h1>
      <a class="back" href="/">&larr; back to dashboard</a>
    </header>

    <div class="layout">
      <aside class="panel">
        <h2>Levels</h2>
        <div id="levels-list" class="levels-list"></div>
        <div class="new-row">
          <input id="new-level-name" type="text" placeholder="new-level.json" />
          <button id="new-level-btn" type="button">+</button>
        </div>
      </aside>

      <main>
        <div class="toolbar">
          <button id="add-wave-btn" type="button">+ Wave</button>
          <button id="save-btn" type="button">Save</button>
          <button id="reload-btn" class="secondary" type="button">Reload server</button>
          <button id="delete-btn" class="danger" type="button">Delete level</button>
          <span id="status" class="status"></span>
        </div>
        <div class="legend">
          <span><span class="swatch" style="background:#00ff88"></span> S = static</span>
          <span><span class="swatch" style="background:#ff4488"></span> P = patrol</span>
          <span>Click a cell to cycle: empty &rarr; S &rarr; P &rarr; empty</span>
        </div>
        <br>
        <div id="waves" class="waves">
          <div class="empty-state">Select a level to edit</div>
        </div>
      </main>
    </div>
  </div>

<script>
const DEFAULT_COLS = 13;
let state = { name: null, level: null };

function setStatus(msg, kind) {
  const el = document.getElementById('status');
  el.textContent = msg || '';
  el.className = 'status' + (kind ? ' ' + kind : '');
  if (msg) setTimeout(() => { if (el.textContent === msg) { el.textContent = ''; el.className = 'status'; } }, 3000);
}

async function api(path, opts) {
  const res = await fetch(path, opts);
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || res.statusText);
  }
  if (res.status === 204) return null;
  return res.json();
}

async function loadLevels() {
  const data = await api('/api/levels');
  const list = document.getElementById('levels-list');
  list.innerHTML = '';
  for (const name of data.levels) {
    const btn = document.createElement('button');
    btn.textContent = name + (name === data.current ? ' *' : '');
    btn.className = state.name === name ? 'active' : '';
    btn.onclick = () => loadLevel(name);
    list.appendChild(btn);
  }
}

async function loadLevel(name) {
  const data = await api('/api/levels/' + encodeURIComponent(name));
  state.name = name;
  state.level = data;
  if (!Array.isArray(state.level.waves)) state.level.waves = [];
  await loadLevels();
  render();
}

function cycle(ch) {
  if (ch === '.') return 'S';
  if (ch === 'S') return 'P';
  return '.';
}

function padRow(row, cols) {
  if (row.length >= cols) return row.slice(0, cols);
  return row + '.'.repeat(cols - row.length);
}

function render() {
  const root = document.getElementById('waves');
  root.innerHTML = '';
  if (!state.level) {
    root.innerHTML = '<div class="empty-state">Select a level to edit</div>';
    return;
  }
  state.level.waves.forEach((wave, wi) => {
    const cols = wave.rows.reduce((m, r) => Math.max(m, r.length), DEFAULT_COLS);
    wave.rows = wave.rows.map(r => padRow(r, cols));

    const w = document.createElement('div');
    w.className = 'wave';
    w.innerHTML =
      '<div class="wave-header">' +
        '<label>Wave ' + (wi + 1) + '</label>' +
        '<input type="text" data-field="name" value="' + escapeAttr(wave.name || '') + '" placeholder="Name">' +
        '<label>row delay</label>' +
        '<input type="number" data-field="rowDelay" min="1" value="' + (wave.rowDelay || 30) + '">' +
        '<div class="wave-actions">' +
          '<button data-act="add-row">+ row</button>' +
          '<button data-act="add-col">+ col</button>' +
          '<button data-act="remove-col">- col</button>' +
          '<button data-act="move-up">&uarr;</button>' +
          '<button data-act="move-down">&darr;</button>' +
          '<button data-act="remove" class="danger">Remove</button>' +
        '</div>' +
      '</div>' +
      '<div class="grid"></div>';

    w.querySelector('[data-field="name"]').oninput = e => { wave.name = e.target.value; };
    w.querySelector('[data-field="rowDelay"]').oninput = e => { wave.rowDelay = parseInt(e.target.value, 10) || 30; };
    w.querySelectorAll('[data-act]').forEach(btn => {
      btn.onclick = () => waveAction(wi, btn.dataset.act);
    });

    const grid = w.querySelector('.grid');
    wave.rows.forEach((row, ri) => {
      const r = document.createElement('div');
      r.className = 'grid-row';
      for (let ci = 0; ci < row.length; ci++) {
        const ch = row[ci];
        const cell = document.createElement('div');
        cell.className = 'cell ' + (ch === '.' ? 'empty' : ch);
        cell.textContent = ch === '.' ? '' : ch;
        cell.onclick = () => {
          const next = cycle(ch);
          const updated = wave.rows[ri].split('');
          updated[ci] = next;
          wave.rows[ri] = updated.join('');
          render();
        };
        r.appendChild(cell);
      }
      const actions = document.createElement('div');
      actions.className = 'row-actions';
      const upBtn = document.createElement('button'); upBtn.textContent = '↑'; upBtn.onclick = () => rowAction(wi, ri, 'up');
      const downBtn = document.createElement('button'); downBtn.textContent = '↓'; downBtn.onclick = () => rowAction(wi, ri, 'down');
      const delBtn = document.createElement('button'); delBtn.textContent = '✕'; delBtn.onclick = () => rowAction(wi, ri, 'remove');
      actions.append(upBtn, downBtn, delBtn);
      r.appendChild(actions);
      grid.appendChild(r);
    });
    root.appendChild(w);
  });
}

function waveAction(wi, act) {
  const wave = state.level.waves[wi];
  const cols = wave.rows.length ? wave.rows[0].length : DEFAULT_COLS;
  switch (act) {
    case 'add-row': wave.rows.push('.'.repeat(cols)); break;
    case 'add-col': wave.rows = wave.rows.map(r => r + '.'); break;
    case 'remove-col': wave.rows = wave.rows.map(r => r.length > 1 ? r.slice(0, -1) : r); break;
    case 'move-up':
      if (wi > 0) { [state.level.waves[wi - 1], state.level.waves[wi]] = [state.level.waves[wi], state.level.waves[wi - 1]]; }
      break;
    case 'move-down':
      if (wi < state.level.waves.length - 1) { [state.level.waves[wi + 1], state.level.waves[wi]] = [state.level.waves[wi], state.level.waves[wi + 1]]; }
      break;
    case 'remove':
      if (confirm('Remove wave ' + (wi + 1) + '?')) state.level.waves.splice(wi, 1);
      break;
  }
  render();
}

function rowAction(wi, ri, act) {
  const wave = state.level.waves[wi];
  switch (act) {
    case 'up':
      if (ri > 0) { [wave.rows[ri - 1], wave.rows[ri]] = [wave.rows[ri], wave.rows[ri - 1]]; }
      break;
    case 'down':
      if (ri < wave.rows.length - 1) { [wave.rows[ri + 1], wave.rows[ri]] = [wave.rows[ri], wave.rows[ri + 1]]; }
      break;
    case 'remove':
      wave.rows.splice(ri, 1);
      break;
  }
  render();
}

function escapeAttr(s) {
  return String(s).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');
}

document.getElementById('add-wave-btn').onclick = () => {
  if (!state.level) { setStatus('Select or create a level first', 'error'); return; }
  state.level.waves.push({ name: 'New Wave', rowDelay: 30, rows: ['.'.repeat(DEFAULT_COLS)] });
  render();
};

document.getElementById('save-btn').onclick = async () => {
  if (!state.level || !state.name) return;
  try {
    const body = JSON.stringify(state.level, null, 2);
    await fetch('/api/levels/' + encodeURIComponent(state.name), {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body,
    }).then(async r => { if (!r.ok) throw new Error(await r.text() || r.statusText); });
    setStatus('Saved', 'success');
  } catch (e) {
    setStatus('Save failed: ' + e.message, 'error');
  }
};

document.getElementById('reload-btn').onclick = async () => {
  try {
    await api('/api/levels/reload', { method: 'POST' });
    setStatus('Server reloaded', 'success');
  } catch (e) {
    setStatus('Reload failed: ' + e.message, 'error');
  }
};

document.getElementById('delete-btn').onclick = async () => {
  if (!state.name) return;
  if (!confirm('Delete ' + state.name + ' from disk? (Embedded fallback may still apply)')) return;
  try {
    await fetch('/api/levels/' + encodeURIComponent(state.name), { method: 'DELETE' })
      .then(async r => { if (!r.ok) throw new Error(await r.text() || r.statusText); });
    state.name = null; state.level = null;
    await loadLevels();
    render();
    setStatus('Deleted', 'success');
  } catch (e) {
    setStatus('Delete failed: ' + e.message, 'error');
  }
};

document.getElementById('new-level-btn').onclick = async () => {
  const input = document.getElementById('new-level-name');
  let name = input.value.trim();
  if (!name) return;
  if (!name.endsWith('.json')) name += '.json';
  const empty = { waves: [{ name: 'Wave 1', rowDelay: 30, rows: ['.'.repeat(DEFAULT_COLS)] }] };
  try {
    await fetch('/api/levels/' + encodeURIComponent(name), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(empty, null, 2),
    }).then(async r => { if (!r.ok) throw new Error(await r.text() || r.statusText); });
    input.value = '';
    await loadLevel(name);
    setStatus('Created', 'success');
  } catch (e) {
    setStatus('Create failed: ' + e.message, 'error');
  }
};

loadLevels().catch(e => setStatus('Failed to load levels: ' + e.message, 'error'));
</script>
</body>
</html>`

'use strict';

if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/sw.js');
}

// ── DOM refs ──
const chat = document.getElementById('chat');
const input = document.getElementById('msg-input');
const btn = document.getElementById('send-btn');
const charSelect = document.getElementById('char-select');
const panelToggle = document.getElementById('panel-toggle');
const panel = document.getElementById('panel');

let isStreaming = false;

// ── Panel toggle (mobile drawer) ──
panelToggle.addEventListener('click', () => panel.classList.toggle('open'));
panel.addEventListener('click', (e) => {
  if (e.target === panel && window.innerWidth <= 768) panel.classList.remove('open');
});

// ── Character list ──
fetch('/api/characters').then(r => r.json()).then(d => {
  charSelect.innerHTML = '';
  for (const name of d.characters) {
    const opt = document.createElement('option');
    opt.value = name; opt.textContent = name;
    if (name === d.active) opt.selected = true;
    charSelect.appendChild(opt);
  }
});

charSelect.addEventListener('change', async () => {
  const name = charSelect.value;
  try {
    const resp = await fetch('/api/switch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ character: name })
    });
    if (!resp.ok) { alert('切换失败'); return; }

    const data = await resp.json();
    chat.innerHTML = '';
    addMsg('system', '已切换到: ' + name);

    if (data.npc_actions && data.npc_actions.length > 0) {
      const lines = data.npc_actions.map(a => `  ${a.character}: ${a.summary}`);
      addMsg('system', '你不在时发生的事:\n' + lines.join('\n'));
    }
  } catch (err) { alert('切换错误'); }
});

// ── Chat ──
function addMsg(role, text) {
  const div = document.createElement('div');
  div.className = 'msg ' + role;
  const rl = document.createElement('div');
  rl.className = 'role';
  rl.textContent = role === 'user' ? 'YOU' : role === 'system' ? 'SYS' : charSelect.value || 'NPC';
  const bubble = document.createElement('div');
  bubble.className = 'bubble';
  bubble.textContent = text;
  const time = document.createElement('div');
  time.className = 'time';
  time.textContent = new Date().toLocaleTimeString('zh-CN', {hour:'2-digit', minute:'2-digit'});
  div.appendChild(rl);
  div.appendChild(bubble);
  div.appendChild(time);
  chat.appendChild(div);
  chat.scrollTop = chat.scrollHeight;
  return bubble;
}

async function send() {
  const text = input.value.trim();
  if (!text || isStreaming) return;
  input.value = '';
  addMsg('user', text);
  isStreaming = true; btn.disabled = true;

  const bubble = addMsg('assistant', '');

  try {
    const resp = await fetch('/api/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message: text })
    });

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop();
      for (const line of lines) {
        const trimmed = line.trim();
        if (trimmed.startsWith('data: ')) {
          const data = trimmed.slice(6);
          if (data === '[DONE]') continue;
          bubble.textContent += data;
          chat.scrollTop = chat.scrollHeight;
        }
      }
    }
  } catch (err) { bubble.textContent += '\n[连接错误]'; }

  isStreaming = false; btn.disabled = false;
  input.focus();
  refreshPanel();
}

btn.addEventListener('click', send);
input.addEventListener('keydown', e => { if (e.key === 'Enter') send(); });

// ── Panel refresh ──
async function refreshPanel() {
  try {
    // World state
    const stateResp = await fetch('/api/state');
    const state = await stateResp.json();
    document.getElementById('ws-loc').textContent = state.scene?.location || '--';
    document.getElementById('ws-time').textContent = (state.scene?.time_of_day || '') + ' · D' + (state.clock?.day || 0);
    document.getElementById('ws-weather').textContent = state.scene?.weather || '--';
    document.getElementById('ws-chars').textContent = (state.scene?.characters || []).join(', ') || '--';

    // Tension
    const t = state.tension || 0;
    const tPct = Math.min(100, Math.round(t * 100));
    document.getElementById('tension-fill').style.width = tPct + '%';
    document.getElementById('tension-val').textContent = t.toFixed(1);
    if (t > 0.7) document.getElementById('tension-fill').style.background = 'var(--danger)';
    else if (t > 0.35) document.getElementById('tension-fill').style.background = 'var(--warn)';
    else document.getElementById('tension-fill').style.background = 'var(--accent)';

    // Clock
    const clk = state.clock || {};
    document.getElementById('clock-val').textContent =
      String(clk.hour || 0).padStart(2,'0') + ':' + String(clk.minute || 0).padStart(2,'0');

    // Debug info
    const debugResp = await fetch('/api/debug/memory');
    const debug = await debugResp.json();
    document.getElementById('ws-nstate').textContent = debug.narrative_state || '--';
    document.getElementById('mem-facts').textContent = (debug.canonical_events || 0);
    document.getElementById('mem-mode').textContent = debug.vector_search ? '向量' : '关键词';
    document.getElementById('mem-mode').className = debug.vector_search ? 'value info' : 'value';
    document.getElementById('mem-dialogue').textContent = (debug.dialogue_in_memory || 0);
    document.getElementById('mem-compress').textContent = (debug.quarantined_events || 0);

    // NPC feed
    if (debug.npc_actions && debug.npc_actions.length > 0) {
      const feed = document.getElementById('npc-feed');
      feed.innerHTML = '';
      for (const a of debug.npc_actions.slice(-8)) {
        feed.innerHTML += `<div class="item"><span class="actor">${a.character}</span> ${a.summary} <span class="ts">T${a.tick}</span></div>`;
      }
    }

    // Usage stats
    try {
      const usageResp = await fetch('/api/usage');
      const usage = await usageResp.json();
      document.getElementById('us-calls').textContent = usage.total_calls || 0;
      document.getElementById('us-tokens').textContent = (usage.total_tokens || 0).toLocaleString();
      document.getElementById('us-cost').textContent = usage.estimated_cost || '¥0';
      const maxT = 100000;
      const barPct = Math.min(100, ((usage.total_tokens || 0) / maxT) * 100);
      document.getElementById('us-bar').style.width = barPct + '%';
      document.getElementById('token-val').textContent = ((usage.total_tokens || 0) / 1000).toFixed(1) + 'K';
    } catch (_) {}
  } catch (_) {}
}

// ── Initial load + periodic refresh ──
refreshPanel();
setInterval(refreshPanel, 15000);

// ── Keyboard shortcut: toggle panel ──
document.addEventListener('keydown', e => {
  if (e.ctrlKey && e.key === 'b') { e.preventDefault(); panel.classList.toggle('open'); }
});

input.focus();

'use strict';

if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/sw.js');
}

// ── DOM ──
const chatScroll = document.getElementById('chat-scroll');
const input = document.getElementById('msg-input');
const btn = document.getElementById('send-btn');
const charSelect = document.getElementById('char-select');
const panelToggle = document.getElementById('panel-toggle');
const panel = document.getElementById('panel');

let isStreaming = false;
let lastSpeaker = null;
let msgCount = 0;

// ── Theme toggle (3-way: dark → light → kraft) ──
const themes = ['dark', 'light', 'kraft'];
const themeIcons = { dark: '◐', light: '◑', kraft: '◒' };
const themeToggle = document.getElementById('theme-toggle');
const savedTheme = themes.includes(localStorage.getItem('corerp-theme')) ? localStorage.getItem('corerp-theme') : 'dark';
document.documentElement.setAttribute('data-theme', savedTheme);
themeToggle.textContent = themeIcons[savedTheme];
themeToggle.addEventListener('click', () => {
  const cur = document.documentElement.getAttribute('data-theme');
  const idx = themes.indexOf(cur);
  const next = themes[(idx + 1) % themes.length];
  document.documentElement.setAttribute('data-theme', next);
  themeToggle.textContent = themeIcons[next];
  localStorage.setItem('corerp-theme', next);
});

// ── Panel toggle (mobile) ──
panelToggle.addEventListener('click', () => panel.classList.toggle('open'));
panel.addEventListener('click', e => {
  if (e.target === panel && window.innerWidth <= 768) panel.classList.remove('open');
});

// ── Character list ──
async function loadChars() {
  const d = await fetch('/api/characters').then(r => r.json());
  const count = d.characters.length;
  charSelect.innerHTML = '';

  // Panel character list
  const charPanel = document.getElementById('pan-chars');
  const charSection = document.getElementById('char-panel-section');
  if (charPanel) {
    charPanel.innerHTML = '';
    for (const name of d.characters) {
      const row = document.createElement('div');
      row.className = 'stat-row';
      row.style.cursor = 'pointer';
      const key = document.createElement('span');
      key.className = 'stat-key';
      key.textContent = (name === d.active ? '● ' : '○ ') + name;
      if (name === d.active) key.style.color = 'var(--accent)';
      row.appendChild(key);
      row.addEventListener('click', () => {
        charSelect.value = name;
        charSelect.dispatchEvent(new Event('change'));
      });
      charPanel.appendChild(row);
    }
    charSection.style.display = count > 1 ? '' : 'none';
  }

  // Header dropdown
  if (count <= 1) {
    charSelect.style.display = 'none';
    return;
  }
  charSelect.style.display = '';
  for (const name of d.characters) {
    const opt = document.createElement('option');
    opt.value = name; opt.textContent = (name === d.active ? '● ' : '○ ') + name;
    if (name === d.active) opt.selected = true;
    charSelect.appendChild(opt);
  }
}
loadChars();

charSelect.addEventListener('change', async () => {
  const name = charSelect.value;
  const resp = await fetch('/api/switch', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ character: name })
  });
  if (!resp.ok) return;
  const data = await resp.json();
  chatScroll.innerHTML = '';
  lastSpeaker = null; msgCount = 0;
  addSceneDivider('切换到 ' + name);

  if (data.npc_actions && data.npc_actions.length > 0) {
    const lines = data.npc_actions.map(a => a.summary);
    addMsg('system', '你不在时的动态', lines.join(' / '));
  }
  loadChars(); // refresh active indicator
  refreshPanel();
});

// ── Chat rendering ──
function addSceneDivider(text) {
  const div = document.createElement('div');
  div.className = 'scene-divider';
  div.textContent = text;
  chatScroll.appendChild(div);
  chatScroll.scrollTop = chatScroll.scrollHeight;
}

function addMsg(role, title, text) {
  msgCount++;
  const div = document.createElement('div');
  div.className = 'msg ' + role;
  div.id = 'msg-' + msgCount;

  if (role !== 'system') {
    const byline = document.createElement('div');
    byline.className = 'byline';
    const nm = document.createElement('span');
    nm.className = 'name';
    nm.textContent = role === 'user' ? 'YOU' : (title || charSelect.value);
    byline.appendChild(nm);
    const tm = document.createElement('span');
    tm.className = 'time';
    tm.textContent = new Date().toLocaleTimeString('zh-CN', {hour:'2-digit', minute:'2-digit'});
    byline.appendChild(tm);
    div.appendChild(byline);
  }

  const bubble = document.createElement('div');
  bubble.className = 'bubble';
  bubble.textContent = text;
  div.appendChild(bubble);
  chatScroll.appendChild(div);
  chatScroll.scrollTop = chatScroll.scrollHeight;
  lastSpeaker = role;
  return bubble;
}

async function send() {
  const text = input.value.trim();
  if (!text || isStreaming) return;
  input.value = '';
  addMsg('user', null, text);
  isStreaming = true; btn.disabled = true;

  const bubble = addMsg('assistant', null, '');

  try {
    const resp = await fetch('/api/chat', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message: text })
    });
    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buf = '';
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      const lines = buf.split('\n');
      buf = lines.pop();
      for (const line of lines) {
        const t = line.trim();
        if (t.startsWith('data: ')) {
          const d = t.slice(6);
          if (d === '[DONE]') continue;
          bubble.textContent += d;
          chatScroll.scrollTop = chatScroll.scrollHeight;
        }
      }
    }
  } catch (_) { bubble.textContent += '\n[连接中断]'; }

  isStreaming = false; btn.disabled = false;
  input.focus();
  refreshPanel();
}

btn.addEventListener('click', send);
input.addEventListener('keydown', e => { if (e.key === 'Enter') send(); });

// ── Panel refresh ──
async function refreshPanel() {
  try {
    const [state, debug] = await Promise.all([
      fetch('/api/state').then(r => r.json()),
      fetch('/api/debug/memory').then(r => r.json())
    ]);

    // Scene
    const scene = state.scene || {};
    const wd = scene.description
      ? scene.description.replace(/^.\*?[：:]\\s*/, '').substring(0, 120)
      : '--';
    document.getElementById('pan-scene').textContent = wd || '--';
    document.getElementById('pan-loc').textContent = scene.location || '--';
    document.getElementById('pan-time').textContent = (scene.time_of_day || '') + ' · D' + (state.clock?.day || 0);
    document.getElementById('pan-weather').textContent = scene.weather || '--';
    document.getElementById('pan-nstate').textContent = debug.narrative_state || '--';

    // Tension
    const t = state.tension || 0;
    const tPct = Math.min(100, Math.round(Math.abs(t) * 100));
    document.getElementById('pan-tension-bar').style.width = tPct + '%';
    if (t > 0.7) document.getElementById('pan-tension-bar').style.background = '#EF4444';
    else if (t > 0.35) document.getElementById('pan-tension-bar').style.background = '#F59E0B';
    else document.getElementById('pan-tension-bar').style.background = 'var(--accent)';
    document.getElementById('tension-dot').style.background = t > 0.35 ? (t > 0.7 ? '#EF4444' : '#F59E0B') : 'var(--accent)';
    document.getElementById('tension-val').textContent = t.toFixed(2);

    // Clock
    const clk = state.clock || {};
    document.getElementById('clock-display').textContent =
      'D' + (clk.day || 0) + ' · ' +
      String(clk.hour || 0).padStart(2,'0') + ':' + String(clk.minute || 0).padStart(2,'0');

    // Memory
    document.getElementById('pan-facts').textContent = debug.canonical_events || '--';
    document.getElementById('pan-vmode').textContent = debug.vector_search ? '向量' : '关键词';
    document.getElementById('pan-vmode').className = debug.vector_search ? 'stat-val highlight' : 'stat-val';
    document.getElementById('pan-dialogue').textContent = debug.dialogue_in_memory || 0;
    document.getElementById('pan-events').textContent = debug.quarantined_events || 0;

    // NPC feed
    const npcActions = debug.npc_actions;
    if (npcActions && npcActions.length > 0) {
      let html = '';
      for (const a of npcActions.slice(-5).reverse()) {
        html += `<div class="npc-item"><span class="who">${a.character}</span> <span class="what">${a.summary}</span> <span class="when">· T${a.tick}</span></div>`;
      }
      document.getElementById('pan-npc').innerHTML = html;
    }

    // Usage
    try {
      const usage = await fetch('/api/usage').then(r => r.json());
      document.getElementById('pan-calls').textContent = usage.total_calls || 0;
      document.getElementById('pan-tokens').textContent = ((usage.total_tokens || 0) / 1000).toFixed(1) + 'K';
      document.getElementById('pan-cost').textContent = usage.estimated_cost || '¥0';
      document.getElementById('token-stat').querySelector('.val').textContent = ((usage.total_tokens || 0) / 1000).toFixed(1) + 'K';
    } catch (_) {}
  } catch (_) {}
}

// ── LLM Config management ──
const cfgForm = document.getElementById('cfg-form');
const cfgAddBtn = document.getElementById('cfg-add-btn');
document.getElementById('cfg-cancel').addEventListener('click', () => { cfgForm.style.display = 'none'; });
cfgAddBtn.addEventListener('click', () => { cfgForm.style.display = 'block'; });

document.getElementById('cfg-save').addEventListener('click', async () => {
  const cfg = {
    name: document.getElementById('cfg-name').value.trim(),
    endpoint: document.getElementById('cfg-endpoint').value.trim(),
    api_key: document.getElementById('cfg-key').value.trim(),
    model: document.getElementById('cfg-model').value.trim()
  };
  if (!cfg.name || !cfg.endpoint) return;
  await fetch('/api/llm-configs', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cfg)
  });
  cfgForm.style.display = 'none';
  document.getElementById('cfg-name').value = '';
  document.getElementById('cfg-endpoint').value = '';
  document.getElementById('cfg-key').value = '';
  document.getElementById('cfg-model').value = '';
  loadLLMConfigs();
});

async function loadLLMConfigs() {
  const [configs, active] = await Promise.all([
    fetch('/api/llm-configs').then(r => r.json()),
    fetch('/api/llm-active').then(r => r.json()).catch(() => ({}))
  ]);
  const el = document.getElementById('pan-llm-configs');

  // Active config first
  const activeName = active.name || '';
  let html = '';
  if (activeName) {
    html += `<div style="font-size:9px;color:var(--accent);margin-bottom:4px;font-weight:600">● 当前使用</div>`;
    html += `<div class="stat-row">
      <span class="stat-key" style="font-size:10px">${activeName}</span>
      <span class="stat-val" style="font-size:9px">${active.model || ''}</span>
    </div>`;
    html += `<div style="font-size:9px;color:var(--fg-tertiary);margin-bottom:6px">${active.endpoint || ''}</div>`;
  }

  // Saved configs
  if (configs.length > 0) html += `<div style="font-size:9px;color:var(--fg-tertiary);margin-bottom:4px;margin-top:8px">已保存</div>`;
  html += configs.filter(c => c.name !== activeName).map(c => `
    <div class="stat-row">
      <span class="stat-key" style="font-size:10px">${c.name}</span>
      <span class="stat-val" style="font-size:9px;cursor:pointer;color:var(--fg-tertiary)" data-del="${c.name}" title="删除">✕</span>
    </div>
    <div style="font-size:9px;color:var(--fg-tertiary);margin-bottom:4px">${c.model} @ ${c.endpoint}</div>
  `).join('');
  el.innerHTML = html;
  el.querySelectorAll('[data-del]').forEach(btn => {
    btn.addEventListener('click', async () => {
      await fetch('/api/llm-configs/' + btn.dataset.del, { method: 'DELETE' });
      loadLLMConfigs();
    });
  });
}

// Pricing popup
const pricingPopup = document.getElementById('pricing-popup');
document.getElementById('pricing-btn').addEventListener('click', async () => {
  if (pricingPopup.style.display === 'block') { pricingPopup.style.display = 'none'; return; }
  const active = await fetch('/api/llm-active').then(r => r.json()).catch(() => ({}));
  document.getElementById('price-prompt').value = active.prompt_price || 1.0;
  document.getElementById('price-comp').value = active.completion_price || 4.0;
  pricingPopup.style.display = 'block';
});
document.getElementById('price-save').addEventListener('click', async () => {
  const active = await fetch('/api/llm-active').then(r => r.json()).catch(() => ({}));
  active.prompt_price = parseFloat(document.getElementById('price-prompt').value) || 1.0;
  active.completion_price = parseFloat(document.getElementById('price-comp').value) || 4.0;
  await fetch('/api/llm-active', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(active)
  });
  pricingPopup.style.display = 'none';
  refreshPanel();
});

// ── Init ──
refreshPanel();
loadLLMConfigs();
setInterval(refreshPanel, 15000);

document.addEventListener('keydown', e => {
  if (e.ctrlKey && e.key === 'b') { e.preventDefault(); panel.classList.toggle('open'); }
});

input.focus();

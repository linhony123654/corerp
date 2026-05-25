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

// ── Reset button ──
document.getElementById('reset-btn').addEventListener('click', resetConversation);

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

    // Message count
    const dCount = debug.dialogue_in_memory || 0;
    document.getElementById('msg-count').textContent = dCount + '条';

    // World info
    try {
      const world = await fetch('/api/world').then(r => r.json());
      document.getElementById('pan-world-name').textContent = world.name || '--';
    } catch(_) {}
    try {
      const branches = await fetch('/api/branches').then(r => r.json());
      document.getElementById('pan-branches').textContent = (branches.branches || []).length;
    } catch(_) {}
    try {
      const comp = await fetch('/api/compression-stats').then(r => r.json());
      document.getElementById('pan-compressed').textContent = comp.compressed_events || 0;
      document.getElementById('pan-summaries').textContent = comp.summary_events || 0;
    } catch(_) {}

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

document.getElementById('cfg-fetch-models').addEventListener('click', async () => {
  const ep = document.getElementById('cfg-endpoint').value.trim();
  const key = document.getElementById('cfg-key').value.trim();
  if (!ep) return alert('请先填写 API 地址');
  const tn = '_fetch_tmp';
  await fetch('/api/llm-configs', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({name:tn,endpoint:ep,api_key:key,model:''}) });
  try {
    const r = await fetch('/api/llm-models?config='+tn).then(r=>r.json());
    const sel = document.getElementById('cfg-model-select');
    sel.innerHTML = ''; sel.style.display = 'block';
    (r.models||[]).forEach(m => { const o=document.createElement('option');o.value=m;o.textContent=m;sel.appendChild(o); });
    sel.onchange = () => { document.getElementById('cfg-model').value = sel.value; };
  } catch(_) { alert('拉取失败'); }
  fetch('/api/llm-configs/'+tn, {method:'DELETE'});
});

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
      <span class="stat-val" style="font-size:9px;cursor:pointer;color:var(--fg-tertiary)" data-del="${activeName}" title="删除">✕</span>
    </div>`;
    html += `<div style="font-size:9px;color:var(--fg-tertiary);margin-bottom:2px">${active.model || ''}</div>`;
    html += `<div style="font-size:8px;color:var(--fg-tertiary);margin-bottom:6px">${active.endpoint || ''}</div>`;
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

  // Routes table
  try {
    const routes = await fetch('/api/llm-routes').then(r => r.json());
    html += `<div style="font-size:9px;color:var(--fg-tertiary);margin-top:8px;margin-bottom:2px">路由表</div>`;
    for (const [task, adapter] of Object.entries(routes.routes || {})) {
      html += `<div style="font-size:9px;padding:1px 0;color:var(--fg-tertiary)">${task} → <span style="color:var(--fg-secondary)">${adapter}</span></div>`;
    }
  } catch(_) {}
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

// ── Message limit control ──
const msgLimitSlider = document.getElementById('msg-limit-slider');
const msgLimitVal = document.getElementById('msg-limit-val');
msgLimitSlider.addEventListener('input', () => {
  msgLimitVal.textContent = msgLimitSlider.value;
  localStorage.setItem('corerp-msg-limit', msgLimitSlider.value);
});
const savedMsgLimit = parseInt(localStorage.getItem('corerp-msg-limit')) || 30;
msgLimitSlider.value = savedMsgLimit;
msgLimitVal.textContent = savedMsgLimit;

// ── Dialogue restore on load ──
async function restoreDialogue() {
  const limit = msgLimitSlider.value || 30;
  const d = await fetch('/api/dialogue?limit=' + limit).then(r => r.json()).catch(() => ({}));
  const msgs = d.messages || [];
  if (msgs.length === 0) return;
  chatScroll.innerHTML = '';
  for (const m of msgs) {
    addMsg(m.role, null, m.content);
  }
}

// ── Reset conversation ──
async function resetConversation() {
  if (!confirm('确定要重新开始对话吗？当前对话将被清除。')) return;
  await fetch('/api/dialogue/reset', { method: 'POST' });
  chatScroll.innerHTML = '';
  msgCount = 0;
  lastSpeaker = null;
  addMsg('system', '对话已重置');
}

// ── Tension slider ──
const tensionSlider = document.getElementById('tension-slider');
let tensionTimeout;
tensionSlider.addEventListener('input', () => {
  clearTimeout(tensionTimeout);
  tensionTimeout = setTimeout(async () => {
    await fetch('/api/director', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: 'set_tension', value: tensionSlider.value / 100 })
    });
    refreshPanel();
  }, 300);
});

// ── Compress button ──
document.getElementById('compress-btn').addEventListener('click', async () => {
  const total = parseInt(document.getElementById('pan-events')?.textContent) || 0;
  if (total < 100) return alert('事件数不足，无需压缩');
  await fetch('/api/compress', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ from: 0, to: Math.min(total, 200) })
  });
  refreshPanel();
});

// ── Causal chain modal ──
const chainModal = document.getElementById('chain-modal');
document.getElementById('chain-close').addEventListener('click', () => { chainModal.style.display = 'none'; });
chainModal.addEventListener('click', e => { if (e.target === chainModal) chainModal.style.display = 'none'; });

async function showCausalChain(eventId) {
  try {
    const r = await fetch('/api/causality?id=' + eventId + '&depth=3').then(r => r.json());
    document.getElementById('chain-content').textContent = r.summary || '无因果数据';
    // Replay state
    const replay = await fetch('/api/replay?id=' + eventId).then(r => r.json());
    document.getElementById('chain-replay').innerHTML = `
      <div style="font-size:10px;color:var(--fg-tertiary);margin-bottom:4px">回放状态: 第${replay.clock?.day||0}天 ${replay.clock?.hour||0}:${String(replay.clock?.minute||0).padStart(2,'0')} | ${replay.scene?.location||'?'} | 张力${(replay.tension||0).toFixed(2)}</div>
    `;
    chainModal.style.display = 'flex';
  } catch (_) { alert('查询失败'); }
}

// ── Timeline ──
async function loadTimeline() {
  try {
    const [branches, tl] = await Promise.all([
      fetch('/api/branches').then(r => r.json()),
      fetch('/api/timeline?limit=30').then(r => r.json())
    ]);
    const sel = document.getElementById('tl-branch');
    sel.innerHTML = '';
    for (const b of branches.branches || ['main']) {
      const opt = document.createElement('option');
      opt.value = b; opt.textContent = b;
      if (b === 'main') opt.selected = true;
      sel.appendChild(opt);
    }
    sel.onchange = () => loadTimelineBranch(sel.value);
    renderTimeline(tl.timeline || []);
  } catch (_) {}
}

async function loadTimelineBranch(branch) {
  const tl = await fetch('/api/timeline?branch=' + branch + '&limit=30').then(r => r.json());
  renderTimeline(tl.timeline || []);
}

function renderTimeline(timeline) {
  const el = document.getElementById('pan-timeline');
  if (!timeline.length) { el.innerHTML = '<span style="color:var(--fg-tertiary);font-size:10px">无事件</span>'; return; }
  el.innerHTML = timeline.slice(-15).reverse().map(t => {
    const e = t.event;
    const icon = e.type === 'user_message' ? '⬤' : e.type === 'dialogue' ? '◆' : e.type === 'dice_roll' ? '◈' : '·';
    return `<div style="font-size:10px;padding:2px 0;color:var(--fg-tertiary);border-bottom:1px solid var(--border-subtle);cursor:pointer" onclick="showCausalChain('${e.id}')" title="点击查看因果链">
      <span style="color:var(--fg-secondary)">${icon}</span> ${e.type}: ${(e.actor||'?').substring(0,8)} → ${(e.target||'?').substring(0,8)}
      <span style="font-size:8px;color:var(--fg-tertiary)">#${t.index}</span>
    </div>`;
  }).join('');
}

document.getElementById('tl-fork-btn').addEventListener('click', async () => {
  const branch = prompt('新分叉名称:');
  if (!branch) return;
  const tl = await fetch('/api/timeline?limit=1').then(r => r.json());
  const lastEvent = (tl.timeline || [])[tl.timeline.length - 1];
  if (!lastEvent) return alert('没有可分叉的事件');
  await fetch('/api/fork', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ event_id: lastEvent.event.id, branch })
  });
  loadTimeline();
});

// ── Init ──
restoreDialogue();
refreshPanel();
loadLLMConfigs();
loadTimeline();
setInterval(refreshPanel, 15000);
setInterval(loadTimeline, 30000);

document.addEventListener('keydown', e => {
  if (e.ctrlKey && e.key === 'b') { e.preventDefault(); panel.classList.toggle('open'); }
});

input.focus();

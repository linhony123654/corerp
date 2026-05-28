'use strict';

const $ = id => document.getElementById(id);

const els = {
  name: $('world-name'),
  concept: $('world-concept'),
  mode: $('draft-mode'),
  rules: $('world-rules'),
  location: $('starting-location'),
  focus: $('focus-character'),
  draft: $('draft-btn'),
  create: $('create-btn'),
  status: $('status'),
  starterJSON: $('starter-json'),
};

let lastStarter = null;

function setStatus(message, error = false) {
  els.status.textContent = message;
  els.status.classList.toggle('error', error);
}

function setBusy(button, busy, label) {
  button.disabled = busy;
  if (label) {
    button.dataset.idleLabel = button.dataset.idleLabel || button.textContent;
    button.textContent = busy ? label : button.dataset.idleLabel;
  }
}

function readStarterFromEditor() {
  const text = els.starterJSON.value.trim();
  if (!text) return null;
  return JSON.parse(text);
}

function syncQuickFields(starter) {
  if (!starter) return;
  els.location.value = starter.starting_location || '';
  els.focus.value = starter.focus_character || '';
}

function applyQuickFields(starter) {
  const next = { ...(starter || {}) };
  if (els.location.value.trim()) {
    next.starting_location = els.location.value.trim();
  }
  if (els.focus.value.trim()) {
    next.focus_character = els.focus.value.trim();
  }
  return next;
}

async function generateDraft() {
  const concept = els.concept.value.trim();
  if (!concept) {
    setStatus('一句话概念不能为空。', true);
    return;
  }
  setBusy(els.draft, true, '生成中...');
  els.create.disabled = true;
  try {
    const resp = await fetch('/api/worlds/draft', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ concept, mode: els.mode.value || 'local' }),
    });
    if (!resp.ok) {
      throw new Error((await resp.text()) || `${resp.status} ${resp.statusText}`);
    }
    const data = await resp.json();
    lastStarter = data.starter || {};
    syncQuickFields(lastStarter);
    els.starterJSON.value = JSON.stringify(lastStarter, null, 2);
    els.create.disabled = false;
    setStatus(data.source === 'local_draft' ? '已生成本地结构化草稿。' : '已生成 AI 草稿，请确认 JSON 后创建。');
  } catch (err) {
    setStatus(`草稿生成失败：${err.message}`, true);
  } finally {
    setBusy(els.draft, false, '生成中...');
  }
}

async function createWorld() {
  const name = els.name.value.trim();
  if (!name) {
    setStatus('世界名称不能为空。', true);
    return;
  }
  let starter;
  try {
    starter = applyQuickFields(readStarterFromEditor());
  } catch (err) {
    setStatus(`草稿 JSON 无法解析：${err.message}`, true);
    return;
  }
  if (!starter) {
    setStatus('请先生成或填写 starter JSON。', true);
    return;
  }
  setBusy(els.create, true, '创建中...');
  try {
    const createResp = await fetch('/api/worlds', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name,
        core_rules: els.rules.value.trim(),
        starter,
      }),
    });
    if (!createResp.ok) {
      throw new Error((await createResp.text()) || `${createResp.status} ${createResp.statusText}`);
    }
    const created = await createResp.json();
    if (created.path) {
      const enterResp = await fetch('/api/worlds/enter-clean', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: created.path }),
      }).catch(() => null);
      if (enterResp && enterResp.ok) {
        const entered = await enterResp.json().catch(() => ({}));
        if (entered.instance_id) {
          localStorage.setItem('corerp-instance-id', entered.instance_id);
        }
      }
    }
    setStatus('世界已创建，正在返回运行台。');
    window.location.href = '/';
  } catch (err) {
    setStatus(`创建失败：${err.message}`, true);
    els.create.disabled = false;
  } finally {
    setBusy(els.create, false, '创建中...');
  }
}

function init() {
  els.draft.addEventListener('click', generateDraft);
  els.create.addEventListener('click', createWorld);
  els.starterJSON.addEventListener('input', () => {
    els.create.disabled = !els.starterJSON.value.trim();
  });
}

init();

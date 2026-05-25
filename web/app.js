'use strict';

if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/sw.js');
}

const chat = document.getElementById('chat');
const input = document.getElementById('msg-input');
const btn = document.getElementById('send-btn');
const statusEl = document.getElementById('status');
const meta = document.getElementById('world-meta');
const charSelect = document.getElementById('char-select');

let isStreaming = false;

fetch('/api/world').then(r => r.json()).then(d => {
  meta.textContent = d.name || '未知世界';
});

// Load character list
fetch('/api/characters').then(r => r.json()).then(d => {
  charSelect.innerHTML = '';
  for (const name of d.characters) {
    const opt = document.createElement('option');
    opt.value = name;
    opt.textContent = name;
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
    if (!resp.ok) {
      alert('切换失败: ' + (await resp.text()));
      return;
    }
    // Clear chat on switch
    chat.innerHTML = '';
    addMsg('system', '已切换到: ' + name);
  } catch (err) {
    alert('切换错误: ' + err.message);
  }
});

function addMsg(role, text) {
  const div = document.createElement('div');
  div.className = 'msg ' + role;
  const bubble = document.createElement('div');
  bubble.className = 'bubble';
  bubble.textContent = text;
  const time = document.createElement('div');
  time.className = 'time';
  time.textContent = new Date().toLocaleTimeString('zh-CN', {hour:'2-digit', minute:'2-digit'});
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
  isStreaming = true;
  btn.disabled = true;
  statusEl.textContent = '生成中...';

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
  } catch (err) {
    bubble.textContent += '\n[连接错误]';
  }

  isStreaming = false;
  btn.disabled = false;
  statusEl.textContent = '';
  input.focus();
}

btn.addEventListener('click', send);
input.addEventListener('keydown', e => { if (e.key === 'Enter') send(); });
input.focus();

'use strict';

if ('serviceWorker' in navigator) {
  navigator.serviceWorker.getRegistrations().then(registrations => {
    registrations.forEach(reg => reg.unregister());
  }).catch(() => {});
}

const $ = id => document.getElementById(id);

const els = {
  chatScroll: $('chat-scroll'),
  composer: $('composer'),
  input: $('msg-input'),
  sendBtn: $('send-btn'),
  resetBtn: $('reset-btn'),
  instanceSelect: $('instance-select'),
  worldSelect: $('world-select'),
  charSelect: $('char-select'),
  themeToggle: $('theme-toggle'),
  themeIndicator: $('theme-indicator'),
  panel: $('panel'),
  panelToggle: $('panel-toggle'),
  panelClose: $('panel-close'),
  storySpotlightToggle: $('story-spotlight-toggle'),
  chainModal: $('chain-modal'),
  chainClose: $('chain-close'),
  chainContent: $('chain-content'),
  chainReplay: $('chain-replay'),
  chainStrongest: $('chain-strongest'),
  chainStrongestCause: $('chain-strongest-cause'),
  chainStrongestFocus: $('chain-strongest-focus'),
  chainStrongestEffect: $('chain-strongest-effect'),
  tensionSlider: $('tension-slider'),
  msgLimitSlider: $('msg-limit-slider'),
  msgLimitVal: $('msg-limit-val'),
  pricingPopup: $('pricing-popup'),
  priceTargetLabel: $('price-target-label'),
  pricePrompt: $('price-prompt'),
  priceComp: $('price-comp'),
  priceSave: $('price-save'),
  priceCancel: $('price-cancel'),
  cfgAddBtn: $('cfg-add-btn'),
  cfgCancel: $('cfg-cancel'),
  cfgSave: $('cfg-save'),
  cfgFetchModels: $('cfg-fetch-models'),
  cfgForm: $('cfg-form'),
  cfgName: $('cfg-name'),
  cfgEndpoint: $('cfg-endpoint'),
  cfgKey: $('cfg-key'),
  cfgModel: $('cfg-model'),
  cfgModelSelect: $('cfg-model-select'),
  llmConfigs: $('pan-llm-configs'),
  timelineBranch: $('tl-branch'),
  timelineForkBtn: $('tl-fork-btn'),
  diffRefreshBtn: $('diff-refresh-btn'),
  branchDiffA: $('branch-diff-a'),
  branchDiffB: $('branch-diff-b'),
  branchDiffRunBtn: $('branch-diff-run-btn'),
  branchMergeBtn: $('branch-merge-btn'),
  saveDiffA: $('save-diff-a'),
  saveDiffB: $('save-diff-b'),
  saveDiffRunBtn: $('save-diff-run-btn'),
  timeline: $('pan-timeline'),
  compressBtn: $('compress-btn'),
  exportJSONBtn: $('export-json-btn'),
  exportMDBtn: $('export-md-btn'),
  memoryRefreshBtn: $('memory-refresh-btn'),
  saveRefreshBtn: $('save-refresh-btn'),
  saveName: $('save-name'),
  saveNote: $('save-note'),
  saveCreateBtn: $('save-create-btn'),
  presetRefreshBtn: $('preset-refresh-btn'),
  presetName: $('preset-name'),
  presetNote: $('preset-note'),
  presetCreateBtn: $('preset-create-btn'),
  worldcfgReloadBtn: $('worldcfg-reload-btn'),
  worldcfgSaveBtn: $('worldcfg-save-btn'),
  worldcfgName: $('worldcfg-name'),
  worldcfgPath: $('worldcfg-path'),
  worldcfgFormat: $('worldcfg-format'),
  worldcfgRules: $('worldcfg-rules'),
  scenecfgReloadBtn: $('scenecfg-reload-btn'),
  scenecfgSaveBtn: $('scenecfg-save-btn'),
  scenecfgSelect: $('scenecfg-select'),
  scenecfgPath: $('scenecfg-path'),
  scenecfgLocation: $('scenecfg-location'),
  scenecfgTime: $('scenecfg-time'),
  scenecfgWeather: $('scenecfg-weather'),
  scenecfgChars: $('scenecfg-chars'),
  scenecfgDesc: $('scenecfg-desc'),
  factsReloadBtn: $('facts-reload-btn'),
  factsSaveBtn: $('facts-save-btn'),
  factsPath: $('facts-path'),
  factsEditor: $('facts-editor'),
  quarantineRefreshBtn: $('quarantine-refresh-btn'),
  quarantinePromoteAllBtn: $('quarantine-promote-all-btn'),
  quarantineRejectAllBtn: $('quarantine-reject-all-btn'),
  quarantineFilter: $('quarantine-filter'),
  pendingRefreshBtn: $('pending-refresh-btn'),
  pendingConfirmAllBtn: $('pending-confirm-all-btn'),
  pendingDeleteAllBtn: $('pending-delete-all-btn'),
  pendingPromoteAllBtn: $('pending-promote-all-btn'),
  pendingFilter: $('pending-filter'),
  traceRefreshBtn: $('trace-refresh-btn'),
  tracePrevBtn: $('trace-prev-btn'),
  traceNextBtn: $('trace-next-btn'),
  traceStatus: $('trace-status'),
  traceHistory: $('trace-history'),
  charcfgReloadBtn: $('charcfg-reload-btn'),
  charcfgSaveBtn: $('charcfg-save-btn'),
  charcfgName: $('charcfg-name'),
  charcfgPath: $('charcfg-path'),
  charcfgWorldPath: $('charcfg-world-path'),
  charcfgVoiceStyle: $('charcfg-voice-style'),
  charcfgVoiceRhythm: $('charcfg-voice-rhythm'),
  charcfgImmutable: $('charcfg-immutable'),
  charcfgForbidden: $('charcfg-forbidden'),
  charcfgAdaptive: $('charcfg-adaptive'),
  charcfgGoals: $('charcfg-goals'),
  charcfgWriting: $('charcfg-writing'),
  playerRoleName: $('player-role-name'),
  playerRoleBound: $('player-role-bound'),
  playerRoleDesc: $('player-role-desc'),
  playerRoleSaveBtn: $('player-role-save-btn'),
  instancesRefreshBtn: $('instances-refresh-btn'),
  instanceSummary: $('instance-summary'),
  instanceList: $('instance-list'),
  instanceCreateID: $('instance-create-id'),
  instanceCreateLabel: $('instance-create-label'),
  instanceCreateCharacter: $('instance-create-character'),
  instanceCreateBtn: $('instance-create-btn'),
  directorMode: $('director-mode'),
  directorMaxSpeakers: $('director-max-speakers'),
  directorWeights: $('director-weights'),
  directorSaveBtn: $('director-save-btn'),
  structureReloadBtn: $('structure-reload-btn'),
  structureSaveBtn: $('structure-save-btn'),
  structurePath: $('structure-path'),
  structurePremise: $('structure-premise'),
  structureSituation: $('structure-situation'),
  structureStartScene: $('structure-start-scene'),
  structureTimeAnchor: $('structure-time-anchor'),
  structureStability: $('structure-stability'),
  structureFactions: $('structure-factions'),
  structureLocations: $('structure-locations'),
  structurePressures: $('structure-pressures'),
  structureRules: $('structure-rules'),
  popcfgReloadBtn: $('popcfg-reload-btn'),
  popcfgSaveBtn: $('popcfg-save-btn'),
  popcfgPath: $('popcfg-path'),
  popcfgBackground: $('popcfg-background'),
  popcfgPromoteThreshold: $('popcfg-promote-threshold'),
  popcfgMajorThreshold: $('popcfg-major-threshold'),
  popcfgInteractionWeight: $('popcfg-interaction-weight'),
  popcfgMentionWeight: $('popcfg-mention-weight'),
  popcfgEventWeight: $('popcfg-event-weight'),
  popcfgRelationshipWeight: $('popcfg-relationship-weight'),
  popcfgSceneWeight: $('popcfg-scene-weight'),
  popcfgPromotedList: $('popcfg-promoted-list'),
  popcfgIdentityList: $('popcfg-identity-list'),
  worldCreateBtn: $('world-create-btn'),
  worldCreateModal: $('world-create-modal'),
  worldCreateClose: $('world-create-close'),
  worldCreateName: $('world-create-name'),
  worldCreateRules: $('world-create-rules'),
  worldCreateSubmit: $('world-create-submit'),
  worldConvertBtn: $('world-convert-btn'),
  dwMentioned: $('dw-mentioned'),
  dwMentionOrder: $('dw-mention-order'),
  dwContinuity: $('dw-continuity'),
  dwPresent: $('dw-present'),
  dwLocationMatch: $('dw-location-match'),
  dwFactionMatch: $('dw-faction-match'),
  dwPressureMatch: $('dw-pressure-match'),
  dwHookMatch: $('dw-hook-match'),
  dwSilenceDivisor: $('dw-silence-divisor'),
  dwSilenceCap: $('dw-silence-cap'),
  dwTrust: $('dw-trust'),
  dwIntimacy: $('dw-intimacy'),
  dwFear: $('dw-fear'),
  dwKindPersona: $('dw-kind-persona'),
  dwKindNPC: $('dw-kind-npc'),
  dwSourcePromoted: $('dw-source-promoted'),
  dwSourceDefinition: $('dw-source-definition'),
  dwSourceBackground: $('dw-source-background'),
  dwLoaded: $('dw-loaded'),
  simRefreshBtn: $('sim-refresh-btn'),
  simStatus: $('sim-status'),
  simTickCount: $('sim-tick-count'),
  simWorldAdvance: $('sim-world-advance'),
  simTurnCount: $('sim-turn-count'),
  simTickBtn: $('sim-tick-btn'),
  simPauseBtn: $('sim-pause-btn'),
  simResumeBtn: $('sim-resume-btn'),
  simPressureStates: $('sim-pressure-states'),
  simFactionTensions: $('sim-faction-tensions'),
  simNpcExposure: $('sim-npc-exposure'),
};

const state = {
  isStreaming: false,
  msgCount: 0,
  refreshTimer: null,
  timelineTimer: null,
  theme: 'dark',
  pricingTarget: null,
  editingConfigName: null,
  playerRole: { name: '玩家', description: '', bound_character: '' },
  scenes: [],
  quarantineEvents: [],
  pendingFacts: [],
  directorConfig: { mode: 'manual', max_speakers: 1, weights: {} },
  branches: [],
  saves: [],
  presets: [],
  worlds: [],
  traceHistoryItems: [],
  instances: [],
  defaultInstanceID: '',
  selectedInstanceID: '',
  selectedTraceTurn: null,
  mobileSpotlightOpen: false,
  panelGroups: {
    runtime: true,
    authoring: true,
    world: false,
    ops: false,
  },
};

const themeOrder = ['dark', 'light', 'kraft'];
const themeIcons = { dark: '◐', light: '◑', kraft: '◒' };

function setTheme(theme) {
  state.theme = themeOrder.includes(theme) ? theme : 'dark';
  document.documentElement.setAttribute('data-theme', state.theme);
  els.themeToggle.textContent = themeIcons[state.theme];
  els.themeIndicator.textContent = state.theme;
  localStorage.setItem('corerp-theme', state.theme);
}

function nextTheme() {
  const idx = themeOrder.indexOf(state.theme);
  setTheme(themeOrder[(idx + 1) % themeOrder.length]);
}

function loadPanelGroupState() {
  try {
    const raw = localStorage.getItem('corerp-panel-groups');
    if (!raw) {
      return;
    }
    const saved = JSON.parse(raw);
    for (const key of Object.keys(state.panelGroups)) {
      if (typeof saved[key] === 'boolean') {
        state.panelGroups[key] = saved[key];
      }
    }
  } catch (err) {
    console.error(err);
  }
}

function savePanelGroupState() {
  localStorage.setItem('corerp-panel-groups', JSON.stringify(state.panelGroups));
}

function applyPanelGroupState() {
  Object.entries(state.panelGroups).forEach(([group, expanded]) => {
    document.querySelectorAll(`[data-panel-group="${group}"]`).forEach(node => {
      node.classList.toggle('is-collapsed', !expanded);
    });
    document.querySelectorAll(`[data-panel-toggle="${group}"]`).forEach(node => {
      node.setAttribute('aria-expanded', expanded ? 'true' : 'false');
    });
    document.querySelectorAll(`[data-panel-jump="${group}"]`).forEach(node => {
      node.classList.toggle('is-collapsed', !expanded);
    });
  });
}

function togglePanelGroup(group) {
  if (!(group in state.panelGroups)) {
    return;
  }
  state.panelGroups[group] = !state.panelGroups[group];
  savePanelGroupState();
  applyPanelGroupState();
}

function revealPanelGroup(group) {
  if (!(group in state.panelGroups)) {
    return;
  }
  if (!state.panelGroups[group]) {
    state.panelGroups[group] = true;
    savePanelGroupState();
    applyPanelGroupState();
  }
}

const _etagCache = {};

async function fetchJSON(url, options) {
  const fullURL = buildAPIURL(url);
  const headers = { ...(options && options.headers) };
  const cached = _etagCache[fullURL];
  if (cached) headers['If-None-Match'] = cached.etag;
  const resp = await fetch(fullURL, { ...options, headers });
  if (resp.status === 304 && cached) return cached.data;
  if (!resp.ok) {
    const detail = await resp.text().catch(() => '');
    throw new Error(`${resp.status} ${resp.statusText}${detail ? `: ${detail.trim()}` : ''}`);
  }
  const etag = resp.headers.get('ETag');
  const data = await resp.json();
  if (etag) _etagCache[fullURL] = { etag, data };
  return data;
}

function buildAPIURL(url) {
  const current = new URL(url, window.location.origin);
  if (!current.pathname.startsWith('/api/')) {
    return current.pathname + current.search + current.hash;
  }
  if (current.pathname.startsWith('/api/instances')) {
    return current.pathname + current.search + current.hash;
  }
  if (current.pathname.startsWith('/api/llm-') || current.pathname.startsWith('/api/llm-configs') || current.pathname === '/api/change-password') {
    return current.pathname + current.search + current.hash;
  }
  const selected = String(state.selectedInstanceID || '').trim();
  if (selected && !current.searchParams.has('instance_id')) {
    current.searchParams.set('instance_id', selected);
  }
  return current.pathname + current.search + current.hash;
}

async function apiFetch(url, options = {}) {
  return fetch(buildAPIURL(url), options);
}

async function fetchTraceJSON(turn) {
  try {
    if (Number.isFinite(Number(turn)) && Number(turn) > 0) {
      return await fetchJSON(`/api/trace?turn=${Number(turn)}`);
    }
    return await fetchJSON('/api/trace/latest');
  } catch (err) {
    if (String(err.message || '').startsWith('404 ')) {
      try {
        return await fetchJSON('/api/trace');
      } catch (fallbackErr) {
        if (String(fallbackErr.message || '').startsWith('404 ')) {
          const stateResp = await fetchJSON('/api/state');
          if (stateResp?.latest_trace) {
            return stateResp.latest_trace;
          }
          return null;
        }
        throw fallbackErr;
      }
    }
    throw err;
  }
}

function safeText(value, fallback = '--') {
  if (value === null || value === undefined || value === '') {
    return fallback;
  }
  return String(value);
}

function truncate(text, limit) {
  const clean = String(text || '').replace(/\s+/g, ' ').trim();
  if (!clean) {
    return '';
  }
  return clean.length > limit ? `${clean.slice(0, limit)}...` : clean;
}

function closePanelOnMobile() {
  if (window.innerWidth <= 1040) {
    els.panel.classList.remove('open');
  }
}

function applyMobileSpotlightState() {
  const enabled = window.innerWidth <= 560 && state.mobileSpotlightOpen;
  document.body.classList.toggle('mobile-spotlight-open', enabled);
  if (els.storySpotlightToggle) {
    els.storySpotlightToggle.textContent = enabled ? '收起摘要' : '视角摘要';
    els.storySpotlightToggle.setAttribute('aria-expanded', enabled ? 'true' : 'false');
  }
}

function setMobileSpotlightOpen(nextOpen) {
  state.mobileSpotlightOpen = Boolean(nextOpen);
  localStorage.setItem('corerp-mobile-spotlight-open', state.mobileSpotlightOpen ? '1' : '0');
  applyMobileSpotlightState();
}

function closeChainModal() {
  els.chainModal.classList.remove('open');
}

function closePricingPopup() {
  state.pricingTarget = null;
  els.pricingPopup.style.display = 'none';
}

function openPricingPopup(config) {
  state.pricingTarget = config;
  els.priceTargetLabel.textContent = `价格配置：${config.name}`;
  els.pricePrompt.value = config.prompt_price || 1.0;
  els.priceComp.value = config.completion_price || 4.0;
  els.pricingPopup.style.display = 'grid';
}

function renderSceneDivider(text) {
  const node = document.createElement('div');
  node.className = 'scene-divider';
  node.textContent = text;
  els.chatScroll.appendChild(node);
}

function renderMessage(role, title, text) {
  state.msgCount += 1;

  const wrap = document.createElement('div');
  wrap.className = `msg ${role}`;
  wrap.id = `msg-${state.msgCount}`;

  if (role !== 'system') {
    const byline = document.createElement('div');
    byline.className = 'byline';

    const name = document.createElement('span');
    name.textContent = role === 'user' ? (state.playerRole.name || 'USER') : (title || els.charSelect.value || '视角');
    byline.appendChild(name);

    const time = document.createElement('span');
    time.textContent = new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
    byline.appendChild(time);

    wrap.appendChild(byline);
  }

  const bubble = document.createElement('div');
  bubble.className = 'bubble';
  bubble.textContent = text;
  wrap.appendChild(bubble);

  els.chatScroll.appendChild(wrap);
  els.chatScroll.scrollTop = els.chatScroll.scrollHeight;
  return bubble;
}

async function loadCharacters() {
  const data = await fetchJSON('/api/characters');
  const details = Array.isArray(data.participant_details) ? data.participant_details : [];
  const chars = details.length
    ? details.filter(item => item.switchable !== false).map(item => item.name)
    : (data.participants || data.characters || []);
  const active = data.focus_character || data.active || '';
  const detailByName = new Map(details.map(item => [item.name, item]));

  els.charSelect.innerHTML = '';
  chars.forEach(name => {
    const opt = document.createElement('option');
    opt.value = name;
    opt.textContent = name;
    opt.selected = name === active;
    els.charSelect.appendChild(opt);
  });
  els.charSelect.style.display = chars.length > 1 ? '' : 'none';

  const panel = $('pan-chars');
  panel.innerHTML = '';
  details.forEach(detail => {
    const name = detail.name || '';
    const isSwitchable = detail.switchable !== false;
    const meta = [participantKindLabel(detail.kind), participantSourceLabel(detail.source), detail.loaded === false ? '未加载' : '已加载', detail.present ? '在场' : '']
      .filter(Boolean)
      .join(' · ');
    const row = document.createElement(isSwitchable ? 'button' : 'div');
    if (isSwitchable) {
      row.type = 'button';
      row.className = 'interactive-row';
      row.style.cursor = 'pointer';
    } else {
      row.className = 'interactive-row';
      row.style.opacity = '0.6';
      row.style.cursor = 'not-allowed';
    }
    const actionTag = name === active
      ? '<span class="pill">使用中</span>'
      : (isSwitchable ? '<span class="tag">切换</span>' : '<span class="tag" style="background:#333;color:#888;">不可切换</span>');
    const switchReason = !isSwitchable
      ? (detail.source === 'player_role' ? '玩家身份不可切换' : detail.source === 'scene_shell' ? 'scene_shell 不可切换' : '不可切换')
      : '';
    row.innerHTML = `
      <div class="row-main">
        <div class="row-title">${name}</div>
        <div class="row-subtitle">${name === active ? '当前叙事视角' : (isSwitchable ? '切换到该参与者视角' : switchReason)}</div>
        ${meta ? `<div class="row-subtitle">${meta}</div>` : ''}
      </div>
      <div class="row-actions">
        ${actionTag}
      </div>
    `;
    if (isSwitchable) {
      row.addEventListener('click', async () => {
        if (name === els.charSelect.value) {
          return;
        }
        els.charSelect.value = name;
        await switchCharacter(name);
      });
    }
    panel.appendChild(row);
  });

  const bound = state.playerRole.bound_character || '';
  els.playerRoleBound.innerHTML = '<option value="">未绑定</option>';
  chars.forEach(name => {
    const opt = document.createElement('option');
    opt.value = name;
    opt.textContent = name;
    opt.selected = name === bound;
    els.playerRoleBound.appendChild(opt);
  });

  $('char-panel-section').style.display = chars.length > 1 ? '' : 'none';
}

async function loadPlayerRole() {
  try {
    const role = await fetchJSON('/api/player-role');
    state.playerRole = {
      name: role.name || '玩家',
      description: role.description || '',
      bound_character: role.bound_character || '',
    };
    els.playerRoleName.value = state.playerRole.name;
    els.playerRoleDesc.value = state.playerRole.description;
    Array.from(els.playerRoleBound.options).forEach(opt => {
      opt.selected = opt.value === (state.playerRole.bound_character || '');
    });
  } catch (err) {
    console.error(err);
  }
}

async function savePlayerRole() {
  const resp = await apiFetch('/api/player-role', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name: els.playerRoleName.value.trim(),
      bound_character: els.playerRoleBound.value.trim(),
      description: els.playerRoleDesc.value.trim(),
    }),
  });
  if (!resp.ok) {
    alert('用户身份保存失败');
    return;
  }
  const role = await resp.json();
  state.playerRole = {
    name: role.name || '玩家',
    description: role.description || '',
    bound_character: role.bound_character || '',
  };
  els.playerRoleName.value = state.playerRole.name;
  els.playerRoleDesc.value = state.playerRole.description;
  await Promise.all([loadWorlds(), loadCharacters(), restoreDialogue(), refreshPanel()]);
}

function renderDirectorPlan(plan) {
  if (!plan || !Array.isArray(plan.selected) || plan.selected.length === 0) {
    renderInfoList('director-plan', [], '暂无导演决策');
    return;
  }
  const stepLine = Array.isArray(plan.steps) && plan.steps.length
    ? plan.steps.map(step => `${step.index + 1}.${step.speaker}${step.kind === 'followup' ? '↳' : ''}`).join(' · ')
    : plan.selected.join(' -> ');
  const formatCandidate = candidate => {
    const tags = [];
    if (candidate.selected) tags.push('selected');
    if (candidate.mentioned) tags.push('mentioned');
    if (candidate.present) tags.push('present');
    if (candidate.location_match) tags.push('scene');
    if (candidate.faction_match) tags.push('faction');
    if (candidate.pressure_match) tags.push('pressure');
    if (candidate.hook_match) tags.push('hook');
    const reason = safeText(candidate.reason, '--');
    return `${candidate.name} ${Number(candidate.score || 0).toFixed(1)} [${tags.join('/') || 'candidate'}] ${reason}`;
  };
  const selectedDetails = Array.isArray(plan.candidate_details)
    ? plan.candidate_details.filter(candidate => candidate.selected)
    : [];
  const leadDetail = selectedDetails[0] || null;
  const alternateDetails = Array.isArray(plan.candidate_details)
    ? plan.candidate_details.filter(candidate => !candidate.selected).slice(0, 4)
    : [];
  renderInfoList('director-plan', [`
    <div class="interactive-row">
      <div class="row-main">
        <div class="row-title">${plan.mode || 'manual'} · ${plan.selected.join(' -> ')}</div>
        <div class="row-subtitle">${safeText(plan.reason, '--')}</div>
        <div class="row-subtitle">steps：${stepLine}</div>
        <div class="row-subtitle">上位发言者：${safeText(plan.previous_speaker)} · 胜出：${selectedDetails.length ? selectedDetails.map(formatCandidate).join(' · ') : ((plan.candidates || []).join(', ') || '--')}</div>
        ${alternateDetails.length ? `<div class="row-subtitle">未胜出前列：${alternateDetails.map(candidate => `${formatCandidate(candidate)} (${describeCandidateGap(leadDetail, candidate)})`).join(' · ')}</div>` : ''}
      </div>
      <div class="row-actions">
        <span class="tag">${plan.switched ? '已切换' : '未切换'}</span>
      </div>
    </div>
  `], '暂无导演决策');
}

function formatDirectorWeights(weights) {
  return JSON.stringify(weights || {}, null, 2);
}

async function loadDirectorConfig() {
  try {
    const data = await fetchJSON('/api/director-config');
    const cfg = data.config || {};
    state.directorConfig = {
      mode: cfg.mode || 'manual',
      max_speakers: Number(cfg.max_speakers || 1),
      weights: cfg.weights || {},
    };
    els.directorMode.value = state.directorConfig.mode;
    els.directorMaxSpeakers.value = String(state.directorConfig.max_speakers);
    const w = state.directorConfig.weights;
    els.dwMentioned.value = w.mentioned ?? 5;
    els.dwMentionOrder.value = w.mention_order ?? 2;
    els.dwContinuity.value = w.continuity ?? 3;
    els.dwPresent.value = w.present ?? 4;
    els.dwLocationMatch.value = w.location_match ?? 2;
    els.dwFactionMatch.value = w.faction_match ?? 2;
    els.dwPressureMatch.value = w.pressure_match ?? 1.5;
    els.dwHookMatch.value = w.hook_match ?? 3;
    els.dwSilenceDivisor.value = w.silence_divisor ?? 5;
    els.dwSilenceCap.value = w.silence_cap ?? 4;
    els.dwTrust.value = w.trust ?? 0.5;
    els.dwIntimacy.value = w.intimacy ?? 0.3;
    els.dwFear.value = w.fear ?? -0.2;
    els.dwKindPersona.value = w.kind_persona ?? 3;
    els.dwKindNPC.value = w.kind_npc ?? 1;
    els.dwSourcePromoted.value = w.source_promoted ?? 4;
    els.dwSourceDefinition.value = w.source_definition ?? 2;
    els.dwSourceBackground.value = w.source_background ?? 0;
    els.dwLoaded.value = w.loaded ?? 2;
    els.directorWeights.value = formatDirectorWeights(state.directorConfig.weights);
    renderDirectorPlan(data.plan || {});
  } catch (err) {
    console.error(err);
  }
}

async function saveDirectorConfig() {
  const weights = {
    mentioned: Number(els.dwMentioned.value) || 0,
    mention_order: Number(els.dwMentionOrder.value) || 0,
    continuity: Number(els.dwContinuity.value) || 0,
    present: Number(els.dwPresent.value) || 0,
    location_match: Number(els.dwLocationMatch.value) || 0,
    faction_match: Number(els.dwFactionMatch.value) || 0,
    pressure_match: Number(els.dwPressureMatch.value) || 0,
    hook_match: Number(els.dwHookMatch.value) || 0,
    silence_divisor: Number(els.dwSilenceDivisor.value) || 0,
    silence_cap: Number(els.dwSilenceCap.value) || 0,
    trust: Number(els.dwTrust.value) || 0,
    intimacy: Number(els.dwIntimacy.value) || 0,
    fear: Number(els.dwFear.value) || 0,
    kind_persona: Number(els.dwKindPersona.value) || 0,
    kind_npc: Number(els.dwKindNPC.value) || 0,
    source_promoted: Number(els.dwSourcePromoted.value) || 0,
    source_definition: Number(els.dwSourceDefinition.value) || 0,
    source_background: Number(els.dwSourceBackground.value) || 0,
    loaded: Number(els.dwLoaded.value) || 0,
  };
  els.directorWeights.value = JSON.stringify(weights, null, 2);
  const resp = await apiFetch('/api/director-config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      mode: els.directorMode.value,
      max_speakers: Number(els.directorMaxSpeakers.value || 1),
      weights,
    }),
  });
  if (!resp.ok) {
    alert('导演配置保存失败');
    return;
  }
  await Promise.all([loadDirectorConfig(), refreshPanel()]);
}

function renderSimEvolution(data) {
  const ps = data.pressure_states || {};
  const ft = data.faction_tensions || {};
  const ne = data.npc_tick_exposure || {};

  const pressureItems = Object.entries(ps).map(([id, intensity]) => `${safeText(id)}:${Number(intensity).toFixed(2)}`).join(' · ');
  els.simPressureStates.textContent = pressureItems || '暂无 pressure 动态数据';

  const factionItems = Object.entries(ft).map(([id, tension]) => `${safeText(id)}:${Number(tension).toFixed(2)}`).join(' · ');
  els.simFactionTensions.textContent = factionItems || '暂无 faction 紧张度数据';

  const exposureItems = Object.entries(ne).map(([name, count]) => `${safeText(name)}:${count}`).join(' · ');
  els.simNpcExposure.textContent = exposureItems || '暂无 NPC 累积数据';
}

async function loadSimStatus() {
  try {
    const data = await fetchJSON('/api/sim/status');
    els.simStatus.textContent = data.paused ? '已暂停' : data.running ? '运行中' : '未启动';
    els.simStatus.style.color = data.paused ? 'var(--warning)' : data.running ? 'var(--success)' : 'var(--text-muted)';
    els.simTickCount.textContent = String(data.tick_count ?? 0);
    els.simWorldAdvance.textContent = data.world_advance ?? '0s';
    els.simTurnCount.textContent = String(data.turn_count ?? 0);
    els.simPauseBtn.style.display = data.paused ? 'none' : '';
    els.simResumeBtn.style.display = data.paused ? '' : 'none';
    renderSimEvolution(data);
  } catch (err) {
    console.error('sim status error:', err);
  }
}

async function manualTick() {
  try {
    await apiFetch('/api/sim/tick', { method: 'POST' });
    await loadSimStatus();
  } catch (err) {
    alert('手动 Tick 失败');
  }
}

async function pauseTick() {
  try {
    await apiFetch('/api/sim/pause', { method: 'POST' });
    await loadSimStatus();
  } catch (err) {
    alert('暂停失败');
  }
}

async function resumeTick() {
  try {
    await apiFetch('/api/sim/resume', { method: 'POST' });
    await loadSimStatus();
  } catch (err) {
    alert('恢复失败');
  }
}

async function switchCharacter(name) {
  const resp = await apiFetch('/api/switch', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ focus_character: name }),
  });
  if (!resp.ok) {
    alert('切换视角失败');
    return;
  }
  const data = await resp.json();
  els.chatScroll.innerHTML = '';
  state.msgCount = 0;
  renderSceneDivider(`切换到 ${name}`);
  if (Array.isArray(data.npc_actions) && data.npc_actions.length > 0) {
    const summary = data.npc_actions.map(item => item.summary).join(' / ');
    renderMessage('system', '', `你不在时的动态：${summary}`);
  }
  await Promise.all([loadWorlds(), loadCharacters(), refreshPanel(), restoreDialogue()]);
  closePanelOnMobile();
}

async function restoreDialogue() {
  const limit = Number(els.msgLimitSlider.value || 30);
  const data = await fetchJSON(`/api/dialogue?limit=${limit}`).catch(() => ({ messages: [] }));
  const messages = data.messages || [];
  if (messages.length === 0) {
    return;
  }
  els.chatScroll.innerHTML = '';
  state.msgCount = 0;
  messages.forEach(msg => renderMessage(msg.role, null, msg.content));
}

async function sendMessage() {
  const text = els.input.value.trim();
  if (!text || state.isStreaming) {
    return;
  }

  state.isStreaming = true;
  els.sendBtn.disabled = true;
  els.input.value = '';

  renderMessage('user', null, text);
  const bubble = renderMessage('assistant', null, '');

  try {
    const resp = await apiFetch('/api/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message: text }),
    });
    if (!resp.ok || !resp.body) {
      throw new Error('chat failed');
    }

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() || '';
      lines.forEach(line => {
        const trimmed = line.trim();
        if (trimmed.startsWith('data: ')) {
          const chunk = trimmed.slice(6);
          if (chunk !== '[DONE]') {
            bubble.textContent += chunk;
            els.chatScroll.scrollTop = els.chatScroll.scrollHeight;
          }
        }
      });
    }
  } catch (err) {
    bubble.textContent += '\n[连接中断]';
  } finally {
    state.isStreaming = false;
    els.sendBtn.disabled = false;
    els.input.focus();
    state.selectedTraceTurn = null;
    refreshPanel();
    loadTimeline();
    loadTraceHistory();
    loadTraceView();
  }
}

function updateScenePanel(stateData, debugData) {
  const scene = stateData.scene || {};
  const desc = scene.description ? truncate(scene.description.replace(/^.*?[：:]\s*/, ''), 90) : '--';
  $('pan-scene').textContent = desc || '--';
  $('pan-loc').textContent = safeText(scene.location);
  $('pan-time').textContent = `${safeText(scene.time_of_day)} · D${stateData.clock?.day || 0}`;
  $('pan-weather').textContent = safeText(scene.weather);
  $('pan-nstate').textContent = safeText(debugData.narrative_state);
}

function updateTension(stateData) {
  const tension = Number(stateData.tension || 0);
  $('tension-val').textContent = tension.toFixed(2);
  els.tensionSlider.value = Math.max(0, Math.min(100, Math.round(tension * 100)));
}

function updateClock(stateData) {
  const clock = stateData.clock || {};
  $('clock-display').textContent = `D${clock.day || 0} · ${String(clock.hour || 0).padStart(2, '0')}:${String(clock.minute || 0).padStart(2, '0')}`;
}

function updateCharacterCard(character) {
  const identity = character.identity || {};
  const voice = identity.voice || {};
  const adaptive = identity.adaptive || {};
  $('pan-char-name').textContent = safeText(identity.name);
  $('pan-char-style').textContent = safeText(voice.style);
  $('pan-char-traits').textContent = (identity.immutable || []).slice(0, 4).join(' · ') || '--';
  $('pan-char-stats').textContent = Object.entries(adaptive)
    .sort((a, b) => a[0].localeCompare(b[0], 'zh-CN'))
    .map(([key, value]) => `${key}:${value}`)
    .join(' / ') || '--';
}

function updateMemoryPanel(debugData) {
  $('pan-facts').textContent = safeText(debugData.canonical_events, '0');
  $('pan-vmode').textContent = debugData.vector_search ? '向量检索' : '关键词检索';
  $('pan-dialogue').textContent = safeText(debugData.dialogue_in_memory, '0');
  $('pan-events').textContent = safeText(debugData.quarantined_events, '0');
  $('msg-count').textContent = `${debugData.dialogue_in_memory || 0}条`;
}

function renderInfoList(target, items, emptyText) {
  const el = $(target);
  if (!items.length) {
    el.innerHTML = `<div class="note-box">${emptyText}</div>`;
    return;
  }
  el.innerHTML = items.join('');
}

function participantKindLabel(kind) {
  switch (String(kind || '')) {
    case 'player': return '玩家';
    case 'npc': return 'NPC';
    case 'persona': return '人物';
    default: return '参与者';
  }
}

function participantSourceLabel(source) {
  switch (String(source || '')) {
    case 'player_role': return '玩家身份';
    case 'character_definition': return '人物定义';
    case 'promoted_population': return '晋升人口';
    case 'background_population': return '背景人口';
    case 'scene_shell': return '现场壳';
    case 'scene_presence': return '现场存在';
    default: return '';
  }
}

function describeCandidateGap(winner, candidate) {
  if (!winner || !candidate) return '';
  const gap = Number((winner.score || 0) - (candidate.score || 0));
  const missing = [];
  const keys = [
    ['mentioned', '点名'],
    ['mention_order', '点名顺位'],
    ['continuity', '连续性'],
    ['present', '在场'],
    ['location_match', '地点'],
    ['faction_match', '势力'],
    ['pressure_match', 'pressure'],
    ['hook_match', 'hook'],
    ['silence_boost', '静默补偿'],
    ['trust', 'trust'],
    ['intimacy', 'intimacy'],
    ['fear', 'fear'],
    ['opened_by_user', '用户开场'],
    ['tension_switch', '紧张切换'],
    ['kind_persona', 'persona类型'],
    ['kind_npc', 'npc类型'],
    ['source_promoted', '晋升人口'],
    ['source_definition', '人物定义'],
    ['source_background', '背景人口'],
    ['loaded', '已加载'],
  ];
  const winnerBreakdown = winner.score_breakdown || {};
  const candidateBreakdown = candidate.score_breakdown || {};
  for (const [key, label] of keys) {
    const delta = Number((winnerBreakdown[key] || 0) - (candidateBreakdown[key] || 0));
    if (delta > 0.01) {
      missing.push(`${label}-${delta.toFixed(1)}`);
    }
    if (missing.length >= 3) break;
  }
  const gapText = gap > 0 ? `落后 ${gap.toFixed(1)}` : '并列或更高';
  return `${gapText}${missing.length ? ` · ${missing.join(' / ')}` : ''}`;
}

function renderTraceStep(stepTrace) {
  const step = stepTrace?.step || {};
  const memories = Array.isArray(stepTrace?.memories) ? stepTrace.memories.slice(0, 3) : [];
  const goals = Array.isArray(stepTrace?.active_goals) ? stepTrace.active_goals.slice(0, 3) : [];
  const events = Array.isArray(stepTrace?.events) ? stepTrace.events.slice(0, 4) : [];
  const episodic = Array.isArray(stepTrace?.episodic_events) ? stepTrace.episodic_events.slice(0, 2) : [];
  const facts = Array.isArray(stepTrace?.semantic_facts) ? stepTrace.semantic_facts.slice(0, 3) : [];

  return `
    <div class="interactive-row">
      <div class="row-main">
        <div class="row-title">step ${Number(step.index || 0) + 1} · ${safeText(step.speaker)} · ${safeText(step.kind, 'lead')}</div>
        <div class="row-subtitle">${safeText(step.reason, '--')} · budget ${safeText(step.budget_mode, 'normal')} · tokens ${safeText(stepTrace.used_tokens, 0)}/${safeText(stepTrace.token_budget, 0)}</div>
        ${goals.length ? `<div class="row-subtitle">goals: ${goals.map(goal => `${goal.id}(p${goal.priority})`).join(' / ')}</div>` : ''}
        ${memories.length ? `<div class="row-subtitle">memories: ${memories.map(memory => `${memory.type}:${truncate(memory.content || '', 28)}`).join(' / ')}</div>` : ''}
        ${facts.length ? `<div class="row-subtitle">facts: ${facts.map(fact => `${fact.subject}-${fact.predicate}-${truncate(fact.object || '', 18)}`).join(' / ')}</div>` : ''}
        ${episodic.length ? `<div class="row-subtitle">episodic: ${episodic.map(item => truncate(item.description || item.type || '', 24)).join(' / ')}</div>` : ''}
        ${stepTrace.validator?.blocked ? `<div class="row-subtitle">validator blocked: ${safeText(stepTrace.validator.reason, '--')}</div>` : ''}
        ${stepTrace.action_frame?.action ? `<div class="row-subtitle">action: ${stepTrace.action_frame.action} -> ${safeText(stepTrace.action_frame.target, '--')} · ${safeText(stepTrace.action_frame.intent, '--')}</div>` : ''}
        ${events.length ? `<div class="row-subtitle">events: ${events.map(event => `${event.type}${event.target ? `->${event.target}` : ''}`).join(' / ')}</div>` : ''}
        ${stepTrace.error ? `<div class="row-subtitle">error: ${safeText(stepTrace.error, '--')}</div>` : ''}
        ${stepTrace.narrative ? `<div class="row-subtitle">narrative: ${truncate(stepTrace.narrative, 140)}</div>` : ''}
      </div>
      <div class="row-actions">
        <span class="tag mono">${safeText(stepTrace.focus_character || stepTrace.character, '--')}</span>
      </div>
    </div>
  `;
}

function parseLineList(text) {
  return String(text || '')
    .split('\n')
    .map(line => line.trim())
    .filter(Boolean);
}

function selectedTraceIndex() {
  return state.traceHistoryItems.findIndex(trace => Number(trace.turn || 0) === Number(state.selectedTraceTurn || 0));
}

function updateTraceControls() {
  const idx = selectedTraceIndex();
  const hasSelection = idx >= 0;
  const selected = hasSelection ? state.traceHistoryItems[idx] : null;
  els.traceStatus.textContent = selected ? `turn ${selected.turn} · ${safeText(selected.focus_character || selected.character)}` : 'turn --';
  els.tracePrevBtn.disabled = !hasSelection || idx >= state.traceHistoryItems.length - 1;
  els.traceNextBtn.disabled = !hasSelection || idx <= 0;
}

async function selectTraceTurn(turn) {
  const nextTurn = Number(turn || 0) || null;
  if (!nextTurn) {
    return;
  }
  state.selectedTraceTurn = nextTurn;
  await Promise.all([loadTraceHistory(), loadTraceView(nextTurn)]);
}

function resolveCheckpointTraceTurn(slot) {
  const createdAt = Date.parse(slot?.created_at || '');
  const sameCharacter = state.traceHistoryItems.filter(trace => { const fc = slot?.focus_character || slot?.character; return !fc || trace.character === fc; });
  const pool = sameCharacter.length ? sameCharacter : state.traceHistoryItems;
  if (!pool.length) {
    return null;
  }
  if (!Number.isNaN(createdAt)) {
    const olderOrSame = pool.find(trace => {
      const traceTime = Date.parse(trace.created_at || '');
      return !Number.isNaN(traceTime) && traceTime <= createdAt;
    });
    if (olderOrSame?.turn) {
      return olderOrSame.turn;
    }
  }
  return pool[pool.length - 1]?.turn || null;
}

function parseAdaptiveLines(text) {
  const adaptive = {};
  parseLineList(text).forEach(line => {
    const [key, rawValue] = line.split('=');
    const value = Number((rawValue || '').trim());
    if (key && Number.isFinite(value)) {
      adaptive[key.trim()] = value;
    }
  });
  return adaptive;
}

function parseGoals(text) {
  return parseLineList(text).map(line => {
    const parts = line.split('|').map(item => item.trim());
    return {
      type: parts[0] || 'primary',
      id: parts[1] || '',
      priority: Number(parts[2] || 0),
      condition: parts[3] || '',
      target: parts[4] || '',
      known_by: parts[5] ? parts[5].split(',').map(v => v.trim()).filter(Boolean) : [],
      reveal_condition: parts[6] || '',
      cooldown_turns: Number(parts[7] || 0),
    };
  }).filter(goal => goal.id);
}

function renderFactsForEditor(facts) {
  return (facts || []).map(fact => [
    fact.subject || '',
    fact.predicate || '',
    fact.object || '',
    Number(fact.confidence || 1).toFixed(2),
  ].join('|')).join('\n');
}

function parseFacts(text) {
  return parseLineList(text).map(line => {
    const parts = line.split('|').map(item => item.trim());
    return {
      subject: parts[0] || '',
      predicate: parts[1] || '',
      object: parts[2] || '',
      confidence: Number(parts[3] || 1),
    };
  }).filter(fact => fact.subject && fact.predicate && fact.object);
}

function setFieldError(input, message) {
  if (!input) {
    return;
  }
  input.value = message || '';
}

function setFactsEditorMessage(message) {
  if (!els.factsEditor) {
    return;
  }
  els.factsEditor.value = message || '';
}

function updateNPCPanels(debugData, actionStats, actionLog) {
  const npcActions = Array.isArray(debugData.npc_actions) ? debugData.npc_actions.slice(-5).reverse() : [];
  renderInfoList('pan-npc', npcActions.map(item => `
    <div class="interactive-row">
      <div class="row-main">
        <div class="row-title">${item.character}</div>
        <div class="row-subtitle">${item.summary}</div>
      </div>
      <div class="row-actions"><span class="tag mono">T${item.tick}</span></div>
    </div>
  `), '空闲中');

  $('pan-action-fired').textContent = safeText(actionStats.fired, '0');
  $('pan-action-blocked').textContent = safeText(actionStats.blocked, '0');
  $('pan-action-threshold').textContent = safeText(actionStats.below_threshold, '0');

  const entries = Array.isArray(actionLog.entries) ? actionLog.entries.slice(-6).reverse() : [];
  renderInfoList('pan-action-log', entries.map(entry => {
    const status = entry.fired
      ? `${entry.action_type || 'act'} -> ${entry.target || '-'}`
      : `blocked: ${entry.blocked_by || 'unknown'}`;
    const reason = entry.reason || entry.strongest_desire || entry.dominant_emotion || '--';
    return `
      <div class="interactive-row">
        <div class="row-main">
          <div class="row-title">${entry.character} · ${status}</div>
          <div class="row-subtitle">${reason}</div>
        </div>
        <div class="row-actions"><span class="tag mono">P${Number(entry.pressure_total || 0).toFixed(2)}</span></div>
      </div>
    `;
  }), '暂无记录');
}

function updatePopulationRuntimePanel(populationInsights) {
  if (populationInsights && populationInsights.__error) {
    renderInfoList('pan-population-runtime', [], `人口运行数据加载失败：${safeText(populationInsights.error, 'unknown error')}`);
    return;
  }
  const promoted = Array.isArray(populationInsights.promoted) ? populationInsights.promoted : [];
  const background = Array.isArray(populationInsights.background) ? populationInsights.background : [];

  function renderAttentionBar(att) {
    if (!att) return '';
    const max = Math.max(att.direct_interactions || 0, att.mentions || 0, att.shared_events || 0, att.scene_carryover || 0, 1);
    const segments = [
      { label: '互动', value: att.direct_interactions || 0 },
      { label: '提及', value: att.mentions || 0 },
      { label: '事件', value: att.shared_events || 0 },
      { label: '场景', value: att.scene_carryover || 0 },
    ].filter(s => s.value > 0);
    if (segments.length === 0) return '';
    return `<div class="row-subtitle" style="display:flex;gap:4px;flex-wrap:wrap;">${segments.map(s => `<span class="tag" style="font-size:10px;">${s.label}:${s.value}</span>`).join('')}</div>`;
  }

  function renderAdaptiveDrift(history) {
    if (!Array.isArray(history) || history.length === 0) return '';
    const lastShift = history.find(h => h.type === 'population_identity_shift');
    if (!lastShift || !lastShift.adaptive) return '';
    const parts = Object.entries(lastShift.adaptive)
      .sort((a, b) => a[0].localeCompare(b[0], 'zh-CN'))
      .map(([key, value]) => {
        const v = Number(value || 0);
        const icon = v >= 0 ? '↑' : '↓';
        return `<span style="color:${v >= 0 ? '#4ade80' : '#f87171'}">${icon}${key}:${Math.abs(v).toFixed(1)}</span>`;
      });
    return parts.length ? `<div class="row-subtitle" style="font-size:10px;">drift: ${parts.join(' ')}</div>` : '';
  }

  const promotedItems = promoted.slice(0, 4).map(item => {
    const adaptive = item.adaptive || {};
    const adaptiveLine = Object.entries(adaptive)
      .sort((a, b) => a[0].localeCompare(b[0], 'zh-CN'))
      .map(([key, value]) => `${key}:${Number(value || 0).toFixed(2)}`)
      .join(' / ');
    const history = Array.isArray(item.history) ? item.history.slice(0, 3) : [];
    return `
      <div class="interactive-row">
        <div class="row-main">
          <div class="row-title">${item.name} · ${safeText(item.status, 'promoted')}</div>
          <div class="row-subtitle">${safeText(item.growth_summary, '--')}</div>
          ${renderAttentionBar(item.attention)}
          <div class="row-subtitle">score ${Number(item.attention?.score || 0).toFixed(2)} · ${adaptiveLine || 'adaptive --'}</div>
          <div class="row-subtitle">core ${safeText(item.identity_core, '--')}</div>
          ${renderAdaptiveDrift(history)}
          ${history.length ? `<div class="row-subtitle" style="font-size:10px;">${history.map(entry => truncate(entry.summary || entry.type || '', 24)).join(' / ')}</div>` : ''}
        </div>
        <div class="row-actions"><span class="tag mono">${safeText(item.world_path, '--')}</span></div>
      </div>
    `;
  });

  const backgroundItems = background.slice(0, 3).map(item => `
    <div class="interactive-row">
      <div class="row-main">
        <div class="row-title">${item.name}</div>
        <div class="row-subtitle">${safeText(item.growth_summary, '--')}</div>
        ${renderAttentionBar(item.attention)}
      </div>
      <div class="row-actions"><span class="tag mono">${Number(item.attention?.score || 0).toFixed(2)}</span></div>
    </div>
  `);

  const emptyMessage = background.length === 0 ? '当前世界还没有背景人口' : '暂无晋升与生长数据';
  renderInfoList('pan-population-runtime', [...promotedItems, ...backgroundItems], emptyMessage);
}

function updateWorldPanel(world, branches, compression) {
  $('pan-world-name').textContent = safeText(world.name);
  $('pan-branches').textContent = safeText((branches.branches || []).length, '0');
  $('pan-compressed').textContent = safeText(compression.compressed_events, '0');
  $('pan-summaries').textContent = safeText(compression.summary_events, '0');
}

function updateUsagePanel(usage) {
  $('pan-calls').textContent = safeText(usage.total_calls, '0');
  $('pan-tokens').textContent = `${((usage.total_tokens || 0) / 1000).toFixed(1)}K`;
  $('pan-cost').textContent = safeText(usage.estimated_cost, '¥0');
  $('token-stat').querySelector('.val').textContent = `${((usage.total_tokens || 0) / 1000).toFixed(1)}K`;
}

function updateDirectorPanel(stateData) {
  if (stateData.director_config) {
    state.directorConfig = stateData.director_config;
    els.directorMode.value = state.directorConfig.mode || 'manual';
    els.directorMaxSpeakers.value = String(state.directorConfig.max_speakers || 1);
    const w = state.directorConfig.weights || {};
    els.dwMentioned.value = w.mentioned ?? 5;
    els.dwMentionOrder.value = w.mention_order ?? 2;
    els.dwContinuity.value = w.continuity ?? 3;
    els.dwPresent.value = w.present ?? 4;
    els.dwLocationMatch.value = w.location_match ?? 2;
    els.dwFactionMatch.value = w.faction_match ?? 2;
    els.dwPressureMatch.value = w.pressure_match ?? 1.5;
    els.dwHookMatch.value = w.hook_match ?? 3;
    els.dwSilenceDivisor.value = w.silence_divisor ?? 5;
    els.dwSilenceCap.value = w.silence_cap ?? 4;
    els.dwTrust.value = w.trust ?? 0.5;
    els.dwIntimacy.value = w.intimacy ?? 0.3;
    els.dwFear.value = w.fear ?? -0.2;
    els.dwKindPersona.value = w.kind_persona ?? 3;
    els.dwKindNPC.value = w.kind_npc ?? 1;
    els.dwSourcePromoted.value = w.source_promoted ?? 4;
    els.dwSourceDefinition.value = w.source_definition ?? 2;
    els.dwSourceBackground.value = w.source_background ?? 0;
    els.dwLoaded.value = w.loaded ?? 2;
    els.directorWeights.value = formatDirectorWeights(state.directorConfig.weights);
  }
  renderDirectorPlan(stateData.director_plan || {});
}

async function loadMemoryView() {
  try {
    const data = await fetchJSON(`/api/memory?focus_character=${encodeURIComponent(els.charSelect.value || '')}&facts=8&episodic=6&dialogue=8`);
    $('memory-working').textContent = `Working Memory: ${safeText(data.working_memory, '--')}`;
    renderInfoList('memory-facts', (data.facts || []).slice(0, 8).map(fact => `
      <div class="interactive-row">
        <div class="row-main">
          <div class="row-title">${fact.subject} · ${fact.predicate}</div>
          <div class="row-subtitle">${fact.object}</div>
        </div>
        <div class="row-actions"><span class="tag mono">${Number(fact.confidence || 0).toFixed(2)}</span></div>
      </div>
    `), '暂无事实');
    renderInfoList('memory-episodic', (data.episodic || []).slice(-6).reverse().map(item => `
      <div class="interactive-row">
        <div class="row-main">
          <div class="row-title">${timelineTypeLabel(item.type || 'memory')}</div>
          <div class="row-subtitle">${truncate(item.description || '', 70)}</div>
        </div>
        <div class="row-actions"><span class="tag mono">${Number(item.emotional_weight || 0).toFixed(2)}</span></div>
      </div>
    `), '暂无事件记忆');
  } catch (err) {
    console.error(err);
  }
}

function instanceStatusLabel(status) {
  switch (String(status || '').trim()) {
    case 'running':
      return '运行中';
    case 'stopped':
      return '已停止';
    default:
      return safeText(status, '--');
  }
}

async function setDefaultInstance(id) {
  const resp = await apiFetch('/api/instances/default', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id }),
  });
  if (!resp.ok) {
    const message = await resp.text();
    alert(`切换默认实例失败：${message || resp.statusText}`);
    return;
  }
  renderSceneDivider(`默认实例已切换到 ${id}`);
  await Promise.all([loadInstancesView(), loadWorlds(), loadCharacters(), loadPlayerRole(), restoreDialogue(), refreshPanel(), loadTimeline(), loadMemoryView(), loadCharacterConfig(), loadSaveSlots(), loadScenarioPresets(), loadTraceHistory(), loadTraceView()]);
}

async function stopInstance(id) {
  const resp = await apiFetch('/api/instances/stop', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id }),
  });
  if (!resp.ok) {
    const message = await resp.text();
    alert(`停止实例失败：${message || resp.statusText}`);
    return;
  }
  renderSceneDivider(`实例 ${id} 已停止`);
  await Promise.all([loadInstancesView(), refreshPanel()]);
}

async function deleteInstance(id) {
  if (!confirm(`删除实例“${id}”？该实例目录和实例级 SQLite 数据会被清理。`)) {
    return;
  }
  const resp = await apiFetch('/api/instances/delete', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id }),
  });
  if (!resp.ok) {
    const message = await resp.text();
    alert(`删除实例失败：${message || resp.statusText}`);
    return;
  }
  renderSceneDivider(`实例 ${id} 已删除`);
  await Promise.all([loadInstancesView(), refreshPanel()]);
}

async function createInstance() {
  const id = els.instanceCreateID.value.trim();
  if (!id) {
    alert('请先填写实例 ID');
    return;
  }
  const resp = await apiFetch('/api/instances/create', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      id,
      label: els.instanceCreateLabel.value.trim(),
      focus_character: els.instanceCreateCharacter.value.trim(),
    }),
  });
  if (!resp.ok) {
    const message = await resp.text();
    alert(`创建实例失败：${message || resp.statusText}`);
    return;
  }
  els.instanceCreateID.value = '';
  els.instanceCreateLabel.value = '';
  els.instanceCreateCharacter.value = '';
  renderSceneDivider(`实例 ${id} 已创建`);
  await loadInstancesView();
}

async function loadInstancesView() {
  try {
    const data = await fetchJSON('/api/instances');
    state.instances = data.instances || [];
    state.defaultInstanceID = data.default || '';
    const savedSelection = String(state.selectedInstanceID || localStorage.getItem('corerp-instance-id') || '').trim();
    const knownIDs = new Set(state.instances.map(instance => instance.id));
    const selected = knownIDs.has(savedSelection)
      ? savedSelection
      : (knownIDs.has(state.defaultInstanceID) ? state.defaultInstanceID : (state.instances[0]?.id || ''));
    state.selectedInstanceID = selected;
    localStorage.setItem('corerp-instance-id', selected);

    els.instanceSelect.innerHTML = '';
    state.instances.forEach(instance => {
      const opt = document.createElement('option');
      opt.value = instance.id;
      opt.textContent = `${instance.id}${instance.is_default ? ' · default' : ''}`;
      opt.selected = instance.id === selected;
      els.instanceSelect.appendChild(opt);
    });
    els.instanceSummary.textContent = `默认实例：${safeText(state.defaultInstanceID)} · 当前观察实例：${safeText(state.selectedInstanceID)} · 共 ${state.instances.length} 个`;
    renderInfoList('instance-list', state.instances.map(instance => `
      <div class="interactive-row">
        <div class="row-main">
          <div class="row-title">${instance.label || instance.id} · ${instance.id}</div>
          <div class="row-subtitle">${instance.world_name || '--'} · 视角 ${instance.focus_character || instance.active_character || '--'} · 参与者 ${((instance.participant_details || []).length ? instance.participant_details.map(item => item.name) : (instance.participants || instance.loaded_characters || [])).join(', ') || '--'}</div>
          <div class="row-subtitle">${instanceStatusLabel(instance.status)}${instance.is_default ? ' · 默认实例' : ''} · 创建于 ${instance.created_at ? new Date(instance.created_at).toLocaleString('zh-CN') : '--'}</div>
        </div>
        <div class="row-actions">
          ${instance.is_default ? '<span class="pill">默认</span>' : `<button type="button" class="ghost-button" data-instance-default="${instance.id}">设为默认</button>`}
          ${instance.id === state.selectedInstanceID ? '<span class="pill">当前实例</span>' : `<button type="button" class="ghost-button" data-instance-view="${instance.id}">查看</button>`}
          ${instance.status === 'running' ? `<button type="button" class="ghost-button" data-instance-stop="${instance.id}">停止</button>` : '<span class="tag">已停止</span>'}
          ${instance.is_default ? '' : `<button type="button" class="ghost-button danger-button" data-instance-delete="${instance.id}">删除</button>`}
        </div>
      </div>
    `), '暂无实例数据');

    els.instanceList.querySelectorAll('[data-instance-default]').forEach(node => {
      node.addEventListener('click', () => setDefaultInstance(node.dataset.instanceDefault));
    });
    els.instanceList.querySelectorAll('[data-instance-view]').forEach(node => {
      node.addEventListener('click', () => switchInstanceView(node.dataset.instanceView));
    });
    els.instanceList.querySelectorAll('[data-instance-stop]').forEach(node => {
      node.addEventListener('click', () => stopInstance(node.dataset.instanceStop));
    });
    els.instanceList.querySelectorAll('[data-instance-delete]').forEach(node => {
      node.addEventListener('click', () => deleteInstance(node.dataset.instanceDelete));
    });
  } catch (err) {
    console.error(err);
    els.instanceSummary.textContent = '默认实例：读取失败';
    renderInfoList('instance-list', [], `读取失败：${err.message}`);
  }
}

async function switchInstanceView(id) {
  const next = String(id || '').trim();
  if (!next || next === state.selectedInstanceID) {
    return;
  }
  state.selectedInstanceID = next;
  state.selectedTraceTurn = null;
  localStorage.setItem('corerp-instance-id', next);
  els.instanceSelect.value = next;
  els.chatScroll.innerHTML = '';
  state.msgCount = 0;
  renderSceneDivider(`切换到实例 ${next}`);
  await Promise.all([loadInstancesView(), loadWorlds(), loadCharacters(), loadPlayerRole(), restoreDialogue(), refreshPanel(), loadTimeline(), loadMemoryView(), loadCharacterConfig(), loadSaveSlots(), loadScenarioPresets(), loadTraceHistory(), loadTraceView()]);
}

function renderGoalsForEditor(goals) {
  return (goals || []).map(goal => {
    const knownBy = Array.isArray(goal.known_by) ? goal.known_by.join(',') : '';
    return [
      goal.type || 'primary',
      goal.id || '',
      goal.priority || 0,
      goal.condition || '',
      goal.target || '',
      knownBy,
      goal.reveal_condition || '',
      goal.cooldown_turns || 0,
    ].join('|');
  }).join('\n');
}

async function loadCharacterConfig() {
  try {
    const data = await fetchJSON(`/api/character-config?focus_character=${encodeURIComponent(els.charSelect.value || '')}`);
    const card = data.card || {};
    const identity = card.identity || {};
    els.charcfgName.value = data.focus_character || data.character || identity.name || '';
    els.charcfgPath.value = data.path || '';
    els.charcfgWorldPath.value = data.world_path || '';
    els.charcfgVoiceStyle.value = identity.voice?.style || '';
    els.charcfgVoiceRhythm.value = identity.voice?.rhythm || '';
    els.charcfgImmutable.value = (identity.immutable || []).join('\n');
    els.charcfgForbidden.value = (identity.forbidden || []).join('\n');
    els.charcfgAdaptive.value = Object.entries(identity.adaptive || {}).map(([key, value]) => `${key}=${value}`).join('\n');
    els.charcfgGoals.value = renderGoalsForEditor(card.goals || []);
    els.charcfgWriting.value = identity.writing_guide || '';
  } catch (err) {
    console.error(err);
  }
}

async function loadWorldConfig() {
  try {
    const data = await fetchJSON('/api/world-config');
    els.worldcfgName.value = data.name || '';
    els.worldcfgPath.value = data.path || '';
    els.worldcfgFormat.value = data.format || '';
    els.worldcfgRules.value = data.core_rules || '';
  } catch (err) {
    console.error(err);
    setFieldError(els.worldcfgName, '');
    setFieldError(els.worldcfgPath, '读取失败');
    setFieldError(els.worldcfgFormat, '接口不可用');
    setFieldError(els.worldcfgRules, `读取失败：${err.message}`);
  }
}

async function saveWorldConfig() {
  const resp = await apiFetch('/api/world-config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name: els.worldcfgName.value.trim(),
      core_rules: els.worldcfgRules.value.trim(),
    }),
  });
  if (!resp.ok) {
    alert('世界规则保存失败');
    return;
  }
  await Promise.all([loadWorldConfig(), refreshPanel()]);
}

function parsePipeLine(line) {
  return line.split('|').map(s => s.trim()).filter(Boolean);
}

function renderPipeList(items) {
  return (items || []).join(', ');
}

async function loadWorldStructure() {
  try {
    const data = await fetchJSON('/api/world-structure');
    els.structurePath.value = data.path || '';
    els.structurePremise.value = data.seed?.premise || '';
    els.structureSituation.value = data.seed?.current_situation || '';
    els.structureStartScene.value = data.seed?.starting_scene || '';
    els.structureTimeAnchor.value = data.seed?.time_anchor || '';
    els.structureStability.value = data.seed?.stability || '';
    els.structureFactions.value = (data.factions || []).map(f =>
      [f.id, f.name, f.role, f.description, (f.goals || []).join(','), (f.relationships || []).join(',')].join('|')
    ).join('\n');
    els.structureLocations.value = (data.locations || []).map(l =>
      [l.id, l.name, l.kind, l.description, l.controller, (l.tags || []).join(',')].join('|')
    ).join('\n');
    els.structurePressures.value = (data.pressures || []).map(p =>
      [p.id, p.name, p.kind, p.description, p.intensity, p.target, (p.escalates || []).join(',')].join('|')
    ).join('\n');
    els.structureRules.value = (data.ruleset?.rules || []).map(r =>
      [r.id, r.title, r.summary, (r.constraints || []).join(','), (r.effects || []).join(',')].join('|')
    ).join('\n');
  } catch (err) {
    console.error(err);
    setFieldError(els.structurePath, '读取失败');
  }
}

async function saveWorldStructure() {
  const factions = els.structureFactions.value.split('\n').filter(Boolean).map(line => {
    const parts = parsePipeLine(line);
    return { id: parts[0] || '', name: parts[1] || '', role: parts[2] || '', description: parts[3] || '', goals: (parts[4] || '').split(',').filter(Boolean), relationships: (parts[5] || '').split(',').filter(Boolean) };
  });
  const locations = els.structureLocations.value.split('\n').filter(Boolean).map(line => {
    const parts = parsePipeLine(line);
    return { id: parts[0] || '', name: parts[1] || '', kind: parts[2] || '', description: parts[3] || '', controller: parts[4] || '', tags: (parts[5] || '').split(',').filter(Boolean) };
  });
  const pressures = els.structurePressures.value.split('\n').filter(Boolean).map(line => {
    const parts = parsePipeLine(line);
    return { id: parts[0] || '', name: parts[1] || '', kind: parts[2] || '', description: parts[3] || '', intensity: parseFloat(parts[4]) || 0, target: parts[5] || '', escalates: (parts[6] || '').split(',').filter(Boolean) };
  });
  const rules = els.structureRules.value.split('\n').filter(Boolean).map(line => {
    const parts = parsePipeLine(line);
    return { id: parts[0] || '', title: parts[1] || '', summary: parts[2] || '', constraints: (parts[3] || '').split(',').filter(Boolean), effects: (parts[4] || '').split(',').filter(Boolean) };
  });
  const resp = await apiFetch('/api/world-structure', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      seed: {
        premise: els.structurePremise.value.trim(),
        current_situation: els.structureSituation.value.trim(),
        starting_scene: els.structureStartScene.value.trim(),
        time_anchor: els.structureTimeAnchor.value.trim(),
        stability: els.structureStability.value.trim(),
      },
      factions, locations, pressures,
      ruleset: { rules },
    }),
  });
  if (!resp.ok) {
    alert('世界结构保存失败');
    return;
  }
  await Promise.all([loadWorldStructure(), refreshPanel()]);
}

async function loadPopulationConfig() {
  try {
    const data = await fetchJSON('/api/population');
    els.popcfgPath.value = data.path || '';
    els.popcfgBackground.value = (data.background_npcs || []).map(n =>
      [n.id, n.name, n.role, n.location, n.faction, (n.traits || []).join(','), (n.hooks || []).join(',')].join('|')
    ).join('\n');
    const policy = data.policy || {};
    els.popcfgPromoteThreshold.value = policy.promote_threshold ?? 8;
    els.popcfgMajorThreshold.value = policy.major_threshold ?? 20;
    els.popcfgInteractionWeight.value = policy.interaction_weight ?? 3;
    els.popcfgMentionWeight.value = policy.mention_weight ?? 1;
    els.popcfgEventWeight.value = policy.event_weight ?? 2;
    els.popcfgRelationshipWeight.value = policy.relationship_weight ?? 4;
    els.popcfgSceneWeight.value = policy.scene_weight ?? 2;
    const promoted = data.promoted_npcs || [];
    els.popcfgPromotedList.innerHTML = promoted.length ? promoted.map(n =>
      `<div class="interactive-row"><div class="row-main"><div class="row-title">${safeText(n.name)}</div><div class="row-subtitle">${safeText(n.status)} · core ${safeText(n.identity_core)}</div></div></div>`
    ).join('') : '<div class="note-box">暂无晋升 NPC</div>';
    const cores = data.identity_cores || [];
    els.popcfgIdentityList.innerHTML = cores.length ? cores.map(c => {
      const adaptive = Object.entries(c.adaptive || {}).map(([k, v]) => `${k}:${Number(v).toFixed(1)}`).join(' / ');
      return `<div class="interactive-row"><div class="row-main"><div class="row-title">${safeText(c.name)}</div><div class="row-subtitle">${safeText(c.id)} · ${adaptive || '--'}</div></div></div>`;
    }).join('') : '<div class="note-box">暂无 identity core</div>';
  } catch (err) {
    console.error(err);
    setFieldError(els.popcfgPath, '读取失败');
  }
}

async function savePopulationConfig() {
  const background_npcs = els.popcfgBackground.value.split('\n').filter(Boolean).map(line => {
    const parts = parsePipeLine(line);
    return { id: parts[0] || '', name: parts[1] || '', role: parts[2] || '', location: parts[3] || '', faction: parts[4] || '', traits: (parts[5] || '').split(',').filter(Boolean), hooks: (parts[6] || '').split(',').filter(Boolean) };
  });
  const resp = await apiFetch('/api/population', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      background_npcs,
      policy: {
        promote_threshold: parseFloat(els.popcfgPromoteThreshold.value) || 8,
        major_threshold: parseFloat(els.popcfgMajorThreshold.value) || 20,
        interaction_weight: parseFloat(els.popcfgInteractionWeight.value) || 3,
        mention_weight: parseFloat(els.popcfgMentionWeight.value) || 1,
        event_weight: parseFloat(els.popcfgEventWeight.value) || 2,
        relationship_weight: parseFloat(els.popcfgRelationshipWeight.value) || 4,
        scene_weight: parseFloat(els.popcfgSceneWeight.value) || 2,
      },
    }),
  });
  if (!resp.ok) {
    alert('人口配置保存失败');
    return;
  }
  await Promise.all([loadPopulationConfig(), refreshPanel()]);
}

function applySceneToEditor(scene) {
  els.scenecfgPath.value = scene?.path || '';
  els.scenecfgLocation.value = scene?.scene?.location || '';
  els.scenecfgTime.value = scene?.scene?.time_of_day || '';
  els.scenecfgWeather.value = scene?.scene?.weather || '';
  els.scenecfgChars.value = (scene?.scene?.characters || []).join('\n');
  els.scenecfgDesc.value = scene?.scene?.description || '';
}

async function loadSceneConfigs() {
  try {
    const data = await fetchJSON('/api/scenes');
    state.scenes = data.scenes || [];
    const selected = data.selected || (state.scenes[0]?.name || 'default');
    els.scenecfgSelect.innerHTML = '';
    state.scenes.forEach(scene => {
      const opt = document.createElement('option');
      opt.value = scene.name || 'default';
      opt.textContent = scene.name || 'default';
      opt.selected = opt.value === selected;
      els.scenecfgSelect.appendChild(opt);
    });
    applySceneToEditor(state.scenes.find(scene => scene.name === selected) || state.scenes[0]);
  } catch (err) {
    console.error(err);
    els.scenecfgSelect.innerHTML = '<option value="">读取失败</option>';
    setFieldError(els.scenecfgPath, '接口不可用');
    setFieldError(els.scenecfgLocation, '');
    setFieldError(els.scenecfgTime, '');
    setFieldError(els.scenecfgWeather, '');
    setFieldError(els.scenecfgChars, '');
    setFieldError(els.scenecfgDesc, `读取失败：${err.message}`);
  }
}

async function saveSceneConfig() {
  const name = (els.scenecfgSelect.value || 'default').trim() || 'default';
  const resp = await apiFetch('/api/scenes', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name,
      scene: {
        location: els.scenecfgLocation.value.trim(),
        time_of_day: els.scenecfgTime.value.trim(),
        weather: els.scenecfgWeather.value.trim(),
        characters: parseLineList(els.scenecfgChars.value),
        description: els.scenecfgDesc.value.trim(),
      },
    }),
  });
  if (!resp.ok) {
    alert('场景保存失败');
    return;
  }
  await Promise.all([loadSceneConfigs(), refreshPanel()]);
}

async function loadCanonFacts() {
  try {
    const data = await fetchJSON('/api/canon-facts');
    els.factsPath.value = data.path || '';
    els.factsEditor.value = renderFactsForEditor(data.facts || []);
    if (!data.path) {
      els.factsPath.value = '当前世界没有独立 facts 文件';
    }
    if (!data.facts || data.facts.length === 0) {
      setFactsEditorMessage('# 暂无 canonical facts\n# 如果当前世界是旧的单文件格式，这是正常现象。');
    }
  } catch (err) {
    console.error(err);
    setFieldError(els.factsPath, '接口不可用');
    setFactsEditorMessage(`# 读取失败：${err.message}\n# 如果你刚改完代码但没重编译/重启服务，这里通常会失败。`);
  }
}

async function reviewQuarantine(id, action) {
  const resp = await apiFetch(`/api/quarantine/${action}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id }),
  });
  if (!resp.ok) {
    alert(`quarantine ${action} 失败`);
    return;
  }
  await Promise.all([loadQuarantineView(), refreshPanel()]);
}

async function reviewPendingFact(id, action) {
  const resp = await apiFetch(`/api/pending-facts/${action}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id }),
  });
  if (!resp.ok) {
    alert(`pending fact ${action} 失败`);
    return;
  }
  await Promise.all([loadPendingFactsView(), loadMemoryView(), refreshPanel()]);
}

async function reviewAll(items, getID, actionFn) {
  for (const item of items) {
    // Sequential on purpose to keep server writes predictable.
    await actionFn(getID(item));
  }
}

async function loadQuarantineView() {
  try {
    const character = encodeURIComponent(els.charSelect.value || '');
    const data = await fetchJSON(`/api/quarantine?focus_character=${character}&n=12`);
    state.quarantineEvents = data.events || [];
    const filter = (els.quarantineFilter.value || '').trim();
    const events = state.quarantineEvents.filter(event => {
      if (!filter) {
        return true;
      }
      const hay = `${event.type || ''} ${event.actor || ''}`.toLowerCase();
      return hay.includes(filter.toLowerCase());
    });
    renderInfoList('quarantine-list', events.map(event => `
      <div class="interactive-row">
        <div class="row-main">
          <div class="row-title">${event.type || 'event'} · ${event.actor || '--'}</div>
          <div class="row-subtitle">${truncate(JSON.stringify(event.payload || {}), 90)}</div>
          <div class="row-subtitle">置信度 ${Number(event.confidence || 0).toFixed(2)} · 确认 ${event.confirmations || 0}</div>
        </div>
        <div class="row-actions">
          <button type="button" class="ghost-button" data-quarantine-action="promote" data-quarantine-id="${event.id}">提升</button>
          <button type="button" class="ghost-button" data-quarantine-action="reject" data-quarantine-id="${event.id}">拒绝</button>
        </div>
      </div>
    `), '暂无 quarantined events');
    $('quarantine-list').querySelectorAll('[data-quarantine-action]').forEach(node => {
      node.addEventListener('click', () => reviewQuarantine(node.dataset.quarantineId, node.dataset.quarantineAction));
    });
  } catch (err) {
    console.error(err);
    renderInfoList('quarantine-list', [], `读取失败：${err.message}`);
  }
}

async function loadPendingFactsView() {
  try {
    const character = encodeURIComponent(els.charSelect.value || '');
    const data = await fetchJSON(`/api/pending-facts?focus_character=${character}&n=12`);
    state.pendingFacts = data.facts || [];
    const filter = (els.pendingFilter.value || '').trim();
    const facts = state.pendingFacts.filter(fact => {
      if (!filter) {
        return true;
      }
      const hay = `${fact.subject || ''} ${fact.predicate || ''} ${fact.object || ''}`.toLowerCase();
      return hay.includes(filter.toLowerCase());
    });
    $('pending-total').textContent = String(data.stats?.pending_total || state.pendingFacts.length || 0);
    $('pending-character').textContent = safeText(els.charSelect.value || '--');
    renderInfoList('pending-facts-list', facts.map(fact => `
      <div class="interactive-row">
        <div class="row-main">
          <div class="row-title">${fact.subject} · ${fact.predicate}</div>
          <div class="row-subtitle">${truncate(fact.object || '', 90)}</div>
          <div class="row-subtitle">${fact.source || '--'} · 置信度 ${Number(fact.confidence || 0).toFixed(2)} · 确认 ${fact.confirmations || 0}</div>
        </div>
        <div class="row-actions">
          <button type="button" class="ghost-button" data-pending-action="confirm" data-pending-id="${fact.id}">确认</button>
          <button type="button" class="ghost-button" data-pending-action="promote" data-pending-id="${fact.id}">提升</button>
          <button type="button" class="ghost-button" data-pending-action="delete" data-pending-id="${fact.id}">删除</button>
        </div>
      </div>
    `), '暂无 pending facts');
    $('pending-facts-list').querySelectorAll('[data-pending-action]').forEach(node => {
      node.addEventListener('click', () => reviewPendingFact(node.dataset.pendingId, node.dataset.pendingAction));
    });
  } catch (err) {
    console.error(err);
    $('pending-total').textContent = '0';
    $('pending-character').textContent = safeText(els.charSelect.value || '--');
    renderInfoList('pending-facts-list', [], `读取失败：${err.message}`);
  }
}

async function loadTraceHistory() {
  try {
    const data = await fetchJSON('/api/traces?limit=20').catch(() => ({ traces: [] }));
    const traces = data.traces || [];
    state.traceHistoryItems = traces;
    if (!state.selectedTraceTurn && traces.length) {
      state.selectedTraceTurn = traces[0].turn || null;
    }
    if (state.selectedTraceTurn && !traces.some(trace => Number(trace.turn || 0) === Number(state.selectedTraceTurn))) {
      state.selectedTraceTurn = traces[0]?.turn || null;
    }
    renderInfoList('trace-history', traces.map(trace => `
      <div class="interactive-row ${Number(state.selectedTraceTurn) === Number(trace.turn) ? 'trace-selected' : ''}">
        <div class="row-main">
          <div class="row-title">turn ${trace.turn || 0} · ${safeText(trace.focus_character || trace.character)}</div>
          <div class="row-subtitle">${safeText(trace.user_input, '--')}</div>
          <div class="row-subtitle">${trace.created_at ? new Date(trace.created_at).toLocaleString('zh-CN') : '--'}</div>
        </div>
        <div class="row-actions">
          <button type="button" class="ghost-button" data-trace-turn="${trace.turn || 0}">${Number(state.selectedTraceTurn) === Number(trace.turn) ? '当前' : '查看'}</button>
        </div>
      </div>
    `), '暂无 turn 历史');
    els.traceHistory.querySelectorAll('[data-trace-turn]').forEach(node => {
      node.addEventListener('click', async () => selectTraceTurn(node.dataset.traceTurn));
    });
    updateTraceControls();
    await loadSaveSlots(false);
  } catch (err) {
    console.error(err);
    state.traceHistoryItems = [];
    updateTraceControls();
    renderInfoList('trace-history', [], `读取失败：${err.message}`);
  }
}

async function loadTraceView(turn = state.selectedTraceTurn) {
  try {
    const trace = await fetchTraceJSON(turn);
    if (!trace) {
      updateTraceControls();
      renderInfoList('trace-panel', [], '暂无 trace，请先进行一轮对话');
      return;
    }
    state.selectedTraceTurn = trace.turn || null;
    updateTraceControls();
    const items = [];
    items.push(`
      <div class="note-box mono">turn ${trace.turn || 0} · ${safeText(trace.focus_character || trace.character)} · input: ${safeText(trace.user_input, '--')}</div>
    `);
    if (Array.isArray(trace.participant_details) && trace.participant_details.length) {
      const parts = trace.participant_details.map(p => {
        const tags = [];
        if (p.kind) tags.push(participantKindLabel(p.kind));
        if (p.source) tags.push(participantSourceLabel(p.source));
        if (p.loaded === false) tags.push('未加载');
        if (p.present) tags.push('在场');
        if (p.focus) tags.push('当前视角');
        if (p.switchable === false) tags.push('不可切换');
        return `${safeText(p.name)}${tags.length ? '[' + tags.join('·') + ']' : ''}`;
      }).join(' · ');
      items.push(`<div class="note-box mono">participants: ${parts}</div>`);
    }
    if (trace.director_plan?.selected?.length) {
      items.push(`
        <div class="note-box mono">director: ${trace.director_plan.mode || 'manual'} -> ${trace.director_plan.selected.join(' -> ')} (${safeText(trace.director_plan.reason, '--')})</div>
      `);
    }
    if (Array.isArray(trace.director_plan?.candidate_details) && trace.director_plan.candidate_details.length) {
      const selectedCandidates = trace.director_plan.candidate_details.filter(candidate => candidate.selected);
      const winner = selectedCandidates[0] || null;
      const candidateMeta = (c) => {
        const tags = [];
        if (c.kind) tags.push(participantKindLabel(c.kind));
        if (c.source) tags.push(participantSourceLabel(c.source));
        if (c.loaded === false) tags.push('未加载');
        if (c.present) tags.push('在场');
        if (c.switchable === false) tags.push('不可切换');
        return tags.length ? '[' + tags.join('·') + ']' : '';
      };
      const selected = selectedCandidates.map(candidate => `${safeText(candidate.name)}:${Number(candidate.score || 0).toFixed(1)}${candidate.location_match ? '[scene]' : ''}${candidate.faction_match ? '[faction]' : ''}${candidate.pressure_match ? '[pressure]' : ''}${candidate.hook_match ? '[hook]' : ''}${candidateMeta(candidate)}`).join(' · ');
      const alternates = trace.director_plan.candidate_details
        .filter(candidate => !candidate.selected)
        .slice(0, 4)
        .map(candidate => `${safeText(candidate.name)}:${Number(candidate.score || 0).toFixed(1)}${candidate.location_match ? '[scene]' : ''}${candidate.faction_match ? '[faction]' : ''}${candidate.pressure_match ? '[pressure]' : ''}${candidate.hook_match ? '[hook]' : ''}${candidateMeta(candidate)}(${describeCandidateGap(winner, candidate)})`)
        .join(' · ');
      items.push(`
        <div class="note-box mono">selected: ${selected || '--'}</div>
      `);
      if (alternates) {
        items.push(`
          <div class="note-box mono">alternates: ${alternates}</div>
        `);
      }
      const candidateNames = new Set(trace.director_plan.candidate_details.map(c => c.name));
      const excluded = (trace.participant_details || []).filter(p => p.switchable !== false && !candidateNames.has(p.name));
      if (excluded.length) {
        const reasons = excluded.map(p => {
          let reason = '';
          if (p.source === 'scene_shell') reason = 'scene_shell 不参与导演选角';
          else if (p.source === 'player_role') reason = '玩家身份不参与导演选角';
          else if (p.kind === 'npc' && p.source === 'background_population') reason = '背景人口默认不进入候选（需晋升）';
          else reason = '导演评分未达标';
          return `${safeText(p.name)}[${participantKindLabel(p.kind)}·${participantSourceLabel(p.source)}] — ${reason}`;
        }).join(' · ');
        items.push(`<div class="note-box mono">excluded: ${reasons}</div>`);
      }
    }
    if (Array.isArray(trace.director_plan?.steps) && trace.director_plan.steps.length) {
      items.push(`
        <div class="note-box mono">steps: ${trace.director_plan.steps.map(step => `${step.index + 1}.${step.speaker}[${step.kind || 'lead'}|${step.budget_mode || 'normal'}]`).join(' -> ')}</div>
      `);
    }
    if (Array.isArray(trace.step_traces) && trace.step_traces.length) {
      items.push(...trace.step_traces.map(renderTraceStep));
    } else if (Array.isArray(trace.active_goals) && trace.active_goals.length) {
      items.push(...trace.active_goals.slice(0, 6).map(goal => `
        <div class="note-box mono">goal.${goal.type}: ${goal.id} p${goal.priority} ${safeText(goal.condition, '')}</div>
      `));
      if (Array.isArray(trace.memories) && trace.memories.length) {
        items.push(...trace.memories.slice(0, 6).map(memory => `
          <div class="note-box mono">memory.${memory.type}: ${truncate(memory.content || '', 100)} [${Number(memory.score || 0).toFixed(2)}]</div>
        `));
      }
      if (trace.validator?.blocked) {
        items.push(`<div class="note-box mono">validator blocked: ${safeText(trace.validator.reason, '--')}</div>`);
      }
      if (trace.action_frame?.action) {
        items.push(`<div class="note-box mono">action: ${trace.action_frame.action} -> ${safeText(trace.action_frame.target, '--')} · ${safeText(trace.action_frame.intent, '--')}</div>`);
      }
      if (trace.narrative) {
        items.push(`<div class="note-box mono">narrative: ${truncate(trace.narrative, 140)}</div>`);
      }
    }
    renderInfoList('trace-panel', items, '暂无 trace');
  } catch (err) {
    console.error(err);
    if (String(err.message || '').startsWith('404 ')) {
      updateTraceControls();
      renderInfoList('trace-panel', [], '暂无 trace，请先进行一轮对话');
      return;
    }
    updateTraceControls();
    renderInfoList('trace-panel', [], `读取失败：${err.message}`);
  }
}

async function saveCanonFacts() {
  const resp = await apiFetch('/api/canon-facts', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ facts: parseFacts(els.factsEditor.value) }),
  });
  if (!resp.ok) {
    alert('Canonical facts 保存失败');
    return;
  }
  await Promise.all([loadCanonFacts(), loadMemoryView(), refreshPanel()]);
}

async function saveCharacterConfig() {
  const current = els.charSelect.value || '';
  const body = {
    focus_character: current,
    card: {
      identity: {
        name: current,
        immutable: parseLineList(els.charcfgImmutable.value),
        adaptive: parseAdaptiveLines(els.charcfgAdaptive.value),
        forbidden: parseLineList(els.charcfgForbidden.value),
        voice: {
          style: els.charcfgVoiceStyle.value.trim(),
          rhythm: els.charcfgVoiceRhythm.value.trim(),
        },
        writing_guide: els.charcfgWriting.value.trim(),
      },
      goals: parseGoals(els.charcfgGoals.value),
    },
  };

  const resp = await apiFetch('/api/character-config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!resp.ok) {
    const message = (await resp.text()) || '人物定义保存失败';
    alert(`人物定义保存失败：${message}`);
    return;
  }
  await Promise.all([loadCharacterConfig(), refreshPanel()]);
}

async function loadSaveSlots(refreshSelectors = true) {
  try {
    const data = await fetchJSON('/api/checkpoints').catch(() => ({ checkpoints: [] }));
    const saves = data.checkpoints || [];
    state.saves = saves;
    renderInfoList('save-list', saves.map(slot => `
      <div class="interactive-row">
        <div class="row-main">
          <div class="row-title">${slot.name}</div>
          <div class="row-subtitle">${safeText(slot.note, slot.preview || '--')}</div>
          <div class="row-subtitle">${safeText(slot.focus_character || slot.character)} · ${slot.branch} · ${new Date(slot.created_at).toLocaleString('zh-CN')}</div>
        </div>
        <div class="row-actions">
          ${resolveCheckpointTraceTurn(slot) ? `<button type="button" class="ghost-button" data-trace-from-save="${slot.name}">依据</button>` : ''}
          <button type="button" class="ghost-button" data-load-save="${slot.name}">回滚</button>
        </div>
      </div>
    `), '暂无 checkpoint');
    $('save-list').querySelectorAll('[data-load-save]').forEach(node => {
      node.addEventListener('click', () => loadSaveSlot(node.dataset.loadSave));
    });
    $('save-list').querySelectorAll('[data-trace-from-save]').forEach(node => {
      node.addEventListener('click', async () => {
        const slot = state.saves.find(item => item.name === node.dataset.traceFromSave);
        const turn = resolveCheckpointTraceTurn(slot);
        if (!turn) {
          alert('当前 checkpoint 尚未匹配到 trace');
          return;
        }
        await selectTraceTurn(turn);
      });
    });
    if (refreshSelectors) {
      refreshDiffSelectors();
    }
  } catch (err) {
    console.error(err);
  }
}

async function createSaveSlot() {
  const name = els.saveName.value.trim();
  if (!name) {
    alert('请输入 checkpoint 名');
    return;
  }
  const resp = await apiFetch('/api/checkpoints', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name,
      branch: els.timelineBranch.value || 'main',
      note: els.saveNote.value.trim(),
    }),
  });
  if (!resp.ok) {
    alert('checkpoint 保存失败');
    return;
  }
  els.saveName.value = '';
  els.saveNote.value = '';
  await loadSaveSlots();
}

async function loadSaveSlot(name) {
  if (!name) {
    return;
  }
  const resp = await apiFetch('/api/checkpoints/load', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  if (!resp.ok) {
    alert('rollback 失败');
    return;
  }
  const slot = await resp.json();
  state.selectedTraceTurn = null;
  renderSceneDivider(`已回滚到 checkpoint ${slot.name}`);
  await Promise.all([loadWorlds(), loadCharacters(), loadPlayerRole(), restoreDialogue(), refreshPanel(), loadTimeline(slot.branch), loadMemoryView(), loadCharacterConfig(), loadSaveSlots(), loadTraceHistory(), loadTraceView()]);
}

async function loadScenarioPresets() {
  try {
    const data = await fetchJSON('/api/presets').catch(() => ({ presets: [] }));
    const presets = data.presets || [];
    state.presets = presets;
    const currentWorldPath = String(els.worldSelect?.value || '');
    const isNeonBlock = /(^|\/)neon_block$/.test(currentWorldPath);
    const recommendedPreset = isNeonBlock
      ? presets.find(preset => preset.name === 'opening_witness_conflict')
      : null;
    const recommendedItem = recommendedPreset ? [`
      <div class="interactive-row">
        <div class="row-main">
          <div class="row-title">推荐开场 · ${recommendedPreset.name}</div>
          <div class="row-subtitle">${safeText(recommendedPreset.note, recommendedPreset.preview || '--')}</div>
          <div class="row-subtitle">适用于当前 world：霓虹里街区 · 直接进入蓝姐 / 谭叔冲突现场</div>
        </div>
        <div class="row-actions">
          <button type="button" class="ghost-button" data-apply-preset="${recommendedPreset.name}">立即套用</button>
        </div>
      </div>
    `] : [];
    const presetItems = presets
      .filter(preset => !recommendedPreset || preset.name !== recommendedPreset.name)
      .map(preset => `
      <div class="interactive-row">
        <div class="row-main">
          <div class="row-title">${preset.name}</div>
          <div class="row-subtitle">${safeText(preset.note, preset.preview || '--')}</div>
          <div class="row-subtitle">${safeText(preset.focus_character || preset.character)} · ${safeText(preset.branch, 'main')} · ${new Date(preset.created_at).toLocaleString('zh-CN')}</div>
        </div>
        <div class="row-actions">
          <button type="button" class="ghost-button" data-apply-preset="${preset.name}">套用</button>
        </div>
      </div>
    `);
    renderInfoList('preset-list', [...recommendedItem, ...presetItems], '暂无 scenario preset');
    $('preset-list').querySelectorAll('[data-apply-preset]').forEach(node => {
      node.addEventListener('click', () => applyScenarioPreset(node.dataset.applyPreset));
    });
  } catch (err) {
    console.error(err);
  }
}

async function createScenarioPreset() {
  const name = els.presetName.value.trim();
  if (!name) {
    alert('请输入 preset 名');
    return;
  }
  const resp = await apiFetch('/api/presets', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name,
      branch: els.timelineBranch.value || 'main',
      note: els.presetNote.value.trim(),
    }),
  });
  if (!resp.ok) {
    alert('scenario preset 保存失败');
    return;
  }
  els.presetName.value = '';
  els.presetNote.value = '';
  await loadScenarioPresets();
}

async function applyScenarioPreset(name) {
  if (!name) {
    return;
  }
  const resp = await apiFetch('/api/presets/apply', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  if (!resp.ok) {
    alert('scenario preset 套用失败');
    return;
  }
  const preset = await resp.json();
  state.selectedTraceTurn = null;
  renderSceneDivider(`已套用 preset ${preset.name}`);
  await Promise.all([loadWorlds(), loadCharacters(), loadPlayerRole(), restoreDialogue(), refreshPanel(), loadTimeline(preset.branch), loadMemoryView(), loadCharacterConfig(), loadScenarioPresets(), loadTraceHistory(), loadTraceView()]);
}

function exportSession(format) {
  const limit = Number(els.msgLimitSlider.value || 30);
  window.open(buildAPIURL(`/api/export?format=${encodeURIComponent(format)}&limit=${limit}`), '_blank');
}

function refreshDiffSelectors() {
  const branchA = els.branchDiffA.value;
  const branchB = els.branchDiffB.value;
  els.branchDiffA.innerHTML = '';
  els.branchDiffB.innerHTML = '';
  state.branches.forEach(branch => {
    const a = document.createElement('option');
    a.value = branch;
    a.textContent = branch;
    a.selected = branch === branchA || (!branchA && branch === state.branches[0]);
    els.branchDiffA.appendChild(a);
    const b = document.createElement('option');
    b.value = branch;
    b.textContent = branch;
    b.selected = branch === branchB || (!branchB && branch === state.branches[1]);
    els.branchDiffB.appendChild(b);
  });

  const saveA = els.saveDiffA.value;
  const saveB = els.saveDiffB.value;
  els.saveDiffA.innerHTML = '';
  els.saveDiffB.innerHTML = '';
  state.saves.forEach(slot => {
    const a = document.createElement('option');
    a.value = slot.name;
    a.textContent = slot.name;
    a.selected = slot.name === saveA || (!saveA && slot.name === state.saves[0]?.name);
    els.saveDiffA.appendChild(a);
    const b = document.createElement('option');
    b.value = slot.name;
    b.textContent = slot.name;
    b.selected = slot.name === saveB || (!saveB && slot.name === state.saves[1]?.name);
    els.saveDiffB.appendChild(b);
  });
}

async function loadWorlds() {
  if (!els.worldSelect) {
    return;
  }
  const data = await fetchJSON('/api/worlds');
  const worlds = data.worlds || [];
  const active = data.active || '';
  const activePath = data.active_path || '';
  state.worlds = worlds;
  state.activeWorldPath = activePath;

  els.worldSelect.innerHTML = '';
  worlds.forEach(world => {
    const opt = document.createElement('option');
    opt.value = world.path || world.id || world.name;
    const label = world.loaded_character
      ? `${world.name || world.id || world.path} · ${world.loaded_character}`
      : (world.name || world.id || world.path);
    opt.textContent = label;
    opt.title = [
      world.path,
      `${world.character_count || 0} 视角种子`,
      `${world.location_count || 0} 地点`,
      `${world.event_count || 0} 事件`,
      `${world.background_npc_count || 0} 背景NPC`,
      `${world.promoted_npc_count || 0} 晋升人物`
    ].filter(Boolean).join(' · ');
    opt.selected = activePath ? normalizePath(world.path) === normalizePath(activePath) : world.name === active;
    els.worldSelect.appendChild(opt);
  });
  els.worldSelect.disabled = worlds.length === 0;
  if (worlds.length === 0) {
    const opt = document.createElement('option');
    opt.textContent = '未发现世界';
    els.worldSelect.appendChild(opt);
  }
  updateWorldConvertButton();
}

async function enterWorld(path) {
  if (!path) {
    return;
  }
  if (normalizePath(path) === normalizePath(state.activeWorldPath || '')) {
    return;
  }
  const resp = await apiFetch('/api/worlds', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  });
  if (!resp.ok) {
    const message = await resp.text();
    alert(`进入 world 失败：${message || resp.status}`);
    await loadWorlds();
    return;
  }
  const data = await resp.json();
  els.chatScroll.innerHTML = '';
  state.msgCount = 0;
  renderSceneDivider(`进入 ${safeText(data.world, path)} · 视角 ${safeText(data.focus_character || data.character, '--')}`);
  await Promise.all([
    loadWorlds(),
    loadCharacters(),
    loadPlayerRole(),
    restoreDialogue(),
    refreshPanel(),
    loadMemoryView(),
    loadCharacterConfig(),
    loadScenarioPresets(),
    loadTraceHistory(),
    loadTraceView(),
  ]);
}

function showWorldCreateModal() {
  els.worldCreateName.value = '';
  els.worldCreateRules.value = '';
  els.worldCreateModal.style.display = 'flex';
  els.worldCreateName.focus();
}

function hideWorldCreateModal() {
  els.worldCreateModal.style.display = 'none';
}

async function createWorld() {
  const name = els.worldCreateName.value.trim();
  if (!name) {
    alert('世界名称不能为空');
    return;
  }
  const resp = await apiFetch('/api/worlds', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, core_rules: els.worldCreateRules.value.trim() }),
  });
  if (!resp.ok) {
    const message = await resp.text();
    alert(`创建世界失败：${message || resp.status}`);
    return;
  }
  const data = await resp.json();
  hideWorldCreateModal();
  await loadWorlds();
  if (data.path) {
    await enterWorld(data.path);
  }
}

async function convertWorld() {
  const path = els.worldSelect.value;
  if (!path) return;
  if (!confirm(`将单文件世界转换为目录格式？\n\n原文件会被备份为 .bak 后缀。`)) return;
  const resp = await apiFetch('/api/worlds', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  });
  if (!resp.ok) {
    const message = await resp.text();
    alert(`转换失败：${message || resp.status}`);
    return;
  }
  const data = await resp.json();
  alert(`转换成功：${data.new_path}`);
  await loadWorlds();
  if (data.new_path) {
    await enterWorld(data.new_path);
  }
}

function updateWorldConvertButton() {
  const selected = state.worlds.find(w => normalizePath(w.path) === normalizePath(els.worldSelect.value || ''));
  const isSingleFile = selected && selected.format === 'single_file';
  if (els.worldConvertBtn) {
    els.worldConvertBtn.style.display = isSingleFile ? '' : 'none';
  }
}

function normalizePath(path) {
  return String(path || '').replace(/\\/g, '/').replace(/\/+$/, '');
}

function renderDiffObject(diff) {
  const lines = [];
  if (diff.scene) {
    Object.entries(diff.scene).forEach(([key, value]) => lines.push(`scene.${key}: ${JSON.stringify(value.a)} -> ${JSON.stringify(value.b)}`));
  }
  if (diff.clock) {
    lines.push(`clock: ${JSON.stringify(diff.clock.a)} -> ${JSON.stringify(diff.clock.b)}`);
  }
  if (diff.tension) {
    lines.push(`tension: ${JSON.stringify(diff.tension.a)} -> ${JSON.stringify(diff.tension.b)}`);
  }
  if (diff.flags) {
    Object.entries(diff.flags).forEach(([key, value]) => lines.push(`flag.${key}: ${JSON.stringify(value.a)} -> ${JSON.stringify(value.b)}`));
  }
  if (diff.variables) {
    Object.entries(diff.variables).forEach(([key, value]) => lines.push(`var.${key}: ${JSON.stringify(value.a)} -> ${JSON.stringify(value.b)}`));
  }
  if (diff.relationships) {
    Object.entries(diff.relationships).forEach(([key, value]) => lines.push(`rel.${key}: ${JSON.stringify(value.a)} -> ${JSON.stringify(value.b)}`));
  }
  return lines;
}

async function runBranchDiff() {
  const a = els.branchDiffA.value;
  const b = els.branchDiffB.value;
  if (!a || !b) {
    alert('请选择两个分支');
    return;
  }
  const diff = await fetchJSON(`/api/branches/diff?a=${encodeURIComponent(a)}&b=${encodeURIComponent(b)}`);
  const lines = renderDiffObject(diff);
  renderInfoList('diff-results', lines.map(line => `<div class="note-box mono">${line}</div>`), '两个分支当前没有差异');
}

async function mergeBranchDiff() {
  const source = els.branchDiffB.value;
  const target = els.branchDiffA.value;
  if (!source || !target) {
    alert('请选择源分支和目标分支');
    return;
  }
  const resp = await apiFetch('/api/branches/merge', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ source, target, merge_flags: true, merge_variables: true }),
  });
  if (!resp.ok) {
    alert('分支合并失败');
    return;
  }
  const result = await resp.json();
  renderInfoList('diff-results', [
    `<div class="note-box mono">merge ${result.source_branch} -> ${result.target_branch}: flags ${result.flags_merged}, vars ${result.variables_merged}, events ${result.events_appended}</div>`,
  ], '无结果');
}

async function runSaveDiff() {
  const a = els.saveDiffA.value;
  const b = els.saveDiffB.value;
  if (!a || !b) {
    alert('请选择两个存档');
    return;
  }
  const diff = await fetchJSON(`/api/saves/diff?a=${encodeURIComponent(a)}&b=${encodeURIComponent(b)}`);
  const lines = renderDiffObject(diff);
  renderInfoList('diff-results', lines.map(line => `<div class="note-box mono">${line}</div>`), '两个存档当前没有差异');
}

async function refreshPanel() {
  try {
    const [stateData, debugData, character, actionStats, actionLog, populationInsights, world, branches, compression, usage, simStatus] = await Promise.all([
      fetchJSON('/api/state'),
      fetchJSON('/api/debug/memory'),
      fetchJSON('/api/character').catch(() => ({})),
      fetchJSON('/api/npc-action-log?stats=1').catch(() => ({})),
      fetchJSON('/api/npc-action-log?n=8').catch(() => ({ entries: [] })),
      fetchJSON('/api/population-insights').catch(err => ({ __error: true, error: err.message, promoted: [], background: [] })),
      fetchJSON('/api/world').catch(() => ({})),
      fetchJSON('/api/branches').catch(() => ({ branches: [] })),
      fetchJSON('/api/compression-stats').catch(() => ({})),
      fetchJSON('/api/usage').catch(() => ({})),
      fetchJSON('/api/sim/status').catch(() => ({})),
    ]);

    updateScenePanel(stateData, debugData);
    updateTension(stateData);
    updateClock(stateData);
    updateCharacterCard(character);
    updateMemoryPanel(debugData);
    updateNPCPanels(debugData, actionStats, actionLog);
    updatePopulationRuntimePanel(populationInsights);
    updateWorldPanel(world, branches, compression);
    updateUsagePanel(usage);
    state.branches = branches.branches || [];
    refreshDiffSelectors();
    updateDirectorPanel(stateData);
    if (simStatus && simStatus.tick_count != null) {
      els.simStatus.textContent = simStatus.paused ? '已暂停' : simStatus.running ? '运行中' : '未启动';
      els.simStatus.style.color = simStatus.paused ? 'var(--warning)' : simStatus.running ? 'var(--success)' : 'var(--text-muted)';
      els.simTickCount.textContent = String(simStatus.tick_count ?? 0);
      els.simWorldAdvance.textContent = simStatus.world_advance ?? '0s';
      els.simTurnCount.textContent = String(simStatus.turn_count ?? 0);
      els.simPauseBtn.style.display = simStatus.paused ? 'none' : '';
      els.simResumeBtn.style.display = simStatus.paused ? '' : 'none';
      renderSimEvolution(simStatus);
    }
    await Promise.all([loadInstancesView(), loadMemoryView(), loadCharacterConfig(), loadSaveSlots(), loadScenarioPresets(), loadWorldConfig(), loadSceneConfigs(), loadCanonFacts(), loadQuarantineView(), loadPendingFactsView(), loadDirectorConfig(), loadTraceHistory(), loadTraceView(), loadWorldStructure(), loadPopulationConfig()]);
  } catch (err) {
    console.error(err);
  }
}

function togglePricingPopup() {
  els.pricingPopup.style.display = els.pricingPopup.style.display === 'grid' ? 'none' : 'grid';
}

function toggleConfigForm(show) {
  els.cfgForm.style.display = show ? 'grid' : 'none';
}

function clearConfigForm() {
  state.editingConfigName = null;
  els.cfgName.value = '';
  els.cfgEndpoint.value = '';
  els.cfgKey.value = '';
  els.cfgModel.value = '';
  els.cfgModelSelect.style.display = 'none';
  els.cfgModelSelect.innerHTML = '';
}

async function editConfig(name) {
  if (!name) {
    return;
  }
  try {
    const cfg = await fetchJSON(`/api/llm-configs/${encodeURIComponent(name)}`);
    state.editingConfigName = name;
    els.cfgName.value = cfg.name || '';
    els.cfgEndpoint.value = cfg.endpoint || '';
    els.cfgKey.value = '';
    els.cfgModel.value = cfg.model || '';
    els.cfgModelSelect.style.display = 'none';
    els.cfgModelSelect.innerHTML = '';
    toggleConfigForm(true);
  } catch (err) {
    alert('读取配置失败');
  }
}

async function fetchModelsForDraftConfig() {
  const endpoint = els.cfgEndpoint.value.trim();
  const apiKey = els.cfgKey.value.trim();
  if (!endpoint) {
    alert('请先填写 API 地址');
    return;
  }
  const tempName = '_fetch_tmp';
  await fetch('/api/llm-configs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: tempName, endpoint, api_key: apiKey, model: '' }),
  });
  try {
    const result = await fetchJSON(`/api/llm-models?config=${encodeURIComponent(tempName)}`);
    const models = result.models || [];
    els.cfgModelSelect.innerHTML = '';
    models.forEach(model => {
      const option = document.createElement('option');
      option.value = model;
      option.textContent = model;
      els.cfgModelSelect.appendChild(option);
    });
    els.cfgModelSelect.style.display = models.length ? 'block' : 'none';
    els.cfgModelSelect.onchange = () => {
      els.cfgModel.value = els.cfgModelSelect.value;
    };
    if (models[0]) {
      els.cfgModel.value = models[0];
    }
  } catch (err) {
    alert('拉取模型列表失败');
  } finally {
    fetch(`/api/llm-configs/${encodeURIComponent(tempName)}`, { method: 'DELETE' });
  }
}

async function saveConfig() {
  const cfg = {
    name: els.cfgName.value.trim(),
    endpoint: els.cfgEndpoint.value.trim(),
    api_key: els.cfgKey.value.trim(),
    model: els.cfgModel.value.trim(),
  };
  if (!cfg.name || !cfg.endpoint) {
    alert('至少填写配置名称和 API 地址');
    return;
  }
  const isEditing = !!state.editingConfigName;
  const url = isEditing
    ? `/api/llm-configs/${encodeURIComponent(state.editingConfigName)}`
    : '/api/llm-configs';
  const resp = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(cfg),
  });
  if (!resp.ok) {
    alert(isEditing ? '更新配置失败' : '保存配置失败');
    return;
  }
  clearConfigForm();
  toggleConfigForm(false);
  await loadLLMConfigs();
}

async function loadLLMConfigs() {
  const [configs, active] = await Promise.all([
    fetchJSON('/api/llm-configs').catch(() => []),
    fetchJSON('/api/llm-active').catch(() => ({})),
  ]);

  const activeName = active.name || '';
  const parts = [];

  if (activeName) {
    parts.push(`
      <div class="interactive-row">
        <div class="row-main">
          <div class="row-title">${activeName}</div>
          <div class="row-subtitle">${safeText(active.model, '未指定模型')} · ${safeText(active.endpoint, '--')}</div>
          <div class="row-subtitle">价格：输入 ¥${active.prompt_price || 1.0} / 输出 ¥${active.completion_price || 4.0}</div>
        </div>
        <div class="row-actions">
          <button type="button" class="ghost-button" data-price="${activeName}">价格</button>
          <span class="pill">当前使用</span>
        </div>
      </div>
    `);
  }

  const savedConfigs = configs.filter(cfg => cfg.name !== activeName);
  savedConfigs.forEach(cfg => {
    const invalid = !cfg.endpoint || !cfg.model;
    parts.push(`
      <div class="interactive-row">
        <div class="row-main">
          <div class="row-title">${cfg.name}</div>
          <div class="row-subtitle">${safeText(cfg.model, '未指定模型')} · ${safeText(cfg.endpoint, '--')}</div>
          <div class="row-subtitle">价格：输入 ¥${cfg.prompt_price || 1.0} / 输出 ¥${cfg.completion_price || 4.0}</div>
          ${invalid ? '<div class="row-subtitle" style="color:var(--danger)">配置不完整，请先编辑补全后再启用</div>' : ''}
        </div>
        <div class="row-actions">
          <button type="button" class="ghost-button" data-price="${cfg.name}">价格</button>
          <button type="button" class="ghost-button" data-edit="${cfg.name}">编辑</button>
          <button type="button" class="ghost-button" data-use="${cfg.name}" ${invalid ? 'disabled style="opacity:.45;cursor:not-allowed"' : ''}>启用</button>
          <button type="button" class="ghost-button danger-button" data-del="${cfg.name}">删除</button>
        </div>
      </div>
    `);
  });

  try {
    const routes = await fetchJSON('/api/llm-routes');
    parts.push(`
      <div class="note-box">
        <strong style="display:block;margin-bottom:8px;color:var(--fg)">路由表</strong>
        ${Object.entries(routes.routes || {}).map(([task, adapter]) => `<div style="margin-bottom:4px">${task} → <span class="mono">${adapter}</span></div>`).join('') || '暂无路由'}
      </div>
    `);
  } catch (err) {
    // ignore
  }

  els.llmConfigs.innerHTML = parts.join('') || '<div class="note-box">暂无已保存配置</div>';

  els.llmConfigs.querySelectorAll('[data-del]').forEach(node => {
    node.addEventListener('click', async () => {
      const name = node.dataset.del;
      if (!name) {
        return;
      }
      if (!confirm(`删除 LLM 配置“${name}”？`)) {
        return;
      }
      const resp = await fetch(`/api/llm-configs/${encodeURIComponent(name)}`, { method: 'DELETE' });
      if (!resp.ok) {
        alert('删除失败');
        return;
      }
      await loadLLMConfigs();
    });
  });

  els.llmConfigs.querySelectorAll('[data-use]').forEach(node => {
    node.addEventListener('click', async () => {
      if (node.disabled) {
        return;
      }
      const name = node.dataset.use;
      if (!name) {
        return;
      }
      const resp = await fetch('/api/llm-active', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
      });
      if (!resp.ok) {
        alert('切换失败');
        return;
      }
      await Promise.all([loadLLMConfigs(), refreshPanel()]);
    });
  });

  els.llmConfigs.querySelectorAll('[data-edit]').forEach(node => {
    node.addEventListener('click', () => editConfig(node.dataset.edit));
  });

  els.llmConfigs.querySelectorAll('[data-price]').forEach(node => {
    node.addEventListener('click', () => {
      const name = node.dataset.price;
      if (!name) {
        return;
      }
      const source = activeName === name ? active : configs.find(cfg => cfg.name === name);
      if (!source) {
        alert('读取价格配置失败');
        return;
      }
      openPricingPopup(source);
    });
  });
}

async function savePricing() {
  const target = state.pricingTarget;
  if (!target || !target.name) {
    alert('请先选择一个要编辑价格的配置');
    return;
  }
  const resp = await fetch(`/api/llm-configs/${encodeURIComponent(target.name)}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      prompt_price: parseFloat(els.pricePrompt.value) || 1.0,
      completion_price: parseFloat(els.priceComp.value) || 4.0,
    }),
  });
  if (!resp.ok) {
    alert('保存定价失败');
    return;
  }
  closePricingPopup();
  await Promise.all([loadLLMConfigs(), refreshPanel()]);
}

function isNarrativeTimelineEvent(event) {
  return !['clock_advance', 'variable_set', 'scene_init', 'fact_extracted', 'observe'].includes(event.type);
}

function timelineIcon(type) {
  const icons = {
    user_message: '⬤',
    dialogue: '◆',
    dice_roll: '◈',
    trust_change: '△',
    fear_change: '▴',
    tension_change: '⋯',
    world_pressure: '☍',
    population_promoted: '⬆',
    population_identity_shift: '≈',
    scene_change: '↦',
    attack: '✦',
    threat: '⚠',
    negotiation: '≋',
  };
  return icons[type] || '·';
}

function timelineTypeLabel(type) {
  const labels = {
    user_message: '用户输入',
    dialogue: '对话',
    dice_roll: '判定',
    trust_change: '信任变化',
    fear_change: '恐惧变化',
    tension_change: '张力变化',
    world_pressure: '世界压力',
    population_promoted: '人物晋升',
    population_identity_shift: '人格漂移',
    scene_change: '场景切换',
    negotiation: '交涉',
    threat: '威胁',
    threaten: '威胁',
    attack: '攻击',
    npc_action: 'NPC行动',
    hide: '躲藏',
    move: '移动',
    go: '移动',
    speak: '发言',
    talk: '交谈',
  };
  return labels[type] || type;
}

function timelineDetail(event) {
  const payload = event.payload || {};
  if ((event.type === 'user_message' || event.type === 'dialogue') && payload.content) {
    return `“${truncate(payload.content, 42)}”`;
  }
  if (event.type === 'user_message') {
    return '用户输入未保留文本摘要';
  }
  if (event.type === 'dialogue') {
    return '该对话事件缺少文本摘要';
  }
  if (event.type === 'dice_roll' && payload.summary) {
    return payload.summary;
  }
  if ((event.type === 'trust_change' || event.type === 'fear_change' || event.type === 'tension_change') && payload.delta !== undefined) {
    return `delta ${Number(payload.delta).toFixed(2)}`;
  }
  if (event.type === 'world_pressure') {
    return `${safeText(payload.name, payload.pressure_id || '--')} · ${safeText(payload.target, '--')} · intensity ${Number(payload.intensity || 0).toFixed(2)}`;
  }
  if (event.type === 'population_promoted') {
    return `${safeText(payload.npc_name, event.target || '--')} 晋升为 ${safeText(payload.status, 'promoted')} · score ${Number(payload.score || 0).toFixed(2)}`;
  }
  if (event.type === 'population_identity_shift') {
    return `${safeText(payload.npc_name, event.target || '--')} · ${safePayloadString(payload.summary, 'adaptive 漂移')}`;
  }
  if (event.type === 'scene_change' && payload.location) {
    return `前往 ${payload.location}`;
  }
  if (payload.intent) {
    return truncate(payload.intent, 34);
  }
  return '';
}

function safePayloadString(value, fallback = '--') {
  if (typeof value !== 'string' || !value.trim()) {
    return fallback;
  }
  return value;
}

function renderTimeline(timeline) {
  const events = (timeline || []).filter(item => isNarrativeTimelineEvent(item.event));
  if (!events.length) {
    els.timeline.innerHTML = '<div class="note-box">暂无叙事事件</div>';
    return;
  }

  els.timeline.innerHTML = events.slice(-15).reverse().map(item => {
    const event = item.event;
    const actor = truncate(event.actor || '?', 8);
    const target = truncate(event.target || '?', 8);
    const detail = timelineDetail(event);
    const direction = event.target ? `${actor} → ${target}` : actor;
    return `
      <div class="timeline-item" data-cause="${event.id}">
        <div class="timeline-top">
          <div class="timeline-icon">${timelineIcon(event.type)}</div>
          <div class="row-main">
            <div class="row-title">${timelineTypeLabel(event.type)} · ${direction}</div>
            ${detail ? `<div class="timeline-detail">${detail}</div>` : ''}
          </div>
          <div class="timeline-index">#${item.index}</div>
        </div>
      </div>
    `;
  }).join('');

  els.timeline.querySelectorAll('[data-cause]').forEach(node => {
    node.addEventListener('click', () => showCausalChain(node.dataset.cause));
  });
}

async function loadTimeline(branch) {
  try {
    const [branches, timelineResp] = await Promise.all([
      fetchJSON('/api/branches').catch(() => ({ branches: ['main'] })),
      fetchJSON(`/api/timeline?limit=30${branch ? `&branch=${encodeURIComponent(branch)}` : ''}`).catch(() => ({ timeline: [] })),
    ]);

    const list = branches.branches || ['main'];
    const currentBranch = branch || els.timelineBranch.value || 'main';
    els.timelineBranch.innerHTML = '';
    list.forEach(name => {
      const option = document.createElement('option');
      option.value = name;
      option.textContent = name;
      option.selected = name === currentBranch;
      els.timelineBranch.appendChild(option);
    });

    renderTimeline(timelineResp.timeline || []);
  } catch (err) {
    console.error(err);
  }
}

async function forkTimeline() {
  const branch = prompt('新分叉名称:');
  if (!branch) {
    return;
  }
  const data = await fetchJSON(`/api/timeline?branch=${encodeURIComponent(els.timelineBranch.value || 'main')}&limit=1`).catch(() => ({ timeline: [] }));
  const lastEvent = (data.timeline || [])[data.timeline.length - 1];
  if (!lastEvent) {
    alert('没有可分叉的事件');
    return;
  }
  await apiFetch('/api/fork', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ event_id: lastEvent.event.id, branch }),
  });
  loadTimeline();
}

function renderChainCard(el, evt, label, weight) {
  if (!evt || !evt.id) { el.style.display = 'none'; return; }
  el.style.display = '';
  const actors = `${evt.actor || '?'} → ${evt.target || '?'}`;
  const detail = evt.payload?.content || evt.payload?.narrative || evt.payload?.intent || evt.payload?.summary || '';
  const snippet = detail.length > 80 ? detail.slice(0, 80) + '...' : detail;
  el.innerHTML = `
    <span class="cc-weight">${weight != null ? 'w=' + weight.toFixed(2) : ''}</span>
    <div class="cc-type">${label} · ${evt.type || '?'}</div>
    <div class="cc-actors">${safeText(actors)}</div>
    ${snippet ? `<div class="cc-detail">"${safeText(snippet)}"</div>` : ''}
  `;
}

function findStrongestCause(chain) {
  if (!chain?.causes?.length) return null;
  let best = null, bestWeight = -1;
  for (const c of chain.causes) {
    const w = (c.event?.causes || []).find(x => x.event_id === chain.event?.id)?.weight ?? 0.3;
    if (w > bestWeight) { bestWeight = w; best = c; }
  }
  return best ? { event: best.event, weight: bestWeight, chain: best } : null;
}

function findStrongestEffect(chain) {
  if (!chain?.effects?.length) return null;
  let best = null, bestWeight = -1;
  for (const e of chain.effects) {
    const w = (chain.event?.causes || []).find(x => x.event_id === e.event?.id)?.weight
      ?? (e.event?.causes || []).find(x => x.event_id === chain.event?.id)?.weight ?? 0.3;
    if (w > bestWeight) { bestWeight = w; best = e; }
  }
  return best ? { event: best.event, weight: bestWeight, chain: best } : null;
}

async function showCausalChain(eventID) {
  if (!eventID) {
    return;
  }
  try {
    const [causality, replay] = await Promise.all([
      fetchJSON(`/api/causality?id=${encodeURIComponent(eventID)}&depth=3&mode=narrative`),
      fetchJSON(`/api/replay?id=${encodeURIComponent(eventID)}`).catch(() => ({})),
    ]);
    els.chainContent.textContent = causality.summary || '无因果数据';

    const chain = causality.chain;
    if (chain?.event) {
      const sc = findStrongestCause(chain);
      const se = findStrongestEffect(chain);
      renderChainCard(els.chainStrongestCause, sc?.event, '最强因', sc?.weight);
      renderChainCard(els.chainStrongestFocus, chain.event, '当前事件', null);
      renderChainCard(els.chainStrongestEffect, se?.event, '最强果', se?.weight);
      els.chainStrongest.style.display = (sc || se) ? '' : 'none';
    } else {
      els.chainStrongest.style.display = 'none';
    }

    els.chainReplay.textContent = `回放状态：第${replay.clock?.day || 0}天 ${String(replay.clock?.hour || 0).padStart(2, '0')}:${String(replay.clock?.minute || 0).padStart(2, '0')} · ${safeText(replay.scene?.location, '?')} · 张力 ${(replay.tension || 0).toFixed ? replay.tension.toFixed(2) : '0.00'}`;
    els.chainModal.classList.add('open');
  } catch (err) {
    alert('查询因果链失败');
  }
}

async function showOlderTrace() {
  const idx = selectedTraceIndex();
  if (idx < 0 || idx >= state.traceHistoryItems.length - 1) {
    return;
  }
  await selectTraceTurn(state.traceHistoryItems[idx + 1].turn);
}

async function showNewerTrace() {
  const idx = selectedTraceIndex();
  if (idx <= 0) {
    return;
  }
  await selectTraceTurn(state.traceHistoryItems[idx - 1].turn);
}

async function resetConversation() {
  if (!confirm('确定要重新开始对话吗？当前对话将被清除。')) {
    return;
  }
  await apiFetch('/api/dialogue/reset', { method: 'POST' });
  els.chatScroll.innerHTML = '';
  state.msgCount = 0;
  state.selectedTraceTurn = null;
  renderMessage('system', '', '对话已重置');
  refreshPanel();
}

async function compressEvents() {
  const total = parseInt(($('pan-events').textContent || '0'), 10) || 0;
  if (total < 100) {
    alert('事件数不足，无需压缩');
    return;
  }
  await apiFetch('/api/compress', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ from: 0, to: Math.min(total, 200) }),
  });
  refreshPanel();
}

function bindEvents() {
  els.themeToggle.addEventListener('click', nextTheme);

  if (els.storySpotlightToggle) {
    els.storySpotlightToggle.addEventListener('click', () => {
      setMobileSpotlightOpen(!state.mobileSpotlightOpen);
    });
  }

  els.composer.addEventListener('submit', event => {
    event.preventDefault();
    sendMessage();
  });

  els.instanceSelect.addEventListener('change', event => switchInstanceView(event.target.value));
  els.resetBtn.addEventListener('click', resetConversation);
  els.charSelect.addEventListener('change', event => switchCharacter(event.target.value));

  els.panelToggle.addEventListener('click', () => {
    els.panel.classList.toggle('open');
  });
  if (els.panelClose) {
    els.panelClose.addEventListener('click', closePanelOnMobile);
  }
  document.querySelectorAll('[data-panel-toggle]').forEach(node => {
    node.addEventListener('click', () => togglePanelGroup(node.dataset.panelToggle));
  });
  document.querySelectorAll('[data-panel-jump]').forEach(node => {
    node.addEventListener('click', event => {
      const group = node.dataset.panelJump;
      const target = document.getElementById(`panel-${group}`);
      if (!group || !target) {
        return;
      }
      event.preventDefault();
      revealPanelGroup(group);
      target.scrollIntoView({ behavior: 'smooth', block: 'start' });
    });
  });

  els.chainClose.addEventListener('click', closeChainModal);
  els.chainModal.addEventListener('click', event => {
    if (event.target === els.chainModal) {
      closeChainModal();
    }
  });

  els.tensionSlider.addEventListener('input', () => {
    clearTimeout(state.refreshTimer);
    state.refreshTimer = setTimeout(async () => {
      await apiFetch('/api/director', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'set_tension', value: Number(els.tensionSlider.value) / 100 }),
      });
      refreshPanel();
    }, 280);
  });

  els.msgLimitSlider.addEventListener('input', () => {
    els.msgLimitVal.textContent = els.msgLimitSlider.value;
    localStorage.setItem('corerp-msg-limit', els.msgLimitSlider.value);
  });

  els.priceSave.addEventListener('click', savePricing);
  els.priceCancel.addEventListener('click', closePricingPopup);

  els.cfgAddBtn.addEventListener('click', () => toggleConfigForm(true));
  els.cfgCancel.addEventListener('click', () => {
    clearConfigForm();
    toggleConfigForm(false);
  });
  els.cfgFetchModels.addEventListener('click', fetchModelsForDraftConfig);
  els.cfgSave.addEventListener('click', saveConfig);

  els.timelineBranch.addEventListener('change', event => loadTimeline(event.target.value));
  if (els.worldSelect) {
    els.worldSelect.addEventListener('change', event => { enterWorld(event.target.value); updateWorldConvertButton(); });
  }
  if (els.worldCreateBtn) {
    els.worldCreateBtn.addEventListener('click', showWorldCreateModal);
  }
  if (els.worldCreateClose) {
    els.worldCreateClose.addEventListener('click', hideWorldCreateModal);
  }
  if (els.worldCreateSubmit) {
    els.worldCreateSubmit.addEventListener('click', createWorld);
  }
  if (els.worldConvertBtn) {
    els.worldConvertBtn.addEventListener('click', convertWorld);
  }
  if (els.worldCreateModal) {
    els.worldCreateModal.addEventListener('click', event => {
      if (event.target === els.worldCreateModal) hideWorldCreateModal();
    });
  }
  els.timelineForkBtn.addEventListener('click', forkTimeline);
  els.diffRefreshBtn.addEventListener('click', async () => {
    await Promise.all([loadSaveSlots(), refreshPanel()]);
  });
  els.branchDiffRunBtn.addEventListener('click', runBranchDiff);
  els.branchMergeBtn.addEventListener('click', mergeBranchDiff);
  els.saveDiffRunBtn.addEventListener('click', runSaveDiff);
  els.compressBtn.addEventListener('click', compressEvents);
  els.exportJSONBtn.addEventListener('click', () => exportSession('json'));
  els.exportMDBtn.addEventListener('click', () => exportSession('markdown'));
  els.instancesRefreshBtn.addEventListener('click', loadInstancesView);
  els.instanceCreateBtn.addEventListener('click', createInstance);
  els.memoryRefreshBtn.addEventListener('click', loadMemoryView);
  els.saveRefreshBtn.addEventListener('click', loadSaveSlots);
  els.saveCreateBtn.addEventListener('click', createSaveSlot);
  els.presetRefreshBtn.addEventListener('click', loadScenarioPresets);
  els.presetCreateBtn.addEventListener('click', createScenarioPreset);
  els.worldcfgReloadBtn.addEventListener('click', loadWorldConfig);
  els.worldcfgSaveBtn.addEventListener('click', saveWorldConfig);
  els.scenecfgReloadBtn.addEventListener('click', loadSceneConfigs);
  els.scenecfgSaveBtn.addEventListener('click', saveSceneConfig);
  els.structureReloadBtn.addEventListener('click', loadWorldStructure);
  els.structureSaveBtn.addEventListener('click', saveWorldStructure);
  els.popcfgReloadBtn.addEventListener('click', loadPopulationConfig);
  els.popcfgSaveBtn.addEventListener('click', savePopulationConfig);
  els.quarantineRefreshBtn.addEventListener('click', loadQuarantineView);
  els.quarantinePromoteAllBtn.addEventListener('click', async () => {
    await reviewAll(state.quarantineEvents, item => item.id, id => reviewQuarantine(id, 'promote'));
  });
  els.quarantineRejectAllBtn.addEventListener('click', async () => {
    await reviewAll(state.quarantineEvents, item => item.id, id => reviewQuarantine(id, 'reject'));
  });
  els.quarantineFilter.addEventListener('input', loadQuarantineView);
  els.pendingRefreshBtn.addEventListener('click', loadPendingFactsView);
  els.pendingConfirmAllBtn.addEventListener('click', async () => {
    await reviewAll(state.pendingFacts, item => item.id, id => reviewPendingFact(id, 'confirm'));
  });
  els.pendingDeleteAllBtn.addEventListener('click', async () => {
    await reviewAll(state.pendingFacts, item => item.id, id => reviewPendingFact(id, 'delete'));
  });
  els.pendingPromoteAllBtn.addEventListener('click', async () => {
    await reviewAll(state.pendingFacts, item => item.id, id => reviewPendingFact(id, 'promote'));
  });
  els.pendingFilter.addEventListener('input', loadPendingFactsView);
  els.tracePrevBtn.addEventListener('click', showOlderTrace);
  els.traceNextBtn.addEventListener('click', showNewerTrace);
  els.traceRefreshBtn.addEventListener('click', async () => {
    await Promise.all([loadTraceHistory(), loadTraceView()]);
  });
  els.scenecfgSelect.addEventListener('change', () => {
    const active = state.scenes.find(scene => scene.name === els.scenecfgSelect.value);
    applySceneToEditor(active);
  });
  els.factsReloadBtn.addEventListener('click', loadCanonFacts);
  els.factsSaveBtn.addEventListener('click', saveCanonFacts);
  els.charcfgReloadBtn.addEventListener('click', loadCharacterConfig);
  els.charcfgSaveBtn.addEventListener('click', saveCharacterConfig);
  els.playerRoleSaveBtn.addEventListener('click', savePlayerRole);
  els.directorSaveBtn.addEventListener('click', saveDirectorConfig);

  els.simRefreshBtn.addEventListener('click', loadSimStatus);
  els.simTickBtn.addEventListener('click', manualTick);
  els.simPauseBtn.addEventListener('click', pauseTick);
  els.simResumeBtn.addEventListener('click', resumeTick);

  document.addEventListener('keydown', event => {
    if (event.key === 'Escape') {
      closeChainModal();
      closePanelOnMobile();
    }
    if (event.ctrlKey && event.key.toLowerCase() === 'b') {
      event.preventDefault();
      els.panel.classList.toggle('open');
    }
  });

  window.addEventListener('resize', applyMobileSpotlightState);
}

async function init() {
  const savedTheme = localStorage.getItem('corerp-theme') || 'dark';
  const savedLimit = parseInt(localStorage.getItem('corerp-msg-limit') || '30', 10);
  state.mobileSpotlightOpen = localStorage.getItem('corerp-mobile-spotlight-open') === '1';
  state.selectedInstanceID = String(localStorage.getItem('corerp-instance-id') || '').trim();
  loadPanelGroupState();
  els.msgLimitSlider.value = String(savedLimit);
  els.msgLimitVal.textContent = String(savedLimit);
  setTheme(savedTheme);

  bindEvents();
  applyPanelGroupState();
  applyMobileSpotlightState();

  await loadInstancesView();
  await loadWorlds();
  await loadCharacters();
  await loadPlayerRole();

  await Promise.all([
    restoreDialogue(),
    refreshPanel(),
    loadInstancesView(),
    loadWorlds(),
    loadLLMConfigs(),
    loadTimeline(),
    loadMemoryView(),
    loadWorldConfig(),
    loadSceneConfigs(),
    loadCanonFacts(),
    loadQuarantineView(),
    loadPendingFactsView(),
    loadDirectorConfig(),
    loadTraceHistory(),
    loadTraceView(),
    loadCharacterConfig(),
    loadSaveSlots(),
    loadScenarioPresets(),
  ]);

  setInterval(refreshPanel, 15000);
  setInterval(() => loadTimeline(els.timelineBranch.value || ''), 30000);
  els.input.focus();
}

init();

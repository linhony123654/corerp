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
  instanceCompareSelect: $('instance-compare-select'),
  instanceCompareSummary: $('instance-compare-summary'),
  instanceList: $('instance-list'),
  instanceCreateID: $('instance-create-id'),
  instanceCreateLabel: $('instance-create-label'),
  instanceCreateFocus: $('instance-create-focus'),
  instanceCreateBtn: $('instance-create-btn'),
  instanceCreateExperimentBtn: $('instance-create-experiment-btn'),
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
  simBatchCount: $('sim-batch-count'),
  simBatchBtn: $('sim-batch-btn'),
  simBatchCompareBtn: $('sim-batch-compare-btn'),
  simPauseBtn: $('sim-pause-btn'),
  simResumeBtn: $('sim-resume-btn'),
  simPressureStates: $('sim-pressure-states'),
  simFactionTensions: $('sim-faction-tensions'),
  simNpcExposure: $('sim-npc-exposure'),
  simPopulationHighlights: $('sim-population-highlights'),
  simDiagnostics: $('sim-diagnostics'),
  simTrajectorySummary: $('sim-trajectory-summary'),
  simLastTickSummary: $('sim-last-tick-summary'),
  simTickHistory: $('sim-tick-history'),
  simCompareSummary: $('sim-compare-summary'),
  simInstanceCompareSummary: $('sim-instance-compare-summary'),
  simExperimentSummary: $('sim-experiment-summary'),
  simReportName: $('sim-report-name'),
  simReportNote: $('sim-report-note'),
  simReportSaveBtn: $('sim-report-save-btn'),
  simReportRefreshBtn: $('sim-report-refresh-btn'),
  simReportBatchReplayBtn: $('sim-report-batch-replay-btn'),
  simReportBatchRefreshBtn: $('sim-report-batch-refresh-btn'),
  simReportExportJSONBtn: $('sim-report-export-json-btn'),
  simReportExportMDBtn: $('sim-report-export-md-btn'),
  simReportExportBaselineJSONBtn: $('sim-report-export-baseline-json-btn'),
  simReportExportBaselineMDBtn: $('sim-report-export-baseline-md-btn'),
  simReportList: $('sim-report-list'),
  runtimeAuditRefreshBtn: $('runtime-audit-refresh-btn'),
  runtimeAuditFilter: $('runtime-audit-filter'),
  runtimeAuditCause: $('runtime-audit-cause'),
  runtimeAuditSummary: $('runtime-audit-summary'),
  runtimeAuditPanel: $('runtime-audit-panel'),
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
  compareInstanceID: '',
  experimentReports: [],
  experimentWorldFilter: '',
  runtimeAudit: null,
  proofAudits: null,
  runtimeAuditCheckpointName: '',
  runtimeAuditFilter: 'all',
  runtimeAuditCause: 'all',
  runtimeAuditReplayTrace: null,
  runtimeAuditReplayStepIndex: 0,
  runtimeAuditReportName: '',
  experimentReplayRuns: {},
  experimentReplayBatchSummary: null,
  selectedTraceTurn: null,
  lastSimStatus: null,
  compareSimStatus: null,
  lastStructureSnapshot: null,
  lastStructureChangeSummary: [],
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

async function fetchJSON(url, options, allowRecover = true) {
  const fullURL = buildAPIURL(url);
  const headers = { ...(options && options.headers) };
  const cached = _etagCache[fullURL];
  if (cached) headers['If-None-Match'] = cached.etag;
  const resp = await fetch(fullURL, { ...options, headers });
  if (resp.status === 304 && cached) return cached.data;
  if (!resp.ok) {
    const detail = await resp.text().catch(() => '');
    const error = new Error(`${resp.status} ${resp.statusText}${detail ? `: ${detail.trim()}` : ''}`);
    if (allowRecover && isInstanceNotFoundError(error) && !fullURL.startsWith('/api/instances')) {
      const recovered = await recoverSelectedInstanceFromNotFound();
      if (recovered) {
        return fetchJSON(url, options, false);
      }
    }
    throw error;
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

function buildInstanceAPIURL(url, instanceID) {
  const current = new URL(url, window.location.origin);
  const id = String(instanceID || '').trim();
  if (!current.pathname.startsWith('/api/')) {
    return current.pathname + current.search + current.hash;
  }
  if (id) {
    current.searchParams.set('instance_id', id);
  }
  return current.pathname + current.search + current.hash;
}

async function apiFetch(url, options = {}) {
  return fetch(buildAPIURL(url), options);
}

async function apiFetchForInstance(url, instanceID, options = {}) {
  return fetch(buildInstanceAPIURL(url, instanceID), options);
}

async function fetchJSONForInstance(url, instanceID, options, allowRecover = true) {
  const fullURL = buildInstanceAPIURL(url, instanceID);
  const headers = { ...(options && options.headers) };
  const cached = _etagCache[fullURL];
  if (cached) headers['If-None-Match'] = cached.etag;
  const resp = await fetch(fullURL, { ...options, headers });
  if (resp.status === 304 && cached) return cached.data;
  if (!resp.ok) {
    const detail = await resp.text().catch(() => '');
    const error = new Error(`${resp.status} ${resp.statusText}${detail ? `: ${detail.trim()}` : ''}`);
    if (allowRecover && isInstanceNotFoundError(error)) {
      const recovered = await recoverSelectedInstanceFromNotFound();
      if (recovered) {
        return fetchJSONForInstance(url, instanceID, options, false);
      }
    }
    throw error;
  }
  const etag = resp.headers.get('ETag');
  const data = await resp.json();
  if (etag) _etagCache[fullURL] = { etag, data };
  return data;
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

function isInstanceNotFoundError(err) {
  const message = String(err?.message || '');
  return message.startsWith('404 ') && message.includes('Not found');
}

async function recoverSelectedInstanceFromNotFound() {
  const previous = String(state.selectedInstanceID || '').trim();
  await loadInstancesView();
  const next = String(state.selectedInstanceID || '').trim();
  return Boolean(next) && next !== previous;
}

async function loadRuntimeAuditReplay(turn, stepIndex = 0) {
  try {
    const trace = await fetchTraceJSON(turn);
    state.runtimeAuditReplayTrace = trace || null;
    state.runtimeAuditReplayStepIndex = Math.max(0, Number(stepIndex || 0));
    renderRuntimeAudit();
  } catch (err) {
    console.error('runtime audit replay error:', err);
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

function downloadText(filename, content, mime = 'text/plain;charset=utf-8') {
  const blob = new Blob([content], { type: mime });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
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
    : (data.participants || []);
  const active = data.focus_character || '';
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
    const dominant = Array.isArray(candidate.dominant_factors) && candidate.dominant_factors.length
      ? ` · 主导 ${candidate.dominant_factors.map(item => safeText(item)).join('/')}`
      : '';
    return `${candidate.name} ${Number(candidate.score || 0).toFixed(1)} [${tags.join('/') || 'candidate'}] ${reason}${dominant}`;
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
        ${Array.isArray(plan.world_signals) && plan.world_signals.length ? `<div class="row-subtitle">world：${plan.world_signals.map(item => safeText(item)).join(' · ')}</div>` : ''}
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
  const ph = Array.isArray(data.population_highlights) ? data.population_highlights : [];

  const pressureItems = Object.entries(ps).map(([id, intensity]) => `${safeText(id)}:${Number(intensity).toFixed(2)}`).join(' · ');
  els.simPressureStates.textContent = pressureItems || '暂无 pressure 动态数据';

  const factionItems = Object.entries(ft).map(([id, tension]) => `${safeText(id)}:${Number(tension).toFixed(2)}`).join(' · ');
  els.simFactionTensions.textContent = factionItems || '暂无 faction 紧张度数据';

  const exposureItems = Object.entries(ne).map(([name, count]) => `${safeText(name)}:${count}`).join(' · ');
  els.simNpcExposure.textContent = exposureItems || '暂无 NPC 累积数据';
  els.simPopulationHighlights.textContent = ph.length ? ph.join(' · ') : '暂无 population 摘要';
}

function renderSimDiagnostics(data) {
  const diagnostics = Array.isArray(data.diagnostics) ? data.diagnostics : [];
  if (!diagnostics.length) {
    els.simDiagnostics.textContent = '暂无诊断建议';
    return;
  }
  els.simDiagnostics.innerHTML = diagnostics.map(item => {
    const level = safeText(item.level || 'info');
    const metric = safeText(item.metric || '--');
    const target = item.target ? ` · ${safeText(item.target)}` : '';
    const value = typeof item.value === 'number' ? ` · ${Number(item.value).toFixed(2)}` : '';
    const tone = item.level === 'critical'
      ? 'background:#452222;color:#ffb3b3;'
      : item.level === 'warning'
        ? 'background:#4a3b16;color:#ffd27a;'
        : 'background:#1d2b35;color:#9cd6ff;';
    return `<div class="note-box mono" style="margin-top:4px;${tone}">${level} · ${metric}${target}${value} · ${safeText(item.message, '--')}</div>`;
  }).join('');
}

function renderLastTickSummary(data) {
  const items = Array.isArray(data.last_tick_summary) ? data.last_tick_summary : [];
  if (!items.length) {
    els.simLastTickSummary.textContent = '暂无最近变化摘要';
    return;
  }
  els.simLastTickSummary.innerHTML = items.map(item => `<div class="note-box mono" style="margin-top:4px">${safeText(item)}</div>`).join('');
}

function renderTrajectorySummary(data) {
  const items = Array.isArray(data.trajectory_summary) ? data.trajectory_summary : [];
  if (!items.length) {
    els.simTrajectorySummary.textContent = '暂无长期总结';
    return;
  }
  els.simTrajectorySummary.innerHTML = items.map(item => `<div class="note-box mono" style="margin-top:4px">${safeText(item)}</div>`).join('');
}

function renderTickHistory(data) {
  const history = Array.isArray(data.tick_history) ? data.tick_history : [];
  if (!history.length) {
    els.simTickHistory.textContent = '暂无近期轨迹';
    return;
  }
  const items = history.slice().reverse().map(item => {
    const tick = Number(item.tick || 0);
    const tension = typeof item.tension === 'number' ? item.tension.toFixed(2) : '--';
    const pressure = Object.entries(item.pressure_states || {}).slice(0, 2).map(([id, value]) => `${safeText(id)}:${Number(value).toFixed(2)}`).join(' · ');
    const summary = Array.isArray(item.summary) && item.summary.length ? safeText(item.summary[0]) : '无摘要';
    const pop = Array.isArray(item.population_highlights) && item.population_highlights.length ? ` · ${safeText(item.population_highlights[0])}` : '';
    const diagnostics = Array.isArray(item.diagnostics) && item.diagnostics.length ? ` · ${safeText(item.diagnostics[0].metric || 'diag')}` : '';
    const pressureText = pressure ? ` · ${pressure}` : '';
    return `<div class="note-box mono" style="margin-top:4px">#${tick} · tension ${tension}${pressureText}${pop}${diagnostics}<br>${summary}</div>`;
  });
  els.simTickHistory.innerHTML = items.join('');
}

function summarizeMapChange(label, before = {}, after = {}, formatter = value => String(value)) {
  const keys = new Set([...Object.keys(before || {}), ...Object.keys(after || {})]);
  const changes = [];
  keys.forEach(key => {
    const a = before?.[key];
    const b = after?.[key];
    if (a === b) return;
    changes.push(`${safeText(key)} ${formatter(a ?? 0)} -> ${formatter(b ?? 0)}`);
  });
  if (!changes.length) return '';
  return `${label}: ${changes.slice(0, 3).join(' · ')}`;
}

function pickTopMetric(metrics = {}) {
  const entries = Object.entries(metrics || {}).filter(([, value]) => Number.isFinite(Number(value)));
  if (!entries.length) return null;
  entries.sort((a, b) => {
    const gap = Number(b[1] || 0) - Number(a[1] || 0);
    if (gap !== 0) return gap;
    return String(a[0]).localeCompare(String(b[0]));
  });
  const [key, value] = entries[0];
  return { key: String(key), value: Number(value || 0) };
}

function sumMetricMap(metrics = {}) {
  return Object.values(metrics || {}).reduce((sum, value) => sum + Number(value || 0), 0);
}

function formatTopMetric(entry, digits = 2) {
  if (!entry) return '--';
  return `${safeText(entry.key)}:${Number(entry.value || 0).toFixed(digits)}`;
}

function collectInstanceDiffDetails(current, compare, currentInstance, compareInstance) {
  if (!current || !compare) return [];
  const currentID = safeText(currentInstance?.id || state.selectedInstanceID || 'current');
  const compareID = safeText(compareInstance?.id || state.compareInstanceID || 'compare');
  const lines = [];

  const currentTension = Number(current.tension ?? 0);
  const compareTension = Number(compare.tension ?? 0);
  if (currentTension !== compareTension) {
    const leader = currentTension > compareTension ? currentID : compareID;
    lines.push(`tension lead: ${leader} (${currentTension.toFixed(2)} vs ${compareTension.toFixed(2)}, gap ${Math.abs(currentTension - compareTension).toFixed(2)})`);
  }

  const currentPressure = pickTopMetric(current.pressure_states || {});
  const comparePressure = pickTopMetric(compare.pressure_states || {});
  if (currentPressure || comparePressure) {
    const pressureLeader = sumMetricMap(current.pressure_states || {}) > sumMetricMap(compare.pressure_states || {}) ? currentID : compareID;
    if (sumMetricMap(current.pressure_states || {}) === sumMetricMap(compare.pressure_states || {})) {
      lines.push(`pressure split: ${formatTopMetric(currentPressure)} | ${formatTopMetric(comparePressure)}`);
    } else {
      lines.push(`pressure split: ${pressureLeader} · ${formatTopMetric(currentPressure)} | ${formatTopMetric(comparePressure)}`);
    }
  }

  const currentFaction = pickTopMetric(current.faction_tensions || {});
  const compareFaction = pickTopMetric(compare.faction_tensions || {});
  if (currentFaction || compareFaction) {
    const currentFactionTotal = sumMetricMap(current.faction_tensions || {});
    const compareFactionTotal = sumMetricMap(compare.faction_tensions || {});
    const factionLeader = currentFactionTotal > compareFactionTotal ? currentID : compareID;
    if (currentFactionTotal === compareFactionTotal) {
      lines.push(`faction split: ${formatTopMetric(currentFaction)} | ${formatTopMetric(compareFaction)}`);
    } else {
      lines.push(`faction split: ${factionLeader} · ${formatTopMetric(currentFaction)} | ${formatTopMetric(compareFaction)}`);
    }
  }

  const currentPop = Array.isArray(current.population_highlights) ? current.population_highlights.join(' · ') : '';
  const comparePop = Array.isArray(compare.population_highlights) ? compare.population_highlights.join(' · ') : '';
  if (currentPop || comparePop) {
    if (currentPop === comparePop) {
      lines.push(`population split: aligned (${safeText(currentPop || '--')})`);
    } else {
      lines.push(`population split: ${safeText(currentPop || '--')} | ${safeText(comparePop || '--')}`);
    }
  }

  const currentDiag = Array.isArray(current.trajectory_summary) ? current.trajectory_summary.find(line => String(line).startsWith('recent diagnostics:')) : '';
  const compareDiag = Array.isArray(compare.trajectory_summary) ? compare.trajectory_summary.find(line => String(line).startsWith('recent diagnostics:')) : '';
  if (currentDiag || compareDiag) {
    lines.push(`diagnostic split: ${safeText(currentDiag || '--')} | ${safeText(compareDiag || '--')}`);
  }

  return lines;
}

function collectReplayDriverLines(currentAudit, compareAudit, currentInstance, compareInstance) {
  const lines = [];
  if (!currentAudit && !compareAudit) return lines;
  const currentID = safeText(currentInstance?.id || 'current');
  const compareID = safeText(compareInstance?.id || 'compare');

  const currentSignals = Array.isArray(currentAudit?.director_plan?.world_signals) ? currentAudit.director_plan.world_signals.slice(0, 2) : [];
  const compareSignals = Array.isArray(compareAudit?.director_plan?.world_signals) ? compareAudit.director_plan.world_signals.slice(0, 2) : [];
  if (currentSignals.length || compareSignals.length) {
    lines.push(`driver signals: ${currentID}=${safeText(currentSignals.join(' · ') || '--')} | ${compareID}=${safeText(compareSignals.join(' · ') || '--')}`);
  }

  const currentDiag = Array.isArray(currentAudit?.sim_status?.diagnostics) ? currentAudit.sim_status.diagnostics.slice(0, 2) : [];
  const compareDiag = Array.isArray(compareAudit?.sim_status?.diagnostics) ? compareAudit.sim_status.diagnostics.slice(0, 2) : [];
  if (currentDiag.length || compareDiag.length) {
    const currentDiagText = currentDiag.map(item => `${safeText(item.metric || '--')}:${safeText(item.message || '--')}`).join(' · ');
    const compareDiagText = compareDiag.map(item => `${safeText(item.metric || '--')}:${safeText(item.message || '--')}`).join(' · ');
    lines.push(`driver diagnostics: ${currentID}=${safeText(currentDiagText || '--')} | ${compareID}=${safeText(compareDiagText || '--')}`);
  }

  const currentTrace = currentAudit?.latest_trace || null;
  const compareTrace = compareAudit?.latest_trace || null;
  if (currentTrace || compareTrace) {
    const currentTraceText = currentTrace ? `turn ${Number(currentTrace.turn || 0)} · ${safeText(currentTrace.focus_character || '--')} · ${safeText(currentTrace.user_input || '--')}` : '--';
    const compareTraceText = compareTrace ? `turn ${Number(compareTrace.turn || 0)} · ${safeText(compareTrace.focus_character || '--')} · ${safeText(compareTrace.user_input || '--')}` : '--';
    lines.push(`driver trace: ${currentID}=${currentTraceText} | ${compareID}=${compareTraceText}`);
  }
  lines.push(...collectReplayStepDiffLines(currentTrace, compareTrace, currentID, compareID));

  const currentPromoted = Array.isArray(currentAudit?.population?.promoted) ? currentAudit.population.promoted.slice(0, 2) : [];
  const comparePromoted = Array.isArray(compareAudit?.population?.promoted) ? compareAudit.population.promoted.slice(0, 2) : [];
  if (currentPromoted.length || comparePromoted.length) {
    const currentPromotedText = currentPromoted.map(item => `${safeText(item.name)}(${Number(item.attention?.score || 0).toFixed(1)})`).join(' · ');
    const comparePromotedText = comparePromoted.map(item => `${safeText(item.name)}(${Number(item.attention?.score || 0).toFixed(1)})`).join(' · ');
    lines.push(`driver promoted: ${currentID}=${safeText(currentPromotedText || '--')} | ${compareID}=${safeText(comparePromotedText || '--')}`);
  }

  const currentBackground = Array.isArray(currentAudit?.population?.background) ? currentAudit.population.background.slice(0, 1) : [];
  const compareBackground = Array.isArray(compareAudit?.population?.background) ? compareAudit.population.background.slice(0, 1) : [];
  if (currentBackground.length || compareBackground.length) {
    const currentBackgroundText = currentBackground.map(item => `${safeText(item.name)}(${Number(item.attention?.score || 0).toFixed(1)})`).join(' · ');
    const compareBackgroundText = compareBackground.map(item => `${safeText(item.name)}(${Number(item.attention?.score || 0).toFixed(1)})`).join(' · ');
    lines.push(`driver rising: ${currentID}=${safeText(currentBackgroundText || '--')} | ${compareID}=${safeText(compareBackgroundText || '--')}`);
  }

  return lines;
}

function collectReplayStepDiffLines(currentTrace, compareTrace, currentID, compareID) {
  const currentSteps = Array.isArray(currentTrace?.step_traces) ? currentTrace.step_traces : [];
  const compareSteps = Array.isArray(compareTrace?.step_traces) ? compareTrace.step_traces : [];
  if (!currentSteps.length && !compareSteps.length) {
    return [];
  }
  const lines = [];
  const maxLen = Math.max(currentSteps.length, compareSteps.length);
  for (let i = 0; i < maxLen; i++) {
    const current = currentSteps[i] || null;
    const compare = compareSteps[i] || null;
    if (!current && !compare) {
      continue;
    }
    const currentSpeaker = String(current?.speaker || '--');
    const compareSpeaker = String(compare?.speaker || '--');
    const currentAction = String(current?.action_frame?.action || current?.kind || '--');
    const compareAction = String(compare?.action_frame?.action || compare?.kind || '--');
    const currentReason = String(current?.reason || '');
    const compareReason = String(compare?.reason || '');
    const currentBlocked = current?.validator?.blocked ? `blocked:${safeText(current.validator.reason || '--')}` : '';
    const compareBlocked = compare?.validator?.blocked ? `blocked:${safeText(compare.validator.reason || '--')}` : '';
    const currentTokens = Number(current?.used_tokens || 0);
    const compareTokens = Number(compare?.used_tokens || 0);

    if (currentSpeaker !== compareSpeaker || currentAction !== compareAction || currentBlocked !== compareBlocked || currentReason !== compareReason) {
      lines.push(`step ${i + 1}: ${currentID}=${safeText(currentSpeaker)} / ${safeText(currentAction)}${currentBlocked ? ` / ${currentBlocked}` : ''} | ${compareID}=${safeText(compareSpeaker)} / ${safeText(compareAction)}${compareBlocked ? ` / ${compareBlocked}` : ''}`);
    } else if (currentTokens !== compareTokens) {
      lines.push(`step ${i + 1}: token delta ${currentID}=${currentTokens} | ${compareID}=${compareTokens}`);
    }
  }
  if (!lines.length) {
    lines.push(`step diff: latest trace steps still aligned (${currentID} / ${compareID})`);
  }
  return lines.slice(0, 4);
}

function collectReplayTimelineLines(currentAudit, compareAudit, currentInstance, compareInstance) {
  const lines = [];
  if (!currentAudit && !compareAudit) return lines;
  const currentID = safeText(currentInstance?.id || 'current');
  const compareID = safeText(compareInstance?.id || 'compare');

  const currentHistory = Array.isArray(currentAudit?.sim_status?.tick_history) ? currentAudit.sim_status.tick_history : [];
  const compareHistory = Array.isArray(compareAudit?.sim_status?.tick_history) ? compareAudit.sim_status.tick_history : [];
  const tickMap = new Map();
  currentHistory.forEach(item => {
    const tick = Number(item?.tick || 0);
    if (tick > 0) tickMap.set(`c:${tick}`, item);
  });
  compareHistory.forEach(item => {
    const tick = Number(item?.tick || 0);
    if (tick > 0) tickMap.set(`p:${tick}`, item);
  });
  const sharedTicks = [...new Set([
    ...currentHistory.map(item => Number(item?.tick || 0)),
    ...compareHistory.map(item => Number(item?.tick || 0)),
  ])].filter(Boolean).sort((a, b) => a - b);
  for (const tick of sharedTicks) {
    const current = tickMap.get(`c:${tick}`) || null;
    const compare = tickMap.get(`p:${tick}`) || null;
    if (!current || !compare) continue;
    const currentTopPressure = pickTopMetric(current.pressure_states || {});
    const compareTopPressure = pickTopMetric(compare.pressure_states || {});
    const currentSummary = Array.isArray(current.summary) ? String(current.summary[0] || '') : '';
    const compareSummary = Array.isArray(compare.summary) ? String(compare.summary[0] || '') : '';
    if (
      Number(current.tension || 0) !== Number(compare.tension || 0) ||
      formatTopMetric(currentTopPressure) !== formatTopMetric(compareTopPressure) ||
      currentSummary !== compareSummary
    ) {
      lines.push(`tick ${tick}: ${currentID}=tension ${Number(current.tension || 0).toFixed(2)} / ${formatTopMetric(currentTopPressure)} / ${safeText(currentSummary || '--')} | ${compareID}=tension ${Number(compare.tension || 0).toFixed(2)} / ${formatTopMetric(compareTopPressure)} / ${safeText(compareSummary || '--')}`);
    }
  }

  const currentTraces = Array.isArray(currentAudit?.recent_traces) ? currentAudit.recent_traces : [];
  const compareTraces = Array.isArray(compareAudit?.recent_traces) ? compareAudit.recent_traces : [];
  const currentTraceByTurn = new Map(currentTraces.map(trace => [Number(trace?.turn || 0), trace]));
  const compareTraceByTurn = new Map(compareTraces.map(trace => [Number(trace?.turn || 0), trace]));
  const sharedTurns = [...new Set([
    ...currentTraces.map(trace => Number(trace?.turn || 0)),
    ...compareTraces.map(trace => Number(trace?.turn || 0)),
  ])].filter(Boolean).sort((a, b) => a - b);
  for (const turn of sharedTurns) {
    const current = currentTraceByTurn.get(turn) || null;
    const compare = compareTraceByTurn.get(turn) || null;
    if (!current || !compare) continue;
    const currentLead = Array.isArray(current.director_plan?.selected) ? String(current.director_plan.selected[0] || '') : '';
    const compareLead = Array.isArray(compare.director_plan?.selected) ? String(compare.director_plan.selected[0] || '') : '';
    const currentInput = String(current.user_input || '');
    const compareInput = String(compare.user_input || '');
    const divergence = describeTraceDivergence(current, compare);
    if (divergence.differs) {
      lines.push(`turn ${turn}: ${currentID}=lead ${safeText(currentLead || '--')} / ${safeText(currentInput || '--')} | ${compareID}=lead ${safeText(compareLead || '--')} / ${safeText(compareInput || '--')} · ${safeText(divergence.reason)}`);
    }
  }

  if (!lines.length) {
    lines.push(`timeline: recent ticks / turns still aligned (${currentID} / ${compareID})`);
  }
  return lines.slice(0, 6);
}

function summarizeStructureChanges(before, after) {
  if (!before || !after) return [];
  const lines = [];
  if ((before.seed?.premise || '') !== (after.seed?.premise || '')) {
    lines.push(`premise 更新: ${after.seed?.premise || '--'}`);
  }
  if ((before.seed?.current_situation || '') !== (after.seed?.current_situation || '')) {
    lines.push(`situation 更新: ${after.seed?.current_situation || '--'}`);
  }
  const countPairs = [
    ['factions', (before.factions || []).length, (after.factions || []).length],
    ['locations', (before.locations || []).length, (after.locations || []).length],
    ['pressures', (before.pressures || []).length, (after.pressures || []).length],
    ['rules', (before.ruleset?.rules || []).length, (after.ruleset?.rules || []).length],
  ];
  countPairs.forEach(([label, a, b]) => {
    if (a !== b) lines.push(`${label}: ${a} -> ${b}`);
  });
  const beforePressureIDs = new Set((before.pressures || []).map(item => item.id || item.name).filter(Boolean));
  const afterPressureIDs = new Set((after.pressures || []).map(item => item.id || item.name).filter(Boolean));
  const addedPressure = [...afterPressureIDs].filter(id => !beforePressureIDs.has(id));
  const removedPressure = [...beforePressureIDs].filter(id => !afterPressureIDs.has(id));
  if (addedPressure.length) lines.push(`新增 pressure: ${addedPressure.slice(0, 3).join(', ')}`);
  if (removedPressure.length) lines.push(`移除 pressure: ${removedPressure.slice(0, 3).join(', ')}`);
  return lines;
}

function collectSimDeltaLines(data, previous) {
  if (!previous) return [];
  const lines = [];
  if (typeof previous.tension === 'number' || typeof data.tension === 'number') {
    const beforeTension = Number(previous.tension ?? 0);
    const afterTension = Number(data.tension ?? 0);
    if (beforeTension !== afterTension) {
      lines.push(`tension: ${beforeTension.toFixed(2)} -> ${afterTension.toFixed(2)}`);
    }
  }
  if ((previous.tick_count ?? 0) !== (data.tick_count ?? 0)) {
    lines.push(`tick: ${previous.tick_count ?? 0} -> ${data.tick_count ?? 0}`);
  }
  if ((previous.turn_count ?? 0) !== (data.turn_count ?? 0)) {
    lines.push(`turn: ${previous.turn_count ?? 0} -> ${data.turn_count ?? 0}`);
  }
  const pressureLine = summarizeMapChange('pressure', previous.pressure_states, data.pressure_states, value => Number(value || 0).toFixed(2));
  if (pressureLine) lines.push(pressureLine);
  const factionLine = summarizeMapChange('faction', previous.faction_tensions, data.faction_tensions, value => Number(value || 0).toFixed(2));
  if (factionLine) lines.push(factionLine);
  const exposureLine = summarizeMapChange('exposure', previous.npc_tick_exposure, data.npc_tick_exposure, value => String(value || 0));
  if (exposureLine) lines.push(exposureLine);
  const prevPop = Array.isArray(previous.population_highlights) ? previous.population_highlights.join(' · ') : '';
  const nextPop = Array.isArray(data.population_highlights) ? data.population_highlights.join(' · ') : '';
  if (prevPop !== nextPop && nextPop) {
    lines.push(`population: ${safeText(nextPop)}`);
  }
  return lines;
}

function renderSimCompareSummary(data, previous) {
  const simDeltaLines = collectSimDeltaLines(data, previous);
  const structureLines = Array.isArray(state.lastStructureChangeSummary)
    ? state.lastStructureChangeSummary.map(line => `structure: ${line}`)
    : [];
  const lines = [];
  if (structureLines.length) {
    lines.push(...structureLines);
    if (simDeltaLines.length) {
      lines.push(...simDeltaLines.map(line => `response: ${line}`));
    } else {
      lines.push('response: awaiting world response');
    }
  } else {
    lines.push(...simDeltaLines);
  }
  if (!lines.length) {
    els.simCompareSummary.textContent = '与上次相比暂无关键变化';
    return;
  }
  els.simCompareSummary.innerHTML = lines.map(line => `<div class="note-box mono" style="margin-top:4px">${safeText(line)}</div>`).join('');
  if (structureLines.length && simDeltaLines.length) {
    state.lastStructureChangeSummary = [];
  }
}

function collectInstanceOutcomeLines(current, compare, currentInstance, compareInstance) {
  if (!current || !compare) return [];
  const currentID = safeText(currentInstance?.id || state.selectedInstanceID || 'current');
  const compareID = safeText(compareInstance?.id || state.compareInstanceID || 'compare');
  const lines = [`${currentID} vs ${compareID}`];

  const currentTension = Number(current.tension ?? 0);
  const compareTension = Number(compare.tension ?? 0);
  if (currentTension !== compareTension) {
    lines.push(`tension gap: ${currentTension.toFixed(2)} vs ${compareTension.toFixed(2)}`);
  }

  const currentTrajectory = Array.isArray(current.trajectory_summary) ? current.trajectory_summary : [];
  const compareTrajectory = Array.isArray(compare.trajectory_summary) ? compare.trajectory_summary : [];
  if (currentTrajectory[0] || compareTrajectory[0]) {
    lines.push(`trend: ${safeText(currentTrajectory[0] || '--')} | ${safeText(compareTrajectory[0] || '--')}`);
  }

  const currentPop = Array.isArray(current.population_highlights) ? current.population_highlights.join(' · ') : '';
  const comparePop = Array.isArray(compare.population_highlights) ? compare.population_highlights.join(' · ') : '';
  if (currentPop || comparePop) {
    lines.push(`population: ${safeText(currentPop || '--')} | ${safeText(comparePop || '--')}`);
  }

  const currentDiag = Array.isArray(current.trajectory_summary) ? current.trajectory_summary.find(line => String(line).startsWith('recent diagnostics:')) : '';
  const compareDiag = Array.isArray(compare.trajectory_summary) ? compare.trajectory_summary.find(line => String(line).startsWith('recent diagnostics:')) : '';
  if (currentDiag || compareDiag) {
    lines.push(`diagnostics: ${safeText(currentDiag || '--')} | ${safeText(compareDiag || '--')}`);
  }

  const currentPressure = Object.entries(current.pressure_states || {}).sort((a, b) => Number(b[1] || 0) - Number(a[1] || 0))[0];
  const comparePressure = Object.entries(compare.pressure_states || {}).sort((a, b) => Number(b[1] || 0) - Number(a[1] || 0))[0];
  if (currentPressure || comparePressure) {
    const currentText = currentPressure ? `${safeText(currentPressure[0])}:${Number(currentPressure[1] || 0).toFixed(2)}` : '--';
    const compareText = comparePressure ? `${safeText(comparePressure[0])}:${Number(comparePressure[1] || 0).toFixed(2)}` : '--';
    lines.push(`pressure lead: ${currentText} | ${compareText}`);
  }

  lines.push(...collectInstanceDiffDetails(current, compare, currentInstance, compareInstance));

  return lines;
}

function renderInstanceCompareSummary(current, compare) {
  if (!compare || !state.compareInstanceID) {
    els.instanceCompareSummary.textContent = '当前未启用实例对照';
    els.simInstanceCompareSummary.textContent = '当前未启用实例对照';
    els.simExperimentSummary.textContent = '当前未生成实验结论';
    return;
  }
  const currentInstance = state.instances.find(item => item.id === state.selectedInstanceID);
  const compareInstance = state.instances.find(item => item.id === state.compareInstanceID);
  const lines = collectInstanceOutcomeLines(current, compare, currentInstance, compareInstance);
  if (!lines.length) {
    els.instanceCompareSummary.textContent = '实例对照暂无可用结果';
    els.simInstanceCompareSummary.textContent = '实例对照暂无可用结果';
    return;
  }
  els.instanceCompareSummary.innerHTML = lines.slice(0, 2).map(line => `<div class="mono">${safeText(line)}</div>`).join('');
  els.simInstanceCompareSummary.innerHTML = lines.map(line => `<div class="note-box mono" style="margin-top:4px">${safeText(line)}</div>`).join('');
  els.simExperimentSummary.innerHTML = buildExperimentConclusion(current, compare, currentInstance, compareInstance)
    .map(line => `<div class="note-box mono" style="margin-top:4px">${safeText(line)}</div>`)
    .join('');
}

function buildExperimentConclusion(current, compare, currentInstance, compareInstance) {
  const currentID = safeText(currentInstance?.id || state.selectedInstanceID || 'current');
  const compareID = safeText(compareInstance?.id || state.compareInstanceID || 'compare');
  const lines = [];

  const currentTension = Number(current?.tension ?? 0);
  const compareTension = Number(compare?.tension ?? 0);
  if (currentTension !== compareTension) {
    const leader = currentTension > compareTension ? currentID : compareID;
    const gap = Math.abs(currentTension - compareTension).toFixed(2);
    lines.push(`长期张力主导：${leader}（gap ${gap}）`);
  } else {
    lines.push('长期张力主导：两侧接近');
  }

  const currentPop = Array.isArray(current?.population_highlights) ? current.population_highlights.join(' · ') : '';
  const comparePop = Array.isArray(compare?.population_highlights) ? compare.population_highlights.join(' · ') : '';
  if (currentPop !== comparePop) {
    const leader = currentPop.includes('promoted:') && !comparePop.includes('promoted:')
      ? currentID
      : comparePop.includes('promoted:') && !currentPop.includes('promoted:')
        ? compareID
        : '两侧都发生了不同人口演化';
    lines.push(`人口结果：${leader}`);
  } else if (currentPop) {
    lines.push(`人口结果：两侧一致 (${currentPop})`);
  }

  const currentDiag = Array.isArray(current?.trajectory_summary) ? current.trajectory_summary.find(line => String(line).startsWith('recent diagnostics:')) : '';
  const compareDiag = Array.isArray(compare?.trajectory_summary) ? compare.trajectory_summary.find(line => String(line).startsWith('recent diagnostics:')) : '';
  if (currentDiag || compareDiag) {
    const leader = (currentDiag || '').length > (compareDiag || '').length ? currentID : (compareDiag || '').length > (currentDiag || '').length ? compareID : '两侧';
    lines.push(`诊断密度：${leader}`);
  }

  const currentPressure = Object.entries(current?.pressure_states || {}).reduce((sum, [, value]) => sum + Number(value || 0), 0);
  const comparePressure = Object.entries(compare?.pressure_states || {}).reduce((sum, [, value]) => sum + Number(value || 0), 0);
  if (currentPressure !== comparePressure) {
    const leader = currentPressure > comparePressure ? currentID : compareID;
    lines.push(`结构压力主导：${leader}`);
  }

  return lines.length ? lines : ['当前实验尚未形成可解释分叉'];
}

function collectParticipantsFromInstance(instance) {
  const details = Array.isArray(instance?.participant_details) ? instance.participant_details : [];
  if (details.length) {
    return details.map(item => item.name).filter(Boolean);
  }
  return Array.isArray(instance?.participants) ? instance.participants.filter(Boolean) : [];
}

function collectParticipantDetails(statePayload, instance) {
  const details = Array.isArray(statePayload?.participant_details) ? statePayload.participant_details : [];
  if (details.length) {
    return details;
  }
  return Array.isArray(instance?.participant_details) ? instance.participant_details : [];
}

function buildExperimentSnapshot(status, instance, statePayload) {
  if (!status) return null;
  const participantDetails = collectParticipantDetails(statePayload, instance);
  const participants = participantDetails.length
    ? participantDetails.map(item => item.name).filter(Boolean)
    : collectParticipantsFromInstance(instance);
  return {
    instance_id: instance?.id || '',
    label: instance?.label || '',
    world_name: instance?.world_name || '',
    focus_character: statePayload?.focus_character || instance?.focus_character || '',
    participants,
    participant_details: participantDetails,
    scene_location: statePayload?.scene?.location || status.scene_control?.location || '',
    scene_description: statePayload?.scene?.description || '',
    tick_count: Number(status.tick_count ?? 0),
    turn_count: Number(status.turn_count ?? 0),
    tension: Number(statePayload?.tension ?? status.tension ?? 0),
    pressure_states: status.pressure_states || {},
    faction_tensions: status.faction_tensions || {},
    npc_tick_exposure: status.npc_tick_exposure || {},
    population_highlights: Array.isArray(status.population_highlights) ? status.population_highlights : [],
    diagnostics: Array.isArray(status.diagnostics) ? status.diagnostics : [],
    last_tick_summary: Array.isArray(status.last_tick_summary) ? status.last_tick_summary : [],
    tick_history: Array.isArray(status.tick_history) ? status.tick_history : [],
    trajectory_summary: Array.isArray(status.trajectory_summary) ? status.trajectory_summary : [],
    director_plan: statePayload?.director_plan || null,
    latest_trace: statePayload?.latest_trace || null,
  };
}

function makeDefaultExperimentReportName() {
  const source = String(state.selectedInstanceID || 'current').trim();
  const compare = String(state.compareInstanceID || 'solo').trim() || 'solo';
  const ticks = Number(state.lastSimStatus?.tick_count ?? 0);
  return `${source}-${compare}-${ticks}t`;
}

function buildExperimentCheckpointName(reportName, side) {
  const base = safeText(reportName, 'experiment').replace(/[^\w.-]+/g, '-').toLowerCase();
  const stamp = new Date().toISOString().replace(/[-:TZ.]/g, '').slice(0, 12);
  return `${base}-${side}-${stamp}`;
}

async function createCheckpointForInstance(instanceID, name, note) {
  const resp = instanceID === state.selectedInstanceID
    ? await apiFetch('/api/checkpoints', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, branch: els.timelineBranch.value || 'main', note }),
    })
    : await apiFetchForInstance('/api/checkpoints', instanceID, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, branch: 'main', note }),
    });
  if (!resp.ok) {
    const message = await resp.text().catch(() => '');
    throw new Error(message || `checkpoint create failed: ${resp.status}`);
  }
  return resp.json();
}

async function loadCheckpointIntoInstance(instanceID, checkpointName, label) {
  if (!instanceID || !checkpointName) {
    return;
  }
  const resp = instanceID === state.selectedInstanceID
    ? await apiFetch('/api/checkpoints/load', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: checkpointName }),
    })
    : await apiFetchForInstance('/api/checkpoints/load', instanceID, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: checkpointName }),
    });
  if (!resp.ok) {
    const message = await resp.text().catch(() => '');
    throw new Error(message || `checkpoint load failed: ${resp.status}`);
  }
  if (instanceID === state.selectedInstanceID) {
    await Promise.all([
      restoreDialogue(),
      refreshPanel(),
      loadTimeline(),
      loadMemoryView(),
      loadCharacterConfig(),
      loadSaveSlots(),
      loadScenarioPresets(),
      loadTraceHistory(),
      loadTraceView(),
      loadExperimentReports(),
      loadRuntimeAudit(),
    ]);
    renderSceneDivider(`已从实验归档恢复 ${safeText(label, checkpointName)}`);
    return;
  }
  await Promise.all([loadInstancesView(), loadExperimentReports(), loadRuntimeAudit(), loadCompareInstanceStatus(state.lastSimStatus)]);
}

async function replayExperimentReportIntoBranches(reportName, options = {}) {
  const name = String(reportName || '').trim();
  const switchToCurrent = options.switchToCurrent !== false;
  const notify = options.notify !== false;
  const quiet = options.quiet === true;
  if (!name) {
    if (notify) {
      alert('当前没有可复现的实验报告');
    }
    return null;
  }
  const resp = await apiFetch('/api/experiment-reports/replay', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  if (!resp.ok) {
    const message = await resp.text().catch(() => '');
    const error = new Error(message || resp.statusText || 'experiment replay failed');
    if (notify) {
      alert(`派生复现实验失败：${error.message}`);
    }
    if (options.throwOnError) {
      throw error;
    }
    return null;
  }
  const replay = await resp.json();
  const replayRun = await fetchExperimentReplayRun(replay).catch(() => ({ replay }));
  state.experimentReplayRuns[name] = replayRun;
  const currentID = String(replay.current_instance?.id || '').trim();
  const compareID = String(replay.compare_instance?.id || '').trim();
  if (compareID) {
    state.compareInstanceID = compareID;
    localStorage.setItem('corerp-compare-instance-id', compareID);
  }
  if (currentID && switchToCurrent) {
    renderSceneDivider(`已从实验归档 ${safeText(name)} 派生复现实例 ${safeText(currentID)}${compareID ? ` / ${safeText(compareID)}` : ''}`);
    await switchInstanceView(currentID);
    return replayRun;
  }
  if (!quiet) {
    renderSceneDivider(`已为实验归档 ${safeText(name)} 派生复现实例 ${safeText(currentID || '--')}${compareID ? ` / ${safeText(compareID)}` : ''}`);
  }
  await Promise.all([loadInstancesView(), refreshPanel(), loadRuntimeAudit(), loadExperimentReports()]);
  return replayRun;
}

async function fetchExperimentReplayRun(replay) {
  const currentID = String(replay?.current_instance?.id || '').trim();
  const compareID = String(replay?.compare_instance?.id || '').trim();
  const currentEvidence = replay?.current_evidence || null;
  const compareEvidence = replay?.compare_evidence || null;
  const currentEvidenceAudit = currentEvidence ? {
    sim_status: currentEvidence.sim_status || null,
    latest_trace: currentEvidence.latest_trace || null,
    population: currentEvidence.population || null,
    audit_summary: currentEvidence.audit_summary || [],
  } : null;
  const compareEvidenceAudit = compareEvidence ? {
    sim_status: compareEvidence.sim_status || null,
    latest_trace: compareEvidence.latest_trace || null,
    population: compareEvidence.population || null,
    audit_summary: compareEvidence.audit_summary || [],
  } : null;
  const currentStatus = currentID ? await fetchJSONForInstance('/api/sim/status', currentID).catch(() => currentEvidence?.sim_status || null) : null;
  const compareStatus = compareID ? await fetchJSONForInstance('/api/sim/status', compareID).catch(() => compareEvidence?.sim_status || null) : null;
  const auditURL = '/api/runtime-audit?trace_limit=6&checkpoint_limit=2&preset_limit=2&report_limit=2&population_limit=2';
  const currentAudit = currentID ? await fetchJSONForInstance(auditURL, currentID).catch(() => currentEvidenceAudit) : null;
  const compareAudit = compareID ? await fetchJSONForInstance(auditURL, compareID).catch(() => compareEvidenceAudit) : null;
  return {
    replay,
    currentStatus,
    compareStatus,
    currentAudit,
    compareAudit,
    updatedAt: new Date().toISOString(),
  };
}

async function refreshExperimentReplayRun(reportName) {
  const name = String(reportName || '').trim();
  const existing = state.experimentReplayRuns[name];
  if (!existing?.replay) {
    return replayExperimentReportIntoBranches(name);
  }
  state.experimentReplayRuns[name] = await fetchExperimentReplayRun(existing.replay).catch(() => existing);
  renderExperimentReports();
  renderRuntimeAudit();
  return state.experimentReplayRuns[name];
}

function buildReplayAdvanceEntry(reportName, replay) {
  const name = String(reportName || '').trim();
  const currentID = String(replay?.current_instance?.id || '').trim();
  const compareID = String(replay?.compare_instance?.id || '').trim();
  if (!name || !currentID) {
    return null;
  }
  return {
    report_name: name,
    world_name: String(replay?.world_name || replay?.current_instance?.world_name || replay?.compare_instance?.world_name || '').trim(),
    current_instance_id: currentID,
    compare_instance_id: compareID,
  };
}

async function advanceExperimentReplayRun(reportName, options = {}) {
  const name = String(reportName || '').trim();
  const count = Number(options.count || getBatchTickCount() || 1);
  if (!name) {
    alert('当前没有可推进的复现实验');
    return null;
  }
  let run = state.experimentReplayRuns[name];
  if (!run?.replay) {
    run = await replayExperimentReportIntoBranches(name, { switchToCurrent: false, notify: false, quiet: true, throwOnError: true });
  }
  if (!run?.replay) {
    throw new Error(`report ${name} replay unavailable`);
  }
  const entry = buildReplayAdvanceEntry(name, run.replay);
  if (!entry) {
    throw new Error(`report ${name} replay unavailable`);
  }
  const resp = await apiFetch('/api/experiment-reports/replay-advance', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      count,
      report_names: [name],
      replays: [entry],
    }),
  });
  if (!resp.ok) {
    const message = await resp.text().catch(() => '');
    throw new Error(message || resp.statusText || 'experiment replay advance failed');
  }
  const payload = await resp.json().catch(() => null);
  const failure = Array.isArray(payload?.failures) ? payload.failures.find(item => String(item?.report_name || '').trim() === name) : null;
  if (failure?.error) {
    throw new Error(failure.error);
  }
  const advancedReplay = (Array.isArray(payload?.results) ? payload.results.find(item => String(item?.report_name || '').trim() === name)?.replay : null) || run.replay;
  const refreshed = await fetchExperimentReplayRun(advancedReplay).catch(() => ({ ...run, replay: advancedReplay }));
  state.experimentReplayRuns[name] = refreshed;
  if ([run.replay?.current_instance?.id, run.replay?.compare_instance?.id].includes(String(state.selectedInstanceID || '').trim())) {
    await loadSimStatus();
  } else {
    await Promise.all([loadInstancesView(), loadExperimentReports(), loadRuntimeAudit()]);
    renderExperimentReports();
    renderRuntimeAudit();
  }
  return refreshed;
}

async function advanceExperimentReplayBatch(options = {}) {
  const count = Number(options.count || getBatchTickCount() || 1);
  const worldName = normalizeWorldName(options.worldName);
  const reportNames = Array.isArray(options.reportNames)
    ? options.reportNames.map(item => String(item || '').trim()).filter(Boolean)
    : [];
  let reports = filterReportsByWorld(options.reports || state.experimentReports, worldName);
  if (reportNames.length) {
    const include = new Set(reportNames);
    reports = reports.filter(report => include.has(String(report?.name || '').trim()));
  }
  const loadedReports = reports.filter(report => state.experimentReplayRuns?.[report.name]?.replay);
  if (!loadedReports.length) {
    alert(worldName ? `世界 ${worldName} 当前没有已加载的 replay branches` : '当前没有已加载的 replay branches');
    return;
  }
  const entries = loadedReports
    .map(report => buildReplayAdvanceEntry(String(report?.name || '').trim(), state.experimentReplayRuns?.[report.name]?.replay))
    .filter(Boolean);
  const resp = await apiFetch('/api/experiment-reports/replay-advance', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      world_name: worldName,
      report_names: reportNames,
      count,
      replays: entries,
    }),
  });
  if (!resp.ok) {
    const message = await resp.text().catch(() => '');
    throw new Error(message || resp.statusText || 'experiment replay advance batch failed');
  }
  const batch = await resp.json();
  const successes = Array.isArray(batch?.successes) ? batch.successes : [];
  const failureItems = Array.isArray(batch?.failures) ? batch.failures : [];
  state.experimentReplayBatchSummary = {
    mode: 'tick',
    count,
    worldName,
    total: Number(batch?.total || loadedReports.length),
    successes,
    failures: failureItems.map(item => ({ name: item?.report_name || '--', message: item?.error || 'unknown error' })),
    updatedAt: batch?.created_at || new Date().toISOString(),
  };
  const replayResults = Array.isArray(batch?.results) ? batch.results : [];
  await Promise.all(replayResults
    .filter(item => item?.replay && successes.includes(String(item?.report_name || '').trim()))
    .map(async item => {
      const reportName = String(item?.report_name || '').trim();
      if (!reportName) return;
      const existing = state.experimentReplayRuns?.[reportName] || {};
      state.experimentReplayRuns[reportName] = await fetchExperimentReplayRun(item.replay).catch(() => ({ ...existing, replay: item.replay }));
    }));
  await Promise.all([loadInstancesView(), loadExperimentReports(), loadRuntimeAudit()]);
  renderExperimentReports();
  renderRuntimeAudit();
  const scope = worldName ? `世界 ${worldName}` : '全部世界';
  renderSceneDivider(`批量推进 replay 完成：${scope} · ${count} ticks · 成功 ${successes.length}/${loadedReports.length}${failureItems.length ? ` · 失败 ${failureItems.length}` : ''}`);
}

async function openExperimentReplayInstance(reportName, side = 'current') {
  const name = String(reportName || '').trim();
  const targetSide = side === 'compare' ? 'compare' : 'current';
  if (!name) {
    alert('当前没有可打开的复现实验');
    return;
  }
  let run = state.experimentReplayRuns[name];
  if (!run?.replay) {
    run = await replayExperimentReportIntoBranches(name, { switchToCurrent: false, notify: false, quiet: true, throwOnError: true });
  }
  if (!run?.replay) {
    alert(`实验 ${name} 尚未生成 replay branches`);
    return;
  }
  const targetID = String(
    targetSide === 'compare'
      ? run.replay?.compare_instance?.id || ''
      : run.replay?.current_instance?.id || '',
  ).trim();
  if (!targetID) {
    alert(targetSide === 'compare' ? '当前 report 没有对照 replay instance' : '当前 report 没有 current replay instance');
    return;
  }
  if (targetSide === 'compare') {
    await switchCompareInstance(targetID);
    renderSceneDivider(`已将对照实例切到 replay ${safeText(targetID)}`);
    return;
  }
  await switchInstanceView(targetID);
  renderSceneDivider(`已切到 replay 实例 ${safeText(targetID)}`);
}

async function openReplayTraceTurn(instanceID, turn) {
  const id = String(instanceID || '').trim();
  const turnNumber = Number(turn || 0);
  if (!id || !turnNumber) {
    return;
  }
  if (state.selectedInstanceID !== id) {
    await switchInstanceView(id);
  }
  await Promise.all([selectTraceTurn(turnNumber), loadRuntimeAuditReplay(turnNumber, 0)]);
}

function flattenTraceEventIDs(trace) {
  const ids = [];
  (Array.isArray(trace?.events) ? trace.events : []).forEach(event => {
    if (event?.id) ids.push(String(event.id));
  });
  (Array.isArray(trace?.step_traces) ? trace.step_traces : []).forEach(step => {
    (Array.isArray(step?.events) ? step.events : []).forEach(event => {
      if (event?.id) ids.push(String(event.id));
    });
    (Array.isArray(step?.handoff?.events) ? step.handoff.events : []).forEach(event => {
      if (event?.id) ids.push(String(event.id));
    });
  });
  return ids;
}

function collectTraceEventEntries(events) {
  return (Array.isArray(events) ? events : [])
    .map((event, index) => {
      const id = String(event?.id || '').trim();
      if (!id) return null;
      return {
        id,
        index,
        type: String(event?.type || ''),
        actor: String(event?.actor || ''),
        target: String(event?.target || ''),
        branch: String(event?.branch || ''),
        canonical: Boolean(event?.canonical),
      };
    })
    .filter(Boolean);
}

function buildTraceStepSignature(step) {
  return [
    String(step?.speaker || ''),
    String(step?.action_frame?.action || step?.kind || ''),
    String(step?.reason || ''),
    step?.validator?.blocked ? `blocked:${String(step?.validator?.reason || '')}` : '',
  ].join('|');
}

function pickFirstSequenceDifference(currentEntries, compareEntries) {
  const maxLen = Math.max(currentEntries.length, compareEntries.length);
  for (let i = 0; i < maxLen; i++) {
    const current = currentEntries[i] || null;
    const compare = compareEntries[i] || null;
    if (!current && !compare) continue;
    if (!current || !compare) return current || compare;
    if (
      current.id !== compare.id ||
      current.type !== compare.type ||
      current.actor !== compare.actor ||
      current.target !== compare.target ||
      current.branch !== compare.branch ||
      current.canonical !== compare.canonical
    ) {
      return current || compare;
    }
  }
  return null;
}

function pickFirstAvailableTraceEvent(trace) {
  return flattenTraceEventIDs(trace)[0] || '';
}

function pickFirstAvailableStepEvent(step) {
  const stepEvent = collectTraceEventEntries(step?.events)[0];
  if (stepEvent?.id) return stepEvent.id;
  const handoffEvent = collectTraceEventEntries(step?.handoff?.events)[0];
  return handoffEvent?.id || '';
}

function describeTraceDivergence(currentTrace, compareTrace) {
  const rootDiff = pickFirstSequenceDifference(
    collectTraceEventEntries(currentTrace?.events),
    collectTraceEventEntries(compareTrace?.events),
  );
  if (rootDiff?.id) {
    return { differs: true, reason: 'trace events diverged', eventID: rootDiff.id, strict: true };
  }

  const currentSteps = Array.isArray(currentTrace?.step_traces) ? currentTrace.step_traces : [];
  const compareSteps = Array.isArray(compareTrace?.step_traces) ? compareTrace.step_traces : [];
  const maxSteps = Math.max(currentSteps.length, compareSteps.length);
  for (let i = 0; i < maxSteps; i++) {
    const currentStep = currentSteps[i] || null;
    const compareStep = compareSteps[i] || null;
    if (!currentStep || !compareStep) {
      const eventID = pickFirstAvailableStepEvent(currentStep || compareStep) || pickFirstAvailableTraceEvent(currentTrace) || pickFirstAvailableTraceEvent(compareTrace);
      return { differs: true, reason: `step ${i + 1} missing`, eventID, strict: Boolean(eventID) };
    }
    if (buildTraceStepSignature(currentStep) !== buildTraceStepSignature(compareStep)) {
      const eventID = pickFirstAvailableStepEvent(currentStep) || pickFirstAvailableStepEvent(compareStep) || pickFirstAvailableTraceEvent(currentTrace) || pickFirstAvailableTraceEvent(compareTrace);
      return { differs: true, reason: `step ${i + 1} speaker/action diverged`, eventID, strict: Boolean(eventID) };
    }
    const stepDiff = pickFirstSequenceDifference(
      collectTraceEventEntries(currentStep?.events),
      collectTraceEventEntries(compareStep?.events),
    );
    if (stepDiff?.id) {
      return { differs: true, reason: `step ${i + 1} events diverged`, eventID: stepDiff.id, strict: true };
    }
    const handoffDiff = pickFirstSequenceDifference(
      collectTraceEventEntries(currentStep?.handoff?.events),
      collectTraceEventEntries(compareStep?.handoff?.events),
    );
    if (handoffDiff?.id) {
      return { differs: true, reason: `step ${i + 1} handoff diverged`, eventID: handoffDiff.id, strict: true };
    }
  }

  const currentLead = Array.isArray(currentTrace?.director_plan?.selected) ? String(currentTrace.director_plan.selected[0] || '') : '';
  const compareLead = Array.isArray(compareTrace?.director_plan?.selected) ? String(compareTrace.director_plan.selected[0] || '') : '';
  const currentInput = String(currentTrace?.user_input || '');
  const compareInput = String(compareTrace?.user_input || '');
  if (currentLead !== compareLead || currentInput !== compareInput) {
    const eventID = pickFirstAvailableTraceEvent(currentTrace) || pickFirstAvailableTraceEvent(compareTrace);
    return { differs: true, reason: 'director lead or input diverged', eventID, strict: false };
  }

  return { differs: false, reason: '', eventID: '', strict: false };
}

function renderReplayTimelineHTML(replayRun) {
  const currentAudit = replayRun?.currentAudit || null;
  const compareAudit = replayRun?.compareAudit || null;
  const currentInstance = replayRun?.replay?.current_instance || null;
  const compareInstance = replayRun?.replay?.compare_instance || null;
  const currentID = safeText(currentInstance?.id || 'current');
  const compareID = safeText(compareInstance?.id || 'compare');
  const blocks = [];

  const currentHistory = Array.isArray(currentAudit?.sim_status?.tick_history) ? currentAudit.sim_status.tick_history : [];
  const compareHistory = Array.isArray(compareAudit?.sim_status?.tick_history) ? compareAudit.sim_status.tick_history : [];
  const historyByTickCurrent = new Map(currentHistory.map(item => [Number(item?.tick || 0), item]));
  const historyByTickCompare = new Map(compareHistory.map(item => [Number(item?.tick || 0), item]));
  const sharedTicks = [...new Set([...historyByTickCurrent.keys(), ...historyByTickCompare.keys()])].filter(Boolean).sort((a, b) => a - b);
  const tickLines = [];
  for (const tick of sharedTicks) {
    const current = historyByTickCurrent.get(tick);
    const compare = historyByTickCompare.get(tick);
    if (!current || !compare) continue;
    const currentTopPressure = pickTopMetric(current.pressure_states || {});
    const compareTopPressure = pickTopMetric(compare.pressure_states || {});
    const currentSummary = Array.isArray(current.summary) ? String(current.summary[0] || '') : '';
    const compareSummary = Array.isArray(compare.summary) ? String(compare.summary[0] || '') : '';
    if (
      Number(current.tension || 0) !== Number(compare.tension || 0) ||
      formatTopMetric(currentTopPressure) !== formatTopMetric(compareTopPressure) ||
      currentSummary !== compareSummary
    ) {
      tickLines.push(`<div class="mono" style="margin-top:4px">tick ${tick}: ${currentID}=tension ${Number(current.tension || 0).toFixed(2)} / ${safeText(formatTopMetric(currentTopPressure))} / ${safeText(currentSummary || '--')} | ${compareID}=tension ${Number(compare.tension || 0).toFixed(2)} / ${safeText(formatTopMetric(compareTopPressure))} / ${safeText(compareSummary || '--')}</div>`);
    }
  }
  if (tickLines.length) {
    blocks.push(`<div class="mono" style="margin-top:6px;font-weight:600">Divergence Ticks</div>${tickLines.slice(0, 3).join('')}`);
  }

  const currentTraces = Array.isArray(currentAudit?.recent_traces) ? currentAudit.recent_traces : [];
  const compareTraces = Array.isArray(compareAudit?.recent_traces) ? compareAudit.recent_traces : [];
  const currentTraceByTurn = new Map(currentTraces.map(trace => [Number(trace?.turn || 0), trace]));
  const compareTraceByTurn = new Map(compareTraces.map(trace => [Number(trace?.turn || 0), trace]));
  const sharedTurns = [...new Set([...currentTraceByTurn.keys(), ...compareTraceByTurn.keys()])].filter(Boolean).sort((a, b) => a - b);
  const turnLines = [];
  for (const turn of sharedTurns) {
    const current = currentTraceByTurn.get(turn);
    const compare = compareTraceByTurn.get(turn);
    if (!current || !compare) continue;
    const currentLead = Array.isArray(current.director_plan?.selected) ? String(current.director_plan.selected[0] || '') : '';
    const compareLead = Array.isArray(compare.director_plan?.selected) ? String(compare.director_plan.selected[0] || '') : '';
    const currentInput = String(current.user_input || '');
    const compareInput = String(compare.user_input || '');
    const divergence = describeTraceDivergence(current, compare);
    if (divergence.differs) {
      const causeLabel = divergence.strict ? '首个分叉事件' : '回退因果链';
      turnLines.push(`<div class="mono" style="margin-top:4px">turn ${turn}: ${currentID}=lead ${safeText(currentLead || '--')} / ${safeText(currentInput || '--')} <button type="button" class="ghost-button" data-replay-open-trace="${safeText(currentInstance?.id || '')}" data-replay-turn="${turn}">打开</button> | ${compareID}=lead ${safeText(compareLead || '--')} / ${safeText(compareInput || '--')} <button type="button" class="ghost-button" data-replay-open-trace="${safeText(compareInstance?.id || '')}" data-replay-turn="${turn}">打开</button> · ${safeText(divergence.reason)}${divergence.eventID ? ` <button type="button" class="ghost-button" data-replay-cause="${safeText(divergence.eventID)}">${causeLabel}</button>` : ''}</div>`);
    }
  }
  if (turnLines.length) {
    blocks.push(`<div class="mono" style="margin-top:6px;font-weight:600">Divergence Turns</div>${turnLines.slice(0, 3).join('')}`);
  }
  return blocks.join('');
}

function renderExperimentReplayRunSummary(report, replayRun) {
  if (!report || !replayRun?.replay) {
    return '';
  }
  const replay = replayRun.replay;
  const currentInstance = replay.current_instance || null;
  const compareInstance = replay.compare_instance || null;
  const currentStatus = replayRun.currentStatus || null;
  const compareStatus = replayRun.compareStatus || null;
  const currentAudit = replayRun.currentAudit || null;
  const compareAudit = replayRun.compareAudit || null;
  const timelineLines = collectReplayTimelineLines(currentAudit, compareAudit, currentInstance, compareInstance);
  const timelineHTML = renderReplayTimelineHTML(replayRun);
  const lines = currentStatus && compareStatus
    ? collectInstanceOutcomeLines(currentStatus, compareStatus, currentInstance, compareInstance)
    : currentStatus
      ? [
          `replay instance: ${safeText(currentInstance?.id || '--')}`,
          `trend: ${safeText((currentStatus.trajectory_summary || [])[0] || '--')}`,
          `population: ${safeText((currentStatus.population_highlights || []).join(' · ') || '--')}`,
        ]
      : [];
  const driverLines = collectReplayDriverLines(currentAudit, compareAudit, currentInstance, compareInstance);
  const conclusions = currentStatus && compareStatus
    ? buildExperimentConclusion(currentStatus, compareStatus, currentInstance, compareInstance)
    : [];
  return `
    <div class="note-box" style="margin-top:8px">
      <div class="mono" style="font-weight:600;margin-bottom:6px">Replay Branches</div>
      <div class="mono" style="margin-top:4px">${safeText(currentInstance?.id || '--')}${compareInstance?.id ? ` vs ${safeText(compareInstance.id)}` : ''}</div>
      ${replayRun.updatedAt ? `<div class="mono" style="margin-top:4px">updated: ${safeText(new Date(replayRun.updatedAt).toLocaleString('zh-CN'))}</div>` : ''}
      <div class="mono" style="margin-top:4px"><button type="button" class="ghost-button" data-report-advance-replay="${encodeURIComponent(String(report.name || ''))}">推进 replay ${getBatchTickCount()} ticks</button></div>
      ${lines.map(line => `<div class="mono" style="margin-top:4px">${safeText(line)}</div>`).join('')}
      ${driverLines.map(line => `<div class="mono" style="margin-top:4px">${safeText(line)}</div>`).join('')}
      ${timelineLines.map(line => `<div class="mono" style="margin-top:4px">${safeText(line)}</div>`).join('')}
      ${timelineHTML}
      ${conclusions.map(line => `<div class="mono" style="margin-top:4px">${safeText(line)}</div>`).join('')}
      <div class="mono" style="margin-top:4px"><button type="button" class="ghost-button" data-report-refresh-replay="${encodeURIComponent(String(report.name || ''))}">刷新复现结果</button></div>
    </div>
  `;
}

function collectSnapshotParticipants(snapshot) {
  if (!snapshot) return [];
  if (Array.isArray(snapshot.participant_details) && snapshot.participant_details.length) {
    return snapshot.participant_details.map(item => String(item?.name || '').trim()).filter(Boolean);
  }
  return Array.isArray(snapshot.participants) ? snapshot.participants.map(item => String(item || '').trim()).filter(Boolean) : [];
}

function collectExperimentSnapshotDiffLines(report) {
  const current = report?.current || null;
  const compare = report?.compare || null;
  if (!current || !compare) {
    return [];
  }
  const currentID = safeText(current.instance_id || report?.source_instance_id || 'current');
  const compareID = safeText(compare.instance_id || report?.compare_instance_id || 'compare');
  const lines = [];

  if (report?.current_checkpoint || report?.compare_checkpoint) {
    lines.push(`checkpoint anchors: ${currentID}=${safeText(report?.current_checkpoint || '--')} | ${compareID}=${safeText(report?.compare_checkpoint || '--')}`);
  }

  const currentScene = `${safeText(current.scene_location || '--')} / ${safeText(current.focus_character || '--')}`;
  const compareScene = `${safeText(compare.scene_location || '--')} / ${safeText(compare.focus_character || '--')}`;
  if (currentScene !== compareScene) {
    lines.push(`scene/focus: ${currentID}=${currentScene} | ${compareID}=${compareScene}`);
  }

  const currentParticipants = collectSnapshotParticipants(current).join(' · ');
  const compareParticipants = collectSnapshotParticipants(compare).join(' · ');
  if (currentParticipants !== compareParticipants) {
    lines.push(`participants: ${currentID}=${safeText(currentParticipants || '--')} | ${compareID}=${safeText(compareParticipants || '--')}`);
  }

  const currentTension = Number(current.tension || 0);
  const compareTension = Number(compare.tension || 0);
  if (currentTension !== compareTension) {
    const leader = currentTension > compareTension ? currentID : compareID;
    lines.push(`tension gap: ${leader} (${currentTension.toFixed(2)} vs ${compareTension.toFixed(2)})`);
  }

  const currentPressure = pickTopMetric(current.pressure_states || {});
  const comparePressure = pickTopMetric(compare.pressure_states || {});
  if (formatTopMetric(currentPressure) !== formatTopMetric(comparePressure)) {
    lines.push(`pressure lead: ${currentID}=${formatTopMetric(currentPressure)} | ${compareID}=${formatTopMetric(comparePressure)}`);
  }

  const currentFaction = pickTopMetric(current.faction_tensions || {});
  const compareFaction = pickTopMetric(compare.faction_tensions || {});
  if (formatTopMetric(currentFaction) !== formatTopMetric(compareFaction)) {
    lines.push(`faction lead: ${currentID}=${formatTopMetric(currentFaction)} | ${compareID}=${formatTopMetric(compareFaction)}`);
  }

  const currentExposure = pickTopMetric(current.npc_tick_exposure || {});
  const compareExposure = pickTopMetric(compare.npc_tick_exposure || {});
  if (formatTopMetric(currentExposure, 0) !== formatTopMetric(compareExposure, 0)) {
    lines.push(`exposure lead: ${currentID}=${formatTopMetric(currentExposure, 0)} | ${compareID}=${formatTopMetric(compareExposure, 0)}`);
  }

  const currentPop = Array.isArray(current.population_highlights) ? current.population_highlights.join(' · ') : '';
  const comparePop = Array.isArray(compare.population_highlights) ? compare.population_highlights.join(' · ') : '';
  if (currentPop !== comparePop) {
    lines.push(`population outcome: ${currentID}=${safeText(currentPop || '--')} | ${compareID}=${safeText(comparePop || '--')}`);
  }

  const currentDiag = Array.isArray(current.trajectory_summary) ? current.trajectory_summary.find(line => String(line).startsWith('recent diagnostics:')) || current.diagnostics?.[0] || '' : current.diagnostics?.[0] || '';
  const compareDiag = Array.isArray(compare.trajectory_summary) ? compare.trajectory_summary.find(line => String(line).startsWith('recent diagnostics:')) || compare.diagnostics?.[0] || '' : compare.diagnostics?.[0] || '';
  if (String(currentDiag) !== String(compareDiag)) {
    lines.push(`diagnostic split: ${currentID}=${safeText(currentDiag || '--')} | ${compareID}=${safeText(compareDiag || '--')}`);
  }

  const currentTrajectory = Array.isArray(current.trajectory_summary) ? String(current.trajectory_summary[0] || '') : '';
  const compareTrajectory = Array.isArray(compare.trajectory_summary) ? String(compare.trajectory_summary[0] || '') : '';
  if (currentTrajectory !== compareTrajectory) {
    lines.push(`trajectory split: ${currentID}=${safeText(currentTrajectory || '--')} | ${compareID}=${safeText(compareTrajectory || '--')}`);
  }

  const traceDivergence = describeTraceDivergence(current.latest_trace, compare.latest_trace);
  if (traceDivergence.differs) {
    lines.push(`latest trace divergence: ${safeText(traceDivergence.reason)}`);
  }
  const stepDiffLines = collectReplayStepDiffLines(current.latest_trace, compare.latest_trace, currentID, compareID)
    .filter(line => !String(line || '').includes('still aligned'))
    .slice(0, 2)
    .map(line => `latest trace ${line}`);
  lines.push(...stepDiffLines);

  return lines.slice(0, 10);
}

function getReplayEligibleReports(reports = state.experimentReports) {
  return (Array.isArray(reports) ? reports : []).filter(report => String(report?.current_checkpoint || '').trim());
}

function normalizeWorldName(value) {
  return String(value || '').trim();
}

function filterReportsByWorld(reports = state.experimentReports, worldName = '') {
  const target = normalizeWorldName(worldName);
  const items = Array.isArray(reports) ? reports : [];
  if (!target) {
    return items;
  }
  return items.filter(report => normalizeWorldName(report?.current?.world_name) === target);
}

function setExperimentWorldFilter(worldName = '') {
  state.experimentWorldFilter = normalizeWorldName(worldName);
}

function getActiveExperimentWorldFilter(fallback = '') {
  return normalizeWorldName(state.experimentWorldFilter || fallback);
}

function renderExperimentReplayBatchSummary() {
  const summary = state.experimentReplayBatchSummary;
  if (!summary) {
    return '';
  }
  const successes = Array.isArray(summary.successes) ? summary.successes : [];
  const failures = Array.isArray(summary.failures) ? summary.failures : [];
  const updatedAt = summary.updatedAt ? new Date(summary.updatedAt).toLocaleString('zh-CN') : '--';
  const mode = summary.mode === 'refresh'
    ? '批量刷新复现'
    : summary.mode === 'tick'
      ? '批量推进 replay'
      : '批量派生复现';
  const scope = summary.worldName ? `world ${safeText(summary.worldName)}` : 'all worlds';
  const successText = successes.length ? successes.map(item => safeText(item)).join(' · ') : '--';
  const failureText = failures.length ? failures.map(item => `${safeText(item.name)}:${safeText(item.message)}`).join(' · ') : '无';
  return `
    <div class="note-box" style="margin-bottom:8px">
      <div class="mono" style="font-weight:600">${mode}</div>
      <div class="mono" style="margin-top:4px">scope: ${scope}</div>
      <div class="mono" style="margin-top:4px">updated: ${safeText(updatedAt)}${summary.mode === 'tick' ? ` · ticks ${Number(summary.count || 0)}` : ''} · success ${successes.length}/${Number(summary.total || 0)} · fail ${failures.length}</div>
      <div class="mono" style="margin-top:4px">success: ${safeText(successText)}</div>
      <div class="mono" style="margin-top:4px">failures: ${safeText(failureText)}</div>
    </div>
  `;
}

function buildExperimentPortfolioRow(report, replayRun) {
  const current = report?.current || null;
  const compare = report?.compare || null;
  const currentWorld = safeText(current?.world_name || '--');
  const batch = Number(report?.batch_count || 0);
  const currentTension = Number(current?.tension || 0);
  const compareTension = Number(compare?.tension || 0);
  const archivedLeader = compare
    ? currentTension === compareTension
      ? 'archived tie'
      : currentTension > compareTension
        ? `archived ${safeText(current?.instance_id || report?.source_instance_id || 'current')}`
        : `archived ${safeText(compare?.instance_id || report?.compare_instance_id || 'compare')}`
    : `archived ${safeText(current?.instance_id || report?.source_instance_id || 'current')}`;
  const archivedTrend = safeText((current?.trajectory_summary || [])[0] || (compare?.trajectory_summary || [])[0] || '--');
  const archivedPop = safeText((current?.population_highlights || [])[0] || (compare?.population_highlights || [])[0] || '--');

  if (!replayRun?.replay) {
    return `report ${safeText(report?.name || '--')} · world ${currentWorld} · batch ${batch} · ${archivedLeader} · ${archivedTrend} · ${archivedPop} · replay pending`;
  }

  const liveCurrent = replayRun.currentStatus || null;
  const liveCompare = replayRun.compareStatus || null;
  const replayCurrentID = safeText(replayRun.replay?.current_instance?.id || '--');
  const replayCompareID = safeText(replayRun.replay?.compare_instance?.id || '--');
  const liveLeader = liveCurrent && liveCompare
    ? Number(liveCurrent.tension || 0) === Number(liveCompare.tension || 0)
      ? 'live tie'
      : Number(liveCurrent.tension || 0) > Number(liveCompare.tension || 0)
        ? `live ${replayCurrentID}`
        : `live ${replayCompareID}`
    : `live ${replayCurrentID}`;
  const liveTrend = liveCurrent && liveCompare
    ? `${safeText((liveCurrent.trajectory_summary || [])[0] || '--')} | ${safeText((liveCompare.trajectory_summary || [])[0] || '--')}`
    : safeText((liveCurrent?.trajectory_summary || [])[0] || '--');
  return `report ${safeText(report?.name || '--')} · world ${currentWorld} · batch ${batch} · ${archivedLeader} · ${liveLeader} · replay ${replayCurrentID}${liveCompare ? `/${replayCompareID}` : ''} · ${liveTrend}`;
}

function collectExperimentWorldBaselineSummaries(reports = state.experimentReports, replayRuns = state.experimentReplayRuns) {
  const groups = new Map();
  (Array.isArray(reports) ? reports : []).forEach(report => {
    const world = String(report?.current?.world_name || '--').trim() || '--';
    if (!groups.has(world)) {
      groups.set(world, []);
    }
    groups.get(world).push(report);
  });
  return [...groups.entries()]
    .sort((a, b) => a[0].localeCompare(b[0], 'zh-CN'))
    .map(([world, items]) => {
      const sortedItems = items
        .slice()
        .sort((a, b) => String(b?.created_at || '').localeCompare(String(a?.created_at || '')));
      const currentTensions = items.map(report => Number(report?.current?.tension || 0));
      const minTension = currentTensions.length ? Math.min(...currentTensions) : 0;
      const maxTension = currentTensions.length ? Math.max(...currentTensions) : 0;
      const tensionRange = maxTension - minTension;
      const trajectorySet = new Set(items.map(report => String((report?.current?.trajectory_summary || [])[0] || '').trim()).filter(Boolean));
      const populationSet = new Set(items.map(report => String((report?.current?.population_highlights || [])[0] || '').trim()).filter(Boolean));
      const compareCount = items.filter(report => report?.compare).length;
      const replayLoaded = items.filter(report => replayRuns?.[report.name]?.replay).length;
      const archivedDiverged = items.filter(report => report?.compare && Number(report?.current?.tension || 0) !== Number(report?.compare?.tension || 0)).length;
      const liveDiverged = items.filter(report => {
        const run = replayRuns?.[report.name];
        return Boolean(run?.currentStatus && run?.compareStatus && Number(run.currentStatus.tension || 0) !== Number(run.compareStatus.tension || 0));
      }).length;
      const baselineStatus = tensionRange >= 1.5 || trajectorySet.size >= 3 || archivedDiverged >= Math.max(2, Math.ceil(compareCount * 0.6))
        ? '分叉'
        : tensionRange >= 0.6 || trajectorySet.size >= 2 || populationSet.size >= 2
          ? '波动'
          : '稳定';
      const latest = sortedItems[0] || null;
      const previous = sortedItems[1] || null;
      const latestTrend = latest
        ? safeText((latest.current?.trajectory_summary || [])[0] || (latest.compare?.trajectory_summary || [])[0] || '--')
        : '--';
      const latestReplayTrend = latest && replayRuns?.[latest.name]?.currentStatus
        ? safeText((replayRuns[latest.name].currentStatus.trajectory_summary || [])[0] || '--')
        : '--';
      const latestCurrentTension = Number(latest?.current?.tension || 0);
      const previousCurrentTension = Number(previous?.current?.tension || 0);
      const tensionDrift = previous ? `${previousCurrentTension.toFixed(2)} -> ${latestCurrentTension.toFixed(2)}` : '--';
      const previousTrend = previous
        ? safeText((previous.current?.trajectory_summary || [])[0] || (previous.compare?.trajectory_summary || [])[0] || '--')
        : '--';
      const latestPop = latest ? safeText((latest.current?.population_highlights || [])[0] || '--') : '--';
      const previousPop = previous ? safeText((previous.current?.population_highlights || [])[0] || '--') : '--';
      return {
        world_name: world,
        status: baselineStatus,
        report_count: items.length,
        compare_count: compareCount,
        replay_loaded: replayLoaded,
        archived_split_count: archivedDiverged,
        live_split_count: liveDiverged,
        tension_min: Number(minTension.toFixed(2)),
        tension_max: Number(maxTension.toFixed(2)),
        tension_drift: tensionDrift,
        trajectory_variants: trajectorySet.size,
        population_variants: populationSet.size,
        previous_trend: previousTrend,
        latest_trend: latestTrend,
        previous_population: previousPop,
        latest_population: latestPop,
        latest_live_trend: latestReplayTrend,
        latest_report_name: String(latest?.name || ''),
      };
    });
}

function buildExperimentWorldBaselineRows(reports = state.experimentReports, replayRuns = state.experimentReplayRuns) {
  return collectExperimentWorldBaselineSummaries(reports, replayRuns)
    .map(item => `world ${safeText(item.world_name)} · status ${safeText(item.status)} · reports ${item.report_count} · compare ${item.compare_count} · replay ${item.replay_loaded} · archived split ${item.archived_split_count}/${item.compare_count || 0} · live split ${item.live_split_count}/${item.replay_loaded || 0} · tension range ${Number(item.tension_min || 0).toFixed(2)}-${Number(item.tension_max || 0).toFixed(2)} · tension drift ${safeText(item.tension_drift)} · trend variants ${item.trajectory_variants} · trend ${safeText(item.previous_trend)} -> ${safeText(item.latest_trend)} · population variants ${item.population_variants} · population ${safeText(item.previous_population)} -> ${safeText(item.latest_population)} · latest live ${safeText(item.latest_live_trend)}`);
}

function renderExperimentWorldBaselineSummary(reports = state.experimentReports, replayRuns = state.experimentReplayRuns) {
  const items = collectExperimentWorldBaselineSummaries(reports, replayRuns);
  if (!items.length) {
    return '';
  }
  return `
    <div class="note-box" style="margin-bottom:8px">
      <div class="mono" style="font-weight:600">World Baselines</div>
      ${items.map(item => `
        <div class="mono" style="margin-top:6px">
          world ${safeText(item.world_name)} · status ${safeText(item.status)} · reports ${item.report_count} · compare ${item.compare_count} · replay ${item.replay_loaded} · archived split ${item.archived_split_count}/${item.compare_count || 0} · live split ${item.live_split_count}/${item.replay_loaded || 0} · tension range ${Number(item.tension_min || 0).toFixed(2)}-${Number(item.tension_max || 0).toFixed(2)} · tension drift ${safeText(item.tension_drift)} · trend variants ${item.trajectory_variants} · trend ${safeText(item.previous_trend)} -> ${safeText(item.latest_trend)} · population variants ${item.population_variants} · population ${safeText(item.previous_population)} -> ${safeText(item.latest_population)} · latest live ${safeText(item.latest_live_trend)}
        </div>
        <div class="mono" style="margin-top:4px">
          <button type="button" class="ghost-button" data-world-baseline-focus="${encodeURIComponent(String(item.world_name || ''))}">聚焦该世界</button>
          <button type="button" class="ghost-button" data-world-baseline-replay="${encodeURIComponent(String(item.world_name || ''))}">派生该世界</button>
          <button type="button" class="ghost-button" data-world-baseline-refresh="${encodeURIComponent(String(item.world_name || ''))}">刷新该世界</button>
          <button type="button" class="ghost-button" data-world-baseline-export-json="${encodeURIComponent(String(item.world_name || ''))}">导出 JSON</button>
          <button type="button" class="ghost-button" data-world-baseline-export-md="${encodeURIComponent(String(item.world_name || ''))}">导出 MD</button>
        </div>
      `).join('')}
    </div>
  `;
}

function renderCurrentWorldBaselineGapSummary(audit, reports = state.experimentReports, replayRuns = state.experimentReplayRuns) {
  const gap = collectCurrentWorldBaselineGap(audit, reports, replayRuns);
  if (!gap) {
    return '';
  }
  return `
    <div class="note-box" style="margin-top:8px">
      <div class="mono" style="font-weight:600">Current vs World Baseline</div>
      <div class="mono" style="margin-top:4px">world ${safeText(gap.worldName)} · baseline ${safeText(gap.baseline.status)} · latest report ${safeText(gap.latestReport?.name || gap.baseline.latest_report_name || '--')}</div>
      <div class="mono" style="margin-top:4px">tension: current ${gap.currentTension.toFixed(2)} vs baseline ${gap.baselineMin.toFixed(2)}-${gap.baselineMax.toFixed(2)} · ${safeText(gap.tensionState)}</div>
      <div class="mono" style="margin-top:4px">trend gap: ${safeText(gap.trendGap)}</div>
      <div class="mono" style="margin-top:4px">population gap: ${safeText(gap.populationGap)}</div>
      <div class="mono" style="margin-top:6px">
        <button type="button" class="ghost-button" data-world-baseline-focus="${encodeURIComponent(String(gap.worldName || ''))}">聚焦该世界</button>
      </div>
    </div>
  `;
}

function collectCurrentWorldBaselineGap(audit, reports = state.experimentReports, replayRuns = state.experimentReplayRuns) {
  const worldName = String(audit?.instance?.world_name || audit?.state?.world_name || '').trim();
  const currentStatus = state.lastSimStatus || null;
  if (!worldName || !currentStatus) {
    return null;
  }
  const baseline = collectExperimentWorldBaselineSummaries(reports, replayRuns)
    .find(item => String(item.world_name || '').trim() === worldName);
  if (!baseline) {
    return null;
  }
  const latestReport = (Array.isArray(reports) ? reports : [])
    .filter(report => String(report?.current?.world_name || '').trim() === worldName)
    .sort((a, b) => String(b?.created_at || '').localeCompare(String(a?.created_at || '')))[0] || null;
  const currentTension = Number(currentStatus.tension || 0);
  const baselineMin = Number(baseline.tension_min || 0);
  const baselineMax = Number(baseline.tension_max || 0);
  let tensionState = '区间内';
  if (currentTension < baselineMin) tensionState = '低于基线';
  if (currentTension > baselineMax) tensionState = '高于基线';
  const latestArchivedTrend = safeText(baseline.latest_trend || '--');
  const latestArchivedPopulation = safeText(baseline.latest_population || '--');
  const currentTrend = safeText((currentStatus.trajectory_summary || [])[0] || '--');
  const currentPopulation = safeText((currentStatus.population_highlights || [])[0] || '--');
  return {
    worldName,
    baseline,
    latestReport,
    currentTension,
    baselineMin,
    baselineMax,
    currentTrend,
    currentPopulation,
    tensionState,
    trendGap: currentTrend === latestArchivedTrend ? '对齐' : `${latestArchivedTrend} -> ${currentTrend}`,
    populationGap: currentPopulation === latestArchivedPopulation ? '对齐' : `${latestArchivedPopulation} -> ${currentPopulation}`,
  };
}

function summarizeCheckpointDelta(slot, audit) {
  if (!slot || !audit?.state) {
    return [];
  }
  const lines = [];
  const checkpointState = slot.world_state || {};
  const liveState = audit.state || {};

  const checkpointScene = `${safeText(checkpointState?.scene?.location || '--')} / ${safeText(slot.focus_character || '--')}`;
  const liveScene = `${safeText(liveState?.scene?.location || '--')} / ${safeText(audit.focus_character || '--')}`;
  if (checkpointScene !== liveScene) {
    lines.push(`scene/focus: checkpoint=${checkpointScene} | live=${liveScene}`);
  }

  const checkpointTension = Number(checkpointState?.tension || 0);
  const liveTension = Number(liveState?.tension || 0);
  if (checkpointTension !== liveTension) {
    lines.push(`tension: checkpoint ${checkpointTension.toFixed(2)} | live ${liveTension.toFixed(2)}`);
  }

  const checkpointWeather = safeText(checkpointState?.scene?.weather || '--');
  const liveWeather = safeText(liveState?.scene?.weather || '--');
  if (checkpointWeather !== liveWeather) {
    lines.push(`weather: checkpoint ${checkpointWeather} | live ${liveWeather}`);
  }

  const checkpointFlags = Object.keys(checkpointState?.flags || {}).filter(key => checkpointState.flags[key]);
  const liveFlags = Object.keys(liveState?.flags || {}).filter(key => liveState.flags[key]);
  if (checkpointFlags.join('|') !== liveFlags.join('|')) {
    lines.push(`active flags: checkpoint ${safeText(checkpointFlags.join(' · ') || '--')} | live ${safeText(liveFlags.join(' · ') || '--')}`);
  }

  return lines.slice(0, 4);
}

function renderRuntimeAuditCheckpointBrowser(audit) {
  const checkpoints = Array.isArray(audit?.checkpoints) ? audit.checkpoints : [];
  if (!checkpoints.length) {
    return '';
  }
  const selectedName = String(state.runtimeAuditCheckpointName || checkpoints[0]?.name || '').trim();
  const selected = checkpoints.find(slot => String(slot?.name || '').trim() === selectedName) || checkpoints[0] || null;
  if (!selected) {
    return '';
  }
  if (selectedName !== state.runtimeAuditCheckpointName) {
    state.runtimeAuditCheckpointName = String(selected?.name || '');
  }
  const checkpointState = selected.world_state || {};
  const diffLines = summarizeCheckpointDelta(selected, audit);
  const chips = checkpoints.map(slot => {
    const active = String(slot?.name || '') === String(selected?.name || '')
      ? ' style="background:rgba(198,89,17,.18);border-color:rgba(198,89,17,.5)"'
      : '';
    return `<button type="button" class="ghost-button" data-audit-checkpoint-name="${encodeURIComponent(String(slot?.name || ''))}"${active}>${safeText(slot?.name || '--')}</button>`;
  }).join(' ');
  return `
    <div class="note-box" style="margin-top:8px">
      <div class="mono" style="font-weight:600;margin-bottom:6px">Checkpoint Browser</div>
      <div class="mono" style="margin-top:4px">${chips}</div>
      <div class="mono" style="margin-top:6px">selected ${safeText(selected.name)} · branch ${safeText(selected.branch || '--')} · focus ${safeText(selected.focus_character || '--')} · ${selected.created_at ? new Date(selected.created_at).toLocaleString('zh-CN') : '--'}</div>
      ${selected.note ? `<div class="mono" style="margin-top:4px">note: ${safeText(selected.note)}</div>` : ''}
      ${selected.preview ? `<div class="mono" style="margin-top:4px">preview: ${safeText(selected.preview)}</div>` : ''}
      <div class="mono" style="margin-top:4px">scene ${safeText(checkpointState?.scene?.location || '--')} · ${safeText(checkpointState?.scene?.time_of_day || '--')} · ${safeText(checkpointState?.scene?.weather || '--')}</div>
      <div class="mono" style="margin-top:4px">player role ${safeText(selected.player_role?.name || '--')} → ${safeText(selected.player_role?.bound_character || '--')}</div>
      <div class="mono" style="margin-top:4px">world state: tension ${Number(checkpointState?.tension || 0).toFixed(2)} · flags ${Object.keys(checkpointState?.flags || {}).filter(key => checkpointState.flags[key]).length} · vars ${Object.keys(checkpointState?.variables || {}).length}</div>
      <div class="mono" style="margin-top:6px"><button type="button" class="ghost-button" data-audit-checkpoint-restore="${encodeURIComponent(String(selected.name || ''))}">恢复这个 checkpoint</button></div>
      ${diffLines.length ? `<div class="mono" style="margin-top:8px;font-weight:600">Checkpoint vs Live</div>${diffLines.map(line => `<div class="mono" style="margin-top:4px">${safeText(line)}</div>`).join('')}` : ''}
    </div>
  `;
}

function selectExperimentReportContext(reportName, reports = state.experimentReports) {
  const name = String(reportName || '').trim();
  if (!name) {
    return null;
  }
  const items = Array.isArray(reports) ? reports : [];
  const report = items.find(item => String(item?.name || '').trim() === name) || null;
  state.runtimeAuditReportName = name;
  if (report) {
    setExperimentWorldFilter(String(report?.current?.world_name || ''));
    state.runtimeAuditCheckpointName = String(report?.current_checkpoint || report?.compare_checkpoint || state.runtimeAuditCheckpointName || '').trim();
  }
  return report;
}

function renderExperimentWorldOpsSummary(reports = state.experimentReports, replayRuns = state.experimentReplayRuns, options = {}) {
  const activeWorld = getActiveExperimentWorldFilter(options.worldName);
  if (!activeWorld) {
    return '';
  }
  const scopedReports = filterReportsByWorld(reports, activeWorld);
  const baselines = collectExperimentWorldBaselineSummaries(reports, replayRuns);
  const baseline = baselines.find(item => normalizeWorldName(item.world_name) === activeWorld) || null;
  const replayEligible = getReplayEligibleReports(scopedReports).length;
  const latest = scopedReports
    .slice()
    .sort((a, b) => String(b?.created_at || '').localeCompare(String(a?.created_at || '')))[0] || null;
  const selectedReport = scopedReports.find(report => String(report?.name || '') === String(state.runtimeAuditReportName || '')) || latest;
  const selectedReplay = selectedReport ? (replayRuns?.[selectedReport.name]?.replay || null) : null;
  const availableCheckpoints = [...new Set(scopedReports.flatMap(report => [report?.current_checkpoint, report?.compare_checkpoint]).map(item => String(item || '').trim()).filter(Boolean))];
  const selectedCheckpointName = availableCheckpoints.includes(String(state.runtimeAuditCheckpointName || '').trim())
    ? String(state.runtimeAuditCheckpointName || '').trim()
    : String(selectedReport?.current_checkpoint || selectedReport?.compare_checkpoint || availableCheckpoints[0] || '').trim();
  const runtimeAuditCheckpoints = Array.isArray(state.runtimeAudit?.checkpoints) ? state.runtimeAudit.checkpoints : [];
  const selectedCheckpoint = runtimeAuditCheckpoints.find(slot => String(slot?.name || '').trim() === selectedCheckpointName) || null;
  const reportChips = scopedReports
    .slice()
    .sort((a, b) => String(b?.created_at || '').localeCompare(String(a?.created_at || '')))
    .slice(0, 6)
    .map(report => {
      const active = String(report?.name || '') === String(selectedReport?.name || '')
        ? ' style="background:rgba(198,89,17,.18);border-color:rgba(198,89,17,.5)"'
        : '';
      return `<button type="button" class="ghost-button" data-world-ops-report-view="${encodeURIComponent(String(report?.name || ''))}"${active}>${safeText(report?.name || '--')}</button>`;
    })
    .join(' ');
  const checkpointChips = availableCheckpoints
    .slice(0, 6)
    .map(name => {
      const active = name === selectedCheckpointName
        ? ' style="background:rgba(198,89,17,.18);border-color:rgba(198,89,17,.5)"'
        : '';
      return `<button type="button" class="ghost-button" data-world-ops-checkpoint="${encodeURIComponent(name)}"${active}>${safeText(name)}</button>`;
    })
    .join(' ');
  const recentRows = scopedReports
    .slice()
    .sort((a, b) => String(b?.created_at || '').localeCompare(String(a?.created_at || '')))
    .slice(0, 4)
    .map(report => {
      const replay = replayRuns?.[report.name]?.replay || null;
      const replayLabel = replay?.current_instance?.id
        ? `${safeText(replay.current_instance.id)}${replay.compare_instance?.id ? ` / ${safeText(replay.compare_instance.id)}` : ''}`
        : 'replay pending';
      return `
        <div class="mono" style="margin-top:4px">
          report ${safeText(report.name)} · ${report.created_at ? new Date(report.created_at).toLocaleString('zh-CN') : '--'} · checkpoint ${safeText(report.current_checkpoint || '--')} / ${safeText(report.compare_checkpoint || '无对照')} · ${safeText(replayLabel)}
          <button type="button" class="ghost-button" data-world-ops-report-view="${encodeURIComponent(String(report.name || ''))}">查看</button>
          ${report.current_checkpoint ? `<button type="button" class="ghost-button" data-report-replay-instance="${encodeURIComponent(String(report.name || ''))}">派生</button>` : ''}
          <button type="button" class="ghost-button" data-world-ops-report-export="${encodeURIComponent(String(report.name || ''))}">导出 MD</button>
        </div>
      `;
    })
    .join('');
  return `
    <div class="note-box" style="margin-bottom:8px">
      <div class="mono" style="font-weight:600">World Experiment Ops</div>
      <div class="mono" style="margin-top:4px">scope ${safeText(activeWorld)} · reports ${scopedReports.length} · replay ready ${replayEligible} · baseline ${safeText(baseline?.status || '--')}</div>
      <div class="mono" style="margin-top:4px">latest report ${safeText(latest?.name || baseline?.latest_report_name || '--')} · trend ${safeText(baseline?.latest_trend || '--')} · population ${safeText(baseline?.latest_population || '--')}</div>
      ${reportChips ? `<div class="mono" style="margin-top:8px;font-weight:600">Focused Report</div><div class="mono" style="margin-top:4px">${reportChips}</div>` : ''}
      ${selectedReport ? `<div class="mono" style="margin-top:4px">selected ${safeText(selectedReport.name)} · checkpoint ${safeText(selectedReport.current_checkpoint || '--')} / ${safeText(selectedReport.compare_checkpoint || '无对照')} · replay ${safeText(selectedReplay?.current_instance?.id || 'pending')}${selectedReplay?.compare_instance?.id ? ` / ${safeText(selectedReplay.compare_instance.id)}` : ''}</div>` : ''}
      ${checkpointChips ? `<div class="mono" style="margin-top:8px;font-weight:600">Focused Checkpoint</div><div class="mono" style="margin-top:4px">${checkpointChips}</div>` : ''}
      ${selectedCheckpoint ? `<div class="mono" style="margin-top:4px">checkpoint ${safeText(selectedCheckpoint.name)} · focus ${safeText(selectedCheckpoint.focus_character || '--')} · scene ${safeText(selectedCheckpoint.world_state?.scene?.location || '--')} · tension ${Number(selectedCheckpoint.world_state?.tension || 0).toFixed(2)}</div>` : (selectedCheckpointName ? `<div class="mono" style="margin-top:4px">checkpoint ${safeText(selectedCheckpointName)} · 当前 runtime audit 未加载该锚点详情</div>` : '')}
      ${recentRows ? `<div class="mono" style="margin-top:8px;font-weight:600">Recent Reports</div>${recentRows}` : ''}
      <div class="mono" style="margin-top:6px">
        <button type="button" class="ghost-button" data-world-baseline-replay="${encodeURIComponent(String(activeWorld || ''))}">派生该世界</button>
        <button type="button" class="ghost-button" data-world-baseline-refresh="${encodeURIComponent(String(activeWorld || ''))}">刷新该世界</button>
        <button type="button" class="ghost-button" data-world-baseline-export-json="${encodeURIComponent(String(activeWorld || ''))}">导出 JSON</button>
        <button type="button" class="ghost-button" data-world-baseline-export-md="${encodeURIComponent(String(activeWorld || ''))}">导出 MD</button>
        <button type="button" class="ghost-button" data-world-baseline-clear="1">取消聚焦</button>
      </div>
    </div>
  `;
}

function renderWorldExperimentPanel(audit, reports = state.experimentReports, replayRuns = state.experimentReplayRuns, options = {}) {
  const activeWorld = getActiveExperimentWorldFilter(options.worldName || String(audit?.instance?.world_name || audit?.state?.world_name || ''));
  if (!activeWorld) {
    return '';
  }
  const scopedReports = filterReportsByWorld(reports, activeWorld);
  const baselines = collectExperimentWorldBaselineSummaries(reports, replayRuns);
  const baseline = baselines.find(item => normalizeWorldName(item.world_name) === activeWorld) || null;
  const sortedReports = scopedReports
    .slice()
    .sort((a, b) => String(b?.created_at || '').localeCompare(String(a?.created_at || '')));
  const latest = sortedReports[0] || null;
  const selectedReport = scopedReports.find(report => String(report?.name || '') === String(state.runtimeAuditReportName || '')) || latest;
  const selectedReplay = selectedReport ? (replayRuns?.[selectedReport.name] || null) : null;
  const runtimeAuditCheckpoints = Array.isArray(audit?.checkpoints) ? audit.checkpoints : [];
  const availableCheckpoints = [...new Set(scopedReports.flatMap(report => [report?.current_checkpoint, report?.compare_checkpoint]).map(item => String(item || '').trim()).filter(Boolean))];
  const selectedCheckpointName = availableCheckpoints.includes(String(state.runtimeAuditCheckpointName || '').trim())
    ? String(state.runtimeAuditCheckpointName || '').trim()
    : String(selectedReport?.current_checkpoint || selectedReport?.compare_checkpoint || availableCheckpoints[0] || '').trim();
  const selectedCheckpoint = runtimeAuditCheckpoints.find(slot => String(slot?.name || '').trim() === selectedCheckpointName) || null;
  const selectedReplayState = selectedReplay?.currentStatus || null;
  const selectedCompareReplayState = selectedReplay?.compareStatus || null;
  const selectedCurrent = selectedReport?.current || null;
  const selectedCompare = selectedReport?.compare || null;
  const gap = collectCurrentWorldBaselineGap(audit, reports, replayRuns);
  const checkpointDiffLines = selectedCheckpoint ? summarizeCheckpointDelta(selectedCheckpoint, audit) : [];
  const reportDiffLines = selectedReport ? collectExperimentSnapshotDiffLines(selectedReport).slice(0, 4) : [];
  const timelineLines = selectedReplay ? collectReplayTimelineLines(
    selectedReplay.currentAudit,
    selectedReplay.compareAudit,
    selectedReplay.replay?.current_instance,
    selectedReplay.replay?.compare_instance,
  ).slice(0, 3) : [];
  const actions = `
    <button type="button" class="ghost-button" data-world-baseline-replay="${encodeURIComponent(String(activeWorld || ''))}">派生该世界</button>
    <button type="button" class="ghost-button" data-world-baseline-refresh="${encodeURIComponent(String(activeWorld || ''))}">刷新该世界</button>
    <button type="button" class="ghost-button" data-world-replay-advance="${encodeURIComponent(String(activeWorld || ''))}">推进该世界 replay</button>
    <button type="button" class="ghost-button" data-world-baseline-export-json="${encodeURIComponent(String(activeWorld || ''))}">导出 JSON</button>
    <button type="button" class="ghost-button" data-world-baseline-export-md="${encodeURIComponent(String(activeWorld || ''))}">导出 MD</button>
    <button type="button" class="ghost-button" data-world-proof-export-json="${encodeURIComponent(String(activeWorld || ''))}">证据包 JSON</button>
    <button type="button" class="ghost-button" data-world-proof-export-md="${encodeURIComponent(String(activeWorld || ''))}">证据包 MD</button>
    <button type="button" class="ghost-button" data-world-baseline-clear="1">取消聚焦</button>
  `;
  return `
    <div class="note-box" style="margin-bottom:8px">
      <div class="mono" style="font-weight:600">World Experiment Panel</div>
      <div class="mono" style="margin-top:4px">scope ${safeText(activeWorld)} · reports ${scopedReports.length} · baseline ${safeText(baseline?.status || '--')} · replay loaded ${scopedReports.filter(report => replayRuns?.[report.name]?.replay).length}</div>
      <div class="mono" style="margin-top:4px">live instance ${safeText(audit?.instance?.id || state.selectedInstanceID || '--')} · focus ${safeText(audit?.focus_character || audit?.state?.focus_character || '--')} · scene ${safeText(audit?.state?.scene?.location || '--')} · tension ${Number(audit?.state?.tension || 0).toFixed(2)}</div>
      <div class="mono" style="margin-top:6px">${actions}</div>
      ${gap && gap.worldName === activeWorld ? `
        <div class="mono" style="margin-top:8px;font-weight:600">Live vs Baseline</div>
        <div class="mono" style="margin-top:4px">current ${gap.currentTension.toFixed(2)} vs baseline ${gap.baselineMin.toFixed(2)}-${gap.baselineMax.toFixed(2)} · ${safeText(gap.tensionState)}</div>
        <div class="mono" style="margin-top:4px">trend ${safeText(gap.trendGap)} · population ${safeText(gap.populationGap)}</div>
      ` : ''}
      ${selectedReport ? `
        <div class="mono" style="margin-top:8px;font-weight:600">Focused Report</div>
        <div class="mono" style="margin-top:4px">${safeText(selectedReport.name)} · ${selectedReport.created_at ? new Date(selectedReport.created_at).toLocaleString('zh-CN') : '--'} · ${safeText(selectedReport.source_instance_id || '--')}${selectedReport.compare_instance_id ? ` vs ${safeText(selectedReport.compare_instance_id)}` : ''}</div>
        <div class="mono" style="margin-top:4px">archived current ${safeText(selectedCurrent?.focus_character || '--')} @ ${safeText(selectedCurrent?.scene_location || '--')} · tension ${Number(selectedCurrent?.tension || 0).toFixed(2)}${selectedCompare ? ` | compare ${safeText(selectedCompare?.focus_character || '--')} @ ${safeText(selectedCompare?.scene_location || '--')} · ${Number(selectedCompare?.tension || 0).toFixed(2)}` : ''}</div>
        <div class="mono" style="margin-top:4px">
          <button type="button" class="ghost-button" data-world-ops-report-view="${encodeURIComponent(String(selectedReport.name || ''))}">选中报告</button>
          <button type="button" class="ghost-button" data-audit-report-replay-instance="${encodeURIComponent(String(selectedReport.name || ''))}">派生复现</button>
          ${selectedReplay?.replay ? `<button type="button" class="ghost-button" data-report-refresh-replay="${encodeURIComponent(String(selectedReport.name || ''))}">刷新复现</button>` : ''}
          ${selectedReplay?.replay ? `<button type="button" class="ghost-button" data-report-advance-replay="${encodeURIComponent(String(selectedReport.name || ''))}">推进 replay</button>` : ''}
          ${selectedReplay?.replay?.current_instance?.id ? `<button type="button" class="ghost-button" data-report-open-replay-current="${encodeURIComponent(String(selectedReport.name || ''))}">打开 current</button>` : ''}
          ${selectedReplay?.replay?.compare_instance?.id ? `<button type="button" class="ghost-button" data-report-open-replay-compare="${encodeURIComponent(String(selectedReport.name || ''))}">打开 compare</button>` : ''}
          <button type="button" class="ghost-button" data-audit-report-name="${encodeURIComponent(String(selectedReport.name || ''))}">导出 MD</button>
        </div>
        ${reportDiffLines.length ? reportDiffLines.map(line => `<div class="mono" style="margin-top:4px">${safeText(line)}</div>`).join('') : ''}
      ` : ''}
      ${selectedCheckpointName ? `
        <div class="mono" style="margin-top:8px;font-weight:600">Focused Checkpoint</div>
        <div class="mono" style="margin-top:4px">
          <button type="button" class="ghost-button" data-audit-checkpoint-open="${encodeURIComponent(String(selectedCheckpointName || ''))}">${safeText(selectedCheckpointName)}</button>
          ${selectedCheckpoint ? `<button type="button" class="ghost-button" data-audit-checkpoint-restore="${encodeURIComponent(String(selectedCheckpoint.name || ''))}">恢复该锚点</button>` : ''}
        </div>
        ${selectedCheckpoint ? `
          <div class="mono" style="margin-top:4px">focus ${safeText(selectedCheckpoint.focus_character || '--')} · scene ${safeText(selectedCheckpoint.world_state?.scene?.location || '--')} · tension ${Number(selectedCheckpoint.world_state?.tension || 0).toFixed(2)}</div>
          ${checkpointDiffLines.length ? checkpointDiffLines.map(line => `<div class="mono" style="margin-top:4px">${safeText(line)}</div>`).join('') : '<div class="mono" style="margin-top:4px">checkpoint 与 live 当前一致</div>'}
        ` : `<div class="mono" style="margin-top:4px">runtime audit 当前未加载 ${safeText(selectedCheckpointName)} 详情</div>`}
      ` : ''}
      ${selectedReplay?.replay ? `
        <div class="mono" style="margin-top:8px;font-weight:600">Focused Replay</div>
        <div class="mono" style="margin-top:4px">${safeText(selectedReplay.replay?.current_instance?.id || '--')}${selectedReplay.replay?.compare_instance?.id ? ` vs ${safeText(selectedReplay.replay.compare_instance.id)}` : ''} · ${selectedReplay.updatedAt ? new Date(selectedReplay.updatedAt).toLocaleString('zh-CN') : '--'}</div>
        <div class="mono" style="margin-top:4px">
          <button type="button" class="ghost-button" data-report-advance-replay="${encodeURIComponent(String(selectedReport?.name || ''))}">推进当前 replay ${getBatchTickCount()} ticks</button>
          ${selectedReplay.replay?.current_instance?.id ? `<button type="button" class="ghost-button" data-report-open-replay-current="${encodeURIComponent(String(selectedReport?.name || ''))}">切到 current</button>` : ''}
          ${selectedReplay.replay?.compare_instance?.id ? `<button type="button" class="ghost-button" data-report-open-replay-compare="${encodeURIComponent(String(selectedReport?.name || ''))}">切到 compare</button>` : ''}
        </div>
        <div class="mono" style="margin-top:4px">live trend ${safeText((selectedReplayState?.trajectory_summary || [])[0] || '--')}${selectedCompareReplayState ? ` | ${safeText((selectedCompareReplayState?.trajectory_summary || [])[0] || '--')}` : ''}</div>
        <div class="mono" style="margin-top:4px">live population ${safeText((selectedReplayState?.population_highlights || [])[0] || '--')}${selectedCompareReplayState ? ` | ${safeText((selectedCompareReplayState?.population_highlights || [])[0] || '--')}` : ''}</div>
        ${timelineLines.length ? timelineLines.map(line => `<div class="mono" style="margin-top:4px">${safeText(line)}</div>`).join('') : ''}
      ` : selectedReport ? `<div class="mono" style="margin-top:8px">selected report replay pending</div>` : ''}
    </div>
  `;
}

function collectExperimentPortfolioSnapshot(reports = state.experimentReports, replayRuns = state.experimentReplayRuns) {
  const items = Array.isArray(reports) ? reports : [];
  const worldBaselines = collectExperimentWorldBaselineSummaries(items, replayRuns);
  const portfolioRows = items
    .slice()
    .sort((a, b) => String(b?.created_at || '').localeCompare(String(a?.created_at || '')))
    .map(report => {
      const replayRun = replayRuns?.[report.name] || null;
      return {
        name: String(report?.name || ''),
        created_at: String(report?.created_at || ''),
        world_name: String(report?.current?.world_name || ''),
        batch_count: Number(report?.batch_count || 0),
        source_instance_id: String(report?.source_instance_id || ''),
        compare_instance_id: String(report?.compare_instance_id || ''),
        archived_summary: buildExperimentPortfolioRow(report, null),
        live_summary: replayRun?.replay ? buildExperimentPortfolioRow(report, replayRun) : 'replay pending',
      };
    });
  return {
    created_at: new Date().toISOString(),
    report_count: items.length,
    world_count: worldBaselines.length,
    replay_loaded_count: items.filter(report => replayRuns?.[report.name]?.replay).length,
    world_baselines: worldBaselines,
    reports: portfolioRows,
  };
}

function formatExperimentBaselineMarkdown(snapshot) {
  const lines = [
    '# Experiment Baselines',
    '',
    `- Created At: ${snapshot.created_at || '--'}`,
    `- Reports: ${Number(snapshot.report_count || 0)}`,
    `- Worlds: ${Number(snapshot.world_count || 0)}`,
    `- Replay Loaded: ${Number(snapshot.replay_loaded_count || 0)}`,
    '',
    '## World Baselines',
  ];
  (Array.isArray(snapshot.world_baselines) ? snapshot.world_baselines : []).forEach(item => {
    lines.push(`- ${item.world_name || '--'} | status ${item.status || '--'} | reports ${item.report_count || 0} | compare ${item.compare_count || 0} | replay ${item.replay_loaded || 0} | archived split ${item.archived_split_count || 0}/${item.compare_count || 0} | live split ${item.live_split_count || 0}/${item.replay_loaded || 0} | tension range ${Number(item.tension_min || 0).toFixed(2)}-${Number(item.tension_max || 0).toFixed(2)} | drift ${item.tension_drift || '--'} | trend ${item.previous_trend || '--'} -> ${item.latest_trend || '--'} | population ${item.previous_population || '--'} -> ${item.latest_population || '--'} | latest live ${item.latest_live_trend || '--'}`);
  });
  lines.push('', '## Portfolio');
  (Array.isArray(snapshot.reports) ? snapshot.reports : []).forEach(item => {
    lines.push(`- ${item.name || '--'} | ${item.world_name || '--'} | batch ${item.batch_count || 0} | ${item.live_summary || item.archived_summary || '--'}`);
  });
  return lines.join('\n');
}

function collectWorldExperimentProofBundle(audit, options = {}) {
  const worldName = normalizeWorldName(options.worldName || getActiveExperimentWorldFilter(String(audit?.instance?.world_name || audit?.state?.world_name || '')));
  const reports = filterReportsByWorld(state.experimentReports, worldName);
  const baselineSnapshot = collectExperimentPortfolioSnapshot(reports, state.experimentReplayRuns);
  const baseline = (Array.isArray(baselineSnapshot.world_baselines) ? baselineSnapshot.world_baselines : [])[0] || null;
  const selectedReportName = String(options.reportName || state.runtimeAuditReportName || '').trim();
  const selectedReport = reports.find(report => String(report?.name || '').trim() === selectedReportName) || reports[0] || null;
  const replayRun = selectedReport ? (state.experimentReplayRuns?.[selectedReport.name] || null) : null;
  const runtimeAuditCheckpoints = Array.isArray(audit?.checkpoints) ? audit.checkpoints : [];
  const checkpointName = String(
    options.checkpointName
      || state.runtimeAuditCheckpointName
      || selectedReport?.current_checkpoint
      || selectedReport?.compare_checkpoint
      || '',
  ).trim();
  const selectedCheckpoint = runtimeAuditCheckpoints.find(slot => String(slot?.name || '').trim() === checkpointName) || null;
  const gap = collectCurrentWorldBaselineGap(audit, state.experimentReports, state.experimentReplayRuns);
  return {
    created_at: new Date().toISOString(),
    world_name: worldName,
    live_instance: audit?.instance || null,
    live_focus_character: audit?.focus_character || audit?.state?.focus_character || '',
    live_state: audit?.state || null,
    baseline_snapshot: baselineSnapshot,
    baseline_summary: baseline,
    live_vs_baseline: gap && gap.worldName === worldName ? gap : null,
    selected_report: selectedReport || null,
    report_diff_lines: selectedReport ? collectExperimentSnapshotDiffLines(selectedReport) : [],
    selected_checkpoint: selectedCheckpoint || null,
    checkpoint_diff_lines: selectedCheckpoint ? summarizeCheckpointDelta(selectedCheckpoint, audit) : [],
    replay_run: replayRun || null,
    replay_timeline_lines: replayRun ? collectReplayTimelineLines(
      replayRun.currentAudit,
      replayRun.compareAudit,
      replayRun.replay?.current_instance,
      replayRun.replay?.compare_instance,
    ) : [],
    batch_summary: state.experimentReplayBatchSummary || null,
  };
}

function formatWorldExperimentProofBundleMarkdown(bundle) {
  const lines = [
    '# World Experiment Proof Bundle',
    '',
    `- Created At: ${bundle.created_at || '--'}`,
    `- World: ${bundle.world_name || '--'}`,
    `- Live Instance: ${bundle.live_instance?.id || '--'}`,
    `- Live Focus: ${bundle.live_focus_character || '--'}`,
  ];
  if (bundle.live_state) {
    lines.push(`- Live Scene: ${bundle.live_state.scene?.location || '--'}`);
    lines.push(`- Live Tension: ${Number(bundle.live_state.tension || 0).toFixed(2)}`);
  }
  if (bundle.live_vs_baseline) {
    lines.push('');
    lines.push('## Live vs Baseline');
    lines.push(`- Baseline Status: ${bundle.live_vs_baseline.baseline?.status || '--'}`);
    lines.push(`- Latest Report: ${bundle.live_vs_baseline.latestReport?.name || bundle.live_vs_baseline.baseline?.latest_report_name || '--'}`);
    lines.push(`- Tension: ${bundle.live_vs_baseline.currentTension.toFixed(2)} vs ${bundle.live_vs_baseline.baselineMin.toFixed(2)}-${bundle.live_vs_baseline.baselineMax.toFixed(2)} (${bundle.live_vs_baseline.tensionState || '--'})`);
    lines.push(`- Trend Gap: ${bundle.live_vs_baseline.trendGap || '--'}`);
    lines.push(`- Population Gap: ${bundle.live_vs_baseline.populationGap || '--'}`);
  }
  lines.push('');
  lines.push('## World Baselines');
  (Array.isArray(bundle.baseline_snapshot?.world_baselines) ? bundle.baseline_snapshot.world_baselines : []).forEach(item => {
    lines.push(`- ${item.world_name || '--'} | status ${item.status || '--'} | reports ${item.report_count || 0} | replay ${item.replay_loaded || 0} | trend ${item.previous_trend || '--'} -> ${item.latest_trend || '--'} | population ${item.previous_population || '--'} -> ${item.latest_population || '--'} | latest live ${item.latest_live_trend || '--'}`);
  });
  lines.push('', '## Portfolio');
  (Array.isArray(bundle.baseline_snapshot?.reports) ? bundle.baseline_snapshot.reports : []).forEach(item => {
    lines.push(`- ${item.name || '--'} | ${item.world_name || '--'} | ${item.live_summary || item.archived_summary || '--'}`);
  });
  if (bundle.selected_report) {
    lines.push('', `## Selected Report: ${bundle.selected_report.name || '--'}`, '');
    lines.push(`- Source Instance: ${bundle.selected_report.source_instance_id || '--'}`);
    lines.push(`- Compare Instance: ${bundle.selected_report.compare_instance_id || '--'}`);
    lines.push(`- Current Checkpoint: ${bundle.selected_report.current_checkpoint || '--'}`);
    lines.push(`- Compare Checkpoint: ${bundle.selected_report.compare_checkpoint || '--'}`);
    (Array.isArray(bundle.report_diff_lines) ? bundle.report_diff_lines : []).forEach(line => lines.push(`- ${line}`));
  }
  if (bundle.selected_checkpoint) {
    lines.push('', `## Selected Checkpoint: ${bundle.selected_checkpoint.name || '--'}`, '');
    lines.push(`- Branch: ${bundle.selected_checkpoint.branch || '--'}`);
    lines.push(`- Focus: ${bundle.selected_checkpoint.focus_character || '--'}`);
    lines.push(`- Scene: ${bundle.selected_checkpoint.world_state?.scene?.location || '--'}`);
    lines.push(`- Tension: ${Number(bundle.selected_checkpoint.world_state?.tension || 0).toFixed(2)}`);
    (Array.isArray(bundle.checkpoint_diff_lines) ? bundle.checkpoint_diff_lines : []).forEach(line => lines.push(`- ${line}`));
  }
  if (bundle.replay_run?.replay) {
    lines.push('', '## Replay Run', '');
    lines.push(`- Current Replay: ${bundle.replay_run.replay?.current_instance?.id || '--'}`);
    lines.push(`- Compare Replay: ${bundle.replay_run.replay?.compare_instance?.id || '--'}`);
    lines.push(`- Updated At: ${bundle.replay_run.updatedAt || '--'}`);
    (Array.isArray(bundle.replay_timeline_lines) ? bundle.replay_timeline_lines : []).forEach(line => lines.push(`- ${line}`));
  }
  if (bundle.batch_summary) {
    lines.push('', '## Batch Summary', '');
    lines.push(`- Mode: ${bundle.batch_summary.mode || '--'}`);
    lines.push(`- Scope: ${bundle.batch_summary.worldName || '--'}`);
    lines.push(`- Count: ${Number(bundle.batch_summary.count || 0)}`);
    lines.push(`- Success: ${(bundle.batch_summary.successes || []).join(' · ') || '--'}`);
    lines.push(`- Failures: ${(bundle.batch_summary.failures || []).map(item => `${item.name}:${item.message}`).join(' · ') || '无'}`);
  }
  return lines.join('\n');
}

function exportWorldExperimentProofBundle(format, options = {}) {
  const bundle = collectWorldExperimentProofBundle(state.runtimeAudit, options);
  const stamp = new Date().toISOString().replace(/[:.]/g, '-');
  const worldSuffix = bundle.world_name ? `-${bundle.world_name.replace(/[^\w.-]+/g, '-')}` : '';
  if (format === 'json') {
    downloadText(`world-experiment-proof${worldSuffix}-${stamp}.json`, JSON.stringify(bundle, null, 2), 'application/json;charset=utf-8');
    return;
  }
  downloadText(`world-experiment-proof${worldSuffix}-${stamp}.md`, formatWorldExperimentProofBundleMarkdown(bundle), 'text/markdown;charset=utf-8');
}

function exportExperimentBaselines(format, options = {}) {
  const worldName = normalizeWorldName(options.worldName);
  const reports = filterReportsByWorld(state.experimentReports, worldName);
  const snapshot = collectExperimentPortfolioSnapshot(reports, state.experimentReplayRuns);
  const stamp = new Date().toISOString().replace(/[:.]/g, '-');
  const worldSuffix = worldName ? `-${worldName.replace(/[^\w.-]+/g, '-')}` : '';
  if (format === 'json') {
    downloadText(`experiment-baselines${worldSuffix}-${stamp}.json`, JSON.stringify(snapshot, null, 2), 'application/json;charset=utf-8');
    return;
  }
  downloadText(`experiment-baselines${worldSuffix}-${stamp}.md`, formatExperimentBaselineMarkdown(snapshot), 'text/markdown;charset=utf-8');
}

function renderExperimentPortfolioSummary(reports = state.experimentReports, replayRuns = state.experimentReplayRuns) {
  const items = Array.isArray(reports) ? reports : [];
  if (!items.length) {
    return '';
  }
  const compareCount = items.filter(report => report?.compare).length;
  const replayLoaded = items.filter(report => replayRuns?.[report.name]?.replay).length;
  const worlds = [...new Set(items.map(report => String(report?.current?.world_name || '').trim()).filter(Boolean))];
  const archivedDiverged = items.filter(report => {
    const current = Number(report?.current?.tension || 0);
    const compare = Number(report?.compare?.tension || 0);
    return report?.compare && current !== compare;
  }).length;
  const liveDiverged = items.filter(report => {
    const run = replayRuns?.[report.name];
    if (!run?.currentStatus || !run?.compareStatus) return false;
    return Number(run.currentStatus.tension || 0) !== Number(run.compareStatus.tension || 0);
  }).length;
  const rows = items
    .slice()
    .sort((a, b) => String(b?.created_at || '').localeCompare(String(a?.created_at || '')))
    .map(report => `<div class="mono" style="margin-top:4px">${safeText(buildExperimentPortfolioRow(report, replayRuns?.[report.name]))}</div>`)
    .join('');
  return `
    <div class="note-box" style="margin-bottom:8px">
      <div class="mono" style="font-weight:600">Experiment Portfolio</div>
      <div class="mono" style="margin-top:4px">reports ${items.length} · compare ${compareCount} · replay loaded ${replayLoaded} · worlds ${worlds.length}</div>
      <div class="mono" style="margin-top:4px">archived tension split ${archivedDiverged}/${compareCount || 0} · live tension split ${liveDiverged}/${replayLoaded || 0}</div>
      <div class="mono" style="margin-top:4px">worlds: ${safeText(worlds.join(' · ') || '--')}</div>
      ${rows}
    </div>
  `;
}

async function replayExperimentReportsBatch(options = {}) {
  const mode = options.mode === 'refresh' ? 'refresh' : 'replay';
  const worldName = normalizeWorldName(options.worldName);
  const scopedReports = filterReportsByWorld(options.reports || state.experimentReports, worldName);
  const reports = getReplayEligibleReports(scopedReports);
  if (!reports.length) {
    alert(worldName ? `世界 ${worldName} 当前没有带 checkpoint 的实验报告` : '当前没有带 checkpoint 的实验报告');
    return;
  }
  const successes = [];
  const failures = [];
  if (mode === 'replay') {
    const reportNames = reports.map(report => String(report?.name || '').trim()).filter(Boolean);
    const resp = await apiFetch('/api/experiment-reports/replay-batch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        world_name: worldName,
        report_names: reportNames,
      }),
    });
    if (!resp.ok) {
      const message = await resp.text().catch(() => '');
      throw new Error(message || resp.statusText || 'experiment replay batch failed');
    }
    const batch = await resp.json();
    const results = Array.isArray(batch?.results) ? batch.results : [];
    for (const item of results) {
      const name = String(item?.report_name || '').trim();
      if (!name) continue;
      if (item?.replay) {
        const replayRun = await fetchExperimentReplayRun(item.replay).catch(() => ({ replay: item.replay }));
        state.experimentReplayRuns[name] = replayRun;
        successes.push(name);
      } else {
        failures.push({ name, message: item?.error || 'unknown error' });
      }
    }
  } else {
    for (const report of reports) {
      const name = String(report?.name || '').trim();
      if (!name) continue;
      try {
        const existing = state.experimentReplayRuns[name];
        if (existing?.replay) {
          state.experimentReplayRuns[name] = await fetchExperimentReplayRun(existing.replay).catch(() => existing);
        } else {
          await replayExperimentReportIntoBranches(name, { switchToCurrent: false, notify: false, quiet: true, throwOnError: true });
        }
        successes.push(name);
      } catch (err) {
        failures.push({ name, message: err?.message || 'unknown error' });
      }
    }
  }
  state.experimentReplayBatchSummary = {
    mode,
    worldName,
    total: reports.length,
    successes,
    failures,
    updatedAt: new Date().toISOString(),
  };
  await Promise.all([loadInstancesView(), loadExperimentReports(), loadRuntimeAudit()]);
  renderExperimentReports();
  renderRuntimeAudit();
  const label = mode === 'refresh' ? '批量刷新复现' : '批量派生复现';
  const scope = worldName ? `世界 ${worldName}` : '全部世界';
  renderSceneDivider(`${label}完成：${scope} · 成功 ${successes.length}/${reports.length}${failures.length ? ` · 失败 ${failures.length}` : ''}`);
}

async function buildCurrentExperimentReportPayload() {
  const currentStatus = state.lastSimStatus;
  if (!currentStatus) {
    return null;
  }
  const currentInstance = state.instances.find(item => item.id === state.selectedInstanceID) || null;
  const compareInstance = state.instances.find(item => item.id === state.compareInstanceID) || null;
  const compareStatus = state.compareSimStatus;
  const currentStatePayload = await fetchJSON('/api/state').catch(() => null);
  const compareStatePayload = compareStatus && state.compareInstanceID
    ? await fetchJSONForInstance('/api/state', state.compareInstanceID).catch(() => null)
    : null;
  const outcomeSummary = compareStatus
    ? collectInstanceOutcomeLines(currentStatus, compareStatus, currentInstance, compareInstance)
    : [
        `instance: ${safeText(currentInstance?.id || state.selectedInstanceID || 'current')}`,
        `trend: ${safeText((currentStatus.trajectory_summary || [])[0] || '--')}`,
        `population: ${safeText((currentStatus.population_highlights || []).join(' · ') || '--')}`,
      ];
  const conclusion = compareStatus
    ? buildExperimentConclusion(currentStatus, compareStatus, currentInstance, compareInstance)
    : ['当前未启用实例对照，已归档当前实例长期状态'];
  return {
    name: els.simReportName.value.trim() || makeDefaultExperimentReportName(),
    note: els.simReportNote.value.trim(),
    batch_count: getBatchTickCount(),
    source_instance_id: String(state.selectedInstanceID || '').trim(),
    compare_instance_id: String(state.compareInstanceID || '').trim(),
    outcome_summary: outcomeSummary,
    conclusion,
    current: buildExperimentSnapshot(currentStatus, currentInstance, currentStatePayload),
    compare: compareStatus ? buildExperimentSnapshot(compareStatus, compareInstance, compareStatePayload) : null,
  };
}

function formatExperimentReportMarkdown(report) {
  const lines = [
    `# Experiment Report: ${report.name || '--'}`,
    '',
    `- Created At: ${report.created_at || '--'}`,
    `- Source Instance: ${report.source_instance_id || report.current?.instance_id || '--'}`,
    `- Compare Instance: ${report.compare_instance_id || report.compare?.instance_id || '--'}`,
    `- Batch Count: ${report.batch_count ?? 0}`,
  ];
  if (report.current_checkpoint) {
    lines.push(`- Current Checkpoint: ${report.current_checkpoint}`);
  }
  if (report.compare_checkpoint) {
    lines.push(`- Compare Checkpoint: ${report.compare_checkpoint}`);
  }
  if (report.note) {
    lines.push(`- Note: ${report.note}`);
  }
  lines.push('', '## Outcome Summary');
  (Array.isArray(report.outcome_summary) ? report.outcome_summary : ['--']).forEach(line => lines.push(`- ${line}`));
  lines.push('', '## Conclusion');
  (Array.isArray(report.conclusion) ? report.conclusion : ['--']).forEach(line => lines.push(`- ${line}`));

  const appendSnapshot = (title, snapshot) => {
    if (!snapshot) return;
    lines.push('', `## ${title}`, '');
    lines.push(`- Instance: ${snapshot.instance_id || '--'}`);
    lines.push(`- World: ${snapshot.world_name || '--'}`);
    lines.push(`- Focus: ${snapshot.focus_character || '--'}`);
    lines.push(`- Location: ${snapshot.scene_location || '--'}`);
    lines.push(`- Scene: ${snapshot.scene_description || '--'}`);
    lines.push(`- Tick/Turn: ${snapshot.tick_count ?? 0} / ${snapshot.turn_count ?? 0}`);
    lines.push(`- Tension: ${Number(snapshot.tension ?? 0).toFixed(2)}`);
    lines.push(`- Participants: ${(snapshot.participants || []).join(', ') || '--'}`);
    if (Array.isArray(snapshot.participant_details) && snapshot.participant_details.length) {
      lines.push(`- Participant Details: ${snapshot.participant_details.map(item => `${item.name}:${item.kind || '--'}/${item.source || '--'}`).join(' ; ')}`);
    }
    if (snapshot.director_plan?.selected?.length) {
      lines.push(`- Director: ${snapshot.director_plan.mode || 'manual'} -> ${snapshot.director_plan.selected.join(' -> ')}`);
    }
    if (Array.isArray(snapshot.director_plan?.world_signals) && snapshot.director_plan.world_signals.length) {
      lines.push(`- Director Signals: ${snapshot.director_plan.world_signals.join(' | ')}`);
    }
    if (snapshot.latest_trace) {
      lines.push(`- Latest Trace: turn ${snapshot.latest_trace.turn || 0} · ${snapshot.latest_trace.focus_character || '--'} · ${snapshot.latest_trace.user_input || '--'}`);
    }
    lines.push('', 'Trajectory:');
    (Array.isArray(snapshot.trajectory_summary) ? snapshot.trajectory_summary : ['--']).forEach(line => lines.push(`- ${line}`));
  };

  appendSnapshot('Current Snapshot', report.current);
  appendSnapshot('Compare Snapshot', report.compare);
  return lines.join('\n');
}

function exportExperimentReport(report, format) {
  if (!report) {
    alert('当前没有可导出的实验报告');
    return;
  }
  const name = safeText(report.name, 'experiment-report').replace(/[^\w.-]+/g, '-');
  if (format === 'json') {
    downloadText(`${name}.json`, JSON.stringify(report, null, 2), 'application/json;charset=utf-8');
    return;
  }
  downloadText(`${name}.md`, formatExperimentReportMarkdown(report), 'text/markdown;charset=utf-8');
}

function renderExperimentReports() {
  const allReports = Array.isArray(state.experimentReports) ? state.experimentReports : [];
  const activeWorld = getActiveExperimentWorldFilter();
  const reports = filterReportsByWorld(allReports, activeWorld);
  const worldOpsHTML = renderExperimentWorldOpsSummary(allReports, state.experimentReplayRuns, { worldName: activeWorld });
  const worldBaselineHTML = renderExperimentWorldBaselineSummary(allReports, state.experimentReplayRuns);
  const portfolioHTML = renderExperimentPortfolioSummary(reports, state.experimentReplayRuns);
  const batchSummaryHTML = renderExperimentReplayBatchSummary();
  renderInfoList('sim-report-list', [
    worldOpsHTML ? `<div>${worldOpsHTML}</div>` : '',
    worldBaselineHTML ? `<div>${worldBaselineHTML}</div>` : '',
    portfolioHTML ? `<div>${portfolioHTML}</div>` : '',
    batchSummaryHTML ? `<div>${batchSummaryHTML}</div>` : '',
    ...reports.map(report => `
    <div class="interactive-row">
      <div class="row-main">
        <div class="row-title">${safeText(report.name)}</div>
        <div class="row-subtitle">${safeText(report.source_instance_id)} vs ${safeText(report.compare_instance_id || '无对照')} · ${report.created_at ? new Date(report.created_at).toLocaleString('zh-CN') : '--'}</div>
        <div class="row-subtitle">${safeText(report.note, report.conclusion?.[0] || '--')}</div>
        ${(report.current_checkpoint || report.compare_checkpoint) ? `<div class="row-subtitle">checkpoint: ${safeText(report.current_checkpoint, '--')} / ${safeText(report.compare_checkpoint, '无对照')}</div>` : ''}
        ${state.experimentReplayRuns[report.name]?.replay?.current_instance?.id ? `<div class="row-subtitle">replay: ${safeText(state.experimentReplayRuns[report.name].replay.current_instance.id)}${state.experimentReplayRuns[report.name].replay.compare_instance?.id ? ` / ${safeText(state.experimentReplayRuns[report.name].replay.compare_instance.id)}` : ''}</div>` : ''}
      </div>
      <div class="row-actions">
        ${report.current_checkpoint ? `<button type="button" class="ghost-button" data-report-replay-instance="${encodeURIComponent(String(report.name || ''))}">派生复现</button>` : ''}
        ${report.current_checkpoint ? `<button type="button" class="ghost-button" data-report-refresh-replay="${encodeURIComponent(String(report.name || ''))}">刷新复现</button>` : ''}
        ${state.experimentReplayRuns[report.name]?.replay?.current_instance?.id ? `<button type="button" class="ghost-button" data-report-advance-replay="${encodeURIComponent(String(report.name || ''))}">推进 replay</button>` : ''}
        ${state.experimentReplayRuns[report.name]?.replay?.current_instance?.id ? `<button type="button" class="ghost-button" data-report-open-replay-current="${encodeURIComponent(String(report.name || ''))}">打开 current</button>` : ''}
        ${state.experimentReplayRuns[report.name]?.replay?.compare_instance?.id ? `<button type="button" class="ghost-button" data-report-open-replay-compare="${encodeURIComponent(String(report.name || ''))}">打开 compare</button>` : ''}
        ${report.current_checkpoint ? `<button type="button" class="ghost-button" data-report-restore-current="${safeText(report.name, '')}">恢复当前</button>` : ''}
        ${report.compare_checkpoint && report.compare_instance_id ? `<button type="button" class="ghost-button" data-report-restore-compare="${safeText(report.name, '')}">恢复对照</button>` : ''}
        <button type="button" class="ghost-button" data-report-export-json="${safeText(report.name, '')}">JSON</button>
        <button type="button" class="ghost-button" data-report-export-md="${safeText(report.name, '')}">Markdown</button>
      </div>
    </div>
    ${renderExperimentReplayRunSummary(report, state.experimentReplayRuns[report.name])}
  `)
  ].filter(Boolean), activeWorld ? `当前 world ${activeWorld} 下暂无实验归档` : '暂无实验归档');
  els.simReportList.querySelectorAll('[data-report-export-json]').forEach(node => {
    node.addEventListener('click', () => {
      const report = reports.find(item => item.name === node.dataset.reportExportJson);
      exportExperimentReport(report, 'json');
    });
  });
  els.simReportList.querySelectorAll('[data-report-export-md]').forEach(node => {
    node.addEventListener('click', () => {
      const report = reports.find(item => item.name === node.dataset.reportExportMd);
      exportExperimentReport(report, 'markdown');
    });
  });
  els.simReportList.querySelectorAll('[data-report-replay-instance]').forEach(node => {
    node.addEventListener('click', async () => {
      await replayExperimentReportIntoBranches(decodeURIComponent(node.dataset.reportReplayInstance || ''));
    });
  });
  els.simReportList.querySelectorAll('[data-report-refresh-replay]').forEach(node => {
    node.addEventListener('click', async () => {
      await refreshExperimentReplayRun(decodeURIComponent(node.dataset.reportRefreshReplay || ''));
    });
  });
  els.simReportList.querySelectorAll('[data-world-baseline-replay]').forEach(node => {
    node.addEventListener('click', async () => {
      await replayExperimentReportsBatch({
        mode: 'replay',
        worldName: decodeURIComponent(node.dataset.worldBaselineReplay || ''),
      });
    });
  });
  els.simReportList.querySelectorAll('[data-world-baseline-refresh]').forEach(node => {
    node.addEventListener('click', async () => {
      await replayExperimentReportsBatch({
        mode: 'refresh',
        worldName: decodeURIComponent(node.dataset.worldBaselineRefresh || ''),
      });
    });
  });
  els.simReportList.querySelectorAll('[data-world-replay-advance]').forEach(node => {
    node.addEventListener('click', async () => {
      await advanceExperimentReplayBatch({
        worldName: decodeURIComponent(node.dataset.worldReplayAdvance || ''),
        count: getBatchTickCount(),
      });
    });
  });
  els.simReportList.querySelectorAll('[data-world-baseline-export-json]').forEach(node => {
    node.addEventListener('click', () => {
      exportExperimentBaselines('json', {
        worldName: decodeURIComponent(node.dataset.worldBaselineExportJson || ''),
      });
    });
  });
  els.simReportList.querySelectorAll('[data-world-baseline-export-md]').forEach(node => {
    node.addEventListener('click', () => {
      exportExperimentBaselines('markdown', {
        worldName: decodeURIComponent(node.dataset.worldBaselineExportMd || ''),
      });
    });
  });
  els.simReportList.querySelectorAll('[data-world-proof-export-json]').forEach(node => {
    node.addEventListener('click', () => {
      exportWorldExperimentProofBundle('json', {
        worldName: decodeURIComponent(node.dataset.worldProofExportJson || ''),
      });
    });
  });
  els.simReportList.querySelectorAll('[data-world-proof-export-md]').forEach(node => {
    node.addEventListener('click', () => {
      exportWorldExperimentProofBundle('markdown', {
        worldName: decodeURIComponent(node.dataset.worldProofExportMd || ''),
      });
    });
  });
  els.simReportList.querySelectorAll('[data-world-baseline-focus]').forEach(node => {
    node.addEventListener('click', () => {
      setExperimentWorldFilter(decodeURIComponent(node.dataset.worldBaselineFocus || ''));
      renderExperimentReports();
      renderRuntimeAudit();
    });
  });
  els.simReportList.querySelectorAll('[data-world-baseline-clear]').forEach(node => {
    node.addEventListener('click', () => {
      setExperimentWorldFilter('');
      renderExperimentReports();
      renderRuntimeAudit();
    });
  });
  els.simReportList.querySelectorAll('[data-world-ops-report-view]').forEach(node => {
    node.addEventListener('click', () => {
      selectExperimentReportContext(decodeURIComponent(node.dataset.worldOpsReportView || ''), allReports);
      renderExperimentReports();
      renderRuntimeAudit();
    });
  });
  els.simReportList.querySelectorAll('[data-world-ops-checkpoint]').forEach(node => {
    node.addEventListener('click', () => {
      state.runtimeAuditCheckpointName = decodeURIComponent(node.dataset.worldOpsCheckpoint || '');
      renderExperimentReports();
      renderRuntimeAudit();
    });
  });
  els.simReportList.querySelectorAll('[data-world-ops-report-export]').forEach(node => {
    node.addEventListener('click', () => {
      const report = allReports.find(item => item.name === decodeURIComponent(node.dataset.worldOpsReportExport || ''));
      exportExperimentReport(report, 'markdown');
    });
  });
  els.simReportList.querySelectorAll('[data-replay-open-trace]').forEach(node => {
    node.addEventListener('click', async () => {
      await openReplayTraceTurn(node.dataset.replayOpenTrace, node.dataset.replayTurn);
    });
  });
  els.simReportList.querySelectorAll('[data-report-advance-replay]').forEach(node => {
    node.addEventListener('click', async () => {
      try {
        await advanceExperimentReplayRun(decodeURIComponent(node.dataset.reportAdvanceReplay || ''), {
          count: getBatchTickCount(),
        });
      } catch (err) {
        alert(`推进 replay 失败：${err.message}`);
      }
    });
  });
  els.simReportList.querySelectorAll('[data-report-open-replay-current]').forEach(node => {
    node.addEventListener('click', async () => {
      try {
        await openExperimentReplayInstance(decodeURIComponent(node.dataset.reportOpenReplayCurrent || ''), 'current');
      } catch (err) {
        alert(`打开 current replay 失败：${err.message}`);
      }
    });
  });
  els.simReportList.querySelectorAll('[data-report-open-replay-compare]').forEach(node => {
    node.addEventListener('click', async () => {
      try {
        await openExperimentReplayInstance(decodeURIComponent(node.dataset.reportOpenReplayCompare || ''), 'compare');
      } catch (err) {
        alert(`打开 compare replay 失败：${err.message}`);
      }
    });
  });
  els.simReportList.querySelectorAll('[data-replay-cause]').forEach(node => {
    node.addEventListener('click', () => showCausalChain(node.dataset.replayCause));
  });
  els.simReportList.querySelectorAll('[data-report-restore-current]').forEach(node => {
    node.addEventListener('click', async () => {
      const report = reports.find(item => item.name === node.dataset.reportRestoreCurrent);
      if (!report?.current_checkpoint) return;
      try {
        await loadCheckpointIntoInstance(String(report.source_instance_id || state.selectedInstanceID || '').trim(), report.current_checkpoint, `${report.name} 当前实验`);
      } catch (err) {
        alert(`恢复当前实验失败：${err.message}`);
      }
    });
  });
  els.simReportList.querySelectorAll('[data-report-restore-compare]').forEach(node => {
    node.addEventListener('click', async () => {
      const report = reports.find(item => item.name === node.dataset.reportRestoreCompare);
      if (!report?.compare_checkpoint || !report?.compare_instance_id) return;
      try {
        await loadCheckpointIntoInstance(String(report.compare_instance_id).trim(), report.compare_checkpoint, `${report.name} 对照实验`);
      } catch (err) {
        alert(`恢复对照实验失败：${err.message}`);
      }
    });
  });
}

function buildAuditSection(title, rows, causes = []) {
  if (!rows.length) return null;
  return { title, rows, causes };
}

function buildRawAuditSection(html, causes = []) {
  if (!html) return null;
  return { raw: true, html, causes };
}

function renderAuditSection(section) {
  if (!section) return '';
  if (section.raw) {
    return section.html || '';
  }
  if (!Array.isArray(section.rows) || !section.rows.length) return '';
  return `
    <div class="note-box" style="margin-top:8px">
      <div class="mono" style="font-weight:600;margin-bottom:6px">${safeText(section.title)}</div>
      ${section.rows.map(row => `<div class="mono" style="margin-top:4px">${row}</div>`).join('')}
    </div>
  `;
}

function matchesAuditCause(section, cause) {
  if (!section) {
    return false;
  }
  if (!cause || cause === 'all') {
    return true;
  }
  return Array.isArray(section.causes) && section.causes.includes(cause);
}

function getRuntimeAuditReplayStep(trace) {
  const steps = Array.isArray(trace?.step_traces) ? trace.step_traces : [];
  if (!steps.length) {
    return { stepTrace: null, stepIndex: -1, stepCount: 0 };
  }
  const maxIndex = steps.length - 1;
  const current = Math.max(0, Math.min(Number(state.runtimeAuditReplayStepIndex || 0), maxIndex));
  return { stepTrace: steps[current], stepIndex: current, stepCount: steps.length };
}

function renderRuntimeAuditReplaySection() {
  const trace = state.runtimeAuditReplayTrace;
  const steps = Array.isArray(trace?.step_traces) ? trace.step_traces : [];
  if (!trace || !steps.length) {
    return '';
  }
  const { stepTrace, stepIndex, stepCount } = getRuntimeAuditReplayStep(trace);
  if (!stepTrace) {
    return '';
  }
  const step = stepTrace.step || {};
  const goals = Array.isArray(stepTrace.active_goals) ? stepTrace.active_goals.slice(0, 5) : [];
  const memories = Array.isArray(stepTrace.memories) ? stepTrace.memories.slice(0, 4) : [];
  const facts = Array.isArray(stepTrace.semantic_facts) ? stepTrace.semantic_facts.slice(0, 4) : [];
  const events = Array.isArray(stepTrace.events) ? stepTrace.events.slice(0, 5) : [];
  const allowedActions = Array.isArray(stepTrace.allowed_actions) ? stepTrace.allowed_actions.slice(0, 7) : [];
  const chips = steps.map((item, index) => {
    const itemStep = item?.step || {};
    const active = index === stepIndex ? ' style="background:rgba(198,89,17,.18);border-color:rgba(198,89,17,.5)"' : '';
    return `<button type="button" class="ghost-button" data-audit-replay-step="${index}"${active}>${index + 1}.${safeText(itemStep.speaker || '--')}</button>`;
  }).join(' ');
  return `
    <div class="note-box" style="margin-top:8px">
      <div class="mono" style="font-weight:600;margin-bottom:6px">Phase Replay</div>
      <div class="mono" style="margin-top:4px">trace turn ${Number(trace.turn || 0)} · ${safeText(trace.focus_character || '--')} · ${safeText(trace.user_input || '--')}</div>
      <div class="mono" style="margin-top:6px">
        <button type="button" class="ghost-button" data-audit-replay-prev ${stepIndex <= 0 ? 'disabled' : ''}>上一阶段</button>
        <button type="button" class="ghost-button" data-audit-replay-next ${stepIndex >= stepCount - 1 ? 'disabled' : ''}>下一阶段</button>
        当前 ${stepIndex + 1}/${stepCount} · ${safeText(step.speaker || '--')} · ${safeText(step.kind || 'lead')}
      </div>
      <div class="mono" style="margin-top:6px">${chips}</div>
      <div class="mono" style="margin-top:6px">reason: ${safeText(step.reason || '--')} · budget ${safeText(step.budget_mode || 'normal')} · tokens ${safeText(stepTrace.used_tokens, 0)}/${safeText(stepTrace.token_budget, 0)}</div>
      ${stepTrace.handoff ? `<div class="mono" style="margin-top:4px">handoff: ${safeText(stepTrace.handoff.from_speaker || '--')} · ${safeText(stepTrace.handoff.kind || '--')} · ${safeText(stepTrace.handoff.action || '--')} -> ${safeText(stepTrace.handoff.target || '--')}</div>` : ''}
      ${goals.length ? `<div class="mono" style="margin-top:4px">goals: ${goals.map(goal => `${goal.id}(p${goal.priority})`).join(' / ')}</div>` : ''}
      ${allowedActions.length ? `<div class="mono" style="margin-top:4px">allowed: ${allowedActions.map(action => safeText(action)).join(' / ')}</div>` : ''}
      ${stepTrace.action_frame?.action ? `<div class="mono" style="margin-top:4px">action: ${safeText(stepTrace.action_frame.action)} -> ${safeText(stepTrace.action_frame.target || '--')} · ${safeText(stepTrace.action_frame.intent || '--')}</div>` : ''}
      ${stepTrace.validator?.blocked ? `<div class="mono" style="margin-top:4px">validator blocked: ${safeText(stepTrace.validator.reason || '--')}</div>` : ''}
      ${memories.length ? `<div class="mono" style="margin-top:4px">memories: ${memories.map(memory => `${memory.type}:${truncate(memory.content || '', 36)}`).join(' / ')}</div>` : ''}
      ${facts.length ? `<div class="mono" style="margin-top:4px">facts: ${facts.map(fact => `${fact.subject}-${fact.predicate}-${truncate(fact.object || '', 22)}`).join(' / ')}</div>` : ''}
      ${events.length ? `<div class="mono" style="margin-top:4px">events: ${events.map(event => `${event.type}${event.target ? `->${event.target}` : ''}`).join(' / ')}</div>` : ''}
      ${stepTrace.narrative ? `<div class="mono" style="margin-top:4px">narrative: ${safeText(stepTrace.narrative)}</div>` : ''}
      ${stepTrace.error ? `<div class="mono" style="margin-top:4px">error: ${safeText(stepTrace.error)}</div>` : ''}
    </div>
  `;
}

function buildExperimentOpsMatrixRows(report, replayRun, selectedCheckpoint, audit) {
  const rows = [];
  const appendRow = (label, values) => {
    rows.push(`
      <div class="mono" style="margin-top:4px">
        <span style="display:inline-block;min-width:118px">${safeText(label)}</span>
        archived current ${safeText(values.current || '--')} | archived compare ${safeText(values.compare || '--')} | checkpoint ${safeText(values.checkpoint || '--')} | live replay ${safeText(values.live || '--')}
      </div>
    `);
  };

  const current = report?.current || null;
  const compare = report?.compare || null;
  const checkpointState = selectedCheckpoint?.world_state || null;
  const currentLive = replayRun?.currentStatus || null;
  const compareLive = replayRun?.compareStatus || null;

  appendRow('focus/scene', {
    current: current ? `${safeText(current.focus_character || '--')} @ ${safeText(current.scene_location || '--')}` : '--',
    compare: compare ? `${safeText(compare.focus_character || '--')} @ ${safeText(compare.scene_location || '--')}` : '--',
    checkpoint: checkpointState ? `${safeText(selectedCheckpoint?.focus_character || '--')} @ ${safeText(checkpointState?.scene?.location || '--')}` : '--',
    live: currentLive ? `${safeText(report?.current?.focus_character || replayRun?.replay?.current_instance?.focus_character || '--')} @ ${safeText(audit?.state?.scene?.location || '--')}` : '--',
  });

  appendRow('tension', {
    current: current ? Number(current.tension || 0).toFixed(2) : '--',
    compare: compare ? Number(compare.tension || 0).toFixed(2) : '--',
    checkpoint: checkpointState ? Number(checkpointState.tension || 0).toFixed(2) : '--',
    live: currentLive && compareLive
      ? `${Number(currentLive.tension || 0).toFixed(2)} / ${Number(compareLive.tension || 0).toFixed(2)}`
      : currentLive
        ? Number(currentLive.tension || 0).toFixed(2)
        : '--',
  });

  appendRow('trajectory', {
    current: current ? safeText((current.trajectory_summary || [])[0] || '--') : '--',
    compare: compare ? safeText((compare.trajectory_summary || [])[0] || '--') : '--',
    checkpoint: selectedCheckpoint ? safeText(selectedCheckpoint.preview || selectedCheckpoint.note || '--') : '--',
    live: currentLive && compareLive
      ? `${safeText((currentLive.trajectory_summary || [])[0] || '--')} / ${safeText((compareLive.trajectory_summary || [])[0] || '--')}`
      : currentLive
        ? safeText((currentLive.trajectory_summary || [])[0] || '--')
        : '--',
  });

  appendRow('population', {
    current: current ? safeText((current.population_highlights || [])[0] || '--') : '--',
    compare: compare ? safeText((compare.population_highlights || [])[0] || '--') : '--',
    checkpoint: checkpointState ? safeText((checkpointState.scene?.characters || []).join(' · ') || '--') : '--',
    live: currentLive && compareLive
      ? `${safeText((currentLive.population_highlights || [])[0] || '--')} / ${safeText((compareLive.population_highlights || [])[0] || '--')}`
      : currentLive
        ? safeText((currentLive.population_highlights || [])[0] || '--')
        : '--',
  });

  return rows.join('');
}

function formatProofAuditMarkdown(item, root = '') {
  const lines = [
    '# Proof Audit',
    '',
    `- Name: ${item?.name || '--'}`,
    `- Overall: ${item?.overall || '--'}`,
    `- Created At: ${item?.created_at || '--'}`,
  ];
  if (root) {
    lines.push(`- Root: ${root}`);
  }
  if (item?.summary_path) {
    lines.push(`- Summary Path: ${item.summary_path}`);
  }
  const files = Array.isArray(item?.files) ? item.files : [];
  if (files.length) {
    lines.push('');
    lines.push('## Files');
    lines.push('');
    files.forEach(file => {
      lines.push(`- ${file.name || '--'} (${Number(file.size || 0)} bytes)`);
    });
  }
  const preview = String(item?.summary_preview || '').trim();
  if (preview) {
    lines.push('');
    lines.push('## Preview');
    lines.push('');
    lines.push(preview);
  }
  return lines.join('\n');
}

function exportProofAuditSummary(item, format = 'markdown') {
  if (!item) {
    return;
  }
  const stamp = new Date().toISOString().replace(/[:.]/g, '-');
  if (format === 'json') {
    downloadText(`proof-audit-${item.name || 'latest'}-${stamp}.json`, JSON.stringify({
      root: state.proofAudits?.root || '',
      proof_audit: item,
    }, null, 2), 'application/json;charset=utf-8');
    return;
  }
  downloadText(`proof-audit-${item.name || 'latest'}-${stamp}.md`, formatProofAuditMarkdown(item, state.proofAudits?.root || ''), 'text/markdown;charset=utf-8');
}

function renderProofAuditArchiveSummary() {
  const payload = state.proofAudits;
  const items = Array.isArray(payload?.proof_audits) ? payload.proof_audits : [];
  const root = String(payload?.root || '').trim();
  const error = String(payload?.error || '').trim();
  if (!items.length && !root && !error) {
    return '';
  }
  if (error) {
    return `
      <div class="note-box" style="margin-top:8px">
        <div class="mono" style="font-weight:600">Proof Audits</div>
        <div class="mono" style="margin-top:4px">读取失败：${safeText(error)}</div>
      </div>
    `;
  }
  const rows = items.map(item => {
    const files = Array.isArray(item?.files) ? item.files : [];
    const createdAt = item?.created_at ? new Date(item.created_at).toLocaleString('zh-CN') : safeText(item?.name || '--');
    const preview = String(item?.summary_preview || '').trim()
      .split('\n')
      .slice(0, 4)
      .map(line => safeText(line))
      .join('<br>');
    return `
      <div class="note-box" style="margin-top:8px">
        <div class="mono" style="font-weight:600">${safeText(item?.name || '--')} · ${safeText(item?.overall || '--')}</div>
        <div class="mono" style="margin-top:4px">created: ${createdAt}</div>
        ${item?.summary_path ? `<div class="mono" style="margin-top:4px">summary: ${safeText(item.summary_path)}</div>` : ''}
        ${files.length ? `<div class="mono" style="margin-top:4px">files: ${files.map(file => `${safeText(file.name)}(${Number(file.size || 0)})`).join(' · ')}</div>` : ''}
        ${preview ? `<div class="mono" style="margin-top:6px">${preview}</div>` : ''}
        <div class="mono" style="margin-top:6px">
          <button type="button" class="ghost-button" data-proof-audit-export="${encodeURIComponent(String(item?.name || ''))}" data-proof-audit-format="json">导出 JSON</button>
          <button type="button" class="ghost-button" data-proof-audit-export="${encodeURIComponent(String(item?.name || ''))}" data-proof-audit-format="markdown">导出 MD</button>
        </div>
      </div>
    `;
  }).join('');
  return `
    <div class="note-box" style="margin-top:8px">
      <div class="mono" style="font-weight:600">Proof Audits</div>
      <div class="mono" style="margin-top:4px">latest ${items.length} run(s)${root ? ` · root ${safeText(root)}` : ''}</div>
      ${rows || '<div class="mono" style="margin-top:6px">暂无 proof audit 归档</div>'}
    </div>
  `;
}

function renderRuntimeAuditReportSection(report) {
  if (!report) {
    return '';
  }
  const replayRun = state.experimentReplayRuns[report.name] || null;
  const checkpointDiffLines = collectExperimentSnapshotDiffLines(report);
  const auditCheckpoints = Array.isArray(state.runtimeAudit?.checkpoints) ? state.runtimeAudit.checkpoints : [];
  const selectedCheckpoint = auditCheckpoints.find(slot => String(slot?.name || '') === String(state.runtimeAuditCheckpointName || '')) || auditCheckpoints[0] || null;
  const opsMatrixHTML = buildExperimentOpsMatrixRows(report, replayRun, selectedCheckpoint, state.runtimeAudit);
  const renderSnapshot = (title, snapshot, side) => {
    if (!snapshot) {
      return '';
    }
    const latestTrace = snapshot.latest_trace;
    const checkpoint = side === 'compare' ? report.compare_checkpoint : report.current_checkpoint;
    const restoreAttr = side === 'compare' ? 'data-audit-report-restore-compare' : 'data-audit-report-restore-current';
    return `
      <div class="mono" style="margin-top:8px;font-weight:600">${safeText(title)}</div>
      <div class="mono" style="margin-top:4px">instance ${safeText(snapshot.instance_id || '--')} · world ${safeText(snapshot.world_name || '--')} · focus ${safeText(snapshot.focus_character || '--')}</div>
      <div class="mono" style="margin-top:4px">scene ${safeText(snapshot.scene_location || '--')} · tension ${Number(snapshot.tension || 0).toFixed(2)} · tick/turn ${Number(snapshot.tick_count || 0)}/${Number(snapshot.turn_count || 0)}</div>
      ${checkpoint ? `<div class="mono" style="margin-top:4px">checkpoint: ${safeText(checkpoint)} <button type="button" class="ghost-button" ${restoreAttr}="${encodeURIComponent(String(report.name || ''))}">恢复</button></div>` : ''}
      ${Array.isArray(snapshot.trajectory_summary) && snapshot.trajectory_summary.length ? `<div class="mono" style="margin-top:4px">trajectory: ${snapshot.trajectory_summary.map(item => safeText(item)).join(' | ')}</div>` : ''}
      ${latestTrace ? `<div class="mono" style="margin-top:4px">latest trace: turn ${Number(latestTrace.turn || 0)} · ${safeText(latestTrace.focus_character || '--')} · ${safeText(latestTrace.user_input || '--')} <button type="button" class="ghost-button" data-audit-report-replay="${safeText(side, '')}">回放</button></div>` : ''}
    `;
  };
  return `
    <div class="note-box" style="margin-top:8px">
      <div class="mono" style="font-weight:600;margin-bottom:6px">Experiment Replay</div>
      <div class="mono" style="margin-top:4px">${safeText(report.name || '--')} · ${safeText(report.source_instance_id || '--')} vs ${safeText(report.compare_instance_id || '无对照')}</div>
      ${report.note ? `<div class="mono" style="margin-top:4px">note: ${safeText(report.note)}</div>` : ''}
      ${Array.isArray(report.outcome_summary) && report.outcome_summary.length ? `<div class="mono" style="margin-top:4px">outcome: ${report.outcome_summary.map(item => safeText(item)).join(' | ')}</div>` : ''}
      ${Array.isArray(report.conclusion) && report.conclusion.length ? `<div class="mono" style="margin-top:4px">conclusion: ${report.conclusion.map(item => safeText(item)).join(' | ')}</div>` : ''}
      ${report.current_checkpoint ? `<div class="mono" style="margin-top:4px"><button type="button" class="ghost-button" data-audit-report-replay-instance="${encodeURIComponent(String(report.name || ''))}">派生复现实验</button></div>` : ''}
      ${opsMatrixHTML ? `<div class="mono" style="margin-top:8px;font-weight:600">Ops Matrix</div>${opsMatrixHTML}` : ''}
      ${checkpointDiffLines.length ? `<div class="mono" style="margin-top:8px;font-weight:600">Checkpoint Diff</div>${checkpointDiffLines.map(line => `<div class="mono" style="margin-top:4px">${safeText(line)}</div>`).join('')}` : ''}
      ${renderSnapshot('Current Snapshot', report.current, 'current')}
      ${renderSnapshot('Compare Snapshot', report.compare, 'compare')}
      ${renderExperimentReplayRunSummary(report, replayRun)}
    </div>
  `;
}

function renderRuntimeAudit() {
  const audit = state.runtimeAudit;
  if (!audit) {
    renderInfoList('runtime-audit-summary', [], '暂无统一审计摘要');
    renderInfoList('runtime-audit-panel', [], '暂无统一审计数据');
    return;
  }

  const cause = state.runtimeAuditCause || 'all';
  let summary = Array.isArray(audit.audit_summary) ? audit.audit_summary : [];
  if (cause !== 'all') {
    summary = summary.filter(line => {
      const text = String(line || '').toLowerCase();
      if (cause === 'director') return text.includes('director') || text.includes('trace') || text.includes('participant');
      if (cause === 'pressure') return text.includes('pressure') || text.includes('tension') || text.includes('diagnostic');
      if (cause === 'faction') return text.includes('faction');
      if (cause === 'population') return text.includes('population') || text.includes('promot') || text.includes('background');
      if (cause === 'archive') return text.includes('checkpoint') || text.includes('preset') || text.includes('report') || text.includes('asset');
      return true;
    });
  }
  renderInfoList('runtime-audit-summary', summary.map(line => `<div class="note-box mono" style="margin-top:4px">${safeText(line)}</div>`), '暂无统一审计摘要');

  const filter = state.runtimeAuditFilter || 'all';
  const sections = [];

  if (filter === 'all' || filter === 'trace') {
    const traceRows = [];
    if (audit.latest_trace) {
      traceRows.push(`latest trace: turn ${Number(audit.latest_trace.turn || 0)} · ${safeText(audit.latest_trace.focus_character || '--')} · ${safeText(audit.latest_trace.user_input || '--')} <button type="button" class="ghost-button" data-audit-trace-turn="${Number(audit.latest_trace.turn || 0)}">打开</button>`);
    }
    if (audit.director_plan?.selected?.length) {
      traceRows.push(`director: ${safeText(audit.director_plan.mode || 'manual')} -> ${audit.director_plan.selected.map(item => safeText(item)).join(' -> ')}`);
    }
    if (Array.isArray(audit.director_plan?.world_signals) && audit.director_plan.world_signals.length) {
      traceRows.push(`director-world: ${audit.director_plan.world_signals.map(item => safeText(item)).join(' · ')}`);
    }
    const recentTraces = Array.isArray(audit.recent_traces) ? audit.recent_traces : [];
    if (recentTraces.length) {
      traceRows.push(...recentTraces.map(trace => `turn ${Number(trace.turn || 0)} · ${safeText(trace.focus_character || '--')} · ${safeText(trace.user_input || '--')} <button type="button" class="ghost-button" data-audit-trace-turn="${Number(trace.turn || 0)}">查看</button>`));
    }
    sections.push(buildAuditSection('Trace / Director', traceRows, ['director']));
    const replayHTML = renderRuntimeAuditReplaySection();
    if (replayHTML) {
      sections.push(buildRawAuditSection(replayHTML, ['director']));
    }
  }

  if (filter === 'all' || filter === 'world') {
    const simStatus = audit.sim_status || {};
    const trajectory = Array.isArray(simStatus.trajectory_summary) ? simStatus.trajectory_summary : [];
    const pressureRows = [];
    if (trajectory.length) {
      pressureRows.push(`trajectory: ${trajectory.map(line => safeText(line)).join(' | ')}`);
    }
    const diagnostics = Array.isArray(simStatus.diagnostics) ? simStatus.diagnostics : [];
    if (diagnostics.length) {
      pressureRows.push(`diagnostics: ${diagnostics.slice(0, 3).map(item => `${safeText(item.metric || '--')}:${safeText(item.message || '--')}`).join(' · ')}`);
    }
    const pressureStates = simStatus.pressure_states || {};
    if (Object.keys(pressureStates).length) {
      pressureRows.push(`pressure states: ${formatMetricMap(pressureStates, value => Number(value).toFixed(2))}`);
    }
    sections.push(buildAuditSection('World Pressure', pressureRows, ['pressure']));

    const factionRows = [];
    const factionTensions = simStatus.faction_tensions || {};
    if (Object.keys(factionTensions).length) {
      factionRows.push(`faction tensions: ${formatMetricMap(factionTensions, value => Number(value).toFixed(2))}`);
    }
    if (Array.isArray(audit.director_plan?.world_signals)) {
      const factionSignals = audit.director_plan.world_signals.filter(item => String(item || '').includes('faction'));
      if (factionSignals.length) {
        factionRows.push(`director-faction signals: ${factionSignals.map(item => safeText(item)).join(' · ')}`);
      }
    }
    sections.push(buildAuditSection('Faction Signals', factionRows, ['faction']));

    const population = audit.population || {};
    const populationRows = [];
    const promoted = Array.isArray(population.promoted) ? population.promoted : [];
    if (promoted.length) {
      populationRows.push(`promoted: ${promoted.map(item => `${safeText(item.name)}(${Number(item.attention?.score || 0).toFixed(1)})`).join(' · ')}`);
    }
    const background = Array.isArray(population.background) ? population.background : [];
    if (background.length) {
      populationRows.push(`rising background: ${background.map(item => `${safeText(item.name)}(${Number(item.attention?.score || 0).toFixed(1)})`).join(' · ')}`);
    }
    sections.push(buildAuditSection('Population Growth', populationRows, ['population']));
  }

  if (filter === 'all' || filter === 'archive') {
    const archiveRows = [];
    const archiveReports = Array.isArray(audit.experiment_reports) ? audit.experiment_reports : state.experimentReports;
    const activeWorld = getActiveExperimentWorldFilter(String(audit?.instance?.world_name || ''));
    const worldPanelHTML = renderWorldExperimentPanel(audit, archiveReports, state.experimentReplayRuns, { worldName: activeWorld });
    const scopedArchiveReports = filterReportsByWorld(archiveReports, activeWorld);
    const worldOpsHTML = renderExperimentWorldOpsSummary(archiveReports, state.experimentReplayRuns, { worldName: activeWorld });
    const worldBaselineHTML = renderExperimentWorldBaselineSummary(archiveReports, state.experimentReplayRuns);
    const currentBaselineGapHTML = renderCurrentWorldBaselineGapSummary(audit, archiveReports, state.experimentReplayRuns);
    const portfolioHTML = renderExperimentPortfolioSummary(scopedArchiveReports, state.experimentReplayRuns);
    const replayEligibleCount = getReplayEligibleReports(scopedArchiveReports).length;
    archiveRows.push('baseline export: <button type="button" class="ghost-button" data-audit-export-baseline="json">导出基线 JSON</button> <button type="button" class="ghost-button" data-audit-export-baseline="markdown">导出基线 MD</button>');
    archiveRows.push('proof bundle: <button type="button" class="ghost-button" data-audit-export-proof="json">导出证据包 JSON</button> <button type="button" class="ghost-button" data-audit-export-proof="markdown">导出证据包 MD</button>');
    if (replayEligibleCount) {
      archiveRows.push(`replay workflow: ${replayEligibleCount} reports ready${activeWorld ? ` · scope ${safeText(activeWorld)}` : ''} <button type="button" class="ghost-button" data-audit-report-replay-all="1">批量派生</button> <button type="button" class="ghost-button" data-audit-report-refresh-all="1">批量刷新</button> <button type="button" class="ghost-button" data-audit-report-advance-all="1">批量推进 replay</button>`);
    }
    const checkpoints = Array.isArray(audit.checkpoints) ? audit.checkpoints : [];
    if (checkpoints.length) {
      archiveRows.push(`checkpoints: ${checkpoints.map(slot => `${safeText(slot.name)}@${safeText(slot.branch || 'main')}`).join(' · ')} <button type="button" class="ghost-button" data-audit-checkpoint-open="${encodeURIComponent(String(checkpoints[0]?.name || ''))}">浏览</button>`);
    }
    const presets = Array.isArray(audit.presets) ? audit.presets : [];
    if (presets.length) {
      archiveRows.push(`presets: ${presets.map(preset => `${safeText(preset.name)}→${safeText(preset.focus_character || '--')}`).join(' · ')}`);
    }
    const reports = scopedArchiveReports;
    if (reports.length) {
      archiveRows.push(...reports.map(report => `report: ${safeText(report.name)} · ${safeText(report.source_instance_id || '--')} vs ${safeText(report.compare_instance_id || '无对照')} <button type="button" class="ghost-button" data-audit-report-view="${encodeURIComponent(String(report.name || ''))}">展开</button> <button type="button" class="ghost-button" data-audit-report-replay-instance="${encodeURIComponent(String(report.name || ''))}">派生</button> <button type="button" class="ghost-button" data-audit-report-name="${encodeURIComponent(String(report.name || ''))}">导出 MD</button>`));
    }
    sections.push(buildAuditSection('Archive / Replay', archiveRows, ['archive']));
    const proofAuditHTML = renderProofAuditArchiveSummary();
    if (proofAuditHTML) {
      sections.push(buildRawAuditSection(proofAuditHTML, ['archive']));
    }
    if (worldPanelHTML) {
      sections.push(buildRawAuditSection(worldPanelHTML, ['archive']));
    }
    if (worldOpsHTML) {
      sections.push(buildRawAuditSection(worldOpsHTML, ['archive']));
    }
    if (worldBaselineHTML) {
      sections.push(buildRawAuditSection(worldBaselineHTML, ['archive']));
    }
    if (currentBaselineGapHTML) {
      sections.push(buildRawAuditSection(currentBaselineGapHTML, ['archive']));
    }
    if (portfolioHTML) {
      sections.push(buildRawAuditSection(portfolioHTML, ['archive']));
    }
    const checkpointBrowserHTML = renderRuntimeAuditCheckpointBrowser(audit);
    if (checkpointBrowserHTML) {
      sections.push(buildRawAuditSection(checkpointBrowserHTML, ['archive']));
    }
    const batchSummaryHTML = renderExperimentReplayBatchSummary();
    if (batchSummaryHTML) {
      sections.push(buildRawAuditSection(batchSummaryHTML, ['archive']));
    }
    const selectedReportName = state.runtimeAuditReportName || (reports[0]?.name || '');
    const selectedReport = reports.find(report => report.name === selectedReportName) || null;
    if (selectedReport) {
      sections.push(buildRawAuditSection(renderRuntimeAuditReportSection(selectedReport), ['archive']));
    }
  }

  renderInfoList('runtime-audit-panel', sections.filter(section => section && matchesAuditCause(section, cause)).map(renderAuditSection), '当前筛选下暂无统一审计证据');

  if (els.runtimeAuditPanel) {
    els.runtimeAuditPanel.querySelectorAll('[data-audit-trace-turn]').forEach(node => {
      node.addEventListener('click', async () => {
        const turn = node.dataset.auditTraceTurn;
        await Promise.all([selectTraceTurn(turn), loadRuntimeAuditReplay(turn, 0)]);
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-report-name]').forEach(node => {
        node.addEventListener('click', () => {
          const reports = Array.isArray(state.runtimeAudit?.experiment_reports) ? state.runtimeAudit.experiment_reports : [];
          const report = reports.find(item => item.name === decodeURIComponent(node.dataset.auditReportName || ''));
          exportExperimentReport(report, 'markdown');
        });
      });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-report-view]').forEach(node => {
      node.addEventListener('click', () => {
        selectExperimentReportContext(
          decodeURIComponent(node.dataset.auditReportView || ''),
          Array.isArray(state.runtimeAudit?.experiment_reports) ? state.runtimeAudit.experiment_reports : state.experimentReports,
        );
        renderRuntimeAudit();
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-report-replay-instance]').forEach(node => {
      node.addEventListener('click', async () => {
        await replayExperimentReportIntoBranches(decodeURIComponent(node.dataset.auditReportReplayInstance || ''));
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-export-baseline]').forEach(node => {
      node.addEventListener('click', () => {
        exportExperimentBaselines(node.dataset.auditExportBaseline || 'markdown', {
          worldName: getActiveExperimentWorldFilter(String(audit?.instance?.world_name || '')),
        });
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-export-proof]').forEach(node => {
      node.addEventListener('click', () => {
        exportWorldExperimentProofBundle(node.dataset.auditExportProof || 'markdown', {
          worldName: getActiveExperimentWorldFilter(String(audit?.instance?.world_name || '')),
        });
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-world-baseline-replay]').forEach(node => {
      node.addEventListener('click', async () => {
        await replayExperimentReportsBatch({
          mode: 'replay',
          worldName: decodeURIComponent(node.dataset.worldBaselineReplay || ''),
        });
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-world-baseline-refresh]').forEach(node => {
      node.addEventListener('click', async () => {
        await replayExperimentReportsBatch({
          mode: 'refresh',
          worldName: decodeURIComponent(node.dataset.worldBaselineRefresh || ''),
        });
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-world-baseline-export-json]').forEach(node => {
      node.addEventListener('click', () => {
        exportExperimentBaselines('json', {
          worldName: decodeURIComponent(node.dataset.worldBaselineExportJson || ''),
        });
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-world-baseline-export-md]').forEach(node => {
      node.addEventListener('click', () => {
        exportExperimentBaselines('markdown', {
          worldName: decodeURIComponent(node.dataset.worldBaselineExportMd || ''),
        });
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-world-proof-export-json]').forEach(node => {
      node.addEventListener('click', () => {
        exportWorldExperimentProofBundle('json', {
          worldName: decodeURIComponent(node.dataset.worldProofExportJson || ''),
        });
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-world-proof-export-md]').forEach(node => {
      node.addEventListener('click', () => {
        exportWorldExperimentProofBundle('markdown', {
          worldName: decodeURIComponent(node.dataset.worldProofExportMd || ''),
        });
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-world-replay-advance]').forEach(node => {
      node.addEventListener('click', async () => {
        await advanceExperimentReplayBatch({
          worldName: decodeURIComponent(node.dataset.worldReplayAdvance || ''),
          count: getBatchTickCount(),
        });
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-world-baseline-focus]').forEach(node => {
      node.addEventListener('click', () => {
        setExperimentWorldFilter(decodeURIComponent(node.dataset.worldBaselineFocus || ''));
        renderExperimentReports();
        renderRuntimeAudit();
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-world-baseline-clear]').forEach(node => {
      node.addEventListener('click', () => {
        setExperimentWorldFilter('');
        renderExperimentReports();
        renderRuntimeAudit();
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-world-ops-report-view]').forEach(node => {
      node.addEventListener('click', () => {
        selectExperimentReportContext(
          decodeURIComponent(node.dataset.worldOpsReportView || ''),
          Array.isArray(state.runtimeAudit?.experiment_reports) ? state.runtimeAudit.experiment_reports : state.experimentReports,
        );
        renderExperimentReports();
        renderRuntimeAudit();
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-world-ops-checkpoint]').forEach(node => {
      node.addEventListener('click', () => {
        state.runtimeAuditCheckpointName = decodeURIComponent(node.dataset.worldOpsCheckpoint || '');
        renderExperimentReports();
        renderRuntimeAudit();
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-world-ops-report-export]').forEach(node => {
      node.addEventListener('click', () => {
        const reports = Array.isArray(state.runtimeAudit?.experiment_reports) ? state.runtimeAudit.experiment_reports : state.experimentReports;
        const report = reports.find(item => item.name === decodeURIComponent(node.dataset.worldOpsReportExport || ''));
        exportExperimentReport(report, 'markdown');
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-report-replay-all]').forEach(node => {
      node.addEventListener('click', async () => {
        await replayExperimentReportsBatch({
          mode: 'replay',
          worldName: getActiveExperimentWorldFilter(String(audit?.instance?.world_name || '')),
        });
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-report-refresh-all]').forEach(node => {
      node.addEventListener('click', async () => {
        await replayExperimentReportsBatch({
          mode: 'refresh',
          worldName: getActiveExperimentWorldFilter(String(audit?.instance?.world_name || '')),
        });
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-report-advance-all]').forEach(node => {
      node.addEventListener('click', async () => {
        await advanceExperimentReplayBatch({
          worldName: getActiveExperimentWorldFilter(String(audit?.instance?.world_name || '')),
          count: getBatchTickCount(),
        });
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-checkpoint-open]').forEach(node => {
      node.addEventListener('click', () => {
        state.runtimeAuditCheckpointName = decodeURIComponent(node.dataset.auditCheckpointOpen || '');
        renderRuntimeAudit();
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-checkpoint-name]').forEach(node => {
      node.addEventListener('click', () => {
        state.runtimeAuditCheckpointName = decodeURIComponent(node.dataset.auditCheckpointName || '');
        renderRuntimeAudit();
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-checkpoint-restore]').forEach(node => {
      node.addEventListener('click', async () => {
        const checkpointName = decodeURIComponent(node.dataset.auditCheckpointRestore || '');
        if (!checkpointName) return;
        try {
          await loadCheckpointIntoInstance(String(state.selectedInstanceID || '').trim(), checkpointName, checkpointName);
        } catch (err) {
          alert(`恢复 checkpoint 失败：${err.message}`);
        }
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-report-refresh-replay]').forEach(node => {
      node.addEventListener('click', async () => {
        await refreshExperimentReplayRun(decodeURIComponent(node.dataset.reportRefreshReplay || ''));
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-report-advance-replay]').forEach(node => {
      node.addEventListener('click', async () => {
        try {
          await advanceExperimentReplayRun(decodeURIComponent(node.dataset.reportAdvanceReplay || ''), {
            count: getBatchTickCount(),
          });
        } catch (err) {
          alert(`推进 replay 失败：${err.message}`);
        }
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-report-open-replay-current]').forEach(node => {
      node.addEventListener('click', async () => {
        try {
          await openExperimentReplayInstance(decodeURIComponent(node.dataset.reportOpenReplayCurrent || ''), 'current');
        } catch (err) {
          alert(`打开 current replay 失败：${err.message}`);
        }
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-report-open-replay-compare]').forEach(node => {
      node.addEventListener('click', async () => {
        try {
          await openExperimentReplayInstance(decodeURIComponent(node.dataset.reportOpenReplayCompare || ''), 'compare');
        } catch (err) {
          alert(`打开 compare replay 失败：${err.message}`);
        }
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-replay-open-trace]').forEach(node => {
      node.addEventListener('click', async () => {
        await openReplayTraceTurn(node.dataset.replayOpenTrace, node.dataset.replayTurn);
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-replay-cause]').forEach(node => {
      node.addEventListener('click', () => showCausalChain(node.dataset.replayCause));
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-report-replay]').forEach(node => {
      node.addEventListener('click', () => {
        const reports = Array.isArray(state.runtimeAudit?.experiment_reports) ? state.runtimeAudit.experiment_reports : [];
        const report = reports.find(item => item.name === state.runtimeAuditReportName) || reports[0];
        const snapshot = node.dataset.auditReportReplay === 'compare' ? report?.compare : report?.current;
        if (snapshot?.latest_trace) {
          state.runtimeAuditReplayTrace = snapshot.latest_trace;
          state.runtimeAuditReplayStepIndex = 0;
          state.runtimeAuditFilter = 'trace';
          state.runtimeAuditCause = 'director';
          renderRuntimeAudit();
        }
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-report-restore-current]').forEach(node => {
      node.addEventListener('click', async () => {
        const reports = Array.isArray(state.runtimeAudit?.experiment_reports) ? state.runtimeAudit.experiment_reports : [];
        const report = reports.find(item => item.name === decodeURIComponent(node.dataset.auditReportRestoreCurrent || ''));
        if (!report?.current_checkpoint) return;
        try {
          await loadCheckpointIntoInstance(String(report.source_instance_id || state.selectedInstanceID || '').trim(), report.current_checkpoint, `${report.name} 当前实验`);
        } catch (err) {
          alert(`恢复当前实验失败：${err.message}`);
        }
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-report-restore-compare]').forEach(node => {
      node.addEventListener('click', async () => {
        const reports = Array.isArray(state.runtimeAudit?.experiment_reports) ? state.runtimeAudit.experiment_reports : [];
        const report = reports.find(item => item.name === decodeURIComponent(node.dataset.auditReportRestoreCompare || ''));
        if (!report?.compare_checkpoint || !report?.compare_instance_id) return;
        try {
          await loadCheckpointIntoInstance(String(report.compare_instance_id).trim(), report.compare_checkpoint, `${report.name} 对照实验`);
        } catch (err) {
          alert(`恢复对照实验失败：${err.message}`);
        }
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-replay-step]').forEach(node => {
      node.addEventListener('click', () => {
        state.runtimeAuditReplayStepIndex = Math.max(0, Number(node.dataset.auditReplayStep || 0));
        renderRuntimeAudit();
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-replay-prev]').forEach(node => {
      node.addEventListener('click', () => {
        state.runtimeAuditReplayStepIndex = Math.max(0, Number(state.runtimeAuditReplayStepIndex || 0) - 1);
        renderRuntimeAudit();
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-audit-replay-next]').forEach(node => {
      node.addEventListener('click', () => {
        const steps = Array.isArray(state.runtimeAuditReplayTrace?.step_traces) ? state.runtimeAuditReplayTrace.step_traces : [];
        state.runtimeAuditReplayStepIndex = Math.min(Math.max(steps.length - 1, 0), Number(state.runtimeAuditReplayStepIndex || 0) + 1);
        renderRuntimeAudit();
      });
    });
    els.runtimeAuditPanel.querySelectorAll('[data-proof-audit-export]').forEach(node => {
      node.addEventListener('click', () => {
        const name = decodeURIComponent(String(node.dataset.proofAuditExport || ''));
        const format = String(node.dataset.proofAuditFormat || 'markdown').toLowerCase();
        const item = (state.proofAudits?.proof_audits || []).find(entry => String(entry?.name || '') === name);
        if (!item) {
          alert('当前 proof audit 不存在');
          return;
        }
        exportProofAuditSummary(item, format === 'json' ? 'json' : 'markdown');
      });
    });
  }
}

async function loadRuntimeAudit() {
  try {
    const [data, proofAudits] = await Promise.all([
      fetchJSON('/api/runtime-audit?trace_limit=8&checkpoint_limit=5&preset_limit=5&report_limit=5&population_limit=5'),
      fetchJSON('/api/proof-audits?limit=5').catch(err => ({ error: err.message || String(err) })),
    ]);
    state.runtimeAudit = data || null;
    state.proofAudits = proofAudits || null;
    const reports = Array.isArray(data?.experiment_reports) ? data.experiment_reports : [];
    const checkpoints = Array.isArray(data?.checkpoints) ? data.checkpoints : [];
    if (!reports.some(report => report.name === state.runtimeAuditReportName)) {
      state.runtimeAuditReportName = reports[0]?.name || '';
    }
    if (!checkpoints.some(slot => slot?.name === state.runtimeAuditCheckpointName)) {
      state.runtimeAuditCheckpointName = checkpoints[0]?.name || '';
    }
    const preferredTrace = state.runtimeAuditReplayTrace && Number(state.runtimeAuditReplayTrace.turn || 0) > 0
      ? Number(state.runtimeAuditReplayTrace.turn || 0)
      : Number(data?.latest_trace?.turn || data?.recent_traces?.[0]?.turn || 0);
    if (preferredTrace > 0) {
      const replayAlreadyLoaded = Number(state.runtimeAuditReplayTrace?.turn || 0) === preferredTrace;
      if (!replayAlreadyLoaded) {
        const trace = await fetchTraceJSON(preferredTrace).catch(() => null);
        state.runtimeAuditReplayTrace = trace;
        state.runtimeAuditReplayStepIndex = 0;
      }
    } else {
      state.runtimeAuditReplayTrace = null;
      state.runtimeAuditReplayStepIndex = 0;
    }
    renderRuntimeAudit();
  } catch (err) {
    if (isInstanceNotFoundError(err) && await recoverSelectedInstanceFromNotFound()) {
      await loadRuntimeAudit();
      return;
    }
    console.error('runtime audit error:', err);
    state.runtimeAudit = null;
    state.proofAudits = null;
    state.runtimeAuditReplayTrace = null;
    state.runtimeAuditReplayStepIndex = 0;
    renderInfoList('runtime-audit-summary', [], `读取失败：${err.message}`);
    renderInfoList('runtime-audit-panel', [], `读取失败：${err.message}`);
  }
}

async function loadExperimentReports() {
  try {
    const data = await fetchJSON('/api/experiment-reports');
    state.experimentReports = Array.isArray(data.reports) ? data.reports : [];
    renderExperimentReports();
  } catch (err) {
    if (isInstanceNotFoundError(err) && await recoverSelectedInstanceFromNotFound()) {
      await loadExperimentReports();
      return;
    }
    console.error('experiment reports error:', err);
    state.experimentReports = [];
    state.experimentReplayBatchSummary = null;
    renderInfoList('sim-report-list', [], `读取失败：${err.message}`);
  }
}

async function saveExperimentReport() {
  const payload = await buildCurrentExperimentReportPayload();
  if (!payload || !payload.current) {
    alert('当前还没有可归档的 simulation 数据');
    return;
  }
  if (!payload.name) {
    alert('请输入实验报告名');
    return;
  }
  try {
    const currentCheckpoint = await createCheckpointForInstance(
      String(payload.source_instance_id || state.selectedInstanceID || '').trim(),
      buildExperimentCheckpointName(payload.name, 'current'),
      `experiment report ${payload.name} current snapshot`,
    );
    payload.current_checkpoint = currentCheckpoint?.name || '';
    if (payload.compare && payload.compare_instance_id) {
      const compareCheckpoint = await createCheckpointForInstance(
        String(payload.compare_instance_id).trim(),
        buildExperimentCheckpointName(payload.name, 'compare'),
        `experiment report ${payload.name} compare snapshot`,
      );
      payload.compare_checkpoint = compareCheckpoint?.name || '';
    }
  } catch (err) {
    alert(`创建实验 checkpoint 失败：${err.message}`);
    return;
  }
  const resp = await apiFetch('/api/experiment-reports', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!resp.ok) {
    const message = await resp.text();
    alert(`保存实验报告失败：${message || resp.statusText}`);
    return;
  }
  const saved = await resp.json();
  els.simReportName.value = saved.name || '';
  renderSceneDivider(`已保存实验报告 ${safeText(saved.name)}`);
  await Promise.all([loadExperimentReports(), loadRuntimeAudit()]);
}

async function exportCurrentExperimentReport(format) {
  const payload = await buildCurrentExperimentReportPayload();
  if (!payload || !payload.current) {
    alert('当前没有可导出的实验数据');
    return;
  }
  payload.created_at = new Date().toISOString();
  exportExperimentReport(payload, format);
}

function applySimStatus(data, previous = state.lastSimStatus) {
  els.simStatus.textContent = data.paused ? '已暂停' : data.running ? '运行中' : '未启动';
  els.simStatus.style.color = data.paused ? 'var(--warning)' : data.running ? 'var(--success)' : 'var(--text-muted)';
  els.simTickCount.textContent = String(data.tick_count ?? 0);
  els.simWorldAdvance.textContent = data.world_advance ?? '0s';
  els.simTurnCount.textContent = String(data.turn_count ?? 0);
  els.simPauseBtn.style.display = data.paused ? 'none' : '';
  els.simResumeBtn.style.display = data.paused ? '' : 'none';
  renderSimEvolution(data);
  renderSimDiagnostics(data);
  renderTrajectorySummary(data);
  renderLastTickSummary(data);
  renderTickHistory(data);
  renderSimCompareSummary(data, previous);
  state.lastSimStatus = JSON.parse(JSON.stringify(data || {}));
}

async function loadSimStatus() {
  try {
    const data = await fetchJSON('/api/sim/status');
    applySimStatus(data);
    await loadCompareInstanceStatus(data);
    await loadRuntimeAudit();
  } catch (err) {
    console.error('sim status error:', err);
  }
}

function getBatchTickCount() {
  const value = Number(els.simBatchCount?.value || 1);
  if (!Number.isFinite(value)) return 1;
  return Math.max(1, Math.min(200, Math.trunc(value)));
}

async function manualTick() {
  try {
    await apiFetch('/api/sim/tick', { method: 'POST' });
    await loadSimStatus();
  } catch (err) {
    alert('手动 Tick 失败');
  }
}

async function batchTickCurrent() {
  const count = getBatchTickCount();
  try {
    await apiFetch('/api/sim/tick', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ count }),
    });
    renderSceneDivider(`实例 ${safeText(state.selectedInstanceID, '--')} 批量推进 ${count} ticks`);
    await loadSimStatus();
  } catch (err) {
    alert(`批量 Tick 失败：${err.message}`);
  }
}

async function batchTickCompareInstances() {
  const count = getBatchTickCount();
  const compareID = String(state.compareInstanceID || '').trim();
  if (!compareID || compareID === state.selectedInstanceID) {
    alert('请先选择一个对照实例');
    return;
  }
  try {
    await apiFetch('/api/sim/tick', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ count }),
    });
    await fetch(buildInstanceAPIURL('/api/sim/tick', compareID), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ count }),
    }).then(async resp => {
      if (!resp.ok) {
        const detail = await resp.text().catch(() => '');
        throw new Error(`${resp.status} ${resp.statusText}${detail ? `: ${detail.trim()}` : ''}`);
      }
      return resp.json();
    });
    renderSceneDivider(`实例 ${safeText(state.selectedInstanceID, '--')} 与 ${safeText(compareID, '--')} 同步推进 ${count} ticks`);
    await loadSimStatus();
  } catch (err) {
    alert(`双实例同步推进失败：${err.message}`);
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
        <span class="tag mono">${safeText(stepTrace.speaker, '--')}</span>
      </div>
    </div>
  `;
}

function formatMetricMap(metrics, formatter) {
  const entries = Object.entries(metrics || {});
  if (!entries.length) return '--';
  return entries.map(([key, value]) => `${safeText(key)}:${formatter(value)}`).join(' · ');
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
  els.traceStatus.textContent = selected ? `turn ${selected.turn} · ${safeText(selected.focus_character)}` : 'turn --';
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
  const sameCharacter = state.traceHistoryItems.filter(trace => { const fc = slot?.focus_character; return !fc || trace.focus_character === fc; });
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
      focus_character: els.instanceCreateFocus.value.trim(),
    }),
  });
  if (!resp.ok) {
    const message = await resp.text();
    alert(`创建实例失败：${message || resp.statusText}`);
    return;
  }
  els.instanceCreateID.value = '';
  els.instanceCreateLabel.value = '';
  els.instanceCreateFocus.value = '';
  renderSceneDivider(`实例 ${id} 已创建`);
  await loadInstancesView();
}

function buildExperimentInstanceDraft() {
  const sourceID = String(state.selectedInstanceID || '').trim();
  const stamp = new Date().toISOString().replace(/[-:TZ.]/g, '').slice(0, 12);
  const id = (els.instanceCreateID.value.trim() || `${sourceID || 'exp'}-lab-${stamp}`).toLowerCase();
  const label = els.instanceCreateLabel.value.trim() || `实验分支 ${sourceID || '--'} ${stamp}`;
  const focusCharacter = els.instanceCreateFocus.value.trim();
  return { sourceID, id, label, focusCharacter };
}

async function createExperimentInstance() {
  const draft = buildExperimentInstanceDraft();
  if (!draft.sourceID) {
    alert('当前没有可派生的源实例');
    return;
  }
  const resp = await apiFetch('/api/instances/create', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      source_id: draft.sourceID,
      id: draft.id,
      label: draft.label,
      focus_character: draft.focusCharacter,
    }),
  });
  if (!resp.ok) {
    const message = await resp.text();
    alert(`创建实验分支失败：${message || resp.statusText}`);
    return;
  }
  const summary = await resp.json();
  state.compareInstanceID = summary.id || draft.id;
  localStorage.setItem('corerp-compare-instance-id', state.compareInstanceID);
  els.instanceCreateID.value = '';
  els.instanceCreateLabel.value = '';
  els.instanceCreateFocus.value = '';
  renderSceneDivider(`已从 ${safeText(draft.sourceID)} 派生实验分支 ${safeText(state.compareInstanceID)}`);
  await loadInstancesView();
  await refreshInstanceCompare(state.lastSimStatus);
}

function syncCompareInstanceSelection() {
  const candidates = state.instances.filter(instance => instance.id !== state.selectedInstanceID);
  const saved = String(state.compareInstanceID || localStorage.getItem('corerp-compare-instance-id') || '').trim();
  const knownIDs = new Set(candidates.map(instance => instance.id));
  const fallback = candidates.find(instance => instance.id === state.defaultInstanceID)?.id || candidates[0]?.id || '';
  const selected = knownIDs.has(saved) ? saved : fallback;
  state.compareInstanceID = selected;
  if (selected) {
    localStorage.setItem('corerp-compare-instance-id', selected);
  } else {
    localStorage.removeItem('corerp-compare-instance-id');
  }

  els.instanceCompareSelect.innerHTML = '';
  const empty = document.createElement('option');
  empty.value = '';
  empty.textContent = '不对照';
  empty.selected = !selected;
  els.instanceCompareSelect.appendChild(empty);
  candidates.forEach(instance => {
    const opt = document.createElement('option');
    opt.value = instance.id;
    opt.textContent = `${instance.label || instance.id} · ${instance.world_name || '--'}`;
    opt.selected = instance.id === selected;
    els.instanceCompareSelect.appendChild(opt);
  });
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
    syncCompareInstanceSelection();

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
          <div class="row-subtitle">${instance.world_name || '--'} · 视角 ${instance.focus_character || '--'} · 参与者 ${((instance.participant_details || []).length ? instance.participant_details.map(item => item.name) : (instance.participants || [])).join(', ') || '--'}</div>
          <div class="row-subtitle">${instanceStatusLabel(instance.status)}${instance.is_default ? ' · 默认实例' : ''} · 创建于 ${instance.created_at ? new Date(instance.created_at).toLocaleString('zh-CN') : '--'}</div>
        </div>
        <div class="row-actions">
          ${instance.is_default ? '<span class="pill">默认</span>' : `<button type="button" class="ghost-button" data-instance-default="${instance.id}">设为默认</button>`}
          ${instance.id === state.selectedInstanceID ? '<span class="pill">当前实例</span>' : `<button type="button" class="ghost-button" data-instance-view="${instance.id}">查看</button>`}
          ${instance.id === state.compareInstanceID ? '<span class="pill">对照实例</span>' : (instance.id === state.selectedInstanceID ? '' : `<button type="button" class="ghost-button" data-instance-compare="${instance.id}">设为对照</button>`)}
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
    els.instanceList.querySelectorAll('[data-instance-compare]').forEach(node => {
      node.addEventListener('click', () => switchCompareInstance(node.dataset.instanceCompare));
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
    els.instanceCompareSummary.textContent = '实例对照读取失败';
    els.simInstanceCompareSummary.textContent = '实例对照读取失败';
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
  await Promise.all([loadInstancesView(), loadWorlds(), loadCharacters(), loadPlayerRole(), restoreDialogue(), refreshPanel(), loadTimeline(), loadMemoryView(), loadCharacterConfig(), loadSaveSlots(), loadScenarioPresets(), loadTraceHistory(), loadTraceView(), loadExperimentReports()]);
}

async function switchCompareInstance(id) {
  state.compareInstanceID = String(id || '').trim();
  if (state.compareInstanceID) {
    localStorage.setItem('corerp-compare-instance-id', state.compareInstanceID);
  } else {
    localStorage.removeItem('corerp-compare-instance-id');
  }
  await loadInstancesView();
  await loadCompareInstanceStatus(state.lastSimStatus);
}

async function loadCompareInstanceStatus(currentStatus = state.lastSimStatus) {
  const compareID = String(state.compareInstanceID || '').trim();
  if (!compareID || compareID === state.selectedInstanceID) {
    state.compareSimStatus = null;
    renderInstanceCompareSummary(currentStatus, null);
    return;
  }
  try {
    const compareStatus = await fetchJSONForInstance('/api/sim/status', compareID);
    state.compareSimStatus = JSON.parse(JSON.stringify(compareStatus || {}));
    renderInstanceCompareSummary(currentStatus, compareStatus);
  } catch (err) {
    console.error('compare instance sim status error:', err);
    state.compareSimStatus = null;
    els.instanceCompareSummary.textContent = `实例对照读取失败：${err.message}`;
    els.simInstanceCompareSummary.textContent = `实例对照读取失败：${err.message}`;
  }
}

async function refreshInstanceCompare(currentStatus = state.lastSimStatus) {
  if (!state.instances.length) {
    els.instanceCompareSummary.textContent = '当前未启用实例对照';
    els.simInstanceCompareSummary.textContent = '当前未启用实例对照';
    return;
  }
  await loadCompareInstanceStatus(currentStatus);
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
    const data = await fetchJSON(`/api/focus-definition-config?focus_character=${encodeURIComponent(els.charSelect.value || '')}`);
    const card = data.card || {};
    const identity = card.identity || {};
    els.charcfgName.value = data.focus_character || identity.name || '';
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
    state.lastStructureSnapshot = JSON.parse(JSON.stringify(data || {}));
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
  const previous = state.lastStructureSnapshot ? JSON.parse(JSON.stringify(state.lastStructureSnapshot)) : null;
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
  const nextSnapshot = {
    seed: {
      premise: els.structurePremise.value.trim(),
      current_situation: els.structureSituation.value.trim(),
      starting_scene: els.structureStartScene.value.trim(),
      time_anchor: els.structureTimeAnchor.value.trim(),
      stability: els.structureStability.value.trim(),
    },
    factions,
    locations,
    pressures,
    ruleset: { rules },
  };
  state.lastStructureChangeSummary = summarizeStructureChanges(previous, nextSnapshot);
  await Promise.all([loadWorldStructure(), refreshPanel(), loadSimStatus()]);
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
    $('pending-focus').textContent = safeText(els.charSelect.value || '--');
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
    $('pending-focus').textContent = safeText(els.charSelect.value || '--');
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
          <div class="row-title">turn ${trace.turn || 0} · ${safeText(trace.focus_character)}</div>
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
      <div class="note-box mono">turn ${trace.turn || 0} · ${safeText(trace.focus_character)} · input: ${safeText(trace.user_input, '--')}</div>
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
    if (Array.isArray(trace.director_plan?.world_signals) && trace.director_plan.world_signals.length) {
      items.push(`<div class="note-box mono">director-world: ${trace.director_plan.world_signals.map(item => safeText(item)).join(' · ')}</div>`);
    }
    if (trace.world_metrics && (typeof trace.world_metrics.tension === 'number' || Object.keys(trace.world_metrics.pressure_states || {}).length || Object.keys(trace.world_metrics.faction_tensions || {}).length || Object.keys(trace.world_metrics.npc_exposure || {}).length)) {
      items.push(`
        <div class="note-box mono">world: tension ${Number(trace.world_metrics.tension || 0).toFixed(2)} · pressure ${formatMetricMap(trace.world_metrics.pressure_states, value => Number(value).toFixed(2))} · faction ${formatMetricMap(trace.world_metrics.faction_tensions, value => Number(value).toFixed(2))} · exposure ${formatMetricMap(trace.world_metrics.npc_exposure, value => String(value))}</div>
      `);
      if (Array.isArray(trace.world_metrics.population_highlights) && trace.world_metrics.population_highlights.length) {
        items.push(`<div class="note-box mono">population: ${trace.world_metrics.population_highlights.map(item => safeText(item)).join(' · ')}</div>`);
      }
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
      const dominant = selectedCandidates
        .map(candidate => `${safeText(candidate.name)}:${Array.isArray(candidate.dominant_factors) && candidate.dominant_factors.length ? candidate.dominant_factors.map(item => safeText(item)).join('/') : '--'}`)
        .join(' · ');
      if (dominant) {
        items.push(`<div class="note-box mono">drivers: ${dominant}</div>`);
      }
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

  const resp = await apiFetch('/api/focus-definition-config', {
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
          <div class="row-subtitle">${safeText(slot.focus_character)} · ${slot.branch} · ${new Date(slot.created_at).toLocaleString('zh-CN')}</div>
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
          <div class="row-subtitle">${safeText(preset.focus_character)} · ${safeText(preset.branch, 'main')} · ${new Date(preset.created_at).toLocaleString('zh-CN')}</div>
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
  renderSceneDivider(`进入 ${safeText(data.world, path)} · 视角 ${safeText(data.focus_character, '--')}`);
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
    const [stateData, debugData, characterConfig, actionStats, actionLog, populationInsights, world, branches, compression, usage, simStatus] = await Promise.all([
      fetchJSON('/api/state'),
      fetchJSON('/api/debug/memory'),
      fetchJSON(`/api/focus-definition-config?focus_character=${encodeURIComponent(els.charSelect.value || '')}`).catch(() => ({})),
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
    updateCharacterCard(characterConfig.card || {});
    updateMemoryPanel(debugData);
    updateNPCPanels(debugData, actionStats, actionLog);
    updatePopulationRuntimePanel(populationInsights);
    updateWorldPanel(world, branches, compression);
    updateUsagePanel(usage);
    state.branches = branches.branches || [];
    refreshDiffSelectors();
    updateDirectorPanel(stateData);
    if (simStatus && simStatus.tick_count != null) {
      applySimStatus(simStatus);
    }
    await Promise.all([loadInstancesView(), loadMemoryView(), loadCharacterConfig(), loadSaveSlots(), loadScenarioPresets(), loadWorldConfig(), loadSceneConfigs(), loadCanonFacts(), loadQuarantineView(), loadPendingFactsView(), loadDirectorConfig(), loadTraceHistory(), loadTraceView(), loadWorldStructure(), loadPopulationConfig(), loadRuntimeAudit()]);
    await refreshInstanceCompare(simStatus);
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
  els.instanceCompareSelect.addEventListener('change', event => switchCompareInstance(event.target.value));
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
  els.instanceCreateExperimentBtn.addEventListener('click', createExperimentInstance);
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
  if (els.runtimeAuditRefreshBtn) {
    els.runtimeAuditRefreshBtn.addEventListener('click', loadRuntimeAudit);
  }
  if (els.runtimeAuditFilter) {
    els.runtimeAuditFilter.value = state.runtimeAuditFilter;
    els.runtimeAuditFilter.addEventListener('change', event => {
      state.runtimeAuditFilter = event.target.value || 'all';
      renderRuntimeAudit();
    });
  }
  if (els.runtimeAuditCause) {
    els.runtimeAuditCause.value = state.runtimeAuditCause;
    els.runtimeAuditCause.addEventListener('change', event => {
      state.runtimeAuditCause = event.target.value || 'all';
      renderRuntimeAudit();
    });
  }
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
  els.simBatchBtn.addEventListener('click', batchTickCurrent);
  els.simBatchCompareBtn.addEventListener('click', batchTickCompareInstances);
  els.simPauseBtn.addEventListener('click', pauseTick);
  els.simResumeBtn.addEventListener('click', resumeTick);
  els.simReportSaveBtn.addEventListener('click', () => { saveExperimentReport(); });
  els.simReportRefreshBtn.addEventListener('click', loadExperimentReports);
  els.simReportBatchReplayBtn.addEventListener('click', () => { replayExperimentReportsBatch({ mode: 'replay' }); });
  els.simReportBatchRefreshBtn.addEventListener('click', () => { replayExperimentReportsBatch({ mode: 'refresh' }); });
  els.simReportExportJSONBtn.addEventListener('click', () => { exportCurrentExperimentReport('json'); });
  els.simReportExportMDBtn.addEventListener('click', () => { exportCurrentExperimentReport('markdown'); });
  els.simReportExportBaselineJSONBtn.addEventListener('click', () => { exportExperimentBaselines('json'); });
  els.simReportExportBaselineMDBtn.addEventListener('click', () => { exportExperimentBaselines('markdown'); });

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
  state.compareInstanceID = String(localStorage.getItem('corerp-compare-instance-id') || '').trim();
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
    loadExperimentReports(),
    loadRuntimeAudit(),
  ]);

  setInterval(refreshPanel, 15000);
  setInterval(() => loadTimeline(els.timelineBranch.value || ''), 30000);
  els.input.focus();
}

init();

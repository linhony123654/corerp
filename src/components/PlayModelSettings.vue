<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import PlayIcon from './PlayIcon.vue'
import {
  type ApiProfile,
  type PresetTemplate,
  PRESET_TEMPLATES,
  loadProfiles,
  getActiveProfileId,
  setActiveProfileId,
  upsertProfile,
  deleteProfile,
  duplicateProfile,
  createEmptyProfile,
  testApiConnection,
  fetchAvailableModels,
  exportProfilesJson,
  importProfilesJson,
} from '../lib/apiProfiles'

const props = defineProps<{
  serverDecisionMode: string
  serverNarrativeMode: string
  authToken?: string
}>()

const emit = defineEmits<{
  select: [profile: ApiProfile | null]
  update: []
}>()

const profiles = ref<ApiProfile[]>([])
const activeId = ref<string | null>(null)
const editingProfile = ref<ApiProfile | null>(null)
const form = computed(() => editingProfile.value || createEmptyProfile())
const isNew = ref(false)
const showApiKey = ref(false)
const showAdvanced = ref(false)
const testing = ref(false)
const testResult = ref<{ ok: boolean; latencyMs: number; message: string } | null>(null)
const fetchingModels = ref(false)
const fetchedModels = ref<string[]>([])
const fetchModelStatus = ref('')
const fetchModelOk = ref(false)
const toastMsg = ref('')
const showImport = ref(false)
const importRaw = ref('')
const importError = ref('')

function notify(msg: string) {
  toastMsg.value = msg
  setTimeout(() => {
    if (toastMsg.value === msg) toastMsg.value = ''
  }, 2600)
}

function reload() {
  profiles.value = loadProfiles()
  const saved = getActiveProfileId()
  activeId.value = profiles.value.some(p => p.id === saved && p.protocol === 'chat_completions') ? saved : null
  if (saved && !activeId.value) setActiveProfileId(null)
}

onMounted(() => {
  reload()
})

function chooseActive(id: string | null) {
  const candidate = id ? profiles.value.find(p => p.id === id) : null
  if (candidate && candidate.protocol !== 'chat_completions') {
    notify('此配置使用暂不支持的 Responses 协议，不能设为当前模型。')
    return
  }
  setActiveProfileId(id)
  activeId.value = id
  const target = id ? profiles.value.find(p => p.id === id) || null : null
  emit('select', target)
  emit('update')
  notify(target ? `已切换到模型配置：${target.name} (${target.model})` : '已切回系统默认')
}

function startCreate(preset?: PresetTemplate) {
  isNew.value = true
  showApiKey.value = false
  showAdvanced.value = false
  testResult.value = null
  fetchedModels.value = []
  fetchModelStatus.value = ''
  fetchModelOk.value = false
  if (preset) {
    editingProfile.value = createEmptyProfile({
      name: preset.name,
      protocol: preset.protocol,
      endpoint: preset.endpoint,
      model: preset.model,
      temperature: preset.temperature,
      maxTokens: preset.maxTokens,
    })
  } else {
    editingProfile.value = createEmptyProfile()
  }
}

function startEdit(profile: ApiProfile) {
  isNew.value = false
  showApiKey.value = false
  showAdvanced.value = false
  testResult.value = null
  fetchedModels.value = []
  fetchModelStatus.value = ''
  fetchModelOk.value = false
  editingProfile.value = {
    reasoningEffort: '', disableThinking: false, decisionMaxTokens: 0, interactionMaxTokens: 0, decisionFormat: '',
    ...JSON.parse(JSON.stringify(profile)),
  }
}

function startEditWithPull(profile: ApiProfile) {
  startEdit(profile)
  void pullModels()
}

function applyPreset(preset: PresetTemplate) {
  if (!editingProfile.value) return
  editingProfile.value.protocol = preset.protocol
  editingProfile.value.endpoint = preset.endpoint
  editingProfile.value.model = preset.model
  if (preset.temperature !== undefined) editingProfile.value.temperature = preset.temperature
  if (preset.maxTokens !== undefined) editingProfile.value.maxTokens = preset.maxTokens
  notify(`已填入 ${preset.name} 预设`)
}

function cancelEdit() {
  editingProfile.value = null
  testResult.value = null
  fetchedModels.value = []
  fetchModelStatus.value = ''
}

async function pullModels() {
  if (!editingProfile.value || fetchingModels.value) return
  if (editingProfile.value.protocol !== 'chat_completions') { notify('Responses 配置暂不支持查询，请新建兼容配置。'); return }
  const endpoint = editingProfile.value.endpoint?.trim()
  if (!endpoint) {
    notify('请先填写接口 Endpoint 地址')
    return
  }
  fetchingModels.value = true
  fetchModelStatus.value = ''
  try {
    const res = await fetchAvailableModels(
      endpoint,
      editingProfile.value.apiKey,
      editingProfile.value.timeoutSeconds || 15,
      props.authToken
    )
    fetchModelOk.value = res.ok
    fetchModelStatus.value = res.message
    if (res.ok) {
      fetchedModels.value = res.models
      notify(res.message)
    } else {
      notify(res.message)
    }
  } catch (err: unknown) {
    fetchModelOk.value = false
    fetchModelStatus.value = (err as Error).message || '拉取模型异常'
    notify('拉取失败')
  } finally {
    fetchingModels.value = false
  }
}

function onModelDropdownChange(e: Event) {
  const target = e.target as HTMLSelectElement
  if (target && target.value && editingProfile.value) {
    editingProfile.value.model = target.value
    notify(`已选择模型：${target.value}`)
  }
}

async function runConnectionTest() {
  if (!editingProfile.value || testing.value) return
  if (editingProfile.value.protocol !== 'chat_completions') { notify('Responses 配置暂不支持测试，请新建兼容配置。'); return }
  testing.value = true
  testResult.value = null
  try {
    const res = await testApiConnection(editingProfile.value, props.authToken)
    testResult.value = res
  } catch (err: unknown) {
    testResult.value = {
      ok: false,
      latencyMs: 0,
      message: (err as Error).message || '连接异常',
    }
  } finally {
    testing.value = false
  }
}

function saveAndStay() {
  if (!editingProfile.value) return
  if (editingProfile.value.protocol !== 'chat_completions') { notify('当前仅支持 Chat Completions，请新建兼容配置。'); return }
  if (!editingProfile.value.name.trim()) {
    notify('请填写配置名称')
    return
  }
  if (!editingProfile.value.endpoint.trim()) {
    notify('请填写 Endpoint 地址')
    return
  }
  try { upsertProfile(editingProfile.value) }
  catch (err) { notify((err as Error).message); return }
  reload()
  emit('update')
  notify('配置已保存')
  editingProfile.value = null
}

function saveAndActivate() {
  if (!editingProfile.value) return
  if (editingProfile.value.protocol !== 'chat_completions') { notify('当前仅支持 Chat Completions，请新建兼容配置。'); return }
  if (!editingProfile.value.name.trim()) {
    notify('请填写配置名称')
    return
  }
  if (!editingProfile.value.endpoint.trim()) {
    notify('请填写 Endpoint 地址')
    return
  }
  try { upsertProfile(editingProfile.value) }
  catch (err) { notify((err as Error).message); return }
  const savedId = editingProfile.value.id
  reload()
  chooseActive(savedId)
  editingProfile.value = null
}

function duplicate(id: string) {
  const created = duplicateProfile(id)
  reload()
  emit('update')
  if (created) notify(`已复制一份新配置：${created.name}`)
}

function remove(id: string) {
  const p = profiles.value.find(item => item.id === id)
  const name = p ? p.name : '该配置'
  if (window.confirm(`确认删除 API 配置「${name}」？此操作只移除浏览器本地存储。`)) {
    deleteProfile(id)
    reload()
    emit('update')
    notify(`已删除配置：${name}`)
  }
}

function downloadExport() {
  const json = exportProfilesJson()
  const blob = new Blob([json], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `corerp-api-profiles-${new Date().toISOString().slice(0, 10)}.json`
  a.click()
  URL.revokeObjectURL(url)
  notify('已导出配置文件')
}

function doImport() {
  importError.value = ''
  if (!importRaw.value.trim()) {
    importError.value = '请粘贴 JSON 文本'
    return
  }
  const res = importProfilesJson(importRaw.value)
  if (res.ok) {
    reload()
    emit('update')
    showImport.value = false
    importRaw.value = ''
    notify(`成功导入 ${res.count} 项配置`)
  } else {
    importError.value = res.error || '导入失败，请检查格式'
  }
}
</script>

<template>
  <div class="model-settings-pane">
    <div v-if="toastMsg" class="settings-toast" role="status">{{ toastMsg }}</div>

    <Transition name="pane-fade" mode="out-in">
      <!-- Mode A: Profiles List -->
      <div v-if="!editingProfile" key="list" class="settings-view-wrapper">
        <p class="detail-note">
        当前仅支持 Chat Completions 兼容接口。切换配置后，输入框左侧显示所选模型；系统默认是否使用 AI 由服务端配置决定，实际成功情况见调用回执。
      </p>

      <div class="setting-group" role="radiogroup" aria-label="模型与 API 配置">
        <!-- System Default / Deterministic -->
        <button
          type="button"
          class="detail-row clickable option-row"
          role="radio"
          :aria-checked="activeId === null"
          @click="chooseActive(null)"
        >
          <span class="row-symbol"><PlayIcon name="settings" :size="18" /></span>
          <span class="row-copy">
            <strong>系统默认 · {{ serverDecisionMode === 'chat_completions' ? 'AI 人物' : '确定性人物' }}</strong>
            <small>{{ serverDecisionMode === 'chat_completions' ? '服务端已连接在线模型 · 受世界规则约束' : '本地确定性规则 · 无需外部 API' }}</small>
          </span>
          <span class="row-end">
            <PlayIcon v-if="activeId === null" name="check" :size="17" />
          </span>
        </button>

        <!-- Custom Profiles -->
        <div
          v-for="p in profiles"
          :key="p.id"
          class="detail-row option-row custom-profile-row"
          role="radio"
          :aria-checked="activeId === p.id"
          :aria-disabled="p.protocol !== 'chat_completions'"
          @click="chooseActive(p.id)"
        >
          <span class="row-symbol"><PlayIcon name="settings" :size="18" /></span>
          <span class="row-copy">
            <strong>
              {{ p.name }}
              <span class="model-badge">{{ p.model }}</span>
            </strong>
            <small>{{ p.protocol === 'chat_completions' ? p.endpoint : 'Responses 协议暂不支持 · 不可启用' }}</small>
          </span>
          <div class="profile-actions" @click.stop>
            <button
              v-if="activeId === p.id"
              type="button"
              class="active-tag"
              title="当前生效配置"
            >
              <PlayIcon name="check" :size="15" />
              <span>当前生效</span>
            </button>
            <button
              type="button"
              class="icon-action-btn"
              title="拉取该端点模型列表"
              aria-label="拉取该端点模型列表"
              @click="startEditWithPull(p)"
            >
              <PlayIcon name="refresh" :size="15" />
            </button>
            <button
              type="button"
              class="icon-action-btn"
              title="编辑该配置"
              aria-label="编辑该配置"
              @click="startEdit(p)"
            >
              <PlayIcon name="edit" :size="15" />
            </button>
            <button
              type="button"
              class="icon-action-btn"
              title="复制副本"
              aria-label="复制副本"
              @click="duplicate(p.id)"
            >
              <PlayIcon name="copy" :size="15" />
            </button>
            <button
              type="button"
              class="icon-action-btn danger-action"
              title="删除配置"
              aria-label="删除配置"
              @click="remove(p.id)"
            >
              <PlayIcon name="trash" :size="15" />
            </button>
          </div>
        </div>
      </div>

      <div class="preset-section">
        <h3 class="section-heading">快速新建或应用模板</h3>
        <div class="preset-chips">
          <button
            v-for="preset in PRESET_TEMPLATES"
            :key="preset.name"
            type="button"
            class="preset-chip"
            :title="preset.description"
            @click="startCreate(preset)"
          >
            <span>+ {{ preset.name }}</span>
          </button>
        </div>
      </div>

      <div class="list-management-actions">
        <button type="button" class="solid-button" @click="startCreate()">
          <PlayIcon name="plus" :size="16" />
          <span>新建空白配置</span>
        </button>
        <button type="button" class="flat-button" @click="downloadExport">
          <PlayIcon name="export" :size="16" />
          <span>导出配置</span>
        </button>
        <button type="button" class="flat-button" @click="showImport = !showImport">
          <span>{{ showImport ? '收起导入' : '导入配置' }}</span>
        </button>
      </div>

      <!-- Import Drawer / Box -->
      <div v-if="showImport" class="import-box">
        <label class="form-label" for="import-json-area">粘贴导出的 JSON 配置文件内容：</label>
        <textarea
          id="import-json-area"
          v-model="importRaw"
          class="text-field import-textarea"
          rows="4"
          placeholder='{"version": 1, "profiles": [...]}'
        />
        <p v-if="importError" class="import-error-msg">{{ importError }}</p>
        <div class="form-actions">
          <button type="button" class="flat-button" @click="showImport = false">取消</button>
          <button type="button" class="solid-button" @click="doImport">确认导入</button>
        </div>
      </div>
      </div>

      <!-- Mode B: Profile Edit / Create Form -->
      <div v-else key="edit" class="settings-view-wrapper">
        <div class="edit-nav-bar">
        <button type="button" class="flat-button back-to-list-btn" @click="cancelEdit">
          <PlayIcon name="back" :size="16" />
          <span>返回列表</span>
        </button>
        <strong class="edit-mode-title">{{ isNew ? '新建 API 配置' : '编辑配置' }}</strong>
      </div>

      <div class="preset-section">
        <div class="section-heading">一键套用服务商预设</div>
        <div class="preset-chips">
          <button
            v-for="preset in PRESET_TEMPLATES"
            :key="preset.name"
            type="button"
            class="preset-chip"
            :title="preset.description"
            @click="applyPreset(preset)"
          >
            <span>{{ preset.name }}</span>
          </button>
        </div>
      </div>

      <form class="profile-form" @submit.prevent="saveAndActivate">
        <div class="form-item">
          <label class="form-label" for="cfg-name">配置名称 <span class="required-star">*</span></label>
          <input
            id="cfg-name"
            v-model="form.name"
            class="text-field"
            maxlength="40"
            placeholder="例如：DeepSeek V3 (主力)"
            required
          />
        </div>

        <div class="form-item">
          <label class="form-label" for="cfg-endpoint">接口 Endpoint <span class="required-star">*</span></label>
          <input
            id="cfg-endpoint"
            v-model="form.endpoint"
            class="text-field"
            placeholder="https://api.deepseek.com/v1/chat/completions"
            required
          />
          <small class="form-hint">支持完整 completions 端点或兼容 OpenAI 规范的 Base URL</small>
        </div>

        <div class="form-item">
          <div class="label-with-tool">
            <label class="form-label" for="cfg-model">模型名称 (Model ID) <span class="required-star">*</span></label>
            <button
              type="button"
              class="fetch-models-btn"
              :disabled="fetchingModels || !form.endpoint || form.protocol !== 'chat_completions'"
              :title="form.endpoint ? '从端点在线拉取所有可用模型列表' : '请先填写 Endpoint 地址'"
              @click="pullModels"
            >
              <PlayIcon name="refresh" :size="13" :class="{ 'spin-anim': fetchingModels }" />
              <span>{{ fetchingModels ? '正在拉取…' : '拉取模型' }}</span>
            </button>
          </div>

          <div class="model-input-group">
            <input
              id="cfg-model"
              v-model="form.model"
              class="text-field"
              list="fetched-models-options"
              placeholder="例如：deepseek-chat, gpt-4o, qwen2.5:7b"
              required
            />
            <datalist id="fetched-models-options">
              <option v-for="m in fetchedModels" :key="m" :value="m" />
            </datalist>

            <select
              v-if="fetchedModels.length"
              class="model-dropdown-select"
              aria-label="选择已拉取的模型"
              @change="onModelDropdownChange"
            >
              <option value="" disabled selected>选择已拉取模型 (共 {{ fetchedModels.length }} 个)</option>
              <option v-for="m in fetchedModels" :key="m" :value="m">{{ m }}</option>
            </select>
          </div>

          <div
            v-if="fetchModelStatus"
            class="fetch-status-bar"
            :class="{ 'is-ok': fetchModelOk, 'is-fail': !fetchModelOk }"
          >
            <span class="status-icon">{{ fetchModelOk ? '✓' : '✕' }}</span>
            <span>{{ fetchModelStatus }}</span>
          </div>

          <div v-if="fetchedModels.length" class="fetched-model-chips">
            <span class="chips-hint">点击选择：</span>
            <button
              v-for="m in fetchedModels.slice(0, 10)"
              :key="m"
              type="button"
              class="model-chip-tag"
              :class="{ 'is-chosen': form.model === m }"
              @click="form.model = m"
            >
              {{ m }}
            </button>
            <span v-if="fetchedModels.length > 10" class="more-models-note">
              + 更多可在下拉列表或输入框联想选择
            </span>
          </div>

          <small class="form-hint">支持手动输入或点击上方「拉取模型」在线获取服务商所有可用模型，选择后将直接显示在底栏人物胶囊中</small>
        </div>

        <div class="form-item">
          <div class="label-with-tool">
            <label class="form-label" for="cfg-key">API 密钥 (API Key)</label>
            <button type="button" class="text-toggle-btn" @click="showApiKey = !showApiKey">
              {{ showApiKey ? '隐藏密钥' : '显示密钥' }}
            </button>
          </div>
          <input
            id="cfg-key"
            v-model="form.apiKey"
            :type="showApiKey ? 'text' : 'password'"
            class="text-field"
            placeholder="sk-..."
          />
          <small class="form-hint">配置保存在此浏览器；设为当前生效后，会在每次行动时随请求加密（HTTPS）发送给本站服务器用于调用模型，服务器只在内存中使用、不落盘。本地 Ollama 等私有模型可留空。</small>
        </div>

        <div class="form-item">
          <label class="form-label prose-toggle">
            <input v-model="form.fullProse" type="checkbox" />
            小说式呈现（类似酒馆的段落正文）
          </label>
          <small class="form-hint">开启后，模型会把已确认的事实改写成连贯的小说段落（氛围、神态、动作与对白混合）；对白原文仍逐字锁定，校验不通过会自动回退标准叙述。关闭则保持「某某说：…」的标准台词格式。</small>
        </div>

        <!-- Advanced Toggle -->
        <div class="advanced-section">
          <button
            type="button"
            class="advanced-toggle-btn"
            @click="showAdvanced = !showAdvanced"
          >
            <span>高级参数（超时与推理预算）</span>
            <PlayIcon :name="showAdvanced ? 'back' : 'down'" :size="14" style="transform: rotate(-90deg);" />
          </button>

          <div v-if="showAdvanced" class="advanced-fields">
            <div class="form-item">
              <label class="form-label" for="cfg-timeout">超时时间 (秒)</label>
              <input
                id="cfg-timeout"
                v-model.number="form.timeoutSeconds"
                type="number"
                min="3"
                max="120"
                class="text-field"
              />
              <small class="form-hint">角色决策和输入解析最多各等 120 秒，包含重试；小说式呈现最多 90 秒。</small>
            </div>
            <div class="form-item">
              <label class="form-label" for="cfg-reasoning">推理强度</label>
              <select id="cfg-reasoning" v-model="form.reasoningEffort" class="text-field">
                <option value="">服务商默认</option>
                <option value="low">低（low）</option>
                <option value="medium">中（medium）</option>
                <option value="high">高（high）</option>
              </select>
            </div>
            <div class="form-item">
              <label class="form-label" for="cfg-decision-format">角色输出格式</label>
              <select id="cfg-decision-format" v-model="form.decisionFormat" class="text-field">
                <option value="">默认（JSON Schema）</option>
                <option value="json_schema">JSON Schema</option>
                <option value="json_object">兼容 JSON Mode</option>
                <option value="tool_call">原生函数提案（Tool Call）</option>
              </select>
              <small class="form-hint">选择服务商支持的格式。函数调用只返回角色提案，不执行工具；角色与世界校验仍会执行。</small>
            </div>
            <div class="form-item">
              <label class="form-label" for="cfg-decision-budget">角色决策输出预算</label>
              <select id="cfg-decision-budget" v-model="form.decisionMaxTokens" class="text-field">
                <option :value="0">默认（1024 tokens）</option>
                <option v-if="form.decisionMaxTokens && ![512, 1024, 2048, 4096, 8192].includes(form.decisionMaxTokens)" :value="form.decisionMaxTokens">{{ form.decisionMaxTokens }} tokens</option>
                <option v-for="budget in [512, 1024, 2048, 4096, 8192]" :key="budget" :value="budget">{{ budget }} tokens</option>
              </select>
            </div>
            <div class="form-item">
              <label class="form-label" for="cfg-interaction-budget">输入解析输出预算</label>
              <select id="cfg-interaction-budget" v-model="form.interactionMaxTokens" class="text-field">
                <option :value="0">默认（1600 tokens）</option>
                <option v-if="form.interactionMaxTokens && ![512, 1024, 1600, 2048, 4096].includes(form.interactionMaxTokens)" :value="form.interactionMaxTokens">{{ form.interactionMaxTokens }} tokens</option>
                <option v-for="budget in [512, 1024, 1600, 2048, 4096]" :key="budget" :value="budget">{{ budget }} tokens</option>
              </select>
            </div>
            <div class="form-item">
              <label class="form-label prose-toggle">
                <input v-model="form.disableThinking" type="checkbox" />
                请求关闭思考
              </label>
              <small class="form-hint">以上推理选项用于角色决策、输入解析和叙事呈现，需服务商支持；关闭思考不保证生效。角色决策与输入解析的输出预算可能包含思考用量，不代表对白长度。</small>
            </div>
          </div>
        </div>

        <!-- Connection Test Box -->
        <div class="test-connection-panel">
          <div class="test-btn-row">
            <button
              type="button"
              class="flat-button test-btn"
              :disabled="testing || !form.endpoint || form.protocol !== 'chat_completions'"
              @click="runConnectionTest"
            >
              <span v-if="testing">正在测试连通性…</span>
              <span v-else>测试连接</span>
            </button>
            <span class="test-tip">发起单次轻量验证，检验 Endpoint 与 Key 是否有效</span>
          </div>

          <div v-if="testResult" class="test-feedback-box" :class="{ 'is-ok': testResult.ok, 'is-fail': !testResult.ok }">
            <div class="feedback-head">
              <strong>{{ testResult.ok ? '✓ 连通正常' : '✕ 连接失败' }}</strong>
              <span v-if="testResult.ok" class="latency-tag">{{ testResult.latencyMs }}ms</span>
            </div>
            <p class="feedback-msg">{{ testResult.message }}</p>
          </div>
        </div>

        <div class="form-actions edit-footer-actions">
          <button type="button" class="flat-button" @click="cancelEdit">取消</button>
          <button type="button" class="flat-button" @click="saveAndStay">保存</button>
          <button type="submit" class="solid-button">保存并设为当前生效</button>
        </div>
      </form>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.model-settings-pane {
  position: relative;
  font-family: var(--ui);
}

.prose-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.prose-toggle input {
  accent-color: var(--accent);
  width: 16px;
  height: 16px;
}

.settings-view-wrapper {
  will-change: opacity, transform;
}

.pane-fade-enter-active {
  transition: opacity 220ms cubic-bezier(0.16, 1, 0.3, 1), transform 220ms cubic-bezier(0.16, 1, 0.3, 1);
}

.pane-fade-leave-active {
  transition: opacity 160ms cubic-bezier(0.25, 0.1, 0.25, 1), transform 160ms cubic-bezier(0.25, 0.1, 0.25, 1);
}

.pane-fade-enter-from {
  opacity: 0;
  transform: translateY(8px);
}

.pane-fade-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

.settings-toast {
  position: sticky;
  top: 0;
  z-index: 10;
  background: var(--accent);
  color: var(--on-primary);
  font-size: 12px;
  padding: 8px 14px;
  border-radius: 10px;
  margin-bottom: 12px;
  box-shadow: 0 4px 12px rgba(var(--ink-rgb), 0.12);
  text-align: center;
}

.custom-profile-row {
  cursor: pointer;
  transition: background-color var(--motion);
  flex-wrap: wrap;
  row-gap: 8px;
}

.custom-profile-row:active {
  background: var(--hover);
}

.custom-profile-row .row-copy {
  flex: 1 1 160px;
}

.custom-profile-row .row-copy strong {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 0;
}

.custom-profile-row .row-copy small {
  overflow-wrap: anywhere;
}

.model-badge {
  display: inline-block;
  max-width: 100%;
  font-size: 11px;
  padding: 1px 7px;
  border-radius: 9px;
  background: var(--soft);
  color: var(--secondary);
  margin-left: 6px;
  font-weight: 400;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  vertical-align: middle;
}

.profile-actions {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-left: auto;
  flex: none;
}

/* 窄屏（手机）下操作按钮独占一行右对齐，避免把名称挤成竖排 */
@media (max-width: 520px) {
  .custom-profile-row {
    padding-bottom: 10px;
  }

  .profile-actions {
    flex-basis: 100%;
    justify-content: flex-end;
    padding-top: 8px;
    border-top: 1px dashed var(--line);
  }
}

.active-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border-radius: 12px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 11px;
  font-weight: 500;
  border: none;
}

.icon-action-btn {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: var(--soft);
  color: var(--secondary);
  border: none;
  cursor: pointer;
  transition: background-color var(--motion), transform 150ms cubic-bezier(0.16, 1, 0.3, 1);
}

.icon-action-btn:hover {
  background: var(--hover);
  color: var(--ink);
}

.icon-action-btn:active {
  transform: scale(0.92);
}

.icon-action-btn.danger-action:hover {
  background: rgba(220, 38, 38, 0.12);
  color: var(--danger);
}

.preset-section {
  margin: 16px 0 12px;
}

.preset-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.preset-chip {
  font-size: 12px;
  padding: 6px 12px;
  border-radius: 16px;
  background: var(--surface);
  border: 1px solid var(--line);
  color: var(--secondary);
  cursor: pointer;
  transition: background-color var(--motion), border-color var(--motion), color var(--motion), transform 150ms cubic-bezier(0.16, 1, 0.3, 1);
}

.preset-chip:hover {
  background: var(--soft);
  color: var(--ink);
  border-color: var(--accent);
}

.preset-chip:active {
  transform: scale(0.96);
}

.list-management-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid var(--line);
}

.import-box {
  margin-top: 14px;
  padding: 14px;
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 14px;
}

.import-textarea {
  font-family: var(--font-mono, monospace);
  font-size: 12px;
  margin-top: 6px;
}

.import-error-msg {
  color: var(--danger);
  font-size: 12px;
  margin: 6px 0 0;
}

.edit-nav-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--line);
}

.back-to-list-btn {
  min-height: 32px;
  padding: 4px 10px;
  font-size: 12px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.edit-mode-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--ink);
}

.profile-form {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.form-item {
  display: flex;
  flex-direction: column;
}

.required-star {
  color: var(--danger);
}

.label-with-tool {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.text-toggle-btn {
  background: none;
  border: none;
  font-size: 11px;
  color: var(--accent);
  cursor: pointer;
  padding: 0 4px;
}

.form-hint {
  font-size: 11px;
  color: var(--muted);
  margin-top: 4px;
  line-height: 1.5;
}

.advanced-section {
  margin: 8px 0;
  border: 1px solid var(--line);
  border-radius: 12px;
  overflow: hidden;
}

.advanced-toggle-btn {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  background: var(--surface);
  border: none;
  font-size: 12px;
  color: var(--secondary);
  cursor: pointer;
}

.advanced-fields {
  padding: 12px 14px;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 10px;
  background: var(--sheet);
  border-top: 1px solid var(--line);
}

.test-connection-panel {
  margin: 12px 0;
  padding: 12px 14px;
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 14px;
}

.test-btn-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.test-btn {
  min-height: 34px;
  padding: 6px 14px;
  font-size: 12px;
}

.test-tip {
  font-size: 11px;
  color: var(--muted);
}

.test-feedback-box {
  margin-top: 10px;
  padding: 10px 12px;
  border-radius: 10px;
  font-size: 12px;
  animation: content-glide 200ms cubic-bezier(0.16, 1, 0.3, 1);
}

.test-feedback-box.is-ok {
  background: rgba(34, 197, 94, 0.1);
  border: 1px solid rgba(34, 197, 94, 0.3);
  color: #15803d;
}

.test-feedback-box.is-fail {
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid rgba(239, 68, 68, 0.3);
  color: var(--danger);
}

.feedback-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 500;
}

.latency-tag {
  font-size: 11px;
  font-family: var(--font-mono, monospace);
}

.feedback-msg {
  margin: 4px 0 0;
  font-size: 11px;
  line-height: 1.5;
  word-break: break-all;
}

.edit-footer-actions {
  margin-top: 16px;
  padding-top: 14px;
  border-top: 1px solid var(--line);
}

.fetch-models-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  background: var(--soft);
  border: 1px solid var(--line);
  border-radius: 8px;
  font-size: 11px;
  color: var(--accent);
  cursor: pointer;
  padding: 3px 8px;
  transition: background-color var(--motion), border-color var(--motion), transform 150ms cubic-bezier(0.16, 1, 0.3, 1);
}

.fetch-models-btn:hover:not(:disabled) {
  background: var(--hover);
  border-color: var(--accent);
}

.fetch-models-btn:active:not(:disabled) {
  transform: scale(0.95);
}

.fetch-models-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.model-input-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.model-dropdown-select {
  display: block;
  width: 100%;
  border: 1px solid var(--line);
  background: var(--surface);
  border-radius: 12px;
  font-size: 13px;
  padding: 8px 12px;
  color: var(--ink);
  outline: none;
  cursor: pointer;
}

.model-dropdown-select:focus {
  border-color: var(--accent);
}

.fetch-status-bar {
  margin-top: 6px;
  padding: 6px 10px;
  border-radius: 8px;
  font-size: 11px;
  display: flex;
  align-items: center;
  gap: 6px;
  animation: content-glide 200ms cubic-bezier(0.16, 1, 0.3, 1);
}

.fetch-status-bar.is-ok {
  background: rgba(34, 197, 94, 0.1);
  color: #15803d;
  border: 1px solid rgba(34, 197, 94, 0.3);
}

.fetch-status-bar.is-fail {
  background: rgba(239, 68, 68, 0.1);
  color: var(--danger);
  border: 1px solid rgba(239, 68, 68, 0.3);
}

.fetched-model-chips {
  margin-top: 8px;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.chips-hint {
  font-size: 11px;
  color: var(--muted);
}

.model-chip-tag {
  font-size: 11px;
  padding: 3px 8px;
  border-radius: 10px;
  background: var(--surface);
  border: 1px solid var(--line);
  color: var(--secondary);
  cursor: pointer;
  transition: background-color var(--motion), border-color var(--motion), color var(--motion), transform 150ms cubic-bezier(0.16, 1, 0.3, 1);
}

.model-chip-tag:hover {
  background: var(--soft);
  color: var(--ink);
  border-color: var(--accent);
}

.model-chip-tag:active {
  transform: scale(0.95);
}

.model-chip-tag.is-chosen {
  background: var(--accent-soft);
  color: var(--accent);
  border-color: var(--accent);
  font-weight: 500;
}

.more-models-note {
  font-size: 10px;
  color: var(--muted);
}

.spin-anim {
  animation: spin-turn 1s linear infinite;
}

@keyframes spin-turn {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>

export type ApiProtocol = 'chat_completions' | 'responses'

export interface ApiProfile {
  id: string
  name: string
  protocol: ApiProtocol
  endpoint: string
  apiKey: string
  model: string
  timeoutSeconds: number
  reasoningEffort?: '' | 'low' | 'medium' | 'high'
  disableThinking?: boolean
  decisionMaxTokens?: number
  interactionMaxTokens?: number
  decisionFormat?: '' | 'json_schema' | 'json_object' | 'tool_call'
  /** 模型组织叙事：选择已确认事实的节奏/措辞；保留 full_prose 请求语义。 */
  fullProse?: boolean
  temperature?: number
  maxTokens?: number
  createdAt: string
  updatedAt: string
}

/* Wire format for the per-request model override consumed by the backend
 * (core.RPModelOverride). Sent over HTTPS with each world request while the
 * profile is active; the server resolves it in memory and never persists it. */
export function profileModelOverride(profile: ApiProfile | null): Record<string, unknown> | undefined {
  if (!profile) return undefined
  if (profile.protocol !== 'chat_completions') throw new Error('当前 RP 仅支持 Chat Completions 协议。')
  validateDecisionOptions(profile)
  return {
    endpoint: resolveChatCompletionsEndpoint(profile.endpoint),
    model: profile.model.trim(),
    api_key: profile.apiKey,
    ...(profile.timeoutSeconds > 0 ? { timeout_seconds: Math.round(profile.timeoutSeconds) } : {}),
    ...(profile.fullProse ? { full_prose: true } : {}),
    ...(profile.reasoningEffort ? { reasoning_effort: profile.reasoningEffort } : {}),
    ...(profile.disableThinking ? { disable_thinking: true } : {}),
    ...(profile.decisionMaxTokens ? { decision_max_tokens: profile.decisionMaxTokens } : {}),
    ...(profile.interactionMaxTokens ? { interaction_max_tokens: profile.interactionMaxTokens } : {}),
    ...(profile.decisionFormat ? { decision_format: profile.decisionFormat } : {}),
  }
}

function validateDecisionOptions(profile: Partial<ApiProfile>): void {
  if (profile.decisionFormat !== undefined && !['', 'json_schema', 'json_object', 'tool_call'].includes(profile.decisionFormat)) {
    throw new Error('角色输出格式须为默认、JSON Schema、JSON Mode 或函数提案。')
  }
  if (profile.reasoningEffort !== undefined && !['', 'low', 'medium', 'high'].includes(profile.reasoningEffort)) {
    throw new Error('推理强度须为服务商默认、low、medium 或 high。')
  }
  if (profile.disableThinking !== undefined && typeof profile.disableThinking !== 'boolean') {
    throw new Error('关闭思考须为布尔值。')
  }
  for (const [value, max, label] of [
    [profile.decisionMaxTokens, 8192, '角色决策'],
    [profile.interactionMaxTokens, 4096, '输入解析'],
  ] as const) {
    if (value !== undefined && value !== 0 && (!Number.isInteger(value) || value < 512 || value > max)) {
      throw new Error(`${label}输出预算须为默认值或 512–${max} 的整数。`)
    }
  }
}

export interface PresetTemplate {
  name: string
  protocol: ApiProtocol
  endpoint: string
  model: string
  description: string
  temperature?: number
  maxTokens?: number
}

export const PRESET_TEMPLATES: PresetTemplate[] = [
  {
    name: 'DeepSeek V3',
    protocol: 'chat_completions',
    endpoint: 'https://api.deepseek.com/v1/chat/completions',
    model: 'deepseek-chat',
    description: 'DeepSeek 官方 API (OpenAI 兼容)',
    temperature: 0.7,
    maxTokens: 2048,
  },
  {
    name: 'OpenAI GPT-4o',
    protocol: 'chat_completions',
    endpoint: 'https://api.openai.com/v1/chat/completions',
    model: 'gpt-4o',
    description: 'OpenAI 官方旗舰多模态模型',
    temperature: 0.7,
    maxTokens: 2048,
  },
  {
    name: '硅基流动 (SiliconFlow)',
    protocol: 'chat_completions',
    endpoint: 'https://api.siliconflow.cn/v1/chat/completions',
    model: 'deepseek-ai/DeepSeek-V3',
    description: 'SiliconFlow 国内高性价比推理平台',
    temperature: 0.7,
    maxTokens: 2048,
  },
  {
    name: '本地 Ollama',
    protocol: 'chat_completions',
    endpoint: 'http://127.0.0.1:11434/v1/chat/completions',
    model: 'qwen2.5:7b',
    description: '本地私有化部署 (无需 API Key)',
    temperature: 0.7,
    maxTokens: 2048,
  },
  {
    name: 'StepFun (阶跃星辰)',
    protocol: 'chat_completions',
    endpoint: 'https://api.stepfun.com/v1/chat/completions',
    model: 'step-1-8k',
    description: '阶跃星辰多模态长上下文模型',
    temperature: 0.7,
    maxTokens: 2048,
  },
  {
    name: 'OpenRouter',
    protocol: 'chat_completions',
    endpoint: 'https://openrouter.ai/api/v1/chat/completions',
    model: 'deepseek/deepseek-chat',
    description: '全球多模型统一聚合路由',
    temperature: 0.7,
    maxTokens: 2048,
  },
]

export const PROFILES_STORAGE_KEY = 'corerp.api_profiles.v1'
export const ACTIVE_PROFILE_KEY = 'corerp.active_api_profile_id.v1'

export function loadProfiles(): ApiProfile[] {
  try {
    const raw = localStorage.getItem(PROFILES_STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter(item => item && typeof item === 'object' && typeof item.id === 'string' && typeof item.name === 'string')
  } catch {
    return []
  }
}

export function saveProfiles(profiles: ApiProfile[]): void {
  try {
    localStorage.setItem(PROFILES_STORAGE_KEY, JSON.stringify(profiles))
  } catch (err) {
    console.error('保存 API 配置失败:', err)
  }
}

export function getActiveProfileId(): string | null {
  try {
    const id = localStorage.getItem(ACTIVE_PROFILE_KEY)
    if (!id || id.trim() === '' || id === 'null') return null
    return id.trim()
  } catch {
    return null
  }
}

export function setActiveProfileId(id: string | null): void {
  try {
    if (!id) {
      localStorage.removeItem(ACTIVE_PROFILE_KEY)
    } else {
      localStorage.setItem(ACTIVE_PROFILE_KEY, id)
    }
  } catch (err) {
    console.error('设置激活 API 配置失败:', err)
  }
}

export function getActiveProfile(): ApiProfile | null {
  const activeId = getActiveProfileId()
  if (!activeId) return null
  const profiles = loadProfiles()
  // Imported legacy Responses profiles are visible for export/removal, but
  // cannot masquerade as a Chat Completions request in Play.
  return profiles.find(p => p.id === activeId && p.protocol === 'chat_completions') || null
}

export function upsertProfile(profile: ApiProfile): void {
  validateDecisionOptions(profile)
  const profiles = loadProfiles()
  const idx = profiles.findIndex(p => p.id === profile.id)
  const now = new Date().toISOString()
  if (idx >= 0) {
    profiles[idx] = { ...profile, updatedAt: now }
  } else {
    profiles.push({ ...profile, createdAt: profile.createdAt || now, updatedAt: now })
  }
  saveProfiles(profiles)
}

export function deleteProfile(id: string): void {
  const profiles = loadProfiles().filter(p => p.id !== id)
  saveProfiles(profiles)
  if (getActiveProfileId() === id) {
    setActiveProfileId(null)
  }
}

export function duplicateProfile(id: string): ApiProfile | null {
  const profiles = loadProfiles()
  const target = profiles.find(p => p.id === id)
  if (!target) return null
  const now = new Date().toISOString()
  const copy: ApiProfile = {
    ...target,
    id: `profile_${Date.now()}_${Math.random().toString(36).slice(2, 7)}`,
    name: `${target.name} (副本)`,
    createdAt: now,
    updatedAt: now,
  }
  profiles.push(copy)
  saveProfiles(profiles)
  return copy
}

export function createEmptyProfile(template?: Partial<ApiProfile>): ApiProfile {
  const now = new Date().toISOString()
  return {
    id: `profile_${Date.now()}_${Math.random().toString(36).slice(2, 7)}`,
    name: template?.name || '新模型配置',
    protocol: template?.protocol || 'chat_completions',
    endpoint: template?.endpoint || 'https://api.deepseek.com/v1/chat/completions',
    apiKey: template?.apiKey || '',
    model: template?.model || 'deepseek-chat',
    timeoutSeconds: template?.timeoutSeconds || 30,
    reasoningEffort: template?.reasoningEffort ?? '',
    disableThinking: template?.disableThinking ?? false,
    decisionMaxTokens: template?.decisionMaxTokens ?? 0,
    interactionMaxTokens: template?.interactionMaxTokens ?? 0,
    decisionFormat: template?.decisionFormat ?? '',
    temperature: template?.temperature ?? 0.7,
    maxTokens: template?.maxTokens ?? 2048,
    createdAt: now,
    updatedAt: now,
  }
}

export function resolveChatCompletionsEndpoint(endpoint: string): string {
  const url = endpoint.trim().replace(/\/+$/, '')
  if (url.endsWith('/chat/completions')) {
    return url
  }
  if (url.endsWith('/completions')) {
    return url.slice(0, -'/completions'.length) + '/chat/completions'
  }
  if (url.endsWith('/models')) {
    return url.slice(0, -'/models'.length) + '/chat/completions'
  }
  if (url.endsWith('/v1')) {
    return url + '/chat/completions'
  }
  if (/\/v1(\/.*)?$/.test(url)) {
    return url.replace(/\/v1(\/.*)?$/, '/v1/chat/completions')
  }
  return url + '/v1/chat/completions'
}

export function resolveModelsEndpoint(endpoint: string): string {
  const url = endpoint.trim().replace(/\/+$/, '')
  if (url.endsWith('/chat/completions')) {
    return url.slice(0, -'/chat/completions'.length) + '/models'
  }
  if (url.endsWith('/completions')) {
    return url.slice(0, -'/completions'.length) + '/models'
  }
  if (url.endsWith('/models')) {
    return url
  }
  if (url.endsWith('/v1')) {
    return url + '/models'
  }
  if (/\/v1(\/.*)?$/.test(url)) {
    return url.replace(/\/v1(\/.*)?$/, '/v1/models')
  }
  return url + '/v1/models'
}

export async function fetchAvailableModels(
  endpoint: string,
  apiKey: string,
  timeoutSeconds = 15,
  authToken?: string
): Promise<{ ok: boolean; models: string[]; message: string }> {
  const cleanEndpoint = endpoint.trim()
  if (!cleanEndpoint) {
    return { ok: false, models: [], message: '请先填写接口 Endpoint 地址' }
  }

  // 1. 优先使用服务端代理中继，彻底绕过浏览器的 CORS 跨域限制与 Mixed-Content 拦截
  try {
    const proxyRes = await fetch('/api/v1/proxy/models', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(authToken ? { Authorization: `Bearer ${authToken}` } : {}),
      },
      body: JSON.stringify({
        endpoint: cleanEndpoint,
        apiKey: apiKey ? apiKey.trim() : '',
        timeoutSeconds,
      }),
    })
    if (!proxyRes.ok) {
      return { ok: false, models: [], message: proxyRes.status === 401 ? '请先完成 Play 鉴权' : '服务端代理请求失败' }
    }
    const json = await proxyRes.json()
    if (json && json.data) return json.data
    return { ok: false, models: [], message: '服务端代理响应无效' }
  } catch {
    return { ok: false, models: [], message: '无法连接 CoreRP 服务端代理' }
  }


}

export async function testApiConnection(profile: ApiProfile, authToken?: string): Promise<{ ok: boolean; latencyMs: number; message: string }> {
  const rawEndpoint = profile.endpoint?.trim()
  if (!rawEndpoint) {
    return { ok: false, latencyMs: 0, message: '请填写接口 Endpoint 地址' }
  }

  const endpoint = resolveChatCompletionsEndpoint(rawEndpoint)

  const timeoutSec = Math.max(profile.timeoutSeconds || 15, 3)

  // Connection tests must pass through the authenticated server policy; never forward credentials from the browser directly.
  const start = performance.now()
  try {
    const proxyRes = await fetch('/api/v1/proxy/test', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(authToken ? { Authorization: `Bearer ${authToken}` } : {}),
      },
      body: JSON.stringify({
        endpoint,
        apiKey: profile.apiKey ? profile.apiKey.trim() : '',
        model: profile.model?.trim() || 'deepseek-chat',
        timeoutSeconds: timeoutSec,
      }),
    })
    if (!proxyRes.ok) {
      return { ok: false, latencyMs: Math.round(performance.now() - start), message: proxyRes.status === 401 ? '请先完成 Play 鉴权' : '服务端代理请求失败' }
    }
    const json = await proxyRes.json()
    if (json && json.data) return json.data
    return { ok: false, latencyMs: Math.round(performance.now() - start), message: '服务端代理响应无效' }
  } catch {
    return { ok: false, latencyMs: Math.round(performance.now() - start), message: '无法连接 CoreRP 服务端代理' }
  }


}

export function exportProfilesJson(): string {
  const profiles = loadProfiles()
  const activeId = getActiveProfileId()
  return JSON.stringify({ version: 1, exportedAt: new Date().toISOString(), activeProfileId: activeId, profiles }, null, 2)
}

export function importProfilesJson(jsonStr: string): { ok: boolean; count: number; error?: string } {
  try {
    const data = JSON.parse(jsonStr)
    const list = Array.isArray(data) ? data : data.profiles
    if (!Array.isArray(list)) return { ok: false, count: 0, error: '格式无效：缺少 profiles 数组' }

    let count = 0
    const existing = loadProfiles()
    for (const item of list) {
      if (item && typeof item === 'object' && item.name && item.endpoint) {
        const validItem: ApiProfile = {
          id: item.id || `profile_${Date.now()}_${Math.random().toString(36).slice(2, 7)}`,
          name: String(item.name).slice(0, 50),
          protocol: item.protocol === 'responses' ? 'responses' : 'chat_completions',
          endpoint: String(item.endpoint),
          apiKey: String(item.apiKey || ''),
          model: String(item.model || 'deepseek-chat'),
          timeoutSeconds: Number(item.timeoutSeconds) || 30,
          reasoningEffort: item.reasoningEffort,
          disableThinking: item.disableThinking,
          decisionMaxTokens: item.decisionMaxTokens,
          interactionMaxTokens: item.interactionMaxTokens,
          decisionFormat: item.decisionFormat,
          temperature: typeof item.temperature === 'number' ? item.temperature : 0.7,
          maxTokens: Number(item.maxTokens) || 2048,
          createdAt: item.createdAt || new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        }
        validateDecisionOptions(validItem)
        const idx = existing.findIndex(p => p.id === validItem.id)
        if (idx >= 0) existing[idx] = validItem
        else existing.push(validItem)
        count++
      }
    }
    saveProfiles(existing)
    return { ok: true, count }
  } catch (err: unknown) {
    const msg = (err as Error).message || 'JSON 解析失败'
    return { ok: false, count: 0, error: msg }
  }
}

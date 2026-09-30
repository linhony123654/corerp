import assert from 'node:assert/strict'

// Polyfill localStorage & performance for node environment test
const storage = new Map()
globalThis.localStorage = {
  getItem: (key) => storage.get(key) || null,
  setItem: (key, val) => storage.set(key, String(val)),
  removeItem: (key) => storage.delete(key),
  clear: () => storage.clear(),
}

const {
  loadProfiles,
  saveProfiles,
  getActiveProfileId,
  setActiveProfileId,
  getActiveProfile,
  profileModelOverride,
  upsertProfile,
  deleteProfile,
  duplicateProfile,
  createEmptyProfile,
  testApiConnection,
  exportProfilesJson,
  importProfilesJson,
  resolveModelsEndpoint,
  fetchAvailableModels,
  PRESET_TEMPLATES,
} = await import('../src/lib/apiProfiles.ts')

console.log('--- Testing API Profiles Core Logic ---')

// 1. Initial empty state
assert.deepEqual(loadProfiles(), [])
assert.equal(getActiveProfileId(), null)
assert.equal(getActiveProfile(), null)

// 2. Presets template test
assert.ok(PRESET_TEMPLATES.length >= 6)
const deepseek = PRESET_TEMPLATES.find(p => p.name === 'DeepSeek V3')
assert.ok(deepseek)
assert.equal(deepseek.model, 'deepseek-chat')
assert.equal(deepseek.endpoint, 'https://api.deepseek.com/v1/chat/completions')

// 3. Create empty profile from preset
const p1 = createEmptyProfile({
  name: deepseek.name,
  protocol: deepseek.protocol,
  endpoint: deepseek.endpoint,
  model: deepseek.model,
})
assert.ok(p1.id)
assert.equal(p1.name, 'DeepSeek V3')
assert.equal(p1.model, 'deepseek-chat')

// 4. Upsert and active selection
upsertProfile(p1)
assert.equal(loadProfiles().length, 1)
setActiveProfileId(p1.id)
assert.equal(getActiveProfileId(), p1.id)
assert.equal(getActiveProfile()?.id, p1.id)
assert.equal(getActiveProfile()?.model, 'deepseek-chat')
assert.deepEqual(Object.keys(profileModelOverride(getActiveProfile())).sort(), ['api_key', 'endpoint', 'model', 'timeout_seconds'])
const unsupported = { ...p1, id: 'legacy-responses', protocol: 'responses', endpoint: 'https://api.example.invalid/v1/responses' }
upsertProfile(unsupported)
setActiveProfileId(unsupported.id)
assert.equal(getActiveProfile(), null, 'unsupported Responses profile was silently treated as Chat Completions')
assert.throws(() => profileModelOverride(unsupported), /Chat Completions/)
deleteProfile(unsupported.id)
setActiveProfileId(p1.id)

// Optional RP tuning must survive saving/export/import and reach the wire.
const tuned = createEmptyProfile({
  ...p1, name: 'Step diagnostic', model: 'step-5-preview', timeoutSeconds: 120,
  reasoningEffort: 'low', disableThinking: true, decisionMaxTokens: 4096, interactionMaxTokens: 3072,
  decisionFormat: 'json_object',
})
upsertProfile(tuned)
const tunedWire = profileModelOverride(tuned)
assert.equal(tunedWire.reasoning_effort, 'low')
assert.equal(tunedWire.disable_thinking, true)
assert.equal(tunedWire.decision_max_tokens, 4096)
assert.equal(tunedWire.interaction_max_tokens, 3072)
assert.equal(tunedWire.decision_format, 'json_object')
assert.equal(tunedWire.timeout_seconds, 120)
assert.equal(importProfilesJson(exportProfilesJson()).ok, true)
assert.deepEqual(profileModelOverride(loadProfiles().find(p => p.id === tuned.id)), tunedWire)
deleteProfile(tuned.id)
const nativeTool = createEmptyProfile({ ...p1, decisionFormat: 'tool_call' })
upsertProfile(nativeTool)
assert.equal(profileModelOverride(nativeTool).decision_format, 'tool_call')
assert.equal(importProfilesJson(exportProfilesJson()).ok, true)
assert.equal(profileModelOverride(loadProfiles().find(p => p.id === nativeTool.id)).decision_format, 'tool_call')
deleteProfile(nativeTool.id)
for (const invalid of [
  { reasoningEffort: 'unknown' }, { disableThinking: 'false' },
  { decisionMaxTokens: 511 }, { decisionMaxTokens: 8193 },
  { interactionMaxTokens: 4097 }, { decisionMaxTokens: 1024.5 },
  { decisionFormat: 'text' }, { decisionFormat: 'auto' },
]) {
  assert.throws(() => profileModelOverride({ ...p1, ...invalid }))
  const before = JSON.stringify(loadProfiles())
  assert.equal(importProfilesJson(JSON.stringify([{ ...p1, ...invalid }])).ok, false)
  assert.equal(JSON.stringify(loadProfiles()), before, 'invalid tuning changed saved profiles')
}

// 5. Duplicate profile
const p1Copy = duplicateProfile(p1.id)
assert.ok(p1Copy)
assert.notEqual(p1Copy.id, p1.id)
assert.equal(p1Copy.name, 'DeepSeek V3 (副本)')
assert.equal(loadProfiles().length, 2)

// 6. Add second profile (e.g. OpenAI)
const gpt = PRESET_TEMPLATES.find(p => p.name.includes('GPT-4o'))
assert.ok(gpt)
const p2 = createEmptyProfile({
  name: gpt.name,
  protocol: gpt.protocol,
  endpoint: gpt.endpoint,
  model: gpt.model,
})
upsertProfile(p2)
assert.equal(loadProfiles().length, 3)

// Switch active to p2
setActiveProfileId(p2.id)
assert.equal(getActiveProfile()?.model, 'gpt-4o')

// 7. Delete active profile -> should fallback to null
deleteProfile(p2.id)
assert.equal(loadProfiles().length, 2)
assert.equal(getActiveProfileId(), null)
assert.equal(getActiveProfile(), null)

// 8. Export and Import
const exported = exportProfilesJson()
assert.ok(typeof exported === 'string')
assert.ok(exported.includes('DeepSeek V3'))

localStorage.clear()
assert.equal(loadProfiles().length, 0)
const imported = importProfilesJson(exported)
assert.equal(imported.ok, true)
assert.equal(imported.count, 2)
assert.equal(loadProfiles().length, 2)

// 9. Connection test with empty endpoint
const emptyRes = await testApiConnection({
  id: 'test',
  name: 'Empty',
  protocol: 'chat_completions',
  endpoint: '',
  apiKey: '',
  model: 'test',
  timeoutSeconds: 5,
  createdAt: '',
  updatedAt: '',
})
assert.equal(emptyRes.ok, false)
assert.ok(emptyRes.message.includes('Endpoint'))

// 10. resolveModelsEndpoint
assert.equal(resolveModelsEndpoint('https://api.deepseek.com/v1/chat/completions'), 'https://api.deepseek.com/v1/models')
assert.equal(resolveModelsEndpoint('https://api.openai.com/v1/chat/completions'), 'https://api.openai.com/v1/models')
assert.equal(resolveModelsEndpoint('http://127.0.0.1:11434/v1/chat/completions'), 'http://127.0.0.1:11434/v1/models')
assert.equal(resolveModelsEndpoint('https://api.siliconflow.cn/v1'), 'https://api.siliconflow.cn/v1/models')
assert.equal(resolveModelsEndpoint('https://api.example.com/v1/models'), 'https://api.example.com/v1/models')

// 11. fetchAvailableModels with empty endpoint
const emptyFetch = await fetchAvailableModels('', '')
assert.equal(emptyFetch.ok, false)
assert.ok(emptyFetch.message.includes('Endpoint'))

// Authenticated endpoint checks must go through the local CoreRP proxy only.
const originalFetch = globalThis.fetch
const proxyCalls = []
globalThis.fetch = async (url, options) => {
  proxyCalls.push({ url: String(url), options })
  const data = String(url).endsWith('/proxy/test')
    ? { ok: true, latencyMs: 1, message: 'fixture success' }
    : { ok: true, models: ['fixture-model'], message: 'fixture success' }
  return new Response(JSON.stringify({ data }), { status: 200, headers: { 'Content-Type': 'application/json' } })
}
try {
  const fixtureProfile = { ...p1, endpoint: 'https://fixture.invalid/v1/chat/completions', apiKey: 'fixture-provider-key' }
  assert.equal((await testApiConnection(fixtureProfile, 'play-session-token')).ok, true)
  assert.equal((await fetchAvailableModels(fixtureProfile.endpoint, fixtureProfile.apiKey, 5, 'play-session-token')).ok, true)
  assert.equal(proxyCalls.length, 2)
  assert.deepEqual(proxyCalls.map(call => call.url), ['/api/v1/proxy/test', '/api/v1/proxy/models'])
  for (const call of proxyCalls) {
    assert.equal(call.options.headers.Authorization, 'Bearer play-session-token')
    assert.equal(JSON.parse(call.options.body).apiKey, 'fixture-provider-key')
  }
} finally {
  globalThis.fetch = originalFetch
}

console.log('✓ All API profile tests passed!')

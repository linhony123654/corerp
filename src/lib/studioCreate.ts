export type StudioDraft = {
  name: string; population: number; money: number; stock: number
  home: string; square: string; playerName: string; neighbourName: string
  playerPersona: string; neighbourPersona: string; publicPresentation: string
  relationshipMode: 'unknown' | 'authored'
  neighbourRole: string; playerRole: string
  neighbourAddress: string; playerAddress: string
  neighbourSelfReference: string; playerSelfReference: string
  authorityInstance: string; authorityBranch: string; playerPrincipal: string
  budget: number; narration: 'plain' | 'dialogue' | 'detailed'
}
export type CreateRequest = {
  authority_instance_id: string; authority_branch_id: string; instance_id: string
  idempotency_key: string; player_principal_id: string
  spec: Record<string, unknown>; system_package: unknown; narrative_package: unknown
}
export type CreateReceipt = {
  instance_id: string; branch_id: string; entity_id: string; player_principal_id: string
  ready_event_id: string; event_sequence: number; status: 'ready'; replayed: boolean
  rp_readiness?: {
    status: 'READY' | 'INCOMPLETE'
    characters: { key: string; name: string; readiness: { persona: 'READY' | 'MISSING'; relationship_to_interlocutor: 'READY' | 'UNKNOWN'; address_to_interlocutor: 'READY' | 'MISSING' | 'UNKNOWN' }; public_presentation: 'READY' | 'MISSING' }[]
    incomplete_relationships: { from: string; to: string }[]
  }
}
export function freshStudioDraft(): StudioDraft {
  return { name: '一座小小的世界', population: 2, money: 20, stock: 2, home: '家', square: '街角广场', playerName: '林', neighbourName: '蔡', playerPersona: '', neighbourPersona: '', publicPresentation: '', relationshipMode: 'unknown', neighbourRole: '', playerRole: '', neighbourAddress: '', playerAddress: '', neighbourSelfReference: '', playerSelfReference: '', authorityInstance: '', authorityBranch: 'br_main', playerPrincipal: '', budget: 4, narration: 'plain' }
}
// Matches the server's restricted canonical JSON: UTF-16 key ordering and safe
// integers, not a general floating-point JSON canonicalization implementation.
function canonical(value: unknown): string {
  if (value === null || typeof value === 'boolean') return JSON.stringify(value)
  if (typeof value === 'string') {
    for (const point of value) { const code = point.codePointAt(0)!; if (code >= 0xd800 && code <= 0xdfff) throw new Error('配置包含无效的 Unicode 字符。') }
    return JSON.stringify(value)
  }
  if (typeof value === 'number' && Number.isSafeInteger(value)) return JSON.stringify(value)
  if (Array.isArray(value)) return `[${value.map(canonical).join(',')}]`
  if (value && typeof value === 'object' && Object.getPrototypeOf(value) === Object.prototype) {
    return `{${Object.keys(value).sort().map(key => `${canonical(key)}:${canonical((value as Record<string, unknown>)[key])}`).join(',')}}`
  }
  throw new Error('配置只能包含安全整数、文本和声明式 JSON。')
}
async function packageBundle(kind: 'system' | 'narrative', draft: StudioDraft) {
  const content = { version: 'corerp.studio-package.v1', ...(kind === 'system'
    ? { system_rules: { npc_daily_action_budget: draft.budget, rp_execution_mode: 'orchestrated', max_active_responders: 1 } }
    : { narrative_style: { version: 'corerp.style.v1', pov: 'second_person', tense: 'present', verbosity: draft.narration === 'dialogue' ? 'terse' : draft.narration === 'detailed' ? 'detailed' : 'normal', dialogue_ratio: draft.narration === 'detailed' ? 60 : 100, description_density: draft.narration === 'detailed' ? 70 : 0, inner_monologue_policy: 'none', prose_instructions: '', forbidden_patterns: [], narrative_pack_ref: draft.narration === 'dialogue' ? 'builtin/dialogue@1' : 'builtin/plain@1' } }) }
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(canonical(content)))
  const hash = Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('')
  return { manifest: { schema_version: 'm0-draft-2026-09-22', id: `studio.${kind}`, kind, version: kind === 'system' ? '1.1.0' : '1.0.0', engine_api: 'm0-draft-2026-09-22', requires: [], optional: [], capabilities: [kind === 'system' ? 'rules.npc.daily_budget' : 'narrative.style'], schema_hash: 'sha256:0b7d979a3384df06726118c293ec3533e501cd0740f0819067e8e6191c2667a0', content_hash: `sha256:${hash}`, content_files: [`${kind}.json`] }, content }
}
function importedBundle(raw: string): unknown {
  if (new TextEncoder().encode(raw).length > 65536) throw new Error('单个包的 JSON 不能超过 64 KiB。')
  try { const value: unknown = JSON.parse(raw); canonical(value); return value } catch { throw new Error('包内容必须是有效的声明式 JSON，且数值必须是安全整数。') }
}
export async function buildCreateRequest(draft: StudioDraft, systemJSON: string, narrativeJSON: string): Promise<CreateRequest> {
  const values = [draft.population, draft.money, draft.stock, draft.budget]
  if (values.some(value => !Number.isSafeInteger(value)) || draft.population < 2 || draft.population > 1000000 || draft.money < 0 || draft.stock < 0 || draft.budget < 1 || draft.budget > 64) throw new Error('请检查人口、资源和人物行动预算的整数范围。')
  for (const name of [draft.name, draft.home, draft.square, draft.playerName, draft.neighbourName]) {
    if (!name.trim() || name !== name.trim() || [...name].length > 100 || /[\u0000\r\n]/.test(name)) throw new Error('名称须为 1–100 个字符，且不能有首尾空格或换行。')
  }
  if (![draft.authorityInstance, draft.authorityBranch, draft.playerPrincipal].every(value => value.trim())) throw new Error('请填写管理员提供的创建授权范围和玩家身份。')
  const playerPersona = draft.playerPersona.trim(), persona = draft.neighbourPersona.trim(), presentation = draft.publicPresentation.trim()
  if (!persona) throw new Error('请先填写 NPC 人设，再保存角色扮演世界。')
  for (const text of [playerPersona, persona, presentation]) {
    if ([...text].length > 500 || /[\u0000\r]/.test(text)) throw new Error('角色人设与公开说话风格最多 500 个字符，不能包含无效控制字符。')
    canonical(text)
  }
  if (draft.relationshipMode !== 'unknown' && draft.relationshipMode !== 'authored') throw new Error('请选择是否声明双方关系。')
  const relationships: Record<string, unknown>[] = []
  if (draft.relationshipMode === 'authored') {
    for (const direction of [
      { from: 'neighbour', to: 'player', role: draft.neighbourRole, address: draft.neighbourAddress, self: draft.neighbourSelfReference },
      { from: 'player', to: 'neighbour', role: draft.playerRole, address: draft.playerAddress, self: draft.playerSelfReference }
    ]) {
      const role = direction.role.trim(), address = direction.address.trim(), self = direction.self.trim()
      if (!role || !address) throw new Error('已声明双方关系时，请分别填写双方的关系身份与称呼。')
      if ([...role].length > 60 || [...address].length > 40 || [...self].length > 40 || /[\u0000\r\n]/.test(role + address + self)) throw new Error('关系身份最多 60 个字符，称呼与自称最多 40 个字符，均须为单行。')
      canonical(role); canonical(address); canonical(self)
      relationships.push({ from: direction.from, to: direction.to, role, address_to: [address], ...(self ? { self_reference: self } : {}) })
    }
  }
  return {
    authority_instance_id: draft.authorityInstance.trim(), authority_branch_id: draft.authorityBranch.trim(), instance_id: `world_${crypto.randomUUID()}`, idempotency_key: crypto.randomUUID(), player_principal_id: draft.playerPrincipal.trim(),
    system_package: systemJSON.trim() ? importedBundle(systemJSON) : await packageBundle('system', draft),
    narrative_package: narrativeJSON.trim() ? importedBundle(narrativeJSON) : await packageBundle('narrative', draft),
    spec: { version: 'corerp.studio-world.v1', name: draft.name, start_world_time: '2026-09-22T00:00:00Z', population: draft.population, opening_money_minor: draft.money, opening_stock_minor: draft.stock,
      places: [{ key: 'home', name: draft.home, kind: 'home' }, { key: 'square', name: draft.square, kind: 'public' }], links: [{ from: 'home', to: 'square', minutes: 1 }],
      people: [{ key: 'player', name: draft.playerName, place: 'home', player: true, ...(playerPersona ? { persona: playerPersona } : {}) }, { key: 'neighbour', name: draft.neighbourName, place: 'home', player: false, persona, ...(presentation ? { public_presentation: presentation } : {}) }],
      ...(relationships.length ? { acquaintances: [['player', 'neighbour']], relationships } : {}) }
  }
}

export type StyleScope = { session_id: string; instance_id: string; branch_id: string }
export type StyleProfile = {
	context_budget_bytes?: number
  narrative_density?: '' | 'concise' | 'standard' | 'long'
  full_prose?: boolean
  pov: string; tense: string; verbosity: string; dialogue_ratio: number; description_density: number
  inner_monologue_policy: string; prose_instructions: string; forbidden_patterns: string[]; narrative_pack_ref: string
}
export type StyleRead = { profile: StyleProfile; session_revision: number }
export type StyleWrite = StyleScope & { scope: 'session'; expected_revision: number; idempotency_key: string; patch: StyleProfile }
export const pendingStyleKey = (session: string) => `corerp.style.pending.${session}`
export const stylePresets = {
  plain: { verbosity: 'normal', dialogue_ratio: 100, description_density: 0, narrative_pack_ref: 'builtin/plain@1' },
  dialogue: { verbosity: 'terse', dialogue_ratio: 100, description_density: 0, narrative_pack_ref: 'builtin/dialogue@1' },
  detailed: { verbosity: 'detailed', dialogue_ratio: 60, description_density: 70, narrative_pack_ref: 'builtin/plain@1' }
} satisfies Record<string, Partial<StyleProfile>>

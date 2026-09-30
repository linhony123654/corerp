import assert from 'node:assert/strict'
import { failedDecision, incompleteRPContext, providerCallSummary } from '../src/lib/providerReceipt.ts'

const incomplete = { phase: 'decision', provider_kind: 'chat_completions', model_id: 'fixture', attempted: false, attempt_count: 0, result: 'not_used', fallback_kind: 'rp_context_not_ready', started_at_utc: '2026-09-30T00:00:00Z' }
assert.equal(incompleteRPContext([incomplete]), true)
assert.equal(failedDecision([incomplete]), false)
assert.match(providerCallSummary(incomplete), /角色设定尚未就绪，未调用模型/)
assert.match(providerCallSummary(incomplete), /世界创建者/)
assert.doesNotMatch(providerCallSummary(incomplete), /故障|超时|再次表达|秘钥|persona/)

const failed = { ...incomplete, attempted: true, attempt_count: 2, result: 'failed', fallback_kind: 'silence' }
assert.equal(incompleteRPContext([failed]), false)
assert.equal(failedDecision([failed]), true)
assert.match(providerCallSummary(failed), /NPC 因技术故障保持沉默/)

const unused = { ...incomplete, fallback_kind: 'no_eligible_listener' }
assert.equal(incompleteRPContext([unused]), false)
assert.equal(failedDecision([unused]), false)
assert.match(providerCallSummary(unused), /本次无需调用/)
assert.equal(incompleteRPContext([{ ...incomplete, phase: 'narrative' }]), false)
assert.equal(incompleteRPContext(undefined), false)
console.log('RP not-ready/technical-failure/unused receipt distinctions PASS')

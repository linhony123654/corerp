import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { buildCreateRequest, freshStudioDraft } from '../src/lib/studioCreate.ts';

const draft = () => ({ ...freshStudioDraft(), authorityInstance: 'inst_test', playerPrincipal: 'principal_test' });
await assert.rejects(buildCreateRequest(draft(), '', ''), /NPC 人设/);
await assert.rejects(buildCreateRequest({ ...draft(), neighbourPersona: ' \n\t' }, '', ''), /NPC 人设/);

const base = { ...draft(), neighbourName: '贾母', playerName: '宝玉', neighbourPersona: '  明确填写的测试人设  ', playerPersona: '私有玩家背景标记' };
const unknown = await buildCreateRequest(Object.freeze({ ...base, neighbourRole: '祖母', playerRole: '孙儿', neighbourAddress: '宝玉', playerAddress: '老祖宗' }), '', '');
assert.equal(unknown.spec.people[1].persona, '明确填写的测试人设');
assert.equal(unknown.spec.people[0].persona, base.playerPersona);
assert.equal(unknown.spec.people[1].public_presentation, undefined, 'private persona must not become narrator style');
assert.equal(unknown.spec.acquaintances, undefined, 'names or hidden stale form data must not declare familiarity');
assert.equal(unknown.spec.relationships, undefined, 'unknown must not become a novel relationship');
assert.equal(base.neighbourPersona, '  明确填写的测试人设  ', 'validation must not mutate the draft');
assert.deepEqual(unknown.system_package.content.system_rules, { npc_daily_action_budget: base.budget, rp_execution_mode: 'orchestrated', max_active_responders: 1 }, 'new worlds must use the bounded conversation policy');
assert.equal(unknown.system_package.manifest.version, '1.1.0');
assert.equal(unknown.narrative_package.manifest.version, '1.0.0');
const canonical = value => Array.isArray(value) ? `[${value.map(canonical).join(',')}]` : value && typeof value === 'object' ? `{${Object.keys(value).sort().map(key => `${JSON.stringify(key)}:${canonical(value[key])}`).join(',')}}` : JSON.stringify(value);
const hash = content => 'sha256:' + createHash('sha256').update(canonical(content)).digest('hex');
assert.equal(unknown.system_package.manifest.content_hash, hash(unknown.system_package.content), 'conversation policy must be included in the locked package hash');
const customSystem = structuredClone(unknown.system_package);
delete customSystem.content.system_rules.rp_execution_mode;
delete customSystem.content.system_rules.max_active_responders;
customSystem.manifest.version = '1.0.0';
customSystem.manifest.content_hash = hash(customSystem.content);
const imported = await buildCreateRequest(base, JSON.stringify(customSystem), '');
assert.deepEqual(imported.system_package, customSystem, 'an imported authored package must preserve its declared legacy behavior');

const authored = { ...base, relationshipMode: 'authored', neighbourRole: '测试长辈', playerRole: '测试晚辈', neighbourAddress: '小林', playerAddress: 'Nora', neighbourSelfReference: '我', playerSelfReference: '', publicPresentation: '语气亲切' };
const request = await buildCreateRequest(authored, '', '');
assert.deepEqual(request.spec.acquaintances, [['player', 'neighbour']]);
assert.deepEqual(request.spec.relationships, [
  { from: 'neighbour', to: 'player', role: '测试长辈', address_to: ['小林'], self_reference: '我' },
  { from: 'player', to: 'neighbour', role: '测试晚辈', address_to: ['Nora'] },
]);
assert.equal(request.spec.people[1].public_presentation, '语气亲切');
await assert.rejects(buildCreateRequest({ ...authored, playerRole: '' }, '', ''), /分别填写/);
await assert.rejects(buildCreateRequest({ ...authored, neighbourAddress: ' ' }, '', ''), /分别填写/);
for (const invalid of [{ neighbourPersona: '中'.repeat(501) }, { publicPresentation: '中'.repeat(501) }, { playerPersona: '坏\0字符' }, { neighbourAddress: '中'.repeat(41) }, { playerRole: '中'.repeat(61) }, { playerSelfReference: '换\n行' }]) {
  await assert.rejects(buildCreateRequest({ ...authored, ...invalid }, '', ''));
}
console.log(JSON.stringify({ status: 'PASS', checks: ['required NPC persona', 'unknown not inferred from names/stale form', 'independent directional relationships', 'private/public separation', 'bounded authored fields', 'immutable draft', 'default bounded conversation policy with signed content', 'explicit imported package preserved'] }));

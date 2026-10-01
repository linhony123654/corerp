import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { buildRongqingCreateRequest, freshStudioDraft } from '../src/lib/studioCreate.ts';
import { readNarrativeStream, validateNarrativeComposition } from '../src/lib/narrativeStream.ts';

const source = readFileSync(new URL('../docs/rp-runtime-r1/rongqing-product-world-spec-2026-10-01.json', import.meta.url));
assert.equal(createHash('sha256').update(source).digest('hex'), '31b6ce345006c380e34cf1d4e8384283e45c5aaa8f34e51b9d06dfce8dc44cf7');
const draft = { ...freshStudioDraft(), authorityInstance: 'fixture-authority', playerPrincipal: 'fixture-player' };
const request = await buildRongqingCreateRequest(draft, '', '');
assert.deepEqual(request.spec, JSON.parse(source));
assert.equal(request.authority_instance_id, draft.authorityInstance);
assert.equal(request.player_principal_id, draft.playerPrincipal);
assert.equal(draft.neighbourPersona, '', 'preset must not overwrite the custom draft');
const imported = await buildRongqingCreateRequest(draft, JSON.stringify(request.system_package), JSON.stringify(request.narrative_package));
assert.deepEqual(imported.system_package, request.system_package);
assert.deepEqual(imported.narrative_package, request.narrative_package);
request.spec.people[1].persona = 'fixture local mutation';
assert.deepEqual((await buildRongqingCreateRequest(draft, '', '')).spec, JSON.parse(source), 'preset declarations must not share mutable requests');
await assert.rejects(buildRongqingCreateRequest({ ...draft, playerPrincipal: '' }, '', ''), /授权范围和玩家身份/);
await assert.rejects(buildRongqingCreateRequest({ ...draft, budget: 0 }, '', ''), /行动预算/);

const version = 'corerp.fact-composition.v2';
const view = { lines: ['第一段原话。', '第二段原话。'], event_ids: ['fact-1', 'fact-2', 'fact-3'], warnings: [], composition_version: version, fact_groups: [['fact-1', 'fact-2'], ['fact-3']] };
const response = frames => new Response(frames.map(frame => JSON.stringify(frame) + '\n').join(''), { headers: { 'Content-Type': 'application/x-ndjson' } });
const frames = value => [
  ...value.lines.map((line, index) => ({ type: 'line', chunk: { index, line, event_ids: value.fact_groups[index] } })),
  { type: 'done', count: value.lines.length, ...value, lines: undefined },
];
for (const composition_version of ['corerp.fact-composition.v1', version]) {
  const expected = { ...view, composition_version };
  validateNarrativeComposition(expected);
  assert.deepEqual((await readNarrativeStream(response(frames(expected)), () => {})).view, expected);
}
const legacy = [
  { type: 'line', chunk: { index: 0, line: '旧第一段。', event_ids: ['fact-1'] } },
  { type: 'line', chunk: { index: 1, line: '旧第二段。', event_ids: ['fact-1'] } },
  { type: 'done', count: 2, event_ids: ['fact-1'], warnings: [] },
];
assert.deepEqual((await readNarrativeStream(response(legacy), () => {})).view, { lines: ['旧第一段。', '旧第二段。'], event_ids: ['fact-1'], warnings: [] });
validateNarrativeComposition({ lines: [] }); // Old pending/history format.
validateNarrativeComposition({ ...view, composition_version: 'corerp.fact-composition.v1', event_ids: undefined }); // Earlier saved v1 history.
const emptyV2 = { lines: [], event_ids: [], warnings: [], composition_version: version, fact_groups: [] };
validateNarrativeComposition(emptyV2);
assert.deepEqual((await readNarrativeStream(response(frames(emptyV2)), () => assert.fail('empty view must not emit preview'))).view, emptyV2);
for (const bad of [
  { ...emptyV2, composition_version: 'corerp.fact-composition.v1' },
  { ...emptyV2, composition_version: undefined },
  { ...emptyV2, event_ids: ['fact-1'] },
  { ...emptyV2, event_ids: undefined },
  { ...emptyV2, fact_groups: [['fact-1']] },
  { ...emptyV2, fact_groups: undefined },
  { ...emptyV2, lines: ['uncovered line'] },
]) assert.throws(() => validateNarrativeComposition(bad));
for (const bad of [
  { ...emptyV2, composition_version: 'corerp.fact-composition.v1' },
  { ...emptyV2, composition_version: undefined, fact_groups: undefined },
  { ...emptyV2, event_ids: ['fact-1'] },
  { ...emptyV2, fact_groups: [[]] },
]) await assert.rejects(readNarrativeStream(response([{ type: 'done', count: 0, ...bad, lines: undefined }]), () => {}));
for (const bad of [
  { ...view, composition_version: 'corerp.fact-composition.v99' },
  { ...view, composition_version: undefined },
  { ...view, fact_groups: undefined },
  { ...view, fact_groups: [['fact-1', 'fact-2'], ['fact-2']] },
  { ...view, fact_groups: [['fact-1'], ['fact-3']] },
  { ...view, fact_groups: [['fact-2', 'fact-1'], ['fact-3']] },
  { ...view, fact_groups: [['fact-1', 'fact-2', 'fact-3']] },
  { ...view, fact_groups: [[], ['fact-3']] },
  { ...view, event_ids: undefined },
  { ...view, fact_groups: null },
]) assert.throws(() => validateNarrativeComposition(bad));
const wrongGroups = frames(view);
wrongGroups[2].fact_groups = [['fact-1'], ['fact-2', 'fact-3']];
await assert.rejects(readNarrativeStream(response(wrongGroups), () => {}), /分组/);
const duplicateChunks = frames(view);
duplicateChunks[1].chunk.event_ids = ['fact-2', 'fact-3'];
await assert.rejects(readNarrativeStream(response(duplicateChunks), () => {}), /分组/);
const unknown = frames(view);
unknown[2].composition_version = 'corerp.fact-composition.v99';
await assert.rejects(readNarrativeStream(response(unknown), () => {}), /版本/);
console.log(JSON.stringify({ status: 'PASS', checks: ['exact authored preset/source hash', 'shared authority/package/request path', 'custom draft preservation', 'clone isolation', 'v1/v2 metadata retained', 'saved ordered exact-once coverage', 'stream group agreement', 'unknown/missing/duplicate/reordered sources rejected', 'legacy repeated-source prose preserved'] }));

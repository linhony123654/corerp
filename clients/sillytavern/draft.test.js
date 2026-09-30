import assert from 'node:assert/strict';
import test from 'node:test';
import { buildStudioImportDraft, presentationMapping, reconnectDelay, samePresentationMapping } from './draft.js';

test('group mapping controls one runtime player and never clones cards', () => {
  const host = { groupId: 7, groups: [{ id: 7, members: ['b.png', 'a.png'] }], characters: [] };
  assert.deepEqual(presentationMapping(host), {
    kind: 'group', group_id: '7', member_avatars: ['a.png', 'b.png'], runtime_control: 'one_player_session', cards_are_presentation_only: true,
  });
  assert.equal(samePresentationMapping(presentationMapping(host), presentationMapping(host)), true);
});

test('card and world info produce a non-authoritative Studio draft only', () => {
  const host = { groupId: null, characterId: 0, characters: [{ avatar: 'c.png', name: 'Cai', description: '管家', personality: '谨慎', first_mes: '你好' }] };
  const draft = buildStudioImportDraft({ host, worldInfoJSON: JSON.stringify({ entries: { one: { comment: '宅邸', content: '一座旧宅。' } } }), createdAt: '2026-09-27T00:00:00Z' });
  assert.equal(draft.status, 'draft');
  assert.equal(draft.references.character_cards[0].name, 'Cai');
  assert.equal(draft.references.world_info[0].title, '宅邸');
  assert.equal(draft.authority_boundary.creates_runtime_entities, false);
  assert.equal(draft.authority_boundary.activates_packages, false);
  assert.equal(draft.authority_boundary.imported_text_is_canonical_truth, false);
});

test('automatic reconnect backoff is bounded', () => {
  assert.deepEqual([0, 1, 2, 3, 4, 5, 10].map(reconnectDelay), [1000, 2000, 4000, 8000, 16000, 16000, 16000]);
});

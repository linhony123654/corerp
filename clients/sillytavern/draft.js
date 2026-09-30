const text = value => typeof value === 'string' ? value.trim() : '';

export function presentationMapping(host) {
  if (host.groupId == null) {
    return {
      kind: 'single',
      character_avatar: text(host.characters?.[host.characterId]?.avatar),
      runtime_control: 'one_player_session',
      cards_are_presentation_only: true,
    };
  }
  const group = host.groups?.find?.(item => String(item?.id) === String(host.groupId));
  const members = Array.isArray(group?.members) ? group.members.map(String).sort() : [];
  return {
    kind: 'group',
    group_id: String(host.groupId),
    member_avatars: members,
    runtime_control: 'one_player_session',
    cards_are_presentation_only: true,
  };
}

export function samePresentationMapping(left, right) {
  return JSON.stringify(left ?? null) === JSON.stringify(right ?? null);
}

function cardReference(card) {
  const source = card?.data ?? card ?? {};
  const persona = [source.description, source.personality, source.scenario]
    .map(text).filter(Boolean).join('\n\n').slice(0, 500);
  return {
    name: text(source.name) || '未命名角色',
    persona_excerpt: persona,
    greeting_reference: text(source.first_mes).slice(0, 1000),
    example_reference: text(source.mes_example).slice(0, 2000),
  };
}

function worldInfoReferences(raw) {
  if (!text(raw)) return [];
  const parsed = JSON.parse(raw);
  const entries = Array.isArray(parsed) ? parsed : Array.isArray(parsed.entries) ? parsed.entries : Object.values(parsed.entries ?? {});
  return entries.slice(0, 200).map(entry => ({
    title: text(entry?.comment ?? entry?.name ?? entry?.key).slice(0, 120),
    reference_text: text(entry?.content ?? entry?.text).slice(0, 4000),
  })).filter(entry => entry.title || entry.reference_text);
}

export function buildStudioImportDraft({ host, worldInfoJSON = '', createdAt = new Date().toISOString() }) {
  const mapping = presentationMapping(host);
  let cards = [];
  if (mapping.kind === 'single') {
    const card = host.characters?.[host.characterId];
    if (card) cards = [cardReference(card)];
  } else {
    const byAvatar = new Map((host.characters ?? []).map(card => [String(card?.avatar), card]));
    cards = mapping.member_avatars.map(avatar => byAvatar.get(avatar)).filter(Boolean).map(cardReference);
  }
  return {
    version: 'corerp.studio-import-draft.v1',
    status: 'draft',
    created_at_utc: createdAt,
    source: { kind: 'sillytavern', presentation: mapping },
    references: { character_cards: cards, world_info: worldInfoReferences(worldInfoJSON) },
    authority_boundary: {
      creates_runtime_entities: false,
      activates_packages: false,
      imported_text_is_canonical_truth: false,
      required_next_step: 'Review and submit through an authorized CoreRP Studio creation/activation workflow.',
    },
  };
}

export function reconnectDelay(attempt) {
  if (!Number.isInteger(attempt) || attempt < 0) throw new Error('invalid reconnect attempt');
  return Math.min(16_000, 1_000 * (2 ** attempt));
}

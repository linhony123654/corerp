import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { join } from 'node:path';

const friends = ['faye', 'gita', 'hana', 'ivan'].map(name => `entity_final_${name}`);
const quote = value => `'${value.replaceAll("'", "''")}'`;
const at = (day, hour) => new Date(Date.UTC(2026, 8, 22 + day, hour, 30)).toISOString().replace('.000Z', 'Z');

export function finalLongActions(tool, read) {
  const observe = () => tool('corerp_observe', read);
  async function wait(target, key = target) {
    const view = await observe();
    const request = { ...read, expected_cursor: view.observation_cursor, idempotency_key: `final-long-wait-${key}`, target_world_time: target, budget: 1000 };
    for (let i = 0; i < 40; i++) {
      const out = await tool('corerp_wait', request);
      if (out.status === 'budget_exhausted') continue;
      assert.equal(out.status, 'completed'); assert.equal(out.current_world_time, target);
      return out;
    }
    throw new Error(`Unfinished bounded wait: ${target}`);
  }
  async function social(target, action, key, extra = {}) {
    const view = await observe();
    return tool('corerp_command', { operation: 'social', request: { ...read, expected_cursor: view.observation_cursor, idempotency_key: key, target_entity_id: target, action, ...extra } });
  }
  async function move(target, key) {
    for (let step = 0; step < 3; step++) {
      const view = await observe();
      if (view.place_id === target) return;
      const destination = view.place_id === 'place_m2_cafe' || target === 'place_m2_cafe' ? target : 'place_m2_cafe';
      assert.ok(view.reachable_places.some(place => place.place_id === destination && place.can_move_now));
      await tool('corerp_command', { operation: 'move', request: { ...read, expected_cursor: view.observation_cursor, idempotency_key: `${key}-${step}`, from_place_id: view.place_id, to_place_id: destination } });
    }
    throw new Error('Movement did not reach destination');
  }
  return { observe, wait, social, move };
}

export async function prepareFinalClientFriendships(actions) {
  for (const time of ['2026-09-22T07:10:00Z', '2026-09-22T07:20:00Z']) {
    const promises = new Map();
    for (const id of friends) promises.set(id, await actions.social(id, 'promise_meeting', `long-promise-${time}-${id}`, { meeting_place_id: 'place_m2_cafe', meeting_world_time: time }));
    await actions.wait(time);
    for (const id of friends) await actions.social(id, 'keep_meeting', `long-keep-${time}-${id}`, { promise_event_id: promises.get(id).event_id });
  }
}

export async function runFinalClientLong({ actions, tool, read, say, sql, restart, temp, screenshot }) {
  const turns = new Set();
  const state = { status: 'RUNNING', turns: 0, waits: 0, quietWaits: 0, gifts: 0, rareReceipts: 0, rareSelected: 0, rareMovements: 0, restarts: 0 };
  const topics = ['今天工作顺利吗？', '最近的日常开销还好吗？', '有空再说说邻里的生活吧。', '你有什么想自己决定的事？', '我会记住你之前的意见。', '今天不必安排什么大事。', '谢谢你愿意解释自己的想法。', '我们可以安静待一会儿。', '明天有机会再见面。', '不用急着答应我的请求。'];
  for (let day = 3; day <= 32; day++) {
    for (const hour of [0, 6, 12, 18]) {
      // Offstage windows leave remembered public meetings untouched until a
      // real visit happens; noon is actual continuing life with Nora.
      await actions.move(hour === 12 ? 'place_m2_cafe' : 'place_m2_home_bo', `long-place-${day}-${hour}`);
      const wait = await actions.wait(at(day, hour));
      state.waits++;
      if ((wait.initiatives ?? []).every(item => ['silence', 'wait'].includes(item.action))) state.quietWaits++;
      const payload = JSON.parse(sql(`SELECT payload FROM events WHERE event_id=${quote(wait.event_id)}`));
      for (const receipt of payload.visit_opportunities ?? []) {
        if (!receipt.rare) continue;
        state.rareReceipts++;
        assert.ok(receipt.draw.chance_basis_points <= 100 && !receipt.quiet);
        assert.ok(Date.parse(wait.current_world_time) - Date.parse(receipt.source.remembered_world_time) >= 7 * 86400000);
        if (!receipt.draw.selected) continue;
        state.rareSelected++;
        const effects = JSON.parse(sql(`SELECT json_group_array(json_object('event_id',event_id,'payload',json(payload))) FROM events WHERE event_type='RPWarmDecisionRecorded' AND json_extract(payload,'$.trigger_event_id')=${quote(wait.event_id)} AND json_extract(payload,'$.decision.actor_id')=${quote(receipt.actor_id)}`));
        for (const effect of effects) {
          if (effect.payload.decision.action !== 'leave' || effect.payload.decision.reason !== 'sourced_visit') continue;
          assert.equal(effect.payload.decision.to_place_id, receipt.source.place_id);
          assert.equal(Number(sql(`SELECT COUNT(*) FROM agent_movements WHERE event_id=${quote(effect.event_id)}`)), 1);
          state.rareMovements++;
        }
      }
      if (hour !== 12) continue;
      const view = await actions.observe();
      assert.ok(view.present_entities.some(item => item.entity_id === 'entity_final_nora'), 'Nora must really be present for daily relationship');
      if (day === 3) for (let i = 0; i < 2; i++) await actions.social('entity_final_nora', 'apologize', `long-repair-${i}`);
      await actions.social('entity_final_nora', 'gift', `long-gift-${day}`, { amount_minor: 1 }); state.gifts++;
      for (let i = 0; i < 10; i++) {
        const text = `第${day}天，${topics[(day + i) % topics.length]}`;
        let result;
        if (i % 2 === 0) result = await say(text);
        else {
          const current = await actions.observe();
          result = await tool('corerp_dialogue', { ...read, expected_cursor: current.observation_cursor, idempotency_key: `long-turn-${day}-${i}`, text });
        }
        assert.equal(result.status, 'settled'); assert.ok(!turns.has(result.turn_run_id)); turns.add(result.turn_run_id);
        state.turns = turns.size;
      }
    }
    if ([10, 20, 30].includes(day)) { await screenshot(`day-${day}`); await restart(); state.restarts++; }
    const progress = { ...state, day, worldTime: (await actions.observe()).world_time, artifacts: temp };
    await writeFile(join(temp, 'long-progress.json'), JSON.stringify(progress, null, 2));
    console.log(JSON.stringify(progress));
  }
  assert.equal(state.turns, 300); assert.equal(state.gifts, 30); assert.equal(state.restarts, 3);
  assert.ok(state.quietWaits >= 20);
  assert.ok(state.rareMovements >= 1, `Fixed stream has not established a rare movement: ${JSON.stringify(state)}`);
  const elapsedDays = (Date.parse((await actions.observe()).world_time) - Date.parse('2026-09-22T07:03:00Z')) / 86400000;
  assert.ok(elapsedDays >= 30);
  return { ...state, status: 'LONG_CLIENT_PASS', elapsedDays };
}

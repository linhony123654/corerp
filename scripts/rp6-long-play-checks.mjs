import assert from 'node:assert/strict'
import { join } from 'node:path'

// Actual UI commands against the temporary economic world. Initial gifts and
// policy installation are explicit fixture setup, never counted as UI turns.
export async function prepareLongPlay({ page, sql, credential, creatorCredential }) {
  const call = async (path, token, body) => {
    const response = await fetch(`http://127.0.0.1:8080/api/v1/${path}`, { method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
    const result = await response.json()
    assert.equal(response.status, 200, JSON.stringify(result))
    return result.data
  }
  const session = await page.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.v1')).session)
  const initialTime = sql("SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main'")
  const scope = { instance_id: 'inst_m2_t09', branch_id: 'br_main' }
  const npc = 'entity_emergent_nora'
  const head = () => Number(sql("SELECT head_sequence FROM branches WHERE instance_id='inst_m2_t09' AND branch_id='br_main'"))
  const materialized = await call('commands/materialize-cohort', creatorCredential, { ...scope, command_id: 'cmd_rp6_nora', materialization_id: 'mat_rp6_nora', capability_id: 'world.cohort.materialize', idempotency_key: 'rp6-nora', expected_head: head(), world_time: initialTime, source_cohort_id: 'cohort_block_a', entity_id: npc, display_name: 'Nora', population_count: 1, asset_minor: 200, inventory_minor: 1, receivable_minor: 0, liability_minor: 0, allocation_algorithm_version: 'equal-share-v1' })
  const at = hours => new Date(Date.parse(initialTime) + hours * 3600000).toISOString().replace('.000Z', 'Z')
  const schedule = Array.from({ length: 6 }, (_, day) => [{ world_time: at(12 + day * 24), place_id: 'place_m2_home_bo', activity_code: 'home' }, { world_time: at(24 + day * 24), place_id: 'place_m2_cafe', activity_code: 'present' }]).flat()
  const background = await call('rp/background/materialize', creatorCredential, { ...scope, entity_id: npc, expected_head: materialized.last_sequence, idempotency_key: 'rp6-nora-background', age_min: 25, age_max: 34, residence_place_id: 'place_m2_home_bo', initial_place_id: 'place_m2_cafe', schedule })
  console.log(JSON.stringify({ longPlay: 'sourced-disposition', disposition: background.background.disposition }))
  const gifts = []
  for (const target of ['entity_m2_rp_cai', npc]) for (let i = 0; i < 2; i++) {
    const view = await call('rp/observe', credential, { session_id: session })
    const gift = await call('rp/actions/social', credential, { session_id: session, target_entity_id: target, action: 'gift', amount_minor: 1, expected_cursor: view.observation_cursor, idempotency_key: `long-play-gift-${target}-${i}` })
    if (target === npc) gifts.push(gift.event_id)
  }
  const define = (path, body) => call(path, creatorCredential, { binding: { instance_id: 'inst_m2_t09', branch_id: 'br_main', expected_head: Number(sql("SELECT head_sequence FROM branches WHERE instance_id='inst_m2_t09' AND branch_id='br_main'")), idempotency_key: `long-play-${path}` }, ...body })
  await define('opportunities/policy/define', { policy: { stream_seed: 'rp6-ui-long-play-v1', contact_basis_points: 1500, cooldown_hours: 6, history_hours: 240, warm_enabled: true } })
  await define('opportunities/environment/define', { source: { place_id: 'place_m2_cafe', rain_basis_points: 1500, cooldown_hours: 6 } })
  await page.getByRole('button', { name: '环顾四周', exact: true }).click()
  return { session, npc, gifts, initialTime, turns: new Set(), waits: new Set(), visibleContacts: 0, quietWaits: 0, rememberedReplies: { 'before-restart': 0, 'after-restart': 0 } }
}

export async function playLongSegment({ page, sql, temp, state, segment }) {
  for (let i = 0; i < 52; i++) {
    const text = `长程生活 ${segment}-${i + 1}：你好，今天这里怎么样？`
    const response = page.waitForResponse(r => r.url().endsWith('/rp/turns/run'))
    await page.getByLabel('你想说的话').fill(text)
    await page.getByRole('button', { name: '说出 →', exact: true }).click()
    const result = await (await response).json()
    assert.ok(result.data?.turn_run_id, JSON.stringify(result))
    assert.ok(!state.turns.has(result.data.turn_run_id), 'every speech must be a distinct committed turn')
    state.turns.add(result.data.turn_run_id)
    await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
    assert.ok((await page.getByRole('main', { name: '对话历史' }).innerText()).includes(text), 'latest actual speech stays readable')
    const latest = await page.locator('.reading article.turn').last().innerText()
    if (latest.includes('Nora 回应：「我愿意相信你，接着说吧。')) state.rememberedReplies[segment]++
    assert.ok(await page.locator('.reading article.turn').count() <= 50, 'bounded history')
    assert.equal(await page.getByRole('alert').count(), 0)
    assert.equal(await page.getByLabel('你想说的话').isEnabled(), true)
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
    if ((i + 1) % 4 === 0) {
      let pending = page.waitForResponse(r => r.url().endsWith('/rp/actions/wait'))
      await page.getByRole('button', { name: '等四小时', exact: true }).click()
      let wait = await (await pending).json()
      for (let drain = 0; wait.data?.status === 'budget_exhausted' && drain < 20; drain++) {
        pending = page.waitForResponse(r => r.url().endsWith('/rp/actions/wait'))
        await page.getByRole('button', { name: '继续未完成的行动', exact: true }).click()
        wait = await (await pending).json()
      }
      assert.equal(wait.data?.status, 'completed', JSON.stringify(wait))
      assert.ok(wait.data.event_id)
      assert.ok(!state.waits.has(wait.data.event_id))
      state.waits.add(wait.data.event_id)
      await page.waitForFunction(() => !JSON.parse(localStorage.getItem('corerp.play.v1')).pending)
      const contact = wait.data.initiatives?.find(effect => effect.npc_entity_id === state.npc && effect.action === 'respond')
      if (contact) {
        const receipt = JSON.parse(sql(`SELECT payload FROM events WHERE event_id='${wait.data.event_id.replaceAll("'", "''")}'`)).contact_opportunities?.find(item => item.actor_id === state.npc)
        assert.equal(receipt?.draw.selected, true, 'actual spontaneous contact requires a selected own-source draw')
        assert.equal(Number(sql(`SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id='${state.npc}' AND subject_agent_id='entity_m2_rp_lin' AND source_event_id='${receipt.source_event_id.replaceAll("'", "''")}'`)), 1, 'contact source must be personally known player relationship evidence')
        assert.ok((await page.getByRole('main', { name: '对话历史' }).innerText()).includes('又见面了，最近过得怎么样？'), 'actual spontaneous speech visible after wait')
        state.visibleContacts++
      }
      if (wait.data.initiatives?.every(effect => ['silence', 'wait'].includes(effect.action))) state.quietWaits++
    }
    if ((i + 1) % 13 === 0) console.log(JSON.stringify({ longPlay: 'progress', segment, turns: state.turns.size, waits: state.waits.size }))
  }
  await page.screenshot({ path: join(temp, `long-play-${segment}-mobile.png`) })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.screenshot({ path: join(temp, `long-play-${segment}-desktop.png`) })
  await page.setViewportSize({ width: 390, height: 844 })
  const turns = Number(sql(`SELECT COUNT(*) FROM rp_turn_runs WHERE session_id='${state.session.replaceAll("'", "''")}' AND status='settled'`))
  assert.equal(turns, state.turns.size, 'UI count equals persisted settled turns')
}

export function verifyLongPlay({ sql, state }) {
  const elapsedHours = (Date.parse(sql("SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main'")) - Date.parse(state.initialTime)) / 3600000
  const wages = Number(sql("SELECT COUNT(*) FROM m2_economic_obligations WHERE kind='wage'"))
  const consumed = Number(sql("SELECT COUNT(*) FROM m2_consumption_outcomes WHERE status='consumed'"))
  const knowledge = Number(sql("SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id='entity_m2_rp_lin'"))
  const rain = Number(sql("SELECT COUNT(*) FROM events WHERE event_type='RPWaitCompleted' AND json_extract(payload,'$.environment.condition')='rain'"))
  assert.equal(state.turns.size, 104)
  assert.equal(state.waits.size, 26)
  assert.ok(elapsedHours >= 104)
  assert.ok(wages >= 4 && consumed >= 4, 'real multi-day economy must execute')
  assert.ok(knowledge > 2, 'real own knowledge persists')
  assert.ok(state.visibleContacts > 0, 'same-run selected contact must have a real visible consequence')
  assert.ok(state.quietWaits > state.visibleContacts, 'ordinary quiet time is not crowded out')
  assert.ok(state.rememberedReplies['before-restart'] > 0 && state.rememberedReplies['after-restart'] > 0, 'own remembered trust affects speech before and after restart')
  for (const event of state.gifts) assert.equal(Number(sql(`SELECT COUNT(*) FROM agent_knowledge WHERE observer_agent_id='${state.npc}' AND source_event_id='${event.replaceAll("'", "''")}'`)), 1, 'original personally witnessed gift remains known')
  // Rain evidence is reported, not silently manufactured by rerolling seeds.
  return { turns: state.turns.size, waits: state.waits.size, elapsedHours, wages, consumed, knowledge, rain, visibleContacts: state.visibleContacts, quietWaits: state.quietWaits, rememberedReplies: state.rememberedReplies }
}

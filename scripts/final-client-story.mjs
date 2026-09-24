import assert from 'node:assert/strict';

// Explicit actor choices exercise real HTTP authority; only Lin's dialogue,
// social actions and time advance are submitted through player clients.
export async function runFinalClientStory({ call, command, tool, read, sql, say }) {
  const scope = { instance_id: 'inst_m2_t09', branch_id: 'br_main' };
  const nora = 'entity_final_nora', ada = 'entity_m2_agent_ada', bo = 'entity_m2_agent_bo';
  const quote = value => `'${value.replaceAll("'", "''")}'`;
  const event = (type, kind) => sql(`SELECT event_id FROM events WHERE event_type=${quote(type)} AND json_extract(payload,'$.kind')=${quote(kind)} ORDER BY event_sequence LIMIT 1`);
  const law = event('RPInstitutionFactRecorded', 'law_enactment'), culture = event('RPCultureFactRecorded', 'definition');
  assert.ok(law && culture);
  const action = (turn, expected) => {
    const rows = JSON.parse(sql(`SELECT json_group_array(json_object('action',d.action,'event',e.event_id,'type',e.event_type)) FROM rp_npc_decisions d JOIN events e ON e.event_id=d.event_id WHERE d.parent_turn_id=${quote(turn.player_turn_id)} AND d.npc_entity_id=${quote(nora)} AND e.actor_id=d.npc_entity_id`));
    assert.equal(rows.length, 1);
    assert.equal(rows[0].action, expected);
    assert.ok(turn.npc_event_ids.includes(rows[0].event));
    if (expected === 'respond') assert.equal(rows[0].type, 'RPSpeechAccepted');
  };
  async function speak(text, expected) { const result = await say(text); action(result, expected); return result; }
  async function wait(at) {
    const view = await tool('corerp_observe', read);
    const request = { ...read, expected_cursor: view.observation_cursor, idempotency_key: `story-wait-${at}`, target_world_time: at, budget: 1000 };
    let result;
    for (let i = 0; i < 30; i++) { result = await tool('corerp_wait', request); if (result.status !== 'budget_exhausted') break; }
    assert.equal(result.status, 'completed'); assert.equal(result.current_world_time, at);
  }
  async function gift(amount, key) {
    const view = await tool('corerp_observe', read);
    return tool('corerp_command', { operation: 'social', request: { ...read, expected_cursor: view.observation_cursor, idempotency_key: key, target_entity_id: nora, action: 'gift', amount_minor: amount } });
  }
  const balance = entity => Number(sql(`SELECT b.balance_minor FROM materialized_entities e JOIN account_balances b ON b.account_id=e.asset_account_id WHERE e.entity_id=${quote(entity)}`));
  const initialBalance = balance(nora);
  await speak('今天有空聊聊自己的打算吗？', 'refuse');
  await command('career/applications/submit', 'nora', { application_id: 'final-client-application', position_id: 'position_coop_assistant', candidate_id: nora, statement: 'I choose to apply for the neighborhood job.' });
  await command('career/interviews/invite', 'bo', { interview_id: 'final-client-interview', application_id: 'final-client-application', question: 'Explain safe work.' });
  await command('career/interviews/answer', 'nora', { interview_id: 'final-client-interview', answer: 'Inspect equipment and exits.' });
  await command('career/evaluations/record', 'bo', { evaluation_id: 'final-client-evaluation', interview_id: 'final-client-interview', decision: 'advance', assessments: [{ code: 'safety_training', passed: true, reason: 'Assessment of the actual safety procedure answer.' }], reason: 'Manager assessment of the recorded interview.' });
  const offer = await command('career/offers/make', 'bo', { offer_id: 'final-client-offer', evaluation_id: 'final-client-evaluation', starts_on_day: 2, probation_days: 7, expires_at: '2026-09-24T00:00:00Z' });
  const accept = { offer_id: offer.fact.record_id, after_work_place_id: 'place_m2_cafe' };
  await command('career/offers/accept', 'nora', accept, 'BRANCH_VERSION_CONFLICT');
  const exit = await command('career/aggregate-employment/exit', 'nora', { candidate_id: nora, contract_id: 'contract_m2_cohort_wage_18', final_earned_day: 1, notice: 'I choose to leave after the earned first period.' });
  await wait('2026-09-23T12:00:00Z');
  const transmission = await command('culture/transmit', 'bo', { speaker_id: bo, definition_event_id: culture });
  const stance = { entity_id: nora, transmission_event_id: transmission.event_id, stance: 'rebel' };
  const rebel = await command('culture/internalize', 'nora', stance);
  const unwelcome = await gift(200, 'final-client-unwelcome-help');
  assert.ok(balance(nora) >= initialBalance + 200);
  await speak('现在手头宽裕一些了，可以聊聊邻里赠礼的习惯吗？', 'refuse');
  await command('culture/internalize', 'nora', { ...stance, stance: 'accept' });
  const welcome = await gift(1, 'final-client-welcome-help');
  await speak('谢谢你解释自己的想法，我愿意慢慢了解。', 'respond');
  await wait('2026-09-23T12:01:00Z');
  const job = await command('career/offers/accept', 'nora', accept);
  assert.equal(job.fact.employment.employee_id, nora);
  await wait('2026-09-24T12:00:00Z');
  await command('laws/announce', 'ada', { speaker_id: ada, enactment_event_id: law });
  const restrained = await speak('工作结束了，现在还能继续交谈吗？', 'silence');
  const beforeFine = balance('entity_m2_rp_lin');
  const violation = await command('laws/violations/record', 'bo', { institution_id: 'final-council', enforcer_id: bo, enactment_event_id: law, action_event_id: restrained.player_event_id });
  const fine = await command('laws/enforce', 'bo', { institution_id: 'final-council', enforcer_id: bo, violation_event_id: violation.event_id });
  assert.equal(balance('entity_m2_rp_lin'), beforeFine - 2);
  const proposal = await command('laws/propose', 'ada', { institution_id: 'final-council', proposer_id: ada, previous_enactment_event_id: law, repealed: true, law: { law_id: 'final-quiet', prohibited_action: 'speak', fine_minor: 0, text: 'The temporary quiet period ends; ordinary conversation may resume.' } });
  const repeal = await command('laws/enact', 'ada', { institution_id: 'final-council', legislator_id: ada, proposal_event_id: proposal.event_id, effective_world_time: '2026-09-24T13:00:00Z' });
  await wait('2026-09-24T13:00:00Z');
  const unheard = await speak('现在街区的规定改变了吗？', 'silence');
  await command('laws/violations/record', 'bo', { institution_id: 'final-council', enforcer_id: bo, enactment_event_id: law, action_event_id: unheard.player_event_id }, 'INVALID_ARGUMENT');
  await command('laws/announce', 'ada', { speaker_id: ada, enactment_event_id: repeal.event_id });
  await speak('听过新的公告后，我们可以继续聊日常了。', 'respond');
  assert.equal(balance('entity_m2_rp_lin'), beforeFine - 2, 'repeal changed historical fine');
  await wait('2026-09-25T00:01:00Z');
  const contract = job.fact.employment.contract_id;
  const attendance = await call('career/attendance/read', 'nora', { ...scope, contract_id: contract, day: 2 });
  assert.equal(attendance.attendance.recorded_seconds, 14400);
  assert.equal(Number(sql(`SELECT amount_paid_minor FROM wage_obligations WHERE contract_id=${quote(contract)} AND period_start_day=2`)), 12);
  await call('career/records/read', 'player', { ...scope, kind: 'evaluation', record_id: 'final-client-evaluation' }, 'PERMISSION_DENIED');
  return { pressureRefusal: true, initialBalance, culture: { definition: culture, transmission: transmission.event_id, rebel: rebel.event_id, unwelcome: unwelcome.event_id, welcome: welcome.event_id }, exit: exit.event_id, employment: job.event_id, contract, attendance: attendance.event_id, paid: 12, law, fine: fine.event_id, repeal: repeal.event_id, unheardRepealSilence: true, heardRepealResponse: true, privateInterviewDenied: true };
}

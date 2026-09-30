import assert from 'node:assert/strict'
import { chromium } from 'playwright'

// Browser + explicit API fixture, never a live model or proof of provider quality.
const origin = process.env.CORERP_R1_UI_URL || 'http://127.0.0.1:5175'
const browser = await chromium.launch({ headless: true })
try {
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } })
  const calls = [{ phase: 'decision', provider_kind: 'chat_completions', model_id: 'fixture-model', attempted: true, attempt_count: 2, result: 'timeout', fallback_kind: 'silence', started_at_utc: '2026-09-27T12:00:00Z' },
    { phase: 'narrative', provider_kind: 'deterministic', attempted: false, attempt_count: 0, result: 'success', render_source: 'template', started_at_utc: '2026-09-27T12:00:01Z' },
    { phase: 'narrative', provider_kind: 'full_prose', attempted: false, attempt_count: 0, result: 'pending', started_at_utc: '2026-09-27T12:00:02Z' }]
  const observation = {
    session_id: 'fixture-session', instance_id: 'fixture-world', branch_id: 'br_main',
    controlled_entity: { entity_id: 'fixture-player', display_name: '阿临' },
    present_entities: [], reachable_places: [], active_journey: null,
    place_id: 'fixture-place', place_name: '茶铺', world_time: '2026-09-27T12:00:00Z', observation_cursor: 1,
    decision_mode: 'chat_completions', narrative_mode: 'deterministic',
    recent_turns: [{ turn_run_id: 'fixture-turn', narrative_lines: ['你说：「你好。」', '阿梅保持沉默。'], provider_calls: calls, can_regenerate: false }]
  }
  await page.route('**/api/v1/rp/**', async route => {
    const request = route.request()
    assert.equal(request.headers().authorization, 'Bearer fixture-player-token')
    const name = new URL(request.url()).pathname.split('/').slice(-2).join('/')
    const data = name === 'bindings/list' ? { bindings: [{ instance_id: 'fixture-world', branch_id: 'br_main', entity_id: 'fixture-player', display_name: '阿临' }] }
      : name === 'sessions/open' || name === 'sessions/resume' ? { session_id: 'fixture-session', instance_id: 'fixture-world', branch_id: 'br_main', controlled_entity_id: 'fixture-player' }
        : name === 'rp/observe' ? observation : null
    await route.fulfill({ status: data ? 200 : 404, contentType: 'application/json', body: JSON.stringify(data ? { data } : { error: { message: `unexpected ${name}` } }) })
  })
  await page.goto(origin)
  await page.getByLabel('玩家访问凭证').fill('fixture-player-token')
  await page.getByRole('button', { name: '进入世界' }).click()
  await page.getByText('本轮人物模型调用失败或超时', { exact: false }).waitFor()
  const warning = await page.locator('.provider-alert').innerText()
  assert.match(warning, /技术回退/)
  assert.doesNotMatch(warning, /fixture-model/)
  await page.getByRole('button', { name: '打开侧边栏' }).click()
  await page.locator('.drawer-nav').getByRole('button', { name: '行动与功能' }).click()
  await page.getByRole('button', { name: /查看本角色的模型调用记录/ }).click()
  const inspector = await page.locator('.summary-timeline').innerText()
  assert.match(inspector, /fixture-model/)
  assert.match(inspector, /提供器超时/)
  assert.match(inspector, /已尝试 HTTP 请求 2 次/)
  assert.match(inspector, /未记录到 HTTP 请求/)
  assert.match(inspector, /结果未知（可能已发送） · HTTP 请求次数未知/)
  assert.match(inspector, /本次生成来源 template/)
  assert.doesNotMatch(inspector, /private-key|fixture-player-token|npc_entity_id/)
  console.log(JSON.stringify({ rpTruthR1UI: 'PASS', source: 'browser with mocked scoped observe response', checks: ['technical silence warning', 'model receipt in scoped inspector', 'no credential in rendered diagnostic'] }))
} finally {
  await browser.close()
}

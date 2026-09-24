import assert from 'node:assert/strict'
import { join } from 'node:path'

// Real service/SQLite acceptance. Only fault injection and format boundary cases
// use fixtures; no mocked balance can establish the owner-backed read assertion.
export async function checkWallet({ page, sql, temp, stage }) {
  const before = sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main')")
  const bookmark = await page.evaluate(() => localStorage.getItem('corerp.play.v1'))
  const opener = page.getByRole('button', { name: '钱包', exact: true })
  const dialog = page.getByRole('dialog', { name: '钱包', exact: true })
  const response = page.waitForResponse(r => r.url().endsWith('/rp/wallet/read'))
  await opener.click()
  const { data } = await (await response).json()
  assert.equal(data.balance_minor, sql("SELECT balance_minor FROM account_balances WHERE account_id=(SELECT asset_account_id FROM materialized_entities WHERE entity_id='entity_m2_rp_lin')"))
  assert.equal(typeof data.balance_minor, 'string')
  await page.getByTestId('wallet-amount').waitFor()
  assert.equal(await dialog.getByRole('heading', { name: '钱包' }).evaluate(el => getComputedStyle(el).color), 'rgb(52, 61, 53)', 'wallet title must retain paper/ink contrast')
  assert.equal(await dialog.getByText('Lin 的随身账户', { exact: true }).count(), 1)
  assert.equal(await dialog.evaluate(el => el.contains(document.activeElement)), true)
  await page.keyboard.press('Shift+Tab')
  assert.equal(await dialog.evaluate(el => el.contains(document.activeElement)), true, 'focus stays in modal')
  assert.equal(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth), true, 'wallet mobile overflow')
  await page.screenshot({ path: join(temp, `wallet-${stage}-mobile.png`) })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.screenshot({ path: join(temp, `wallet-${stage}-desktop.png`) })
  await page.setViewportSize({ width: 390, height: 844 })
  await page.keyboard.press('Escape')
  await dialog.waitFor({ state: 'hidden' })
  assert.equal(await opener.evaluate(el => document.activeElement === el), true, 'Escape restores opener focus')

  // Error is local to this read; it cannot create a pending world action.
  await page.route('**/api/v1/rp/wallet/read', route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { message: '测试读取暂不可用' } }) }), { times: 1 })
  await opener.click()
  await dialog.getByRole('alert').waitFor()
  assert.equal(await page.getByTestId('wallet-amount').count(), 0, 'no invented/stale balance on failed read')
  assert.equal(await page.evaluate(() => localStorage.getItem('corerp.play.v1')), bookmark)
  await dialog.getByRole('button', { name: '重新查看' }).click()
  await page.getByTestId('wallet-amount').waitFor()
  await dialog.getByRole('button', { name: '关闭钱包' }).click()
  await dialog.waitFor({ state: 'hidden' })

  let release
  const held = new Promise(resolve => { release = resolve })
  await page.route('**/api/v1/rp/wallet/read', async route => { await held; await route.continue() }, { times: 1 })
  await opener.click()
  await dialog.getByRole('status').waitFor()
  assert.equal(await dialog.getByRole('button', { name: '刷新余额' }).isDisabled(), true)
  await page.keyboard.press('Escape') // Closing never waits for the network.
  await dialog.waitFor({ state: 'hidden' })
  release()
  await opener.click()
  await page.getByTestId('wallet-amount').waitFor()
  await page.keyboard.press('Escape')
  await dialog.waitFor({ state: 'hidden' })

  const formats = await page.evaluate(async () => {
    const { formatMinorAmount } = await import('/src/lib/wallet.ts')
    const values = [['9007199254740993', 2], ['-1', 2], ['0', 0], ['1', 18], ['12345', 0]]
    let rejected = 0
    for (const input of [['1.1', 2], ['01', 2], ['1', 19], [9007199254740993, 2]]) {
      try { formatMinorAmount(...input) } catch { rejected++ }
    }
    return { formatted: values.map(input => formatMinorAmount(...input)), rejected }
  })
  assert.deepEqual(formats, { formatted: ['90,071,992,547,409.93', '−0.01', '0', '0.000000000000000001', '12,345'], rejected: 4 })
  assert.equal(sql("SELECT (SELECT COUNT(*) FROM events)||':'||(SELECT current_world_time FROM world_clocks WHERE instance_id='inst_m2_t09' AND branch_id='br_main')"), before)
  assert.equal(await page.evaluate(() => localStorage.getItem('corerp.play.v1')), bookmark)
  // Return the reading viewport to its normal end, as the main journey expects.
  await page.evaluate(() => window.scrollTo({ top: document.documentElement.scrollHeight }))
  console.log(JSON.stringify({ walletScenario: 'PASS', stage, balanceMinor: data.balance_minor, checks: ['real SQLite balance', 'modal focus and Escape', 'error/read retry', 'loading close/reopen', 'exact formatting boundaries', 'desktop/mobile artifacts', 'no world or bookmark mutation'] }))
}

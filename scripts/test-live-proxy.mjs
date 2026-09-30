import { chromium } from 'playwright'
import assert from 'node:assert/strict'

console.log('--- Testing Live Deployed Proxy Integration ---')

const browser = await chromium.launch({
  headless: true,
  args: ['--no-sandbox', '--disable-setuid-sandbox', '--ignore-certificate-errors'],
})

try {
  const page = await browser.newPage({ ignoreHTTPSErrors: true })

  page.on('console', msg => console.log('BROWSER:', msg.text()))
  page.on('pageerror', err => console.log('PAGE ERROR:', err))

  await page.goto('https://127.0.0.1:4188/')
  await page.waitForLoadState('networkidle')

  console.log('Testing testApiConnection from within page with Base URL (/v1)...')
  const testBaseRes = await page.evaluate(async () => {
    const res = await fetch('/api/v1/proxy/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        endpoint: 'https://api.deepseek.com/v1',
        apiKey: 'sk-test-invalid-key',
        model: 'deepseek-chat',
        timeoutSeconds: 8,
      }),
    })
    const text = await res.text()
    let data = null
    try { data = JSON.parse(text) } catch { data = text }
    return {
      status: res.status,
      body: data,
    }
  })

  console.log('Live Base URL (/v1) Test Result:', JSON.stringify(testBaseRes, null, 2))
  assert.equal(testBaseRes.status, 200)
  assert.equal(testBaseRes.body.data.ok, false)
  // It should NOT be 404, it should be 401 from DeepSeek because it resolved to /v1/chat/completions!
  assert.match(testBaseRes.body.data.message, /401/)
  console.log('✓ Base URL /v1 successfully resolved to /v1/chat/completions with 0 404 error!')

  console.log('Testing models proxy from within page...')
  const modelsRes = await page.evaluate(async () => {
    const res = await fetch('/api/v1/proxy/models', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        endpoint: 'https://api.deepseek.com/v1/chat/completions',
        apiKey: 'sk-test-invalid-key',
        timeoutSeconds: 8,
      }),
    })
    const text = await res.text()
    let data = null
    try { data = JSON.parse(text) } catch { data = text }
    return {
      status: res.status,
      body: data,
    }
  })

  console.log('Live Models Proxy Result:', JSON.stringify(modelsRes, null, 2))
  assert.equal(modelsRes.status, 200)
  assert.equal(modelsRes.body.data.ok, false)
  assert.match(modelsRes.body.data.message, /401/)
  console.log('✓ Models proxy also returns proper API error instead of "Failed to fetch"!')

  console.log('Testing full UI interaction on live preview...')
  const tokenInput = page.getByLabel('玩家访问凭证')
  if (await tokenInput.isVisible()) {
    await tokenInput.fill('qgLwN4Sl3ZTegihUAPJpr01qsNP_srN-ZsUF761FyzU')
    await page.getByRole('button', { name: /进入世界|继续这段生活/ }).click()
  }

  const modelPill = page.locator('.model-pill')
  await modelPill.waitFor({ timeout: 5000 })
  await modelPill.click()

  const sheet = page.locator('.sheet')
  await sheet.waitFor({ timeout: 5000 })

  // Click "+ DeepSeek V3"
  await page.locator('.preset-chip', { hasText: '+ DeepSeek V3' }).click()
  await page.locator('#cfg-key').fill('sk-invalid-test-key')

  // Click "测试连接"
  console.log('Clicking "测试连接" on UI...')
  await page.locator('.test-btn').click()

  const feedbackBox = page.locator('.test-feedback-box')
  await feedbackBox.waitFor({ timeout: 5000 })
  const feedbackText = await feedbackBox.innerText()
  console.log('UI Feedback text:', feedbackText)

  // Verify that it does NOT say "Failed to fetch"
  assert.ok(!feedbackText.includes('Failed to fetch'), 'UI must NOT say Failed to fetch!')
  assert.ok(feedbackText.includes('401'), 'UI should show 401 error from service')
  console.log('✓ UI successfully tested with 0 CORS/Failed-to-fetch issues!')

  // Also test "拉取模型" button
  console.log('Clicking "拉取模型" on UI...')
  await page.locator('.fetch-models-btn').click()
  const statusBar = page.locator('.fetch-status-bar')
  await statusBar.waitFor({ timeout: 5000 })
  const statusText = await statusBar.innerText()
  console.log('UI Fetch Model status text:', statusText)
  assert.ok(!statusText.includes('Failed to fetch'), 'Fetch model must NOT say Failed to fetch!')
  assert.ok(statusText.includes('401'), 'Fetch model status should show 401')
  console.log('✓ Pull models button also successfully proxied without Failed to fetch!')

  console.log('🎉 All Live Proxy tests PASSED!')
} finally {
  await browser.close()
}

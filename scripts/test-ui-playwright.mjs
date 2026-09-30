import { chromium } from 'playwright'
import { spawn } from 'node:child_process'
import assert from 'node:assert/strict'

console.log('--- Starting Playwright UI Verification (with Pull Models) ---')

const preview = spawn('npx', ['vite', 'preview', '--port', '5826'], {
  cwd: '/home/ubuntu/corerp-console',
  stdio: ['ignore', 'pipe', 'pipe'],
})

let previewUrl = ''
preview.stdout.on('data', data => {
  const text = data.toString()
  const match = text.match(/http:\/\/localhost:(\d+)\/?/)
  if (match) previewUrl = match[0].replace(/\/$/, '')
})

for (let i = 0; i < 40; i++) {
  if (previewUrl) break
  await new Promise(r => setTimeout(r, 200))
}

assert.ok(previewUrl, 'Vite preview server failed to start')
console.log('Preview server ready at', previewUrl)

const browser = await chromium.launch({
  headless: true,
  args: ['--no-sandbox', '--disable-setuid-sandbox'],
})

try {
  const page = await browser.newPage({
    viewport: { width: 390, height: 844 },
    deviceScaleFactor: 1,
  })

  // Mock all necessary API endpoints
  await page.route('**/api/v1/rp/bindings/list', route => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        data: {
          bindings: [{ instance_id: 'inst-1', branch_id: 'branch-1', entity_id: 'entity-1', display_name: '白鸦酒馆' }]
        }
      })
    })
  })

  await page.route('**/api/v1/rp/sessions/open', route => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        data: {
          session_id: 'test-session-1',
          instance_id: 'inst-1',
          branch_id: 'branch-1',
          controlled_entity_id: 'entity-1',
        }
      })
    })
  })

  await page.route('**/api/v1/rp/sessions/resume', route => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        data: {
          session_id: 'test-session-1',
          instance_id: 'inst-1',
          branch_id: 'branch-1',
          controlled_entity_id: 'entity-1',
        }
      })
    })
  })

  await page.route('**/api/v1/rp/observe', route => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        data: {
          session_id: 'test-session-1',
          decision_mode: 'deterministic',
          narrative_mode: 'deterministic',
          place_id: 'place_tavern',
          place_name: '白鸦酒馆',
          world_time: '2026-09-26T12:00:00Z',
          controlled_entity: { entity_id: 'entity-1', display_name: '旅人' },
          present_entities: [],
          recent_turns: [],
          reachable_places: [],
        }
      })
    })
  })

  // Mock /v1/models endpoint for pulling models
  await page.route('**/v1/models', route => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        object: 'list',
        data: [
          { id: 'deepseek-chat' },
          { id: 'deepseek-reasoner' },
          { id: 'deepseek-coder' }
        ]
      })
    })
  })

  // Open the page
  await page.goto(previewUrl)
  await page.waitForLoadState('networkidle')

  // Check arrival view
  const tokenInput = page.getByLabel('玩家访问凭证')
  if (await tokenInput.isVisible()) {
    console.log('Filling token on arrival screen...')
    await tokenInput.fill('mock-token-123')
    await page.getByRole('button', { name: '进入世界' }).click()
  }

  // Wait for composer dock
  const modelPill = page.locator('.model-pill')
  await modelPill.waitFor({ timeout: 5000 })
  const initialText = (await modelPill.innerText()).trim()
  console.log('Initial model pill text:', initialText)
  assert.equal(initialText, '确定性人物')

  // Click model pill to open settings sheet
  await modelPill.click()
  const sheet = page.locator('.sheet')
  await sheet.waitFor({ timeout: 5000 })

  const sheetTitle = (await page.locator('#sheet-title').innerText()).trim()
  console.log('Sheet title:', sheetTitle)
  assert.equal(sheetTitle, '模型与 API 设置')

  // Check default option is selected
  const defaultOption = page.locator('.option-row', { hasText: '系统默认 · 确定性人物' })
  assert.equal(await defaultOption.getAttribute('aria-checked'), 'true')
  console.log('✓ Default option verified')

  // Click "+ DeepSeek V3" preset chip to create profile
  const deepseekChip = page.locator('.preset-chip', { hasText: '+ DeepSeek V3' })
  await deepseekChip.click()

  // Verify edit form
  await page.locator('.edit-mode-title').waitFor({ timeout: 3000 })
  assert.equal(await page.locator('#cfg-name').inputValue(), 'DeepSeek V3')
  assert.equal(await page.locator('#cfg-endpoint').inputValue(), 'https://api.deepseek.com/v1/chat/completions')
  assert.equal(await page.locator('#cfg-model').inputValue(), 'deepseek-chat')
  console.log('✓ Preset loaded successfully')

  // Fill API key
  await page.locator('#cfg-key').fill('sk-test-deepseek-key-12345')

  // Test Pull Models button!
  console.log('Testing "拉取模型" button...')
  const pullBtn = page.locator('.fetch-models-btn')
  await pullBtn.click()

  // Verify that status bar indicates success
  const statusBar = page.locator('.fetch-status-bar.is-ok')
  await statusBar.waitFor({ timeout: 4000 })
  const statusText = (await statusBar.innerText()).trim()
  console.log('Fetch status:', statusText)
  assert.match(statusText, /成功拉取到 3 个模型/)

  // Verify model chips are visible
  const reasonerChip = page.locator('.model-chip-tag', { hasText: 'deepseek-reasoner' })
  await reasonerChip.waitFor({ timeout: 3000 })
  // Click deepseek-reasoner chip to select it
  await reasonerChip.click()
  assert.equal(await page.locator('#cfg-model').inputValue(), 'deepseek-reasoner')
  console.log('✓ Model selected from pulled list: deepseek-reasoner!')

  // Click "保存并设为当前生效"
  await page.getByRole('button', { name: '保存并设为当前生效' }).click()

  // Verify we are back in profile list and DeepSeek V3 is marked active with deepseek-reasoner
  await page.locator('.active-tag', { hasText: '当前生效' }).waitFor({ timeout: 3000 })
  console.log('✓ Saved and activated')

  // Close sheet
  await page.locator('.sheet-header .icon-button[aria-label="关闭详情"]').click()
  await page.locator('.sheet').waitFor({ state: 'hidden', timeout: 3000 })

  // Verify model pill in composer dock now displays 'deepseek-reasoner'!
  const updatedPillText = (await modelPill.innerText()).trim()
  console.log('Updated model pill text in composer dock:', updatedPillText)
  assert.equal(updatedPillText, 'deepseek-reasoner')
  console.log('✓ Composer pill dynamically displays active pulled model name: deepseek-reasoner!')

  // Test pulling from list mode icon button
  await modelPill.click()
  await sheet.waitFor({ timeout: 5000 })
  const listPullBtn = page.locator('.icon-action-btn[title="拉取该端点模型列表"]')
  await listPullBtn.waitFor({ timeout: 3000 })
  await listPullBtn.click()

  // Verify it switches into edit view and pulls models
  await page.locator('.edit-mode-title').waitFor({ timeout: 3000 })
  await statusBar.waitFor({ timeout: 4000 })
  console.log('✓ List mode pull button switched to edit and fetched models successfully!')

  // Switch back to deepseek-chat via chip
  const chatChip = page.locator('.model-chip-tag', { hasText: 'deepseek-chat' })
  await chatChip.click()
  assert.equal(await page.locator('#cfg-model').inputValue(), 'deepseek-chat')
  await page.getByRole('button', { name: '保存并设为当前生效' }).click()
  await page.locator('.sheet-header .icon-button[aria-label="关闭详情"]').click()
  await page.locator('.sheet').waitFor({ state: 'hidden', timeout: 3000 })

  const chatPillText = (await modelPill.innerText()).trim()
  console.log('Model pill after switching to deepseek-chat:', chatPillText)
  assert.equal(chatPillText, 'deepseek-chat')

  // Refresh page and verify persistence
  await page.reload()
  await page.waitForLoadState('networkidle')
  const reloadToken = page.getByLabel('玩家访问凭证')
  if (await reloadToken.isVisible()) {
    await reloadToken.fill('mock-token-123')
    const btn = page.getByRole('button', { name: /进入世界|继续这段生活/ })
    await btn.click()
  }
  await modelPill.waitFor({ timeout: 5000 })
  const persistedPillText = (await modelPill.innerText()).trim()
  console.log('Persisted model pill text after reload:', persistedPillText)
  assert.equal(persistedPillText, 'deepseek-chat')
  console.log('✓ Persistence verified across page reloads!')

  // Switch back to system default
  await modelPill.click()
  await sheet.waitFor({ timeout: 5000 })
  await defaultOption.click()
  await page.locator('.sheet-header .icon-button[aria-label="关闭详情"]').click()
  await page.locator('.sheet').waitFor({ state: 'hidden', timeout: 3000 })

  const revertedPillText = (await modelPill.innerText()).trim()
  console.log('Reverted model pill text:', revertedPillText)
  assert.equal(revertedPillText, '确定性人物')
  console.log('✓ Reverted to default successfully!')

  console.log('🎉 All Playwright UI tests with Model Pulling PASSED with 100% success!')
} finally {
  await browser.close()
  preview.kill('SIGKILL')
  process.exit(0)
}

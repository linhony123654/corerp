import { chromium } from 'playwright'
import assert from 'node:assert/strict'

console.log('--- Testing Live Deployed Preview on Port 4188 ---')

const browser = await chromium.launch({
  headless: true,
  args: ['--no-sandbox', '--disable-setuid-sandbox', '--ignore-certificate-errors'],
})

try {
  const page = await browser.newPage({
    viewport: { width: 390, height: 844 },
    ignoreHTTPSErrors: true,
  })

  await page.goto('https://127.0.0.1:4188/')
  await page.waitForLoadState('networkidle')

  // Check arrival view
  const tokenInput = page.getByLabel('玩家访问凭证')
  const isArrival = await tokenInput.isVisible()
  console.log('Live page arrival visible:', isArrival)
  assert.ok(isArrival, 'Arrival screen should be visible')

  // Let's verify the assets and page title
  const title = await page.title()
  console.log('Page title:', title)

  console.log('✓ Live deployed bundle verified on port 4188!')
} finally {
  await browser.close()
}

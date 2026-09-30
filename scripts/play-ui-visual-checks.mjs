import assert from 'node:assert/strict'
import { mkdir } from 'node:fs/promises'
import { join, resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

const palette = { claude: 'rgb(242, 240, 233)', cream: 'rgb(243, 237, 222)', sage: 'rgb(237, 241, 236)', white: 'rgb(247, 247, 245)' }
const screenshot = (page, path) => page.screenshot({ path, animations: 'disabled' })

// Reference and real app share exactly the same viewport; screenshots are for human
// composition review, while geometry, focus, theme and scroll assertions are executable.
export async function checkPlayVisual({ page, browser, temp }) {
  const dir = join(temp, 'ui')
  await mkdir(dir)
  const reference = await browser.newPage({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 1, reducedMotion: 'reduce' })
  const prototype = pathToFileURL(resolve(import.meta.dirname, '../docs/ui/corerp_themes.html')).href
  await reference.goto(prototype)
  await screenshot(reference, join(dir, 'reference-main.png'))
  await reference.getByRole('button', { name: '打开侧边栏' }).click()
  await screenshot(reference, join(dir, 'reference-drawer.png'))
  await reference.getByRole('button', { name: '关闭侧边栏' }).click()
  await reference.getByRole('button', { name: '故事操作' }).click()
  await screenshot(reference, join(dir, 'reference-menu.png'))
  await reference.getByRole('menuitem', { name: '故事设置' }).click()
  await screenshot(reference, join(dir, 'reference-settings.png'))
  await reference.getByRole('button', { name: /主题与外观/ }).click()
  for (const [name, label] of [['claude', 'Claude 风格'], ['cream', '奶油纸'], ['sage', '雾青'], ['white', '纯净白']]) {
    await reference.getByRole('radio', { name: new RegExp(label) }).click()
    await screenshot(reference, join(dir, `reference-${name}.png`))
  }
  await reference.getByRole('button', { name: '关闭详情' }).click()
  await reference.getByRole('button', { name: /在场 3/ }).click()
  await reference.getByRole('button', { name: /艾琳/ }).click()
  await screenshot(reference, join(dir, 'reference-detail.png'))
  await reference.getByRole('button', { name: '关闭详情' }).click()
  await reference.getByRole('button', { name: '继续剧情' }).click()
  await screenshot(reference, join(dir, 'reference-thinking.png'))
  await reference.locator('.thinking-line.is-running').click()
  await screenshot(reference, join(dir, 'reference-thinking-sheet.png'))
  await reference.close()

  assert.equal(await page.locator('.play-app').evaluate(el => el.clientWidth), 390)
  await screenshot(page, join(dir, 'play-main.png'))
  await page.getByRole('button', { name: '打开侧边栏' }).click()
  const drawer = page.getByRole('dialog', { name: 'CoreRP' })
  await drawer.waitFor()
  await screenshot(page, join(dir, 'play-drawer.png'))
  await page.keyboard.press('Tab')
  assert.equal(await drawer.evaluate(el => el.contains(document.activeElement)), true)
  await page.getByRole('button', { name: '关闭侧边栏' }).click()
  await page.getByRole('button', { name: '故事操作' }).click()
  await page.keyboard.press('ArrowDown')
  assert.equal(await page.getByRole('menuitem', { name: '导出当前记录' }).evaluate(el => el === document.activeElement), true)
  await screenshot(page, join(dir, 'play-menu.png'))
  await page.keyboard.press('Escape')
  assert.equal(await page.getByRole('button', { name: '故事操作' }).evaluate(el => el === document.activeElement), true)
  await page.getByRole('button', { name: '故事操作' }).click()
  await page.getByRole('menuitem', { name: '故事设置' }).click()
  await screenshot(page, join(dir, 'play-settings.png'))
  await page.getByRole('button', { name: /主题与外观/ }).click()
  const choices = page.getByRole('radiogroup', { name: '浅色主题' })
  await choices.getByRole('radio', { name: /Claude 风格/ }).focus()
  await page.keyboard.press('ArrowRight')
  assert.equal(await page.locator('.play-app').getAttribute('data-theme'), 'cream', 'theme arrows select and preview')
  for (const [name, label] of [['claude', 'Claude 风格'], ['cream', '奶油纸'], ['sage', '雾青'], ['white', '纯净白']]) {
    await choices.getByRole('radio', { name: new RegExp(label) }).click()
    assert.equal(await page.locator('.play-app').evaluate(el => getComputedStyle(el).backgroundColor), palette[name])
    await screenshot(page, join(dir, `play-${name}.png`))
  }
  await choices.getByRole('radio', { name: /Claude 风格/ }).click()
  assert.equal(await page.evaluate(() => JSON.parse(localStorage.getItem('corerp.play.appearance.v1')).theme), 'claude')
  await page.keyboard.press('Escape')
  await page.getByRole('heading', { name: '阅读与故事设置' }).waitFor()
  await page.getByRole('button', { name: '关闭详情' }).click()
  await page.getByRole('navigation', { name: '当前场景' }).getByRole('button').nth(1).click()
  await screenshot(page, join(dir, 'play-detail.png'))
  await page.getByRole('button', { name: '关闭详情' }).click()

  let release
  const held = new Promise(resolve => { release = resolve })
  await page.route('**/api/v1/rp/observe', async route => { await held; await route.continue() }, { times: 1 })
  await page.getByRole('button', { name: '环顾四周' }).click()
  await page.locator('.thinking-line').getByText('正在读取当前观察').waitFor()
  await screenshot(page, join(dir, 'play-thinking.png'))
  await page.locator('.thinking-line').click()
  const summary = page.getByRole('dialog', { name: '公开状态摘要' })
  await summary.getByText('不是模型思维链', { exact: false }).waitFor()
  await screenshot(page, join(dir, 'play-thinking-sheet.png'))
  await page.getByRole('button', { name: '关闭详情' }).click()
  release()
  await page.locator('.thinking-line').waitFor({ state: 'hidden' })

  const input = page.getByLabel('你想说的话')
  await input.focus()
  await input.fill('先保留这一行')
  await input.press('Enter')
  assert.equal(await input.inputValue(), '先保留这一行\n', 'Enter creates a line break without sending')
  await input.fill(Array.from({ length: 12 }, (_, n) => `多行输入测试 ${n + 1}：保留用户草稿。`).join('\n'))
  await page.setViewportSize({ width: 390, height: 500 })
  await page.waitForFunction(() => document.querySelector('.play-app')?.classList.contains('keyboard-open'))
  const bounds = await page.evaluate(() => {
    const root = document.querySelector('.play-app'), dock = document.querySelector('.composer-dock'), textarea = document.querySelector('#words'), scroll = document.querySelector('.scrollport')
    scroll.scrollTop = scroll.scrollHeight
    return { app: root.getBoundingClientRect().bottom, dock: dock.getBoundingClientRect().bottom, inputHeight: textarea.getBoundingClientRect().height, overflow: root.scrollWidth > root.clientWidth }
  })
  assert.equal(bounds.overflow, false, 'narrow viewport must not scroll sideways')
  assert.ok(bounds.inputHeight <= 76 && bounds.inputHeight >= 37, 'multiline composer is capped in a short viewport')
  assert.ok(bounds.dock <= bounds.app + 1, 'keyboard-size viewport must not hide the composer')
  await screenshot(page, join(dir, 'play-keyboard-multiline.png'))
  await page.setViewportSize({ width: 390, height: 844 })
  await page.evaluate(() => {
    Object.defineProperty(window.visualViewport, 'height', { configurable: true, value: 510 })
    window.visualViewport.dispatchEvent(new Event('resize'))
  })
  await page.waitForFunction(() => document.querySelector('.play-app')?.clientHeight === 510 && document.querySelector('.play-app')?.classList.contains('keyboard-open'))
  const visualKeyboard = await page.evaluate(() => {
    const root = document.querySelector('.play-app'), dock = document.querySelector('.composer-dock'), textarea = document.querySelector('#words'), scroll = document.querySelector('.scrollport')
    scroll.scrollTop = scroll.scrollHeight
    return { dock: dock.getBoundingClientRect().bottom, app: root.getBoundingClientRect().bottom, inputHeight: textarea.getBoundingClientRect().height, clear: document.querySelector('.reading .turn:last-of-type').getBoundingClientRect().bottom <= dock.getBoundingClientRect().top }
  })
  assert.ok(visualKeyboard.inputHeight <= 76 && visualKeyboard.dock <= visualKeyboard.app + 1 && visualKeyboard.clear, 'visualViewport-only keyboard shrink keeps input and last turn accessible')
  await screenshot(page, join(dir, 'play-visual-viewport-keyboard.png'))
  await page.evaluate(() => { delete window.visualViewport.height; window.visualViewport.dispatchEvent(new Event('resize')) })
  await input.fill('')
  await page.getByRole('main', { name: '故事对话' }).evaluate(el => { el.scrollTop = el.scrollHeight })
  await page.waitForTimeout(40)
  const clearance = await page.locator('.reading .turn').last().evaluate(el => el.getBoundingClientRect().bottom <= document.querySelector('.composer-dock').getBoundingClientRect().top)
  assert.equal(clearance, true, 'last story turn remains above floating composer')
  console.log(JSON.stringify({ playUiVisual: 'PASS', reference: 'docs/ui/corerp_themes.html', viewport: '390×844', artifacts: dir, states: ['chat', 'drawer', 'anchored menu', 'settings', 'second-level theme', 'four palettes', 'detail', 'real request status and summary', 'layout and visualViewport keyboard shrink/multiline/scroll clearance'] }))
}

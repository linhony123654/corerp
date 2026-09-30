// Navigate the approved chat-first Play surface without exposing hidden toolbars.
export async function openPlayTools(page) {
  await page.getByRole('button', { name: '行动与功能', exact: true }).click()
  return page.getByRole('dialog', { name: '行动与功能' })
}

export async function clickPlayWait(page, hours) {
  const tools = await openPlayTools(page)
  await tools.getByRole('button', { name: new RegExp(`^等${hours === 1 ? '一' : '四'}小时`) }).click()
}

export async function openPlayLocation(page) {
  await page.getByRole('navigation', { name: '当前场景' }).getByRole('button').first().click()
  return page.getByRole('dialog', { name: '地点与可前往的区域' })
}

export async function movePlayTo(page, name) {
  const location = await openPlayLocation(page)
  await location.getByRole('button', { name: new RegExp(name) }).click()
}

export async function openPlayBusiness(page, kind) {
  if (kind === '地图') {
    const location = await openPlayLocation(page)
    await location.getByRole('button', { name: /查看附近地图/ }).click()
    return page.getByRole('dialog', { name: '附近地图' })
  }
  const tools = await openPlayTools(page)
  await tools.getByRole('button', { name: new RegExp(`^${kind}`) }).click()
  return page.getByRole('dialog', { name: kind, exact: kind !== '手机' })
}

export async function setPlayInputMode(page, mode) {
  const names = { speech: '只说话', AUTO: '自然输入', DIALOGUE: '明确说话', SCENE: '场景指令' }
  await page.getByRole('button', { name: /^输入方式：/ }).click()
  const sheet = page.getByRole('dialog', { name: '输入方式' })
  await sheet.getByRole('radio', { name: new RegExp(`^${names[mode]}`) }).click()
}

export async function openPlayWorldPicker(page) {
  await page.getByRole('button', { name: '打开侧边栏' }).click()
  await page.getByRole('dialog', { name: 'CoreRP' }).getByRole('button', { name: '选择其他世界' }).click()
}

export async function scrollPlayToEnd(page) {
  await page.getByRole('main', { name: '故事对话' }).evaluate(el => { el.scrollTop = el.scrollHeight })
}

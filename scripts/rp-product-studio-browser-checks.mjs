import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { chromium } from 'playwright';
import { checkStreamParser } from './rp6-stream-parser-checks.mjs';

// Run against the local Vite client. Abort creation before it reaches a server;
// no fabricated success, world creation, backend credential or model call.
const origin = process.argv[2] || 'http://127.0.0.1:4317';
assert.ok(['127.0.0.1', 'localhost'].includes(new URL(origin).hostname), 'local client only');
const expected = JSON.parse(readFileSync(new URL('../docs/rp-runtime-r1/rongqing-product-world-spec-2026-10-01.json', import.meta.url)));
const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.route('**/api/**', route => route.abort('failed'));
  await page.goto(`${origin}/studio/create`);
  await page.getByLabel('世界名称', { exact: true }).fill('保留的自定义草稿');
  await page.getByLabel('起点示例', { exact: true }).selectOption('rongqing');
  assert.equal(await page.getByLabel('NPC 人设', { exact: true }).count(), 0);
  await page.getByText('查看完整示例设定', { exact: true }).click();
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  await page.getByLabel('起点示例', { exact: true }).selectOption('custom');
  assert.equal(await page.getByLabel('世界名称', { exact: true }).inputValue(), '保留的自定义草稿');
  await page.getByLabel('起点示例', { exact: true }).selectOption('rongqing');
  await page.getByLabel('创建者访问凭证', { exact: true }).fill('fixture-credential-never-sent');
  await page.getByLabel('授权来源世界', { exact: true }).fill('fixture-authority');
  await page.getByLabel('玩家身份标识', { exact: true }).fill('fixture-player');
  assert.equal(await page.getByRole('button', { name: '保存世界与安装包', exact: true }).isDisabled(), true);
  await page.getByRole('checkbox').check();
  let submitted;
  await page.route('**/api/v1/studio/worlds/create', async route => {
    submitted = route.request().postDataJSON();
    await route.abort('failed');
  });
  await page.getByRole('button', { name: '保存世界与安装包', exact: true }).click();
  await page.getByRole('alert').waitFor();
  assert.deepEqual(submitted.spec, expected);
  assert.equal(await page.getByRole('heading', { name: '世界已保存', exact: true }).count(), 0);
  assert.ok(!JSON.stringify(await page.context().storageState()).includes('fixture-credential-never-sent'));
  await page.reload();
  assert.equal(await page.getByLabel('创建者访问凭证', { exact: true }).inputValue(), '');
  assert.equal(await page.getByRole('button', { name: '重试 / 核对原保存请求', exact: true }).count(), 1);
  const saved = await page.evaluate(() => JSON.parse(localStorage.getItem(localStorage.getItem('corerp.studio.create.current'))));
  assert.deepEqual(saved, submitted);
  await checkStreamParser(page);
  assert.deepEqual(errors, []);
  console.log(JSON.stringify({ studioPreset: 'PASS', checks: ['mobile declaration review without overflow', 'custom draft roundtrip', 'consent/authority/credential required', 'exact authored request', 'aborted creation retains immutable recovery', 'no false saved receipt', 'credential cleared/not stored', 'no page errors'], worldCreated: false, providerCalls: 0 }));
} finally { await browser.close(); }

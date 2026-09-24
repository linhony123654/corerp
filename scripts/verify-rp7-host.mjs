import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { chromium } from 'playwright';

// Run only against the disposable, loopback host described in rp7/sillytavern.md.
// This is a real-host API smoke check, not extension compatibility E2E.
const target = new URL(process.argv[2] ?? 'http://127.0.0.1:4187');
assert.equal(target.hostname, '127.0.0.1');
assert.equal(target.protocol, 'http:');
assert.equal(target.pathname, '/');
assert.equal(target.username + target.password + target.search + target.hash, '');
const fixture = JSON.parse(await readFile(new URL('../clients/sillytavern/host-fixture.json', import.meta.url), 'utf8'));
const browser = await chromium.launch({ headless: true });
try {
  const context = await browser.newContext();
  // No model/third-party browser requests are authorized for fixture inspection.
  await context.route('**/*', route => {
    const url = new URL(route.request().url());
    return url.origin === target.origin ? route.continue() : route.abort();
  });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.goto(target.href, { waitUntil: 'domcontentloaded' });
  await page.waitForFunction(() => typeof globalThis.SillyTavern?.getContext === 'function');
  const evidence = await page.evaluate(async () => {
    const context = globalThis.SillyTavern.getContext();
    const version = await fetch('/version').then(response => response.json());
    const extensions = await import('/scripts/extensions.js');
    const required = ['setExtensionPrompt', 'saveMetadata', 'saveSettingsDebounced', 'getCurrentChatId', 'addOneMessage', 'saveChat'];
    const functions = Object.fromEntries(required.map(key => [key, typeof context[key]]));
    return {
      version, functions,
      interceptorType: typeof extensions.runGenerationInterceptors,
      chatChanged: context.eventTypes?.CHAT_CHANGED,
      appReady: context.eventTypes?.APP_READY,
      settingsType: typeof context.extensionSettings,
      slashParserType: typeof context.SlashCommandParser?.addCommandObject,
    };
  });
  assert.equal(evidence.version.pkgVersion, fixture.version);
  for (const value of Object.values(evidence.functions)) assert.equal(value, 'function');
  assert.equal(evidence.interceptorType, 'function');
  assert.equal(evidence.slashParserType, 'function');
  assert.equal(evidence.settingsType, 'object');
  assert.equal(typeof evidence.chatChanged, 'string');
  assert.equal(typeof evidence.appReady, 'string');
  assert.deepEqual(errors, []);
  console.log(JSON.stringify({ result: 'PASS', scope: 'actual-host-api-smoke-only', ...evidence }, null, 2));
} finally {
  await browser.close();
}

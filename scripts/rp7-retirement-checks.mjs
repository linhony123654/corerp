import assert from 'node:assert/strict';

export async function checkRP7OpenRetirement({ page, token, cardAvatar, authoritySnapshot, hostUITimeout = 120_000 }) {
  const panel = page.locator('#corerp-runtime');
  const before = authoritySnapshot();
  await panel.locator('[name=instance]').fill('invalid-world-for-retirement-test');
  await panel.locator('[data-action=connect]').click();
  await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false' && Boolean(SillyTavern.getContext().chatMetadata.corerp_runtime?.pending));
  const pending = await page.evaluate(() => structuredClone(SillyTavern.getContext().chatMetadata.corerp_runtime.pending));
  assert.equal(pending.operation, 'open');
  assert.equal(await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.session_id), undefined);
  await page.reload();
  await page.waitForSelector('#corerp-runtime', { state: 'attached', timeout: hostUITimeout });
  await page.evaluate(async avatar => {
    const ctx = SillyTavern.getContext(); await ctx.getCharacters();
    const id = SillyTavern.getContext().characters.findIndex(character => character.avatar === avatar);
    await SillyTavern.getContext().selectCharacterById(String(id));
  }, cardAvatar);
  assert.deepEqual(await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.pending), pending);
  assert.equal(await panel.locator('[name=token]').inputValue(), '');
  await page.locator('#extensions-settings-button').click();
  await panel.locator('[name=token]').fill(token);
  let unexpectedOpens = 0;
  const countOpen = request => { if (request.url().endsWith('/api/v1/rp/sessions/open')) unexpectedOpens++; };
  page.on('request', countOpen);
  await panel.locator('[data-action=retire]').click();
  await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false' && !SillyTavern.getContext().chatMetadata.corerp_runtime.pending);
  page.off('request', countOpen);
  assert.equal(unexpectedOpens, 0, 'retirement implicitly retried invalid open');
  assert.equal(authoritySnapshot(), before, 'invalid open/retirement changed world');
  await panel.getByText('新建绑定：引用现有世界与角色', { exact: true }).click();
  await panel.locator('[name=instance]').fill('inst_m2_t09');
  await panel.locator('[name=branch]').fill('br_main');
  await panel.locator('[name=entity]').fill('entity_m2_rp_lin');
  await panel.locator('[name=token]').fill(token);
  return { invalidOpenReloaded: true, retiredWithoutRetryingOpen: true, worldUnchanged: true };
}

export async function checkRP7Retirement({ page, browserContext, token, authoritySnapshot, runtimeOrigin = 'http://127.0.0.1:4188' }) {
  const panel = page.locator('#corerp-runtime');
  const before = authoritySnapshot();
  await panel.locator('[name=operation]').selectOption('wait');
  await panel.locator('[name=command]').fill(JSON.stringify({ target_world_time: '2020-01-01T00:00:00Z', budget: 1 }));
  const rejected = page.waitForResponse(response => response.url().endsWith('/api/v1/rp/actions/wait') && response.request().method() === 'POST');
  await panel.locator('[data-action=submit]').click();
  assert.equal((await rejected).status(), 400);
  await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false' && Boolean(SillyTavern.getContext().chatMetadata.corerp_runtime.pending));
  const pending = await page.evaluate(() => structuredClone(SillyTavern.getContext().chatMetadata.corerp_runtime.pending));
  assert.equal(authoritySnapshot(), before, 'rejected wait changed world');

  // Commit a real fence, lose its reply, and keep the exact pending request.
  await browserContext.route('**/api/v1/rp/requests/retire', async route => {
    const response = await route.fetch();
    assert.equal(response.status(), 200);
    assert.equal((await response.json()).data.status, 'retired');
    await route.abort('failed');
  }, { times: 1 });
  await panel.locator('[data-action=retire]').click();
  await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false');
  assert.deepEqual(await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.pending), pending);

  // A delayed original request cannot start even though the client missed the
  // fence acknowledgement. Use a syntactically valid but now retired request.
  const late = await fetch('http://127.0.0.1:4198/api/v1/rp/actions/wait', {
    method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify(pending.body), redirect: 'error',
  });
  assert.equal(late.status, 409);
  assert.equal((await late.json()).error.code, 'REQUEST_RETIRED');

  // Even after a confirmed fence, failed host acknowledgement storage restores
  // the pending key in memory. Repeating retirement is safe and finally clears it.
  const failClear = async route => {
    const binding = route.request().postDataJSON().chat?.[0]?.chat_metadata?.corerp_runtime;
    if (binding?.session_id === pending.body.session_id && !binding.pending) {
      await route.fulfill({ status: 503, contentType: 'application/json', body: '{"error":"retirement-save-test"}' });
    } else await route.continue();
  };
  await browserContext.route('**/api/chats/save', failClear);
  await panel.locator('[data-action=retire]').click();
  await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false' && document.querySelector('#corerp-runtime [role=status]').textContent.includes('未保存'));
  assert.deepEqual(await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.pending), pending);
  await browserContext.unroute('**/api/chats/save', failClear);
  await panel.locator('[data-action=retire]').click();
  await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false' && !SillyTavern.getContext().chatMetadata.corerp_runtime.pending);
  assert.equal(authoritySnapshot(), before, 'retirement changed world');
  return { rejectedWait: true, lostFenceReplyRetained: true, lateRequestBlocked: true, failedAcknowledgementRetained: true, worldUnchanged: true };
}

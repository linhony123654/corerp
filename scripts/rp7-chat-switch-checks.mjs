import assert from 'node:assert/strict';

export async function checkRP7AcceptedWriteChatSwitch({ page, browserContext, token, cardAvatar, authoritySnapshot }) {
  const panel = page.locator('#corerp-runtime');
  const original = await page.evaluate(() => structuredClone(SillyTavern.getContext().chatMetadata.corerp_runtime));
  let release, sawCommit;
  const delivered = new Promise(resolve => { release = resolve; });
  const committed = new Promise(resolve => { sawCommit = resolve; });
  let afterCommit;
  const delayed = async route => {
    const response = await route.fetch();
    assert.equal(response.status(), 200);
    afterCommit = authoritySnapshot(); sawCommit();
    await delivered;
    await route.fulfill({ response });
  };
  await browserContext.route('**/api/v1/rp/turns/run', delayed, { times: 1 });
  try {
    await panel.locator('[name=operation]').selectOption('dialogue');
    await panel.locator('[name=command]').fill('旧聊天提交成功后，迟到结果不能污染新聊天。');
    await panel.locator('[data-action=submit]').click();
    await Promise.race([committed, new Promise((_, reject) => { const timer = setTimeout(() => reject(new Error('old-chat commit never reached boundary')), 15000); timer.unref(); })]);
    const pending = await page.evaluate(() => structuredClone(SillyTavern.getContext().chatMetadata.corerp_runtime.pending));
    assert.equal(pending.operation, 'dialogue');
    await page.evaluate(async name => {
      const ctx = SillyTavern.getContext();
      const response = await fetch('/api/characters/create', { method: 'POST', headers: ctx.getRequestHeaders(), body: JSON.stringify({ ch_name: name, description: 'New presentation chat only', first_mes: '此聊天未绑定世界。' }) });
      if (!response.ok) throw new Error('new-chat fixture failed');
      const avatar = await response.text(); await ctx.getCharacters();
      const id = SillyTavern.getContext().characters.findIndex(c => c.avatar === avatar);
      await SillyTavern.getContext().selectCharacterById(String(id));
    }, `CoreRP write switch ${Date.now()}`);
    release();
    await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false');
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    assert.deepEqual(await page.evaluate(() => ({ binding: SillyTavern.getContext().chatMetadata.corerp_runtime ?? null, prompt: SillyTavern.getContext().extensionPrompts.corerp_runtime?.value ?? '', token: document.querySelector('#corerp-runtime [name=token]').value, scene: document.querySelector('#corerp-runtime pre').textContent })), { binding: null, prompt: '', token: '', scene: '' });
    assert.equal(await page.evaluate(async () => (await import('/scripts/extensions.js')).runGenerationInterceptors([], 4096, 'normal')), false);
    await page.evaluate(async avatar => { const ctx = SillyTavern.getContext(); const id = ctx.characters.findIndex(c => c.avatar === avatar); await ctx.selectCharacterById(String(id)); }, cardAvatar);
    assert.equal(await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.session_id), original.session_id);
    assert.deepEqual(await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.pending), pending);
    if (!await panel.isVisible()) await page.locator('#extensions-settings-button').click();
    await panel.locator('[name=token]').fill(token); await panel.locator('[data-action=connect]').click();
    await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false');
    await panel.locator('[data-action=retry]').click();
    await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false' && !SillyTavern.getContext().chatMetadata.corerp_runtime.pending);
    assert.equal(authoritySnapshot(), afterCommit, 'switch/recovery repeated accepted world write');
    return { acceptedOldReplyIsolated: true, originalPendingRecovered: true, noDuplicateEffects: true };
  } finally { release(); await browserContext.unroute('**/api/v1/rp/turns/run', delayed); }
}

// Deliberately deliver a real old-chat response after selecting a different
// actual host card. No fake host global or alternate runtime is introduced.
export async function checkRP7ChatSwitch({ page, browserContext, token, cardAvatar }) {
  const panel = page.locator('#corerp-runtime');
  const original = await page.evaluate(() => structuredClone(SillyTavern.getContext().chatMetadata.corerp_runtime));
  let seenResolve, releaseResolve, finishedResolve;
  const seen = new Promise(resolve => { seenResolve = resolve; });
  const release = new Promise(resolve => { releaseResolve = resolve; });
  const finished = new Promise(resolve => { finishedResolve = resolve; });
  const routePattern = '**/api/v1/rp/context/read';
  const delayed = async route => {
    const response = await route.fetch();
    assert.equal(response.status(), 200);
    seenResolve();
    await release;
    try { await route.fulfill({ response }); }
    finally { finishedResolve(); }
  };
  await browserContext.route(routePattern, delayed, { times: 1 });
  await panel.locator('[data-action=refresh]').click();
  await Promise.race([seen, new Promise((_, reject) => {
    const timer = setTimeout(() => reject(new Error('old-context request did not arrive')), 10000);
    timer.unref();
  })]);
  try {
    await page.evaluate(async name => {
      const ctx = SillyTavern.getContext();
      const response = await fetch('/api/characters/create', { method: 'POST', headers: ctx.getRequestHeaders(), body: JSON.stringify({ ch_name: name, description: 'Unbound test presentation', first_mes: '这是另一个未绑定聊天。' }) });
      if (!response.ok) throw new Error(`create switch fixture ${response.status}`);
      const avatar = await response.text();
      await ctx.getCharacters();
      const id = SillyTavern.getContext().characters.findIndex(character => character.avatar === avatar);
      if (id < 0) throw new Error('switch card missing');
      await SillyTavern.getContext().selectCharacterById(String(id));
    }, `CoreRP unbound ${Date.now()}`);
    assert.equal(await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime), undefined);
    releaseResolve();
    await finished;
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    const isolated = await page.evaluate(() => ({
      prompt: SillyTavern.getContext().extensionPrompts.corerp_runtime?.value ?? '',
      token: document.querySelector('#corerp-runtime [name=token]').value,
      scene: document.querySelector('#corerp-runtime pre').textContent,
      bound: Boolean(SillyTavern.getContext().chatMetadata.corerp_runtime),
    }));
    assert.deepEqual(isolated, { prompt: '', token: '', scene: '', bound: false });
    const blocked = await page.evaluate(async () => (await import('/scripts/extensions.js')).runGenerationInterceptors([], 4096, 'normal'));
    assert.equal(blocked, false, 'unbound chat must not inherit the previous generation block');
    await page.evaluate(async avatar => {
      const ctx = SillyTavern.getContext();
      const id = ctx.characters.findIndex(character => character.avatar === avatar);
      if (id < 0) throw new Error('original card missing');
      await ctx.selectCharacterById(String(id));
    }, cardAvatar);
    assert.equal(await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.session_id), original.session_id);
    assert.equal(await page.evaluate(() => SillyTavern.getContext().extensionPrompts.corerp_runtime?.value ?? ''), '');
    if (!await panel.isVisible()) await page.locator('#extensions-settings-button').click();
    await panel.locator('[name=token]').fill(token);
    await panel.locator('[data-action=connect]').click();
    await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false' && SillyTavern.getContext().extensionPrompts.corerp_runtime?.value.includes('observer_entity_id'));
    return { delayedOldResponseDiscarded: true, unboundChatIsolated: true, originalBindingRecovered: true };
  } finally {
    releaseResolve();
    await browserContext.unroute(routePattern, delayed);
  }
}

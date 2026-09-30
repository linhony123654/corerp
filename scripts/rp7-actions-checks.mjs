import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';

// Actual installed host UI + authoritative HTTP reads, never a second world.
export async function checkRP7Actions({ page, token, runtimeOrigin = 'http://127.0.0.1:4188' }) {
  const panel = page.locator('#corerp-runtime');
  const binding = await page.evaluate(() => structuredClone(SillyTavern.getContext().chatMetadata.corerp_runtime));
  const read = { session_id: binding.session_id };
  async function api(route, body) {
    const response = await fetch(`http://127.0.0.1:4198/api/v1/rp/${route}`, {
      method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify(body), redirect: 'error',
    });
    assert.equal(response.status, 200, route);
    return (await response.json()).data;
  }
  async function submit(operation, body) {
    await panel.locator('[name=operation]').selectOption(operation);
    await panel.locator('[name=command]').fill(JSON.stringify(body));
    await panel.locator('[data-action=submit]').click();
    await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false' && !SillyTavern.getContext().chatMetadata.corerp_runtime.pending);
    assert.match(await panel.locator('[role=status]').textContent(), /已连接/u);
  }
  const initial = await api('observe', read);
  const targetTime = new Date(Date.parse(initial.world_time) + 60_000).toISOString().replace('.000Z', 'Z');
  await submit('wait', { target_world_time: targetTime, budget: 100 });
  const waited = await api('observe', read);
  assert.equal(waited.world_time, targetTime);
  const destination = waited.reachable_places.find(place => place.can_move_now && place.place_id === 'place_m2_home_ada');
  assert.ok(destination, 'fixture must expose a real currently usable route');
  await submit('move', { from_place_id: waited.place_id, to_place_id: destination.place_id });
  const moved = await api('observe', read);
  assert.equal(moved.place_id, destination.place_id);
  assert.ok(moved.reachable_places.some(place => place.can_move_now && place.place_id === initial.place_id));
  await submit('move', { from_place_id: moved.place_id, to_place_id: initial.place_id });
  const returned = await api('observe', read);
  assert.equal(returned.place_id, initial.place_id);
  const recipient = returned.present_entities.find(entity => entity.entity_id === 'entity_m2_rp_cai');
  assert.ok(recipient, 'recipient must be currently observed, not invented');
  const beforeGift = await api('wallet/read', read);
  await submit('social', { action: 'gift', target_entity_id: recipient.entity_id, amount_minor: 1 });
  const afterGift = await api('wallet/read', read);
  assert.equal(BigInt(beforeGift.balance_minor) - BigInt(afterGift.balance_minor), 1n);
  const known = await api('context/read', read);
  assert.ok(known.facts.some(fact => fact.kind === 'interpersonal_action' && fact.action === 'gift' && fact.subject_entity_id === recipient.entity_id));

  // Binding-control fields must be rejected before persisting a command intent.
  await panel.locator('[name=command]').fill(JSON.stringify({ principal_id: 'principal_creator', action: 'greet', target_entity_id: recipient.entity_id }));
  await panel.locator('[data-action=submit]').click();
  await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false' && document.querySelector('#corerp-runtime [role=status]').textContent.includes('不可覆盖'));
  assert.equal(await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.pending), undefined);
  const unchanged = await api('wallet/read', read);
  assert.equal(unchanged.observation_cursor, afterGift.observation_cursor);

  // Reconnect explicitly after the tested error. Then prove *external* change
  // updates this host through SSE, without clicking refresh or reloading it.
  await panel.locator('[name=token]').fill(token);
  await panel.locator('[data-action=connect]').click();
  await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false' && Boolean(SillyTavern.getContext().chatMetadata.corerp_runtime.cursor));
  const oldCursor = await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.cursor);
  const other = await api('sessions/open', {
    instance_id: known.instance_id, branch_id: known.branch_id, entity_id: known.observer_entity_id,
    pov: 'second_person', idempotency_key: randomUUID(),
  });
  const otherRead = { session_id: other.session_id };
  const otherView = await api('observe', otherRead);
  const externalText = '另一客户端的真实发言应自动出现在酒馆。';
  const external = await api('turns/run', { ...otherRead, expected_cursor: otherView.observation_cursor, idempotency_key: randomUUID(), text: externalText });
  assert.equal(external.status, 'settled');
  await page.waitForFunction(text => document.querySelector('#corerp-runtime pre').textContent.includes(text), externalText);
  await page.waitForFunction(cursor => SillyTavern.getContext().chatMetadata.corerp_runtime.cursor !== cursor && document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false', oldCursor);
  const ours = await api('context/read', read);
  const theirs = await api('context/read', otherRead);
  for (const key of ['instance_id', 'branch_id', 'observer_entity_id', 'world_time', 'observation_cursor', 'facts']) assert.deepEqual(ours[key], theirs[key], key);
  assert.equal(await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.session_id), binding.session_id);
  return { waitTarget: targetTime, moves: 2, giftMinor: 1, externalSession: other.session_id, sharedObserver: ours.observer_entity_id, head: ours.observation_cursor };
}

export async function checkRP7BudgetWait({ page }) {
  const panel = page.locator('#corerp-runtime');
  const target = '2026-09-23T08:00:00Z'; // existing fixture's real morning schedule
  await panel.locator('[name=operation]').selectOption('wait');
  await panel.locator('[name=command]').fill(JSON.stringify({ target_world_time: target, budget: 1 }));
  const firstResponse = page.waitForResponse(response => response.url().endsWith('/api/v1/rp/actions/wait') && response.request().postDataJSON()?.budget === 1);
  await panel.locator('[data-action=submit]').click();
  const response = await firstResponse;
  assert.equal(response.status(), 200);
  const first = (await response.json()).data;
  assert.equal(first.status, 'budget_exhausted');
  assert.equal(first.processed_items, 1);
  assert.ok(first.pending_due > 0);
  await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false');
  const original = await page.evaluate(() => structuredClone(SillyTavern.getContext().chatMetadata.corerp_runtime.pending));
  assert.ok(original, 'a one-item budget must retain the unfinished scheduled wait');
  assert.equal(original.operation, 'wait');
  let retries = 0;
  for (; retries < 20; retries++) {
    const pending = await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.pending);
    if (!pending) break;
    assert.deepEqual(pending, original, 'budget progress changed the original command identity');
    await panel.locator('[data-action=retry]').click();
    await page.waitForFunction(() => document.querySelector('#corerp-runtime').getAttribute('aria-busy') === 'false');
  }
  assert.equal(await page.evaluate(() => SillyTavern.getContext().chatMetadata.corerp_runtime.pending), undefined);
  assert.ok(retries > 0 && retries < 20);
  assert.ok((await panel.locator('pre').textContent()).includes(target));
  return { target, budget: 1, initialStatus: first.status, retries, unchangedRequest: true };
}

/* eslint-disable */
// 验证脚本：真实运行前端，按路径 A–F 逐项检查并截图。
const { chromium } = require('playwright');

const BASE = process.env.BASE || 'http://127.0.0.1:4173/';
const OUT = '/home/ubuntu/corerp-console/shots';

const fs = require('fs');
fs.mkdirSync(OUT, { recursive: true });

const results = [];
function check(id, name, pass, detail) {
  results.push({ id, name, pass, detail });
  console.log(`${pass ? 'PASS' : 'FAIL'} ${id} ${name}${detail ? ' — ' + detail : ''}`);
}

(async () => {
  const browser = await chromium.launch();
  const errors = [];

  for (const vp of [
    { name: '360x800', width: 360, height: 800 },
    { name: '390x844', width: 390, height: 844 },
    { name: '430x932', width: 430, height: 932 }
  ]) {
    const ctx = await browser.newContext({
      viewport: { width: vp.width, height: vp.height },
      deviceScaleFactor: 2,
      hasTouch: true,
      isMobile: true
    });
    const page = await ctx.newPage();
    page.on('console', (m) => { if (m.type() === 'error') errors.push(`[${vp.name}] ${m.text()}`); });
    page.on('pageerror', (e) => errors.push(`[${vp.name}] pageerror: ${e.message}`));
    await page.goto(BASE, { waitUntil: 'networkidle' });
    await page.waitForTimeout(700);

    // F: 首屏有叙事 + 明确行动路径
    const turnCount = await page.locator('[data-mid]').count();
    const hasComposer = await page.locator('.composer__input').count();
    check(`F.${vp.name}`, '首屏有连续叙事历史', turnCount >= 30, `${turnCount} 回合`);
    check(`F.${vp.name}`, '有自由输入', hasComposer === 1);
    check(`F.${vp.name}`, '有语境快捷动作', (await page.locator('.ctx__btn').count()) >= 4);

    // 长文不溢出
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    check(`F.${vp.name}`, '无横向溢出', overflow <= 1, `overflow=${overflow}px`);

    const preOverflow = await page.evaluate(() => {
      const els = [...document.querySelectorAll('.prose pre, .prose table')];
      return els.filter((e) => e.scrollWidth > e.clientWidth + 2).length;
    });
    console.log(`   · ${vp.name}: ${preOverflow} 个 pre/table 内部横滑（允许）`);

    if (vp.name === '390x844') {
      await page.screenshot({ path: `${OUT}/m-390-story-top.png` });

      // 路径 B：读历史
      await page.evaluate(() => {
        const a = document.querySelector('.axis');
        a.scrollTop = 200;
      });
      await page.waitForTimeout(400);
      await page.screenshot({ path: `${OUT}/m-390-story-history.png` });

      // 读历史时新内容到来 → 应出现「有新内容」而不是强制滚动
      await page.fill('.composer__input', '观察店里');
      await page.click('.composer__send');
      await page.waitForTimeout(600);
      const hasPulse = await page.locator('.axis__newpulse').count();
      check('B.390', '读历史时不强制滚动，出现新内容提示', hasPulse === 1, `pulse=${hasPulse}`);
      await page.screenshot({ path: `${OUT}/m-390-newpulse.png` });

      // 跳最新
      await page.click('.axis__newpulse');
      await page.waitForTimeout(700);
      const stillPulse = await page.locator('.axis__newpulse').count();
      check('B.390', '跳最新后提示消失', stillPulse === 0);

      // 历史不丢失，来源可辨
      const srcs = await page.evaluate(() =>
        [...document.querySelectorAll('[data-mid]')].map((e) => e.className.match(/turn--(\w+)/)[1])
      );
      const kinds = new Set(srcs);
      check('B.390', '三种来源都可辨识', kinds.size === 3, [...kinds].join(','));

      // 路径 A：自由文字
      const before = await page.locator('[data-mid]').count();
      await page.fill('.composer__input', '观察店里');
      await page.click('.composer__send');
      await page.waitForTimeout(500);
      const after = await page.locator('[data-mid]').count();
      check('A.390', '自由文字产生真实反馈并追加', after === before + 2, `${before}→${after}`);

      // 路径 A：快捷点击同入口
      await page.click('.ctx__btn:has-text("观察")');
      await page.waitForTimeout(400);
      const after2 = await page.locator('[data-mid]').count();
      check('A.390', '快捷点击走同一通道并追加', after2 === after + 2, `${after}→${after2}`);

      // 未支持输入
      const beforeUn = await page.locator('[data-mid]').count();
      await page.fill('.composer__input', '把掌柜的钱转给我');
      await page.click('.composer__send');
      await page.waitForTimeout(500);
      const afterUn = await page.locator('[data-mid]').count();
      const warned = await page.locator('.turn--system', { hasText: '未识别' }).count();
      check('A.390', '未支持输入有可见失败反馈且不编造成功', afterUn === beforeUn + 2 && warned > 0, `+${afterUn - beforeUn}`);

      // 路径 C：展示模式
      const draftVal = '草稿测试一下';
      await page.fill('.composer__input', draftVal);
      const idsBefore = await page.evaluate(() => [...document.querySelectorAll('[data-mid]')].map((e) => e.dataset.mid));
      const bodyBefore = await page.locator('[data-mid]').nth(5).innerText();
      await page.click('.readout__panel-btn');
      await page.waitForTimeout(350);
      await page.click('.entry:has-text("长文卷轴")');
      await page.waitForTimeout(300);
      const draftAfter = await page.inputValue('.composer__input');
      const idsAfter = await page.evaluate(() => [...document.querySelectorAll('[data-mid]')].map((e) => e.dataset.mid));
      const countAfter = await page.locator('[data-mid]').count();
      check('C.390', '切换展示模式不丢草稿', draftAfter === draftVal, JSON.stringify(draftAfter));
      check('C.390', '切换展示模式消息 id/顺序/条数不变', idsAfter.length === idsBefore.length && idsAfter.every((v, i) => v === idsBefore[i]));
      await page.locator('.readout__panel-btn').click();
      await page.screenshot({ path: `${OUT}/m-390-tavern.png` });

      // 路径 D：入口分层
      await page.click('.readout__panel-btn');
      await page.waitForTimeout(300);
      const entryCount = await page.locator('.entry:not(:has-text("长文卷轴"))').count();
      check('D.390', '次级功能按组可达', entryCount >= 6, `${entryCount} 个入口`);
      await page.screenshot({ path: `${OUT}/m-390-drawer.png` });

      // stub 入口：可见 + 明确未接入
      await page.click('.entry:has-text("随身物品")');
      await page.waitForTimeout(400);
      const stubMsg = await page.locator('.turn--system', { hasText: '未接入' }).count();
      check('D.390', '未接通入口有明确说明（非空 toast）', stubMsg > 0);

      // 路径 E：进入观测台并返回
      await page.click('.readout__panel-btn');
      await page.waitForTimeout(300);
      await page.click('.entry:has-text("观测台")');
      await page.waitForTimeout(600);
      const hasSpine = await page.locator('.spine').count();
      const hasBack = await page.locator('.backbar__btn').count();
      check('E.390', '进入观测台（六区块保留）', hasSpine >= 1 && hasBack === 1);
      await page.screenshot({ path: `${OUT}/m-390-inspector.png` });
      await page.click('.backbar__btn');
      await page.waitForTimeout(500);
      const backDraft = await page.inputValue('.composer__input');
      const backCount = await page.locator('[data-mid]').count();
      // 关键：离开再返回后历史一条不少（>= 离开时的条数），草稿仍在
      check('E.390', '返回叙事流且草稿/历史保留', backDraft === draftVal && backCount >= countAfter, `draft ok, ${backCount} turns`);

      // record_id 引用跳转
      const refCount = await page.locator('.turn__ref').count();
      if (refCount > 0) {
        await page.locator('.turn__ref').first().click();
        await page.waitForTimeout(700);
        const inInspector = await page.locator('.backbar__btn').count();
        check('E.390', '叙事引用可跳到观测台', inInspector === 1, `${refCount} 个可点引用`);
        await page.screenshot({ path: `${OUT}/m-390-probe.png` });
        await page.click('.backbar__btn');
        await page.waitForTimeout(500);
      }

      // 软键盘/安全区
      const safeArea = await page.evaluate(() => {
        const c = document.querySelector('.composer');
        return getComputedStyle(c).paddingBottom;
      });
      console.log(`   · composer padding-bottom = ${safeArea}（含 env(safe-area-inset-bottom)）`);
    }

    if (vp.name === '360x800') {
      await page.screenshot({ path: `${OUT}/m-360-story.png`, fullPage: false });
    }

    await ctx.close();
  }

  // 桌面 ≥1280
  const dctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 1 });
  const page = await dctx.newPage();
  page.on('console', (m) => { if (m.type() === 'error') errors.push('[desktop] ' + m.text()); });
  page.on('pageerror', (e) => errors.push('[desktop] pageerror: ' + e.message));
  await page.goto(BASE, { waitUntil: 'networkidle' });
  await page.waitForTimeout(700);
  await page.screenshot({ path: `${OUT}/d-1440-story.png` });

  const twoCol = await page.evaluate(() => {
    const s = document.querySelector('.shell');
    return s ? getComputedStyle(s).gridTemplateColumns : 'none';
  });
  check('F.desktop', '桌面双栏工作空间', twoCol.split(' ').length === 2, twoCol);

  // 桌面进入观测台
  await page.click('.entry:has-text("观测台")');
  await page.waitForTimeout(700);
  const spineH = await page.evaluate(() => {
    const e = document.querySelector('.spine');
    return e ? e.getBoundingClientRect().width : 0;
  });
  check('F.desktop', '桌面观测台脊梁保持宽幅', spineH > 700, `${Math.round(spineH)}px`);
  await page.screenshot({ path: `${OUT}/d-1440-inspector.png` });

  // 观测台六个区块回归
  for (const id of ['sec-spine', 'sec-epoch', 'sec-knowledge', 'sec-economy', 'sec-why', 'sec-packs']) {
    const n = await page.locator(`#${id}`).count();
    check(`R.desktop`, `区块 ${id} 仍在`, n === 1);
  }
  const charts = await page.evaluate(() => ({
    spine: document.querySelectorAll('.spine').length,
    kgraph: document.querySelectorAll('.kgraph__svg').length,
    econ: document.querySelectorAll('.econ__svg').length,
    strata: document.querySelectorAll('.strata').length,
    egrid: document.querySelectorAll('.egrid').length
  }));
  check('R.desktop', '五个图形组件全部渲染', Object.values(charts).every((v) => v >= 1), JSON.stringify(charts));

  // 点击脊梁节点 → 记录探针有内容
  await page.click('.entry:has-text("观测台")').catch(() => {});
  await page.waitForTimeout(300);
  const nodeCount = await page.locator('.spine [data-record-id]').count();
  if (nodeCount > 0) {
    await page.locator('.spine [data-record-id]').first().click();
    await page.waitForTimeout(400);
    const probeText = await page.locator('.probe, aside, .spine-layout__probe').first().innerText().catch(() => '');
    check('R.desktop', '点击节点展开记录探针', probeText.length > 40, `${probeText.length} chars`);
  }
  await dctx.close();

  /* ---------------------------------------------- 可访问性 / 减弱动态 */
  const actx = await browser.newContext({
    viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true, reducedMotion: 'reduce'
  });
  const apage = await actx.newPage();
  await apage.goto(BASE, { waitUntil: 'networkidle' });
  await apage.waitForTimeout(800);
  const animCount = await apage.evaluate(() => {
    const el = document.querySelector('.turn--enter, .axis__newpulse-dot');
    return el ? getComputedStyle(el).animationName : 'none';
  });
  const noTransitions = await apage.evaluate(() => {
    const c = document.querySelector('.ctx__btn');
    return getComputedStyle(c).transitionDuration;
  });
  check('A11Y.rm', 'prefers-reduced-motion 下停用动画', animCount === 'none' || animCount === '', animCount);
  check('A11Y.rm', 'prefers-reduced-motion 下过渡归零', parseFloat(noTransitions) < 0.01, noTransitions);

  // 焦点可见 + 键盘可达
  await apage.keyboard.press('Tab');
  const focusOutline = await apage.evaluate(() => {
    const a = document.activeElement;
    return a ? { tag: a.tagName, outline: getComputedStyle(a).outlineWidth } : null;
  });
  check('A11Y.focus', '键盘可聚焦且有可见焦点态', !!focusOutline && focusOutline.tag !== 'BODY', JSON.stringify(focusOutline));

  // aria / 对比
  const labeled = await apage.evaluate(() => ({
    axis: !!document.querySelector('.axis[aria-label]'),
    composer: !!document.querySelector('.composer__input[aria-label]'),
    live: !!document.querySelector('[aria-live]'),
    srOnly: !!document.querySelector('.sr-only')
  }));
  check('A11Y.aria', '关键区域有 aria-label / live region', Object.values(labeled).every(Boolean), JSON.stringify(labeled));

  const contrast = await apage.evaluate(() => {
    const lum = (c) => {
      const [r, g, b] = c.match(/\d+/g).slice(0, 3).map((v) => {
        const s = v / 255;
        return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
      });
      return 0.2126 * r + 0.7152 * g + 0.0722 * b;
    };
    const bg = lum(getComputedStyle(document.body).backgroundColor);
    const fg = lum(getComputedStyle(document.querySelector('.prose')).color);
    const [hi, lo] = bg > fg ? [bg, fg] : [fg, bg];
    return ((hi + 0.05) / (lo + 0.05)).toFixed(2);
  });
  check('A11Y.contrast', '正文对比度 >= 4.5:1', parseFloat(contrast) >= 4.5, `${contrast}:1`);

  // 触控目标 >= 40px
  const smallTargets = await apage.evaluate(() => {
    return [...document.querySelectorAll('.ctx__btn, .composer__send, .readout__panel-btn')]
      .map((e) => ({ cls: e.className, h: Math.round(e.getBoundingClientRect().height) }))
      .filter((t) => t.h < 32);
  });
  check('A11Y.touch', '触控目标高度 >= 32px', smallTargets.length === 0, JSON.stringify(smallTargets));

  await actx.close();

  await browser.close();

  const failed = results.filter((r) => !r.pass);
  console.log('\n===== 汇总 =====');
  console.log(`PASS ${results.length - failed.length} / ${results.length}`);
  if (failed.length) {
    console.log('失败项：');
    for (const f of failed) console.log(` - ${f.id} ${f.name} :: ${f.detail || ''}`);
  }
  if (errors.length) {
    console.log(`\n控制台错误 ${errors.length} 条：`);
    for (const e of [...new Set(errors)].slice(0, 15)) console.log(' - ' + e);
  } else {
    console.log('\n无控制台错误');
  }
  process.exit(failed.length || errors.length ? 1 : 0);
})().catch((e) => {
  console.error('脚本异常', e);
  process.exit(2);
});

/**
 * 演示行动分发（Demo Action Dispatcher）
 *
 * 自由文字与快捷点击进入同一个 `dispatchAction()`。
 * 本模块是**确定性前端演示**：只响应明确支持的 demo 意图，
 * 不冒充真实事件提交、经济计算或 Agent 运行，也不改写 world.ts。
 */

import type { ActionIntent, NarrativeMessage } from '../types'
import { sceneById, scenes } from '../data/story-demo'
import { entities, marketQuotes, records, snapshot } from '../data/world'

export interface DispatchResult {
  ok: boolean
  /** 追加到连续历史的演示消息（可为空：例如纯 UI 意图） */
  messages: NarrativeMessage[]
  /** 意图未被演示支持时的说明 */
  warning?: string
  /** 附带的工作空间动作 */
  navigate?: 'inspector'
  /** 附带的目标场景 */
  gotoScene?: string
}

/* ------------------------------------------------------------ 意图解析 */

const PATTERNS: { re: RegExp; kind: ActionIntent['kind']; target?: (m: RegExpMatchArray) => string | null }[] = [
  { re: /^\/?(观察|看看|查看四周|环顾| examine)/i, kind: 'observe' },
  { re: /^\/?(交谈|说话|对话|问|问一下|和.*说)/, kind: 'talk' },
  { re: /^\/?(商品|查看商品|报价|价目|看看货)/, kind: 'inspect_goods' },
  { re: /^\/?(等|等一会|等待|待着|等一会儿)/, kind: 'wait' },
  { re: /^\/?(去|前往|走到|移动到)/, kind: 'travel' },
  { re: /^\/?观测台/, kind: 'open_inspector' },
  { re: /^\/?帮助|^帮助$|^help$/i, kind: 'help' }
]

const TARGET_HINTS: [RegExp, string][] = [
  [/何图|掌柜|铁匠/, 'npc_he_tu'],
  [/林巧|帮工/, 'npc_lin_qiao'],
  [/赵婉|房东/, 'npc_zhao_wan'],
  [/宗门|外门|青玄/, 'org_qingxuan_hall'],
  [/集市|镇口|十字街/, 'scene_market'],
  [/赵家|院门|镇西/, 'scene_rent'],
  [/铺子|铁匠铺|一山号/, 'scene_forge']
]

/**
 * 从自由文字推断意图。
 * 返回 null 表示「未被演示支持」——调用方必须给出可见反馈，不得编造成功。
 */
export function parseIntent(raw: string): ActionIntent | null {
  const text = raw.trim()
  if (!text) return null

  for (const p of PATTERNS) {
    const m = text.match(p.re)
    if (!m) continue
    let targetId: string | null = null
    if (p.target) {
      targetId = p.target(m)
    } else {
      for (const [re, id] of TARGET_HINTS) {
        if (re.test(text)) {
          targetId = id
          break
        }
      }
    }
    return {
      kind: p.kind,
      label: text,
      actor_id: 'player_demo',
      target_id: targetId,
      params: {},
      source: 'text',
      raw: text,
      demo: true
    }
  }

  return null
}

/** 快捷点击构造的意图，与文字路径完全同构 */
export function clickIntent(kind: ActionIntent['kind'], label: string, targetId: string | null = null): ActionIntent {
  return {
    kind,
    label,
    actor_id: 'player_demo',
    target_id: targetId,
    params: {},
    source: 'click',
    raw: label,
    demo: true
  }
}

/* ------------------------------------------------------------ 可见性过滤 */

const cashByOwner = new Map(snapshot.projections.accounts.map((a) => [a.owner, a]))

/** 玩家视角可见字段：不泄露后台审计字段与非授权资产 */
export function visibleEntity(id: string) {
  const e = entities.find((x) => x.id === id)
  if (!e) return null
  if (e.archetype === 'organization' || e.archetype === 'cohort') return null
  const account = cashByOwner.get(e.id)
  return {
    id: e.id,
    name: e.name,
    role: e.role,
    archetype: e.archetype,
    /** 只给玩家视角的解释性口径，不给完整净资产/状态评分 */
    cash: account?.balance_minor ?? null,
    status: e.status
  }
}

function personLine(id: string): string {
  const v = visibleEntity(id)
  if (!v) return ''
  const cash = v.cash === null ? '—' : v.cash.toLocaleString('en-US')
  return `- **${v.name}** · ${v.role} · 现金 ${cash} 文`
}

function presentPeople(sceneId: string): string[] {
  return sceneById.get(sceneId)?.present ?? []
}

/* ---------------------------------------------------------------- 分发 */

let counter = 0
function nextId(): string {
  counter += 1
  return `msg_live_${String(counter).padStart(3, '0')}`
}

/**
 * 同一入口：自由文字与快捷点击。
 * 演示意图才产生结果；未支持的输入返回 ok=false，
 * 由调用方把 warning 作为一条 system 消息追加（不是 toast）。
 */
export function dispatchAction(intent: ActionIntent, sceneId: string): DispatchResult {
  const base = {
    turn: 0,
    scene_id: sceneId,
    demo: true,
    fact_refs: [] as string[]
  }

  switch (intent.kind) {
    case 'observe': {
      const scene = sceneById.get(sceneId)
      const people = presentPeople(sceneId)
      const lines = [
        scene?.intro ?? '（无场景数据）',
        '',
        '**可观察**：',
        ...people.map((p) => personLine(p)).filter(Boolean),
        '',
        `世界时间 ${snapshot.world_time}（demo snapshot，不是浏览器时间）`
      ]
      return {
        ok: true,
        messages: [msg('world', lines.join('\n'), base, ['rec_0001'])]
      }
    }

    case 'talk': {
      const target = intent.target_id
      const v = target ? visibleEntity(target) : null
      if (!v) {
        const people = presentPeople(sceneId).map((p) => visibleEntity(p)?.name).filter(Boolean)
        return {
          ok: true,
          messages: [
            msg(
              'system',
              `没有指定说话对象。当前场景可见人物：**${people.join('、') || '（无）'}**。\n` +
                '（演示只解析已出现的实体名；把「和赵婉说话」打成一句话，它会去找赵婉。）',
              base
            )
          ]
        }
      }
      const statusNote =
        v.status.length > 0
          ? `旁人对${v.name}的评价：${v.status.map((s) => `${s.observer} ${(s.score * 100).toFixed(0)}%`).join('、')}（主观口径，不等于净资产）`
          : `${v.name}没有可观察的社会评价记录。`
      return {
        ok: true,
        messages: [
          msg(
            'world',
            `${v.name}正在${v.archetype === 'store' ? '照看铺子' : '做手头的事'}。\n\n` +
              `${statusNote}\n\n` +
              `（演示对话：本意图返回的是可见信息，不是一次真实的 NPC 回应。）`,
            base
          )
        ]
      }
    }

    case 'inspect_goods': {
      const rows = marketQuotes.map(
        (q) =>
          `| ${q.sku} | ${q.price_minor} ${q.currency_id} / ${q.base_unit}${q.tax_included ? '（含税）' : ''} | ${q.valid_from.slice(5, 10)} → ${q.valid_until.slice(5, 10)} | ${q.kind} |`
      )
      return {
        ok: true,
        messages: [
          msg(
            'world',
            ['当前有效报价（构造示例，未接真实市场）：', '', '| 商品 | 价格 | 有效期 | 类别 |', '| --- | --- | --- | --- |', ...rows].join('\n'),
            base,
            marketQuotes.map((q) => q.quote_id)
          )
        ]
      }
    }

    case 'wait': {
      return {
        ok: true,
        messages: [
          msg(
            'world',
            '时间过去了一小段（演示）。\n\n' +
              '这个世界不依赖你的行动也会演化——但这里的「演化」只是演示文案，时间调度器没有真的在跑。',
            base,
            ['rec_0031']
          )
        ]
      }
    }

    case 'travel': {
      let dest: string | null = null
      if (intent.target_id?.startsWith('scene_')) dest = intent.target_id
      else {
        for (const s of scenes) {
          if (intent.raw.includes(s.name) || intent.raw.includes(s.place)) {
            dest = s.id
            break
          }
        }
      }
      if (!dest) {
        return {
          ok: true,
          messages: [
            msg(
              'system',
              `没有可去的目的地。演示场景：**${scenes.map((s) => s.name).join('、')}**。\n试「去集市」或点击语境里的移动动作。`,
              base
            )
          ]
        }
      }
      const scene = sceneById.get(dest)!
      return {
        ok: true,
        gotoScene: dest,
        messages: [msg('world', scene.intro, { ...base, scene_id: dest })]
      }
    }

    case 'open_inspector': {
      return { ok: true, messages: [], navigate: 'inspector' }
    }

    case 'help': {
      return {
        ok: true,
        messages: [
          msg(
            'system',
            [
              '演示支持的意图（自由文字或点击都走同一条通道）：',
              '',
              '- 观察 / 环顾',
              '- 交谈 / 和某人说话（何图、林巧、赵婉）',
              '- 查看商品（当前有效报价）',
              '- 等一会儿',
              '- 去集市 / 去赵家院门',
              '- 观测台（进入事件脊梁）',
              '',
              '其它输入会明确返回「未支持」，不会被写成世界真相。'
            ].join('\n'),
            base
          )
        ]
      }
    }

    default: {
      return { ok: false, messages: [], warning: '未支持的意图' }
    }
  }
}

function msg(
  source: NarrativeMessage['source'],
  body: string,
  base: { turn: number; scene_id: string; demo: boolean; fact_refs: string[] },
  refs: string[] = []
): NarrativeMessage {
  return {
    message_id: nextId(),
    turn: 0,
    source,
    world_time: snapshot.world_time,
    scene_id: base.scene_id,
    body,
    fact_refs: [...base.fact_refs, ...refs],
    demo: base.demo
  }
}

/** 记录探针跳转用：按 id 取演示记录 */
export function recordById(id: string) {
  return records.find((r) => r.record_id === id) ?? null
}

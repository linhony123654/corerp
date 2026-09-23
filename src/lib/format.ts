import type { RecordType, WorldRecord } from '../types'

export interface RecordKindStyle {
  /** 记录类型中文标签 */
  label: string
  /** 权威性中文标签 */
  authority: string
  /** 主色（CSS 变量名） */
  accent: string
  /** 在线之上 / 线下 */
  plane: 'above' | 'below'
  /** 单字标识，用于脊梁节点核心 */
  glyph: string
  description: string
}

const KINDS: Record<RecordType, RecordKindStyle> = {
  event: {
    label: '世界事件',
    authority: '权威',
    accent: 'var(--amber-400)',
    plane: 'above',
    glyph: '◆',
    description: '验证后原子提交的实际结果；持久状态只能由它改变。'
  },
  agent_decision: {
    label: 'Agent 决策',
    authority: '审计',
    accent: 'var(--teal-400)',
    plane: 'below',
    glyph: '◇',
    description: '目标、可见信息引用、候选、选择与来源；模型提案不具备事实效力。'
  },
  rule_validation: {
    label: '规则验证',
    authority: '审计',
    accent: 'var(--violet-400)',
    plane: 'below',
    glyph: '◈',
    description: '资格、检定、拒绝原因；非法输出拒绝，不写入事实。'
  },
  observation: {
    label: '观察记录',
    authority: '审计',
    accent: 'var(--lime-400)',
    plane: 'below',
    glyph: '○',
    description: '谁在何时通过什么渠道知道了什么；非事实写入接口。'
  },
  runtime_diagnostic: {
    label: '运行诊断',
    authority: '非权威',
    accent: 'var(--ink-600)',
    plane: 'below',
    glyph: '△',
    description: '异常、模型超时、性能、DLC 加载；不得当作权威历史。'
  },
  intervention: {
    label: '干预记录',
    authority: '审计',
    accent: 'var(--rose-400)',
    plane: 'below',
    glyph: '✦',
    description: '授权主体、策略版本、类型、目标、原因与结果事件。'
  }
}

export function kindStyle(r: WorldRecord): RecordKindStyle {
  return KINDS[r.record_type]
}export const allKinds = KINDS

export function fmtNum(n: number): string {
  return n.toLocaleString('en-US')
}

export function fmtTime(iso: string): string {
  return iso.replace('T', ' · ').slice(5, 16)
}

export function fmtFullTime(iso: string): string {
  return iso.replace('T', ' ')
}

export function shortHash(h: string): string {
  return h.length <= 12 ? h : `${h.slice(0, 6)}…${h.slice(-4)}`
}

export function money(n: number): string {
  const sign = n < 0 ? '−' : ''
  return `${sign}${fmtNum(Math.abs(n))}`
}

export function pct(n: number): string {
  return `${(n * 100).toFixed(0)}%`
}

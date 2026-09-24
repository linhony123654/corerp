export type WalletSnapshot = {
  balance_minor: string
  currency_id: string
  currency_symbol: string
  currency_scale: number
  world_time: string
  observation_cursor: number
}

// Never round a ledger amount through a JavaScript Number.
export function formatMinorAmount(minor: string, scale: number): string {
  if (typeof minor !== 'string' || !/^-?(0|[1-9]\d*)$/.test(minor) || !Number.isInteger(scale) || scale < 0 || scale > 18) {
    throw new Error('账户金额格式无效，请重新查看。')
  }
  const negative = minor.startsWith('-')
  const digits = (negative ? minor.slice(1) : minor).padStart(scale + 1, '0')
  const integer = scale ? digits.slice(0, -scale) : digits
  const fraction = scale ? `.${digits.slice(-scale)}` : ''
  return `${negative ? '−' : ''}${integer.replace(/\B(?=(\d{3})+(?!\d))/g, ',')}${fraction}`
}

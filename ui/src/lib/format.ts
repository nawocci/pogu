export function prettyDate(value: string | null | undefined): string {
  return value ? new Date(value).toLocaleString() : 'Never'
}

export function providerTypeLabel(type: string): string {
  if (type === 'anthropic') return 'Anthropic';
  if (type === 'openai-responses') return 'OpenAI Responses';
  return 'OpenAI';
}

export function keySelectionLabel(sel: string): string {
  return sel === 'round_robin' ? 'round robin' : 'primary first'
}

const compactFmt = new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 })
const intFmt = new Intl.NumberFormat('en')

export function int(n: number): string {
  return intFmt.format(n)
}

export function compact(n: number | null | undefined): string {
  if (n === null || n === undefined) return '—'
  return compactFmt.format(n)
}

export function latency(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return '—'
  if (ms < 1000) return `${Math.round(ms)}ms`
  if (ms < 60_000) {
    const s = ms / 1000
    return `${s >= 10 ? Math.round(s) : s.toFixed(2).replace(/0$/, '')}s`
  }
  const m = Math.floor(ms / 60_000)
  const s = Math.round((ms % 60_000) / 1000)
  return `${m}m ${String(s).padStart(2, '0')}s`
}

export function pct(rate: number | null | undefined): string {
  if (rate === null || rate === undefined) return '—'
  return `${(rate * 100).toFixed(1)}%`
}

export function delta(cur: number | null | undefined, prev: number | null | undefined): string {
  if (!cur || !prev || prev <= 0) return ''
  const d = ((cur - prev) / prev) * 100
  if (!isFinite(d) || Math.abs(d) < 0.05) return '±0%'
  return `${d > 0 ? '+' : ''}${d >= 100 ? Math.round(d) : d.toFixed(1)}%`
}

export function clockTime(ts: string | number | Date, withDate = false): string {
  const d = new Date(ts)
  if (withDate) {
    return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  }
  return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

export function fullTime(ts: string | number | Date): string {
  return new Date(ts).toLocaleString()
}

export function ago(ts: string | number | Date, now = Date.now()): string {
  const s = Math.max(0, (now - new Date(ts).getTime()) / 1000)
  if (s < 8) return 'now'
  if (s < 60) return `${Math.floor(s)}s`
  if (s < 3600) return `${Math.floor(s / 60)}m`
  if (s < 86400) return `${Math.floor(s / 3600)}h`
  return `${Math.floor(s / 86400)}d`
}

export function bucketLabel(t: string | Date, interval: string): string {
  const d = new Date(t)
  if (interval === 'hour') return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
  if (interval === 'month') return d.toLocaleDateString(undefined, { month: 'short' })
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

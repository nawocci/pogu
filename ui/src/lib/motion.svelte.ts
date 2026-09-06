import { cubicOut } from 'svelte/easing'

export const reducedMotion = (() => {
  let current = $state(false)
  if (typeof window !== 'undefined' && 'matchMedia' in window) {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    current = mq.matches
    mq.addEventListener('change', (e) => (current = e.matches))
  }
  return { get current() { return current } }
})()

export function blipFade(_node: Element, { duration = 260 }: { duration?: number } = {}) {
  const durMs = reducedMotion.current ? 0 : duration
  return {
    duration: durMs,
    easing: cubicOut,
    css: (t: number, u: number) => `opacity: ${t}; transform: translateY(${7 * u}px) scale(${1 - 0.02 * u});`,
  }
}

export function quickFade(_node: Element, { duration = 140 }: { duration?: number } = {}) {
  const durMs = reducedMotion.current ? 0 : duration
  return {
    duration: durMs,
    easing: cubicOut,
    css: (t: number) => `opacity: ${t};`,
  }
}

export type RevealKind = 'rise' | 'blip' | 'pop'

const REVEAL_STEP_MS: Record<RevealKind, number> = { rise: 90, blip: 65, pop: 45 }
const REVEAL_CAP: Record<RevealKind, number> = { rise: 5, blip: 9, pop: 11 }

export interface RevealParams {
  kind?: RevealKind
  i?: number
}

export function reveal(node: HTMLElement, params: RevealParams = {}) {
  const kind = params.kind ?? 'blip'
  if (typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) {
    return {}
  }
  const delay = Math.round(Math.min(params.i ?? 0, REVEAL_CAP[kind]) * REVEAL_STEP_MS[kind])
  const cls = `rv-${kind}`
  node.classList.add(cls)
  node.style.setProperty('--rvd', `${delay}ms`)
  let done = false
  const finish = () => {
    if (done) return
    done = true
    node.classList.remove('rv-rise', 'rv-blip', 'rv-pop')
    node.style.removeProperty('--rvd')
  }
  node.addEventListener('animationend', finish, { once: true })
  const t = setTimeout(finish, delay + 1600)
  return {
    destroy() {
      clearTimeout(t)
      finish()
    },
  }
}

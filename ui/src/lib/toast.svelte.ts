export type ToastKind = 'success' | 'warn' | 'error'

export interface ToastAction {
  label: string
  href: string
}

export interface Toast {
  id: number
  kind: ToastKind
  message: string
  action?: ToastAction
}

let nextId = 1
const timers = new Map<number, ReturnType<typeof setTimeout>>()

export const MAX_TOASTS = 4

export const toasts = $state<{ items: Toast[] }>({ items: [] })

function defaultTimeout(kind: ToastKind): number {
  return kind === 'success' ? 4000 : kind === 'warn' ? 6500 : 8000
}

export function toast(
  kind: ToastKind,
  message: string,
  opts?: { timeoutMs?: number; action?: ToastAction },
): number {
  const id = nextId++
  toasts.items.push({ id, kind, message, action: opts?.action })
  while (toasts.items.length > MAX_TOASTS) {
    const oldest = toasts.items[0]
    if (oldest === undefined || oldest.id === id) break
    dismiss(oldest.id)
  }
  arm(id, opts?.timeoutMs ?? defaultTimeout(kind))
  return id
}

function arm(id: number, ms: number) {
  if (ms <= 0) return
  timers.set(
    id,
    setTimeout(() => dismiss(id), ms),
  )
}

export function dismiss(id: number) {
  const i = toasts.items.findIndex((t) => t.id === id)
  if (i >= 0) toasts.items.splice(i, 1)
  const t = timers.get(id)
  if (t) {
    clearTimeout(t)
    timers.delete(id)
  }
}

export function hold(id: number) {
  const t = timers.get(id)
  if (t) {
    clearTimeout(t)
    timers.delete(id)
  }
}

export function resume(id: number) {
  arm(id, 2500)
}

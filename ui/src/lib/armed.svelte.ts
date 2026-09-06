export function createArmed(timeoutMs = 2500) {
  let armed = $state<string | null>(null)
  let timer: ReturnType<typeof setTimeout> | undefined

  function is(id: string): boolean {
    return armed === id
  }

  function confirm(id: string, act: () => void): void {
    const fire = armed === id
    clearTimeout(timer)
    if (fire) {
      armed = null
      act()
      return
    }
    armed = id
    timer = setTimeout(() => (armed = null), timeoutMs)
  }

  return { is, confirm }
}

<script lang="ts">
  import { toast } from '../toast.svelte'
  import Fit from './Fit.svelte'

  let { value, label = 'Copy', silent = false }: { value: string; label?: string; silent?: boolean } = $props()

  let copied = $state(false)
  let timer: ReturnType<typeof setTimeout>

  async function copy() {
    try {
      await navigator.clipboard.writeText(value)
    } catch {
      const ta = document.createElement('textarea')
      ta.value = value
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      ta.remove()
    }
    copied = true
    clearTimeout(timer)
    timer = setTimeout(() => (copied = false), 1600)
    if (!silent) toast('success', 'Copied to clipboard')
  }
</script>

<button
  class="btn-ghost {copied ? 'btn-ghost-ok' : ''}"
  onclick={copy}
  aria-label={copied ? 'Copied' : `Copy ${label}`}
>
  <Fit text={copied ? 'Copied' : label} wide="Copied" />
</button>

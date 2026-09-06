<script lang="ts">
  import Fit from './Fit.svelte'
  import { fade } from 'svelte/transition'
  import { reducedMotion } from '../motion.svelte'

  let { state = 'off', label, wide = '' }: { state?: 'on' | 'off' | 'err' | 'live'; label?: string; wide?: string } = $props()

  const pill: Record<string, string> = {
    on: 'text-success-muted border-success-soft bg-success-soft',
    off: 'text-tertiary border-line-soft bg-raised',
    err: 'text-error-muted border-error-soft bg-error-soft',
    live: 'text-accent-ink border-accent-dim bg-accent-dim',
  }

  const dot: Record<string, string> = {
    on: 'bg-success shadow-[0_0_6px_var(--success-soft)]',
    off: 'bg-muted',
    err: 'bg-error shadow-[0_0_6px_var(--error-soft)]',
    live: 'bg-accent animate-pulse-dot',
  }
</script>

{#if label}
  <span
    class="inline-flex items-center gap-[7px] rounded-full border border-transparent pt-0.5 pb-0.5 pr-2.5 pl-2 font-mono text-[10px] leading-[1.5] font-medium tracking-[0.07em] whitespace-nowrap uppercase {pill[state]}"
  >
    <span class="size-[7px] shrink-0 rounded-full {dot[state]}" aria-hidden="true"></span>
    <span class="grid min-w-0">
      {#key state}
        <span
          class="col-start-1 row-start-1"
          in:fade|local={{ duration: reducedMotion.current ? 0 : 180 }}
          out:fade|local={{ duration: reducedMotion.current ? 0 : 120 }}
        >
          <Fit text={label} {wide} />
        </span>
      {/key}
    </span>
  </span>
{:else}
  <span class="inline-block size-[7px] shrink-0 rounded-full {dot[state]}"></span>
{/if}

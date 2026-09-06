<script lang="ts">
  import { fly, fade } from 'svelte/transition';
  import { flip } from 'svelte/animate';
  import { cubicOut } from 'svelte/easing';
  import { toasts, dismiss, hold, resume } from '../toast.svelte';
  import { reducedMotion } from '../motion.svelte';

  const dotColor: Record<string, string> = {
    success: 'bg-success-muted shadow-[0_0_8px_var(--success-soft)]',
    warn: 'bg-warning-muted shadow-[0_0_8px_var(--warning-soft)]',
    error: 'bg-error-muted shadow-[0_0_8px_var(--error-soft)]',
  };
</script>

<div
  class="pointer-events-none absolute right-5 bottom-5 z-60 flex max-h-[min(60dvh,480px)] w-[min(380px,calc(100%-40px))] flex-col justify-end gap-2.5 overflow-hidden max-compact:fixed max-compact:right-3.5 max-compact:bottom-3.5"
  aria-live="polite"
>
  {#each toasts.items as t (t.id)}
    <div
      in:fly={{ y: 10, duration: reducedMotion.current ? 0 : 240, easing: cubicOut }}
      out:fade={{ duration: reducedMotion.current ? 0 : 140 }}
      animate:flip={{ duration: reducedMotion.current ? 0 : 200 }}
      class="pointer-events-auto flex items-start gap-3 rounded-sm border border-line border-l-[3px] bg-raised-2 py-3 pr-3 pl-4 shadow-toast"
      role={t.kind === 'error' ? 'alert' : 'status'}
      onmouseenter={() => hold(t.id)}
      onmouseleave={() => resume(t.id)}
    >
      <span class="mt-1 size-[9px] shrink-0 rounded-full {dotColor[t.kind]}" aria-hidden="true"></span>
      <p class="m-0 flex-1 text-[13px] leading-[1.5] text-primary [overflow-wrap:anywhere]">{t.message}</p>
      {#if t.action}
        <a
          class="mt-px shrink-0 self-center rounded-sm border border-accent-dim bg-accent-dim px-2 py-[3px] font-mono text-[10px] font-medium tracking-[0.07em] text-accent-ink uppercase transition-colors hover:border-accent hover:bg-accent hover:text-accent-on"
          href={t.action.href}
          onclick={() => dismiss(t.id)}
        >
          {t.action.label}
        </a>
      {/if}
      <button
        class="shrink-0 rounded-sm p-1 px-1.5 text-sm leading-none text-muted opacity-70 transition-[opacity,color] hover:bg-hover hover:text-primary hover:opacity-100"
        onclick={() => dismiss(t.id)}
        aria-label="Dismiss"
      >
        ✕
      </button>
    </div>
  {/each}
</div>

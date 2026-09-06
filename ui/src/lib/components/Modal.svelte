<script lang="ts">
  import type { Snippet } from 'svelte'
  import { reducedMotion } from '../motion.svelte'

  let {
    title,
    onclose,
    children,
    wide = false,
  }: { title: string; onclose: () => void; children: Snippet; wide?: boolean } = $props()

  let dlg = $state<HTMLDialogElement>()
  let closing = $state(false)
  let safety: ReturnType<typeof setTimeout> | undefined

  $effect(() => {
    dlg?.showModal()
    return () => clearTimeout(safety)
  })

  function beginClose() {
    if (closing || !dlg) return
    closing = true
    if (reducedMotion.current) return dlg.close()
    dlg.classList.add('closing')
    const onEnd = (e: AnimationEvent) => {
      if (!/modal-out|backdrop-out/.test(e.animationName)) return
      dlg?.close()
    }
    dlg.addEventListener('animationend', onEnd)
    safety = setTimeout(() => dlg?.close(), 300)
  }

  function oncancel(e: Event) {
    e.preventDefault()
    beginClose()
  }
</script>

<dialog bind:this={dlg} class:wide closedby="any" oncancel={oncancel} onclose={onclose}>
  <header class="flex items-center justify-between border-b border-line-soft px-[22px] py-[18px]">
    <h2 class="text-xl tracking-[-0.01em]">{title}</h2>
    <button
      class="rounded-sm p-1 px-2 text-xl leading-none text-muted opacity-70 transition-[opacity,color] hover:bg-hover hover:text-primary hover:opacity-100"
      onclick={beginClose}
      aria-label="Close"
    >
      ×
    </button>
  </header>
  <div class="p-[22px]">
    {@render children()}
  </div>
</dialog>

<style>
  dialog {
    background: var(--raised);
    color: var(--primary);
    border: 1px solid var(--line);
    border-radius: 6px;
    padding: 0;
    position: fixed;
    inset: 0;
    margin: auto;
    height: fit-content;
    width: min(480px, calc(100vw - 32px));
    box-shadow: var(--sh-pop);
  }

  dialog.wide {
    width: min(640px, calc(100vw - 32px));
  }

  dialog::backdrop {
    background: var(--backdrop);
    animation: backdrop-in 0.18s ease-out both;
  }

  dialog[open] {
    animation: modal-in 0.26s cubic-bezier(0.22, 1, 0.36, 1) both;
  }

  :global(dialog.closing[open]) {
    animation: modal-out 0.13s cubic-bezier(0.55, 0.06, 0.68, 0.19) forwards;
    pointer-events: none;
  }

  :global(dialog.closing::backdrop) {
    animation: backdrop-out 0.13s ease-in forwards;
  }

  @keyframes modal-in {
    from {
      opacity: 0;
      transform: translateY(12px) scale(0.97);
    }
  }

  @keyframes backdrop-in {
    from {
      opacity: 0;
    }
  }

  @keyframes modal-out {
    to {
      opacity: 0;
      transform: translateY(8px) scale(0.98);
    }
  }

  @keyframes backdrop-out {
    to {
      opacity: 0;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    dialog[open],
    dialog::backdrop {
      animation-duration: 0.01ms;
    }

    :global(dialog.closing),
    :global(dialog.closing::backdrop) {
      animation-duration: 0.01ms;
    }
  }
</style>

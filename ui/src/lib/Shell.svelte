<script lang="ts">
  import { api } from './api';
  import { store } from './store.svelte';
  import { router, navigate } from './router.svelte';
  import { setAuthenticated } from './authMorph.svelte';
  import { theme, toggleTheme } from './theme.svelte';
  import { reveal, quickFade } from './motion.svelte';
  import { fade } from 'svelte/transition';
  import { createArmed } from './armed.svelte';
  import Toasts from './components/Toasts.svelte';
  import Icon from './components/Icon.svelte';
  import type { Snippet } from 'svelte';

  let { children }: { children: Snippet } = $props();

  const armed = createArmed();

  let expanded = $state(true);

  const isConnections = $derived(router.route.name === 'connections');
  const isProviders = $derived(router.route.name === 'providers' || router.route.name === 'provider-detail');

  async function signOut() {
    try {
      await api.logout();
    } finally {
      navigate('/login', { replace: true });
      setAuthenticated(false);
    }
  }
</script>

<div
  class="relative grid h-full w-full grid-cols-[var(--rail-w)_minmax(0,1fr)] overflow-hidden transition-[grid-template-columns] duration-300 ease-out max-compact:grid-cols-1 max-compact:content-start max-compact:transition-none rail-collapsed:duration-200 rail-collapsed:grid-cols-[var(--rail-w-compact)_minmax(0,1fr)]"
  class:rail-collapsed={!expanded}
>
  <nav
    aria-label="Primary"
    class="flex min-w-0 flex-col border-r border-line bg-rail pb-4 max-compact:flex-row max-compact:items-center max-compact:gap-4 max-compact:border-r-0 max-compact:border-b max-compact:px-4 max-compact:py-3"
  >
    <a
      use:reveal={{ kind: 'blip', i: 0 }}
      class="flex h-[72px] shrink-0 items-center justify-center overflow-hidden no-underline max-compact:h-auto max-compact:p-0 max-compact:py-1"
      href="/"
      aria-label="pogu home"
    >
      {#if expanded}
        <span class="brand-mark" in:fade={{ duration: 180 }}>POGU</span>
      {:else}
        <span class="brand-mark brand-mark-p" in:fade={{ duration: 180 }}>P</span>
      {/if}
    </a>
    <div class="mx-4 border-t border-line-soft max-compact:hidden" aria-hidden="true"></div>

    <div class="mt-3 flex flex-col gap-0.5 px-2.5 max-compact:mt-0 max-compact:flex-row max-compact:p-0">
      <a
        href="/"
        class="nav-item {isConnections ? 'nav-item-active' : ''}"
        aria-current={isConnections ? 'page' : undefined}
        title="Connections"
      >
        <span class="flex min-w-0 items-center gap-2.5">
          <span class="grid size-5 shrink-0 place-items-center"><Icon name="connections" size={18} /></span>
          {#if expanded}<span class="truncate" in:quickFade={{ duration: 160 }}>Connections</span>{/if}
        </span>
      </a>
      <a
        href="/providers"
        class="nav-item {isProviders ? 'nav-item-active' : ''}"
        aria-current={isProviders ? 'page' : undefined}
        title="Providers"
      >
        <span class="flex min-w-0 items-center gap-2.5">
          <span class="grid size-5 shrink-0 place-items-center"><Icon name="providers" size={18} /></span>
          {#if expanded}<span class="truncate" in:quickFade={{ duration: 160 }}>Providers</span>{/if}
        </span>
        {#if expanded}
          <span class="count rounded-full bg-well px-[7px] py-px font-mono text-[10.5px] text-tertiary">{store.providers.length}</span>
        {/if}
      </a>
    </div>

    <footer class="mt-auto flex flex-col gap-0.5 px-2.5 pb-1 max-compact:hidden">
      <button
        class="nav-item {!expanded ? 'justify-center' : ''}"
        onclick={toggleTheme}
        aria-label="Switch color theme"
        title={theme() === 'light' ? 'Switch to dark' : 'Switch to light'}
      >
        <span class="flex min-w-0 items-center gap-2.5">
          <span class="grid size-5 shrink-0 place-items-center"><Icon name={theme() === 'light' ? 'moon' : 'sun'} size={18} /></span>
          {#if expanded}<span class="truncate" in:quickFade={{ duration: 160 }}>{theme() === 'light' ? 'Dark mode' : 'Light mode'}</span>{/if}
        </span>
      </button>
      <button
        class="nav-item linkish-del {!expanded ? 'justify-center' : ''}"
        onclick={() => armed.confirm('logout', signOut)}
        aria-label={armed.is('logout') ? 'Confirm sign out' : 'Sign out'}
        title={armed.is('logout') ? 'Confirm sign out' : 'Sign out'}
      >
        <span class="flex min-w-0 items-center gap-2.5">
          <span class="grid size-5 shrink-0 place-items-center"><Icon name="signout" size={18} /></span>
          {#if expanded}<span class="truncate" in:quickFade={{ duration: 160 }}>{armed.is('logout') ? 'Confirm?' : 'Sign out'}</span>{/if}
        </span>
      </button>
      <div class="mx-2 my-1.5 border-t border-line-soft" aria-hidden="true"></div>
      <button
        class="nav-item {!expanded ? 'justify-center' : ''}"
        onclick={() => (expanded = !expanded)}
        aria-label={expanded ? 'Collapse sidebar' : 'Expand sidebar'}
        aria-expanded={expanded}
        title={expanded ? 'Collapse sidebar' : 'Expand sidebar'}
      >
        <span class="flex min-w-0 items-center gap-2.5">
          <span class="grid size-5 shrink-0 place-items-center"><Icon name={expanded ? 'collapse' : 'expand'} size={18} /></span>
          {#if expanded}<span class="truncate" in:quickFade={{ duration: 160 }}>Collapse</span>{/if}
        </span>
      </button>
    </footer>
  </nav>

  <main class="min-w-0 overflow-y-auto px-(--content-x) py-12 [scrollbar-gutter:stable] [--content-x:clamp(24px,4vw,64px)] max-compact:overflow-visible max-compact:px-4 max-compact:py-7">
    {@render children()}
  </main>

  <Toasts />
</div>

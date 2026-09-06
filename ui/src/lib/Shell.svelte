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
  const isGroups = $derived(router.route.name === 'groups' || router.route.name === 'group-detail');
  const isMonitoring = $derived(router.route.name === 'monitoring');
  const isConfigurations = $derived(router.route.name === 'configurations');

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
    class="flex min-w-0 flex-col border-r border-line bg-rail pb-4 max-compact:hidden"
  >
    <a
      use:reveal={{ kind: 'blip', i: 0 }}
      class="flex h-[72px] shrink-0 items-center justify-center overflow-hidden no-underline"
      href="/"
      aria-label="pogu home"
    >
      {#if expanded}
        <span class="brand-mark" in:fade={{ duration: 180 }}>POGU</span>
      {:else}
        <span class="brand-mark brand-mark-p" in:fade={{ duration: 180 }}>P</span>
      {/if}
    </a>
    <div class="mx-4 border-t border-line-soft" aria-hidden="true"></div>

    <div class="mt-3 flex flex-col gap-0.5 px-2.5">
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
      <a
        href="/groups"
        class="nav-item {isGroups ? 'nav-item-active' : ''}"
        aria-current={isGroups ? 'page' : undefined}
        title="Groups"
      >
        <span class="flex min-w-0 items-center gap-2.5">
          <span class="grid size-5 shrink-0 place-items-center"><Icon name="groups" size={18} /></span>
          {#if expanded}<span class="truncate" in:quickFade={{ duration: 160 }}>Groups</span>{/if}
        </span>
        {#if expanded}
          <span class="count rounded-full bg-well px-[7px] py-px font-mono text-[10.5px] text-tertiary">{store.groups.length}</span>
        {/if}
      </a>
      <a
        href="/monitoring"
        class="nav-item {isMonitoring ? 'nav-item-active' : ''}"
        aria-current={isMonitoring ? 'page' : undefined}
        title="Monitoring"
      >
        <span class="flex min-w-0 items-center gap-2.5">
          <span class="grid size-5 shrink-0 place-items-center"><Icon name="monitoring" size={18} /></span>
          {#if expanded}<span class="truncate" in:quickFade={{ duration: 160 }}>Monitoring</span>{/if}
        </span>
      </a>
      <a
        href="/configurations"
        class="nav-item {isConfigurations ? 'nav-item-active' : ''}"
        aria-current={isConfigurations ? 'page' : undefined}
        title="Configurations"
      >
        <span class="flex min-w-0 items-center gap-2.5">
          <span class="grid size-5 shrink-0 place-items-center"><Icon name="configurations" size={18} /></span>
          {#if expanded}<span class="truncate" in:quickFade={{ duration: 160 }}>Configurations</span>{/if}
        </span>
      </a>
    </div>

    <footer class="mt-auto flex flex-col gap-0.5 px-2.5 pb-1">
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

  <header class="hidden border-b border-line bg-rail px-4 pt-[env(safe-area-inset-top)] max-compact:block max-compact:col-span-full">
    <div class="flex items-center gap-2 py-3">
      <a class="flex shrink-0 items-center no-underline" href="/" aria-label="pogu home">
        <span class="brand-mark">POGU</span>
      </a>
      <span class="flex-1" aria-hidden="true"></span>
      <button
        class="grid size-10 shrink-0 place-items-center rounded-sm text-tertiary transition-colors hover:bg-hover hover:text-primary"
        onclick={toggleTheme}
        aria-label="Switch color theme"
        title={theme() === 'light' ? 'Switch to dark' : 'Switch to light'}
      >
        <Icon name={theme() === 'light' ? 'moon' : 'sun'} size={18} />
      </button>
      {#if armed.is('logout')}
        <button
          class="btn-ghost shrink-0 border-error-soft text-error-muted"
          onclick={() => armed.confirm('logout', signOut)}
          aria-label="Confirm sign out"
          title="Confirm sign out"
        >
          Confirm?
        </button>
      {:else}
        <button
          class="grid size-10 shrink-0 place-items-center rounded-sm text-tertiary transition-colors hover:bg-hover hover:text-primary"
          onclick={() => armed.confirm('logout', signOut)}
          aria-label="Sign out"
          title="Sign out"
        >
          <Icon name="signout" size={18} />
        </button>
      {/if}
    </div>
  </header>

  <main class="min-w-0 overflow-y-auto px-(--content-x) py-12 [scrollbar-gutter:stable] [--content-x:clamp(24px,4vw,64px)] max-compact:overflow-visible max-compact:px-4 max-compact:pt-7 max-compact:pb-[calc(5.5rem+env(safe-area-inset-bottom))] max-compact:col-span-full">
    {@render children()}
  </main>

  <nav
    aria-label="Primary"
    class="hidden max-compact:flex fixed inset-x-0 bottom-0 z-40 border-t border-line bg-rail px-2 pb-[env(safe-area-inset-bottom)]"
  >
    <a
      href="/"
      class="nav-item {isConnections ? 'nav-item-active' : ''} max-compact:flex-1 max-compact:flex-col max-compact:items-center max-compact:justify-center max-compact:gap-0 max-compact:rounded-none max-compact:px-1 max-compact:py-3"
      aria-current={isConnections ? 'page' : undefined}
      aria-label="Connections"
      title="Connections"
    >
      <span class="grid size-5 place-items-center"><Icon name="connections" size={20} /></span>
    </a>
    <a
      href="/providers"
      class="nav-item {isProviders ? 'nav-item-active' : ''} max-compact:flex-1 max-compact:flex-col max-compact:items-center max-compact:justify-center max-compact:gap-0 max-compact:rounded-none max-compact:px-1 max-compact:py-3"
      aria-current={isProviders ? 'page' : undefined}
      aria-label="Providers"
      title="Providers"
    >
      <span class="grid size-5 place-items-center"><Icon name="providers" size={20} /></span>
    </a>
    <a
      href="/groups"
      class="nav-item {isGroups ? 'nav-item-active' : ''} max-compact:flex-1 max-compact:flex-col max-compact:items-center max-compact:justify-center max-compact:gap-0 max-compact:rounded-none max-compact:px-1 max-compact:py-3"
      aria-current={isGroups ? 'page' : undefined}
      aria-label="Groups"
      title="Groups"
    >
      <span class="grid size-5 place-items-center"><Icon name="groups" size={20} /></span>
    </a>
    <a
      href="/monitoring"
      class="nav-item {isMonitoring ? 'nav-item-active' : ''} max-compact:flex-1 max-compact:flex-col max-compact:items-center max-compact:justify-center max-compact:gap-0 max-compact:rounded-none max-compact:px-1 max-compact:py-3"
      aria-current={isMonitoring ? 'page' : undefined}
      aria-label="Monitoring"
      title="Monitoring"
    >
      <span class="grid size-5 place-items-center"><Icon name="monitoring" size={20} /></span>
    </a>
    <a
      href="/configurations"
      class="nav-item {isConfigurations ? 'nav-item-active' : ''} max-compact:flex-1 max-compact:flex-col max-compact:items-center max-compact:justify-center max-compact:gap-0 max-compact:rounded-none max-compact:px-1 max-compact:py-3"
      aria-current={isConfigurations ? 'page' : undefined}
      aria-label="Configurations"
      title="Configurations"
    >
      <span class="grid size-5 place-items-center"><Icon name="configurations" size={20} /></span>
    </a>
  </nav>

  <Toasts />
</div>

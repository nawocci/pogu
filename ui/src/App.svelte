<script lang="ts">
  import { onMount } from 'svelte';
  import { fade } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { api } from './lib/api';
  import { app, store } from './lib/store.svelte';
  import { router, initRouter, navigate, getSafeRedirect } from './lib/router.svelte';
  import { theme } from './lib/theme.svelte';
  import { reducedMotion } from './lib/motion.svelte';
  import { morphState, setMorphElement } from './lib/authMorph.svelte';
  import Login from './lib/Login.svelte';
  import Shell from './lib/Shell.svelte';
  import Providers from './lib/pages/Providers.svelte';
  import ProviderDetail from './lib/pages/ProviderDetail.svelte';
  import Groups from './lib/pages/Groups.svelte';
  import GroupDetail from './lib/pages/GroupDetail.svelte';
  import Connections from './lib/pages/Connections.svelte';
  import Monitoring from './lib/pages/Monitoring.svelte';
  import NotFound from './lib/pages/NotFound.svelte';

  let ready = $state(false);

  onMount(() => initRouter());

  $effect(() => {
    api
      .me()
      .catch(() => null)
      .then((me) => {
        app.authenticated = Boolean(me?.authenticated);
        ready = true;
      });
  });

  $effect(() => {
    if (!ready) return;
    if (!app.authenticated && router.route.name !== 'login') {
      const dest = router.path + (router.search || '');
      navigate('/login?redirect=' + encodeURIComponent(dest), { replace: true });
    } else if (app.authenticated && router.route.name === 'login') {
      const dest = getSafeRedirect(router.search, '/');
      navigate(dest, { replace: true });
    }
  });

  $effect(() => {
    if (app.authenticated) void store.refresh();
  });

  $effect(() => {
    document
      .querySelector('meta[name="theme-color"]')
      ?.setAttribute('content', theme() === 'light' ? '#e4ebf3' : '#0a0e14');
  });

  let morphEl = $state<HTMLDivElement | null>(null);
  $effect(() => {
    setMorphElement(morphEl);
    return () => setMorphElement(null);
  });
</script>

{#if !ready}
  <div class="grid min-h-dvh place-items-center bg-bg">
    <span
      class="inline-flex items-center gap-3 font-mono text-[11px] tracking-[0.07em] text-tertiary uppercase"
      role="status"
    >
      <span class="size-1.5 animate-pulse-dot rounded-full bg-accent" aria-hidden="true"></span>
      Connecting…
    </span>
  </div>
{:else}
  <div
    class="flex min-h-dvh items-center justify-center bg-bg {app.authenticated
      ? 'p-(--inset-y) px-(--inset-x) [--inset-y:clamp(14px,2.8vh,30px)] [--inset-x:clamp(14px,2.4vw,38px)] max-compact:bg-ink max-compact:p-0'
      : 'px-5'}"
  >
    <div
      bind:this={morphEl}
      role={!app.authenticated ? 'document' : undefined}
      aria-label={!app.authenticated ? 'Sign in' : undefined}
      class="w-full overflow-hidden border border-line shadow-workspace {app.authenticated
        ? 'h-[calc(100dvh-2*var(--inset-y))] max-w-[1840px] rounded-lg bg-ink max-compact:h-auto max-compact:min-h-dvh max-compact:max-w-none max-compact:rounded-none max-compact:border-0 max-compact:shadow-none'
        : 'max-w-[400px] rounded-lg bg-raised'}"
    >
      {#if app.authenticated}
        <Shell>
          <div class="view-stack mx-auto max-w-[1200px]">
            {#key router.path}
              <div class="min-w-0" out:fade={{ duration: reducedMotion.current || morphState.active ? 0 : 110, easing: cubicOut }}>
                {#if router.route.name === 'connections'}
                  <Connections />
                {:else if router.route.name === 'providers'}
                  <Providers />
                {:else if router.route.name === 'provider-detail'}
                  <ProviderDetail providerId={router.route.providerId} />
                {:else if router.route.name === 'groups'}
                  <Groups />
                {:else if router.route.name === 'group-detail'}
                  <GroupDetail groupId={router.route.groupId} />
                {:else if router.route.name === 'monitoring'}
                  <Monitoring />
                {:else}
                  <NotFound path={router.path} />
                {/if}
              </div>
            {/key}
          </div>
        </Shell>
      {:else}
        <Login />
      {/if}
    </div>
  </div>
{/if}

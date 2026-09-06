<script lang="ts">
  import { fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { monitor, type FeedEntry } from '../../monitor.svelte';
  import { reducedMotion } from '../../motion.svelte';
  import { latency, compact, ago, int } from '../../format';
  import { outcomeMeta } from './outcome';

  const enter = { y: -8, duration: reducedMotion.current ? 0 : 180, easing: cubicOut };

  const VIEWPORT = 420;

  let now = $state(Date.now());
  $effect(() => {
    const t = setInterval(() => (now = Date.now()), 5000);
    return () => clearInterval(t);
  });

  function tokens(e: FeedEntry): string {
    if (e.inputTokens === undefined && e.outputTokens === undefined) return '';
    return `${compact(e.inputTokens ?? null)} → ${compact(e.outputTokens ?? null)}`;
  }

  function attribution(e: FeedEntry): string {
    const parts = [e.keyLabel];
    const route: string[] = [];
    if (e.resolvedModel && e.resolvedModel !== e.model) route.push(e.resolvedModel);
    const servedBase = e.resolvedModel ?? e.model;
    if (e.servedModel && e.servedModel !== servedBase) route.push(e.servedModel);
    if (route.length > 0) parts.push(route.join(' → '));
    if (e.provider) parts.push(e.provider);
    return parts.join(' · ');
  }
</script>

<section aria-labelledby="live-feed-h" class="flex min-w-0 flex-col rounded-sm border border-line bg-raised">
  <header class="flex items-center gap-2.5 border-b border-line-soft px-3.5 py-2.5">
    <h2 id="live-feed-h" class="text-[15px]">Live requests</h2>
    <span
      class="inline-flex items-center gap-[6px] rounded-full border py-px pr-2 pl-1.5 font-mono text-[10px] font-medium tracking-[0.07em] uppercase {monitor
        .liveConnected
        ? 'border-accent-dim bg-accent-dim text-accent-ink'
        : 'border-line-soft bg-raised text-tertiary'}"
    >
      <span
        class="-translate-y-px size-1.5 rounded-full {monitor.liveConnected
          ? 'bg-accent animate-pulse-dot'
          : 'bg-muted'}"
        aria-hidden="true"></span>
      {monitor.liveConnected ? 'Live' : 'Offline'}
    </span>
    <span class="ml-auto font-mono text-[11px] text-tertiary">
      {monitor.activeEntries.length} in flight
    </span>
  </header>

  <div class="overflow-y-auto" style="height: {VIEWPORT}px">
    {#if monitor.feed.length === 0}
      <div class="flex h-full flex-col items-center justify-center px-6 text-center">
        <p class="m-0 text-[13px] text-secondary">No requests yet</p>
        <p class="mt-1.5 mb-0 max-w-[44ch] text-xs text-tertiary">
          Activity appears here the moment a client reaches pogu.
        </p>
      </div>
    {:else}
      <ol class="m-0 list-none p-0">
        {#each monitor.activeEntries as e (e.requestId)}
          {@render Row({ e, now })}
        {/each}
        {#each monitor.finishedEntries.slice(0, 60) as e (e.requestId)}
          {@render Row({ e, now })}
        {/each}
      </ol>
    {/if}
  </div>
</section>

{#snippet Row({ e, now }: { e: FeedEntry; now: number })}
  {@const live = e.state === 'active' || e.state === 'streaming'}
  {@const meta = outcomeMeta(e.state)}
  <li
    in:fly|local={enter}
    class="relative border-b border-line-soft px-3.5 py-2.5 last:border-b-0 {live
      ? 'bg-accent-dim/50 shadow-[inset_2px_0_0_var(--accent)]'
      : 'opacity-80 hover:opacity-100'}"
  >
    <div class="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3">
      <span class="relative inline-flex size-[9px] shrink-0 items-center justify-center" aria-hidden="true">
        {#if live && !reducedMotion.current}
          <span class="absolute inset-0 rounded-full bg-accent animate-ring-pulse"></span>
        {/if}
        <span class="size-[7px] shrink-0 rounded-full {meta.dot}"></span>
      </span>

      <div class="min-w-0">
        <div class="flex min-w-0 items-baseline gap-2">
          <code class="truncate font-mono text-[12.5px] font-semibold text-paper">{e.model}</code>
          {#if e.stream}
            <span class="shrink-0 font-mono text-[10px] tracking-[0.07em] text-accent-ink/80 uppercase" title="Streaming request">str</span>
          {/if}
          {#if e.state === 'failed' && e.errType}
            <code class="shrink-0 rounded-full border border-error-soft bg-error-soft px-1.5 font-mono text-[10px] text-error-muted">{e.errType}</code>
          {/if}
        </div>
        <div class="mt-0.5 truncate font-mono text-[10.5px] leading-[1.4] {live ? 'text-primary' : 'text-tertiary'}">
          {attribution(e)}
        </div>
      </div>

      <div class="text-right whitespace-nowrap">
        {#if live}
          <span class="inline-flex items-center gap-1.5 rounded-full border border-accent-soft bg-raised px-2 py-px font-mono text-[10px] font-semibold tracking-[0.07em] text-accent-ink uppercase">
            <span class="size-1 rounded-full bg-accent animate-pulse-dot" aria-hidden="true"></span>
            {meta.label}…
          </span>
        {:else}
          <div class="font-mono text-[11.5px] text-primary">{latency(e.latencyMs)}</div>
          {#if tokens(e)}
            <div class="font-mono text-[10px] text-tertiary" title="{int(e.totalTokens ?? 0)} total tokens">{tokens(e)} tok</div>
          {/if}
        {/if}
        <div class="font-mono text-[10px] text-tertiary">{ago(e.ts, now)}</div>
      </div>
    </div>

    {#if e.state === 'streaming'}
      <span class="absolute inset-x-0 bottom-0 h-px overflow-hidden" aria-hidden="true">
        <span class="absolute inset-y-0 w-full bg-gradient-to-r from-transparent via-accent to-transparent animate-sweep"></span>
      </span>
    {/if}
  </li>
{/snippet}

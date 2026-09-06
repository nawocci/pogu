<script lang="ts">
  import { fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import { monitor } from '../../monitor.svelte';
  import { reducedMotion } from '../../motion.svelte';
  import { int } from '../../format';

  const BODY = 420;
  const MAX_CALLERS = 2;

  const GRID_WIDE = 'grid-cols-[minmax(70px,132px)_max-content_minmax(28px,1fr)_max-content_64px]';
  const GRID_TIGHT = 'grid-cols-[max-content_minmax(24px,1fr)_max-content]';

  let width = $state(0);
  const compact = $derived(width > 0 && width < 430);

  const view = $derived(monitor.laneView);
  const lanes = $derived(view.lanes);
  const overflow = $derived(view.overflow);
  const activeLanes = $derived(lanes.filter((l) => l.active).length);
  const providers = $derived(new Set(lanes.map((l) => l.provider)).size);
  const maxCount = $derived(Math.max(1, ...lanes.map((l) => l.count)));

  function share(count: number): string {
    return `${Math.max(7, Math.round((count / maxCount) * 100))}%`;
  }

  const enter = { y: -6, duration: reducedMotion.current ? 0 : 160, easing: cubicOut };
</script>

<section aria-labelledby="serving-h" class="flex min-w-0 flex-col rounded-sm border border-line bg-raised">
  <header class="flex items-baseline gap-2.5 border-b border-line-soft px-3.5 py-2.5">
    <h2 id="serving-h" class="text-[15px]">Serving now</h2>
    <p class="m-0 hidden font-mono text-[10px] tracking-[0.07em] text-tertiary uppercase lg:block">
      inbound → requested → served
    </p>
    <span class="ml-auto whitespace-nowrap font-mono text-[10px] text-tertiary">
      {#if lanes.length === 0}
        idle
      {:else if activeLanes > 0}
        {activeLanes} active route{activeLanes === 1 ? '' : 's'}
      {:else}
        {lanes.length} route{lanes.length === 1 ? '' : 's'}{providers > 1 ? ` · ${providers} providers` : ''}
      {/if}
    </span>
  </header>

  <div class="overflow-y-auto" style="height: {BODY}px">
    {#if lanes.length === 0}
      <div class="flex h-full flex-col items-center justify-center px-6 text-center">
        <p class="m-0 text-[13px] text-secondary">Nothing is being served right now</p>
        <p class="mt-1.5 mb-0 max-w-[46ch] text-xs text-tertiary">
          When clients reach pogu, their routes appear here: who is calling,
          what they ask for, and which provider answers.
        </p>
      </div>
    {:else}
      {#if !compact}
        <div class="grid items-center gap-x-2.5 px-3.5 pt-2 pb-1 font-mono text-[10px] tracking-[0.07em] whitespace-nowrap text-tertiary uppercase {GRID_WIDE}">
          <span>Inbound</span>
          <span>Requested</span>
          <span></span>
          <span>Served from</span>
          <span class="text-right">Traffic</span>
        </div>
      {/if}
      <ol class="m-0 list-none p-0" bind:clientWidth={width}>
        {#each lanes as lane (lane.id)}
            {@const failedOnly = lane.failed >= lane.count && !lane.active}
            {@const resolved = lane.resolved}
            {@const servedName = lane.served}
            {@const dest = servedName && resolved && servedName !== resolved
              ? `${resolved} → ${servedName}`
              : (servedName ?? resolved ?? lane.upstream ?? (lane.active ? undefined : lane.model))}
            <li
              in:fly|local={enter}
              class="relative px-3.5 py-2 {lane.active
                ? 'bg-accent-dim/40 shadow-[inset_2px_0_0_var(--accent)]'
                : ''}"
            >
              <div
                class="grid items-center gap-x-2.5 gap-y-1 {compact ? GRID_TIGHT : GRID_WIDE}"
                role="group"
                aria-label="{lane.callers.map((c) => c.label).join(' + ')} → {lane.model} → {dest ?? 'unresolved'} · {lane.provider}, {lane.count} requests"
              >
                <div class="flex min-w-0 flex-wrap items-center gap-1 {compact ? 'order-3 col-span-3' : ''}">
                  {#each lane.callers.slice(0, MAX_CALLERS) as c (c.label)}
                    <span
                      class="max-w-[120px] truncate rounded-full border border-line-soft px-1.5 py-px font-mono text-[10px] text-secondary"
                      title={c.label}
                    >
                      {c.label}
                    </span>
                  {/each}
                  {#if lane.callers.length > MAX_CALLERS}
                    <span class="font-mono text-[10px] text-tertiary" title={[...lane.callers.map((c) => c.label)].slice(MAX_CALLERS).join(', ')}>
                      +{lane.callers.length - MAX_CALLERS}
                    </span>
                  {/if}
                </div>

                <code class="max-w-[150px] truncate font-mono text-[12.5px] font-semibold text-paper" title={lane.model}>{lane.model}</code>

                <div class="relative flex h-[9px] items-center" aria-hidden="true">
                  <div
                    class="h-px w-full {failedOnly
                      ? 'trace trace-failed'
                      : lane.active && !reducedMotion.current
                        ? 'trace trace-active'
                        : lane.active
                          ? 'trace trace-active-static'
                          : 'trace'}"
                  ></div>
                  <svg
                    width="5"
                    height="9"
                    viewBox="0 0 5 9"
                    class="ml-[-1px] shrink-0 {failedOnly ? 'text-error-muted' : lane.active ? 'text-accent' : 'text-line'}"
                  >
                    <path d="M0 0 L5 4.5 L0 9" fill="none" stroke="currentColor" stroke-width="1.2"></path>
                  </svg>
                </div>

                <div
                  class="min-w-[86px] rounded-sm border px-1.5 py-1 {failedOnly
                    ? 'border-error-soft bg-error-soft'
                    : dest
                      ? 'border-line bg-well'
                      : 'border-dashed border-line'}"
                  title="{dest ? `${lane.model} → ${dest}` : lane.active ? 'routing not resolved yet' : 'served as requested'} · {lane.provider}"
                >
                  <code class="block truncate text-left font-mono text-[12px] leading-tight font-semibold {failedOnly
                    ? 'text-error-muted'
                    : dest
                      ? 'text-paper'
                      : 'text-tertiary'}">
                    {dest ?? 'resolving…'}
                  </code>
                  <span class="block truncate text-left font-mono text-[10px] leading-tight text-tertiary">{lane.provider}</span>
                </div>

                <div class="w-[64px] text-right" title="{int(lane.count)} request{lane.count === 1 ? '' : 's'} through this route{lane.failed ? `, ${int(lane.failed)} failed` : ''}{lane.cancelled ? `, ${int(lane.cancelled)} cancelled` : ''}">
                  <div class="inline-flex items-center gap-1.5 font-mono text-[12.5px] tabular-nums text-primary">
                    {#if lane.failed > 0}
                      <span class="size-1 rounded-full {failedOnly ? 'bg-error' : 'bg-error/70'}" aria-label="{lane.failed} failed"></span>
                    {/if}
                    {int(lane.count)}
                  </div>
                  <div class="mt-1 h-[3px] w-full overflow-hidden rounded-full bg-well" aria-hidden="true">
                    <div
                      class="h-full rounded-full {failedOnly ? 'bg-error-muted' : lane.active ? 'bg-accent' : 'bg-accent-soft'}"
                      style="width: {share(lane.count)}"
                    ></div>
                  </div>
                </div>
              </div>
            </li>
        {/each}
      </ol>
      {#if overflow > 0}
        <p class="py-1.5 m-0 w-full text-center font-mono text-[10px] text-tertiary">
          +{int(overflow)} quieter relation{overflow === 1 ? 'ship' : 'ships'}
        </p>
      {/if}
    {/if}
  </div>
</section>

<style>
  .trace {
    background-image: repeating-linear-gradient(90deg, var(--line) 0 3px, transparent 3px 7px);
  }
  .trace-failed {
    background-image: repeating-linear-gradient(90deg, var(--error-muted) 0 3px, transparent 3px 7px);
  }
  .trace-active-static {
    background-image: repeating-linear-gradient(90deg, var(--accent) 0 3px, transparent 3px 7px);
  }
  .trace-active {
    background-image: repeating-linear-gradient(90deg, var(--accent) 0 3px, transparent 3px 7px);
    animation: charge 0.85s linear infinite;
  }
  @keyframes charge {
    to {
      background-position-x: 7px;
    }
  }
</style>

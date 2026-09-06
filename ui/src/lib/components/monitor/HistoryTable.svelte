<script lang="ts">
  import { slide } from 'svelte/transition';
  import { monitor } from '../../monitor.svelte';
  import { reveal } from '../../motion.svelte';
  import { latency, compact, clockTime, fullTime, int } from '../../format';
  import { outcomeMeta, outcomeOfRecord } from './outcome';
  import type { MonitorRecord } from '../../api';

  let expanded = $state<string | null>(null);

  function toggle(id: string) {
    expanded = expanded === id ? null : id;
  }

  const rangeStart = $derived.by(() => {
    const page = monitor.history;
    if (!page || page.total === 0) return 0;
    return (page.page - 1) * page.page_size + 1;
  });
  const rangeEnd = $derived.by(() => {
    const page = monitor.history;
    if (!page) return 0;
    return Math.min(page.total, page.page * page.page_size);
  });

  const hasActiveFilters = $derived(
    Boolean(
      monitor.filters.keyId ||
        monitor.filters.provider ||
        monitor.filters.model ||
        monitor.filters.protocol ||
        monitor.filters.group ||
        monitor.filters.outcome ||
        monitor.filters.stream,
    ),
  );

  function keyLabel(rec: MonitorRecord): string {
    if (rec.key_name) return rec.key_name;
    if (rec.key_id != null) return 'Removed key';
    return 'Unknown key';
  }

  function overheadMs(rec: MonitorRecord): number | null {
    if (rec.upstream_ms === undefined || rec.upstream_ms === null) return null;
    if (rec.latency_ms === undefined || rec.latency_ms === null) return null;
    return Math.max(0, rec.latency_ms - rec.upstream_ms);
  }
</script>

<section use:reveal={{ kind: 'rise', i: 4 }} aria-labelledby="history-h" class="mt-[42px]">
  <div class="sec-head">
    <h2 id="history-h">
      Request history
      {#if monitor.history?.total}
        <span class="ml-[5px] font-mono text-[13px] text-accent-ink [vertical-align:3px]">{int(monitor.history.total)}</span>
      {/if}
    </h2>
    <button class="btn btn-sm" onclick={() => monitor.refreshHistory()} disabled={monitor.historyLoading} aria-busy={monitor.historyLoading}>
      Refresh
    </button>
  </div>

  {#if monitor.loadError}
    <p class="m-0 py-6 text-center text-[13px] text-tertiary">History unavailable — {monitor.loadError}</p>
  {:else if monitor.historyLoading && !monitor.history}
    <div class="flex animate-pulse flex-col gap-2.5" aria-hidden="true">
      {#each Array(6) as _, i (i)}
        <div class="h-[38px] rounded-sm bg-well" style="width: {96 - i * 3}%"></div>
      {/each}
    </div>
  {:else if !monitor.history || monitor.history.items.length === 0}
    <div use:reveal={{ kind: 'blip' }} class="flex flex-col items-center rounded-sm border border-dashed border-line px-5 py-12 text-center">
      <p class="m-0 text-[13px] text-secondary">
        {#if hasActiveFilters}
          No requests match the current period and filters.
        {:else}
          No requests recorded in this period yet.
        {/if}
      </p>
      <p class="mt-1.5 mb-0 max-w-[52ch] text-xs text-tertiary">
        Finished requests are kept indefinitely — widen the time range or clear filters to see more.
      </p>
    </div>
  {:else}
    <div use:reveal={{ kind: 'blip' }} class="-mx-1 overflow-x-auto px-1">
      <table class="table-data min-w-[760px]">
        <thead>
          <tr>
            <th>Time</th>
            <th>API key</th>
            <th>Model</th>
            <th>Provider</th>
            <th>Status</th>
            <th>Stream</th>
            <th class="text-right">Tokens in / out</th>
            <th class="text-right">Latency</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each monitor.history.items as rec (rec.request_id + rec.ts)}
            {@const meta = outcomeMeta(outcomeOfRecord(rec))}
            {@const isOpen = expanded === rec.request_id}
            {@const resolved = rec.resolved_model && rec.resolved_model !== rec.model ? rec.resolved_model : undefined}
            {@const servedBase = resolved ?? rec.model}
            {@const served = rec.served_model && rec.served_model !== servedBase ? rec.served_model : undefined}
            <tr class="cursor-pointer" onclick={() => toggle(rec.request_id)} aria-expanded={isOpen}>
              <td><time class="font-mono text-[11px] whitespace-nowrap text-tertiary" title={fullTime(rec.ts)}>{clockTime(rec.ts, monitor.range !== 'daily')}</time></td>
              <td class="max-w-[180px] truncate" title={keyLabel(rec)}>{keyLabel(rec)}</td>
              <td><code class="font-mono text-[12px] text-paper" title={served ? `served as ${served}` : resolved ? `served as ${resolved}` : rec.model}>{rec.model}</code>
                {#if resolved}
                  <span class="ml-1 font-mono text-[10.5px] text-tertiary" title="Resolved model">→ {resolved}</span>
                {/if}
                {#if served}
                  <span class="ml-1 font-mono text-[10.5px] text-tertiary" title="Upstream-reported model">→ {served}</span>
                {/if}
              </td>
              <td class="max-w-[140px] truncate font-mono text-[12px] text-secondary">{rec.provider || '—'}</td>
              <td>
                <span class="inline-flex items-center gap-[6px] rounded-full border px-1.5 py-px font-mono text-[10px] tracking-[0.07em] uppercase {meta.badge}">
                  <span class="size-1 rounded-full {meta.dot}" aria-hidden="true"></span>
                  {meta.label}
                </span>
              </td>
              <td class="font-mono text-[11px] text-tertiary">{rec.stream ? 'stream' : 'no'}</td>
              <td class="text-right font-mono text-[11.5px] whitespace-nowrap text-primary">
                {compact(rec.input_tokens ?? null)} / {compact(rec.output_tokens ?? null)}
              </td>
              <td class="text-right font-mono text-[11.5px] whitespace-nowrap text-primary">{latency(rec.latency_ms)}</td>
              <td class="w-8 text-right">
                <button
                  class="-m-1 inline-flex size-7 items-center justify-center rounded-sm p-1 font-mono text-[10px] text-tertiary transition-colors hover:text-paper"
                  aria-label={isOpen ? 'Collapse details' : 'Expand details'}
                  aria-expanded={isOpen}
                  onclick={(e) => {
                    e.stopPropagation();
                    toggle(rec.request_id);
                  }}
                >
                  ▸
                </button>
              </td>
            </tr>
            {#if isOpen}
              <tr>
                <td colspan="9" class="bg-well p-0" transition:slide|local={{ duration: 150 }}>
                  <dl class="m-0 grid grid-cols-[repeat(auto-fit,minmax(160px,1fr))] gap-x-6 gap-y-2.5 px-4 py-3.5 font-mono text-[11px]">
                    <span>
                      <dt class="mb-0.5 text-[10px] tracking-[0.07em] text-tertiary uppercase">Request ID</dt>
                      <dd class="m-0 text-primary [overflow-wrap:anywhere]">{rec.request_id}</dd>
                    </span>
                    <span>
                      <dt class="mb-0.5 text-[10px] tracking-[0.07em] text-tertiary uppercase">Started</dt>
                      <dd class="m-0 text-primary">{fullTime(rec.ts)}</dd>
                    </span>
                    <span>
                      <dt class="mb-0.5 text-[10px] tracking-[0.07em] text-tertiary uppercase">Protocol</dt>
                      <dd class="m-0 text-primary">{rec.protocol}</dd>
                    </span>
                    <span>
                      <dt class="mb-0.5 text-[10px] tracking-[0.07em] text-tertiary uppercase">HTTP status</dt>
                      <dd class="m-0 text-primary">{rec.http_status ?? '—'}</dd>
                    </span>
                    <span>
                      <dt class="mb-0.5 text-[10px] tracking-[0.07em] text-tertiary uppercase">Error category</dt>
                      <dd class="m-0 text-primary">{rec.error || '—'}</dd>
                    </span>
                    <span>
                      <dt class="mb-0.5 text-[10px] tracking-[0.07em] text-tertiary uppercase">Requested model</dt>
                      <dd class="m-0 text-primary">{rec.model}</dd>
                    </span>
                    {#if rec.group_name}
                      <span>
                        <dt class="mb-0.5 text-[10px] tracking-[0.07em] text-tertiary uppercase">Group</dt>
                        <dd class="m-0 text-primary">{rec.group_name}</dd>
                      </span>
                    {/if}
                    {#if rec.resolved_model}
                      <span>
                        <dt class="mb-0.5 text-[10px] tracking-[0.07em] text-tertiary uppercase">Resolved model</dt>
                        <dd class="m-0 text-primary">{rec.resolved_model}</dd>
                      </span>
                    {/if}
                    {#if rec.served_model}
                      <span>
                        <dt class="mb-0.5 text-[10px] tracking-[0.07em] text-tertiary uppercase">Served model</dt>
                        <dd class="m-0 text-primary">{rec.served_model}</dd>
                      </span>
                    {/if}
                    <span>
                      <dt class="mb-0.5 text-[10px] tracking-[0.07em] text-tertiary uppercase">Provider</dt>
                      <dd class="m-0 text-primary">{rec.provider || '—'}</dd>
                    </span>
                    <span>
                      <dt class="mb-0.5 text-[10px] tracking-[0.07em] text-tertiary uppercase">Total tokens</dt>
                      <dd class="m-0 text-primary">{compact(rec.total_tokens ?? null)}</dd>
                    </span>
                    <span>
                      <dt class="mb-0.5 text-[10px] tracking-[0.07em] text-tertiary uppercase">TTFT</dt>
                      <dd class="m-0 text-primary">
                        {rec.stream ? latency(rec.ttft_ms ?? null) : 'n/a'}
                      </dd>
                    </span>
                    <span>
                      <dt class="mb-0.5 text-[10px] tracking-[0.07em] text-tertiary uppercase">Upstream</dt>
                      <dd class="m-0 text-primary">{latency(rec.upstream_ms ?? null)}</dd>
                    </span>
                    <span>
                      <dt class="mb-0.5 text-[10px] tracking-[0.07em] text-tertiary uppercase">Gateway overhead</dt>
                      <dd class="m-0 text-primary">{latency(overheadMs(rec))}</dd>
                    </span>
                  </dl>
                  {#if rec.attempts.length > 1}
                    <div class="border-t border-line-soft px-4 py-3">
                      <p class="m-0 mb-2 font-mono text-[10px] tracking-[0.07em] text-tertiary uppercase">
                        Attempts ({rec.attempts.length})
                      </p>
                      <table class="table-data">
                        <thead>
                          <tr><th>#</th><th>Provider</th><th>Upstream model</th><th>Status</th><th>Error</th><th class="text-right">Latency</th><th class="text-right">Tokens in / out</th></tr>
                        </thead>
                        <tbody>
                          {#each rec.attempts as a (a.number)}
                            <tr>
                              <td class="font-mono text-[11px] text-tertiary">{a.number}</td>
                              <td class="font-mono text-[11px] text-primary">{a.provider_name}</td>
                              <td><code class="font-mono text-[11px] text-secondary">{a.upstream_model}</code></td>
                              <td class="font-mono text-[11px] {a.success ? 'text-success-muted' : 'text-error-muted'}">{a.http_status ?? '—'}</td>
                              <td class="font-mono text-[11px] text-tertiary">{a.error_category || '—'}</td>
                              <td class="text-right font-mono text-[11px] text-primary">{latency(a.duration_ms)}</td>
                              <td class="text-right font-mono text-[11px] text-primary">{compact(a.input_tokens)} / {compact(a.output_tokens)}</td>
                            </tr>
                          {/each}
                        </tbody>
                      </table>
                    </div>
                  {/if}
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    </div>

    <div class="mt-3 flex items-center gap-4">
      <p class="m-0 font-mono text-[11px] text-tertiary">
        {rangeStart}–{rangeEnd} of {int(monitor.history.total)}
      </p>
      <div class="ml-auto flex gap-2">
        <button
          class="btn btn-sm"
          disabled={monitor.history.page <= 1 || monitor.historyLoading}
          onclick={() => monitor.refreshHistory(monitor.history!.page - 1)}
        >
          Prev
        </button>
        <button
          class="btn btn-sm"
          disabled={rangeEnd >= monitor.history.total || monitor.historyLoading}
          onclick={() => monitor.refreshHistory(monitor.history!.page + 1)}
        >
          Next
        </button>
      </div>
    </div>
  {/if}
</section>

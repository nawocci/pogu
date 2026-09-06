<script lang="ts">
  import { monitor } from '../../monitor.svelte';
  import { reveal } from '../../motion.svelte';
  import { compact, latency, int, bucketLabel, clockTime } from '../../format';
  import type { MonitorBucket } from '../../api';

  type Metric = 'requests' | 'input_tokens' | 'output_tokens' | 'avg_latency_ms';

  const METRICS: { key: Metric; label: string; color: string; mean?: boolean }[] = [
    { key: 'requests', label: 'Requests', color: 'var(--accent)' },
    { key: 'input_tokens', label: 'Input tokens', color: 'var(--success)' },
    { key: 'output_tokens', label: 'Output tokens', color: 'var(--warning)' },
    { key: 'avg_latency_ms', label: 'Latency', color: 'var(--secondary)', mean: true },
  ];

  let metric = $state<Metric>('requests');
  let width = $state(0);
  let hoverIndex = $state<number | null>(null);

  const H = 240;
  const PAD = { top: 12, right: 14, bottom: 24, left: 46 };

  const selected = $derived(METRICS.find((m) => m.key === metric) ?? METRICS[0]);
  const interval = $derived(monitor.summary?.interval ?? 'hour');

  interface Row {
    t: Date;
    value: number | null;
    bucket: MonitorBucket;
  }

  const rows = $derived<Row[]>(
    monitor.buckets.map((b) => ({
      t: new Date(b.t),
      value: b[metric] ?? (metric === 'requests' ? 0 : null),
      bucket: b,
    })),
  );

  const innerW = $derived(Math.max(0, width - PAD.left - PAD.right));
  const innerH = $derived(H - PAD.top - PAD.bottom);

  const yMax = $derived.by(() => {
    const vals = rows.map((r) => r.value ?? 0);
    const raw = Math.max(1, ...vals);
    const exp = Math.floor(Math.log10(raw));
    const base = Math.pow(10, exp);
    for (const m of [1, 2, 2.5, 5, 10]) {
      if (raw <= m * base) return m * base;
    }
    return raw;
  });

  function x(i: number): number {
    const n = rows.length;
    if (n <= 1) return PAD.left + innerW / 2;
    return PAD.left + (i / (n - 1)) * innerW;
  }
  function y(v: number): number {
    return PAD.top + innerH - (v / yMax) * innerH;
  }

  const segments = $derived.by(() => {
    const segs: Row[][] = [];
    let cur: Row[] = [];
    rows.forEach((r) => {
      if (r.value !== null) {
        cur.push(r);
      } else if (cur.length) {
        segs.push(cur);
        cur = [];
      }
    });
    if (cur.length) segs.push(cur);
    return segs;
  });

  function linePath(seg: Row[]): string {
    return seg.map((r, i) => `${i === 0 ? 'M' : 'L'}${x(rows.indexOf(r)).toFixed(1)},${y(r.value as number).toFixed(1)}`).join('');
  }
  function areaPath(seg: Row[]): string {
    const first = rows.indexOf(seg[0]);
    const last = rows.indexOf(seg[seg.length - 1]);
    return (
      `M${x(first).toFixed(1)},${y(0).toFixed(1)}` +
      seg.map((r, i) => `L${x(first + i).toFixed(1)},${y(r.value as number).toFixed(1)}`).join('') +
      `L${x(last).toFixed(1)},${y(0).toFixed(1)}Z`
    );
  }

  const yTicks = $derived([0, 0.25, 0.5, 0.75, 1].map((f) => f * yMax));

  const xTickIdx = $derived.by(() => {
    const n = rows.length;
    if (n === 0) return [];
    if (n <= 7) return rows.map((_, i) => i);
    const step = Math.ceil(n / 6);
    const idx: number[] = [];
    for (let i = 0; i < n; i += step) idx.push(i);
    return idx;
  });

  function onMouseMove(ev: MouseEvent) {
    const rect = (ev.currentTarget as HTMLElement).getBoundingClientRect();
    const mx = ev.clientX - rect.left - PAD.left;
    const n = rows.length;
    if (n === 0 || innerW <= 0) return;
    hoverIndex = Math.max(0, Math.min(n - 1, Math.round((mx / innerW) * (n - 1))));
  }

  const hovered = $derived(hoverIndex !== null ? (rows[hoverIndex] ?? null) : null);

  const tipLeft = $derived.by(() => {
    if (hoverIndex === null) return 0;
    const px = x(hoverIndex);
    return Math.max(8, Math.min(px + 14, Math.max(8, width - 210)));
  });
</script>

<section use:reveal={{ kind: 'rise', i: 3 }} aria-labelledby="usage-chart-h">
  <div class="sec-head">
    <h2 id="usage-chart-h">Usage over time</h2>
    <div class="flex flex-wrap items-center gap-x-4 gap-y-2">
      <span class="font-mono text-[10px] tracking-[0.07em] text-tertiary uppercase">{interval}ly buckets</span>
      <div class="flex gap-0.5 rounded-sm border border-line bg-well p-0.5" role="group" aria-label="Chart metric">
        {#each METRICS as m (m.key)}
          <button
            aria-pressed={metric === m.key}
            class="rounded-[2px] px-2.5 py-1 font-mono text-[10px] font-medium tracking-[0.07em] uppercase transition-colors {metric === m.key
              ? 'bg-raised text-paper shadow-[0_0_0_1px_var(--line)]'
              : 'text-tertiary hover:text-primary'}"
            onclick={() => {
              metric = m.key;
              hoverIndex = null;
            }}
          >
            {m.label}
          </button>
        {/each}
      </div>
    </div>
  </div>

  <div class="panel overflow-hidden">
  <div class="relative px-2 pt-3 pb-1">
    {#if monitor.loadError}
      <p class="m-0 py-16 text-center text-[13px] text-tertiary" style="height: {H}px">
        Usage data unavailable — {monitor.loadError}
      </p>
    {:else if !monitor.loaded}
      <div class="flex animate-pulse items-end gap-1.5 px-11 pb-6" style="height: {H}px" aria-hidden="true">
        {#each Array(28) as _, i (i)}
          <div class="flex-1 rounded-t-[2px] bg-well" style="height: {25 + ((i * 37) % 60)}%"></div>
        {/each}
      </div>
    {:else if rows.length === 0 || segments.length === 0}
      <div class="flex flex-col items-center justify-center py-12 text-center" style="height: {H}px">
        <p class="m-0 text-[13px] text-secondary">No usage recorded in this period</p>
        <p class="mt-1.5 mb-0 max-w-[52ch] text-xs text-tertiary">
          {#if monitor.filters.keyId || monitor.filters.provider || monitor.filters.model || monitor.filters.protocol || monitor.filters.group || monitor.filters.outcome || monitor.filters.stream}
            Nothing matches the current filters — try clearing them or widening the time range.
          {:else}
            Send a request through pogu and the chart fills in as traffic lands.
          {/if}
        </p>
      </div>
    {:else}
      <div
        use:reveal={{ kind: 'blip' }}
        bind:clientWidth={width}
        role="img"
        aria-label="{selected.label} over the selected period, {rows.length} {interval}ly buckets"
        style="height: {H}px"
        onmousemove={onMouseMove}
        onmouseleave={() => (hoverIndex = null)}
      >
        <svg {width} height={H} class="block select-none">
          {#each yTicks as tv (tv)}
            <line x1={PAD.left} x2={PAD.left + innerW} y1={y(tv)} y2={y(tv)} class="stroke-line-soft" stroke-width="1"></line>
            <text x={PAD.left - 7} y={y(tv)} dy="0.32em" text-anchor="end" class="fill-tertiary font-mono" font-size="10">
              {selected.mean ? latency(tv) : compact(tv)}
            </text>
          {/each}

          {#each segments as seg (seg[0].bucket.t)}
            <path d={areaPath(seg)} fill={selected.color} fill-opacity="0.1"></path>
            <path d={linePath(seg)} fill="none" stroke={selected.color} stroke-width="1.5" stroke-linejoin="round"></path>
          {/each}

          {#each xTickIdx as i (i)}
            <text x={x(i)} y={H - 7} text-anchor="middle" class="fill-tertiary font-mono" font-size="10">
              {bucketLabel(rows[i].t, interval)}
            </text>
          {/each}

          {#if hovered && hoverIndex !== null}
            <line x1={x(hoverIndex)} x2={x(hoverIndex)} y1={PAD.top} y2={PAD.top + innerH} class="stroke-line"></line>
            {#if hovered.value !== null}
              <circle cx={x(hoverIndex)} cy={y(hovered.value)} r="3.5" fill={selected.color} stroke="var(--raised)" stroke-width="1.5"></circle>
            {/if}
          {/if}
        </svg>

        {#if hovered}
          <div
            class="pointer-events-none absolute top-2 rounded-sm border border-line bg-raised px-3 py-2 font-mono text-[11px] shadow-pop"
            style="left: {tipLeft}px; min-width: 190px"
            role="status"
          >
            <p class="m-0 mb-1.5 text-[10px] tracking-[0.07em] text-tertiary uppercase">
              {bucketLabel(hovered.t, interval)} · {clockTime(hovered.t)}
            </p>
            <dl class="m-0 space-y-1">
              <span class="flex items-baseline justify-between gap-4">
                <dt class="text-tertiary">Requests</dt>
                <dd class="m-0 text-paper">{int(hovered.bucket.requests)}</dd>
              </span>
              <span class="flex items-baseline justify-between gap-4">
                <dt class="text-tertiary">Input tok</dt>
                <dd class="m-0 text-paper">{compact(hovered.bucket.input_tokens)}</dd>
              </span>
              <span class="flex items-baseline justify-between gap-4">
                <dt class="text-tertiary">Output tok</dt>
                <dd class="m-0 text-paper">{compact(hovered.bucket.output_tokens)}</dd>
              </span>
              <span class="flex items-baseline justify-between gap-4">
                <dt class="text-tertiary">Avg latency</dt>
                <dd class="m-0 text-paper">{latency(hovered.bucket.avg_latency_ms)}</dd>
              </span>
            </dl>
          </div>
        {/if}
      </div>
    {/if}
  </div>
  <p class="m-0 flex items-center gap-2 border-t border-line-soft px-3.5 py-2.5 font-mono text-[10px] text-tertiary">
    <span class="inline-block size-2 rounded-full" style="background: {selected.color}" aria-hidden="true"></span>
    {selected.label}{selected.mean ? ' · mean per bucket · gaps had no traffic' : ''}
  </p>
  </div>
</section>

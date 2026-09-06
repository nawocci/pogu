import type { FeedState } from '../../monitor.svelte';

export interface OutcomeMeta {
  label: string;
  badge: string;
  dot: string;
}

export function outcomeMeta(state: FeedState): OutcomeMeta {
  switch (state) {
    case 'active':
      return {
        label: 'Routing',
        badge: 'text-accent-ink border-accent-dim bg-accent-dim',
        dot: 'bg-accent animate-pulse-dot',
      };
    case 'streaming':
      return {
        label: 'Streaming',
        badge: 'text-accent-ink border-accent-dim bg-accent-dim',
        dot: 'bg-accent animate-pulse-dot',
      };
    case 'completed':
      return {
        label: 'Completed',
        badge: 'text-success-muted border-success-soft bg-success-soft',
        dot: 'bg-success shadow-[0_0_6px_var(--success-soft)]',
      };
    case 'failed':
      return {
        label: 'Failed',
        badge: 'text-error-muted border-error-soft bg-error-soft',
        dot: 'bg-error shadow-[0_0_6px_var(--error-soft)]',
      };
    case 'cancelled':
      return {
        label: 'Cancelled',
        badge: 'text-warning-muted border-warning-soft bg-warning-soft',
        dot: 'bg-warning shadow-[0_0_6px_var(--warning-soft)]',
      };
    default:
      return {
        label: 'Completed',
        badge: 'text-success-muted border-success-soft bg-success-soft',
        dot: 'bg-success shadow-[0_0_6px_var(--success-soft)]',
      };
  }
}

export function outcomeOfRecord(rec: {
  cancelled: boolean;
  http_status: number | null;
  error: string;
}): FeedState {
  if (rec.cancelled) return 'cancelled';
  if ((rec.http_status != null && rec.http_status >= 400) || rec.error) return 'failed';
  return 'completed';
}

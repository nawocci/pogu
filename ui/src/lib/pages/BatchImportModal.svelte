<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import Busy from '../components/Busy.svelte';
  import { api, type ImportResult } from '../api';
  import { toast } from '../toast.svelte';

  let { providerId, onclose, onsaved }: { providerId: number; onclose: () => void; onsaved: () => void } = $props();

  let text = $state('');
  let preview = $state<ImportResult | null>(null);
  let busy = $state(false);
  let error = $state('');

  async function run(fn: () => Promise<void>) {
    error = '';
    busy = true;
    try {
      await fn();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }

  const checkPreview = () =>
    run(async () => {
      if (!text.trim()) {
        error = 'Paste at least one key — one per line.';
        return;
      }
      preview = await api.importProviderKeys(providerId, text, true);
    });

  const commitImport = () =>
    run(async () => {
      const r = await api.importProviderKeys(providerId, text, false);
      toast('success', `Imported ${r.valid} key${r.valid === 1 ? '' : 's'}`);
      onsaved();
      onclose();
    });
</script>

<Modal title="Batch import API keys" {onclose} wide>
  <div class="grid gap-4">
    <p class="m-0 text-[13px] text-secondary">
      One key per line. Format <code class="font-mono text-xs text-accent-ink">name|key</code> or just
      <code class="font-mono text-xs text-accent-ink">key</code> (auto-named). Nothing is stored until you confirm.
    </p>
    <label class="field mb-0">
      <span>Keys list</span>
      <textarea bind:value={text} oninput={() => (preview = null)} placeholder={'work|sk-abc\nsk-def\nbackup|sk-ghi'}></textarea>
    </label>

    {#if preview}
      <div class="rounded-sm border border-line-soft bg-well px-3.5 py-3">
        <p class="m-0 mb-2 flex flex-wrap items-center gap-2.5 font-mono text-[11px] tracking-[0.07em] text-secondary uppercase">
          <b class="font-semibold text-paper">{preview.total} entr{preview.total === 1 ? 'y' : 'ies'} detected</b>
          <span class="rounded-full border border-success-soft bg-success-soft px-2 py-px text-success-muted">{preview.valid} valid</span>
          {#if preview.invalid > 0}
            <span class="rounded-full border border-error-soft bg-error-soft px-2 py-px text-error-muted">{preview.invalid} invalid</span>
          {/if}
        </p>
        <ul class="m-0 grid max-h-[200px] list-none gap-1 overflow-y-auto p-0 font-mono text-[12px]">
          {#each preview.entries as entry (entry.line_number)}
            <li class="flex items-center gap-2 rounded-sm px-2 py-1 {entry.valid ? 'bg-success-soft text-success-muted' : 'bg-error-soft text-error-muted'}">
              <span class="w-3 shrink-0 font-semibold">{entry.valid ? '✓' : '✗'}</span>
              {#if entry.valid}
                <span class="font-semibold">{entry.name}</span>
                <span class="truncate text-tertiary">{entry.masked_key}</span>
              {:else}
                <span>Line {entry.line_number} — {entry.error}</span>
              {/if}
            </li>
          {/each}
        </ul>
      </div>
    {/if}

    {#if error}<p class="error-line enter-blip">{error}</p>{/if}

    <div class="modal-actions">
      <button type="button" class="btn" onclick={onclose}>Cancel</button>
      {#if !preview || preview.valid === 0}
        <button type="button" class="btn btn-primary" disabled={busy || !text.trim()} onclick={checkPreview} aria-busy={busy}>
          <Busy busy={busy} text="Preview import" wide="Validating…" />
        </button>
      {:else}
        <button type="button" class="btn" disabled={busy} onclick={checkPreview} aria-busy={busy}>
          <Busy busy={busy} text="Re-evaluate" />
        </button>
        <button type="button" class="btn btn-primary" disabled={busy || preview.valid === 0} onclick={commitImport} aria-busy={busy}>
          <Busy busy={busy} text={`Import ${preview.valid} valid key${preview.valid === 1 ? '' : 's'}`} wide="Importing…" />
        </button>
      {/if}
    </div>
  </div>
</Modal>

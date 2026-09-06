<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import Copy from '../components/Copy.svelte';

  let { secret, onclose }: { secret: string; onclose: () => void } = $props();
</script>

<Modal title="API key created" {onclose}>
  <p class="mb-4 text-[13px] text-tertiary">
    Copy this secret now — <b class="font-semibold text-warning-muted">it will not be shown again</b>.
  </p>
  <div class="panel mb-4 flex items-center gap-2 px-3.5 py-3">
    <code class="min-w-0 flex-1 font-mono text-[13px] break-all text-accent-ink select-all">{secret}</code>
    <Copy value={secret} label="Copy key" silent />
  </div>
  <div class="modal-actions">
    <button class="btn btn-primary" onclick={onclose}>Done</button>
  </div>
</Modal>

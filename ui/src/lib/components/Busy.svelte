<script lang="ts">
  import Spinner from './Spinner.svelte';

  let { busy = false, text, wide = '' }: { busy?: boolean; text: string; wide?: string } = $props();
</script>

<span class="relative inline-grid items-center justify-center">
  <span class="invisible col-start-1 row-start-1 whitespace-nowrap" aria-hidden="true">{wide || text}</span>
  {#if !busy}
    <span class="col-start-1 row-start-1 whitespace-nowrap">{text}</span>
  {:else}
    <span class="absolute inset-0 grid place-items-center"><Spinner size={11} /></span>
  {/if}
</span>

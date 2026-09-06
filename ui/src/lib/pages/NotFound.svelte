<script lang="ts">
  let { path = '' }: { path?: string } = $props();
</script>

<div class="empty">
  <h2 class="mb-2">Page not found</h2>
  <p class="mt-2 mb-[18px] text-tertiary">
    The requested route <code class="font-mono text-xs">{path}</code> does not exist.
  </p>
  <a class="btn btn-primary" href="/">Back to providers</a>
</div>

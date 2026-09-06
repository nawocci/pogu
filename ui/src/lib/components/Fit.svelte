<script lang="ts">
  let { text, wide = '' }: { text: string; wide?: string } = $props()
</script>

<span class="grid">
  <span class="invisible col-start-1 row-start-1 whitespace-nowrap" aria-hidden="true">{wide || text}</span>
  <span class="col-start-1 row-start-1 whitespace-nowrap">{text}</span>
</span>

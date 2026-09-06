<script lang="ts">
  let { size = 10 }: { size?: number } = $props()
</script>

<span
  class="inline-block shrink-0 animate-spin rounded-full border-current border-t-transparent"
  style="width:{size}px;height:{size}px;border-width:1.5px"
  aria-hidden="true"
></span>

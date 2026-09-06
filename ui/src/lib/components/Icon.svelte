<script lang="ts">
  export type IconName =
    | 'connections'
    | 'providers'
    | 'groups'
    | 'monitoring'
    | 'configurations'
    | 'collapse'
    | 'expand'
    | 'chevron'
    | 'search'
    | 'sun'
    | 'moon'
    | 'signout';

  let { name, size = 15 }: { name: IconName; size?: number } = $props();

  const paths: Record<IconName, string> = {
    connections:
      '<path d="M3 6h13m0 0-3-3m3 3-3 3M17 14H4m0 0 3-3m-3 3 3 3"/>',
    providers:
      '<rect x="3" y="3.5" width="14" height="5.5" rx="1"/><rect x="3" y="11" width="14" height="5.5" rx="1"/><path d="M6 6.25h.01M6 13.75h.01"/>',
    groups:
      '<path d="m10 2.5 7 4-7 4-7-4 7-4Z"/><path d="m3 10.5 7 4 7-4"/><path d="m3 14.5 7 4 7-4"/>',
    monitoring: '<path d="M2 10h4l2.5-6 3 12 2.5-6H18"/>',
    configurations:
      '<circle cx="10" cy="10" r="2.5"/><path d="M10 1.5v2.5M10 16v2.5M18.5 10h-2.5M3.5 10H1M16 4l-1.8 1.8M5.8 12.2 4 14M16 16l-1.8-1.8M5.8 7.8 4 6"/>',
    collapse: '<path d="m8.5 4-5 6 5 6M15.5 4l-5 6 5 6"/>',
    expand: '<path d="m5.5 4 5 6-5 6M12.5 4l5 6-5 6"/>',
    chevron: '<path d="m5 8 5 5 5-5"/>',
    search: '<circle cx="9" cy="9" r="5.5"/><path d="m13.5 13.5 3 3"/>',
    sun: '<circle cx="10" cy="10" r="3.5"/><path d="M10 1.5v2M10 16.5v2M18.5 10h-2M3.5 10h-2M16 4l-1.4 1.4M5.4 14.6 4 16M16 16l-1.4-1.4M5.4 5.4 4 4"/>',
    moon: '<path d="M16.5 12.5A7 7 0 0 1 7.5 3.5a7 7 0 1 0 9 9Z"/>',
    signout:
      '<path d="M12 3H5a1.5 1.5 0 0 0-1.5 1.5v11A1.5 1.5 0 0 0 5 17h7M13 6.5 16.5 10 13 13.5M16.5 10H7.5"/>',
  };
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 20 20"
  fill="none"
  stroke="currentColor"
  stroke-width="1.5"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  {@html paths[name]}
</svg>

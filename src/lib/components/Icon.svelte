<script lang="ts">
  import { HugeiconsIcon, type IconSvgElement } from "@hugeicons/svelte";

  let { icon, size = 16, class: className = "" }: { icon: IconSvgElement; size?: number; class?: string } = $props();
</script>

<!-- HugeiconsIcon reads `icon` only on mount, so re-mount when it changes. Icons are decorative; labels live on the buttons. -->
{#key icon}
  <HugeiconsIcon {icon} {size} strokeWidth={1.5} class={className} aria-hidden="true" />
{/key}

<script lang="ts">
  // The mark: one envelope in front of another. Many accounts, one inbox. The front one is filled, as unread mail is marked.
  let { size = 32, gap = "var(--color-canvas)" }: { size?: number; gap?: string } = $props();
</script>

<svg width={size} height={size} viewBox="0 0 32 32" fill="none" aria-hidden="true">
  <rect x="10" y="4" width="19" height="14" rx="3" stroke="currentColor" stroke-width="2" />
  <path d="M10.5 6.5 19.5 12.5 28.5 6.5" stroke="currentColor" stroke-width="2" stroke-linejoin="round" />
  <!-- The stroke, painted under the fill, cuts a gap between the two envelopes in whatever color is behind. -->
  <rect x="2" y="12" width="21" height="16" rx="3.5" fill="var(--color-accent)" stroke={gap} stroke-width="3" paint-order="stroke" />
  <path
    d="M5 15.5 12.5 20.5 20 15.5"
    stroke="var(--color-on-accent)"
    stroke-width="2"
    stroke-linecap="round"
    stroke-linejoin="round"
  />
</svg>

<script lang="ts">
  // The mark: an inbox tray, filled as unread mail is marked. One line with a dip is where mail lands.
  let { size = 32 }: { size?: number } = $props();
</script>

<svg width={size} height={size} viewBox="0 0 32 32" fill="none" aria-hidden="true">
  <rect x="3" y="4" width="26" height="24" rx="7" fill="var(--color-accent)" />
  <path
    d="M3 17.5H10l2 3.5h8l2-3.5h7"
    stroke="var(--color-on-accent)"
    stroke-width="2.25"
    stroke-linejoin="round"
  />
</svg>

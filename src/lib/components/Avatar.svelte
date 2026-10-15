<script lang="ts">
  let { name, color, size = 28 }: { name: string; color?: string; size?: number } = $props();

  const initials = $derived(
    name.split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0].toUpperCase()).join(""),
  );
  // Same name, same color, every time.
  const hue = $derived([...name].reduce((h, c) => (h * 31 + c.charCodeAt(0)) % 360, 0));
</script>

<span
  aria-hidden="true"
  class="grid shrink-0 place-items-center rounded-full font-semibold text-ink"
  style:width="{size}px"
  style:height="{size}px"
  style:font-size="{Math.round(size * 0.4)}px"
  style:background={color ?? `oklch(0.45 0.09 ${hue})`}
>
  {initials}
</span>

<script lang="ts">
  import { onMount } from "svelte";
  import { rise } from "$lib/motion";

  // A swatch grid for text color or highlight. Arrow keys move, Enter picks, Esc closes (handled here, so compose stays).
  let {
    label,
    colors,
    none,
    current,
    onpick,
    onclose,
  }: { label: string; colors: string[]; none: string; current: string | null; onpick: (c: string | null) => void; onclose: () => void } =
    $props();

  const all = $derived([null, ...colors]);
  let grid = $state<HTMLElement>();
  let at = $state(0);

  // Opens on the color in use, focused, so the arrow keys start from there.
  onMount(() => {
    at = Math.max(0, all.indexOf(current));
    grid?.querySelectorAll<HTMLElement>("button")[at]?.focus();
  });

  function keydown(e: KeyboardEvent) {
    const step = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: 5, ArrowUp: -5 }[e.key];
    if (step) {
      // The "none" button is a row of its own above the swatches.
      at = at === 0 && step === 5 ? 1 : at > 0 && at <= 5 && step === -5 ? 0 : (at + step + all.length) % all.length;
      grid?.querySelectorAll<HTMLElement>("button")[at]?.focus();
    } else if (e.key === "Escape") onclose();
    else return;
    e.preventDefault();
  }
</script>

<svelte:window onmousedown={(e) => !grid?.contains(e.target as Node) && onclose()} />

<!-- svelte-ignore a11y_no_noninteractive_element_interactions (a roving-tabindex group: its arrow keys live here) -->
<div
  bind:this={grid}
  role="group"
  aria-label={label}
  transition:rise
  onkeydown={keydown}
  class="shadow-float absolute top-full z-30 mt-1 grid w-max grid-cols-5 gap-1.5 rounded-xl border border-line bg-float p-2"
>
  {#each all as c, i (c)}
    <button
      type="button"
      tabindex={i === at ? 0 : -1}
      aria-label={c ?? none}
      title={c ?? none}
      aria-pressed={c === current}
      class="grid size-6 place-items-center rounded-md border border-line transition-transform duration-100 active:scale-90 aria-pressed:ring-2 aria-pressed:ring-accent {c ? '' : 'col-span-5 w-full text-caption text-ink-muted hover:bg-ink/5'}"
      style:background-color={c}
      onclick={() => onpick(c)}
    >
      {#if !c}{none}{/if}
    </button>
  {/each}
</div>

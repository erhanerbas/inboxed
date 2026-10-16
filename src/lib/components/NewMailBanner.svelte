<script lang="ts">
  import { Cancel01Icon } from "@hugeicons/core-free-icons";
  import Icon from "./Icon.svelte";
  import { rise } from "$lib/motion";
  import { dismissNewMail, openNewMail, ui } from "$lib/state.svelte";

  const newest = $derived(ui.newMail[0]);
</script>

<!-- Always rendered, so screen readers announce new mail when it appears. -->
<div role="status" class="pointer-events-none fixed inset-x-0 top-4 z-30 flex justify-center px-4">
  {#if newest}
    <div
      transition:rise
      class="pointer-events-auto flex max-w-xl min-w-0 items-center gap-3 rounded-xl border border-line bg-raised py-2 pr-2 pl-4 shadow-[0_12px_32px_rgb(0_0_0/0.4)]"
    >
      <span class="size-2 shrink-0 rounded-full bg-accent" aria-hidden="true"></span>
      <p class="min-w-0 flex-1 truncate">
        <span class="font-semibold">{newest.from}</span>
        <span class="text-ink-muted">{newest.subject}</span>
        {#if ui.newMail.length > 1}<span class="text-caption text-ink-muted">· {ui.newMail.length - 1} more</span>{/if}
      </p>
      <button class="btn-primary shrink-0" onclick={openNewMail}>Open</button>
      <button class="icon-btn shrink-0" aria-label="Dismiss" title="Dismiss" onclick={dismissNewMail}>
        <Icon icon={Cancel01Icon} size={14} />
      </button>
    </div>
  {/if}
</div>

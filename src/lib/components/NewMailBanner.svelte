<script lang="ts">
  import { Cancel01Icon } from "@hugeicons/core-free-icons";
  import Icon from "./Icon.svelte";
  import { rise } from "$lib/motion";
  import { dismissNewMail, openNewMail, ui } from "$lib/state.svelte";

  const newest = $derived(ui.newMail[0]);
  // Conversations, not messages: three replies in one thread are one more thing to read.
  const more = $derived(new Set(ui.newMail.map((m) => m.threadId)).size - 1);
</script>

<!-- Always rendered, so screen readers announce new mail when it appears. Top right, below the toolbars, where
     macOS puts its own notifications: it never covers the window's drag areas or the search field. -->
<div role="status" class="pointer-events-none fixed top-16 right-6 z-30 flex max-w-[calc(100vw-3rem)] justify-end">
  {#if newest}
    <div
      transition:rise
      class="pointer-events-auto flex max-w-xl min-w-0 items-center gap-3 rounded-xl border border-line bg-raised py-2 pr-2 pl-4 shadow-[0_12px_32px_rgb(0_0_0/0.4)]"
    >
      <span class="size-2 shrink-0 rounded-full bg-accent" aria-hidden="true"></span>
      <p class="min-w-0 flex-1 truncate">
        <span class="font-semibold">{newest.from}</span>
        <span class="text-ink-muted">{newest.subject}</span>
        {#if more > 0}<span class="text-caption text-ink-muted">· {more} more</span>{/if}
      </p>
      <button class="btn-primary shrink-0" onclick={openNewMail}>Open</button>
      <button class="icon-btn shrink-0" aria-label="Dismiss" title="Dismiss" onclick={dismissNewMail}>
        <Icon icon={Cancel01Icon} size={14} />
      </button>
    </div>
  {/if}
</div>

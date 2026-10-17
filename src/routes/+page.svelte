<script lang="ts">
  import AddAccount from "$lib/components/AddAccount.svelte";
  import Compose from "$lib/components/Compose.svelte";
  import NewMailBanner from "$lib/components/NewMailBanner.svelte";
  import MessageList from "$lib/components/MessageList.svelte";
  import RemoveAccount from "$lib/components/RemoveAccount.svelte";
  import Settings from "$lib/components/Settings.svelte";
  import Sidebar from "$lib/components/Sidebar.svelte";
  import ThreadView from "$lib/components/ThreadView.svelte";
  import { runShortcut } from "$lib/menus";
  import { compose, data, showList, start, ui } from "$lib/state.svelte";
  import { startTheme } from "$lib/theme.svelte";
  import { onMount } from "svelte";

  const isMac = typeof document !== "undefined" && document.documentElement.dataset.platform === "mac";

  onMount(() => {
    startTheme();
    start();
  });
  // The list follows the view and the search query.
  $effect(() => {
    void [ui.accountId, ui.mailboxKey, ui.query];
    showList();
  });

  function onkeydown(e: KeyboardEvent) {
    const mod = isMac ? e.metaKey : e.ctrlKey;
    const key = e.key.toLowerCase();
    if (mod && !e.shiftKey && !e.altKey && key === "n") {
      e.preventDefault();
      compose();
      return;
    }
    if (mod && !e.shiftKey && !e.altKey && key === ",") {
      e.preventDefault();
      ui.settingsOpen = true;
      return;
    }
    // Thread commands (reply, archive, ⌫ …) never fire while typing or inside compose and dialogs.
    if ((e.target as HTMLElement).closest("input, textarea, select, [contenteditable], form, dialog")) return;
    const selected = data.list.find((t) => t.id === ui.threadId) ?? data.thread ?? undefined;
    if (runShortcut(e, selected)) e.preventDefault();
  }

  // The webview's own menu (Reload, Inspect) doesn't belong in a shipped app. Text you can select keeps
  // the system menu for Copy and Look Up; development builds keep Inspect.
  function oncontextmenu(e: MouseEvent) {
    if (import.meta.env.DEV || e.defaultPrevented) return;
    if (!(e.target as HTMLElement).closest("input, textarea, .selectable")) e.preventDefault();
  }
</script>

<svelte:window {onkeydown} {oncontextmenu} />

<div class="flex h-screen overflow-hidden">
  <Sidebar />
  <div class="sidebar-material w-[26rem] shrink-0 border-l border-line">
    <MessageList />
  </div>
  <main class="min-w-0 flex-1 border-l border-line bg-canvas">
    {#key ui.threadId}
      <ThreadView />
    {/key}
  </main>
</div>

<NewMailBanner />

<!-- Every new message re-mounts the panel, so focus and its state start fresh; its transition is global, so it
     animates although its block is the {#key}'s. -->
{#key ui.composeKey}
  {#if ui.composing}<Compose />{/if}
{/key}
{#if ui.addingAccount}
  <AddAccount />
{/if}
{#if ui.removingAccount}
  <RemoveAccount />
{/if}
{#if ui.settingsOpen}
  <Settings />
{/if}

<!-- Always rendered, so screen readers announce the message when it appears. -->
<div role="status" class="pointer-events-none fixed inset-x-0 bottom-5 z-30 flex justify-center px-4">
  {#if ui.error}
    <p
      class="shadow-float pointer-events-auto flex max-w-xl items-center gap-4 rounded-2xl border border-line bg-float py-2 pr-2 pl-4"
    >
      {ui.error}
      <button class="btn-primary shrink-0" onclick={start}>Try again</button>
    </p>
  {/if}
</div>

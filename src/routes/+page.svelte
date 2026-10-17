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
  import { SidebarLeftIcon } from "@hugeicons/core-free-icons";
  import Icon from "$lib/components/Icon.svelte";
  import { loadFonts, saveSettings, settings } from "$lib/settings.svelte";
  import { startTheme } from "$lib/theme.svelte";
  import { onMount } from "svelte";

  const isMac = typeof document !== "undefined" && document.documentElement.dataset.platform === "mac";

  onMount(() => {
    startTheme();
    loadFonts();
    start();
  });
  // The list follows the view and the search query.
  $effect(() => {
    void [ui.accountId, ui.mailboxKey, ui.query];
    showList();
  });

  function toggleSidebar() {
    settings.sidebarHidden = !settings.sidebarHidden;
    saveSettings();
  }

  function onkeydown(e: KeyboardEvent) {
    const mod = isMac ? e.metaKey : e.ctrlKey;
    const key = e.key.toLowerCase();
    // ⌃⌘S, as in Mail and Finder (Ctrl+Shift+S elsewhere).
    if (isMac ? e.metaKey && e.ctrlKey && !e.shiftKey && !e.altKey && key === "s" : e.ctrlKey && e.shiftKey && key === "s") {
      e.preventDefault();
      toggleSidebar();
      return;
    }
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
  <!-- The rail and the list fold away together; inert while hidden, so Tab never lands in them. -->
  <div
    class="shrink-0 overflow-hidden transition-[width] duration-200 ease-out motion-reduce:transition-none"
    style:width={settings.sidebarHidden ? "0" : "29.5rem"}
    inert={settings.sidebarHidden}
  >
    <div class="flex h-full w-[29.5rem]">
      <Sidebar />
      <div class="sidebar-material w-[26rem] shrink-0">
        <MessageList />
      </div>
    </div>
  </div>
  <main class="relative min-w-0 flex-1 bg-canvas {settings.sidebarHidden ? '' : 'border-l border-line'}">
    <!-- With the sidebar hidden, the window's traffic lights are over the reading pane: the button moves past them. -->
    <button
      class="icon-btn absolute top-2.5 z-10"
      style:left={settings.sidebarHidden ? "5.5rem" : "0.75rem"}
      aria-label={settings.sidebarHidden ? "Show sidebar" : "Hide sidebar"}
      title="{settings.sidebarHidden ? 'Show' : 'Hide'} sidebar ({isMac ? '⌃⌘S' : 'Ctrl+Shift+S'})"
      onclick={toggleSidebar}
    >
      <Icon icon={SidebarLeftIcon} size={17} />
    </button>
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

<script lang="ts">
  import { PencilEdit02Icon, SidebarLeft01Icon } from "@hugeicons/core-free-icons";
  import AddAccount from "$lib/components/AddAccount.svelte";
  import Compose from "$lib/components/Compose.svelte";
  import Icon from "$lib/components/Icon.svelte";
  import MessageList from "$lib/components/MessageList.svelte";
  import Sidebar from "$lib/components/Sidebar.svelte";
  import ThreadView from "$lib/components/ThreadView.svelte";
  import { runShortcut } from "$lib/menus";
  import { compose, data, toggleSidebar, ui } from "$lib/state.svelte";

  const isMac = typeof document !== "undefined" && document.documentElement.dataset.platform === "mac";

  function onkeydown(e: KeyboardEvent) {
    const mod = isMac ? e.metaKey : e.ctrlKey;
    const key = e.key.toLowerCase();
    if (mod && !e.shiftKey && !e.altKey && key === "n") {
      e.preventDefault();
      compose();
      return;
    }
    if (key === "s" && (isMac ? e.metaKey && e.ctrlKey : e.ctrlKey && e.shiftKey)) {
      e.preventDefault();
      toggleSidebar();
      return;
    }
    // Thread commands (reply, archive, ⌫ …) never fire while typing or inside compose and dialogs.
    if ((e.target as HTMLElement).closest("input, textarea, select, [contenteditable], form, dialog")) return;
    if (runShortcut(e, data.threads.find((t) => t.id === ui.threadId))) e.preventDefault();
  }

  // The webview's own menu (Reload, Inspect) doesn't belong in a shipped app. Text you can select keeps
  // the system menu for Copy and Look Up; development builds keep Inspect.
  function oncontextmenu(e: MouseEvent) {
    if (import.meta.env.DEV || e.defaultPrevented) return;
    if (!(e.target as HTMLElement).closest("input, textarea, .selectable")) e.preventDefault();
  }
</script>

<svelte:window {onkeydown} {oncontextmenu} />

<!-- Fixed beside the traffic lights, so these never move when the sidebar collapses. -->
<div class="fixed top-0 left-[5.25rem] z-10 flex h-13 items-center gap-0.5">
  <button
    class="icon-btn"
    aria-label={ui.sidebarCollapsed ? "Show sidebar" : "Hide sidebar"}
    title="{ui.sidebarCollapsed ? 'Show' : 'Hide'} sidebar ({isMac ? '⌃⌘S' : 'Ctrl+Shift+S'})"
    onclick={toggleSidebar}
  >
    <Icon icon={SidebarLeft01Icon} size={17} />
  </button>
  <button class="icon-btn" aria-label="New message" title="New message ({isMac ? '⌘N' : 'Ctrl+N'})" onclick={() => compose()}>
    <Icon icon={PencilEdit02Icon} size={17} />
  </button>
</div>

<div class="flex h-screen overflow-hidden">
  <Sidebar />
  <div class="w-[22rem] shrink-0 border-l border-line bg-panel">
    <MessageList />
  </div>
  <main class="min-w-0 flex-1 border-l border-line bg-canvas">
    {#key ui.threadId}
      <ThreadView />
    {/key}
  </main>
</div>

{#if ui.composing}
  <Compose />
{/if}
{#if ui.addingAccount}
  <AddAccount />
{/if}

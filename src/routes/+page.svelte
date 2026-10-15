<script lang="ts">
  import Compose from "$lib/components/Compose.svelte";
  import MessageList from "$lib/components/MessageList.svelte";
  import Sidebar from "$lib/components/Sidebar.svelte";
  import ThreadView from "$lib/components/ThreadView.svelte";
  import { ui } from "$lib/state.svelte";

  function onkeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "n") {
      e.preventDefault();
      ui.composing = true;
    }
  }
</script>

<svelte:window {onkeydown} />

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

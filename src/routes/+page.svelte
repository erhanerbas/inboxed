<script lang="ts">
  import MessageList from "$lib/components/MessageList.svelte";
  import Sidebar from "$lib/components/Sidebar.svelte";
  import ThreadView from "$lib/components/ThreadView.svelte";
  import { ui } from "$lib/state.svelte";
</script>

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

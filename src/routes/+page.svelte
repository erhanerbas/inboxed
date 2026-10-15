<script lang="ts">
  import MessageList from "$lib/components/MessageList.svelte";
  import Sidebar from "$lib/components/Sidebar.svelte";
</script>

<div class="flex h-screen overflow-hidden">
  <Sidebar />
  <div class="w-[22rem] shrink-0 border-l border-line bg-panel">
    <MessageList />
  </div>
  <main class="min-w-0 flex-1 border-l border-line bg-canvas">
    <div data-tauri-drag-region class="h-13"></div>
  </main>
</div>

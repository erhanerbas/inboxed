<script lang="ts">
  import Sidebar from "$lib/components/Sidebar.svelte";
</script>

<div class="flex h-screen overflow-hidden">
  <Sidebar />
  <section class="w-[22rem] shrink-0 border-l border-line bg-panel">
    <div data-tauri-drag-region class="h-13"></div>
  </section>
  <main class="min-w-0 flex-1 border-l border-line bg-canvas">
    <div data-tauri-drag-region class="h-13"></div>
  </main>
</div>

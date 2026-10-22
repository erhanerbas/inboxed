<script lang="ts">
  import type { Contact } from "$lib/types";

  // The people a field or an @mention could mean. The field keeps focus (and handles the keys); this only shows them.
  let { id, items, active, onpick }: { id: string; items: Contact[]; active: number; onpick: (c: Contact) => void } = $props();
</script>

<ul
  {id}
  role="listbox"
  aria-label="Suggestions"
  class="shadow-float max-h-72 w-80 max-w-[calc(100vw-3rem)] overflow-y-auto rounded-xl border border-line bg-float p-1"
>
  {#each items as c, i (c.email)}
    <!-- mousedown, not click: the field mustn't lose focus (and its caret) first. -->
    <li
      id="{id}-{i}"
      role="option"
      aria-selected={i === active}
      class="flex cursor-default flex-col rounded-lg px-2.5 py-1.5 transition-colors duration-100 {i === active ? 'bg-selected' : 'hover:bg-ink/5'}"
      onmousedown={(e) => {
        e.preventDefault();
        onpick(c);
      }}
    >
      <span class="truncate">{c.name || c.email}</span>
      {#if c.name}<span class="truncate text-caption text-ink-muted">{c.email}</span>{/if}
    </li>
  {/each}
</ul>

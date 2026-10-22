<script lang="ts">
  import { matchContacts, pick, tokenAt } from "$lib/contacts";
  import type { Contact } from "$lib/types";
  import type { Snippet } from "svelte";
  import Suggestions from "./Suggestions.svelte";

  // To, Cc or Bcc: free text the engine parses, with the people you've mailed suggested for the piece being typed.
  let {
    label,
    value = $bindable(),
    contacts,
    exclude,
    input = $bindable(),
    end,
  }: { label: string; value: string; contacts: Contact[]; exclude: string[]; input?: HTMLInputElement; end?: Snippet } = $props();

  const id = $derived(`suggest-${label.toLowerCase()}`);
  let caret = $state(0);
  let open = $state(false);
  let active = $state(0);
  const items = $derived(open ? matchContacts(contacts, tokenAt(value, caret).query, exclude) : []);

  function typed(e: Event & { currentTarget: HTMLInputElement }) {
    caret = e.currentTarget.selectionStart ?? value.length;
    open = true;
    active = 0;
  }

  function choose(c: Contact) {
    const next = pick(value, caret, c);
    value = next.value;
    open = false;
    queueMicrotask(() => input?.setSelectionRange(next.caret, next.caret));
  }

  // The list's keys; everything else is the field's. Esc closes only the list (the panel's Esc sees it handled).
  function keydown(e: KeyboardEvent) {
    if (!items.length || e.isComposing || e.metaKey || e.ctrlKey || e.altKey) return; // ⌘Enter sends
    if (e.key === "ArrowDown" || e.key === "ArrowUp") active = (active + (e.key === "ArrowDown" ? 1 : -1) + items.length) % items.length;
    else if (e.key === "Enter" || e.key === "Tab") choose(items[active]);
    else if (e.key === "Escape") open = false;
    else return;
    e.preventDefault();
  }
</script>

<div class="relative">
  <label class="field">
    <span>{label}</span>
    <input
      bind:this={input}
      bind:value
      oninput={typed}
      onkeydown={keydown}
      onblur={() => (open = false)}
      autocomplete="off"
      spellcheck="false"
      role="combobox"
      aria-autocomplete="list"
      aria-expanded={items.length > 0}
      aria-controls={id}
      aria-activedescendant={items.length ? `${id}-${active}` : undefined}
    />
    {@render end?.()}
  </label>
  {#if items.length}
    <div class="absolute top-full left-18 z-30 -mt-1">
      <Suggestions {id} {items} {active} onpick={choose} />
    </div>
  {/if}
</div>

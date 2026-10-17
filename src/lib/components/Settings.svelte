<script lang="ts">
  import { ui } from "$lib/state.svelte";
  import { FONTS, setMailFont, settings } from "$lib/settings.svelte";

  let dialog = $state<HTMLDialogElement>();
  $effect(() => dialog?.showModal());
</script>

<!-- Choices apply as they're made: there's nothing to save, so the only button is Done. -->
<dialog
  bind:this={dialog}
  onclose={() => (ui.settingsOpen = false)}
  aria-labelledby="settings-title"
  class="m-auto w-[28rem] max-w-[calc(100vw-2rem)] shadow-float rounded-2xl border border-line bg-float p-6 text-ink backdrop:bg-ink/30"
>
  <h2 id="settings-title" class="text-heading">Settings</h2>

  <fieldset class="mt-5">
    <legend class="font-medium">Mail font</legend>
    <p class="mt-1 text-caption text-ink-muted">Every message is shown in this font, whatever the sender chose.</p>
    <div class="mt-3 flex flex-col gap-1">
      {#each FONTS as f (f.id)}
        <label
          class="flex cursor-default items-center gap-3 rounded-lg px-3 py-2 transition-colors duration-100 hover:bg-ink/5 has-checked:bg-selected"
        >
          <input
            type="radio"
            name="mail-font"
            value={f.id}
            checked={settings.mailFont === f.id}
            onchange={() => setMailFont(f.id)}
            class="accent-accent"
          />
          <span class="min-w-0 flex-1" style:font-family={f.family}>
            <span class="block text-read">{f.name}</span>
            <span class="block truncate text-ink-muted">Thanks for the notes. I'll send the draft on Monday.</span>
          </span>
        </label>
      {/each}
    </div>
  </fieldset>

  <div class="mt-6 flex justify-end">
    <button class="btn-primary" onclick={() => dialog?.close()}>Done</button>
  </div>
</dialog>

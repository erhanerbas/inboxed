<script lang="ts">
  import { data, removeAccount, ui } from "$lib/state.svelte";

  let dialog = $state<HTMLDialogElement>();
  $effect(() => dialog?.showModal());
  const account = $derived(data.accounts.find((a) => a.id === ui.removingAccount));

  function remove() {
    const id = ui.removingAccount;
    dialog?.close();
    if (id) removeAccount(id);
  }
</script>

<!-- A modal, so focus starts on Cancel: the first control, and the safe one. -->
<dialog
  bind:this={dialog}
  onclose={() => (ui.removingAccount = null)}
  aria-labelledby="remove-account-title"
  aria-describedby="remove-account-body"
  class="m-auto w-[26rem] max-w-[calc(100vw-2rem)] shadow-float rounded-2xl border border-line bg-float p-6 text-ink backdrop:bg-ink/30"
>
  <h2 id="remove-account-title" class="text-heading wrap-anywhere">Remove “{account?.name}”?</h2>
  <p id="remove-account-body" class="mt-2 text-ink-muted">
    Its mail is deleted from this computer. Nothing changes on the server, and you can add the account again later.
  </p>
  <div class="mt-6 flex justify-end gap-2">
    <button class="btn-secondary" onclick={() => dialog?.close()}>Cancel</button>
    <button class="btn-danger" onclick={remove}>Remove account</button>
  </div>
</dialog>

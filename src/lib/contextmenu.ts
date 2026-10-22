import type { Action } from "svelte/action";
import type { MenuEntry } from "./menus";

const inTauri = () => typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;

/** Pops up a native menu (a real macOS menu, not an HTML imitation). */
async function popup(entries: MenuEntry[], x: number, y: number) {
  const { Menu, MenuItem, PredefinedMenuItem } = await import("@tauri-apps/api/menu");
  const { LogicalPosition } = await import("@tauri-apps/api/dpi");
  const items = await Promise.all(
    entries.map((e) =>
      e === "separator"
        ? PredefinedMenuItem.new({ item: "Separator" })
        : MenuItem.new({ text: e.text, action: e.action, accelerator: e.accelerator, enabled: e.enabled ?? true }),
    ),
  );
  const menu = await Menu.new({ items });
  await menu.popup(new LogicalPosition(x, y));
}

/** A native menu under a button (the compose toolbar's •••). Outside Tauri there's no native menu: nothing opens. */
export function menuAt(el: HTMLElement, entries: MenuEntry[]) {
  if (!inTauri()) return;
  const r = el.getBoundingClientRect();
  popup(entries, r.left, r.bottom + 4).catch((err) => console.error("menu", err));
}

/**
 * `use:contextmenu={() => entries}` opens the menu on right-click, and from the keyboard
 * with Shift+F10 or the context-menu key (placed under the element).
 * Outside Tauri (plain browser) it does nothing and the browser's own menu shows.
 */
export const contextmenu: Action<HTMLElement, () => MenuEntry[]> = (node, getEntries) => {
  let entries = getEntries;
  const open = (x: number, y: number) => popup(entries(), x, y).catch((err) => console.error("context menu", err));

  function onContextMenu(e: MouseEvent) {
    if (!inTauri()) return;
    e.preventDefault();
    open(e.clientX, e.clientY);
  }
  function onKeyDown(e: KeyboardEvent) {
    if (!inTauri() || !(e.key === "ContextMenu" || (e.shiftKey && e.key === "F10"))) return;
    e.preventDefault();
    const r = node.getBoundingClientRect();
    open(r.left + 8, r.bottom);
  }

  node.addEventListener("contextmenu", onContextMenu);
  node.addEventListener("keydown", onKeyDown);
  return {
    update: (next) => (entries = next),
    destroy() {
      node.removeEventListener("contextmenu", onContextMenu);
      node.removeEventListener("keydown", onKeyDown);
    },
  };
};

import { getCurrentWindow } from "@tauri-apps/api/window";

// app.html sets data-theme before the first paint (stored choice, else the system's); this keeps it current.
const KEY = "inboxed.theme";
const system = typeof matchMedia === "function" ? matchMedia("(prefers-color-scheme: dark)") : undefined;

export const theme = $state({
  current: (typeof document !== "undefined" && document.documentElement.dataset.theme === "dark" ? "dark" : "light") as
    | "light"
    | "dark",
  // What the person picked in Settings (or with the rail's button); "system" until they pick.
  choice: "system" as "system" | "light" | "dark",
});

function apply(next: "light" | "dark", remember: boolean) {
  theme.current = next;
  document.documentElement.dataset.theme = next;
  if (remember) {
    try {
      localStorage.setItem(KEY, next);
    } catch {
      // Not remembered; the switch still works for this session.
    }
  }
  // The window's appearance (traffic lights, sidebar vibrancy) follows the app's choice once there is one. Until then
  // it stays on the system's (null): pinning it would also pin the webview's prefers-color-scheme, and the system
  // listener below would never hear a change.
  if ("__TAURI_INTERNALS__" in window) getCurrentWindow().setTheme(remember || stored() ? next : null).catch(console.error);
}

const stored = () => {
  try {
    return localStorage.getItem(KEY);
  } catch {
    return null;
  }
};

/** Follows the system until the person picks a theme; after that their choice sticks. */
export function startTheme() {
  const saved = stored();
  if (saved === "light" || saved === "dark") theme.choice = saved;
  apply(theme.current, false);
  system?.addEventListener("change", (e) => {
    if (!stored()) apply(e.matches ? "dark" : "light", false);
  });
}

export const toggleTheme = () => chooseTheme(theme.current === "dark" ? "light" : "dark");

/** Settings' Appearance: a fixed theme, or back to following the system. */
export function chooseTheme(choice: "system" | "light" | "dark") {
  theme.choice = choice;
  if (choice !== "system") return apply(choice, true);
  try {
    localStorage.removeItem(KEY);
  } catch {
    // Still follows the system for this session.
  }
  apply(system?.matches ? "dark" : "light", false);
}

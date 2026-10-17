import { getCurrentWindow } from "@tauri-apps/api/window";

// app.html sets data-theme before the first paint (stored choice, else the system's); this keeps it current.
const KEY = "inboxed.theme";
const system = typeof matchMedia === "function" ? matchMedia("(prefers-color-scheme: dark)") : undefined;

export const theme = $state({
  current: (typeof document !== "undefined" && document.documentElement.dataset.theme === "dark" ? "dark" : "light") as
    | "light"
    | "dark",
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
  // The window's own appearance (traffic lights, sidebar vibrancy) follows the app's, not the system's.
  if ("__TAURI_INTERNALS__" in window) getCurrentWindow().setTheme(next).catch(console.error);
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
  apply(theme.current, false);
  system?.addEventListener("change", (e) => {
    if (!stored()) apply(e.matches ? "dark" : "light", false);
  });
}

export const toggleTheme = () => apply(theme.current === "dark" ? "light" : "dark", true);

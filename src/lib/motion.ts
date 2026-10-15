import { cubicOut } from "svelte/easing";
import { prefersReducedMotion } from "svelte/motion";
import { fade, fly } from "svelte/transition";

/** Rises into place and leaves the way it came. Cross-fades instead when the user prefers reduced motion. */
export function rise(node: Element) {
  return prefersReducedMotion.current
    ? fade(node, { duration: 150 })
    : fly(node, { y: 16, duration: 240, easing: cubicOut });
}

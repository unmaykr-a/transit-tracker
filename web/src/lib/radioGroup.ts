import type { KeyboardEvent } from "react";

// The ARIA radio group pattern for button-based radios: one Tab stop per group
// (the checked option, or the first), and arrow keys move to and select the
// next option.

const STEP: Record<string, number> = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 };

export function radioKeyDown(e: KeyboardEvent<HTMLElement>) {
  const group = e.currentTarget.closest('[role="radiogroup"]');
  if (!group) return;
  const radios = [...group.querySelectorAll<HTMLElement>('[role="radio"]:not([disabled])')];
  const i = radios.indexOf(e.currentTarget);
  let next: HTMLElement | undefined;
  if (e.key in STEP) next = radios[(i + STEP[e.key]! + radios.length) % radios.length];
  else if (e.key === "Home") next = radios[0];
  else if (e.key === "End") next = radios.at(-1);
  if (!next) return;
  e.preventDefault();
  next.focus();
  next.click();
}

/** tabIndex for option `index` of a group: only the checked one (or the first, when none is) is a Tab stop. */
export function radioTabIndex(checked: boolean, index: number, anyChecked: boolean): 0 | -1 {
  return checked || (!anyChecked && index === 0) ? 0 : -1;
}

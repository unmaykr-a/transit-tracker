import type { KeyboardEvent } from "react";
import { describe, expect, it } from "vitest";

import { radioKeyDown, radioTabIndex } from "./radioGroup";

function group() {
  document.body.innerHTML = `<div role="radiogroup">
    <button role="radio" id="a"></button><button role="radio" id="b" disabled></button><button role="radio" id="c"></button>
  </div>`;
  const clicked: string[] = [];
  for (const b of document.querySelectorAll("button")) b.addEventListener("click", () => clicked.push(b.id));
  const press = (id: string, key: string) => {
    let prevented = false;
    radioKeyDown({ key, currentTarget: document.getElementById(id)!, preventDefault: () => (prevented = true) } as unknown as KeyboardEvent<HTMLElement>);
    return prevented;
  };
  return { clicked, press };
}

describe("radio groups", () => {
  it("arrow keys move to and select the next enabled option, wrapping around", () => {
    const { clicked, press } = group();
    expect(press("a", "ArrowRight")).toBe(true);
    expect(document.activeElement?.id).toBe("c"); // the disabled one is skipped
    press("c", "ArrowDown");
    press("a", "ArrowLeft");
    press("c", "Home");
    expect(clicked).toEqual(["c", "a", "c", "a"]);
    expect(press("a", "Enter")).toBe(false);
  });

  it("has one Tab stop: the checked option, or the first when none is", () => {
    expect([radioTabIndex(false, 0, true), radioTabIndex(true, 1, true)]).toEqual([-1, 0]);
    expect([radioTabIndex(false, 0, false), radioTabIndex(false, 1, false)]).toEqual([0, -1]);
  });
});

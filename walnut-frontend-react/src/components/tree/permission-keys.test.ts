import { describe, expect, it } from "vitest";
import { allKeys, togglePermission, withAncestors } from "./permission-keys";
const tree = [
  { id: 1, children: [{ id: 2, children: [{ id: 3 }, { id: 4 }] }, { id: 5 }] },
];
describe("role permission API contract", () => {
  it("submits half-checked ancestors without granting sibling permissions", () => {
    expect(withAncestors(tree, [3]).toSorted()).toEqual([1, 2, 3]);
  });
  it("does not duplicate explicit ancestors", () => {
    expect(withAncestors(tree, [1, 2, 3]).toSorted()).toEqual([1, 2, 3]);
  });
  it("selects and removes an associated subtree without touching siblings", () => {
    expect(
      togglePermission(tree, [5], tree[0]!.children[0]!, true, true).toSorted(),
    ).toEqual([2, 3, 4, 5]);
    expect(
      togglePermission(tree, [2, 3, 4, 5], tree[0]!.children[0]!, false, true),
    ).toEqual([5]);
  });
  it("handles IDs as opaque API keys", () => {
    expect(
      allKeys([{ id: "9007199254740993", children: [{ id: "2" }] }]),
    ).toEqual(["9007199254740993", "2"]);
  });
  it("keeps independent child menus separate while selecting a menu and its buttons", () => {
    const node = {
      id: 1,
      children: [
        { id: 2, menuType: "C" },
        { id: 3, menuType: "F" },
      ],
    };
    expect(togglePermission([node], [], node, true, false)).toEqual([1, 3]);
  });
});

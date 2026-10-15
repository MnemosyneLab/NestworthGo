import { beforeEach, expect, it } from "vitest";
import { useHoldingGroupStore } from "./holdings";

beforeEach(() => useHoldingGroupStore.getState().reset());

it("starts expanded and toggles groups independently per scope", () => {
  const store = useHoldingGroupStore.getState();
  expect(store.collapsed).toEqual({ portfolio: [], account: [] });
  store.toggleGroup("portfolio", "stock");
  store.toggleGroup("portfolio", "crypto");
  store.toggleGroup("account", "stock");
  expect(useHoldingGroupStore.getState().collapsed).toEqual({ portfolio: ["stock", "crypto"], account: ["stock"] });
  store.toggleGroup("portfolio", "stock");
  expect(useHoldingGroupStore.getState().collapsed.portfolio).toEqual(["crypto"]);
  store.setCollapsed("account", []);
  expect(useHoldingGroupStore.getState().collapsed.account).toEqual([]);
});

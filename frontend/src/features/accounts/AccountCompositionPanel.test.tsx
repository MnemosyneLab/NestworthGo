import { describe, expect, it } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AccountCompositionPanel } from "./AccountCompositionPanel";
import type { AccountCompositionDTO, AccountCompositionItemDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

type CompositionFixture = Omit<AccountCompositionDTO, "types" | "details" | "detailSlices"> & { types: AccountCompositionItemDTO[]; details: AccountCompositionItemDTO[]; detailSlices: AccountCompositionItemDTO[] };

function item(key: string, typeKey = "stock", amount = "1", shareBps = 10000): AccountCompositionItemDTO {
  return { key, typeKey, label: `Original ${key}`, detailRefs: [key], baseAmountExact: amount, shareBps, nativeAmount: amount, nativeCurrency: "USD", valuedCount: 1, missingCount: 0 };
}
function projection(count = 1): CompositionFixture {
  const details = Array.from({ length: count }, (_, index) => item(`detail-${index}`, "stock", "1", count === 1 ? 10000 : count === 10 ? 1000 : count === 11 ? index === 0 ? 910 : 909 : 500));
  const remainder = { ...item("account-composition:remainder", "", String(count - 10), count === 11 ? 909 : 5000), detailRefs: details.slice(10).map((row) => row.key), nativeAmount: "", nativeCurrency: "", valuedCount: count - 10 };
  return { currency: "USD", totalExact: String(count), complete: true, status: count ? "ready" : "empty", valuedCount: count, missingCount: 0,
    types: count ? [{ ...item("account-composition:type:stock", "stock", String(count)), nativeAmount: "", nativeCurrency: "", detailRefs: details.map((row) => row.key), valuedCount: count }] : [],
    details, detailSlices: count > 10 ? [...details.slice(0, 10), remainder] : details };
}

describe("account composition panel", () => {
  it("defaults to type and shows a single-category 100% chart", () => {
    render(<AccountCompositionPanel composition={projection()} />);
    expect(screen.getByRole("button", { name: "By type" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("composition-slice-account-composition:type:stock")).toHaveTextContent("100.0%");
    expect(screen.getByTestId("composition-chart")).toBeInTheDocument();
  });

  it("shows every present type without generic top-five collapsing", () => {
    const composition = projection(10);
    const typeKeys = ["cash", "stock", "etf", "mutual_fund", "crypto", "bond", "precious_metal", "bank_investment_product", "other", "unknown"];
    composition.details = composition.details.map((row, index) => ({ ...row, typeKey: typeKeys[index] }));
    composition.detailSlices = composition.details;
    composition.types = typeKeys.map((key, index) => ({ ...item(`account-composition:type:${key}`, key, "1", 1000), nativeAmount: "", nativeCurrency: "", detailRefs: [composition.details[index].key] }));
    render(<AccountCompositionPanel composition={composition} />);
    for (const row of composition.types) expect(screen.getByTestId(`composition-slice-${row.key}`)).toBeInTheDocument();
    expect(screen.getByTestId("composition-slice-account-composition:type:bank_investment_product")).toHaveTextContent("Investment products & term deposits");
    expect(screen.getByTestId("composition-slice-account-composition:type:unknown")).toHaveTextContent("Unknown type");
    expect(screen.queryByTestId("composition-slice-other")).not.toBeInTheDocument();
  });

  it.each([0, 1, 10, 11, 20])("shows top ten and expands all %i originals without duplicating the remainder", async (count) => {
    const user = userEvent.setup();
    const composition = projection(count);
    render(<AccountCompositionPanel composition={composition} />);
    await user.click(screen.getByRole("button", { name: "By detail" }));
    if (!count) { expect(screen.getByRole("status")).toHaveTextContent("No positive valued balance"); return; }
    expect(screen.getAllByTestId(/^composition-slice-/)).toHaveLength(count > 10 ? 11 : count);
    if (count > 10) {
      expect(screen.getByTestId("composition-slice-account-composition:remainder")).toHaveTextContent(`Remaining ${count - 10} items`);
      const expand = screen.getByRole("button", { name: `Show all ${count} items` });
      expect(expand).toHaveAttribute("aria-expanded", "false");
      await user.click(expand);
      expect(screen.getAllByTestId(/^composition-slice-/)).toHaveLength(count);
      expect(screen.queryByTestId("composition-slice-account-composition:remainder")).not.toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: "Show top 10" }));
      expect(screen.getAllByTestId(/^composition-slice-/)).toHaveLength(11);
    } else expect(screen.queryByRole("button", { name: /Show all/ })).not.toBeInTheDocument();
    await user.click(screen.getByText("View data table"));
    const table = screen.getByRole("table");
    expect(within(table).getAllByRole("row")).toHaveLength(count + 1);
    expect(within(table).queryByText(/Remaining/)).not.toBeInTheDocument();
    for (const row of composition.details) expect(within(table).getByText(row.label)).toBeInTheDocument();
  });

  it("preserves mode on refresh, clears selection and hover, and resets for a new account key", async () => {
    const user = userEvent.setup();
    const composition = projection(20);
    const { rerender } = render(<AccountCompositionPanel key="first" composition={composition} updatedAt={1} />);
    const detailToggle = screen.getByRole("button", { name: "By detail" });
    detailToggle.focus();
    await user.keyboard("{Enter}");
    const slice = screen.getByTestId("composition-slice-detail-0");
    await user.click(slice);
    await user.hover(slice);
    expect(slice).toHaveAttribute("aria-pressed", "true");
    rerender(<AccountCompositionPanel key="first" composition={{ ...composition }} updatedAt={2} />);
    expect(screen.getByRole("button", { name: "By detail" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByTestId("composition-slice-detail-0")).toHaveAttribute("aria-pressed", "false");
    rerender(<AccountCompositionPanel key="second" composition={composition} updatedAt={2} />);
    expect(screen.getByRole("button", { name: "By type" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.queryByRole("button", { name: "Show top 10" })).not.toBeInTheDocument();
  });

  it.each(["unavailable", "empty", "invalid"])("provides original evidence without a pie for %s", async (status) => {
    const composition = projection();
    composition.status = status;
    if (status === "unavailable") {
      composition.complete = false;
      composition.details[0] = { ...composition.details[0], baseAmountExact: "", nativeAmount: "", valuedCount: 0, missingCount: 1 };
      composition.types[0] = { ...composition.types[0], baseAmountExact: "", valuedCount: 0, missingCount: 1 };
    } else {
      const amount = status === "empty" ? "0" : "-1";
      composition.details[0] = { ...composition.details[0], baseAmountExact: amount, nativeAmount: amount, shareBps: 0 };
      composition.types[0] = { ...composition.types[0], baseAmountExact: amount, shareBps: 0 };
      composition.totalExact = amount;
    }
    composition.detailSlices = composition.details;
    render(<AccountCompositionPanel composition={composition} />);
    expect(screen.queryByTestId("composition-chart")).not.toBeInTheDocument();
    await userEvent.click(screen.getByText("View data table"));
    const table = screen.getByRole("table");
    expect(within(table).getByText("Original detail-0")).toBeInTheDocument();
    if (status === "unavailable") expect(within(table).queryByText("$0.00")).not.toBeInTheDocument();
  });

  it("keeps types, top-ten details and expansion available when all values are missing", async () => {
    const composition = projection(20);
    composition.status = "unavailable";
    composition.complete = false;
    composition.totalExact = undefined;
    composition.valuedCount = 0;
    composition.missingCount = 20;
    const missing = (row: AccountCompositionItemDTO) => ({ ...row, baseAmountExact: "", shareBps: 0, valuedCount: 0, missingCount: row.detailRefs?.length ?? 1 });
    composition.types = composition.types.map(missing);
    composition.details = composition.details.map(missing);
    composition.detailSlices = composition.detailSlices.map(missing);
    render(<AccountCompositionPanel composition={composition} />);
    expect(screen.getByTestId("account-composition-row-account-composition:type:stock")).toHaveTextContent("Stock");
    await userEvent.click(screen.getByRole("button", { name: "By detail" }));
    expect(screen.getAllByTestId(/^account-composition-row-/)).toHaveLength(11);
    await userEvent.click(screen.getByRole("button", { name: "Show all 20 items" }));
    expect(screen.getAllByTestId(/^account-composition-row-/)).toHaveLength(20);
    expect(screen.queryByTestId("composition-chart")).not.toBeInTheDocument();
  });

  it("keeps tiny exact components visible even when money display rounds to zero", async () => {
    const composition = projection(2);
    composition.totalExact = "0.0001";
    composition.details = [item("a", "etf", "0.00005", 5000), item("b", "etf", "0.00005", 5000)];
    composition.detailSlices = composition.details;
    composition.types = [item("account-composition:type:etf", "etf", "0.0001")];
    render(<AccountCompositionPanel composition={composition} />);
    await userEvent.click(screen.getByRole("button", { name: "By detail" }));
    expect(screen.getByTestId("composition-slice-a")).toHaveTextContent("50.0%");
    expect(screen.getByTestId("composition-slice-b")).toHaveTextContent("50.0%");
  });
});

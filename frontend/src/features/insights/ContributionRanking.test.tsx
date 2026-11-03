import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { ContributionRowDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { ContributionRanking, rankedContributions } from "./ContributionRanking";

const rows = [
  { key: "gain", label: "Gain", amount: { amount: "50", currency: "USD" } },
  { key: "loss", label: "Loss", amount: { amount: "-100", currency: "USD" } },
  { key: "zero", label: "Zero", amount: { amount: "0", currency: "USD" } },
  { key: "missing", label: "Missing" },
] as ContributionRowDTO[];

describe("contribution ranking", () => {
  it("orders by exact magnitude and keeps zero distinct from missing", () => {
    expect(rankedContributions(rows, "all", "10").map(row => row.key)).toEqual(["loss", "gain", "zero"]);
    expect(rankedContributions(rows, "positive", "all").map(row => row.key)).toEqual(["gain"]);
    expect(rankedContributions(rows, "negative", "all").map(row => row.key)).toEqual(["loss"]);
    const precise = [{ ...rows[0], key: "a", amount: { amount: "100000000000.0001", currency: "USD" } }, { ...rows[1], key: "b", amount: { amount: "-100000000000.0002", currency: "USD" } }];
    expect(rankedContributions(precise, "all", "10")[0].key).toBe("b");
  });

  it("uses one linear zero axis for both signs and opens details with keyboard and pointer", async () => {
    const user = userEvent.setup();
    const select = vi.fn();
    render(<ContributionRanking rows={rankedContributions(rows, "all", "all")} labelFor={row => row.label} onSelect={select} label="Main contributions" />);
    const bars = screen.getAllByTestId("contribution-bar");
    expect(bars[0]).toHaveStyle({ width: "50%", left: "0%" });
    expect(bars[1]).toHaveStyle({ width: "25%", left: "50%" });
    expect(bars[2]).toHaveStyle({ width: "0%" });
    await user.tab();
    expect(screen.getByRole("button", { name: /Loss/ })).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(select).toHaveBeenCalledWith("loss");
    fireEvent.click(screen.getByRole("button", { name: /Gain/ }));
    expect(select).toHaveBeenCalledWith("gain");
  });
});

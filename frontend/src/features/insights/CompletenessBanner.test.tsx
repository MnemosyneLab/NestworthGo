import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AvailabilityMarks, CompletenessBanner } from "./CompletenessBanner";

afterEach(cleanup);

describe("analysis completeness and rate coverage", () => {
  it("does not call complete amounts with an undefined rate a data problem", () => {
    const { container } = render(<><CompletenessBanner issues={[]} ratedDays={3} totalDays={4} /><AvailabilityMarks status="ok" ratedDays={3} totalDays={4} /></>);
    expect(container).toBeEmptyDOMElement();
  });

  it("still reports a real missing day", () => {
    render(<CompletenessBanner issues={[{ date: "2026-09-14", status: "partial", missingReason: "daily return is incomplete" }]} ratedDays={3} totalDays={4} />);
    expect(screen.getByText("Partial coverage")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Issues/ })).toBeInTheDocument();
  });

  it("shows the funding-gap reason instead of a raw sentence", async () => {
    const user = userEvent.setup();
    render(<CompletenessBanner issues={[{ date: "2026-10-02", status: "partial", missingReason: "funding cash is recorded after the product start" }]} ratedDays={1} totalDays={4} />);
    await user.click(screen.getByRole("button", { name: /Issues/ }));
    expect(await screen.findByText("Funding cash is recorded after the product start.")).toBeInTheDocument();
    expect(screen.queryByText("funding cash is recorded after the product start")).not.toBeInTheDocument();
  });
});

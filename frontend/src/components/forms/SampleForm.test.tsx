import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SampleForm } from "./SampleForm";

describe("SampleForm (React Hook Form + Zod wiring)", () => {
  it("calls onSubmit with validated values", async () => {
    const onSubmit = vi.fn();
    render(<SampleForm onSubmit={onSubmit} />);
    await userEvent.type(screen.getByLabelText("Name"), "Alice");
    await userEvent.click(screen.getByRole("button", { name: "Submit" }));
    expect(onSubmit).toHaveBeenCalledWith({ name: "Alice" }, expect.anything());
  });

  it("shows a validation error and does not submit for an empty name", async () => {
    const onSubmit = vi.fn();
    render(<SampleForm onSubmit={onSubmit} />);
    await userEvent.click(screen.getByRole("button", { name: "Submit" }));
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();
  });
});

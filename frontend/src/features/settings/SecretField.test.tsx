import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SecretField } from "./SecretField";

function Field({ hasValue = true, remove = vi.fn() }) {
  const [value, setValue] = useState("");
  return <SecretField id="key" label="Provider key" hasValue={hasValue} value={value} onChange={setValue} onRemove={remove} />;
}
describe("SecretField", () => {
  it("uses only a fixed mask for stored credentials and reveals only new input", async () => {
    const user = userEvent.setup();
    render(<Field />);
    expect(screen.getByText("••••••••")).toBeVisible();
    expect(screen.queryByLabelText("Provider key")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Replace" }));
    const input = screen.getByLabelText("Provider key");
    expect(input).toHaveValue("");
    await user.type(input, "new-input-only");
    expect(input).toHaveAttribute("type", "password");
    await user.click(screen.getByRole("button", { name: "Show new input" }));
    expect(input).toHaveAttribute("type", "text");
    expect(input).toHaveValue("new-input-only");
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByLabelText("Provider key")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Replace" }));
    expect(screen.getByLabelText("Provider key")).toHaveValue("");
    expect(screen.getByLabelText("Provider key")).toHaveAttribute("type", "password");
  });
  it("requires explicit removal; empty input and cancel never remove a stored key", async () => {
    const user = userEvent.setup();
    const remove = vi.fn();
    render(<Field remove={remove} />);
    await user.click(screen.getByRole("button", { name: "Replace" }));
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(remove).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Remove" }));
    expect(remove).toHaveBeenCalledOnce();
  });
  it("starts an unconfigured field empty and hides typed input", () => {
    render(<Field hasValue={false} />);
    expect(screen.getByLabelText("Provider key")).toHaveValue("");
    expect(screen.getByLabelText("Provider key")).toHaveAttribute("type", "password");
    expect(screen.queryByText("••••••••")).not.toBeInTheDocument();
  });
});

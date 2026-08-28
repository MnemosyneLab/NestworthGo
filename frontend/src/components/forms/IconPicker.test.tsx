import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { EntityIcon, hasIcon } from "@/components/icons/EntityIcon";
import { IconPicker } from "@/components/forms/IconPicker";
import { ICON_CATALOG } from "@/lib/iconCatalog";

function PickerHarness() {
  const [value, setValue] = useState("bank");
  return <IconPicker id="entity-icon" value={value} kind="institution" onChange={setValue} />;
}

describe("IconPicker", () => {
  it("shows the selected preview and updates the selected state from the grid", async () => {
    render(<PickerHarness />);
    const trigger = screen.getByLabelText("Choose icon");
    expect(trigger.querySelector(".lucide-landmark")).toBeInTheDocument();
    expect(trigger).toHaveTextContent("Bank");

    trigger.focus();
    await userEvent.keyboard("{Enter}");
    const brokerage = screen.getByRole("button", { name: "Brokerage" });
    brokerage.focus();
    await userEvent.keyboard("{Enter}");

    expect(brokerage).toHaveAttribute("aria-pressed", "true");
    expect(trigger.querySelector(".lucide-chart-no-axes-combined")).toBeInTheDocument();
    expect(trigger).toHaveTextContent("Brokerage");
  });

  it("falls back safely when a stored key is unknown", () => {
    const { container } = render(<EntityIcon iconKey="removed-history-key" kind="member" />);
    expect(container.querySelector(".lucide-user-round")).toBeInTheDocument();
  });

  it("only offers keys the backend catalog can persist", () => {
    for (const choice of ICON_CATALOG) {
      expect(hasIcon(choice.key)).toBe(true);
    }
  });
});

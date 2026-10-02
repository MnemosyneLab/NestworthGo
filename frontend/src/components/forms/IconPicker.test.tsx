import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { EntityIcon, hasIcon } from "@/components/icons/EntityIcon";
import { IconPicker } from "@/components/forms/IconPicker";
import i18n from "@/i18n";
import { ICON_CATALOG } from "@/lib/iconCatalog";

function PickerHarness() {
  const [value, setValue] = useState("bank");
  return <IconPicker id="entity-icon" value={value} kind="institution" onChange={setValue} />;
}

describe("IconPicker", () => {
  it("keeps icon choices out of the tab order until opened", () => {
    render(<PickerHarness />);
    expect(screen.queryByRole("button", { name: "Brokerage" })).not.toBeInTheDocument();
  });

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

function InheritedPicker() {
  const [value, setValue] = useState("");
  return <IconPicker id="account-icon" value={value} kind="account" inheritIconKey="bank-logo:icbc" onChange={setValue} />;
}

it("selects a searched logo by keyboard, keeps its colors in dark mode, and resets to inheritance", async () => {
  document.documentElement.classList.add("dark");
  try {
    render(<InheritedPicker />);
    const trigger = screen.getByLabelText("Choose icon");
    expect(trigger.querySelector("img")).toHaveAttribute("src", "/bank-logos/icbc.svg");
    trigger.focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.selectOptions(screen.getByLabelText("Icon category"), "bank");
    await userEvent.type(screen.getByLabelText("Search icons"), "boc");
    const logo = screen.getByRole("button", { name: "中国银行 (boc)" });
    logo.focus();
    await userEvent.keyboard(" ");
    expect(logo).toHaveAttribute("aria-pressed", "true");
    expect(trigger.querySelector("img")).toHaveAttribute("src", "/bank-logos/boc.svg");
    expect(trigger.querySelector("img")).toHaveClass("object-contain", "bg-white");
    await userEvent.keyboard("{Escape}");
    expect(trigger).toHaveFocus();
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    const reset = screen.getByRole("button", { name: "Use institution icon" });
    reset.focus();
    await userEvent.keyboard("{Enter}");
    expect(reset).toHaveAttribute("aria-pressed", "true");
    expect(trigger.querySelector("img")).toHaveAttribute("src", "/bank-logos/icbc.svg");
  } finally { document.documentElement.classList.remove("dark"); }
});

it("reports no matches and keeps generic categories searchable", async () => {
  render(<PickerHarness />);
  await userEvent.click(screen.getByLabelText("Choose icon"));
  await userEvent.type(screen.getByLabelText("Search icons"), "nonexistent-bank-xyz");
  expect(screen.getByRole("status")).toHaveTextContent("No matching icons");
  await userEvent.clear(screen.getByLabelText("Search icons"));
  await userEvent.type(screen.getByLabelText("Search icons"), "wallet");
  expect(screen.getByRole("button", { name: "Wallet" })).toBeInTheDocument();
});

it("does not offer bank categories on unrelated entities and never uses an unknown key as a URL", async () => {
  const { container } = render(<IconPicker id="member-icon" kind="member" value="bank-logo:../../remote" onChange={() => undefined} />);
  await userEvent.click(screen.getByLabelText("Choose icon"));
  expect(screen.queryByLabelText("Icon category")).not.toBeInTheDocument();
  expect(screen.queryByLabelText("Search icons")).not.toBeInTheDocument();
  expect(container.querySelector("img")).not.toBeInTheDocument();
});


it.each(["en", "zh-CN", "zh-TW"])("localizes the bank category and inheritance action in %s", async (language) => {
  await i18n.changeLanguage(language);
  render(<InheritedPicker />);
  expect(screen.getByRole("button", { name: i18n.t("icons.inheritInstitution") })).toBeInTheDocument();
  await userEvent.click(screen.getByLabelText(i18n.t("common.chooseIcon")));
  await userEvent.selectOptions(screen.getByLabelText(i18n.t("icons.categoryLabel")), "bank");
  await userEvent.type(screen.getByLabelText(i18n.t("icons.search")), "Bank of America");
  expect(screen.getAllByRole("button").some((button) => button.textContent?.includes("bankofamerica"))).toBe(true);
});


it.each(["__proto__", "constructor", "bank-logo:unknown"])("falls back safely for unknown stored ID %s", (key) => {
  const { container } = render(<EntityIcon iconKey={key} kind="account" />);
  expect(container.querySelector(".lucide-wallet")).toBeInTheDocument();
  expect(hasIcon(key)).toBe(false);
});

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import i18n from "@/i18n";
import { NavigationHistoryControls } from "./NavigationHistoryControls";

vi.mock("@/queries/accounts", () => ({ useAccounts: () => ({ data: [{ account: { id: "synthetic-account", name: "Synthetic account" } }] }) }));
vi.mock("@/queries/investments", () => ({ useInstruments: () => ({ data: [] }) }));
beforeEach(async () => { await i18n.changeLanguage("en"); });

it.each(["en", "zh-CN", "zh-TW"])("labels history destinations and preserves keyboard/empty-stack semantics in %s", async language => {
  await i18n.changeLanguage(language);
  const back = vi.fn();
  const forward = vi.fn();
  const props = { canBack: true, canForward: true, back, forward, recent: [], backTarget: { page: "accounts", accountId: "synthetic-account" } as const, forwardTarget: { page: "accounts", accountId: "deleted-synthetic-account" } as const };
  const { rerender } = render(<NavigationHistoryControls {...props} />);
  const backLabel = i18n.t("navigationHistory.backTo", { destination: `${i18n.t("nav.accounts")} · Synthetic account` });
  const forwardLabel = i18n.t("navigationHistory.forwardTo", { destination: `${i18n.t("nav.accounts")} · ${i18n.t("navigationHistory.unavailableAccount")}` });
  const user = userEvent.setup();
  const button = screen.getByRole("button", { name: backLabel });
  expect(button).toHaveAttribute("title", backLabel);
  expect(screen.getByRole("button", { name: forwardLabel })).toHaveAttribute("title", forwardLabel);
  expect(document.body).not.toHaveTextContent("deleted-synthetic-account");
  button.focus();
  await user.keyboard("{Enter}");
  expect(back).toHaveBeenCalledTimes(1);
  await user.tab();
  await user.keyboard(" ");
  expect(forward).toHaveBeenCalledTimes(1);
  rerender(<NavigationHistoryControls {...props} canBack={false} canForward={false} />);
  expect(screen.getByRole("button", { name: i18n.t("navigationHistory.back") })).toBeDisabled();
  expect(screen.getByRole("button", { name: i18n.t("navigationHistory.forward") })).toBeDisabled();
});

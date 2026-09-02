import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { BlockedStartupPage } from "./BlockedStartupPage";
import i18n from "@/i18n";
import { AppProviders } from "@/app/providers";

vi.mock("@/queries/data", () => ({
  useInspectBackup: () => ({ mutate: vi.fn(), isPending: false }),
  useConfirmRestore: () => ({ mutate: vi.fn(), isPending: false }),
}));

describe("BlockedStartupPage", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("en");
  });

  const cases = [
    { code: "database_upgrade_required", titleKey: "startup.upgradeTitle", guidanceKey: "startup.upgradeGuidance" },
    { code: "database_from_newer_version", titleKey: "startup.newerTitle", guidanceKey: "startup.newerGuidance" },
    { code: "database_integrity_failed", titleKey: "startup.integrityTitle", guidanceKey: "startup.integrityGuidance" },
    { code: "database_unavailable", titleKey: "startup.unavailableTitle", guidanceKey: "startup.unavailableGuidance" },
  ] as const;

  it.each(cases)("shows distinct copy and Restore for $code", ({ code, titleKey, guidanceKey }) => {
    render(
      <AppProviders>
        <BlockedStartupPage startup={{ available: false, code, field: "database", foundSchemaVersion: 7, supportedSchemaVersion: 9 }} />
      </AppProviders>,
    );
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent(i18n.t(titleKey));
    expect(alert).toHaveTextContent(i18n.t(guidanceKey));
    expect(screen.getByRole("button", { name: i18n.t("startup.restore") })).toBeInTheDocument();
    expect(alert).not.toHaveTextContent("/Users");
    expect(alert).not.toHaveTextContent("sqlite");
  });
});

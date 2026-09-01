import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PageChrome, PageChromeProvider } from "./PageChrome";

describe("PageChrome", () => {
  it("keeps a semantic heading and actions when rendered without AppShell", () => {
    render(<PageChrome pageId="accounts" title="Accounts" actions={<button type="button">Add account</button>} />);

    expect(screen.getByRole("heading", { name: "Accounts" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add account" })).toBeInTheDocument();
  });

  it("portals only the active page chrome into the shell target", () => {
    const target = document.createElement("div");
    document.body.appendChild(target);

    render(
      <PageChromeProvider activePageId="accounts" target={target}>
        <PageChrome pageId="accounts" title="Accounts" actions={<button type="button">Add account</button>} />
        <PageChrome pageId="history" title="History" actions={<button type="button">Record change</button>} />
      </PageChromeProvider>,
    );

    expect(within(target).getByRole("heading", { name: "Accounts" })).toBeInTheDocument();
    expect(within(target).getByRole("button", { name: "Add account" })).toBeInTheDocument();
    expect(within(target).queryByRole("heading", { name: "History" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Record change" })).not.toBeInTheDocument();
    target.remove();
  });
});

import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { AppFocusBoundary } from "./AppFocusBoundary";
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

describe("AppFocusBoundary", () => {
  it("preserves normal input/select traversal and skips disabled and hidden controls at the edge", async () => {
    const user = userEvent.setup();
    render(<AppFocusBoundary>
      <input aria-label="Name" />
      <select aria-label="Choice"><option value="a">A</option><option value="b">B</option></select>
      <button>Last</button><button disabled>Disabled</button>
      <div hidden><button>Hidden</button></div>
    </AppFocusBoundary>);
    await user.tab();
    expect(screen.getByLabelText("Name")).toHaveFocus();
    await user.keyboard("unchanged input");
    await user.tab();
    expect(screen.getByLabelText("Choice")).toHaveFocus();
    await user.selectOptions(screen.getByLabelText("Choice"), "b");
    await user.tab();
    expect(screen.getByRole("button", { name: "Last" })).toHaveFocus();
    await user.tab();
    expect(screen.getByLabelText("Name")).toHaveFocus();
    expect(screen.getByLabelText("Name")).toHaveValue("unchanged input");
    expect(screen.getByLabelText("Choice")).toHaveValue("b");
    await user.tab({ shift: true });
    expect(screen.getByRole("button", { name: "Last" })).toHaveFocus();
  });

  it("re-enters from outside at the nearest edge without intercepting native shortcuts", () => {
    const { container } = render(<><button>Outside</button><AppFocusBoundary><button>First</button><button>Last</button><div {...{ inert: "" }}><button>Inert</button></div></AppFocusBoundary></>);
    const outside = screen.getByRole("button", { name: "Outside" });
    outside.focus();
    (container.querySelector('[data-app-focus-guard="end"]') as HTMLElement).focus();
    expect(screen.getByRole("button", { name: "Last" })).toHaveFocus();
    for (const event of [{ key: "Tab", ctrlKey: true }, { key: "Tab", altKey: true }, { key: "Tab", metaKey: true }, { key: "F6" }, { key: "F10" }, { key: "Escape" }]) {
      expect(fireEvent.keyDown(document.activeElement!, event)).toBe(true);
    }
    outside.focus();
    (container.querySelector('[data-app-focus-guard="start"]') as HTMLElement).focus();
    expect(screen.getByRole("button", { name: "First" })).toHaveFocus();
  });

  it("leaves Portal dialog cycling and cancellation focus to the dialog", async () => {
    const user = userEvent.setup();
    render(<AppFocusBoundary><button>First</button><AlertDialog>
      <AlertDialogTrigger>Open confirmation</AlertDialogTrigger>
      <AlertDialogContent><AlertDialogTitle>Confirm</AlertDialogTitle><AlertDialogDescription>Test confirmation.</AlertDialogDescription>
        <button>Dialog first</button><AlertDialogCancel>Cancel</AlertDialogCancel>
      </AlertDialogContent>
    </AlertDialog></AppFocusBoundary>);
    await user.tab();
    await user.tab();
    const trigger = screen.getByRole("button", { name: "Open confirmation" });
    expect(trigger).toHaveFocus();
    await user.keyboard("{Enter}");
    const dialog = await screen.findByRole("alertdialog");
    const cancel = within(dialog).getByRole("button", { name: "Cancel" });
    cancel.focus(); // Stand in for native dialog autofocus in jsdom.
    await user.tab();
    await waitFor(() => expect(dialog).toContainElement(document.activeElement as HTMLElement));
    await user.tab({ shift: true });
    await waitFor(() => expect(cancel).toHaveFocus());
    await user.keyboard("{Enter}");
    await waitFor(() => expect(trigger).toHaveFocus());
    await user.tab();
    expect(screen.getByRole("button", { name: "First" })).toHaveFocus();
  });

  it("keeps the active tab panel reachable and excludes mounted hidden panel controls", async () => {
    const user = userEvent.setup();
    render(<AppFocusBoundary><Tabs defaultValue="one"><TabsList><TabsTrigger value="one">One</TabsTrigger><TabsTrigger value="two">Two</TabsTrigger></TabsList>
      <TabsContent value="one" keepMounted><button>Visible last</button></TabsContent>
      <TabsContent value="two" keepMounted><button>Hidden last</button></TabsContent>
    </Tabs></AppFocusBoundary>);
    await user.tab();
    expect(screen.getByRole("tab", { name: "One" })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("tabpanel")).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("button", { name: "Visible last" })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("tab", { name: "One" })).toHaveFocus();
  });
});

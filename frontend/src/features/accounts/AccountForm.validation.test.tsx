import { expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AccountForm } from "./AccountForm";
import { TEST_CATALOG } from "@/test/catalog";
vi.mock("@/queries/directory", () => ({
 useMembers: (archived = false) => ({ data: archived ? [{id:"old-owner",name:"Former Owner",archivedAt:"2026-01-01"},{id:"other-owner",name:"Unrelated Owner",archivedAt:"2026-01-01"},{id:"new-owner",name:"Active Owner"}] : [{id:"new-owner",name:"Active Owner"}] }),
 useInstitutions: (archived = false) => ({ data: archived ? [{id:"old-bank",name:"Former Bank",iconKey:"bank",archivedAt:"2026-01-01"},{id:"other-bank",name:"Unrelated Bank",iconKey:"bank",archivedAt:"2026-01-01"}] : [] }),
 useGroups: (archived = false) => ({ data: archived ? [{id:"old-group",name:"Former Group",iconKey:"folder",archivedAt:"2026-01-01"},{id:"other-group",name:"Unrelated Group",iconKey:"folder",archivedAt:"2026-01-01"}] : [] }),
}));
vi.mock("@/queries/settings", () => ({useSupportedCurrencies:()=>({data:["USD"]})}));
vi.mock("@/queries/catalog", () => ({useCatalog:()=>({data:TEST_CATALOG})}));
vi.mock("@/queries/household", () => ({useBootstrap:()=>({data:{household:{baseCurrency:"USD"}}})}));
vi.mock("@/queries/history", () => ({useHistoryOrigin:()=>({data:null})}));

it("associates name errors and focuses owners after the name is corrected", async () => {
 const submit = vi.fn();
 render(<AccountForm onSubmit={submit} submitLabel="Save" isSubmitting={false} />);
 const name = screen.getByLabelText("Name");
 const save = screen.getByRole("button", { name: "Save" });
 expect(name).toHaveAttribute("aria-invalid", "false");
 expect(save).toBeEnabled();
 await userEvent.click(save);
 await waitFor(() => expect(name).toHaveFocus());
 expect(name).toHaveAttribute("aria-invalid", "true");
 expect(name).toHaveAccessibleDescription("Must not be empty");
 await userEvent.type(name, "Synthetic");
 await waitFor(() => expect(name).toHaveAttribute("aria-invalid", "false"));
 expect(name).not.toHaveAttribute("aria-describedby");
 await userEvent.click(save);
 const owner = screen.getByLabelText("Active Owner");
 const group = owner.closest("fieldset")!;
 await waitFor(() => expect(group).toHaveFocus());
 expect(group).toHaveAttribute("aria-invalid", "true");
 expect(owner).toHaveAttribute("aria-describedby", group.getAttribute("aria-describedby"));
 expect(document.getElementById(group.getAttribute("aria-describedby")!)).toHaveAttribute("role", "alert");
 expect(submit).not.toHaveBeenCalled();
 await userEvent.click(owner);
 await waitFor(() => expect(group).toHaveAttribute("aria-invalid", "false"));
 expect(owner).not.toHaveAttribute("aria-describedby");
 await userEvent.click(save);
 await waitFor(() => expect(submit).toHaveBeenCalledTimes(1));
});

it("keeps external submit available for validation and disables it while pending", async () => {
 const canSubmit = vi.fn();
 const view = render(<AccountForm formId="edit" hideSubmit onCanSubmitChange={canSubmit} onSubmit={vi.fn()} submitLabel="Save" isSubmitting={false} />);
 await waitFor(() => expect(canSubmit).toHaveBeenLastCalledWith(true));
 view.rerender(<AccountForm formId="edit" hideSubmit onCanSubmitChange={canSubmit} onSubmit={vi.fn()} submitLabel="Save" isSubmitting />);
 await waitFor(() => expect(canSubmit).toHaveBeenLastCalledWith(false));
 expect(screen.getByLabelText("Name")).toBeDisabled();
});

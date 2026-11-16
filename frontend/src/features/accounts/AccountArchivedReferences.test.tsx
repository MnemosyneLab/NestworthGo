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
const record = {account:{id:"a",name:"Synthetic",accountType:"bank_account",balanceSheetRole:"asset",trackingMode:"balance",defaultCurrency:"USD",institutionId:"old-bank",groupId:"old-group",includeInNetWorth:true,includeInPortfolio:false,includeInLiquidAssets:false}, ownership:[{memberId:"old-owner",shareBps:5000},{memberId:"new-owner",shareBps:5000}],latestValue:null};

it("shows only retained archived references and permits removing them", async () => {
 const submit = vi.fn();
 render(<AccountForm record={record as never} onSubmit={submit} submitLabel="Save" isSubmitting={false}/>);
 expect(screen.getByLabelText("Former Owner (Archived)")).toBeChecked();
 expect(screen.queryByText(/Unrelated/)).not.toBeInTheDocument();
 expect(screen.getByLabelText("Active Owner")).toBeChecked();
 await userEvent.click(screen.getByRole("button",{name:"Details"}));
 expect(document.querySelector("#account-institution")).toHaveValue("Former Bank (Archived)");
 expect(document.querySelector("#account-group")).toHaveValue("Former Group (Archived)");
 await userEvent.click(screen.getByRole("button",{name:"Save"}));
 await waitFor(()=>expect(submit).toHaveBeenCalled());
 expect(submit.mock.calls[0][0]).toMatchObject({institutionId:"old-bank",groupId:"old-group",ownership:[{memberId:"old-owner",shareBps:5000},{memberId:"new-owner",shareBps:5000}]});
 expect(screen.queryByText(/Unrelated/)).not.toBeInTheDocument();
 await userEvent.click(screen.getByLabelText("Former Owner (Archived)"));
 await userEvent.click(screen.getByRole("button",{name:"Save"}));
 await waitFor(()=>expect(submit).toHaveBeenCalledTimes(2));
 expect(submit.mock.calls[1][0].ownership).toEqual([{memberId:"new-owner",shareBps:10000}]);
});

it.each([
  { field: "institutionId", label: "Institutions", selected: "Former Bank (Archived)" },
  { field: "groupId", label: "Groups", selected: "Former Group (Archived)" },
])("keeps the $field error until a different choice is committed", async ({ field, label, selected }) => {
  const user = userEvent.setup();
  const props = { record: record as never, onSubmit: vi.fn(), submitLabel: "Save", isSubmitting: false };
  const view = render(<AccountForm {...props} />);
  await user.click(screen.getByRole("button", { name: "Details" }));
  const input = screen.getByRole("combobox", { name: label });
  const cause = new Error(JSON.stringify({ code: "validation", field, message: "reference is inactive" }));
  view.rerender(<AccountForm {...props} submissionCause={cause} submissionError="Choose an active reference" />);
  await waitFor(() => expect(input).toHaveFocus());

  await user.type(input, "missing");
  await user.keyboard("{Escape}");
  expect(input).toHaveValue(selected);
  expect(input).toHaveAttribute("aria-invalid", "true");
  expect(input).toHaveAccessibleDescription("Choose an active reference");

  await user.type(input, "Former");
  await user.keyboard("{Enter}");
  expect(input).toHaveAttribute("aria-expanded", "false");
  expect(input).toHaveValue(selected);
  expect(input).toHaveAccessibleDescription("Choose an active reference");

  await user.type(input, "Unassigned");
  await user.keyboard("{Enter}");
  expect(input).toHaveAttribute("aria-expanded", "false");
  expect(input).toHaveValue("Unassigned");
  expect(input).toHaveAttribute("aria-invalid", "false");
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(props.onSubmit).toHaveBeenCalledWith(expect.objectContaining({ [field]: undefined }), {}));
});

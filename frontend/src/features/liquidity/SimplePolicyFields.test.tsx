import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { SimplePolicyFields, policyPreset, simplePolicy } from "./SimplePolicyFields";
import { defaultProductPolicy } from "./productPolicy";
import type { ProductPolicyInput } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
vi.mock("@/queries/settings", () => ({useSettings: () => ({data: {}})}));
function Form({initial}: {initial: ProductPolicyInput}) {
  const [policy, setPolicy] = useState(initial);
  return <><SimplePolicyFields value={policy} onChange={setPolicy} /><output data-testid="policy">{JSON.stringify(policy)}</output></>;
}
it("applies a cash estimate in one choice and hides advanced parameters", async () => {
  render(<Form initial={{...defaultProductPolicy(null), accessKind: "unknown", settlementDays: null, normalExitFee: null}} />);
  expect(screen.queryByLabelText("Settlement days")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("radio", {name: /Available now/}));
  expect(JSON.parse(screen.getByTestId("policy").textContent!)).toMatchObject({accessKind:"on_request", settlementDays:0, normalExitFee:"0", earlyKind:"not_allowed"});
  await userEvent.click(screen.getByText("Advanced settings · timing, fees and limits"));
  expect(await screen.findByLabelText("Settlement days")).toHaveValue(0);
});
it("preserves an existing custom policy without silently replacing fees or limits", () => {
  const initial = {...defaultProductPolicy(null), settlementDays: 4, normalExitFee:"15", accessibleAmountCap:"200"};
  render(<Form initial={initial} />);
  expect(screen.getByLabelText("Settlement days")).toHaveValue(4);
  expect(JSON.parse(screen.getByTestId("policy").textContent!)).toEqual(initial);
  expect(policyPreset(initial)).toBeNull();
});
it("makes soon, dated and excluded presets explicit and removes stale overrides", () => {
  const old = {...defaultProductPolicy("2027-01-01"), receiptOnOverride:"2027-02-01", normalExitFee:"12"};
  expect(simplePolicy("soon",old)).toMatchObject({accessKind:"on_request",settlementDays:1,dayBasis:"weekdays",unlockOn:null,receiptOnOverride:null,normalExitFee:"0"});
  expect(simplePolicy("date",old)).toMatchObject({accessKind:"on_date",unlockOn:"2027-01-01"});
  expect(simplePolicy("excluded",old).accessKind).toBe("excluded");
});

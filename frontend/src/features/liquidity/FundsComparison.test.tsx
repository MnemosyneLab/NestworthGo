import { render, screen, within } from "@testing-library/react";
import { expect, it } from "vitest";
import { FundsComparison } from "./FundsComparison";
import type { LiquiditySourceDTO, LiquidityBucketDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
it("shows known zero and sub-cent values in desktop and mobile breakdowns without missing dashes", () => {
 const date="2026-10-14";const money={amount:"0.004",currency:"USD"};
 const source={nativeCurrency:"USD",excluded:false,sourceRef:{kind:"account_cash"},normalRoute:{actionRequired:"withdraw"},bucketResults:[{horizonOn:date,selectedRoute:{actionRequired:"withdraw"},status:"complete",netNative:money,netBase:money}]} as LiquiditySourceDTO;
 const total={horizonOn:date,fullUnreserved:money,nativeCurrencyGroups:[{currency:"USD",fullUnreserved:money,appliedReserveSubtotal:{amount:"0",currency:"USD"}}]} as LiquidityBucketDTO;
 const {container}=render(<FundsComparison horizons={[{label:"Today",bucket:total}]} selected={date} onSelect={()=>{}} sources={[source]} baseCurrency="USD" />);
 const table=screen.getByRole("table");expect(table).toHaveTextContent("$0.00 · + nonzero, below 0.01");expect(table).not.toHaveTextContent("—");
 expect(within(table).getAllByText("$0.00").length).toBeGreaterThan(0);
 expect(container.querySelector('.md\\:hidden')).toHaveTextContent("$0.00 · + nonzero, below 0.01");
});

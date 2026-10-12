import { sortByCanonicalDesc } from "@/lib/money";
import type { ValuationComponentDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export function componentHouseholdAmount(component: ValuationComponentDTO, householdCurrency: string): string | undefined {
  if (component.baseAmount?.amount) {
    return component.baseAmount.amount;
  }
  if (component.available && component.nativeAmount && component.nativeCurrency === householdCurrency) {
    return component.nativeAmount;
  }
  return undefined;
}

function componentKey(component: ValuationComponentDTO): string {
  return component.holdingId ?? `cash-${component.nativeCurrency}`;
}

export function sortValuationComponents(components: ValuationComponentDTO[], householdCurrency: string): ValuationComponentDTO[] {
  return sortByCanonicalDesc(
    components,
    (component) => componentHouseholdAmount(component, householdCurrency) ?? "0",
    (left, right) => componentKey(left).localeCompare(componentKey(right)),
  );
}

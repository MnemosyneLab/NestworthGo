import type { ChartCategory } from "@/components/charts/collapseCategories";
import { allocateShareBps, isPositiveCanonical, sortByCanonicalDesc } from "@/lib/money";
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

/**
 * Builds donut slices from an account's valued components. Slice size is the
 * household-currency amount so mixed native currencies stay comparable.
 */
export function accountCompositionItems(components: ValuationComponentDTO[], householdCurrency: string): ChartCategory[] {
  const valued = sortValuationComponents(
    components.filter((component) => {
      const amount = componentHouseholdAmount(component, householdCurrency);
      return Boolean(amount) && isPositiveCanonical(amount as string);
    }),
    householdCurrency,
  );
  const amounts = valued.map((component) => componentHouseholdAmount(component, householdCurrency) as string);
  const shares = allocateShareBps(amounts);
  return valued.map((component, index) => ({
    key: componentKey(component),
    label: component.instrumentName || component.nativeCurrency,
    amount: amounts[index],
    shareBps: shares[index],
    nativeAmount: component.nativeAmount,
    nativeCurrency: component.nativeCurrency,
  }));
}

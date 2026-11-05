import type { NavigationResult } from "./contracts";

/** Published by the mounted trend component, including its effective query range.
 * Store assignment or a dispatch acknowledgement alone is never success. */
export type TrendObservation = {
  accountId: string; from: string; to: string; valuation: string; includeCash: boolean;
  display: string; currency: string; assetClass: string;
  status: "loading" | "ready" | "unavailable";
};

export class TrendVerification {
  private pending?: { expected: TrendObservation; finish: (result: NavigationResult) => void };
  begin(expected: TrendObservation): Promise<NavigationResult> {
    this.cancel();
    return new Promise(resolve => {
      const timer = setTimeout(() => finish("unverified"), 5000);
      const finish = (result: NavigationResult) => {
        clearTimeout(timer);
        if (this.pending?.finish === finish) this.pending = undefined;
        resolve(result);
      };
      this.pending = { expected, finish };
    });
  }
  cancel() { this.pending?.finish("unverified"); }
  observe(value: TrendObservation) {
    const pending = this.pending;
    if (!pending) return;
    const same = (Object.keys(pending.expected) as (keyof TrendObservation)[])
      .every(key => key === "status" || value[key] === pending.expected[key]);
    if (!same || value.status === "unavailable") pending.finish("unverified");
    else if (value.status === "ready") pending.finish("opened");
  }
}

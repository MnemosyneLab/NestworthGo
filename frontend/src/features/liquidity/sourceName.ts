import type { LiquiditySourceDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
type Translator = (key: string, options?: Record<string, unknown>) => string;

export function sourceName(t: Translator, source: LiquiditySourceDTO): string {
  if (source.sourceRef.kind === "account_cash" && source.displayName.endsWith(" cash")) {
    return t("availableFunds.cashSource", { name: source.displayName.slice(0, -5) });
  }
  return source.displayName;
}

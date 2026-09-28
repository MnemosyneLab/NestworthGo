import type { ActivityDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { formatAmount } from "@/lib/money";

type Translator = (key: string, options?: Record<string, unknown>) => string;

// Use the recorded product linkage, never names or a quantity of one, to
// distinguish managed contracts from ordinary securities.
export function productActivityText(t: Translator, activity: ActivityDTO, accounts: Map<string, string>, instruments: Map<string, string>) {
  const context = activity.productContext;
  if (!context) return null;
  const deposit = context.productKind === "term_deposit";
  const keys: Record<string, string> = {
    acquisition: deposit ? "depositOpened" : "productSubscribed",
    existing_position: "productRecorded",
    redemption: deposit ? "depositPrincipalReceived" : "productRedeemed",
    interest: "productInterestReceived",
    reversal: "productReversed",
  };
  const key = activity.reversesActivityId ? "productReversed" : keys[context.purpose];
  if (!key) return null;
  const effects = activity.effects ?? [];
  const principal = effects.find((effect) => effect.role === "principal");
  const cash = effects.find((effect) => effect.money);
  const accountId = (principal ?? cash ?? effects.find((effect) => effect.accountId))?.accountId;
  const money = context.purpose === "interest" ? cash?.money : activity.tradeDetail?.gross ?? principal?.money;
  const amount = money ? formatAmount(money.amount, money.currency) : t("availableFunds.unknownAmount");
  const instrument = instruments.get(context.instrumentId) ?? t("history.unknownInstrument");
  const account = (accountId && accounts.get(accountId)) || t("history.unknownAccount");
  return {
    label: t(`history.productActivity.${key}`),
    sentence: t(`history.sentence.${key}`, {account, instrument, amount}),
    amount,
    amountLabel: t(deposit && ["acquisition", "redemption"].includes(context.purpose) ? "availableFunds.principal" : "history.amount"),
    account,
    instrument,
    showAmount: ["acquisition", "redemption", "interest"].includes(context.purpose) && !activity.reversesActivityId,
  };
}

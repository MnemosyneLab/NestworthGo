import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAccounts } from "@/queries/accounts";
import { useInstruments, useAllHoldingsFlat } from "@/queries/investments";
import { usePreviewChange, useRecordChange } from "@/queries/history";
import type { ChangeCommandRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";
import type { EndpointViewDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { formatAmount } from "@/lib/money";

const KIND_LABELS: Record<string, string> = {
  money_added: "Money added",
  money_removed: "Money removed",
  cash_transfer: "Cash transfer",
  fx_conversion: "FX conversion",
  position_transfer: "Position transfer",
  position_adjustment: "Position adjustment",
  trade: "Buy / Sell",
  value_update: "Value update",
  debt_draw: "Debt draw",
  debt_payment: "Debt payment",
};

const KINDS = Object.keys(KIND_LABELS);

function AccountSelect({ id, label, value, onChange, accounts }: { id: string; label: string; value: string; onChange: (value: string) => void; accounts: { id: string; name: string }[] }) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <select id={id} value={value} onChange={(event) => onChange(event.target.value)} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
        <option value="">—</option>
        {accounts.map((account) => (
          <option key={account.id} value={account.id}>
            {account.name}
          </option>
        ))}
      </select>
    </div>
  );
}

function MoneyFields({ prefix, label, amount, currency, onAmount, onCurrency }: { prefix: string; label: string; amount: string; currency: string; onAmount: (v: string) => void; onCurrency: (v: string) => void }) {
  return (
    <div className="flex gap-2">
      <div className="flex flex-1 flex-col gap-1.5">
        <Label htmlFor={`${prefix}-amount`}>{label}</Label>
        <Input id={`${prefix}-amount`} inputMode="decimal" value={amount} onChange={(event) => onAmount(event.target.value)} />
      </div>
      <div className="flex w-24 flex-col gap-1.5">
        <Label htmlFor={`${prefix}-currency`}>{"\u00A0"}</Label>
        <Input id={`${prefix}-currency`} value={currency} onChange={(event) => onCurrency(event.target.value.toUpperCase())} maxLength={3} />
      </div>
    </div>
  );
}

/**
 * RecordChangeForm implements the "record a change with ordinary
 * language" flow (interaction brief Sec8.4) for every one of the ten
 * domain.PreviewChange kinds via history.ChangeCommandRequest's tagged
 * union (technical design Sec6 — "the single trickiest mapping"). It
 * always previews before committing (Sec8.4's "预览后确认保存"): Preview
 * calls PreviewChange (no side effect) and shows the resulting
 * balances/quantities; Confirm re-submits the identical request to
 * RecordChange, which re-validates and commits.
 */
export function RecordChangeForm({ onRecorded }: { onRecorded: () => void }) {
  const accounts = useAccounts({});
  const instruments = useInstruments();
  const allAccountIds = (accounts.data ?? []).map((record) => record.account.id);
  const holdings = useAllHoldingsFlat(allAccountIds);
  const preview = usePreviewChange();
  const record = useRecordChange();

  const [kind, setKind] = useState<string>("money_added");
  const [fields, setFields] = useState<Record<string, string>>({ currency: "USD" });
  const [added, setAdded] = useState(true);
  const [previewResult, setPreviewResult] = useState<EndpointViewDTO[] | null>(null);

  const set = (key: string, value: string) => {
    setFields((prev) => ({ ...prev, [key]: value }));
    setPreviewResult(null);
  };

  const accountOptions = (accounts.data ?? []).map((record) => ({ id: record.account.id, name: record.account.name }));
  const holdingsAccountOptions = (accounts.data ?? [])
    .filter((record) => record.account.trackingMode === "holdings")
    .map((record) => ({ id: record.account.id, name: record.account.name }));

  const buildRequest = (): ChangeCommandRequest => ({
    kind: kind as ChangeCommandRequest["kind"],
    added,
    ...fields,
  });

  const runPreview = () => {
    preview.mutate(buildRequest(), { onSuccess: (result) => setPreviewResult(result.resulting) });
  };

  const runConfirm = () => {
    record.mutate(buildRequest(), {
      onSuccess: () => {
        setPreviewResult(null);
        setFields({ currency: "USD" });
        onRecorded();
      },
    });
  };

  const error = preview.error ?? record.error;

  return (
    <div className="flex flex-col gap-4" role="form" aria-label="Record change">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="change-kind">Type of change</Label>
        <select
          id="change-kind"
          value={kind}
          onChange={(event) => {
            setKind(event.target.value);
            setFields({ currency: "USD" });
            setPreviewResult(null);
          }}
          className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground"
        >
          {KINDS.map((value) => (
            <option key={value} value={value}>
              {KIND_LABELS[value]}
            </option>
          ))}
        </select>
      </div>

      {kind === "money_added" && (
        <>
          <AccountSelect id="change-account" label="Account" value={fields.accountId ?? ""} onChange={(v) => set("accountId", v)} accounts={accountOptions} />
          <MoneyFields prefix="change" label="Amount" amount={fields.amount ?? ""} currency={fields.currency ?? "USD"} onAmount={(v) => set("amount", v)} onCurrency={(v) => set("currency", v)} />
        </>
      )}

      {kind === "money_removed" && (
        <>
          <AccountSelect id="change-account" label="Account" value={fields.accountId ?? ""} onChange={(v) => set("accountId", v)} accounts={accountOptions} />
          <MoneyFields prefix="change" label="Amount" amount={fields.amount ?? ""} currency={fields.currency ?? "USD"} onAmount={(v) => set("amount", v)} onCurrency={(v) => set("currency", v)} />
        </>
      )}

      {kind === "value_update" && (
        <>
          <AccountSelect id="change-account" label="Account" value={fields.accountId ?? ""} onChange={(v) => set("accountId", v)} accounts={accountOptions} />
          <MoneyFields prefix="change" label="New value" amount={fields.newValue ?? ""} currency={fields.newValueCurrency ?? "USD"} onAmount={(v) => set("newValue", v)} onCurrency={(v) => set("newValueCurrency", v)} />
        </>
      )}

      {kind === "cash_transfer" && (
        <>
          <AccountSelect id="change-from-account" label="From account" value={fields.fromAccountId ?? ""} onChange={(v) => set("fromAccountId", v)} accounts={accountOptions} />
          <AccountSelect id="change-to-account" label="To account" value={fields.toAccountId ?? ""} onChange={(v) => set("toAccountId", v)} accounts={accountOptions} />
          <MoneyFields prefix="change-sent" label="Sent" amount={fields.sent ?? ""} currency={fields.sentCurrency ?? "USD"} onAmount={(v) => set("sent", v)} onCurrency={(v) => set("sentCurrency", v)} />
          <MoneyFields prefix="change-received" label="Received" amount={fields.received ?? ""} currency={fields.receivedCurrency ?? "USD"} onAmount={(v) => set("received", v)} onCurrency={(v) => set("receivedCurrency", v)} />
        </>
      )}

      {kind === "fx_conversion" && (
        <>
          <AccountSelect id="change-account" label="Account" value={fields.accountId ?? ""} onChange={(v) => set("accountId", v)} accounts={holdingsAccountOptions} />
          <MoneyFields prefix="change-sold" label="Sold" amount={fields.sold ?? ""} currency={fields.soldCurrency ?? "USD"} onAmount={(v) => set("sold", v)} onCurrency={(v) => set("soldCurrency", v)} />
          <MoneyFields prefix="change-bought" label="Bought" amount={fields.bought ?? ""} currency={fields.boughtCurrency ?? "USD"} onAmount={(v) => set("bought", v)} onCurrency={(v) => set("boughtCurrency", v)} />
        </>
      )}

      {kind === "position_transfer" && (
        <>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-from-holding">From holding</Label>
            <select id="change-from-holding" value={fields.fromHoldingId ?? ""} onChange={(event) => set("fromHoldingId", event.target.value)} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
              <option value="">—</option>
              {holdings.data.map((holding) => (
                <option key={holding.id} value={holding.id}>
                  {holding.instrumentName}
                </option>
              ))}
            </select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-to-holding">To holding</Label>
            <select id="change-to-holding" value={fields.toHoldingId ?? ""} onChange={(event) => set("toHoldingId", event.target.value)} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
              <option value="">—</option>
              {holdings.data.map((holding) => (
                <option key={holding.id} value={holding.id}>
                  {holding.instrumentName}
                </option>
              ))}
            </select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-quantity">Quantity</Label>
            <Input id="change-quantity" inputMode="decimal" value={fields.quantity ?? ""} onChange={(event) => set("quantity", event.target.value)} />
          </div>
        </>
      )}

      {kind === "position_adjustment" && (
        <>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-holding">Holding</Label>
            <select id="change-holding" value={fields.holdingId ?? ""} onChange={(event) => set("holdingId", event.target.value)} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
              <option value="">—</option>
              {holdings.data.map((holding) => (
                <option key={holding.id} value={holding.id}>
                  {holding.instrumentName}
                </option>
              ))}
            </select>
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={added} onChange={(event) => setAdded(event.target.checked)} /> Added (unchecked = removed)
          </label>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-quantity">Quantity</Label>
            <Input id="change-quantity" inputMode="decimal" value={fields.quantity ?? ""} onChange={(event) => set("quantity", event.target.value)} />
          </div>
          {added && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="change-unit-cost">Unit cost (optional)</Label>
              <Input id="change-unit-cost" inputMode="decimal" value={fields.unitCost ?? ""} onChange={(event) => set("unitCost", event.target.value)} />
            </div>
          )}
        </>
      )}

      {kind === "trade" && (
        <>
          <AccountSelect id="change-settlement-account" label="Settlement account" value={fields.settlementAccountId ?? ""} onChange={(v) => set("settlementAccountId", v)} accounts={holdingsAccountOptions} />
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-instrument">Instrument</Label>
            <select id="change-instrument" value={fields.instrumentId ?? ""} onChange={(event) => set("instrumentId", event.target.value)} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
              <option value="">—</option>
              {(instruments.data ?? []).map((instrument) => (
                <option key={instrument.id} value={instrument.id}>
                  {instrument.name}
                </option>
              ))}
            </select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-side">Side</Label>
            <select id="change-side" value={fields.side ?? "buy"} onChange={(event) => set("side", event.target.value)} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
              <option value="buy">Buy</option>
              <option value="sell">Sell</option>
            </select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-quantity">Quantity</Label>
            <Input id="change-quantity" inputMode="decimal" value={fields.quantity ?? ""} onChange={(event) => set("quantity", event.target.value)} />
          </div>
          <MoneyFields prefix="change-gross" label="Gross total" amount={fields.gross ?? ""} currency={fields.grossCurrency ?? "USD"} onAmount={(v) => set("gross", v)} onCurrency={(v) => set("grossCurrency", v)} />
          <MoneyFields prefix="change-fee" label="Fee (optional)" amount={fields.fee ?? ""} currency={fields.feeCurrency ?? fields.grossCurrency ?? "USD"} onAmount={(v) => set("fee", v)} onCurrency={(v) => set("feeCurrency", v)} />
        </>
      )}

      {(kind === "debt_draw" || kind === "debt_payment") && (
        <>
          <AccountSelect id="change-debt-account" label="Debt account" value={fields.debtAccountId ?? ""} onChange={(v) => set("debtAccountId", v)} accounts={accountOptions} />
          <AccountSelect id="change-cash-account" label="Cash account" value={fields.cashAccountId ?? ""} onChange={(v) => set("cashAccountId", v)} accounts={accountOptions} />
          <MoneyFields prefix="change-principal" label="Principal" amount={fields.principal ?? ""} currency={fields.principalCurrency ?? "USD"} onAmount={(v) => set("principal", v)} onCurrency={(v) => set("principalCurrency", v)} />
          {kind === "debt_payment" && (
            <MoneyFields prefix="change-interest" label="Interest or fee (optional)" amount={fields.interestOrFee ?? ""} currency={fields.interestOrFeeCurrency ?? fields.principalCurrency ?? "USD"} onAmount={(v) => set("interestOrFee", v)} onCurrency={(v) => set("interestOrFeeCurrency", v)} />
          )}
        </>
      )}

      {error && (
        <p role="alert" className="text-sm text-destructive">
          {(error as Error).message}
        </p>
      )}

      {previewResult && (
        <div role="status" className="flex flex-col gap-1 rounded-md border border-border bg-muted p-3 text-sm">
          <p className="font-medium">Preview</p>
          {previewResult.map((endpoint, index) => (
            <p key={index}>
              {endpoint.name}: {endpoint.amount ? formatAmount(endpoint.amount, endpoint.currency) : formatAmount(endpoint.quantity ?? "0")}
            </p>
          ))}
        </div>
      )}

      <div className="flex gap-2">
        {!previewResult ? (
          <Button type="button" onClick={runPreview} disabled={preview.isPending}>
            Preview
          </Button>
        ) : (
          <Button type="button" onClick={runConfirm} disabled={record.isPending}>
            Confirm
          </Button>
        )}
      </div>
    </div>
  );
}

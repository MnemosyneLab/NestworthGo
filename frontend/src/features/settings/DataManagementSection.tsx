import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { displayError } from "@/lib/display";
import {
  useLastBackupStatus,
  useCreateBackup,
  useInspectBackup,
  useConfirmRestore,
  useExportCSV,
  useSelectCSV,
  usePreviewCSV,
  useConfirmCSV,
  useCommitCSV,
  useCancelCSV,
  useDownloadCSVErrors,
  type CSVPreviewDTO,
} from "@/queries/data";

const ACCOUNT_FIELDS = [
  "account_name", "account_type", "balance_sheet_role", "tracking_mode", "currency",
  "current_value", "value_date", "ownership", "include_in_net_worth", "institution_name",
  "institution_type", "group_name", "include_in_portfolio", "include_in_liquid_assets", "icon_key", "note",
];
const HOLDING_FIELDS = [
  "account_name", "instrument_type", "instrument_name", "quantity", "quote_currency",
  "unit_price", "quote_date", "symbol", "market_code", "country_code", "isin", "note",
];

type UnresolvedChoice = { kind: string; name: string; action: string; mapTo: string };

function unresolvedKey(kind: string, name: string) {
  return `${kind}\0${name}`;
}

export function DataManagementSection() {
  const { t } = useTranslation();
  const status = useLastBackupStatus();
  const createBackup = useCreateBackup();
  const inspect = useInspectBackup();
  const confirmRestore = useConfirmRestore();
  const exportCSV = useExportCSV();
  const selectCSV = useSelectCSV();
  const previewCSV = usePreviewCSV();
  const confirmCSV = useConfirmCSV();
  const commitCSV = useCommitCSV();
  const cancelCSV = useCancelCSV();
  const cancelCSVRef = useRef(cancelCSV.mutateAsync);
  useEffect(() => {
    cancelCSVRef.current = cancelCSV.mutateAsync;
  }, [cancelCSV.mutateAsync]);
  const downloadErrors = useDownloadCSVErrors();
  const [restoreOpen, setRestoreOpen] = useState(false);
  const [csvOpen, setCsvOpen] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const [restoreChrome, setRestoreChrome] = useState(false);
  const [restoreFormat, setRestoreFormat] = useState(false);
  const [restoreRouting, setRestoreRouting] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const [includeArchived, setIncludeArchived] = useState(false);
  const [csvStep, setCsvStep] = useState(1);
  const [csvToken, setCsvToken] = useState("");
  const csvTokenRef = useRef("");
  const [csvProfile, setCsvProfile] = useState("accounts");
  const [accountsHeaders, setAccountsHeaders] = useState<string[]>([]);
  const [holdingsHeaders, setHoldingsHeaders] = useState<string[]>([]);
  const [accountsMapping, setAccountsMapping] = useState<Record<string, string>>({});
  const [holdingsMapping, setHoldingsMapping] = useState<Record<string, string>>({});
  const [csvDelimiter, setCsvDelimiter] = useState("comma");
  const [csvDateFormat, setCsvDateFormat] = useState("iso");
  const [csvDecimalSep, setCsvDecimalSep] = useState(".");
  const [csvGroupingSep, setCsvGroupingSep] = useState("none");
  const [unresolvedChoices, setUnresolvedChoices] = useState<Record<string, UnresolvedChoice>>({});
  const [csvPreview, setCsvPreview] = useState<CSVPreviewDTO | null>(null);
  const preview = inspect.data && !inspect.data.cancelled ? inspect.data : null;
  const restoreReady = acknowledged && confirmation.trim().toLowerCase() === "restore" && Boolean(preview?.token);
  const mappingFields = csvProfile === "holdings" ? HOLDING_FIELDS : ACCOUNT_FIELDS;
  const csvHeaders = csvProfile === "holdings" ? holdingsHeaders : accountsHeaders;
  const csvMapping = csvProfile === "holdings" ? holdingsMapping : accountsMapping;
  const setCsvMapping = csvProfile === "holdings" ? setHoldingsMapping : setAccountsMapping;
  const hasAccountsFile = accountsHeaders.length > 0;
  const hasHoldingsFile = holdingsHeaders.length > 0;
  const csvNumberFormatConflict = csvGroupingSep !== "none" && csvGroupingSep !== "space" && csvGroupingSep === csvDecimalSep;

  const resetCSV = (cancelSession = true) => {
    if (cancelSession && csvTokenRef.current) {
      void cancelCSVRef.current(csvTokenRef.current).catch(() => undefined);
    }
    csvTokenRef.current = "";
    setCsvStep(1);
    setCsvToken("");
    setCsvPreview(null);
    setAccountsHeaders([]);
    setHoldingsHeaders([]);
    setAccountsMapping({});
    setHoldingsMapping({});
    setUnresolvedChoices({});
    setCsvProfile("accounts");
  };

  useEffect(() => () => {
    if (csvTokenRef.current) {
      void cancelCSVRef.current(csvTokenRef.current).catch(() => undefined);
    }
  }, []);

  const runBackup = () => {
    createBackup.mutate(undefined, {
      onSuccess: (result) => {
        if (!result.cancelled && result.fileName) {
          toast.success(t("settings.data.backupCreated", { fileName: result.fileName }));
        }
      },
      onError: (error) => toast.error(displayError(error, t("settings.saveError"))),
    });
  };

  const runInspect = () => {
    inspect.mutate(undefined, {
      onSuccess: (result) => {
        if (!result.cancelled) {
          setRestoreOpen(true);
        }
      },
      onError: (error) => toast.error(displayError(error, t("settings.saveError"))),
    });
  };

  const runRestore = () => {
    if (!preview?.token) {
      return;
    }
    confirmRestore.mutate(
      { token: preview.token, confirmation, acknowledged, restoreChrome, restoreFormat, restoreRouting },
      {
        onSuccess: (result) => {
          if (result.restartRequired) {
            setRestarting(true);
          }
        },
        onError: (error) => toast.error(displayError(error, t("settings.saveError"))),
      },
    );
  };

  const runExport = (profile: string) => {
    exportCSV.mutate(
      { profile, includeArchived },
      {
        onSuccess: (result) => {
          if (!result.cancelled && result.fileName) {
            toast.success(t("settings.data.csvExported", { rows: result.rows ?? 0, fileName: result.fileName }));
          }
        },
        onError: (error) => toast.error(displayError(error, t("settings.saveError"))),
      },
    );
  };

  const pickCSV = (profile: string, token = csvToken) => {
    selectCSV.mutate(
      { profile, token },
      {
        onSuccess: (result) => {
          if (result.cancelled) {
            return;
          }
          setCsvProfile(profile);
          csvTokenRef.current = result.token ?? "";
          setCsvToken(result.token ?? "");
          setCsvDelimiter(result.delimiter ?? "comma");
          const next: Record<string, string> = {};
          const fields = profile === "holdings" ? HOLDING_FIELDS : ACCOUNT_FIELDS;
          for (const field of fields) {
            if ((result.headers ?? []).includes(field)) {
              next[field] = field;
            }
          }
          if (profile === "holdings") {
            setHoldingsHeaders(result.headers ?? []);
            setHoldingsMapping(next);
          } else {
            setAccountsHeaders(result.headers ?? []);
            setAccountsMapping(next);
          }
          setCsvStep(2);
          setCsvOpen(true);
        },
        onError: (error) => toast.error(displayError(error, t("settings.saveError"))),
      },
    );
  };

  const runPreview = () => {
    previewCSV.mutate(
      {
        token: csvToken,
        profile: csvProfile,
        delimiter: csvDelimiter,
        dateFormat: csvDateFormat,
        decimalSep: csvDecimalSep,
        groupingSep: csvGroupingSep,
        mapping: csvMapping,
        accountsMapping,
        holdingsMapping,
        unresolved: Object.values(unresolvedChoices).map((item) => ({
          kind: item.kind,
          name: item.name,
          action: item.action,
          mapTo: item.mapTo,
        })),
      },
      {
        onSuccess: (result) => {
          setCsvPreview(result);
          setUnresolvedChoices((current) => {
            const next = { ...current };
            for (const item of result.unresolved) {
              const key = unresolvedKey(item.kind, item.name);
              if (!next[key]) {
                next[key] = { kind: item.kind, name: item.name, action: "block", mapTo: "" };
              }
            }
            return next;
          });
          setCsvStep(3);
        },
        onError: (error) => toast.error(displayError(error, t("settings.saveError"))),
      },
    );
  };

  const runConfirm = () => {
    confirmCSV.mutate(csvToken, {
      onSuccess: () => setCsvStep(4),
      onError: (error) => toast.error(displayError(error, t("settings.saveError"))),
    });
  };

  const runCommit = () => {
    commitCSV.mutate(csvToken, {
      onSuccess: (stats) => {
        toast.success(t("settings.data.csvImported", { accounts: stats.createAccounts, holdings: stats.createHoldings }));
        setCsvOpen(false);
        resetCSV(false);
      },
      onError: (error) => toast.error(displayError(error, t("settings.saveError"))),
    });
  };

  return (
    <section className="flex max-w-2xl flex-col gap-3" aria-labelledby="settings-data-title">
      <div>
        <h2 id="settings-data-title" className="text-base font-semibold">{t("settings.data.title")}</h2>
        <p className="text-sm text-muted-foreground">{t("settings.data.description")}</p>
      </div>
      <p className="text-sm text-muted-foreground">{t("settings.data.backupSensitive")}</p>
      <p className="text-sm">
        <span className="font-medium">{t("settings.data.lastBackup")}: </span>
        {status.data?.available
          ? t("settings.data.lastBackupSummary", { fileName: status.data.backupFileName, schema: status.data.schemaVersion, result: status.data.verificationResult })
          : t("settings.data.lastBackupNone")}
      </p>
      <div className="flex flex-wrap gap-2">
        <Button type="button" onClick={runBackup} disabled={createBackup.isPending}>{t("settings.data.backup")}</Button>
        <Button type="button" variant="outline" onClick={runInspect} disabled={inspect.isPending}>{t("settings.data.restore")}</Button>
        <Button type="button" variant="outline" onClick={() => { setCsvOpen(true); setCsvStep(1); }}>{t("settings.data.csv")}</Button>
      </div>

      <AlertDialog open={restoreOpen || restarting} onOpenChange={setRestoreOpen}>
        <AlertDialogContent className="max-w-lg">
          <AlertDialogHeader>
            <AlertDialogTitle>{restarting ? t("settings.data.restoreRestart") : t("settings.data.restoreTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("settings.data.restoreDescription")}</AlertDialogDescription>
          </AlertDialogHeader>
          {preview && !restarting ? (
            <div className="grid gap-3 text-sm sm:grid-cols-2">
              <div>
                <p className="font-medium">{t("settings.data.restoreCurrent")}</p>
                <p>{t("settings.data.restoreHousehold", { name: preview.currentHousehold || "—", currency: preview.currentCurrency || "—" })}</p>
                <p>{t("settings.data.restoreCounts", { accounts: preview.currentAccounts ?? 0, holdings: preview.currentHoldings ?? 0, activities: preview.currentActivities ?? 0 })}</p>
              </div>
              <div>
                <p className="font-medium">{t("settings.data.restoreBackup")}</p>
                <p>{t("settings.data.restoreHousehold", { name: preview.backupHousehold || "—", currency: preview.backupCurrency || "—" })}</p>
                <p>{t("settings.data.restoreCounts", { accounts: preview.backupAccounts ?? 0, holdings: preview.backupHoldings ?? 0, activities: preview.backupActivities ?? 0 })}</p>
              </div>
            </div>
          ) : null}
          {!restarting ? (
            <div className="flex flex-col gap-2 text-sm">
              <p>{t("settings.data.restoreClassesHelp")}</p>
              <label className="flex items-center gap-2"><input type="checkbox" checked={restoreChrome} onChange={(event) => setRestoreChrome(event.target.checked)} /> {t("settings.data.restoreChrome")}</label>
              <label className="flex items-center gap-2"><input type="checkbox" checked={restoreFormat} onChange={(event) => setRestoreFormat(event.target.checked)} /> {t("settings.data.restoreFormat")}</label>
              <label className="flex items-center gap-2"><input type="checkbox" checked={restoreRouting} onChange={(event) => setRestoreRouting(event.target.checked)} /> {t("settings.data.restoreRouting")}</label>
              <label className="flex items-center gap-2"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /> {t("settings.data.restoreAcknowledge")}</label>
              <Label htmlFor="restore-confirm">{t("settings.data.restoreTypeLabel")}</Label>
              <Input id="restore-confirm" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} autoComplete="off" />
            </div>
          ) : null}
          <AlertDialogFooter>
            {!restarting ? <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel> : null}
            {!restarting ? (
              <Button type="button" onClick={runRestore} disabled={!restoreReady || confirmRestore.isPending}>{t("settings.data.restoreConfirm")}</Button>
            ) : null}
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={csvOpen}
        onOpenChange={(open) => {
          setCsvOpen(open);
          if (!open) {
            resetCSV();
          }
        }}
      >
        <AlertDialogContent className="max-w-3xl">
          <AlertDialogHeader>
            <AlertDialogTitle>{t("settings.data.csvTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {csvStep === 1 ? t("settings.data.csvStepFile") : csvStep === 2 ? t("settings.data.csvStepMapping") : csvStep === 3 ? t("settings.data.csvStepPreview") : t("settings.data.csvStepConfirm")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {csvStep === 1 ? (
            <div className="flex flex-col gap-3">
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={includeArchived} onChange={(event) => setIncludeArchived(event.target.checked)} />
                {t("settings.data.csvIncludeArchived")}
              </label>
              <div className="flex flex-wrap gap-2">
                <Button type="button" onClick={() => runExport("accounts")}>{t("settings.data.csvExport")} · {t("settings.data.csvProfileAccounts")}</Button>
                <Button type="button" onClick={() => runExport("holdings")}>{t("settings.data.csvExport")} · {t("settings.data.csvProfileHoldings")}</Button>
                <Button type="button" variant="outline" onClick={() => pickCSV("accounts", "")}>{t("settings.data.csvImport")} · {t("settings.data.csvProfileAccounts")}</Button>
                <Button type="button" variant="outline" onClick={() => pickCSV("holdings", "")}>{t("settings.data.csvImport")} · {t("settings.data.csvProfileHoldings")}</Button>
              </div>
            </div>
          ) : null}
          {csvStep === 2 ? (
            <div className="flex flex-col gap-3">
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <div>
                  <Label htmlFor="csv-delimiter">{t("settings.data.csvDelimiter")}</Label>
                  <NativeSelect id="csv-delimiter" value={csvDelimiter} onChange={(event) => setCsvDelimiter(event.target.value)}>
                    <option value="comma">{t("settings.data.csvDelimiterComma")}</option>
                    <option value="semicolon">{t("settings.data.csvDelimiterSemicolon")}</option>
                    <option value="tab">{t("settings.data.csvDelimiterTab")}</option>
                  </NativeSelect>
                </div>
                <div>
                  <Label htmlFor="csv-date">{t("settings.data.csvDateFormat")}</Label>
                  <NativeSelect id="csv-date" value={csvDateFormat} onChange={(event) => setCsvDateFormat(event.target.value)}>
                    <option value="iso">{t("settings.data.csvDateISO")}</option>
                    <option value="day-first">{t("settings.data.csvDateDayFirst")}</option>
                    <option value="month-first">{t("settings.data.csvDateMonthFirst")}</option>
                  </NativeSelect>
                </div>
                <div>
                  <Label htmlFor="csv-decimal">{t("settings.data.csvDecimal")}</Label>
                  <NativeSelect id="csv-decimal" value={csvDecimalSep} onChange={(event) => setCsvDecimalSep(event.target.value)}>
                    <option value=".">.</option>
                    <option value=",">,</option>
                  </NativeSelect>
                </div>
                <div>
                  <Label htmlFor="csv-grouping">{t("settings.data.csvGrouping")}</Label>
                  <NativeSelect id="csv-grouping" value={csvGroupingSep} onChange={(event) => setCsvGroupingSep(event.target.value)}>
                    <option value="none">{t("settings.data.csvGroupingNone")}</option>
                    <option value=",">,</option>
                    <option value=".">.</option>
                    <option value="space">{t("settings.data.csvGroupingSpace")}</option>
                  </NativeSelect>
                </div>
              </div>
              {hasAccountsFile && hasHoldingsFile ? (
                <div>
                  <Label htmlFor="csv-mapping-profile">{t("settings.data.csvMappingProfile")}</Label>
                  <NativeSelect id="csv-mapping-profile" value={csvProfile} onChange={(event) => setCsvProfile(event.target.value)}>
                    <option value="accounts">{t("settings.data.csvProfileAccounts")}</option>
                    <option value="holdings">{t("settings.data.csvProfileHoldings")}</option>
                  </NativeSelect>
                </div>
              ) : (
                <p className="text-sm text-muted-foreground">{t("settings.data.csvAddHoldings")}</p>
              )}
              {csvToken && !hasHoldingsFile ? (
                <Button type="button" variant="outline" onClick={() => pickCSV("holdings", csvToken)}>{t("settings.data.csvProfileHoldings")}</Button>
              ) : null}
              <div className="grid max-h-64 gap-2 overflow-auto sm:grid-cols-2">
                {mappingFields.map((field) => (
                  <div key={field}>
                    <Label htmlFor={`csv-map-${field}`}>{field}</Label>
                    <NativeSelect
                      id={`csv-map-${field}`}
                      value={csvMapping[field] ?? ""}
                      onChange={(event) => setCsvMapping((current) => ({ ...current, [field]: event.target.value }))}
                    >
                      <option value="">{t("common.selectOption")}</option>
                      {csvHeaders.map((header) => (
                        <option key={header} value={header}>{header}</option>
                      ))}
                    </NativeSelect>
                  </div>
                ))}
              </div>
              {csvNumberFormatConflict ? <p role="alert" className="text-sm text-destructive">{t("settings.data.csvNumberFormatConflict")}</p> : null}
              <Button type="button" onClick={runPreview} disabled={csvNumberFormatConflict || previewCSV.isPending}>{t("settings.data.csvStepPreview")}</Button>
            </div>
          ) : null}
          {csvStep === 3 && csvPreview ? (
            <div className="flex max-h-[50vh] flex-col gap-3 overflow-auto text-sm">
              <p>{t("settings.data.csvStats", { creates: csvPreview.stats.createAccounts + csvPreview.stats.createHoldings, errors: csvPreview.stats.errors, warnings: csvPreview.stats.warnings })}</p>
              {csvPreview.unresolved.length > 0 ? (
                <div className="flex flex-col gap-2">
                  <p className="font-medium">{t("settings.data.csvUnresolvedTitle")}</p>
                  <p className="text-muted-foreground">{t("settings.data.csvUnresolvedHelp")}</p>
                  {csvPreview.unresolved.map((item) => {
                    const key = unresolvedKey(item.kind, item.name);
                    const choice = unresolvedChoices[key] ?? { kind: item.kind, name: item.name, action: "block", mapTo: "" };
                    return (
                      <div key={key} className="grid gap-2 sm:grid-cols-3">
                        <p className="self-center">{item.kind}: {item.name}</p>
                        <NativeSelect
                          aria-label={`${item.kind} ${item.name}`}
                          value={choice.action}
                          onChange={(event) => setUnresolvedChoices((current) => ({
                            ...current,
                            [key]: { ...choice, action: event.target.value },
                          }))}
                        >
                          <option value="block">{t("settings.data.csvUnresolvedBlock")}</option>
                          <option value="create">{t("settings.data.csvUnresolvedCreate")}</option>
                          <option value="map">{t("settings.data.csvUnresolvedMap")}</option>
                        </NativeSelect>
                        {choice.action === "map" ? (
                          <Input
                            aria-label={t("settings.data.csvUnresolvedMapTo")}
                            value={choice.mapTo}
                            onChange={(event) => setUnresolvedChoices((current) => ({
                              ...current,
                              [key]: { ...choice, mapTo: event.target.value },
                            }))}
                          />
                        ) : null}
                      </div>
                    );
                  })}
                  <Button type="button" variant="outline" onClick={runPreview}>{t("settings.data.csvRetryPreview")}</Button>
                </div>
              ) : null}
              {csvPreview.errors.length > 0 ? (
                <div className="flex flex-col gap-2">
                  <p className="font-medium">{t("settings.data.csvErrors")}</p>
                  <div className="overflow-x-auto">
                    <table className="w-full text-left text-xs" aria-label={t("settings.data.csvErrors")}>
                      <thead>
                        <tr>
                          <th className="border-b px-2 py-1">{t("settings.data.csvErrorRow")}</th>
                          <th className="border-b px-2 py-1">{t("settings.data.csvErrorField")}</th>
                          <th className="border-b px-2 py-1">{t("settings.data.csvErrorSource")}</th>
                          <th className="border-b px-2 py-1">{t("settings.data.csvErrorCode")}</th>
                          <th className="border-b px-2 py-1">{t("settings.data.csvErrorMessage")}</th>
                          <th className="border-b px-2 py-1">{t("settings.data.csvErrorSuggestion")}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {csvPreview.errors.map((item, index) => (
                          <tr key={`${item.row}-${item.field}-${index}`}>
                            <td className="border-b px-2 py-1">{item.row}</td>
                            <td className="border-b px-2 py-1">{item.field}</td>
                            <td className="border-b px-2 py-1">{item.source || "—"}</td>
                            <td className="border-b px-2 py-1">{item.code}</td>
                            <td className="border-b px-2 py-1">{item.message}</td>
                            <td className="border-b px-2 py-1">{item.suggestion || "—"}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                  <Button type="button" variant="outline" onClick={() => downloadErrors.mutate(csvToken)}>{t("settings.data.csvDownloadErrors")}</Button>
                </div>
              ) : null}
              <div className="overflow-x-auto">
                <table className="w-full text-left text-xs" aria-label={t("settings.data.csvPreviewRows")}>
                  <thead>
                    <tr>{csvPreview.headers.map((header) => <th key={header} className="border-b px-2 py-1">{header}</th>)}</tr>
                  </thead>
                  <tbody>
                    {csvPreview.previewRows.map((row, index) => (
                      <tr key={index}>{row.map((cell, cellIndex) => <td key={cellIndex} className="border-b px-2 py-1">{cell}</td>)}</tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <Button type="button" onClick={runConfirm} disabled={!csvPreview.canCommit || confirmCSV.isPending}>{t("settings.data.csvStepConfirm")}</Button>
            </div>
          ) : null}
          {csvStep === 4 && csvPreview ? (
            <div className="flex flex-col gap-3">
              <p>{t("settings.data.csvCreateOnly")}</p>
              <Button type="button" onClick={runCommit} disabled={commitCSV.isPending}>{t("settings.data.csvImportAll")}</Button>
            </div>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}

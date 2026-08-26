import { useTranslation } from "react-i18next";
import type { StartupDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/app/models";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { LoadingState } from "@/components/layout/PageState";
import { parseWailsError, translateWailsError, type WireError } from "@/lib/wails";

export function StartupLoadingPage({ label }: { label: string }) {
  return (
    <main className="flex min-h-[100dvh] items-center justify-center bg-background p-6">
      <div className="w-full max-w-lg">
        <div className="mb-5 flex items-center gap-3">
          <div className="flex size-10 items-center justify-center rounded-xl bg-primary text-lg font-semibold text-primary-foreground" aria-hidden="true">
            N
          </div>
          <div>
            <p className="text-sm font-semibold text-foreground">Nestworth</p>
            <p className="text-xs text-muted-foreground">{label}</p>
          </div>
        </div>
        <LoadingState label={label} />
      </div>
    </main>
  );
}

/**
 * BlockedStartupPage is shown when the local database could not be opened,
 * so business writes stay disabled. Retry
 * is "quit and relaunch" rather than an in-process reopen — Wails
 * services are constructed once at process start.
 */
export function BlockedStartupPage({ startup, failure }: { startup?: StartupDTO | null; failure?: unknown }) {
  const { t } = useTranslation();
  const wireError: WireError = startup?.code
    ? { code: startup.code, field: startup.field, message: startup.message ?? "" }
    : failure && typeof failure === "object" && "wireError" in failure
      ? ((failure as Error & { wireError: WireError }).wireError ?? parseWailsError(failure))
      : parseWailsError(failure);
  const message = startup?.code || failure ? translateWailsError(wireError) : t("startup.genericFailure");
  const diagnosticCode = startup?.code ?? wireError.code;

  return (
    <main className="flex min-h-[100dvh] w-screen items-center justify-center bg-background p-6">
      <Card className="w-full max-w-lg" role="alert">
        <CardHeader>
          <CardTitle>{t("startup.blockedTitle")}</CardTitle>
          <CardDescription>{t("startup.blockedDescription")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <p className="text-sm text-muted-foreground">{t("startup.readOnly")}</p>
          <p className="rounded-md border border-warning/40 bg-warning/10 p-3 text-sm text-warning-foreground">{message}</p>
          <div className="flex flex-wrap items-center gap-2">
            <Button type="button" onClick={() => window.location.reload()}>
              {t("startup.retry")}
            </Button>
            <details className="text-sm text-muted-foreground">
              <summary className="cursor-pointer rounded-md px-2 py-1 underline underline-offset-4 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50">
                {t("startup.diagnosis")}
              </summary>
              <p className="mt-2 max-w-sm">{t("startup.diagnosisDescription")}</p>
              <code className="mt-1 block rounded bg-muted px-2 py-1 text-xs text-foreground">{diagnosticCode}</code>
            </details>
          </div>
        </CardContent>
      </Card>
    </main>
  );
}

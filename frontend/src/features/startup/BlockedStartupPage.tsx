import { useTranslation } from "react-i18next";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

/**
 * BlockedStartupPage mirrors internal/ui.NewBlockedStartupPage: the local
 * database could not be opened, so business writes stay disabled. Retry
 * is "quit and relaunch" rather than an in-process reopen — Wails
 * services are constructed once at process start.
 */
export function BlockedStartupPage() {
  const { t } = useTranslation();
  return (
    <div className="flex h-screen w-screen items-center justify-center bg-background p-6">
      <Card className="max-w-lg" role="alert">
        <CardHeader>
          <CardTitle>{t("startup.blockedTitle")}</CardTitle>
          <CardDescription>{t("startup.blockedDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">{t("startup.readOnly")}</p>
        </CardContent>
      </Card>
    </div>
  );
}

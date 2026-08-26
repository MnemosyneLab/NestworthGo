import { useTranslation } from "react-i18next";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { useAppInfo } from "@/queries/app";

/**
 * AboutPage is the dedicated About surface (implementation plan Phase 5)
 * backed by AppService.AppInfo(). It is also embedded on Settings.
 */
export function AboutPage() {
  const { t } = useTranslation();
  const appInfo = useAppInfo();
  const info = appInfo.data;
  const versionLine = info
    ? t("about.version").includes("%s")
      ? t("about.version").replace("%s", info.version).replace("%s", info.build)
      : t("about.version", { version: info.version, build: info.build })
    : "";

  return (
    <Card aria-label={t("about.title")}>
      <CardHeader>
        <CardTitle>{info?.name ?? t("app.name")}</CardTitle>
        <CardDescription>{t("about.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-1 text-sm text-muted-foreground">
        {info && <p>{versionLine}</p>}
        <p>{t("about.license")}</p>
        {info?.appId && <p>{info.appId}</p>}
      </CardContent>
    </Card>
  );
}

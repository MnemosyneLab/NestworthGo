import { useState } from "react";
import { useTranslation } from "react-i18next";
import { AlertTriangle } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import type { ReturnIssueDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export function CompletenessBanner({
  issues,
  ratedDays,
  totalDays,
}: {
  issues: ReturnIssueDTO[];
  ratedDays: number;
  totalDays: number;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const partial = ratedDays < totalDays || issues.length > 0;
  if (!partial) return null;

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-warning/40 bg-warning/10 px-4 py-3 text-sm" role="status">
        <div className="flex items-center gap-2 text-warning-foreground">
          <AlertTriangle className="size-4 shrink-0" aria-hidden="true" />
          <span>{t("insights.partial")}</span>
          <Badge variant="warning">{ratedDays}/{totalDays}</Badge>
        </div>
        {issues.length > 0 && <Button type="button" variant="outline" size="sm" onClick={() => setOpen(true)}>{t("insights.issues")} ({issues.length})</Button>}
      </div>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent side="right">
          <SheetHeader>
            <SheetTitle>{t("insights.issues")}</SheetTitle>
            <p className="text-sm text-muted-foreground">{t("insights.coverage")}: {ratedDays}/{totalDays}</p>
          </SheetHeader>
          <ul className="flex flex-col gap-2 overflow-y-auto text-sm">
            {issues.map((issue) => (
              <li key={`${issue.date}-${issue.status}`} className="rounded-md border border-border px-3 py-2">
                <div className="font-medium">{issue.date}</div>
                <div className="text-muted-foreground">{issue.missingReason ?? issue.status}</div>
              </li>
            ))}
          </ul>
        </SheetContent>
      </Sheet>
    </>
  );
}

import { Badge } from "@/components/ui/badge";
import { useTranslation } from "react-i18next";

export function reviewPriority(value: string): "p0" | "p1" | "p2" {
  return value === "p0" ? "p0" : value === "p1" || value === "warning" ? "p1" : "p2";
}

export function ReviewPriority({ severity }: { severity: string }) {
  const { t } = useTranslation();
  const priority = reviewPriority(severity);
  return <Badge variant={priority === "p0" ? "destructive" : priority === "p1" ? "warning" : "outline"} className="shrink-0 font-semibold">{t(`reviewV2.priority.${priority}`)}</Badge>;
}

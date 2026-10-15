import { ChevronDown, ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { InstrumentLabel } from "@/components/forms/InstrumentLabel";
import { instrumentDisplayLabel, type InstrumentIdentityFields } from "@/lib/instrumentDisplay";

/** Shared row identity and disclosure; row actions remain independent siblings. */
export function HoldingDisclosure({ expanded, controls, onToggle, ...identity }: InstrumentIdentityFields & {
  expanded: boolean;
  controls: string;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  return <div className="flex min-w-0 flex-1 items-center gap-2">
    <Button type="button" variant="ghost" size="icon" className={`shrink-0 ${expanded ? "bg-muted" : ""}`}
      aria-label={t(expanded ? "holdingDetail.collapse" : "holdingDetail.toggle", { name: identity.name || instrumentDisplayLabel(identity) })}
      aria-expanded={expanded} aria-controls={controls} onClick={onToggle}>
      {expanded ? <ChevronDown aria-hidden="true" /> : <ChevronRight aria-hidden="true" />}
    </Button>
    <InstrumentLabel {...identity} className="flex-1" />
  </div>;
}

import { ChevronDown, ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { InstrumentLabel } from "@/components/forms/InstrumentLabel";
import { instrumentDisplayLabel, type InstrumentIdentityFields } from "@/lib/instrumentDisplay";

/** Shared row identity and disclosure; row actions remain independent siblings. */
export function HoldingDisclosure({ expanded, controls, onToggle, toggleLabel, children, ...identity }: InstrumentIdentityFields & {
  expanded: boolean;
  controls: string;
  onToggle: () => void;
  toggleLabel?: string;
  children?: React.ReactNode;
}) {
  const { t } = useTranslation();
  return <div className="flex min-w-0 flex-1 items-center gap-2">
    <Button type="button" variant="ghost" size="icon" className={`shrink-0 ${expanded ? "bg-muted" : ""}`}
      aria-label={toggleLabel ?? t(expanded ? "holdingDetail.collapse" : "holdingDetail.toggle", { name: identity.name || instrumentDisplayLabel(identity) })}
      aria-expanded={expanded} aria-controls={controls} onClick={onToggle}>
      {expanded ? <ChevronDown aria-hidden="true" /> : <ChevronRight aria-hidden="true" />}
    </Button>
    {children ?? <InstrumentLabel {...identity} className="flex-1" />}
  </div>;
}

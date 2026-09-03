import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@/components/ui/label";
import { ICON_CATALOG, iconChoicesByCategory } from "@/lib/iconCatalog";
import { EntityIcon, type EntityIconKind } from "@/components/icons/EntityIcon";
import { cn } from "@/lib/utils";

export function IconPicker({
  id,
  value,
  onChange,
  kind,
}: {
  id: string;
  value: string;
  onChange: (key: string) => void;
  kind: EntityIconKind;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const selected = ICON_CATALOG.find((choice) => choice.key === value);
  const selectedLabel = selected ? t(selected.labelKey) : t("common.chooseIcon");
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{t("common.icon")}</Label>
      <details className="rounded-md border border-border bg-background" open={open}>
        <summary
          id={id}
          aria-label={t("common.chooseIcon")}
          className="flex cursor-pointer list-none items-center gap-2 px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50"
          onClick={(event) => {
            event.preventDefault();
            setOpen((value) => !value);
          }}
          onKeyDown={(event) => {
            if (event.key !== "Enter" && event.key !== " ") return;
            event.preventDefault();
            setOpen((value) => !value);
          }}
        >
          <EntityIcon iconKey={value} kind={kind} className="size-5 text-primary" />
          <span>{selectedLabel}</span>
        </summary>
        {open && (
          <div className="max-h-72 overflow-y-auto border-t border-border p-3">
            {iconChoicesByCategory().map((group) => (
              <section key={group.categoryKey} className="mb-4 last:mb-0">
                <h4 className="mb-2 text-xs font-medium text-muted-foreground">{t(group.categoryKey)}</h4>
                <div className="grid grid-cols-3 gap-2 sm:grid-cols-4">
                  {group.choices.map((choice) => (
                    <button key={choice.key} type="button" aria-pressed={value === choice.key} onClick={() => onChange(choice.key)}
                      className={cn("flex min-h-16 flex-col items-center justify-center gap-1 rounded-md border px-2 py-2 text-center text-xs hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50", value === choice.key && "border-primary bg-primary/10 text-primary")}>
                      <EntityIcon iconKey={choice.key} kind={kind} className="size-5" />
                      <span>{t(choice.labelKey)}</span>
                    </button>
                  ))}
                </div>
              </section>
            ))}
          </div>
        )}
      </details>
    </div>
  );
}

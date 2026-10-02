import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/select";
import { ICON_CATALOG, iconChoicesByCategory } from "@/lib/iconCatalog";
import { BANK_LOGOS, bankLogo } from "@/lib/bankLogos";
import { EntityIcon, type EntityIconKind } from "@/components/icons/EntityIcon";
import { cn } from "@/lib/utils";

export function IconPicker({ id, value, onChange, kind, inheritIconKey }: {
  id: string;
  value: string;
  onChange: (key: string) => void;
  kind: EntityIconKind;
  inheritIconKey?: string;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [category, setCategory] = useState(bankLogo(value) ? "bank" : "generic");
  const [search, setSearch] = useState("");
  const trigger = useRef<HTMLElement>(null);
  const canInherit = kind === "account" && inheritIconKey !== undefined;
  const canChooseBank = kind === "institution" || kind === "account";
  const selected = ICON_CATALOG.find((choice) => choice.key === value);
  const selectedLabel = !value && canInherit ? t("icons.inheritInstitution") : bankLogo(value)?.name ?? (selected ? t(selected.labelKey) : t("common.chooseIcon"));
  const normalizeSearch = (text: string) => text.normalize("NFKC").toLocaleLowerCase().replace(/[\s_-]+/g, "");
  const query = normalizeSearch(search);
  const groups = category === "bank" && canChooseBank
    ? [{ categoryKey: "icons.category.bankLogos", choices: BANK_LOGOS.filter((logo) => [logo.name, ...logo.aliases].some((text) => normalizeSearch(text).includes(query))).map((logo) => ({ key: logo.key, label: logo.name })) }]
    : iconChoicesByCategory().map((group) => ({ categoryKey: group.categoryKey, choices: group.choices.map((choice) => ({ key: choice.key, label: t(choice.labelKey) })).filter((choice) => normalizeSearch(`${choice.label} ${choice.key}`).includes(query)) }));
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{t("common.icon")}</Label>
      <details className="rounded-md border border-border bg-background" open={open} onKeyDown={(event) => {
        if (event.key === "Escape") { event.preventDefault(); setOpen(false); trigger.current?.focus(); }
      }}>
        <summary ref={trigger} id={id} aria-label={t("common.chooseIcon")} aria-expanded={open}
          className="flex cursor-pointer list-none items-center gap-2 px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50"
          onClick={(event) => { event.preventDefault(); setOpen((value) => !value); }}
          onKeyDown={(event) => {
            if (event.key !== "Enter" && event.key !== " ") return;
            event.preventDefault(); setOpen((value) => !value);
          }}>
          <EntityIcon iconKey={value || inheritIconKey} kind={kind} className="size-5 text-primary" />
          <span>{selectedLabel}</span>
        </summary>
        {open && (
          <div className="border-t border-border p-3">
            {canChooseBank && <div className="mb-3 flex flex-col gap-2">
              <Label htmlFor={`${id}-category`}>{t("icons.categoryLabel")}</Label>
              <NativeSelect id={`${id}-category`} value={category} onChange={(event) => setCategory(event.target.value)}>
                <option value="generic">{t("icons.genericIcons")}</option>
                <option value="bank">{t("icons.category.bankLogos")}</option>
              </NativeSelect>
              <Label htmlFor={`${id}-search`}>{t("icons.search")}</Label>
              <Input id={`${id}-search`} type="search" value={search} onChange={(event) => setSearch(event.target.value)} />
            </div>}
            <div className="max-h-72 overflow-y-auto">
              {!groups.some((group) => group.choices.length) && <p role="status" className="py-4 text-sm text-muted-foreground">{t("icons.noResults")}</p>}
              {groups.filter((group) => group.choices.length).map((group) => (
                <section key={group.categoryKey} className="mb-4 last:mb-0">
                  <h4 className="mb-2 text-xs font-medium text-muted-foreground">{t(group.categoryKey)}</h4>
                  <div className="grid grid-cols-3 gap-2 sm:grid-cols-4">
                    {group.choices.map((choice) => (
                      <button key={choice.key} type="button" aria-pressed={value === choice.key} onClick={() => onChange(choice.key)}
                        className={cn("flex min-h-16 flex-col items-center justify-center gap-1 rounded-md border px-2 py-2 text-center text-xs hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50", value === choice.key && "border-primary bg-primary/10 text-primary")}>
                        <EntityIcon iconKey={choice.key} kind={kind} className={category === "bank" ? "size-8" : "size-5"} />
                        <span className="break-words">{choice.label}</span>
                      </button>
                    ))}
                  </div>
                </section>
              ))}
            </div>
          </div>
        )}
      </details>
      {canInherit && <button type="button" aria-pressed={!value} onClick={() => onChange("")}
        className="self-start rounded text-sm text-primary underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50">{t("icons.inheritInstitution")}</button>}
      {canInherit && <p className="text-xs text-muted-foreground">{t("icons.inheritHint")}</p>}
    </div>
  );
}

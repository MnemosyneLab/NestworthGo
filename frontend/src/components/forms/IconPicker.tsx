import { useTranslation } from "react-i18next";
import { Label } from "@/components/ui/label";
import { iconChoicesByCategory } from "@/lib/iconCatalog";

export function IconPicker({
  id,
  value,
  onChange,
}: {
  id: string;
  value: string;
  onChange: (key: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{t("common.icon")}</Label>
      <select
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground"
        aria-label={t("common.chooseIcon")}
      >
        <option value="">{t("accounts.none")}</option>
        {iconChoicesByCategory().map((group) => (
          <optgroup key={group.categoryKey} label={t(group.categoryKey)}>
            {group.choices.map((choice) => (
              <option key={choice.key} value={choice.key}>
                {t(choice.labelKey)}
              </option>
            ))}
          </optgroup>
        ))}
      </select>
    </div>
  );
}

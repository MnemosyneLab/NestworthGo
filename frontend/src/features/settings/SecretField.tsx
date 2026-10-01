import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

/** Stored values never enter this component. Visibility applies to new input only. */
export function SecretField({ id, label, hasValue, value, onChange, onRemove, disabled = false, removeLabel }: {
  id: string; label: string; hasValue: boolean; value: string; onChange: (value: string) => void;
  onRemove?: () => void; disabled?: boolean; removeLabel?: string;
}) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);
  const [visible, setVisible] = useState(false);
  const showInput = !hasValue || editing || value !== "";
  return <div className="flex flex-col gap-2">
    <Label id={`${id}-label`} htmlFor={showInput ? id : undefined}>{label}</Label>
    {hasValue && <div className="flex flex-wrap items-center gap-2">
      <span aria-label={t("secretField.configured")} className="font-mono">••••••••</span>
      {!showInput && <Button aria-describedby={`${id}-label`} type="button" variant="outline" onClick={() => setEditing(true)} disabled={disabled}>{t("secretField.replace")}</Button>}
      {onRemove && <Button aria-describedby={`${id}-label`} type="button" variant="outline" onClick={onRemove} disabled={disabled}>{removeLabel ?? t("secretField.remove")}</Button>}
    </div>}
    {showInput && <div className="flex items-center gap-2">
      <Input id={id} type={visible ? "text" : "password"} autoComplete="new-password" spellCheck={false} value={value} onChange={(event) => onChange(event.target.value)} disabled={disabled} />
      <Button aria-describedby={`${id}-label`} type="button" variant="outline" aria-controls={id} aria-pressed={visible} onClick={() => setVisible(!visible)} disabled={disabled}>{t(visible ? "secretField.hide" : "secretField.show")}</Button>
      {hasValue && <Button aria-describedby={`${id}-label`} type="button" variant="ghost" onClick={() => { onChange(""); setVisible(false); setEditing(false); }} disabled={disabled}>{t("common.cancel")}</Button>}
    </div>}
  </div>;
}

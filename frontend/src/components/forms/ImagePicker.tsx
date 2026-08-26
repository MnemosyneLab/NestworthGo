import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { usePickImage } from "@/queries/media";

/**
 * ImagePicker is the native file-dialog flow (MediaService.PickImage)
 * wired into a form. The pick is confirm-time deferred: the selected PNG
 * stays as pending base64 until the surrounding form submits and the
 * caller persists it via CreateMediaAsset + SetXxxLogo/Avatar.
 */
export function ImagePicker({
  label,
  value,
  existingAssetId,
  onChange,
}: {
  label: string;
  value?: string;
  existingAssetId?: string | null;
  onChange: (dataBase64: string | undefined) => void;
}) {
  const { t } = useTranslation();
  const pickImage = usePickImage();
  const [error, setError] = useState<string | null>(null);

  const choose = async () => {
    setError(null);
    try {
      const picked = await pickImage.mutateAsync(label);
      if (picked) {
        onChange(picked);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t("media.pickError"));
    }
  };

  return (
    <div className="flex flex-col gap-2">
      <Label>{label}</Label>
      <div className="flex items-center gap-3">
        {(value || existingAssetId) && (
          <div className="size-12 overflow-hidden rounded-md border border-border bg-muted" aria-label={value ? t("common.selectedImage") : t("common.currentImage")}>
            {value ? (
              <img src={`data:image/png;base64,${value}`} alt={t("common.selectedImage")} className="size-full object-cover" />
            ) : (
              <div className="size-full bg-muted" aria-hidden />
            )}
          </div>
        )}
        <Button type="button" variant="outline" size="sm" onClick={() => void choose()} disabled={pickImage.isPending}>
          {label}
        </Button>
        {value && (
          <Button type="button" variant="ghost" size="sm" onClick={() => onChange(undefined)} aria-label={t("common.removeImage")}>
            {t("common.removeImage")}
          </Button>
        )}
      </div>
      {error && (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}

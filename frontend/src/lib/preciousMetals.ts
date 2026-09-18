import type { TFunction } from "i18next";

export function metalUnitLabel(unit: string | undefined, t: TFunction): string {
  return unit === "g" || unit === "troy_oz" ? t(`metals.${unit}`) : "";
}

export function metalPriceSuffix(instrument: { quantityUnit?: string } | undefined, t: TFunction): string {
  const unit = metalUnitLabel(instrument?.quantityUnit, t);
  return unit ? ` / ${unit}` : "";
}

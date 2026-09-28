import { useState } from "react";
import type { ProductCommandRequest, ProductOperationPreviewDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

// A response belongs to the command that requested it, even if the form changes
// while the request is in flight. Never enable confirmation for another command.
export function useReviewedCommand(command: ProductCommandRequest | null) {
  const key = JSON.stringify(command);
  const [result, setResult] = useState<{ key: string; preview: ProductOperationPreviewDTO } | null>(null);
  return {
    reviewed: result?.key === key ? result.preview : null,
    setReviewed: (preview: ProductOperationPreviewDTO | null) => setResult(preview ? { key, preview } : null),
  };
}

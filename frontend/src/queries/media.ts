import { useMutation } from "@tanstack/react-query";
import { Service as MediaService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/media";
import { callService } from "@/lib/wails";

export function usePickImage() {
  return useMutation({
    mutationFn: (title: string) => callService(() => MediaService.PickImage(title)),
  });
}

export async function persistPickedImage(
  dataBase64: string,
  attach: (mediaAssetId: string) => Promise<unknown>,
): Promise<void> {
  const asset = await callService(() => MediaService.CreateMediaAsset("image/png", dataBase64));
  await attach(asset.id);
}

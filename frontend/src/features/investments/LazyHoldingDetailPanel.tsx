import { lazy, Suspense, type ComponentProps } from "react";
import { useTranslation } from "react-i18next";
import { LoadingState } from "@/components/layout/PageState";

const Detail = lazy(() => import("./HoldingDetailPanel").then(module => ({ default: module.HoldingDetailPanel })));

export function HoldingDetailPanel(props: ComponentProps<typeof Detail>) {
  const { t } = useTranslation();
  return <Suspense fallback={<LoadingState label={t("ui.state.loadingPage")} />}><Detail {...props} /></Suspense>;
}

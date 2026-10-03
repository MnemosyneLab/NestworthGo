import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";

/** Native WebKit/overlay scrollbars need an explicit, keyboard-accessible fallback. */
export function ComparisonScroll({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const id = useId();
  const viewport = useRef<HTMLDivElement>(null);
  const [bounds, setBounds] = useState({ left: 0, max: 0, compact: false });
  const measure = () => {
    const element = viewport.current;
    if (!element) return;
    const next = { left: element.scrollLeft, max: Math.max(0, element.scrollWidth - element.clientWidth), compact: element.clientWidth < 360 };
    setBounds(previous => previous.left === next.left && previous.max === next.max && previous.compact === next.compact ? previous : next);
  };
  useEffect(() => {
    const element = viewport.current;
    if (!element) return;
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    if (element.firstElementChild) observer.observe(element.firstElementChild);
    return () => observer.disconnect();
  }, []);
  const scroll = (direction: -1 | 1) => {
    const element = viewport.current;
    if (!element) return;
    const column = element.querySelector<HTMLElement>("[data-account-column]");
    const occludedWidth = column && getComputedStyle(column).position === "sticky"
      ? column.getBoundingClientRect().width : 0;
    // Overlap the visible monetary area, not the full viewport: fixed names
    // cover part of each scroll position. Measuring also handles resized text.
    const step = Math.max(1, (element.clientWidth - occludedWidth) * 0.75);
    element.scrollLeft = Math.max(0, Math.min(element.scrollWidth - element.clientWidth, element.scrollLeft + direction * step));
    measure();
  };
  return <div className="min-w-0">
    {bounds.max > 0 && <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
      <p className="text-xs text-muted-foreground">{t("historicalOverview.scrollHint")}</p>
      <div className="flex flex-wrap gap-2">
        <Button size="sm" variant="outline" aria-controls={id} disabled={bounds.left <= 0} onClick={() => scroll(-1)}>
          <ChevronLeft aria-hidden="true" />{t("historicalOverview.scrollLeft")}
        </Button>
        <Button size="sm" variant="outline" aria-controls={id} disabled={bounds.left >= bounds.max - 1} onClick={() => scroll(1)}>
          {t("historicalOverview.scrollRight")}<ChevronRight aria-hidden="true" />
        </Button>
      </div>
    </div>}
    <div ref={viewport} id={id} role="region" aria-label={t("historicalOverview.tableRegion")} tabIndex={0}
      className={`min-w-0 overflow-x-auto rounded-xl border border-border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 ${bounds.compact ? "[&_[data-account-column]]:static" : ""}`}
      onScroll={measure} onKeyDown={event => {
        if (event.target !== event.currentTarget || event.altKey || event.ctrlKey || event.metaKey) return;
        if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
          event.preventDefault();
          scroll(event.key === "ArrowLeft" ? -1 : 1);
        }
      }}>
      {children}
    </div>
  </div>;
}

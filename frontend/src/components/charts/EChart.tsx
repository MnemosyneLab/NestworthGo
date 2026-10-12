import * as React from "react";
import * as echarts from "echarts/core";
import { LineChart, BarChart, PieChart } from "echarts/charts";
import { GridComponent, TooltipComponent, LegendComponent, DataZoomComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import type { ComposeOption } from "echarts/core";
import type { LineSeriesOption, BarSeriesOption, PieSeriesOption } from "echarts/charts";
import type { GridComponentOption, TooltipComponentOption, LegendComponentOption, DataZoomComponentOption } from "echarts/components";
import { cssColor, prefersReducedMotion } from "@/components/charts/chartTheme";

echarts.use([LineChart, BarChart, PieChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent, CanvasRenderer]);

export type EChartsOption = ComposeOption<
  LineSeriesOption | BarSeriesOption | PieSeriesOption | GridComponentOption | TooltipComponentOption | LegendComponentOption | DataZoomComponentOption
>;

export function applyChartPolicy(option: EChartsOption, reducedMotion = prefersReducedMotion()): EChartsOption {
  // CSS OKLCH colors render normally, but zrender 6 cannot parse them for
  // its automatic emphasis color lift. ECharts' inherit path preserves the
  // exact normal/selected fill, including opacity and per-point theme colors.
  const keepEmphasisFill = <T extends LineSeriesOption | BarSeriesOption | PieSeriesOption>(series: T): T => ({
    ...series,
    emphasis: {
      ...series.emphasis,
      itemStyle: { ...series.emphasis?.itemStyle, color: series.emphasis?.itemStyle?.color ?? "inherit" },
    },
  });
  const series = Array.isArray(option.series)
    ? option.series.map(keepEmphasisFill)
    : option.series ? keepEmphasisFill(option.series) : option.series;
  const tooltip = option.tooltip && typeof option.tooltip === "object" && !Array.isArray(option.tooltip)
    ? {
      ...option.tooltip,
      renderMode: "html" as const,
      appendTo: "body",
      backgroundColor: cssColor("--color-card", "#ffffff"),
      borderColor: cssColor("--color-border", "#d7dbe7"),
      borderWidth: option.tooltip.borderWidth ?? 1,
      textStyle: {
        ...option.tooltip.textStyle,
        color: cssColor("--color-card-foreground", "#2a2f3a"),
      },
      extraCssText: [
        option.tooltip.extraCssText,
        "z-index: 10000",
        "opacity: 1",
        "pointer-events: none",
        "white-space: nowrap",
      ].filter(Boolean).join(";"),
    }
    : option.tooltip;
  return { ...option, series, tooltip, animation: !reducedMotion };
}

/** Account-only controlled pie selection. Merge updates retain selectedMap
 * even when data.selected is false; selection actions are authoritative. */
export function synchronizePieSelection(chart: Pick<echarts.ECharts, "dispatchAction">, option: EChartsOption, selectedName: string | null): void {
  const series = Array.isArray(option.series) ? option.series : option.series ? [option.series] : [];
  series.forEach((item, seriesIndex) => {
    if (item.type !== "pie") return;
    const dataIndex = Array.from({ length: item.data?.length ?? 0 }, (_, index) => index);
    chart.dispatchAction({ type: "unselect", seriesIndex, dataIndex });
    if (selectedName) chart.dispatchAction({ type: "select", seriesIndex, name: selectedName });
  });
}

export interface EChartProps {
  option: EChartsOption;
  className?: string;
  style?: React.CSSProperties;
  height?: number;
  ariaLabel: string;
  summary?: string;
  dataTableLabel?: string;
  dataTableColumns?: string[];
  dataTableRows?: string[][];
  onSelectName?: (name: string | null) => void;
  onHoverName?: (name: string | null) => void;
  center?: React.ReactNode;
  /** Opt-in account legend highlighting; undefined preserves existing behavior. */
  highlightedName?: string | null;
  /** Undefined preserves native selection for other chart consumers. */
  selectedName?: string | null;
}

/**
 * EChart is the shared React wrapper used by every Nestworth chart so chart
 * instance lifecycle handling lives in one place.
 */
export function EChart({
  option,
  className,
  style,
  height = 320,
  ariaLabel,
  summary,
  dataTableLabel,
  dataTableColumns,
  dataTableRows,
  onSelectName,
  onHoverName,
  center,
  highlightedName,
  selectedName,
}: EChartProps) {
  const containerRef = React.useRef<HTMLDivElement | null>(null);
  const chartRef = React.useRef<echarts.ECharts | null>(null);
  const summaryId = React.useId();
  const onSelectNameRef = React.useRef(onSelectName);
  const onHoverNameRef = React.useRef(onHoverName);
  React.useEffect(() => {
    onSelectNameRef.current = onSelectName;
  }, [onSelectName]);
  React.useEffect(() => {
    onHoverNameRef.current = onHoverName;
  }, [onHoverName]);

  React.useEffect(() => {
    if (!containerRef.current) {
      return;
    }
    let chart: echarts.ECharts | null = null;
    try {
      chart = echarts.init(containerRef.current);
    } catch {
      return;
    }
    chartRef.current = chart;
    const onClick = (params: { name?: string }) => {
      onSelectNameRef.current?.(params.name ?? null);
    };
    const onMouseOver = (params: { name?: string }) => {
      onHoverNameRef.current?.(params.name ?? null);
    };
    const onMouseOut = () => {
      onHoverNameRef.current?.(null);
    };
    chart.on("click", onClick);
    chart.on("mouseover", onMouseOver);
    chart.on("mouseout", onMouseOut);
    chart.on("globalout", onMouseOut);

    const resizeObserver = new ResizeObserver(() => {
      try {
        chart?.resize();
      } catch {
        // jsdom has no layout; ignore resize failures.
      }
    });
    resizeObserver.observe(containerRef.current);

    return () => {
      resizeObserver.disconnect();
      try {
        chart?.off("click", onClick);
        chart?.off("mouseover", onMouseOver);
        chart?.off("mouseout", onMouseOut);
        chart?.off("globalout", onMouseOut);
        chart?.dispose();
      } catch {
        // jsdom canvas disposal is best-effort.
      }
      chartRef.current = null;
    };
  }, []);

  React.useEffect(() => {
    const reducedMotion = prefersReducedMotion();
    try {
      // The shared accessibility policy must be the final value so an
      // individual chart cannot accidentally re-enable animation. Dynamic
      // labels are rendered as richText so they cannot inject HTML.
      chartRef.current?.setOption(
        applyChartPolicy(option, reducedMotion),
        { notMerge: false, lazyUpdate: true },
      );
    } catch {
      // Drawing is optional; the data table remains the accessible source.
    }
  }, [option]);

  React.useEffect(() => {
    if (selectedName === undefined || !chartRef.current) return;
    try { synchronizePieSelection(chartRef.current, option, selectedName); }
    catch { /* The source table remains available when drawing is unavailable. */ }
  }, [selectedName, option]);

  React.useEffect(() => {
    if (highlightedName === undefined) return;
    try {
      chartRef.current?.dispatchAction({ type: "downplay", seriesIndex: 0 });
      if (highlightedName) {
        chartRef.current?.dispatchAction({ type: "highlight", seriesIndex: 0, name: highlightedName });
      }
    } catch {
      // The original rows and data table remain usable without canvas drawing.
    }
  }, [highlightedName, option]);

  return (
    <figure className="flex min-w-0 w-full max-w-full flex-col gap-3">
      <div className="relative min-w-0 w-full max-w-full">
        <div
          ref={containerRef}
          className={className}
          style={{ width: "100%", maxWidth: "100%", height, ...style }}
          role="img"
          aria-label={ariaLabel}
          aria-describedby={summary ? summaryId : undefined}
        />
        {center && (
          <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center text-center">
            {center}
          </div>
        )}
      </div>
      {summary && <figcaption id={summaryId} className="sr-only">{summary}</figcaption>}
      {dataTableLabel && dataTableColumns && dataTableRows && (
        <details className="relative z-10">
          <summary className="cursor-pointer rounded-lg text-sm text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50">
            {dataTableLabel}
          </summary>
          <div className="mt-2 overflow-x-auto">
            <table className="num w-full text-left text-sm">
              <thead className="sticky top-0 bg-card">
                <tr className="border-b border-border text-muted-foreground">
                  {dataTableColumns.map((column) => (
                    <th key={column} className="px-2 py-1.5 font-medium">{column}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {dataTableRows.map((row, index) => (
                  <tr key={`${row.join("-")}-${index}`} className="border-b border-border transition-colors last:border-0 hover:bg-muted/40">
                    {row.map((cell, cellIndex) => (
                      <td key={`${cell}-${cellIndex}`} className="px-2 py-1.5">{cell}</td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </details>
      )}
    </figure>
  );
}

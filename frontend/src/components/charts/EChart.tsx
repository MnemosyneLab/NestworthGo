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
  return { ...option, tooltip, animation: !reducedMotion };
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
  center?: React.ReactNode;
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
  center,
}: EChartProps) {
  const containerRef = React.useRef<HTMLDivElement | null>(null);
  const chartRef = React.useRef<echarts.ECharts | null>(null);
  const summaryId = React.useId();
  const onSelectNameRef = React.useRef(onSelectName);
  React.useEffect(() => {
    onSelectNameRef.current = onSelectName;
  }, [onSelectName]);

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
    chart.on("click", onClick);

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
          <summary className="cursor-pointer text-sm text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50">
            {dataTableLabel}
          </summary>
          <div className="mt-2 overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-border">
                  {dataTableColumns.map((column) => (
                    <th key={column} className="px-2 py-1 font-medium">{column}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {dataTableRows.map((row, index) => (
                  <tr key={`${row.join("-")}-${index}`} className="border-b border-border last:border-0">
                    {row.map((cell, cellIndex) => (
                      <td key={`${cell}-${cellIndex}`} className="px-2 py-1">{cell}</td>
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

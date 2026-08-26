import * as React from "react";
import * as echarts from "echarts/core";
import { LineChart, BarChart, PieChart } from "echarts/charts";
import { GridComponent, TooltipComponent, LegendComponent, DataZoomComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import type { ComposeOption } from "echarts/core";
import type { LineSeriesOption, BarSeriesOption, PieSeriesOption } from "echarts/charts";
import type { GridComponentOption, TooltipComponentOption, LegendComponentOption, DataZoomComponentOption } from "echarts/components";

// Only the chart/component types Nestworth actually uses are registered. The
// tree-shaken subset keeps the bundle focused on Net Worth Trend,
// Asset/Liability, and Investment Gain.
echarts.use([LineChart, BarChart, PieChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent, CanvasRenderer]);

export type EChartsOption = ComposeOption<
  LineSeriesOption | BarSeriesOption | PieSeriesOption | GridComponentOption | TooltipComponentOption | LegendComponentOption | DataZoomComponentOption
>;

export interface EChartProps {
  option: EChartsOption;
  className?: string;
  style?: React.CSSProperties;
  ariaLabel: string;
  summary?: string;
  dataTableLabel?: string;
  dataTableColumns?: [string, string];
  dataTableRows?: Array<[string, string]>;
}

/**
 * EChart is the shared React wrapper used by every Nestworth chart (Net Worth Trend, Asset/Liability,
 * Investment Gain, ...) so chart instance lifecycle handling (init,
 * resize, dispose, option updates) lives in exactly one place.
 */
export function EChart({ option, className, style, ariaLabel, summary, dataTableLabel, dataTableColumns, dataTableRows }: EChartProps) {
  const containerRef = React.useRef<HTMLDivElement | null>(null);
  const chartRef = React.useRef<echarts.ECharts | null>(null);
  const summaryId = React.useId();

  React.useEffect(() => {
    if (!containerRef.current) {
      return;
    }
    const chart = echarts.init(containerRef.current);
    chartRef.current = chart;

    const resizeObserver = new ResizeObserver(() => chart.resize());
    resizeObserver.observe(containerRef.current);

    return () => {
      resizeObserver.disconnect();
      chart.dispose();
      chartRef.current = null;
    };
    // Deliberately re-created only on mount/unmount; option updates below
    // reuse the same instance via setOption instead of full re-init.
  }, []);

  React.useEffect(() => {
    chartRef.current?.setOption(option, true);
  }, [option]);

  return (
    <figure className="flex h-full flex-col gap-3">
      <div
        ref={containerRef}
        className={className}
        style={{ width: "100%", height: "100%", ...style }}
        role="img"
        aria-label={ariaLabel}
        aria-describedby={summary ? summaryId : undefined}
      />
      {summary && <figcaption id={summaryId} className="sr-only">{summary}</figcaption>}
      {dataTableLabel && dataTableColumns && dataTableRows && (
        <details>
          <summary className="cursor-pointer text-sm text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50">
            {dataTableLabel}
          </summary>
          <div className="mt-2 overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-border">
                  <th className="px-2 py-1 font-medium">{dataTableColumns[0]}</th>
                  <th className="px-2 py-1 font-medium">{dataTableColumns[1]}</th>
                </tr>
              </thead>
              <tbody>
                {dataTableRows.map(([label, value]) => (
                  <tr key={`${label}-${value}`} className="border-b border-border last:border-0">
                    <td className="px-2 py-1">{label}</td>
                    <td className="px-2 py-1">{value}</td>
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

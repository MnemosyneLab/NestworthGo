import * as React from "react";
import * as echarts from "echarts/core";
import { LineChart, BarChart, PieChart } from "echarts/charts";
import { GridComponent, TooltipComponent, LegendComponent, DataZoomComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import type { ComposeOption } from "echarts/core";
import type { LineSeriesOption, BarSeriesOption, PieSeriesOption } from "echarts/charts";
import type { GridComponentOption, TooltipComponentOption, LegendComponentOption, DataZoomComponentOption } from "echarts/components";

// Only the chart/component types Nestworth actually uses are registered,
// per the frontend stack decision Sec7's "轻量 React wrapper" goal: the
// full `echarts` package pulls in every chart type and is multiple
// megabytes; this tree-shaken subset covers every chart this application
// currently needs (Net Worth Trend, Asset/Liability, Investment Gain).
echarts.use([LineChart, BarChart, PieChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent, CanvasRenderer]);

export type EChartsOption = ComposeOption<
  LineSeriesOption | BarSeriesOption | PieSeriesOption | GridComponentOption | TooltipComponentOption | LegendComponentOption | DataZoomComponentOption
>;

export interface EChartProps {
  option: EChartsOption;
  className?: string;
  style?: React.CSSProperties;
}

/**
 * EChart is the lightweight React wrapper the frontend stack decision
 * Sec7 calls for ("建议自己封装一个轻量 React wrapper: <EChart option={option} />"),
 * used by every Nestworth chart (Net Worth Trend, Asset/Liability,
 * Investment Gain, ...) so chart instance lifecycle handling (init,
 * resize, dispose, option updates) lives in exactly one place.
 */
export function EChart({ option, className, style }: EChartProps) {
  const containerRef = React.useRef<HTMLDivElement | null>(null);
  const chartRef = React.useRef<echarts.ECharts | null>(null);

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

  return <div ref={containerRef} className={className} style={{ width: "100%", height: "100%", ...style }} />;
}

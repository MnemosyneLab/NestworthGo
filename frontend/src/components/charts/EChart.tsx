import * as React from "react";
import * as echarts from "echarts";
import type { EChartsOption } from "echarts";

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

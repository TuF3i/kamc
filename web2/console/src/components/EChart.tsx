import { createEffect, onCleanup, onMount } from 'solid-js';
import * as echarts from 'echarts/core';
import { BarChart, LineChart, PieChart } from 'echarts/charts';
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';
import type { EChartsCoreOption } from 'echarts/core';

echarts.use([
  LineChart,
  BarChart,
  PieChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  CanvasRenderer,
]);

export interface EChartProps {
  option: EChartsCoreOption;
  class?: string;
}

/** ECharts 6/Solid 薄封装：随 option 变化重组，容器尺寸变化自动 resize。 */
export function EChart(props: EChartProps) {
  let host: HTMLDivElement | undefined;
  let chart: echarts.ECharts | undefined;
  let observer: ResizeObserver | undefined;

  onMount(() => {
    if (!host) return;
    chart = echarts.init(host, undefined, { renderer: 'canvas' });
    chart.setOption(props.option, true);
    observer = new ResizeObserver(() => chart?.resize());
    observer.observe(host);
    onCleanup(() => {
      observer?.disconnect();
      chart?.dispose();
      chart = undefined;
    });
  });

  createEffect(() => {
    const option = props.option;
    chart?.setOption(option, true);
  });

  return <div ref={host} class={props.class} style={{ width: '100%', height: '100%' }} />;
}

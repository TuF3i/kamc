import type { EChartsCoreOption } from 'echarts/core';

export interface ChartPalette {
  ops: string;
  blocked: string;
  logins: string;
  grid: string;
  axis: string;
  layer: string;
  tooltipBg: string;
  tooltipBorder: string;
  tooltipText: string;
  opsFillTop: string;
  opsFillBottom: string;
  blockedFillTop: string;
  blockedFillBottom: string;
  palette: string[];
}

export function chartPalette(dark: boolean): ChartPalette {
  if (dark) {
    return {
      ops: '#6ea8ff',
      blocked: '#e06c75',
      logins: '#5fb99a',
      grid: 'rgba(232, 235, 240, 0.09)',
      axis: '#93a3b8',
      layer: '#161c25',
      tooltipBg: '#1b222c',
      tooltipBorder: 'rgba(232, 235, 240, 0.14)',
      tooltipText: '#e8ebf0',
      opsFillTop: 'rgba(110, 168, 255, 0.35)',
      opsFillBottom: 'rgba(110, 168, 255, 0)',
      blockedFillTop: 'rgba(224, 108, 117, 0.3)',
      blockedFillBottom: 'rgba(224, 108, 117, 0)',
      palette: [
        '#6ea8ff',
        '#5fb99a',
        '#d9ad63',
        '#a78bfa',
        '#e06c75',
        '#4fb8c9',
        '#d98bb0',
        '#94a3b8',
      ],
    };
  }
  return {
    ops: '#3b6fb6',
    blocked: '#c0504d',
    logins: '#2f8f6f',
    grid: 'rgba(40, 50, 65, 0.08)',
    axis: '#5d6b7e',
    layer: '#f3f5f8',
    tooltipBg: '#ffffff',
    tooltipBorder: 'rgba(40, 50, 65, 0.14)',
    tooltipText: '#28323f',
    opsFillTop: 'rgba(59, 111, 182, 0.24)',
    opsFillBottom: 'rgba(59, 111, 182, 0)',
    blockedFillTop: 'rgba(192, 80, 77, 0.2)',
    blockedFillBottom: 'rgba(192, 80, 77, 0)',
    palette: [
      '#3b6fb6',
      '#2f8f6f',
      '#b8863b',
      '#7b61c9',
      '#c0504d',
      '#2f8b9b',
      '#b0668c',
      '#6b7789',
    ],
  };
}

function tooltipStyle(p: ChartPalette) {
  return {
    backgroundColor: p.tooltipBg,
    borderColor: p.tooltipBorder,
    borderWidth: 1,
    textStyle: {
      color: p.tooltipText,
      fontSize: 12,
      fontFamily: 'Reverier Mono, monospace',
    },
    extraCssText: 'border-radius:8px;box-shadow:0 18px 40px -28px rgba(0,0,0,.6);',
  };
}

function axisStyle(p: ChartPalette) {
  return {
    axisLabel: { color: p.axis, fontSize: 11, fontFamily: 'Reverier Mono, monospace' },
    axisLine: { lineStyle: { color: p.grid } },
    axisTick: { show: false },
    splitLine: { lineStyle: { color: p.grid, type: 'dashed' } },
  };
}

export function trendOption(
  data: { date: string; logins: number; ops: number; blocked: number }[],
  p: ChartPalette,
): EChartsCoreOption {
  return {
    backgroundColor: 'transparent',
    color: [p.ops, p.blocked, p.logins],
    grid: { left: 6, right: 8, top: 30, bottom: 4, containLabel: true },
    tooltip: { trigger: 'axis', ...tooltipStyle(p) },
    legend: {
      top: 0,
      left: 0,
      icon: 'roundRect',
      itemWidth: 10,
      itemHeight: 6,
      itemGap: 18,
      textStyle: { color: p.axis, fontSize: 11, fontFamily: 'Reverier Mono, monospace' },
    },
    xAxis: {
      type: 'category',
      data: data.map((point) => point.date),
      ...axisStyle(p),
      splitLine: { show: false },
    },
    yAxis: [
      {
        type: 'value',
        minInterval: 1,
        ...axisStyle(p),
      },
      {
        type: 'value',
        minInterval: 1,
        ...axisStyle(p),
        splitLine: { show: false },
      },
    ],
    series: [
      {
        name: 'K8s 操作',
        type: 'line',
        smooth: true,
        symbol: 'none',
        data: data.map((point) => point.ops),
        lineStyle: { width: 2, color: p.ops },
        itemStyle: { color: p.ops },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: p.opsFillTop },
              { offset: 1, color: p.opsFillBottom },
            ],
          },
        },
      },
      {
        name: '操作拦截',
        type: 'line',
        smooth: true,
        symbol: 'none',
        data: data.map((point) => point.blocked),
        lineStyle: { width: 2, color: p.blocked },
        itemStyle: { color: p.blocked },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: p.blockedFillTop },
              { offset: 1, color: p.blockedFillBottom },
            ],
          },
        },
      },
      {
        name: '登录次数',
        type: 'line',
        yAxisIndex: 1,
        smooth: true,
        showSymbol: false,
        data: data.map((point) => point.logins),
        lineStyle: { width: 2, color: p.logins },
        itemStyle: { color: p.logins },
      },
    ],
  };
}

export function donutOption(
  data: { name: string; value: number }[],
  p: ChartPalette,
): EChartsCoreOption {
  return {
    backgroundColor: 'transparent',
    color: p.palette,
    tooltip: {
      trigger: 'item',
      formatter: '{b}: {c}',
      ...tooltipStyle(p),
    },
    series: [
      {
        type: 'pie',
        radius: ['58%', '88%'],
        center: ['50%', '50%'],
        avoidLabelOverlap: false,
        padAngle: 2,
        label: { show: false },
        labelLine: { show: false },
        itemStyle: { borderColor: p.layer, borderWidth: 2 },
        data,
      },
    ],
  };
}

export function resourceBarOption(
  data: { name: string; allowed: number; denied: number }[],
  p: ChartPalette,
): EChartsCoreOption {
  return {
    backgroundColor: 'transparent',
    color: [p.ops, p.blocked],
    grid: { left: 4, right: 12, top: 30, bottom: 4, containLabel: true },
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      ...tooltipStyle(p),
    },
    legend: {
      top: 0,
      left: 0,
      icon: 'roundRect',
      itemWidth: 10,
      itemHeight: 6,
      itemGap: 18,
      textStyle: { color: p.axis, fontSize: 11, fontFamily: 'Reverier Mono, monospace' },
    },
    xAxis: {
      type: 'value',
      minInterval: 1,
      ...axisStyle(p),
    },
    yAxis: {
      type: 'category',
      data: data.map((item) => item.name),
      axisLabel: { color: p.axis, fontSize: 11, fontFamily: 'Reverier Mono, monospace' },
      axisLine: { show: false },
      axisTick: { show: false },
    },
    series: [
      {
        name: '放行',
        type: 'bar',
        stack: 'resource',
        data: data.map((item) => item.allowed),
        itemStyle: { color: p.ops, borderRadius: [0, 4, 4, 0] },
      },
      {
        name: '拦截',
        type: 'bar',
        stack: 'resource',
        data: data.map((item) => item.denied),
        itemStyle: { color: p.blocked, borderRadius: [0, 4, 4, 0] },
      },
    ],
  };
}

import { useEffect, useRef } from 'react';
import { usePreferences } from '@vben/preferences';
import * as echarts from 'echarts/core';
import { BarChart, LineChart, PieChart, RadarChart } from 'echarts/charts';
import {
  GridComponent,
  LegendComponent,
  RadarComponent,
  TitleComponent,
  TooltipComponent,
} from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';
import type { EChartsOption } from 'echarts';
echarts.use([
  BarChart,
  LineChart,
  PieChart,
  RadarChart,
  GridComponent,
  LegendComponent,
  RadarComponent,
  TitleComponent,
  TooltipComponent,
  CanvasRenderer,
]);
export function EChart({
  option,
  height = 300,
  label,
}: {
  option: EChartsOption;
  height?: number;
  label: string;
}) {
  const element = useRef<HTMLDivElement>(null);
  const { isDark } = usePreferences();
  useEffect(() => {
    if (!element.current) return;
    const chart = echarts.init(element.current, isDark ? 'dark' : undefined);
    chart.setOption({ ...option, backgroundColor: 'transparent' });
    const observer = new ResizeObserver(() => {
      if (element.current?.clientWidth) chart.resize();
    });
    observer.observe(element.current);
    return () => {
      observer.disconnect();
      chart.dispose();
    };
  }, [option, isDark]);
  return (
    <div
      ref={element}
      role="img"
      aria-label={label}
      style={{ height, width: '100%', minWidth: 0 }}
    />
  );
}

import { useState } from 'react';
import type { ReactNode } from 'react';
import { cn } from '@walnut-core/shared/utils';
import { CountTo } from '@/components/count-to';
import { EChart } from '@/components/chart/EChart';
import {
  analytics_trends,
  analytics_visits,
  analytics_visits_data,
  analytics_visits_source,
  analytics_visits_sales,
} from './options';
// 彩色 SVG 与 Vue 基线图标库的 svg:card/cake/download/bell 同源同文件
// （离线图标包原件拷贝），内联渲染保证配色/描边与 Vue 逐像素一致。
import bellSvg from './icons/bell.svg?raw';
import cakeSvg from './icons/cake.svg?raw';
import cardSvg from './icons/card.svg?raw';
import downloadSvg from './icons/download.svg?raw';

/**
 * 分析页（对应 Vue 侧 views/dashboard/analytics/index.vue）。
 *
 * 外壳逐条对齐 Vue 基线 common-ui 的三个 analysis 组件（shadcn 卡片体系）：
 * - AnalysisOverview：Card rounded-xl border 无阴影；CardHeader p-5 + CardTitle
 *   text-xl；CardContent/CardFooter 均 p-6 pt-0（**无分隔线**）；数值走
 *   CountToAnimator（startVal=1、1500ms 线性滚动）；图标 size-8 彩色 SVG。
 * - AnalysisChartsTabs：card-box（bg-card rounded-xl border）px-4 pt-3 pb-5；
 *   shadcn TabsList（bg-muted h-9 rounded-md p-1）+ TabsTrigger（active 时
 *   bg-background shadow-sm），**不是** antd Tabs 的下划线样式；内容 mt-2 pt-4。
 * - AnalysisChartCard：同款 shadcn 卡片 + text-xl 标题；底部三卡 md:flex 各占
 *   1/3、前两卡 md:mr-4（Vue 原布局）。
 * 文案与 Vue 同为硬编码中文（Vue 原行为，对等保留）。
 */
const items = [
  { title: '用户量', totalTitle: '总用户量', totalValue: 120_000, value: 2000, icon: cardSvg },
  { title: '访问量', totalTitle: '总访问量', totalValue: 500_000, value: 20_000, icon: cakeSvg },
  { title: '下载量', totalTitle: '总下载量', totalValue: 120_000, value: 8000, icon: downloadSvg },
  { title: '使用量', totalTitle: '总使用量', totalValue: 50_000, value: 5000, icon: bellSvg },
];
const chartTabs = [
  { label: '流量趋势', value: 'trends' },
  { label: '月访问量', value: 'visits' },
];
const bottomCards = [
  { title: '访问数量', option: analytics_visits_data },
  { title: '访问来源', option: analytics_visits_source },
  { title: '访问来源', option: analytics_visits_sales },
];

function ChartCard({
  children,
  className,
  title,
}: {
  children: ReactNode;
  className?: string;
  title: string;
}) {
  return (
    <div className={cn('border-border bg-card text-card-foreground rounded-xl border', className)}>
      <div className="flex flex-col gap-y-1.5 p-5">
        <h3 className="text-xl leading-none font-semibold tracking-tight">{title}</h3>
      </div>
      <div className="p-6 pt-0">{children}</div>
    </div>
  );
}

export default function AnalyticsPage() {
  const [tab, setTab] = useState('trends');
  return (
    <div className="p-5">
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        {items.map(({ title, totalTitle, totalValue, value, icon }) => (
          <div
            key={title}
            className="border-border bg-card text-card-foreground w-full rounded-xl border"
          >
            <div className="flex flex-col gap-y-1.5 p-5">
              <h3 className="text-xl leading-none font-semibold tracking-tight">{title}</h3>
            </div>
            <div className="flex items-center justify-between p-6 pt-0">
              <CountTo className="text-xl" endVal={value} startVal={1} />
              <span
                className="size-8 shrink-0 [&>svg]:size-full"
                dangerouslySetInnerHTML={{ __html: icon }}
              />
            </div>
            <div className="flex items-center justify-between p-6 pt-0">
              <span>{totalTitle}</span>
              <CountTo endVal={totalValue} startVal={1} />
            </div>
          </div>
        ))}
      </div>
      <div className="bg-card text-card-foreground border-border mt-5 w-full rounded-xl border px-4 pt-3 pb-5">
        <div
          className="bg-muted text-muted-foreground inline-flex h-9 items-center justify-center rounded-md p-1"
          role="tablist"
        >
          {chartTabs.map((t) => (
            <button
              key={t.value}
              type="button"
              role="tab"
              aria-selected={tab === t.value}
              className={cn(
                'inline-flex items-center justify-center rounded-md px-3 py-1 text-sm font-medium whitespace-nowrap transition-all',
                tab === t.value && 'bg-background text-foreground shadow-sm',
              )}
              onClick={() => setTab(t.value)}
            >
              {t.label}
            </button>
          ))}
        </div>
        <div className="mt-2 pt-4">
          <EChart
            label={tab === 'trends' ? '流量趋势' : '月访问量'}
            option={tab === 'trends' ? analytics_trends : analytics_visits}
          />
        </div>
      </div>
      <div className="mt-5 w-full md:flex">
        {bottomCards.map(({ title, option }, i) => (
          <ChartCard
            key={i}
            className={cn('mt-5 md:mt-0 md:w-1/3', i < 2 && 'md:mr-4')}
            title={title}
          >
            <EChart label={title} option={option} />
          </ChartCard>
        ))}
      </div>
    </div>
  );
}
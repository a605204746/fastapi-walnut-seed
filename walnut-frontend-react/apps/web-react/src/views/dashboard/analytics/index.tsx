import { useState } from 'react';
import { Card, Tabs } from 'antd';
import { Bell, CreditCard, Download, Users } from 'lucide-react';
import { EChart } from '@/components/chart/EChart';
import {
  analytics_trends,
  analytics_visits,
  analytics_visits_data,
  analytics_visits_source,
  analytics_visits_sales,
} from './options';
const items = [
  { title: '用户量', totalTitle: '总用户量', totalValue: 120000, value: 2000, Icon: CreditCard },
  { title: '访问量', totalTitle: '总访问量', totalValue: 500000, value: 20000, Icon: Users },
  { title: '下载量', totalTitle: '总下载量', totalValue: 120000, value: 8000, Icon: Download },
  { title: '使用量', totalTitle: '总使用量', totalValue: 50000, value: 5000, Icon: Bell },
];
export default function AnalyticsPage() {
  const [tab, setTab] = useState('trends');
  return (
    <div className="p-5 dashboard-page">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {items.map(({ title, totalTitle, totalValue, value, Icon }) => (
          <Card
            key={title}
            title={title}
            extra={<span className="rounded bg-accent px-2 text-xs">月</span>}
          >
            <div className="flex items-center justify-between pb-4">
              <span className="text-3xl font-semibold">{value.toLocaleString()}</span>
              <Icon className="size-10 text-primary" />
            </div>
            <div className="flex justify-between border-t pt-3">
              <span>{totalTitle}</span>
              <span>{totalValue.toLocaleString()}</span>
            </div>
          </Card>
        ))}
      </div>
      <Card className="mt-5">
        <Tabs
          activeKey={tab}
          onChange={setTab}
          items={[
            { key: 'trends', label: '流量趋势' },
            { key: 'visits', label: '月访问量' },
          ]}
        />
        <EChart
          label={tab === 'trends' ? '流量趋势' : '月访问量'}
          option={tab === 'trends' ? analytics_trends : analytics_visits}
        />
      </Card>
      <div className="mt-5 grid grid-cols-1 gap-4 md:grid-cols-3">
        {[
          ['访问数量', analytics_visits_data],
          ['访问来源', analytics_visits_source],
          ['访问来源', analytics_visits_sales],
        ].map(([title, option], i) => (
          <Card key={i} title={title as string}>
            <EChart label={title as string} option={option as typeof analytics_trends} />
          </Card>
        ))}
      </div>
    </div>
  );
}

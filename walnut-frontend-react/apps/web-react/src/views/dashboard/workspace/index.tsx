import { useState } from 'react';
import { Avatar, Card, Checkbox, Timeline } from 'antd';
import { useNavigate } from 'react-router';
import { useUserStore } from '@vben/stores';
import { usePreferences } from '@vben/preferences';
import { VbenIcon } from '@vben-core/shadcn-ui';
import { EChart } from '@/components/chart/EChart';
import { analytics_visits_source } from '../analytics/options';
import { projectItems, quickNavItems, todoItems, trendItems } from './data';
export default function WorkspacePage() {
  const user = useUserStore((s) => s.userInfo);
  const { preferences } = usePreferences();
  const [todos, setTodos] = useState(todoItems);
  const navigate = useNavigate();
  function nav(url: string) {
    if (/^https?:/.test(url)) window.open(url, '_blank', 'noopener,noreferrer');
    else navigate(url);
  }
  return (
    <div className="p-5 dashboard-page">
      <Card>
        <div className="flex items-center gap-5">
          <Avatar size={72} src={user?.avatar || preferences.app.defaultAvatar} />
          <div>
            <h1 className="text-xl font-semibold">早安，{user?.realName}，开始您一天的工作吧！</h1>
            <p className="mt-2 text-muted-foreground">今日晴，20℃ - 32℃！</p>
          </div>
          <div className="ml-auto hidden gap-8 text-center md:flex">
            {[
              ['待办', '2/10'],
              ['项目', '8'],
              ['团队', '300'],
            ].map(([title, value]) => (
              <div key={title}>
                <p>{title}</p>
                <strong className="text-2xl">{value}</strong>
              </div>
            ))}
          </div>
        </div>
      </Card>
      <div className="mt-5 grid grid-cols-1 gap-5 xl:grid-cols-3">
        <div className="space-y-5 xl:col-span-2">
          <Card title="项目" styles={{ body: { padding: 0 } }}>
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
              {projectItems.map((item) => (
                <button
                  key={item.title}
                  className="border-b border-r p-5 text-left transition-colors hover:bg-accent"
                  onClick={() => nav(item.url)}
                >
                  <div className="flex items-center gap-3">
                    <VbenIcon icon={item.icon} className="size-7" style={{ color: item.color }} />
                    <strong>{item.title}</strong>
                  </div>
                  <p className="my-3 text-muted-foreground">{item.content}</p>
                  <div className="flex justify-between text-xs text-muted-foreground">
                    <span>{item.group}</span>
                    <span>{item.date}</span>
                  </div>
                </button>
              ))}
            </div>
          </Card>
          <Card title="最新动态">
            <Timeline
              items={trendItems.map((item) => ({
                content: (
                  <div>
                    <strong>{item.title}</strong>
                    <p>{item.content.replace(/<[^>]+>/g, '')}</p>
                    <small className="text-muted-foreground">{item.date}</small>
                  </div>
                ),
              }))}
            />
          </Card>
        </div>
        <div className="space-y-5">
          <Card title="快捷导航">
            <div className="grid grid-cols-3 gap-5">
              {quickNavItems.map((item) => (
                <button
                  key={item.title}
                  className="flex flex-col items-center gap-2 rounded p-2 hover:bg-accent"
                  onClick={() => nav(item.url)}
                >
                  <VbenIcon icon={item.icon} className="size-6" style={{ color: item.color }} />
                  <span>{item.title}</span>
                </button>
              ))}
            </div>
          </Card>
          <Card title="待办事项">
            {todos.map((item, i) => (
              <div className="border-b py-3" key={item.title}>
                <Checkbox
                  checked={item.completed}
                  onChange={(e) =>
                    setTodos(
                      todos.map((todo, index) =>
                        index === i ? { ...todo, completed: e.target.checked } : todo,
                      ),
                    )
                  }
                >
                  <span className={item.completed ? 'line-through text-muted-foreground' : ''}>
                    {item.title}
                  </span>
                </Checkbox>
                <p className="mt-2 text-xs text-muted-foreground">{item.content}</p>
              </div>
            ))}
          </Card>
          <Card title="访问来源">
            <EChart label="工作台访问来源" option={analytics_visits_source} />
          </Card>
        </div>
      </div>
    </div>
  );
}

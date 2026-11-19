import { Card, Descriptions, Tag } from 'antd';
import { usePreferences } from '@vben/preferences';
export default function AboutPage() {
  const { preferences } = usePreferences();
  return (
    <div className="p-5">
      <Card title="关于项目">
        <p className="mb-4">
          {preferences.app.name} 是基于 FastAPI、React、TypeScript 与 Ant Design
          的开源后台管理模板。
        </p>
        <Descriptions
          bordered
          column={1}
          items={[
            { key: 'framework', label: '前端框架', children: <Tag color="blue">React 19</Tag> },
            {
              key: 'repo',
              label: '项目地址',
              children: (
                <a
                  href="https://github.com/a605204746/fastapi-walnut-seed"
                  target="_blank"
                  rel="noreferrer"
                >
                  fastapi-walnut-seed
                </a>
              ),
            },
            { key: 'license', label: '开源许可', children: 'MIT' },
          ]}
        />
      </Card>
    </div>
  );
}

import { useMemo, useState } from 'react';
import { Button, Input, Space } from 'antd';
import { getMessageApi } from '@/services/message';
function JsonNode({
  value,
  name,
  depth,
  expand,
  filter,
}: {
  value: unknown;
  name?: string;
  depth: number;
  expand: boolean;
  filter: string;
}) {
  const object = value !== null && typeof value === 'object';
  const entries = object ? Object.entries(value) : [];
  if (
    filter &&
    !JSON.stringify({ [name ?? 'root']: value })
      .toLowerCase()
      .includes(filter)
  )
    return null;
  const label = name === undefined ? '' : `${JSON.stringify(name)}: `;
  if (!object)
    return (
      <div className="whitespace-pre-wrap break-all">
        <span className="text-primary">{label}</span>
        <span
          className={
            typeof value === 'string'
              ? 'text-green-600 dark:text-green-400'
              : 'text-orange-600 dark:text-orange-400'
          }
        >
          {JSON.stringify(value)}
        </span>
      </div>
    );
  return (
    <details open={expand || !!filter} className="ml-3">
      <summary className="cursor-pointer select-none">
        <span className="text-primary">{label}</span>
        {Array.isArray(value) ? '[' : '{'}
        <span className="ml-2 text-muted-foreground">{entries.length} 项</span>
      </summary>
      <div className="ml-3 border-l border-border pl-3">
        {entries.map(([key, item]) => (
          <JsonNode
            key={key}
            name={key}
            value={item}
            depth={depth + 1}
            expand={expand}
            filter={filter}
          />
        ))}
      </div>
      {Array.isArray(value) ? ']' : '}'}
    </details>
  );
}
export function JsonPreview({ value }: { value: unknown }) {
  const [filter, setFilter] = useState('');
  const [expand, setExpand] = useState(true);
  const text = useMemo(() => JSON.stringify(value, null, 2), [value]);
  return (
    <div className="rounded bg-accent p-3 text-xs">
      <Space className="mb-3" wrap>
        <Input.Search
          aria-label="搜索 JSON"
          size="small"
          placeholder="搜索 JSON"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <Button size="small" onClick={() => setExpand(!expand)}>
          {expand ? '折叠全部' : '展开全部'}
        </Button>
        <Button
          size="small"
          onClick={async () => {
            await navigator.clipboard.writeText(text);
            getMessageApi()?.message.success('已复制');
          }}
        >
          复制
        </Button>
      </Space>
      <div className="max-h-96 overflow-auto font-mono">
        <JsonNode value={value} depth={0} expand={expand} filter={filter.toLowerCase()} />
      </div>
    </div>
  );
}

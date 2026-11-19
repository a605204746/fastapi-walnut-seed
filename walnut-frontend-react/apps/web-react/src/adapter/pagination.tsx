import { Button, Select } from 'antd';
import {
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  SkipBack,
  SkipForward,
} from 'lucide-react';
export function GridPagination({
  current,
  pageSize,
  total,
  onChange,
}: {
  current: number;
  pageSize: number;
  total: number;
  onChange: (page: number, size: number) => void;
}) {
  const max = Math.max(1, Math.ceil(total / pageSize));
  const start = Math.max(1, Math.min(current - 3, max - 6));
  const pages = Array.from({ length: Math.min(max, 7) }, (_, i) => start + i);
  function button(label: string, page: number, Icon: typeof ChevronLeft, disabled: boolean) {
    return (
      <Button
        aria-label={label}
        type="text"
        size="small"
        disabled={disabled}
        icon={<Icon className="size-3.5" />}
        onClick={() => onChange(page, pageSize)}
      />
    );
  }
  return (
    <div className="vben-grid-pagination flex shrink-0 items-center gap-2">
      <span className="whitespace-nowrap">共 {total} 条记录</span>
      <Select
        aria-label="每页条数"
        className="mr-auto"
        size="small"
        style={{ width: 108, marginRight: 'auto' }}
        value={pageSize}
        options={[10, 20, 30, 50, 100, 200].map((size) => ({ label: `${size}条/页`, value: size }))}
        onChange={(size) => onChange(1, size)}
      />
      <div className="flex items-center gap-1">
        {button('第一页', 1, SkipBack, current <= 1)}
        {button('前五页', Math.max(1, current - 5), ChevronsLeft, current <= 1)}
        {button('上一页', current - 1, ChevronLeft, current <= 1)}
        {pages.map((page) => (
          <Button
            aria-label={`第${page}页`}
            aria-current={page === current ? 'page' : undefined}
            key={page}
            type={page === current ? 'primary' : 'text'}
            size="small"
            onClick={() => onChange(page, pageSize)}
          >
            {page}
          </Button>
        ))}
        {button('下一页', current + 1, ChevronRight, current >= max)}
        {button('后五页', Math.min(max, current + 5), ChevronsRight, current >= max)}
        {button('最后一页', max, SkipForward, current >= max)}
      </div>
    </div>
  );
}

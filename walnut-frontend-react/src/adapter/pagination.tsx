import { Button, Select } from 'antd';
import { $t } from '@/core/locales';
import { usePreferences } from '@/core/preferences';
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
  // 订阅 preferences：locale 切换时重渲染，分页文案随语言重译
  // （对应 Vue 侧 VxeUI.setI18n 随 locale 切换，文案取自 vxe-pc-ui 语言包）
  usePreferences();
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
    <div className="walnut-grid-pagination flex shrink-0 items-center gap-2">
      <span className="whitespace-nowrap">{$t('component.pager.total', [total])}</span>
      <Select
        aria-label={$t('component.pager.pagesize', [pageSize])}
        className="mr-auto"
        size="small"
        style={{ width: 108, marginRight: 'auto' }}
        value={pageSize}
        options={[10, 20, 30, 50, 100, 200].map((size) => ({ label: $t('component.pager.pagesize', [size]), value: size }))}
        onChange={(size) => onChange(1, size)}
      />
      <div className="flex items-center gap-1">
        {button($t('component.pager.homePage'), 1, SkipBack, current <= 1)}
        {button($t('component.pager.prevJump'), Math.max(1, current - 5), ChevronsLeft, current <= 1)}
        {button($t('component.pager.prevPage'), current - 1, ChevronLeft, current <= 1)}
        {pages.map((page) => (
          <Button
            aria-label={`${page} ${$t('component.pager.pageClassifier')}`}
            aria-current={page === current ? 'page' : undefined}
            key={page}
            type={page === current ? 'primary' : 'text'}
            size="small"
            onClick={() => onChange(page, pageSize)}
          >
            {page}
          </Button>
        ))}
        {button($t('component.pager.nextPage'), current + 1, ChevronRight, current >= max)}
        {button($t('component.pager.nextJump'), Math.min(max, current + 5), ChevronsRight, current >= max)}
        {button($t('component.pager.endPage'), max, SkipForward, current >= max)}
      </div>
    </div>
  );
}

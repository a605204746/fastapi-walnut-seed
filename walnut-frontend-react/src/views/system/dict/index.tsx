import { Page } from '@/components/page/Page';

import { DictTypePanel } from './type';

/**
 * 字典管理页（对应 Vue 侧 views/system/dict/index.vue）。
 *
 * 后端菜单 `component: 'system/dict/index'` 指向本文件；本页只是
 * `Page` 容器 + 字典类型面板，字典数据面板由类型表格的行点击弹出。
 *
 * ⚠️ 注意同目录下还有一个 `data.tsx` 占位文件（对应后端菜单
 * `component: 'system/dict/data'`），它与 `data/` 目录并存，
 * 解析时精确匹配 `.tsx` 文件优先，两端行为一致（详见该文件的注释）。
 */
export default function DictPage() {
  return (
    <Page autoContentHeight>
      <DictTypePanel className="h-full overflow-hidden" />
    </Page>
  );
}

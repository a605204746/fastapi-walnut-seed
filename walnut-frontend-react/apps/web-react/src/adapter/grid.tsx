import { ChevronDown, ChevronUp } from 'lucide-react';
import { GridPagination } from './pagination';
import './grid.css';
import type { ComponentType, ReactNode } from 'react';

import type { FormProps, FormSchema } from '@vben-core/form-ui';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { $t } from '@vben/locales';
import { listToTree } from '@vben/utils';
import { useVbenForm } from '@vben-core/form-ui';
import { Store, useStore } from '@vben-core/shared/store';
import { cn } from '@vben-core/shared/utils';
import { Button, Checkbox, Empty, Popover, Table } from 'antd';
import type { TableProps } from 'antd';
import {
  FullscreenOutlined,
  ReloadOutlined,
  SettingOutlined,
} from '@ant-design/icons';

type Key = number | string;

/** 排序项（对应 vxe-table 的 `ProxyAjaxQuerySortCheckedParams` 子集） */
export interface GridSortParam {
  field: string;
  order: 'asc' | 'desc';
}

export interface GridColumn {
  align?: 'center' | 'left' | 'right';
  field?: string;
  fixed?: 'left' | 'right';
  /**
   * 单元格格式化（对应 vxe 的 `formatter`）。
   * 与 `slots.default` 同时存在时 slots 优先。
   */
  formatter?: (params: { cellValue: any; row: any }) => ReactNode;
  /**
   * 最小列宽（对应 vxe 的 minWidth）。
   * ⚠️ 仅作为 schema 层面的记录，**不会**透传给 antd（ColumnType 无此属性），
   * 实际列宽由 `scroll.x = 'max-content'` 的内容自适应决定。
   */
  minWidth?: number | string;
  resizable?: boolean;
  slots?: {
    default?: ((params: { row: any }) => ReactNode) | string;
  };
  /** 可排序（配合 `sortConfig.remote` 走后端排序） */
  sortable?: boolean;
  title?: string;
  /** 树形表格的展开图标挂在这一列（对应 vxe 的 `treeNode: true`） */
  treeNode?: boolean;
  type?: 'checkbox';
  width?: number | string;
}

export interface GridOptions {
  checkboxConfig?: {
    /**
     * 行是否可勾选（对应 vxe 的 `checkboxConfig.checkMethod`）。
     * 返回 false 的行复选框置灰，例如客户端管理页禁止勾选 id === 1 的内置记录。
     */
    checkMethod?: (params: { row: any }) => boolean;
    highlight?: boolean;
    reserve?: boolean;
  };
  columns: GridColumn[];
  /**
   * 表格事件（对应 vxe 的 `gridEvents`）。
   * 只实现业务页真正用到的两个：`cellClick`（字典类型页点行打开数据面板）、
   * `cellDblclick`（部门/菜单页双击展开子节点）。
   */
  gridEvents?: {
    cellClick?: (e: { row: any }) => void;
    cellDblclick?: (e: { row: any }) => void;
  };
  height?: string;
  id?: string;
  /** 表格最小高度（px），弹窗内的表格用来避免空数据时高度塌陷 */
  minHeight?: number;
  pagerConfig?: false | Record<string, any>;
  proxyConfig?: {
    ajax: {
      query: (
        params: {
          page: { currentPage: number; pageSize: number };
          /** 当前排序状态，配合 `addSortParams` 转成后端参数 */
          sorts: GridSortParam[];
        },
        formValues?: Record<string, any>,
      ) => Promise<{ rows: any[]; total?: number }>;
      /** 查询成功后的回调（对应 vxe 的 `proxyConfig.ajax.querySuccess`） */
      querySuccess?: (rows: any[]) => void;
    };
  };
  rowConfig?: { keyField?: string };
  /** 行 class（对应 vxe 的 rowClassName），可以是字符串或按行计算的函数 */
  rowClassName?: string | ((params: { row: any }) => string);
  /** 排序配置（对应 vxe 的 sortConfig）；`remote: true` 时排序变化触发重新查询 */
  sortConfig?: { multiple?: boolean; remote?: boolean };
  /**
   * 树形表格配置（对应 vxe 的 treeConfig）。
   * - `transform: true`：后端返回**平铺列表**，由适配层用 `listToTree` 组成树
   * - `transform: false`：页面自己在 query 里组好树再返回（如菜单管理）
   * - `reserve: true`：刷新后保留展开状态
   */
  treeConfig?: {
    parentField?: string;
    reserve?: boolean;
    rowField?: string;
    transform?: boolean;
  };
  toolbarConfig?: { search?: boolean };
}

export interface UseGridOptions {
  formOptions?: FormProps;
  gridOptions: GridOptions;
}

export interface GridToolbarContext {
  clearCheckboxRow: () => void;
  getCheckboxRecords: () => any[];
}

export interface GridComponentProps {
  class?: string;
  slots?: Record<string, (params: { row: any }) => ReactNode>;
  tableTitle?: ReactNode;
  /** 标题旁的帮助提示（对应 vxe toolbar 的 tableTitleHelp） */
  tableTitleHelp?: ReactNode;
  toolbarTools?: ((ctx: GridToolbarContext) => ReactNode) | ReactNode;
}

interface GridState {
  current: number;
  dataSource: any[];
  /** 树形表格的展开行 key（对应 vxe 的 row.expand 标记，React 侧集中管理） */
  expandedKeys: Key[];
  loading: boolean;
  pageSize: number;
  selectedKeys: Key[];
  selectedRows: any[];
  showSearchForm: boolean;
  sorts: GridSortParam[];
  total: number;
}

export interface GridApi {
  formApi?: ReturnType<typeof useVbenForm>[1];
  grid: {
    clearCheckboxRow: () => void;
    /** 当前展示的数据（树形表格下为组树后的结果，对应 vxe 的 grid.getData()） */
    getData: () => any[];
    getCheckboxRecords: () => any[];
    /** 展开/折叠全部树节点（对应 vxe 的 setAllTreeExpand） */
    setAllTreeExpand: (expanded: boolean) => void;
    /** 展开/折叠单个节点（对应 vxe 的 setTreeExpand） */
    setTreeExpand: (row: any, expanded: boolean) => void;
  };
  query: () => Promise<void>;
  reload: (formValues?: Record<string, any>) => Promise<void>;
}

/**
 * 平铺列表 → 树（对应 vxe `treeConfig.transform: true`）。
 * 先浅拷贝每一行，避免 `listToTree` 往接口返回的对象上写 `children`。
 */
function buildTreeData(
  rows: any[],
  treeConfig?: GridOptions['treeConfig'],
): any[] {
  if (!treeConfig?.transform) return rows;
  const tree = listToTree<any>(
    rows.map((row) => ({ ...row })),
    {
      id: treeConfig.rowField ?? 'id',
      pid: treeConfig.parentField ?? 'parentId',
    },
  );
  pruneEmptyChildren(tree);
  return tree;
}

/**
 * 删掉空的 `children` 数组。`listToTree` 会给每个节点补 `children: []`，
 * 而 antd Table 判断「有无子节点」是真值检查，空数组会让叶子节点也长出展开箭头。
 *
 * 导出给页面复用：`treeConfig.transform: false` 的页面（如菜单管理）自己在
 * query 里组树，需要手动调这个函数。
 */
export function pruneEmptyChildren(nodes: any[]) {
  for (const node of nodes) {
    const children = node.children;
    if (Array.isArray(children) && children.length > 0) {
      pruneEmptyChildren(children);
    } else {
      delete node.children;
    }
  }
}

/** 收集所有「有子节点」的行 key，用于一键展开全部 */
function collectParentKeys(nodes: any[], keyField: string): Key[] {
  const keys: Key[] = [];
  const walk = (list: any[]) => {
    for (const node of list) {
      if (Array.isArray(node.children) && node.children.length > 0) {
        keys.push(node[keyField]);
        walk(node.children);
      }
    }
  };
  walk(nodes);
  return keys;
}

/**
 * vxe-grid → antd Table 适配层（对应 Vue 侧 plugins/vxe-table/use-vxe-grid.vue +
 * adapter/vxe-table.ts）。保留 `useVbenVxeGrid(options) → [Grid, gridApi]` 签名。
 *
 * 架构：表格状态放在 TanStack Store（与 Vben 一致），返回的 Grid 组件身份稳定，
 * 通过 useStore 订阅 → 避免每次渲染重挂载子树，同时让工具栏能响应选中态变化。
 *
 * ⚠️ 像素保真（方案 C 最大风险点）：vxe-table 与 antd Table 的行高/边框/表头底色/
 * 选中态/分页样式存在原生差异，此处用 bordered + size=small 逼近，剩余差距由
 * e2e/visual 像素门禁量化，作为后续表格策略决策依据。
 */
export function useGrid(
  options: UseGridOptions,
): [(props: GridComponentProps) => ReactNode, GridApi] {
  const { formOptions, gridOptions } = options;
  const keyField = gridOptions.rowConfig?.keyField ?? 'id';

  const [Form, formApi] = useVbenForm({
    ...(formOptions ?? { schema: [] as FormSchema[] }),
    // 查询表单用紧凑模式（对应 Vue 侧 useTableForm 的 compact: true）
    compact: true,
    showDefaultActions: false,
  });

  const storeRef = useRef<Store<GridState> | undefined>(undefined);
  if (!storeRef.current) {
    storeRef.current = new Store<GridState>({
      current: 1,
      dataSource: [],
      expandedKeys: [],
      loading: false,
      pageSize: 10,
      selectedKeys: [],
      selectedRows: [],
      showSearchForm: true,
      sorts: [],
      total: 0,
    });
  }
  const store = storeRef.current;

  // 用 ref 持有最新的 gridOptions/formApi，供稳定的 runQuery 读取
  const latest = useRef({ formApi, formOptions, gridOptions, keyField });
  latest.current = { formApi, formOptions, gridOptions, keyField };

  const runQuery = useCallback(async (page: number, size: number) => {
    const { formApi: fa, formOptions: fo, gridOptions: go } = latest.current;
    if (!go.proxyConfig?.ajax?.query) return;
    store.setState((prev) => ({ ...prev, loading: true }));
    try {
      const formValues = fo ? await fa.getValues() : {};
      const result = await go.proxyConfig.ajax.query(
        {
          page: { currentPage: page, pageSize: size },
          // 排序状态从 store 取，保证 query 闭包拿到的是最新值
          sorts: store.state.sorts,
        },
        formValues,
      );
      const rows = result?.rows ?? [];
      // 树形表格：平铺 → 树。store 里存的就是最终展示用的数据，
      // 因此 grid.getData() 与 vxe 的语义一致（返回组树后的结果）
      const data = buildTreeData(rows, go.treeConfig);
      store.setState((prev) => ({
        ...prev,
        current: page,
        dataSource: data,
        // 新数据到达后的展开态：treeConfig.reserve 为 true 时保留（对应 vxe 的
        // 「刷新接口后记录展开行的情况」），否则清空、由页面的 querySuccess 决定
        expandedKeys: go.treeConfig
          ? go.treeConfig.reserve
            ? prev.expandedKeys
            : []
          : prev.expandedKeys,
        loading: false,
        pageSize: size,
        selectedKeys: go.checkboxConfig?.reserve ? prev.selectedKeys : [],
        selectedRows: go.checkboxConfig?.reserve ? prev.selectedRows : [],
        total: result?.total ?? 0,
      }));
      go.proxyConfig.ajax.querySuccess?.(rows);
    } catch {
      store.setState((prev) => ({ ...prev, loading: false }));
    }
  }, [store]);

  const runQueryRef = useRef(runQuery);
  runQueryRef.current = runQuery;

  useEffect(() => {
    void runQuery(1, 10);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const api = useMemo<GridApi>(
    () => ({
      formApi,
      grid: {
        clearCheckboxRow: () => {
          store.setState((prev) => ({
            ...prev,
            selectedKeys: [],
            selectedRows: [],
          }));
        },
        getData: () => store.state.dataSource,
        getCheckboxRecords: () => store.state.selectedRows,
        setAllTreeExpand: (expanded: boolean) => {
          store.setState((prev) => ({
            ...prev,
            expandedKeys: expanded
              ? collectParentKeys(prev.dataSource, keyField)
              : [],
          }));
        },
        setTreeExpand: (row: any, expanded: boolean) => {
          const key = row?.[keyField];
          if (key === undefined || key === null) return;
          store.setState((prev) => ({
            ...prev,
            expandedKeys: expanded
              ? [...new Set([...prev.expandedKeys, key as Key])]
              : prev.expandedKeys.filter((item) => item !== key),
          }));
        },
      },
      query: async () => {
        await runQueryRef.current(store.state.current, store.state.pageSize);
      },
      reload: async () => {
        await runQueryRef.current(1, store.state.pageSize);
      },
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [formApi, store],
  );

  const antdColumns = useMemo<TableProps<any>['columns']>(() => {
    return gridOptions.columns
      .filter((col) => col.type !== 'checkbox')
      .map((col) => ({
        // ⚠️ vxe 的全局配置是 `align: 'center'`（见 Vue 侧 setupVbenVxeTable），
        // 即「未显式声明对齐方式的列一律居中」。antd Table 默认左对齐，
        // 不补这个默认值会导致所有表格与 Vue 端肉眼可见地不一致。
        align: col.align ?? 'center',
        dataIndex: col.field,
        ellipsis: col.field !== 'action',
        fixed: col.fixed,
        key: col.field,
        // ⚠️ `minWidth` 不透传：antd 的 ColumnType 没有这个属性（vxe 专有）。
        // 表格已设 `scroll={{x:'max-content'}}`，列宽按内容自适应，效果接近 minWidth。
        render:
          typeof col.slots?.default === 'function'
            ? (_: any, row: any) =>
                (col.slots!.default as (p: { row: any }) => ReactNode)({ row })
            : col.formatter
              ? (_: any, row: any) =>
                  col.formatter!({ cellValue: row[col.field ?? ''], row })
              : undefined,
        // `sortable: true` + 远程排序：交给 antd 渲染排序箭头，实际排序由后端完成
        sorter: col.sortable ? true : undefined,
        title: col.title,
        width: col.width === 'auto' ? (col.field === 'action' ? 114 : undefined) : col.width,
      }));
  }, [gridOptions.columns]);

  const hasCheckbox = gridOptions.columns.some((col) => col.type === 'checkbox');
  // pagerConfig 支持两种关闭写法：`false` 或 `{ enabled: false }`（后者是 vxe 的形式）
  const pager =
    gridOptions.pagerConfig !== false &&
    (gridOptions.pagerConfig as Record<string, any> | undefined)?.enabled !==
      false;
  // 只要声明了 treeConfig 就是树形表格；`transform` 只决定「是否由适配层做平铺转树」
  // （菜单管理页 transform: false，由页面自己在 query 里组树，但同样需要展开/双击能力）
  const isTree = !!gridOptions.treeConfig;
  const reserve = !!gridOptions.checkboxConfig?.reserve;
  const remoteSort = !!gridOptions.sortConfig?.remote;
  const columns = gridOptions.columns;

  // 稳定的 Grid 组件：通过 useStore 订阅状态，身份不随渲染变化
  const Grid = useCallback(
    (props: GridComponentProps) => (
      <GridView
        {...props}
        antdColumns={antdColumns}
        api={api}
        checkMethod={gridOptions.checkboxConfig?.checkMethod}
        columns={columns}
        Form={Form}
        formApi={formApi}
        gridEvents={gridOptions.gridEvents}
        hasCheckbox={hasCheckbox}
        isTree={isTree}
        keyField={keyField}
        minHeight={gridOptions.minHeight}
        pager={pager}
        remoteSort={remoteSort}
        reserve={reserve}
        rowClassName={gridOptions.rowClassName}
        runQuery={runQuery}
        store={store}
      />
    ),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [antdColumns, api, columns, Form, formApi, hasCheckbox, isTree, keyField, pager, remoteSort, reserve, runQuery, store],
  );

  return [Grid, api];
}

interface GridViewProps extends GridComponentProps {
  antdColumns: TableProps<any>['columns'];
  api: GridApi;
  checkMethod?: (params: { row: any }) => boolean;
  columns: GridColumn[];
  Form: ComponentType<any>;
  formApi: FormApi2;
  gridEvents?: GridOptions['gridEvents'];
  hasCheckbox: boolean;
  isTree: boolean;
  keyField: string;
  minHeight?: number;
  pager: boolean;
  remoteSort: boolean;
  reserve: boolean;
  rowClassName?: GridOptions['rowClassName'];
  runQuery: (page: number, size: number) => Promise<void>;
  store: Store<GridState>;
}

type FormApi2 = ReturnType<typeof useVbenForm>[1];

function GridView({
  antdColumns,
  api,
  checkMethod,
  class: className,
  columns,
  Form,
  formApi,
  gridEvents,
  hasCheckbox,
  isTree,
  keyField,
  minHeight,
  pager,
  remoteSort,
  reserve,
  rowClassName,
  runQuery,
  slots,
  store,
  tableTitle,
  tableTitleHelp,
  toolbarTools,
}: GridViewProps) {
  const {
    current,
    dataSource,
    expandedKeys,
    loading,
    pageSize,
    selectedKeys,
    showSearchForm,
    total,
  } = useStore(store);

  const [tableElement, setTableElement] = useState<HTMLDivElement | null>(null);
  const [tableWidth, setTableWidth] = useState(0);
  useEffect(() => {
    if (!tableElement) return;
    const resize = () => setTableWidth(tableElement.clientWidth);
    const observer = new ResizeObserver(resize); observer.observe(tableElement); resize();
    return () => observer.disconnect();
  }, [tableElement]);
  const [hiddenColumns, setHiddenColumns] = useState<string[]>([]);
  const [maximized, setMaximized] = useState(false);
  // 解析列里用具名字符串引用的 slot（如 action）
  const resolvedColumns = (antdColumns ?? []).filter((col: any) => !hiddenColumns.includes(String(col.key))).map((col: any) => {
    const raw = columns.find(
      (c) => c.field === (col.dataIndex as string) && c.type !== 'checkbox',
    );
    const slotName = raw?.slots?.default;
    if (typeof slotName === 'string' && slots?.[slotName]) {
      const renderer = slots[slotName];
      return { ...col, render: (_: any, row: any) => renderer({ row }) };
    }
    return col;
  });

  // Vxe distributes remaining pixels to flexible columns using integer boundaries.
  // Avoid fractional cell centers that blur text and drift relative to the Vue baseline.
  const fixedWidth = resolvedColumns.reduce((sum: number, column: any) => sum + (Number(column.width) || 0), hasCheckbox ? Number(columns.find(c => c.type === 'checkbox')?.width ?? 60) : 0);
  const flexible = resolvedColumns.filter((column: any) => !column.width).length;
  const availableWidth = tableWidth < 640 ? Math.max(760,tableWidth) : tableWidth;
  let flexibleIndex = 0;
  const sizedColumns = resolvedColumns.map((column: any) => {
    if (column.width || !availableWidth || !flexible) return column;
    const remaining = Math.max(0, availableWidth-fixedWidth);
    const width = Math.floor((flexibleIndex+1)*remaining/flexible)-Math.floor(flexibleIndex*remaining/flexible);
    flexibleIndex++;
    return { ...column, width };
  });

  const rowSelection: TableProps<any>['rowSelection'] = hasCheckbox
    ? {
        // checkMethod 返回 false 的行禁用复选框（antd 用 getCheckboxProps 表达）
        getCheckboxProps: checkMethod
          ? (row: any) => ({ disabled: !checkMethod({ row }) })
          : undefined,
        onChange: (keys: any, rows: any) => {
          store.setState((prev) => ({
            ...prev,
            selectedKeys: keys as Key[],
            selectedRows: rows as any[],
          }));
        },
        columnWidth: columns.find(column => column.type === 'checkbox')?.width ?? 60,
        preserveSelectedRowKeys: reserve,
        selectedRowKeys: selectedKeys,
      }
    : undefined;

  /**
   * 树形表格的展开控制。antd 用受控的 `expandedRowKeys`，
   * 等价于 vxe 侧写在每行上的 `row.expand` 标记。
   */
  const expandable: TableProps<any>['expandable'] = isTree
    ? {
        expandedRowKeys: expandedKeys,
        onExpand: (expanded, record) => {
          const key = record?.[keyField];
          if (key === undefined || key === null) return;
          store.setState((prev) => ({
            ...prev,
            expandedKeys: expanded
              ? [...new Set([...prev.expandedKeys, key as Key])]
              : prev.expandedKeys.filter((item) => item !== key),
          }));
        },
      }
    : undefined;

  /** 双击行展开/收起子节点（对应 Vue 侧 gridEvents.cellDblclick） */
  const onRow = (row: any) => ({
    onClick: () => {
      gridEvents?.cellClick?.({ row });
    },
    onDoubleClick: () => {
      if (isTree && row?.children) {
        const key = row[keyField] as Key;
        const expanded = !expandedKeys.includes(key);
        store.setState((prev) => ({
          ...prev,
          expandedKeys: expanded
            ? [...new Set([...prev.expandedKeys, key])]
            : prev.expandedKeys.filter((item) => item !== key),
        }));
      }
      gridEvents?.cellDblclick?.({ row });
    },
  });

  const toolbarCtx: GridToolbarContext = {
    clearCheckboxRow: () => api.grid.clearCheckboxRow(),
    getCheckboxRecords: () => api.grid.getCheckboxRecords(),
  };

  /**
   * antd Table 的分页与排序都走这一个 onChange（Table 级 onChange 会同时收到
   * pagination / filters / sorter，若再给 pagination 单独挂 onChange 会触发两次查询）。
   */
  function handleTableChange(
    pagination: any,
    _filters: any,
    sorter: any,
  ) {
    const list: any[] = Array.isArray(sorter)
      ? sorter
      : sorter
        ? [sorter]
        : [];
    // antd 的 'ascend'/'descend' → 后端约定的 'asc'/'desc'
    const nextSorts: GridSortParam[] = list
      .filter((item) => item?.order && item?.field)
      .map((item) => ({
        field: String(item.field),
        order: item.order === 'ascend' ? 'asc' : 'desc',
      }));

    const sortsChanged =
      JSON.stringify(nextSorts) !== JSON.stringify(store.state.sorts);
    if (sortsChanged) {
      store.setState((prev) => ({ ...prev, sorts: nextSorts }));
      // 远程排序：排序变化后回到第一页重新查询（对应 vxe 的 gridEvents.sortChange）
      if (remoteSort) {
        void runQuery(1, pagination?.pageSize ?? store.state.pageSize);
        return;
      }
    }
    void runQuery(
      pagination?.current ?? 1,
      pagination?.pageSize ?? store.state.pageSize,
    );
  }

  return (
    <div className={cn('h-full rounded-md bg-card vben-grid', className)} style={maximized ? { position: 'fixed', inset: 0, zIndex: 1100 } : undefined}>
      <div className="flex h-full flex-col p-2">
        {formApi ? (
          <div className="vben-grid-query relative rounded-sm py-3 pb-8" hidden={!showSearchForm}>
            <Form />
            <div className="mt-2 flex justify-end gap-2">
              <Button
                onClick={async () => {
                  // formOptions.handleReset 存在时完全接管（对应 Vue 侧语义），
                  // 用于「重置时还要清掉表单之外的联动状态」的页面
                  const customReset = formApi.latestOptions.handleReset;
                  if (customReset) {
                    await customReset();
                    return;
                  }
                  await formApi.resetForm();
                  await runQuery(1, pageSize);
                }}
              >
                {$t('common.reset')}
              </Button>
              <Button onClick={() => runQuery(1, pageSize)} type="primary">
                {$t('common.search')}
              </Button>
              <Button
                onClick={() =>
                  store.setState((prev) => ({
                    ...prev,
                    showSearchForm: !prev.showSearchForm,
                  }))
                }
                type="link"
              >
                {showSearchForm ? '收起' : '展开'}{showSearchForm ? <ChevronUp className="size-3" /> : <ChevronDown className="size-3" />}
              </Button>
            </div>
            <div className="bg-background-deep absolute -left-2 bottom-1 z-100 h-2 w-[calc(100%+1rem)] overflow-hidden md:bottom-2 md:h-3"></div>
          </div>
        ) : null}

        {tableTitle || toolbarTools ? (
          <div className="vben-grid-toolbar mb-2 flex items-center justify-between">
            <div className="flex-center gap-1 text-[1rem] font-bold">
              {tableTitle}
              {!showSearchForm && <Button type="link" size="small" onClick={() => store.setState(prev => ({ ...prev, showSearchForm: true }))}>展开搜索</Button>}
              {tableTitleHelp ? (
                <span className="text-muted-foreground text-xs font-normal">
                  {tableTitleHelp}
                </span>
              ) : null}
            </div>
            <div className="flex items-center gap-2">
              {typeof toolbarTools === 'function'
                ? toolbarTools(toolbarCtx)
                : toolbarTools}
              {/* vxe toolbar 内建工具：刷新 / 全屏 / 列设置（对应 Vue 侧圆形图标按钮） */}
              <Button
                icon={<ReloadOutlined />}
                onClick={() => runQuery(current, pageSize)}
                shape="circle"
              />
              <Button aria-label="maximize-table" icon={<FullscreenOutlined />} shape="circle" onClick={() => setMaximized(!maximized)} />
              <Popover trigger="click" placement="bottomRight" title="列设置" content={<div className="flex flex-col gap-2">{(antdColumns ?? []).map((column: any) => <Checkbox key={column.key} checked={!hiddenColumns.includes(String(column.key))} onChange={event => setHiddenColumns(event.target.checked ? hiddenColumns.filter(key => key !== String(column.key)) : [...hiddenColumns, String(column.key)])}>{column.title}</Checkbox>)}</div>}><Button aria-label="table-columns" icon={<SettingOutlined />} shape="circle" /></Popover>
            </div>
          </div>
        ) : null}

        <div
          ref={setTableElement}
          className="vben-grid-table min-h-0 flex-1"
          style={minHeight === undefined ? undefined : { minHeight }}
        >
          <Table
            columns={sizedColumns}
            dataSource={dataSource}
            expandable={expandable}
            loading={loading}
            locale={{
              emptyText: (
                <Empty
                  description={$t('common.noData')}
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                />
              ),
            }}
            pagination={false}
            onChange={handleTableChange}
            onRow={onRow}
            rowClassName={
              typeof rowClassName === 'function'
                ? (row: any) => rowClassName({ row })
                : rowClassName
            }
            rowKey={keyField}
            rowSelection={rowSelection}
            tableLayout="fixed"
            scroll={{ x: '100%' }}
            size="small"
          />
        </div>
        {pager && <GridPagination current={current} pageSize={pageSize} total={total} onChange={(page,size) => void runQuery(page,size)} />}
      </div>
    </div>
  );
}

/** 保留 Vue 侧同名导出，便于调用点零改动 */
export const useVbenVxeGrid = useGrid;

/** 是否有勾选行（对应 adapter/vxe-table.ts 的 vxeCheckboxChecked） */
export function vxeCheckboxChecked(gridApi: GridApi): boolean {
  return gridApi.grid.getCheckboxRecords().length > 0;
}

/**
 * 把排序状态转成后端约定的查询参数（对应 Vue 侧 adapter/vxe-table.ts 的 addSortParams）。
 *
 * 后端契约：`orderByColumn` 为逗号分隔的字段名、`isAsc` 为逗号分隔的 asc/desc，
 * 两者按位置一一对应，支持多字段排序（见 `api/common.ts` 里 PageQuery 的注释）。
 * 排序为空时**不写这两个参数**，避免后端收到空串。
 */
export function addSortParams(
  params: Record<string, any>,
  sortList: GridSortParam[],
) {
  if (!sortList || sortList.length === 0) {
    return;
  }
  params.orderByColumn = sortList.map((item) => item.field).join(',');
  params.isAsc = sortList.map((item) => item.order).join(',');
}

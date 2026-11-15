import type { ComponentType, ReactNode } from 'react';

import type { FormProps, FormSchema } from '@vben-core/form-ui';

import { useCallback, useEffect, useMemo, useRef } from 'react';

import { $t } from '@vben/locales';
import { useVbenForm } from '@vben-core/form-ui';
import { Store, useStore } from '@vben-core/shared/store';
import { cn } from '@vben-core/shared/utils';
import { Button, Empty, Table } from 'antd';
import type { TableProps } from 'antd';
import {
  FullscreenOutlined,
  ReloadOutlined,
  SettingOutlined,
} from '@ant-design/icons';

type Key = number | string;

export interface GridColumn {
  align?: 'center' | 'left' | 'right';
  field?: string;
  fixed?: 'left' | 'right';
  resizable?: boolean;
  slots?: {
    default?: ((params: { row: any }) => ReactNode) | string;
  };
  title?: string;
  type?: 'checkbox';
  width?: number | string;
}

export interface GridOptions {
  checkboxConfig?: { highlight?: boolean; reserve?: boolean };
  columns: GridColumn[];
  height?: string;
  id?: string;
  pagerConfig?: false | Record<string, any>;
  proxyConfig?: {
    ajax: {
      query: (
        params: { page: { currentPage: number; pageSize: number } },
        formValues?: Record<string, any>,
      ) => Promise<{ rows: any[]; total: number }>;
    };
  };
  rowConfig?: { keyField?: string };
  toolbarConfig?: { search?: boolean };
}

export interface UseGridOptions {
  formOptions?: FormProps;
  gridOptions: GridOptions;
}

export interface GridToolbarContext {
  getCheckboxRecords: () => any[];
}

export interface GridComponentProps {
  class?: string;
  slots?: Record<string, (params: { row: any }) => ReactNode>;
  tableTitle?: ReactNode;
  toolbarTools?: ((ctx: GridToolbarContext) => ReactNode) | ReactNode;
}

interface GridState {
  current: number;
  dataSource: any[];
  loading: boolean;
  pageSize: number;
  selectedKeys: Key[];
  selectedRows: any[];
  showSearchForm: boolean;
  total: number;
}

export interface GridApi {
  formApi?: ReturnType<typeof useVbenForm>[1];
  grid: { getCheckboxRecords: () => any[] };
  query: () => Promise<void>;
  reload: (formValues?: Record<string, any>) => Promise<void>;
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
      loading: false,
      pageSize: 10,
      selectedKeys: [],
      selectedRows: [],
      showSearchForm: true,
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
        { page: { currentPage: page, pageSize: size } },
        formValues,
      );
      store.setState((prev) => ({
        ...prev,
        current: page,
        dataSource: result?.rows ?? [],
        loading: false,
        pageSize: size,
        selectedKeys: go.checkboxConfig?.reserve ? prev.selectedKeys : [],
        selectedRows: go.checkboxConfig?.reserve ? prev.selectedRows : [],
        total: result?.total ?? 0,
      }));
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
        getCheckboxRecords: () => store.state.selectedRows,
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
        align: col.align,
        dataIndex: col.field,
        fixed: col.fixed,
        key: col.field,
        render:
          typeof col.slots?.default === 'function'
            ? (_: any, row: any) =>
                (col.slots!.default as (p: { row: any }) => ReactNode)({ row })
            : undefined,
        title: col.title,
        width: col.width === 'auto' ? undefined : col.width,
      }));
  }, [gridOptions.columns]);

  const hasCheckbox = gridOptions.columns.some((col) => col.type === 'checkbox');
  const pager = gridOptions.pagerConfig !== false;
  const reserve = !!gridOptions.checkboxConfig?.reserve;
  const columns = gridOptions.columns;

  // 稳定的 Grid 组件：通过 useStore 订阅状态，身份不随渲染变化
  const Grid = useCallback(
    (props: GridComponentProps) => (
      <GridView
        {...props}
        antdColumns={antdColumns}
        api={api}
        columns={columns}
        Form={Form}
        formApi={formApi}
        hasCheckbox={hasCheckbox}
        keyField={keyField}
        pager={pager}
        reserve={reserve}
        runQuery={runQuery}
        store={store}
      />
    ),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [antdColumns, api, columns, Form, formApi, hasCheckbox, keyField, pager, reserve, runQuery, store],
  );

  return [Grid, api];
}

interface GridViewProps extends GridComponentProps {
  antdColumns: TableProps<any>['columns'];
  api: GridApi;
  columns: GridColumn[];
  Form: ComponentType<any>;
  formApi: FormApi2;
  hasCheckbox: boolean;
  keyField: string;
  pager: boolean;
  reserve: boolean;
  runQuery: (page: number, size: number) => Promise<void>;
  store: Store<GridState>;
}

type FormApi2 = ReturnType<typeof useVbenForm>[1];

function GridView({
  antdColumns,
  api,
  class: className,
  columns,
  Form,
  formApi,
  hasCheckbox,
  keyField,
  pager,
  reserve,
  runQuery,
  slots,
  store,
  tableTitle,
  toolbarTools,
}: GridViewProps) {
  const { current, dataSource, loading, pageSize, selectedKeys, showSearchForm, total } =
    useStore(store);

  // 解析列里用具名字符串引用的 slot（如 action）
  const resolvedColumns = (antdColumns ?? []).map((col: any) => {
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

  const rowSelection: TableProps<any>['rowSelection'] = hasCheckbox
    ? {
        onChange: (keys: any, rows: any) => {
          store.setState((prev) => ({
            ...prev,
            selectedKeys: keys as Key[],
            selectedRows: rows as any[],
          }));
        },
        preserveSelectedRowKeys: reserve,
        selectedRowKeys: selectedKeys,
      }
    : undefined;

  const toolbarCtx: GridToolbarContext = {
    getCheckboxRecords: () => api.grid.getCheckboxRecords(),
  };

  return (
    <div className={cn('h-full rounded-md bg-card', className)}>
      <div className="flex h-full flex-col p-2">
        {formApi ? (
          <div className="relative rounded-sm py-3 pb-8" hidden={!showSearchForm}>
            <Form />
            <div className="mt-2 flex justify-end gap-2">
              <Button
                onClick={async () => {
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
                {showSearchForm ? '收起' : '展开'}
              </Button>
            </div>
            <div className="bg-background-deep absolute -left-2 bottom-1 z-100 h-2 w-[calc(100%+1rem)] overflow-hidden md:bottom-2 md:h-3"></div>
          </div>
        ) : null}

        {tableTitle || toolbarTools ? (
          <div className="mb-2 flex items-center justify-between">
            <div className="flex-center gap-1 text-[1rem] font-bold">
              {tableTitle}
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
              <Button icon={<FullscreenOutlined />} shape="circle" />
              <Button icon={<SettingOutlined />} shape="circle" />
            </div>
          </div>
        ) : null}

        <div className="min-h-0 flex-1">
          <Table
            bordered
            columns={resolvedColumns}
            dataSource={dataSource}
            loading={loading}
            locale={{
              emptyText: (
                <Empty
                  description={$t('common.noData')}
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                />
              ),
            }}
            pagination={
              pager
                ? {
                    current,
                    onChange: (page, size) => runQuery(page, size),
                    pageSize,
                    pageSizeOptions: [10, 20, 30, 50, 100, 200],
                    showQuickJumper: true,
                    showSizeChanger: true,
                    showTotal: (t) => `共 ${t} 条记录`,
                    size: 'small',
                    total,
                  }
                : false
            }
            rowKey={keyField}
            rowSelection={rowSelection}
            scroll={{ x: 'max-content' }}
            size="small"
          />
        </div>
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

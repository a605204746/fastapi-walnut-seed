import { useRef } from 'react';

import type { DictType } from '@/api/system/dict/dict-type-model';

import { $t } from '@walnut/locales';
import { Button, Popconfirm, Space } from 'antd';

import { useGrid, vxeCheckboxChecked } from '@/adapter/grid';
import {
  dictTypeExport,
  dictTypeList,
  dictTypeRemove,
  refreshDictTypeCache,
} from '@/api/system/dict/dict-type';
import { Access } from '@/components/access/Access';
import { getMessageApi } from '@/services/message';
import { useBlobExport } from '@/utils/file/export';

import {
  DictDataModal,
  type DictDataModalHandle,
} from '../data/dict-data-modal';
import { columns, querySchema } from './data';
import { DictTypeModal, type DictTypeModalHandle } from './dict-type-modal';

/**
 * 字典类型面板（对应 Vue 侧 views/system/dict/type/index.vue，210 行）。
 *
 * 交互：**点击表格任意一行**打开该字典类型的数据面板（Vue 侧用
 * `gridEvents.cellClick` + `dictDataModalRef.value?.open(row)`，React 侧
 * 由适配层的 `gridEvents.cellClick` + ref 命令式打开，形态一致）。
 *
 * ⚠️ 已知视觉差异：Vue 侧 `rowConfig.isCurrent: true` + 一段 scss 让「当前行」
 * 加粗（`div#dict-type .vxe-body--row.row--current { font-semibold }`）。
 * antd Table 没有 current-row 概念，这里只保留了 `hover:cursor-pointer`，
 * 当前行高亮列入 B 阶段的像素补齐项。
 */
export function DictTypePanel({ className }: { className?: string }) {
  const typeModalRef = useRef<DictTypeModalHandle>(null);
  const dataModalRef = useRef<DictDataModalHandle>(null);

  const formOptions = {
    commonConfig: {
      componentProps: { allowClear: true },
      labelWidth: 70,
    },
    handleReset: async () => {
      await tableApi.formApi?.resetForm();
      await tableApi.reload();
    },
    schema: querySchema(),
    wrapperClass: 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3',
  };

  const gridOptions = {
    checkboxConfig: { highlight: true, reserve: true },
    columns,
    gridEvents: {
      // 点击行打开字典数据面板
      cellClick: ({ row }: { row: DictType }) => {
        dataModalRef.current?.open(row);
      },
    },
    height: 'auto',
    id: 'system-dict-type-index',
    pagerConfig: {},
    proxyConfig: {
      ajax: {
        query: async (
          { page }: { page: { currentPage: number; pageSize: number } },
          formValues: Record<string, any> = {},
        ) => {
          return await dictTypeList({
            pageNum: page.currentPage,
            pageSize: page.pageSize,
            ...formValues,
          });
        },
      },
    },
    rowClassName: 'hover:cursor-pointer',
    rowConfig: { keyField: 'id' },
  };

  const [BasicTable, tableApi] = useGrid({ formOptions, gridOptions });

  const { buildExportFileName, exportBlob, exportLoading } = useBlobExport(
    dictTypeExport,
  );

  async function handleExport() {
    const formValues = await tableApi.formApi?.getValues();
    const fileName = buildExportFileName('字典类型数据');
    await exportBlob({ data: formValues, fileName });
  }

  async function handleDelete(row: DictType) {
    await dictTypeRemove([row.id]);
    await tableApi.query();
  }

  function handleMultiDelete() {
    const rows = tableApi.grid.getCheckboxRecords();
    const ids = rows.map((row: DictType) => row.id);
    getMessageApi()?.modal.confirm({
      content: `确认删除选中的${ids.length}条记录吗？`,
      okType: 'danger',
      onOk: async () => {
        await dictTypeRemove(ids);
        await tableApi.query();
      },
      title: $t('pages.common.tip'),
    });
  }

  async function handleRefreshCache() {
    await refreshDictTypeCache();
    await tableApi.query();
  }

  return (
    <div className={className}>
      <BasicTable
        slots={{
          action: ({ row }) => (
            <Space>
              <Access code={['system:dict:edit']}>
                <Button
                  onClick={(e) => {
                    e.stopPropagation();
                    typeModalRef.current?.openEdit((row as DictType).id);
                  }}
                  size="small"
                  type="link"
                >
                  {$t('pages.common.edit')}
                </Button>
              </Access>
              <Access code={['system:dict:remove']}>
                <Popconfirm
                  onConfirm={() => handleDelete(row as DictType)}
                  placement="left"
                  title="确认删除？"
                >
                  <Button
                    danger
                    onClick={(e) => e.stopPropagation()}
                    size="small"
                    type="link"
                  >
                    {$t('pages.common.delete')}
                  </Button>
                </Popconfirm>
              </Access>
            </Space>
          ),
        }}
        tableTitle="字典类型列表"
        toolbarTools={() => (
          <Space>
            <Access code={['system:dict:edit']}>
              <Button onClick={handleRefreshCache}>刷新缓存</Button>
            </Access>
            <Access code={['system:dict:export']}>
              <Button
                disabled={exportLoading}
                loading={exportLoading}
                onClick={handleExport}
              >
                {$t('pages.common.export')}
              </Button>
            </Access>
            <Access code={['system:dict:remove']}>
              <Button
                danger
                disabled={!vxeCheckboxChecked(tableApi)}
                onClick={handleMultiDelete}
                type="primary"
              >
                {$t('pages.common.delete')}
              </Button>
            </Access>
            <Access code={['system:dict:add']}>
              <Button
                onClick={() => typeModalRef.current?.openAdd()}
                type="primary"
              >
                {$t('pages.common.add')}
              </Button>
            </Access>
          </Space>
        )}
      />
      <DictTypeModal onReload={() => void tableApi.query()} ref={typeModalRef} />
      <DictDataModal ref={dataModalRef} />
    </div>
  );
}

export default DictTypePanel;

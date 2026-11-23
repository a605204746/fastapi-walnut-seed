import { forwardRef, useImperativeHandle, useRef, useState } from 'react';

import type { DictData } from '@/api/system/dict/dict-data-model';
import type { DictType } from '@/api/system/dict/dict-type-model';

import { $t } from '@/core/locales';
import { useWalnutModal } from '@/core/popup-ui';
import { Button, Popconfirm, Space } from 'antd';

import { useGrid, vxeCheckboxChecked } from '@/adapter/grid';
import {
  dictDataExport,
  dictDataList,
  dictDataRemove,
} from '@/api/system/dict/dict-data';
import { Access } from '@/components/access/Access';
import { getMessageApi } from '@/services/message';
import { useDictStore } from '@/store/dict';
import { useBlobExport } from '@/utils/file/export';

import { columns, querySchema } from './data';
import {
  DictDataItemModal,
  type DictDataItemModalHandle,
} from './dict-data-item-modal';

export interface DictDataModalHandle {
  /** 打开指定字典类型的数据面板 */
  open: (row: DictType) => void;
}

/**
 * 字典数据面板（对应 Vue 侧 dict/data/dict-data-modal.vue，208 行）。
 *
 * 由字典类型表格的行点击触发（见 `../type/index.tsx`），弹窗内嵌一张表格 +
 * 新增/编辑弹窗，形成「类型 → 数据 → 单条」三层嵌套。
 *
 * 加载时序与 Vue 侧的差异：Vue 的表格随弹窗内容懒挂载，首次自动查询
 * 发生在 open() 设好 dictType 之后；React 侧 useGrid 的首查 effect 在页面
 * 挂载时就跑（那时 dictType 还是空），故置 `autoLoad: false`，每次打开
 * 都在 onOpened 里手动 reload。
 */
export const DictDataModal = forwardRef<DictDataModalHandle>(
  function DictDataModal(_props, ref) {
    useDictStore((state) => state.map);
    const itemModalRef = useRef<DictDataItemModalHandle>(null);
    const [dictName, setDictName] = useState('');
    /** dictType 用 ref 存：查询闭包要读到最新值，state 更新是异步的 */
    const dictTypeRef = useRef('');

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
      wrapperClass: 'grid-cols-1 md:grid-cols-2',
    };

    const gridOptions = {
      checkboxConfig: { highlight: true, reserve: true },
      columns,
      height: 'auto',
      id: 'system-dict-data-modal',
      minHeight: 300,
      pagerConfig: {},
      proxyConfig: {
        // 页面挂载时不首查（dictType 尚未选定），打开弹窗时再 reload
        autoLoad: false,
        ajax: {
          query: async (
            { page }: { page: { currentPage: number; pageSize: number } },
            formValues: Record<string, any> = {},
          ) => {
            return await dictDataList({
              pageNum: page.currentPage,
              pageSize: page.pageSize,
              dictType: dictTypeRef.current,
              ...formValues,
            });
          },
        },
      },
      rowConfig: { keyField: 'id' },
    };

    const [BasicTable, tableApi] = useGrid({ formOptions, gridOptions });

    const [BasicModal, modalApi] = useWalnutModal({
      fullscreenButton: true,
      onOpened: async () => {
        // 每次打开都按当前 dictType 重新加载（含首次）
        await tableApi.reload();
      },
    });

    useImperativeHandle(
      ref,
      () => ({
        open: (row) => {
          dictTypeRef.current = row.dictType;
          setDictName(row.dictName);
          modalApi.open();
        },
      }),
      [modalApi],
    );

    function handleAdd() {
      itemModalRef.current?.open({ dictType: dictTypeRef.current });
    }

    function handleEdit(record: DictData) {
      itemModalRef.current?.open({
        dictType: dictTypeRef.current,
        id: record.id,
      });
    }

    async function handleDelete(row: DictData) {
      await dictDataRemove([row.id]);
      await tableApi.query();
    }

    function handleMultiDelete() {
      const rows = tableApi.grid.getCheckboxRecords();
      const ids = rows.map((row: DictData) => row.id);
      getMessageApi()?.modal.confirm({
        content: `确认删除选中的${ids.length}条记录吗？`,
        okType: 'danger',
        onOk: async () => {
          await dictDataRemove(ids);
          await tableApi.query();
        },
        title: $t('pages.common.tip'),
      });
    }

    const { buildExportFileName, exportBlob, exportLoading } = useBlobExport(
      dictDataExport,
    );

    async function handleExport() {
      const formValues = await tableApi.formApi?.getValues();
      const fileName = buildExportFileName('字典数据');
      await exportBlob({
        data: { ...formValues, dictType: dictTypeRef.current },
        fileName,
      });
    }

    return (
      <BasicModal class="w-[900px]" title={`字典数据【${dictName}】`}>
        <BasicTable
          slots={{
            action: ({ row }) => (
              <Space>
                <Access code={['system:dict:edit']}>
                  <Button
                    onClick={(e) => {
                      e.stopPropagation();
                      handleEdit(row as DictData);
                    }}
                    size="small"
                    type="link"
                  >
                    {$t('pages.common.edit')}
                  </Button>
                </Access>
                <Access code={['system:dict:remove']}>
                  <Popconfirm
                    onConfirm={() => handleDelete(row as DictData)}
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
          tableTitle="字典数据列表"
          toolbarTools={() => (
            <Space>
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
              <Access code={['system:dict:export']}>
                <Button
                  disabled={exportLoading}
                  loading={exportLoading}
                  onClick={handleExport}
                >
                  {$t('pages.common.export')}
                </Button>
              </Access>
              <Access code={['system:dict:add']}>
                <Button onClick={handleAdd} type="primary">
                  {$t('pages.common.add')}
                </Button>
              </Access>
            </Space>
          )}
        />
        <DictDataItemModal
          onReload={() => void tableApi.query()}
          ref={itemModalRef}
        />
      </BasicModal>
    );
  },
);

export default DictDataModal;

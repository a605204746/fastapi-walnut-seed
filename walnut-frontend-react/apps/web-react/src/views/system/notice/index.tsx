import { useState } from 'react';
import { Button, Popconfirm, Space } from 'antd';
import { useGrid, vxeCheckboxChecked } from '@/adapter/grid';
import { noticeList, noticeRemove } from '@/api/system/notice';
import { Access } from '@/components/access/Access';
import { Page } from '@/components/page/Page';
import { getMessageApi } from '@/services/message';
import { useDictStore } from '@/store/dict';
import { columns, querySchema } from './data';
import { NoticeEditor } from './notice-editor';

export default function NoticePage() {
  useDictStore((s) => s.map);
  const [editor, setEditor] = useState<{ id?: number | string } | null>(null);
  const [BasicTable, tableApi] = useGrid({
    formOptions: {
      commonConfig: { labelWidth: 80, componentProps: { allowClear: true } },
      schema: querySchema(),
      wrapperClass: 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4',
    },
    gridOptions: {
      id: 'system-notice-index',
      columns,
      height: 'auto',
      pagerConfig: {},
      checkboxConfig: { highlight: true, reserve: true },
      rowConfig: { keyField: 'id' },
      proxyConfig: {
        ajax: {
          query: ({ page }, values) =>
            noticeList({ pageNum: page.currentPage, pageSize: page.pageSize, ...values }),
        },
      },
    },
  });
  const reload = () => {
    void tableApi.query();
  };
  async function remove(ids: (number | string)[]) {
    await noticeRemove(ids);
    reload();
  }
  return (
    <Page autoContentHeight>
      <BasicTable
        tableTitle="通知公告列表"
        toolbarTools={() => (
          <Space>
            <Access code={['system:notice:remove']}>
              <Button
                type="primary"
                danger
                disabled={!vxeCheckboxChecked(tableApi)}
                onClick={() => {
                  const ids = tableApi.grid.getCheckboxRecords().map((row) => row.id);
                  getMessageApi()?.modal.confirm({
                    title: '提示',
                    content: `确认删除选中的${ids.length}条记录吗？`,
                    okType: 'danger',
                    onOk: () => remove(ids),
                  });
                }}
              >
                删除
              </Button>
            </Access>
            <Access code={['system:notice:add']}>
              <Button type="primary" onClick={() => setEditor({})}>
                新增
              </Button>
            </Access>
          </Space>
        )}
        slots={{
          action: ({ row }) => (
            <Space>
              <Access code={['system:notice:edit']}>
                <Button type="link" size="small" onClick={() => setEditor({ id: row.id })}>
                  编辑
                </Button>
              </Access>
              <Access code={['system:notice:remove']}>
                <Popconfirm title="确认删除？" onConfirm={() => remove([row.id])}>
                  <Button type="link" size="small" danger>
                    删除
                  </Button>
                </Popconfirm>
              </Access>
            </Space>
          ),
        }}
      />
      {editor && <NoticeEditor {...editor} onSaved={reload} onClose={() => setEditor(null)} />}
    </Page>
  );
}

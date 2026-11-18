import { forwardRef, useImperativeHandle, useState } from 'react';

import type { UploadFile } from 'antd';

import { useVbenModal } from '@vben-core/popup-ui';
import { InboxOutlined } from '@ant-design/icons';
import { Button, Modal, Switch, Upload } from 'antd';

import { downloadImportTemplate, userImportData } from '@/api/system/user';
import { getMessageApi } from '@/services/message';
import { useBlobExport } from '@/utils/file/export';

const { Dragger } = Upload;

export interface UserImportModalHandle {
  open: () => void;
}

export interface UserImportModalProps {
  onReload: () => void;
}

/**
 * 用户导入弹窗（对应 Vue 侧 views/system/user/user-import-modal.vue，115 行）。
 *
 * 与 Vue 版的差异：
 * - `Upload.Dragger` 的 `v-model:file-list` → 受控 `fileList` + `onChange`。
 * - `icon-[vscode-icons--file-type-excel]` 类名图标 → 无对应 antd 图标，
 *   改为文字「下载模板」（两套 tailwind 配置都没装 iconify class 插件）。
 * - `window.modal.success/error` → `getMessageApi()?.modal`，未初始化时回退 antd 静态方法。
 * - 导入结果 `msg` 是后端拼好的 HTML（后端已做 XSS 处理），沿用 `innerHTML`。
 */
export const UserImportModal = forwardRef<
  UserImportModalHandle,
  UserImportModalProps
>(function UserImportModal({ onReload }, ref) {
  const [fileList, setFileList] = useState<UploadFile[]>([]);
  const [checked, setChecked] = useState(false);

  const [BasicModal, modalApi] = useVbenModal({
    closeOnClickModal: false,
    fullscreenButton: false,
    onCancel: handleCancel,
    onConfirm: handleSubmit,
  });

  useImperativeHandle(
    ref,
    () => ({
      open: () => {
        modalApi.setData({});
        modalApi.open();
      },
    }),
    [modalApi],
  );

  const { exportBlob, exportLoading } = useBlobExport(downloadImportTemplate);

  function handleExport() {
    void exportBlob({ data: {}, fileName: '用户导入模板.xlsx' });
  }

  function handleCancel() {
    void modalApi.close();
    setFileList([]);
    setChecked(false);
  }

  function showResult(type: 'error' | 'success', msg: string) {
    const config = {
      content: (
        <div
          className="max-h-[260px] overflow-y-auto"
          // 后端已处理 XSS，此处与 Vue 侧一致直接注入
          dangerouslySetInnerHTML={{ __html: msg }}
        />
      ),
      title: '提示',
    };
    const modal = getMessageApi()?.modal;
    if (modal) {
      if (type === 'success') {
        modal.confirm({ ...config, okCancel: false });
      } else {
        modal.error(config);
      }
    } else if (type === 'success') {
      Modal.success(config);
    } else {
      Modal.error(config);
    }
  }

  async function handleSubmit() {
    try {
      modalApi.modalLoading(true);
      if (fileList.length !== 1) {
        handleCancel();
        return;
      }
      const data = {
        file: fileList[0]!.originFileObj as Blob,
        updateSupport: checked,
      };
      const { code, msg } = await userImportData(data);
      if (code === 200) {
        onReload();
        handleCancel();
        showResult('success', msg);
      } else {
        handleCancel();
        showResult('error', msg);
      }
    } catch (error) {
      console.warn(error);
      void modalApi.close();
    } finally {
      modalApi.modalLoading(false);
    }
  }

  return (
    <BasicModal title="用户导入">
      {/* 手动处理，而不是放入文件就上传 */}
      <Dragger
        accept="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet, application/vnd.ms-excel"
        beforeUpload={() => false}
        fileList={fileList}
        maxCount={1}
        onChange={(info) => setFileList(info.fileList)}
        showUploadList
      >
        <p className="ant-upload-drag-icon flex items-center justify-center">
          <InboxOutlined className="size-[48px] text-primary" />
        </p>
        <p className="ant-upload-text">点击或者拖拽到此处上传文件</p>
      </Dragger>
      <div className="mt-2 flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <span>允许导入xlsx, xls文件</span>
          <Button
            disabled={exportLoading}
            loading={exportLoading}
            onClick={handleExport}
            type="link"
          >
            <span className="flex items-center gap-[4px]">下载模板</span>
          </Button>
        </div>
        <div className="flex items-center gap-2">
          <span className={checked ? 'text-red-500' : undefined}>
            是否更新/覆盖已存在的用户数据
          </span>
          <Switch checked={checked} onChange={setChecked} />
        </div>
      </div>
    </BasicModal>
  );
});

export default UserImportModal;

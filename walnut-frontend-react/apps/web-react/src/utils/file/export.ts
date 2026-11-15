import { useState } from 'react';

import { $t } from '@vben/locales';
import dayjs from 'dayjs';

export interface ExportBlobFuncOptions {
  data?: any;
  fileName: string;
}

type ExportBlobApi = (data: any) => Promise<Blob>;

/** 通过 Blob 触发浏览器下载（对应 Vue 侧 file/download.ts 的 downloadByData） */
function downloadByData(blob: Blob, fileName: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = fileName;
  document.body.append(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

/**
 * 导出 hook（对应 Vue 侧 utils/file/export.tsx 的 useBlobExport）。
 * PoC 简化：去掉 alova useRequest 的 abort/loading 悬浮按钮，保留核心的
 * 「调导出接口 → 下载 blob → 文件名构建」链路。
 */
export function useBlobExport(api: ExportBlobApi) {
  const [exportLoading, setExportLoading] = useState(false);

  async function exportBlob(options: ExportBlobFuncOptions) {
    const { data, fileName } = options;
    setExportLoading(true);
    try {
      const blob = await api(data ?? {});
      downloadByData(blob, fileName);
    } catch (error) {
      console.error(error);
    } finally {
      setExportLoading(false);
    }
  }

  function buildExportFileName(module: string, extension = 'xlsx') {
    return `${module} ${$t('common.export.title')} - ${dayjs().valueOf()}.${extension}`;
  }

  return { buildExportFileName, exportBlob, exportLoading };
}

import type { AxiosRequestConfig } from '@/core/request';

import { request } from '@/lib/request';
import { ContentTypeEnum } from '@/api/helper';

/**
 * Axios上传进度事件
 */
export type AxiosProgressEvent = AxiosRequestConfig['onUploadProgress'];

/**
 * 上传结果 与后端 app/common/dataclasses.py UploadResult 对应
 */
export interface UploadResult {
  /** 原始文件名 */
  original_filename?: string;
  /** 文件访问地址 */
  url: string;
}

/**
 * 通用文件上传接口
 * 后端端点: POST /common/file/upload (multipart 字段名 file)
 * @param file 上传的文件
 * @param options 一些配置项
 * @param options.otherData 其他请求参数 后端拓展可能会用到
 * @returns 上传结果
 */
export function uploadApi(
  file: Blob | File,
  options?: {
    otherData?: Record<string, any>;
  },
) {
  const { otherData = {} } = options ?? {};
  return request.post<UploadResult>(
    '/common/file/upload',
    { file, ...otherData },
    {
      timeout: 60_000,
      headers: {
        'Content-Type': ContentTypeEnum.FORM_DATA,
      },
    },
  );
}

/**
 * 上传api type
 */
export type UploadApi = typeof uploadApi;

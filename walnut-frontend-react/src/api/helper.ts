import { request } from '@/lib/request';

export const ContentTypeEnum = {
  FORM_DATA: 'multipart/form-data;charset=UTF-8',
  FORM_URLENCODED: 'application/x-www-form-urlencoded;charset=UTF-8',
  JSON: 'application/json;charset=UTF-8',
} as const;

/**
 * 通用下载接口封装（对应 Vue 侧 api/helper.ts 的 commonExport）。
 * @returns blob 二进制
 */
export function commonExport(url: string, data: Record<string, any>) {
  return request.post<Blob>(url, data, {
    headers: { 'Content-Type': ContentTypeEnum.FORM_URLENCODED },
    isTransformResponse: false,
    responseType: 'blob',
  });
}

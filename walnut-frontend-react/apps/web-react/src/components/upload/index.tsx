import { Button, Upload } from "antd";
import { useEffect, useRef } from "react";
import type { UploadFile } from "antd";
import { uploadApi } from "@/api/core/upload";
import { getMessageApi } from "@/services/message";
function FileControl({
  value = "",
  onChange,
  image = false,
  maxCount = 5,
  maxSize = 10,
  disabled,
  accept,
}: {
  value?: string | string[];
  onChange?: (value: string | string[]) => void;
  image?: boolean;
  maxCount?: number;
  maxSize?: number;
  disabled?: boolean;
  accept?: string;
}) {
  const urls = Array.isArray(value) ? value : value ? value.split(",") : [];
  const latestUrls = useRef(urls);
  useEffect(() => {
    latestUrls.current = urls;
  }, [value]);
  const files: UploadFile[] = urls.map((url) => ({
    uid: url,
    name: decodeURIComponent(url.split("/").at(-1) ?? "file"),
    status: "done",
    url,
  }));
  function update(urls: string[]) {
    latestUrls.current = urls;
    onChange?.(Array.isArray(value) ? urls : urls.join(","));
  }
  return (
    <Upload
      disabled={disabled}
      accept={accept ?? (image ? "image/*" : undefined)}
      listType={image ? "picture-card" : "text"}
      maxCount={maxCount}
      fileList={files}
      beforeUpload={(file) => {
        if (
          file.size > maxSize * 1024 * 1024 ||
          (image && !file.type.startsWith("image/"))
        ) {
          getMessageApi()?.message.error(
            `请选择 ${maxSize}MB 以内${image ? "的图片" : "的文件"}`,
          );
          return Upload.LIST_IGNORE;
        }
        return true;
      }}
      customRequest={async ({ file, onSuccess, onError }) => {
        try {
          const result = await uploadApi(file as File);
          update([...latestUrls.current, result.url].slice(-maxCount));
          onSuccess?.(result);
        } catch (error) {
          onError?.(error as Error);
        }
      }}
      onRemove={(file) => {
        update(latestUrls.current.filter((url) => url !== file.url));
        return true;
      }}
    >
      {files.length < maxCount ? (
        <Button disabled={disabled}>{image ? "上传图片" : "上传文件"}</Button>
      ) : null}
    </Upload>
  );
}
export function FileUpload(
  props: Omit<Parameters<typeof FileControl>[0], "image">,
) {
  return <FileControl {...props} />;
}
export function ImageUpload(
  props: Omit<Parameters<typeof FileControl>[0], "image">,
) {
  return <FileControl {...props} image />;
}

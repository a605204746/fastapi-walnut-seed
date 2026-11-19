import { useEffect, useState } from 'react';
import Cropper, { type Area } from 'react-easy-crop';
import { Button, Modal, Slider, Space, Upload } from 'antd';
import { RotateCcw, RotateCw } from 'lucide-react';
import { getMessageApi } from '@/services/message';

export function CropperAvatar({
  src,
  upload,
  onSaved,
}: {
  src: string;
  upload: (file: File) => Promise<unknown>;
  onSaved?: () => void;
}) {
  const [image, setImage] = useState<string | null>(null);
  const [crop, setCrop] = useState({ x: 0, y: 0 });
  const [zoom, setZoom] = useState(1);
  const [rotation, setRotation] = useState(0);
  const [area, setArea] = useState<Area | null>(null);
  const [saving, setSaving] = useState(false);
  useEffect(
    () => () => {
      if (image) URL.revokeObjectURL(image);
    },
    [image],
  );
  function reset() {
    setCrop({ x: 0, y: 0 });
    setZoom(1);
    setRotation(0);
  }
  async function save() {
    if (!image || !area) return;
    setSaving(true);
    try {
      const original = new Image();
      original.src = image;
      await original.decode();
      const radians = (rotation * Math.PI) / 180;
      const canvas = document.createElement('canvas');
      canvas.width =
        Math.abs(Math.cos(radians) * original.width) +
        Math.abs(Math.sin(radians) * original.height);
      canvas.height =
        Math.abs(Math.sin(radians) * original.width) +
        Math.abs(Math.cos(radians) * original.height);
      const context = canvas.getContext('2d')!;
      context.translate(canvas.width / 2, canvas.height / 2);
      context.rotate(radians);
      context.drawImage(original, -original.width / 2, -original.height / 2);
      const output = document.createElement('canvas');
      output.width = 400;
      output.height = 400;
      output
        .getContext('2d')!
        .drawImage(canvas, area.x, area.y, area.width, area.height, 0, 0, 400, 400);
      const blob = await new Promise<Blob>((resolve, reject) =>
        output.toBlob(
          (blob) => (blob ? resolve(blob) : reject(new Error('裁剪失败'))),
          'image/png',
        ),
      );
      await upload(new File([blob], 'avatar.png', { type: 'image/png' }));
      setImage(null);
      onSaved?.();
    } finally {
      setSaving(false);
    }
  }
  return (
    <>
      <Upload
        accept="image/*"
        showUploadList={false}
        beforeUpload={(file) => {
          if (!file.type.startsWith('image/') || file.size > 10 * 1024 * 1024) {
            getMessageApi()?.message.error('请选择 10MB 以内的图片');
            return false;
          }
          reset();
          setImage(URL.createObjectURL(file));
          return false;
        }}
      >
        <button type="button" aria-label="上传并裁剪头像">
          <img alt="头像" className="size-[120px] rounded-full object-cover" src={src} />
        </button>
      </Upload>
      <Modal
        open={image !== null}
        title="裁剪头像"
        width={650}
        onCancel={() => !saving && setImage(null)}
        onOk={() => void save()}
        confirmLoading={saving}
        okButtonProps={{ disabled: !area }}
      >
        <div className="relative h-80 bg-black">
          {image && (
            <Cropper
              image={image}
              crop={crop}
              zoom={zoom}
              rotation={rotation}
              aspect={1}
              cropShape="round"
              showGrid
              onCropChange={setCrop}
              onZoomChange={setZoom}
              onCropComplete={(_, pixels) => setArea(pixels)}
            />
          )}
        </div>
        <Slider aria-label="缩放" min={1} max={3} step={0.1} value={zoom} onChange={setZoom} />
        <Space>
          <Button
            aria-label="向左旋转"
            icon={<RotateCcw className="size-4" />}
            onClick={() => setRotation(rotation - 90)}
          />
          <Button
            aria-label="向右旋转"
            icon={<RotateCw className="size-4" />}
            onClick={() => setRotation(rotation + 90)}
          />
          <Button onClick={reset}>重置</Button>
        </Space>
      </Modal>
    </>
  );
}

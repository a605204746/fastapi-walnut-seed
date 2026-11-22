import { useEffect, useState } from 'react';
import { Form, Input, Modal, Radio, Spin } from 'antd';
import { DictEnum } from '@walnut/constants';
import { noticeAdd, noticeInfo, noticeUpdate } from '@/api/system/notice';
import { Tiptap } from '@/components/tiptap';
import { useDirtyClose } from '@/hooks/use-dirty-close';
import { useDictStore } from '@/store/dict';
import { getDictOptions } from '@/utils/dict';

export function NoticeEditor({
  id,
  onClose,
  onSaved,
}: {
  id?: number | string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  useDictStore((s) => s.map);
  const dirty = useDirtyClose(() => form.getFieldsValue(true));
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const record = id
          ? await noticeInfo(id)
          : { noticeTitle: '', noticeType: '1', status: '0', noticeContent: '' };
        if (!cancelled) {
          form.setFieldsValue(record);
          dirty.mark();
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [id, form]);
  async function save() {
    let values;
    try {
      values = await form.validateFields();
    } catch {
      return;
    }
    setSaving(true);
    try {
      await (id ? noticeUpdate({ ...values, id }) : noticeAdd(values));
      onSaved();
      onClose();
    } catch {
      /* Keep the draft; request errors are displayed by the request client. */
    } finally {
      setSaving(false);
    }
  }
  return (
    <Modal
      open
      width={fullscreen ? '100vw' : 800}
      style={fullscreen ? { top: 0, paddingBottom: 0 } : undefined}
      title={
        <span>
          {id ? '编辑' : '新增'}
          <button
            type="button"
            className="ml-4 text-sm text-primary"
            onClick={() => setFullscreen(!fullscreen)}
          >
            {fullscreen ? '还原' : '全屏'}
          </button>
        </span>
      }
      onCancel={() => dirty.close(onClose)}
      onOk={() => void save()}
      okButtonProps={{ disabled: loading }}
      confirmLoading={saving}
      mask={{ closable: !saving }}
    >
      <Spin spinning={loading}>
        <Form form={form} layout="vertical">
          <Form.Item label="公告标题" name="noticeTitle" rules={[{ required: true }]}>
            <Input maxLength={50} />
          </Form.Item>
          <div className="grid grid-cols-1 lg:grid-cols-2">
            <Form.Item label="公告状态" name="status" rules={[{ required: true }]}>
              <Radio.Group
                optionType="button"
                buttonStyle="solid"
                options={getDictOptions(DictEnum.SYS_NOTICE_STATUS)}
              />
            </Form.Item>
            <Form.Item label="公告类型" name="noticeType" rules={[{ required: true }]}>
              <Radio.Group
                optionType="button"
                buttonStyle="solid"
                options={getDictOptions(DictEnum.SYS_NOTICE_TYPE)}
              />
            </Form.Item>
          </div>
          <Form.Item
            label="公告内容"
            name="noticeContent"
            rules={[
              {
                validator: (_, value) =>
                  /<(img|hr)\b/i.test(value ?? '') ||
                  (value ?? '')
                    .replace(/<[^>]*>/g, '')
                    .replace(/&nbsp;/g, '')
                    .trim()
                    ? Promise.resolve()
                    : Promise.reject(new Error('请输入公告内容')),
              },
            ]}
          >
            <Tiptap />
          </Form.Item>
        </Form>
      </Spin>
    </Modal>
  );
}

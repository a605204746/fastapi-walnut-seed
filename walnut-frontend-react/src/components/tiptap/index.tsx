import { useEffect, useRef, useState } from 'react';
import { EditorContent, useEditor, useEditorState } from '@tiptap/react';
import StarterKit from '@tiptap/starter-kit';
import Highlight from '@tiptap/extension-highlight';
import TextAlign from '@tiptap/extension-text-align';
import Placeholder from '@tiptap/extension-placeholder';
import { Button, ColorPicker, Input, Modal, Select, Space, Tooltip, Upload } from 'antd';
import { uploadApi } from '@/api/core/upload';
import { getMessageApi } from '@/services/message';
import { ResizableImage } from './resizable-image';
import { TextStyle } from './text-style';
import './tiptap.css';

/** HTML remains the persistence format, including image width and data-oss-id. */
export function Tiptap({
  value = '',
  onChange,
  disabled = false,
  placeholder = '请输入内容',
}: {
  value?: string;
  onChange?: (html: string) => void;
  disabled?: boolean;
  placeholder?: string;
}) {
  const callback = useRef(onChange);
  callback.current = onChange;
  const [source, setSource] = useState<string | null>(null);
  const [link, setLink] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: { levels: [1, 2, 3] },
        link: {
          openOnClick: false,
          defaultProtocol: 'https',
          HTMLAttributes: { rel: 'noopener noreferrer nofollow', target: '_blank' },
        },
      }),
      Highlight,
      TextStyle,
      ResizableImage.configure({ allowBase64: false }),
      TextAlign.configure({ types: ['heading', 'paragraph'] }),
      Placeholder.configure({ placeholder }),
    ],
    content: value,
    editable: !disabled,
    editorProps: {
      attributes: { 'aria-label': '公告内容编辑器', role: 'textbox', 'aria-multiline': 'true' },
    },
    onUpdate: ({ editor }) => callback.current?.(editor.isEmpty ? '' : editor.getHTML()),
  });
  useEditorState({ editor, selector: ({ editor }) => (editor ? editor.state : null) });
  useEffect(() => {
    if (editor && value !== editor.getHTML() && !(editor.isEmpty && !value))
      editor.commands.setContent(value, { emitUpdate: false });
  }, [editor, value]);
  useEffect(() => {
    editor?.setEditable(!disabled);
  }, [editor, disabled]);
  if (!editor) return null;
  function action(title: string, text: string, run: () => void, active = false, enabled = true) {
    return (
      <Tooltip title={title}>
        <Button
          size="small"
          aria-label={title}
          type={active ? 'primary' : 'text'}
          disabled={disabled || !enabled}
          onMouseDown={(e) => e.preventDefault()}
          onClick={run}
        >
          {text}
        </Button>
      </Tooltip>
    );
  }
  return (
    <div className="tiptap-editor border rounded-md overflow-hidden">
      <div className="tiptap-toolbar flex flex-wrap items-center gap-1 border-b bg-accent p-2">
        <Select
          aria-label="段落格式"
          size="small"
          value={[1, 2, 3].find((level) => editor.isActive('heading', { level })) ?? 0}
          style={{ width: 90 }}
          options={[
            { label: '正文', value: 0 },
            ...[1, 2, 3].map((level) => ({ label: `标题 ${level}`, value: level })),
          ]}
          onChange={(level) =>
            level
              ? editor
                  .chain()
                  .focus()
                  .toggleHeading({ level: level as 1 | 2 | 3 })
                  .run()
              : editor.chain().focus().setParagraph().run()
          }
        />
        {action(
          '粗体',
          'B',
          () => editor.chain().focus().toggleBold().run(),
          editor.isActive('bold'),
        )}
        {action(
          '斜体',
          'I',
          () => editor.chain().focus().toggleItalic().run(),
          editor.isActive('italic'),
        )}
        {action(
          '下划线',
          'U',
          () => editor.chain().focus().toggleUnderline().run(),
          editor.isActive('underline'),
        )}
        {action(
          '删除线',
          'S',
          () => editor.chain().focus().toggleStrike().run(),
          editor.isActive('strike'),
        )}
        {action(
          '高亮',
          '▰',
          () => editor.chain().focus().toggleHighlight().run(),
          editor.isActive('highlight'),
        )}
        <ColorPicker
          size="small"
          aria-label="文字颜色"
          onChangeComplete={(color) =>
            editor.chain().focus().setMark('textStyle', { color: color.toHexString() }).run()
          }
        />
        {action(
          '无序列表',
          '• ≡',
          () => editor.chain().focus().toggleBulletList().run(),
          editor.isActive('bulletList'),
        )}
        {action(
          '有序列表',
          '1. ≡',
          () => editor.chain().focus().toggleOrderedList().run(),
          editor.isActive('orderedList'),
        )}
        {action(
          '引用',
          '❞',
          () => editor.chain().focus().toggleBlockquote().run(),
          editor.isActive('blockquote'),
        )}
        {action(
          '代码块',
          '</>',
          () => editor.chain().focus().toggleCodeBlock().run(),
          editor.isActive('codeBlock'),
        )}
        {(['left', 'center', 'right', 'justify'] as const).map((align, i) => (
          <span key={align}>
            {action(
              ['左对齐', '居中', '右对齐', '两端对齐'][i]!,
              ['☰', '≡', '☰', '▤'][i]!,
              () => editor.chain().focus().setTextAlign(align).run(),
              editor.isActive({ textAlign: align }),
            )}
          </span>
        ))}
        {action(
          '链接',
          '↗',
          () => setLink(editor.getAttributes('link').href ?? ''),
          editor.isActive('link'),
        )}
        <Upload
          showUploadList={false}
          accept="image/*"
          disabled={disabled || uploading}
          beforeUpload={async (file) => {
            if (!file.type.startsWith('image/') || file.size > 10 * 1024 * 1024) {
              getMessageApi()?.message.error('请选择 10MB 以内的图片');
              return false;
            }
            setUploading(true);
            try {
              const result = await uploadApi(file);
              editor.chain().focus().setImage({ src: result.url, alt: file.name }).run();
            } finally {
              setUploading(false);
            }
            return false;
          }}
        >
          <Button size="small" type="text" loading={uploading} aria-label="上传图片">
            图片
          </Button>
        </Upload>
        {action('水平线', '—', () => editor.chain().focus().setHorizontalRule().run())}
        {action('清除格式', 'Tx', () => editor.chain().focus().unsetAllMarks().clearNodes().run())}
        {action('撤销', '↶', () => editor.chain().focus().undo().run(), false, editor.can().undo())}
        {action('重做', '↷', () => editor.chain().focus().redo().run(), false, editor.can().redo())}
        {action('HTML源码', 'HTML', () => setSource(editor.getHTML()))}
      </div>
      <EditorContent editor={editor} />
      <div className="border-t px-3 py-1 text-right text-xs text-muted-foreground">
        {editor.getText().length} 字
      </div>
      <Modal
        open={source !== null}
        title="HTML源码"
        onCancel={() => setSource(null)}
        onOk={() => {
          if (source !== null) {
            editor.commands.setContent(source);
            callback.current?.(editor.getHTML());
          }
          setSource(null);
        }}
      >
        <Input.TextArea
          aria-label="HTML源码"
          value={source ?? ''}
          rows={14}
          onChange={(e) => setSource(e.target.value)}
        />
      </Modal>
      <Modal
        open={link !== null}
        title="编辑链接"
        onCancel={() => setLink(null)}
        onOk={() => {
          if (link && !/^(https?:|mailto:|tel:|\/|#)/i.test(link)) {
            getMessageApi()?.message.error('请输入有效的链接');
            return;
          }
          if (link) editor.chain().focus().extendMarkRange('link').setLink({ href: link }).run();
          else editor.chain().focus().unsetLink().run();
          setLink(null);
        }}
      >
        <Input
          aria-label="链接地址"
          value={link ?? ''}
          onChange={(e) => setLink(e.target.value)}
          placeholder="https://"
        />
      </Modal>
    </div>
  );
}

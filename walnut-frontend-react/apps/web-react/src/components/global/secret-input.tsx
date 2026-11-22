import { buildUUID } from '@walnut/utils';
import { ReloadOutlined } from '@ant-design/icons';
import { Button, Input, Space } from 'antd';

export interface SecretInputProps {
  disabled?: boolean;
  /** 受控值（React 统一 value + onChange） */
  onChange?: (value: string) => void;
  placeholder?: string;
  value?: string;
}

/**
 * 密钥输入框（对应 Vue 侧 views/system/client/secret-input.vue）。
 *
 * 输入框 + 「随机生成」按钮，点击按钮用 `buildUUID()` 填一个随机密钥。
 *
 * 与 Vue 版的差异：
 * - `defineModel('value')` → 受控 `value` + `onChange`，因此可以直接注册进
 *   表单组件表当作 `component: 'SecretInput'` 用。Vue 侧是通过
 *   `<BasicForm><template #clientSecret>` **表单插槽**注入的，而 React 版
 *   form-ui 没有字段级插槽机制；注册成控件是等价且更简单的做法。
 * - `defineExpose({ refreshSecret })` 未移植：Vue 侧注释说明它只是「万一要在
 *   每次新增时打开 Drawer 刷新」的备用口，实际调用点为零。
 * - `Iconify(charm--refresh)` 类名图标 → antd `ReloadOutlined`（两套 tailwind
 *   配置都没装 iconify class 插件，类名会渲染成空）。
 * - Vue 的 scoped scss（去掉 addon 内边距、按钮左侧直角）改用 `Space.Compact`
 *   的原生贴合效果，视觉差异列入 B 阶段像素补齐。
 */
export function SecretInput({
  disabled = false,
  onChange,
  placeholder = '请输入密钥或随机生成',
  value,
}: SecretInputProps) {
  function refreshSecret() {
    onChange?.(buildUUID());
  }

  return (
    <Space.Compact className="w-full">
      <Input
        disabled={disabled}
        name="clientSecret"
        onChange={(e) => onChange?.(e.target.value)}
        placeholder={placeholder}
        value={value}
      />
      {disabled ? null : (
        <Button onClick={refreshSecret} type="primary">
          <span className="flex items-center gap-[4px]">
            <ReloadOutlined />
            {/* Vue 侧此处是硬编码中文「随机生成」，词典里没有对应键，保持一致 */}
            <span>随机生成</span>
          </span>
        </Button>
      )}
    </Space.Compact>
  );
}

export default SecretInput;

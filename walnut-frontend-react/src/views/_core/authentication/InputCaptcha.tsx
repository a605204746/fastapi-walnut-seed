import { Input, Space } from 'antd';

import { $t } from '@/locales';

import './InputCaptcha.css';

export interface InputCaptchaProps {
  captcha?: string;
  label?: string;
  loading?: boolean;
  onChange?: (value: string) => void;
  onCaptchaClick?: () => void;
  placeholder?: string;
  value?: string;
}

/**
 * 图片验证码输入框（对应 Vue 侧 views/_core/authentication/input-captcha.vue）。
 * 作为表单字段组件，接收 value/onChange（受控）。
 */
export function InputCaptcha({
  captcha = '',
  loading = false,
  onChange,
  onCaptchaClick,
  placeholder = $t('authentication.code'),
  value = '',
}: InputCaptchaProps) {
  return (
    <div className="w-full flex-1">
      <Space.Compact className="w-full">
        <Input
          autoComplete="off"
          id="code"
          name="code"
          onChange={(e) => onChange?.(e.target.value)}
          placeholder={placeholder}
          required
          size="large"
          type="text"
          value={value}
        />
        <div className="relative rounded-r-lg border">
          <img
            className={`h-[40px] w-[150px] cursor-pointer rounded-r-lg ${
              loading ? 'pointer-events-none' : ''
            }`}
            onClick={onCaptchaClick}
            src={captcha}
          />
          {loading ? (
            <div className="absolute inset-0 flex cursor-not-allowed items-center justify-center rounded-r-lg bg-black/30">
              <span className="captcha-loading"></span>
            </div>
          ) : null}
        </div>
      </Space.Compact>
    </div>
  );
}

export default InputCaptcha;

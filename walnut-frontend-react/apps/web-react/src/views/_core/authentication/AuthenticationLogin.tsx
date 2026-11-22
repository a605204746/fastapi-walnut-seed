import type { ReactNode } from 'react';

import type { FormApi, FormSchema } from '@walnut-core/form-ui';

import { forwardRef, useCallback, useImperativeHandle, useState } from 'react';

import { $t } from '@walnut/locales';
import { useWalnutForm } from '@walnut-core/form-ui';
import { cn } from '@walnut-core/shared/utils';
import { Button, Checkbox } from 'antd';
import { useNavigate } from 'react-router';

export interface AuthenticationLoginProps {
  codeLoginPath?: string;
  formSchema: FormSchema[];
  forgetPasswordPath?: string;
  loading?: boolean;
  onSubmit: (values: Record<string, any>) => void | Promise<void>;
  qrCodeLoginPath?: string;
  showCodeLogin?: boolean;
  showForgetPassword?: boolean;
  showQrcodeLogin?: boolean;
  showRememberMe?: boolean;
  showThirdPartyLogin?: boolean;
  submitButtonText?: string;
  submitDisabled?: boolean;
  subTitle?: string;
  thirdPartyLogin?: ReactNode;
  title?: string;
}

export interface AuthenticationLoginHandle {
  getFormApi: () => FormApi;
}

/** 标题区（对应 common-ui authentication/auth-title.vue） */
function AuthTitle({ desc, title }: { desc?: ReactNode; title?: ReactNode }) {
  return (
    <div className="mb-7 sm:mx-auto sm:w-full sm:max-w-md">
      <h2 className="mb-3 text-3xl/9 font-bold tracking-tight text-foreground lg:text-4xl">
        {title}
      </h2>
      <p className="lg:text-md text-sm text-muted-foreground">{desc}</p>
    </div>
  );
}

const REMEMBER_ME_KEY = `REMEMBER_ME_USERNAME_${location.hostname}`;

/**
 * 登录表单（对应 common-ui authentication/login.vue 的 AuthenticationLogin）。
 * 表单控件按 app 层配置使用 antd（Button/Checkbox），字段由 formSchema 驱动。
 */
export const AuthenticationLogin = forwardRef<
  AuthenticationLoginHandle,
  AuthenticationLoginProps
>(function AuthenticationLogin(
  {
    codeLoginPath = '/auth/code-login',
    formSchema,
    forgetPasswordPath = '/auth/forget-password',
    loading = false,
    onSubmit,
    qrCodeLoginPath = '/auth/qrcode-login',
    showCodeLogin = true,
    showForgetPassword = true,
    showQrcodeLogin = true,
    showRememberMe = true,
    showThirdPartyLogin = true,
    submitButtonText = '',
    submitDisabled = false,
    subTitle = '',
    thirdPartyLogin,
    title = '',
  },
  ref,
) {
  const navigate = useNavigate();
  const [Form, formApi] = useWalnutForm({
    commonConfig: { hideLabel: true, hideRequiredMark: true },
    schema: formSchema,
    showDefaultActions: false,
  });

  useImperativeHandle(ref, () => ({ getFormApi: () => formApi }), [formApi]);

  const localUsername = localStorage.getItem(REMEMBER_ME_KEY) || '';
  const [rememberMe, setRememberMe] = useState(!!localUsername);

  const handleSubmit = useCallback(async () => {
    const { valid } = await formApi.validate();
    if (!valid) return;
    const values = await formApi.getValues();
    localStorage.setItem(REMEMBER_ME_KEY, rememberMe ? (values.username ?? '') : '');
    values.grantType = 'password';
    await onSubmit(values);
  }, [formApi, onSubmit, rememberMe]);

  return (
    <div onKeyDown={(e) => e.key === 'Enter' && void handleSubmit()}>
      <AuthTitle
        desc={
          <span className="text-muted-foreground">
            {subTitle || $t('authentication.loginSubtitle')}
          </span>
        }
        title={title || `${$t('authentication.welcomeBack')} 👋🏻`}
      />

      <Form class="mb-2" />

      {showRememberMe || showForgetPassword ? (
        <div className="mb-6 flex justify-between">
          <div className="flex-center">
            {showRememberMe ? (
              <Checkbox
                checked={rememberMe}
                name="rememberMe"
                onChange={(e) => setRememberMe(e.target.checked)}
              >
                {$t('authentication.rememberMe')}
              </Checkbox>
            ) : null}
          </div>
          {showForgetPassword ? (
            <span
              className="walnut-link text-sm font-normal"
              onClick={() => navigate(forgetPasswordPath)}
            >
              {$t('authentication.forgetPassword')}
            </span>
          ) : null}
        </div>
      ) : null}

      <Button
        aria-label="login"
        block
        className={cn('h-10 w-full', loading && 'cursor-wait')}
        disabled={submitDisabled}
        loading={loading}
        onClick={() => void handleSubmit()}
        size="large"
        type="primary"
      >
        {submitButtonText || $t('common.login')}
      </Button>

      {showCodeLogin || showQrcodeLogin ? (
        <div className="mt-4 mb-2 flex items-center justify-between">
          {showCodeLogin ? (
            <Button
              className="w-1/2"
              onClick={() => navigate(codeLoginPath)}
              size="large"
            >
              {$t('authentication.mobileLogin')}
            </Button>
          ) : null}
          {showQrcodeLogin ? (
            <Button
              className="ml-4 w-1/2"
              onClick={() => navigate(qrCodeLoginPath)}
              size="large"
            >
              {$t('authentication.qrcodeLogin')}
            </Button>
          ) : null}
        </div>
      ) : null}

      {showThirdPartyLogin ? thirdPartyLogin : null}
    </div>
  );
});

export default AuthenticationLogin;

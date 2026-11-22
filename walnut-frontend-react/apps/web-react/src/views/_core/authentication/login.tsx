import type { FormSchema } from '@walnut-core/form-ui';

import type { AuthenticationLoginHandle } from './AuthenticationLogin';

import { useEffect, useMemo, useRef, useState } from 'react';

import type { CaptchaResponse } from '@/api/core/captcha';
import type { TenantResp } from '@/api/core/auth';

import { DEFAULT_TENANT_ID } from '@walnut/constants';
import { $t } from '@walnut/locales';
import { useUserStore } from '@walnut/stores';
import { getPreferences } from '@walnut/preferences';
import { Input, Select } from 'antd';
import { useNavigate, useSearchParams } from 'react-router';

import { tenantList } from '@/api/core/auth';
import { captchaImage } from '@/api/core/captcha';
import { appConfig } from '@/lib/config';
import { useAuthStore } from '@/store/auth';

import { AuthenticationLogin } from './AuthenticationLogin';
import { InputCaptcha } from './InputCaptcha';
import { OAuthLogin } from './OAuthLogin';

/**
 * 登录页（对应 Vue 侧 apps/web-antd/src/views/_core/authentication/login.vue）。
 * formSchema 使用 antd 控件（Select/Input/Input.Password）+ 自定义 InputCaptcha。
 */
export function LoginPage({ embedded = false, onSuccess }: { embedded?: boolean; onSuccess?: () => void } = {}) {
  const currentUsername = useUserStore(s => s.userInfo?.username);
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const loginRef = useRef<AuthenticationLoginHandle>(null);
  const loginLoading = useAuthStore((state) => state.loginLoading);
  const authLogin = useAuthStore((state) => state.authLogin);

  const [captchaInfo, setCaptchaInfo] = useState<CaptchaResponse>({
    captchaEnabled: false,
    img: '',
    uuid: '',
  });
  const [captchaLoading, setCaptchaLoading] = useState(false);
  const [tenantInfo, setTenantInfo] = useState<TenantResp>({
    tenantEnabled: false,
    voList: [],
  });
  const [tenantLoading, setTenantLoading] = useState(false);

  async function loadCaptcha() {
    const timer = setTimeout(() => setCaptchaLoading(true), 300);
    try {
      const resp = await captchaImage();
      if (resp.captchaEnabled) {
        resp.img = `data:image/png;base64,${resp.img}`;
      }
      setCaptchaInfo(resp);
    } catch (error) {
      console.error(error);
    } finally {
      setCaptchaLoading(false);
      clearTimeout(timer);
    }
  }

  async function loadTenant() {
    setTenantLoading(true);
    try {
      const resp = await tenantList();
      setTenantInfo(resp);
      if (resp.tenantEnabled && resp.voList.length > 0 && resp.voList[0]) {
        loginRef.current
          ?.getFormApi()
          .setFieldValue('tenantId', resp.voList[0].tenantId);
      }
    } finally {
      setTenantLoading(false);
    }
  }

  useEffect(() => {
    void loadCaptcha();
    void loadTenant();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const formSchema = useMemo<FormSchema[]>(
    () => [
      {
        component: Select,
        componentProps: {
          allowClear: true,
          className: 'w-full',
          optionFilterProp: 'label',
          options: tenantInfo.voList?.map((item) => ({
            label: item.companyName,
            value: item.tenantId,
          })),
          placeholder: $t('ui.formRules.selectRequired'),
          showSearch: true,
          size: 'large',
        },
        defaultValue: DEFAULT_TENANT_ID,
        dependencies: {
          if: () => tenantInfo.tenantEnabled,
          triggerFields: ['tenantId'],
        },
        fieldName: 'tenantId',
        label: $t('authentication.selectAccount'),
        modelPropName: 'value',
        rules: { message: $t('authentication.selectAccount'), required: true },
      },
      {
        component: Input,
        componentProps: {
          allowClear: true,
          placeholder: $t('authentication.usernameTip'),
          size: 'large',
        },
        defaultValue: embedded ? currentUsername : 'admin',
        fieldName: 'username',
        label: $t('authentication.username'),
        modelPropName: 'value',
        rules: { message: $t('authentication.usernameTip'), required: true },
      },
      {
        component: Input.Password,
        componentProps: {
          placeholder: $t('authentication.passwordTip'),
          size: 'large',
        },
        defaultValue: embedded ? '' : 'admin123',
        fieldName: 'password',
        label: $t('authentication.password'),
        modelPropName: 'value',
        rules: [
          { message: $t('authentication.passwordTip'), required: true },
          { min: 5, message: $t('authentication.passwordTip') },
        ],
      },
      {
        component: InputCaptcha,
        componentProps: {
          captcha: captchaInfo.img,
          loading: captchaLoading,
          onCaptchaClick: loadCaptcha,
          placeholder: $t('authentication.code'),
        },
        dependencies: {
          if: () => captchaInfo.captchaEnabled,
          triggerFields: ['code'],
        },
        fieldName: 'code',
        label: $t('authentication.code'),
        rules: {
          message: $t('authentication.verifyRequiredTip'),
          required: true,
        },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [tenantInfo, captchaInfo, captchaLoading, embedded, currentUsername],
  );

  async function handleAccountLogin(values: Record<string, any>) {
    try {
      const param: Record<string, any> = {
        clientId: appConfig.clientId,
        grantType: 'password',
        password: values.password,
        tenantId: values.tenantId ?? DEFAULT_TENANT_ID,
        username: values.username,
      };
      if (captchaInfo.captchaEnabled) {
        param.code = values.code;
        param.uuid = captchaInfo.uuid;
      }
      const { userInfo } = await authLogin(param as any);
      if (embedded) { onSuccess?.(); return; }
      const redirect = searchParams.get('redirect');
      // 落地优先级与 Vue 侧 guard.ts 一致：redirect > userInfo.homePath > defaultHomePath
      const homePath =
        (userInfo?.homePath as string) ||
        getPreferences().app.defaultHomePath;
      navigate(redirect ? decodeURIComponent(redirect) : homePath, {
        replace: true,
      });
    } catch (error) {
      console.error(error);
      loginRef.current?.getFormApi().setFieldValue('code', '');
      await loadCaptcha();
    }
  }

  return (
    <AuthenticationLogin
      formSchema={formSchema}
      loading={loginLoading}
      onSubmit={handleAccountLogin}
      ref={loginRef}
      showThirdPartyLogin={!embedded}
      showRememberMe={!embedded}
      showForgetPassword={!embedded}
      showCodeLogin={!embedded}
      showQrcodeLogin={!embedded}
      submitDisabled={captchaLoading || tenantLoading}
      thirdPartyLogin={<OAuthLogin />}
    />
  );
}

export default LoginPage;

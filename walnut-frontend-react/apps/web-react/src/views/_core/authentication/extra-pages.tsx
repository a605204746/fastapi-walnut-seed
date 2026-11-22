import { useEffect, useState } from 'react';
import { Alert, Button, Checkbox, Form, Input, QRCode, Select } from 'antd';
import { Link, useNavigate } from 'react-router';
import { DEFAULT_TENANT_ID } from '@walnut/constants';
import { getPreferences } from '@walnut/preferences';
import { tenantList, type TenantResp } from '@/api/core/auth';
import { sendSmsCode } from '@/api/core/captcha';
import { useAuthStore } from '@/store/auth';
import { getMessageApi } from '@/services/message';
import { $t } from '@/locales';

function Back() {
  return (
    <Link className="mt-4 block text-center text-primary" to="/auth/login">
      {$t('common.back')}
    </Link>
  );
}
function Title({ title, description }: { title: string; description: string }) {
  return (
    <div className="mb-6">
      <h1 className="mb-3 text-3xl font-bold">{title}</h1>
      <p className="text-muted-foreground">{description}</p>
    </div>
  );
}
export function RegisterPage() {
  return (
    <div>
      <Title
        title={$t('authentication.register')}
        description={$t('authentication.registerSubtitle')}
      />
      <Alert className="mb-4" type="info" title="此页面沿用上游注册演示，尚未接入注册服务。" />
      <Form
        layout="vertical"
        onFinish={() => getMessageApi()?.message.warning('表单校验通过；请联系管理员开通账号。')}
      >
        <Form.Item
          name="username"
          label={$t('authentication.username')}
          rules={[{ required: true }]}
        >
          <Input size="large" autoComplete="username" />
        </Form.Item>
        <Form.Item
          name="password"
          label={$t('authentication.password')}
          rules={[{ required: true, min: 6 }]}
        >
          <Input.Password size="large" autoComplete="new-password" />
        </Form.Item>
        <Form.Item
          name="confirmPassword"
          dependencies={['password']}
          label={$t('authentication.confirmPassword')}
          rules={[
            { required: true },
            ({ getFieldValue }) => ({
              validator: (_, value) =>
                !value || value === getFieldValue('password')
                  ? Promise.resolve()
                  : Promise.reject(new Error($t('authentication.confirmPasswordTip'))),
            }),
          ]}
        >
          <Input.Password size="large" autoComplete="new-password" />
        </Form.Item>
        <Form.Item
          name="agreePolicy"
          valuePropName="checked"
          rules={[
            {
              validator: (_, value) =>
                value
                  ? Promise.resolve()
                  : Promise.reject(new Error($t('authentication.agreeTip'))),
            },
          ]}
        >
          <Checkbox>
            {$t('authentication.agree')} {$t('authentication.privacyPolicy')} &amp;{' '}
            {$t('authentication.terms')}
          </Checkbox>
        </Form.Item>
        <Button block type="primary" size="large" htmlType="submit">
          {$t('authentication.register')}
        </Button>
      </Form>
      <Back />
    </div>
  );
}
export function ForgetPasswordPage() {
  return (
    <div>
      <Title
        title={$t('authentication.forgetPassword')}
        description={$t('authentication.forgetPasswordSubtitle')}
      />
      <Form
        layout="vertical"
        onFinish={() =>
          getMessageApi()?.message.warning('当前为上游演示流程，请联系管理员重置密码。')
        }
      >
        <Form.Item
          name="email"
          label={$t('authentication.email')}
          rules={[{ required: true, type: 'email' }]}
        >
          <Input size="large" autoComplete="email" placeholder="example@example.com" />
        </Form.Item>
        <Button block type="primary" size="large" htmlType="submit">
          {$t('common.confirm')}
        </Button>
      </Form>
      <Back />
    </div>
  );
}
export function CodeLoginPage() {
  const [form] = Form.useForm();
  const [tenant, setTenant] = useState<TenantResp>({ tenantEnabled: false, voList: [] });
  const [remaining, setRemaining] = useState(0);
  const [sending, setSending] = useState(false);
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();
  useEffect(() => {
    let cancelled = false;
    void tenantList().then((data) => {
      if (!cancelled) {
        setTenant(data);
        form.setFieldValue('tenantId', data.voList[0]?.tenantId ?? DEFAULT_TENANT_ID);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [form]);
  useEffect(() => {
    if (remaining <= 0) return;
    const timer = setTimeout(() => setRemaining(remaining - 1), 1000);
    return () => clearTimeout(timer);
  }, [remaining]);
  async function send() {
    const { phonenumber } = await form.validateFields(['phonenumber']);
    setSending(true);
    try {
      await sendSmsCode(phonenumber);
      setRemaining(60);
    } finally {
      setSending(false);
    }
  }
  async function login(values: Record<string, string>) {
    setLoading(true);
    try {
      await useAuthStore
        .getState()
        .authLogin({
          ...values,
          tenantId: values.tenantId ?? DEFAULT_TENANT_ID,
          grantType: 'sms',
        } as any);
      navigate(getPreferences().app.defaultHomePath, { replace: true });
    } finally {
      setLoading(false);
    }
  }
  return (
    <div>
      <Title
        title={$t('authentication.mobileLogin')}
        description={$t('authentication.loginSubtitle')}
      />
      <Alert className="mb-4" type="info" title="短信登录需要后端启用 sms 授权策略。" />
      <Form
        form={form}
        layout="vertical"
        onFinish={login}
        initialValues={{ tenantId: DEFAULT_TENANT_ID }}
      >
        {tenant.tenantEnabled && (
          <Form.Item name="tenantId" label={$t('authentication.selectAccount')} rules={[{ required: true }]}>
            <Select
              size="large"
              options={tenant.voList.map((item) => ({
                label: item.companyName,
                value: item.tenantId,
              }))}
            />
          </Form.Item>
        )}
        <Form.Item
          name="phonenumber"
          label={$t('authentication.mobile')}
          rules={[
            { required: true, pattern: /^\d{11}$/, message: $t('authentication.mobileErrortip') },
          ]}
        >
          <Input size="large" autoComplete="tel" />
        </Form.Item>
        <div className="flex gap-2">
          <Form.Item name="smsCode" className="flex-1" rules={[{ required: true, len: 4 }]}>
            <Input
              size="large"
              aria-label={$t('authentication.code')}
              autoComplete="one-time-code"
              maxLength={4}
              placeholder={$t('authentication.code')}
            />
          </Form.Item>
          <Button
            size="large"
            disabled={remaining > 0}
            loading={sending}
            onClick={() => void send()}
          >
            {remaining > 0 ? `${remaining}s` : $t('authentication.sendCode')}
          </Button>
        </div>
        <Button block type="primary" size="large" htmlType="submit" loading={loading}>
          {$t('common.login')}
        </Button>
      </Form>
      <Back />
    </div>
  );
}
export function QrcodeLoginPage() {
  return (
    <div>
      <Title
        title={`${$t('authentication.welcomeBack')} 📱`}
        description={$t('authentication.qrcodeSubtitle')}
      />
      <div className="flex flex-col items-center gap-4">
        <QRCode value={window.location.origin} size={200} />
        <p className="text-sm text-muted-foreground">二维码演示，扫码打开当前站点。</p>
      </div>
      <Back />
    </div>
  );
}

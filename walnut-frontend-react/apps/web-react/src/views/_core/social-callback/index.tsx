import { useEffect, useRef, useState } from 'react';
import { Button, Result, Spin } from 'antd';
import { useNavigate, useSearchParams } from 'react-router';
import { DEFAULT_TENANT_ID } from '@vben/constants';
import { getPreferences } from '@vben/preferences';
import { useAccessStore } from '@vben/stores';
import { authCallback } from '@/api/core/auth';
import { useAuthStore } from '@/store/auth';
import { accountBindList } from '../oauth-common';

export default function SocialCallback() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const started = useRef(false);
  const [error, setError] = useState('');
  useEffect(() => {
    if (started.current) return;
    started.current = true;
    void (async () => {
      try {
        const code = params.get('code');
        const state = params.get('state');
        const source = params.get('source');
        if (!code || !state || !accountBindList.some((item) => item.source === source))
          throw new Error('无效的第三方登录回调');
        const details = JSON.parse(atob(state));
        // Reject a forged cross-domain callback rather than sending authorization codes to it.
        if (details.domain && details.domain !== location.host)
          throw new Error('回调域名与当前站点不一致，请从登录页重新授权');
        const data = {
          grantType: 'social' as const,
          socialCode: code,
          socialState: state,
          source: source!,
          tenantId: details.tenantId ?? DEFAULT_TENANT_ID,
        };
        if (useAccessStore.getState().accessToken) await authCallback(data);
        else await useAuthStore.getState().authLogin(data);
        navigate(getPreferences().app.defaultHomePath, { replace: true });
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : '授权失败');
      }
    })();
  }, [params, navigate]);
  return (
    <div className="flex-center min-h-screen">
      {error ? (
        <Result
          status="error"
          title="授权失败"
          subTitle={error}
          extra={<Button onClick={() => navigate('/auth/login')}>返回登录</Button>}
        />
      ) : (
        <Spin size="large" />
      )}
    </div>
  );
}

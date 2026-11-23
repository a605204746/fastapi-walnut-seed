import { Modal } from 'antd';
import { useAccessStore } from '@/core/stores';
import { usePreferences } from '@/core/preferences';
import { $t } from '@/locales';
import { LoginPage } from '@/views/_core/authentication/login';
export function LoginExpiredModal() {
  const expired = useAccessStore((s) => s.loginExpired);
  // 订阅偏好：locale 变化时重译标题
  usePreferences();
  return (
    <Modal
      open={expired}
      title={$t('authentication.loginAgainTitle')}
      footer={null}
      closable={false}
      mask={{ closable: false }}
      keyboard={false}
      width={480}
      destroyOnHidden
    >
      {expired && <LoginPage embedded onSuccess={() => window.location.reload()} />}
    </Modal>
  );
}

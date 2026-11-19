import { Modal } from 'antd';
import { useAccessStore } from '@vben/stores';
import { LoginPage } from '@/views/_core/authentication/login';
export function LoginExpiredModal() {
  const expired = useAccessStore((s) => s.loginExpired);
  return (
    <Modal
      open={expired}
      title="登录已过期"
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

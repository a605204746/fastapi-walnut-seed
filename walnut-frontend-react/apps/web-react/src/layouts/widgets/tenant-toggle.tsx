import { useEffect, useState } from 'react';
import { Select } from 'antd';
import { usePreferences } from '@walnut/preferences';
import { useTabbarStore } from '@walnut/stores';
import { $t } from '@/locales';
import { tenantList, type TenantOption } from '@/api/core/auth';
import { useAccess } from '@/components/access/Access';
import { request } from '@/lib/request';
import { useDictStore } from '@/store/dict';
import { useTenantStore } from '@/store/tenant';
export function TenantToggle() {
  // 订阅偏好：locale 变化时重译 placeholder
  usePreferences();
  const { hasAccessByRoles } = useAccess();
  const superAdmin = hasAccessByRoles(['superadmin']);
  const [options, setOptions] = useState<TenantOption[]>([]);
  const [enabled, setEnabled] = useState(false);
  const [busy, setBusy] = useState(false);
  const { selected, setSelected } = useTenantStore();
  useEffect(() => {
    if (!superAdmin) return;
    let cancelled = false;
    void tenantList().then((data) => {
      if (!cancelled) {
        setEnabled(data.tenantEnabled);
        setOptions(data.voList);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [superAdmin]);
  async function change(value?: string) {
    if (value === selected) return;
    setBusy(true);
    try {
      await request.get(
        value
          ? `/system/tenant/dynamic/${encodeURIComponent(value)}`
          : '/system/tenant/dynamic/clear',
      );
      setSelected(value);
      useDictStore.getState().resetCache();
      useTabbarStore.getState().$reset();
      // Rebuild menus, user info and all cached API results for the new tenant.
      window.location.assign('/');
    } finally {
      setBusy(false);
    }
  }
  return enabled && superAdmin ? (
    <Select
      aria-label={$t('component.tenantToggle.placeholder')}
      className="mr-2 hidden md:block"
      style={{ width: 240 }}
      allowClear
      showSearch
      disabled={busy}
      loading={busy}
      value={selected}
      options={options.map((item) => ({ label: item.companyName, value: item.tenantId }))}
      optionFilterProp="label"
      onChange={(value) => void change(value)}
      placeholder={$t('component.tenantToggle.placeholder')}
    />
  ) : null;
}

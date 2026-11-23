import type { UserProfile } from '@/api/system/profile/model';

import { Tabs } from 'antd';

import { AccountBind } from './components/account-bind';
import { BaseSetting } from './components/base-setting';
import { SecureSetting } from './components/secure-setting';

export interface SettingPanelProps {
  className?: string;
  /** 基本设置更新成功后重载左侧资料卡（对应 Vue 侧 emitter 的 updateProfile） */
  onProfileUpdate?: () => Promise<void> | void;
  profile: UserProfile;
}

/**
 * 设置页签面板（对应 Vue 侧 views/_core/profile/setting-panel.vue）。
 * 三个页签：基本设置 / 安全设置 / 账号绑定，与 Vue 侧 settingList 顺序一致。
 */
export function SettingPanel({
  className,
  onProfileUpdate,
  profile,
}: SettingPanelProps) {
  return (
    <Tabs
      className={className}
      items={[
        {
          children: (
            <BaseSetting onProfileUpdate={onProfileUpdate} profile={profile} />
          ),
          key: '1',
          label: '基本设置',
        },
        {
          children: <SecureSetting />,
          key: '2',
          label: '安全设置',
        },
        {
          children: <AccountBind />,
          key: '3',
          label: '账号绑定',
        },
      ]}
      tabBarStyle={{ paddingInline: 16 }}
    />
  );
}

export default SettingPanel;

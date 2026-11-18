import { useCallback, useEffect, useState } from 'react';

import type { UserProfile } from '@/api/system/profile/model';

import { useUserStore } from '@vben/stores';

import { userProfile } from '@/api/system/profile';
import { Page } from '@/components/page/Page';
import { useAuthStore } from '@/store/auth';

import { ProfilePanel } from './profile-panel';
import { SettingPanel } from './setting-panel';

/**
 * 个人中心（对应 Vue 侧 views/_core/profile/index.vue）。
 *
 * 左：个人资料卡片（头像 + 账号信息）；右：设置页签。
 *
 * 与 Vue 版的差异：Vue 用 `mitt` 的 `emitter.on('updateProfile')` 在子组件里
 * 触发父组件重载；React 侧直接传一个 `onProfileChange` 回调，语义相同、少一个事件总线。
 */
export default function ProfilePage() {
  const [profile, setProfile] = useState<UserProfile | undefined>();
  const fetchUserInfo = useAuthStore((state) => state.fetchUserInfo);
  const setUserInfo = useUserStore((state) => state.setUserInfo);

  const loadProfile = useCallback(async () => {
    const resp = await userProfile();
    setProfile(resp);
  }, []);

  useEffect(() => {
    void loadProfile();
  }, [loadProfile]);

  /** 头像上传完毕：重载 profile + 刷新全局 userInfo（对应 Vue 侧 handleUploadFinish） */
  async function handleUploadFinish() {
    await loadProfile();
    const userInfo = await fetchUserInfo();
    setUserInfo(userInfo);
  }

  return (
    <Page>
      <div className="flex flex-col gap-[16px] lg:flex-row">
        {/* 左侧 */}
        <ProfilePanel
          onUploadFinish={() => void handleUploadFinish()}
          profile={profile}
        />
        {/* 右侧 */}
        {profile ? (
          <SettingPanel className="flex-1 overflow-hidden" profile={profile} />
        ) : null}
      </div>
    </Page>
  );
}

import type { DescriptionsProps } from 'antd';

import type { UserProfile } from '@/api/system/profile/model';

import { usePreferences } from '@vben/preferences';
import { Card, Descriptions, Tag, Tooltip, Upload } from 'antd';

import { userUpdateAvatar } from '@/api/system/profile';
import { getMessageApi } from '@/services/message';

export interface ProfilePanelProps {
  onUploadFinish?: () => void;
  profile?: UserProfile;
}

/**
 * 个人资料卡片（对应 Vue 侧 views/_core/profile/profile-panel.vue）。
 *
 * 与 Vue 版的差异：
 * - 头像 Vue 用 `CropperAvatar`（裁剪 + 上传，768 行）；React 侧裁剪组件属
 *   D 类待迁项（`react-cropper`），本轮先用 antd `Upload` 直传原图，
 *   **没有裁剪交互**，接口调用与刷新链路（userUpdateAvatar → 重载 profile →
 *   刷新全局 userInfo）与 Vue 完全一致。裁剪 UI 列入 B/C 阶段。
 * - 「今日诗词」仍是外链 SVG（jinrishici），暗色下换白色字体，与 Vue 一致。
 */
export function ProfilePanel({ onUploadFinish, profile }: ProfilePanelProps) {
  const { isDark, preferences } = usePreferences();

  const avatar = profile?.user.avatar || preferences.app.defaultAvatar;
  const poetrySrc = `https://v2.jinrishici.com/one.svg?font-size=12&color=${
    isDark ? 'white' : 'gray'
  }`;

  const items: DescriptionsProps['items'] = profile
    ? [
        { children: profile.user.userName, label: '账号' },
        {
          children: profile.user.phonenumber || '未绑定手机号',
          label: '手机号码',
        },
        { children: profile.user.email || '未绑定邮箱', label: '邮箱' },
        {
          children: (
            <div className="flex flex-wrap gap-1">
              <Tag color="processing">
                {profile.user.deptName ?? '未分配部门'}
              </Tag>
              {profile.postGroup ? (
                <Tag color="processing">{profile.postGroup}</Tag>
              ) : null}
            </div>
          ),
          label: '部门',
        },
        { children: profile.user.loginDate, label: '上次登录' },
      ]
    : [];

  async function handleUpload(file: File) {
    try {
      await userUpdateAvatar({ file, filename: file.name, name: file.name });
      getMessageApi()?.message.success('头像更新成功');
      onUploadFinish?.();
    } catch (error) {
      console.error(error);
    }
  }

  return (
    <Card className="h-full lg:w-1/3" loading={!profile}>
      {profile ? (
        <div className="flex flex-col items-center gap-[24px]">
          <div className="flex flex-col items-center gap-[20px]">
            <Tooltip title="点击上传头像">
              <Upload
                accept="image/*"
                beforeUpload={(file) => {
                  void handleUpload(file);
                  // 阻止 antd 自动上传，走自定义逻辑
                  return false;
                }}
                showUploadList={false}
              >
                {/* eslint-disable-next-line jsx-a11y/alt-text */}
                <img
                  className="size-[120px] cursor-pointer rounded-full object-cover"
                  src={avatar}
                />
              </Upload>
            </Tooltip>
            <div className="flex flex-col items-center gap-[8px]">
              <span className="text-foreground text-xl font-bold">
                {profile.user.nickName ?? '未知'}
              </span>
              {/* https://www.jinrishici.com/doc/#image */}
              <img alt="poetry" src={poetrySrc} />
            </div>
          </div>
          <div className="px-[24px]">
            <Descriptions column={1} items={items} />
          </div>
        </div>
      ) : null}
    </Card>
  );
}

export default ProfilePanel;

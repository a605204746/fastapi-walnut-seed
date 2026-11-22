import type { DescriptionsProps } from 'antd';

import type { UserProfile } from '@/api/system/profile/model';

import { usePreferences } from '@walnut/preferences';
import { Card, Descriptions, Tag, Tooltip } from 'antd';

import { CropperAvatar } from '@/components/cropper';
import { userUpdateAvatar } from '@/api/system/profile';

export interface ProfilePanelProps {
  onUploadFinish?: () => void;
  profile?: UserProfile;
}

/**
 * 个人资料卡片；头像通过 CropperAvatar 裁剪为 PNG，再上传并刷新资料。
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

  return (
    <Card className="h-full lg:w-1/3" loading={!profile}>
      {profile ? (
        <div className="flex flex-col items-center gap-[24px]">
          <div className="flex flex-col items-center gap-[20px]">
            <Tooltip title="点击上传头像">
              <CropperAvatar src={avatar} upload={file => userUpdateAvatar({ file, filename: file.name, name: file.name })} onSaved={onUploadFinish} />
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

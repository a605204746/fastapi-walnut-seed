import { forwardRef, useImperativeHandle, useState } from 'react';

import type { DescriptionsProps } from 'antd';

import type { User } from '@/api/system/user/model';

import { useVbenModal } from '@vben-core/popup-ui';
import { DictEnum } from '@vben/constants';
import { Descriptions, Tag } from 'antd';
import dayjs from 'dayjs';
import duration from 'dayjs/plugin/duration';
import relativeTime from 'dayjs/plugin/relativeTime';
import 'dayjs/locale/zh-cn';

import { findUserInfo } from '@/api/system/user';
import { DictTag } from '@/components/dict';
import { useDictStore } from '@/store/dict';
import { getDictOptions } from '@/utils/dict';

dayjs.extend(duration);
dayjs.extend(relativeTime);

export interface UserInfoModalHandle {
  open: (id: number | string) => void;
}

interface UserWithNames extends User {
  postNames: string[];
  roleNames: string[];
}

/**
 * 用户信息详情弹窗（对应 Vue 侧 views/system/user/user-info-modal.vue，155 行）。
 *
 * 与 Vue 版的差异：
 * - `shallowRef` → `useState`。
 * - Descriptions item 的 `content` → `children`（antd React 的字段名）。
 * - `dayjs.locale('zh-cn')` 从 computed 内部（副作用）提到模块顶层，
 *   避免每次渲染都改全局 locale。
 */
export const UserInfoModal = forwardRef<UserInfoModalHandle>(
  function UserInfoModal(_props, ref) {
    // 订阅字典 store：状态 Tag 依赖字典
    useDictStore((state) => state.map);
    const [currentUser, setCurrentUser] = useState<null | UserWithNames>(null);

    const [BasicModal, modalApi] = useVbenModal({
      footer: false,
      fullscreenButton: false,
      onClosed: () => setCurrentUser(null),
      onOpenChange: async (isOpen) => {
        if (!isOpen) return;
        modalApi.modalLoading(true);

        const { id } = modalApi.getData<{ id: number | string }>();
        const response = await findUserInfo(id);
        // 外部的 roleIds / postIds 才是真正对应的，新增时为空
        // posts 有为 null 的情况，需要给默认值
        const {
          postIds = [],
          posts = [],
          roleIds = [],
          roles = [],
          user,
        } = response ?? ({} as any);

        const postNames = (posts ?? [])
          .filter((item: any) => postIds.includes(item.id))
          .map((item: any) => item.postName);
        const roleNames = (roles ?? [])
          .filter((item: any) => roleIds.includes(item.id))
          .map((item: any) => item.roleName);

        setCurrentUser({
          ...(user as User),
          postNames,
          roleNames,
        } as UserWithNames);

        modalApi.modalLoading(false);
      },
    });

    useImperativeHandle(
      ref,
      () => ({
        open: (id) => {
          modalApi.setData({ id });
          modalApi.open();
        },
      }),
      [modalApi],
    );

    /** 「上次登录时间」的人性化距今时长（x 天 / x 小时 …） */
    function diffLoginTime(): string {
      if (!currentUser?.loginDate) return '';
      dayjs.locale('zh-cn');
      const diffSeconds = dayjs().diff(dayjs(currentUser.loginDate), 'second');
      return dayjs.duration(diffSeconds, 'seconds').humanize();
    }

    const statusOptions = getDictOptions(DictEnum.SYS_NORMAL_DISABLE);

    const items: DescriptionsProps['items'] = (() => {
      if (!currentUser) return [];
      const { id, status } = currentUser;
      const mixInfo = `${currentUser.userName} / ${currentUser.nickName} / ${
        currentUser.deptName ?? '-'
      }`;
      const diffText = diffLoginTime();
      return [
        { children: id, label: 'userId' },
        {
          children: <DictTag dicts={statusOptions} value={status} />,
          label: '用户状态',
        },
        { children: mixInfo, label: '用户信息' },
        { children: currentUser.phonenumber || '-', label: '手机号' },
        { children: currentUser.email || '-', label: '邮箱' },
        {
          children: (
            <div className="flex flex-wrap gap-0.5">
              {currentUser.postNames.map((item) => (
                <Tag key={item}>{item}</Tag>
              ))}
            </div>
          ),
          label: '岗位',
        },
        {
          children: (
            <div className="flex flex-wrap gap-0.5">
              {currentUser.roleNames.map((item) => (
                <Tag key={item}>{item}</Tag>
              ))}
            </div>
          ),
          label: '权限',
        },
        { children: currentUser.createTime, label: '创建时间' },
        { children: currentUser.loginIp || '-', label: '上次登录IP' },
        {
          children: (
            <>
              <span>{currentUser.loginDate ?? '-'}</span>
              {diffText ? (
                <Tag className="ml-2" color="processing">
                  {diffText}前
                </Tag>
              ) : null}
            </>
          ),
          label: '上次登录时间',
        },
        { children: currentUser.remark || '-', label: '备注' },
      ];
    })();

    return (
      <BasicModal title="用户信息">
        {currentUser ? (
          <Descriptions bordered column={1} items={items} size="small" />
        ) : null}
      </BasicModal>
    );
  },
);

export default UserInfoModal;

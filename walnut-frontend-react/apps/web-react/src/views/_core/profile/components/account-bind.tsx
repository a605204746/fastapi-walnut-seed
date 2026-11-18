import { useEffect, useState } from 'react';

import type { BindItem } from '../../oauth-common';

import type { SocialInfo } from '@/api/system/social/model';

import { Avatar, Button, Card, Empty, Tooltip } from 'antd';

import { authUnbinding } from '@/api/core/auth';
import { socialList } from '@/api/system/social';
import { getMessageApi } from '@/services/message';

import { accountBindList, handleAuthBinding } from '../../oauth-common';

interface BindItemWithInfo extends BindItem {
  bound?: boolean;
  info?: SocialInfo;
}

/**
 * 账号绑定（对应 Vue 侧 views/_core/profile/components/account-bind.vue）。
 *
 * 与 Vue 版的差异：图标 Vue 用 iconify 组件 + 内联 style，React 侧
 * `oauth-common.accountBindList` 已改成 `public/icons/*.svg` 路径（见该文件注释），
 * 这里直接 `<img>` 渲染。
 */
export function AccountBind() {
  const [bindList, setBindList] = useState<BindItemWithInfo[]>([]);

  async function loadData() {
    const resp = await socialList();
    const list: BindItemWithInfo[] = accountBindList.map((item) => ({
      ...item,
    }));
    const sources = (resp ?? []).map((social) => social.source.toLowerCase());
    list.forEach((item) => {
      item.bound = sources.includes(item.source.toLowerCase());
      if (item.bound) {
        item.info = (resp ?? []).find(
          (social) => social.source.toLowerCase() === item.source,
        );
      }
    });
    setBindList(list);
  }

  useEffect(() => {
    void loadData();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /** 解绑账号 */
  function handleUnbind(record: BindItemWithInfo) {
    if (!record.info) return;
    getMessageApi()?.modal.confirm({
      content: `确定解绑[${record.source}]平台的[${record.info.userName}]账号吗？`,
      onOk: async () => {
        await authUnbinding(record.info!.id);
        await loadData();
      },
      title: '提示',
    });
  }

  if (bindList.length === 0) {
    return (
      <Empty
        className="py-8"
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description="暂无绑定数据"
      />
    );
  }

  return (
    <div className="flex flex-col gap-4 pb-4">
      <div className="grid grid-cols-1 gap-4 p-4 lg:grid-cols-2 2xl:grid-cols-3">
        {bindList.map((item) => (
          <Card
            className="transition-shadow duration-300 hover:shadow-md"
            key={item.source}
          >
            <div className="flex w-full items-center gap-4">
              {item.icon ? (
                <img alt={item.title} className="size-[40px]" src={item.icon} />
              ) : null}
              <div className="flex flex-1 items-center justify-between">
                <div className="flex flex-col">
                  <h4 className="text-foreground mb-[4px] text-[14px]">
                    {item.title}
                  </h4>
                  <span className="text-muted-foreground">
                    {!item.bound ? item.description : null}
                    {item.bound && item.info ? (
                      <Tooltip
                        title={
                          <div className="flex flex-col items-center gap-2 p-2">
                            <Avatar size={36} src={item.info.avatar} />
                            <div>绑定时间: {item.info.createTime}</div>
                          </div>
                        }
                      >
                        <div className="cursor-pointer">
                          已绑定: {item.info.nickName}
                        </div>
                      </Tooltip>
                    ) : null}
                  </span>
                </div>
                <Button
                  onClick={() =>
                    item.bound
                      ? handleUnbind(item)
                      : void handleAuthBinding(item.source)
                  }
                  size="small"
                  type={item.bound ? 'default' : 'link'}
                >
                  {item.bound ? '取消绑定' : '绑定'}
                </Button>
              </div>
            </div>
          </Card>
        ))}
      </div>
    </div>
  );
}

export default AccountBind;

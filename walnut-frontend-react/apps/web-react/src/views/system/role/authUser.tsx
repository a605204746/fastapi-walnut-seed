import type { ComponentType } from 'react';

import { lazy, Suspense } from 'react';

import { Result, Spin } from 'antd';

/**
 * 对应 Vue 侧 `views/system/role/authUser.vue`。
 *
 * Vue 版全文只做一件事：`import RoleAssignPage from '#/views/system/role-assign/index.vue'`
 * 然后渲染它（注释说明「后端版本 >= 5.4.0 这个从本地路由变为从后台返回，
 * 未修改文件名而是新加了这个文件」）。后端菜单里对应
 * `component: 'system/role/authUser'`、路径 `role-auth/user/:roleId`、`hidden: true`。
 *
 * React 侧用 `import.meta.glob` 而非静态 import 指向 `role-assign`：
 * 该页面属于迁移计划的 A2 阶段，目前还没落地，静态 import 会直接编译不过。
 * glob 的 key 是字面量字符串，文件不存在时得到空对象，**等 A2 建好
 * `views/system/role-assign/index.tsx` 后本页自动切换为真实内容，无需回来改这个文件**。
 *
 * ⚠️ 文件必须有 `export default`（见 `system/user/authRole.tsx` 的说明）。
 */
const ROLE_ASSIGN_PATH = '../role-assign/index.tsx';

const roleAssignModules = import.meta.glob<{ default: ComponentType<any> }>(
  '../role-assign/index.tsx',
);

const RoleAssignPage = roleAssignModules[ROLE_ASSIGN_PATH]
  ? lazy(roleAssignModules[ROLE_ASSIGN_PATH]!)
  : undefined;

export function RoleAuthUser() {
  if (!RoleAssignPage) {
    return (
      <Result
        status="info"
        subTitle="Vue 侧本文件仅 re-export views/system/role-assign/index.vue；该页面属于迁移计划 A2 阶段，尚未落地。"
        title="分配用户页待迁移"
      />
    );
  }

  return (
    <Suspense fallback={<Spin className="flex-center min-h-[200px]" />}>
      <RoleAssignPage />
    </Suspense>
  );
}

export default RoleAuthUser;

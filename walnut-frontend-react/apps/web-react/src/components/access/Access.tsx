import type { ReactNode } from 'react';

import { useAccessStore, useUserStore } from '@walnut/stores';

export interface AccessProps {
  /** 权限码，OR 语义（对应 v-access:code） */
  code?: string[];
  /** 角色，OR 语义（对应 v-access:role） */
  role?: string[];
  children?: ReactNode;
}

const SUPER_ROLE = 'superadmin';
const ALL_CODES = '*:*:*';

/**
 * 权限判断 hook（对应 Vue 基线 access 包的 `useAccess()`）。
 *
 * `<Access>` 组件只能做「显隐」，但有些场景需要在逻辑里判断权限
 * （例如客户端管理页：非超管或无 `system:client:edit` 时，状态开关要 disabled
 * 而不是隐藏），所以把判断逻辑抽成 hook 供两处复用。
 */
export function useAccess() {
  const accessCodes = useAccessStore((state) => state.accessCodes);
  const userRoles = useUserStore((state) => state.userRoles);

  function hasAccessByRoles(roles: string[]) {
    return (
      userRoles.includes(SUPER_ROLE) || roles.some((r) => userRoles.includes(r))
    );
  }

  function hasAccessByCodes(codes: string[]) {
    return (
      accessCodes.includes(ALL_CODES) || codes.some((c) => accessCodes.includes(c))
    );
  }

  return { accessCodes, hasAccessByCodes, hasAccessByRoles, userRoles };
}

/**
 * 权限显隐组件（对应 Vue 侧 packages/effects/access 的 access-control.vue + directive.ts）。
 * 75 处 `v-access:code` 调用点改为 `<Access code={[...]}>`，权限串逐字一致。
 *
 * ⚠️ `code` 与 `role` **同时给出时是 AND 语义**：Vue 侧同一元素上写
 * `v-access:code` + `v-access:role` 是两个独立指令，都要通过才渲染
 * （菜单管理页的增删改按钮就是「超管角色 且 system:menu:add 权限」）。
 * 早前的实现是 else-if（role 优先、忽略 code），已修正。
 */
export function Access({ children, code, role }: AccessProps) {
  const { hasAccessByCodes, hasAccessByRoles } = useAccess();

  let allowed = true;
  if (role && role.length > 0) {
    allowed = allowed && hasAccessByRoles(role);
  }
  if (code && code.length > 0) {
    allowed = allowed && hasAccessByCodes(code);
  }

  return allowed ? <>{children}</> : null;
}

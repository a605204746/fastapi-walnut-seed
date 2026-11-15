import type { ReactNode } from 'react';

import { useAccessStore, useUserStore } from '@vben/stores';

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
 * 权限显隐组件（对应 Vue 侧 packages/effects/access 的 access-control.vue + directive.ts）。
 * 75 处 `v-access:code` 调用点改为 `<Access code={[...]}>`，权限串逐字一致。
 */
export function Access({ children, code, role }: AccessProps) {
  const accessCodes = useAccessStore((state) => state.accessCodes);
  const userRoles = useUserStore((state) => state.userRoles);

  const hasByRoles = (roles: string[]) =>
    userRoles.includes(SUPER_ROLE) || roles.some((r) => userRoles.includes(r));

  const hasByCodes = (codes: string[]) =>
    accessCodes.includes(ALL_CODES) || codes.some((c) => accessCodes.includes(c));

  let allowed = true;
  if (role && role.length > 0) {
    allowed = hasByRoles(role);
  } else if (code && code.length > 0) {
    allowed = hasByCodes(code);
  }

  return allowed ? <>{children}</> : null;
}

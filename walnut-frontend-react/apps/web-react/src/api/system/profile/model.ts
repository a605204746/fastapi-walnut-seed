export interface Dept {
  id: number;
  parentId: number;
  parentName?: any;
  ancestors: string;
  deptName: string;
  orderNum: number;
  leader: string;
  phone?: any;
  email: string;
  status: string;
  createTime?: any;
}

export interface Role {
  id: number;
  roleName: string;
  roleKey: string;
  roleSort: number;
  dataScope: string;
  menuCheckStrictly?: any;
  deptCheckStrictly?: any;
  status: string;
  remark: string;
  createTime?: any;
  flag: boolean;
  superAdmin: boolean;
}

export interface User {
  id: number;
  tenantId: string;
  deptId: number;
  userName: string;
  nickName: string;
  userType: string;
  email: string;
  phonenumber: string;
  sex: string;
  avatar: string;
  status: string;
  loginIp: string;
  loginDate: string;
  remark: string;
  createTime: string;
  dept: Dept;
  roles: Role[];
  roleIds?: string[];
  postIds?: string[];
  roleId: number;
  deptName: string;
}

/**
 * @description 用户个人主页信息
 * @param user 用户信息
 * @param roleGroup 角色名称
 * @param postGroup 岗位名称
 */
export interface UserProfile {
  user: User;
  roleGroup: string;
  postGroup: string;
}

export interface UpdatePasswordParam {
  oldPassword: string;
  newPassword: string;
}

/**
 * ⚠️ 迁移差异：Vue 侧 `model.d.ts` 里这个 interface **没有 `export`**，
 * 但 `.d.ts` 的模块级声明对 TS 而言是隐式可见的，`profile/index.ts` 照样能
 * `import type { FileCallBack } from './model'`。改名为 `.ts` 后该隐式规则失效，
 * 必须显式 `export`。全仓仅此一处（其余 model 声明本来就带 export）。
 */
export interface FileCallBack {
  name: string;
  file: Blob;
  filename: string;
}

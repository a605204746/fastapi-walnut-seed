import { request } from '@/lib/request';

export interface Role {
  dataScope: string;
  flag: boolean;
  id: number;
  roleKey: string;
  roleName: string;
  roleSort: number;
  status: string;
  superAdmin: boolean;
}

export interface User {
  avatar: string;
  createTime: string;
  deptId: number;
  deptName: string;
  email: string;
  id: number;
  loginDate: string;
  loginIp: string;
  nickName: string;
  phonenumber: string;
  remark: string;
  roles: Role[];
  sex: string;
  status: string;
  tenantId: string;
  userName: string;
  userType: string;
}

export interface UserInfoResp {
  permissions: string[];
  roles: string[];
  user: User;
}

/**
 * 获取用户信息
 * 存在返回 null 的情况(401) 不会抛出异常 需要手动抛异常
 */
export async function getUserInfoApi() {
  return request.get<null | UserInfoResp>('/system/user/getInfo');
}

import type { Page } from '@playwright/test';
import menuTree from '../../../src/__fixtures__/backend-menu-tree.json' with { type: 'json' };
import { installMockApi } from './mock-api';
export async function installFullApi(page: Page) {
  await installMockApi(page);
  const role = {
    id: 2,
    roleName: '测试角色',
    roleKey: 'test',
    roleSort: 1,
    status: '0',
    dataScope: '2',
    menuCheckStrictly: true,
    deptCheckStrictly: true,
    superAdmin: false,
    createTime: '2026-01-01 00:00:00',
  };
  let roles: any[] = [role];
  let notices: any[] = [
    {
      id: 1,
      noticeTitle: '迁移验收公告',
      noticeType: '1',
      status: '0',
      noticeContent: '<h2>历史公告</h2><p><strong>兼容内容</strong></p>',
      createByName: '管理员',
      createTime: '2026-01-01 00:00:00',
    },
  ];
  const user = {
    id: 2,
    userName: 'tester',
    nickName: '测试用户',
    status: '0',
    sex: '0',
    phonenumber: '15888888888',
    email: 'test@example.com',
    deptId: 1,
    deptName: '研发部',
    dept: { deptName: '研发部' },
    roles: [role],
    createTime: '2026-01-01 00:00:00',
  };
  const tree = [
    {
      id: 1,
      parentId: 0,
      label: '系统管理',
      menuType: 'M',
      children: [
        {
          id: 2,
          parentId: 1,
          label: '用户管理',
          menuType: 'C',
          children: [
            { id: 3, parentId: 2, label: '新增用户', menuType: 'F' },
            { id: 4, parentId: 2, label: '删除用户', menuType: 'F' },
          ],
        },
      ],
    },
  ];
  const depts = [
    {
      id: 1,
      parentId: 0,
      label: '总公司',
      deptName: '总公司',
      children: [{ id: 2, parentId: 1, label: '研发部', deptName: '研发部' }],
    },
  ];
  const mutations: { path: string; method: string; body: any }[] = [];
  await page.route(
    (url) => /^\/(?:prod-api|api)\//.test(url.pathname),
    async (route) => {
      const req = route.request();
      const path = new URL(req.url()).pathname.replace(/^\/(?:prod-api|api)/, '');
      const method = req.method();
      const reply = (data: unknown) =>
        route.fulfill({
          contentType: 'application/json',
          body: JSON.stringify({ code: 200, msg: '操作成功', data }),
        });
      if (path === '/system/menu/getRouters') return reply(menuTree);
      if (path.startsWith('/system/dict/data/type/')) {
        const type = path.split('/').at(-1)!;
        if (type === 'sys_yes_no') return route.fallback();
        return reply([
          {
            dictLabel: type === 'sys_notice_type' ? '通知' : '正常',
            dictValue: type === 'sys_notice_type' ? '1' : '0',
            listClass: 'success',
          },
          {
            dictLabel: type === 'sys_notice_type' ? '公告' : '停用',
            dictValue: type === 'sys_notice_type' ? '2' : '1',
            listClass: 'danger',
          },
        ]);
      }
      if (path === '/system/role/list') return reply({ rows: roles, total: roles.length });
      if (path === '/system/notice/list') return reply({ rows: notices, total: notices.length });
      if (path.startsWith('/system/menu/roleMenuTreeselect/'))
        return reply({ menus: tree, checkedKeys: [3] });
      if (path === '/system/menu/treeselect') return reply(tree);
      if (path.startsWith('/system/role/deptTree/')) return reply({ depts, checkedKeys: [2] });
      if (path === '/system/user/deptTree' || path.endsWith('/deptTreeSelect')) return reply(depts);
      if (path === '/system/user/profile')
        return reply({ user, roleGroup: '测试角色', postGroup: '工程师' });
      if (path === '/system/user/authRole/2') return reply({ user, roles });
      if (
        path === '/system/role/authUser/allocatedList' ||
        path === '/system/role/authUser/unallocatedList' ||
        path === '/system/user/list'
      )
        return reply({ rows: [user], total: 1 });
      if (path === '/system/role/2' && method === 'GET')
        return reply(roles.find((row) => row.id === 2));
      if (/^\/system\/notice\/\d+$/.test(path) && method === 'GET')
        return reply(notices.find((row) => row.id === Number(path.split('/').at(-1))));
      if (
        (path === '/system/role' ||
          path === '/system/role/dataScope' ||
          path === '/system/role/changeStatus' ||
          path === '/system/notice') &&
        ['POST', 'PUT'].includes(method)
      ) {
        const body = req.postDataJSON();
        mutations.push({ path, method, body });
        if (path === '/system/role') {
          if (method === 'POST') roles.push({ ...role, ...body, id: 3 });
          else roles = roles.map((row) => (row.id === body.id ? { ...row, ...body } : row));
        }
        if (path === '/system/notice') {
          if (method === 'POST') notices.push({ ...body, id: notices.length + 1 });
          else notices = notices.map((row) => (row.id === body.id ? { ...row, ...body } : row));
        }
        return reply(null);
      }
      if (method === 'DELETE' && /^\/system\/(role|notice)\//.test(path)) {
        const ids = path.split('/').at(-1)!.split(',').map(Number);
        mutations.push({ path, method, body: null });
        if (path.includes('/role/')) roles = roles.filter((row) => !ids.includes(row.id));
        else notices = notices.filter((row) => !ids.includes(row.id));
        return reply(null);
      }
      if (path === '/system/dept/list')
        return reply([{ id: 1, parentId: 0, deptName: '总公司', orderNum: 1, status: '0' }]);
      if (path === '/system/menu/list')
        return reply([
          {
            id: 1,
            parentId: 0,
            menuName: '系统管理',
            menuType: 'M',
            orderNum: 1,
            status: '0',
            visible: '0',
            icon: 'lucide:settings',
          },
        ]);
      if (path === '/system/post/list')
        return reply({
          rows: [{ id: 1, postName: '工程师', postCode: 'dev', postSort: 1, status: '0' }],
          total: 1,
        });
      if (path === '/system/client/list')
        return reply({
          rows: [{ id: 2, clientName: '测试客户端', clientId: 'test', status: '0' }],
          total: 1,
        });
      if (path === '/system/dict/type/1')
        return reply({ id: 1, dictName: '通知类型', dictType: 'sys_notice_type' });
      if (path === '/system/dict/data/list')
        return reply({
          rows: [
            {
              id: 1,
              dictLabel: '通知',
              dictValue: '1',
              dictType: 'sys_notice_type',
              dictSort: 1,
              status: '0',
            },
          ],
          total: 1,
        });
      if (path === '/system/dict/type/list')
        return reply({
          rows: [{ id: 1, dictName: '通知类型', dictType: 'sys_notice_type', status: '0' }],
          total: 1,
        });
      if (path.endsWith('/list') && path.startsWith('/monitor/'))
        return reply({ rows: [], total: 0 });
      return route.fallback();
    },
  );
  return { mutations };
}

/**
 * 对应 Vue 侧 `views/system/user/authRole.vue` —— **占位页，无实际业务意义**。
 *
 * 存在理由：后端 `sys_menu.component` 里有一行 `system/user/authRole`
 * （菜单「分配角色」，路径 `user-auth/role/:userId`，`hidden: true`），
 * `generateRoutesByBackend` 会按 `/system/user/authRole.tsx` 精确查 pageMap，
 * 查不到就退化成 `NotFoundComponent` 并在控制台报「未找到对应组件」。
 * Vue 侧同样是为此保留该文件（见迁移清单 §12 的「不可删」警告）。
 *
 * ⚠️ 文件必须有 `export default`：`createLazyPageMap` 用 `React.lazy` 装载
 * `import.meta.glob('../views/**\/*.tsx')`，缺 default export 只会在渲染时炸，
 * 类型检查发现不了（清单 §16.4③）。
 */
export function UserAuthRole() {
  return (
    <div>
      ele版本会使用这个文件 只是为了不报错`未找到对应组件`才新建的这个文件
      无实际意义
    </div>
  );
}

export default UserAuthRole;

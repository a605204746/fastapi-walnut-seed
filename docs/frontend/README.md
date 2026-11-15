# 前端开发

前端位于 `walnut-frontend/`，是基于 Vue 3、Vben、antdv-next、pnpm 和 Turbo 的 monorepo。主应用为 `apps/web-antd/`。

## 开发命令

```bash
cd walnut-frontend
pnpm install
pnpm dev:antd
pnpm build:antd
pnpm check:type
```

## 关键约定

- API 请求位于 `apps/web-antd/src/api/`。
- 页面位于 `apps/web-antd/src/views/`。
- 动态菜单由后端 `GET /system/menu/getRouters` 驱动。
- `sys_menu.component` 要能映射到 `src/views/*.vue`。
- 按钮使用 `v-access:code`，权限串必须与后端 `AuthPermission` 一致。
- 前端只负责交互和权限显隐，后端是最终安全边界。

## 前端相关文档

- [接口契约](./01-接口契约.md)
- [权限与动态路由](./02-权限与动态路由.md)
- [接口加解密](./03-接口加解密.md)
- [国际化](./04-国际化.md)
- [React 迁移逐文件清单](./05-React迁移-逐文件清单.md) —— Vue → React 改写的全量文件级迁移计划（阶段 ①–⑤ 已实施，见 §13–§15；⑥ 起待做）

本目录内容只针对前端；后端请从 [文档总入口](../README.md) 进入 Python 或 Java 目录。

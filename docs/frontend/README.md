# 前端开发

两个前端并存，共用同一套后端接口契约：

| 仓库 | 定位 | 技术栈 | dev 端口 |
| --- | --- | --- | --- |
| `walnut-frontend-react/` | **默认前端、唯一活跃**：新功能与修复只落这里 | React 19 + antd 6 + Tailwind 4 + Zustand | 8020 |
| `walnut-frontend/` | **Vue 3 基线，2026-11-22 起冻结**：仅作迁移的像素/行为对比参照，不再同步新功能与修复，无 CI | Vue 3 + antdv-next + Pinia | 8010 |

## 开发命令

React（默认）：

```bash
cd walnut-frontend-react
pnpm install
pnpm dev          # http://localhost:8020
pnpm check        # lint / typecheck / unit / build
```

Vue 基线（仅对比用）：

```bash
cd walnut-frontend
pnpm install
pnpm dev:antd     # http://localhost:8010
```

## 关键约定

- 两端动态菜单均由后端 `GET /system/menu/getRouters` 驱动；前端只负责交互和权限显隐，后端是最终安全边界。
- React：API 位于 `apps/web-react/src/api/`，页面位于 `apps/web-react/src/views/`（`sys_menu.component` 映射到 `src/views/*.tsx`）；权限用 `<Access>` / `useAccess`，权限串必须与后端 `AuthPermission` 一致。
- Vue 基线：API 位于 `apps/web-antd/src/api/`，页面位于 `apps/web-antd/src/views/`（`*.vue`）；权限用 `v-access:code`。
- React 仓包名统一为 `@walnut/*` / `@walnut-core/*`（2026-11-22 完成标识符级去 vben 改名）；跨仓引用统一称「Vue 基线」。
- 部署：默认 Docker 编排的前端镜像仍由 Vue 基线构建；React 部署走 `docker/docker-compose.react.yml` overlay。默认编排的切换属部署决策，不随冻结自动变更。

## 前端相关文档

- [接口契约](./01-接口契约.md)
- [权限与动态路由](./02-权限与动态路由.md)
- [接口加解密](./03-接口加解密.md)
- [国际化](./04-国际化.md)
- [React 迁移逐文件清单](./05-React迁移-逐文件清单.md) —— Vue → React 改写的全量文件级迁移记录
- [React 迁移总计划与进度](./06-React迁移-总计划与进度.md) —— 历史计划与阶段记录（数字为当时快照），顶部含最新状态与冻结决策
- [React 迁移验收记录](./07-React迁移-验收记录.md) —— 范围、实测结果与联调边界

本目录内容只针对前端；后端请从 [文档总入口](../README.md) 进入 Python、Java 或 Go 目录。
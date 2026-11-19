# React 迁移验收记录

2026-11-19 · Vue 基线 `96f77f6` · React 19 / antd 6。本文为当前状态，05/06 中带日期的阶段记录保留作历史参考。

## 覆盖范围

按现有 Vue 的可达功能和已确认的 D1/D4 范围，前端迁移实施覆盖 **100%**，后端菜单 fixture 的 **14/14 component** 经过真实路由解析单测，无 404 替代页。没有按 Vue 源文件行数或依赖数量计算进度。

| 功能面 | 本次补齐 |
| --- | --- |
| 角色 | CRUD、导出、状态、用户分配、菜单/按钮权限、父子关联与独立勾选、数据范围和部门选择；提交只补祖先、不授权兄弟节点 |
| 公告 | CRUD、字典、Tiptap HTML 读写、历史 HTML 回显、图片上传、源代码、全屏、未保存关闭确认 |
| 仪表盘与通用页 | analytics、workspace、about、500、offline、coming-soon、社交登录回调 |
| 隐藏业务路由 | 用户分配角色、角色分配用户、字典数据详情；保留后端 component 路径兼容 |
| 共享控件 | ApiSelect/TreeSelect/Cascader、IconPicker、文件/图片上传、JSON 预览、ECharts、头像裁剪与 400×400 PNG 上传 |
| 外壳 | 七种布局、深浅/半暗主题、横向混合导航、菜单标记、面包屑、移动抽屉、最大化恢复、内容水印、响应式尺寸 |
| 交互 | Activity 有界页签缓存、刷新当前页、固定/拖拽/上下文菜单、全局搜索与快捷键、锁屏、通知已读/清空、SSE、租户切换、时区、更新提示 |
| 偏好与表格 | 滚动顶栏、按钮效果、偏好开关接通、列显隐、表格最大化、查询折叠/恢复、行高/列宽/分页及工具栏间距 |
| 工程 | pnpm check、ESLint、CI、生产配置注入、命名空间隔离、AES 持久化、Docker React 镜像与 Compose overlay |

D1 已排除的无调用 Vue 原语不逐文件复制；D4 只保留 Tiptap，`RichTextarea` 映射到同一编辑器。两套前端目录并存，默认部署选择保持显式。

## 实测验证

本机 Windows、Node 24、pnpm 11、Chromium；API 由确定性 fixture 拦截。

- `pnpm check`：lint、全部 workspace 类型检查、36 个单测文件 / 424 个用例、生产构建通过。
- `playwright test specs/migration.spec.ts specs/layout.spec.ts specs/session.spec.ts --workers=2`：13/13 通过。包含 17 个页面入口、角色菜单祖先与数据范围请求体、公告 HTML 编辑/回显、页签缓存/刷新、搜索、七布局重载、深浅/半暗、最大化恢复、会话失效弹窗、锁屏与 390px 移动抽屉。
- `PRODUCTION_BASE=http://127.0.0.1:18020 ... playwright test specs/production.spec.ts`：1/1 通过。实际生产包登录、localStorage 密文、刷新恢复、富文本/图表延迟加载和页面无运行时错误。
- `capture:golden`、`capture:react`、`compare`：5 个双端样式目标通过；参数页、整页外壳门限已收紧为原计划目标。

| 对比目标 | 差异像素占比 | 门限 |
| --- | ---: | ---: |
| 参数表格卡片 1024×600 | 1.174% | 1.5% |
| 浅色外壳 1280×720 | 1.347% | 2% |
| 深色外壳 1280×720 | 1.654% | 2% |
| 浅色登录 | 0.021% | 0.3% |
| 深色登录 | 0.021% | 0.3% |

截图在同机同浏览器重新生成 Vue golden：两端统一登录后等待落到参数页，冻结动画并隐藏临时 toast/通知/进度覆盖层，避免把瞬时状态当作页面样式。未隐藏业务内容、菜单、表格或字段。差异仍包含 antd Table 与 vxe-table 的字形/边框等细节，不声称逐像素完全一致。

生产分包不递归捕获共享依赖，业务路由交给自动分包，严格执行模块初始化顺序。实测登录页不会请求 charts、rich-editor、avatar-cropper；编辑器和图表分别在打开公告编辑与仪表盘时加载。

## 复现

```sh
cd walnut-frontend-react
pnpm install --frozen-lockfile
pnpm check
pnpm --filter @walnut-seed/e2e-visual install:browsers
pnpm dev
# 另一个终端：
pnpm --filter @walnut-seed/e2e-visual exec playwright test specs/migration.spec.ts specs/layout.spec.ts specs/session.spec.ts --workers=2
```

双端像素对比另外启动 `pnpm --dir ../walnut-frontend --filter @walnut-seed/web-admin dev`（8010），React 为 8020，然后依次执行 `pnpm --filter @walnut-seed/e2e-visual capture:golden`、`capture:react`、`compare`。先生成本机 golden，以免跨 OS 字体渲染造成误差。

生产验收先运行 `pnpm --filter @walnut-seed/web-react exec vite preview --host 127.0.0.1 --port 18020`，再给 production.spec 设置 `PRODUCTION_BASE`。PowerShell 用 `$env:PRODUCTION_BASE='http://127.0.0.1:18020'`。

## 部署

生产静态文件位于 `walnut-frontend-react/apps/web-react/dist`，代理沿用 `docker/config/nginx.conf`，API 前缀为 `/prod-api`，上传为 `/upload`。配置来自 `.env.production` 和构建生成的 `_app-config-*.js`；持久化命名空间包含环境与应用名，生产 store 使用 SecureLS AES。

可选 React 镜像（Compose 2.24.4+，overlay 相对第一份 compose 的 docker 目录解析）：

```sh
docker compose -f docker/docker-compose.go.demo.yml -f docker/docker-compose.react.yml up -d --build
```

## 验证边界

本机没有 Docker，未执行镜像构建或容器后端联调。新增 CI 会在 GitHub 上执行 Linux 工程/浏览器/生产预览验收和 Windows 同机 Vue/React 像素对比；远端 CI 状态以 PR 检查结果为准。

注册、找回密码和二维码在 Vue 中属于示例流程，React 明确展示示例提示；短信与社交登录保留真实 API 契约，能否登录取决于所选后端实现及第三方配置。生产数据、真实 OSS、SSE、租户切换和第三方 OAuth 需在目标部署环境进行联调；本次没有把 fixture 通过等同于三后端全栈验收通过。

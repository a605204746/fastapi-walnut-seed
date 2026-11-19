# Vue → React 迁移逐文件清单

> **最新状态（2026-11-19）：现有 Vue 功能迁移实施覆盖 100%，后端菜单组件 14/14。** 角色/公告、仪表盘、共享控件、七种布局与外壳交互均已补齐，工程与样式门禁通过。最新范围、实测结果和联调边界见 [07-React迁移-验收记录](./07-React迁移-验收记录.md)。
>
> 下文是 2026-11-05 至 2026-11-19 的历史计划/阶段记录；其中约 50%、12/14、剩余人日等数字属于当时快照，不代表当前状态。

> 生成日期：2026-11-05 · 源：`walnut-frontend/`（Vben 5.7 fork，Vue 3.5）
> 目标栈（方案 B）：Vite 8 + React 19 + React Router 7 + antd 6 + @ant-design/pro-components + Zustand + TanStack Query + Tailwind 4
> 目标目录：`walnut-frontend-react/`（与三个后端对称，两套前端并存）

本清单覆盖 **1,035 个源文件 / ~95,000 行**。每个文件给出：行数、迁移分类、目标路径、动作说明。

---

## 0. 分类图例

| 代号 | 含义 | 改动量 | 说明 |
|---|---|---|---|
| **A** | 原样搬运 | 0–5% | 纯 TS / 纯资源，copy 过去即可 |
| **B** | 机械替换 | 5–30% | 换 import 或 API 名，逻辑与结构不变 |
| **C** | 重写 | 50–100% | 逻辑/契约保留，渲染层与响应式层重写 |
| **D** | 库替代 | — | React 生态有现成对应物，源码删除 |
| **E** | 丢弃 | — | 死代码、Vue 专有概念、或构建产物 |

⚠️ = 关键路径 / 高风险项 · ⭐ = 高价值可复用资产

---

## 1. 硬契约（迁移前必须锁死）

### 1.1 视图文件路径 = 后端 `sys_menu.component`

三个后端（Python / Java / Go）的种子数据**完全一致**，共 14 条 `component` 值。React 侧必须在
`apps/web-react/src/views/` 下保持**同名同路径**（仅扩展名 `.vue` → `.tsx`），否则前端回退 NotFound：

| # | `sys_menu.component` | 必须存在的文件 | 备注 |
|---|---|---|---|
| 1 | `system/user/index` | `views/system/user/index.tsx` | |
| 2 | `system/user/authRole` | `views/system/user/authRole.tsx` | ⚠️ 源文件仅 6 行、注释写「无实际意义」，但**后端菜单指向它**，必须保留占位 |
| 3 | `system/role/index` | `views/system/role/index.tsx` | |
| 4 | `system/role/authUser` | `views/system/role/authUser.tsx` | ⚠️ 11 行薄包装，转发到 `role-assign/index`；路径不能改 |
| 5 | `system/menu/index` | `views/system/menu/index.tsx` | |
| 6 | `system/dept/index` | `views/system/dept/index.tsx` | |
| 7 | `system/post/index` | `views/system/post/index.tsx` | |
| 8 | `system/dict/index` | `views/system/dict/index.tsx` | 11 行，包 `dict/type/index` |
| 9 | `system/dict/data` | `views/system/dict/data.tsx` | ⚠️ 同 #2，6 行占位但被后端引用 |
| 10 | `system/config/index` | `views/system/config/index.tsx` | 👈 **建议作为第一个 PoC 页** |
| 11 | `system/notice/index` | `views/system/notice/index.tsx` | |
| 12 | `system/client/index` | `views/system/client/index.tsx` | |
| 13 | `monitor/logininfor/index` | `views/monitor/logininfor/index.tsx` | |
| 14 | `monitor/operlog/index` | `views/monitor/operlog/index.tsx` | |

另有 4 种特殊 `component` 字符串由 `router/access.ts` 的转换函数处理，**不产生视图文件**：
`Layout`（一级目录）、`ParentView`（多级目录父级）、`InnerLink`（iframe 内嵌，配 `meta.link`）、`Link`（外链）。

### 1.2 其他不可变契约

| 契约 | 位置 | React 侧要求 |
|---|---|---|
| 响应信封 `{code,msg,data}` | 全部接口 | 拆包逻辑照搬 |
| 分页信封 `{rows,total}` | 全部列表接口 | ⚠️ ProTable 期望 `{data,total,success}`，转换层必须保留（现于 `adapter/vxe-table.ts`） |
| `Authorization: Bearer` + `clientid` 头 | 全部请求 | 照搬 |
| `encrypt-key` 头 + AES body | POST/PUT 且 `encrypt:true` | ⭐ 加密层零改动搬运 |
| 权限串 `模块:实体:操作` | 75 处 `v-access:code` | 改为 `<Access code={[...]}>`，字符串逐字一致 |
| `.env` 中 `VITE_GLOB_*` | 5 个 env 文件 | 变量名不变（含 RSA 公私钥、加密开关、SSE/WS 开关） |
| `window._VBEN_ADMIN_PRO_APP_CONF_` | `internal/vite-config/plugins/extra-app-config.ts` | 运行时配置注入机制保留 |

---

## 2. 仓库基建层（86 文件 / ~6,900 行）

### 2.1 根配置（15 文件）

| 源文件 | 行 | 类 | 动作 |
|---|---|---|---|
| `package.json` | 90 | B | scripts 保留；devDeps：`@vitejs/plugin-vue`→`plugin-react`、`vue-tsc`→`tsc`、删 `vue` |
| `pnpm-workspace.yaml` | 223 | **C** | ⚠️ catalog 需重写：删 `vue/pinia/antdv-next/reka-ui/vue-i18n/vee-validate/@vueuse/*/vue-router/lucide-vue-next/@iconify/vue/vxe-table/vxe-pc-ui/motion-v/@tiptap/vue-3/@tinymce/tinymce-vue/vue-codemirror6/vue-json-*/naive-ui/vue-tippy/vue-eslint-parser/stylelint-config-recommended-vue`；加 `react/react-dom/@types/react/react-router/antd/@ant-design/pro-components/zustand/@tanstack/react-query/@tanstack/react-store/i18next/react-i18next/react-hook-form/@hookform/resolvers/lucide-react/@iconify/react/framer-motion/@tiptap/react/@tinymce/tinymce-react/@uiw/react-codemirror`；`overrides` 里删 `pinia`/`vue` |
| `turbo.json` | 54 | A | 任务名 `@walnut-seed/web-admin#*` → `@walnut-seed/web-react#*` |
| `vitest.config.ts` | 28 | B | `happy-dom` 保留；setup 换 `@testing-library/react` + `@testing-library/jest-dom` |
| `eslint.config.mjs` | 3 | A | |
| `oxfmt.config.ts` | 26 | A | |
| `oxlint.config.ts` | 5 | A | |
| `stylelint.config.mjs` | 4 | B | 去 `stylelint-config-recommended-vue` |
| `cspell.json` | 103 | A | |
| `.browserslistrc` `.npmrc` `.node-version` `.stylelintignore` `.editorconfig` `.gitattributes` `.commitlintrc.js` `.dockerignore` | ~30 | A | 全部照搬 |
| `vben-admin.code-workspace` | — | B | 改名 + 路径更新 |
| `pnpm-lock.yaml` | 20,035 | E | 重新生成 |
| `backup/` | 5 文件 | **E** | 死代码（20260811 旧版 dict drawer 备份） |

### 2.2 `internal/` — 构建与规范基建（72 文件 / 5,301 行）

| 源路径 | 行 | 类 | 动作 |
|---|---|---|---|
| **`tailwind-config/src/theme.css`** | 579 | **A** ⭐ | 主题资产，Tailwind 4 `@theme` 定义，直接搬 |
| `tailwind-config/src/index.ts` + `package.json` + `tsconfig.json` | 45 | A | |
| **`tsconfig/`** base/library/node/web/web-app | 111 | B | `web.json` 去掉 vue 相关 `types`/`jsx: preserve`；改 `jsx: react-jsx` |
| **`node-utils/src/`** 11 文件 | 300 | **A** ⭐ | 纯 Node 工具（fs/git/hash/monorepo/path/spinner/date/formatter） |
| `node-utils/src/__tests__/` 2 文件 | 119 | A | |
| **`vite-config/src/plugins/extra-app-config.ts`** | 92 | **A** ⭐ | 运行时配置注入（RSA 密钥/加密开关），契约不变 |
| `vite-config/src/plugins/importmap.ts` | 245 | A | |
| `vite-config/src/plugins/inject-app-loading/` (index+2 html) | 286 | A | 首屏 loading |
| `vite-config/src/plugins/inject-metadata.ts` | 111 | A | |
| `vite-config/src/plugins/` archiver 75 / html 35 / license 55 / nitro-mock 98 / print 28 / tailwind-reference 40 / dayjs 70 | 401 | A | |
| `vite-config/src/plugins/index.ts` | 261 | B | 去掉 vue 插件注册 |
| **`vite-config/src/plugins/vxe-table.ts`** | 20 | **E** | 不再有 vxe |
| `vite-config/src/config/application.ts` | 125 | B | `plugin-vue` + `plugin-vue-jsx` → `@vitejs/plugin-react` |
| `vite-config/src/config/code-split.ts` | 414 | **B** | ⚠️ 手动分包名单：`vue/vue-router/pinia/antdv-next/vxe-table/reka-ui` → `react/react-dom/react-router/antd/@ant-design/pro-components` |
| `vite-config/src/config/` common 13 / index 37 / library 59 | 109 | B | |
| `vite-config/src/options.ts` 45 / `typing.ts` 364 / `index.ts` 5 / `utils/env.ts` 109 | 523 | B | |
| **`lint-configs/eslint-config/src/configs/`** javascript 182 / jsonc 269 / node 81 / perfectionist 105 / pnpm 38 / typescript 58 / unicorn 44 / yaml 93 / ignores 60 / index 10 | 948 | **A** | |
| `lint-configs/eslint-config/src/` custom-config 158 / index 46 / util 8 / tsdown.config 16 | 228 | A | |
| **`lint-configs/eslint-config/src/configs/vue.ts`** | 151 | **E** | → 新建 `react.ts`（`eslint-plugin-react-hooks` + `eslint-plugin-react-refresh`） |
| **`lint-configs/oxlint-config/src/configs/`** 13 文件 | 530 | **A** | |
| `lint-configs/oxlint-config/src/configs/vue.ts` | 9 | **E** | → `react.ts` |
| `lint-configs/oxfmt-config/src/` + tsdown | 50 | A | |
| `lint-configs/stylelint-config/index.mjs` | 148 | B | 去 vue 相关 extends |
| `lint-configs/commitlint-config/index.mjs` + `.d.ts` | 160 | A | |
| `lint-configs/*/dist/*` | ~1,490 | E | 构建产物 |

### 2.3 `scripts/`（19 文件 / ~1,450 行）

| 源文件 | 行 | 类 | 动作 |
|---|---|---|---|
| `vsh/src/` check-circular 217 / check-dep 147 / publint 184 / lint 76 / code-workspace 78 / index 77 / bin 3 / tsdown 11 | 793 | **A** | |
| `turbo-run/src/` run 67 / index 23 / bin 3 / tsdown 11 | 104 | **A** | |
| `clean.mjs` | 141 | A | |
| `deploy/build-local-docker-image.sh` | 56 | A | |
| `generate-offline-icons.js` | 163 | **B** | iconify 产物从 Vue 组件改 React 组件 |
| `*/dist/*` | ~485 | E | 构建产物 |

---

## 3. 可直接搬运的内核 ⭐（~4,900 行 + ~2,900 行测试）

这一层是本项目最有价值的迁移红利：**纯 TypeScript，零 Vue 依赖，copy 过去就能跑。**

### 3.1 `packages/@core/base/shared/src/` → `packages/@core/shared/src/`

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`store.ts`** | 1 | **B** ⭐ | 全仓唯一改动点：`export * from '@tanstack/vue-store'` → `'@tanstack/react-store'`。FormApi/ModalApi/DrawerApi/GridApi 全靠它 |
| `cache/storage-manager.ts` | 146 | **A** ⭐ | 驱动式异步存储抽象 |
| `cache/indexeddb-driver.ts` | 137 | A | |
| `cache/local-storage-driver.ts` | 70 | A | |
| `cache/memory-storage-driver.ts` | 32 | A | |
| `cache/types.ts` 39 / `index.ts` 5 | 44 | A | |
| **`color/generator.ts`** | 45 | **A** ⭐ | CSS 变量调色板生成器（主题系统核心） |
| **`color/convert.ts`** | 62 | **A** ⭐ | |
| `color/color.ts` 9 / `index.ts` 3 | 12 | A | |
| `constants/globals.ts` 16 / `vben.ts` 30 / `index.ts` 2 | 48 | A | |
| `global-state.ts` | 45 | A | `GlobalShareState` 单例，跨包注入 message 组件 |
| **`utils/tree.ts`** | 125 | **A** ⭐ | `mapTree/filterTree/findNode`，RBAC 菜单树/部门树强依赖 |
| **`utils/download.ts`** | 157 | **A** ⭐ | |
| `utils/inference.ts` | 164 | A | |
| `utils/dom.ts` 107 / `utils/stack.ts` 103 / `utils/diff.ts` 96 / `utils/date.ts` 78 | 384 | A | |
| `utils/state-handler.ts` | 50 | A | Promise 条件等待器（Modal/Drawer lock 依赖） |
| `utils/letter.ts` 47 / `util.ts` 44 / `nprogress.ts` 43 / `window.ts` 37 / `update-css-variables.ts` 35 / `index.ts` 21 / `resources.ts` 21 / `to.ts` 21 / `unique.ts` 15 / `merge.ts` 10 / `cn.ts` 10 | 275 | A | |
| `__tests__/` 13 文件 | 1,470 | A | vitest 配置不变 |

**小计：1,866 行源码（A 1,865 / B 1）+ 1,470 行测试**

### 3.2 `packages/effects/request/src/` → 同名

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`request-client/preset-interceptors.ts`** | 165 | **A** ⭐⭐ | 含 **401 刷新令牌排队重放**（`isRefreshing` + `refreshTokenQueue`）、`{code,data}` 信封拆包、HTTP 状态→i18n 消息 |
| **`request-client/request-client.ts`** | 164 | **A** ⭐ | axios 封装 + `qs` 四种参数序列化 |
| `request-client/modules/sse.ts` | 134 | A | SSE over fetch stream |
| `request-client/types.ts` | 86 | A | |
| `request-client/modules/downloader.ts` | 60 | A | |
| `request-client/modules/uploader.ts` | 42 | A | |
| `request-client/modules/interceptor.ts` | 40 | A | |
| `index.ts` 3 / `request-client/index.ts` 3 | 6 | A | |
| `__tests__` 4 文件 | 510 | A | |

**小计：697 行（全 A）+ 510 行测试。全包 grep 无任何 Vue import。**

### 3.3 `packages/utils/src/` → 同名

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`encryption/base.ts`** | 71 | **A** ⭐⭐ | 非对称/对称加密抽象基类 |
| **`encryption/impl/rsa.ts`** | 30 | **A** ⭐⭐ | jsencrypt |
| **`encryption/impl/aes.ts`** | 28 | **A** ⭐⭐ | crypto-js，ECB + PkCS7 |
| **`encryption/impl/sm2.ts`** 50 / **`sm4.ts`** 43 | 93 | **A** ⭐ | 国密备选实现 |
| `encryption/crypto.ts` | 30 | A | `randomStr(32)` / base64 |
| `encryption/index.ts` | 6 | A | |
| **`helpers/tree.ts`** | 400 | **A** ⭐ | |
| **`helpers/mitt.ts`** | 135 | **A** | mitt 框架无关 |
| `helpers/generate-menus.ts` | 92 | **B** | 菜单树生成逻辑不变，图标字段渲染改 React |
| `helpers/enum-options.ts` | 47 | A | |
| `helpers/find-menu-by-path.ts` | 37 | A | |
| `helpers/uuid.ts` 29 / `merge-route-modules.ts` 28 / `request.ts` 24 / `safe.ts` 10 / `get-popup-container.ts` 10 / `index.ts` 14 | 115 | A/B | `get-popup-container` 改 antd `getPopupContainer` 签名 |
| **`helpers/generate-routes-backend.ts`** | 111 | **C** ⚠️ | 输出类型 `RouteRecordRaw` → RR7 `RouteObject`；过滤逻辑照搬 |
| `helpers/generate-routes-frontend.ts` | 58 | C | 同上 |
| `helpers/unmount-global-loading.ts` | 31 | B | DOM 操作保留，改 React root |
| **`helpers/reset-routes.ts`** | 31 | **E** | RR7 用 `patchRoutes`，无需重置路由 |
| `__tests__/` 5 文件 | 613 | A | |

**小计：1,321 行（A ~830 / B ~130 / C ~170 / E 31）+ 613 行测试**

### 3.4 样式与设计令牌 → `packages/@core/design/src/`

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`design-tokens/default.css`** | 491 | **A** ⭐⭐ | 亮色全套 CSS 变量 |
| **`design-tokens/dark.css`** | 446 | **A** ⭐⭐ | 暗色全套 |
| `css/ui.css` | 101 | A | |
| `css/nprogress.css` | 61 | A | |
| **`css/transition.css`** | 236 | **B** ⚠️ | 类名是 Vue `<Transition>` 约定（`v-enter-from` 等）→ 改为 React 侧类名或交给 framer-motion |
| `css/global.css` 1 / `index.ts` 6 / `design-tokens/index.ts` 2 / `vite.config.ts` 10 | 19 | A | |
| `scss-bem/bem.scss` 34 / `constants.scss` 5 | 39 | A | 配合 `use-namespace` |
| `packages/styles/src/global/index.scss` 1 / `index.ts` 1 / `style-exports.d.ts` 3 | 5 | A | |
| **`packages/styles/src/antdv-next/index.css`** | 80 | **B** | 改写为 antd 6 覆盖样式 |
| `packages/styles/src/antd/index.css` 86 / `ele/index.css` 44 / `naive/index.css` 27 | 157 | **E** | 全仓无引用（已 grep 确认） |

**小计：1,629 行（A 1,257 / B 316 / E 157）**

---

## 4. 移植改造的核心层（~5,900 行）

### 4.1 `packages/@core/preferences/src/`（1,609 行）

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`types.ts`** | 448 | **A** ⭐ | 偏好类型契约 |
| **`config.ts`** | 150 | **A** ⭐ | 默认偏好值 |
| **`constants.ts`** | 116 | **A** ⭐ | `BUILT_IN_THEME_PRESETS` 内置主题预设 |
| **`update-css-variables.ts`** | 129 | **A** ⭐⭐ | 写 ~30 个 CSS 变量到 `documentElement` + 切 `dark` class + `data-theme`。**新增**：映射到 antd 6 `ConfigProvider` theme token |
| **`preferences.ts`** | 463 | **C** ⚠️ | `PreferenceManager` 单例：Vue `reactive/readonly/watch` + `useDebounceFn` → **Zustand `subscribeWithSelector`** + 自写防抖。StorageManager 持久化、`handleUpdates` 管道、`setupWatcher`（breakpoints + `prefers-color-scheme`）逻辑保留 |
| **`use-preferences.ts`** | 280 | **C** | ~25 个 `computed`（isDark/isMobile/layout/showHeader/sidebarWidth/shortcutKeys…）→ Zustand selector + `useMemo` |
| `index.ts` | 23 | A | |
| `packages/preferences/src/index.ts` | 27 | A | 应用层覆盖入口 |
| `__tests__/` | — | B | |

**小计：A 843 / C 743（≈ 52% 可直接搬）**

### 4.2 `packages/stores/src/`（1,191 行）→ Zustand

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`setup.ts`** | 81 | **C** | `pinia-plugin-persistedstate` → `zustand/middleware persist`；key 命名空间 `${namespace}-${storeId}` 保留；⭐ **生产环境 `secure-ls`（AES+压缩）作为 storage adapter 的逻辑照搬**，密钥 `VITE_APP_STORE_SECURE_KEY` 不变 |
| **`modules/tabbar.ts`** | 770 | **C** ⚠️⚠️ | 最大单文件。`cachedRoutes: Map<string,{component: VNode, key, route}>` 的 **VNode 缓存必须重构**为 `<Activity>` + LRU key 集合（见 §7.3）。affix tabs / 拖拽索引 / max-tab-count / 右键菜单动作 / nprogress 集成的**状态语义全部保留** |
| `modules/access.ts` | 129 | **C** | `accessCodes/accessMenus/accessToken/refreshToken/isAccessChecked/loginExpired/isLockScreen/lockScreenPassword` 照搬；`accessRoutes` 类型 `RouteRecordRaw` → `RouteObject` |
| `modules/timezone.ts` | 132 | B | Vue `ref` → Zustand |
| `modules/user.ts` | 72 | B | `userInfo` + `userRoles` |
| `modules/index.ts` 4 / `index.ts` 3 | 7 | A | |
| `__tests__/` tabbar 300 / access 46 / user 37 | 383 | B | |

**小计：A 7 / B 204 / C 980**

### 4.3 `packages/effects/access/src/`（389 行）

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`use-access.ts`** | 63 | **B** ⭐ | `hasAccessByRoles`（`superadmin` 直通）/ `hasAccessByCodes`（`*:*:*` 直通，OR 语义）逻辑照搬，仅换 store |
| **`accessible.ts`** | 235 | **C** ⚠️ | `generateAccessible(mode, options)`：`router.addRoute/removeRoute` + 嵌套到 `Root` → RR7 **`patchRoutes`**。树过滤与 `generateMenus` 调用逻辑保留 |
| **`access-control.vue`** | 47 | **C** | renderless slot 包装 → `<Access code={} role={}>{children}</Access>`，React 侧更简单 |
| **`directive.ts`** | 40 | **E** ⚠️ | Vue 指令无对应物。**75 处 `v-access:code` 调用点需批量改**（见 §6.9 views） |
| `index.ts` | 4 | A | |

### 4.4 `packages/effects/hooks/src/`（864 行）

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`use-app-config.ts`** | 43 | **A** ⭐ | 读 `window._VBEN_ADMIN_PRO_APP_CONF_` / `import.meta.env`，含 `enableEncrypt`、RSA 密钥、SSE/WS 开关 |
| `use-design-tokens.ts` | 321 | **B** | `getComputedStyle` 读 CSS 变量；reactive ref → `useState` + `useSyncExternalStore` |
| `use-hover-toggle.ts` | 163 | B | pointer enter/leave + 延迟 |
| `use-watermark.ts` | 84 | **B** | `@vueuse useWatermark` → 直接用 `watermark-js-plus`（框架无关）或自写 40 行 |
| `use-pagination.ts` | 72 | **D** | → TanStack Query |
| `use-content-maximize.ts` | 24 | B | |
| `use-refresh.ts` | 16 | B | `router.replace` → `useNavigate` |
| **`use-tabs.ts`** | 132 | **C** | router ↔ tabbar store 桥接（close/refresh/pin），依赖 §7.3 的 Activity 方案 |
| `index.ts` | 9 | A | |

### 4.5 `packages/@core/composables/src/`（436 行）→ `packages/@core/hooks/`

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`use-namespace.ts`** | 106 | **B** | BEM 类名生成器（`b()/e()/m()/is()`），纯字符串 + 读 store |
| `use-layout-style.ts` | 84 | B | ResizeObserver → 写 `--vben-content-height/width`，纯 DOM |
| `use-scroll-lock.ts` | 54 | B | body 滚动锁 + 滚动条宽度补偿，纯 DOM |
| `use-simple-locale/index.ts` 26 / `messages.ts` 50 | 76 | **B** | provide/inject → React Context |
| `use-sortable.ts` | 29 | **D** | → `react-sortablejs`（同 core） |
| `use-is-mobile.ts` | 7 | **D** | → `useMediaQuery` |
| **`use-priority-value.ts`** | 93 | **E** ⚠️ | Vue slot/attrs/props/state 优先级概念，React 无对应；调用点内联为 `children ?? prop ?? state` |
| `index.ts` 13 / `__tests__` 48 | 61 | A/B | |

### 4.6 `packages/@core/base/typings/src/`（584 行）

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`menu-record.ts`** | 82 | **A** ⭐⭐ | 后端菜单契约类型，三后端共用，零改动 |
| `helper.d.ts` | 150 | A | |
| `app.d.ts` | 126 | A | |
| `basic.d.ts` | 49 | A | |
| `tabs.ts` | 8 | A | |
| **`vue-router.d.ts`** | 163 | **E** | → 新建 `react-router.d.ts`（`RouteMeta` 声明合并到 RR7 `RouteObject`） |
| `index.ts` | 6 | A | |

### 4.7 `packages/locales/` `constants/` `types/`（~1,300 行）

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`locales/src/langs/{zh-CN,en-US}/*.json`** 10 文件 | 960 | **A** ⭐⭐ | `authentication`(65) / `common`(24) / `preferences`(206) / `profile`(4) / `ui`(166) × 2 语言，直接搬 |
| **`locales/src/i18n.ts`** | 146 | **C** | vue-i18n `createI18n` → `i18next` + `react-i18next` 初始化；`$t` 在 API/util/router 中被直接调用 → 导出 `i18n.t` |
| `locales/src/index.ts` 30 / `typing.ts` 25 | 55 | B | |
| **`constants/src/core.ts`** | 71 | **A** ⭐ | |
| `constants/src/dict-enum.ts` 18 / `index.ts` 3 | 21 | A | |
| `types/src/user.ts` 11 / `index.ts` 2 | 13 | A | |

### 4.8 `packages/icons/` + `@core/base/icons/`（~600 行）

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`icons/src/svg/icons/*.svg`** 20 个 | ~230 | **A** ⭐ | dingding/github/google/qq/wechat/max-key/topiam/snail-job/avatar 等，资源直接搬 |
| `icons/src/svg/index.ts` 37 / `load.ts` 83 | 120 | **B** | `vite-plugin-svg-icons` → `@svgr/react` 或保留同插件 |
| **`icons/src/iconify-offline/offline-icons.ts`** | 258 | **B** | 用 `scripts/generate-offline-icons.js` 重新生成 React 版 |
| `icons/src/iconify/index.ts` | 19 | B | `@iconify/vue` → `@iconify/react` |
| `base/icons/lucide.ts` | 96 | **D** | → `lucide-react` |
| `base/icons/create-icon.ts` | 30 | **E** | Vue `defineComponent` 工厂 |
| `icons/src/icons/empty-icon.vue` | 27 | C | |
| 各 `index.ts` | 40 | A/B | |

---

## 5. UI-Kit 层（~22,900 行，重写主战场）

### 5.1 `@core/ui-kit/shadcn-ui`（174 vue / 78 ts，8,210 行）

| 分组 | 文件数 | 行 | 类 | 说明 |
|---|---|---|---|---|
| **`src/ui/` 33 个原语**：accordion, alert-dialog, avatar, badge, breadcrumb, button, card, checkbox, context-menu, dialog, dropdown-menu, form, hover-card, input, label, number-field, pagination, pin-input, popover, radio-group, resizable, scroll-area, select, separator, sheet, switch, tabs, textarea, toggle, toggle-group, tooltip, tree | ~131 vue | ~5,600 | **D** ⭐ | 全部改用**上游 shadcn/ui (React) CLI 安装**。Vben 的 shadcn-ui 本就是 shadcn-vue（源自 shadcn/ui React），**class 名、CVA variants、Tailwind token 一致**，可对照搬 variant 定义 |
| **`src/components/` 26 个 vben 复合组件**：avatar, back-top, breadcrumb, button(+icon-button/check-button-group), checkbox, collapsible(-params), context-menu, count-to-animator, dropdown-menu, expandable-arrow, full-screen, hover-card, icon, input-captcha, input-password(+强度条), logo, pin-input, popover, scrollbar, segmented, select, spine-text, spinner, tooltip | ~40 vue | ~1,700 | **C** | 每个 50–150 行，逐个重写；量大但复杂度低 |
| **`src/components/render-content/`** | 3 | ~100 | **E** | Vue slot/render-function 适配器；React 用 `children` / render prop 原生解决 |
| `src/**/types.ts` + CVA variant 定义 | 78 ts | 813 | **A/B** ⭐ | 类型与 variant 表是资产 |

### 5.2 `@core/ui-kit/form-ui`（6 vue / 12 ts，2,974 行）

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`types.ts`** | 536 | **B** ⭐ | FormSchema 类型契约；32 个 `useVbenForm` 使用点全依赖它 |
| **`form-api.ts`** | 699 | **C** ⚠️ | `FormApi` 类建在 TanStack Store 上（Store 部分 **B**）；校验委托 vee-validate `FormActions` → **react-hook-form `UseFormReturn`** |
| **`form-render/dependencies.ts`** | 171 | **C** ⚠️ | 字段联动（show/if/disabled/required/rules/trigger）→ RHF `watch` + 条件渲染 + zod refine |
| `form-render/expandable.ts` | 105 | C | 折叠/展开行 |
| `config.ts` | 87 | B | |
| `use-form-context.ts` 109 / `form-render/context.ts` 24 | 133 | **B** | provide/inject → Context（`componentMap` 注册表） |
| `form-render/helper.ts` | 60 | B | |
| **`use-vben-form.ts`** | 51 | **C** ⭐ | ⚠️ `useVbenForm(options) → [FormComponent, formApi]` **签名必须保留**（32 处使用点）；Vue `defineComponent + h()` → 返回 JSX 元素 |
| `form-render/form-field.vue` | 464 | **D** | ProForm `valueType` 机制覆盖大部分；自定义渲染用 RHF `Controller` |
| `form-render/form.vue` 192 / `form-label.vue` 32 | 224 | D | |
| `components/form-actions.vue` | 184 | C | 提交/重置按钮区 |
| `vben-use-form.vue` 151 / `vben-form.vue` 79 | 230 | C | |
| `field-name.ts` 14 / `index.ts` 13 | 27 | A | |

### 5.3 `@core/ui-kit/popup-ui`（3 vue / 15 ts，2,764 行）— 最易迁移的 UI-Kit 包

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`modal/modal-api.ts`** | 201 | **B** ⭐ | TanStack Store + `StateHandler`，换 `react-store` 即可 |
| **`drawer/drawer-api.ts`** | 188 | **B** ⭐ | 同上 |
| `modal/modal.ts` 199 / `drawer/drawer.ts` 183 | 382 | **B** | 配置项类型契约 |
| **`use-modal.ts`** 168 / **`use-drawer.ts`** 152 | 320 | **C** | `connectedComponent` 挂载机制 → React portal / JSX 引用 |
| `modal/modal.vue` 372 / `drawer/drawer.vue` 339 | 711 | **D** | → antd `Modal` / `Drawer`（或 Radix Dialog + 自绘） |
| **`alert/AlertBuilder.ts`** | 246 | **C** ⚠️ | `createVNode + render` 命令式挂载 → `createRoot` 或 antd `App.useApp().modal.confirm` |
| `alert/alert.ts` | 101 | B | |
| `alert/alert.vue` | 219 | D | |
| `modal/use-modal-draggable.ts` | 141 | C | 拖拽 → framer-motion `drag` |
| `index.ts` × 5 | 24 | A | |
| `__tests__/` modal-api 117 / drawer-api 115 | 232 | B | |

### 5.4 `@core/ui-kit/layout-ui`（8 vue / 6 ts，1,663 行）

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`vben-layout.ts`** | 180 | **B** ⭐ | props 契约 + 6 种布局模式定义，是资产 |
| **`vben-layout.vue`** | 634 | **D** | ProLayout 覆盖（方案 C 则为 C：z-index 分层 / fixed-auto header / 折叠宽度 / 移动端遮罩的 CSS 变量计算逻辑照搬） |
| `components/layout-sidebar.vue` | 381 | D/C | |
| `components/layout-header.vue` 77 / `layout-content.vue` 65 / `layout-footer.vue` 44 / `layout-tabbar.vue` 30 | 216 | C/D | |
| **`hooks/use-sidebar-drag.ts`** | 152 | **C** | 侧边栏拖拽调宽，需自写 pointer 事件 |
| `hooks/use-layout.ts` | 53 | B | |
| `components/widgets/sidebar-collapse-button.vue` 19 / `sidebar-fixed-button.vue` 19 | 38 | C | |
| `index.ts` × 3 | 9 | A | |

### 5.5 `@core/ui-kit/menu-ui`（10 vue / 10 ts，2,243 行）

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`types.ts`** | 154 | **A** ⭐ | 菜单数据结构 |
| **`components/menu.vue`** | 884 | **D** | ProLayout 自带菜单（方案 C：C，含 popper 定位折叠子菜单） |
| `components/sub-menu.vue` 276 / `menu-item.vue` 138 / `sub-menu-content.vue` 105 | 519 | D | |
| **`components/collapse-transition.vue`** | 96 | **E** | Vue `<Transition>` JS 钩子 → CSS transition |
| `components/menu-badge.vue` 57 / `menu-badge-dot.vue` 28 | 85 | C | → antd `Badge` |
| `components/normal-menu/` (vue 162 + ts 27 + index 2) | 191 | D | |
| `hooks/use-menu-context.ts` 55 / `use-menu.ts` 48 / `use-menu-scroll.ts` 46 | 149 | C | provide/inject → Context |
| `utils/index.ts` 52 / `menu.vue` 32 / `sub-menu.vue` 71 / `index.ts` × 2 | 161 | C | |

### 5.6 `@core/ui-kit/tabs-ui`（6 vue / 6 ts，927 行）

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`types.ts`** | 73 | **A** ⭐ | |
| **`use-tabs-view-scroll.ts`** | 202 | **B** | 滚轮横向滚动，纯 DOM |
| `use-tabs-drag.ts` | 123 | **D** | → `react-sortablejs` |
| **`components/tabs-chrome/tabs.vue`** | 211 | **C** ⚠️ | Chrome 风格 SVG 曲率页签，**视觉资产**，需手工重画 JSX + SVG path |
| `components/tabs/tabs.vue` 148 / `tabs-view.vue` 107 | 255 | C | |
| `components/widgets/tool-more.vue` 18 / `tool-refresh.vue` 18 / `tool-screen.vue` 19 | 55 | C | |
| `index.ts` × 3 | 8 | A | |

---

## 6. `packages/effects/` 层（~24,000 行）

### 6.1 `common-ui/src/components/`（17 组件，~4,900 行）

| 组件 | 文件(行) | 类 | React 目标 |
|---|---|---|---|
| **`resize/`** | resize.vue **1,269** | **D** ⚠️ | → antd 6 `Splitter` 或 `react-resizable-panels`（省下最大一块工作量） |
| **`cropper/`** | cropper.vue **981** | **D** | → `react-cropper`（同 cropperjs core） |
| **`captcha/`** | 8 文件 **1,131**：types 209 / slider-translate 311 / slider 251 / slider-rotate 213 / point-selection 175+84 / bar 40 / content 52 / action 63 / hooks 19 | **C** | canvas + DOM 计算逻辑可搬，UI 重写（滑块/旋转/点选三种） |
| **`api-component/`** | vue 253 / types 72 / index 7 | **C** ⭐ | ApiSelect/ApiTreeSelect/ApiCascader 异步选项；`adapter/component` 中 6 处依赖 → TanStack Query + antd Select/TreeSelect/Cascader |
| **`icon-picker/`** | vue 300 / icons 56 / types 22 | **C** | |
| **`ellipsis-text/`** | vue 232 | **B** | 测量逻辑纯 DOM |
| **`markdown/`** | editor 136 / preview 102 | **B** | `vditor` 框架无关，换 React 挂载 |
| **`json-viewer/`** | vue 128 / types 44 | **D** | → `@microlink/react-json-view` |
| **`count-to/`** | vue 123 / types 53 | **B** | rAF 动画逻辑可搬 |
| **`col-page/`** | vue 111 / types 26 | **C** | 左右分栏页容器 |
| **`page/`** | vue 106 / types 23 / test 88 | **C** ⭐ | `Page` 容器，业务页大量使用（`auto-content-height` 等） |
| **`tippy/`** | directive 100 / index 67 | **D/E** | → antd `Tooltip`/`Popover` 或 `@tippyjs/react`；Vue 指令部分 **E** |
| **`code-mirror/`** | vue 66 / data 24 | **D** | → `@uiw/react-codemirror` |
| **`loading/`** | directive 132 / vue 39 / spinner 28 | **E** | Vue 指令 → antd `Spin` / React `Suspense` |
| **`json-preview/`** | vue 18 | **D** | |
| **`tree/`** | vue 25 | **C** | → antd `Tree` |
| `index.ts` | 45 | B | |

### 6.2 `common-ui/src/ui/`（~3,100 行）

| 分组 | 文件(行) | 类 | 说明 |
|---|---|---|---|
| **`authentication/`** | login 204 / code-login 129 / register 121 / types 117 / forget-password 114 / dingding-login 113 / qrcode-login 102 / login-expired-modal 95 / third-party-login 41 / auth-title 13 / index 12 | **C** | ⚠️ 与 `apps/src/views/_core/authentication/` **功能重叠**，迁移时二选一（建议保留 apps 层定制版，删 common-ui 版） |
| **`fallback/`** | fallback.vue 162 / fallback.ts 25 / index 2 | **C** | → antd `Result` |
| **`fallback/icons/`** | icon-403 151 / icon-404 154 / icon-500 215 / icon-coming-soon 262 / icon-offline 112 = **894** | **B** ⭐ | 纯 SVG，转 JSX 是机械操作（可用 SVGR 批量） |
| **`dashboard/analysis/`** | chart-card 24 / charts-tabs 40 / overview 55 / index 3 | **C** | |
| **`dashboard/workbench/`** | header 46 / project 65 / quick-nav 56 / todo 63 / trends 64 / index 5 | **C** | |
| `dashboard/typing.ts` 48 / `index.ts` 3 | 51 | A | |
| **`profile/`** | profile 62 / base-setting 58 / password-setting 59 / security-setting 53 / notification-setting 53 / types 21 / index 6 | **C** | 与 `apps/src/views/_core/profile/` 重叠，同上二选一 |
| **`about/`** | vue 181 / ts 14 / index 1 | **C** | |

### 6.3 `layouts/src/`（71 vue / 31 ts，12,652 行）⚠️ 最重的一层

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`authentication/icons/slogan.vue`** | **4,568** | **B** ⭐ | 全仓最大文件，但是**纯 SVG 登录页插画** → SVGR 机械转 JSX，最低风险最高回报 |
| **`basic/layout.vue`** | 441 | **D/C** | ProLayout 覆盖 |
| **`basic/menu/`** | use-mixed-menu 173 / use-extra-menu 133 / use-navigation 74 / menu.vue 45 / mixed-menu 43 / extra-menu 41 / index 5 | **C** ⚠️ | 441 行，6 种布局模式的混合导航核心逻辑 |
| **`basic/tabbar/`** | use-tabbar.ts 227 / tabbar.vue 84 | **C** ⚠️ | 依赖 §7.3 Activity 方案 |
| **`basic/header/header.vue`** | 225 | **C** | |
| **`basic/content/`** | content.vue 88 / use-content-spinner.ts 50 / content-spinner.vue 12 | **C** | |
| **`route-cached/`** | route-cached-view.vue 98 / route-cached-page.vue 36 / index 2 | **E → 重建** ⚠️ | Vue `<KeepAlive>` 无对应物，重建为 `<Activity>` LRU 容器（§7.3） |
| **`iframe/`** | iframe-router-view.vue 84 / iframe-view.vue 3 / index 2 | **C** | `InnerLink` 菜单类型依赖 |
| `authentication/` authentication.vue 217 / form.vue 38 / toolbar.vue 49 / index 2 / types 1 | 307 | C | |
| `basic/copyright/copyright.vue` 48 / `basic/footer/footer.vue` 11 | 59 | B | |
| `hooks/index.ts` | 98 | B | |
| `basic/index.ts` 各 | ~10 | A | |

### 6.4 `layouts/src/widgets/`（63 文件，~4,700 行）

| 分组 | 文件(行) | 类 | 说明 |
|---|---|---|---|
| **`preferences/preferences-drawer.vue`** | **577** | **C** ⚠️ | 偏好设置抽屉，主题系统总入口 |
| **`preferences/blocks/`** 24 文件 | ~1,400：layout.vue 109 / sidebar 106 / tabbar 102 / custom 121 / theme 113 / builtin 160 / header 74 / widget 79 / general 77 / select-item 74 / number-field-item 74 / input-item 71 / checkbox-item 63 / font-size 62 / breadcrumb 56 / switch-item 53 / animation 54 / content 52 / navigation 45 / button-wave-mode 43 / radius 38 / color-mode 26 / block 22 / footer 17 / copyright 44 / shortcut-keys-global 55 | **C** | 可用 ProForm 大幅简化（都是 switch/select/number/checkbox 项） |
| **`preferences/icons/`** 9 文件 | **1,296**：header-mixed-nav 202 / sidebar-mixed-nav 173 / mixed-nav 161 / header-sidebar-nav 177 / sidebar-nav 153 / header-nav 119 / content-compact 119 / full-content 50 / setting 12 | **B** ⭐ | 布局示意 SVG，机械转 JSX |
| `preferences/` preferences.vue 91 / preferences-button 30 / use-open-preferences 16 / index 3 | 140 | C | |
| **`global-search/`** | search-panel.vue 299 / global-search.vue 159 | **C** | Cmd+K 模糊路由搜索 |
| **`user-dropdown/`** | user-dropdown.vue 304 | **C** | |
| **`notification/`** | notification.vue 208 / types.ts 20 | **C** ⚠️ | SSE 通知面板；**连接必须提到 service 层**以兼容 `<Activity>` hidden 清理 effect（§7.3） |
| **`lock-screen/`** | lock-screen.vue 167 / lock-screen-modal.vue 110 | **C** | |
| **`theme-toggle/`** | theme-button.vue 164 / theme-toggle.vue 82 | **B** | |
| **`check-updates/`** | check-updates.vue 134 | **B** | `version-polling` 框架无关 |
| **`timezone/`** | timezone-button.vue 87 | **C** | |
| `breadcrumb.vue` | 71 | **D** | ProLayout 自带 |
| `color-toggle.vue` 63 / `layout-toggle.vue` 61 / `language-toggle.vue` 38 | 162 | B/C | |
| `index.ts` × 6 | ~20 | A | |

### 6.5 `plugins/`（2 vue / 16 ts，2,155 行 + 165 css）

| 文件 | 行 | 类 | 说明 |
|---|---|---|---|
| **`vxe-table/use-viewed-row.ts`** | **537** | **C** ⚠️⚠️ | fork 自研「已读/未读行高亮」+ IndexedDB/localStorage 持久化适配器，**无现成替代，必须重写** |
| **`vxe-table/use-vxe-grid.vue`** | 520 | **D** | ProTable 覆盖（查询表单 + 工具栏 + 分页 + loading） |
| **`vxe-table/api.ts`** | 184 | **C** | `VxeGridApi`（TanStack Store）→ 重建为 `GridApi`；Store 部分 **B** |
| **`vxe-table/types.ts`** | 187 | **B** ⭐ | 列/表单/工具栏配置类型，作为 ProTable 列类型的参照 |
| `vxe-table/init.ts` | 154 | **E** | `VxeUI.setConfig` + `plugin-render-antd` 注册 + 主题同步 watcher |
| `vxe-table/extends.ts` | 80 | **C** | `CellImage` / `CellLink` 自定义渲染器 → ProTable `render` |
| `vxe-table/use-vxe-grid.ts` | 84 | **C** | ⚠️ `useVbenVxeGrid(options) → [Grid, gridApi]` **签名必须保留**（18 处使用点） |
| `vxe-table/style.css` | 165 | **E** | |
| `vxe-table/index.ts` 10 / `README.md` 50 | 10 | B | |
| **`echarts/use-echarts.ts`** | 204 | **B** ⭐ | 主题联动（暗色 + 内置主题色）init/resize/dispose 逻辑保留 |
| `echarts/echarts.ts` 49 / `types.ts` 28 / `echarts-ui.vue` 15 / `index.ts` 4 | 96 | **B/D** | → `echarts-for-react` 或 80 行自建 hook |
| `motion/` index 8 / types 26 / README 26 | 34 | **D** | → `framer-motion` |
| `plugins-context.ts` / `types.ts` / `index.ts` | ~60 | B | |

---

## 7. 三个高风险项的详细方案

### 7.1 vxe-table → ProTable（18 使用点 / 1,775 行 wrapper）

| 现有能力 | ProTable 对应 | 缺口 |
|---|---|---|
| `proxyConfig` 自动请求 | `request` prop | ✅ 语义几乎 1:1 |
| 分页信封 `{rows,total}` → `{result,page}` | `request` 返回值转 `{data,total,success}` | ⭐ `adapter/vxe-table.ts` 的转换逻辑照搬 |
| 查询表单（form-ui 联动） | `search` + `columns[].valueType` | ⚠️ Vben 的 schema 是独立 `formOptions`，需写适配层 |
| 工具栏（刷新/密度/列设置/全屏） | `options` prop | ✅ 内置 |
| 树表格（menu/dept 页） | antd Table `childrenColumnName` | ✅ |
| 可编辑单元格 | `editable` | ✅ |
| 虚拟滚动 | antd 6 `virtual` | ✅ |
| `CellImage` / `CellLink` 渲染器 | `columns[].render` | 需重写（小） |
| `addSortParams` 排序参数 | `request(params, sort, filter)` | ✅ |
| **`use-viewed-row`（537 行）** | **无** | ⚠️ 必须手工重写 |

### 7.2 后端动态菜单 → React Router 7

```
后端 GET /system/menu/getRouters
  → backMenuToRouteObjects()      ← 现 backMenuToVbenMenu，判断逻辑照搬
      · Layout / ParentView / Link / InnerLink / RootMenu 五种 component 字符串
  → Zustand accessStore.routeTree
  → router.patchRoutes()          ← 现 router.addRoute/removeRoute
  → 组件解析：import.meta.glob('../views/**/*.tsx') + React.lazy
      （与现有 '.vue' glob 完全同构）
  → 守卫：guard.ts 全局前置 → loader + <RequireAuth> 布局路由
```

**必须保留的时序**：登录页独立挂载 → 拿 token → `getInfo` 存 permissions → `getRouters` → `patchRoutes` → 进主布局。

### 7.3 KeepAlive → React 19.2 `<Activity>` + LRU

```tsx
// route-cached 的重建形态
<Activity mode={isActive ? 'visible' : 'hidden'}>
  <TabContent route={route} />
</Activity>
// 超过 maxCachedTabs 的 tab 真正卸载（LRU 淘汰）
```

⚠️ **必须配套改造**：`<Activity mode="hidden">` 会**清理 effect**。现有 `apps/src/store/notify.ts`（136 行）在 store 层用 `useEventSource`/`useWebSocket` 收 SSE，`layouts/widgets/notification` 也直接订阅。
→ **把连接与消息缓存提到独立的 `services/notification.ts`（模块级单例，非组件 effect）**，页面/widget 只做展示层订阅。这同时是架构改进。

备选：`keepalive-for-react`（支持 `max` / LRU / transition）。**不推荐** `react-activation`（需 babel 插件、与 StrictMode 冲突）。

---

## 8. `apps/web-antd/src/` → `apps/web-react/src/`（231 文件 / 24,899 行）

### 8.1 应用入口与根（12 文件）

| 源文件 | 行 | 类 | 目标 / 动作 |
|---|---|---|---|
| **`.env` `.env.development` `.env.production` `.env.test` `.env.analyze`** | — | **A** ⭐⭐ | 变量名全保留（`VITE_GLOB_ENABLE_ENCRYPT` / `VITE_GLOB_RSA_PUBLIC_KEY` / `VITE_GLOB_RSA_PRIVATE_KEY` / SSE·WS 开关 / `VITE_APP_STORE_SECURE_KEY`） |
| **`preferences.ts`** | 117 | **A** ⭐ | 应用级偏好覆盖（含 `accessMode: 'mixed'`），纯配置 |
| `main.ts` | 31 | B | `createApp().mount()` → `createRoot().render()` |
| `bootstrap.ts` | 85 | B | 初始化顺序保留：i18n → store → preferences → router |
| **`app.vue`** | 79 | **C** | → `App.tsx`：`ConfigProvider`(antd locale + theme token) + `AntdApp` + `QueryClientProvider` + `RouterProvider` + preferences 订阅 |
| `vite.config.ts` | 25 | B | |
| `index.html` 22 / `loading.html` 58 | 80 | A | |
| `package.json` | 66 | B | |
| `tsconfig.json` 11 / `tsconfig.node.json` 10 | 21 | B | |
| `public/tinymce/**` | — | A | 静态资源 |
| `types/` | — | B | |
| `dist/` `dist.zip` `.turbo/` | — | E | 构建产物 |

### 8.2 `adapter/`（3 文件 / 560 行）— Vben 解耦层，迁移的枢纽

| 源文件 | 行 | 类 | 动作 |
|---|---|---|---|
| **`component/index.ts`** | 350 | **C** ⚠️⭐ | 31 个表单组件注册表：`Input/Select/DatePicker/ApiSelect/ApiTreeSelect/ApiCascader/TreeSelect/Cascader/Upload/ImageUpload/FileUpload/RichTextarea/IconPicker/...`。React 侧改为 `componentMap: Record<ComponentType, React.ComponentType>`；`defineAsyncComponent` → `React.lazy`；`withDefaultPlaceholder` HOC 保留；**`ComponentType` / `ComponentPropsMap` 类型名必须保留**（所有 `data.ts(x)` 引用） |
| **`form.ts`** | 56 | **B** | `setupVbenForm` 配置：`baseModelPropName`/`modelPropNameMap`（`value`/`checked`/`fileList`/`modelValue`）是 **v-model 概念 → React 受控组件统一 `value`+`onChange`，此文件大幅简化**；`defineRules.required/selectRequired` 的 i18n 规则照搬 |
| **`vxe-table.ts`** | 154 | **C** ⚠️ | → ProTable 全局配置。**必须保留语义**：分页信封 `{rows,total}` 转换、`CellImage`/`CellLink`、`vxeCheckboxChecked`、`addSortParams`、row-key 处理；导出 `useVbenVxeGrid` 同名同签名 |

### 8.3 `api/`（40 文件 / 2,090 行）— **全 B，机械替换 import 路径** ⭐

| 分组 | 文件(行) | 类 |
|---|---|---|
| 公共 | `common.d.ts` 42 / `helper.ts` 97 / `index.ts` 1 | **A/B** |
| `core/` | `auth.ts` 158 / `captcha.ts` 42 / `menu.ts` 45 / `upload.ts` 51 / `user.ts` 46 / `index.ts` 4 | **B** |
| `monitor/` | `logininfor/index.ts` 60 + `model.d.ts` 13 / `operlog/index.ts` 47 + `model.d.ts` 21 / `index.ts` 2 | **B** |
| `system/user/` | `index.ts` 171 / `model.d.ts` 117 | **B / A** |
| `system/role/` | `index.ts` 166 / `model.d.ts` 29 | B / A |
| `system/menu/` | `index.ts` 80 / `model.d.ts` 57 | B / A |
| `system/dict/` | `dict-type.ts` 85 / `dict-type-model.d.ts` 8 / `dict-data.ts` 75 / `dict-data-model.d.ts` 17 / `index.ts` 2 | B / A |
| `system/post/` | `index.ts` 85 / `model.d.ts` 12 | B / A |
| `system/config/` | `index.ts` 76 / `model.d.ts` 9 | B / A |
| `system/client/` | `index.ts` 75 / `model.d.ts` 12 | B / A |
| `system/profile/` | `index.ts` 65 / `model.d.ts` 75 | B / **A** |
| `system/dept/` | `index.ts` 62 / `model.d.ts` 19 | B / A |
| `system/notice/` | `index.ts` 52 / `model.d.ts` 11 | B / A |
| `system/social/` | `index.ts` 25 / `model.d.ts` 26 | B / A |
| `system/index.ts` | 11 | A |

> **全部 `model.d.ts`（~430 行）为 A 类零改动。** `index.ts` 函数体只需把 `#/utils/http` 的 import 指向新 axios 客户端；`encrypt: true` 选项语义不变。

### 8.4 `components/`（10 组 / ~5,380 行）

| 文件 | 行 | 类 | 动作 |
|---|---|---|---|
| **`tiptap/src/toolbar.ts`** | 539 | **B** ⭐ | 工具栏定义，tiptap core 不变 |
| **`tiptap/src/resizable-image.ts`** | 240 | **A** ⭐ | tiptap extension，**框架无关** |
| `tiptap/src/editor.vue` | 296 | **D** | → `@tiptap/react` `useEditor` + `EditorContent` |
| `tiptap/src/upload.ts` | 196 | **B** | 上传逻辑（OSS/S3）保留 |
| `tiptap/src/styles.ts` 78 / `extensions.ts` 50 / `text-style.ts` 50 / `helpers.ts` 34 / `type.ts` 29 / `helper.ts` 14 / `index.ts` 11 | 266 | **A/B** | `extensions.ts`/`text-style.ts` 为 A |
| **`tree/src/menu-select-table.vue`** | 406 | **C** ⚠️ | 角色菜单授权矩阵（vxe-grid + 树 + 展开收起 + 全选），依赖 §7.1 |
| `tree/src/tree-select-panel.vue` | 192 | **C** | → antd `Tree` |
| `tree/src/helper.tsx` | 182 | **C** | 含 `useVbenVxeGrid` 引用 |
| `tree/src/data.tsx` 81 / `hook.tsx` 62 / `index.ts` 2 | 145 | **B** | |
| **`upload/src/hook.ts`** | 357 | **C** | 上传核心逻辑（分片/进度/OSS 直传）保留，改 React hook |
| `upload/src/image-upload.vue` 196 / `file-upload.vue` 151 | 347 | **C** | → antd `Upload` |
| **`upload/src/props.d.ts`** | 125 | **A** ⭐ | |
| `upload/src/helper.ts` 28 / `index.ts` 2 | 30 | A | |
| **`cropper/src/cropper-modal.vue`** | 382 | **C** | |
| `cropper/src/cropper.vue` | 207 | **D** | → `react-cropper`（同 core） |
| `cropper/src/cropper-avatar.vue` | 168 | **C** | |
| `cropper/src/typing.ts` 8 / `index.ts` 3 | 11 | A | |
| **`tinymce/src/editor.vue`** | 268 | **D** | → `@tinymce/tinymce-react`（配置照搬） |
| `tinymce/src/helper.ts` 14 / `tinymce.ts` 11 / `type.d.ts` 7 / `index.ts` 1 | 33 | **A** | |
| **`walnut-slogan/src/walnut-slogan.vue`** | 236 | **B** ⭐ | 纯 SVG + CSS 动画，机械转 JSX |
| `global/button-wave.ts` | 149 | **D** | `@antdv-next/happy-work-theme` 波纹 → antd React `Button` 自带 wave |
| `global/api-switch.vue` | 127 | **C** | |
| `global/button.ts` 26 / `index.ts` 16 | 42 | **C/B** | |
| `global/slot.ts` | 27 | **E** | Vue slot 工具 |
| **`dict/src/index.vue`** | 142 | **C** | `DictTag`，被大量列表页使用 |
| `dict/src/data.tsx` 44 / `type.d.ts` 7 / `index.ts` 2 | 53 | **B/A** | |
| `tenant-toggle/src/index.vue` | 188 | **C** | 多租户切换 |
| `tenant-toggle/index.ts` | 1 | A | |
| `table/src/options-tag.vue` 21 / `index.ts` 1 | 22 | **C/A** | |

### 8.5 `layouts/`（3 文件 / 236 行）

| 文件 | 行 | 类 | 动作 |
|---|---|---|---|
| `basic.vue` | 183 | **C** | `BasicLayout` 组装 + widgets 挂载点 |
| `auth.vue` | 47 | **C** | 认证布局（登录页外壳） |
| `index.ts` | 6 | A | |

### 8.6 `locales/`（15 文件 / 475 行）

| 文件 | 行 | 类 | 动作 |
|---|---|---|---|
| **`langs/{zh-CN,en-US}/*.json`** 14 文件 | 372 | **A** ⭐⭐ | `common`(7) `component`(59) `demos`(14) `http`(7) `menu`(55) `page`(16) `pages`(28) × 2 |
| **`index.ts`** | 103 | **C** | vue-i18n → i18next；`import.meta.glob('./langs/**/*.json')` 懒加载保留；**antdv-next locale + dayjs locale 切换 → antd `ConfigProvider` locale + dayjs** |

### 8.7 `router/`（6 文件 / 630 行）⚠️ 关键路径

| 文件 | 行 | 类 | 动作 |
|---|---|---|---|
| **`access.ts`** | 239 | **C** ⚠️⭐ | `backMenuToVbenMenu()` → `backMenuToRouteObjects()`：**`Layout`/`ParentView`/`Link`/`InnerLink`/`RootMenu` 五种字符串的判断逻辑逐行照搬**，只改输出类型；`pageMap = import.meta.glob('../views/**/*.vue')` → `'../views/**/*.tsx'`；`layoutMap = {BasicLayout, IFrameView, NotFoundComponent}` 保留；`forbiddenComponent` 兜底保留 |
| **`guard.ts`** | 143 | **C** | 全局前置守卫 → RR7 `loader` + `<RequireAuth>`；nprogress 保留；`redirect` query 参数保留 |
| `routes/core.ts` | 105 | **B** | 静态路由：根 / 登录+auth 布局 / `/social-callback` / 404 catch-all |
| `routes/index.ts` | 39 | **B** | `import.meta.glob('./modules/**/*.ts')` |
| `routes/modules/dashboard.ts` 36 / `profile.ts` 19 | 55 | **B** | 仅这 2 个静态模块，其余全后端驱动 |
| `index.ts` | 49 | **C** | `createRouter` → `createBrowserRouter` + `patchRoutes` |

### 8.8 `store/`（6 文件 / 469 行）

| 文件 | 行 | 类 | 动作 |
|---|---|---|---|
| **`notify.ts`** | 136 | **C** ⚠️ | SSE/WebSocket 通知。`useEventSource`/`useWebSocket`(@vueuse) → 原生 `EventSource`/`WebSocket`，且**必须提到组件外的 service 单例**（§7.3） |
| **`auth.ts`** | 159 | **B** | `login/logout/fetchUserInfo`；`accessStore.setAccessCodes()` 时机不变 |
| **`dict.ts`** | 112 | **C** | 字典选项缓存 + 请求去重 → **TanStack Query**（`queryKey: ['dict', type]`，天然去重+缓存，是升级） |
| `tenant.ts` | 44 | B | |
| `loading.ts` | 16 | B | |
| `index.ts` | 2 | A | |

### 8.9 `utils/`（16 文件 / 1,530 行）

| 文件 | 行 | 类 | 动作 |
|---|---|---|---|
| **`http/index.ts`** | 358 | **C** ⚠️⭐ | 核心。**alova 实例 → axios**（复用 §3.2 的 `RequestClient`）。**必须原样保留的逻辑**：① `encrypt:true` 时 `randomStr(32)` → `encrypt-key` 头 = `RSA_pub(base64(key))` → body = `AES(JSON.stringify(data), key)` → `transformRequest: d=>d`；② 响应侧 `encrypt-key` 头存在则 `RSA_priv` → base64 decode → `AES_decrypt` → `JSON.parse`；③ `clientid` 头 / `Accept-Language`；④ 业务码拆包；⑤ 401 登出；⑥ `get/post/put/delete` + `*WithMsg` 变体 |
| **`http/checkStatus.ts`** | 51 | **A** ⭐ | HTTP 状态 → i18n 消息 |
| **`http/exception.ts`** | 21 | **A** | |
| `http/helper.ts` | 77 | **B** | 未授权登出（触碰 store + router） |
| `http/popup.ts` | 49 | **D** | → antd `App.useApp()` / `message` 静态方法 |
| **`file/download.ts`** | 129 | **A** ⭐ | 纯 Blob/DOM |
| **`file/base64Conver.ts`** | 46 | **A** ⭐ | |
| **`file/index.ts`** | 31 | **A** | 文件大小格式化 |
| `file/export.tsx` | 76 | **B** | alova `useRequest` + Vue `ref` → React hook |
| **`render.tsx`** | 237 | **C** | Vue VNode 渲染助手（单元格）→ JSX |
| `popup.ts` | 126 | **D** | `window.message/modal/notification` 全局挂载 → antd `App` context |
| `message.ts` | 93 | **C** | SSE/WS，@vueuse → 原生 |
| `modal.tsx` | 78 | **D** | → antd `Modal.confirm` |
| `check-update.tsx` | 64 | **B** | `version-polling` 框架无关 |
| `dict.ts` | 53 | **B** | 字典 hook |
| **`context.tsx`** | 37 | **E** | provide/inject → React Context |

### 8.10 `views/`（94 文件 / 10,023 行）

> **模式统一**：几乎每个 CRUD 屏 = `data.ts(x)`（列 + 表单 schema）→ `index.vue`（grid + toolbar）→ `*-drawer.vue`/`*-modal.vue`（编辑表单）。
> **分类规律**：`data.ts(x)` = **B**（schema 结构照搬，组件名字符串指向新 `componentMap`）；`index.vue` = **C**（`useVbenVxeGrid` → `useGrid`）；`*-drawer/modal.vue` = **C**（`useVbenForm` + `useVbenModal/Drawer` → RHF + antd Drawer/Modal）。
> ⚠️ 全部 `index.vue` 含 `v-access:code` → 改 `<Access code={[...]}>`。

#### `_core/`（29 文件 / ~1,570 行）

| 文件 | 行 | 类 | 备注 |
|---|---|---|---|
| **`authentication/login.vue`** | 205 | **C** ⭐ | 登录 + 验证码 + 租户 + 记住我；**建议第 2 个 PoC** |
| `authentication/code-login.vue` | 144 | C | |
| `authentication/register.vue` | 95 | C | |
| `authentication/input-captcha.vue` | 78 | C | |
| `authentication/oauth-login.vue` | 44 | C | |
| `authentication/forget-password.vue` | 42 | C | |
| `authentication/qrcode-login.vue` | 10 | C | |
| **`oauth-common.tsx`** | 92 | **B** | OAuth token 处理逻辑 |
| `social-callback/index.vue` | 88 | C | 路由 `/social-callback` |
| **`profile/components/account-bind.vue`** | 152 | C | |
| `profile/components/base-setting.vue` | 119 | C | |
| `profile/components/secure-setting.vue` | 104 | C | |
| `profile/profile-panel.vue` | 95 | C | |
| `profile/base-setting.vue` 65 / `password-setting.vue` 61 / `index.vue` 54 / `security-setting.vue` 43 / `notification-setting.vue` 31 / `setting-panel.vue` 36 | 290 | C | |
| `profile/mitt.ts` | 7 | **A** | |
| **`fallback/`** 5 文件 | 43 | **D** | 每个 7–9 行，→ antd `Result` 或复用 common-ui fallback |
| `about/index.vue` | 9 | **B** | 仅包 `<About />` |

#### `dashboard/`（7 文件 / 698 行）

| 文件 | 行 | 类 | 备注 |
|---|---|---|---|
| `workspace/index.vue` | 266 | **C** | 卡片 + 快捷导航 |
| `analytics/index.vue` | 90 | **C** | |
| `analytics/analytics-trends.vue` | 97 | **B** ⭐ | ECharts `option` 配置照搬，`EchartsUI`/`useEcharts` → React hook |
| `analytics/analytics-visits-data.vue` 81 / `-visits-source.vue` 64 / `-visits.vue` 54 / `-visits-sales.vue` 46 | 245 | **B** ⭐ | 同上 |

#### `monitor/`（6 文件 / 849 行）

| 文件 | 行 | 类 | 备注 |
|---|---|---|---|
| `logininfor/index.vue` | 209 | **C** | CRUD + 导出/解锁/清空 |
| **`logininfor/data.tsx`** | 116 | **B** ⭐ | |
| `logininfor/login-info-modal.vue` | 111 | **C** | |
| `operlog/index.vue` | 190 | **C** | |
| **`operlog/data.tsx`** | 92 | **B** ⭐ | |
| `operlog/operation-preview-drawer.vue` | 131 | **C** | JSON 预览抽屉 |

#### `system/`（52 文件 / ~6,900 行）

| 模块 | 文件(行) | 类 | 备注 |
|---|---|---|---|
| **`user/`** ⚠️最复杂 | **index.vue 329** / **data.tsx 214** / user-drawer 250 / dept-tree 174 / user-info-modal 155 / user-import-modal 115 / user-reset-pwd-modal 115 / authRole.vue 6 | C / **B** / C×5 / **A** | 左树右表 + Excel 导入 + 重置密码 + 详情 + 角色分配。**`authRole.vue` 是后端菜单契约占位（§1.1 #2），必须建同名 `.tsx`** |
| **`menu/`** | index.vue 262 / **data.tsx 400** / menu-drawer 179 | C / **B** / C | 树表格（目录/菜单/按钮三类型 + 图标选择器 + 权限串）；另有 `v-access:role="['superadmin']"` 守卫 |
| **`role/`** | index.vue 250 / **data.tsx 252** / role-drawer 166 / role-auth-modal 124 / authUser.vue 11 | C / **B** / C / C / **B** | `role-auth-modal` = 菜单权限树 + 数据范围；`authUser.vue` 薄包装必须保留同名（§1.1 #4） |
| **`dict/type/`** | index.vue 210 / **data.ts 77** / dict-type-modal 93 / **index-refactor.vue 282** | C / **B** / C / **E** ⚠️ | `index-refactor.vue` **全仓无引用（已 grep 确认）→ 死代码丢弃**，或择优替代 index.vue |
| **`dict/data/`** | **data.tsx 147** / dict-data-modal 208 / dict-data-item-modal 101 | **B** / C / C | 主从联动 |
| **`dict/`** | index.vue 11 / **data.vue 6** / mitt.ts 11 | C / **A**⚠️ / A | `data.vue` 是后端菜单契约占位（§1.1 #9），必须建同名 `.tsx`；`mitt.ts` 跨组件事件（框架无关） |
| **`client/`** | index.vue 203 / **data.tsx 202** / client-drawer 146 / secret-input 54 | C / **B** / C / C | OAuth 客户端 + 密钥输入 |
| **`post/`** | index.vue 201 / **data.ts 137** / post-drawer 113 | C / **B** / C | + 部门树侧筛 + 导出 |
| **`dept/`** | index.vue 183 / **data.ts 149** / dept-drawer 179 | C / **B** / C | 树表格 |
| **`config/`** 👈**PoC 首选** | index.vue 181 / **data.ts 130** / config-modal 89 | C / **B** / C | 最简单的完整 CRUD，用于打通全链路 |
| **`notice/`** | index.vue 146 / **data.ts 130** / notice-modal 187 | C / **B** / C | modal 内含富文本编辑器 |
| **`role-assign/`** | index.vue 150 / **data.tsx 43** / role-assign-drawer 88 | C / **B** / C | |

---

## 9. React 侧需**新建**的文件（无 Vue 源对应）

| 新文件 | 用途 | 估行 |
|---|---|---|
| `packages/effects/access/src/access.tsx` | `<Access code role>` 组件，替代 `directive.ts`（75 处调用点） | 60 |
| `packages/@core/hooks/use-media-query.ts` | 替代 `use-is-mobile` | 25 |
| `packages/effects/layouts/src/route-cached/activity-cache.tsx` | `<Activity>` + LRU 容器，替代 `route-cached/` | 150 |
| `apps/web-react/src/services/notification.ts` | ⚠️ SSE/WS 模块级单例（脱离组件 effect），配合 Activity | 180 |
| `apps/web-react/src/services/message.ts` | antd `App.useApp()` 全局桥接，替代 `utils/popup.ts` | 80 |
| `apps/web-react/src/router/route-tree.ts` | `backMenuToRouteObjects` + `patchRoutes` 装配 | 200 |
| `apps/web-react/src/router/require-auth.tsx` | 布局路由守卫，替代 `guard.ts` 的 token 检查 | 90 |
| `apps/web-react/src/adapter/grid.ts` | `useGrid`：ProTable 包装，保留 `useVbenVxeGrid` 签名 | 250 |
| `apps/web-react/src/adapter/form.tsx` | `useVbenForm`：RHF + ProForm 包装，保留 `[Component, api]` 签名 | 200 |
| `apps/web-react/src/plugins/viewed-row.ts` | 重写 `use-viewed-row.ts`（已读行 + IndexedDB） | 400 |
| `internal/lint-configs/eslint-config/src/configs/react.ts` | 替代 `vue.ts` | 120 |
| `internal/lint-configs/oxlint-config/src/configs/react.ts` | 替代 `vue.ts` | 15 |
| `packages/@core/base/typings/src/react-router.d.ts` | `RouteMeta` 声明合并，替代 `vue-router.d.ts` | 120 |
| `packages/@core/preferences/src/antd-theme-bridge.ts` | Vben CSS 变量 → antd 6 `ConfigProvider` token | 120 |
| **合计** | | **~2,010** |

---

## 10. 汇总统计

### 10.1 按迁移分类

| 类 | 文件数（约） | 行数（约） | 占比 | 说明 |
|---|---|---|---|---|
| **A 原样搬运** | 210 | **11,300** | 12% | 加密、请求客户端、shared 工具、design tokens、全部 `model.d.ts`、locale JSON、SVG 资源、`.env`、构建脚本 |
| **B 机械替换** | 145 | **9,900** | 10% | 40 个 API 文件、`data.ts(x)` schema、SVG→JSX（含 4,568 行 slogan）、echarts option、tiptap toolbar |
| **C 重写** | 265 | **38,500** | 41% | 全部业务 views、layouts、common-ui 屏、widgets、form/grid/adapter、tabbar |
| **D 库替代** | 215 | **26,700** | 28% | shadcn-ui 33 原语、vxe-grid、cropper、resize、codemirror、tinymce、tiptap editor、json-viewer、tippy、motion |
| **E 丢弃** | 200 | **7,600** | 8% | Vue 指令/slot/priority-value、`reset-routes`、`transition.css` 部分、未使用的 ele/naive/antd 样式、`backup/`、`dist/`、死代码 |
| **合计** | **1,035** | **~94,000** | | |

### 10.2 净产出预估

```
需搬运/改造的源码    ≈ 59,700 行（A + B + C）
其中 A/B 低成本      ≈ 21,200 行（36%）
需重写              ≈ 38,500 行 Vue → 约 26,000 行 React
                     （D 类 26,700 行被库吸收；ProComponents 抵消大量 layout/form/grid 代码）
需新建              ≈  2,010 行
────────────────────────────────────
React 侧最终代码量   ≈ 39,000 行（现 Vue 侧 94,000 行 → 缩减 ~58%）
```

### 10.3 工作量分配

| 阶段 | 覆盖章节 | 人日 |
|---|---|---|
| P0 基建 | §2 + §3 | 4–6 |
| P1 核心层 | §4 + §7.2 + §7.3 | 8–12 |
| P2 UI-Kit | §5 | 8–12 |
| P3 effects | §6 + §7.1 | 10–14 |
| P4 业务应用 | §8 | 14–18 |
| P5 联调交付 | §9 + 文档 + Docker + e2e | 6–8 |
| **合计** | | **50–70 人日** |

---

## 11. 建议执行顺序（依赖拓扑）

```
① §2 基建 + §3 内核搬运（A 类，零风险）
      ↓  可跑通 vitest，验证加密/请求/shared 测试全绿
② §4.1 preferences + §3.4 design tokens + antd-theme-bridge
      ↓  主题切换可视化验证
③ §4.2 stores(Zustand) + §4.7 locales(i18next) + §8.9 utils/http
      ↓  能登录拿 token（用 curl 或临时页面验证）
④ §8.7 router + §7.2 动态菜单 + §4.3 access          ⚠️ PoC 关卡 1
      ↓  后端菜单驱动路由渲染成功
⑤ §8.2 adapter + §7.1 grid + §5.2 form-ui            ⚠️ PoC 关卡 2
      ↓  system/config 页跑通完整 CRUD
⑥ §6.3/6.4 layouts + widgets + §7.3 Activity 页签缓存
      ↓  完整后台外壳
⑦ §8.10 剩余 12 个 system 页 + 2 monitor + dashboard（可多人并行）
⑧ §8.4 富组件（upload/cropper/tiptap/tinymce/tree/dict）（可并行）
⑨ §8.10 _core 认证全流程 + profile
⑩ §9 收尾 + e2e + Docker 编排 + docs
```

**两个 PoC 关卡（④⑤）不通则全盘返工，建议各留 1–2 天先单独验证。**

---

## 12. 迁移前可先清理的死代码（与 React 无关，建议现在就删）

| 文件 | 行 | 证据 |
|---|---|---|
| `apps/web-antd/src/views/system/dict/type/index-refactor.vue` | 282 | 全仓 grep `index-refactor` **零引用** |
| `backup/20260811-dict-data-drawer-old/` | 5 文件 | 旧版备份，已被 `dict/data/` 替代 |
| `packages/styles/src/ele/index.css` | 44 | grep 零引用 |
| `packages/styles/src/naive/index.css` | 27 | grep 零引用 |
| `packages/styles/src/antd/index.css` | 86 | grep 零引用（当前用 `antdv-next/`） |
| `apps/web-antd/dist/` `dist.zip` `.turbo/` | — | 构建产物，已被 `.gitignore` 排除（`git ls-files` 确认未跟踪），迁移时无需处理 |

> ⚠️ 但 `views/system/user/authRole.vue`、`views/system/role/authUser.vue`、`views/system/dict/data.vue` **虽注释写「无实际意义」，仍被三后端 seed 的 `sys_menu.component` 引用，不可删**（见 §1.1）。

---

## 13. 阶段 ① 实施记录（2026-11-05）

阶段 ① 已落地在 `walnut-frontend-react/`，产出与实测修正如下。

### 13.1 已完成

| 项 | 结果 |
|---|---|
| monorepo 骨架 | pnpm workspace（12 个项目）+ Vite 8 + React 19.3 + TS 6 + vitest 4 |
| 搬运的包 | `@core/shared`、`@core/typings`、`@core/design`、`constants`、`types`、`locales`、`utils`、`request`、`internal/tsconfig`、`internal/tailwind-config` |
| 测试 | **23 文件 / 222 用例全绿**，其中新增 20 个加解密用例 + 14 个 i18n 用例（源项目这两块均无测试） |
| 类型检查 | 根 tsconfig 与 app tsconfig 均零错误 |
| 生产构建 | 711 模块，主 chunk 557 kB（jsencrypt + crypto-js 占大头） |
| 浏览器实测 | dev server 8020，自检面板 7 项全绿 |
| 真实后端链路 | `login → getInfo → getRouters` 跑通，信封拆包正确 |

### 13.2 对本清单的修正

**① §3.3 加密层不是「一行不改」**

`impl/rsa.ts` 有一处缺陷需修：jsencrypt 在密钥不匹配时返回 `null` 而非 `false`，
原判断 `if (ret === false)` 拦不住，`null` 会流到 `decodeBase64()` 抛出难定位的
`TypeError: Cannot read properties of null`。改为 `ret === false || ret === null`。
**建议同步回 Vue 侧同一文件。**

**② §1.2 加密契约需补全「响应方向」**

RSA 是**两对密钥**，不是一对：

- `VITE_GLOB_RSA_PUBLIC_KEY` → 前端加密请求，后端配对私钥解密
- `VITE_GLOB_RSA_PRIVATE_KEY` → 后端加密响应，前端私钥解密

响应方向后端**另生成一次性 AES 密钥**、用响应公钥加密后放进响应的 `encrypt-key` 头，
**不是**把请求头原样回传。§8.9 `http/index.ts` 的搬运要照这个语义。

**③ 新增风险项：Go demo 容器当前关闭了服务端解密**

`docker-compose.go.demo.yml` 未配 `API_DECRYPT_*`，
`walnut-backend-go/internal/config/validate.go` 遂将 `APIDecryptDisabledReason`
置为「RSA 密钥未配置」，解密中间件不生效；而 `.env.development` 里
`VITE_GLOB_ENABLE_ENCRYPT=true`。两边不一致时后端把密文当 JSON 解析，
返回 `请求数据格式错误（JSON 解析失败）`。

→ **§7 应增加第 4 个前置项**：迁移期间先对齐前后端的加密开关，
否则阶段 ③ 联调会误判为前端 bug。

**④ §1.1 视图路径契约已对活后端验证通过**

实抓 `GET /system/menu/getRouters` 得到 16 节点菜单树，14 条 `component` 与三后端 seed 完全一致，
三个占位文件（`system/user/authRole`、`system/role/authUser`、`system/dict/data`）确实被引用。
样本已存为 `apps/web-react/src/__fixtures__/backend-menu-tree.json`，
**§7.2 的 `backMenuToRouteObjects` 应以它为输入写单测**。

补充实测到的节点形状（§7.2 需处理）：

- 字段：`{name, path, hidden, redirect, component, alwaysShow, meta, children}`
- `meta`：`{title, icon, noCache, link, activeMenu}`
- `component` 实际只出现 `Layout`（一级目录）与 `ParentView`（多级目录）两种特殊值
- `name` 带菜单 ID 后缀（`System1`、`User100`），且含 `/` 与 `:`
- **带参路径**：`user-auth/role/:userId`、`role-auth/user/:roleId`、`dict-data/index/:dictId`，
  配 `hidden: true` + `meta.activeMenu` 指回列表页；React Router 7 的 `:param` 语法一致，可直接沿用
- `dashboard` / `profile` 不在后端返回中，来自前端静态模块（`accessMode: 'mixed'` 得证）

**⑤ `@vben-core/shared` 的 `filterTree` 有两个坑（影响 §7.2 菜单过滤）**

```ts
if (node[childProps]) {
  node[childProps] = _filterTree(node[childProps]);  // ← 原地写回入参
}
```

- **会改写入参**，过滤前需 `structuredClone`
- **不保留匹配节点的祖先**：过滤 `{id:1,children:[{id:3,children:[{id:4}]}]}`
  保留 `id===4` 得到空数组（id:3 不匹配即整枝剪掉）

**⑥ 阶段 ① 有意跳过的文件**

| 文件 | 原因 | 归属阶段 |
|---|---|---|
| `utils/helpers/generate-routes-{backend,frontend}.ts`、`merge-route-modules.ts`、`generate-menus.ts` | 依赖 `vue-router` 的 `RouteRecordRaw`，需先定 React Router 路由对象形态 | ④ |
| `utils/helpers/reset-routes.ts` | E 类，RR7 用 `patchRoutes` 不需要 | — |
| `typings/src/tabs.ts` | `TabDefinition extends RouteLocationNormalized` | ⑥ |
| `typings/src/vue-router.d.ts` | E 类，待建 `react-router.d.ts` | ④ |
| `@core/design` 根入口（`css/global.css`） | 内含 `@import '@vben/tailwind-config/theme'`，需 Tailwind 4 插件；阶段 ① 只引入 `./tokens`（纯 CSS 变量） | ② |

**⑦ axios 类型扩展需注意泛型参数**

axios 1.21 的 `AxiosRequestConfig<D = any, P = any>` 是双泛型，
模块合并要求类型参数逐字一致，否则报「所有声明必须有相同的类型参数」。
Vue 侧 `apps/web-antd/types/alova.d.ts` 的写法（无泛型参数）在严格检查下会踩这个坑。

**⑧ §8.9 `utils/http` 搬运时要给解密包一层错误处理**

`AesEncryption.decrypt` 在密钥错配时抛的是 crypto-js 的 `Malformed UTF-8 data`
（乱码偶然构成合法 UTF-8 时则返回垃圾字符串，行为不确定）。
前后端密钥没配对时用户看到的就是这条，而非「密钥错误」，排查成本高。
建议在响应解密处捕获并转成可定位的错误信息。

### 13.3 阶段 ② 待办

1. `internal/vite-config` 移植（`extra-app-config` 运行时注入、`code-split` 分包名单改 React）
2. Tailwind 4 + `@vben/tailwind-config` 接线，打通 `@core/design` 根入口
3. lint 体系：`react.ts` 替代 `vue.ts`，接入 turbo
4. `@core/preferences` → Zustand，CSS 变量生成器对接 antd 6 `ConfigProvider`
5. 补齐 §13.2⑥ 跳过的路由/菜单生成函数（改为输出 RR7 `RouteObject`）

## 14. 阶段 ② 实施记录（2026-11-06）

### 14.1 已完成

| 项 | 结果 |
|---|---|
| `internal/node-utils` + `internal/vite-config` 移植 | tsdown 构建，`dist/index.mjs` 27.67 kB |
| vite-config 裁剪 | 删 `vxe-table.ts` / `nitro-mock.ts` / `importmap.ts` / `config/library.ts`；`@vitejs/plugin-vue`+`plugin-vue-jsx` → `@vitejs/plugin-react`；删 `unplugin-dts`、`vite-plugin-vue-devtools`、`@intlify/unplugin-vue-i18n` |
| `code-split.ts` 重写 | antdv-next 系列 → antd 6 系列（`antd/es/**` + `rc-*`）；vue/vueuse/pinia → react/react-dom/scheduler/react-router；新增 `pro-components` 组 |
| Tailwind 4 接线 | `@vben-core/design` 根入口打通（`css/global.css` → `@vben/tailwind-config/theme`）；`theme.css` 去掉不存在的 `@source docs/ playground/`；`tailwind-reference.ts` 从「注入 Vue SFC `<style>`」改为「注入 .css/.scss」 |
| `@core/preferences` → Zustand | `preferences.ts`(463) 重写为 vanilla store；`use-preferences.ts`(280) 由 `computed` 改 `useMemo`；`types/config/constants/update-css-variables` 四个 A 类文件原样搬运 |
| `@vben/hooks` 新建 | `useAntdDesignTokens` —— 从 CSS 变量回读 token 喂 antd（对应 Vue 侧 `use-design-tokens.ts`） |
| antd 6.7 接入 | `ConfigProvider` + `App`，`algorithm` 按 `isDark` / `app.compact` 组装，与 Vue 侧 `app.vue` 一致 |
| 验证 | **263 测试全绿**、typecheck 零错误、生产构建成功、浏览器实测明暗主题切换 + antd 按钮色与 `--primary` 精确相等 |

生产构建分包实测：`framework` 219 kB · `vben-core` 187 kB · `antd-theme` 125 kB ·
`vben-request` 111 kB · `vben-locale` 91 kB · `antd-overlay` 86 kB · `antd-form` 47 kB ·
`antd` 46 kB · `app-core` 29 kB · `antd-icons` 12 kB。

### 14.2 对本清单的修正

**① §3.4「样式与设计令牌」补一条硬约束**

`@vben-core/design` 的 `package.json` **不能**声明 `sideEffects: ["**/*.css"]`。
它的入口 `src/index.ts` 只做 CSS 导入，一旦声明就会被判定为无副作用而整体摇掉，
连带 design-tokens / transition / nprogress / ui 全部 CSS 不进产物。
**dev 模式看不出来，只有 `vite build` 暴露。** Vue 侧该包没有这个字段。

**② §2.2 `internal/vite-config` 的可移植度要下调**

原估 ~80% A 类，实测需要改动的比预期多：

- tsdown 只把 `dependencies`/`peerDependencies` 当外部，**`devDependencies` 会被打进产物**。
  `vite`/`@vitejs/plugin-react`/`sass-embedded` 等必须移到 dependencies（vite 放 peer），
  否则 dist 从 36 kB 膨胀到 3.2 MB
- tsdown 的 dts 生成对本仓两个包都不可用（vite-config 报 postcss `MISSING_EXPORT`；
  node-utils 产出的 `index.d.ts` 语法有误）。统一改双轨：`types → src`、`default → dist`
- ⚠️ **`utils/env.ts` 的 `getConfFiles()` 靠正则匹配 `npm_lifecycle_script` 里的 `--mode xxx`
  判断环境**，脚本漏写 `--mode development` 会静默加载 `.env.production`。
  §8.1 的 `package.json`（B 类）必须连带改 scripts，不只是换依赖

**③ §2.2 `code-split.ts` 的 priority 有隐藏约束**

`framework`（react/react-dom）组的 priority **必须高于所有 antd 组**，
否则 react-dom 会被吸进 `antd-theme`（实测 336 kB → 修正后 125 kB）。
Vue 侧 framework=30 高于 antdv 各组吗？——不，Vue 侧 antdv 组是 40~47、framework 是 30，
但 vue 与 antdv-next 之间没有这种吸附；**React 侧 antd 直接依赖 react-dom，必须调整**。

**④ §4.1 preferences 的移植方式需明确「双读接口」**

Vue 侧 `export const preferences = getPreferences()` 导出的是 `reactive` 代理，
组件读取即自动订阅。React 没有隐式订阅，因此：

- 组件内 → `usePreferences()`（订阅 store，返回普通值而非 `ComputedRef`）
- 命令式代码（请求拦截器等）→ `getPreferences()` 取快照，或 `preferences` Proxy（实时但**不触发重渲染**）

⚠️ 另一个坑：`preferencesStore` 是模块级单例，但 `PreferenceManager` 可被多次 `new`
（测试里每个用例都 new 一个）。**构造函数里必须重置 store**，否则状态跨实例串。

**⑤ §4.4 `use-design-tokens.ts` 的分类要从 B 上调为 C，且要修一个色值 bug**

原估「getComputedStyle 读 CSS 变量，trivial」。实测 Vue 侧的 `useAntdDesignTokens`
把 CSS 变量拼成 `` `hsl(${value})` `` 直接传给 antd，而 **antd 解析不了 CSS Color 4 的
空格语法**：`hsl(215 100% 54%)` 被读成 `rgb(20, 103, 220)`，真值是 `rgb(20, 118, 255)`。

React 版改为先 `convertToRgb()` 归一化。**建议回查 Vue 侧**：同一文件的
`useNaiveDesignTokens` 用了 `convertToRgb`，`useAntdDesignTokens` 没用，两者不一致，
Vue 版的 antd 组件很可能与自绘组件存在同样色差。

**⑥ Vue 侧有三个测试是「假绿」的（移植时才暴露）**

| 测试 | 问题 |
|---|---|
| `@core/preferences/__tests__/config.test.ts` | 快照过期（缺 `buttonWaveMode`，`mode`/`lockScreen` 是被污染的旧值）。**在 Vue 仓库里直接跑也是红的**（已实测确认） |
| `preferences.test.ts` · does not update undefined preferences | `originalPreferences` 与 `getPreferences()` 是同一个 `readonly` 代理，断言恒真 |
| `preferences.test.ts` · ignores updates with invalid preference value types | 同上；实际主偏好**不做类型校验**，只有扩展偏好走 `sanitizeCustomPreferences` |

React 版 `getPreferences()` 返回不可变快照，三条立刻暴露。已按真实行为重写，
并补一条「快照语义」测试（更新后必须是新引用）。

**⑦ 阶段 ② 有意跳过**

| 项 | 归属阶段 |
|---|---|
| `internal/lint-configs`（eslint/oxlint 的 `react.ts`） | ③ |
| turbo 接入 | ③ |
| `@vben/hooks` 其余 8 个 hook（`use-app-config` / `use-pagination` / `use-watermark` / `use-tabs` …） | ③④⑥ |
| `update-css-variables.ts` 里 `builtinTypeColorPrimary` 的死代码（算出来但赋值那行是注释掉的） | 保持原样，未清理 |
| `injectGlobalScss`（`@vben-core/design/bem`） | 默认关闭，等 ui-kit 阶段需要 BEM 时再开 |

### 14.3 阶段 ③ 待办

1. `packages/stores` → Zustand（access / user / tabbar / timezone），沿用 `secure-ls`
2. `apps/web-react/src/lib/request.ts` 补全为完整 `utils/http`（`*WithMsg` 变体、401 登出、业务异常、下载/导出）
3. SSE/WebSocket 通知提到组件外的 service 单例
4. lint 体系 + turbo
5. `generate-routes-*` / `generate-menus` 改输出 React Router `RouteObject`

---

## 15. 阶段 ③④⑤ 实施记录（2026-11-14 补记）

> **本段为补记**：③④⑤ 的代码在 2026-11-06 之后陆续落地，当时未回写本文档，
> 导致文档停在阶段 ②、`docs/frontend/README.md` 还写着「规划中，尚未实施」。
> 以下全部是 2026-11-14 对 `walnut-frontend-react/` 实际代码逐文件核对的结果，非计划转述。

### 15.1 已完成

| 阶段 | 项 | 结果 |
|---|---|---|
| ③ | `packages/stores` → Zustand | ⚠️ 只建了 `modules/{access,user}.ts`；**§4.2 原计划的 tabbar / timezone 两个 store 未建** |
| ③ | `utils/http` 全量装配 | ✅ `apps/web-react/src/lib/request.ts`(204)：信封拆包 + 业务码 + `BusinessException` + `errorMessageMode`/`successMessageMode` + `*WithMsg` 四变体 + 401 清态跳登录；请求侧 token / `Accept-Language` / `ClientID` / 加解密 / GET·DELETE 数组参数 repeat 序列化 |
| ③ | SSE·WebSocket 提到 service 单例 | ✅ `apps/web-react/src/services/message.ts` |
| ③ | lint 体系 + turbo | ❌ **未做**：无 `internal/lint-configs`、无 `turbo.json` |
| ③ | `generate-routes-*` / `generate-menus` | ✅ `packages/utils/src/helpers/generate-{accessible,menus,routes-backend,routes-frontend}.ts`，已输出 RR7 结构 |
| ④ | 后端动态菜单 → React Router 7（**PoC 关卡 1**） | ⚠️ **只完成转换层，运行时未接线**（→ 已于 §16 接通）。`router/back-menu.ts`(212) + `router/route-tree.ts`(162) + `router/access.ts`(36) 均已就位且单测覆盖（back-menu 15 例 + route-tree 19 例），但 `router/index.tsx`(59) **仍是静态 PoC 路由**（只挂 `/auth/login` 与 `/system/config`）；`generateAccess` / `toRouteObjects` / `createLazyPageMap` 三者在全仓**只被测试引用，运行时无任何调用点**。即「后端菜单能正确转成 RR7 路由树」已证明，「应用真的按后端菜单渲染」尚未发生 |
| ④ | 真实契约 fixture | ✅ `src/__fixtures__/backend-menu-tree.json`（Go 后端实抓 16 节点）驱动 `back-menu.test.ts` 15 例 |
| ⑤ | vxe-table → antd Table（**PoC 关卡 2**） | ✅ **通过**。`adapter/grid.tsx`(361) + `adapter/component.tsx`(24) |
| ⑤ | form-ui / popup-ui | ✅ `@core/ui-kit/form-ui`(5 文件) + `popup-ui/modal`(3 文件)；**popup-ui 的 drawer 未建**（§5.3 另一半） |
| ⑤ | shadcn-ui 原语 | ⚠️ 仅 4 个（button / dropdown-menu / toggle-group / tooltip），§5.1 需 33 个，**缺 ~29** |
| ⑤ | 首个业务页 | ✅ `views/system/config/`（index + config-modal + data.ts）完整 CRUD |
| ⑤ | 认证外壳 | ✅ `layouts/authentication/*`(5 文件) + `views/_core/authentication/*`(4 文件，含 InputCaptcha / OAuthLogin) |
| ⑥ | 后台外壳 | ❌ **未做**。`layouts/BasicLayout.tsx` 是 **42 行 PoC 占位**：左侧 224px 是空 div 假装的，无 sidebar / header / tabbar。**这是阶段 ⑥ 的起点，也是当前唯一的可见性阻塞项** |

### 15.2 像素门禁（§11 未规划，阶段 ⑤ 自建的验收设施）

`e2e/visual/` —— Playwright + pixelmatch，**以 Vue 端截图为 golden**：

- `specs/mock-api.ts`(161)：拦截 login / getInfo / getRouters / config 列表，**免后端即可跑双端**
- `specs/capture.spec.ts`(125)：`VISUAL_CAPTURE=vue|react` 切端，支持元素级截图与暗色
- `specs/compare.spec.ts`(92)：双端尺寸不同时裁剪到交集区域再比，diff 图落 `artifacts/diff/`

| 目标 | 实测差异 | 阈值 | 说明 |
|---|---|---|---|
| `login-light` | **0.28%** | 0.3% | 全自绘复刻，外壳层达到像素级一致 |
| `login-dark` | **0.02%** | 0.3% | |
| `config` | **5.2%** | 6%（按实测校准） | 残余全部来自 vxe-table ↔ antd Table 原生差异（垂直节奏 / 分页「总数在左+页码在右」/ 分页字形）+ PoC 最小布局与 Vue 完整布局的高差。**1.5% 的设计目标留待阶段 ⑥ 真实布局 + antd Table 主题覆盖层达成**（§7.1 风险 1 决策 (a)） |

`artifacts/test-results/.last-run.json` = `passed`。

**入库策略**：`artifacts/vue/`（golden，再生需同时起 Vue 端 + 后端，昂贵）提交；
`artifacts/react/`、`artifacts/diff/`、`test-results/`、`report/`（单文件 507 kB）加进 `.gitignore`——均可由 React 端单独再生。

### 15.3 验证基线（2026-11-14 实测）

- `pnpm test:unit`：**30 文件 / 316 用例全绿**（阶段 ② 为 263，+53）
- `pnpm typecheck`：15 个 project 全部 Done，零错误
- `pnpm build`：成功，`dist.zip` 2.46 MB
- 分包（gzip 后）：`antd-form` 131.5 · `framework` 98.1 · `vben-core` 72.4 · `antd-theme` 49.4 ·
  `antd-data` 41.9 · `antd-overlay` 37.8 · `vben-request` 36.2 · `vben-locale` 28.6 kB
  （`antd-form` 组从阶段 ② 的 47 kB 涨到 131.5 kB，因 form-ui + config 页落地）

### 15.4 对本清单的修正

**① §14.3 的 5 项待办里 2 项没做完**：lint + turbo 完全没动；stores 只建了 access/user。
其中 **tabbar store 是阶段 ⑥ tabbar 的硬前置**，不能继续往后拖。

**② `lib/request.ts:67` 的注释已过期**：写着「PoC 用 `window.location` 跳转，完整 router
联动在阶段 ④ 接入」，但阶段 ④ 早已完成，该处仍是 `window.location.assign(LOGIN_PATH?redirect=…)`。
行为本身可接受（整页刷新能把状态清干净），但注释误导，阶段 ⑥ 顺手删注释或改成 RR7 `navigate`。

**③ §11 的拓扑漏了「验收设施」这一环**：原计划 e2e 放在 ⑩ 收尾，实际像素门禁在 ⑤ 就建好、
并直接成了 PoC 关卡的判据。**建议正式前移为每阶段的固定门禁**——阶段 ⑥ 起每完成一层就加对应
截图目标（shell / menu / tabbar，之后每个业务页一个），否则「完成了」只能靠肉眼。

**④ §10.3 的工作量表需要修正**：P0(4–6) + P1(8–12) 已实际完成，P2 完成一部分
（form-ui / popup-ui-modal / shadcn 4 原语）。**剩余 P2 尾部 + P3 + P4 + P5 ≈ 38–52 人日**，
其中 §6.3 + §6.4 外壳层（12,652 + 4,700 = 17,352 行）是单一最大块。

**⑤ 后端不再是阻塞项**：`walnut-backend-go/internal/module/system/` 现有
client / config / dept / dict / log / menu / notice / post / role / social / user + `web/auth`，
即 §8.10 要铺的业务页基本都有 Go 端可连（记忆中「64/122 路由、menu/role/user/monitor 未做」
的快照已过期）。

### 15.5 阶段 ⑥ 待办（下一步）

决策：**先 ProLayout 跑通、后补 1:1**——先让业务页能铺开，视觉细节留到第二趟。

**⑥-a 可用外壳**（目标：登录后进入真实后台，解除 ⑦ 的阻塞）

1. `packages/stores/src/modules/tabbar.ts` → Zustand（页签状态 + 持久化，沿用 `secure-ls`）
2. 重写 `BasicLayout.tsx`：antd `Layout` + 真实 sidebar（消费 `router/back-menu` 产出的菜单树）+ header + 内容区
3. `@core/ui-kit/menu-ui`：折叠/展开、多级子菜单、`meta.icon` 渲染、`meta.activeMenu` 高亮回指
4. `@core/ui-kit/tabs-ui` + tabbar：页签增删 / 关闭其他 / 右键菜单
5. `--vben-content-height` 改由真实内容区测量，移除 224px 占位 div
6. 像素门禁新增 `shell` / `menu` / `tabbar` 三个目标；`config` 阈值从 6% 收紧回 1.5% 的路径要在这一阶段验证

**⑥-b 1:1 补齐**（可与 ⑦ 业务页并行）

- 6 种布局模式（§6.3 `basic/menu/` 441 行混合导航逻辑）
- 偏好抽屉 577 行 + 24 个 block + 9 个布局示意 SVG（B 类，SVGR 机械转）
- widgets：global-search(Cmd+K) / user-dropdown / notification(SSE) / lock-screen / theme-toggle / check-updates / timezone / breadcrumb
- `<Activity>` + LRU 页签缓存（§7.3）
- `@core/ui-kit/layout-ui`、shadcn-ui 剩余 ~29 原语、popup-ui 的 drawer

---

## 16. 阶段 ⑥-a 实施记录（2026-11-15）

决策是「**先跑通、后补 1:1**」，但原计划的 ProLayout 路线不可用（见 16.1），改走 antd 6 原生
`Layout` + `Menu`。本阶段交付「可用外壳」：登录后进入真实后台，侧边栏由后端菜单驱动，
多页签可开关，1:1 的视觉细节全部推到 ⑥-b。

### 16.1 硬约束：ProLayout 与 antd 6 不兼容 ⚠️

```
$ npm view @ant-design/pro-components@2.8.10 peerDependencies
{ antd: '^4.24.15 || ^5.11.2', react: '>=18.0.0', 'react-dom': '>=18.0.0' }
```

本仓 catalog 锁 `antd: ^6.7.0`。为用 ProLayout 而降回 antd 5，会推翻阶段 ② 已验收的
antd 6 主题桥接（`useAntdDesignTokens` + 像素门禁实测按钮色与 `--primary` 精确相等）
与全部 381 个单测，得不偿失。

**因此清单里两条 D 类判断作废**：
- §6.3「`basic/layout.vue` 441 行 → ProLayout 覆盖」→ 改为 antd `Layout` 自绘
- §6.4「`breadcrumb.vue` 71 行 → ProLayout 自带」→ 改为 antd `Breadcrumb` 自绘

### 16.2 已完成

| 项 | 产出 |
|---|---|
| **阶段 ④ 遗留的运行时接线** | `router/guard.ts`（`ensureAccessRoutes`：并行拉 userInfo + 菜单 → `patchRoutes` 注入 Root → 重导航）、`layouts/RootLayout.tsx`（鉴权 + bootstrap + 加载/错误态）、`router/routes/`（mixed 模式的前端静态模块聚合，`modules/` 暂空）、`views/_core/fallback/`（403/404）、`router/access.ts` 补 `pageMap`/`layoutMap`/`forbiddenComponent` |
| **BasicLayout 真实外壳** | 42 行 PoC 占位 → antd `Layout.Sider`（logo + 菜单 + 折叠）+ `Header`（折叠按钮 / 面包屑 / 主题 / 语言 / 用户下拉）+ `Content`；`--vben-content-height` 由内容区 `ResizeObserver` 实测，224px 假侧边栏移除 |
| **菜单** | `adapter/menu.tsx`：`MenuRecordRaw` 树 → antd `Menu` items；`selectedKeys` 走 `meta.activePath` 让带参详情页高亮回指列表页；`openKeys` 按 `parents` 链展开且保留用户手动展开项；外链 `window.open` |
| **图标** | `shadcn-ui/components/icon.tsx` 新增 `VbenIcon`（组件 / http 图 / Iconify 名 / fallback 四态，判断顺序与 Vue `icon.vue` 一致）；新增依赖 `@iconify/react ^5.2.1`（与 Vue 侧 `@iconify/vue ^5` 同主版本） |
| **tabbar store** | `@core/typings/src/tabs.ts`（自包含 `TabDefinition`，不引路由库类型）+ `stores/src/modules/tabbar.ts`（Zustand）+ 60 例单测 |
| **多页签栏** | `adapter/tabs.tsx` + `tabs.css`：antd `Tabs` 承载，路由变化登记页签、点击切换、关闭按钮、右键菜单（关闭/固定/新窗口/关闭左侧·右侧·其它·全部），文案复用已移植的 `preferences.tabbar.contextMenu.*` |
| **像素门禁** | capture 新增 `shell` / `shell-dark` 两个整页目标；启动 Vue 端（8010）用同一份 mock 重采 golden；阈值按实测校准 |

### 16.3 三个踩坑（都不是读代码能发现的）

**① `router.patchRoutes()` 不会对当前 location 重新匹配。**
重匹配只发生在 RR7 的 `patchRoutesOnNavigation` 钩子内部，而本仓用不上那个钩子——Root 带
静态 `*` 子路由，任何路径都能匹配上，钩子根本不触发（它只在「无法匹配」或「匹配到含参数的
路由」时才被调用）。第一版就栽在这：菜单拉到了、路由注入了，页面却停在 `*` 兜底的 404 上，
要手点一次菜单才恢复。**修法**：patch 之后主动 `await router.navigate(同址, {replace:true})`。

> 为什么保留静态 `*` 而不是靠 `patchRoutesOnNavigation`：未登录时若没有任何路由匹配，
> RR 会渲染它自己的默认 404 错误页，而不是跳登录页。有了 `*` 兜底，未登录的任意路径都会
> 匹配到 Root，`RootLayout` 就能把它重定向到登录页。

**② `setIsAccessChecked(true)` 的时机导致页签永久丢失。**
现象：页签栏渲染了，但 `.ant-tabs-nav-list` 是空的。**截图看不出来，是 DOM 探针发现的**。
根因：`isAccessChecked` 是 `RootLayout` 渲染外壳的开关，而它原先在 `patchRoutes`/`navigate`
**之前**置位，于是外壳在「当前匹配仍是 `*` 兜底路由」的那一刻挂载，`meta.hideInTab` 为真让
`addTab` 直接跳过；随后重导航完成、地址未变，只依赖 `fullPath` 的 effect 不会重跑。
**修法**：`setIsAccessChecked` 挪到 `await router.navigate()` 之后；页签 effect 的依赖加上
`currentTab.name`（同一地址在动态路由注入前后匹配到不同路由，只看 fullPath 会漏）。

**③ `@ant-design/pro-components` 的 peer 冲突**（见 16.1）。查 `npm view` 只要几秒钟，
但如果不查就照着清单 §6.3 写下去，会在装依赖时才发现，届时外壳代码已经按 ProLayout 的
API 写了一遍。

### 16.4 对本清单的修正

**① §5.5 menu-ui / §5.6 tabs-ui 的落点改了。** 不新建 `@core/ui-kit/menu-ui`、
`@core/ui-kit/tabs-ui` 两个包，改为放 app 的 `adapter/menu.tsx`、`adapter/tabs.tsx`。
理由：`@core/*` 在 vben 的分层里是 **UI 库无关层**，往里塞 antd 会破坏该约束；而 app 的
`adapter/` 本来就是干这个的（§8.2「Vben 解耦层，迁移的枢纽」，已有 `grid.tsx`/`component.tsx`）。
⑥-b 若做自绘版，再把它下沉成 `@core` 包，adapter 转为委托。

**② §4.8 icons 不必照搬。** 不新建 `@vben-core/icons` 包：Vue 的集中再导出层
（`lucide.ts` 一次性 re-export ~100 个图标）是 SFC/tree-shaking 的产物，React 侧直接
`import { X } from 'lucide-react'` 就是惯用写法，照搬只是多一层无意义的间接。
真正需要新建的只有 Iconify 渲染能力（后端 `sys_menu.icon` 下发的是 `eos-icons:system-group`
这类 Iconify 名），已落在 `VbenIcon` 里。

**③ 新增一条硬约束：页面文件必须有 `export default`。**
`createLazyPageMap` 靠 `React.lazy` 装载 `import.meta.glob('../views/**\/*.tsx')`，
没有 default export 的页面在渲染时才会炸（`lazy` 不校验未使用的条目，所以类型检查也发现不了）。
`views/system/config/index.tsx` 已改；⑦ 铺开业务页时每个页面都要遵守。

**④ `preferences.app.defaultHomePath` 暂时被覆盖为 `/system/config`。**
默认值 `/analytics` 由前端静态模块 `dashboard.ts` 提供，而 dashboard 视图要到 ⑦ 才移植，
`router/routes/modules/` 目前是空的，指向 `/analytics` 会落到 404。
**⑦ 补上 dashboard 模块后必须删掉这条覆盖。**

**⑤ §15.2 的「免后端」说法得到验证**：像素门禁的 mock 确实生效（DOM 探针实测
`SIDEBAR_MENU=["系统管理","参数配置"]`、表格 3 行、`共 3 条`，与 `mock-api.ts` 的 fixture
逐项吻合）。⚠️ 但**肉眼读截图会误判**——排查时曾把「参数配置」看成「参数设置」、
把 3 行看成 10 行，进而错误地断定 mock 失效、e2e 打到了真实后端。
结论：涉及「渲染内容是否正确」的判断，一律用 DOM 探针（`innerText`/`count`），不要看缩略图。

**⑥ Playwright 的实际视口是 1280×720，不是配置里写的 1440×900。**
`playwright.config.ts` 顶层 `use.viewport` 设了 1440×900，但 `projects[0].use` 展开了
`devices['Desktop Chrome']`，其中自带的 viewport 覆盖了顶层设置。golden 与 React 端用的是
同一份配置，所以不影响比对结论，但「1440×900」这个数字是假的。

### 16.5 量化基线（2026-11-15 实测）

- `pnpm typecheck`：16 个 project 零错误
- `pnpm test:unit`：**31 文件 / 381 用例全绿**（⑥ 之前为 30/321；tabbar store 贡献 +60，
  `stripNestedLayout` +5）
- 像素门禁 5 项全过：

| 目标 | 实测 | 阈值 | 性质 |
|---|---|---|---|
| `login-light` | 0.277% | 0.3% | 已达标 |
| `login-dark` | 0.021% | 0.3% | 已达标 |
| `config` | 5.245% | 6% | 回归基线，目标 1.5% |
| `shell` | **9.316%** | 10% | 回归基线，⑥-b 持续下调 |
| `shell-dark` | **4.719%** | 5.5% | 回归基线，⑥-b 持续下调 |

- **外壳几何已对齐**：`config` 元素截图从 1024×688 变为 **1024×599**，Vue golden 是 1024×600，
  差 1px。header(50) + tabbar(38) = 88px 正好补回 PoC 占位期缺的高度。
  这也证明 `config` 残余的 5.245% 与布局无关，纯粹是 vxe-table ↔ antd Table 的渲染差异。
- 重采 golden 后 `login-light`/`login-dark` 字节未变、`config` 的 diff 从 5.244% → 5.245%，
  说明门禁本身是确定性的。

### 16.6 阶段 ⑥-b 待办（按 shell diff 图的差异面积排序）

从 `artifacts/diff/shell.png` 直接读出来的施工清单：

1. **顶栏右侧 widget 群**（差异最大的一块）：全局搜索(Cmd+K) / 通知(SSE) / 全屏 / 偏好设置抽屉入口 / 锁屏 / 时区 / 检查更新。对应 §6.4 的 4,700 行 widgets
2. **页签形状**：Vue 是自绘 chrome 弧形页签（§5.6 的 927 行），React 暂用 antd card 页签；同时补拖拽排序、滚轮横滚、中键关闭、`reload` 与 `maximize` 两个右键项（依赖 `<Activity>`）
3. **菜单排版与 logo 区**：自绘 menu-ui（§5.5 的 2,243 行），含手风琴、动画、`badge`/`activeIcon`
4. **折叠按钮位置**：从顶栏左侧移到侧边栏右缘悬浮（vben 的形态）
5. **`<Activity>` + LRU 页签缓存**（§7.3）：`cachedTabs`/`excludeCachedTabs`/`refresh()` 在 store 里已就位，只差消费者
6. **偏好设置抽屉** 577 行 + 24 个 block + 9 个布局示意 SVG，以及 6 种布局模式（§6.3 `basic/menu/` 441 行混合导航逻辑）
7. **antd Table 主题覆盖层**：把 `config` 的 5.245% 压向 1.5% 的设计目标（§7.1 风险1 决策 (a)）

---

## 相关文档

- [接口契约](./01-接口契约.md) · [权限与动态路由](./02-权限与动态路由.md) · [接口加解密](./03-接口加解密.md) · [国际化](./04-国际化.md)
- 方案对比与选型理由见本次迁移分析（方案 A/B/C 对比、三后端 × 两前端仓库布局）

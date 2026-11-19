# walnut-frontend-react

WalnutSeed 的 **React 版前端**，与 `walnut-frontend`（Vue 3 / Vben 5）并存，共用同一套后端接口契约。

> **2026-11-19：现有 Vue 前端功能迁移完成（100% 实施覆盖）**。后端菜单契约组件 **14/14**，不再有迁移占位页；角色、公告、双仪表盘、隐藏授权页及原有管理页均已接通。
> 七种布局、主题/半暗、页签缓存/刷新、搜索、锁屏、通知、租户切换、会话失效弹窗、头像裁剪和富文本已实现。
>
> 验收与部署说明见 [迁移验收记录](../docs/frontend/07-React迁移-验收记录.md)。这里的 100% 指现有前端功能的实施覆盖；注册/找回密码/二维码保留 Vue 示例性质，真实后端与 Docker 集成仍需部署环境联调。

## 技术栈

| 领域 | 选型 | 对应 Vue 侧 |
| --- | --- | --- |
| 构建 | Vite 8 + `internal/vite-config`（自建插件链） | 同（共用 catalog 版本） |
| 框架 | React 19 | Vue 3.5 |
| UI 库 | antd 6 | antdv-next 1.3 |
| 样式 | Tailwind 4 + CSS 变量设计令牌 | 同 |
| 状态 | Zustand（vanilla store + React 绑定） | Pinia |
| 命令式 API | `@tanstack/react-store` | `@tanstack/vue-store` |
| HTTP | axios（`@vben/request`，原样搬运） | alova + `@alova/adapter-axios` |
| 加解密 | crypto-js + jsencrypt（原样搬运） | 同 |
| 国际化 | i18next | vue-i18n |
| 打包 | tsdown（仅 `internal/*`）+ rolldown 分包 | 同 |
| 测试 | vitest + happy-dom | vitest + happy-dom |

## 目录

```
walnut-frontend-react/
  apps/web-react/          # React 管理应用
    src/__fixtures__/      # 从运行中后端抓取的真实契约样本
  packages/
    @core/shared/          # ← @vben-core/shared       纯 TS，原样搬运
    @core/typings/         # ← @vben-core/typings      类型契约
    @core/design/          # ← @vben-core/design       CSS 变量设计令牌 + Tailwind 入口
    @core/preferences/     # ← @vben-core/preferences  偏好/主题（Zustand 重写）
    preferences/           # ← @vben/preferences       应用层覆盖入口
    hooks/                 # ← @vben/hooks             useAntdDesignTokens
    constants/  types/     # 常量与类型
    locales/               # i18next 实现 + 与 Vue 侧共用的词典 JSON
    utils/                 # RSA/AES/SM2/SM4 加密 + tree/mitt/uuid 等
    request/               # axios 客户端 + 401 刷新队列 + SSE/上传/下载
  internal/
    tsconfig/              # TS 预设（web.json 已改 jsx: react-jsx）
    tailwind-config/       # Tailwind 4 主题
    node-utils/            # Node 侧工具（tsdown 构建）
    vite-config/           # 自建 Vite 配置与插件链（tsdown 构建）
```

## 命令

```bash
cd walnut-frontend-react
pnpm install            # postinstall 会自动 tsdown 构建 internal/*
pnpm check              # lint → typecheck → unit → production build
pnpm test:unit          # vitest，424 个用例
pnpm typecheck          # tsc --noEmit
pnpm dev                # http://localhost:8020
pnpm build
```

开发端口为 **8020**（`VITE_PORT`），避开 Vue 版容器的 8010。
API 经 Vite 代理 `/api` → `http://localhost:8011`（三后端任选其一）。

> ⚠️ `dev` 脚本必须带 `--mode development`。`internal/vite-config` 的 `getConfFiles()`
> 是靠正则匹配 `npm_lifecycle_script` 里的 `--mode xxx` 来判断环境的，
> 漏写就会静默加载 `.env.production`（表现为端口掉回 5173）。

## 历史实施记录：阶段 ①

`pnpm dev` 打开 http://localhost:8020 是一个自检面板，覆盖：

1. **工作区内核解析** —— `@vben/request`、`@vben/constants`、`@vben-core/shared`、`@vben/utils`、`@vben-core/design` 在浏览器 bundle 里全部可用
2. **浏览器内加解密** —— AES-ECB/PKCS7 往返一致；用真实开发环境公钥做 RSA 加密得到 172 字符的 `encrypt-key`（1024-bit 预期值）
3. **设计令牌** —— `--primary` 等 CSS 变量随 `@vben-core/design/tokens` 注入并生效
4. **`.env` 契约** —— `VITE_GLOB_*` 变量名与 Vue 侧完全一致
5. **国际化** —— i18next 加载与 Vue 侧**逐字节相同**的词典 JSON，`{0}` 占位符在加载时转成 `{{0}}`
6. **真实后端链路** —— `POST /auth/login` → `GET /system/user/getInfo` → `GET /system/menu/getRouters`，响应信封 `{code,msg,data}` 由 `defaultResponseInterceptor` 拆包

测试覆盖：23 个测试文件 / 222 个用例，其中新增 **20 个加解密用例**（源项目该模块无任何测试）与 **14 个 i18n 用例**。

## 阶段 ② 验证了什么

自检面板新增第 5 张卡片「主题系统」，覆盖：

1. **`internal/vite-config` 全链路** —— `_app-config-*.js` 运行时配置注入（`window._VBEN_ADMIN_PRO_APP_CONF_`，含 RSA 密钥与加密开关）、`inject-app-loading` 首屏加载与暗色预判、HTML 压缩、license、archiver（`dist.zip`）、gzip/brotli 压缩
2. **rolldown 分包** —— `framework`(219 kB) / `antd-theme`(125 kB) / `vben-core`(187 kB) / `vben-request`(111 kB) / `vben-locale`(91 kB) / `antd-overlay`(86 kB) 等按组拆分
3. **Tailwind 4** —— `@vben/tailwind-config/theme` 经 `@vben-core/design` 根入口生效，`.text-3xl → font-size: 30px` 实测通过（面板里有自动校验）
4. **preferences → Zustand** —— `updatePreferences` 驱动状态、`localStorage` 持久化（命名空间 `walnut-seed-admin`，与 Vue 侧同 key 方案）、刷新后恢复、`<html class="dark">` 与 `data-theme` 同步
5. **CSS 变量 → antd 6 单向数据流** —— `useAntdDesignTokens()` 把 `--primary`/`--background`/`--radius` 等读回来喂 `ConfigProvider`；实测明暗两种模式下 antd 主按钮渲染色与 `--primary` **精确相等**（`rgb(20, 118, 255)`），`borderRadius` 由 `radius: '0.375'` rem 正确换算成 6px

测试覆盖增至 **25 个文件 / 263 个用例**（新增 preferences 的 26 个）。

## 搬运过程中的实际发现

### 1. `RsaEncryption.decrypt` 有一个潜在缺陷（已修）

`jsencrypt` 在密钥不匹配时返回 **`null`**，而源码只判断了 `ret === false`：

```ts
// 修改前
if (ret === false) throw new Error('RsaEncryption decrypt error');
// 修改后
if (ret === false || ret === null) throw new Error('RsaEncryption decrypt error');
```

不修的话 `null` 会流到 `decodeBase64()`，抛出一个难以定位的
`TypeError: Cannot read properties of null (reading 'length')`。
**建议同步回 `walnut-frontend`（Vue 侧同一处代码）。**

### 2. RSA 是「两对密钥」，不是一对

`.env` 注释里已写明，但容易看漏：

- `VITE_GLOB_RSA_PUBLIC_KEY` → 前端**加密请求**，后端用配对的私钥解密
- `VITE_GLOB_RSA_PRIVATE_KEY` → 后端**加密响应**，前端用这把私钥解密

响应方向后端会**另生成一次性 AES 密钥**、用响应公钥加密后放进响应的
`encrypt-key` 头，并不是把请求头原样回传。`packages/utils/src/encryption/__tests__/encryption.test.ts`
里的信封测试把这套双向流程固定下来了。

### 3. Go demo 容器当前**关闭**了服务端解密

`docker-compose.go.demo.yml` 没有配 `API_DECRYPT_PUBLIC_KEY` / `API_DECRYPT_PRIVATE_KEY`，
于是 `walnut-backend-go/internal/config/validate.go` 把
`APIDecryptDisabledReason` 置为「RSA 密钥未配置」，解密中间件不生效。

而 `.env.development` 里 `VITE_GLOB_ENABLE_ENCRYPT=true`。
两边不一致时，后端会把 AES 密文当 JSON 解析，返回
`请求数据格式错误（JSON 解析失败）`。

**要跑通加密链路**，给 Go 容器补上配对的密钥（可取
`walnut-backend-python/env/.env.dev` 里的那一对），或把前端
`VITE_GLOB_ENABLE_ENCRYPT` 设为 `false`。自检面板的登录区默认走未加密路径，
并把这个原因显示在界面上。

### 4. `filterTree` 会原地改写入参

```ts
if (node[childProps]) {
  node[childProps] = _filterTree(node[childProps]);  // ← 直接写回原对象
}
```

而且它**不保留匹配节点的祖先**：过滤 `{id:1,children:[{id:3,children:[{id:4}]}]}`
保留 `id===4` 会得到空数组，因为 id:3 不匹配就被整枝剪掉了。
阶段 ④ 用它过滤后端菜单时必须先 `structuredClone`，并注意这个剪枝语义。

### 5. 后端菜单树的真实形状（已抓成 fixture）

`src/__fixtures__/backend-menu-tree.json` 是从运行中的 Go 后端实抓的 16 节点菜单树。
阶段 ④ 的 `backMenuToRouteObjects` 应以它为输入写单测。关键点：

- 节点字段：`{name, path, hidden, redirect, component, alwaysShow, meta, children}`
- `meta`：`{title, icon, noCache, link, activeMenu}`
- `component` 特殊值实测出现两种：`Layout`（一级目录）、`ParentView`（多级目录）
- `name` 带菜单 ID 后缀（`System1`、`User100`），且可能含 `/` 与 `:`
- **带参路径**：`user-auth/role/:userId`、`role-auth/user/:roleId`、`dict-data/index/:dictId`
  —— 这些配 `hidden: true` + `meta.activeMenu` 指回列表页。
  React Router 7 的 `:param` 语法与 vue-router 一致，可直接沿用
- `dashboard` / `profile` **不在**后端返回里，来自前端静态模块（`accessMode: 'mixed'`）

### 6. `AesEncryption.decrypt` 在密钥错配时会抛 `Malformed UTF-8 data`

```ts
return decrypted.toString(CryptoJS.enc.Utf8);  // ← 乱码不是合法 UTF-8 时抛错
```

用错误的 key 解密得到的是随机字节，`toString(Utf8)` 大概率抛
`Malformed UTF-8 data`；只有当乱码偶然构成合法 UTF-8 时才返回垃圾字符串。
这个不确定性最初让加解密测试偶发失败（约 1/5）。

实际影响：**前后端密钥没配对时，看到的会是这条 crypto-js 报错，而不是「密钥错误」**。
排查加密链路问题时先怀疑密钥配置。阶段 ③ 搬 `utils/http` 时建议在解密处包一层，
把它转成可定位的错误信息。

### 7. `sideEffects` 字段会让整条 CSS 链被摇掉（阶段 ① 埋的雷）

给 `@vben-core/design` 加上 `"sideEffects": ["**/*.css"]` 后，因为入口是
`src/index.ts`（一个只做 `import './xxx.css'` 的 TS 文件），打包器认为它无副作用而整体丢弃，
连带 design-tokens / transition / nprogress / ui 全部 CSS 都不进产物。

**dev 模式看不出来**（Vite 按模块注入样式），只有 `vite build` 才暴露。
Vue 侧该包**没有** `sideEffects` 字段。教训：入口是 TS、内容全是 CSS 导入的包，不要声明 sideEffects。

### 8. tsdown 会把 `devDependencies` 打进产物

`skipNodeModulesBundle: true` 挡不住——tsdown 只把 `dependencies`/`peerDependencies` 视为外部。
`vite`、`@vitejs/plugin-react`、`sass-embedded` 等原先放在 devDependencies，
结果 `internal/vite-config/dist` 膨胀到 **3.2 MB**（把整个 vite 打进去了）。
移到 dependencies + vite 改 peerDependencies 后降到 **36 KB**。

### 9. tsdown 的 dts 生成在这两个包上不可用

- `vite-config`：postcss 类型报一片 `MISSING_EXPORT`
- `node-utils`：生成的 `dist/index.d.ts` 本身语法有误（`TS1005: ',' expected`）

改用双轨 exports 绕过：`types → ./src/index.ts`、`default → ./dist/index.mjs`。
类型走源码、运行时走产物，与 Vue 侧各包的做法一致。

### 10. rolldown 分包 priority：framework 必须高于 antd

`framework` 组原先 priority 30、`antd-theme` 46，结果 **react-dom 被吸进了 antd-theme**
（336 kB）。把 framework 提到 60 后恢复正常（antd-theme 降到 125 kB）。
共享底座类的分组优先级必须最高。

### 11. antd 解析不了 CSS Color 4 的空格语法 HSL ⚠️

`hsl(215 100% 54%)` 传给 antd 的 `colorPrimary`，实测渲染成 **`rgb(20, 103, 220)`**，
而该颜色的真实值是 `rgb(20, 118, 255)` —— 饱和度与亮度都被读偏了。
必须先 `convertToRgb()` 归一化成 `rgb(...)` 再传。

**这条建议回查 Vue 侧**：`packages/effects/hooks/src/use-design-tokens.ts` 里
`useAntdDesignTokens` 正是直接拼 `` `hsl(${value})` `` 传的，而同文件的
`useNaiveDesignTokens` 却用了 `convertToRgb` —— 两个函数不一致，
Vue 版的 antd 组件很可能与自绘组件存在同样的细微色差。

### 12. Vue 侧有三个测试是「假绿」的

移植时跑出来的，都不是 React 版引入的问题：

| 测试 | 问题 |
| --- | --- |
| `@core/preferences/__tests__/config.test.ts` | 快照已过期（缺 `buttonWaveMode`，且 `mode`/`lockScreen` 是被污染的旧值）。**在 Vue 仓库里直接跑也是红的** |
| `preferences.test.ts` · "does not update undefined preferences" | `expect(getPreferences()).toEqual(originalPreferences)`，而 `originalPreferences` 就是 `readonly(state)` 同一个代理对象，更新后两边指向同一份数据，**断言恒真** |
| `preferences.test.ts` · "ignores updates with invalid preference value types" | 同上。实际上主偏好**根本不做类型校验**，只有扩展偏好走 `sanitizeCustomPreferences` |

React 版 `getPreferences()` 返回不可变快照，这三条立刻暴露，已按真实行为重写并补了一条
「快照语义」测试（更新后必须是新引用，否则 React 感知不到变化）。

### 13. 量 DOM 别用 rAF，也别用 useLayoutEffect

自检面板要读 antd 按钮的实际渲染色，踩了两个坑：

- `requestAnimationFrame`：页面在**隐藏标签页**（`visibilityState: hidden`）里根本不触发，探针永远停在「测量中」
- `useLayoutEffect`：触发太早，antd 的 CSS-in-JS 还没把新 token 落到样式表，暗色首屏会读到旧色

最终用 `useEffect` + `setTimeout(…, 80)`：隐藏页里 setTimeout 只是被节流、仍会执行，时机也够晚。

## 与 Vue 侧的差异（有意为之）

| 项 | 说明 |
| --- | --- |
| 包名仍是 `@vben/*` / `@vben-core/*` | 让 ~50 个搬运文件的内部 import 零改动。改名是一次纯机械 codemod，留待后续 |
| 业务包 `exports` 直指 `src/*.ts` | 不跑 tsdown 预构建，Vite/vitest 直接消费 TS 源码 |
| `internal/*` 例外，仍用 tsdown 构建 | Vite 加载配置文件时会 externalize 裸导入，Node ESM 无法解析无扩展名的目录导入，必须产出 `dist/index.mjs`。类型走 `src`、运行时走 `dist` |
| `@vben/locales` 用 i18next 重写 | 词典 JSON 与 Vue 侧共用；`{0}` → `{{0}}` 在加载时转换 |
| `usePreferences()` 返回普通值而非 `ComputedRef` | 键名与 Vue 版逐一对应，搬调用点时去掉 `.value` 即可 |
| `getPreferences()` 返回不可变快照 | 命令式代码（请求拦截器等）每次调用取最新值；**组件里必须用 `usePreferences()`**，快照不会触发重渲染。`preferences` 那个导出做成了实时 Proxy 以兼容旧写法，但同样不触发重渲染 |
| `useAntdDesignTokens` 用 `convertToRgb` 归一化 | 修掉 Vue 侧直传 `hsl(H S% L%)` 导致的色差，见发现 11 |
| vite-config 裁掉 `importmap` / `nitro-mock` / `vxe-table` / library 构建 | 分别对应：CDN externals（未用）、mock 服务（有真后端）、vxe-table（换 ProTable）、包构建（源码直出） |
| 暂未搬运 | `generate-routes-*` / `generate-menus` / `reset-routes`（依赖 vue-router 类型，属阶段 ③④）、`typings/tabs.ts`（依赖 `RouteLocationNormalized`，属阶段 ⑥）、`@vben/hooks` 其余 8 个 hook |

## 下一步（阶段 ③）

1. `@vben/stores`：Pinia → Zustand（access / user / tabbar / timezone），沿用 `secure-ls` 生产加密存储
2. `utils/http`：alova → axios，把 RSA+AES 装配、`*WithMsg` 变体、401 登出接到 `@vben/request`
3. SSE/WebSocket 通知提到组件外的 service 单例（为阶段 ⑥ 的 `<Activity>` 页签缓存铺路）
4. lint 体系：eslint/oxlint 的 `react.ts` 替代 `vue.ts`，接入 turbo
5. 补齐阶段 ①/② 跳过的 `generate-routes-*`、`generate-menus`（改为输出 React Router `RouteObject`）

> 阶段 ② 的实施记录与踩坑清单见
> [`../docs/frontend/05-React迁移-逐文件清单.md` §14](../docs/frontend/05-React迁移-逐文件清单.md)。


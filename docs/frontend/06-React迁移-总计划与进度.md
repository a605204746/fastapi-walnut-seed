# React 迁移总计划与进度

> **最新状态（2026-11-19）：现有 Vue 功能迁移实施覆盖 100%，后端菜单组件 14/14。** 角色/公告、仪表盘、共享控件、七种布局与外壳交互均已补齐，工程与样式门禁通过。最新范围、实测结果和联调边界见 [07-React迁移-验收记录](./07-React迁移-验收记录.md)。
>
> **2026-11-22 冻结决策**：Vue 基线（`walnut-frontend/`）冻结作对比参照，React（`walnut-frontend-react/`）为唯一活跃前端；React 仓同日完成标识符级去 vben 改名（`@walnut/*` / `@walnut-core/*`，CSS 类与运行时配置键同步），迁移进入验收维护期。
>
> 下文是 2026-11-05 至 2026-11-19 的历史计划/阶段记录；其中约 50%、12/14、剩余人日等数字属于当时快照，不代表当前状态。

> 基线：commit `7eb4afd`（2026-11-15）· 源：`walnut-frontend/`（Vue 3.5 / Vben 5.7）· 目标：`walnut-frontend-react/`（React 19 / antd 6）
>
> **本文所有数字均为 2026-11-15 实测**，非计划转述。逐文件级的迁移分类见 [05-React迁移-逐文件清单](./05-React迁移-逐文件清单.md)；本文只回答两个问题：**现在到哪了**、**剩下怎么排**。
>
> 复核命令见 §11。

---

## 0. 结论速览

> 进度快照：**A1 通路 + A2 全部 7 页 + A3 的 user 页已于 2026-11-15 完成**（实施记录见 §9、§10）。
> 下表「基线」列是开工前的评估，保留作对照；「当前」列为最新实测。

| 问题 | 基线（开工前） | 当前 |
|---|---|---|
| **总体进度** | ≈ 35%（区间 30–40%，口径见 §2） | **约 50%** |
| 地基 / 骨架 | ≈ 85% | **≈ 95%**（表单/表格/弹窗适配层已覆盖全部页面形态，仅剩富文本与自绘 UI） |
| 业务功能面覆盖 | **7%**（14 个契约 component 命中 1 个） | **86%**（命中 12 个，剩 `role` / `notice`） |
| 已消耗工作量 | ≈ 26–30 人日 | — |
| **剩余工作量** | 63–88 人日（清单原口径 38–52，差异见 §5.0） | **约 35–55 人日**（§10.5 砍掉 ~6,400 行死代码 + 7 个页面已交付） |
| 质量基线 | 单测 31 文件 / 381 用例 | **34 文件 / 415 用例**；typecheck 16 project 零错误；像素门禁 5 目标全过且**三次复测数值零漂移** |
| 最大风险 | 「1:1 像素级复刻」独占剩余工作量 29% | 仍是范围问题：阶段 B（18–26 人日）现在占剩余量的 **约 45%**，且只有视觉收益 |
| 当前阻塞 | 13 个业务页退化为 `NotFoundComponent` | 无硬阻塞。`role` 缺 `#/components/tree`(923)、`notice` 缺 `#/components/tiptap`(1,526，需装 `@tiptap/react`) |

**一句话**：最难、最不可并行、最容易全盘返工的部分（加密链路、动态路由注入、vxe→antd 表格适配、antd 6 主题桥接）已经过去并且有门禁兜底；剩下的是量大、低风险、**可以多人并行铺开**的工作。

---

## 1. 实测基线

### 1.1 可运行的验证（本次实际执行）

```
$ pnpm install          # 成功，exit 0
$ pnpm test:unit        # Test Files 31 passed (31) / Tests 381 passed (381) / 4.24s
$ pnpm typecheck        # 16 个 project 全部 Done，零错误
$ pnpm build            # 成功，exit 0；dist.zip 2,610,153 bytes（2.49 MB）
                        # gzip 分包：antd-form 131.5 · framework 98.2 · vben-core 74.7 ·
                        #   antd-theme 49.4 · antd-data 41.9 · antd-overlay 39.5 ·
                        #   vben-request 36.2 · vben-locale 28.6 kB
```

四项与清单 §16.5 记录的基线**逐项吻合**（`dist.zip` 从 2.46 → 2.49 MB，分包大小完全一致）。**文档记录可信。**

> 唯一未复现的是像素门禁：需 Playwright 浏览器 + 同时起两端 dev server，成本高。§16.5 有实测记录，本次采信但未重跑（`e2e/visual/artifacts/vue/` 的 5 张 golden 已入库，可单独再生 React 端比对）。

### 1.2 代码量对比

统计口径：`.vue/.ts/.tsx/.css/.scss`，排除 `node_modules`、`dist`、`__tests__`、`*.test.*`、`*.spec.*`；不含 `internal/`、`scripts/`、根配置。

| 层 | Vue 行数 | React 行数 | 说明 |
|---|---|---|---|
| `packages/@core` | 24,715 | 6,839 | React 缺 menu-ui / tabs-ui / layout-ui / shadcn 大部分原语 |
| `packages/effects` | 26,189 | — | React 侧已拆解：request → `packages/request`(697)，其余未迁 |
| `packages/{utils,stores,locales,constants,types,preferences,hooks,request}` | 3,584 | 3,540 | **基本迁完**（缺 timezone store） |
| `packages/{icons,styles}` | 682 | 0 | icons 经 §16.4② 决策**不迁**；styles 并入 `@core/design` |
| `apps/*/src` | 21,288 | 5,029 | 业务层是主缺口 |
| **合计** | **75,776**（923 文件） | **15,508**（190 文件） | **20.5%** |

### 1.3 功能面对比（最硬的指标）

行数会被 D/E 类删减扭曲，**页面覆盖率不会**。以 `apps/web-react/src/__fixtures__/backend-menu-tree.json`（Go 后端实抓的 16 节点菜单树）里出现的 14 个 `component` 为准：

| 后端下发的 component | 基线（开工前） | 当前 |
|---|---|---|
| `system/config/index` | ✅ 已实现（403 行） | ✅ |
| `system/post/index` | ❌ | ✅ A2 |
| `system/dept/index` | ❌ | ✅ A2（树形表格） |
| `system/dict/index` | ❌ | ✅ A2（三层嵌套） |
| `system/dict/data` | ❌ | ✅ A1（占位，与 Vue 一致） |
| `system/client/index` | ❌ | ✅ A2 |
| `system/role/authUser` | ❌ | ✅ A1 转发 + A2 role-assign 落地后**已生效** |
| `system/user/authRole` | ❌ | ✅ A1（占位，与 Vue 一致） |
| `system/user/index` | ❌ | ✅ A3（含 4 个子弹窗） |
| `monitor/logininfor/index` | ❌ | ✅ A2 |
| `monitor/operlog/index` | ❌ | ✅ A2 |
| `system/menu/index` | ❌ | ✅ A2（树形表格 + 角色门禁） |
| `system/role/index` | ❌ | ⏳ A3（缺 `#/components/tree`，923 行） |
| `system/notice/index` | ❌ | ⏳ A3（缺 `#/components/tiptap`，1,526 行） |

**基线命中 1 / 14 = 7% → 当前 12 / 14 = 86%。** 只剩 `role` 与 `notice` 两页，
各缺一个富组件（菜单/部门权限树、Tiptap 富文本），详见 §10.5。

已落地页面对应的路由不再退化为 `NotFoundComponent`（兜底逻辑见 `router/access.ts:38-41`），
由 `router/__tests__/page-map.test.ts` 的 4 条用例持续守门。

> ⚠️ 三个占位页 `system/user/authRole`、`system/role/authUser`、`system/dict/data` 被后端 `sys_menu.component` 直接引用（清单 §12 已警告不可删）。React 侧缺失它们意味着**带参详情页 `user-auth/role/:userId` 等一律 404**。

其它可量化的覆盖面：

| 面 | Vue | React | 覆盖率 |
|---|---|---|---|
| 表单控件注册表（`adapter/component`） | 31 个组件名（350 行） | ~~9 个~~ → **A1 后 24 个** | ~~29%~~ → **77%**（余 7 个富控件在 A3） |
| API 文件 | 40 个 / 2,051 行 | ~~13 个 / 335 行~~ → **A1 后 40 个 / 1,781 行** | ~~core 5/6、业务 5/31~~ → **40/40 = 100%** |
| shadcn-ui 原语 | 24 个组件目录 / 8,211 行 | 5 个文件 / 510 行 | **17%**（Vue 的 24 个中已覆盖 4 个：button / dropdown-menu / icon / tooltip；`toggle-group` 为 React 侧新增） |
| app 级富组件（`components/`） | 10 个目录 / 5,009 行 | 417 行（dict / page / access / walnut-slogan，其中 page·access 对应 Vue 的 `common-ui`·`effects/access`） | **8%**；缺最重的 5 个：upload / tiptap / tree / cropper / tinymce（合计 4,416 行） |
| 顶栏 widgets | 8 个子目录 + 5 个根级 `.vue` / 5,742 行 | 后台顶栏 4 项（主题切换 / 语言 / 简化版用户下拉 / antd 原生面包屑）+ 认证页 4 项 | **≈ 30%**；缺 global-search / notification / lock-screen / preferences 抽屉 / timezone / check-updates / fullscreen |
| 前端 CI | 无 | 无 | `.github/workflows/` 只有 `go-ci.yml` |
| Docker 编排 | 有（nginx） | 无 | `docker/` 下无 react 前端 |

### 1.4 为什么「行数 20%」不等于「进度 20%」

三类系统性缩水，导致 React 侧行数天然少于 Vue：

- **D 类（库替代，源码删除）**：`resize` 1,269 → antd `Splitter`；`cropper` 981 → `react-cropper`；`code-mirror` 90 → `@uiw/react-codemirror`；`json-viewer` 172 → `@microlink/react-json-view`；vxe-table 插件 2,320 → `adapter/grid.tsx` 395 行
- **E 类（Vue 专有 / 死代码）**：`loading` 指令 199、`tippy` 指令 167、`route-cached` 136、`icons` 集中再导出层 440
- **决策性不迁**：`@vben-core/icons` 包（§16.4②）、`common-ui` 中**未被 app/layouts 导入面引用**的组件

最后一条是本次新查出来的**范围收缩**，值得单独记账。从 `@vben/common-ui` 的 barrel 实际导入面反查（命令见 §11），app + layouts 真正消费的只有：`Page`、`Fallback`、`About`、`ApiComponent`、`IconPicker`、`JsonPreview`、`Analysis*`(3)、`Workbench*`(6)、`Authentication*`(6)、`Profile*Setting`(4)、`prompt`、`globalShareState`。

**以下 common-ui 组件在本仓无任何导入点，建议直接从迁移范围移除**（合计 ~4,800 行 Vue 源码）：

`captcha`(1,131) · `resize`(1,269) · `cropper`(981，app 用的是自己的 `#/components/cropper`) · `ellipsis-text`(232) · `markdown`(238) · `json-viewer`(172) · `col-page`(137) · `count-to`(176) · `tippy`(167) · `code-mirror`(90) · `tree`(25，app 用的是自己的 `#/components/tree`) · `loading`(199)

> 清单 §6.1 把这 17 个组件全列为待迁（~4,900 行）。按导入面裁剪后只剩 5 个（`api-component` / `icon-picker` / `json-preview` / `page` / `captcha` 的 app 层替代版）。**这一刀省 8–12 人日，且不影响任何现有功能。** 需你确认（§8 决策项 D1）。

---

## 2. 三个口径的进度，别混用

同一个项目会得出三个差很多的数字，因为它们回答的是不同问题：

| 口径 | 数值 | 含义 | 该对谁说 |
|---|---|---|---|
| **纵向切片**（能不能跑通） | **≈ 85%** | 登录 → 拿 token → 拉菜单 → 注入路由 → 渲染外壳 → 页签 → 一个完整 CRUD 页，全链路已通且有两个 PoC 关卡验收 | 对技术评审：**架构风险已清零** |
| **阶段加权**（清单 ①–⑩ 拓扑） | **≈ 35%** | ①②④⑤⑥-a 完成，③ 大部分完成，⑥-b⑦⑧⑨⑩ 未开工或部分 | 对项目排期：**这是唯一该用于汇报的总数** |
| **横向覆盖**（功能面） | 基线 **≈ 7–10%** → 当前 **≈ 71%** | 契约页 1/14 → **10/14**；表单控件 9/31 → **25/31** | 对业务方：主要管理页已可用，**但 role/user/notice/menu 四页仍缺，尚不能整站替代 Vue** |

阶段加权明细：

| 阶段 | 内容 | 状态 | 完成度 | 证据 |
|---|---|---|---|---|
| ① | 基建 + 内核搬运 | ✅ | 100% | §13.1 |
| ② | preferences + design tokens + antd 主题桥 | ✅ | 100% | §14 |
| ③ | stores + i18n + `utils/http` | ⚠️ | 80% | access/user/tabbar ✅；**timezone store ❌**；**lint + turbo ❌** |
| ④ | router + 动态菜单 + access（PoC 关卡 1） | ✅ | 100% | 运行时接线在 ⑥-a 补齐（§16.2） |
| ⑤ | adapter + grid + form-ui（PoC 关卡 2） | ⚠️ | 85% | grid 395 ✅；shadcn 原语 5/24；**popup-ui 的 drawer ❌** |
| ⑥-a | 可用后台外壳 | ✅ | 100% | `BasicLayout` + `adapter/menu` + `adapter/tabs` |
| ⑥-b | 外壳 1:1 补齐 | ⚠️ | **~30%**（基线 0%） | 已做：顶栏齿轮/全屏/刷新、侧边栏底栏、面包屑链接色、**偏好设置抽屉功能版**；未做：自绘 menu-ui / tabs-ui、`<Activity>`、6 种布局模式、9 张 SVG 示意图（见 §10.7） |
| ⑦ | 14 个业务页 | ⚠️ | **86%**（基线 7%） | 已落地 12/14；剩 `role`（缺 tree）与 `notice`（缺 tiptap），见 §10.5 |
| ⑧ | 富组件（upload/tiptap/tinymce/tree/cropper） | ⚠️ | 15% | 新增 `ApiSwitch` / `SecretInput`；**`upload` 与 `icon-picker` 经实测为零引用死代码，可不迁**；仍缺 tiptap / tree / cropper |
| ⑨ | `_core` 认证全流程 + profile | ⚠️ | **~55%**（基线 30%） | **profile 已落地**（路由 + 左卡片 + 三页签，见 §10.7）；login ✅ / captcha ✅ / oauth ✅ / 403·404 ✅；register、forget-password、code-login、qrcode-login、social-callback、about、coming-soon、internal-error、offline ❌ |
| ⑩ | e2e + Docker + CI + 文档 | ⚠️ | 35% | 像素门禁 ✅（5 目标）；**CI ❌、Docker ❌** |

---

## 3. 分层进度矩阵

按 Vue 侧行数排序，缺口最大的一眼可见：

| Vue 模块 | Vue 行数 | React 对应 | React 行数 | 状态 |
|---|---|---|---|---|
| `effects/layouts` | 12,652 | `layouts/*` + `adapter/*`（含 grid 395） | 1,662 | ⚠️ 可用外壳，1:1 未做 |
| `effects/common-ui` | 9,167 | `components/page` | 61 | ❌ 按 §1.4 裁剪后仍需做 ~1,600（`ApiComponent` / `IconPicker` / `JsonPreview` / `Analysis*` / `Workbench*` / `Authentication*` / `Profile*Setting` / `Fallback` / `About`） |
| `@core/ui-kit/shadcn-ui` | 8,211 | 5 个原语 | 510 | ❌ 缺 20 个 |
| `apps/src/components` | 5,009 | 4 个目录（dict / page / access / walnut-slogan） | 417 | ❌ 缺 upload / tiptap / tree / cropper / tinymce（合计 4,416 行） |
| `apps/src/views` | 8,465 | config + login + fallback | 1,025 | ❌ 业务页 1/14 |
| `@core/ui-kit/form-ui` | 2,995 | `form-ui` | 522 | ⚠️ 骨架可用，校验/联动待验 |
| `@core/ui-kit/popup-ui` | 2,553 | `popup-ui/modal` | 297 | ⚠️ **drawer 未建** |
| `effects/plugins`(vxe/echarts/motion) | 2,320 | `adapter/grid.tsx` | 395 | ⚠️ 表格 ✅；**echarts ❌** |
| `@core/ui-kit/menu-ui` | 2,264 | `adapter/menu.tsx`（antd Menu） | 85 | ⚠️ 功能等价，自绘未做 |
| `apps/src/api` | 2,051 | `api/*` | 335 | ⚠️ core 5/6（缺 `upload.ts`），业务 5/31 |
| `@core/ui-kit/layout-ui` | 1,684 | — | 0 | ❌ 6 种布局模式 |
| `@core/base/shared` | 1,844 | `@core/shared` | 1,837 | ✅ |
| `@core/preferences` | 1,620 | `@core/preferences` | 1,619 | ✅ |
| `@core/base/design` | 1,354 | `@core/design` | 1,344 | ✅ |
| `utils` / `stores` / `request` / `locales` | 3,919 | 同名包 | 3,408 | ✅（缺 timezone store） |
| `@core/ui-kit/tabs-ui` | 948 | `adapter/tabs.tsx` + `tabs.css`（antd Tabs） | 259 | ⚠️ 功能等价，自绘未做 |
| `effects/{hooks,access}` | 1,253 | `packages/hooks`(61) + `components/access`(38) | 99 | ⚠️ 部分 |

---

## 4. 已建成的验收设施（这是本项目最大的资产）

`walnut-frontend-react/e2e/visual/` —— Playwright + pixelmatch，**以 Vue 端截图为 golden**，免后端可跑双端（`specs/mock-api.ts` 拦截 login / getInfo / getRouters / config 列表）。

| 目标 | 实测差异 | 阈值 | 性质 |
|---|---|---|---|
| `login-light` | 0.277% | 0.3% | ✅ 达标（像素级） |
| `login-dark` | 0.021% | 0.3% | ✅ 达标（像素级） |
| `config` | 5.245% | 6% | 回归基线，目标 1.5% |
| `shell` | **4.753%** | **5%** | 2026-11-15 修掉「浅色主题顶栏渲染成 antd 默认深蓝黑」后从 9.3% 砍半（§10.8），阈值已收紧 |
| `shell-dark` | **4.464%** | **4.6%** | 同上 |

`config` 元素截图 1024×599 vs Vue golden 1024×600 —— **外壳几何已对齐到 1px**，残余 5.245% 纯粹是 vxe-table ↔ antd Table 的渲染差异，与布局无关（§16.5 已证）。

⚠️ **门禁有两个已知盲区**，阶段 A 必须先补：

1. `mock-api.ts` 的 `getRouters` 只返回 **2 个节点**（`Layout` + `system/config/index`），而真实后端返回 **16 节点**。也就是说 `shell` 截图里的侧边栏只有「系统管理 / 参数配置」两项 —— **门禁没有覆盖真实菜单树的渲染**（多级展开、图标、带参隐藏路由、`activeMenu` 回指高亮）。
2. Playwright 实际视口是 1280×720（`devices['Desktop Chrome']` 覆盖了顶层 1440×900，§16.4⑥）。不影响比对结论，但任何"按 1440 设计"的假设都是错的。

---

## 5. 剩余工作总计划

### 5.0 排序原则与人日重估

**三条排序原则：**

1. **先通路后美化。** 阶段 A（业务页）与阶段 B（外壳 1:1）无依赖关系，但 A 有业务价值、B 只有视觉价值。A 先行。
2. **富组件按"被哪个页依赖"拆，不按组件类型批量做。** 实测依赖关系（`grep -rn "from '#/components/..."`）：
   - `system/role` ← `#/components/tree`(925)
   - `system/notice` ← `#/components/tiptap`(1,537) + `tinymce/helper`(301)
   - `_core/profile` ← `#/components/cropper`(768)
   - `system/dict/data`、`system/user/user-info-modal` ← `#/components/dict` ✅ **已迁**
   - `dashboard/analytics` ← `effects/plugins/echarts`
   - `upload`(885) 不由 view 直接导入，而是注册为表单控件 `FileUpload`/`ImageUpload`（`adapter/component/index.ts:53,57`）→ **属于 A1 通路的一部分**
   - 纯表格+表单、无富组件依赖的页：`post` `dept` `client` `menu` `dict` `logininfor` `operlog` `role-assign` —— **8 个页可以立刻并行开工**
3. **每完成一层就加一个像素门禁目标**（清单 §15.4③ 已确立为固定门禁）。

**人日重估 —— 为什么比清单的 38–52 高：**

清单 §15.4④ 给的「剩余 38–52 人日」是 2026-11-14 写的，本次自下而上重估为 **63–88 人日**。三个原因：

| 原因 | 影响 |
|---|---|
| 原估隐含的产能是 900–1,400 行/人日（P4 用 14–18 人日覆盖 15,525 行）。实测锚点：config 页 400 行 Vue → 403 行 React，连带 grid(395)+form-ui(522)+popup-modal(297)+门禁 ≈ 1,200 行基建，约 1.5–2 人日 | 首个页面 ~250 行/人日；基建复用后的边际产能按 **350–500 行/人日**（重复 CRUD 页）与 **200–300 行/人日**（新层，如 widgets / 自绘 ui-kit）分档计 |
| 原估把 §6.3 `basic/layout.vue` 441 行 + §6.4 `breadcrumb` 71 行判为 D 类「ProLayout 覆盖」。实测 `@ant-design/pro-components@2.8.10` 的 peer 是 `antd ^4‖^5`，与本仓 `antd ^6.7` 冲突（§16.1），**这部分工作量回来了**，且连带 menu-ui / tabs-ui 自绘 | +8–12 人日 |
| 原估 P5 只算 e2e + docs，未含前端 CI 与 Docker 编排（当前 `.github/workflows/` 只有 `go-ci.yml`） | +4–6 人日 |

> 这些估算只有**一个实测锚点**，误差 ±30%。**建议在执行完 A2 的前 3 个页面后重新基线化**（届时会有 4 个数据点，可以拟合出真实产能曲线）。

---

### 阶段 A：业务页通路（最高优先，32–43 人日）

目标：**13 个后端菜单页全部在 React 侧渲染出来并能 CRUD**，横向覆盖从 7% → 100%。

#### A1 通路（6–8 人日）—— ✅ **已完成，见 §9**

| 任务 | 产出 | 备注 |
|---|---|---|
| 补齐 `adapter/component.tsx` 注册表 | 9 → 23 个控件 | 先注册 antd 原生可直接映射的 14 个：`AutoComplete` `Cascader` `CheckboxGroup` `DatePicker` `Divider` `InputPassword` `Mentions` `Rate` `Space` `TimePicker` `TimeRangePicker` `Upload` `DefaultButton` `PrimaryButton`。富控件 8 个（`ApiSelect` `ApiTreeSelect` `ApiCascader` `IconPicker` `RichTextarea` `FileUpload` `ImageUpload`）留到 A3 |
| 补齐业务 API 层 | 26 个业务文件 + `core/upload.ts`；Vue 侧 1,435 行 → React ~1,050 行 | **B 类机械转换**（Vue 的 api 文件是纯 TS + 类型声明，实测 config 的 api 85 行 → React 62 行，0.73×）。注意 `model.d.ts` → `model.ts`；`upload.ts` 是 A3 富组件 `FileUpload`/`ImageUpload` 的前置 |
| 建 `popup-ui/drawer` | ~300 行 | Vue 侧 `useVbenDrawer` 被 8 个页面使用（dept / client / menu / post / role / role-assign / user / operlog），**是 A2 的硬前置** |
| 修正 `preferences.app.defaultHomePath` | 删掉 `/system/config` 覆盖 | §16.4④ 明确要求：dashboard 模块挂上后必须删；A1 先挂 `router/routes/modules/dashboard.ts` 占位或保持覆盖到 A4 |
| 三个占位页 | `system/user/authRole`、`system/role/authUser`、`system/dict/data` | 后端菜单直接引用，缺失即 404。**每个页面文件必须有 `export default`**（§16.4③ 硬约束） |

**A1 门禁**：`pageMap` 对 14 个 component 全部命中（写一个单测，遍历 `backend-menu-tree.json` 断言无 `NotFoundComponent` 退化）；`pnpm typecheck` 零错误。

#### A2 纯 CRUD 页 ×8（9–12 人日，**可 2–3 人并行**）—— ⚠️ **7/8 已完成，见 §10**

> 实际交付比计划多一个 `role-assign`（非契约页，但它让 `system/role/authUser` 路由生效），
> 只剩 `system/menu`（缺 `IconPicker`）。**实测转换率是 1.28× 而非计划假设的 1:1**，
> 且 A1 漏估了 5 项适配层能力 —— 详见 §10.3①②。

Vue 侧合计 3,684 行，按 1:1 转换率（config 实测 400→403）：

> 组件口径核对：A2 的 8 行覆盖后端菜单 13 个待做 `component` 中的 **8 个**（`dict` 一行含 `dict/index` + `dict/data` 两个 component，`role-assign` 不在后端菜单里、由 role 页跳入）；A3 覆盖剩下 **5 个**（`user/index`、`user/authRole`、`role/index`、`role/authUser`、`notice/index`）。8 + 5 = 13 ✓

| 页面 | Vue 行数 | 文件 | 难点 |
|---|---|---|---|
| `system/dict`（type + data） | 999 | 9 | 双子页 + 带参路由 `dict-data/index/:dictId` + `activeMenu` 回指 |
| `system/dept` | 511 | 3 | 树形表格（`expandable` + 无分页） |
| `system/post` | 451 | 3 | 最标准，**建议作为第一个做的页，用来标定产能** |
| `system/menu` | 441 | 2 | 树形表格 + `IconPicker` 依赖（A1 未含 → 先用 antd `Select` 降级，A3 换回） |
| `system/client` | 403 | 3 | `secret-input` 自定义控件 |
| `monitor/logininfor` | 320 | 2 | 只读列表 + 详情 modal |
| `monitor/operlog` | 321 | 2 | 只读列表 + **preview drawer**（依赖 A1 的 drawer） |
| `system/role-assign` | 238 | 2 | 带参 + 穿梭选择 |

**A2 门禁**：每页一个像素门禁目标（8 个新增），阈值首轮按实测校准、第二轮收紧；每页至少覆盖「列表渲染 + 新增 + 编辑 + 删除 + 搜索重置」5 条 DOM 探针断言（**不看缩略图，用 `innerText`/`count`** —— §16.4⑤ 的教训）。

#### A3 富组件依赖页（12–16 人日）

| 组件 | Vue 行数 | React 方案 | 估算行数 | 服务的页面 |
|---|---|---|---|---|
| `#/components/tree` | 925 | antd `Tree` + 移植 `helper.tsx`/`hook.tsx` 的纯逻辑 | ~350 | `system/role`(551) |
| `#/components/upload` | 885 | antd `Upload`，`hook.ts` 的 OSS 直传逻辑可搬 | ~400 | 注册为 `FileUpload`/`ImageUpload`，服务 `system/user` |
| `#/components/tiptap` | 1,537 | `@tiptap/react`；`extensions.ts`/`upload.ts`/`text-style.ts`/`resizable-image.ts` 框架无关，可直搬 | ~900 | `system/notice`(463) |
| `#/components/tinymce` | 301 | `@tinymce/tinymce-react` | ~200 | `system/notice` 的 helper |
| `common-ui/api-component` | 332 | TanStack Query + antd `Select`/`TreeSelect`/`Cascader` | ~250 | `ApiSelect`/`ApiTreeSelect`/`ApiCascader` 三个注册控件 |
| `common-ui/icon-picker` | 378 | 重写 | ~300 | `system/menu` |
| 页面本体 | 2,158 | — | ~2,200 | user(1,144) / role(551) / notice(463) |

**A3 门禁**：`system/user` 全链路（部门树筛选 + 新增 + 导入 + 重置密码 + 授权角色跳转）；`system/notice` 富文本存盘后回显一致；`system/role` 菜单权限树勾选态与 Vue 端逐项对齐。

#### A4 Dashboard（5–7 人日）

`effects/plugins/echarts`（约 800 行，含 `useEcharts` 响应式重绘 / 主题联动 / resize 观察）→ React 版 `useEcharts` hook；`common-ui/ui/dashboard` 的 `Analysis*`(3 个 / 119 行) + `Workbench*`(6 个 / 294 行)；页面 `analytics`(432) + `workspace`(266)。

**A4 门禁**：`router/routes/modules/dashboard.ts` + `profile.ts` 挂上，删除 `defaultHomePath` 覆盖；暗色主题下图表重绘（这是 `useEcharts` 最容易漏的一点）。

---

### 阶段 B：外壳 1:1（⑥-b，18–26 人日，**可与 A 全程并行**）

目标：`shell` 9.316% → <2%，`config` 5.245% → 1.5%。

按 §16.6 的差异面积排序：

| # | 任务 | Vue 参照 | 人日 |
|---|---|---|---|
| B1 | 顶栏右侧 widget 群：**新增** 全局搜索(Cmd+K) / 通知(SSE) / 全屏 / 偏好抽屉入口 / 锁屏 / 时区 / 检查更新；**替换** 现有的简化版用户下拉与 antd 原生面包屑为 1:1 版 | `effects/layouts/src/widgets/`（8 个子目录 + 5 个根级 `.vue`，实测 **5,742 行**；React 已迁其中 4 个认证页用的 toggle ≈ 200 行，`BasicLayout.tsx:227-229` 另有主题/语言/用户下拉/面包屑的简化版） | 8–10 |
| B2 | 自绘 menu-ui：手风琴、动画、`badge`/`activeIcon`、折叠按钮移到侧边栏右缘悬浮 | `@core/ui-kit/menu-ui` 2,264 | 4–6 |
| B3 | 自绘 tabs-ui：chrome 弧形页签、拖拽排序、滚轮横滚、中键关闭、`reload`/`maximize` 右键项 | `@core/ui-kit/tabs-ui` 948 | 3–4 |
| B4 | `<Activity>` + LRU 页签缓存（store 侧 `cachedTabs`/`excludeCachedTabs`/`refresh()` 已就位，只差消费者） | `layouts/src/route-cached/` 136 | 2–3 |
| B5 | 6 种布局模式 + 混合导航 | `basic/menu/` 441 + `layout-ui` 1,684 | 4–6 |
| B6 | 偏好设置抽屉 + 24 个 block + 9 个布局示意 SVG（B 类，SVGR 机械转） | `widgets/preferences` 577 | 2–3 |
| B7 | antd Table 主题覆盖层 | — | 1–2 |

> B 阶段的全部收益是视觉。**如果放弃"1:1 像素级"改为"功能等价 + antd 原生外观"，B 可压缩到 6–9 人日（只做 B1 的功能部分 + B4），总剩余量降到 51–71 人日。** 这是本文最大的一个决策项（§8 D2）。

**B 门禁**：`shell`/`shell-dark` 阈值逐轮下调（10% → 6% → 3% → 2%），每轮记录实测值；新增 `menu`/`tabbar`/`preferences-drawer` 三个元素级目标。

---

### 阶段 C：`_core` 全流程（⑨，6–8 人日）

| 任务 | Vue 行数 | 备注 |
|---|---|---|
| `register` / `forget-password` / `code-login` / `qrcode-login` | 698 | 两层：`common-ui` 实现（code-login 129 / register 121 / forget-password 114 / qrcode-login 102 / third-party-login 41）+ `apps/_core/authentication` 薄封装（144 / 95 / 42 / 10）。清单 §6.2 已指出两层**功能重叠，建议只保留 apps 层定制版** |
| `login-expired-modal` | 95 | 需接 `lib/request.ts` 的 401 分支（当前是整页刷新跳转，§15.4②） |
| `social-callback` | 88 | 依赖后端 social 模块 |
| `profile`（11 文件） | 767 + 223 | apps 层 767，加 `common-ui/ui/profile` 的 4 个 `Profile*Setting` 223；依赖 `cropper`(768 → `react-cropper` ~300) |
| `about` / `coming-soon` / `internal-error` / `offline` | ~200 | `Fallback`(187) + `About`(9) |
| fallback 插画 SVG | 894 | 5 个纯 SVG 文件（icon-403/404/500/coming-soon/offline），**B 类，SVGR 机械转**，低风险 |

> `AuthenticationLayout` + `AuthenticationFormView` + `login` + `InputCaptcha` + `OAuthLogin` 已在阶段 ⑤ 就位（apps 层 618 行中的 327 行），本阶段是增量。

**C 门禁**：注册 → 登录 → 改密 → 退出全流程 e2e；`login-expired-modal` 在 token 过期时弹出而非整页刷新。

---

### 阶段 D：交付面（⑩ + ③ 尾巴，7–11 人日）

| 任务 | 人日 | 说明 |
|---|---|---|
| lint 体系 + turbo | 2–3 | §14.3 第 4 项，从阶段 ③ 拖到现在。无 `internal/lint-configs`、无 `turbo.json` |
| **前端 CI** | 2–3 | `.github/workflows/frontend-react-ci.yml`：install → typecheck → test:unit → build → 像素门禁（golden 已入库，React 端可单独再生，**CI 里不需要起 Vue 端和后端**，这是 §15.2 入库策略的直接红利） |
| Docker 编排 | 1–2 | `docker/` 下加 react 前端服务；注意 `.env.production` 的 `VITE_GLOB_ENABLE_ENCRYPT=false` 与 `docker-compose.go.demo.yml:128-138` 的密钥约束 |
| e2e mock 补全 | 1–2 | `mock-api.ts` 的 `getRouters` 从 2 节点换成真实 16 节点（直接用 `backend-menu-tree.json`），补上 §4 的盲区 1 |
| `timezone` store | 0.5 | 132 行，B 类 |
| `.gitattributes` 补 `*.snap text eol=lf` | 0.1 | 实测：Windows 下跑 `pnpm test:unit` 会让 `packages/@core/preferences/__tests__/__snapshots__/config.test.ts.snap` 出现**纯行尾差异**（内容零变化，`git diff` 为空但 `git status` 报 M）。仓库根 `.gitattributes` 目前只对 `*.sh` 强制 LF |
| 文档回写 | 1 | 本文 + 清单 + 两个 README |

**D 门禁**：CI 上一次 push 跑完全部门禁；`docker compose up` 起 React 前端能登录并访问全部 14 页。

---

## 6. 依赖图与排期

```
A1 通路(6-8) ──┬──> A2 纯CRUD×8 (9-12, 可3人并行)
               │         │
               │         └──> A4 dashboard (5-7)
               │
               └──> A3 富组件页 (12-16) ──> C _core全流程 (6-8)
                        │
B 外壳1:1 (18-26) ──────┘  ← 全程独立，任何时候都能并行插入
                        
D 交付面 (7-11) ── 最后收口，但 CI(D) 建议提前到 A2 之后立即做
```

**关键路径**：A1 → A3 → C，约 24–32 人日。
**B 不在关键路径上**（除非选择"1:1 必须达成才算完成"）。

| 人力 | 工期 | 说明 |
|---|---|---|
| 1 人 | 13–18 周 | A→B→C→D 串行 |
| 2 人 | 8–11 周 | 甲走 A/C 关键路径，乙走 B + D |
| 3 人 | 6–8 周 | A2 的 8 个页天然可拆给第三人 |

> **建议把 D 的 CI 那一块提前**：在 A2 完成第一个页面后立刻建 CI。理由是 A2/A3 会产生大量重复性回归（8+ 个页面的门禁），手工跑 `pnpm test:unit && pnpm typecheck && playwright test` 的成本会在第 3 个页面开始超过建 CI 的成本。

---

## 7. 风险登记册

| # | 风险 | 等级 | 现状 | 缓解 |
|---|---|---|---|---|
| R1 | **人日估算只有 1 个实测锚点** | 🔴 高 | config 页是唯一完成的业务页 | A2 前 3 页做完立即重新基线化；每页记录实际耗时 |
| R2 | **antd 6 生态不成熟** | 🟡 中 | 已确认 `pro-components` peer 只到 antd 5（§16.1） | 不再依赖任何 Pro 系组件；遇到类似冲突先 `npm view <pkg> peerDependencies`（几秒钟，但清单里两条 D 类判断因此作废） |
| R3 | **`<Activity>` API 稳定性** | 🟡 中 | React 19.3 的 `<Activity>` 仍属较新特性，B4 完全依赖它 | B4 前做 1 天 spike：验证 LRU 卸载/保活、与 RR7 `patchRoutes` 的交互；不成立则退回"关闭页签即卸载"的降级语义 |
| R4 | **tiptap / tinymce 双富文本** | 🟡 中 | Vue 侧 notice 同时引用两者（1,838 行） | 先确认后端存的 HTML 结构由哪个编辑器产生，**只迁一个**，另一个做只读渲染 |
| R5 | **加密链路的环境不一致** | 🟢 低 | `7eb4afd` 已打通开发前端 → 容器后端；`docker-compose.go.demo.yml` 未配 `API_DECRYPT_*` 时后端自动停用解密 | 联调前先对齐前后端加密开关，否则会把后端配置问题误判为前端 bug（§13.2③ 已踩过） |
| R6 | **门禁盲区导致假绿** | 🟡 中 | mock 只有 2 节点菜单（§4） | D 阶段补全；在此之前**不要用 `shell` 截图的通过率作为菜单功能的验收依据** |
| R7 | **两套前端并存期的双向同步** | 🟡 中 | 清单 §13.2① 已发现 `impl/rsa.ts` 的 `null` 缺陷需回修 Vue 侧 | 建立"Vue 侧 bug 修复必须同步 React 侧"的检查项；反之亦然 |
| R8 | **页面文件漏 `export default`** | 🟢 低 | `React.lazy` 不校验未使用条目，typecheck 也发现不了，只在渲染时炸（§16.4③） | A1 门禁的 `pageMap` 全覆盖单测能一次性拦住 |

---

## 8. 决策项（D1 / D2 / 首个开工面已于 2026-11-15 确认）

| # | 决策 | 结论 | 影响 |
|---|---|---|---|
| **D1** | `common-ui` 中无导入点的 ~4,800 行是否移出迁移范围 | ✅ **已确认：移出**（captcha / resize / cropper / markdown / tippy / ellipsis-text / json-viewer / col-page / count-to / code-mirror / tree / loading） | 省 8–12 人日，零功能损失。**清单 §6.1 的 17 个组件缩减为 5 个** |
| **D2** | **「1:1 像素级复刻」是否仍是硬要求** | ✅ **已确认：保留，阶段 B 全做** | 剩余工作量维持 63–88 人日；`shell` 必须压到 <2%、`config` 压到 1.5% |
| **首个开工面** | A1 通路 / B1 widgets / D CI | ✅ **已确认：A1 通路** | 先解除「13 个页面全是 `NotFoundComponent`」 |
| **D3** | 人力配置 | ⏳ 待定：1 / 2 / 3 人 | 决定工期 13–18 / 8–11 / 6–8 周 |
| **D4** | 富文本编辑器 | 🔎 **实测后建议：只迁 tiptap**（详见 §10.5） | notice 的编辑器是 Tiptap；对 tinymce 的依赖只有一个 **14 行且当前是空实现**的 `contentWithOssIdTransform`。不迁 tinymce 的 268 行编辑器可省 3–5 人日，且不影响现有数据 |
| **D5** | Vue 端是否继续演进 | ⏳ 待定：冻结只修 bug / 继续加功能 | 继续加功能会持续产生双向同步成本（R7） |
| **D6** | React 端何时替代 Vue 端上线 | ⏳ 待定：A 完成即灰度 / 等 A+B+C+D 全完成 | 前者可提前 4–6 周拿到业务价值，但视觉与 Vue 端有差异（D2 已选保留 1:1，故建议等 B 完成再灰度） |

---

## 9. 阶段 A1 实施记录（2026-11-15）

A1「通路」已落地。目标是把 A2 的 8 个纯 CRUD 页所需的**全部公共前置**一次备齐，
使后续页面可以真正并行铺开。

### 9.1 交付项

| 项 | 产出 | 说明 |
|---|---|---|
| **业务 API 层** | 新增 27 个文件（`core/upload.ts` + `system/{client,dept,dict,menu,notice,post,profile,role,social,user}` + `monitor/{logininfo,operlog}`），3 个 barrel 补齐 | 用一次性脚本做机械替换（`#/` → `@/`、`#/utils/http` → `@/lib/request`、`model.d.ts` → `model.ts`、去 BOM），脚本用完即删。**API 文件覆盖 40/40 = 100%**（Vue 2,051 行 → React 1,781 行） |
| **表单控件注册表** | `adapter/component.tsx`：9 → **24 个控件**（Vue 31 个中除 7 个富控件） | 全部走 `lazyAntd()` 懒加载，对齐 Vue 的 `defineAsyncComponent`；新增 `withDefaultPlaceholder` 复刻 Vue 的默认 placeholder 语义（`ui.placeholder.input/select`，渲染期求值以跟随语言切换） |
| **`FormApi.updateSchema`** | `packages/@core/ui-kit/form-ui/src/form-api.ts` | ⚠️ 计划里漏项，实际是 A2 的**硬前置**：8 个抽屉页都在 `onOpenChange` 里靠它注入异步选项。语义与 Vue `form-api.ts:384` 逐条对齐（必须带 fieldName / 只更新已有字段 / 深合并 + 数组整体覆盖 / 累积生效） |
| **`popup-ui/drawer`** | `drawer-api.ts` + `drawer.tsx` + `use-drawer.tsx`（424 行，不含测试） | 与同目录 modal 刻意同构；`useVbenDrawer(options) → [Drawer, drawerApi]` 签名不变，8 个使用点可机械搬运 |
| **`utils/popup.ts`** | `useBeforeCloseDiff` + `defaultFormValueGetter` | 「表单有改动时关闭前二次确认」，Vue 用 `window.modal`，React 改走 `services/message.ts` 桥接单例 |
| **三个菜单引用的页面文件** | `system/user/authRole.tsx`、`system/dict/data.tsx`（占位）、`system/role/authUser.tsx`（转发） | 见 §9.3 的 ⑥⑦ 两条发现 |
| **A1 门禁** | `router/__tests__/page-map.test.ts`（4 例） | 补 `back-menu.test.ts` 的盲区：它只验「component 字符串对不对」，不验「字符串能不能查到组件」 |
| **i18n 补键** | `common.loading`（zh/en） | antd Drawer 无内置 footer 文案，Vue 侧用 spinner 组件绕过；React 侧需要这个键 |

### 9.2 验证结果（全部实测）

| 门禁 | A1 前 | A1 后 |
|---|---|---|
| `pnpm typecheck` | 16 project 零错误 | **16 project 零错误** |
| `pnpm test:unit` | 31 文件 / 381 用例 | **34 文件 / 415 用例**（+3 文件 / +34 用例） |
| `pnpm build` | 成功，dist.zip 2.49 MB | **成功，dist.zip 2.55 MB** |
| 像素门禁 | 5 项全过 | **5 项全过** |

像素门禁实测（`capture:react` + `compare`，golden 未重采）：

| 目标 | A1 前 | A1 后 | 阈值 | 变化 |
|---|---|---|---|---|
| `login-light` | 0.277% | **0.277%** | 0.3% | 持平 |
| `login-dark` | 0.021% | **0.021%** | 0.3% | 持平 |
| `config` | 5.245% | **5.215%** | 6% | ↓ 0.030pp（placeholder 文案现在与 Vue 一致） |
| `shell` | 9.316% | **9.344%** | 10% | ↑ 0.028pp（231 px / 92 万 px，噪声级） |
| `shell-dark` | 4.719% | **4.730%** | 5.5% | ↑ 0.011pp（噪声级） |

代码量：React 侧 15,508 → **17,907 行**（占 Vue 75,776 行的 23.6%）。

### 9.3 踩坑与对计划的修正（都不是读代码能发现的）

**① `.d.ts` → `.ts` 会丢失「隐式导出」。** Vue 的 `api/system/profile/model.d.ts` 里
`interface FileCallBack` **没有 `export`**，但 `.d.ts` 的模块级声明对 TS 隐式可见，
`profile/index.ts` 照样能 import。改名为 `.ts` 后规则失效，编译报 TS2459。
全仓仅此一处，已补 `export`。

**② antd 6 没有 `TimePicker.TimeRangePicker`。** Vue 侧 antdv-next 叫 `TimeRangePicker`，
antd 6 React 是 `TimePicker.RangePicker`。**注册表键名必须沿用 Vue 的 `TimeRangePicker`**，
否则搬运过来的 schema 查不到组件。

**③ `createLazyPageMap` 保留 glob 的原始相对 key**（`../../views/x/y.tsx`），
归一化发生在 `generateRoutesByBackend` **内部**。写门禁测试时若直接拿
`/system/config/index.tsx` 去比 `Object.keys(pageMap)` 会全部落空 —— 必须用同一个
`normalizeViewPath` 复现。第一版测试就栽在这。

**④ `views/` 下 6 个子组件文件缺 `export default`**（`AuthenticationLogin` / `InputCaptcha` /
`OAuthLogin` / `login` / `fallback` / `config-modal`）。这是 **SFC → TSX 的结构性差异**：
Vue 的 `.vue` 天然有隐式 default export，所以 `config-modal.vue` 混在
`import.meta.glob('../views/**/*.vue')` 里毫无风险；TSX 没有这个默认。
已全部补齐，并把门禁收紧为「`views/` 下所有 `.tsx` 都必须有 default export」。

**⑤ antd 6 Drawer 的三处 API 变更**（照抄 antdv-next 会报 deprecation）：
`destroyOnClose` → **`destroyOnHidden`**；`maskClosable` → **`mask={{ closable }}`**
（`MaskConfig` 另有 `enabled` / `blur`）；`width`/`height` 仍可用但官方推荐 `size`。

**⑥ `system/role/authUser` 不是占位页。** 清单 §12 把它和另两个并列为「注释写无实际意义」，
但实测 Vue 侧该文件是 `import RoleAssignPage from '#/views/system/role-assign/index.vue'`
—— 它是**功能页的转发**。React 侧用 `import.meta.glob('../role-assign/index.tsx')` 接线：
文件不存在时渲染显式待迁移提示，**A2 建好 role-assign 后本页自动生效，无需回来改**。

**⑦ `system/dict/data` 的解析优先级。** `generateRoutesByBackend` 把 `system/dict/data`
拼成 `/system/dict/data.tsx` 后**精确匹配**，所以命中的是占位页 `data.tsx`，
而**不是** `data/index.tsx`。Vue 侧 `.vue` 后缀下同理（`data.vue` 优先于 `data/index.vue`），
且 `views/system/dict/data/` 目录里本来就没有 `index.vue`（只有两个弹窗组件）。
**两端行为一致，保持占位即为 1:1。**

**⑧ 分包：动态 import 在本仓的分包配置下拆不出独立 chunk。** 注册 24 个控件后
`antd-form` 从 gzip 131.5 → **174.1 kB**。已按 Vue 的做法改成 `lazy()` +
`antd/es/*` 子路径动态导入，但实测**只比静态 import 小 0.3 kB** ——
根因是 `internal/vite-config/src/config/code-split.ts` 的 `matchAntdFormChunk`
把 `antd/es/{auto-complete,cascader,input,select,...}` 全部强制并进**同一个具名 chunk**，
动态 import 因此无法产生独立分包。

> 保留 lazy 写法（成本仅 0.3 kB，且让首屏从阻塞加载变为异步加载），
> 但**真正的减包杠杆在 `code-split.ts`**：要么细化 `matchAntdFormChunk` 让异步控件单独成块，
> 要么按「页面实际用到的控件」裁剪注册表。列入 A2 观察项，不在 A1 动共享构建配置。

**⑨ Playwright 浏览器版本需单独安装。** 仓库 catalog 指向 chromium **1255**，
本机原有 1223 / 1254，直接跑 `capture:react` 会报 `Executable doesn't exist`。
需要 `pnpm exec playwright install chromium`（国内走
`PLAYWRIGHT_DOWNLOAD_HOST=https://npmmirror.com/mirrors/playwright/`，实测 123.8 MiB 下载成功）。
**阶段 D 的 CI 必须包含这一步**，否则门禁在 CI 上直接失败。

**⑩ Windows 下跑单测会污染快照文件。** `pnpm test:unit` 后
`packages/@core/preferences/__tests__/__snapshots__/config.test.ts.snap` 出现在 `git status`
里，但 `git diff --numstat` 为空 —— **纯行尾差异，内容零变化**。
根因是仓库根 `.gitattributes` 只对 `*.sh` 强制 LF。已列入阶段 D。

### 9.4 A1 有意未做（顺延）

| 项 | 顺延到 | 原因 |
|---|---|---|
| `ApiSelect` / `ApiTreeSelect` / `ApiCascader` / `IconPicker` / `FileUpload` / `ImageUpload` / `RichTextarea` 7 个富控件 | A3 | 依赖 `api-component` / `icon-picker` / `upload` / `tinymce`，属富组件工作 |
| 删除 `preferences.app.defaultHomePath` 的 `/system/config` 覆盖 | A4 | 要先有 dashboard 静态路由模块，否则默认首页会 404（清单 §16.4④） |
| lint 体系 + turbo + CI + Docker | D | 与 A1 无依赖；但**建议 CI 提前到 A2 第一页之后**（见 §6） |
| `timezone` store | D | 132 行，无下游依赖 |

### 9.5 A2 现在可以开工了

A1 之后，`post` / `dept` / `client` / `menu` / `dict` / `logininfor` / `operlog` / `role-assign`
这 8 个页面所需的公共前置（API、抽屉、`updateSchema`、关闭前确认、控件注册表）已全部就位。
按计划先做 **`system/post`（Vue 451 行，最标准的表格 + 抽屉 CRUD）**，用它标定真实产能，
再据此重新基线化 §5.0 的人日估算。

---

## 10. 阶段 A2 实施记录（2026-11-15，进行中）

A2「纯 CRUD 页」按计划从 `system/post` 起步标定，随后铺开。**已完成 6 个契约页 + 1 个非契约页**，
后端菜单契约覆盖从 **1/14 → 10/14**。

### 10.1 交付项

| 页面 | Vue 行数 | React 行数 | 倍率 | 备注 |
|---|---|---|---|---|
| `system/post` | 451 | 504 | 1.12× | **样板页**，表格 + 抽屉 + 部门树联动 |
| `monitor/logininfor` | 320 | 449 | 1.40× | 只读列表 + 详情弹窗 + 浏览器/OS 图标 + 解锁 |
| `monitor/operlog` | 321 | 408 | 1.27× | 只读列表 + **远程排序** + 详情抽屉 + JSON 预览 |
| `system/dept` | 511 | 582 | 1.14× | **树形表格**（平铺转树 + 展开/折叠 + 双击展开） |
| `system/dict` | 717 | 977 | 1.36× | 三层嵌套（类型表格 → 数据弹窗 → 单条弹窗）；Vue 侧 999 行含 282 行死代码 `type/index-refactor.vue`，未迁 |
| `system/client` | 403 | 570 | 1.41× | `ApiSwitch` 状态开关 + `SecretInput` + `checkMethod` 禁选内置记录 |
| `system/role-assign` | 238 | 304 | 1.28× | 非契约页，但**它让 `system/role/authUser` 路由自动生效**（见 §9.3⑥） |
| **合计** | **2,961** | **3,794** | **1.28×** | |

配套新增的公共件：

| 文件 | 行数 | 说明 |
|---|---|---|
| `views/system/user/dept-tree.tsx` | 168 | 部门树侧栏，post / user 共用（Vue 侧 174 行） |
| `components/global/api-switch.tsx` | 115 | 带接口调用的状态开关，失败自动回滚 |
| `components/global/secret-input.tsx` | 62 | 密钥输入框 + 随机生成 |
| `utils/modal.tsx` | 119 | `confirmDeleteModal`：需手动输入「确认删除」的不可逆操作确认 |
| `adapter/form.ts` | 44 | 对应 Vue 侧同名文件；导出 `FormSchemaGetter` + 注册 i18n 校验文案 |

适配层能力补齐（这些都是原计划**漏估**的 A2 硬前置）：

| 能力 | 落点 | 服务于 |
|---|---|---|
| `FormApi.updateSchema` | `form-ui/src/form-api.ts` | 全部 8 个抽屉/弹窗页（注入部门树、字典等异步选项） |
| `FormProps.handleReset` | `form-ui/src/types.ts` + `adapter/grid.tsx` | post / dict（重置时一并清空部门树选中） |
| `FormProps.layout: 'vertical'` | `form-ui/src/form.tsx` | client / dict 的弹窗表单 |
| `FormSchema.help` | `form-ui/src/form.tsx` | dept（手机号/邮箱）、client（超时说明）、dict（回显样式） |
| `FormCommonConfig.formItemClass` | `form-ui/src/form.tsx` | 所有 `grid-cols-2` 的抽屉表单 |
| 必填文案 i18n（`ui.formRules.*`） | `form-ui/src/config.ts` + `adapter/form.ts` | 全局；此前是硬编码中文，**属既有 1:1 缺口，本轮修掉** |
| `treeConfig` 树形表格 | `adapter/grid.tsx` | dept / menu |
| `sortConfig.remote` + `addSortParams` | `adapter/grid.tsx` | operlog |
| `gridEvents.cellClick / cellDblclick` | `adapter/grid.tsx` | dict（点行开数据面板）/ dept（双击展开） |
| `checkboxConfig.checkMethod` | `adapter/grid.tsx` | client（内置记录禁选） |
| `GridColumn.formatter` / `sortable` | `adapter/grid.tsx` | operlog（耗时列、可排序列） |
| `gridApi.clearCheckboxRow` / `getData` / `setAllTreeExpand` / `setTreeExpand` | `adapter/grid.tsx` | logininfor（解锁后清选）/ dept（展开折叠） |
| `GridOptions.minHeight` / `rowClassName` / `tableTitleHelp` | `adapter/grid.tsx` | dict（弹窗内表格）/ dept |
| `renderOsIcon` / `renderBrowserIcon` / `renderHttpMethodTag` / `renderJsonPreview` / `renderTags` / `renderDictTags` | `utils/render.tsx` | logininfor / operlog / client |
| `useAccess()` hook | `components/access/Access.tsx` | client（开关是 disabled 而非隐藏） |
| `tagSelectOptions` | `components/dict/index.tsx` | dict（回显样式下拉） |

### 10.2 验证结果（全部实测）

| 门禁 | A1 后 | A2 当前 |
|---|---|---|
| `pnpm typecheck` | 16 project 零错误 | **16 project 零错误** |
| `pnpm test:unit` | 34 文件 / 415 用例 | **34 文件 / 415 用例** |
| `pnpm build` | dist.zip 2.55 MB | **成功，dist.zip 2.60 MB**；`app-views` gzip 3.5 → **12.55 kB**，`app-core` 14.61 → **18.58 kB** |
| 像素门禁 5 项 | 全过 | **全过，且数值与 A1 后逐项完全一致**（config 5.215% / login-light 0.277% / login-dark 0.021% / shell 9.344% / shell-dark 4.730%）——新页面走 `React.lazy` 懒加载，不进 golden 目标的渲染路径，**证明没有回归** |
| antd 弃用告警 | 2 类 | **0**（修掉了 `Modal destroyOnClose` 与 `notification.message`，见 §10.3⑦） |

代码量：React 侧 17,907 → **22,907 行**（占 Vue 75,776 行的 **30.2%**）。

### 10.3 踩坑与对计划的修正

**① 转换率不是 1:1，是 1.28×。** §5.0 用 config 页单点标定出「400 → 403，约 1:1」，
7 个页面的实测把它推翻：**Vue 2,961 行 → React 3,794 行（1.28×）**。
原因是 SFC 的 `<template>` 比 TSX 紧凑，而 React 侧要多写 `forwardRef` +
`useImperativeHandle` 这套命令式打开的样板（Vue 的 `connectedComponent` + `defineExpose` 更省）。
**A2/A3 的行数估算应统一乘 1.28**，即剩余页面（menu 441 + role 551 + notice 463 + user 1,144 = 2,599 Vue 行）
对应约 **3,330 React 行**，而非原估的 2,600。

**② 原计划把 5 项适配层能力漏估了。** `updateSchema`、`handleReset`、`layout: 'vertical'`、
`FormSchema.help`、树形表格 —— 这 5 项在 §5.1 的 A1 清单里**一个都没列**，但 8 个页面里有 8 个用到
`updateSchema`、2 个用到树形表格。这是 §5.0「只有一个实测锚点」的直接后果（R1 命中）。
好消息是它们都已补齐，**A3 的 3 个页面不会再撞到同类缺口**（已核对 role/notice/user 的依赖面）。

**③ `dict-data.ts` 在 A1 被漏迁了 6/7 个函数。** A1 的迁移脚本用「目标已存在则跳过」的策略，
而 React 侧原本有个只含 `dictDataInfo` 的 12 行残缺版本，于是整份 Vue 的 7 函数版本被跳过。
本轮补齐到 7/7。**教训：批量机械迁移后必须做「导出面逐个比对」，不能只看文件是否存在。**
已复核其余 39 个 api 文件，无同类问题。

**④ `system/dict/data` 的解析优先级得到验证。** §9.3⑦ 的推断（`data.tsx` 优先于 `data/index.tsx`）
在本轮落地后仍然成立：`data/` 目录已建（含 `data.tsx` / 两个弹窗），但契约 component
`system/dict/data` 依旧命中同级的占位文件 `data.tsx`，与 Vue 侧行为一致。

**⑤ React 侧 `views/**/*.tsx` 的 glob 会过度收集。** `data.tsx`、`*-modal.tsx`、`*-drawer.tsx`
都会进 pageMap。Vue 侧 glob 只匹配 `.vue`，天然没这个问题。
门禁因此**不能**写成「views 下所有文件都要有 default export」（会误报 6 个子组件），
改为只校验 14 个契约 component 对应的文件 —— 这才是真实风险面。

**⑥ `isUpdate` 这类"设置后立即读取"的状态必须用 ref。** dept / client 的抽屉在
`onOpenChange` 里 `setIsUpdate(update)` 之后**同一个异步函数内**就要读它来决定拉全量树还是子树，
React 的 state 更新是异步的，闭包里读到的还是旧值。Vue 的 `ref.value = x` 是同步的，所以那边没这个坑。
统一改法：`useState` 负责渲染 + `useRef` 负责同步读取。post / dict-data-item 同理。

**⑦ antd 6 的两处弃用告警是既有的，不是本轮引入。** 跑像素门禁时 console 打出
`[antd: Modal] destroyOnClose is deprecated` 与 `[antd: Notification] message is deprecated`，
分别来自阶段 ⑤ 的 `popup-ui/modal/modal.tsx` 和 `store/auth.ts`。已改为
`destroyOnHidden` / `title`，告警清零。**注意 state 字段名仍保留 `destroyOnClose`** 以与 Vue 侧
`ModalState`/`DrawerState` 对齐，只在渲染层映射到新 prop。

**⑧ `form-ui` 必须保持无 antd 依赖。** 加 `FormSchema.help` 时本想用 antd `Tooltip`，
但 `form-ui/package.json` 的 dependencies 只有 `@vben-core/shared` / `clsx` / `dayjs` / `tailwind-merge`
—— 它是刻意的 UI 库无关层（`popup-ui` 已经为了 Modal/Drawer 破例引了 antd，不该再扩散）。
改为原生 `title` 属性 + `(?)` 标记。**视觉与 Vue 的问号图标不等价，列入 B 阶段像素补齐。**

### 10.4 A2 剩余

| 页面 | Vue 行数 | 阻塞项 |
|---|---|---|
| `system/menu` | 441 | ✅ **已完成**（见 §10.5） |

A2 已 100% 完成（7 个契约页 + role-assign）。

---

### 10.5 A3 进展与三项范围收缩（2026-11-15 同日）

A2 收尾后继续推进 A3，**`system/user` 已完成**（契约覆盖 10/14 → **12/14 = 86%**）。
过程中用「实际引用面反查」又砍掉三块工作量，并补齐了 3 项 form-ui 能力。

#### 交付项

| 项 | Vue 行数 | React 行数 | 说明 |
|---|---|---|---|
| `system/menu`（data + drawer + index） | 841 | ~900 | 树形表格（`transform: false`，页面侧组树）+ 级联删除开关 + `admin/superadmin` 角色门禁 |
| `system/user`（data + drawer + 3 弹窗 + index） | 1,144 | ~1,400 | 头像列 / 状态开关 / 「更多」下拉 / 导入弹窗 / 重置密码 / 用户详情（dayjs duration 人性化时间） |
| `views/system/role/data.tsx` | — | 18 | 仅 `authScopeOptions`：user-drawer 跨模块引用它（Vue 侧同样如此） |
| `components/global/api-switch.tsx` | 105 | 115 | 带接口调用的状态开关，失败回滚 |

适配层新增能力（同样属原计划漏估）：

| 能力 | 落点 | 服务于 |
|---|---|---|
| `dependencies.rules`（动态校验规则） | `form-ui/src/{types,form-api,form}.tsx` | menu 的 `path`/`component`（规则随 `isFrame` 变化）、user 的 `roleIds`（新增可空、编辑必填） |
| `renderComponentContent`（具名插槽 + `default` 替换控件） | `form-ui/src/{types,form}.tsx` | menu 的图标 addonBefore/addonAfter、remark 说明文字；后续 role 的 `TreeSelectPanel` 也走这条 |
| `GridColumn.minWidth` | `adapter/grid.tsx` | user 列宽（⚠️ 仅记录，antd ColumnType 无此属性，不透传） |
| `ModalState.closeOnClickModal` | `popup-ui/modal` | 导入 / 重置密码弹窗防误关 |
| `Access` 的 `code` + `role` **AND 语义** | `components/access/Access.tsx` | menu 的增删改按钮（超管角色 **且** 对应权限） |

> ⚠️ 最后一条是**修正既有 bug**：原实现是 `if (role) ... else if (code) ...`，
> 两个条件同时给出时只校验 role、静默忽略 code。Vue 侧是两个独立指令、都要通过。

#### 三项范围收缩（继 §1.4 的 D1 之后）

用「页面 schema 里实际出现的 `component:` 名」反查 31 个注册控件，结果：

```
实际被使用的只有 10 个：Input(77) Select(21) RadioGroup(15) Textarea(8)
  InputNumber(7) RangePicker(5) InputPassword(5) TreeSelect(4) Radio(2) RichTextarea(1)
零使用的有 21 个，其中三个连带整个组件库文件都是死的：
```

| 组件 | Vue 行数 | 结论 |
|---|---|---|
| `common-ui/icon-picker` | 380 | **零页面使用**。menu 的图标字段是 `Input` + `addonBefore: <VbenIcon>` + 「搜索图标」外链，不是 IconPicker。仅在 `adapter/component` 里注册过 |
| `common-ui/api-component` | 332 | **零页面使用**。`ApiSelect`/`ApiTreeSelect`/`ApiCascader` 三个控件名在所有 schema 里出现 0 次 |
| `components/upload` | 885 | **零页面使用**。`FileUpload`/`ImageUpload`/`Upload` 三个控件名出现 0 次；user 页头像是纯展示 `<Avatar src>`，导入弹窗用的是 antd 原生 `Upload.Dragger` |
| | **合计 1,597 行** | 加上 §1.4 的 4,800 行，**累计砍掉 ~6,400 行 Vue 源码的迁移工作** |

**A3 的剩余阻塞项因此只剩两个：**

| 页面 | 真实阻塞 | 备注 |
|---|---|---|
| `system/role` | `#/components/tree`（923 行：`MenuSelectTable` 406 + `TreeSelectPanel` 192 + helper/hook/data） | 菜单权限树 + 部门权限树，role 页的核心 |
| `system/notice` | `#/components/tiptap`（1,526 行，toolbar 539 + resizable-image 240 + upload 196 …） | 需引入 `@tiptap/react` 及扩展包 |

关于 **D4（富文本编辑器）**，实测后有了明确答案：

- `notice-modal.vue` 用的是 **Tiptap**（`import { Tiptap } from '#/components/tiptap'`）
- 它对 tinymce 的依赖**只有一个 14 行的 helper**：`contentWithOssIdTransform`，
  而该函数当前是个**空实现**（`if (!content) return null; return content;`）
- `data.ts` 里的 `component: 'RichTextarea'` 走 tinymce 的 `Tinymce` 组件（268 行 editor.vue）

→ **建议 D4 定为「只迁 tiptap」**：tinymce 的 268 行编辑器不迁，
`contentWithOssIdTransform` 照抄那 5 行空实现即可，`RichTextarea` 控件改指向 tiptap。
省 3–5 人日，且不影响任何现有数据。

#### 验证结果

| 门禁 | A2 后 | A3（user/menu）后 |
|---|---|---|
| `pnpm typecheck` | 16 project 零错误 | **16 project 零错误** |
| `pnpm test:unit` | 34 文件 / 415 用例 | **34 文件 / 415 用例** |
| `pnpm build` | dist.zip 2.60 MB | **成功，dist.zip 2.62 MB**；`app-views` gzip 12.55 → **19.39 kB** |
| 像素门禁 5 项 | 全过 | **全过，数值第三次逐项完全一致**（5.215 / 0.277 / 0.021 / 9.344 / 4.730%） |
| console 告警 | 0 | **0** |

#### 踩坑补充

**⑨ antd 6 `Modal` 的 `maskClosable` 也已弃用**，与 Drawer 一样要改用
`mask={{ closable }}`。`ModalState` 的字段名保留 `closeOnClickModal`
（与 Vue 侧 `ModalProps` 对齐），只在渲染层映射。

**⑩ `User.id` 的类型标注是 `string`，但后端 seed 实际下发数字。**
Vue 侧 `row?.id !== 1` 因为 vxe 的 `row` 是 `any` 而没暴露问题；React 侧类型收紧后
TS 直接报「string 与 number 无重叠」。改为 `Number(row.id) !== SUPERADMIN_USER_ID`。
**两端共有的类型谎言，建议顺手回修 Vue 侧的 `User.id` 声明。**

**⑪ `listToTree<T>` 与 `treeToList<T>` 的泛型口径不一致**（沿袭 Vue）：
前者 `T` 是**节点**类型、返回 `T[]`；后者 `T` 是**数组**类型、返回 `T`。
写错不报错但类型是错的（`listToTree<Dept[]>` 得到 `Dept[][]`）。已在 dept-drawer 里加注释。

**⑫ 块注释里不能出现 `**/*.vue` 这种 glob**：`*/` 会提前终止注释，
导致后面整段变成语法错误。本轮门禁测试就踩了一次（TS1109/TS1128 一片）。

### 10.6 手工验证记录（真实后端，2026-11-15）

首次用**真实后端**（Docker 中间件 + Go 后端 + 本地 MySQL/Redis）端到端走了一遍，
不是 mock。登录（验证码 + 加解密链路）→ 12 个已落地页面逐个打开 → post 编辑抽屉，全部正常。

⚠️ **手工验证时最容易踩的坑：路由路径 ≠ 组件路径。**
后端 `sys_menu.component`（如 `monitor/operlog/index`）是**页面文件路径**，
而浏览器地址栏里的**路由路径**由菜单树的 `path` 字段拼出来，两者不是一回事：

| 页面 | 组件路径（`component`） | 路由路径（地址栏） |
|---|---|---|
| 操作日志 | `monitor/operlog/index` | **`/system/log/operlog`** |
| 登录日志 | `monitor/logininfor/index` | **`/system/log/logininfor`** |
| 分配用户 | `system/role/authUser` | `/system/role-auth/user/:roleId` |
| 分配角色 | `system/user/authRole` | `/system/user-auth/role/:userId` |
| 字典数据 | `system/dict/data` | `/system/dict-data/index/:dictId` |

直接访问 `/monitor/operlog` 会**正确地**落到 404（它从来就不是一个路由），
排查时不要误判成「页面没迁好」。以侧边栏点进去的地址为准。

顺带验证到的正向信号：operlog 表格第一行就是验证过程本身产生的登录操作记录
（`auth/login · 新增 · ::1 · 成功 · 134 ms`），说明 Go 后端的操作日志管道
与迁移后的页面是端到端打通的。

### 10.7 外壳补齐 + 系统设置 + 个人中心（2026-11-15 晚）

用户实机试用后反馈「布局还有问题、系统设置和个人中心没复刻」。本轮交付：

| 项 | 内容 | 对应 Vue |
|---|---|---|
| **表格列默认居中** | `adapter/grid.tsx` 列 `align` 缺省改为 `'center'` | vxe 全局配置 `align: 'center'`。此前 React 用 antd 默认左对齐，**所有表格**与 Vue 肉眼可见地不一致 |
| **侧边栏底栏** | 左 = 折叠按钮（`«`/`»`），右 = 偏好设置固定入口 | `layout.vue` 的 extra 区。⑥-a 把折叠放在顶栏，本轮按 vben 形态补齐底栏 |
| **顶栏补齐** | 刷新按钮（内容区软刷新，bump `key` 重挂载）、齿轮（设置）、全屏切换 | `header.vue` 的 leftSlots/rightSlots。顺序对齐 Vue |
| **面包屑链接色** | 非末级渲染为主色可点文本 | Vue 面包屑可点项是 primary 色，此前 React 全灰，观感像「禁用」 |
| **头像下拉** | 增加「个人中心」入口 → `/profile` | `user-dropdown.vue` |
| **偏好设置抽屉（系统设置）** | `layouts/widgets/preferences.tsx`（~430 行）：通用 / 布局 / 主题 三分区，17 个开关 + 布局/页签风格/面包屑/导航/内容/页脚等选项 + 14 色主题色板 + 重置 | Vue 侧 **3,753 行**（drawer 577 + 24 block + 9 张 SVG 示意图）。本轮为**功能等价版**：全部接到真实 preferences store、改完即时生效；9 张 SVG 布局示意图改用 Select、shortcut-keys block 未迁 —— 均列入 B 阶段 |
| **个人中心** | `router/routes/modules/profile.ts`（前端静态路由）+ `views/_core/profile/`（index / profile-panel / setting-panel / 3 个 tab，~600 行） | Vue 侧 `views/_core/profile`（767 行）+ common-ui `Profile*Setting`。头像上传本轮用 antd `Upload` 直传（**无裁剪交互**，`CropperAvatar` 768 行属 D 类待迁）；「基本设置」页签保留 Vue 原有的演示性质（mock 角色、提交只提示不落库） |

**实测验证（真实后端，非 mock）：**

- 个人中心渲染真实数据：`核桃管理员 / admin / 15888888888 / admin@walnutseed.com / 研发部门+董事长 / 上次登录时间`
- 设置抽屉联动真实生效：点 violet 色板后 `--primary` 从 `215 100% 54%` → `245 82% 67%`；点「重置」后恢复默认（偏好会持久化到 localStorage，验证完已重置，不污染会话）
- 顶栏新增按钮、侧边栏底栏均在 DOM 中确认渲染

**像素门禁复测（诚实记录：本轮 chrome 补齐没有改善 shell parity）：**

| 目标 | 上轮 | 本轮 | 解读 |
|---|---|---|---|
| `config` | 5.215% | **5.207%** | 表格居中带来微小改善 |
| `shell` | 9.344% | **9.355%** | 基本持平 |
| `shell-dark` | 4.730% | **4.807%** | 略升（+0.077pp，约 700px） |

结论：shell 的残余差异**不在按钮数量**，而在 §16.6 列的页签弧形 chrome、菜单排版、logo 区。
门禁的 mock 菜单只有 2 个节点，也放大了「菜单排版」这一项的占比。
要真正压下 shell，得做 B2（自绘 menu-ui）/ B3（自绘 tabs-ui），不是继续加 widget。

代码量：React 侧 25,241 → **26,504 行**。`pnpm typecheck` 16 project 零错误、
`pnpm test:unit` 34 文件 / 415 用例全绿、`pnpm build` 成功（dist.zip 2.64 MB）。

### 10.8 顶栏深色 bug：本轮最重要的修复（用户实机发现）

用户截图圈出顶栏「很差」。实测确认：**浅色主题下顶栏渲染成 antd `Layout.Header`
的默认深蓝黑 `#001529`（rgb(0,21,41)），而设计变量 `--header` 明明是白色**。
该 bug 自 ⑥-a 起就存在，像素门禁的 `shell` 差异里那条深色横带占了近一半。

**根因（cascade 层级，不是颜色配错）：**

- 外壳写的是 Tailwind 的 `bg-header` / `bg-sidebar`；
- Tailwind v4 的 utilities 编译进 `@layer utilities`；
- antd 的 CSS-in-JS 注入的样式**不在任何 cascade layer**；
- 按 CSS 规则，**无层样式优先级高于有层样式** → antd 的 `headerBg: #001529` 永远赢。

侧边栏此前「看起来正常」只是碰巧：`Sider` 上写了 `theme="light"`，antd 内部把
`siderBg` 换成了白色；而 `Header` 没有等价的 theme 开关。这个 `theme="light"`
同时是**另一个隐藏 bug**：暗色主题下它会强制侧边栏白底。

**修法（走 antd 自己的 token 通道，而不是和它抢 cascade）：**

1. `packages/hooks/src/use-design-tokens.ts` 新增 `useAntdComponentTokens()`，
   把 `Layout.headerBg / siderBg / bodyBg / triggerBg` 与 `Menu.itemBg` 等
   组件级 token 接回 `--header` / `--sidebar` / `--background-deep` 等 CSS 变量
   （与全局 token 同一套「读 CSS 变量 → 转 rgb」逻辑，单一事实来源不变）。
2. `App.tsx` 把 `components` 并入 `ConfigProvider theme`。
3. `BasicLayout` 移除 `Sider` 的 `theme="light"`，暗色下侧边栏跟随变量。

**实测（浏览器 computed style，浅 → 暗 → 浅往返）：**

| 主题 | header | sider | body |
|---|---|---|---|
| light | rgb(255,255,255) | rgb(255,255,255) | rgb(241,243,246) |
| dark | rgb(28,30,35) | rgb(28,30,35) | rgb(20,22,26) |
| 切回 light | rgb(255,255,255) | rgb(255,255,255) | rgb(241,243,246) |

**门禁收益：** `shell` 9.355% → **4.753%**，`shell-dark` 4.807% → **4.464%**，
阈值相应收紧到 5% / 4.6%（`e2e/visual/specs/compare.spec.ts`）。

> ⚠️ **同类风险排查结论**：凡「Tailwind 背景/边框 class 直接打在 antd 组件根节点上」
> 的写法都打不过 antd 默认样式。现存此类写法（如 grid 容器的 `bg-card`）打在**普通 div**
> 上，不受影响；后续若再往 antd 组件根节点挂背景 class，一律改走组件 token 或 inline style。

### 10.9 顶栏按钮对齐 + 偏好设置抽屉 v2（用户第二轮实机反馈）

用户截图圈出顶栏左侧两个按钮「没复刻成功」，并指出设置抽屉不像 Vue。逐条对齐：

**① 顶栏左侧按钮。** Vue 侧是 `VbenIconButton`（无边框图标按钮）+ Iconify
`ep:fold` / `ep:expand`；此前 React 用 antd `Button type="text"` + lucide 箭头，
**字形和按钮盒子都不对**（antd text Button 有焦点/悬停盒子）。已改为
`VbenIconButton` + 内联 SVG。

> ⚠️ 内联而非走 `VbenIcon` 的原因：Vue 的 `ep:fold` 来自**离线图标包**
> （`generate-offline-icons.js` 产物），React 的 `VbenIcon` 走 `@iconify/react`
> **在线请求**。像素门禁的无头环境里在线请求赶不上截图时机，折叠按钮渲染成空白
> （实测复现）。SVG path 取自 `api.iconify.design/ep/{fold,expand}.svg`，
> 落在 `layouts/widgets/sidebar-toggle-icons.tsx`，零网络依赖。

**② 偏好设置抽屉重写为 Vue 同构结构。** 此前是「Segmented 三分区」，与 Vue 的
**单列滚动 Block 列表**不符。v2 对齐 `preferences-drawer.vue`：

- 17 个 Block，顺序与 Vue 一致：通用 → 动画 → 主题 → 内置主题 → 圆角 → 字体大小 →
  其它 → 布局 → 内容 → 侧边栏 → 顶栏 → 导航菜单 → 标签栏 → 面包屑 → 底栏 → 版权 → 小部件
- 行 markup 对齐 `switch-item.vue` / `toggle-item.vue`：
  `my-1 flex justify-between rounded-md px-2 py-2.5 hover:bg-accent` 整行可点 +
  `CircleHelp` 提示；ToggleGroup 选中项 `data-[state=on]:bg-primary`
- 布局预设：7 张预览卡 + `outline-box` / `outline-box-active`（class 在
  `internal/tailwind-config/src/theme.css` 已有，与 Vue 同源）；预览图为**简化 SVG**
  （色块语义一致：primary = 当前导航区），Vue 的精细版列入 B 阶段
- 抽屉底部：重置偏好设置 / 复制偏好设置

**③ 布局预设整块禁用并标注。** React 外壳目前只实现 `sidebar-nav` 一种形态，
其余 6 种属 B5；放开会让开关「点了没反应」，比禁用更糟。B5 落地后删掉包裹层即可。

**实测（真实后端会话）：** 抽屉内关闭「标签栏」开关 → 页签栏立即消失；再开 → 恢复。
证明偏好是真实接到 store 的，不是摆设。

**门禁：** `shell` 4.757% / `shell-dark` 4.469%（阈值 5% / 4.6% 内），其余目标不变。

**header 右侧仍缺的两块红区**（diff 图可见）：全局搜索框（`搜索 Ctrl K`）与通知铃。
二者分别是 `widgets/global-search`（含最近访问 + 菜单模糊搜索）与
`widgets/notification`（SSE 消息列表），属 B1，未在本轮范围。

### 10.10 设置抽屉 v3（对着活 Vue 端逐页签核对）+ 黑屏兜底

用户反馈「点设置黑屏」+「设置没有 1:1」。处理：

**① 黑屏兜底：新增全局 `ErrorBoundary`**（`components/error-boundary.tsx`，包住
`RouterProvider`）。React 19 下未捕获渲染异常会卸载整棵根树 → 空白/黑屏且无任何线索；
现在会显示「页面渲染出错 + 错误 message + 重新加载按钮」。
**在本机真实会话里未能复现崩溃**（抽屉正常打开），最可能是用户浏览器持有 HMR
中途的旧 bundle；硬刷新可解。若再出现，边界会把真实错误暴露出来便于定位。

**② 设置抽屉 v3：纠正 v2 的结构错误。** v2 做成「单列滚动 Block 列表」是错的 ——
对着 **8010 上活着的 Vue 端**打开抽屉逐页签 dump 后确认 Vue 的真实结构是：

- `Drawer`（`sm:max-w-sm` ≈384px）+ title + description 副标题
- `extra`：重置图标按钮（有改动时带主色小圆点）+ 钉住图标按钮（页签条吸顶开关）
- **`VbenSegmented` 四页签，顺序 外观 → 布局 → 快捷键 → 通用，默认 `appearance`**
- 页签 → Block 映射（与 Vue slot 一一对应，已实测逐页签比对一致）：
  - 外观：主题 / 内置主题 / 圆角 / 按钮水波纹样式 / 字体大小 / 其它
  - 布局：布局 / 内容 / 侧边栏 / 顶栏 / 导航菜单 / 面包屑 / 标签栏 / 小部件 / 底栏 / 版权
  - 快捷键：全局（enable + Ctrl K / Alt Q / Alt L / Esc，行内 kbd 展示，未启用时置灰）
  - 通用：通用（语言/时区/动态标题/检查更新/水印/水印内容/复制偏好）+ 动画
- `footer`：复制偏好设置（Copy 图标）+ 清空缓存 & 退出登录，仅在有改动时可用

v1（Segmented 三分区）与 v2（单列）均已废弃；v3 为当前实现。

**仍保留的差异（B 阶段）：** 布局预览 7 张图为简化 SVG 且整块禁用（B5 多布局未落地）；
「有改动」判定用会话加载快照近似 Vue 的与默认值 diff。

**门禁：** 5 项全过，数值不变（shell 4.757% / shell-dark 4.469%）。
typecheck 16 project 零错误、415 用例、build 成功（dist.zip 2.64 MB）。

### 10.11 黑屏真根因：遮罩透明度在颜色转换里被丢掉（已修）

用户第二次反馈黑屏并附截图：**抽屉本身渲染正常，黑的是抽屉后面整个应用** ——
即 antd Drawer 的**遮罩变成了不透明纯黑**。

**根因链：**

1. 设计令牌 `--overlay` 的写法是 CSS Color 4 空格语法带斜杠透明度：
   `--overlay: 0 0% 0% / 45%`（暗色 `0deg 0% 0% / 40%`）
2. `useAntdDesignTokens` 用 `convertToRgb('hsl(' + triplet + ')')` 喂 antd `colorBgMask`
3. `convertToRgb` 内部是 TinyColor，**解析不了空格+斜杠语法**，静默退化为
   **不透明** `rgb(0, 0, 0)`
4. antd 遮罩 = 纯黑不透明 → 打开任何 Drawer/Modal 都把应用盖黑

既有单测只覆盖了逗号语法 `hsla(284, 100%, 50%, 0.92)`，**没覆盖空格+斜杠语法**，
所以一直没暴露。Vue 侧直接把 CSS 变量字符串交给组件库（浏览器原生解析），无此问题。

**修复：** `convertToRgb` 先把空格语法（含可选 `/ alpha%`）归一化成
`hsla(H, S%, L%, A)` 逗号语法再交 TinyColor；新增 2 条回归用例
（`hsl(0 0% 0% / 45%) → rgba(0,0,0,0.45)`、`deg` 变体、与逗号语法等价性）。

**实测：** 浅色遮罩 `rgba(0,0,0,0.45)`、暗色 `rgba(0,0,0,0.4)`，
暗色下侧边栏 `rgb(28,30,35)` 正常可见 —— 应用不再被盖黑。
单测 34 文件 / **417 用例**全绿；build 成功；像素门禁 5 项全过。

> 教训：**「颜色转换函数丢 alpha」是一类静默故障** —— 不报错、不崩溃，只是视觉全黑。
> 凡设计令牌带透明度的（overlay / 未来的半透明色），转换链路必须有回归用例兜底。

### 10.12 偏好消费端接线（用户反馈「设置里的功能都用不了」）

用户指出设置抽屉的开关大多没反应。核查确认：**抽屉把偏好写进了 store，但消费端
大多没迁** —— Page / menu 零消费，tabs 只消费 persist/showIcon/height。本轮把消费端补上：

| 偏好 | 消费端落点 | 实测 |
|---|---|---|
| `app.watermark` / `watermarkContent` | BasicLayout 用 antd `Watermark` 包内容区 | ✅ 水印图层出现 |
| `app.colorGrayMode` / `colorWeakMode` | `hooks/use-preference-effects.ts` 写 html filter | ✅ `grayscale(100%)` |
| `app.dynamicTitle` | 同上，路由变化更新 document.title | ✅ |
| `shortcutKeys.globalLogout` / `globalEscape` | 同上，Alt+Q 退出 / Esc 退最大化 | ✅ |
| `app.contentCompact` / `contentCompactWidth` | `Page` 限宽居中 | ✅ |
| `breadcrumb.showHome` / `showIcon` / `hideOnlyOne` | BasicLayout 面包屑（首页项为纯图标 `mdi:home-outline`，同 Vue） | ✅ |
| `navigation.accordion` / `styleType` | BasicLayout 手风琴 openKeys + `vben-menu-rounded` class | ✅ |
| `theme.semiDarkSidebar` / `semiDarkHeader` | 局部 `ConfigProvider` 暗色算法包 Sider / Header | ✅ |
| `tabbar.styleType` / `wheelable` / `draggable` / `maxCount` / `showRefresh` / `showMore` / `showMaximize` | `adapter/tabs.tsx`：风格 class、滚轮横滚、HTML5 拖拽（store 已有 `sortTabs`）、超上限关最旧、右侧刷新/更多/最大化按钮 | ✅ 最大化隐藏 sider/header |
| `footer.enable` / `fixed` / `copyright.*` | BasicLayout 底栏 + 版权 | ✅ 「2026 WalnutSeed」 |
| `transition.enable` / `name` | 内容区 `page-transition--{name}` CSS 动画（fade/fade-slide/fade-up/fade-down） | ✅ |
| `theme.buttonWaveMode = Disabled` | `ConfigProvider wave.disabled` | ✅ |
| 刷新（顶栏 + 页签） | 新增 `store/ui.ts` 重挂载计数（Vue 靠 Activity 缓存，B4 未落地前的等价实现） | ✅ |

**仍未接（依赖未迁组件，接了也是空操作，已在代码注释标明）：**
`widget.globalSearch`(Ctrl K) / `notification` / `lockScreen` / `timezone` /
`app.enableCheckUpdates` / `sidebar.autoActivateChild` / `expandOnHover` /
`header.menuAlign`（仅 header-nav 布局有意义）/ `shortcutKeys.globalSearch` / `globalLockScreen`。
这些属 B1/B5，随对应组件落地再接。

**⚠️ 本轮踩到一个回归并立即修复：** 曾把 `theme.fontSize` 喂给 antd 的 `fontSize`
token，导致 antd 组件整体变大，像素门禁 login 差异 **0.277% → 1.325%（超阈值失败）**。
Vue 侧 `useAntdDesignTokens` 只喂 borderRadius + 颜色，字号偏好只作用于自绘组件
（`--font-size-base` / `--menu-font-size`）。已撤掉该 token，parity 恢复
（login 0.277% / 0.021%，config 5.207%，shell 4.764% / 4.452%，5 项全过）。
**教训：给 antd 加全局 token 前先核对 Vue 侧喂了哪些，像素门禁会立刻暴露偏差。**

验证：typecheck 16 project 零错误、34 文件 / 417 用例、build 成功、门禁 5 项全过。

---

## 11. 复核命令

本文所有数字可用以下命令复现（在仓库根目录）：

```bash
# 质量基线
cd walnut-frontend-react && pnpm install && pnpm test:unit && pnpm typecheck

# 源码行数（Vue）
cd walnut-frontend && find packages apps/web-antd/src -type f \
  \( -name '*.vue' -o -name '*.ts' -o -name '*.tsx' -o -name '*.css' -o -name '*.scss' \) \
  -not -path '*/node_modules/*' -not -path '*/dist/*' -not -path '*/__tests__/*' \
  -not -name '*.test.*' -not -name '*.spec.*' -exec cat {} + | wc -l     # → 75,776

# 源码行数（React）
cd walnut-frontend-react && find packages apps/web-react/src -type f \
  \( -name '*.tsx' -o -name '*.ts' -o -name '*.css' \) \
  -not -path '*/node_modules/*' -not -path '*/dist/*' -not -path '*/__tests__/*' \
  -not -name '*.test.*' -not -name '*.spec.*' -exec cat {} + | wc -l     # → 15,508

# 业务页覆盖率（后端下发的 component 清单）
grep -o '"component": *"[^"]*"' \
  walnut-frontend-react/apps/web-react/src/__fixtures__/backend-menu-tree.json | sort -u

# 表单控件注册表
grep -cE "^  [A-Z][A-Za-z]*:"  walnut-frontend/apps/web-antd/src/adapter/component/index.ts   # → 31
grep -cE "^    [A-Z][A-Za-z]*[:,]" walnut-frontend-react/apps/web-react/src/adapter/component.tsx  # → 9

# 富组件依赖关系（决定 A3 的拆分）
cd walnut-frontend/apps/web-antd/src/views && \
  grep -rn "from '#/components/\(upload\|cropper\|tiptap\|tinymce\|tree\|dict\)" .

# common-ui 实际导入面（决定 §1.4 的裁剪清单）
cd walnut-frontend && grep -rhoz "import[^;]*from '@vben/common-ui'" \
  apps/web-antd/src packages/effects/layouts --include='*.vue' --include='*.ts' --include='*.tsx' \
  | tr '\0' '\n' | grep -o "{[^}]*}" | tr -d '{}' | tr ',' '\n' | sort -u

# 转换率锚点（config 页）
wc -l walnut-frontend/apps/web-antd/src/views/system/config/*      # → 400
wc -l walnut-frontend-react/apps/web-react/src/views/system/config/*  # → 403
```

---

## 相关文档

- [05-React迁移-逐文件清单](./05-React迁移-逐文件清单.md) —— 1,035 个源文件的逐文件分类（A/B/C/D/E）与阶段 ①–⑥-a 实施记录
- [接口契约](./01-接口契约.md) · [权限与动态路由](./02-权限与动态路由.md) · [接口加解密](./03-接口加解密.md) · [国际化](./04-国际化.md)
- [walnut-frontend-react/README.md](../../walnut-frontend-react/README.md) —— React 端技术栈与踩坑记录


## 12. 2026-11-19 外壳样式复核与修复

本轮基于 `96f77f6` 核对源码并在 Windows / 同机 Chromium 上运行两套前端。
迁移范围仍为 **12/14 个后端菜单 component、总体约 50%**：角色管理和通知公告尚未迁移，
本轮只修复公共外壳，不将视觉修复计为新增业务页。dashboard、Activity 缓存与其它 B/C/D 阶段工作仍待推进。

### 已修复的根因

| 问题 | 修复 |
| --- | --- |
| `header-mixed-nav` / `sidebar-mixed-nav` 的双列导航是普通 div，antd 未识别为 Sider，内容被排到下一行 | 显式设置外层 `Layout.hasSider`，使双列导航与内容保持横排 |
| 切换布局后旧 ResizeObserver 仍观察已卸载的内容节点，回到默认布局时 CSS 内容高度为 0 | 两套外壳共用 `ContentArea`，观察实际挂载的节点并在节点变化时清理、重新绑定 |
| 顶栏 Tailwind 间距被 antd 默认的 50px padding 覆盖 | 通过 Layout 组件 token 设置 12px padding；半暗 Header 使用局部 CSS 变量，避免 antd 6 的 Layout 变量作用域遗漏 |
| 半暗模式自绘文字仍取浅色变量，菜单继承了浅色 antd token | 局部 `.dark` 与嵌套 ConfigProvider 同时覆盖，统一两类组件的对比度 |
| `--radius` 已是 rem，样式又乘以 1rem，浏览器丢弃该声明 | 菜单与面包屑直接使用 `var(--radius)`；导航风格由菜单适配器统一消费 |
| 页签总高多 1px、chrome 风格只有矩形边框 | 总高包含底边框，补主色半透明选中背景及下缘弧形；保留现有 antd 页签的键盘与关闭行为 |
| 多布局忽略 hidden 偏好、最大化后没有恢复入口、全内容模式仍显示页签 | 统一隐藏判断，添加可访问的恢复按钮，全内容布局隐藏页签与页脚 |
| 多布局重复注册全局偏好副作用，混合导航丢弃子菜单后直接跳转父路径 | 副作用仅在 BasicLayout 注册；从原菜单树解析混合导航的首个叶子目标 |

### 验证结果

- `pnpm test:unit`：34 文件 / **417 用例通过**。
- `pnpm typecheck`、`pnpm build`：通过；未改依赖和后端契约。
- 新增 `e2e/visual/specs/layout.spec.ts` 五项浏览器回归：七种布局往返切换及刷新、设计令牌、六种外壳的亮/暗/半暗主题、带水印的最大化与恢复、隐藏偏好与混合导航。
- 同机双端截图与原有五项像素门禁通过，**没有放宽阈值**：

| 目标 | 本轮差异 | 原阈值 |
| --- | --- | --- |
| login-light | 0.277% | 0.3% |
| login-dark | 0.021% | 0.3% |
| config | 5.157% | 6.0% |
| shell | 4.160% | 5.0% |
| shell-dark | 4.223% | 4.6% |

config 的双端截图尺寸均为 **1024 × 600**。原门禁沿用 Playwright Desktop Chrome 的
1280 × 720 视口；新增布局检查显式使用 1440 × 900，避免 device preset 覆盖全局 viewport。
这些检查使用固定 API mock 验证前端展示与交互，不替代真实后端联调；表格和菜单仍有原生组件差异，
不能据此宣称整个 React 迁移或全站像素复刻已经完成。

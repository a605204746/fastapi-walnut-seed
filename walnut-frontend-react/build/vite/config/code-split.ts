const createChunkMatcher = (patterns: string[]) => {
  return (id: string) => {
    const normalizedId = id.replaceAll("\\", "/");
    return patterns.some((pattern) => normalizedId.includes(pattern));
  };
};

/**
 * 分包名单。
 *
 * 与 Vue 侧 internal/vite-config/src/config/code-split.ts 同构，
 * 但把 antdv-next / vue / vueuse / pinia 系列换成了 antd / react 系列，
 * 图表、富文本和头像裁剪独立按路由加载；不递归捕获共享依赖，避免登录页提前加载大包。
 */

// ---------- UI 库 ----------
const matchAntdIconsChunk = createChunkMatcher([
  "/node_modules/.pnpm/@ant-design+icons-svg@",
  "/node_modules/.pnpm/@ant-design+icons@",
  "/node_modules/@ant-design/icons-svg/",
  "/node_modules/@ant-design/icons/",
]);
const matchAntdThemeChunk = createChunkMatcher([
  "/node_modules/.pnpm/@ant-design+colors@",
  "/node_modules/.pnpm/@ant-design+cssinjs@",
  "/node_modules/.pnpm/@ant-design+fast-color@",
  "/node_modules/antd/es/config-provider/",
  "/node_modules/antd/es/locale/",
  "/node_modules/antd/es/theme/",
  "/node_modules/@ant-design/colors/",
  "/node_modules/@ant-design/cssinjs/",
  "/node_modules/@ant-design/fast-color/",
]);
const matchAntdFormChunk = createChunkMatcher([
  "/node_modules/antd/es/auto-complete/",
  "/node_modules/antd/es/cascader/",
  "/node_modules/antd/es/checkbox/",
  "/node_modules/antd/es/color-picker/",
  "/node_modules/antd/es/date-picker/",
  "/node_modules/antd/es/form/",
  "/node_modules/antd/es/input-number/",
  "/node_modules/antd/es/input/",
  "/node_modules/antd/es/mentions/",
  "/node_modules/antd/es/radio/",
  "/node_modules/antd/es/rate/",
  "/node_modules/antd/es/select/",
  "/node_modules/antd/es/slider/",
  "/node_modules/antd/es/switch/",
  "/node_modules/antd/es/time-picker/",
  "/node_modules/antd/es/transfer/",
  "/node_modules/antd/es/tree-select/",
  "/node_modules/antd/es/upload/",
  "/node_modules/rc-field-form/",
]);
const matchAntdOverlayChunk = createChunkMatcher([
  "/node_modules/antd/es/alert/",
  "/node_modules/antd/es/drawer/",
  "/node_modules/antd/es/dropdown/",
  "/node_modules/antd/es/modal/",
  "/node_modules/antd/es/popconfirm/",
  "/node_modules/antd/es/popover/",
  "/node_modules/antd/es/tooltip/",
  "/node_modules/rc-dialog/",
  "/node_modules/rc-drawer/",
  "/node_modules/rc-dropdown/",
  "/node_modules/rc-tooltip/",
  "/node_modules/rc-trigger/",
]);
const matchAntdDataChunk = createChunkMatcher([
  "/node_modules/antd/es/pagination/",
  "/node_modules/antd/es/table/",
  "/node_modules/antd/es/tree/",
  "/node_modules/rc-table/",
  "/node_modules/rc-tree/",
  "/node_modules/rc-virtual-list/",
]);
const matchAntdLayoutChunk = createChunkMatcher([
  "/node_modules/antd/es/breadcrumb/",
  "/node_modules/antd/es/grid/",
  "/node_modules/antd/es/layout/",
  "/node_modules/antd/es/menu/",
  "/node_modules/antd/es/space/",
  "/node_modules/antd/es/splitter/",
  "/node_modules/antd/es/tabs/",
  "/node_modules/rc-menu/",
  "/node_modules/rc-tabs/",
]);
const matchAntdChunk = createChunkMatcher([
  "/node_modules/.pnpm/antd@",
  "/node_modules/antd/",
]);
const matchProComponentsChunk = createChunkMatcher([
  "/node_modules/.pnpm/@ant-design+pro-",
  "/node_modules/@ant-design/pro-",
]);

// ---------- 框架 ----------
const matchFrameworkChunk = createChunkMatcher([
  "/node_modules/.pnpm/react-dom@",
  "/node_modules/.pnpm/react-router@",
  "/node_modules/.pnpm/react@",
  "/node_modules/.pnpm/scheduler@",
  "/node_modules/react-dom/",
  "/node_modules/react-router/",
  "/node_modules/react/",
  "/node_modules/scheduler/",
]);

// ---------- 工作区内核 ----------
const matchWalnutCoreChunk = createChunkMatcher([
  "/src/core/design/",
  "/src/core/shared/",
  "/src/core/typings/",
  "/src/core/constants/",
  "/src/core/types/",
  "/src/core/utils/",
]);
const matchWalnutRequestChunk = createChunkMatcher([
  "/src/core/request/",
]);
const matchWalnutLocaleChunk = createChunkMatcher([
  "/src/core/locales/",
]);

// ---------- 第三方 ----------
const matchUiVendorChunk = createChunkMatcher([
  "/node_modules/.pnpm/@floating-ui+",
  "/node_modules/.pnpm/@iconify+",
  "/node_modules/.pnpm/@radix-ui+",
  "/node_modules/.pnpm/framer-motion@",
  "/node_modules/.pnpm/lucide-react@",
  "/node_modules/@floating-ui/",
  "/node_modules/@iconify/",
  "/node_modules/@radix-ui/",
  "/node_modules/framer-motion/",
  "/node_modules/lucide-react/",
]);
const matchUtilsVendorChunk = createChunkMatcher([
  "/node_modules/.pnpm/@ctrl+tinycolor@",
  "/node_modules/.pnpm/@tanstack+",
  "/node_modules/.pnpm/axios@",
  "/node_modules/.pnpm/crypto-js@",
  "/node_modules/.pnpm/dayjs@",
  "/node_modules/.pnpm/es-toolkit@",
  "/node_modules/.pnpm/i18next@",
  "/node_modules/.pnpm/jsencrypt@",
  "/node_modules/.pnpm/mitt@",
  "/node_modules/.pnpm/nprogress@",
  "/node_modules/.pnpm/qs@",
  "/node_modules/.pnpm/sm-crypto@",
  "/node_modules/.pnpm/zustand@",
  "/node_modules/@ctrl/tinycolor/",
  "/node_modules/@tanstack/",
  "/node_modules/axios/",
  "/node_modules/crypto-js/",
  "/node_modules/dayjs/",
  "/node_modules/es-toolkit/",
  "/node_modules/i18next/",
  "/node_modules/jsencrypt/",
  "/node_modules/mitt/",
  "/node_modules/nprogress/",
  "/node_modules/qs/",
  "/node_modules/sm-crypto/",
  "/node_modules/zustand/",
]);

// ---------- 应用 ----------
const matchAppAuthChunk = createChunkMatcher([
  "/src/api/core/auth",
  "/src/api/core/captcha",
  "/src/views/_core/authentication/",
  "/src/views/_core/social-callback/",
]);
const matchAppLocaleChunk = createChunkMatcher([
  "/src/locales/",
]);
const matchAppCoreChunk = createChunkMatcher([
  "/src/adapter/",
  "/src/api/core/",
  "/src/components/global/",
  "/src/layouts/",
  "/src/lib/",
  "/src/main.tsx",
  "/src/router/",
  "/src/store/",
  "/src/utils/",
]);

function createApplicationCodeSplitting() {
  return {
    includeDependenciesRecursively: false,
    groups: [
      // framework 优先级最高：react / react-dom 是 antd 与业务代码的共同底座，
      // 若让 antd-* 组先匹配到，react-dom 会被并进 antd chunk（实测 336 kB）。
      { name: "framework", priority: 60, test: matchFrameworkChunk },
      {
        name: "charts",
        priority: 59,
        test: createChunkMatcher([
          "/node_modules/echarts/",
          "/node_modules/zrender/",
          "/components/chart/",
          "/views/dashboard/",
        ]),
      },
      {
        name: "rich-editor",
        priority: 58,
        test: createChunkMatcher([
          "/node_modules/@tiptap/",
          "/node_modules/prosemirror-",
          "/components/tiptap/",
        ]),
      },
      {
        name: "avatar-cropper",
        priority: 57,
        test: createChunkMatcher([
          "/node_modules/react-easy-crop/",
          "/components/cropper/",
        ]),
      },
      { name: "pro-components", priority: 50, test: matchProComponentsChunk },
      { name: "antd-icons", priority: 47, test: matchAntdIconsChunk },
      { name: "antd-theme", priority: 46, test: matchAntdThemeChunk },
      { name: "antd-form", priority: 45, test: matchAntdFormChunk },
      { name: "antd-overlay", priority: 44, test: matchAntdOverlayChunk },
      { name: "antd-data", priority: 43, test: matchAntdDataChunk },
      { name: "antd-layout", priority: 42, test: matchAntdLayoutChunk },
      { name: "antd", priority: 40, test: matchAntdChunk },
      { name: "walnut-core", priority: 14, test: matchWalnutCoreChunk },
      { name: "walnut-locale", priority: 13, test: matchWalnutLocaleChunk },
      { name: "walnut-request", priority: 12, test: matchWalnutRequestChunk },
      { name: "ui-vendor", priority: 10, test: matchUiVendorChunk },
      { name: "utils-vendor", priority: 5, test: matchUtilsVendorChunk },
      { name: "app-auth", priority: 4, test: matchAppAuthChunk },
      { name: "app-locales", priority: 3, test: matchAppLocaleChunk },
      { name: "app-core", priority: 2, test: matchAppCoreChunk },
    ],
  };
}

export { createApplicationCodeSplitting };

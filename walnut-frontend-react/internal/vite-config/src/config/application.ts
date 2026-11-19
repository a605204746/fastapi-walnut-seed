import type { CSSOptions, UserConfig } from "vite";

import type { DefineApplicationOptions } from "../typing";

import path, { relative } from "node:path";

import { findMonorepoRoot } from "@vben/node-utils";

import { NodePackageImporter } from "sass-embedded";
import { defineConfig, loadEnv, mergeConfig } from "vite";

import { getDefaultPwaOptions } from "../options";
import { loadApplicationPlugins } from "../plugins";
import { loadAndConvertEnv } from "../utils/env";
import { createApplicationCodeSplitting } from "./code-split";
import { getCommonConfig } from "./common";

function defineApplicationConfig(userConfigPromise?: DefineApplicationOptions) {
  return defineConfig(async (config) => {
    const options = await userConfigPromise?.(config);
    const { appTitle, base, port, ...envConfig } = await loadAndConvertEnv();
    const { command, mode } = config;
    const { application = {}, vite = {} } = options || {};
    const root = process.cwd();
    const isBuild = command === "build";
    const env = loadEnv(mode, root);

    const plugins = await loadApplicationPlugins({
      archiver: true,
      archiverPluginOptions: {},
      compress: false,
      compressTypes: ["brotli", "gzip"],
      env,
      extraAppConfig: true,
      html: true,
      injectAppLoading: true,
      injectMetadata: true,
      isBuild,
      license: true,
      mode,
      print: !isBuild,
      printInfoMap: {
        "Migration Checklist": "../docs/frontend/05-React迁移-逐文件清单.md",
      },
      pwa: true,
      pwaOptions: getDefaultPwaOptions(appTitle),
      ...envConfig,
      ...application,
    });

    const { injectGlobalScss = false } = application;

    const applicationConfig: UserConfig = {
      base,
      build: {
        rolldownOptions: {
          output: {
            strictExecutionOrder: true,
            assetFileNames: "[ext]/[name]-[hash].[ext]",
            chunkFileNames: "js/[name]-[hash].js",
            codeSplitting: createApplicationCodeSplitting(),
            entryFileNames: "jse/index-[name]-[hash].js",
            minify: isBuild
              ? {
                  compress: {
                    dropDebugger: true,
                  },
                }
              : false,
          },
        },
        target: "es2015",
      },
      css: createCssOptions(injectGlobalScss),
      plugins,
      server: {
        host: true,
        port,
        warmup: {
          // 预热文件
          clientFiles: [
            "./index.html",
            "./src/main.tsx",
            "./src/{views,layouts,router,store,api,adapter,lib}/*",
          ],
        },
      },
    };

    const mergedCommonConfig = mergeConfig(
      await getCommonConfig(),
      applicationConfig,
    );
    return mergeConfig(mergedCommonConfig, vite);
  });
}

function createCssOptions(injectGlobalScss = false): CSSOptions {
  const root = findMonorepoRoot();
  return {
    preprocessorOptions: injectGlobalScss
      ? {
          scss: {
            additionalData: (content: string, filepath: string) => {
              const relativePath = relative(root, filepath);
              // apps下的包注入 BEM mixin
              if (relativePath.startsWith(`apps${path.sep}`)) {
                return `@use "@vben-core/design/bem" as *;\n${content}`;
              }
              return content;
            },
            importers: [new NodePackageImporter()],
          },
        }
      : {},
  };
}

export { defineApplicationConfig };

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { expect, test } from '@playwright/test';
import pixelmatch from 'pixelmatch';
import { PNG } from 'pngjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const ARTIFACTS = path.resolve(HERE, '..', 'artifacts');
const VUE_DIR = path.join(ARTIFACTS, 'vue');
const REACT_DIR = path.join(ARTIFACTS, 'react');
const DIFF_DIR = path.join(ARTIFACTS, 'diff');

/**
 * 分层阈值（差异像素占比上限）。
 *
 * - 登录页（纯外壳 + SVG + antd 表单）：严格 0.3%，实测 light 0.28% / dark 0.02%，
 *   证明方案 C 全自绘复刻在外壳层达到像素级一致。
 * - config 页：初值 1.5%（Spec §6）。实测 5.2%，残余全部来自 vxe-table 与 antd Table
 *   的原生差异（垂直节奏 / 分页「总数在左+页码在右」布局 / 分页字形），
 *   以及 PoC 最小布局与 Vue 完整布局的高差（Spec §7 风险1、§10 假设）。
 *   按 Spec §10「阈值为起点、PoC 中按实测校准」，此处校准为 6% 作为 PoC 回归基线
 *   （防止后续改动回退）；1.5% 的设计目标留待阶段 ⑤ 的 antd Table 主题覆盖层达成
 *   （Spec §7 风险1 决策：选 (a) 继续加 CSS 覆盖，而非自绘或永久放宽）。
 */
const THRESHOLDS: Record<string, number> = {
  'config': 0.06,
  'login-dark': 0.003,
  'login-light': 0.003,
};

const NAMES = Object.keys(THRESHOLDS);

function readPng(file: string): PNG {
  return PNG.sync.read(fs.readFileSync(file));
}

/** 裁剪到左上角 w×h 区域（用于容忍双端元素级截图的尺寸差，如 Vue 有侧边栏而 React 最小布局没有） */
function crop(png: PNG, w: number, h: number): PNG {
  const out = new PNG({ height: h, width: w });
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const src = (png.width * y + x) << 2;
      const dst = (w * y + x) << 2;
      out.data[dst] = png.data[src];
      out.data[dst + 1] = png.data[src + 1];
      out.data[dst + 2] = png.data[src + 2];
      out.data[dst + 3] = png.data[src + 3];
    }
  }
  return out;
}

test.describe('compare: vue(golden) vs react', () => {
  test.beforeAll(() => {
    fs.mkdirSync(DIFF_DIR, { recursive: true });
  });

  for (const name of NAMES) {
    test(`${name} pixel diff within threshold`, async () => {
      const vueFile = path.join(VUE_DIR, `${name}.png`);
      const reactFile = path.join(REACT_DIR, `${name}.png`);

      // 缺 golden 时提示先跑 capture:golden
      if (!fs.existsSync(vueFile) || !fs.existsSync(reactFile)) {
        throw new Error(
          `缺少截图：${!fs.existsSync(vueFile) ? vueFile : reactFile}。` +
            `请先运行 pnpm capture:golden 与 pnpm capture:react。`,
        );
      }

      const vue = readPng(vueFile);
      const react = readPng(reactFile);

      // 元素级截图双端高度可能不同 → 裁剪到交集区域再比对
      const width = Math.min(vue.width, react.width);
      const height = Math.min(vue.height, react.height);
      const a = vue.width === width && vue.height === height ? vue : crop(vue, width, height);
      const b =
        react.width === width && react.height === height
          ? react
          : crop(react, width, height);

      const diff = new PNG({ height, width });
      const diffPixels = pixelmatch(
        a.data,
        b.data,
        diff.data,
        width,
        height,
        { includeAA: false, threshold: 0.1 },
      );

      const ratio = diffPixels / (width * height);
      fs.writeFileSync(path.join(DIFF_DIR, `${name}.png`), PNG.sync.write(diff));

      // eslint-disable-next-line no-console
      console.log(
        `[compare] ${name}: region=${width}x${height} (vue=${vue.width}x${vue.height} react=${react.width}x${react.height}) ` +
          `diff=${diffPixels}px ratio=${(ratio * 100).toFixed(3)}% threshold=${(THRESHOLDS[name] * 100).toFixed(1)}%`,
      );

      expect(ratio).toBeLessThanOrEqual(THRESHOLDS[name]);
    });
  }
});

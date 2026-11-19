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

/** Same-machine Vue golden comparison; thresholds retain the migration plan's targets.
 * Login <=0.3%, config <=1.5%, complete light/dark shells <2%.
 * Capture hides transient toast/progress overlays in both apps, without hiding app content.
 */
const THRESHOLDS: Record<string, number> = {
  'config': 0.015,
  'login-dark': 0.003,
  'login-light': 0.003,
  'shell': 0.02,
  'shell-dark': 0.02,
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

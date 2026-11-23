import { describe, expect, it } from 'vitest';

import {
  convertToHsl,
  convertToHslCssVar,
  convertToRgb,
  isValidColor,
} from '../convert';

describe('color conversion functions', () => {
  it('should correctly convert color to HSL format', () => {
    const color = '#ff0000';
    const expectedHsl = 'hsl(0 100% 50%)';
    expect(convertToHsl(color)).toEqual(expectedHsl);
  });

  it('should correctly convert color with alpha to HSL format', () => {
    const color = 'rgba(255, 0, 0, 0.5)';
    const expectedHsl = 'hsl(0 100% 50%) 0.5';
    expect(convertToHsl(color)).toEqual(expectedHsl);
  });

  it('should correctly convert color to HSL CSS variable format', () => {
    const color = '#ff0000';
    const expectedHsl = '0 100% 50%';
    expect(convertToHslCssVar(color)).toEqual(expectedHsl);
  });

  it('should correctly convert color with alpha to HSL CSS variable format', () => {
    const color = 'rgba(255, 0, 0, 0.5)';
    const expectedHsl = '0 100% 50% / 0.5';
    expect(convertToHslCssVar(color)).toEqual(expectedHsl);
  });

  it('should correctly convert color to RGB CSS variable format', () => {
    const color = 'hsl(284, 100%, 50%)';
    const expectedRgb = 'rgb(187, 0, 255)';
    expect(convertToRgb(color)).toEqual(expectedRgb);
  });

  it('should correctly convert color with alpha to RGBA CSS variable format', () => {
    const color = 'hsla(284, 100%, 50%, 0.92)';
    const expectedRgba = 'rgba(187, 0, 255, 0.92)';
    expect(convertToRgb(color)).toEqual(expectedRgba);
  });

  // 回归：CSS Color 4 空格语法 + 斜杠透明度（设计令牌 --overlay 的写法）。
  // 此前 TinyColor 解析失败退化成不透明 rgb(0,0,0)，导致 Drawer/Modal 遮罩纯黑盖住应用。
  it('should preserve alpha for CSS Color 4 space syntax with slash alpha', () => {
    expect(convertToRgb('hsl(0 0% 0% / 45%)')).toEqual('rgba(0, 0, 0, 0.45)');
    expect(convertToRgb('hsl(0deg 0% 0% / 40%)')).toEqual('rgba(0, 0, 0, 0.4)');
    expect(convertToRgb('hsl(215 100% 54% / 0.5)')).toEqual(
      convertToRgb('hsla(215, 100%, 54%, 0.5)'),
    );
  });

  it('should parse CSS Color 4 space syntax without alpha as opaque', () => {
    expect(convertToRgb('hsl(215 100% 54%)')).toEqual(
      convertToRgb('hsl(215, 100%, 54%)'),
    );
  });
});

describe('isValidColor', () => {
  it('isValidColor function', () => {
    // 测试有效颜色
    expect(isValidColor('blue')).toBe(true);
    expect(isValidColor('#000000')).toBe(true);

    // 测试无效颜色
    expect(isValidColor('invalid color')).toBe(false);
    expect(isValidColor()).toBe(false);
  });
});

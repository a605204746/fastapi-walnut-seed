// 构建期工具（仅 build/vite 消费）：日期、内容 hash、路径、chalk、package.json 读取
export * from './date';
export { generatorContentHash } from './hash';
export { toPosixPath } from './path';

export { default as colors } from 'chalk';
export { default as fs } from 'node:fs/promises';
export { type PackageJson, readPackageJSON } from 'pkg-types';
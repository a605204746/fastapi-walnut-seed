import type { CSSProperties } from 'react';

import { useEffect, useRef, useState } from 'react';

import { Outlet } from 'react-router';

/**
 * PoC 最小后台外壳。
 *
 * 完整的 BasicLayout（sidebar/header/tabbar/6 种布局模式）在阶段 ⑥ 落地。
 * 这里只提供承载业务页所需的最小结构：内容区填满视口，并把可用高度写入
 * `--vben-content-height`（Page 的 autoContentHeight 依赖此变量，通过继承生效）。
 *
 * 为便于像素门禁对齐，内容区渲染为满视口（对应 Vue 侧 full-content 布局的可视区域），
 * 并打上 data-poc="content" 供 e2e 元素级截图定位。
 */
export function BasicLayout() {
  const contentRef = useRef<HTMLDivElement>(null);
  const [height, setHeight] = useState(0);

  useEffect(() => {
    const el = contentRef.current;
    if (!el) return;
    const update = () => setHeight(el.clientHeight);
    const observer = new ResizeObserver(update);
    observer.observe(el);
    update();
    return () => observer.disconnect();
  }, []);

  return (
    <div className="bg-background flex h-screen overflow-hidden">
      {/* PoC 左侧占位：模拟 Vue 侧边栏宽度（224px），使内容区宽度与 Vue 一致
          （1280-224-Page内边距32=1024），保证表格列位置可与 golden 对齐。
          阶段 ⑥ 换为真实 sidebar 后移除。 */}
      <div className="h-full shrink-0" style={{ width: 224 }}></div>
      <div
        className="flex h-full flex-1 flex-col overflow-hidden"
        data-poc="content"
        ref={contentRef}
        style={{ '--vben-content-height': `${height}px` } as CSSProperties}
      >
        <Outlet />
      </div>
    </div>
  );
}

/**
 * 侧边栏折叠按钮图标（Element Plus `ep:fold` / `ep:expand` 的内联等价物）。
 *
 * 为什么内联而不走 `WalnutIcon` + Iconify：Vue 侧这两个图标来自**离线图标包**
 * （`generate-offline-icons.js` 产物），而 React 侧 `WalnutIcon` 走 `@iconify/react`
 * 的在线请求。无头/弱网环境下在线请求赶不上截图时机，折叠按钮就会渲染成空白
 * （像素门禁实测复现过）。SVG path 取自 api.iconify.design，viewBox 1024。
 */
export function EpFold({ className = 'size-4' }: { className?: string }) {
  return (
    <svg
      aria-hidden="true"
      className={className}
      fill="none"
      viewBox="0 0 1024 1024"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M896 192H128v128h768zm0 256H384v128h512zm0 256H128v128h768zM320 384L128 512l192 128z"
        fill="currentColor"
      />
    </svg>
  );
}

export function EpExpand({ className = 'size-4' }: { className?: string }) {
  return (
    <svg
      aria-hidden="true"
      className={className}
      fill="none"
      viewBox="0 0 1024 1024"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M128 192h768v128H128zm0 256h512v128H128zm0 256h768v128H128zm576-352l192 160l-192 128z"
        fill="currentColor"
      />
    </svg>
  );
}

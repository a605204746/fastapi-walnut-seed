import { useEffect, useRef, useState } from 'react';

/**
 * 数字滚动动画（对应 Vue 基线 shadcn-ui 的 CountToAnimator）。
 *
 * 语义逐条对齐 count-to-animator.vue：
 * - 挂载时从 startVal 线性（transition 默认 linear）滚动到 endVal，duration 1500ms；
 * - 格式化：toFixed(decimals) 后按 separator（默认 ','）千分位插入，prefix/suffix 拼接；
 * - Vue 用 vueuse useTransition 驱动，React 侧以 rAF 线性插值等价实现。
 */
interface CountToProps {
  className?: string;
  decimals?: number;
  duration?: number;
  endVal: number;
  prefix?: string;
  separator?: string;
  startVal?: number;
  suffix?: string;
}

function formatNumber(
  num: number,
  { decimal = '.', decimals = 0, prefix = '', separator = ',', suffix = '' } = {},
) {
  const fixed = Number(num).toFixed(decimals);
  const x = fixed.split('.');
  let x1 = x[0];
  const x2 = x.length > 1 ? decimal + x[1] : '';
  const rgx = /(\d+)(\d{3})/;
  if (separator && x1) {
    while (rgx.test(x1)) {
      x1 = x1.replace(rgx, `$1${separator}$2`);
    }
  }
  return prefix + x1 + x2 + suffix;
}

export function CountTo({
  className,
  decimals = 0,
  duration = 1500,
  endVal,
  prefix = '',
  separator = ',',
  startVal = 0,
  suffix = '',
}: CountToProps) {
  const [display, setDisplay] = useState(() =>
    formatNumber(startVal, { decimals, prefix, separator, suffix }),
  );
  const frame = useRef(0);

  useEffect(() => {
    const startedAt = performance.now();
    const tick = (now: number) => {
      const progress = Math.min(1, (now - startedAt) / duration);
      const value = startVal + (endVal - startVal) * progress;
      setDisplay(formatNumber(value, { decimals, prefix, separator, suffix }));
      if (progress < 1) {
        frame.current = requestAnimationFrame(tick);
      }
    };
    frame.current = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame.current);
  }, [decimals, duration, endVal, prefix, separator, startVal, suffix]);

  return <span className={className}>{display}</span>;
}
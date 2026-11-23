import { cn } from '@/core/shared/utils';

import './walnut-slogan.css';

/** 核桃外轮廓 */
const OUTLINE =
  'M120 28 C158 28 186 58 190 100 C193 138 176 176 148 196 C138 203 128 208 120 208 C112 208 102 203 92 196 C64 176 47 138 50 100 C54 58 82 28 120 28 Z';

/** 中缝 + 核仁纹路（底层常驻实线，叠层跑流光） */
const VEINS = [
  'M120 34 C116 70 124 120 120 202',
  'M112 52 C92 58 84 74 70 76',
  'M110 84 C92 90 88 106 72 110',
  'M112 118 C96 124 92 140 78 146',
  'M114 152 C102 158 98 168 90 174',
  'M128 52 C148 58 156 74 170 76',
  'M130 84 C148 90 152 106 168 110',
  'M128 118 C144 124 148 140 162 146',
  'M126 152 C138 158 142 168 150 174',
];

/** 脉络节点：[cx, cy, r] */
const NODES: [number, number, number][] = [
  [120, 28, 2.4],
  [120, 208, 2.4],
  [70, 76, 2],
  [72, 110, 2],
  [78, 146, 2],
  [90, 174, 2],
  [170, 76, 2],
  [168, 110, 2],
  [162, 146, 2],
  [150, 174, 2],
  [118, 84, 1.7],
  [122, 140, 1.7],
];

/** 星点装饰（仅暗色显示）：[left %, top %, 动画延迟 s] */
const STARS: [number, number, number][] = [
  [14, 16, 0],
  [26, 34, -1.2],
  [18, 66, -2.1],
  [30, 84, -0.6],
  [44, 10, -1.8],
  [60, 14, -3],
  [74, 20, -0.9],
  [84, 40, -2.4],
  [80, 72, -1.5],
  [64, 88, -2.8],
  [40, 90, -1.1],
  [12, 46, -1.9],
];

/**
 * 核桃品牌登录插画（对应 Vue 侧 components/walnut-slogan/src/walnut-slogan.vue）。
 * SVG 结构与 CSS 动画逐字复刻。
 */
export function WalnutSlogan({ className }: { className?: string }) {
  return (
    <div aria-hidden="true" className={cn('walnut-slogan', className)}>
      {STARS.map((star, index) => (
        <i
          className="star"
          key={index}
          style={{
            animationDelay: `${star[2]}s`,
            left: `${star[0]}%`,
            top: `${star[1]}%`,
          }}
        />
      ))}
      <div className="halo"></div>
      <svg className="walnut-svg" viewBox="0 0 240 240">
        <path className="w-outline" d={OUTLINE} />
        {/* 底层：常驻实线纹路，静态下结构完整 */}
        <g className="w-base">
          {VEINS.map((vein, index) => (
            <path d={vein} key={`b-${index}`} />
          ))}
        </g>
        {/* 叠层：沿纹路循环流动的光脉冲 */}
        <g className="w-pulse">
          {VEINS.map((vein, index) => (
            <path
              d={vein}
              key={`p-${index}`}
              style={{ animationDelay: `${index * 0.4}s` }}
            />
          ))}
        </g>
        <g className="w-nodes">
          {NODES.map((node, index) => (
            <circle cx={node[0]} cy={node[1]} key={`n-${index}`} r={node[2]} />
          ))}
        </g>
      </svg>
    </div>
  );
}

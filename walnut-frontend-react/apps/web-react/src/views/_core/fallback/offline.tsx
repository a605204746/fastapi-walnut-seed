import { Fallback } from './fallback';
export default function Page() {
  return (
    <Fallback
      status={'offline' as any}
      title="网络连接已断开"
      description="请检查网络连接后刷新页面。"
    />
  );
}

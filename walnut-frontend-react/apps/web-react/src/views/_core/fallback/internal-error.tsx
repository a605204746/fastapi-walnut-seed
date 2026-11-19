import { Fallback } from './fallback';
export default function Page() {
  return (
    <Fallback
      status={'500' as any}
      title="服务器错误"
      description="服务器暂时出现问题，请稍后重试。"
    />
  );
}

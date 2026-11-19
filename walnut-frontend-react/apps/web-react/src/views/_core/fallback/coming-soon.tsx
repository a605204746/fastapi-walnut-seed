import { Fallback } from './fallback';
export default function Page() {
  return (
    <Fallback
      status={'coming-soon' as any}
      title="即将上线"
      description="此功能正在建设中，敬请期待。"
    />
  );
}

import type { ErrorInfo, ReactNode } from 'react';

import { Component } from 'react';

import { Button, Result } from 'antd';

interface Props {
  children: ReactNode;
}

interface State {
  error: null | Error;
}

/**
 * 全局错误边界。
 *
 * 背景：React 19 下任何未捕获的渲染异常会把整棵根树卸载，页面变成**空白/黑屏**，
 * 用户只能看到「界面没了」而拿不到任何线索（实机反馈过设置抽屉黑屏）。
 * 有了边界后至少显示错误信息 + 重载按钮，便于定位与自救。
 *
 * 对应 Vue 侧 app.vue 的 errorHandler + 错误页兜底思路。
 */
export class ErrorBoundary extends Component<Props, State> {
  override state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  override componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[ErrorBoundary]', error, info.componentStack);
  }

  override render() {
    if (this.state.error) {
      return (
        <div className="flex-center h-screen">
          <Result
            extra={
              <Button onClick={() => window.location.reload()} type="primary">
                重新加载
              </Button>
            }
            status="error"
            subTitle={this.state.error.message}
            title="页面渲染出错"
          />
        </div>
      );
    }
    return this.props.children;
  }
}

export default ErrorBoundary;

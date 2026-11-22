import type { ReactNode } from 'react';

import { cn } from '@walnut-core/shared/utils';

export interface AuthenticationFormViewProps {
  children?: ReactNode;
  className?: string;
  copyright?: ReactNode;
  dataSide?: 'bottom' | 'left' | 'right' | 'top';
}

/**
 * 认证表单面板容器（对应 Vue 侧 layouts/src/authentication/form.vue）。
 * RouterView + Transition + KeepAlive 在 React 下由父级直接传入 children 替代。
 */
export function AuthenticationFormView({
  children,
  className,
  copyright,
  dataSide,
}: AuthenticationFormViewProps) {
  return (
    <div
      className={cn(
        'relative flex-col-center bg-background px-6 py-10 lg:flex-initial lg:px-8 dark:bg-background-deep',
        className,
      )}
    >
      <div
        className="side-content mt-6 w-full sm:mx-auto md:max-w-md lg:max-w-[400px]"
        data-side={dataSide}
      >
        {children}
      </div>
      <div className="absolute bottom-3 flex text-center text-xs text-muted-foreground">
        {copyright}
      </div>
    </div>
  );
}

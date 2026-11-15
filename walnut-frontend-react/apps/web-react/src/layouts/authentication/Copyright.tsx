import { cn } from '@vben-core/shared/utils';

export interface CopyrightProps {
  className?: string;
  companyName?: string;
  companySiteLink?: string;
  date?: string;
  icp?: string;
  icpLink?: string;
}

/** 对应 Vue 侧 layouts/src/basic/copyright/copyright.vue */
export function Copyright({
  className,
  companyName = 'Vben Admin',
  companySiteLink = '',
  date = '2024',
  icp = '',
  icpLink = '',
}: CopyrightProps) {
  return (
    <div className={cn('text-md flex-center', className)}>
      {icp ? (
        <a
          className="mx-1 hover:text-primary-hover"
          href={icpLink || 'javascript:void(0)'}
          target="_blank"
        >
          {icp}
        </a>
      ) : null}
      Copyright © {date}
      {companyName ? (
        <a
          className="mx-1 hover:text-primary-hover"
          href={companySiteLink || 'javascript:void(0)'}
          target="_blank"
        >
          {companyName}
        </a>
      ) : null}
    </div>
  );
}

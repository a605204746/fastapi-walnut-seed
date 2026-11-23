import { useRef } from 'react';
import { $t } from '@/locales';
import { getMessageApi } from '@/services/message';

/** Both Cancel and the close icon protect unsaved edits. */
export function useDirtyClose(getValues: () => unknown) {
  const initial = useRef('');
  return {
    mark: () => {
      initial.current = JSON.stringify(getValues());
    },
    close: (onClose: () => void) => {
      if (initial.current === JSON.stringify(getValues())) {
        onClose();
        return;
      }
      getMessageApi()?.modal.confirm({
        title: $t('pages.common.tip'),
        content: $t('pages.common.beforeCloseTip'),
        centered: true,
        okButtonProps: { danger: true },
        cancelText: $t('common.cancel'),
        okText: $t('common.confirm'),
        onOk: onClose,
      });
    },
  };
}

import { useRef } from 'react';
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
        title: '放弃未保存的修改？',
        content: '关闭后修改将丢失。',
        onOk: onClose,
      });
    },
  };
}

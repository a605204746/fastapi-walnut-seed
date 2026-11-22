import { Store } from '@walnut-core/shared/store';

export interface ModalState {
  centered?: boolean;
  class?: string;
  /**
   * 点击遮罩是否关闭（对应 Vue 侧 ModalProps.closeOnClickModal）。
   * 默认 true；用户导入 / 重置密码这类弹窗会置 false 防误关。
   */
  closeOnClickModal?: boolean;
  confirmDisabled?: boolean;
  confirmLoading?: boolean;
  contentClass?: string;
  destroyOnClose?: boolean;
  footer?: boolean;
  fullscreenButton?: boolean;
  isOpen?: boolean;
  loading?: boolean;
  showCancelButton?: boolean;
  showConfirmButton?: boolean;
  submitting?: boolean;
  title?: string;
}

export interface ModalApiOptions extends ModalState {
  connectedComponent?: any;
  onBeforeClose?: () => boolean | Promise<boolean>;
  onCancel?: () => void;
  onClosed?: () => void;
  onConfirm?: () => void;
  onOpenChange?: (isOpen: boolean) => void;
  onOpened?: () => void;
}

function isFunction(value: any): value is (...args: any[]) => any {
  return typeof value === 'function';
}

/**
 * 弹窗命令式 API（移植自 Vue 侧 popup-ui/src/modal/modal-api.ts，B 类：仅去 Vue 依赖）。
 * 状态建在 TanStack Store 上，React 渲染组件通过 useStore 订阅。
 */
export class ModalApi {
  public sharedData: Record<'payload', any> = { payload: {} };
  public store: Store<ModalState>;

  private api: Pick<
    ModalApiOptions,
    | 'onBeforeClose'
    | 'onCancel'
    | 'onClosed'
    | 'onConfirm'
    | 'onOpenChange'
    | 'onOpened'
  >;

  private state!: ModalState;

  constructor(options: ModalApiOptions = {}) {
    const {
      connectedComponent: _connected,
      onBeforeClose,
      onCancel,
      onClosed,
      onConfirm,
      onOpenChange,
      onOpened,
      ...storeState
    } = options;

    const defaultState: ModalState = {
      centered: false,
      class: '',
      closeOnClickModal: true,
      confirmDisabled: false,
      confirmLoading: false,
      contentClass: '',
      destroyOnClose: true,
      footer: true,
      fullscreenButton: true,
      isOpen: false,
      loading: false,
      showCancelButton: true,
      showConfirmButton: true,
      title: '',
    };

    this.store = new Store<ModalState>({ ...defaultState, ...storeState });

    this.store.subscribe((state) => {
      const prevIsOpen = this.state?.isOpen;
      this.state = state;
      if (state?.isOpen !== prevIsOpen) {
        this.api.onOpenChange?.(!!state?.isOpen);
      }
    });

    this.state = this.store.state;

    this.api = {
      onBeforeClose,
      onCancel,
      onClosed,
      onConfirm,
      onOpenChange,
      onOpened,
    };

    // 绑定方法，保证解构后 this 正确（替代 Vue 侧的 bindMethods）
    for (const key of Object.getOwnPropertyNames(ModalApi.prototype)) {
      if (key !== 'constructor' && isFunction((this as any)[key])) {
        (this as any)[key] = (this as any)[key].bind(this);
      }
    }
  }

  async close() {
    const allowClose = (await this.api.onBeforeClose?.()) ?? true;
    if (allowClose) {
      this.store.setState((prev) => ({ ...prev, isOpen: false }));
    }
  }

  getData<T extends object = Record<string, any>>() {
    return (this.sharedData?.payload ?? {}) as T;
  }

  lock(isLocked = true) {
    return this.setState({ submitting: isLocked });
  }

  modalLoading(loading: boolean) {
    this.setState({ confirmDisabled: loading, loading });
  }

  onCancel() {
    if (this.api.onCancel) {
      this.api.onCancel?.();
    } else {
      void this.close();
    }
  }

  onClosed() {
    if (!this.state.isOpen) {
      this.api.onClosed?.();
    }
  }

  onConfirm() {
    this.api.onConfirm?.();
  }

  onOpened() {
    if (this.state.isOpen) {
      this.api.onOpened?.();
    }
  }

  open() {
    this.store.setState((prev) => ({ ...prev, isOpen: true, submitting: false }));
  }

  setData<T>(payload: T) {
    this.sharedData.payload = payload;
    return this;
  }

  setState(
    stateOrFn: ((prev: ModalState) => Partial<ModalState>) | Partial<ModalState>,
  ) {
    if (isFunction(stateOrFn)) {
      this.store.setState(stateOrFn);
    } else {
      this.store.setState((prev) => ({ ...prev, ...stateOrFn }));
    }
    return this;
  }

  unlock() {
    return this.lock(false);
  }

  /**
   * 每次渲染同步最新回调，避免 useWalnutModal 构造时捕获的闭包过期。
   */
  updateCallbacks(options: ModalApiOptions) {
    this.api = {
      onBeforeClose: options.onBeforeClose,
      onCancel: options.onCancel,
      onClosed: options.onClosed,
      onConfirm: options.onConfirm,
      onOpenChange: options.onOpenChange,
      onOpened: options.onOpened,
    };
  }
}

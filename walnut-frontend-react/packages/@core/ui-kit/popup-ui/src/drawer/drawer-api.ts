import { Store } from '@vben-core/shared/store';

export type DrawerPlacement = 'bottom' | 'left' | 'right' | 'top';

/**
 * 抽屉状态（对应 Vue 侧 popup-ui/src/drawer/drawer.ts 的 `DrawerProps` + `DrawerState`）。
 *
 * Vue 把「可配置项」与「运行时状态」分成 `DrawerProps` / `DrawerState` 两层，
 * React 侧沿用 modal 的做法合并成一个 `DrawerState`，字段名与默认值逐项对齐。
 */
export interface DrawerState {
  cancelText?: string;
  class?: string;
  closable?: boolean;
  closeOnClickModal?: boolean;
  closeOnPressEscape?: boolean;
  confirmDisabled?: boolean;
  confirmLoading?: boolean;
  confirmText?: string;
  contentClass?: string;
  destroyOnClose?: boolean;
  footer?: boolean;
  header?: boolean;
  isOpen?: boolean;
  loading?: boolean;
  modal?: boolean;
  placement?: DrawerPlacement;
  showCancelButton?: boolean;
  showConfirmButton?: boolean;
  submitting?: boolean;
  title?: string;
  zIndex?: number;
}

export interface DrawerApiOptions extends DrawerState {
  /** 独立的抽屉组件；React 侧接受但忽略（见 use-drawer.tsx 的说明） */
  connectedComponent?: any;
  /** 关闭前回调，返回 false 阻止关闭 */
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
 * 抽屉命令式 API（移植自 Vue 侧 popup-ui/src/drawer/drawer-api.ts）。
 *
 * 与同目录 `modal-api.ts` 完全同构：状态建在 TanStack Store 上，React 渲染组件
 * 通过 `useStore` 订阅。方法名（`drawerLoading` / `lock` / `unlock` / `getData` /
 * `setData` / `setState` / `open` / `close`）与 Vue 侧一致，使搬运过来的调用点无需改动。
 */
export class DrawerApi {
  public sharedData: Record<'payload', any> = { payload: {} };
  public store: Store<DrawerState>;

  private api: Pick<
    DrawerApiOptions,
    | 'onBeforeClose'
    | 'onCancel'
    | 'onClosed'
    | 'onConfirm'
    | 'onOpenChange'
    | 'onOpened'
  >;

  private state!: DrawerState;

  constructor(options: DrawerApiOptions = {}) {
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

    // 默认值逐项对齐 Vue 侧 drawer-api.ts 的 defaultState
    const defaultState: DrawerState = {
      class: '',
      closable: true,
      closeOnClickModal: true,
      closeOnPressEscape: true,
      confirmDisabled: false,
      confirmLoading: false,
      contentClass: '',
      destroyOnClose: true,
      footer: true,
      header: true,
      isOpen: false,
      loading: false,
      modal: true,
      placement: 'right',
      showCancelButton: true,
      showConfirmButton: true,
      submitting: false,
      title: '',
    };

    this.store = new Store<DrawerState>({ ...defaultState, ...storeState });

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
    for (const key of Object.getOwnPropertyNames(DrawerApi.prototype)) {
      if (key !== 'constructor' && isFunction((this as any)[key])) {
        (this as any)[key] = (this as any)[key].bind(this);
      }
    }
  }

  /**
   * 关闭抽屉。会先走 `onBeforeClose`，返回 false 则不关闭
   * （业务页用它做「表单有改动时二次确认」，见 apps 的 `utils/popup.ts`）。
   */
  async close() {
    const allowClose = (await this.api.onBeforeClose?.()) ?? true;
    if (allowClose) {
      this.store.setState((prev) => ({
        ...prev,
        isOpen: false,
        submitting: false,
      }));
    }
  }

  /**
   * loading 与 lock 的区别：loading 允许关闭窗口，lock 不允许。
   */
  drawerLoading(loading: boolean) {
    return this.setState({ confirmDisabled: loading, loading });
  }

  getData<T extends object = Record<string, any>>() {
    return (this.sharedData?.payload ?? {}) as T;
  }

  /** 锁定抽屉（提交过程中的等待状态）：禁用取消按钮、隐藏关闭按钮、确认按钮转 loading */
  lock(isLocked = true) {
    return this.setState({ submitting: isLocked });
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
    stateOrFn:
      | ((prev: DrawerState) => Partial<DrawerState>)
      | Partial<DrawerState>,
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

  /** 每次渲染同步最新回调，避免 useVbenDrawer 构造时捕获的闭包过期 */
  updateCallbacks(options: DrawerApiOptions) {
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

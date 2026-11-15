import { setComponentMap } from '@vben-core/form-ui';
import { Checkbox, DatePicker, Input, InputNumber, Radio, Select, Switch, TreeSelect } from 'antd';

const { RangePicker } = DatePicker;
const { TextArea } = Input;

/**
 * 表单组件注册表（对应 Vue 侧 adapter/component/index.ts 的 componentMap）。
 *
 * PoC 只注册 config 页 + 登录页需要的控件；阶段 ⑤ 补齐 31 个组件全集
 * （ApiSelect/ApiTreeSelect/Upload/ImageUpload/RichTextarea/IconPicker 等）。
 * React 侧统一 value + onChange 受控，无需 Vue 的 modelPropName 映射。
 */
export function setupComponents() {
  setComponentMap({
    Checkbox,
    Input,
    InputNumber,
    RadioGroup: Radio.Group,
    RangePicker,
    Select,
    Switch,
    Textarea: TextArea,
    TreeSelect,
  });
}

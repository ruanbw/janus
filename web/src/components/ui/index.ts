// UI 组件库入口:全局注册(组件名 App* 前缀)+ 按需导出
import type { App, Component } from 'vue';

import AppAlert from './AppAlert.vue';
import AppButton from './AppButton.vue';
import AppCard from './AppCard.vue';
import AppCheckbox from './AppCheckbox.vue';
import AppDescriptions from './AppDescriptions.vue';
import AppDescriptionsItem from './AppDescriptionsItem.vue';
import AppDivider from './AppDivider.vue';
import AppEmpty from './AppEmpty.vue';
import AppForm from './AppForm.vue';
import AppFormItem from './AppFormItem.vue';
import AppInput from './AppInput.vue';
import AppInputNumber from './AppInputNumber.vue';
import AppPopconfirm from './AppPopconfirm.vue';
import AppProgress from './AppProgress.vue';
import AppRadio from './AppRadio.vue';
import AppRadioGroup from './AppRadioGroup.vue';
import AppResult from './AppResult.vue';
import AppSelect from './AppSelect.vue';
import AppSpace from './AppSpace.vue';
import AppSpin from './AppSpin.vue';
import AppTable from './AppTable.vue';
import AppTag from './AppTag.vue';
import AppTextarea from './AppTextarea.vue';
import AppTooltip from './AppTooltip.vue';
import AppUpload from './AppUpload.vue';
import CopyText from './CopyText.vue';

const components: Record<string, Component> = {
  AppAlert,
  AppButton,
  AppCard,
  AppCheckbox,
  AppDescriptions,
  AppDescriptionsItem,
  AppDivider,
  AppEmpty,
  AppForm,
  AppFormItem,
  AppInput,
  AppInputNumber,
  AppPopconfirm,
  AppProgress,
  AppRadio,
  AppRadioGroup,
  AppResult,
  AppSelect,
  AppSpace,
  AppSpin,
  AppTable,
  AppTag,
  AppTextarea,
  AppTooltip,
  AppUpload,
  CopyText,
};

export default {
  install(app: App): void {
    for (const [name, component] of Object.entries(components)) {
      app.component(name, component);
    }
  },
};

export { default as AppButton } from './AppButton.vue';
export { default as AppTable } from './AppTable.vue';
export { default as AppSelect } from './AppSelect.vue';
export { default as AppForm } from './AppForm.vue';
export { default as AppFormItem } from './AppFormItem.vue';
export { useFormItem } from './form';
export { message, toasts, dismiss } from './toast';
export { confirm, confirmAsync, closeConfirm } from './confirm';
export type { ConfirmOptions } from './confirm';
export type { FormRule, TableColumn, TablePaginationConfig, SelectOption } from './types';

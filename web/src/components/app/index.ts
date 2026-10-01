// UI 组件库入口:全局注册(组件名 App* 前缀)+ 按需导出
import type { App, Component } from 'vue';

import AppAlert from './AppAlert.vue';
import AppButton from './AppButton.vue';
import AppCard from './AppCard.vue';
import CardContent from './CardContent.vue';
import CardDescription from './CardDescription.vue';
import CardFooter from './CardFooter.vue';
import CardHeader from './CardHeader.vue';
import CardTitle from './CardTitle.vue';
import AppCheckbox from './AppCheckbox.vue';
import AppDescriptions from './AppDescriptions.vue';
import AppDescriptionsItem from './AppDescriptionsItem.vue';
import AppDialog from './AppDialog.vue';
import AppDivider from './AppDivider.vue';
import AppEmpty from './AppEmpty.vue';
import AppForm from './AppForm.vue';
import AppFormItem from './AppFormItem.vue';
import AppInput from './AppInput.vue';
import AppInputNumber from './AppInputNumber.vue';
import AppModal from './AppModal.vue';
import AppPopconfirm from './AppPopconfirm.vue';
import AppProgress from './AppProgress.vue';
import AppRadio from './AppRadio.vue';
import AppRadioGroup from './AppRadioGroup.vue';
import AppRadioCard from './AppRadioCard.vue';
import AppResult from './AppResult.vue';
import AppSelect from './AppSelect.vue';
import AppSpace from './AppSpace.vue';
import AppSpin from './AppSpin.vue';
import AppSwitch from './AppSwitch.vue';
import AppTable from './AppTable.vue';
import AppTabs from './AppTabs.vue';
import AppTabsList from './AppTabsList.vue';
import AppTabsTrigger from './AppTabsTrigger.vue';
import AppTabsContent from './AppTabsContent.vue';
import AppTag from './AppTag.vue';
import AppTextarea from './AppTextarea.vue';
import AppTooltip from './AppTooltip.vue';
import AppUpload from './AppUpload.vue';
import CopyText from './CopyText.vue';

const components: Record<string, Component> = {
  AppAlert,
  AppButton,
  AppCard,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
  AppCheckbox,
  AppDescriptions,
  AppDescriptionsItem,
  AppDialog,
  AppDivider,
  AppEmpty,
  AppForm,
  AppFormItem,
  AppInput,
  AppInputNumber,
  AppModal,
  AppPopconfirm,
  AppProgress,
  AppRadio,
  AppRadioGroup,
  AppRadioCard,
  AppResult,
  AppSelect,
  AppSpace,
  AppSpin,
  AppSwitch,
  AppTable,
  AppTabs,
  AppTabsList,
  AppTabsTrigger,
  AppTabsContent,
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

export { default as AppAlert } from './AppAlert.vue';
export { default as AppButton, buttonVariants } from './AppButton.vue';
export { default as AppCard } from './AppCard.vue';
export { default as CardHeader } from './CardHeader.vue';
export { default as CardTitle } from './CardTitle.vue';
export { default as CardDescription } from './CardDescription.vue';
export { default as CardContent } from './CardContent.vue';
export { default as CardFooter } from './CardFooter.vue';
export { default as AppCheckbox } from './AppCheckbox.vue';
export { default as AppDescriptions } from './AppDescriptions.vue';
export { default as AppDescriptionsItem } from './AppDescriptionsItem.vue';
export { default as AppDialog } from './AppDialog.vue';
export { default as AppDivider } from './AppDivider.vue';
export { default as AppEmpty } from './AppEmpty.vue';
export { default as AppForm } from './AppForm.vue';
export { default as AppFormItem } from './AppFormItem.vue';
export { default as AppInput } from './AppInput.vue';
export { default as AppInputNumber } from './AppInputNumber.vue';
export { default as AppModal } from './AppModal.vue';
export { default as AppPopconfirm } from './AppPopconfirm.vue';
export { default as AppProgress } from './AppProgress.vue';
export { default as AppRadio } from './AppRadio.vue';
export { default as AppRadioGroup } from './AppRadioGroup.vue';
export { default as AppRadioCard } from './AppRadioCard.vue';
export { default as AppResult } from './AppResult.vue';
export { default as AppSelect } from './AppSelect.vue';
export { default as AppSpace } from './AppSpace.vue';
export { default as AppSpin } from './AppSpin.vue';
export { default as AppSwitch, switchVariants, switchThumbVariants } from './AppSwitch.vue';
export { default as AppTable } from './AppTable.vue';
export { default as AppTabs } from './AppTabs.vue';
export { default as AppTabsList } from './AppTabsList.vue';
export { default as AppTabsTrigger } from './AppTabsTrigger.vue';
export { default as AppTabsContent } from './AppTabsContent.vue';
export { default as AppTag } from './AppTag.vue';
export { default as AppTextarea } from './AppTextarea.vue';
export { default as AppTooltip } from './AppTooltip.vue';
export { default as AppUpload } from './AppUpload.vue';
export { default as CopyText } from './CopyText.vue';

export { useFormItem } from './form';
export { message, toasts, dismiss } from './toast';
export { confirm, confirmAsync, closeConfirm } from './confirm';
export type { ConfirmOptions } from './confirm';
export type { FormRule, TableColumn, TablePaginationConfig, SelectOption } from './types';

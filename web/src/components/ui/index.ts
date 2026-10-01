/**
 * components/ui/ 统一出口。
 *
 * 这一层是「shadcn 下载件」层：kebab-case 文件名、只消费 shadcn 语义层令牌
 * （bg-background / text-foreground / border-input / ring-ring …），
 * 不含任何项目语义：既不反向依赖 components/app 目录，也不写主题变体补丁类。
 * 项目组件（`App*`）必须构建在本层之上，不得直接 import `reka-ui`。
 *
 * 这里只做 re-export，不做全局注册——全局注册只发生在 `components/app/index.ts`。
 */

// ---- 表单控件 ----
export { default as Input } from './input.vue';
export { default as Textarea } from './textarea.vue';
export { default as Label } from './label.vue';
export {
  checkboxIndicatorVariants,
  checkboxVariants,
  default as Checkbox,
} from './checkbox.vue';
export {
  switchThumbVariants,
  switchVariants,
  default as Switch,
} from './switch.vue';
export { default as RadioGroup } from './radio-group.vue';
export { radioGroupItemVariants, default as RadioGroupItem } from './radio-group-item.vue';
export {
  radioGroupIndicatorVariants,
  radioGroupIndicatorDotVariants,
  default as RadioGroupIndicator,
} from './radio-group-indicator.vue';
export { selectTriggerVariants, default as SelectTrigger } from './select.vue';
export { default as SelectValue } from './select-value.vue';

// ---- 展示 ----
export {
  badgeVariants,
  default as Badge,
  type BadgeVariant,
} from './badge.vue';
export {
  buttonVariants,
  default as Button,
  type ButtonSize,
  type ButtonVariant,
} from './button.vue';
export { default as Card } from './card.vue';
export { default as CardHeader } from './card-header.vue';
export { default as CardTitle } from './card-title.vue';
export { default as CardDescription } from './card-description.vue';
export { default as CardContent } from './card-content.vue';
export { default as CardFooter } from './card-footer.vue';
export {
  alertDescriptionVariants,
  alertTitleVariants,
  alertVariants,
  default as Alert,
  type AlertVariant,
} from './alert.vue';
export {
  progressIndicatorVariants,
  progressVariants,
  default as Progress,
} from './progress.vue';
export { separatorVariants, default as Separator } from './separator.vue';

// ---- 浮层 ----
export { default as Dialog } from './dialog.vue';
/** Root 也导出为 DialogRoot：项目层按 reka 的习惯名引用，与 shadcn 官方的 `Dialog` 同指一个组件 */
export { default as DialogRoot } from './dialog.vue';
export { default as DialogTrigger } from './dialog-trigger.vue';
export { default as DialogPortal } from './dialog-portal.vue';
export { dialogOverlayVariants, default as DialogOverlay } from './dialog-overlay.vue';
export {
  dialogContentVariants,
  default as DialogContent,
} from './dialog-content.vue';
export { default as DialogClose } from './dialog-close.vue';
export { default as DialogTitle } from './dialog-title.vue';
export { default as DialogDescription } from './dialog-description.vue';

export { default as AlertDialog } from './alert-dialog.vue';
export { default as AlertDialogTrigger } from './alert-dialog-trigger.vue';
export { default as AlertDialogPortal } from './alert-dialog-portal.vue';
export {
  alertDialogOverlayVariants,
  default as AlertDialogOverlay,
} from './alert-dialog-overlay.vue';
export {
  alertDialogContentVariants,
  default as AlertDialogContent,
} from './alert-dialog-content.vue';
export {
  alertDialogActionVariants,
  default as AlertDialogAction,
} from './alert-dialog-action.vue';
export {
  alertDialogCancelVariants,
  default as AlertDialogCancel,
} from './alert-dialog-cancel.vue';
export { default as AlertDialogTitle } from './alert-dialog-title.vue';
export {
  default as AlertDialogDescription,
} from './alert-dialog-description.vue';

export { default as Popover } from './popover.vue';
export { default as PopoverTrigger } from './popover-trigger.vue';
export { default as PopoverPortal } from './popover-portal.vue';
export {
  popoverContentVariants,
  default as PopoverContent,
} from './popover-content.vue';
export { default as PopoverArrow } from './popover-arrow.vue';

export { default as Tooltip } from './tooltip.vue';
export { default as TooltipTrigger } from './tooltip-trigger.vue';
export { default as TooltipPortal } from './tooltip-portal.vue';
export {
  tooltipContentVariants,
  default as TooltipContent,
} from './tooltip-content.vue';
export { default as TooltipArrow } from './tooltip-arrow.vue';

// ---- 导航 ----
export { default as Tabs } from './tabs.vue';
export { tabsListVariants, default as TabsList } from './tabs-list.vue';
export { tabsTriggerVariants, default as TabsTrigger } from './tabs-trigger.vue';
export { default as TabsContent } from './tabs-content.vue';
export { tabsIndicatorVariants, default as TabsIndicator } from './tabs-indicator.vue';

// ---- 表格 ----
export { default as Table } from './table.vue';
export { default as TableHeader } from './table-header.vue';
export { default as TableBody } from './table-body.vue';
export { default as TableFooter } from './table-footer.vue';
export { default as TableRow } from './table-row.vue';
export { default as TableHead } from './table-head.vue';
export { default as TableCell } from './table-cell.vue';
export { default as TableCaption } from './table-caption.vue';

// ---- 下拉菜单 ----
export { default as DropdownMenu } from './dropdown-menu.vue';
export { default as DropdownMenuRoot } from './dropdown-menu.vue';
export { default as DropdownMenuTrigger } from './dropdown-menu-trigger.vue';
export { default as DropdownMenuPortal } from './dropdown-menu-portal.vue';
export {
  dropdownMenuContentVariants,
  default as DropdownMenuContent,
} from './dropdown-menu-content.vue';
export {
  dropdownMenuItemVariants,
  default as DropdownMenuItem,
} from './dropdown-menu-item.vue';
export { default as DropdownMenuLabel } from './dropdown-menu-label.vue';
export { default as DropdownMenuSeparator } from './dropdown-menu-separator.vue';
# CLOAK Web UI 组件库契约(2025 重构版)

技术栈:Vue 3.5 + TypeScript + Vite 6 + Tailwind CSS 4 + Reka UI(无头组件)+ @lucide/vue 图标。
ant-design-vue 已移除,任何文件不得再 import 自 'ant-design-vue' 或使用 a-* 组件。

## 文件写入注意事项(bash 工具)

- 本会话的 read/edit/write 文件工具对仓库挂载只读;写文件请用 cat > 文件 <<'EOF' ... EOF。
- bash 历史展开会对 ! 报错并挂起 heredoc:执行任何含 ! 的写入前,先在同会话跑一次 set +H(会话内持久;新会话需重跑)。
- 内容里的反斜杠请原样写(heredoc 不转义);避免在内容中使用反引号与 ${(外层程序会转义,统一用字符串拼接)。

## 设计令牌(已在 src/styles/main.css 定义,直接用工具类)

- 品牌色:brand-50..950(靛蓝紫,主色 brand-600 #4f46e5)
- 点缀:accent-300..600(青)
- 语义(明暗自动切换):surface / surface-muted / surface-strong / ink / ink-soft / ink-faint / line / line-strong / ok / warn / err / info
- 用法:bg-surface、bg-surface-muted、text-ink、text-ink-soft、text-ink-faint、border-line、border-line-strong、text-ok、text-warn、text-err、bg-err/10 等
- 深色模式:html.dark 由主题 store 控制,不要手动切换
- 字体:系统栈;等宽用 .mono 类;表格表头 app-table thead th 已加粗

## 全局注册组件(无需 import,模板直接用)

| 组件 | 关键 props | 说明 |
| --- | --- | --- |
| AppButton | type: primary/default/text/dashed/ghost;danger;size: small/middle/large;block;loading;disabled;htmlType | #icon 具名插槽;loading 显示 spinner |
| AppInput | v-model(modelValue);type(text/password);placeholder;maxlength;disabled;autocomplete;size;readonly | #prefix/#suffix 插槽;password 自带眼睛切换;@press-enter |
| AppTextarea | v-model;placeholder;maxlength;rows;showCount;disabled | 右下角计数 |
| AppInputNumber | v-model(数字或 undefined);min;max;precision;placeholder;disabled | 失焦时 clamp+四舍五入 |
| AppCheckbox | v-model(boolean);disabled | 默认插槽为文字 |
| AppRadioGroup | v-model(值);disabled | 内含 AppRadio |
| AppRadio | value;disabled | 需在 AppRadioGroup 内 |
| AppSelect | v-model;options:[{value,label,disabled?}];multiple;placeholder;allowClear;showSearch;maxTagCount;loading;disabled | 单选/多选/搜索/清除;@change |
| AppTag | color: default/blue/cyan/green/success/orange/warning/red/error/purple/gold/geekblue 或任意 hex | 内容用默认插槽 |
| AppTooltip | title 或 #title 插槽;placement: top/bottom/left/right | 包裹触发器 |
| AppPopconfirm | title;okText;cancelText;danger;@confirm | 包裹触发按钮 |
| AppProgress | percent;status: normal/exception/active/success;showInfo;strokeWidth | |
| AppAlert | type: info/warning/success/error;showIcon;message;description | |
| AppResult | status: success/error/info/warning;title;subTitle;#extra 插槽 | 认证结果页用 |
| AppCard | title;bordered;size: default/small | 白卡片容器 |
| AppDescriptions / AppDescriptionsItem | column;item 的 label | 键值展示 |
| AppSpin | spinning;size: default/large/small | 包裹内容时遮罩;无内容时居中转圈 |
| AppEmpty | description | 空态 |
| AppDivider | style(透传 margin);默认插槽为文字 | 分隔线 |
| AppSpace | size: small/middle/large/数字;默认插槽为元素 | flex 间隙 |
| AppTable | columns;dataSource;loading;rowKey;pagination(false 或 {current,pageSize,total,showSizeChanger,showTotal});scroll:{x} | #cell 插槽,作用域 {column, record, index};@change({current,pageSize}) |
| AppForm | model;rules;@finish;ref.validate() | 见下方表单说明 |
| AppFormItem | label;name;extra | 校验错误自动展示 |
| AppUpload | accept;beforeUpload(返回 false 阻止);@select(file) | 无内置上传,只选文件 |
| CopyText | text;默认插槽为内容 | 点击复制+勾选反馈 |

## 脚本 API(import)

import { message } from '@/utils/toast';      // message.success/error/warning/info,与 antd 一致
import { confirm } from '@/components/ui/confirm'; // confirm({ title, content?, okText?, cancelText?, danger?, onOk? })
import type { FormRule, TableColumn, TablePaginationConfig } from '@/components/ui/types';

- Modal.confirm 换成 confirm(...)(同参数,onOk 支持 async)
- antd 的 Rule 换成 FormRule(required/type:'email'/min/max/pattern/validator(rule,value)=>Promise)
- antd 的 TableColumnsType 换成 TableColumn[];TablePaginationConfig 同形

## 表单

AppForm 示例:ref="formRef" :model="form" :rules="rules" @finish="onSubmit";
AppFormItem(label,name,extra)+ 任意字段组件;提交按钮 html-type="submit"。
- @finish 校验通过后触发;或手动 await formRef.value?.validate()
- rules 形状与 antd 兼容,原视图的 rules 对象可直接沿用(类型换成 FormRule)
- 字段输入会自动清除错误;extra 支持多行(whitespace-pre-line)

## 表格

AppTable 示例:columns/dataSource/loading/row-key="id"/:pagination/@change;列自定义用 #cell 插槽(作用域 { column, record })。
- columns 元素:{ title?, key, dataIndex?, width?, ellipsis? }
- pagination 为 false 时不显示分页栏;showTotal: (t) => '共 ' + t + ' 条'
- @change 载荷 { current, pageSize }(与 antd 一致,p.current ?? 1)

## 图标

import { Plus, Trash2, Pencil, ... } from '@lucide/vue'; — 大小用 :size="16",颜色继承 currentColor。
常用映射:PlusOutlined→Plus;DeleteOutlined→Trash2;EditOutlined→Pencil;ArrowLeftOutlined→ArrowLeft;StopOutlined→CircleStop;PlayCircleOutlined→CirclePlay;SyncOutlined→RefreshCw;ReloadOutlined→RefreshCw;SwapOutlined→ArrowRightLeft;MailOutlined→Mail;LockOutlined→Lock;GlobalOutlined→Globe;LinkOutlined→Link2;UploadOutlined→Upload;EyeOutlined→Eye;ExclamationCircleOutlined→CircleAlert;MinusCircleOutlined→CircleMinus;DownOutlined→ChevronDown;UserOutlined→User;LogoutOutlined→LogOut;SettingOutlined→Settings;CrownOutlined→Crown;BarChartOutlined→BarChart3;DeploymentUnitOutlined→Boxes;SafetyCertificateOutlined→ShieldCheck;LineChartOutlined→LineChart

## 布局与页面

- AdminLayout(侧边栏+顶栏+深色切换+用户菜单)已重写,页面无需关心
- 页面结构惯例:PageHeader(标题/描述/actions)+ QuotaBar(配额)+ 内容卡片
- 认证页使用 AuthShell(品牌左栏+表单右栏),auth 视图需同步改造以去除 antd

## 目录

- 共享组件:src/components/ui/(禁止修改,除非契约变更)
- 页面:src/views/**(本次改造目标)
- 路由/store/api/types/utils 均不变

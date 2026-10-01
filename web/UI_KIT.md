# CLOAK Web UI 组件库契约(2025 重构版)

技术栈:Vue 3.5 + TypeScript + Vite 6 + Tailwind CSS 4 + Reka UI(无头组件)+ @lucide/vue 图标。
ant-design-vue 已移除,任何文件不得再 import 自 'ant-design-vue' 或使用 a-* 组件。

## 文件写入注意事项

- 写文件优先用 write/edit 工具（已确认对仓库可写）。
- 备选方案：bash 的 `cat > 文件 <<'EOF' ... EOF`。
- 用 bash heredoc 写文件时，bash 历史展开会对 ! 报错并挂起 heredoc：执行任何含 ! 的写入前，先在同会话跑一次 `set +H`（会话内持久；新会话需重跑）。
- 用 bash heredoc 写文件时，内容里的反斜杠原样写（heredoc 不转义）；避免在内容中使用反引号与 ${（外层程序会转义，统一用字符串拼接）。

## 设计令牌(已在 src/styles/main.css 定义,直接用工具类)

- 品牌色:brand-50..950(靛蓝紫,主色 brand-600 #4f46e5)
- 点缀:accent-300..600(青)
- 语义(明暗自动切换):surface / surface-muted / surface-strong / ink / ink-soft / ink-faint / line / line-strong / ok / warn / err / info
- 用法:bg-surface、bg-surface-muted、text-ink、text-ink-soft、text-ink-faint、border-line、border-line-strong、text-ok、text-warn、text-err、bg-err/10 等
- 深色模式:html.dark 由主题 store 控制,不要手动切换
- 字体:系统栈;等宽用 .mono 类;表格表头 app-table thead th 已加粗
- 字体尺度:使用标准化尺度工具类，严禁使用 `text-[Npx]` 任意像素值：
  - `text-2xs`: 11px (0.6875rem)，用于微型标签、辅助状态元数据
  - `text-xs`: 12px (0.75rem)，用于次级说明文本、紧凑表格内容
  - `text-sm`: 14px (0.875rem)，用于正文、表单控件默认字号
  - `text-base`: 16px (1rem)，用于卡片副标题、强调正文
  - `text-xl` (20px)、`text-2xl` (26px)、`text-3xl` (30px)，用于大标题与 KPI 数值
- 进出场动效:采用 Tailwind CSS 4 原生 `@utility` 指令构建，无需外部依赖即可与 `data-[state=...]:` 变体无缝配合：
  - 进场/出场状态：`animate-in`、`animate-out`
  - 透明度：`fade-in-0`、`fade-in-50`、`fade-out-0`
  - 缩放：`zoom-in-95`、`zoom-out-95`
  - 滑动：`slide-in-from-top-2`、`slide-in-from-bottom-2`、`slide-in-from-left-2`、`slide-in-from-right-2`、`slide-in-from-left-1/2`、`slide-in-from-top-[48%]`、`slide-out-to-left-1/2`、`slide-out-to-top-[48%]` 等

## 代码质量与 UI 门禁检查

运行 `pnpm check:ui`（对应脚本 `scripts/check-ui-consistency.mjs`）自动执行门禁检查：
1. 检查 `main.css` 中是否有破坏原生语义的 `.truncate` 覆盖；
2. 检查 legacy 类（`.btn*`, `.tbl*`, `.kpi*`, `.panel*`, `.badge*`, `.switch` 等）是否已被彻底清理（0 残留）；
3. 检查 Tech-Utility 临时变量（`var(--fg)`, `var(--muted)` 等）是否已被语义令牌彻底取代；
4. 检查是否有未定义的 CSS 变量（严格杜绝 `--brand-500` 类静默丢失样式问题）；
5. 检查默认调色板泄漏（`slate-*`, `cyan-*`, `amber-*`, `emerald-*`，除白名单外）；
6. 检查任意像素字号（`text-[Npx]`）；
7. 检查业务 view 视图中裸 HTML 原语（`<button>`, `<input>`, `<select>`, `<table>`）；
8. 检查生产构建产物中的动效变体规则生成。

## 全局注册组件(无需 import,模板直接用)

| 组件 | 关键 props | 说明 |
| --- | --- | --- |
| AppButton | type: primary/default/text/dashed/ghost;danger;size: small/middle/large;block;loading;disabled;htmlType;to | #icon 具名插槽;loading 显示 spinner;传 to 时渲染 RouterLink(保留 middle-click/新标签页/a11y),disabled+loading 时禁止导航 |
| AppInput | v-model(modelValue);type(text/password);placeholder;maxlength;disabled;autocomplete;size;readonly | #prefix/#suffix 插槽;password 自带眼睛切换;@press-enter |
| AppTextarea | v-model;placeholder;maxlength;rows;showCount;disabled | 右下角计数 |
| AppInputNumber | v-model(数字或 undefined);min;max;precision;placeholder;disabled | 失焦时 clamp+四舍五入 |
| AppCheckbox | v-model(boolean);disabled | 默认插槽为文字 |
| AppSwitch | v-model(boolean);disabled;size: default(34×19)/sm(30×17);name | 开关控件(Reka Switch),**只有控件本身不含文字**,文案由调用方并排放;aria-label/title 直接透传到开关按钮上 |
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
| AppTable | columns;dataSource;loading;rowKey;pagination(false 或 {current,pageSize,total,showSizeChanger,showTotal});scroll:{x};rowClass(record,index);rowProps(record,index);rowClickable | #cell 插槽,作用域 {column, record, index};#empty 插槽(作用域 {columns, colspan});@change({current,pageSize});@row-click(record, event) |
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
- 行级样式:rowClass={(r, i) => '...'} 落到 <tr>,rowProps={(r, i) => ({ 'data-x': ... })} 透传属性;scoped 样式用 :deep() 或属性选择器接
- 行点击:@row-click="(record, event) => {}"。挂了监听即自动获得 tabindex=0 与 Enter/Space 键盘可达;只要样式不要点击行为时传 row-clickable="false"
- 空态:#empty 插槽自定义内容(默认 <AppEmpty description="暂无数据" />),colspan 自动等于 columns.length

## 图标

import { Plus, Trash2, Pencil, ... } from '@lucide/vue'; — 大小用 :size="16",颜色继承 currentColor。
常用映射:PlusOutlined→Plus;DeleteOutlined→Trash2;EditOutlined→Pencil;ArrowLeftOutlined→ArrowLeft;StopOutlined→CircleStop;PlayCircleOutlined→CirclePlay;SyncOutlined→RefreshCw;ReloadOutlined→RefreshCw;SwapOutlined→ArrowRightLeft;MailOutlined→Mail;LockOutlined→Lock;GlobalOutlined→Globe;LinkOutlined→Link2;UploadOutlined→Upload;EyeOutlined→Eye;ExclamationCircleOutlined→CircleAlert;MinusCircleOutlined→CircleMinus;DownOutlined→ChevronDown;UserOutlined→User;LogoutOutlined→LogOut;SettingOutlined→Settings;CrownOutlined→Crown;BarChartOutlined→BarChart3;DeploymentUnitOutlined→Boxes;SafetyCertificateOutlined→ShieldCheck;LineChartOutlined→LineChart

## 布局与页面

- AdminLayout(侧边栏+顶栏+深色切换+用户菜单)已重写,页面无需关心
- 页面结构惯例:PageHeader(标题/描述/actions)+ QuotaBar(配额)+ 内容卡片
- 认证页使用 AuthShell(品牌左栏+表单右栏),auth 视图需同步改造以去除 antd

## 目录

- 共享组件:src/components/ui/(禁止修改,除非契约变更)
- 契约变更需同步本文件;本次变更见 .scratch/ui-style-unify/issues/04-ui-kit-contract-gaps.md(AppSwitch 新增、AppButton 的 to、AppTable 行级 API)
- 页面:src/views/**(本次改造目标)
- 路由/store/api/types/utils 均不变

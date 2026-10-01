# CLOAK Web UI 组件库契约(双层 + nova 主题)

技术栈:Vue 3.5 + TypeScript + Vite 6 + Tailwind CSS 4 + Reka UI(无头组件)+ @lucide/vue 图标。
ant-design-vue 已移除,任何文件不得再 import 自 'ant-design-vue' 或使用 a-* 组件。

## 两层组件库(最重要的一条规矩)

```
web/src/components/
  ui/    ← shadcn 风格原语层。kebab-case 文件名(button.vue / input.vue / switch.vue …)，
          相当于「shadcn 下载件」：可以整目录覆盖升级，改它只改外观与状态。
  app/   ← 项目组件层。App* 前缀(AppButton / AppInput / AppTable …)，
          承载本项目的契约：antd 兼容的 props、插槽、表单联动。全局注册只发生在这里。
```

方向是**单向**的，三条硬约束（门禁第 13/14/15 项守着）：

| 约束 | 说明 |
| --- | --- |
| `ui/` 不得 import `app/` | 下载件不认得项目层，避免升级时被项目代码绑死 |
| `app/` 不得直接 import `reka-ui` | 无头件必须先在 `ui/` 包一层外观，再由 `app/` 加契约。白名单只有 `AppSelect.vue`（select 内容区是无样式件）、`AppDialog.vue`（无头底座要 re-export `DialogTitle` 等） |
| `ui/` 只用 shadcn 语义层令牌 | 不得出现 `--surface` `--ink` `--line` `--ok` `--brand` 等项目层令牌与对应工具类 |

`components/layout/**` 与 `layouts/**`（侧栏、主题切换、页面骨架）同属项目层，同样走 `ui/`，受第 14 项约束。

## App\* → ui/\* 映射（改外观时从哪下手）

| app/ | ui/ |
| --- | --- |
| `AppButton` | `button` |
| `AppInput` `AppTextarea` `AppInputNumber` | `input` `textarea` `button` |
| `AppCheckbox` | `checkbox` `label` |
| `AppSwitch` | `switch` |
| `AppRadio` `AppRadioGroup` `AppRadioCard` | `radio-group` `card` |
| `AppSelect` | `select`（触发器外观）+ reka 无样式的内容区 |
| `AppTag` | `badge` |
| `AppCard` + 5 个 `Card*` | `card` `card-header` `card-title` `card-description` `card-content` `card-footer` |
| `AppAlert` | `alert` |
| `AppDivider` | `separator` |
| `AppModal` `AppDialog` | `dialog` 系列 |
| `ConfirmHost` | `alert-dialog` 系列（Esc 不关 / 点遮罩不关是它的交互契约） |
| `AppPopconfirm` | `popover` + `button` |
| `AppTabs*` | `tabs` 系列 |
| `AppTooltip` | `tooltip` 系列 |
| `AppProgress` | `progress` |
| `AppTable` | `table` 系列 + `button`（分页器） |
| `ThemeSwitcher`（layout） | `dropdown-menu` 系列 + `button` |

`AppEmpty` `AppResult` `AppSpace` `AppSpin` `AppUpload` `AppDescriptions*` `CopyText` `Toaster` `AppForm` `AppFormItem` `form.ts` `toast.ts` `confirm.ts` `types.ts` 是项目专属，不依赖原语层。


## 文件写入注意事项

- 写文件优先用 write/edit 工具（已确认对仓库可写）。
- 备选方案：bash 的 `cat > 文件 <<'EOF' ... EOF`。
- 用 bash heredoc 写文件时，bash 历史展开会对 ! 报错并挂起 heredoc：执行任何含 ! 的写入前，先在同会话跑一次 `set +H`（会话内持久；新会话需重跑）。
- 用 bash heredoc 写文件时，内容里的反斜杠原样写（heredoc 不转义）；避免在内容中使用反引号与 ${（外层程序会转义，统一用字符串拼接）。

## 设计令牌(全部定义在 src/styles/theme.css —— 改主题只改这一个文件)

`main.css` 只剩三块：`@import` 与 `@custom-variant`、全局基础(body / 滚动条 / `.app-field` 焦点环 / 表单错误态 / 页面过渡 / `.app-table`)、动效 `@utility`、世界地图填色。**任何令牌都不许写在 main.css。**

两条硬约定：

- 令牌值一律 **hex 或 `rgb()`/`rgba()`**，不许用 `oklch()` / `color-mix()`：门禁要静态解析颜色算对比度，解析不了的写法等于没有门禁。
- 令牌**变量名一个都不增删**（16 个 view 与门禁都依赖），只换取值；新增只允许成组加（下面这些）。

### 字体

- `--font-sans`：Inter Variable 优先（`@fontsource-variable/inter` 打进产物），CJK 走系统栈（PingFang SC / 微软雅黑）
- `--font-mono`：等宽，短码、密钥、域名用 `.mono` 类
- body 已开 `font-variant-numeric: tabular-nums`（后台表格数字对齐）与 Inter 的 `cv11`/`ss01`

### 字号尺度（紧凑，11/12/13/14/16/20/24/32）

| 工具类 | px | 行高 | 用途 |
| --- | --- | --- | --- |
| `text-2xs` | 11 | 1.45 | 微型标签、辅助状态元数据 |
| `text-xs` | 12 | 1.5 | 次级说明、紧凑内容 |
| `text-sm` | 13 | 1.5 | **正文、表单控件、表格默认字号** |
| `text-base` | 14 | 1.55 | 卡片副标题、强调正文 |
| `text-lg` | 16 | 1.5 | 区块小标题 |
| `text-xl` | 20 | 1.4 | 页面主标题 |
| `text-2xl` | 24 | 1.3 | 大标题 |
| `text-3xl` | 32 | 1.2 | KPI 数值 |

**严禁 `text-[Npx]` 任意像素值**（门禁第 6 项）。表格正文 13px 由 `main.css` 的 `.app-table` 单点定义。

### 圆角 / 阴影 / 控件高度

- 圆角：`rounded-md` 6px（控件：按钮/输入框/下拉）、`rounded-lg` 8px（卡片）、`rounded-xl` 10px（弹窗/浮层）、`rounded-2xl` 12px
- 阴影：`shadow-2xs` `shadow-xs`(卡片) `shadow-sm` `shadow-md` `shadow-lg`(弹窗) `shadow-xl`，明暗各一套（`--elev-*` 随主题换值）
- 控件高度：`h-control-sm`(28px) `h-control-md`(32px) `h-control-lg`(40px)，由 `--control-height-*` 映射；写尺寸时用它，不要新造 `h-[34px]`

### 色板（三层，明暗各一套）

- 品牌色:`--brand`（浅色 `#4f46e5` / 深色 `#818cf8`，工具类 `text-brand` `bg-brand/10` `border-brand/30`）。**nova 下靛蓝只用于焦点环、链接、brand 标签，不再当主色。**`--color-brand-50..950` 色阶仅供世界地图填色。
- 点缀:`--color-accent-300..600`(青，认证页品牌栏在用)
- 三层令牌(都定义在 `theme.css` 的 `:root` 与 `.dark` 两块,经 `@theme inline` 映射成工具类):

  1. 项目层(页面骨架与业务视图消费):surface(卡片 / 组件宿主底色)、surface-muted(页面底色)、surface-strong、ink / ink-soft / ink-faint、line / line-strong、ok / warn / err / info、brand、sidebar-*
  2. shadcn 语义层(**`ui/` 原语层只认这层**):background / foreground / card / card-foreground / popover / popover-foreground / primary / primary-foreground / secondary / secondary-foreground / muted / muted-foreground / accent / accent-foreground / destructive / destructive-foreground / border / input / ring
     - 命名与语义照 shadcn 官方,取值由本项目调色板定制。`--background` 指向 `--surface` 而**不是** `--surface-muted`:shadcn 的 --background 是「组件所处的那层底色」,本项目组件坐在卡片上
     - nova 的 primary 是「墨」不是颜色：浅色 `#18181b` / 深色 `#fafafa`，两者互为前景。层级靠**字重**拉开（500/600），不靠色相。开关开态、分页当前页全用它，所以它必须与 `--card` 有 ≥3:1 对比
  3. 控件状态层(shadcn 没有对应语义,项目补齐):control-bg(输入框控件底色)、control-track(关态轨道 / 复选与单选未选填充)、control-track-hover、control-thumb(关态滑块)、control-thumb-edge(滑块与控件未选发丝描边)
- 用法:bg-surface、bg-surface-muted、text-ink、border-line、text-ok、bg-err/10、text-brand 等
- **组件状态色一律走 shadcn 语义层与控件状态层工具类,禁止在 `components/ui/` `components/app/` 与 `views/` 里写 `dark:` 补丁类名**（门禁第 11 项），所有主题差异均由 `:root` 与 `.dark` 令牌自身换值保证：

  | 场景 | 旧写法(打补丁) | 现在写法(纯令牌) |
  | --- | --- | --- |
  | 开关轨道(关) | `bg-surface-muted` | `data-[state=unchecked]:bg-input`(+ `hover:` 变体) |
  | 开关滑块 | `bg-white dark:bg-foreground` | `data-[state=checked]:bg-primary-foreground` |
  | 复选 / 单选(未选) | `bg-transparent dark:bg-input/30` | `bg-control-track` + `border-control-thumb-edge`（实心填充式统一） |
  | 复选 / 单选(选中) | `bg-brand-600 dark:bg-brand-500` + `text-white` | `data-[state=checked]:bg-primary` + `text-primary-foreground` |
  | 主按钮 / brand 标签 | `bg-brand-600 text-white dark:bg-brand-500` | 主按钮 `bg-primary text-primary-foreground`；brand 标签 `text-brand bg-brand/10 border-brand/30` |
  | destructive 按钮 / 标签 | `bg-err text-white` | `bg-destructive text-destructive-foreground` |
  | 输入框 / 选择器 / 文本域 | `bg-transparent dark:bg-input/30` + `border-input` | `bg-control-bg` + `border-input`（零 `dark:` 补丁） |
  | 表格表头 / 行 hover | `bg-surface-muted` | `bg-muted`(hover 同色) |
  | 分页当前页 | `bg-brand-600 text-white` | `bg-primary text-primary-foreground` |
  | 进度条轨道 | `bg-surface-strong` | `bg-control-track/60` |
  | 视图里的靛蓝强调 | `text-brand-600 dark:text-brand-400` | `text-brand`（浅色深靛、深色浅靛由 `--brand` 换值） |

- 描边分工:容器装饰描边用 `border-line`(卡片、弹窗);控件与可交互边界用 `border-input` / `border-border`(开关、复选、单选、输入框、按钮、表格控件)。别把卡片描边也升到 `--border`,否则每张卡片都是 1.5 对比的硬边框
- 焦点环单一出处:组件只挂 `.app-field`,焦点环与错误态环由 main.css 的 `.app-field:focus-visible` 和 `[data-invalid='true'] .app-field` 提供。这两条是**未分层** CSS,优先级高于 `@layer utilities`,组件里再写 `focus-visible:ring-*` / `ring-offset-surface` 会被盖掉
- 深色模式:html.dark 由主题 store 控制,不要手动切换
- 等宽:用 `.mono` 类;表格表头 `app-table thead th` 已加粗
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
8. 检查生产构建产物中的动效变体规则生成;
9. 状态色对比度:解析 `src/styles/theme.css` 的 `:root` 与 `.dark`,校验 13 对状态色(文字 ≥ 4.5、控件填充与焦点环 ≥ 3、细边界 ≥ 1.5),阈值在脚本顶部 `CONTRAST_MIN` 可调;
10. 硬编码纯白:`src/**/*.vue` 里禁止不带 alpha 的 `bg-white` / `text-white` / `border-white`(永远深色的面——侧栏、认证页品牌栏——在脚本白名单 `alwaysDarkAllowList` 里);
11. 禁 dark: 变体:`src/components/ui/`、`src/components/app/`、`src/views/` 下禁止出现任何 `dark:*` 类名,所有主题差异必须在 `theme.css` 的 `:root` / `.dark` 令牌层换值;
12. 业务视图禁止手搓模态遮罩:业务视图禁止裸写 `fixed inset-0` 遮罩,统一使用 `AppModal` / `AppDialog`;
13. 分层方向:`src/components/ui/**` 不得 import `@/components/app`(下载件不反向依赖项目层);
14. 分层方向:`src/components/app/**`、`src/components/layout/**`、`src/layouts/**` 不得直接 import `reka-ui`(白名单 `directRekaAllowList` = `AppSelect.vue` / `AppDialog.vue`,见 issue 03 的例外说明);
15. 分层方向:`src/components/ui/**` 不得使用项目层令牌(`--surface` `--ink` `--line` `--ok` `--warn` `--err` `--info` `--brand` `--sidebar-*` 及对应工具类),只认 shadcn 语义层。

第 4 项(未定义 CSS 变量)的定义面是 `src/styles/*.css` 全部文件,不是只有 main.css。

改任何组件状态色之后,必须按 **build → check** 的顺序复验:

```bash
pnpm type-check
pnpm build        # 必须先 build
pnpm check:ui     # 第 8 项读 dist/assets,dist 缺失会计入违规并非零退出
```

本文件此前把两条命令并列书写、与 README 7.1 的顺序互相矛盾,而第 8 项依赖 dist ——
顺序写错就会得到一个静默跳过产物检查的"通过"。现统一为 build → check,
并由 `.github/workflows/ci.yml` 与 `docker/Dockerfile` 强制执行。

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
| AppTag | color: default/brand/info/ok/warn/err(6 个项目语义色) 或任意 hex | 内容用默认插槽,已废弃 Ant Design 12 色并全面使用语义令牌 |
| AppDialog | open; defaultOpen; modal; class; overlayClass | Reka UI Dialog 封装底座，包含 DialogRoot/Portal/Overlay/Content/Title/Description/Close |
| AppModal | v-model:open; title; description; size: sm/md/lg/xl/2xl/full; closable; padding | 业务受控模态框，提供 #default, #header, #footer 插槽，内置 ESC 监听、焦点捕获、统一遮罩与居中缩放动效 |
| AppTabs / AppTabsList / AppTabsTrigger / AppTabsContent | v-model(当前值); variant: line(下划线型)/pill(胶囊型) | Reka UI Tabs 封装套件，业务视图禁止裸写 role="tab" 按钮 |
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
import { confirm } from '@/components/app/confirm'; // confirm({ title, content?, okText?, cancelText?, danger?, onOk? })
import type { FormRule, TableColumn, TablePaginationConfig } from '@/components/app/types';

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

- 原语层:src/components/ui/(shadcn 风格,可整目录覆盖升级;只用 shadcn 语义层令牌)
- 项目组件层:src/components/app/(App* 全局注册;构建在 ui/ 之上;禁止直连 reka-ui)
- 布局层:src/components/layout/、src/layouts/(同样走 ui/ 原语)
- 令牌:src/styles/theme.css(唯一令牌出处);src/styles/main.css(基础层 + 动效 + 地图填色)
- 页面:src/views/**(只用项目层令牌与 shadcn 语义层,禁 `dark:` 补丁)
- 路由/store/api/types/utils 均不变
- 契约变更需同步本文件;本次双层重构见 .scratch/ui-layers/(spec + 6 个 issue)与 docs/adr/0011-two-layer-component-library.md

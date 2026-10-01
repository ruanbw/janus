// UI 组件库公共类型(对齐 antd 常用形状,便于视图层平滑迁移)

/** 表格列定义 */
export interface TableColumn {
  title?: string;
  key: string;
  dataIndex?: string;
  width?: number | string;
  minWidth?: number | string;
  ellipsis?: boolean;
  nowrap?: boolean;
  align?: 'left' | 'center' | 'right';
  /**
   * 固定列。当前只支持 'right'：表格横向滚动时操作列吸附在右侧，
   * 避免「要操作某一行，得先把表格滚回最右边」。
   * 固定列必须自带不透明底色（AppTable 会补 bg-surface / bg-muted），
   * 否则右侧内容会从它下面透出来。
   */
  fixed?: 'right';
}

/** 表格分页配置(与 antd TablePaginationConfig 兼容的子集) */
export interface TablePaginationConfig {
  current?: number;
  pageSize?: number;
  total?: number;
  showSizeChanger?: boolean;
  showTotal?: (total: number) => string;
}

/** 表单校验规则(与 antd Rule 兼容的子集:required/type/min/max/pattern/validator) */
export interface FormRule {
  required?: boolean;
  /**
   * 把纯空白（"   "）也判成空。与 required 同时生效时才算真正挡住"必填"：
   * 只写 required 的话，用户打几个空格就能提交过去。
 */
  whitespace?: boolean;
  type?: 'email' | 'url' | 'number';
  min?: number;
  max?: number;
  pattern?: RegExp;
  message?: string;
  validator?: (rule: FormRule, value: unknown) => Promise<void> | void;
}

/** Select 选项 */
export interface SelectOption {
  value: string | number;
  label: string;
  disabled?: boolean;
}

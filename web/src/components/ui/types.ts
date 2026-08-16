// UI 组件库公共类型(对齐 antd 常用形状,便于视图层平滑迁移)

/** 表格列定义 */
export interface TableColumn {
  title?: string;
  key: string;
  dataIndex?: string;
  width?: number | string;
  ellipsis?: boolean;
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

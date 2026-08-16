// 轻量表单校验引擎:规则形状与 antd Rule 兼容(required/type/min/max/pattern/validator),
// 由 AppForm + AppFormItem 驱动;AppInput 等字段组件可注入错误态与清除时机。
import { inject } from 'vue';
import type { InjectionKey, Ref } from 'vue';

import type { FormRule } from './types';

export interface FormItemContext {
  name: string;
  invalid: Ref<boolean>;
  errorMessage: Ref<string>;
  validate: () => Promise<boolean>;
  clearError: () => void;
}

export interface FormContext {
  model: Record<string, unknown>;
  rules: Record<string, FormRule[]>;
  registerItem: (ctx: FormItemContext) => void;
  unregisterItem: (ctx: FormItemContext) => void;
}

export const formContextKey: InjectionKey<FormContext> = Symbol('cloak-form');
export const formItemKey: InjectionKey<FormItemContext> = Symbol('cloak-form-item');

export function useFormItem(): FormItemContext | undefined {
  return inject(formItemKey, undefined);
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function checkSync(rule: FormRule, value: unknown): string | null {
  const isEmpty = value === undefined || value === null || value === '';
  if (rule.required && (isEmpty || (Array.isArray(value) && value.length === 0))) {
    return rule.message ?? '该字段为必填项';
  }
  if (isEmpty) return null;
  if (rule.type === 'email' && typeof value === 'string' && EMAIL_RE.test(value) === false) {
    return rule.message ?? '邮箱格式不正确';
  }
  if (typeof value === 'string') {
    if (rule.min !== undefined && value.length < rule.min) {
      return rule.message ?? '长度不能少于 ' + rule.min + ' 位';
    }
    if (rule.max !== undefined && value.length > rule.max) {
      return rule.message ?? '长度不能超过 ' + rule.max + ' 位';
    }
    if (rule.pattern && rule.pattern.test(value) === false) {
      return rule.message ?? '格式不正确';
    }
  }
  if (Array.isArray(value) && rule.max !== undefined && value.length > rule.max) {
    return rule.message ?? '数量超出限制';
  }
  return null;
}

export async function validateRules(rules: FormRule[], value: unknown): Promise<string | null> {
  for (const rule of rules) {
    const syncError = checkSync(rule, value);
    if (syncError) return syncError;
    if (rule.validator) {
      try {
        await rule.validator(rule, value);
      } catch (error) {
        return error instanceof Error ? error.message : '校验失败';
      }
    }
  }
  return null;
}

// 表单校验引擎: 使用成熟的社区工业级校验库 async-validator
// 由 AppForm + AppFormItem 驱动;AppInput 等字段组件可注入错误态与清除时机。
import { inject } from 'vue';
import type { InjectionKey, Ref } from 'vue';
import Schema from 'async-validator';
import type { RuleItem } from 'async-validator';

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

/**
 * validateRules 使用 async-validator 进行多规则校验，自动处理 sync/async、type、pattern 等。
 */
export async function validateRules(rules: FormRule[], value: unknown): Promise<string | null> {
  if (!rules || rules.length === 0) return null;

  // 将 FormRule 映射为 async-validator 的 RuleItem
  const descriptorRules: RuleItem[] = rules.map((r) => {
    const item: RuleItem = {};
    if (r.required !== undefined) item.required = r.required;
    if (r.message !== undefined) item.message = r.message;
    if (r.type !== undefined) item.type = r.type;
    if (r.min !== undefined) item.min = r.min;
    if (r.max !== undefined) item.max = r.max;
    if (r.pattern !== undefined) item.pattern = r.pattern;
    if (r.validator) {
      const origValidator = r.validator;
      item.asyncValidator = async (rule, val) => {
        await origValidator(r, val);
      };
    }
    return item;
  });

  const validator = new Schema({ value: descriptorRules });

  try {
    await validator.validate({ value }, { first: true });
    return null;
  } catch (err: unknown) {
    if (err && typeof err === 'object' && 'errors' in err) {
      const errors = (err as { errors: Array<{ message?: string }> }).errors;
      if (errors && errors.length > 0 && errors[0].message) {
        return errors[0].message;
      }
    }
    if (err instanceof Error) {
      return err.message;
    }
    return '校验失败';
  }
}

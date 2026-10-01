// 表单校验引擎: 使用成熟的社区工业级校验库 async-validator
// 由 AppForm + AppFormItem 驱动;AppInput 等字段组件可注入错误态与清除时机。
import { inject } from 'vue';
import type { InjectionKey, Ref } from 'vue';
import Schema from 'async-validator';
import type { RuleItem } from 'async-validator';

import type { FormRule } from './types';

export interface FormItemContext {
  name: string;
  /**
   * 该表单项的 DOM id。AppFormItem 用它把 <label for> 真正指向内部控件 ——
   * 此前 label 的 for 写的是模型字段名(如 "email"),而页面上并没有同名 id,
   * 于是 label 与控件从未关联:读屏念不出字段名,点标签也聚焦不到输入框。
   * 字段组件(AppInput / AppSelect / AppTextarea / AppInputNumber /
   * AppCheckbox / AppRadio / AppSwitch)统一消费这个 id。
   */
  id: string;
  /**
   * 外层 <label> 自身的 id。单个控件用 `id` 被 label 的 for 指向即可；
   * 一组控件(如 AppRadioGroup)没法用 for 表达,改由 role="radiogroup" +
   * aria-labelledby 指回这个 label,读屏才会念出「短链类型」而不是「单选按钮」。
   */
  labelId: string;
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

export const formContextKey: InjectionKey<FormContext> = Symbol('janus-form');
export const formItemKey: InjectionKey<FormItemContext> = Symbol('janus-form-item');

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
    if (r.whitespace !== undefined) item.whitespace = r.whitespace;
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

/**
 * 「空」的统一判定：undefined / null / 空串 / 纯空白串 / 空数组都算空。
 *
 * async-validator 的 required 只挡 undefined / null / ''，挡不住 "   "；
 * 而 AppFormItem 的 required 是「显示星号 + 拦截提交」两件事的单一开关，
 * 必须在校验引擎之外再兜一次，否则会出现「有红星、却能提交空值」的不一致。
 */
export function isBlank(value: unknown): boolean {
  if (value === undefined || value === null) return true;
  if (typeof value === 'string') return value.trim() === '';
  if (Array.isArray(value)) return value.length === 0;
  return false;
}

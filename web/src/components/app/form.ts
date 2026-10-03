// 表单校验引擎: 使用 zod
// 由 AppForm + AppFormItem 驱动;AppInput 等字段组件可注入错误态与清除时机。
import { inject } from 'vue';
import type { InjectionKey, Ref } from 'vue';
import { z } from 'zod';

/** 字段 schema 表:每个表单项一个 zod schema,是校验的唯一真源 */
export type FormSchema = Record<string, z.ZodTypeAny>;

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
  /** 外部(如接口层)把服务端错误挂回字段上做内联展示 */
  setError: (message: string) => void;
}

export interface FormContext {
  model: Record<string, unknown>;
  schema: FormSchema;
  registerItem: (ctx: FormItemContext) => void;
  unregisterItem: (ctx: FormItemContext) => void;
}

export const formContextKey: InjectionKey<FormContext> = Symbol('janus-form');
export const formItemKey: InjectionKey<FormItemContext> = Symbol('janus-form-item');

export function useFormItem(): FormItemContext | undefined {
  return inject(formItemKey, undefined);
}

/**
 * 用字段 schema 校验单个值,返回第一个错误文案;通过返回 null。
 */
export async function validateWithSchema(schema: z.ZodTypeAny | undefined, value: unknown): Promise<string | null> {
  if (schema === undefined) return null;
  const result = await schema.safeParseAsync(value);
  if (result.success) return null;
  return result.error.issues[0]?.message ?? '校验失败';
}

/**
 * 字段是否必填:schema 拒绝 undefined 或空串即视为必填。
 * 可选字段(.optional() / .nullable() / 默认值)拒之门外,boolean 开关类除外。
 */
export function isRequiredSchema(schema: z.ZodTypeAny | undefined): boolean {
  if (schema === undefined) return false;
  const defName = (schema as { _def?: { typeName?: string } })._def?.typeName;
  if (defName === 'ZodOptional' || defName === 'ZodDefault' || defName === 'ZodNullable') return false;
  return schema.safeParse(undefined).success === false && schema.safeParse('').success === false;
}

/**
 * 必填未填时的文案:取 schema 对空串的第一个错误 message(通常就是页面写的 min(1, '请输入...')),
 * 取不到再按 label 拼一句兜底。
 */
export function requiredMessageOf(schema: z.ZodTypeAny | undefined, label?: string): string {
  if (schema !== undefined) {
    const res = schema.safeParse('');
    if (!res.success && res.error.issues[0]?.message) return res.error.issues[0].message;
    const resUndef = schema.safeParse(undefined);
    if (!resUndef.success && resUndef.error.issues[0]?.message) return resUndef.error.issues[0].message;
  }
  const l = label?.trim();
  return l ? `请填写${l}` : '此项为必填';
}

/**
 * 「空」的统一判定：undefined / null / 空串 / 纯空白串 / 空数组都算空。
 *
 * zod 的 required 等价判定只挡 undefined / null / ''，挡不住 "   "；
 * 而 AppFormItem 的 required 是「显示星号 + 拦截提交」两件事的单一开关，
 * 必须在校验引擎之外再兜一次，否则会出现「有红星、却能提交空值」的不一致。
 */
export function isBlank(value: unknown): boolean {
  if (value === undefined || value === null) return true;
  if (typeof value === 'string') return value.trim() === '';
  if (Array.isArray(value)) return value.length === 0;
  return false;
}

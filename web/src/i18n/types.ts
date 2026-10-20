/**
 * 字典的形状。
 *
 * 叶子只允许 string：数字/布尔/null 会被 t() 原样塞进界面（显示成 "null"），
 * 而 TypeScript 在 `Record<string, unknown>` 下不会拦下来，所以类型要收紧到字符串。
 * 数组同理不收：条目数是界面逻辑（v-for），不该由字典决定两种语言条目一致。
 */
export type Messages = { [key: string]: string | Messages };

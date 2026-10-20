import type { Messages } from '../types';

/**
 * 中文（默认语言）字典。
 *
 * 这是两份字典里的「形状基准」：`en-US.ts` 用 `satisfies typeof zhCN` 校验，
 * 少一条 key、多一条 key、叶子写成非字符串都会在编译期报错 —— 只有占位符
 * 增减是类型看不出来的，所以那部分由 `tests/b8-i18n/locales.spec.ts` 兜底。
 */
const zhCN = {
  common: {
    cancel: '取消',
    save: '保存',
    delete: '删除',
    search: '搜索',
  },
  auth: {
    login: {
      title: '登录后台',
      subtitle: '管理你的域名、短链与访问统计',
      usernameLabel: '账号',
      usernamePlaceholder: '请输入账号',
      passwordLabel: '密码',
      passwordPlaceholder: '请输入密码',
      // 会话保持时长是产品口径，跟着文案一起走，避免改一处漏一处
      rememberMe: '记住我（会话保持 30 天）',
      submit: '登录',
      noAccount: '还没有账号？',
      registerLink: '注册新租户',
      success: '登录成功',
      errorBanned: '账号已被封禁，无法登录',
      errorFallback: '登录失败，请稍后重试',
    },
    register: {
      title: '注册新租户',
      subtitle: '注册后将自动获得平台默认域名',
      // `{domain}` 由页面用 VITE_PLATFORM_DOMAIN 填入；两种语言的占位符集合必须一致，
      // 否则其中一种语言会把 `{domain}` 原样显示给用户
      slugHint: '<前缀>.{domain}',
      usernameLabel: '账号',
      usernameExtra: '3-32 位小写字母、数字、下划线或连字符，用于登录后台',
      usernamePlaceholder: '例如 mybrand',
      passwordLabel: '密码',
      passwordPlaceholder: '至少 8 位',
      confirmPasswordLabel: '确认密码',
      confirmPasswordPlaceholder: '再次输入密码',
      slugLabel: '默认域名前缀',
      slugExtra: '小写字母或数字开头/结尾，可包含连字符；将用于生成你的平台默认域名',
      slugPlaceholder: '例如 mybrand',
      submit: '注册',
      hasAccount: '已有账号？',
      loginLink: '直接登录',
      success: '注册成功',
      errorConflict: '账号或前缀已被占用',
      errorBadRequest: '注册信息不合法，请检查后重试',
      errorFallback: '注册失败，请稍后重试',
    },
    // 校验文案也进字典：zod 的 message 会原样渲染到字段下方，属于界面文案
    validation: {
      usernameRequired: '请输入账号',
      usernamePattern: '账号需为 3-32 位小写字母、数字、下划线或连字符',
      passwordRequired: '请输入密码',
      passwordMin: '密码至少 {n} 位',
      passwordPattern: '密码需同时包含字母与数字',
      confirmPasswordRequired: '请再次输入密码',
      confirmPasswordMismatch: '两次输入的密码不一致',
      slugRequired: '请输入前缀',
      slugPattern: '仅允许小写字母、数字与连字符，且须以字母或数字开头/结尾',
    },
  },
  // 导航标题与路由 meta.title 对齐，后续迁移页面时不必再改口径
  links: {
    title: '短链与目标',
    count: '共 {n} 条',
  },
  domains: {
    title: '域名池',
  },
  rules: {
    title: '规则引擎',
  },
  overview: {
    title: '系统总览',
  },
} satisfies Messages;

export default zhCN;

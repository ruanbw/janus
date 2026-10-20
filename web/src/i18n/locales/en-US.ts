import zhCN from './zh-CN';

/**
 * 英文字典。
 *
 * `satisfies typeof zhCN` 是编译期的结构对账：少 key / 多 key / 叶子类型不对都会
 * 直接报错，不必等到跑测试。类型查不出占位符差异（两边都是 string），那一层由
 * `tests/b8-i18n/locales.spec.ts` 的占位符集合断言兜底。
 */
const enUS = {
  common: {
    cancel: 'Cancel',
    save: 'Save',
    delete: 'Delete',
    search: 'Search',
  },
  auth: {
    login: {
      title: 'Sign in',
      subtitle: 'Manage your domains, short links and visit analytics',
      usernameLabel: 'Username',
      usernamePlaceholder: 'Enter your username',
      passwordLabel: 'Password',
      passwordPlaceholder: 'Enter your password',
      rememberMe: 'Remember me (session lasts 30 days)',
      submit: 'Sign in',
      noAccount: "Don't have an account?",
      registerLink: 'Create a tenant',
      success: 'Signed in',
      errorBanned: 'This account has been banned and cannot sign in',
      errorFallback: 'Sign-in failed, please try again later',
    },
    register: {
      title: 'Create a tenant',
      subtitle: 'A default platform domain is created for you right after sign-up',
      slugHint: '<prefix>.{domain}',
      usernameLabel: 'Username',
      usernameExtra: '3-32 lowercase letters, digits, underscores or hyphens; used to sign in',
      usernamePlaceholder: 'e.g. mybrand',
      passwordLabel: 'Password',
      passwordPlaceholder: 'At least 8 characters',
      confirmPasswordLabel: 'Confirm password',
      confirmPasswordPlaceholder: 'Enter the password again',
      slugLabel: 'Default domain prefix',
      slugExtra:
        'Must start and end with a lowercase letter or digit; hyphens allowed in between. Used to build your default platform domain',
      slugPlaceholder: 'e.g. mybrand',
      submit: 'Create tenant',
      hasAccount: 'Already have an account?',
      loginLink: 'Sign in',
      success: 'Tenant created',
      errorConflict: 'That username or prefix is already taken',
      errorBadRequest: 'Registration details are invalid, please check and try again',
      errorFallback: 'Registration failed, please try again later',
    },
    validation: {
      usernameRequired: 'Enter your username',
      usernamePattern: 'Username must be 3-32 lowercase letters, digits, underscores or hyphens',
      passwordRequired: 'Enter your password',
      passwordMin: 'Password must be at least {n} characters',
      passwordPattern: 'Password must contain both letters and digits',
      confirmPasswordRequired: 'Enter the password again',
      confirmPasswordMismatch: 'The two passwords do not match',
      slugRequired: 'Enter a prefix',
      slugPattern:
        'Only lowercase letters, digits and hyphens are allowed, and it must start and end with a letter or digit',
    },
  },
  links: {
    title: 'Short links',
    count: '{n} in total',
  },
  domains: {
    title: 'Domains',
  },
  rules: {
    title: 'Rules',
  },
  overview: {
    title: 'Overview',
  },
} satisfies typeof zhCN;

export default enUS;

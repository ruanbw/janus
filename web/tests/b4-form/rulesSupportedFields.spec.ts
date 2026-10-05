/**
 * 规则条件字段的「真实数据源」契约。
 *
 * 背景：规则表单的字段下拉里曾放着一个置灰项「ASN / 运营商（数据源待接入）」，
 * 模拟器访客画像里还有一行「asn · 数据源待接入 · 恒不命中」。
 * 平台并没有 ASN 数据源（只有外部工程注册 FactEnricher 才可能填上值），
 * 这种「看得见、选不了、选了也恒不命中」的占位就是假能力：
 * 用户会以为控制台支持 ASN 条件，配出来的规则却永远不生效。
 *
 * 去掉占位后的契约：
 *   1. 字段下拉的唯一来源 FIELD_OPTIONS 恰好是有真实数据源、能真实求值的 12 个字段；
 *   2. 规则表单渲染出的下拉里有这些字段、没有 asn、没有「待接入」文案、没有置灰项；
 *   3. 模拟器访客画像不出现 asn 占位行，字段数与真实数据源一致；
 *   4. 经 API / 插件写入的含 asn 条件的历史规则读回后保存被明确拒绝（报错点名字段），
 *      不允许为了迁就占位而静默丢弃条件。
 */
import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { defineComponent, h, type PropType } from 'vue';

const toastMock = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
}));
const domainsApi = vi.hoisted(() => ({ listDomains: vi.fn() }));
const linksApi = vi.hoisted(() => ({ listLinks: vi.fn() }));
const rulesApi = vi.hoisted(() => ({
  createRule: vi.fn(),
  updateRule: vi.fn(),
  deleteRule: vi.fn(),
  getRule: vi.fn(),
  simulateRules: vi.fn(),
}));

vi.mock('@/utils/toast', () => ({ message: toastMock }));
vi.mock('@/api/domains', () => domainsApi);
vi.mock('@/api/links', () => linksApi);
vi.mock('@/api/rules', () => rulesApi);
vi.mock('@/components/app/confirm', () => ({
  confirm: vi.fn(),
  confirmAsync: vi.fn(async () => true),
  closeConfirm: vi.fn(),
}));

const routerMock = vi.hoisted(() => ({
  push: vi.fn(),
  replace: vi.fn(),
  currentRoute: { value: { name: 'rule-new', params: {}, query: {} } as Record<string, unknown> },
}));
vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router');
  return {
    ...actual,
    useRouter: () => routerMock,
    useRoute: () => routerMock.currentRoute.value,
  };
});

import UI from '@/components/app';
import RuleFormView from '@/views/rules/RuleFormView.vue';
import { FIELD_OPTIONS } from '@/views/rules/ruleMeta';
import { visitorFactsFromServer, visitorFieldViews } from '@/views/rules/ruleSim';

/** 平台有真实数据源、后端能真实求值的字段集合（有且仅有这 12 个）。 */
const SUPPORTED_FIELD_VALUES = [
  'ip',
  'ipattr',
  'country',
  'lang',
  'ref',
  'utm',
  'ua',
  'devtype',
  'os',
  'browser',
  'path',
  'domain',
];

interface StubOption {
  value: string | number;
  label: string;
  disabled?: boolean;
}

/**
 * AppSelect 替身：把每个选项的文案与是否置灰渲染进 DOM。
 * 真实 AppSelect 的选项内容在 Teleport 弹层里，happy-dom 下要开弹层才能读；
 * 本组断言的是「传给下拉的选项集合」，替换成可读 DOM 更直接。
 */
const AppSelectStub = defineComponent({
  name: 'AppSelect',
  inheritAttrs: false,
  props: {
    options: { type: Array as PropType<StubOption[]>, default: () => [] },
  },
  setup(props) {
    return () =>
      h(
        'div',
        { class: 'stub-select' },
        props.options.map((o) =>
          h(
            'span',
            { class: 'stub-option', 'data-disabled': o.disabled ? 'true' : 'false' },
            o.label,
          ),
        ),
      );
  },
});

function mountRuleForm() {
  return mount(RuleFormView, {
    global: {
      plugins: [createPinia(), UI],
      stubs: {
        AppSelect: AppSelectStub,
        RouterLink: { template: '<a><slot /></a>' },
        AppTooltip: { template: '<div><slot /></div>' },
      },
    },
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  setActivePinia(createPinia());
  routerMock.currentRoute.value = { name: 'rule-new', params: {}, query: {} };
  domainsApi.listDomains.mockResolvedValue([]);
  linksApi.listLinks.mockResolvedValue({ items: [], total: 0 });
});

describe('FIELD_OPTIONS 只含真实数据源字段', () => {
  it('字段集合恰好是平台有数据源的 12 个，不含 asn', () => {
    expect(FIELD_OPTIONS).toHaveLength(SUPPORTED_FIELD_VALUES.length);
    expect(new Set(FIELD_OPTIONS.map((f) => f.value))).toEqual(new Set(SUPPORTED_FIELD_VALUES));
  });

  it('每个字段都标注了数据来源，且没有「待接入」类占位文案', () => {
    for (const f of FIELD_OPTIONS) {
      expect(f.source.trim(), `${f.value} 缺少数据源说明`).not.toBe('');
      expect(`${f.label} ${f.hint ?? ''}`, `${f.value} 含占位文案`).not.toMatch(
        /待接入|未接入|暂不支持/,
      );
    }
  });
});

describe('规则表单字段下拉', () => {
  it('渲染 12 个真实字段，不渲染 asn / 「待接入」/ 置灰项', async () => {
    const wrapper = mountRuleForm();
    await flushPromises();

    const labels = wrapper.findAll('.stub-option').map((n) => n.text());
    // 先确认下拉真的渲染了（否则「没有 asn」只是空列表假绿）
    expect(labels).toContain('IP 地址 / CIDR');
    expect(labels).toContain('请求域名 (Host)');

    expect(labels.some((l) => /ASN/i.test(l)), '字段下拉仍在展示 asn').toBe(false);
    expect(labels.some((l) => /待接入|未接入/.test(l)), '字段下拉仍在展示占位文案').toBe(false);
    expect(
      wrapper.find('.stub-option[data-disabled="true"]').exists(),
      '字段下拉仍存在不可选的置灰项',
    ).toBe(false);

    wrapper.unmount();
  });

  it('编辑含 asn 条件的历史规则：保存被拒绝且报错点名该字段，不静默丢条件', async () => {
    routerMock.currentRoute.value = { name: 'rule-edit', params: { id: '7' }, query: {} };
    rulesApi.getRule.mockResolvedValue({
      id: 7,
      name: '历史 ASN 规则',
      description: '',
      priority: 10,
      scope: 'global',
      enabled: true,
      logic: 'all',
      action: 'pass',
      destination: '',
      pageMode: 'default',
      customHtml: '',
      linkIds: [],
      conditions: [{ field: 'asn', operator: 'in', values: ['AS15169'] }],
    });

    const wrapper = mountRuleForm();
    await flushPromises();

    const save = wrapper.findAll('button').find((b) => /保存规则/.test(b.text()));
    expect(save, '未找到「保存规则」按钮').toBeTruthy();
    await save!.trigger('click');
    await flushPromises();

    expect(rulesApi.updateRule).not.toHaveBeenCalled();
    const lastError = String(toastMock.error.mock.calls.at(-1)?.[0] ?? '');
    expect(lastError, '报错必须点名具体字段，用户才知道改哪条条件').toContain('asn');

    wrapper.unmount();
  });
});

describe('模拟器访客画像', () => {
  it('不展示 asn 占位行与「待接入」文案，字段收敛为 12 个有数据源的字段', () => {
    const facts = visitorFactsFromServer({
      ip: '203.0.113.7',
      ipattr: '',
      country: 'US',
      asn: '',
      lang: 'zh-cn',
      ref: '',
      utm: '',
      ua: 'Mozilla/5.0',
      devtype: 'desktop',
      os: 'macOS',
      browser: 'Chrome',
      path: '/promo',
      domain: 'a.example.com',
    });
    const views = visitorFieldViews(facts);

    expect(views.map((v) => v.field), '画像仍在展示 asn').not.toContain('asn');
    expect(views).toHaveLength(SUPPORTED_FIELD_VALUES.length);
    for (const v of views) {
      expect(`${v.label} ${v.note}`, `${v.field} 含占位文案`).not.toMatch(/待接入|未接入/);
    }
    // 不是空列表假绿：真实画像值仍在
    expect(views.find((v) => v.field === 'country')?.value).toBe('US');
    expect(views.find((v) => v.field === 'ip')?.value).toBe('203.0.113.7');
  });
});

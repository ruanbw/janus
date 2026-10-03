/**
 * AppForm 的 validate() 契约（定义处：AppForm.vue 的 validateAll）。
 *
 * 契约是 resolve boolean、**永不 reject**。仓库曾有 4 处调用方误写成
 * `try { await validate() } catch { return }` —— catch 是死代码，校验被静默跳过、
 * 请求照发。本组把「resolve boolean / 永不 reject」这条契约本身钉住，
 * 防止有人日后把失败路径改成 reject（那会让例行的「填错了」流进应用错误机制）。
 *
 * 各调用点「非法输入必须中止请求」的断言见同目录 callersValidateContract.spec.ts。
 */
import { mount } from '@vue/test-utils';
import { z } from 'zod';

import AppForm from '@/components/app/AppForm.vue';
import AppFormItem from '@/components/app/AppFormItem.vue';

function mountForm(model: Record<string, unknown>) {
  return mount(AppForm, {
    props: {
      model,
      schema: { name: z.string().min(1, '请输入名称') },
    },
    slots: { default: '<AppFormItem name="name" label="名称"><input /></AppFormItem>' },
    global: { components: { AppFormItem } },
  });
}

describe('AppForm.validate() 契约', () => {
  it('全部字段通过时 resolve true', async () => {
    const wrapper = mountForm({ name: 'ok' });
    await expect(wrapper.vm.validate()).resolves.toBe(true);
    wrapper.unmount();
  });

  it('任一字段失败时 resolve false', async () => {
    const wrapper = mountForm({ name: '' });
    await expect(wrapper.vm.validate()).resolves.toBe(false);
    wrapper.unmount();
  });

  it('resolve false 而不是 reject：失败是预期控制流，不该抛进应用错误机制', async () => {
    const wrapper = mountForm({ name: '' });
    // 显式断言「不发生 rejection」——若有人改成 throw，这里会红
    const settled = await wrapper.vm.validate().then(
      (v) => ({ state: 'resolved' as const, value: v }),
      () => ({ state: 'rejected' as const }),
    );
    expect(settled).toEqual({ state: 'resolved', value: false });
    wrapper.unmount();
  });

  it('校验失败时错误文案已挂到字段上', async () => {
    const wrapper = mountForm({ name: '' });
    await wrapper.vm.validate();
    expect(wrapper.text()).toContain('请输入名称');
    wrapper.unmount();
  });

  it('校验失败时 AppForm 自身不 emit finish', async () => {
    const wrapper = mountForm({ name: '' });
    await wrapper.find('form').trigger('submit');
    await new Promise((r) => setTimeout(r, 0));
    expect(wrapper.emitted('finish')).toBeUndefined();
    wrapper.unmount();
  });

  it('校验通过时 emit finish 一次', async () => {
    const wrapper = mountForm({ name: 'ok' });
    await wrapper.find('form').trigger('submit');
    await new Promise((r) => setTimeout(r, 0));
    expect(wrapper.emitted('finish')).toHaveLength(1);
    wrapper.unmount();
  });
});
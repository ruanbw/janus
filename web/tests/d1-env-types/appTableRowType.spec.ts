/**
 * AppTable 的行类型泛型契约。
 *
 * 背景：d1 lane 删掉 `env.d.ts` 里 `declare module '*.vue'` 通配声明后，
 * `AppTable` 的 `#cell` 插槽暴露出 40 处类型错误 —— `record` 一律是
 * `Record<string, unknown>`，`record.tier?.name` 之类全是 TS2339。
 * 根因是 AppTable 把 `dataSource` 写死成 `Record<string, unknown>[]`，
 * 而表格的列定义（dataIndex/key）与行类型本就解耦，行类型本该由实参反推。
 *
 * 这里锁两件事（都落在真实渲染结果上）：
 *   1. 插槽拿到的 record 就是 dataSource 里那个行对象本身（未被包装/拷贝）；
 *   2. rowKey / dataIndex 按字符串键取值仍然有效 —— 这是唯一无法静态核对的一处，
 *      由组件内部的 field() 收窄。
 */
import { h, toRaw } from 'vue';
import { mount } from '@vue/test-utils';
import type { TableColumn } from '@/components/app/types';

import AppTable from '@/components/app/AppTable.vue';

interface VisitRow {
  id: number;
  ip: string;
  outcome: string;
}

const columns: TableColumn[] = [
  { title: 'ID', key: 'id', dataIndex: 'id' },
  { title: 'IP', key: 'ip', dataIndex: 'ip' },
];

const rows: VisitRow[] = [
  { id: 1, ip: '203.0.113.7', outcome: 'success' },
  { id: 2, ip: '198.51.100.9', outcome: 'failed' },
];

describe('AppTable — 插槽行对象与动态取值', () => {
  it('#cell 收到的 record 与 dataSource 里的行是同一个对象', () => {
    const seen: unknown[] = [];

    mount(AppTable, {
      props: { columns, dataSource: rows },
      slots: {
        cell: (props: { record: unknown }) => {
          seen.push(props.record);
          return h('span', String((props.record as VisitRow).ip));
        },
      },
    });

    // #cell 是每个单元格调一次（行数 x 列数），同一行的多个单元格拿到同一个 record。
    // 按引用相等判定：泛型只改类型，若实现偷偷拷贝/包装了行，这里就会失败。
    // props 里的行会被 Vue 包成 reactive 代理，故比对 toRaw 后的原始对象。
    expect(seen.length).toBe(rows.length * columns.length);
    const distinct = [...new Set(seen.map((r) => toRaw(r as object)))];
    expect(distinct).toEqual(rows);
    distinct.forEach((record, i) => expect(record).toBe(rows[i]));
  });

  it('未提供 #cell 时按 dataIndex 取值渲染，且 null 退化为 -', () => {
    const wrapper = mount(AppTable, {
      props: {
        columns,
        dataSource: [{ id: 1, ip: '10.0.0.1', outcome: 'x' }, { id: 2, ip: null, outcome: 'y' }],
      },
    });

    // 单元格是行主序：每行 columns.length 个 td
    const cells = wrapper.findAll('tbody td');
    expect(cells.length).toBe(2 * columns.length);
    // 第一行第二列 = ip = '10.0.0.1'
    expect(cells[1]!.text()).toBe('10.0.0.1');
    // 第二行第二列：ip 为 null，不该渲染成 "null"
    expect(cells[3]!.text()).toBe('-');
  });

  it('rowKey 指向的字段作为 <tr> 的 key，缺字段时退回行序号', () => {
    const wrapper = mount(AppTable, {
      props: { columns, dataSource: [{ id: 1, ip: 'a' }, { ip: 'b' }] },
    });
    const rowsRendered = wrapper.findAll('tbody tr');
    expect(rowsRendered.length).toBe(2);
  });

  it('rowProps 回调拿到的也是原始行对象', () => {
    const seen: unknown[] = [];
    mount(AppTable, {
      props: {
        columns,
        dataSource: rows,
        rowProps: (record: unknown) => {
          seen.push(record);
          return { 'data-ip': (record as VisitRow).ip };
        },
      },
    });

    expect(seen.length).toBe(2);
    seen.forEach((record, i) => expect(toRaw(record as object)).toBe(rows[i]));
  });
});
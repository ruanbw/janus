<template>
  <a-config-provider :locale="zhCN" :theme="theme">
    <router-view />
  </a-config-provider>
</template>

<script setup lang="ts">
import zhCN from 'ant-design-vue/es/locale/zh_CN';

// CLOAK 设计令牌:品牌主色 深青(cyan-700) + 深海军蓝侧边栏
// 令牌名对齐 ant-design-vue 4.2.6(如 Layout.colorBgHeader / Table.tableHeaderBg)
const fontFamily = [
  '-apple-system',
  'BlinkMacSystemFont',
  '"Segoe UI"',
  'Roboto',
  '"PingFang SC"',
  '"Hiragino Sans GB"',
  '"Microsoft YaHei"',
  '"Helvetica Neue"',
  'Arial',
  'sans-serif',
].join(', ');

const theme = {
  token: {
    colorPrimary: '#0e7490',
    colorInfo: '#0e7490',
    colorLink: '#0e7490',
    colorLinkHover: '#0f94b8',
    colorLinkActive: '#155e75',
    colorBgLayout: '#f3f5f7',
    colorBorderSecondary: '#e6ebf1',
    colorTextSecondary: '#5b6b7c',
    colorTextTertiary: '#8b98a5',
    borderRadius: 8,
    fontFamily,
    controlOutline: 'rgba(14, 116, 144, 0.12)',
  },
  components: {
    // 深色侧边栏(顶栏白色由 AdminLayout 的 .header 覆盖)
    Layout: {
      colorBgHeader: '#0b1220',
      colorBgBody: '#f3f5f7',
      colorBgTrigger: '#0e7490',
    },
    Menu: {
      colorItemBg: '#0b1220',
      colorSubItemBg: '#0a101d',
      colorItemBgSelected: '#0e7490',
      colorItemTextSelected: '#ffffff',
      colorItemText: 'rgba(203, 213, 225, 0.68)',
      colorItemTextHover: '#f8fafc',
      colorItemBgHover: 'rgba(148, 163, 184, 0.12)',
      colorGroupTitle: 'rgba(148, 163, 184, 0.5)',
      itemMarginInline: 10,
      menuItemHeight: 40,
    },
    Table: {
      tableHeaderBg: '#f8fafc',
      tableHeaderTextColor: '#5b6b7c',
      tableHeaderCellSplitColor: 'transparent',
      tableRowHoverBg: '#f8fafc',
    },
    Progress: {
      progressRemainingColor: '#e9eef3',
    },
  },
};
</script>

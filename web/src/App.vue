<template>
  <router-view />
  <Toaster />
  <ConfirmHost />
</template>

<script setup lang="ts">
import ConfirmHost from '@/components/ui/ConfirmHost.vue';
import Toaster from '@/components/ui/Toaster.vue';
</script>

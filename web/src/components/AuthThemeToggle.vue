<template>
  <div class="auth-theme-toggle">
    <n-button quaternary circle @click="themeStore.toggle">
      <template #icon>
        <n-icon>
          <moon-outline v-if="!themeStore.isDark" />
          <sunny-outline v-else />
        </n-icon>
      </template>
    </n-button>
  </div>
</template>

<script setup lang="ts">
import { useThemeStore } from '../stores/theme'
import { SunnyOutline, MoonOutline } from '@vicons/ionicons5'

const themeStore = useThemeStore()
</script>

<style scoped>
.auth-theme-toggle {
  position: absolute;
  top: 16px;
  right: 16px;
  z-index: 20;
}
</style>

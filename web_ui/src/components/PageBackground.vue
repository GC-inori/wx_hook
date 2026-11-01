<template>
  <div class="fixed inset-0 w-full h-full -z-10 overflow-hidden pointer-events-none select-none">
    <!-- 用户自定义背景壁纸 (如果设置) -->
    <div
      v-if="themeState.bgImage"
      class="absolute inset-0 w-full h-full bg-cover bg-center transition-all duration-500"
      :style="{ backgroundImage: `url(${themeState.bgImage})` }"
    >
      <div class="absolute inset-0 bg-black/60 backdrop-blur-sm"></div>
    </div>

    <!-- 默认 NapCat 标志性 ACG 静态光斑 -->
    <template v-else>
      <div
        class="absolute top-[-10%] left-[-10%] w-[550px] h-[550px] rounded-full bg-primary/15 blur-[120px] transition-colors duration-500"
      ></div>
      <div
        class="absolute top-[25%] right-[-10%] w-[450px] h-[450px] rounded-full bg-secondary/15 blur-[110px] transition-colors duration-500"
      ></div>
      <div
        class="absolute bottom-[-10%] left-[20%] w-[650px] h-[650px] rounded-full bg-purple-500/10 blur-[130px] transition-colors duration-500"
      ></div>
    </template>
  </div>
</template>

<script setup>
import { themeState } from '../utils/theme'
</script>

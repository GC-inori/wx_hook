<template>
  <div class="relative napcat-glass rounded-2xl p-4 overflow-hidden shadow-sm flex items-center gap-4 select-none">
    <!-- 背景水印图案 -->
    <div class="absolute right-[-12px] bottom-[-16px] text-7xl text-slate-900/[0.04] dark:text-white/[0.04] rotate-12 pointer-events-none select-none">
      <svg class="w-24 h-24" fill="currentColor" viewBox="0 0 24 24">
        <path d="M8.5 2C4.36 2 1 4.91 1 8.5c0 1.98 1.01 3.76 2.61 4.96L3 17l4.13-1.37c.44.11.9.17 1.37.17 4.14 0 7.5-2.91 7.5-6.5S12.64 2 8.5 2zm10.5 7.5c-.34 0-.67.03-.99.08 1.54 1.15 2.49 2.8 2.49 4.67 0 .84-.2 1.63-.58 2.34L21 19l-2.09-.7c-.57.34-1.22.55-1.91.55-.42 0-.82-.07-1.2-.19-.88.75-2.02 1.24-3.3 1.34 1.34.62 2.89 1 4.5 1 3.31 0 6-2.24 6-5s-2.69-5-6-5z" />
      </svg>
    </div>

    <!-- 用户头像及状态点 -->
    <div class="relative flex-shrink-0 z-10">
      <div class="w-14 h-14 rounded-full overflow-hidden ring-2 ring-primary/30 bg-slate-100 dark:bg-slate-800 flex items-center justify-center shadow-md">
        <img
          v-if="avatarUrl"
          :src="avatarUrl"
          alt="Avatar"
          class="w-full h-full object-cover"
        />
        <div v-else class="w-full h-full bg-gradient-to-tr from-emerald-600 to-teal-500 flex items-center justify-center text-white font-bold text-lg select-none">
          {{ nickname ? nickname.slice(0, 1).toUpperCase() : 'WX' }}
        </div>
      </div>
      <div
        :class="[
          'w-3.5 h-3.5 rounded-full absolute right-0 bottom-0 border-2 border-white dark:border-[#121622] z-10 transition-colors shadow-sm',
          isOnline ? 'bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.7)]' : 'bg-slate-400 dark:bg-slate-500'
        ]"
      ></div>
    </div>

    <!-- 文本信息 -->
    <div class="flex-col justify-center z-10 min-w-0">
      <div class="text-base sm:text-lg font-bold truncate text-slate-800 dark:text-white tracking-wide">
        {{ nickname || (isOnline ? '已登录微信用户' : '未登录状态') }}
      </div>
      <div class="font-mono text-xs tracking-wider text-slate-500 dark:text-slate-400 truncate mt-0.5">
        {{ wxid || 'WeChat Native Gateway' }}
      </div>
    </div>
  </div>
</template>

<script setup>
defineProps({
  nickname: {
    type: String,
    default: ''
  },
  wxid: {
    type: String,
    default: ''
  },
  avatarUrl: {
    type: String,
    default: ''
  },
  isOnline: {
    type: Boolean,
    default: false
  }
})
</script>

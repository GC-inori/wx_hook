<template>
  <div class="napcat-glass rounded-2xl p-4 shadow-sm space-y-1 select-none">
    <!-- 架构信息 -->
    <div class="flex text-sm gap-3 py-2 items-center transition-colors text-slate-700 dark:text-slate-300 border-b border-slate-200/80 dark:border-white/[0.05]">
      <div class="w-5 h-5 text-slate-400 flex items-center justify-center shrink-0">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
      </div>
      <div class="w-24 font-medium text-slate-700 dark:text-slate-300">系统架构</div>
      <div class="text-xs font-mono flex-1 text-slate-500 dark:text-slate-400">{{ archInfo || 'Linux x86_64' }}</div>
    </div>

    <!-- 微信版本 -->
    <div class="flex text-sm gap-3 py-2 items-center transition-colors text-slate-700 dark:text-slate-300 border-b border-slate-200/80 dark:border-white/[0.05]">
      <div class="w-5 h-5 text-emerald-500 flex items-center justify-center shrink-0">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
        </svg>
      </div>
      <div class="w-24 font-medium text-slate-700 dark:text-slate-300">微信版本</div>
      <div class="text-xs font-mono flex-1 text-slate-500 dark:text-slate-400">v4.1.15.55</div>
      <span class="text-[10px] px-2 py-0.5 rounded-full font-mono font-semibold bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30">
        Supported
      </span>
    </div>

    <!-- 桥接库 -->
    <div class="flex text-sm gap-3 py-2 items-center transition-colors text-slate-700 dark:text-slate-300 border-b border-slate-200/80 dark:border-white/[0.05]">
      <div class="w-5 h-5 text-orange-500 flex items-center justify-center shrink-0">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
        </svg>
      </div>
      <div class="w-24 font-medium text-slate-700 dark:text-slate-300">Rust 桥接</div>
      <div class="text-xs font-mono flex-1 text-slate-500 dark:text-slate-400">librust_bridge.so</div>
      <span class="text-[10px] px-2 py-0.5 rounded-full font-mono font-semibold bg-orange-500/15 text-orange-600 dark:text-orange-400 border border-orange-500/30">
        Hooked
      </span>
    </div>

    <!-- 网关版本 -->
    <div class="flex text-sm gap-3 py-2 items-center transition-colors text-slate-700 dark:text-slate-300">
      <div class="w-5 h-5 text-primary flex items-center justify-center shrink-0">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
        </svg>
      </div>
      <div class="w-24 font-medium text-slate-700 dark:text-slate-300">网关版本</div>
      <div class="text-xs font-mono flex-1 text-slate-500 dark:text-slate-400">v1.0.0-gateway</div>
      <div class="flex items-center">
        <span class="text-[10px] px-2 py-0.5 rounded-full font-mono font-bold bg-primary/20 text-primary border border-primary/30 animate-pulse">
          Active
        </span>
      </div>
    </div>
  </div>
</template>

<script setup>
defineProps({
  archInfo: {
    type: String,
    default: 'Linux x86_64'
  }
})
</script>

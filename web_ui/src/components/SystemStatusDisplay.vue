<template>
  <div class="napcat-glass rounded-2xl p-5 shadow-sm relative overflow-hidden flex flex-col md:flex-row gap-6 items-center justify-between select-none">
    <!-- 左侧指标列表 -->
    <div class="flex-1 w-full space-y-4">
      <!-- CPU 栏 -->
      <div>
        <h2 class="text-sm font-semibold flex items-center gap-2 mb-2 text-slate-800 dark:text-slate-200">
          <svg class="w-4 h-4 text-primary shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" />
          </svg>
          <span>CPU 处理单元</span>
        </h2>
        <div class="grid grid-cols-2 gap-2 text-xs">
          <div class="col-span-2 flex justify-between items-center py-1 border-b border-slate-200/80 dark:border-white/[0.04]">
            <span class="text-slate-500 dark:text-slate-400 font-medium">型号</span>
            <span class="font-mono text-slate-700 dark:text-slate-300 truncate max-w-[200px]">{{ cpuModel || 'Intel(R) Xeon(R) CPU / KVM' }}</span>
          </div>
          <div class="flex justify-between items-center py-1 border-b border-slate-200/80 dark:border-white/[0.04]">
            <span class="text-slate-500 dark:text-slate-400 font-medium">内核数</span>
            <span class="font-mono text-slate-700 dark:text-slate-300">{{ cpuCores || '4 Cores' }}</span>
          </div>
          <div class="flex justify-between items-center py-1 border-b border-slate-200/80 dark:border-white/[0.04]">
            <span class="text-slate-500 dark:text-slate-400 font-medium">系统总负载</span>
            <span class="font-mono text-slate-700 dark:text-slate-300">{{ cpuUsageSystem.toFixed(1) }}%</span>
          </div>
          <div class="col-span-2 flex justify-between items-center py-1">
            <span class="text-slate-500 dark:text-slate-400 font-medium">微信主进程 CPU</span>
            <span class="font-mono text-primary font-semibold">{{ cpuUsageProcess.toFixed(1) }}%</span>
          </div>
        </div>
      </div>

      <!-- 内存 栏 -->
      <div class="pt-2 border-t border-slate-200/80 dark:border-white/[0.06]">
        <h2 class="text-sm font-semibold flex items-center gap-2 mb-2 text-slate-800 dark:text-slate-200">
          <svg class="w-4 h-4 text-purple-500 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
          </svg>
          <span>内存空间占用</span>
        </h2>
        <div class="grid grid-cols-2 gap-2 text-xs">
          <div class="col-span-2 flex justify-between items-center py-1 border-b border-slate-200/80 dark:border-white/[0.04]">
            <span class="text-slate-500 dark:text-slate-400 font-medium">物理总量</span>
            <span class="font-mono text-slate-700 dark:text-slate-300">{{ memTotalMB || 8192 }} MB</span>
          </div>
          <div class="flex justify-between items-center py-1 border-b border-slate-200/80 dark:border-white/[0.04]">
            <span class="text-slate-500 dark:text-slate-400 font-medium">系统使用量</span>
            <span class="font-mono text-slate-700 dark:text-slate-300">{{ memUsedMB || 2480 }} MB</span>
          </div>
          <div class="col-span-2 flex justify-between items-center py-1">
            <span class="text-slate-500 dark:text-slate-400 font-medium">微信进程使用</span>
            <span class="font-mono text-purple-600 dark:text-purple-400 font-semibold">{{ memProcessMB || 380 }} MB</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 右侧：双 UsagePie 同心环 -->
    <div class="flex flex-row md:flex-col gap-4 flex-shrink-0 w-full md:w-44 justify-center items-center pt-2 md:pt-0">
      <UsagePie
        :size="108"
        :percentage="cpuUsageSystem"
        :process-percentage="cpuUsageProcess"
        color="rgba(7, 193, 96, 0.9)"
        process-color="#38bdf8"
        track-color="rgba(148, 163, 184, 0.15)"
        title="CPU占用"
      />
      <UsagePie
        :size="108"
        :percentage="(memUsedMB / (memTotalMB || 8192)) * 100"
        :process-percentage="(memProcessMB / (memTotalMB || 8192)) * 100"
        color="rgba(168, 85, 247, 0.9)"
        process-color="#f59e0b"
        track-color="rgba(148, 163, 184, 0.15)"
        title="内存占用"
      />
    </div>
  </div>
</template>

<script setup>
import UsagePie from './UsagePie.vue'

defineProps({
  cpuModel: String,
  cpuCores: String,
  cpuUsageSystem: {
    type: Number,
    default: 0
  },
  cpuUsageProcess: {
    type: Number,
    default: 0
  },
  memTotalMB: {
    type: Number,
    default: 8192
  },
  memUsedMB: {
    type: Number,
    default: 0
  },
  memProcessMB: {
    type: Number,
    default: 0
  }
})
</script>

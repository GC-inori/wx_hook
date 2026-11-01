<template>
  <div class="relative group/pie flex flex-col items-center justify-center cursor-pointer">
    <!-- SVG 环形进度条 -->
    <div class="relative w-32 h-32 md:w-36 md:h-36 flex items-center justify-center">
      <svg
        class="w-full h-full -rotate-90 transform"
        :viewBox="`0 0 ${size} ${size}`"
      >
        <!-- 轨道底色 Track -->
        <circle
          :cx="center"
          :cy="center"
          :r="radius"
          fill="none"
          stroke="rgba(255, 255, 255, 0.08)"
          :stroke-width="strokeWidth"
          stroke-linecap="round"
        />

        <!-- 系统整体占用 (其他进程 + 微信) System Usage -->
        <circle
          :cx="center"
          :cy="center"
          :r="radius"
          fill="none"
          stroke="#EF8664"
          :stroke-width="strokeWidth"
          stroke-linecap="round"
          :stroke-dasharray="systemDash"
          class="transition-all duration-700 ease-out"
        />

        <!-- 微信主线程占用 WeChat Process Usage -->
        <circle
          :cx="center"
          :cy="center"
          :r="radius"
          fill="none"
          stroke="#D33FF0"
          :stroke-width="strokeWidth"
          stroke-linecap="round"
          :stroke-dasharray="processDash"
          class="transition-all duration-700 ease-out"
        />
      </svg>

      <!-- 中心百分比与标题 -->
      <div class="absolute inset-0 flex flex-col items-center justify-center pointer-events-none select-none">
        <span v-if="title" class="text-[10px] font-medium mb-0.5 opacity-80 uppercase tracking-widest text-slate-400">
          {{ title }}
        </span>
        <div class="flex items-baseline gap-0.5">
          <span class="text-2xl font-bold font-mono tracking-tight text-white">
            {{ Math.round(cleanSystem) }}
          </span>
          <span class="text-xs font-bold text-slate-400">%</span>
        </div>
      </div>
    </div>

    <!-- 悬浮 Tooltip 提示浮层 (NapCat 风格) -->
    <div class="absolute -top-16 opacity-0 pointer-events-none group-hover/pie:opacity-100 group-hover/pie:pointer-events-auto transition-all duration-200 z-50 bg-[#1e2333] border border-white/10 rounded-xl p-2.5 shadow-xl text-xs space-y-1.5 whitespace-nowrap min-w-[130px]">
      <div class="flex items-center justify-between gap-3">
        <div class="flex items-center gap-1.5">
          <span class="w-2 h-2 rounded-full bg-[#D33FF0]"></span>
          <span class="text-slate-300">微信进程:</span>
        </div>
        <span class="font-mono font-semibold text-white">{{ cleanProcess.toFixed(1) }}%</span>
      </div>
      <div class="flex items-center justify-between gap-3">
        <div class="flex items-center gap-1.5">
          <span class="w-2 h-2 rounded-full bg-[#EF8664]"></span>
          <span class="text-slate-300">其他进程:</span>
        </div>
        <span class="font-mono font-semibold text-white">{{ otherUsage.toFixed(1) }}%</span>
      </div>
      <div class="flex items-center justify-between gap-3 border-t border-slate-700/60 pt-1">
        <div class="flex items-center gap-1.5">
          <span class="w-2 h-2 rounded-full bg-slate-600"></span>
          <span class="text-slate-400">系统空闲:</span>
        </div>
        <span class="font-mono font-semibold text-slate-400">{{ (100 - cleanSystem).toFixed(1) }}%</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  systemUsage: {
    type: Number,
    default: 0
  },
  processUsage: {
    type: Number,
    default: 0
  },
  title: {
    type: String,
    default: ''
  }
})

const size = 100
const strokeWidth = 10
const radius = (size - strokeWidth) / 2
const circumference = 2 * Math.PI * radius
const center = size / 2

const cleanSystem = computed(() => {
  const rawSys = Math.max(props.systemUsage || 0, 0)
  const rawProc = Math.max(props.processUsage || 0, 0)
  return Math.min(Math.max(rawSys, rawProc), 100)
})

const cleanProcess = computed(() => {
  const rawProc = Math.max(props.processUsage || 0, 0)
  return Math.min(rawProc, cleanSystem.value)
})

const otherUsage = computed(() => {
  return Math.max(cleanSystem.value - cleanProcess.value, 0)
})

const systemDash = computed(() => {
  return `${(cleanSystem.value / 100) * circumference} ${circumference}`
})

const processDash = computed(() => {
  return `${(cleanProcess.value / 100) * circumference} ${circumference}`
})
</script>

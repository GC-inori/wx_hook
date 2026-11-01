<template>
  <div class="fixed top-6 left-1/2 -translate-x-1/2 z-50 flex flex-col items-center gap-2 pointer-events-none select-none">
    <transition-group
      enter-active-class="transition duration-300 ease-out transform"
      enter-from-class="-translate-y-4 opacity-0 scale-95"
      enter-to-class="translate-y-0 opacity-100 scale-100"
      leave-active-class="transition duration-200 ease-in transform"
      leave-from-class="translate-y-0 opacity-100 scale-100"
      leave-to-class="-translate-y-3 opacity-0 scale-95"
    >
      <div
        v-for="item in toastState.toasts"
        :key="item.id"
        class="pointer-events-auto bg-[#171c2b]/95 border border-white/15 backdrop-blur-xl rounded-full px-4 py-2.5 shadow-2xl flex items-center gap-2.5 text-xs text-white"
      >
        <!-- Success Icon -->
        <svg v-if="item.type === 'success'" class="w-4 h-4 text-emerald-400 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>

        <!-- Error Icon -->
        <svg v-else-if="item.type === 'error'" class="w-4 h-4 text-rose-400 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>

        <!-- Info Icon -->
        <svg v-else class="w-4 h-4 text-sky-400 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>

        <span class="font-medium tracking-wide">{{ item.message }}</span>

        <button
          @click="toast.remove(item.id)"
          class="ml-1 text-slate-400 hover:text-white p-0.5 rounded-full transition-colors"
        >
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
    </transition-group>
  </div>
</template>

<script setup>
import { toastState, toast } from '../utils/dialog'
</script>

<template>
  <transition
    enter-active-class="transition duration-200 ease-out"
    enter-from-class="opacity-0"
    enter-to-class="opacity-100"
    leave-active-class="transition duration-150 ease-in"
    leave-from-class="opacity-100"
    leave-to-class="opacity-0"
  >
    <div
      v-if="dialogState.visible"
      class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-md select-none"
      @click.self="dialogState.onCancel && dialogState.onCancel()"
    >
      <transition
        enter-active-class="transition duration-300 ease-out transform"
        enter-from-class="scale-95 opacity-0 translate-y-3"
        enter-to-class="scale-100 opacity-100 translate-y-0"
        leave-active-class="transition duration-200 ease-in transform"
        leave-from-class="scale-100 opacity-100 translate-y-0"
        leave-to-class="scale-95 opacity-0 translate-y-3"
      >
        <div
          v-if="dialogState.visible"
          class="napcat-glass rounded-3xl p-6 max-w-md w-full border border-white/15 shadow-2xl space-y-4"
        >
          <!-- 弹窗标题 -->
          <div class="flex items-center justify-between pb-3 border-b border-white/[0.08]">
            <div class="flex items-center gap-2.5">
              <div class="w-8 h-8 rounded-full bg-primary/20 text-primary flex items-center justify-center shrink-0">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
              <h3 class="text-base font-bold text-white tracking-wide">
                {{ dialogState.title }}
              </h3>
            </div>
            <button
              @click="dialogState.onCancel && dialogState.onCancel()"
              class="text-slate-400 hover:text-white p-1 rounded-full hover:bg-white/10 transition-colors"
            >
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>

          <!-- 弹窗正文 -->
          <div class="text-sm text-slate-300 leading-relaxed break-words py-1">
            {{ dialogState.content }}
          </div>

          <!-- 弹窗底部操作按钮 -->
          <div class="flex items-center justify-end gap-3 pt-3 border-t border-white/[0.08]">
            <button
              v-if="dialogState.showCancel"
              @click="dialogState.onCancel && dialogState.onCancel()"
              class="px-5 py-2 rounded-full text-xs font-medium text-slate-300 bg-white/[0.06] hover:bg-white/[0.12] transition-colors border border-white/5 active:scale-95"
            >
              {{ dialogState.cancelText }}
            </button>
            <button
              @click="dialogState.onConfirm && dialogState.onConfirm()"
              class="px-6 py-2 rounded-full text-xs font-semibold text-white bg-primary hover:bg-primary-500 shadow-md shadow-primary/30 transition-all active:scale-95"
            >
              {{ dialogState.confirmText }}
            </button>
          </div>
        </div>
      </transition>
    </div>
  </transition>
</template>

<script setup>
import { dialogState } from '../utils/dialog'
</script>

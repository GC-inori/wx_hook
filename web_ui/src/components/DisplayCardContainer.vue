<template>
  <div class="napcat-glass rounded-2xl overflow-hidden shadow-sm flex flex-col justify-between">
    <!-- 头部：标题胶囊与开关 -->
    <div class="p-4 pb-2 flex items-center justify-between gap-3">
      <div class="inline-flex items-center px-3 py-1 rounded-lg bg-white/[0.06] border border-white/5">
        <span class="font-bold text-white text-sm truncate select-text">
          {{ title }}
        </span>
      </div>
      <!-- 启用状态切换 Switch (NapCat 风格) -->
      <button
        type="button"
        @click="$emit('update:enabled', !enabled)"
        :class="[
          'relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none',
          enabled ? 'bg-primary' : 'bg-slate-700'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-lg ring-0 transition duration-200 ease-in-out',
            enabled ? 'translate-x-4' : 'translate-x-0'
          ]"
        />
      </button>
    </div>

    <!-- 主体：2x2 参数方块或插槽 -->
    <div class="px-4 py-2 text-sm text-slate-300">
      <slot></slot>
    </div>

    <!-- 底部操作按钮栏 (NapCat 风格: 编辑 / 调试 / 复制 / 删除) -->
    <div class="px-4 pb-4 pt-2 border-t border-white/[0.04] mt-2">
      <slot name="action"></slot>
    </div>
  </div>
</template>

<script setup>
defineProps({
  title: {
    type: String,
    required: true
  },
  enabled: {
    type: Boolean,
    default: true
  }
})

defineEmits(['update:enabled'])
</script>

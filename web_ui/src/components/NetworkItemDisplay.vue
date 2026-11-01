<template>
  <div
    :class="[
      'napcat-glass napcat-glass-hover rounded-2xl p-4 flex flex-col items-center justify-center text-center shadow-sm select-none',
      isPrimary ? 'border-primary/40 shadow-[0_0_15px_rgba(7,193,96,0.15)] ring-1 ring-primary/30' : ''
    ]"
  >
    <div
      :class="[
        'font-mono font-bold tracking-tight mb-1',
        isPrimary ? 'text-4xl md:text-5xl text-primary drop-shadow-[0_0_10px_rgba(7,193,96,0.3)]' : 'text-3xl md:text-4xl text-slate-800 dark:text-white'
      ]"
    >
      {{ count }}
    </div>
    <div class="text-xs font-medium text-slate-500 dark:text-slate-400 whitespace-nowrap">
      {{ label }}
    </div>
  </div>
</template>

<script setup>
defineProps({
  count: {
    type: [Number, String],
    default: 0
  },
  label: {
    type: String,
    required: true
  },
  isPrimary: {
    type: Boolean,
    default: false
  }
})
</script>

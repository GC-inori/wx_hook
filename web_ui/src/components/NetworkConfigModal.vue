<template>
  <div
    v-if="visible"
    class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-md select-none"
    @click.self="$emit('close')"
  >
    <div class="napcat-glass rounded-3xl p-6 max-w-lg w-full border border-slate-200 dark:border-white/15 shadow-2xl space-y-5 animate-in fade-in zoom-in-95 duration-200">
      <!-- Modal Header -->
      <div class="flex items-center justify-between pb-3 border-b border-slate-200/80 dark:border-white/[0.08]">
        <div class="flex items-center gap-2.5">
          <div class="w-8 h-8 rounded-full bg-primary/20 text-primary flex items-center justify-center shrink-0">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4" />
            </svg>
          </div>
          <h3 class="text-base font-bold text-slate-800 dark:text-white tracking-wide">
            {{ isEditing ? '编辑网络端点' : '新建网络端点' }}
          </h3>
        </div>
        <button
          @click="$emit('close')"
          class="text-slate-400 hover:text-slate-600 dark:hover:text-white p-1 rounded-full hover:bg-slate-100 dark:hover:bg-white/10 transition-colors"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <!-- Form Body -->
      <form @submit.prevent="handleSave" class="space-y-4 text-xs">
        <div>
          <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">端点类型</label>
          <select
            v-model="form.type"
            :disabled="isEditing"
            class="w-full bg-slate-50 dark:bg-black/40 border border-slate-200 dark:border-white/10 rounded-xl px-3 py-2.5 text-slate-800 dark:text-slate-100 outline-none focus:border-primary font-medium"
          >
            <option value="httpServer">HTTP API 服务端 (HTTP Server)</option>
            <option value="httpClient">HTTP Webhook 客户端 (HTTP Client)</option>
            <option value="wsServer">WebSocket 服务端 (WS Server)</option>
            <option value="wsClient">WebSocket 客户端 / 反向代理 (WS Client)</option>
          </select>
        </div>

        <div>
          <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">配置名称</label>
          <input
            v-model="form.name"
            type="text"
            required
            placeholder="例如: 主应用 API 服务 / NoneBot 消息上报"
            class="w-full bg-slate-50 dark:bg-black/40 border border-slate-200 dark:border-white/10 rounded-xl px-3 py-2.5 text-slate-800 dark:text-slate-100 outline-none focus:border-primary"
          />
        </div>

        <!-- 条件展示：如果是 Client 展示 URL，如果是 Server 展示 Host + Port -->
        <div v-if="form.type === 'httpClient' || form.type === 'wsClient'">
          <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">目标地址 (URL)</label>
          <input
            v-model="form.url"
            type="text"
            required
            placeholder="例如: http://127.0.0.1:5700/webhook 或 ws://127.0.0.1:8080/ws"
            class="w-full bg-slate-50 dark:bg-black/40 border border-slate-200 dark:border-white/10 rounded-xl px-3 py-2.5 text-slate-800 dark:text-slate-100 outline-none focus:border-primary font-mono"
          />
        </div>

        <div v-else class="grid grid-cols-2 gap-3">
          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">监听主机 (Host)</label>
            <input
              v-model="form.host"
              type="text"
              placeholder="0.0.0.0 或 127.0.0.1"
              class="w-full bg-slate-50 dark:bg-black/40 border border-slate-200 dark:border-white/10 rounded-xl px-3 py-2.5 text-slate-800 dark:text-slate-100 outline-none focus:border-primary font-mono"
            />
          </div>
          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">监听端口 (Port)</label>
            <input
              v-model="form.port"
              type="number"
              placeholder="8080"
              class="w-full bg-slate-50 dark:bg-black/40 border border-slate-200 dark:border-white/10 rounded-xl px-3 py-2.5 text-slate-800 dark:text-slate-100 outline-none focus:border-primary font-mono"
            />
          </div>
        </div>

        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">鉴权 Token (可选)</label>
            <input
              v-model="form.token"
              type="text"
              placeholder="Bearer Token 密钥"
              class="w-full bg-slate-50 dark:bg-black/40 border border-slate-200 dark:border-white/10 rounded-xl px-3 py-2.5 text-slate-800 dark:text-slate-100 outline-none focus:border-primary font-mono"
            />
          </div>
          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">心跳间隔 (毫秒)</label>
            <input
              v-model="form.heartbeat"
              type="number"
              placeholder="30000"
              class="w-full bg-slate-50 dark:bg-black/40 border border-slate-200 dark:border-white/10 rounded-xl px-3 py-2.5 text-slate-800 dark:text-slate-100 outline-none focus:border-primary font-mono"
            />
          </div>
        </div>

        <div class="flex items-center gap-2 pt-2">
          <input
            v-model="form.enabled"
            type="checkbox"
            id="endpoint-enable"
            class="rounded bg-slate-100 dark:bg-black/40 border-slate-300 dark:border-slate-700 text-primary focus:ring-0"
          />
          <label for="endpoint-enable" class="text-xs text-slate-700 dark:text-slate-300 cursor-pointer font-medium">创建后立即启用此端点</label>
        </div>

        <!-- Footer -->
        <div class="flex items-center justify-end gap-2.5 pt-4 border-t border-slate-200/80 dark:border-white/[0.08]">
          <button
            type="button"
            @click="$emit('close')"
            class="px-4 py-2 rounded-xl text-xs font-semibold bg-slate-100 dark:bg-white/10 text-slate-600 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-white/15 transition-all"
          >
            取消
          </button>
          <button
            type="submit"
            class="px-5 py-2 rounded-xl text-xs font-semibold bg-primary hover:bg-emerald-600 text-white shadow-md shadow-primary/25 transition-all active:scale-95"
          >
            {{ isEditing ? '保存修改' : '确认创建' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, computed } from 'vue'

const props = defineProps({
  visible: Boolean,
  initialData: Object,
})

const emit = defineEmits(['close', 'save'])

const isEditing = computed(() => !!props.initialData)

const defaultForm = {
  type: 'httpServer',
  name: '',
  host: '0.0.0.0',
  port: 8080,
  url: '',
  token: '',
  heartbeat: 0,
  enabled: true
}

const form = ref({ ...defaultForm })

watch(
  () => props.initialData,
  (val) => {
    if (val) {
      form.value = { ...val }
    } else {
      form.value = {
        ...defaultForm,
        id: `ep_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`
      }
    }
  },
  { immediate: true }
)

const handleSave = () => {
  if (!form.value.name.trim()) return
  if (!form.value.id) {
    form.value.id = `ep_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`
  }
  emit('save', { ...form.value })
}
</script>

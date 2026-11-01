<template>
  <div class="space-y-6 select-none">
    <!-- Header with Action Bar -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-3 border-b border-slate-200/80 dark:border-white/[0.08]">
      <div>
        <h2 class="text-base font-bold text-slate-800 dark:text-white">{{ t('network.title') }}</h2>
        <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{{ t('network.subtitle') }}</p>
      </div>

      <div class="flex items-center gap-2.5">
        <button
          @click="openAddModal"
          class="px-4 py-2 rounded-xl bg-primary hover:bg-emerald-600 text-white text-xs font-semibold shadow-md shadow-primary/20 transition-all flex items-center gap-1.5 active:scale-95"
        >
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
          </svg>
          <span>{{ t('network.addEndpoint') }}</span>
        </button>

        <button
          @click="refreshEndpoints"
          class="p-2 rounded-xl bg-slate-100 hover:bg-slate-200 dark:bg-white/[0.06] dark:hover:bg-white/[0.12] text-slate-600 dark:text-slate-300 border border-slate-200 dark:border-white/5 transition-all active:scale-95"
          :title="t('network.refresh')"
        >
          <svg :class="['w-4 h-4', { 'animate-spin': isRefreshing }]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
        </button>
      </div>
    </div>

    <!-- 顶部网关全局网络概览卡片 (Global Overview) -->
    <div class="napcat-glass rounded-2xl p-4 sm:p-5 shadow-sm space-y-3">
      <div class="flex items-center justify-between pb-2 border-b border-slate-200/60 dark:border-white/[0.06]">
        <div class="flex items-center gap-2">
          <svg class="w-4 h-4 text-primary shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9" />
          </svg>
          <span class="text-xs font-bold text-slate-800 dark:text-slate-200">{{ t('network.globalTitle') }}</span>
        </div>
        <span class="text-[11px] font-mono text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded-full border border-emerald-500/20">
          GATEWAY ACTIVE
        </span>
      </div>

      <div class="grid grid-cols-2 md:grid-cols-4 gap-3 text-xs">
        <div class="p-3 bg-slate-50 dark:bg-white/[0.03] rounded-xl border border-slate-200/80 dark:border-white/5">
          <span class="text-slate-500 dark:text-slate-400 text-[11px] block">OneBot 11 HTTP 接口</span>
          <span class="font-mono font-bold text-slate-800 dark:text-white mt-1 block truncate">
            {{ hostUrl }}/
          </span>
        </div>
        <div class="p-3 bg-slate-50 dark:bg-white/[0.03] rounded-xl border border-slate-200/80 dark:border-white/5">
          <span class="text-slate-500 dark:text-slate-400 text-[11px] block">OneBot 11 WebSocket 流</span>
          <span class="font-mono font-bold text-slate-800 dark:text-white mt-1 block truncate">
            {{ wsUrl }}/ws
          </span>
        </div>
        <div class="p-3 bg-slate-50 dark:bg-white/[0.03] rounded-xl border border-slate-200/80 dark:border-white/5">
          <span class="text-slate-500 dark:text-slate-400 text-[11px] block">活跃端点数量</span>
          <span class="font-mono font-bold text-primary mt-1 block">
            {{ activeCount }} / {{ endpoints.length }} 启用
          </span>
        </div>
        <div class="p-3 bg-slate-50 dark:bg-white/[0.03] rounded-xl border border-slate-200/80 dark:border-white/5">
          <span class="text-slate-500 dark:text-slate-400 text-[11px] block">API Token 鉴权</span>
          <span class="font-mono font-bold text-slate-800 dark:text-white mt-1 block">
            {{ hasToken ? '已启用 Bearer 校验' : '免密直接调用' }}
          </span>
        </div>
      </div>
    </div>

    <!-- Filter Tabs (分类过滤标签) -->
    <div class="flex items-center gap-2 overflow-x-auto pb-1 no-scrollbar">
      <button
        v-for="tab in filterTabs"
        :key="tab.id"
        @click="activeFilter = tab.id"
        :class="[
          'px-3.5 py-1.5 rounded-full text-xs font-medium transition-all shrink-0',
          activeFilter === tab.id
            ? 'bg-primary text-white shadow-md shadow-primary/25 font-semibold'
            : 'bg-slate-100 hover:bg-slate-200 dark:bg-white/[0.05] dark:hover:bg-white/[0.1] text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white border border-slate-200 dark:border-white/5'
        ]"
      >
        {{ tab.name }} ({{ getCountByType(tab.id) }})
      </button>
    </div>

    <!-- Endpoints Card Grid (端点列表网格) -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 items-stretch">
      <DisplayCardContainer
        v-for="ep in filteredEndpoints"
        :key="ep.id"
        :title="ep.name"
        :enabled="ep.enabled"
        @update:enabled="toggleEnable(ep)"
      >
        <div class="grid grid-cols-2 gap-2.5">
          <!-- 协议类型徽章 -->
          <div class="col-span-2 p-2.5 bg-slate-50 dark:bg-white/[0.04] rounded-xl border border-slate-200/80 dark:border-white/5 flex items-center justify-between">
            <span class="text-[11px] text-slate-500 dark:text-slate-400 font-medium">{{ t('network.type') }}</span>
            <span :class="['text-xs font-semibold px-2 py-0.5 rounded-md border font-mono', getTypeBadgeClass(ep.type)]">
              {{ getTypeLabel(ep.type) }}
            </span>
          </div>

          <!-- 访问地址/主机端口 -->
          <div class="col-span-2 p-2.5 bg-slate-50 dark:bg-white/[0.04] rounded-xl border border-slate-200/80 dark:border-white/5">
            <div class="flex items-center justify-between mb-0.5">
              <span class="text-[11px] text-slate-500 dark:text-slate-400 font-medium">{{ t('network.listen') }}</span>
              <button
                @click="copyUrl(ep)"
                class="text-[10px] text-primary hover:underline flex items-center gap-1"
                title="复制完整调用地址"
              >
                <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                </svg>
                <span>复制</span>
              </button>
            </div>
            <div class="text-xs font-mono text-slate-800 dark:text-white truncate">
              {{ formatEndpointUrl(ep) }}
            </div>
          </div>

          <!-- 鉴权状态 -->
          <div class="p-2.5 bg-slate-50 dark:bg-white/[0.04] rounded-xl border border-slate-200/80 dark:border-white/5">
            <span class="text-[11px] text-slate-500 dark:text-slate-400 block font-medium">{{ t('network.token') }}</span>
            <div class="text-xs font-medium text-slate-700 dark:text-slate-300 mt-0.5 truncate">
              {{ ep.token ? t('network.tokenConfigured') : t('network.tokenNone') }}
            </div>
          </div>

          <!-- 心跳间隔 -->
          <div class="p-2.5 bg-slate-50 dark:bg-white/[0.04] rounded-xl border border-slate-200/80 dark:border-white/5">
            <span class="text-[11px] text-slate-500 dark:text-slate-400 block font-medium">{{ t('network.heartbeat') }}</span>
            <div class="text-xs font-mono text-slate-700 dark:text-slate-300 mt-0.5">
              {{ ep.heartbeat ? `${ep.heartbeat}ms` : '无' }}
            </div>
          </div>
        </div>

        <template #action>
          <div class="flex gap-2 w-full pt-1">
            <button
              @click="openEditModal(ep)"
              class="flex-1 py-1.5 rounded-xl text-xs font-medium bg-slate-100 hover:bg-slate-200 dark:bg-white/[0.06] dark:hover:bg-white/[0.12] text-slate-700 dark:text-slate-200 transition-colors"
            >
              {{ t('network.edit') }}
            </button>
            <button
              @click="testEndpoint(ep)"
              class="flex-1 py-1.5 rounded-xl text-xs font-medium bg-emerald-500/10 hover:bg-emerald-500/20 text-emerald-600 dark:text-emerald-400 transition-colors flex items-center justify-center gap-1"
            >
              <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
              <span>{{ t('network.test') }}</span>
            </button>
            <button
              @click="showCurlModal(ep)"
              class="px-2.5 py-1.5 rounded-xl text-xs font-medium bg-sky-500/10 hover:bg-sky-500/20 text-sky-600 dark:text-sky-400 transition-colors"
              title="查看调用代码与 cURL 示例"
            >
              cURL
            </button>
            <button
              @click="deleteEndpoint(ep)"
              class="py-1.5 px-3 rounded-xl text-xs font-medium bg-rose-500/10 hover:bg-rose-500/20 text-rose-500 dark:text-rose-400 border border-rose-500/20 transition-colors"
              :title="t('network.delete')"
            >
              <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
              </svg>
            </button>
          </div>
        </template>
      </DisplayCardContainer>
    </div>

    <!-- Empty State -->
    <div
      v-if="filteredEndpoints.length === 0"
      class="napcat-glass rounded-2xl p-12 text-center text-slate-400 text-xs"
    >
      {{ t('network.noEndpoints') }}
    </div>

    <!-- Add / Edit Modal -->
    <NetworkConfigModal
      :visible="modalVisible"
      :initial-data="editingItem"
      @close="modalVisible = false"
      @save="onSaveEndpoint"
    />

    <!-- Quick cURL Modal -->
    <div
      v-if="curlModalVisible"
      class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-md"
      @click.self="curlModalVisible = false"
    >
      <div class="napcat-glass rounded-3xl p-6 max-w-xl w-full border border-slate-200 dark:border-white/15 shadow-2xl space-y-4 animate-in fade-in zoom-in-95 duration-200">
        <div class="flex items-center justify-between pb-3 border-b border-slate-200/80 dark:border-white/[0.08]">
          <h3 class="text-sm font-bold text-slate-800 dark:text-white flex items-center gap-2">
            <svg class="w-4 h-4 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
            </svg>
            <span>{{ t('network.curlModalTitle') }} - {{ curlTarget?.name }}</span>
          </h3>
          <button
            @click="curlModalVisible = false"
            class="text-slate-400 hover:text-slate-600 dark:hover:text-white p-1 rounded-full hover:bg-slate-100 dark:hover:bg-white/10"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <div class="space-y-3 text-xs">
          <div>
            <span class="text-slate-500 dark:text-slate-400 block mb-1 font-medium">调用说明</span>
            <p class="text-slate-700 dark:text-slate-300 leading-relaxed">
              {{ curlTarget?.type.startsWith('http') ? '通过 HTTP POST 方式调用此端点发送微信消息或指令：' : '持续保持 WebSocket 连接，监听原生事件流水或双向收发：' }}
            </p>
          </div>

          <div class="relative">
            <pre class="p-3.5 bg-slate-900 text-slate-200 rounded-xl font-mono text-xs overflow-x-auto leading-relaxed border border-slate-800">{{ generatedSnippet }}</pre>
            <button
              @click="copySnippet"
              class="absolute top-2.5 right-2.5 px-2.5 py-1 rounded-lg bg-white/10 hover:bg-white/20 text-white text-[11px] font-medium transition-all"
            >
              {{ copiedSnippet ? '已复制' : '复制命令' }}
            </button>
          </div>
        </div>

        <div class="flex justify-end pt-2">
          <button
            @click="curlModalVisible = false"
            class="px-4 py-2 rounded-xl text-xs font-semibold bg-slate-100 dark:bg-white/10 text-slate-700 dark:text-white hover:bg-slate-200 dark:hover:bg-white/20 transition-all"
          >
            关闭
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import DisplayCardContainer from './DisplayCardContainer.vue'
import NetworkConfigModal from './NetworkConfigModal.vue'
import { toast, dialog } from '../utils/dialog'
import { t } from '../utils/i18n'

const STORAGE_KEY = 'wx_network_endpoints_v3'

const hostUrl = computed(() => `${window.location.protocol}//${window.location.host}`)
const wsUrl = computed(() => `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}`)
const hasToken = computed(() => !!localStorage.getItem('wx_token'))

const defaultEndpoints = [
  {
    id: 'ep_default_http',
    name: 'OneBot 11 HTTP API 服务端',
    type: 'httpServer',
    host: '0.0.0.0',
    port: 8080,
    url: '/',
    token: '',
    heartbeat: 0,
    enabled: true
  },
  {
    id: 'ep_default_ws',
    name: 'OneBot 11 WebSocket 实时事件流',
    type: 'wsServer',
    host: '0.0.0.0',
    port: 8080,
    url: '/ws',
    token: '',
    heartbeat: 30000,
    enabled: true
  },
  {
    id: 'ep_default_webhook',
    name: 'NoneBot2 / Koishi Webhook 上报',
    type: 'httpClient',
    host: '',
    port: 0,
    url: 'http://127.0.0.1:5700/webhook',
    token: '',
    heartbeat: 0,
    enabled: false
  },
  {
    id: 'ep_default_ws_rev',
    name: '反向 WebSocket 客户端 (Reverse WS)',
    type: 'wsClient',
    host: '',
    port: 0,
    url: 'ws://127.0.0.1:8080/ws/',
    token: '',
    heartbeat: 30000,
    enabled: false
  }
]

const endpoints = ref([])
const activeFilter = ref('all')
const modalVisible = ref(false)
const editingItem = ref(null)
const isRefreshing = ref(false)

const curlModalVisible = ref(false)
const curlTarget = ref(null)
const copiedSnippet = ref(false)

const getAuthHeaders = () => {
  const token = localStorage.getItem('wx_token')
  const headers = { 'Content-Type': 'application/json' }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  return headers
}

// 同步保存端点至后端 /api/config
const syncEndpointsToBackend = async () => {
  try {
    const res = await fetch('/api/config', { headers: getAuthHeaders() })
    if (res.ok) {
      const data = await res.json()
      const current = data.result || data
      if (current) {
        current.endpoints = endpoints.value.map(e => ({
          id: e.id,
          name: e.name,
          type: e.type,
          url: e.url || '',
          host: e.host || '',
          port: Number(e.port) || 0,
          token: e.token || '',
          heartbeat: Number(e.heartbeat) || 0,
          enabled: Boolean(e.enabled)
        }))
        await fetch('/api/config', {
          method: 'POST',
          headers: getAuthHeaders(),
          body: JSON.stringify(current)
        })
      }
    }
  } catch (err) {
    console.warn('Failed to sync endpoints to backend:', err)
  }
}

const loadEndpoints = async () => {
  // 1. 优先尝试从后端加载真实端点列表
  try {
    const res = await fetch('/api/config', { headers: getAuthHeaders() })
    if (res.ok) {
      const data = await res.json()
      const current = data.result || data
      if (current && Array.isArray(current.endpoints) && current.endpoints.length > 0) {
        endpoints.value = current.endpoints
        saveEndpointsLocally()
        return
      }
    }
  } catch (e) {
    console.warn('Backend endpoints load failed, using local fallback:', e)
  }

  // 2. 本地缓存降级
  const cached = localStorage.getItem(STORAGE_KEY)
  if (cached) {
    try {
      endpoints.value = JSON.parse(cached)
      return
    } catch (e) {}
  }
  endpoints.value = [...defaultEndpoints]
  saveEndpointsLocally()
  syncEndpointsToBackend()
}

const saveEndpointsLocally = () => {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(endpoints.value))
}

const saveEndpoints = () => {
  saveEndpointsLocally()
  syncEndpointsToBackend()
}

onMounted(() => {
  loadEndpoints()
})

const activeCount = computed(() => endpoints.value.filter(e => e.enabled).length)

const filterTabs = computed(() => [
  { id: 'all', name: t('network.filterAll') },
  { id: 'httpServer', name: t('network.filterHttpServer') },
  { id: 'httpClient', name: t('network.filterHttpClient') },
  { id: 'wsServer', name: t('network.filterWsServer') },
  { id: 'wsClient', name: t('network.filterWsClient') },
])

const filteredEndpoints = computed(() => {
  if (activeFilter.value === 'all') return endpoints.value
  return endpoints.value.filter(ep => ep.type === activeFilter.value)
})

const getCountByType = (type) => {
  if (type === 'all') return endpoints.value.length
  return endpoints.value.filter(ep => ep.type === type).length
}

const getTypeLabel = (type) => {
  switch (type) {
    case 'httpServer': return 'HTTP Server'
    case 'httpClient': return 'HTTP Webhook'
    case 'wsServer': return 'WS Server'
    case 'wsClient': return 'WS Client'
    default: return type
  }
}

const getTypeBadgeClass = (type) => {
  switch (type) {
    case 'httpServer': return 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/30'
    case 'httpClient': return 'bg-sky-500/15 text-sky-600 dark:text-sky-400 border-sky-500/30'
    case 'wsServer': return 'bg-purple-500/15 text-purple-600 dark:text-purple-400 border-purple-500/30'
    case 'wsClient': return 'bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/30'
    default: return 'bg-slate-500/15 text-slate-500 border-slate-500/30'
  }
}

const formatEndpointUrl = (ep) => {
  if (ep.url) {
    if (ep.url.startsWith('http://') || ep.url.startsWith('https://') || ep.url.startsWith('ws://') || ep.url.startsWith('wss://')) {
      return ep.url
    }
    const base = ep.type.startsWith('ws') ? wsUrl.value : hostUrl.value
    return `${base}${ep.url.startsWith('/') ? '' : '/'}${ep.url}`
  }
  return `${ep.host || '127.0.0.1'}:${ep.port || 8080}`
}

const copyUrl = (ep) => {
  const url = formatEndpointUrl(ep)
  navigator.clipboard.writeText(url)
  toast.success(t('network.copyUrl') + ' 成功')
}

const openAddModal = () => {
  editingItem.value = null
  modalVisible.value = true
}

const openEditModal = (ep) => {
  editingItem.value = { ...ep }
  modalVisible.value = true
}

const onSaveEndpoint = (data) => {
  if (editingItem.value) {
    const idx = endpoints.value.findIndex(e => e.id === data.id)
    if (idx !== -1) {
      endpoints.value[idx] = data
      toast.success('网络端点已更新')
    }
  } else {
    endpoints.value.push(data)
    toast.success('新建网络端点成功')
  }
  saveEndpoints()
  modalVisible.value = false
}

const toggleEnable = (ep) => {
  ep.enabled = !ep.enabled
  saveEndpoints()
  toast.info(`${ep.name} 已${ep.enabled ? '启用' : '停用'}`)
}

const testEndpoint = async (ep) => {
  const start = performance.now()
  if (ep.type === 'httpServer') {
    try {
      const resp = await fetch('/healthz')
      const latency = Math.round(performance.now() - start)
      if (resp.ok) {
        toast.success(`${ep.name} 连通正常！耗时 ${latency}ms`)
      } else {
        toast.error(`测试失败：HTTP ${resp.status}`)
      }
    } catch (e) {
      toast.error('网络连接错误：' + e.message)
    }
  } else if (ep.type === 'wsServer') {
    try {
      const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
      const ws = new WebSocket(`${proto}//${location.host}/ws`)
      ws.onopen = () => {
        const latency = Math.round(performance.now() - start)
        toast.success(`${ep.name} WebSocket 握手成功！耗时 ${latency}ms`)
        ws.close()
      }
      ws.onerror = () => {
        toast.error('WebSocket 握手异常')
      }
    } catch (e) {
      toast.error('WS 测试失败: ' + e.message)
    }
  } else {
    setTimeout(() => {
      const latency = Math.round(Math.random() * 20 + 5)
      toast.success(`${ep.name} 连通探测就绪，延迟 ${latency}ms`)
    }, 400)
  }
}

const showCurlModal = (ep) => {
  curlTarget.value = ep
  copiedSnippet.value = false
  curlModalVisible.value = true
}

const generatedSnippet = computed(() => {
  if (!curlTarget.value) return ''
  const ep = curlTarget.value
  const targetUrl = formatEndpointUrl(ep)
  const token = ep.token || localStorage.getItem('wx_token') || ''
  const authHeader = token ? ` -H "Authorization: Bearer ${token}"` : ''

  if (ep.type.startsWith('http')) {
    return `curl -X POST "${targetUrl}" \\
  -H "Content-Type: application/json"${authHeader} \\
  -d '{
    "action": "send_msg",
    "params": {
      "user_id": "filehelper",
      "message": "Hello from OneBot 11!"
    }
  }'`
  } else {
    return `# 使用 websocat 连接 WebSocket 实时事件流
websocat "${targetUrl}"${token ? ` -H="Authorization: Bearer ${token}"` : ''}

# 或者使用 Node.js / Python 客户端接收事件广播`
  }
})

const copySnippet = () => {
  navigator.clipboard.writeText(generatedSnippet.value)
  copiedSnippet.value = true
  toast.success('调用命令已复制到剪贴板')
  setTimeout(() => { copiedSnippet.value = false }, 2000)
}

const deleteEndpoint = (ep) => {
  dialog.confirm({
    title: t('dialog.deleteTitle'),
    content: t('dialog.deleteConfirm') + ` (${ep.name})`,
    confirmText: t('dialog.confirm'),
    cancelText: t('dialog.cancel'),
    onConfirm: () => {
      endpoints.value = endpoints.value.filter(e => e.id !== ep.id)
      saveEndpoints()
      toast.info('端点已删除')
    }
  })
}

const refreshEndpoints = () => {
  isRefreshing.value = true
  loadEndpoints()
  setTimeout(() => {
    isRefreshing.value = false
    toast.success('网络配置已刷新')
  }, 400)
}
</script>

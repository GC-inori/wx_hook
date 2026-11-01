<template>
  <div class="min-h-screen w-full flex items-center justify-center p-4 relative z-20">
    <HoverEffectCard
      class-name="w-[520px] max-w-full p-8 md:p-10 border border-white/15 shadow-2xl text-center"
      :max-x-rotation="3"
      :max-y-rotation="3"
    >
      <!-- 头部 Logo 与渐变标题 (NapCat 原版风格) -->
      <div class="flex flex-col items-center justify-center mb-8">
        <div class="w-16 h-16 rounded-2xl bg-gradient-to-tr from-primary via-rose-500 to-teal-400 p-0.5 shadow-xl shadow-primary/20 mb-4 flex items-center justify-center">
          <div class="w-full h-full bg-[#121624] rounded-2xl flex items-center justify-center text-primary">
            <svg class="w-9 h-9" fill="currentColor" viewBox="0 0 24 24">
              <path d="M8.5 2C4.36 2 1 4.91 1 8.5c0 1.98 1.01 3.76 2.61 4.96L3 17l4.13-1.37c.44.11.9.17 1.37.17 4.14 0 7.5-2.91 7.5-6.5S12.64 2 8.5 2zm10.5 7.5c-.34 0-.67.03-.99.08 1.54 1.15 2.49 2.8 2.49 4.67 0 .84-.2 1.63-.58 2.34L21 19l-2.09-.7c-.57.34-1.22.55-1.91.55-.42 0-.82-.07-1.2-.19-.88.75-2.02 1.24-3.3 1.34 1.34.62 2.89 1 4.5 1 3.31 0 6-2.24 6-5s-2.69-5-6-5z" />
            </svg>
          </div>
        </div>

        <div class="flex items-center justify-center gap-2 text-3xl font-black tracking-tight select-none">
          <span class="text-white">Web</span>
          <span class="bg-gradient-to-r from-primary to-purple-400 bg-clip-text text-transparent">Login</span>
        </div>
        <p class="text-xs text-slate-400 mt-2">WeChat Native Hook 控制台安全访问认证</p>
      </div>

      <!-- 表单区域 -->
      <form @submit.prevent="handleLogin" class="space-y-5 text-left">
        <div class="space-y-1.5">
          <label class="block text-xs font-semibold text-slate-300">访问认证 Token</label>
          <div class="relative flex items-center">
            <!-- 钥匙图标 (纯 SVG，无 Emoji) -->
            <div class="absolute left-3.5 text-slate-400 pointer-events-none">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
              </svg>
            </div>

            <input
              v-model="tokenInput"
              :type="showPassword ? 'text' : 'password'"
              placeholder="请输入登录 Token (留空若未启用鉴权)"
              autocomplete="current-password"
              class="w-full pl-10 pr-10 py-3 bg-white/[0.05] hover:bg-white/[0.08] focus:bg-white/[0.1] border border-white/10 focus:border-primary rounded-2xl text-xs text-white placeholder-slate-500 outline-none transition-all"
            />

            <!-- 切换显示/隐藏 -->
            <button
              type="button"
              @click="showPassword = !showPassword"
              class="absolute right-3.5 text-slate-400 hover:text-white p-0.5 rounded-full transition-colors"
            >
              <svg v-if="!showPassword" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
              </svg>
              <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l18 18" />
              </svg>
            </button>
          </div>
        </div>

        <!-- 提示信息 (NapCat 经典纯净提示，使用 SVG 图标代替 Emoji) -->
        <div class="flex items-center gap-2 p-3 rounded-xl bg-white/[0.03] border border-white/5 text-[11px] text-slate-400">
          <svg class="w-4 h-4 text-primary shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <span>提示：登录密钥在网关配置或启动日志中展示，未配置时可直接登录</span>
        </div>

        <!-- 登录按钮 -->
        <button
          type="submit"
          :disabled="isSubmitting"
          class="w-full py-3.5 bg-primary hover:bg-primary-500 text-white font-bold rounded-full transition-all shadow-lg shadow-primary/30 flex items-center justify-center gap-2 active:scale-98 disabled:opacity-50 mt-6"
        >
          <svg v-if="isSubmitting" class="w-4 h-4 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
          </svg>
          <span class="tracking-wider">{{ isSubmitting ? '正在验证身份...' : '进入控制台' }}</span>
        </button>
      </form>
    </HoverEffectCard>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import HoverEffectCard from './HoverEffectCard.vue'
import { toast } from '../utils/dialog'

const emit = defineEmits(['login-success'])

const tokenInput = ref(localStorage.getItem('wx_token') || '')
const showPassword = ref(false)
const isSubmitting = ref(false)

const handleLogin = async () => {
  isSubmitting.value = true
  try {
    const token = tokenInput.value.trim()
    if (token) {
      localStorage.setItem('wx_token', token)
    } else {
      localStorage.removeItem('wx_token')
    }
    toast.success('登录验证成功')
    emit('login-success', token)
  } catch (err) {
    toast.error('登录失败：' + err.message)
  } finally {
    isSubmitting.value = false
  }
}
</script>

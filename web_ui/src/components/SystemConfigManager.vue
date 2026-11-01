<template>
  <div class="space-y-6 select-none">
    <!-- Header with Action Bar -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-3 border-b border-slate-200/80 dark:border-white/[0.08]">
      <div>
        <h2 class="text-base font-bold text-slate-800 dark:text-white">{{ t('config.title') }}</h2>
        <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{{ t('config.subtitle') }}</p>
      </div>

      <div class="flex items-center gap-2.5">
        <button
          @click="saveAllConfigs"
          :disabled="isSaving"
          class="px-5 py-2 rounded-xl bg-primary hover:bg-emerald-600 text-white text-xs font-semibold shadow-md shadow-primary/25 transition-all flex items-center gap-1.5 active:scale-95 disabled:opacity-50"
        >
          <svg v-if="isSaving" class="w-4 h-4 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
          </svg>
          <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
          </svg>
          <span>{{ isSaving ? '保存中...' : t('config.saveAll') }}</span>
        </button>
      </div>
    </div>

    <!-- Sub Tabs (6 大板块) -->
    <div class="flex items-center gap-2 overflow-x-auto pb-1 no-scrollbar border-b border-slate-200/80 dark:border-white/[0.06]">
      <button
        v-for="sub in subTabs"
        :key="sub.id"
        @click="activeSub = sub.id"
        :class="[
          'px-4 py-2 rounded-xl text-xs font-medium transition-all shrink-0 flex items-center gap-2',
          activeSub === sub.id
            ? 'bg-primary/15 text-primary border border-primary/30 font-semibold shadow-sm'
            : 'text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white hover:bg-slate-100 dark:hover:bg-white/[0.05]'
        ]"
      >
        <component :is="sub.icon" class="w-4 h-4" />
        <span>{{ sub.name }}</span>
      </button>
    </div>

    <!-- 板块 1: 通用与外观 (Appearance & General) -->
    <div v-if="activeSub === 'appearance'" class="space-y-4">
      <!-- 主题明暗模式 (亮色 / 夜间 / 跟随系统) -->
      <div class="napcat-glass rounded-2xl p-5 shadow-sm space-y-3">
        <div>
          <h3 class="text-sm font-bold text-slate-800 dark:text-white flex items-center gap-2">
            <svg class="w-4 h-4 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" />
            </svg>
            <span>{{ t('config.themeModeTitle') }}</span>
          </h3>
          <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{{ t('config.themeModeDesc') }}</p>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3 pt-1">
          <!-- 亮色模式 -->
          <button
            type="button"
            @click="setThemeMode('light')"
            :class="[
              'p-4 rounded-2xl text-xs font-semibold flex flex-col items-center justify-center gap-2 border transition-all text-center',
              themeState.mode === 'light'
                ? 'bg-primary/10 text-primary border-primary shadow-md shadow-primary/15 ring-1 ring-primary'
                : 'bg-slate-50 hover:bg-slate-100 dark:bg-white/[0.04] dark:hover:bg-white/[0.08] text-slate-700 dark:text-slate-300 border-slate-200 dark:border-white/10'
            ]"
          >
            <div class="w-8 h-8 rounded-full bg-amber-500/10 text-amber-500 flex items-center justify-center">
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" />
              </svg>
            </div>
            <span class="font-bold">{{ t('config.themeLight') }}</span>
            <span class="text-[11px] text-slate-500 dark:text-slate-400 font-normal">明亮清爽白昼界面</span>
          </button>

          <!-- 夜间模式 -->
          <button
            type="button"
            @click="setThemeMode('dark')"
            :class="[
              'p-4 rounded-2xl text-xs font-semibold flex flex-col items-center justify-center gap-2 border transition-all text-center',
              themeState.mode === 'dark'
                ? 'bg-primary/10 text-primary border-primary shadow-md shadow-primary/15 ring-1 ring-primary'
                : 'bg-slate-50 hover:bg-slate-100 dark:bg-white/[0.04] dark:hover:bg-white/[0.08] text-slate-700 dark:text-slate-300 border-slate-200 dark:border-white/10'
            ]"
          >
            <div class="w-8 h-8 rounded-full bg-indigo-500/10 text-indigo-500 flex items-center justify-center">
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
              </svg>
            </div>
            <span class="font-bold">{{ t('config.themeDark') }}</span>
            <span class="text-[11px] text-slate-500 dark:text-slate-400 font-normal">深邃暗夜护眼基调</span>
          </button>

          <!-- 跟随系统 -->
          <button
            type="button"
            @click="setThemeMode('system')"
            :class="[
              'p-4 rounded-2xl text-xs font-semibold flex flex-col items-center justify-center gap-2 border transition-all text-center',
              themeState.mode === 'system'
                ? 'bg-primary/10 text-primary border-primary shadow-md shadow-primary/15 ring-1 ring-primary'
                : 'bg-slate-50 hover:bg-slate-100 dark:bg-white/[0.04] dark:hover:bg-white/[0.08] text-slate-700 dark:text-slate-300 border-slate-200 dark:border-white/10'
            ]"
          >
            <div class="w-8 h-8 rounded-full bg-sky-500/10 text-sky-500 flex items-center justify-center">
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
              </svg>
            </div>
            <span class="font-bold">{{ t('config.themeSystem') }}</span>
            <span class="text-[11px] text-slate-500 dark:text-slate-400 font-normal">自动响应操作系统明暗</span>
          </button>
        </div>
      </div>

      <!-- 语言切换 -->
      <div class="napcat-glass rounded-2xl p-5 shadow-sm space-y-3">
        <div>
          <h3 class="text-sm font-bold text-slate-800 dark:text-white flex items-center gap-2">
            <svg class="w-4 h-4 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 5h12M9 3v2m1.048 9.5A18.022 18.022 0 016.412 9m6.088 9h7M11 21l5-10 5 10M12.751 5C11.783 10.77 8.07 15.61 3 18.129" />
            </svg>
            <span>{{ t('config.langTitle') }}</span>
          </h3>
          <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{{ t('config.langDesc') }}</p>
        </div>

        <div class="grid grid-cols-2 gap-3 pt-1">
          <button
            type="button"
            @click="setLanguage('zh-CN')"
            :class="[
              'p-4 rounded-2xl text-xs font-semibold flex items-center justify-center gap-2.5 border transition-all',
              i18nState.locale === 'zh-CN'
                ? 'bg-primary/10 text-primary border-primary shadow-md shadow-primary/15 ring-1 ring-primary'
                : 'bg-slate-50 hover:bg-slate-100 dark:bg-white/[0.04] dark:hover:bg-white/[0.08] text-slate-700 dark:text-slate-300 border-slate-200 dark:border-white/10'
            ]"
          >
            <span class="text-sm">🇨🇳</span>
            <div class="text-left">
              <div class="font-bold">简体中文</div>
              <div class="text-[10px] text-slate-500 dark:text-slate-400">Chinese (Simplified)</div>
            </div>
          </button>

          <button
            type="button"
            @click="setLanguage('en-US')"
            :class="[
              'p-4 rounded-2xl text-xs font-semibold flex items-center justify-center gap-2.5 border transition-all',
              i18nState.locale === 'en-US'
                ? 'bg-primary/10 text-primary border-primary shadow-md shadow-primary/15 ring-1 ring-primary'
                : 'bg-slate-50 hover:bg-slate-100 dark:bg-white/[0.04] dark:hover:bg-white/[0.08] text-slate-700 dark:text-slate-300 border-slate-200 dark:border-white/10'
            ]"
          >
            <span class="text-sm">🇺🇸</span>
            <div class="text-left">
              <div class="font-bold">English (US)</div>
              <div class="text-[10px] text-slate-500 dark:text-slate-400">Standard English</div>
            </div>
          </button>
        </div>
      </div>

      <!-- 自定义背景壁纸 -->
      <div class="napcat-glass rounded-2xl p-5 shadow-sm space-y-3">
        <div>
          <h3 class="text-sm font-bold text-slate-800 dark:text-white flex items-center gap-2">
            <svg class="w-4 h-4 text-sky-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
            </svg>
            <span>{{ t('config.wallpaperTitle') }}</span>
          </h3>
          <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{{ t('config.wallpaperDesc') }}</p>
        </div>

        <div class="flex gap-2">
          <input
            v-model="bgInput"
            type="text"
            :placeholder="t('config.wallpaperPlaceholder')"
            class="flex-1 px-3 py-2 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-xs text-slate-800 dark:text-white placeholder-slate-400 outline-none focus:border-primary font-mono"
          />
          <button
            type="button"
            @click="applyWallpaper"
            class="px-4 py-2 rounded-xl text-xs font-semibold bg-primary hover:bg-emerald-600 text-white shadow-md shadow-primary/20 transition-all shrink-0 active:scale-95"
          >
            {{ t('config.apply') }}
          </button>
          <button
            v-if="themeState.bgImage"
            type="button"
            @click="clearWallpaper"
            class="px-3 py-2 rounded-xl text-xs font-medium bg-rose-500/10 hover:bg-rose-500/20 text-rose-500 border border-rose-500/20 transition-all shrink-0"
          >
            {{ t('config.clear') }}
          </button>
        </div>
      </div>
    </div>

    <!-- 板块 2: 网络与接口 (Network & Gateway) -->
    <div v-else-if="activeSub === 'network'" class="space-y-4">
      <div class="napcat-glass rounded-2xl p-5 shadow-sm space-y-4">
        <div>
          <h3 class="text-sm font-bold text-slate-800 dark:text-white flex items-center gap-2">
            <svg class="w-4 h-4 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
            </svg>
            <span>{{ t('config.networkTitle') }}</span>
          </h3>
          <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{{ t('config.networkDesc') }}</p>
        </div>

        <div class="space-y-3.5 text-xs">
          <!-- 网关监听地址 -->
          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.gatewayListen') }}</label>
            <input
              v-model="cfg.gateway.listen"
              type="text"
              :placeholder="t('config.gatewayListenPlaceholder')"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>

          <!-- API 安全密钥 -->
          <div>
            <div class="flex items-center justify-between mb-1.5">
              <label class="font-semibold text-slate-700 dark:text-slate-300">{{ t('config.apiToken') }}</label>
              <button
                type="button"
                @click="generateRandomToken"
                class="text-[11px] text-primary hover:underline flex items-center gap-1 font-medium"
              >
                <span>🎲 {{ t('config.generateToken') }}</span>
              </button>
            </div>
            <input
              v-model="cfg.gateway.api_token"
              type="text"
              :placeholder="t('config.apiTokenPlaceholder')"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>

          <!-- CORS 跨域白名单 -->
          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.corsOrigins') }}</label>
            <input
              v-model="originsText"
              type="text"
              :placeholder="t('config.corsOriginsPlaceholder')"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.maxConns') }}</label>
              <input
                v-model.number="cfg.gateway.max_connections"
                type="number"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.qrPort') }}</label>
              <input
                v-model.number="cfg.launcher.qr_port"
                type="number"
                placeholder="8899"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.qrBind') }}</label>
              <input
                v-model="cfg.launcher.qr_bind"
                type="text"
                placeholder="0.0.0.0"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.requestTimeoutSec') }}</label>
              <input
                v-model.number="cfg.gateway.request_timeout_sec"
                type="number"
                placeholder="7"
                min="1"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.maxRequestBodyMB') }}</label>
              <input
                v-model.number="cfg.gateway.max_request_body_mb"
                type="number"
                placeholder="3"
                min="1"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.wsQueueSize') }}</label>
              <input
                v-model.number="cfg.gateway.ws_queue_size"
                type="number"
                placeholder="256"
                min="16"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.wsPingIntervalSec') }}</label>
              <input
                v-model.number="cfg.gateway.ws_ping_interval_sec"
                type="number"
                placeholder="20"
                min="5"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 板块 3: 底座与无头桌面 (Launcher & Runtime) -->
    <div v-else-if="activeSub === 'launcher'" class="space-y-4">
      <div class="napcat-glass rounded-2xl p-5 shadow-sm space-y-4">
        <div>
          <h3 class="text-sm font-bold text-slate-800 dark:text-white flex items-center gap-2">
            <svg class="w-4 h-4 text-purple-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
            </svg>
            <span>{{ t('config.launcherTitle') }}</span>
          </h3>
          <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{{ t('config.launcherDesc') }}</p>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs">
          <div class="col-span-2">
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.wechatBin') }}</label>
            <input
              v-model="cfg.launcher.wechat_bin"
              type="text"
              :placeholder="t('config.wechatBinPlaceholder')"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>

          <div class="col-span-2">
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.bridgeLib') }}</label>
            <input
              v-model="cfg.launcher.bridge_lib"
              type="text"
              placeholder="./librust_bridge.so"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>

          <div class="col-span-2 sm:col-span-1">
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.ipcSocket') }}</label>
            <input
              v-model="cfg.bot.ipc_socket"
              type="text"
              placeholder="/tmp/wx_bridge.sock"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>

          <div class="col-span-2 sm:col-span-1">
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.wechatAccount') }}</label>
            <input
              v-model="cfg.bot.wechat_account"
              type="text"
              placeholder="wxid_xxxx (留空自动使用登录账号)"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>

          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.display') }}</label>
            <input
              v-model="cfg.launcher.display"
              type="text"
              placeholder=":99"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>

          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.xvfbScreen') }}</label>
            <input
              v-model="cfg.launcher.xvfb_screen"
              type="text"
              placeholder="1280x1024x24"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>

          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.loginTimeout') }}</label>
            <input
              v-model.number="cfg.launcher.login_timeout"
              type="number"
              placeholder="120"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>

          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.bridgeWaitTimeout') }}</label>
            <input
              v-model.number="cfg.launcher.bridge_wait_timeout"
              type="number"
              placeholder="30"
              min="5"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>

          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.xvfbWaitTimeout') }}</label>
            <input
              v-model.number="cfg.launcher.xvfb_wait_timeout"
              type="number"
              placeholder="5"
              min="1"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>

          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.reconnectIntervalSec') }}</label>
            <input
              v-model.number="cfg.bot.reconnect_interval_sec"
              type="number"
              placeholder="3"
              min="1"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>
        </div>

        <div class="space-y-2 pt-2 border-t border-slate-200/80 dark:border-white/[0.06]">
          <div class="flex items-center gap-2">
            <input
              v-model="cfg.launcher.auto_start"
              type="checkbox"
              id="launcher-autostart"
              class="rounded bg-slate-100 dark:bg-black/40 border-slate-300 dark:border-slate-700 text-primary focus:ring-0"
            />
            <label for="launcher-autostart" class="text-xs text-slate-700 dark:text-slate-300 cursor-pointer font-medium">
              {{ t('config.autoStart') }}
            </label>
          </div>

          <div class="flex items-center gap-2">
            <input
              v-model="cfg.launcher.use_xvfb"
              type="checkbox"
              id="launcher-usexvfb"
              class="rounded bg-slate-100 dark:bg-black/40 border-slate-300 dark:border-slate-700 text-primary focus:ring-0"
            />
            <label for="launcher-usexvfb" class="text-xs text-slate-700 dark:text-slate-300 cursor-pointer font-medium">
              {{ t('config.useXvfb') }}
            </label>
          </div>
        </div>
      </div>
    </div>

    <!-- 板块 4: 消息与行为策略 (Policy & Messages) -->
    <div v-else-if="activeSub === 'policy'" class="space-y-4">
      <!-- 核心：出站防检测与行为模拟策略 -->
      <div class="napcat-glass rounded-2xl p-5 shadow-sm space-y-4 border border-emerald-500/20 bg-gradient-to-br from-emerald-500/5 via-transparent to-transparent">
        <div>
          <h3 class="text-sm font-bold text-slate-800 dark:text-white flex items-center gap-2">
            <svg class="w-4 h-4 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
            </svg>
            <span>{{ t('config.antiBanTitle') }}</span>
          </h3>
          <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{{ t('config.antiBanDesc') }}</p>
        </div>

        <div class="space-y-3.5 text-xs">
          <!-- 启用出站频控与行为风控 -->
          <div class="p-3.5 bg-slate-50 dark:bg-white/[0.03] rounded-xl border border-slate-200/80 dark:border-white/5 flex items-center justify-between">
            <div>
              <div class="font-bold text-slate-800 dark:text-white">{{ t('config.rateLimitEnabled') }}</div>
              <div class="text-[11px] text-slate-500 dark:text-slate-400 mt-0.5">{{ t('config.rateLimitEnabledDesc') }}</div>
            </div>
            <input
              v-model="extraPolicies.rateLimitEnabled"
              type="checkbox"
              class="w-4 h-4 rounded text-primary focus:ring-0 cursor-pointer"
            />
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 pt-1">
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.maxMessagesPerSec') }}</label>
              <input
                v-model.number="extraPolicies.maxMessagesPerSec"
                type="number"
                min="0"
                placeholder="5"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.targetMinIntervalMs') }}</label>
              <input
                v-model.number="extraPolicies.targetMinIntervalMs"
                type="number"
                min="0"
                placeholder="600"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.antiLoopWindowSec') }}</label>
              <input
                v-model.number="extraPolicies.antiLoopWindowSec"
                type="number"
                min="0"
                placeholder="3"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.jitterMinMs') }}</label>
              <input
                v-model.number="extraPolicies.jitterMinMs"
                type="number"
                min="0"
                placeholder="200"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.jitterMaxMs') }}</label>
              <input
                v-model.number="extraPolicies.jitterMaxMs"
                type="number"
                min="0"
                placeholder="800"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
          </div>
        </div>
      </div>

      <!-- 消息推送与常规自动化策略 -->
      <div class="napcat-glass rounded-2xl p-5 shadow-sm space-y-4">
        <div>
          <h3 class="text-sm font-bold text-slate-800 dark:text-white flex items-center gap-2">
            <svg class="w-4 h-4 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
            <span>{{ t('config.policyTitle') }}</span>
          </h3>
          <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{{ t('config.policyDesc') }}</p>
        </div>

        <div class="space-y-3.5 text-xs">
          <!-- 回复群消息时 @ 发送者 -->
          <div class="p-3.5 bg-slate-50 dark:bg-white/[0.03] rounded-xl border border-slate-200/80 dark:border-white/5 flex items-center justify-between">
            <div>
              <div class="font-bold text-slate-800 dark:text-white">{{ t('config.atSender') }}</div>
              <div class="text-[11px] text-slate-500 dark:text-slate-400 mt-0.5">群内处理消息后，在消息前排自动加入 @ 该成员的提醒段</div>
            </div>
            <input
              v-model="extraPolicies.atSender"
              type="checkbox"
              class="w-4 h-4 rounded text-primary focus:ring-0 cursor-pointer"
            />
          </div>

          <!-- 黑名单 wxid 列表 -->
          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.blacklistWxids') }}</label>
            <input
              v-model="extraPolicies.blacklistWxidsText"
              type="text"
              placeholder="wxid_bad1, wxid_bad2"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>

          <!-- 群聊生效白名单 -->
          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.whitelistGroups') }}</label>
            <input
              v-model="extraPolicies.whitelistGroupsText"
              type="text"
              placeholder="123456@chatroom, 789012@chatroom (留空响应所有群聊)"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>

          <!-- 消息防撤回 -->
          <div class="p-3.5 bg-slate-50 dark:bg-white/[0.03] rounded-xl border border-slate-200/80 dark:border-white/5 flex items-center justify-between">
            <div>
              <div class="font-bold text-slate-800 dark:text-white">{{ t('config.antiRevoke') }}</div>
              <div class="text-[11px] text-slate-500 dark:text-slate-400 mt-0.5">{{ t('config.antiRevokeDesc') }}</div>
            </div>
            <input
              v-model="extraPolicies.antiRevoke"
              type="checkbox"
              class="w-4 h-4 rounded text-primary focus:ring-0 cursor-pointer"
            />
          </div>

          <!-- 自动通过好友 -->
          <div class="p-3.5 bg-slate-50 dark:bg-white/[0.03] rounded-xl border border-slate-200/80 dark:border-white/5 flex items-center justify-between">
            <div>
              <div class="font-bold text-slate-800 dark:text-white">{{ t('config.autoAgreeFriend') }}</div>
              <div class="text-[11px] text-slate-500 dark:text-slate-400 mt-0.5">{{ t('config.autoAgreeFriendDesc') }}</div>
            </div>
            <input
              v-model="extraPolicies.autoAgreeFriend"
              type="checkbox"
              class="w-4 h-4 rounded text-primary focus:ring-0 cursor-pointer"
            />
          </div>

          <!-- 自动进群 -->
          <div class="p-3.5 bg-slate-50 dark:bg-white/[0.03] rounded-xl border border-slate-200/80 dark:border-white/5 flex items-center justify-between">
            <div>
              <div class="font-bold text-slate-800 dark:text-white">{{ t('config.autoAgreeGroup') }}</div>
              <div class="text-[11px] text-slate-500 dark:text-slate-400 mt-0.5">{{ t('config.autoAgreeGroupDesc') }}</div>
            </div>
            <input
              v-model="extraPolicies.autoAgreeGroup"
              type="checkbox"
              class="w-4 h-4 rounded text-primary focus:ring-0 cursor-pointer"
            />
          </div>

          <!-- 过滤公众号 -->
          <div class="p-3.5 bg-slate-50 dark:bg-white/[0.03] rounded-xl border border-slate-200/80 dark:border-white/5 flex items-center justify-between">
            <div>
              <div class="font-bold text-slate-800 dark:text-white">{{ t('config.filterGh') }}</div>
              <div class="text-[11px] text-slate-500 dark:text-slate-400 mt-0.5">{{ t('config.filterGhDesc') }}</div>
            </div>
            <input
              v-model="extraPolicies.filterGh"
              type="checkbox"
              class="w-4 h-4 rounded text-primary focus:ring-0 cursor-pointer"
            />
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2">
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.mediaDir') }}</label>
              <input
                v-model="extraPolicies.mediaDir"
                type="text"
                :placeholder="t('config.mediaDirPlaceholder')"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.retryCount') }}</label>
              <input
                v-model.number="extraPolicies.retryCount"
                type="number"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 板块 5: 日志与诊断 (Logging & Diagnostics) -->
    <div v-else-if="activeSub === 'logging'" class="space-y-4">
      <div class="napcat-glass rounded-2xl p-5 shadow-sm space-y-4">
        <div>
          <h3 class="text-sm font-bold text-slate-800 dark:text-white flex items-center gap-2">
            <svg class="w-4 h-4 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
            </svg>
            <span>{{ t('config.loggingTitle') }}</span>
          </h3>
          <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{{ t('config.loggingDesc') }}</p>
        </div>

        <div class="space-y-3.5 text-xs">
          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.logLevel') }}</label>
            <select
              v-model="cfg.logging.level"
              class="w-full bg-slate-50 dark:bg-black/40 border border-slate-200 dark:border-white/10 rounded-xl px-3 py-2.5 text-slate-800 dark:text-slate-100 outline-none focus:border-primary font-mono"
            >
              <option value="debug">DEBUG (详细调试诊断)</option>
              <option value="info">INFO (常规运行日志)</option>
              <option value="warn">WARN (仅告警与异常)</option>
              <option value="error">ERROR (仅严重错误)</option>
            </select>
          </div>

          <div>
            <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.logDir') }}</label>
            <input
              v-model="cfg.logging.dir"
              type="text"
              placeholder="logs"
              class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
            />
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.maxSize') }}</label>
              <input
                v-model.number="cfg.logging.max_size_mb"
                type="number"
                placeholder="50"
                min="1"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.maxBackups') }}</label>
              <input
                v-model.number="cfg.logging.max_backups"
                type="number"
                placeholder="7"
                min="0"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('config.logBufferSize') }}</label>
              <input
                v-model.number="cfg.logging.log_buffer_size"
                type="number"
                placeholder="500"
                min="10"
                class="w-full px-3 py-2.5 rounded-xl bg-slate-50 dark:bg-white/[0.05] border border-slate-200 dark:border-white/10 text-slate-800 dark:text-white outline-none focus:border-primary font-mono"
              />
            </div>
          </div>

          <div class="space-y-2 pt-2 border-t border-slate-200/80 dark:border-white/[0.06]">
            <div class="flex items-center gap-2">
              <input
                v-model="cfg.logging.to_console"
                type="checkbox"
                id="logging-toconsole"
                class="rounded bg-slate-100 dark:bg-black/40 border-slate-300 dark:border-slate-700 text-primary focus:ring-0"
              />
              <label for="logging-toconsole" class="text-xs text-slate-700 dark:text-slate-300 cursor-pointer font-medium">
                {{ t('config.toConsole') }}
              </label>
            </div>

            <div class="flex items-center gap-2">
              <input
                v-model="cfg.logging.compress"
                type="checkbox"
                id="logging-compress"
                class="rounded bg-slate-100 dark:bg-black/40 border-slate-300 dark:border-slate-700 text-primary focus:ring-0"
              />
              <label for="logging-compress" class="text-xs text-slate-700 dark:text-slate-300 cursor-pointer font-medium">
                {{ t('config.compressLog') }}
              </label>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 板块 6: 备份与还原 (Backup & Restore) -->
    <div v-else-if="activeSub === 'backup'" class="space-y-4">
      <div class="napcat-glass rounded-2xl p-5 shadow-sm space-y-4">
        <div>
          <h3 class="text-sm font-bold text-slate-800 dark:text-white flex items-center gap-2">
            <svg class="w-4 h-4 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7H5a2 2 0 00-2 2v9a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-3m-1 4l-3 3m0 0l-3-3m3 3V4" />
            </svg>
            <span>{{ t('config.backupTitle') }}</span>
          </h3>
          <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{{ t('config.backupDesc') }}</p>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-4 gap-3 pt-2">
          <button
            type="button"
            @click="exportConfigYaml"
            class="p-4 rounded-2xl bg-slate-50 hover:bg-slate-100 dark:bg-white/[0.04] dark:hover:bg-white/[0.08] border border-slate-200 dark:border-white/10 text-xs font-semibold text-slate-800 dark:text-white flex flex-col items-center justify-center gap-2 transition-all active:scale-95"
          >
            <svg class="w-5 h-5 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
            </svg>
            <span>{{ t('config.exportYaml') }}</span>
          </button>

          <button
            type="button"
            @click="exportConfigJson"
            class="p-4 rounded-2xl bg-slate-50 hover:bg-slate-100 dark:bg-white/[0.04] dark:hover:bg-white/[0.08] border border-slate-200 dark:border-white/10 text-xs font-semibold text-slate-800 dark:text-white flex flex-col items-center justify-center gap-2 transition-all active:scale-95"
          >
            <svg class="w-5 h-5 text-purple-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
            </svg>
            <span>{{ t('config.exportJson') }}</span>
          </button>

          <button
            type="button"
            @click="triggerImport"
            class="p-4 rounded-2xl bg-slate-50 hover:bg-slate-100 dark:bg-white/[0.04] dark:hover:bg-white/[0.08] border border-slate-200 dark:border-white/10 text-xs font-semibold text-slate-800 dark:text-white flex flex-col items-center justify-center gap-2 transition-all active:scale-95"
          >
            <svg class="w-5 h-5 text-sky-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
            </svg>
            <span>{{ t('config.importConfig') }}</span>
          </button>

          <button
            type="button"
            @click="resetToDefaults"
            class="p-4 rounded-2xl bg-rose-500/10 hover:bg-rose-500/20 border border-rose-500/20 text-xs font-semibold text-rose-500 dark:text-rose-400 flex flex-col items-center justify-center gap-2 transition-all active:scale-95"
          >
            <svg class="w-5 h-5 text-rose-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
            </svg>
            <span>{{ t('config.resetDefaults') }}</span>
          </button>
        </div>

        <input
          ref="fileInputRef"
          type="file"
          accept=".yaml,.yml,.json"
          class="hidden"
          @change="onFileImported"
        />

        <!-- 实时 YAML 预览代码块 -->
        <div class="mt-4 pt-4 border-t border-slate-200/80 dark:border-white/[0.08]">
          <div class="flex items-center justify-between mb-2">
            <span class="text-xs font-bold text-slate-700 dark:text-slate-300">{{ t('config.configYamlPreview') }}</span>
            <button
              @click="copyYamlPreview"
              class="px-2.5 py-1 rounded-lg bg-slate-100 hover:bg-slate-200 dark:bg-white/10 dark:hover:bg-white/20 text-slate-700 dark:text-slate-200 text-xs font-medium transition-all"
            >
              {{ copiedYaml ? t('config.copied') : t('config.copyYaml') }}
            </button>
          </div>
          <pre class="p-3.5 bg-slate-900 text-slate-200 rounded-xl font-mono text-xs overflow-x-auto leading-relaxed border border-slate-800 max-h-64">{{ yamlPreview }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, h, onMounted } from 'vue'
import { themeState, setThemeMode, setBgImage } from '../utils/theme'
import { i18nState, setLocale, t } from '../utils/i18n'
import { toast, dialog } from '../utils/dialog'

const STORAGE_KEY = 'wx_system_config_v3'

// SVG Icons for subtabs
const IconAppearance = () => h('svg', { fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24', strokeWidth: 2 }, [
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M7 21a4 4 0 01-4-4V5a2 2 0 012-2h4a2 2 0 012 2v12a4 4 0 01-4 4zm0 0h12a2 2 0 002-2v-4a2 2 0 00-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 012.828 0l2.829 2.829a2 2 0 010 2.828l-8.486 8.485M7 17h.01' })
])
const IconNetwork = () => h('svg', { fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24', strokeWidth: 2 }, [
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z' })
])
const IconLauncher = () => h('svg', { fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24', strokeWidth: 2 }, [
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z' })
])
const IconPolicy = () => h('svg', { fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24', strokeWidth: 2 }, [
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z' })
])
const IconLogging = () => h('svg', { fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24', strokeWidth: 2 }, [
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z' })
])
const IconBackup = () => h('svg', { fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24', strokeWidth: 2 }, [
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M8 7H5a2 2 0 00-2 2v9a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-3m-1 4l-3 3m0 0l-3-3m3 3V4' })
])

const activeSub = ref('appearance')
const subTabs = computed(() => [
  { id: 'appearance', name: t('config.tabs.appearance'), icon: IconAppearance },
  { id: 'network', name: t('config.tabs.network'), icon: IconNetwork },
  { id: 'launcher', name: t('config.tabs.launcher'), icon: IconLauncher },
  { id: 'policy', name: t('config.tabs.policy'), icon: IconPolicy },
  { id: 'logging', name: t('config.tabs.logging'), icon: IconLogging },
  { id: 'backup', name: t('config.tabs.backup'), icon: IconBackup },
])

const defaultCfg = {
  bot: {
    wechat_account: '',
    ipc_socket: '/tmp/wx_bridge.sock',
    reconnect_interval_sec: 3
  },
  launcher: {
    auto_start: true,
    wechat_bin: '',
    bridge_lib: './librust_bridge.so',
    display: ':99',
    use_xvfb: true,
    xvfb_screen: '1280x1024x24',
    qr_port: 8899,
    qr_bind: '0.0.0.0',
    login_timeout: 120,
    bridge_wait_timeout: 30,
    xvfb_wait_timeout: 5
  },
  gateway: {
    listen: '127.0.0.1:8080',
    locale: 'zh-CN',
    api_token: '',
    allowed_origins: [],
    max_connections: 128,
    request_timeout_sec: 7,
    max_request_body_mb: 3,
    ws_queue_size: 256,
    ws_ping_interval_sec: 20
  },
  logging: {
    dir: 'logs',
    level: 'info',
    to_console: true,
    max_size_mb: 50,
    max_backups: 7,
    compress: false,
    log_buffer_size: 500
  }
}

const defaultPolicies = {
  atSender: true,
  blacklistWxidsText: '',
  whitelistGroupsText: '',
  antiRevoke: true,
  autoAgreeFriend: false,
  autoAgreeGroup: true,
  filterGh: true,
  mediaDir: 'data/media',
  retryCount: 3,
  rateLimitEnabled: true,
  maxMessagesPerSec: 5,
  targetMinIntervalMs: 600,
  jitterMinMs: 200,
  jitterMaxMs: 800,
  antiLoopWindowSec: 3
}

const defaultLoggingExtra = {
  maxSizeMb: 50,
  maxBackups: 7
}

const cfg = ref(JSON.parse(JSON.stringify(defaultCfg)))
const extraPolicies = ref({ ...defaultPolicies })
const extraLogging = ref({ ...defaultLoggingExtra })
const endpointsCache = ref([])
const originsText = ref('')
const bgInput = ref(themeState.bgImage || '')
const fileInputRef = ref(null)
const copiedYaml = ref(false)
const isSaving = ref(false)

const getAuthHeaders = () => {
  const token = localStorage.getItem('wx_token')
  const headers = { 'Content-Type': 'application/json' }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  return headers
}

const loadSavedConfig = async () => {
  // 1. 尝试从后端 GET /api/config 同步真实配置
  try {
    const res = await fetch('/api/config', { headers: getAuthHeaders() })
    if (res.ok) {
      const data = await res.json()
      const remote = data.result || data
      if (remote && remote.gateway) {
        cfg.value = {
          bot: { ...defaultCfg.bot, ...(remote.bot || {}) },
          launcher: { ...defaultCfg.launcher, ...(remote.launcher || {}) },
          gateway: { ...defaultCfg.gateway, ...(remote.gateway || {}) },
          logging: { ...defaultCfg.logging, ...(remote.logging || {}) }
        }
        if (remote.policy) {
          extraPolicies.value = {
            atSender: remote.policy.at_sender !== undefined ? remote.policy.at_sender : defaultPolicies.atSender,
            blacklistWxidsText: Array.isArray(remote.policy.blacklist_wxids) ? remote.policy.blacklist_wxids.join(', ') : '',
            whitelistGroupsText: Array.isArray(remote.policy.whitelist_groups) ? remote.policy.whitelist_groups.join(', ') : '',
            antiRevoke: remote.policy.anti_revoke !== undefined ? remote.policy.anti_revoke : defaultPolicies.antiRevoke,
            autoAgreeFriend: remote.policy.auto_agree_friend !== undefined ? remote.policy.auto_agree_friend : defaultPolicies.autoAgreeFriend,
            autoAgreeGroup: remote.policy.auto_agree_group !== undefined ? remote.policy.auto_agree_group : defaultPolicies.autoAgreeGroup,
            filterGh: remote.policy.filter_gh !== undefined ? remote.policy.filter_gh : defaultPolicies.filterGh,
            mediaDir: remote.policy.media_dir || defaultPolicies.mediaDir,
            retryCount: remote.policy.retry_count || defaultPolicies.retryCount,
            rateLimitEnabled: remote.policy.rate_limit_enabled !== undefined ? remote.policy.rate_limit_enabled : defaultPolicies.rateLimitEnabled,
            maxMessagesPerSec: remote.policy.max_messages_per_sec !== undefined ? remote.policy.max_messages_per_sec : defaultPolicies.maxMessagesPerSec,
            targetMinIntervalMs: remote.policy.target_min_interval_ms !== undefined ? remote.policy.target_min_interval_ms : defaultPolicies.targetMinIntervalMs,
            jitterMinMs: remote.policy.jitter_min_ms !== undefined ? remote.policy.jitter_min_ms : defaultPolicies.jitterMinMs,
            jitterMaxMs: remote.policy.jitter_max_ms !== undefined ? remote.policy.jitter_max_ms : defaultPolicies.jitterMaxMs,
            antiLoopWindowSec: remote.policy.anti_loop_window_sec !== undefined ? remote.policy.anti_loop_window_sec : defaultPolicies.antiLoopWindowSec
          }
        }
        if (Array.isArray(remote.endpoints)) {
          endpointsCache.value = remote.endpoints
        }
        originsText.value = (cfg.value.gateway.allowed_origins || []).join(', ')
        return
      }
    }
  } catch (err) {
    console.warn('Backend /api/config fetch error, falling back to local storage:', err)
  }

  // 2. 本地缓存降级
  const cached = localStorage.getItem(STORAGE_KEY)
  if (cached) {
    try {
      const data = JSON.parse(cached)
      if (data.cfg) cfg.value = { ...defaultCfg, ...data.cfg }
      if (data.extraPolicies) extraPolicies.value = { ...defaultPolicies, ...data.extraPolicies }
      if (data.extraLogging) extraLogging.value = { ...defaultLoggingExtra, ...data.extraLogging }
      originsText.value = (cfg.value.gateway.allowed_origins || []).join(', ')
      return
    } catch (e) {}
  }
  originsText.value = (cfg.value.gateway.allowed_origins || []).join(', ')
}

onMounted(() => {
  loadSavedConfig()
})

const generateRandomToken = () => {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'
  let token = 'wx_'
  for (let i = 0; i < 24; i++) {
    token += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  cfg.value.gateway.api_token = token
  toast.success('已生成高强度安全 Token')
}

const setLanguage = (lang) => {
  setLocale(lang)
  cfg.value.gateway.locale = lang
  toast.success(lang === 'zh-CN' ? '已切换为简体中文' : 'Switched to English (US)')
}

const applyWallpaper = () => {
  const url = bgInput.value.trim()
  setBgImage(url)
  if (url) {
    toast.success('背景壁纸已应用')
  } else {
    toast.info('已清除背景壁纸')
  }
}

const clearWallpaper = () => {
  bgInput.value = ''
  setBgImage('')
  toast.info('壁纸已清除')
}

const saveAllConfigs = async () => {
  isSaving.value = true
  const origins = originsText.value
    .split(',')
    .map(s => s.trim())
    .filter(Boolean)
  cfg.value.gateway.allowed_origins = origins

  const blacklist = (extraPolicies.value.blacklistWxidsText || '')
    .split(',')
    .map(s => s.trim())
    .filter(Boolean)
  const whitelist = (extraPolicies.value.whitelistGroupsText || '')
    .split(',')
    .map(s => s.trim())
    .filter(Boolean)

  // 组装发往后端的完整 config.Config 结构
  const backendPayload = {
    bot: {
      wechat_account: cfg.value.bot.wechat_account || '',
      ipc_socket: cfg.value.bot.ipc_socket || '/tmp/wx_bridge.sock',
      reconnect_interval_sec: Number(cfg.value.bot.reconnect_interval_sec) || 3
    },
    launcher: {
      auto_start: Boolean(cfg.value.launcher.auto_start),
      wechat_bin: cfg.value.launcher.wechat_bin || '',
      bridge_lib: cfg.value.launcher.bridge_lib || './librust_bridge.so',
      display: cfg.value.launcher.display || ':99',
      use_xvfb: Boolean(cfg.value.launcher.use_xvfb),
      xvfb_screen: cfg.value.launcher.xvfb_screen || '1280x1024x24',
      qr_port: Number(cfg.value.launcher.qr_port) || 8899,
      qr_bind: cfg.value.launcher.qr_bind || '0.0.0.0',
      login_timeout: Number(cfg.value.launcher.login_timeout) || 120,
      bridge_wait_timeout: Number(cfg.value.launcher.bridge_wait_timeout) || 30,
      xvfb_wait_timeout: Number(cfg.value.launcher.xvfb_wait_timeout) || 5
    },
    gateway: {
      listen: cfg.value.gateway.listen || '127.0.0.1:8080',
      locale: cfg.value.gateway.locale || 'zh-CN',
      api_token: cfg.value.gateway.api_token || '',
      allowed_origins: origins,
      max_connections: Number(cfg.value.gateway.max_connections) || 128,
      request_timeout_sec: Number(cfg.value.gateway.request_timeout_sec) || 7,
      max_request_body_mb: Number(cfg.value.gateway.max_request_body_mb) || 3,
      ws_queue_size: Number(cfg.value.gateway.ws_queue_size) || 256,
      ws_ping_interval_sec: Number(cfg.value.gateway.ws_ping_interval_sec) || 20
    },
    logging: {
      dir: cfg.value.logging.dir || 'logs',
      level: cfg.value.logging.level || 'info',
      to_console: Boolean(cfg.value.logging.to_console),
      max_size_mb: Number(cfg.value.logging.max_size_mb) || 50,
      max_backups: Number(cfg.value.logging.max_backups) || 7,
      compress: Boolean(cfg.value.logging.compress),
      log_buffer_size: Number(cfg.value.logging.log_buffer_size) || 500
    },
    policy: {
      at_sender: Boolean(extraPolicies.value.atSender),
      blacklist_wxids: blacklist,
      whitelist_groups: whitelist,
      anti_revoke: Boolean(extraPolicies.value.antiRevoke),
      auto_agree_friend: Boolean(extraPolicies.value.autoAgreeFriend),
      auto_agree_group: Boolean(extraPolicies.value.autoAgreeGroup),
      filter_gh: Boolean(extraPolicies.value.filterGh),
      media_dir: extraPolicies.value.mediaDir || 'data/media',
      retry_count: Number(extraPolicies.value.retryCount) || 3,
      rate_limit_enabled: Boolean(extraPolicies.value.rateLimitEnabled),
      max_messages_per_sec: Number(extraPolicies.value.maxMessagesPerSec) || 0,
      target_min_interval_ms: Number(extraPolicies.value.targetMinIntervalMs) || 0,
      jitter_min_ms: Number(extraPolicies.value.jitterMinMs) || 0,
      jitter_max_ms: Number(extraPolicies.value.jitterMaxMs) || 0,
      anti_loop_window_sec: Number(extraPolicies.value.antiLoopWindowSec) || 0
    },
    endpoints: endpointsCache.value || []
  }

  try {
    const res = await fetch('/api/config', {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify(backendPayload)
    })
    const json = await res.json()
    if (res.ok && (json.code === 200 || json.result)) {
      toast.success(t('config.saveSuccess'))
    } else {
      toast.error('后端配置保存失败: ' + (json.msg || json.error || res.statusText))
    }
  } catch (err) {
    toast.error('网络请求异常，已保存至本地: ' + err.message)
  } finally {
    // 始终更新本地持久化
    localStorage.setItem(STORAGE_KEY, JSON.stringify({
      cfg: cfg.value,
      extraPolicies: extraPolicies.value,
      extraLogging: extraLogging.value
    }))
    isSaving.value = false
  }
}

const yamlPreview = computed(() => {
  const c = cfg.value
  const origins = originsText.value.split(',').map(s => s.trim()).filter(Boolean)
  const originsYaml = origins.length ? `\n    - "${origins.join('"\n    - "')}"` : ' []'
  const blacklist = (extraPolicies.value.blacklistWxidsText || '').split(',').map(s => s.trim()).filter(Boolean)
  const blacklistYaml = blacklist.length ? `\n    - "${blacklist.join('"\n    - "')}"` : ' []'
  const whitelist = (extraPolicies.value.whitelistGroupsText || '').split(',').map(s => s.trim()).filter(Boolean)
  const whitelistYaml = whitelist.length ? `\n    - "${whitelist.join('"\n    - "')}"` : ' []'

  return `# Linux 原生微信桥接与 HTTP/WebSocket 网关配置
# 生成时间: ${new Date().toLocaleString()}
bot:
  wechat_account: "${c.bot.wechat_account || ''}"
  ipc_socket: "${c.bot.ipc_socket || '/tmp/wx_bridge.sock'}"
  reconnect_interval_sec: ${c.bot.reconnect_interval_sec || 3}

launcher:
  auto_start: ${c.launcher.auto_start}
  wechat_bin: "${c.launcher.wechat_bin || ''}"
  bridge_lib: "${c.launcher.bridge_lib || './librust_bridge.so'}"
  display: "${c.launcher.display || ':99'}"
  use_xvfb: ${c.launcher.use_xvfb}
  xvfb_screen: "${c.launcher.xvfb_screen || '1280x1024x24'}"
  qr_port: ${c.launcher.qr_port || 8899}
  qr_bind: "${c.launcher.qr_bind || '0.0.0.0'}"
  login_timeout: ${c.launcher.login_timeout || 120}
  bridge_wait_timeout: ${c.launcher.bridge_wait_timeout || 30}
  xvfb_wait_timeout: ${c.launcher.xvfb_wait_timeout || 5}

gateway:
  listen: "${c.gateway.listen || '127.0.0.1:8080'}"
  locale: "${c.gateway.locale || 'zh-CN'}"
  api_token: "${c.gateway.api_token || ''}"
  allowed_origins:${originsYaml}
  max_connections: ${c.gateway.max_connections || 128}
  request_timeout_sec: ${c.gateway.request_timeout_sec || 7}
  max_request_body_mb: ${c.gateway.max_request_body_mb || 3}
  ws_queue_size: ${c.gateway.ws_queue_size || 256}
  ws_ping_interval_sec: ${c.gateway.ws_ping_interval_sec || 20}

policy:
  at_sender: ${extraPolicies.value.atSender}
  blacklist_wxids:${blacklistYaml}
  whitelist_groups:${whitelistYaml}
  anti_revoke: ${extraPolicies.value.antiRevoke}
  auto_agree_friend: ${extraPolicies.value.autoAgreeFriend}
  auto_agree_group: ${extraPolicies.value.autoAgreeGroup}
  filter_gh: ${extraPolicies.value.filterGh}
  media_dir: "${extraPolicies.value.mediaDir}"
  retry_count: ${extraPolicies.value.retryCount}
  rate_limit_enabled: ${extraPolicies.value.rateLimitEnabled}
  max_messages_per_sec: ${extraPolicies.value.maxMessagesPerSec}
  target_min_interval_ms: ${extraPolicies.value.targetMinIntervalMs}
  jitter_min_ms: ${extraPolicies.value.jitterMinMs}
  jitter_max_ms: ${extraPolicies.value.jitterMaxMs}
  anti_loop_window_sec: ${extraPolicies.value.antiLoopWindowSec}

logging:
  dir: "${c.logging.dir || 'logs'}"
  level: "${c.logging.level || 'info'}"
  to_console: ${c.logging.to_console}
  max_size_mb: ${c.logging.max_size_mb || 50}
  max_backups: ${c.logging.max_backups || 7}
  compress: ${c.logging.compress || false}
  log_buffer_size: ${c.logging.log_buffer_size || 500}
`
})

const copyYamlPreview = () => {
  navigator.clipboard.writeText(yamlPreview.value)
  copiedYaml.value = true
  toast.success(t('config.copied'))
  setTimeout(() => { copiedYaml.value = false }, 2000)
}

const exportConfigYaml = () => {
  const blob = new Blob([yamlPreview.value], { type: 'text/yaml;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'config.yaml'
  a.click()
  URL.revokeObjectURL(url)
  toast.success('已导出 config.yaml')
}

const exportConfigJson = () => {
  const fullData = {
    cfg: cfg.value,
    extraPolicies: extraPolicies.value,
    extraLogging: extraLogging.value
  }
  const blob = new Blob([JSON.stringify(fullData, null, 2)], { type: 'application/json;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `wx_gateway_config_${Date.now()}.json`
  a.click()
  URL.revokeObjectURL(url)
  toast.success('已导出 JSON 配置文件')
}

const triggerImport = () => {
  if (fileInputRef.value) {
    fileInputRef.value.value = ''
    fileInputRef.value.click()
  }
}

const onFileImported = (event) => {
  const file = event.target.files && event.target.files[0]
  if (!file) return

  const reader = new FileReader()
  reader.onload = (e) => {
    const text = e.target.result
    try {
      if (file.name.endsWith('.json')) {
        const json = JSON.parse(text)
        if (json.cfg) cfg.value = { ...defaultCfg, ...json.cfg }
        if (json.extraPolicies) extraPolicies.value = { ...defaultPolicies, ...json.extraPolicies }
        if (json.extraLogging) extraLogging.value = { ...defaultLoggingExtra, ...json.extraLogging }
        originsText.value = (cfg.value.gateway.allowed_origins || []).join(', ')
        saveAllConfigs()
        toast.success('JSON 配置导入并保存成功')
      } else {
        const lines = text.split('\n')
        lines.forEach(line => {
          const trimmed = line.trim()
          if (trimmed.startsWith('listen:')) cfg.value.gateway.listen = trimmed.replace('listen:', '').replace(/["']/g, '').trim()
          if (trimmed.startsWith('api_token:')) cfg.value.gateway.api_token = trimmed.replace('api_token:', '').replace(/["']/g, '').trim()
          if (trimmed.startsWith('locale:')) cfg.value.gateway.locale = trimmed.replace('locale:', '').replace(/["']/g, '').trim()
          if (trimmed.startsWith('display:')) cfg.value.launcher.display = trimmed.replace('display:', '').replace(/["']/g, '').trim()
          if (trimmed.startsWith('qr_port:')) cfg.value.launcher.qr_port = parseInt(trimmed.replace('qr_port:', '').trim()) || 8899
          if (trimmed.startsWith('level:')) cfg.value.logging.level = trimmed.replace('level:', '').replace(/["']/g, '').trim()
        })
        originsText.value = (cfg.value.gateway.allowed_origins || []).join(', ')
        saveAllConfigs()
        toast.success('YAML 配置解析导入成功')
      }
    } catch (err) {
      toast.error('导入配置文件失败: ' + err.message)
    }
  }
  reader.readAsText(file)
}

const resetToDefaults = () => {
  dialog.confirm({
    title: t('config.resetDefaults'),
    content: '确定要恢复全部出厂默认设置吗？现有未导出的个性化修改将被覆盖。',
    confirmText: t('dialog.confirm'),
    cancelText: t('dialog.cancel'),
    onConfirm: () => {
      cfg.value = JSON.parse(JSON.stringify(defaultCfg))
      extraPolicies.value = { ...defaultPolicies }
      extraLogging.value = { ...defaultLoggingExtra }
      originsText.value = ''
      saveAllConfigs()
      toast.info('已恢复默认出厂设置')
    }
  })
}
</script>

<template>
  <div class="relative min-h-screen text-slate-800 dark:text-slate-100 font-sans selection:bg-primary/20 selection:text-primary transition-colors duration-300">
    <!-- 全局毛玻璃动态背景 -->
    <PageBackground />

    <!-- 全局轻提示与弹窗挂载点 -->
    <ToastContainer />
    <DialogModal />

    <!-- 顶层视图切换 -->
    <transition
      mode="out-in"
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="opacity-0 scale-[0.98]"
      enter-to-class="opacity-100 scale-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="opacity-100 scale-100"
      leave-to-class="opacity-0 scale-[0.98]"
    >
      <!-- 1. WebUI 访问登录页 -->
      <WebLoginPage v-if="!isAuthenticated" @login-success="onWebLoginSuccess" />

      <!-- 2. 控制台主界面 -->
      <div v-else class="flex w-full min-h-screen">
        <!-- 左侧侧边栏 (Sidebar) -->
        <aside
          :class="[
            'bg-white/95 dark:bg-[#101420]/85 border-r border-slate-200/80 dark:border-white/[0.08] flex flex-col justify-between shrink-0 select-none z-40 backdrop-blur-xl transition-all duration-300 ease-in-out shadow-sm',
            openSideBar ? 'w-64' : 'w-20'
          ]"
        >
          <div>
            <!-- 品牌标识 -->
            <div class="h-16 px-5 flex items-center gap-3 border-b border-slate-200/80 dark:border-white/[0.08]">
              <div class="h-5 w-1.5 bg-primary rounded-full shadow-[0_0_8px_var(--primary-glow)] shrink-0 transition-colors duration-300"></div>
              <div v-show="openSideBar" class="overflow-hidden flex items-center gap-2">
                <span class="text-lg font-bold tracking-wide text-slate-800 dark:text-white select-none">WeChat</span>
                <span class="text-[10px] px-1.5 py-0.5 rounded-full font-mono font-semibold bg-primary/15 text-primary border border-primary/30 transition-colors duration-300">
                  {{ t('brandSubtitle') }}
                </span>
              </div>
            </div>

            <!-- 导航菜单项 -->
            <nav class="p-3 space-y-1.5 mt-2">
              <button
                v-for="item in navItems"
                :key="item.id"
                @click="activeTab = item.id"
                :class="[
                  'flex items-center w-full text-left transition-all duration-300 px-3.5 py-2.5 rounded-xl text-sm font-medium group',
                  activeTab === item.id
                    ? 'bg-primary/10 text-primary font-semibold translate-x-1 border border-primary/20'
                    : 'text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white hover:bg-slate-100 dark:hover:bg-white/[0.05] hover:translate-x-1'
                ]"
                :title="item.name"
              >
                <!-- SVG 图标 -->
                <component :is="item.icon" class="w-5 h-5 shrink-0 transition-transform group-hover:scale-110" />

                <!-- 标题文字 -->
                <span v-show="openSideBar" class="ml-3 truncate font-medium">{{ item.name }}</span>

                <!-- 右侧胶囊动画指示器 -->
                <div
                  v-show="openSideBar"
                  :class="[
                    'w-3 h-1.5 rounded-full ml-auto shrink-0 transition-all duration-300',
                    activeTab === item.id
                      ? 'bg-primary animate-nav-spin shadow-[0_0_8px_var(--primary-glow)]'
                      : 'bg-slate-300 dark:bg-white/10 group-hover:bg-slate-400 dark:group-hover:bg-white/30'
                  ]"
                ></div>
              </button>
            </nav>
          </div>

          <!-- 侧边栏底部操作区 (主题模式切换、语言切换、登出) -->
          <div class="p-3 border-t border-slate-200/80 dark:border-white/[0.08] bg-slate-50/70 dark:bg-black/20 space-y-2">
            <!-- 状态与运行时间 -->
            <div v-show="openSideBar" class="flex items-center justify-between text-xs px-2 mb-2">
              <div class="flex items-center gap-2">
                <span :class="['w-2 h-2 rounded-full transition-all', wsConnected ? 'bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.8)]' : 'bg-rose-500']"></span>
                <span class="text-slate-500 dark:text-slate-400">{{ wsConnected ? t('header.online') : t('header.offline') }}</span>
              </div>
              <span class="font-mono text-[11px] text-slate-500 dark:text-slate-400">{{ uptimeStr }}</span>
            </div>

            <!-- 主题模式切换 (亮色 / 夜间 / 跟随系统 - 严格 3 模式) -->
            <div :class="['flex items-center bg-slate-200/70 dark:bg-white/[0.04] p-1 rounded-xl border border-slate-200 dark:border-white/5', openSideBar ? 'justify-between' : 'justify-center']">
              <button
                v-for="m in themeModes"
                :key="m.id"
                type="button"
                @click="setThemeMode(m.id)"
                :title="m.name"
                :class="[
                  'p-1.5 rounded-lg text-xs font-medium transition-all flex items-center justify-center',
                  openSideBar ? 'flex-1 gap-1' : 'w-7 h-7',
                  themeState.mode === m.id
                    ? 'bg-white dark:bg-primary text-primary dark:text-white shadow-sm font-semibold'
                    : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white hover:bg-white/50 dark:hover:bg-white/[0.06]'
                ]"
              >
                <component :is="m.icon" class="w-3.5 h-3.5 shrink-0" />
                <span v-show="openSideBar" class="text-[10px]">{{ m.label }}</span>
              </button>
            </div>

            <!-- 语言切换 (中 / EN) -->
            <button
              @click="handleToggleLocale"
              class="w-full flex items-center justify-center gap-2 px-3 py-2 rounded-xl text-xs font-medium text-slate-700 dark:text-slate-300 bg-white/70 dark:bg-white/[0.04] hover:bg-slate-200/80 dark:hover:bg-white/[0.08] border border-slate-200/80 dark:border-white/5 transition-all shadow-xs"
              :title="i18nState.locale === 'zh-CN' ? 'Switch to English' : '切换为简体中文'"
            >
              <svg class="w-4 h-4 text-emerald-500 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 5h12M9 3v2m1.048 9.5A18.022 18.022 0 016.412 9m6.088 9h7M11 21l5-10 5 10M12.751 5C11.783 10.77 8.07 15.61 3 18.129" />
              </svg>
              <span v-show="openSideBar">{{ i18nState.locale === 'zh-CN' ? 'English (US)' : '简体中文' }}</span>
            </button>

            <!-- 退出 Web 认证 -->
            <button
              @click="handleWebLogout"
              class="w-full flex items-center justify-center gap-2 px-3 py-2 rounded-xl text-xs font-medium text-rose-500 dark:text-rose-400 bg-rose-500/10 hover:bg-rose-500/20 border border-rose-500/20 transition-all"
            >
              <svg class="w-4 h-4 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
              </svg>
              <span v-show="openSideBar">{{ t('dialog.webLogoutTitle') }}</span>
            </button>
          </div>
        </aside>

        <!-- 右侧主体内容容器 -->
        <div class="flex-1 flex flex-col min-w-0 overflow-y-auto">
          <!-- 顶部悬浮胶囊栏 (Floating Capsule Header) -->
          <header class="h-11 flex items-center justify-between backdrop-blur-lg rounded-full bg-white/80 dark:bg-black/40 border border-slate-200/80 dark:border-white/10 shadow-sm z-30 m-3 mb-0 sticky top-3 left-0 px-4 select-none">
            <!-- 左侧折叠按钮与面包屑导航 -->
            <div class="flex items-center gap-2.5">
              <button
                @click="openSideBar = !openSideBar"
                class="p-1 rounded-full text-slate-500 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white hover:bg-slate-100 dark:hover:bg-white/10 transition-colors"
                title="展开/折叠侧边栏"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
                </svg>
              </button>

              <!-- 面包屑路径 -->
              <div class="flex items-center gap-2 text-xs">
                <span class="text-slate-500 dark:text-slate-400 font-medium">{{ t('header.wechatHook') }}</span>
                <span class="text-slate-400 dark:text-slate-600">/</span>
                <span class="text-slate-800 dark:text-slate-200 font-semibold">{{ currentTabTitle }}</span>
              </div>
            </div>

            <!-- 右侧快捷控制区 (语言切换、主题模式、刷新、状态徽章) -->
            <div class="flex items-center gap-2 sm:gap-2.5">
              <!-- 快捷语言切换按钮 (中 / EN) -->
              <button
                @click="handleToggleLocale"
                class="px-2.5 py-1 rounded-full bg-slate-100 hover:bg-slate-200 dark:bg-white/[0.06] dark:hover:bg-white/[0.12] text-xs text-slate-700 dark:text-slate-300 flex items-center gap-1.5 transition-all active:scale-95 border border-slate-200/80 dark:border-white/5"
                :title="i18nState.locale === 'zh-CN' ? 'Switch to English' : '切换为简体中文'"
              >
                <svg class="w-3.5 h-3.5 text-primary shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 5h12M9 3v2m1.048 9.5A18.022 18.022 0 016.412 9m6.088 9h7M11 21l5-10 5 10M12.751 5C11.783 10.77 8.07 15.61 3 18.129" />
                </svg>
                <span class="font-semibold">{{ i18nState.locale === 'zh-CN' ? 'EN' : '中' }}</span>
              </button>

              <!-- 快捷主题切换按钮 -->
              <button
                @click="cycleThemeMode"
                class="p-1.5 rounded-full bg-slate-100 hover:bg-slate-200 dark:bg-white/[0.06] dark:hover:bg-white/[0.12] text-slate-700 dark:text-slate-300 transition-all active:scale-95 border border-slate-200/80 dark:border-white/5"
                :title="`当前主题: ${currentThemeModeLabel} (点击切换模式)`"
              >
                <component :is="currentThemeModeIcon" class="w-3.5 h-3.5 shrink-0" />
              </button>

              <!-- 刷新按钮 -->
              <button
                @click="fetchStatus(true)"
                :disabled="isRefreshing"
                class="px-2.5 py-1 rounded-full bg-slate-100 hover:bg-slate-200 dark:bg-white/[0.06] dark:hover:bg-white/[0.12] text-xs text-slate-700 dark:text-slate-300 flex items-center gap-1.5 transition-all active:scale-95 disabled:opacity-50 border border-slate-200/80 dark:border-white/5"
              >
                <svg :class="['w-3.5 h-3.5', { 'animate-spin': isRefreshing }]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                </svg>
                <span class="hidden sm:inline">{{ t('header.refresh') }}</span>
              </button>

              <!-- 状态徽章 -->
              <span :class="['text-[11px] px-2.5 py-0.5 rounded-full font-medium border flex items-center gap-1.5', statusBadgeClass]">
                <span class="w-1.5 h-1.5 rounded-full bg-current"></span>
                {{ statusText }}
              </span>
            </div>
          </header>

          <!-- 内容页面区 -->
          <main class="p-4 md:p-6 max-w-7xl w-full mx-auto space-y-6">
            <transition
              mode="out-in"
              enter-active-class="transition duration-200 ease-out transform"
              enter-from-class="opacity-0 translate-y-2 scale-[0.99]"
              enter-to-class="opacity-100 translate-y-0 scale-100"
              leave-active-class="transition duration-150 ease-in transform"
              leave-from-class="opacity-100 translate-y-0 scale-100"
              leave-to-class="opacity-0 translate-y-[-2px] scale-[0.99]"
            >
              <!-- Tab 1: 基础信息 -->
              <div v-if="activeTab === 'dashboard'" key="dashboard" class="space-y-5">
                <div class="grid grid-cols-1 lg:grid-cols-3 gap-4 items-stretch">
                  <div class="flex flex-col gap-3">
                    <WeChatInfoCard
                      :nickname="wxid || t('dashboard.unknownUser')"
                      :wxid="wxid || t('dashboard.notLoggedIn')"
                      :is-online="loginStatus === 'logged_in'"
                    />
                    <SystemInfoCard :arch-info="diag.display ? `Linux (${diag.display})` : 'Linux x86_64'" />
                  </div>

                  <SystemStatusDisplay
                    class="lg:col-span-2"
                    :cpu-usage-system="systemMetrics.cpuUsageSystem"
                    :cpu-usage-process="systemMetrics.cpuUsageProcess"
                    :mem-total-m-b="systemMetrics.memTotalMB"
                    :mem-used-m-b="systemMetrics.memUsedMB"
                    :mem-process-m-b="systemMetrics.memProcessMB"
                  />
                </div>

                <!-- 6 列关键网络与消息指标卡片 -->
                <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-3 py-1">
                  <NetworkItemDisplay :count="4" :label="t('dashboard.endpointsCount')" is-primary />
                  <NetworkItemDisplay :count="1" :label="t('dashboard.httpApiCount')" />
                  <NetworkItemDisplay :count="wsConnections" :label="t('dashboard.wsStreamCount')" />
                  <NetworkItemDisplay :count="botInfo.actions_count || 33" :label="t('dashboard.actionsCount')" />
                  <NetworkItemDisplay :count="recvCount" :label="t('dashboard.recvCount')" />
                  <NetworkItemDisplay :count="sendCount" :label="t('dashboard.sendCount')" />
                </div>

                <!-- 底部事件流水卡片 -->
                <div class="napcat-glass rounded-2xl p-5 shadow-sm space-y-3">
                  <div class="flex items-center justify-between pb-3 border-b border-slate-200/80 dark:border-white/[0.06]">
                    <div class="flex items-center gap-2">
                      <div class="w-5 h-5 rounded-md bg-primary/20 text-primary flex items-center justify-center">
                        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 14l-7 7m0 0l-7-7m7 7V3" />
                        </svg>
                      </div>
                      <span class="text-sm font-bold text-slate-800 dark:text-white">{{ t('dashboard.eventStreamTitle') }}</span>
                      <span class="text-xs text-slate-500 dark:text-slate-400">{{ t('dashboard.eventStreamSubtitle') }}</span>
                    </div>
                    <button
                      @click="recentEvents = []"
                      class="px-2.5 py-1 text-xs text-slate-500 hover:text-slate-900 dark:text-slate-400 dark:hover:text-white border border-slate-200 dark:border-white/10 rounded-lg hover:bg-slate-100 dark:hover:bg-white/[0.06] transition-colors"
                    >
                      {{ t('dashboard.clearList') }}
                    </button>
                  </div>

                  <div class="space-y-2 max-h-72 overflow-y-auto pr-1">
                    <div
                      v-for="(ev, idx) in recentEvents"
                      :key="idx"
                      class="flex items-center justify-between p-3 rounded-xl bg-slate-50/80 dark:bg-white/[0.03] border border-slate-200/80 dark:border-white/[0.05] text-xs hover:border-primary/30 transition-colors"
                    >
                      <div class="flex items-center gap-3 overflow-hidden">
                        <span class="px-2 py-0.5 rounded-md bg-primary/15 text-primary border border-primary/20 font-mono font-semibold shrink-0">
                          {{ ev.type }}
                        </span>
                        <span class="text-slate-700 dark:text-slate-200 truncate font-mono">{{ ev.summary }}</span>
                      </div>
                      <span class="text-slate-500 dark:text-slate-400 font-mono shrink-0 ml-4">{{ ev.time }}</span>
                    </div>

                    <div v-if="recentEvents.length === 0" class="text-center py-10 text-slate-500 dark:text-slate-400 text-xs">
                      {{ t('dashboard.noEvents') }}
                    </div>
                  </div>
                </div>
              </div>

              <!-- Tab 2: 扫码登录 -->
              <div v-else-if="activeTab === 'login'" key="login" class="space-y-6">
                <div class="grid grid-cols-1 lg:grid-cols-12 gap-6">
                  <!-- 扫码主卡片 -->
                  <div class="lg:col-span-7 napcat-glass rounded-2xl p-7 shadow-sm flex flex-col items-center text-center relative overflow-hidden">
                    <div class="w-full flex items-center justify-between pb-4 border-b border-slate-200/80 dark:border-white/[0.08] mb-6">
                      <div class="text-left">
                        <h2 class="text-lg font-bold text-slate-800 dark:text-white flex items-center gap-2">
                          <span>{{ t('login.cardTitle') }}</span>
                        </h2>
                        <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{{ t('login.cardSubtitle') }}</p>
                      </div>
                      <span :class="['text-xs px-3 py-1 rounded-full font-semibold border', statusBadgeClass]">
                        {{ statusText }}
                      </span>
                    </div>

                    <!-- 二维码显示区域 -->
                    <div class="relative w-64 h-64 rounded-2xl bg-slate-100 dark:bg-white/[0.03] border-2 border-dashed border-slate-300 dark:border-white/10 flex items-center justify-center p-3 mb-4 group shadow-inner">
                      <!-- 登录中二维码图片 -->
                      <img
                        v-if="qrImageSrc && (loginStatus === 'waiting_scan' || loginStatus === 'scanned')"
                        :src="qrImageSrc"
                        alt="WeChat QR Code"
                        class="w-full h-full object-contain rounded-xl shadow-lg transition-transform duration-300 group-hover:scale-102"
                      />

                      <!-- 扫码成功提示遮罩 -->
                      <div
                        v-if="loginStatus === 'scanned'"
                        class="absolute inset-0 bg-[#0c101a]/95 backdrop-blur-md flex flex-col items-center justify-center p-6 text-white text-center rounded-xl animate-fade-in"
                      >
                        <div class="w-14 h-14 rounded-full bg-sky-500/20 text-sky-400 flex items-center justify-center mb-3">
                          <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 18h.01M8 21h8a2 2 0 002-2V5a2 2 0 00-2-2H8a2 2 0 00-2 2v14a2 2 0 002 2z" />
                          </svg>
                        </div>
                        <h4 class="text-sm font-bold text-white mb-1">{{ t('login.scannedTitle') }}</h4>
                        <p class="text-xs text-slate-300 leading-relaxed">{{ t('login.scannedSubtitle') }}</p>
                      </div>

                      <!-- 登录已就绪卡片 -->
                      <div
                        v-if="loginStatus === 'logged_in'"
                        class="absolute inset-0 bg-emerald-950/90 backdrop-blur-md flex flex-col items-center justify-center p-6 text-white text-center rounded-xl"
                      >
                        <div class="w-14 h-14 rounded-full bg-emerald-500/20 text-emerald-400 flex items-center justify-center mb-3 shadow-[0_0_15px_rgba(16,185,129,0.4)]">
                          <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7" />
                          </svg>
                        </div>
                        <h4 class="text-sm font-bold text-emerald-300 mb-1">{{ t('login.successTitle') }}</h4>
                        <p class="text-xs text-slate-300 font-mono mb-4">{{ wxid }}</p>
                        <button
                          @click="activeTab = 'dashboard'"
                          class="px-4 py-2 rounded-xl bg-emerald-500 hover:bg-emerald-600 text-white text-xs font-semibold shadow-md shadow-emerald-500/30 transition-all active:scale-95"
                        >
                          {{ t('login.goDashboard') }}
                        </button>
                      </div>

                      <!-- 二维码失效遮罩 -->
                      <div
                        v-if="loginStatus === 'expired'"
                        class="absolute inset-0 bg-[#0c101a]/95 backdrop-blur-md flex flex-col items-center justify-center p-6 text-white text-center rounded-xl"
                      >
                        <div class="w-14 h-14 rounded-full bg-rose-500/20 text-rose-400 flex items-center justify-center mb-3">
                          <svg class="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                          </svg>
                        </div>
                        <h4 class="text-sm font-bold text-white mb-2">{{ t('login.expiredTitle') }}</h4>
                        <button
                          @click="refreshQR"
                          :disabled="isActionBusy"
                          class="px-4 py-2 rounded-xl bg-primary hover:bg-emerald-600 text-white text-xs font-semibold shadow-md shadow-primary/25 transition-all active:scale-95 disabled:opacity-50"
                        >
                          {{ t('login.refreshQr') }}
                        </button>
                      </div>

                      <!-- 未运行或加载中 -->
                      <div
                        v-if="!qrImageSrc && loginStatus !== 'logged_in' && loginStatus !== 'scanned'"
                        class="flex flex-col items-center justify-center text-slate-500 dark:text-slate-400 p-4"
                      >
                        <svg class="w-8 h-8 animate-spin text-primary mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                        </svg>
                        <span class="text-xs">{{ t('status.launching') }}</span>
                      </div>
                    </div>

                    <!-- 动态提示文字 -->
                    <div class="text-xs text-slate-600 dark:text-slate-300 font-medium mb-6">
                      {{ tipsText }}
                    </div>

                    <!-- 操作按钮组 -->
                    <div class="flex flex-wrap items-center justify-center gap-3 w-full">
                      <button
                        @click="refreshQR"
                        :disabled="isActionBusy"
                        class="px-4 py-2 rounded-xl bg-slate-100 hover:bg-slate-200 dark:bg-white/[0.08] dark:hover:bg-white/[0.15] text-slate-800 dark:text-white text-xs font-semibold border border-slate-200 dark:border-white/10 transition-all flex items-center gap-2 active:scale-95 disabled:opacity-50"
                      >
                        <svg :class="['w-4 h-4', { 'animate-spin': isActionBusy }]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                        </svg>
                        <span>{{ t('login.refreshQrBtn') }}</span>
                      </button>

                      <button
                        @click="restartWeChat"
                        :disabled="isActionBusy"
                        class="px-4 py-2 rounded-xl bg-slate-100 hover:bg-slate-200 dark:bg-white/[0.08] dark:hover:bg-white/[0.15] text-slate-800 dark:text-white text-xs font-semibold border border-slate-200 dark:border-white/10 transition-all flex items-center gap-2 active:scale-95 disabled:opacity-50"
                      >
                        <svg class="w-4 h-4 text-purple-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                        </svg>
                        <span>{{ t('login.restartEnv') }}</span>
                      </button>

                      <button
                        v-if="loginStatus === 'logged_in'"
                        @click="logoutWeChat"
                        :disabled="isActionBusy"
                        class="px-4 py-2 rounded-xl bg-rose-500/10 hover:bg-rose-500/20 text-rose-500 dark:text-rose-400 text-xs font-semibold border border-rose-500/20 transition-all flex items-center gap-2 active:scale-95 disabled:opacity-50"
                      >
                        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
                        </svg>
                        <span>{{ t('login.logoutWx') }}</span>
                      </button>
                    </div>
                  </div>

                  <!-- 右侧诊断与帮助指南卡片 -->
                  <div class="lg:col-span-5 space-y-4">
                    <!-- 操作步骤指南 -->
                    <div class="napcat-glass rounded-2xl p-6 shadow-sm space-y-4">
                      <div class="pb-2 border-b border-slate-200/80 dark:border-white/[0.08]">
                        <h3 class="text-sm font-bold text-slate-800 dark:text-white flex items-center gap-2">
                          <svg class="w-4 h-4 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                          </svg>
                          <span>{{ t('login.guideTitle') }}</span>
                        </h3>
                      </div>

                      <div class="space-y-3.5 text-xs">
                        <div class="flex items-start gap-3">
                          <span class="w-5 h-5 rounded-full bg-primary/20 text-primary font-bold text-xs flex items-center justify-center shrink-0">1</span>
                          <div>
                            <div class="font-bold text-slate-800 dark:text-white">{{ t('login.step1Title') }}</div>
                            <div class="text-slate-500 dark:text-slate-400 mt-0.5 leading-relaxed">{{ t('login.step1Desc') }}</div>
                          </div>
                        </div>

                        <div class="flex items-start gap-3">
                          <span class="w-5 h-5 rounded-full bg-primary/20 text-primary font-bold text-xs flex items-center justify-center shrink-0">2</span>
                          <div>
                            <div class="font-bold text-slate-800 dark:text-white">{{ t('login.step2Title') }}</div>
                            <div class="text-slate-500 dark:text-slate-400 mt-0.5 leading-relaxed">{{ t('login.step2Desc') }}</div>
                          </div>
                        </div>

                        <div class="flex items-start gap-3">
                          <span class="w-5 h-5 rounded-full bg-primary/20 text-primary font-bold text-xs flex items-center justify-center shrink-0">3</span>
                          <div>
                            <div class="font-bold text-slate-800 dark:text-white">{{ t('login.step3Title') }}</div>
                            <div class="text-slate-500 dark:text-slate-400 mt-0.5 leading-relaxed">{{ t('login.step3Desc') }}</div>
                          </div>
                        </div>
                      </div>
                    </div>

                    <!-- 运行环境自检卡片 -->
                    <div class="napcat-glass rounded-2xl p-6 shadow-sm space-y-3">
                      <div class="flex items-center justify-between pb-2 border-b border-slate-200/80 dark:border-white/[0.08]">
                        <h3 class="text-sm font-bold text-slate-800 dark:text-white flex items-center gap-2">
                          <svg class="w-4 h-4 text-purple-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                          </svg>
                          <span>{{ t('login.diagTitle') }}</span>
                        </h3>
                      </div>

                      <div class="space-y-2 text-xs font-mono">
                        <div class="flex items-center justify-between p-2 rounded-lg bg-slate-50 dark:bg-white/[0.03]">
                          <span class="text-slate-500 dark:text-slate-400">{{ t('login.display') }}</span>
                          <span class="text-slate-800 dark:text-white">{{ diag.display || ':99' }}</span>
                        </div>
                        <div class="flex items-center justify-between p-2 rounded-lg bg-slate-50 dark:bg-white/[0.03]">
                          <span class="text-slate-500 dark:text-slate-400">{{ t('login.pid') }}</span>
                          <span class="text-slate-800 dark:text-white">{{ diag.pid || '-' }}</span>
                        </div>
                        <div class="flex items-center justify-between p-2 rounded-lg bg-slate-50 dark:bg-white/[0.03]">
                          <span class="text-slate-500 dark:text-slate-400">{{ t('login.bridge') }}</span>
                          <span class="text-emerald-600 dark:text-emerald-400">librust_bridge.so</span>
                        </div>
                        <div class="flex items-center justify-between p-2 rounded-lg bg-slate-50 dark:bg-white/[0.03]">
                          <span class="text-slate-500 dark:text-slate-400">{{ t('login.socket') }}</span>
                          <span class="text-slate-800 dark:text-white">{{ diag.socket || '/tmp/wx_bridge.sock' }}</span>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Tab 3: 网络配置 (交互式网络端点管理) -->
              <div v-else-if="activeTab === 'network'" key="network" class="space-y-5">
                <NetworkManager />
              </div>

              <!-- Tab 4: 执行日志 -->
              <div v-else-if="activeTab === 'logs'" key="logs" class="h-[calc(100vh-140px)] napcat-glass rounded-2xl flex flex-col overflow-hidden shadow-2xl">
                <!-- 终端标题栏 (macOS 三色圆点与工具栏) -->
                <div class="px-4 py-3 bg-slate-100/90 dark:bg-black/40 border-b border-slate-200/80 dark:border-white/[0.08] flex items-center justify-between select-none">
                  <div class="flex items-center gap-3">
                    <div class="flex items-center gap-1.5">
                      <span class="w-3 h-3 rounded-full bg-[#ff5f56] inline-block"></span>
                      <span class="w-3 h-3 rounded-full bg-[#ffbd2e] inline-block"></span>
                      <span class="w-3 h-3 rounded-full bg-[#27c93f] inline-block"></span>
                    </div>
                    <span class="text-xs font-mono text-slate-700 dark:text-slate-300 font-medium">{{ t('logs.consoleTitle') }}</span>
                  </div>

                  <!-- 控制栏 -->
                  <div class="flex items-center gap-3">
                    <label class="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-300 cursor-pointer">
                      <input type="checkbox" v-model="autoScrollLogs" class="rounded bg-slate-200 dark:bg-black/40 border-slate-400 dark:border-slate-700 text-primary focus:ring-0" />
                      <span>{{ t('logs.autoScroll') }}</span>
                    </label>

                    <!-- 级别选择 -->
                    <div class="flex items-center bg-slate-200/80 dark:bg-black/40 rounded-lg p-0.5 border border-slate-300/80 dark:border-white/5">
                      <button
                        v-for="lvl in ['ALL', 'INFO', 'WARN', 'ERROR']"
                        :key="lvl"
                        @click="logFilterLevel = lvl"
                        :class="[
                          'px-2 py-0.5 text-[11px] rounded font-mono transition-colors',
                          logFilterLevel === lvl ? 'bg-primary text-white font-semibold' : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white'
                        ]"
                      >
                        {{ lvl }}
                      </button>
                    </div>

                    <!-- 操作按钮 -->
                    <div class="flex items-center gap-1.5">
                      <button
                        @click="clearLogs"
                        class="px-2.5 py-1 text-xs rounded-lg border border-slate-300 dark:border-white/10 text-slate-700 dark:text-slate-300 hover:bg-slate-200/70 dark:hover:bg-white/[0.08] transition-colors"
                      >
                        {{ t('logs.clear') }}
                      </button>
                      <button
                        @click="exportLogs"
                        class="px-2.5 py-1 text-xs rounded-lg border border-slate-300 dark:border-white/10 text-slate-700 dark:text-slate-300 hover:bg-slate-200/70 dark:hover:bg-white/[0.08] transition-colors"
                      >
                        {{ t('logs.export') }}
                      </button>
                    </div>
                  </div>
                </div>

                <!-- 终端正文 -->
                <div ref="terminalRef" class="flex-1 p-4 bg-slate-950 font-mono text-xs overflow-y-auto space-y-1 select-text">
                  <div
                    v-for="(log, idx) in filteredLogs"
                    :key="idx"
                    class="flex items-start gap-2 leading-relaxed"
                  >
                    <span class="text-slate-500 shrink-0 select-none">[{{ log.time }}]</span>
                    <span :class="['shrink-0 font-bold px-1 rounded text-[10px] select-none', logBadgeClass(log.level)]">
                      {{ log.level }}
                    </span>
                    <span class="text-slate-200 break-all">{{ log.msg }}</span>
                  </div>

                  <div v-if="filteredLogs.length === 0" class="text-slate-500 py-10 text-center text-xs">
                    {{ t('logs.noLogs') }}
                  </div>
                </div>
              </div>

              <!-- Tab 5: 接口调试 (REST 测试工作台) -->
              <div v-else-if="activeTab === 'tester'" key="tester" class="grid grid-cols-1 lg:grid-cols-12 gap-6">
                <div class="lg:col-span-6 napcat-glass rounded-2xl p-6 shadow-sm space-y-4">
                  <div class="pb-3 border-b border-slate-200/80 dark:border-white/[0.08]">
                    <h3 class="text-sm font-bold text-slate-800 dark:text-white flex items-center gap-2">
                      <svg class="w-4 h-4 text-primary shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
                      </svg>
                      <span>{{ t('tester.cardTitle') }}</span>
                    </h3>
                    <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">{{ t('tester.cardSubtitle') }}</p>
                  </div>

                  <form @submit.prevent="sendTestAPI" class="space-y-4 text-xs">
                    <div>
                      <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('tester.actionType') }}</label>
                      <select
                        v-model="tester.action"
                        @change="onActionPresetChange"
                        class="w-full bg-slate-50 dark:bg-black/40 border border-slate-200 dark:border-white/10 rounded-xl px-3 py-2.5 text-slate-800 dark:text-slate-100 outline-none focus:border-primary font-mono text-xs"
                      >
                        <optgroup label="OneBot 11 基础信息">
                          <option value="get_login_info">get_login_info (获取登录信息)</option>
                          <option value="get_friend_list">get_friend_list (获取好友列表)</option>
                          <option value="get_group_list">get_group_list (获取群聊列表)</option>
                          <option value="get_group_member_list">get_group_member_list (获取群成员列表)</option>
                          <option value="get_status">get_status (获取运行状态)</option>
                          <option value="get_version_info">get_version_info (获取版本信息)</option>
                        </optgroup>
                        <optgroup label="OneBot 11 消息收发">
                          <option value="send_msg">send_msg (发送通用消息)</option>
                          <option value="send_private_msg">send_private_msg (发送私聊消息)</option>
                          <option value="send_group_msg">send_group_msg (发送群聊消息)</option>
                          <option value="delete_msg">delete_msg (撤回消息)</option>
                        </optgroup>
                        <optgroup label="OneBot 11 关系与群管">
                          <option value="set_group_kick">set_group_kick (移出群成员)</option>
                          <option value="set_group_leave">set_group_leave (退出群聊)</option>
                          <option value="set_friend_add_request">set_friend_add_request (处理好友申请)</option>
                        </optgroup>
                        <optgroup label="微信专属扩展">
                          <option value="send_xml">send_xml (发送卡片 XML)</option>
                          <option value="query_obj">query_obj (查询微信号资料)</option>
                          <option value="set_read_status">set_read_status (设置会话已读)</option>
                        </optgroup>
                      </select>
                    </div>

                    <div>
                      <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('tester.target') }}</label>
                      <input
                        type="text"
                        v-model="tester.target"
                        placeholder="例如: filehelper 或 123456@chatroom"
                        class="w-full bg-slate-50 dark:bg-black/40 border border-slate-200 dark:border-white/10 rounded-xl px-3 py-2.5 text-slate-800 dark:text-slate-100 outline-none focus:border-primary font-mono text-xs"
                      />
                    </div>

                    <div>
                      <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('tester.content') }}</label>
                      <textarea
                        v-model="tester.content"
                        rows="3"
                        placeholder="请输入要发送的消息内容..."
                        class="w-full bg-slate-50 dark:bg-black/40 border border-slate-200 dark:border-white/10 rounded-xl px-3 py-2 text-slate-800 dark:text-slate-100 outline-none focus:border-primary text-xs"
                      ></textarea>
                    </div>

                    <div>
                      <label class="block font-semibold text-slate-700 dark:text-slate-300 mb-1.5">{{ t('tester.extraJson') }}</label>
                      <textarea
                        v-model="tester.extraJson"
                        rows="3"
                        placeholder="{}"
                        class="w-full bg-slate-50 dark:bg-black/40 border border-slate-200 dark:border-white/10 rounded-xl px-3 py-2 text-primary font-mono outline-none focus:border-primary text-xs"
                      ></textarea>
                    </div>

                    <button
                      type="submit"
                      :disabled="tester.loading"
                      class="w-full py-2.5 bg-primary hover:bg-emerald-600 text-white font-semibold rounded-xl transition-all shadow-md shadow-primary/20 disabled:opacity-50 flex items-center justify-center gap-2"
                    >
                      <svg v-if="tester.loading" class="w-4 h-4 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
                      </svg>
                      <span>{{ tester.loading ? t('tester.sendingBtn') : t('tester.sendBtn') }}</span>
                    </button>
                  </form>
                </div>

                <!-- 响应展示区域 -->
                <div class="lg:col-span-6 napcat-glass rounded-2xl p-6 shadow-sm flex flex-col justify-between">
                  <div>
                    <div class="flex items-center justify-between pb-3 border-b border-slate-200/80 dark:border-white/[0.08] mb-4">
                      <h3 class="text-sm font-bold text-slate-800 dark:text-white flex items-center gap-2">
                        <svg class="w-4 h-4 text-emerald-500 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                        </svg>
                        <span>{{ t('tester.responseTitle') }}</span>
                      </h3>
                      <div class="flex items-center gap-3">
                        <span class="text-xs text-slate-500 dark:text-slate-400 font-mono">{{ t('tester.cost') }}: {{ tester.cost }} ms</span>
                        <button
                          @click="copyResponse"
                          class="px-2.5 py-1 rounded-lg border border-slate-200 dark:border-white/10 text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-white/[0.08] text-xs transition-colors"
                        >
                          {{ copiedText }}
                        </button>
                      </div>
                    </div>

                    <pre class="bg-slate-900 p-4 rounded-xl font-mono text-xs text-emerald-400 overflow-auto border border-slate-800 max-h-[460px] leading-relaxed">{{ tester.resultJson }}</pre>
                  </div>

                  <div class="mt-4 pt-3 border-t border-slate-200/80 dark:border-white/[0.06] text-[11px] text-slate-500 dark:text-slate-400 flex items-center justify-between">
                    <span>{{ t('tester.formatTip') }}</span>
                    <span class="font-mono text-slate-700 dark:text-slate-300">OneBot 11 (POST / 或 POST /&lt;action&gt;)</span>
                  </div>
                </div>
              </div>

              <!-- Tab 6: 系统配置 -->
              <div v-else-if="activeTab === 'config'" key="config" class="space-y-6">
                <SystemConfigManager />
              </div>
            </transition>
          </main>
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick, h } from 'vue'
import PageBackground from './components/PageBackground.vue'
import ToastContainer from './components/ToastContainer.vue'
import DialogModal from './components/DialogModal.vue'
import WebLoginPage from './components/WebLoginPage.vue'
import WeChatInfoCard from './components/WeChatInfoCard.vue'
import SystemInfoCard from './components/SystemInfoCard.vue'
import SystemStatusDisplay from './components/SystemStatusDisplay.vue'
import NetworkItemDisplay from './components/NetworkItemDisplay.vue'
import NetworkManager from './components/NetworkManager.vue'
import SystemConfigManager from './components/SystemConfigManager.vue'
import { toast, dialog } from './utils/dialog'
import { themeState, setThemeMode, toggleThemeMode } from './utils/theme'
import { i18nState, toggleLocale, t } from './utils/i18n'

// 纯 SVG 图标定义
const IconDashboard = () => h('svg', { fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24', strokeWidth: 2 }, [
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z' })
])
const IconLogin = () => h('svg', { fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24', strokeWidth: 2 }, [
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M12 4v1m6 11h2m-6 0h-2v4m0-11v3m0 0h.01M12 12h4.01M16 20h4M4 12h4m12 0h.01M5 8h2a1 1 0 001-1V5a1 1 0 00-1-1H5a1 1 0 00-1 1v2a1 1 0 001 1zm12 0h2a1 1 0 001-1V5a1 1 0 00-1-1h-2a1 1 0 00-1 1v2a1 1 0 001 1zM5 20h2a1 1 0 001-1v-2a1 1 0 00-1-1H5a1 1 0 00-1 1v2a1 1 0 001 1z' })
])
const IconNetwork = () => h('svg', { fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24', strokeWidth: 2 }, [
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071c3.904-3.905 10.236-3.905 14.141 0M1.394 9.393c5.857-5.857 15.355-5.857 21.213 0' })
])
const IconLogs = () => h('svg', { fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24', strokeWidth: 2 }, [
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z' })
])
const IconTester = () => h('svg', { fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24', strokeWidth: 2 }, [
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M13 10V3L4 14h7v7l9-11h-7z' })
])
const IconConfig = () => h('svg', { fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24', strokeWidth: 2 }, [
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z' }),
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M15 12a3 3 0 11-6 0 3 3 0 016 0z' })
])

// 3态主题模式切换按钮定义
const IconSun = () => h('svg', { fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24', strokeWidth: 2 }, [
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z' })
])
const IconMoon = () => h('svg', { fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24', strokeWidth: 2 }, [
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z' })
])
const IconSystem = () => h('svg', { fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24', strokeWidth: 2 }, [
  h('path', { strokeLinecap: 'round', strokeLinejoin: 'round', d: 'M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z' })
])

const themeModes = computed(() => [
  { id: 'light', label: t('config.themeLight'), icon: IconSun, name: '亮色' },
  { id: 'dark', label: t('config.themeDark'), icon: IconMoon, name: '夜间' },
  { id: 'system', label: t('config.themeSystem'), icon: IconSystem, name: '跟随系统' },
])

const currentThemeModeLabel = computed(() => {
  const m = themeModes.value.find(item => item.id === themeState.mode)
  return m ? m.label : '主题'
})

const currentThemeModeIcon = computed(() => {
  if (themeState.mode === 'light') return IconSun
  if (themeState.mode === 'dark') return IconMoon
  return IconSystem
})

const cycleThemeMode = () => {
  toggleThemeMode()
  toast.info(`已切换为: ${currentThemeModeLabel.value}`)
}

const handleToggleLocale = () => {
  toggleLocale()
  toast.success(i18nState.locale === 'zh-CN' ? '已切换为简体中文' : 'Switched to English (US)')
}

const navItems = computed(() => [
  { id: 'dashboard', name: t('nav.dashboard'), icon: IconDashboard },
  { id: 'login', name: t('nav.login'), icon: IconLogin },
  { id: 'network', name: t('nav.network'), icon: IconNetwork },
  { id: 'logs', name: t('nav.logs'), icon: IconLogs },
  { id: 'tester', name: t('nav.tester'), icon: IconTester },
  { id: 'config', name: t('nav.config'), icon: IconConfig },
])

// 认证状态
const tokenInUrl = new URLSearchParams(window.location.search).get('token')
if (tokenInUrl) {
  localStorage.setItem('wx_token', tokenInUrl)
}
const isAuthenticated = ref(!!localStorage.getItem('wx_token') || sessionStorage.getItem('wx_guest') === '1')

const onWebLoginSuccess = (token) => {
  isAuthenticated.value = true
}

const handleWebLogout = () => {
  dialog.confirm({
    title: t('dialog.webLogoutTitle'),
    content: t('dialog.webLogoutContent'),
    confirmText: t('dialog.confirm'),
    cancelText: t('dialog.cancel'),
    onConfirm: () => {
      localStorage.removeItem('wx_token')
      sessionStorage.removeItem('wx_guest')
      isAuthenticated.value = false
      toast.info('已退出 Web 认证')
    }
  })
}

const activeTab = ref('dashboard')
const openSideBar = ref(true)

// Titles
const currentTabTitle = computed(() => {
  switch (activeTab.value) {
    case 'dashboard': return t('dashboard.title')
    case 'login': return t('login.title')
    case 'network': return t('network.title')
    case 'logs': return t('logs.title')
    case 'tester': return t('tester.title')
    case 'config': return t('config.title')
    default: return ''
  }
})

// State
const wsConnected = ref(false)
const loginStatus = ref('waiting_scan')
const wxid = ref('')
const qrImageSrc = ref('')
const isRefreshing = ref(false)
const isActionBusy = ref(false)
const diag = ref({ display: ':99', pid: '-', socket: '/tmp/wx_bridge.sock', listen: '' })
const botInfo = ref({ version: 'v4.1.15.55', actions_count: 33 })
const uptime = ref(0)
const recvCount = ref(0)
const sendCount = ref(0)
const wsConnections = ref(1)
const systemMetrics = ref({
  cpuUsageSystem: 0,
  cpuUsageProcess: 0,
  memTotalMB: 8192,
  memUsedMB: 0,
  memProcessMB: 0
})
const recentEvents = ref([])

// Status Badges
const statusBadgeClass = computed(() => {
  switch (loginStatus.value) {
    case 'logged_in': return 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/30'
    case 'waiting_scan': return 'bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/30 animate-pulse'
    case 'scanned': return 'bg-sky-500/15 text-sky-600 dark:text-sky-400 border-sky-500/30'
    case 'expired': return 'bg-rose-500/15 text-rose-600 dark:text-rose-400 border-rose-500/30'
    case 'launching': return 'bg-purple-500/15 text-purple-600 dark:text-purple-400 border-purple-500/30'
    default: return 'bg-slate-500/15 text-slate-600 dark:text-slate-400 border-slate-500/30'
  }
})

const statusText = computed(() => {
  switch (loginStatus.value) {
    case 'logged_in': return t('status.logged_in')
    case 'waiting_scan': return t('status.waiting_scan')
    case 'scanned': return t('status.scanned')
    case 'expired': return t('status.expired')
    case 'launching': return t('status.launching')
    default: return t('status.offline')
  }
})

const tipsText = computed(() => {
  if (loginStatus.value === 'waiting_scan') return t('login.waitingTip')
  if (loginStatus.value === 'scanned') return t('login.scannedSubtitle')
  if (loginStatus.value === 'logged_in') return t('login.successTitle')
  if (loginStatus.value === 'expired') return t('login.expiredTitle')
  return t('status.launching')
})

const uptimeStr = computed(() => {
  const s = uptime.value
  const h = String(Math.floor(s / 3600)).padStart(2, '0')
  const m = String(Math.floor((s % 3600) / 60)).padStart(2, '0')
  const sec = String(s % 60).padStart(2, '0')
  return `${h}:${m}:${sec}`
})

// Logs Terminal
const terminalRef = ref(null)
const autoScrollLogs = ref(true)
const logFilterLevel = ref('ALL')
const logs = ref([])

const filteredLogs = computed(() => {
  if (logFilterLevel.value === 'ALL') return logs.value
  return logs.value.filter(l => l.level === logFilterLevel.value)
})

const logBadgeClass = (lvl) => {
  switch (lvl) {
    case 'ERROR': return 'bg-rose-500/20 text-rose-400'
    case 'WARN': return 'bg-amber-500/20 text-amber-400'
    case 'INFO': return 'bg-sky-500/20 text-sky-400'
    default: return 'bg-slate-800 text-slate-400'
  }
}

const clearLogs = () => {
  logs.value = []
  toast.info(t('logs.clear') + ' OK')
}

const exportLogs = () => {
  if (logs.value.length === 0) {
    toast.error('No logs to export')
    return
  }
  const text = logs.value.map(l => `[${l.time}] [${l.level}] ${l.msg}`).join('\n')
  const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `wxbot_${new Date().toISOString().slice(0, 10)}.log`
  a.click()
  URL.revokeObjectURL(url)
  toast.success('Log exported')
}

const fetchLogs = async () => {
  try {
    const res = await fetch('/api/logs')
    const json = await res.json()
    if (json.result && Array.isArray(json.result)) {
      logs.value = json.result.map(item => ({
        time: item.time || new Date().toLocaleTimeString(),
        level: item.level || 'INFO',
        msg: item.msg || ''
      }))
      if (autoScrollLogs.value) {
        nextTick(() => { if (terminalRef.value) terminalRef.value.scrollTop = terminalRef.value.scrollHeight })
      }
    }
  } catch (err) {}
}

// Tester Form
const tester = ref({
  action: 'send_msg',
  target: 'filehelper',
  content: 'Hello from OneBot 11 WebUI!',
  extraJson: '{}',
  loading: false,
  cost: 0,
  resultJson: 'Waiting for response...'
})

const onActionPresetChange = () => {
  if (tester.value.action === 'send_msg' || tester.value.action === 'send_private_msg') {
    tester.value.target = 'filehelper'
    tester.value.content = 'Hello from OneBot 11 WebUI!'
    tester.value.extraJson = '{}'
  } else if (tester.value.action === 'send_group_msg') {
    tester.value.target = '12345678@chatroom'
    tester.value.content = 'Hello from OneBot 11 WebUI!'
    tester.value.extraJson = '{}'
  } else if (tester.value.action === 'delete_msg') {
    tester.value.target = ''
    tester.value.content = ''
    tester.value.extraJson = '{\n  "message_id": 1001\n}'
  } else if (tester.value.action === 'get_group_member_list') {
    tester.value.target = '12345678@chatroom'
    tester.value.content = ''
    tester.value.extraJson = '{}'
  } else {
    tester.value.target = ''
    tester.value.content = ''
    tester.value.extraJson = '{}'
  }
}

const sendTestAPI = async () => {
  tester.value.loading = true
  const start = performance.now()
  let extra = {}
  try {
    if (tester.value.extraJson.trim()) {
      extra = JSON.parse(tester.value.extraJson)
    }
  } catch (e) {
    toast.error('JSON payload error')
    tester.value.loading = false
    return
  }

  const payload = { ...extra }
  if (tester.value.target) {
    if (tester.value.target.endsWith('@chatroom')) {
      payload.group_id = tester.value.target
    } else {
      payload.user_id = tester.value.target
    }
  }
  if (tester.value.content) payload.message = tester.value.content

  try {
    const token = localStorage.getItem('wx_token')
    const headers = { 'Content-Type': 'application/json' }
    if (token) {
      headers['Authorization'] = `Bearer ${token}`
    }

    const resp = await fetch('/api', {
      method: 'POST',
      headers,
      body: JSON.stringify({
        action: tester.value.action,
        params: payload,
        echo: `test_${Date.now()}`
      })
    })
    const data = await resp.json()
    tester.value.resultJson = JSON.stringify(data, null, 2)
    if (data.status === 'ok' || data.retcode === 0 || data.code === 200) {
      sendCount.value++
      toast.success('OneBot 11 API 请求成功')
    } else {
      toast.error(`${data.message || data.wording || data.msg || 'Error'}`)
    }
  } catch (err) {
    tester.value.resultJson = JSON.stringify({ error: err.message }, null, 2)
    toast.error('API request failed: ' + err.message)
  } finally {
    tester.value.cost = Math.round(performance.now() - start)
    tester.value.loading = false
  }
}

const copiedText = ref(t('tester.copyResult'))
const copyResponse = () => {
  navigator.clipboard.writeText(tester.value.resultJson)
  copiedText.value = t('tester.copied')
  toast.success('Copied to clipboard')
  setTimeout(() => { copiedText.value = t('tester.copyResult') }, 1500)
}

// REST Actions
const fetchStatus = async (showSpin = false) => {
  if (showSpin) isRefreshing.value = true
  try {
    const res = await fetch('/api/login/status')
    const json = await res.json()
    if (json.result) {
      loginStatus.value = json.result.status || 'stopped'
      wxid.value = json.result.wxid || ''
      if (json.result.uptime_secs !== undefined) uptime.value = json.result.uptime_secs
      if (json.result.cpu_usage_system !== undefined) {
        systemMetrics.value.cpuUsageSystem = Number(json.result.cpu_usage_system) || 0
      }
      if (json.result.cpu_usage_process !== undefined) {
        systemMetrics.value.cpuUsageProcess = Number(json.result.cpu_usage_process) || 0
      }
      if (json.result.mem_total_mb !== undefined && json.result.mem_total_mb > 0) {
        systemMetrics.value.memTotalMB = Number(json.result.mem_total_mb)
      }
      if (json.result.mem_used_mb !== undefined) {
        systemMetrics.value.memUsedMB = Number(json.result.mem_used_mb) || 0
      }
      if (json.result.mem_process_mb !== undefined) {
        systemMetrics.value.memProcessMB = Number(json.result.mem_process_mb) || 0
      }
      if (json.result.recv_count !== undefined) {
        recvCount.value = Number(json.result.recv_count) || 0
      }
      if (json.result.send_count !== undefined) {
        sendCount.value = Number(json.result.send_count) || 0
      }
      if (json.result.ws_connections !== undefined) {
        wsConnections.value = Number(json.result.ws_connections) || 0
      }
      diag.value = {
        display: json.result.display || ':99',
        pid: json.result.pid || '-',
        socket: json.result.socket || '/tmp/wx_bridge.sock',
        listen: json.result.listen || window.location.host
      }
      if (loginStatus.value === 'waiting_scan' || loginStatus.value === 'scanned') {
        qrImageSrc.value = `/api/login/qrcode?t=${Date.now()}`
      }
    }
    const infoRes = await fetch('/api/bot/info')
    const infoJson = await infoRes.json()
    if (infoJson.result) {
      botInfo.value = infoJson.result
    }
  } catch (e) {
  } finally {
    if (showSpin) setTimeout(() => { isRefreshing.value = false }, 400)
  }
}

const refreshQR = async () => {
  isActionBusy.value = true
  try {
    await fetch('/api/login/refresh', { method: 'POST' })
    toast.success('QR refreshed')
    setTimeout(() => { fetchStatus() }, 600)
  } catch (e) {
    toast.error('Failed to refresh QR')
  } finally {
    setTimeout(() => { isActionBusy.value = false }, 1000)
  }
}

const restartWeChat = () => {
  dialog.confirm({
    title: t('dialog.restartTitle'),
    content: t('dialog.restartContent'),
    confirmText: t('dialog.confirm'),
    cancelText: t('dialog.cancel'),
    onConfirm: async () => {
      isActionBusy.value = true
      toast.info('Restarting environment...')
      try {
        await fetch('/api/login/restart', { method: 'POST' })
        toast.success('Restart requested')
        setTimeout(() => { fetchStatus() }, 2000)
      } catch (e) {
        toast.error('Restart failed: ' + e.message)
      } finally {
        setTimeout(() => { isActionBusy.value = false }, 2000)
      }
    }
  })
}

const logoutWeChat = () => {
  dialog.confirm({
    title: t('dialog.wxLogoutTitle'),
    content: t('dialog.wxLogoutContent'),
    confirmText: t('dialog.confirm'),
    cancelText: t('dialog.cancel'),
    onConfirm: async () => {
      isActionBusy.value = true
      try {
        await fetch('/api/login/logout', { method: 'POST' })
        toast.success('WeChat logged out')
        setTimeout(() => { fetchStatus() }, 1000)
      } catch (e) {
        toast.error('Logout failed: ' + e.message)
      } finally {
        setTimeout(() => { isActionBusy.value = false }, 1000)
      }
    }
  })
}

// WebSocket Stream
let ws = null
let pollTimer = null
let reconnectTimer = null

const connectWS = () => {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  ws = new WebSocket(`${proto}//${location.host}/ws`)
  ws.onopen = () => {
    wsConnected.value = true
    logs.value.push({
      time: new Date().toLocaleTimeString(),
      level: 'INFO',
      msg: 'WebSocket connected'
    })
  }
  ws.onmessage = (e) => {
    try {
      const msg = JSON.parse(e.data)
      if (msg.event === 'log' || msg.type === 'log') {
        logs.value.push({
          time: msg.data.time || new Date().toLocaleTimeString(),
          level: msg.data.level || 'INFO',
          msg: msg.data.message || msg.data.msg || JSON.stringify(msg.data)
        })
        if (logs.value.length > 500) logs.value.shift()
        if (autoScrollLogs.value) {
          nextTick(() => { if (terminalRef.value) terminalRef.value.scrollTop = terminalRef.value.scrollHeight })
        }
        return
      }
      if (msg.event === 'login_status') {
        loginStatus.value = msg.data.status
        wxid.value = msg.data.wxid || ''
        return
      }
      recvCount.value++
      recentEvents.value.unshift({
        type: msg.type || msg.event || 'message',
        summary: typeof msg.data === 'string' ? msg.data : JSON.stringify(msg.data),
        time: new Date().toLocaleTimeString()
      })
      if (recentEvents.value.length > 50) recentEvents.value.pop()
    } catch (err) {}
  }
  ws.onclose = () => {
    wsConnected.value = false
    clearTimeout(reconnectTimer)
    reconnectTimer = setTimeout(connectWS, 3000)
  }
}

onMounted(() => {
  fetchStatus()
  fetchLogs()
  pollTimer = setInterval(fetchStatus, 2000)
  connectWS()
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
  if (reconnectTimer) clearTimeout(reconnectTimer)
  if (ws) ws.close()
})
</script>

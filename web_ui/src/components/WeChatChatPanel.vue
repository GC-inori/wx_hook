<template>
  <div class="h-[calc(100vh-130px)] napcat-glass rounded-2xl overflow-hidden shadow-2xl flex border border-slate-200/80 dark:border-white/10 select-none">
    <!-- ==================== 左侧：会话 / 通讯录列表栏 ==================== -->
    <div class="w-80 sm:w-88 shrink-0 flex flex-col border-r border-slate-200/80 dark:border-white/10 bg-slate-50/70 dark:bg-black/25">
      <!-- 顶部：搜索框与分类 Switcher -->
      <div class="p-3.5 pb-2.5 border-b border-slate-200/70 dark:border-white/[0.06] space-y-2.5">
        <!-- 搜索输入框 -->
        <div class="relative">
          <input
            v-model="searchQuery"
            type="text"
            :placeholder="t('chat.searchPlaceholder')"
            class="w-full pl-8 pr-7 py-1.5 rounded-lg text-xs bg-slate-200/70 dark:bg-white/[0.06] text-slate-800 dark:text-slate-100 placeholder-slate-400 dark:placeholder-slate-500 border border-transparent focus:border-primary/50 focus:bg-white dark:focus:bg-black/50 outline-none transition-all"
          />
          <svg class="w-3.5 h-3.5 text-slate-400 absolute left-2.5 top-1/2 -translate-y-1/2 pointer-events-none" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <button
            v-if="searchQuery"
            @click="searchQuery = ''"
            class="absolute right-2 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 p-0.5"
          >
            <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <!-- 聊天 vs 通讯录 Tab 切换 -->
        <div class="grid grid-cols-2 gap-1 p-1 bg-slate-200/60 dark:bg-white/[0.04] rounded-lg text-xs font-medium">
          <button
            @click="activeListTab = 'recent'"
            :class="[
              'py-1 rounded-md transition-all flex items-center justify-center gap-1.5',
              activeListTab === 'recent'
                ? 'bg-white dark:bg-primary text-primary dark:text-white font-semibold shadow-xs'
                : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white'
            ]"
          >
            <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
            </svg>
            <span>{{ t('chat.tabChats') }}</span>
            <span
              v-if="totalUnreadCount > 0"
              class="px-1.5 py-0.2 rounded-full text-[10px] bg-rose-500 text-white font-mono font-bold leading-tight"
            >
              {{ totalUnreadCount }}
            </span>
          </button>

          <button
            @click="activeListTab = 'contacts'"
            :class="[
              'py-1 rounded-md transition-all flex items-center justify-center gap-1.5',
              activeListTab === 'contacts'
                ? 'bg-white dark:bg-primary text-primary dark:text-white font-semibold shadow-xs'
                : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white'
            ]"
          >
            <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z" />
            </svg>
            <span>{{ t('chat.tabContacts') }}</span>
          </button>
        </div>
      </div>

      <!-- 列表主滚动区 -->
      <div class="flex-1 overflow-y-auto divide-y divide-slate-200/40 dark:divide-white/[0.03]">
        <!-- 正在加载 -->
        <div v-if="isLoadingContacts" class="py-12 flex flex-col items-center justify-center text-slate-400 text-xs gap-2">
          <svg class="w-5 h-5 animate-spin text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          <span>{{ t('chat.loading') }}</span>
        </div>

        <!-- 模式 A：最近消息会话列表 -->
        <template v-else-if="activeListTab === 'recent'">
          <div
            v-for="item in filteredRecentChats"
            :key="item.targetId"
            @click="selectChat(item)"
            :class="[
              'p-3 px-3.5 flex items-center gap-3 cursor-pointer transition-all border-l-3 select-none relative group',
              activeChat?.targetId === item.targetId
                ? 'bg-slate-200/80 dark:bg-white/[0.08] border-primary shadow-xs'
                : 'border-transparent hover:bg-slate-100 dark:hover:bg-white/[0.04]'
            ]"
          >
            <!-- 头像 -->
            <div class="relative shrink-0">
              <div
                :class="[
                  'w-10 h-10 rounded-xl flex items-center justify-center font-bold text-xs text-white shadow-xs',
                  item.isGroup ? 'bg-gradient-to-br from-indigo-500 to-purple-600' : 'bg-gradient-to-br from-emerald-500 to-teal-600'
                ]"
              >
                <svg v-if="item.isGroup" class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
                </svg>
                <span v-else>{{ getInitials(item.name) }}</span>
              </div>
              <!-- 未读徽章 -->
              <span
                v-if="item.unreadCount > 0"
                class="absolute -top-1 -right-1 px-1.5 py-0.2 rounded-full text-[10px] bg-rose-500 text-white font-mono font-bold border-2 border-white dark:border-[#121620]"
              >
                {{ item.unreadCount }}
              </span>
            </div>

            <!-- 会话信息 -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center justify-between gap-1 mb-0.5">
                <span class="text-xs font-semibold text-slate-800 dark:text-slate-100 truncate">
                  {{ item.name }}
                </span>
                <span class="text-[10px] text-slate-400 dark:text-slate-500 shrink-0 font-mono">
                  {{ item.lastTime || '' }}
                </span>
              </div>
              <p class="text-[11px] text-slate-500 dark:text-slate-400 truncate">
                {{ item.lastMsg || t('chat.noMsgYet') }}
              </p>
            </div>
          </div>

          <div v-if="filteredRecentChats.length === 0" class="py-12 text-center text-slate-400 text-xs">
            {{ searchQuery ? t('chat.noMatch') : t('chat.noRecentChats') }}
          </div>
        </template>

        <!-- 模式 B：通讯录树形列表 (群聊 + 好友) -->
        <template v-else>
          <!-- 群聊分组 -->
          <div class="space-y-0.5">
            <div
              @click="toggleGroupCollapse('groups')"
              class="px-3.5 py-2 text-[11px] font-bold text-slate-500 dark:text-slate-400 uppercase tracking-wider flex items-center justify-between cursor-pointer hover:bg-slate-100 dark:hover:bg-white/[0.03]"
            >
              <div class="flex items-center gap-1.5">
                <svg :class="['w-3.5 h-3.5 transition-transform', isGroupsCollapsed ? '-rotate-90' : '']" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                </svg>
                <span>{{ t('chat.groupChats') }} ({{ groups.length }})</span>
              </div>
              <span class="text-[10px] text-purple-600 dark:text-purple-400 font-mono bg-purple-500/10 px-1.5 py-0.2 rounded">GROUPS</span>
            </div>

            <div v-show="!isGroupsCollapsed">
              <div
                v-for="grp in filteredGroups"
                :key="grp.targetId"
                @click="selectChat(grp)"
                :class="[
                  'p-2.5 px-4 flex items-center gap-3 cursor-pointer transition-all border-l-3 select-none',
                  activeChat?.targetId === grp.targetId
                    ? 'bg-slate-200/80 dark:bg-white/[0.08] border-primary font-semibold'
                    : 'border-transparent hover:bg-slate-100 dark:hover:bg-white/[0.04]'
                ]"
              >
                <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-indigo-500 to-purple-600 flex items-center justify-center text-white shrink-0 shadow-xs">
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
                  </svg>
                </div>
                <div class="flex-1 min-w-0">
                  <div class="text-xs text-slate-800 dark:text-slate-100 truncate">{{ grp.name }}</div>
                  <div class="text-[10px] text-slate-400 font-mono truncate">{{ grp.targetId }}</div>
                </div>
              </div>
            </div>
          </div>

          <!-- 好友联系人分组 -->
          <div class="space-y-0.5 mt-2">
            <div
              @click="toggleGroupCollapse('friends')"
              class="px-3.5 py-2 text-[11px] font-bold text-slate-500 dark:text-slate-400 uppercase tracking-wider flex items-center justify-between cursor-pointer hover:bg-slate-100 dark:hover:bg-white/[0.03]"
            >
              <div class="flex items-center gap-1.5">
                <svg :class="['w-3.5 h-3.5 transition-transform', isFriendsCollapsed ? '-rotate-90' : '']" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                </svg>
                <span>{{ t('chat.contacts') }} ({{ friends.length }})</span>
              </div>
              <span class="text-[10px] text-emerald-600 dark:text-emerald-400 font-mono bg-emerald-500/10 px-1.5 py-0.2 rounded">CONTACTS</span>
            </div>

            <div v-show="!isFriendsCollapsed">
              <div
                v-for="fr in filteredFriends"
                :key="fr.targetId"
                @click="selectChat(fr)"
                :class="[
                  'p-2.5 px-4 flex items-center gap-3 cursor-pointer transition-all border-l-3 select-none',
                  activeChat?.targetId === fr.targetId
                    ? 'bg-slate-200/80 dark:bg-white/[0.08] border-primary font-semibold'
                    : 'border-transparent hover:bg-slate-100 dark:hover:bg-white/[0.04]'
                ]"
              >
                <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-teal-500 to-emerald-600 flex items-center justify-center text-white font-bold text-xs shrink-0 shadow-xs">
                  {{ getInitials(fr.name) }}
                </div>
                <div class="flex-1 min-w-0">
                  <div class="text-xs text-slate-800 dark:text-slate-100 truncate">{{ fr.name }}</div>
                  <div class="text-[10px] text-slate-400 font-mono truncate">{{ fr.targetId }}</div>
                </div>
              </div>
            </div>
          </div>
        </template>
      </div>

      <!-- 底部快捷状态与刷新 -->
      <div class="p-2.5 border-t border-slate-200/70 dark:border-white/[0.06] flex items-center justify-between text-xs text-slate-500 dark:text-slate-400 px-3">
        <span class="font-mono text-[11px]">{{ friends.length }} 好友 · {{ groups.length }} 群聊</span>
        <button
          @click="loadAllContacts"
          :disabled="isLoadingContacts"
          class="p-1 rounded-md hover:bg-slate-200/70 dark:hover:bg-white/10 transition-colors"
          title="刷新通讯录与群列表"
        >
          <svg :class="['w-3.5 h-3.5', { 'animate-spin': isLoadingContacts }]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
        </button>
      </div>
    </div>

    <!-- ==================== 右侧：主聊天视窗与群信息抽屉 ==================== -->
    <div class="flex-1 flex flex-col min-w-0 bg-white/60 dark:bg-black/40 relative">
      <!-- 视窗 1：未选择任何会话时的空状态 -->
      <div
        v-if="!activeChat"
        class="flex-1 flex flex-col items-center justify-center text-slate-400 dark:text-slate-500 p-8 space-y-4"
      >
        <div class="w-20 h-20 rounded-3xl bg-primary/10 text-primary flex items-center justify-center shadow-inner">
          <svg class="w-10 h-10" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
          </svg>
        </div>
        <div class="text-center">
          <h3 class="text-base font-bold text-slate-700 dark:text-slate-300">{{ t('chat.emptyTitle') }}</h3>
          <p class="text-xs text-slate-500 mt-1 max-w-sm">{{ t('chat.emptyDesc') }}</p>
        </div>
      </div>

      <!-- 视窗 2：活跃聊天视窗 -->
      <template v-else>
        <!-- 聊天 Header 栏 -->
        <div class="h-14 px-5 border-b border-slate-200/80 dark:border-white/10 flex items-center justify-between bg-white/80 dark:bg-white/[0.02] backdrop-blur-sm z-10">
          <div class="flex items-center gap-3 min-w-0">
            <div
              :class="[
                'w-9 h-9 rounded-xl flex items-center justify-center font-bold text-xs text-white shrink-0 shadow-xs',
                activeChat.isGroup ? 'bg-gradient-to-br from-indigo-500 to-purple-600' : 'bg-gradient-to-br from-emerald-500 to-teal-600'
              ]"
            >
              <svg v-if="activeChat.isGroup" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
              </svg>
              <span v-else>{{ getInitials(activeChat.name) }}</span>
            </div>

            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <h3 class="text-sm font-bold text-slate-800 dark:text-white truncate">
                  {{ activeChat.name }}
                </h3>
                <span
                  v-if="activeChat.isGroup && currentGroupMembers.length > 0"
                  class="text-xs text-slate-500 dark:text-slate-400 font-normal"
                >
                  ({{ currentGroupMembers.length }}人)
                </span>
              </div>
              <div class="flex items-center gap-2 text-[10px] text-slate-400 font-mono truncate">
                <span>{{ activeChat.targetId }}</span>
                <span v-if="activeChat.onebotId" class="text-primary/80">· OneBot ID: {{ activeChat.onebotId }}</span>
              </div>
            </div>
          </div>

          <!-- 右侧操作工具栏 -->
          <div class="flex items-center gap-1.5">
            <!-- 查看数据库解密密钥 -->
            <button
              @click="openDbKeyModal"
              class="p-2 rounded-lg text-amber-500 hover:text-amber-600 dark:hover:text-amber-400 hover:bg-slate-100 dark:hover:bg-white/[0.06] transition-colors cursor-pointer"
              :title="t('chat.dbKeyTitle')"
            >
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
              </svg>
            </button>

            <!-- 群成员抽屉开关 (仅限群聊) -->
            <button
              v-if="activeChat.isGroup"
              @click="isMemberDrawerOpen = !isMemberDrawerOpen"
              :class="[
                'px-3 py-1.5 rounded-lg text-xs font-medium flex items-center gap-1.5 border transition-all',
                isMemberDrawerOpen
                  ? 'bg-primary text-white border-primary shadow-sm'
                  : 'bg-slate-100 hover:bg-slate-200 dark:bg-white/[0.06] dark:hover:bg-white/[0.12] text-slate-700 dark:text-slate-200 border-slate-200 dark:border-white/5'
              ]"
              :title="t('chat.groupMembers')"
            >
              <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z" />
              </svg>
              <span class="hidden sm:inline">{{ t('chat.members') }}</span>
            </button>

            <!-- 清空当前历史 -->
            <button
              @click="clearActiveHistory"
              class="p-2 rounded-lg text-slate-500 hover:text-slate-800 dark:hover:text-white hover:bg-slate-100 dark:hover:bg-white/[0.06] transition-colors"
              title="清空当前消息窗口"
            >
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
              </svg>
            </button>
          </div>
        </div>

        <!-- 聊天气泡流水区 -->
        <div
          ref="messageScrollRef"
          class="flex-1 p-4 md:p-6 overflow-y-auto space-y-4 select-text"
        >
          <!-- 加载更早历史记录按钮 -->
          <div class="text-center py-1 select-none">
            <button
              @click="loadHistoryMessages"
              :disabled="isLoadingHistory"
              class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium bg-slate-100 hover:bg-slate-200 dark:bg-white/[0.06] dark:hover:bg-white/[0.12] text-slate-600 dark:text-slate-300 transition-all border border-slate-200/80 dark:border-white/5 cursor-pointer shadow-2xs"
            >
              <svg :class="['w-3.5 h-3.5', { 'animate-spin': isLoadingHistory }]" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <span>{{ isLoadingHistory ? t('chat.loadingHistory') : t('chat.loadHistory') }}</span>
            </button>
          </div>
          <!-- 历史消息列表 -->
          <div
            v-for="(msg, idx) in currentMessages"
            :key="msg.id || idx"
            class="space-y-1.5"
          >
            <!-- 时间线胶囊 (若是第一条或距上一条较长) -->
            <div v-if="msg.showTimeDivider" class="text-center select-none my-3">
              <span class="px-2.5 py-0.5 rounded-full bg-slate-200/60 dark:bg-white/[0.06] text-slate-500 dark:text-slate-400 text-[10px] font-mono">
                {{ msg.time }}
              </span>
            </div>

            <!-- 撤回消息胶囊状态 -->
            <div v-if="msg.isRevoked" class="text-center select-none py-1">
              <span class="px-3 py-1 rounded-lg bg-slate-100 dark:bg-white/[0.04] text-slate-500 dark:text-slate-400 text-xs italic">
                {{ msg.sender === 'self' ? t('chat.youRevoked') : t('chat.peerRevoked') }}
              </span>
            </div>

            <!-- 正常消息气泡 -->
            <div
              v-else
              :class="[
                'flex items-start gap-2.5 group relative',
                msg.sender === 'self' ? 'flex-row-reverse' : 'flex-row'
              ]"
            >
              <!-- 头像 -->
              <div
                :class="[
                  'w-8 h-8 rounded-xl flex items-center justify-center font-bold text-xs text-white shrink-0 shadow-xs select-none',
                  msg.sender === 'self'
                    ? 'bg-gradient-to-br from-emerald-500 to-green-600'
                    : 'bg-gradient-to-br from-indigo-500 to-purple-600'
                ]"
              >
                {{ msg.sender === 'self' ? '我' : getInitials(msg.senderName || activeChat.name) }}
              </div>

              <!-- 气泡内容与操作区 -->
              <div
                :class="[
                  'max-w-[75%] sm:max-w-[65%] flex flex-col',
                  msg.sender === 'self' ? 'items-end' : 'items-start'
                ]"
              >
                <!-- 发信人昵称 (群聊非自己时显示) -->
                <span
                  v-if="activeChat.isGroup && msg.sender !== 'self'"
                  class="text-[11px] text-slate-500 dark:text-slate-400 mb-0.5 px-1 font-medium select-none"
                >
                  {{ msg.senderName || msg.senderId }}
                </span>

                <!-- 气泡主体与悬浮撤回条 -->
                <div class="relative group/bubble flex items-center gap-1.5">
                  <!-- 撤回按钮 (我发出的、且有 messageId 时显示) -->
                  <div
                    v-if="msg.sender === 'self' && msg.messageId"
                    class="opacity-0 group-hover/bubble:opacity-100 transition-opacity flex items-center gap-1 select-none"
                  >
                    <button
                      @click="handleRevokeMessage(msg)"
                      class="p-1 px-1.5 rounded bg-slate-200/90 dark:bg-white/10 hover:bg-rose-500 hover:text-white text-slate-600 dark:text-slate-300 text-[10px] font-medium transition-colors shadow-xs"
                      title="撤回该消息 (CGI 536)"
                    >
                      {{ t('chat.revoke') }}
                    </button>
                  </div>

                  <!-- 气泡本身 (标准微信绿 / 雅白) -->
                  <div
                    :class="[
                      'px-3.5 py-2 rounded-2xl text-xs leading-relaxed break-all shadow-xs relative',
                      msg.sender === 'self'
                        ? 'bg-[#95ec69] dark:bg-[#07c160] text-slate-900 dark:text-white rounded-tr-xs selection:bg-black/20'
                        : 'bg-white dark:bg-[#26292e] text-slate-800 dark:text-slate-100 rounded-tl-xs border border-slate-200/70 dark:border-white/[0.05]'
                    ]"
                  >
                    <!-- 消息文本 -->
                    <span class="whitespace-pre-wrap">{{ msg.content }}</span>
                  </div>

                  <!-- 复制按钮 -->
                  <button
                    @click="copyText(msg.content)"
                    class="opacity-0 group-hover/bubble:opacity-100 transition-opacity p-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 text-[10px] select-none"
                    title="复制文本"
                  >
                    <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                    </svg>
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div v-if="currentMessages.length === 0" class="py-16 text-center text-slate-400 text-xs">
            {{ t('chat.startConversation') }}
          </div>
        </div>

        <!-- 底部输入工具栏与文本框 (微信网页版规范) -->
        <div class="border-t border-slate-200/80 dark:border-white/10 bg-slate-50/80 dark:bg-black/30 p-3 px-4 space-y-2">
          <!-- 快捷工具按钮条 -->
          <div class="flex items-center justify-between text-slate-500 dark:text-slate-400 text-xs select-none">
            <div class="flex items-center gap-3">
              <!-- 快捷 @ 成员 (仅限群聊) -->
              <button
                v-if="activeChat.isGroup"
                @click="openAtPicker"
                class="hover:text-primary transition-colors flex items-center gap-1 text-[11px]"
                title="选择成员并在输入框中 @"
              >
                <span class="font-bold">@</span>
                <span>{{ t('chat.atMember') }}</span>
              </button>

              <button
                @click="inputMessage += '😊'"
                class="hover:text-primary transition-colors"
                title="插入表情"
              >
                😊
              </button>
              <button
                @click="inputMessage += '👍'"
                class="hover:text-primary transition-colors"
                title="插入表情"
              >
                👍
              </button>
            </div>

            <!-- 发送快捷键提示 -->
            <span class="text-[11px] text-slate-400 font-normal hidden sm:inline">
              {{ t('chat.shortcutTip') }}
            </span>
          </div>

          <!-- 输入文本框 -->
          <div class="relative flex items-end gap-2">
            <textarea
              v-model="inputMessage"
              @keydown.enter.exact.prevent="handleSendMessage"
              @keydown.ctrl.enter="inputMessage += '\n'"
              rows="3"
              :placeholder="t('chat.inputPlaceholder')"
              class="flex-1 bg-transparent text-xs text-slate-800 dark:text-slate-100 placeholder-slate-400 outline-none resize-none leading-relaxed py-1"
            ></textarea>

            <!-- 发送按钮 (微信经典绿) -->
            <button
              @click="handleSendMessage"
              :disabled="!inputMessage.trim() || isSending"
              class="px-5 py-2 rounded-xl bg-primary hover:bg-emerald-600 text-white font-semibold text-xs shadow-md shadow-primary/25 transition-all active:scale-95 disabled:opacity-40 disabled:cursor-not-allowed flex items-center gap-1.5 shrink-0"
            >
              <svg v-if="isSending" class="w-3.5 h-3.5 animate-spin" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
              </svg>
              <svg v-else class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
              </svg>
              <span>{{ t('chat.send') }}</span>
            </button>
          </div>
        </div>

        <!-- ==================== 右侧滑出：群成员抽屉 ==================== -->
        <transition
          enter-active-class="transition duration-200 ease-out"
          enter-from-class="translate-x-full opacity-0"
          enter-to-class="translate-x-0 opacity-100"
          leave-active-class="transition duration-150 ease-in"
          leave-from-class="translate-x-0 opacity-100"
          leave-to-class="translate-x-full opacity-0"
        >
          <div
            v-if="isMemberDrawerOpen && activeChat.isGroup"
            class="absolute right-0 top-0 bottom-0 w-72 bg-white/95 dark:bg-[#141824]/95 backdrop-blur-xl border-l border-slate-200/80 dark:border-white/10 z-20 shadow-2xl flex flex-col"
          >
            <!-- 抽屉 Header -->
            <div class="p-4 border-b border-slate-200/80 dark:border-white/10 flex items-center justify-between">
              <div>
                <h4 class="text-xs font-bold text-slate-800 dark:text-white flex items-center gap-1.5">
                  <svg class="w-4 h-4 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
                  </svg>
                  <span>{{ t('chat.groupInfoTitle') }}</span>
                </h4>
                <p class="text-[11px] text-slate-400 mt-0.5">共 {{ currentGroupMembers.length }} 名群成员</p>
              </div>
              <button
                @click="isMemberDrawerOpen = false"
                class="p-1 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-white"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            <!-- 成员搜索 -->
            <div class="p-3 border-b border-slate-200/60 dark:border-white/[0.06]">
              <input
                v-model="memberSearchQuery"
                type="text"
                placeholder="搜索群成员..."
                class="w-full px-2.5 py-1.5 rounded-lg text-xs bg-slate-100 dark:bg-white/[0.06] text-slate-800 dark:text-slate-100 placeholder-slate-400 outline-none"
              />
            </div>

            <!-- 成员列表 (带群主徽章) -->
            <div class="flex-1 overflow-y-auto p-2 space-y-1">
              <div
                v-if="isLoadingMembers"
                class="py-10 text-center text-slate-400 text-xs flex flex-col items-center gap-2"
              >
                <svg class="w-4 h-4 animate-spin text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                </svg>
                <span>读取群成员内存...</span>
              </div>

              <div
                v-else
                v-for="m in filteredGroupMembers"
                :key="m.user_id"
                class="p-2 px-2.5 rounded-xl hover:bg-slate-100 dark:hover:bg-white/[0.04] transition-colors flex items-center justify-between text-xs group"
              >
                <div class="flex items-center gap-2.5 min-w-0">
                  <div class="w-7 h-7 rounded-lg bg-gradient-to-br from-indigo-500 to-purple-600 text-white font-bold text-[10px] flex items-center justify-center shrink-0">
                    {{ getInitials(m.nickname || m.card || m.user_id) }}
                  </div>
                  <div class="min-w-0">
                    <div class="font-medium text-slate-800 dark:text-slate-100 truncate text-xs flex items-center gap-1.5">
                      <span>{{ m.nickname || m.card || m.user_id }}</span>
                      <span
                        v-if="m.role === 'owner'"
                        class="px-1 py-0.2 rounded text-[9px] font-bold bg-amber-500/20 text-amber-600 dark:text-amber-400 border border-amber-500/30"
                      >
                        群主
                      </span>
                    </div>
                    <div class="text-[10px] text-slate-400 font-mono truncate">{{ m.user_id }}</div>
                  </div>
                </div>

                <!-- 快捷 @ 按钮 -->
                <button
                  @click="insertAt(m.nickname || m.card || m.user_id)"
                  class="opacity-0 group-hover:opacity-100 px-2 py-0.5 rounded bg-primary/15 text-primary text-[10px] font-semibold hover:bg-primary hover:text-white transition-all shrink-0"
                >
                  @TA
                </button>
              </div>
            </div>
          </div>
        </transition>
      </template>
    </div>

    <!-- ==================== 弹窗：本地数据库解密密钥 (WCDB / SQLCipher) ==================== -->
    <div
      v-if="showDbKeyModal"
      class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-xs select-text"
    >
      <div class="bg-white dark:bg-slate-900 rounded-2xl max-w-lg w-full p-6 shadow-2xl border border-slate-200 dark:border-white/10 space-y-4 animate-in fade-in zoom-in-95 duration-150">
        <div class="flex items-center justify-between border-b border-slate-200 dark:border-white/10 pb-3">
          <div class="flex items-center gap-2 text-amber-500">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
            </svg>
            <h3 class="text-sm font-bold text-slate-800 dark:text-white">{{ t('chat.dbKeyTitle') }}</h3>
          </div>
          <button
            @click="showDbKeyModal = false"
            class="p-1 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-white/10"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <p class="text-xs text-slate-500 dark:text-slate-400">{{ t('chat.dbKeyDesc') }}</p>

        <div v-if="isLoadingDbKey" class="py-8 text-center text-xs text-slate-400 flex items-center justify-center gap-2">
          <svg class="w-4 h-4 animate-spin text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          <span>正在从微信内存中提取并反混淆 AES 密钥...</span>
        </div>

        <div v-else-if="dbKeyInfo" class="space-y-3 text-xs">
          <div>
            <label class="block text-slate-400 text-[11px] mb-1 font-semibold">{{ t('chat.dbKey') }}</label>
            <div class="flex items-center gap-2 bg-slate-100 dark:bg-white/[0.05] p-2.5 rounded-lg border border-slate-200 dark:border-white/5 font-mono break-all select-all text-emerald-600 dark:text-emerald-400">
              <span class="flex-1 text-[11px]">{{ dbKeyInfo.key }}</span>
              <button
                @click="copyToClipboard(dbKeyInfo.key)"
                class="px-2.5 py-1 bg-primary text-white text-[10px] font-semibold rounded hover:bg-primary/90 transition-colors shrink-0 cursor-pointer"
              >
                {{ t('chat.copyKey') }}
              </button>
            </div>
          </div>

          <div class="grid grid-cols-2 gap-2 font-mono text-[11px]">
            <div class="bg-slate-100 dark:bg-white/[0.05] p-2.5 rounded-lg border border-slate-200 dark:border-white/5">
              <span class="text-slate-400 block text-[10px]">{{ t('chat.dbPageSize') }}</span>
              <span class="text-slate-700 dark:text-slate-200 font-semibold">{{ dbKeyInfo.page_size || 4096 }} Bytes</span>
            </div>
            <div class="bg-slate-100 dark:bg-white/[0.05] p-2.5 rounded-lg border border-slate-200 dark:border-white/5">
              <span class="text-slate-400 block text-[10px]">{{ t('chat.dbCipherVer') }}</span>
              <span class="text-slate-700 dark:text-slate-200 font-semibold">SQLCipher v{{ dbKeyInfo.cipher_version || 4 }}</span>
            </div>
          </div>

          <div>
            <label class="block text-slate-400 text-[11px] mb-1 font-semibold">{{ t('chat.dbPath') }}</label>
            <div class="bg-slate-100 dark:bg-white/[0.05] p-2.5 rounded-lg border border-slate-200 dark:border-white/5 font-mono text-[11px] text-slate-600 dark:text-slate-300 break-all select-all">
              {{ dbKeyInfo.db_dir }}
            </div>
          </div>

          <div>
            <label class="block text-slate-400 text-[11px] mb-1 font-semibold">{{ t('chat.dbPragmaTip') }}</label>
            <div class="bg-slate-900 text-slate-200 p-2.5 rounded-lg font-mono text-[10px] leading-relaxed select-all">
              PRAGMA key = "x'{{ dbKeyInfo.key }}'";<br />
              PRAGMA cipher_page_size = 4096;<br />
              PRAGMA cipher_compatibility = 4;
            </div>
          </div>
        </div>

        <div class="pt-2 flex justify-between items-center">
          <button
            @click="fetchDbKey"
            :disabled="isLoadingDbKey"
            class="px-3 py-1.5 rounded-lg text-xs font-medium text-slate-500 hover:text-slate-800 dark:hover:text-white flex items-center gap-1.5"
          >
            <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            <span>重新提取</span>
          </button>
          <button
            @click="showDbKeyModal = false"
            class="px-4 py-1.5 rounded-lg text-xs font-medium bg-slate-100 hover:bg-slate-200 dark:bg-white/10 dark:hover:bg-white/20 text-slate-700 dark:text-white transition-colors"
          >
            关闭
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { toast } from '../utils/dialog'
import { t } from '../utils/i18n'

// 列表 Tab：'recent' (会话) | 'contacts' (通讯录)
const activeListTab = ref('recent')
const searchQuery = ref('')
const isLoadingContacts = ref(false)
const isLoadingMembers = ref(false)
const isSending = ref(false)
const isMemberDrawerOpen = ref(false)
const isGroupsCollapsed = ref(false)
const isFriendsCollapsed = ref(false)

// 数据源
const friends = ref([])
const groups = ref([])
const recentChats = ref([])
const activeChat = ref(null)
const inputMessage = ref('')
const messageScrollRef = ref(null)
const memberSearchQuery = ref('')
const groupMembersMap = ref({}) // key: targetId, value: members array

// 数据库密钥弹窗与历史记录拉取
const showDbKeyModal = ref(false)
const isLoadingDbKey = ref(false)
const dbKeyInfo = ref(null)
const isLoadingHistory = ref(false)

// 本地会话消息存储（支持刷新保持）
const chatHistories = ref({})

// 快捷取首字母/首字
const getInitials = (str) => {
  if (!str) return '?'
  const clean = String(str).trim()
  return clean.slice(0, 1).toUpperCase()
}

// 通信 API 助手
const callAPI = async (action, params = {}) => {
  const token = localStorage.getItem('wx_token')
  const headers = { 'Content-Type': 'application/json' }
  if (token) headers['Authorization'] = `Bearer ${token}`

  const resp = await fetch('/api', {
    method: 'POST',
    headers,
    body: JSON.stringify({
      action,
      params,
      echo: `chat_${Date.now()}`
    })
  })
  return await resp.json()
}

// 打开数据库密钥提取弹窗
const openDbKeyModal = async () => {
  showDbKeyModal.value = true
  if (!dbKeyInfo.value) {
    await fetchDbKey()
  }
}

// 向底层驱动请求提取数据库密钥与信息
const fetchDbKey = async () => {
  isLoadingDbKey.value = true
  try {
    const res = await callAPI('get_db_key')
    if (res && res.data && res.data.key) {
      dbKeyInfo.value = res.data
    } else {
      toast.error('未能提取到数据库密钥，请确认微信已登录')
    }
  } catch (err) {
    toast.error('提取数据库密钥失败: ' + err.message)
  } finally {
    isLoadingDbKey.value = false
  }
}

// 复制密钥到剪贴板
const copyToClipboard = async (text) => {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(t('chat.copied'))
  } catch (err) {
    toast.error('复制失败: ' + err.message)
  }
}

// 拉取当前会话的历史消息记录
const loadHistoryMessages = async () => {
  if (!activeChat.value) return
  isLoadingHistory.value = true
  try {
    const params = {
      chat_id: activeChat.value.targetId,
      count: 20
    }
    if (activeChat.value.isGroup) {
      params.group_id = activeChat.value.onebotId || activeChat.value.targetId
    } else {
      params.user_id = activeChat.value.onebotId || activeChat.value.targetId
    }
    const res = await callAPI('get_history_msg', params)
    const list = res?.data?.messages || (Array.isArray(res?.data) ? res.data : [])
    if (Array.isArray(list) && list.length > 0) {
      const newMsgs = list.map((m) => ({
        id: m.message_id || `hist_${Date.now()}_${Math.random()}`,
        sender: m.sender?.user_id === 'self' || m.user_id === 'self' ? 'self' : 'peer',
        senderName: m.sender?.nickname || m.senderName || activeChat.value.name,
        senderId: m.sender?.user_id || m.user_id || '',
        content: m.raw_message || m.message || (typeof m === 'string' ? m : JSON.stringify(m)),
        time: m.time ? new Date(m.time * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '',
        showTimeDivider: true,
        isRevoked: false
      }))
      const curr = chatHistories.value[activeChat.value.targetId] || []
      chatHistories.value[activeChat.value.targetId] = [...newMsgs, ...curr]
      toast.success(`已加载 ${newMsgs.length} 条历史记录`)
    } else {
      toast.info(t('chat.noMoreHistory'))
    }
  } catch (err) {
    toast.error('拉取历史记录失败: ' + err.message)
  } finally {
    isLoadingHistory.value = false
  }
}

// 加载通讯录与群列表
const loadAllContacts = async () => {
  isLoadingContacts.value = true
  try {
    const [resFriends, resGroups] = await Promise.all([
      callAPI('get_friend_list'),
      callAPI('get_group_list')
    ])

    // 处理好友
    const fList = resFriends.data || []
    friends.value = fList.map((f) => ({
      targetId: f.wxid || f.user_id,
      onebotId: f.user_id,
      name: f.nickname || f.remark || f.wxid || String(f.user_id),
      isGroup: false
    }))

    // 处理群聊
    const gList = resGroups.data || []
    groups.value = gList.map((g) => ({
      targetId: g.chatroom_name || g.group_name_id || g.group_id,
      onebotId: g.group_id,
      name: g.group_name || String(g.group_id),
      isGroup: true
    }))

    // 如果还没有最近会话，默认载入文件传输助手和群聊
    if (recentChats.value.length === 0) {
      initRecentChats()
    }
  } catch (err) {
    toast.error('加载通讯录失败: ' + err.message)
  } finally {
    isLoadingContacts.value = false
  }
}

// 初始化默认最近会话
const initRecentChats = () => {
  const initial = []
  // 1. 文件传输助手
  initial.push({
    targetId: 'filehelper',
    onebotId: '',
    name: '文件传输助手',
    isGroup: false,
    lastMsg: '欢迎使用微信网页版工作台',
    lastTime: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
    unreadCount: 0
  })

  // 2. 加入前几个群和好友
  groups.value.slice(0, 3).forEach((g) => {
    initial.push({
      ...g,
      lastMsg: '点击进入群聊',
      lastTime: '',
      unreadCount: 0
    })
  })
  friends.value.slice(0, 2).forEach((f) => {
    initial.push({
      ...f,
      lastMsg: '点击开始私聊',
      lastTime: '',
      unreadCount: 0
    })
  })

  recentChats.value = initial
  if (recentChats.value.length > 0 && !activeChat.value) {
    selectChat(recentChats.value[0])
  }
}

// 选中某个会话
const selectChat = async (item) => {
  activeChat.value = item
  item.unreadCount = 0

  // 异步上报会话已读 (CGI 251 statusnotify，清空全设备红点)
  if (item.targetId) {
    callAPI('set_read_status', { target: item.targetId }).catch(() => {})
  }

  // 确保在最近列表中
  const existing = recentChats.value.find((c) => c.targetId === item.targetId)
  if (!existing) {
    recentChats.value.unshift({
      ...item,
      lastMsg: '',
      lastTime: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
      unreadCount: 0
    })
  }

  // 初始化当前消息记录
  if (!chatHistories.value[item.targetId]) {
    chatHistories.value[item.targetId] = [
      {
        id: `sys_${Date.now()}`,
        sender: 'peer',
        senderName: item.name,
        senderId: item.targetId,
        content: `已接入与【${item.name}】的会话，支持消息收发与一键撤回。`,
        time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
        showTimeDivider: true,
        isRevoked: false
      }
    ]
  }

  // 若为群聊且未加载过成员列表，拉取群成员
  if (item.isGroup && !groupMembersMap.value[item.targetId]) {
    loadGroupMembers(item.targetId)
  }

  scrollToBottom()
}

// 加载群成员 (基于 get_group_member_list)
const loadGroupMembers = async (groupId) => {
  isLoadingMembers.value = true
  try {
    const res = await callAPI('get_group_member_list', { group_id: groupId })
    if (res.data) {
      groupMembersMap.value[groupId] = res.data
    }
  } catch (err) {
    console.warn('获取群成员失败:', err)
  } finally {
    isLoadingMembers.value = false
  }
}

// 发送消息
const handleSendMessage = async () => {
  const text = inputMessage.value.trim()
  if (!text || !activeChat.value || isSending.value) return

  isSending.value = true
  const nowStr = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  const target = activeChat.value

  try {
    const payload = {
      message: text
    }
    let action = 'send_msg'
    if (target.isGroup) {
      payload.group_id = target.targetId
      payload.message_type = 'group'
      action = 'send_group_msg'
    } else {
      payload.user_id = target.targetId
      payload.message_type = 'private'
      action = 'send_private_msg'
    }

    const res = await callAPI(action, payload)

    if (res.status === 'ok' || res.retcode === 0 || res.code === 200) {
      const msgId = res.data?.message_id || res.message_id
      const newMsg = {
        id: `msg_${Date.now()}`,
        messageId: msgId,
        sender: 'self',
        senderName: '我',
        senderId: 'self',
        content: text,
        time: nowStr,
        showTimeDivider: false,
        isRevoked: false
      }

      if (!chatHistories.value[target.targetId]) {
        chatHistories.value[target.targetId] = []
      }
      chatHistories.value[target.targetId].push(newMsg)

      // 更新最近会话摘要
      const c = recentChats.value.find((x) => x.targetId === target.targetId)
      if (c) {
        c.lastMsg = text
        c.lastTime = nowStr
      }

      inputMessage.value = ''
      scrollToBottom()
      toast.success('消息已送达微信')
    } else {
      toast.error(res.message || res.wording || '发信失败')
    }
  } catch (err) {
    toast.error('网络请求失败: ' + err.message)
  } finally {
    isSending.value = false
  }
}

// 撤回消息 (基于 delete_msg / revoke_msg)
const handleRevokeMessage = async (msg) => {
  if (!msg.messageId) {
    toast.error('该消息缺少 message_id，无法撤回')
    return
  }

  try {
    const res = await callAPI('delete_msg', {
      message_id: msg.messageId
    })

    if (res.status === 'ok' || res.retcode === 0 || res.code === 200) {
      msg.isRevoked = true
      toast.success('已成功发起消息撤回')
    } else {
      toast.error('撤回失败: ' + (res.message || res.wording || '未知错误'))
    }
  } catch (err) {
    toast.error('撤回调用出错: ' + err.message)
  }
}

// 复制消息
const copyText = (text) => {
  navigator.clipboard.writeText(text)
  toast.success('已复制到剪贴板')
}

// 插入 @ 昵称
const insertAt = (nick) => {
  inputMessage.value += `@${nick} `
  isMemberDrawerOpen.value = false
}

// 打开快速 @ 菜单
const openAtPicker = () => {
  isMemberDrawerOpen.value = true
}

// 滚动到底部
const scrollToBottom = () => {
  nextTick(() => {
    if (messageScrollRef.value) {
      messageScrollRef.value.scrollTop = messageScrollRef.value.scrollHeight
    }
  })
}

// 清空当前消息记录
const clearActiveHistory = () => {
  if (!activeChat.value) return
  chatHistories.value[activeChat.value.targetId] = []
  toast.info('已清空当前聊天记录')
}

// 切换折叠
const toggleGroupCollapse = (key) => {
  if (key === 'groups') isGroupsCollapsed.value = !isGroupsCollapsed.value
  if (key === 'friends') isFriendsCollapsed.value = !isFriendsCollapsed.value
}

// 计算属性：当前活跃会话的消息
const currentMessages = computed(() => {
  if (!activeChat.value) return []
  return chatHistories.value[activeChat.value.targetId] || []
})

// 计算属性：当前群的成员列表
const currentGroupMembers = computed(() => {
  if (!activeChat.value || !activeChat.value.isGroup) return []
  return groupMembersMap.value[activeChat.value.targetId] || []
})

// 过滤后的群成员
const filteredGroupMembers = computed(() => {
  const list = currentGroupMembers.value
  if (!memberSearchQuery.value.trim()) return list
  const q = memberSearchQuery.value.toLowerCase()
  return list.filter((m) =>
    (m.nickname && m.nickname.toLowerCase().includes(q)) ||
    (m.card && m.card.toLowerCase().includes(q)) ||
    (m.user_id && String(m.user_id).toLowerCase().includes(q))
  )
})

// 过滤后的最近会话列表
const filteredRecentChats = computed(() => {
  if (!searchQuery.value.trim()) return recentChats.value
  const q = searchQuery.value.toLowerCase()
  return recentChats.value.filter((c) =>
    c.name.toLowerCase().includes(q) || c.targetId.toLowerCase().includes(q)
  )
})

// 过滤后的群聊列表
const filteredGroups = computed(() => {
  if (!searchQuery.value.trim()) return groups.value
  const q = searchQuery.value.toLowerCase()
  return groups.value.filter((g) =>
    g.name.toLowerCase().includes(q) || g.targetId.toLowerCase().includes(q)
  )
})

// 过滤后的好友列表
const filteredFriends = computed(() => {
  if (!searchQuery.value.trim()) return friends.value
  const q = searchQuery.value.toLowerCase()
  return friends.value.filter((f) =>
    f.name.toLowerCase().includes(q) || f.targetId.toLowerCase().includes(q)
  )
})

// 总未读计数
const totalUnreadCount = computed(() => {
  return recentChats.value.reduce((acc, c) => acc + (c.unreadCount || 0), 0)
})

// 监听 WebSocket 事件 (实时捕获外部消息与撤回)
const handleIncomingOneBotEvent = (e) => {
  const eventData = e.detail
  if (!eventData) return

  // 1. 消息接收
  if (eventData.post_type === 'message') {
    const isGrp = eventData.message_type === 'group'
    const targetId = isGrp ? String(eventData.group_id) : String(eventData.user_id)
    const senderName = eventData.sender?.nickname || eventData.sender?.card || String(eventData.user_id)
    const rawMsg = eventData.raw_message || eventData.message || ''
    const nowStr = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

    const incoming = {
      id: `recv_${Date.now()}`,
      messageId: eventData.message_id,
      sender: 'peer',
      senderName,
      senderId: String(eventData.user_id),
      content: typeof rawMsg === 'string' ? rawMsg : JSON.stringify(rawMsg),
      time: nowStr,
      showTimeDivider: false,
      isRevoked: false
    }

    if (!chatHistories.value[targetId]) {
      chatHistories.value[targetId] = []
    }
    chatHistories.value[targetId].push(incoming)

    // 更新最近会话
    let chatItem = recentChats.value.find((c) => c.targetId === targetId)
    if (!chatItem) {
      chatItem = {
        targetId,
        onebotId: isGrp ? eventData.group_id : eventData.user_id,
        name: isGrp ? `群聊 ${targetId}` : senderName,
        isGroup: isGrp,
        lastMsg: rawMsg,
        lastTime: nowStr,
        unreadCount: activeChat.value?.targetId === targetId ? 0 : 1
      }
      recentChats.value.unshift(chatItem)
    } else {
      chatItem.lastMsg = rawMsg
      chatItem.lastTime = nowStr
      if (activeChat.value?.targetId !== targetId) {
        chatItem.unreadCount = (chatItem.unreadCount || 0) + 1
      }
    }

    if (activeChat.value?.targetId === targetId) {
      scrollToBottom()
    }
  }

  // 2. 撤回通知 (notice: group_recall / friend_recall)
  if (eventData.notice_type === 'group_recall' || eventData.notice_type === 'friend_recall') {
    const targetMsgId = eventData.message_id
    Object.values(chatHistories.value).forEach((history) => {
      const match = history.find((m) => m.messageId === targetMsgId)
      if (match) {
        match.isRevoked = true
      }
    })
  }
}

onMounted(() => {
  loadAllContacts()
  window.addEventListener('wx_onebot_event', handleIncomingOneBotEvent)
})

onUnmounted(() => {
  window.removeEventListener('wx_onebot_event', handleIncomingOneBotEvent)
})
</script>

<style scoped>
/* 滚动条美化 */
::-webkit-scrollbar {
  width: 4px;
}
::-webkit-scrollbar-thumb {
  background: rgba(150, 150, 150, 0.2);
  border-radius: 4px;
}
</style>

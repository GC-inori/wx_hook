import { reactive } from 'vue'

export const themeState = reactive({
  mode: localStorage.getItem('wx_theme_mode') || 'system', // 'light' | 'dark' | 'system'
  isDarkEffective: true,
  bgImage: localStorage.getItem('wx_bg_image') || '',
})

let mediaQueryListener = null

export const updateEffectiveDark = () => {
  if (themeState.mode === 'system') {
    const prefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches
    themeState.isDarkEffective = prefersDark
  } else {
    themeState.isDarkEffective = themeState.mode === 'dark'
  }

  const root = document.documentElement
  if (themeState.isDarkEffective) {
    root.classList.add('dark')
    root.classList.remove('light')
  } else {
    root.classList.add('light')
    root.classList.remove('dark')
  }
}

export const setThemeMode = (mode) => {
  if (['light', 'dark', 'system'].includes(mode)) {
    themeState.mode = mode
    localStorage.setItem('wx_theme_mode', mode)
    updateEffectiveDark()
  }
}

export const toggleThemeMode = () => {
  const modes = ['light', 'dark', 'system']
  const nextIdx = (modes.indexOf(themeState.mode) + 1) % modes.length
  setThemeMode(modes[nextIdx])
}

export const setBgImage = (url) => {
  themeState.bgImage = url
  if (url) {
    localStorage.setItem('wx_bg_image', url)
  } else {
    localStorage.removeItem('wx_bg_image')
  }
}

export const initTheme = () => {
  if (window.matchMedia) {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    mediaQueryListener = () => {
      if (themeState.mode === 'system') {
        updateEffectiveDark()
      }
    }
    mq.addEventListener('change', mediaQueryListener)
  }
  updateEffectiveDark()
}

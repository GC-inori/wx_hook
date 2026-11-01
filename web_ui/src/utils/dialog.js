import { reactive } from 'vue'

export const toastState = reactive({
  toasts: []
})

let toastId = 0

export const toast = {
  success(message, duration = 3000) {
    this.add({ type: 'success', message, duration })
  },
  error(message, duration = 3500) {
    this.add({ type: 'error', message, duration })
  },
  info(message, duration = 3000) {
    this.add({ type: 'info', message, duration })
  },
  add({ type, message, duration }) {
    const id = ++toastId
    toastState.toasts.push({ id, type, message })
    if (duration > 0) {
      setTimeout(() => {
        this.remove(id)
      }, duration)
    }
  },
  remove(id) {
    const index = toastState.toasts.findIndex(t => t.id === id)
    if (index !== -1) {
      toastState.toasts.splice(index, 1)
    }
  }
}

export const dialogState = reactive({
  visible: false,
  title: '',
  content: '',
  confirmText: '确定',
  cancelText: '取消',
  showCancel: true,
  onConfirm: null,
  onCancel: null
})

export const dialog = {
  confirm({ title = '提示', content, confirmText = '确定', cancelText = '取消', onConfirm, onCancel }) {
    dialogState.visible = true
    dialogState.title = title
    dialogState.content = content
    dialogState.confirmText = confirmText
    dialogState.cancelText = cancelText
    dialogState.showCancel = true
    dialogState.onConfirm = () => {
      dialogState.visible = false
      if (onConfirm) onConfirm()
    }
    dialogState.onCancel = () => {
      dialogState.visible = false
      if (onCancel) onCancel()
    }
  },
  alert({ title = '提示', content, confirmText = '我知道了', onConfirm }) {
    dialogState.visible = true
    dialogState.title = title
    dialogState.content = content
    dialogState.confirmText = confirmText
    dialogState.showCancel = false
    dialogState.onConfirm = () => {
      dialogState.visible = false
      if (onConfirm) onConfirm()
    }
    dialogState.onCancel = () => {
      dialogState.visible = false
    }
  },
  close() {
    dialogState.visible = false
  }
}

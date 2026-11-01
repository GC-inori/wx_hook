<template>
  <div
    ref="cardRef"
    :class="[
      'relative overflow-hidden napcat-glass rounded-3xl transition-shadow duration-300',
      className
    ]"
    :style="cardStyle"
    @mouseenter="onMouseEnter"
    @mouseleave="onMouseLeave"
    @mousemove="onMouseMove"
  >
    <!-- 动态跟随光晕 (NapCat 原版实现) -->
    <div
      v-show="isShowLight"
      class="absolute rounded-full blur-[100px] pointer-events-none transition-opacity duration-300 bg-gradient-to-r from-primary/30 to-secondary/30 w-[160px] h-[160px] -z-0"
      :style="{
        left: `${lightPos.left}px`,
        top: `${lightPos.top}px`
      }"
    ></div>

    <div class="relative z-10 w-full h-full">
      <slot></slot>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'

const props = defineProps({
  className: {
    type: String,
    default: ''
  },
  maxXRotation: {
    type: Number,
    default: 4
  },
  maxYRotation: {
    type: Number,
    default: 4
  }
})

const cardRef = ref(null)
const isShowLight = ref(false)
const lightPos = reactive({ left: 0, top: 0 })
const cardStyle = ref({
  willChange: 'transform',
  transform: 'perspective(1000px) rotateX(0deg) rotateY(0deg) scale3d(1, 1, 1)'
})

const onMouseEnter = () => {
  if (cardRef.value) {
    cardRef.value.style.transition = 'transform 0.3s ease-out'
  }
}

const onMouseLeave = () => {
  isShowLight.value = false
  if (cardRef.value) {
    cardRef.value.style.transition = 'transform 0.5s cubic-bezier(0.2, 0.8, 0.2, 1)'
    cardRef.value.style.transform = 'perspective(1000px) rotateX(0deg) rotateY(0deg) scale3d(1, 1, 1)'
  }
}

const onMouseMove = (e) => {
  if (!cardRef.value) return
  isShowLight.value = true

  const rect = cardRef.value.getBoundingClientRect()
  const offsetX = e.clientX - rect.left
  const offsetY = e.clientY - rect.top

  lightPos.left = offsetX - 80
  lightPos.top = offsetY - 80

  cardRef.value.style.transition = 'transform 0.08s ease-out'

  const centerX = rect.width / 2
  const centerY = rect.height / 2

  const rotateX = ((offsetY - centerY) / centerY) * -props.maxXRotation
  const rotateY = ((offsetX - centerX) / centerX) * props.maxYRotation

  cardRef.value.style.transform = `perspective(1000px) rotateX(${rotateX.toFixed(2)}deg) rotateY(${rotateY.toFixed(2)}deg) scale3d(1.008, 1.008, 1.008)`
}
</script>

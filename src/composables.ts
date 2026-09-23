import { computed, onMounted, onUnmounted, ref } from 'vue'

/** 全局偏好：减弱动态效果 */
export const reducedMotion = ref(false)

let observer: MediaQueryList | null = null

export function usePrefersReducedMotion() {
  onMounted(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return
    observer = window.matchMedia('(prefers-reduced-motion: reduce)')
    reducedMotion.value = observer.matches
    const onChange = (e: MediaQueryListEvent) => {
      reducedMotion.value = e.matches
    }
    observer.addEventListener?.('change', onChange)
    onUnmounted(() => observer?.removeEventListener?.('change', onChange))
  })
}

/** 元素进入视口时触发一次 */
export function useInView<T extends HTMLElement>(options?: IntersectionObserverInit) {
  const target = ref<T | null>(null)
  const inView = ref(false)
  let io: IntersectionObserver | null = null

  onMounted(() => {
    if (!target.value || typeof IntersectionObserver === 'undefined') {
      inView.value = true
      return
    }
    io = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            inView.value = true
            io?.disconnect()
          }
        }
      },
      { rootMargin: '0px 0px -12% 0px', threshold: 0.08, ...options }
    )
    io.observe(target.value)
  })

  onUnmounted(() => io?.disconnect())

  return { target, inView }
}

/** 世界时钟：按真实时间推进的演示节拍器 */
export function useTicker(intervalMs = 1000) {
  const tick = ref(0)
  let timer: number | undefined
  onMounted(() => {
    timer = window.setInterval(() => tick.value++, intervalMs)
  })
  onUnmounted(() => window.clearInterval(timer))
  return tick
}

export function useNow(intervalMs = 1000) {
  const now = ref(new Date())
  let timer: number | undefined
  onMounted(() => {
    timer = window.setInterval(() => (now.value = new Date()), intervalMs)
  })
  onUnmounted(() => window.clearInterval(timer))
  return now
}

/** 滚动进度 0–1 */
export function useScrollProgress() {
  const progress = ref(0)
  const onScroll = () => {
    const max = document.documentElement.scrollHeight - window.innerHeight
    progress.value = max > 0 ? Math.min(1, Math.max(0, window.scrollY / max)) : 0
  }
  onMounted(() => {
    onScroll()
    window.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('resize', onScroll)
  })
  onUnmounted(() => {
    window.removeEventListener('scroll', onScroll)
    window.removeEventListener('resize', onScroll)
  })
  return progress
}

/** 可靠地滚动到容器底部 */
export function scrollToBottom(el: HTMLElement, smooth = true) {
  el.scrollTo({
    top: el.scrollHeight,
    behavior: smooth && !reducedMotion.value ? 'smooth' : 'auto'
  })
}

/** 已滚动到底部附近 */
export function isNearBottom(el: HTMLElement, threshold = 96) {
  return el.scrollHeight - el.scrollTop - el.clientHeight < threshold
}

/** 数字滚动到目标值 */
export function useCountUp(target: () => number, duration = 900) {
  const value = ref(0)
  let raf = 0
  const start = performance.now()
  const from = 0

  const step = (t: number) => {
    const p = Math.min(1, (t - start) / duration)
    const eased = 1 - Math.pow(1 - p, 3)
    value.value = Math.round(from + (target() - from) * eased)
    if (p < 1) raf = requestAnimationFrame(step)
  }

  onMounted(() => {
    if (reducedMotion.value) {
      value.value = target()
      return
    }
    raf = requestAnimationFrame(step)
  })
  onUnmounted(() => cancelAnimationFrame(raf))

  return value
}

/** 计算属性简写 */
export const c = computed

/* ------------------------------------------------ 叙事流：阅读锚点与草稿 */

/**
 * 软键盘安全区：
 * 键盘弹起时（visualViewport 收缩）标记 expanded，
 * 让输入区改用视觉视口高度而不是布局视口高度。
 */
export function useKeyboardInset() {
  const inset = ref(0)
  const expanded = ref(false)
  let vv: VisualViewport | null = null

  const measure = () => {
    if (!vv) return
    const gap = Math.max(0, window.innerHeight - vv.height - vv.offsetTop)
    inset.value = gap
    expanded.value = gap > 80
  }

  onMounted(() => {
    vv = window.visualViewport ?? null
    if (!vv) return
    measure()
    vv.addEventListener('resize', measure)
    vv.addEventListener('scroll', measure)
    onUnmounted(() => {
      vv?.removeEventListener('resize', measure)
      vv?.removeEventListener('scroll', measure)
    })
  })

  return { inset, expanded }
}

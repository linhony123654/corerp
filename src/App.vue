<script setup lang="ts">
/**
 * CoreRP — 双工作空间外壳
 *
 * Story（玩家）与 Inspector（观测）并列，不是「首页 + 子页面」：
 * 两边都可以随时进入对方，故事流的阅读锚点与草稿在离开时不丢。
 */
import { computed, ref, watch } from 'vue'
import StoryWorkspace from './components/StoryWorkspace.vue'
import InspectorWorkspace from './components/InspectorWorkspace.vue'
import { usePrefersReducedMotion } from './composables'

usePrefersReducedMotion()

type Workspace = 'story' | 'inspector'

const workspace = ref<Workspace>('story')
const pendingRef = ref<string | null>(null)

/** 持久化初始视图：刷新后回到故事流（读历史由用户主动完成） */
const initial = ((): Workspace => {
  try {
    const v = sessionStorage.getItem('corerp.workspace')
    return v === 'inspector' ? 'inspector' : 'story'
  } catch {
    return 'story'
  }
})()

workspace.value = initial

/** 首次真正进入某工作空间时才挂载：之后用 v-show 保持存活，状态不丢 */
const storyMounted = ref(initial !== 'inspector')
const inspectorMounted = ref(initial === 'inspector')

watch(workspace, (v) => {
  if (v === 'story') storyMounted.value = true
  else inspectorMounted.value = true
  try {
    sessionStorage.setItem('corerp.workspace', v)
  } catch {
    /* 忽略 */
  }
})

function enterInspector(refId: string | null) {
  pendingRef.value = refId
  workspace.value = 'inspector'
}

function backToStory() {
  workspace.value = 'story'
}

function consumeRef() {
  pendingRef.value = null
}

const label = computed(() => (workspace.value === 'story' ? '叙事流' : '观测台'))
</script>

<template>
  <div class="root">
    <!--
      工作空间切换用 v-show 而不是 v-if：
      故事流的历史、草稿、滚动锚点必须跨切换存活（v-if 会卸载并丢失 setup 状态）。
      观测台同理保留脊梁选中与因果追溯。
    -->
    <div v-show="workspace === 'story'" class="root__pane">
      <StoryWorkspace
        v-if="storyMounted"
        :pending-ref="pendingRef"
        @enter-inspector="enterInspector"
        @consume-ref="consumeRef"
      />
    </div>
    <div v-show="workspace !== 'story'" class="root__pane">
      <InspectorWorkspace v-if="inspectorMounted" :focus-record-id="pendingRef" @back-to-story="backToStory" />
    </div>

    <p class="sr-only" aria-live="polite">当前工作空间：{{ label }}</p>
  </div>
</template>

<style scoped>
.root {
  position: relative;
}

/* v-show 的面板：隐藏时不能占位、不能抢走焦点 */
.root__pane[hidden],
.root__pane[style*='display: none'] {
  visibility: hidden;
  pointer-events: none;
}
</style>

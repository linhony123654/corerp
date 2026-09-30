<script setup lang="ts">
withDefaults(defineProps<{ name: string; size?: number }>(), { size: 21 })
</script>

<template>
  <svg class="icon" :width="size" :height="size" aria-hidden="true"><use :href="`#play-i-${name}`" /></svg>
</template>

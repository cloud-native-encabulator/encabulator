<script setup lang="ts">
/**
 * A page that is not a plate on this panel.
 */
import { RouterLink, useRoute } from 'vue-router'
import PanelPlate from '@/components/panel/PanelPlate.vue'

const route = useRoute()
</script>

<template>
  <div class="mx-auto max-w-xl">
    <PanelPlate legend="No such plate" sublegend="This console has seven pages, and that is not one">
      <p class="typed text-[0.85rem] text-ink-soft">
        Nothing is mounted at <code>{{ route.fullPath }}</code>. The panel’s plates are the Factory
        Floor, Grammeters, Malaise, Scenarios, Girdlespring, Works and Administration.
      </p>
      <RouterLink to="/" class="knob mt-3 inline-block">Back to the factory floor</RouterLink>
    </PanelPlate>
  </div>
</template>

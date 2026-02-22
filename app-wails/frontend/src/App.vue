<script setup lang="ts">
import { ref } from 'vue'
import SshTab from './components/SshTab.vue'
import P2pTab from './components/P2pTab.vue'

type TabId = 'ssh' | 'p2p'
const activeTab = ref<TabId>('ssh')
const tabs: { id: TabId; label: string }[] = [
  { id: 'ssh', label: 'SSH Tunnel' },
  { id: 'p2p', label: 'P2P Direct' },
]
</script>

<template>
  <div class="flex flex-col h-screen bg-gray-900 text-gray-100 overflow-hidden select-none">
    <!-- Tab bar / title bar -->
    <nav
      class="flex items-end gap-0 px-4 pt-3 border-b border-gray-700/70 flex-shrink-0 bg-gray-900"
      style="--wails-draggable: drag"
    >
      <span class="text-sm font-bold mr-6 mb-2.5 text-indigo-400 tracking-tight">⚡ Direct Connector</span>
      <button
        v-for="tab in tabs"
        :key="tab.id"
        @click="activeTab = tab.id"
        class="px-5 pb-2.5 pt-1.5 text-sm font-medium border-b-2 -mb-px transition-all duration-150 cursor-default outline-none"
        :class="
          activeTab === tab.id
            ? 'border-indigo-400 text-white'
            : 'border-transparent text-gray-500 hover:text-gray-300 hover:border-gray-600'
        "
      >
        {{ tab.label }}
      </button>
    </nav>

    <!-- Tab content -->
    <div class="flex-1 min-h-0">
      <SshTab v-show="activeTab === 'ssh'" class="h-full" />
      <P2pTab v-show="activeTab === 'p2p'" class="h-full" />
    </div>
  </div>
</template>

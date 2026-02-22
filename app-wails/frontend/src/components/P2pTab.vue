<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import {
  P2PGenerateOffer, P2PAcceptAnswer, P2PProvideAnswer, P2PStop, P2PIsActive,
  CopyToClipboard,
} from '../../wailsjs/go/main/App'

// ── State ────────────────────────────────────────────────────────────────────
type Role = 'consumer' | 'provider'
const role       = ref<Role>('consumer')
const ports      = ref('1883')
const outboundSDP = ref('')   // what this side generated (to copy)
const inboundSDP  = ref('')   // what user pastes (from other side)
const active     = ref(false)
const busy       = ref(false) // ICE gathering in progress
const status     = ref('Status: Idle')
const logs       = ref<string[]>([])
const logBox     = ref<HTMLElement | null>(null)

// ── Computed ─────────────────────────────────────────────────────────────────
const statusClass = computed(() => {
  if (status.value.includes('CONNECTED'))    return 'bg-emerald-500/15 text-emerald-400 border-emerald-500/40'
  if (status.value.includes('Connecting') || status.value.includes('waiting'))
    return 'bg-yellow-500/15 text-yellow-400 border-yellow-500/40'
  return 'bg-gray-700/50 text-gray-400 border-gray-600/50'
})

// ── Helpers ───────────────────────────────────────────────────────────────────
function appendLog(msg: string) {
  logs.value.push(msg)
  if (logs.value.length > 300) logs.value = logs.value.slice(-300)
  nextTick(() => { if (logBox.value) logBox.value.scrollTop = logBox.value.scrollHeight })
}

function resetSDP() { outboundSDP.value = ''; inboundSDP.value = '' }

// ── Lifecycle ─────────────────────────────────────────────────────────────────
onMounted(async () => {
  active.value = await P2PIsActive()
  EventsOn('p2p:status', (s: string)   => { status.value = s })
  EventsOn('p2p:log',    (msg: string) => appendLog(msg))
  appendLog('P2P ready. Select Consumer or Provider and follow the steps.')
})

onUnmounted(() => {
  EventsOff('p2p:status')
  EventsOff('p2p:log')
})

// ── Consumer actions ──────────────────────────────────────────────────────────
async function genOffer() {
  if (!ports.value.trim()) { appendLog('⚠  Enter at least one port first.'); return }
  busy.value = true
  resetSDP()
  const result = await P2PGenerateOffer(ports.value.trim())
  busy.value = false
  if (result.startsWith('ERROR:')) { appendLog(result); return }
  outboundSDP.value = result
  active.value = true
  appendLog('Offer ready — copy it and paste into the Provider.')
}

async function applyAnswer() {
  if (!inboundSDP.value.trim()) { appendLog("⚠  Paste the Provider's answer first."); return }
  const err = await P2PAcceptAnswer(inboundSDP.value.trim())
  if (err) { appendLog(err); return }
  appendLog('Answer applied — direct P2P connection forming...')
}

// ── Provider actions ──────────────────────────────────────────────────────────
async function genAnswer() {
  if (!inboundSDP.value.trim()) { appendLog("⚠  Paste the Consumer's offer first."); return }
  busy.value = true
  outboundSDP.value = ''
  const result = await P2PProvideAnswer(inboundSDP.value.trim())
  busy.value = false
  if (result.startsWith('ERROR:')) { appendLog(result); return }
  outboundSDP.value = result
  active.value = true
  appendLog('Answer ready — copy and send it back to the Consumer.')
}

// ── Stop ──────────────────────────────────────────────────────────────────────
async function stopSession() {
  await P2PStop()
  active.value = false
  busy.value = false
  resetSDP()
  appendLog('Session stopped.')
}

async function copyText(text: string) {
  await CopyToClipboard(text)
  appendLog('Copied to clipboard.')
}
</script>

<template>
  <div class="flex h-full overflow-hidden">

    <!-- ── Left: P2P config ────────────────────────────────────────────────── -->
    <div class="w-80 flex-shrink-0 overflow-y-auto p-4 space-y-3 border-r border-gray-700/60 bg-gray-900">

      <!-- Role selector -->
      <div class="bg-gray-800/50 border border-gray-700/60 rounded-xl p-4 space-y-3">
        <h3 class="text-xs font-semibold text-gray-500 uppercase tracking-wider">Role</h3>
        <div class="grid grid-cols-2 gap-2">
          <button
            v-for="r in (['consumer','provider'] as Role[])"
            :key="r"
            @click="if(!active && !busy){ role = r; resetSDP() }"
            class="py-2 rounded-lg text-sm font-medium transition-colors cursor-default"
            :class="role === r
              ? 'bg-indigo-600 text-white'
              : 'bg-gray-700 text-gray-400 hover:text-gray-200'"
          >
            {{ r === 'consumer' ? '📥 Consumer' : '📤 Provider' }}
          </button>
        </div>
        <p class="text-xs text-gray-500 leading-relaxed">
          <template v-if="role === 'consumer'">
            <b class="text-gray-400">Consumer</b> opens local ports that tunnel to the Provider.
            Typically the machine that needs access (laptop/desktop).
          </template>
          <template v-else>
            <b class="text-gray-400">Provider</b> bridges incoming channels to local services.
            Typically the server or WSL machine running Docker.
          </template>
        </p>
      </div>

      <!-- Consumer: Ports -->
      <div v-if="role === 'consumer'" class="bg-gray-800/50 border border-gray-700/60 rounded-xl p-4 space-y-3">
        <h3 class="text-xs font-semibold text-gray-500 uppercase tracking-wider">Ports to Forward</h3>
        <input v-model="ports" :disabled="active || busy" placeholder="1883, 3391"
          class="w-full bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm text-white placeholder-gray-600 focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500/40 disabled:opacity-40 transition-colors" />
        <p class="text-xs text-gray-500">These local ports will forward to matching ports on the Provider.</p>
      </div>

      <!-- Step 1 (Consumer: Generate Offer / Provider: Paste Offer) -->
      <div class="bg-gray-800/50 border border-gray-700/60 rounded-xl p-4 space-y-3">
        <h3 class="text-xs font-semibold text-gray-500 uppercase tracking-wider">
          {{ role === 'consumer' ? 'Step 1 — Generate Offer' : 'Step 1 — Paste Offer' }}
        </h3>

        <!-- Consumer: generate offer -->
        <template v-if="role === 'consumer'">
          <button @click="genOffer" :disabled="active || busy"
            class="w-full py-2 text-sm font-semibold rounded-lg transition-all"
            :class="busy
              ? 'bg-yellow-600/20 border border-yellow-500/40 text-yellow-400 animate-pulse'
              : 'bg-indigo-600 hover:bg-indigo-500 text-white disabled:opacity-40'">
            {{ busy ? '⏳ Gathering ICE candidates…' : '⚡ Generate Offer' }}
          </button>
          <div v-if="outboundSDP" class="space-y-2">
            <label class="block text-xs text-gray-500">Offer SDP — copy and send to Provider:</label>
            <textarea :value="outboundSDP" readonly rows="4"
              class="w-full bg-gray-900 border border-gray-700/60 rounded-lg px-3 py-2 text-xs font-mono text-indigo-300 resize-none focus:outline-none" />
            <button @click="copyText(outboundSDP)"
              class="w-full py-1.5 text-xs font-medium rounded-lg bg-gray-700 hover:bg-gray-600 text-gray-200 transition-colors">
              ⎘ Copy Offer
            </button>
          </div>
        </template>

        <!-- Provider: paste offer -->
        <template v-else>
          <label class="block text-xs text-gray-500">Paste the Consumer's offer SDP here:</label>
          <textarea v-model="inboundSDP" :disabled="active || busy" rows="4" placeholder="Paste offer here…"
            class="w-full bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-xs font-mono text-gray-300 placeholder-gray-600 resize-none focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500/40 disabled:opacity-40 transition-colors" />
          <button @click="genAnswer" :disabled="active || busy || !inboundSDP.trim()"
            class="w-full py-2 text-sm font-semibold rounded-lg transition-all"
            :class="busy
              ? 'bg-yellow-600/20 border border-yellow-500/40 text-yellow-400 animate-pulse'
              : 'bg-indigo-600 hover:bg-indigo-500 text-white disabled:opacity-40'">
            {{ busy ? '⏳ Gathering ICE candidates…' : '⚡ Generate Answer' }}
          </button>
          <div v-if="outboundSDP" class="space-y-2">
            <label class="block text-xs text-gray-500">Answer SDP — copy and send back to Consumer:</label>
            <textarea :value="outboundSDP" readonly rows="4"
              class="w-full bg-gray-900 border border-gray-700/60 rounded-lg px-3 py-2 text-xs font-mono text-indigo-300 resize-none focus:outline-none" />
            <button @click="copyText(outboundSDP)"
              class="w-full py-1.5 text-xs font-medium rounded-lg bg-gray-700 hover:bg-gray-600 text-gray-200 transition-colors">
              ⎘ Copy Answer
            </button>
          </div>
        </template>
      </div>

      <!-- Step 2 (Consumer only: Paste Answer) -->
      <div v-if="role === 'consumer' && outboundSDP" class="bg-gray-800/50 border border-gray-700/60 rounded-xl p-4 space-y-3">
        <h3 class="text-xs font-semibold text-gray-500 uppercase tracking-wider">Step 2 — Apply Answer</h3>
        <label class="block text-xs text-gray-500">Paste the Provider's answer SDP here:</label>
        <textarea v-model="inboundSDP" rows="4" placeholder="Paste answer here…"
          class="w-full bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-xs font-mono text-gray-300 placeholder-gray-600 resize-none focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500/40 transition-colors" />
        <button @click="applyAnswer" :disabled="!inboundSDP.trim()"
          class="w-full py-2 text-sm font-semibold rounded-lg bg-emerald-700 hover:bg-emerald-600 text-white transition-colors disabled:opacity-40">
          ✔  Apply Answer
        </button>
      </div>

    </div>

    <!-- ── Right: Status + Log ─────────────────────────────────────────────── -->
    <div class="flex-1 flex flex-col p-4 gap-3 min-w-0 bg-gray-900">

      <!-- Status + Stop button -->
      <div class="flex items-center gap-3 flex-shrink-0">
        <span
          class="flex-1 text-xs font-medium px-3 py-1.5 rounded-full border text-center truncate"
          :class="statusClass"
        >{{ status }}</span>
        <button v-if="active || busy" @click="stopSession"
          class="px-6 py-1.5 rounded-full text-sm font-semibold bg-red-600/20 border border-red-500/50 text-red-400 hover:bg-red-600/30 transition-all flex-shrink-0">
          ⏹  Stop
        </button>
      </div>

      <!-- Privacy note -->
      <div class="bg-gray-800/30 border border-gray-700/40 rounded-lg px-4 py-2.5 flex-shrink-0">
        <p class="text-xs text-gray-500 leading-relaxed">
          <span class="text-gray-400 font-medium">🔒 100% serverless.</span>
          STUN is used once to discover your public IP — no traffic ever touches it.
          After the SDP exchange, all data flows <span class="text-gray-400">directly</span> between the two machines, end-to-end encrypted.
        </p>
      </div>

      <!-- Log output -->
      <pre
        ref="logBox"
        class="flex-1 overflow-y-auto bg-black/50 rounded-xl p-4 text-xs font-mono text-green-400 leading-relaxed border border-gray-800/80 whitespace-pre-wrap"
      >{{ logs.join('\n') }}</pre>
    </div>
  </div>
</template>

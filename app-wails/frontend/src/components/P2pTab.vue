<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import {
  P2PGenerateOffer, P2PAcceptAnswer, P2PProvideAnswer, P2PStop, P2PIsActive,
  P2PSetConfig,
  RelayStart, RelayStop, RelayIsRunning, RelayGetCredentials,
  CopyToClipboard, GetPublicIP,
} from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import { validatePort, validatePorts } from '../utils/portValidation'

// ── Types ─────────────────────────────────────────────────────────────────────
type Role = 'consumer' | 'provider'
interface TURNEntry { url: string; username: string; credential: string }

// ── P2P state ─────────────────────────────────────────────────────────────────
const role        = ref<Role>('consumer')
const ports       = ref('1883')
const outboundSDP = ref('')
const inboundSDP  = ref('')
const active      = ref(false)
const busy        = ref(false)
const status      = ref('Status: Idle')
const logs        = ref<string[]>([])
const logBox      = ref<HTMLElement | null>(null)

// ── Relay state ───────────────────────────────────────────────────────────────
const relayPort    = ref(3478)
const relayBusy    = ref(false)
const relayRunning = ref(false)
const relayAddr    = ref('')
const relayTcpAddr = ref('')
const relayUser    = ref('')
const relayPass    = ref('')
const publicIP     = ref('')

// ── Advanced / TURN config ────────────────────────────────────────────────────
const showAdvanced     = ref(false)
// Default to 443 so ICE-TCP candidates advertise port 443 — identical to HTTPS
// from a firewall's perspective, the same trick browsers and Teams use.
const tcpMuxPort       = ref(443)
const gatherTimeoutSecs = ref(30)
const turnEntries      = ref<TURNEntry[]>([])  // user-added TURN servers

// ── Computed ──────────────────────────────────────────────────────────────────
const statusClass = computed(() => {
  if (status.value.includes('CONNECTED'))    return 'bg-emerald-500/15 text-emerald-400 border-emerald-500/40'
  if (status.value.includes('Connecting') || status.value.includes('waiting'))
    return 'bg-yellow-500/15 text-yellow-400 border-yellow-500/40'
  return 'bg-gray-700/50 text-gray-400 border-gray-600/50'
})

// Merge relay creds into TURN entries list when relay is running
function buildSessionConfig(): main.P2PSessionConfig {
  const servers: main.P2PTURNServer[] = []
  if (relayRunning.value && relayAddr.value) {
    // Push both UDP and TCP transports so clients behind UDP-blocking firewalls
    // can still reach the relay over TCP.
    servers.push(main.P2PTURNServer.createFrom({ url: relayAddr.value, username: relayUser.value, credential: relayPass.value }))
    if (relayTcpAddr.value) {
      servers.push(main.P2PTURNServer.createFrom({ url: relayTcpAddr.value, username: relayUser.value, credential: relayPass.value }))
    }
  }
  for (const e of turnEntries.value) {
    if (e.url.trim()) servers.push(main.P2PTURNServer.createFrom({ url: e.url.trim(), username: e.username, credential: e.credential }))
  }
  return main.P2PSessionConfig.createFrom({
    turnServers: servers,
    tcpMuxPort: tcpMuxPort.value,
    gatherTimeoutSecs: gatherTimeoutSecs.value,
  })
}

// ── Helpers ───────────────────────────────────────────────────────────────────
function appendLog(msg: string) {
  logs.value.push(msg)
  if (logs.value.length > 300) logs.value = logs.value.slice(-300)
  nextTick(() => { if (logBox.value) logBox.value.scrollTop = logBox.value.scrollHeight })
}

function resetSDP() { outboundSDP.value = ''; inboundSDP.value = '' }

let relayRefreshing = false
async function refreshRelayState() {
  if (relayRefreshing) return
  relayRefreshing = true
  try {
    relayRunning.value = await RelayIsRunning()
    if (relayRunning.value) {
      const creds = await RelayGetCredentials()
      relayAddr.value    = creds.turnAddr
      relayTcpAddr.value = creds.turnTcpAddr ?? ''
      relayUser.value    = creds.username
      relayPass.value    = creds.password
    } else {
      relayAddr.value = relayTcpAddr.value = relayUser.value = relayPass.value = ''
    }
  } finally {
    relayRefreshing = false
  }
}

// ── Lifecycle ─────────────────────────────────────────────────────────────────
onMounted(async () => {
  active.value = await P2PIsActive()
  await refreshRelayState()
  publicIP.value = await GetPublicIP()
  EventsOn('p2p:status', (s: string)   => { status.value = s })
  EventsOn('p2p:log',    (msg: string) => appendLog(msg))
  EventsOn('relay:log',  (msg: string) => appendLog('[relay] ' + msg))
  appendLog('P2P ready. Select Consumer or Provider and follow the steps.')
})

onUnmounted(() => {
  EventsOff('p2p:status')
  EventsOff('p2p:log')
  EventsOff('relay:log')
})

// ── Relay actions ──────────────────────────────────────────────────────────────
async function startRelay() {
  const relayPortErr = validatePort(String(relayPort.value), 'Relay port')
  if (relayPortErr) { appendLog('⚠  ' + relayPortErr); return }
  relayBusy.value = true
  const err = await RelayStart(relayPort.value)
  relayBusy.value = false
  if (err) { appendLog('Relay error: ' + err); return }
  await refreshRelayState()
  publicIP.value = await GetPublicIP()
  appendLog('TURN relay started — credentials refreshed.')
}

async function stopRelay() {
  await RelayStop()
  await refreshRelayState()
  appendLog('TURN relay stopped.')
}

// ── Consumer actions ───────────────────────────────────────────────────────────
async function genOffer() {
  if (!ports.value.trim()) { appendLog('⚠  Enter at least one port first.'); return }
  const portsErr = validatePorts(ports.value, 'Port')
  if (portsErr) { appendLog('⚠  ' + portsErr); return }
  const cfgErr = await P2PSetConfig(buildSessionConfig())
  if (cfgErr) { appendLog('⚠  ' + cfgErr); return }
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

// ── Provider actions ───────────────────────────────────────────────────────────
async function genAnswer() {
  if (!inboundSDP.value.trim()) { appendLog("⚠  Paste the Consumer's offer first."); return }
  const cfgErr = await P2PSetConfig(buildSessionConfig())
  if (cfgErr) { appendLog('⚠  ' + cfgErr); return }
  busy.value = true
  outboundSDP.value = ''
  const result = await P2PProvideAnswer(inboundSDP.value.trim())
  busy.value = false
  if (result.startsWith('ERROR:')) { appendLog(result); return }
  outboundSDP.value = result
  active.value = true
  appendLog('Answer ready — copy and send it back to the Consumer.')
}

// ── Stop ───────────────────────────────────────────────────────────────────────
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

// ── TURN entries management ────────────────────────────────────────────────────
function addTURNEntry() {
  turnEntries.value.push({ url: '', username: '', credential: '' })
}
function removeTURNEntry(idx: number) {
  turnEntries.value.splice(idx, 1)
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

      <!-- ── TURN Relay card ─────────────────────────────────────────────── -->
      <div class="bg-gray-800/50 border border-gray-700/60 rounded-xl p-4 space-y-3">
        <div class="flex items-center justify-between">
          <h3 class="text-xs font-semibold text-gray-500 uppercase tracking-wider">TURN Relay</h3>
          <span v-if="relayRunning"
            class="text-[10px] font-medium px-2 py-0.5 rounded-full bg-emerald-500/15 text-emerald-400 border border-emerald-500/30">
            Running
          </span>
        </div>
        <p class="text-xs text-gray-500 leading-relaxed">
          Start a local TURN relay on this machine when both sides are behind symmetric NAT or CGNAT.
          The relay is fully self-hosted — no third-party servers are involved.
        </p>

        <!-- Cross-network hint when relay not running -->
        <p v-if="!relayRunning" class="text-[10px] text-gray-600 mt-1">
          For machines on completely different networks (symmetric NAT/CGNAT), both sides need
          to reach a shared TURN relay. Start the relay on the machine with a public IP or port-forwarding.
        </p>

        <!-- Port + start button -->
        <div v-if="!relayRunning" class="flex gap-2">
          <input v-model.number="relayPort" type="number" min="1024" max="65535" placeholder="3478"
            class="w-24 bg-gray-900 border border-gray-700 rounded-lg px-3 py-1.5 text-sm text-white placeholder-gray-600 focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500/40 transition-colors" />
          <button @click="startRelay" :disabled="relayBusy"
            class="flex-1 py-1.5 text-sm font-semibold rounded-lg transition-all"
            :class="relayBusy
              ? 'bg-yellow-600/20 border border-yellow-500/40 text-yellow-400 animate-pulse'
              : 'bg-indigo-600 hover:bg-indigo-500 text-white disabled:opacity-40'">
            {{ relayBusy ? '⏳ Starting…' : '▶ Start Relay' }}
          </button>
        </div>

        <!-- Running: credentials display -->
        <template v-else>
          <div class="space-y-1 text-xs font-mono bg-gray-900 border border-gray-700/60 rounded-lg p-3">
            <div class="text-gray-400"><span class="text-gray-600">URL&nbsp;&nbsp;</span> {{ relayAddr }}</div>
            <div class="text-gray-400"><span class="text-gray-600">User&nbsp;</span> {{ relayUser }}</div>
            <div class="text-gray-400"><span class="text-gray-600">Pass&nbsp;</span> {{ relayPass }}</div>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <button @click="copyText(`${relayAddr}\n${relayUser}\n${relayPass}`)"
              class="py-1.5 text-xs font-medium rounded-lg bg-gray-700 hover:bg-gray-600 text-gray-200 transition-colors">
              ⎘ Copy Creds
            </button>
            <button @click="stopRelay"
              class="py-1.5 text-xs font-medium rounded-lg bg-red-600/20 border border-red-500/40 text-red-400 hover:bg-red-600/30 transition-colors">
              ⏹ Stop Relay
            </button>
          </div>
          <p v-if="publicIP" class="text-[10px] text-yellow-400/80 leading-relaxed mt-1">
            ⚠ For cross-network use: ensure UDP/TCP port {{ relayPort }} is open in your
            firewall/router and forwarded to this machine at {{ publicIP }}.
            The other machine must add <span class="font-mono">turn:{{ publicIP }}:{{ relayPort }}</span> to its TURN servers in Advanced settings.
          </p>
          <p class="text-[10px] text-emerald-500/70 leading-relaxed">
            Relay credentials are automatically added to the ICE configuration for this session.
          </p>
        </template>
      </div>

      <!-- Step 1 -->
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

      <!-- ── Advanced / TURN config ──────────────────────────────────────── -->
      <div class="bg-gray-800/50 border border-gray-700/60 rounded-xl overflow-hidden">
        <button @click="showAdvanced = !showAdvanced"
          class="w-full flex items-center justify-between px-4 py-3 text-xs font-semibold text-gray-500 uppercase tracking-wider hover:text-gray-300 transition-colors">
          <span>Advanced / Custom TURN</span>
          <span>{{ showAdvanced ? '▲' : '▼' }}</span>
        </button>

        <div v-show="showAdvanced" class="px-4 pb-4 space-y-3">
          <!-- ICE-TCP mux port -->
          <div>
            <label class="block text-xs text-gray-500 mb-1">ICE-TCP fixed port (0 = disabled)</label>
            <input v-model.number="tcpMuxPort" type="number" min="0" max="65535" placeholder="443"
              class="w-full bg-gray-900 border border-gray-700 rounded-lg px-3 py-1.5 text-sm text-white placeholder-gray-600 focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500/40 transition-colors" />
            <p class="text-[10px] text-gray-600 mt-1">Port 443 (default) makes TCP traffic look like HTTPS — the same trick browsers and Teams use to survive strict firewalls. Set to 0 to disable.</p>
          </div>

          <!-- Gather timeout -->
          <div>
            <label class="block text-xs text-gray-500 mb-1">ICE gather timeout (seconds)</label>
            <input v-model.number="gatherTimeoutSecs" type="number" min="5" max="120" placeholder="30"
              class="w-full bg-gray-900 border border-gray-700 rounded-lg px-3 py-1.5 text-sm text-white placeholder-gray-600 focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500/40 transition-colors" />
          </div>

          <!-- Manual TURN servers -->
          <div>
            <div class="flex items-center justify-between mb-2">
              <label class="text-xs text-gray-500">Manual TURN servers</label>
              <button @click="addTURNEntry"
                class="text-xs text-indigo-400 hover:text-indigo-300 transition-colors">+ Add</button>
            </div>
            <div v-for="(entry, i) in turnEntries" :key="i" class="space-y-1 mb-3 bg-gray-900/60 border border-gray-700/40 rounded-lg p-2">
              <input v-model="entry.url" placeholder="turn:host:3478?transport=udp"
                class="w-full bg-transparent border-b border-gray-700 pb-1 text-xs font-mono text-gray-300 placeholder-gray-600 focus:outline-none focus:border-indigo-500" />
              <div class="grid grid-cols-2 gap-1">
                <input v-model="entry.username" placeholder="username"
                  class="bg-transparent border-b border-gray-700 pb-1 text-xs text-gray-300 placeholder-gray-600 focus:outline-none focus:border-indigo-500" />
                <input v-model="entry.credential" placeholder="credential" type="password"
                  class="bg-transparent border-b border-gray-700 pb-1 text-xs text-gray-300 placeholder-gray-600 focus:outline-none focus:border-indigo-500" />
              </div>
              <button @click="removeTURNEntry(i)"
                class="text-[10px] text-red-500/70 hover:text-red-400 transition-colors">Remove</button>
            </div>
            <p v-if="turnEntries.length === 0" class="text-[10px] text-gray-600">No manual TURN servers. The built-in relay above is recommended.</p>
          </div>
        </div>
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
          {{ busy && !active ? '⏹ Cancel Gathering' : '⏹ Stop' }}
        </button>
      </div>

      <!-- Privacy note -->
      <div class="bg-gray-800/30 border border-gray-700/40 rounded-lg px-4 py-2.5 flex-shrink-0">
        <p class="text-xs text-gray-500 leading-relaxed">
          <span class="text-gray-400 font-medium">🔒 100% serverless.</span>
          STUN is used once to discover your public IP — no traffic ever touches it.
          After the SDP exchange, all data flows <span class="text-gray-400">directly</span> between the two machines, end-to-end encrypted.
          The optional TURN relay runs <span class="text-gray-400">on your own machine</span> — no third-party relay.
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

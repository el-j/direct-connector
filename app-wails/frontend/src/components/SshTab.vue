<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import {
  LoadConfig, SaveConfig,
  AppKeyExists, GenerateAppKey, LoadPublicKey, AppPrivateKeyPath,
  BrowseFile, CopyToClipboard,
  StartTunnel, StopTunnel, IsTunnelRunning,
} from '../../wailsjs/go/main/App'

// ── State ────────────────────────────────────────────────────────────────────
const form = reactive({
  host: '',
  port: '443',
  user: '',
  forwardPorts: '1883',
  ipVersion: '',
  forwardMode: 'R',
  verbose: false,
  useWSLSsh: false,
  keyPath: '',
})

const running  = ref(false)
const status   = ref('Status: Disconnected')
const logs     = ref<string[]>([])
const logBox   = ref<HTMLElement | null>(null)

const keyExists  = ref(false)
const publicKey  = ref('')
const privKey    = ref('')

// ── Computed ─────────────────────────────────────────────────────────────────
const statusClass = computed(() => {
  if (status.value.includes('Connected & Active'))
    return 'bg-emerald-500/15 text-emerald-400 border-emerald-500/40'
  if (status.value.includes('Connecting') || status.value.includes('Reconnecting'))
    return 'bg-yellow-500/15 text-yellow-400 border-yellow-500/40'
  return 'bg-gray-700/50 text-gray-400 border-gray-600/50'
})

// ── Helpers ───────────────────────────────────────────────────────────────────
function appendLog(msg: string) {
  logs.value.push(msg)
  if (logs.value.length > 400) logs.value = logs.value.slice(-400)
  nextTick(() => { if (logBox.value) logBox.value.scrollTop = logBox.value.scrollHeight })
}

// ── Lifecycle ─────────────────────────────────────────────────────────────────
onMounted(async () => {
  const cfg = await LoadConfig()
  form.host         = cfg.host || ''
  form.port         = cfg.port || '443'
  form.user         = cfg.user || ''
  form.forwardPorts = cfg.forwardPorts || '1883'
  form.ipVersion    = cfg.ipVersion || ''
  form.forwardMode  = cfg.forwardMode || 'R'
  form.verbose      = cfg.verbose ?? false
  form.useWSLSsh    = cfg.useWslSsh ?? false
  form.keyPath      = cfg.keyPath || ''

  keyExists.value = await AppKeyExists()
  if (keyExists.value) {
    publicKey.value = await LoadPublicKey()
    privKey.value   = await AppPrivateKeyPath()
  }
  running.value = await IsTunnelRunning()

  EventsOn('tunnel:status', (s: string) => { status.value = s })
  EventsOn('tunnel:log',    (msg: string) => appendLog(msg))
  appendLog('Ready. Configure the connection and press START TUNNEL.')
})

onUnmounted(() => {
  EventsOff('tunnel:status')
  EventsOff('tunnel:log')
})

// ── Key management ────────────────────────────────────────────────────────────
async function generateKey() {
  const err = await GenerateAppKey()
  if (err) { appendLog('Key error: ' + err); return }
  keyExists.value = true
  publicKey.value = await LoadPublicKey()
  privKey.value   = await AppPrivateKeyPath()
  appendLog('New Ed25519 keypair generated: ' + privKey.value)
  appendLog('Copy the public key and add it to ~/.ssh/authorized_keys on the server.')
}

async function copyPublicKey() {
  if (!publicKey.value) return
  await CopyToClipboard(publicKey.value)
  appendLog('Public key copied to clipboard.')
}

async function browseKey() {
  const p = await BrowseFile('Select SSH Private Key', '')
  if (p) form.keyPath = p
}

// ── Tunnel start / stop ───────────────────────────────────────────────────────
async function toggleTunnel() {
  if (running.value) {
    await StopTunnel()
    running.value = false
    return
  }

  const errs: string[] = []
  if (!form.host.trim())         errs.push('Host / DNS is required')
  if (!form.port.trim())         errs.push('SSH Port is required')
  if (!form.user.trim())         errs.push('SSH Username is required')
  if (!form.forwardPorts.trim()) errs.push('At least one Forward Port is required')
  if (errs.length) { appendLog('⚠  ' + errs.join(' | ')); return }

  // Key priority: custom path → app key → SSH default
  let keyPath = form.keyPath.trim()
  if (!keyPath && keyExists.value) keyPath = privKey.value

  await SaveConfig({
    host: form.host, port: form.port, user: form.user,
    forwardPorts: form.forwardPorts, ipVersion: form.ipVersion,
    forwardMode: form.forwardMode, verbose: form.verbose,
    useWslSsh: form.useWSLSsh, keyPath: form.keyPath,
  })

  await StartTunnel({
    host: form.host, port: form.port, user: form.user,
    forwardPorts: form.forwardPorts, ipVersion: form.ipVersion,
    forwardMode: form.forwardMode, verbose: form.verbose,
    useWslSsh: form.useWSLSsh, keyPath,
  })
  running.value = true
}
</script>

<template>
  <div class="flex h-full overflow-hidden">

    <!-- ── Left: Configuration ────────────────────────────────────────────── -->
    <div class="w-80 flex-shrink-0 overflow-y-auto p-4 space-y-3 border-r border-gray-700/60 bg-gray-900">

      <!-- Connection card -->
      <div class="bg-gray-800/50 border border-gray-700/60 rounded-xl p-4 space-y-3">
        <h3 class="text-xs font-semibold text-gray-500 uppercase tracking-wider">Connection</h3>

        <div class="space-y-2">
          <label class="block text-xs text-gray-400">Host / DNS</label>
          <input v-model="form.host" :disabled="running"
            placeholder="myserver.ddns.net"
            class="w-full bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm text-white placeholder-gray-600 focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500/40 disabled:opacity-40 transition-colors" />
        </div>

        <div class="flex gap-2">
          <div class="flex-1 space-y-2">
            <label class="block text-xs text-gray-400">SSH Port</label>
            <input v-model="form.port" :disabled="running" placeholder="443"
              class="w-full bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm text-white placeholder-gray-600 focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500/40 disabled:opacity-40 transition-colors" />
          </div>
          <div class="flex-1 space-y-2">
            <label class="block text-xs text-gray-400">Username</label>
            <input v-model="form.user" :disabled="running" placeholder="user"
              class="w-full bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm text-white placeholder-gray-600 focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500/40 disabled:opacity-40 transition-colors" />
          </div>
        </div>

        <div class="space-y-2">
          <label class="block text-xs text-gray-400">Forward Ports (comma-separated)</label>
          <input v-model="form.forwardPorts" :disabled="running" placeholder="1883, 3391"
            class="w-full bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-sm text-white placeholder-gray-600 focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500/40 disabled:opacity-40 transition-colors" />
        </div>
      </div>

      <!-- Options card -->
      <div class="bg-gray-800/50 border border-gray-700/60 rounded-xl p-4 space-y-4">
        <h3 class="text-xs font-semibold text-gray-500 uppercase tracking-wider">Options</h3>

        <div class="space-y-2">
          <label class="block text-xs text-gray-400">IP Version</label>
          <div class="flex flex-col gap-1.5">
            <label v-for="opt in [['','Auto (OS default)'],['4','Force IPv4'],['6','Force IPv6']]"
              :key="opt[0]"
              class="flex items-center gap-2.5 cursor-default"
              :class="running ? 'opacity-40' : ''">
              <input type="radio" v-model="form.ipVersion" :value="opt[0]" :disabled="running"
                class="accent-indigo-500 w-3.5 h-3.5" />
              <span class="text-sm text-gray-300">{{ opt[1] }}</span>
            </label>
          </div>
        </div>

        <div class="space-y-2">
          <label class="block text-xs text-gray-400">Forward Direction</label>
          <div class="flex flex-col gap-1.5">
            <label v-for="opt in [['R','Remote  -R  (server → this machine)'],['L','Local   -L  (this machine → server)']]"
              :key="opt[0]"
              class="flex items-center gap-2.5 cursor-default"
              :class="running ? 'opacity-40' : ''">
              <input type="radio" v-model="form.forwardMode" :value="opt[0]" :disabled="running"
                class="accent-indigo-500 w-3.5 h-3.5" />
              <span class="text-sm text-gray-300">{{ opt[1] }}</span>
            </label>
          </div>
        </div>

        <div class="flex flex-col gap-2">
          <label class="flex items-center gap-2.5 cursor-default" :class="running ? 'opacity-40' : ''">
            <input type="checkbox" v-model="form.verbose" :disabled="running" class="accent-indigo-500 w-3.5 h-3.5" />
            <span class="text-sm text-gray-300">Verbose SSH logging  <span class="text-xs text-gray-500">(-v)</span></span>
          </label>
          <label class="flex items-center gap-2.5 cursor-default" :class="running ? 'opacity-40' : ''">
            <input type="checkbox" v-model="form.useWSLSsh" :disabled="running" class="accent-indigo-500 w-3.5 h-3.5" />
            <span class="text-sm text-gray-300">Use WSL ssh  <span class="text-xs text-gray-500">(Windows: runs inside WSL where Docker lives)</span></span>
          </label>
        </div>
      </div>

      <!-- SSH Key card -->
      <div class="bg-gray-800/50 border border-gray-700/60 rounded-xl p-4 space-y-3">
        <h3 class="text-xs font-semibold text-gray-500 uppercase tracking-wider">SSH Key</h3>
        <p class="text-xs text-gray-500">
          {{ keyExists ? privKey : 'No app key found.' }}
        </p>
        <div class="flex gap-2">
          <button @click="generateKey" :disabled="running"
            class="flex-1 py-1.5 text-xs font-medium rounded-lg bg-gray-700 hover:bg-gray-600 text-gray-200 transition-colors disabled:opacity-40">
            ＋ Generate Key
          </button>
          <button @click="copyPublicKey" :disabled="!keyExists || running"
            class="flex-1 py-1.5 text-xs font-medium rounded-lg bg-gray-700 hover:bg-gray-600 text-gray-200 transition-colors disabled:opacity-40">
            ⎘ Copy Public Key
          </button>
        </div>
        <textarea v-if="keyExists" :value="publicKey" readonly rows="2"
          class="w-full bg-gray-900 border border-gray-700/60 rounded-lg px-3 py-2 text-xs font-mono text-gray-400 resize-none focus:outline-none" />
      </div>

      <!-- Custom key path card -->
      <div class="bg-gray-800/50 border border-gray-700/60 rounded-xl p-4 space-y-3">
        <h3 class="text-xs font-semibold text-gray-500 uppercase tracking-wider">Custom Key Path</h3>
        <p class="text-xs text-gray-500">Override — use any existing key instead of the app key.</p>
        <div class="flex gap-2">
          <input v-model="form.keyPath" :disabled="running"
            :placeholder="form.useWSLSsh ? '/home/user/.ssh/id_ed25519' : 'Leave blank to use app key'"
            class="flex-1 min-w-0 bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 text-xs text-white placeholder-gray-600 focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500/40 disabled:opacity-40 transition-colors" />
          <button @click="browseKey" :disabled="running"
            class="px-3 py-2 text-xs font-medium rounded-lg bg-gray-700 hover:bg-gray-600 text-gray-200 flex-shrink-0 transition-colors disabled:opacity-40">
            Browse…
          </button>
        </div>
      </div>

    </div>

    <!-- ── Right: Status + Log ─────────────────────────────────────────────── -->
    <div class="flex-1 flex flex-col p-4 gap-3 min-w-0 bg-gray-900">

      <!-- Status + button row -->
      <div class="flex items-center gap-3 flex-shrink-0">
        <span
          class="flex-1 text-xs font-medium px-3 py-1.5 rounded-full border text-center truncate"
          :class="statusClass"
        >{{ status }}</span>
        <button
          @click="toggleTunnel"
          class="px-6 py-1.5 rounded-full text-sm font-semibold transition-all duration-150 flex-shrink-0"
          :class="running
            ? 'bg-red-600/20 border border-red-500/50 text-red-400 hover:bg-red-600/30'
            : 'bg-indigo-600 hover:bg-indigo-500 text-white shadow-lg shadow-indigo-950/60'"
        >
          {{ running ? '⏹  STOP TUNNEL' : '▶  START TUNNEL' }}
        </button>
      </div>

      <!-- Log output -->
      <pre
        ref="logBox"
        class="flex-1 overflow-y-auto bg-black/50 rounded-xl p-4 text-xs font-mono text-green-400 leading-relaxed border border-gray-800/80 whitespace-pre-wrap"
      >{{ logs.join('\n') }}</pre>
    </div>
  </div>
</template>

// Global test setup: mock the Wails runtime and backend bindings
// so Vue components can be mounted in happy-dom without a Wails host.

// ── Wails runtime mock ────────────────────────────────────────────────────────
// eventListeners is reset in beforeEach so callbacks don't leak between tests.
const eventListeners: Record<string, Array<(...args: unknown[]) => void>> = {}

vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn:   vi.fn((event: string, cb: (...args: unknown[]) => void) => {
    ;(eventListeners[event] ??= []).push(cb)
  }),
  EventsOff:  vi.fn((event: string) => { delete eventListeners[event] }),
  EventsEmit: vi.fn(),
}))

// ── Go backend mock ───────────────────────────────────────────────────────────
vi.mock('../../wailsjs/go/main/App', () => ({
  LoadConfig: vi.fn(),
  SaveConfig:          vi.fn(),
  AppKeyExists:        vi.fn(),
  GenerateAppKey:      vi.fn(),
  LoadPublicKey:       vi.fn(),
  AppPrivateKeyPath:   vi.fn(),
  BrowseFile:          vi.fn(),
  CopyToClipboard:     vi.fn(),
  StartTunnel:         vi.fn(),
  StopTunnel:          vi.fn(),
  IsTunnelRunning:     vi.fn(),
  SSHSetupStart:       vi.fn(),
  SSHSetupReply:       vi.fn(),
  SSHSetupCancel:      vi.fn(),
  SSHCheckKeyAuth:     vi.fn(),
  P2PGenerateOffer:    vi.fn(),
  P2PAcceptAnswer:     vi.fn(),
  P2PProvideAnswer:    vi.fn(),
  P2PStop:             vi.fn(),
  P2PIsActive:         vi.fn(),
  P2PSetConfig:        vi.fn(),
  RelayStart:          vi.fn(),
  RelayStop:           vi.fn(),
  RelayIsRunning:      vi.fn(),
  RelayGetCredentials: vi.fn(),
  GetPublicIP:         vi.fn(),
}))

// ── Default implementations ────────────────────────────────────────────────────
// Re-applied in beforeEach so vi.clearAllMocks() (which only resets call counts)
// never leaves functions returning undefined.
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'

beforeEach(() => {
  // Clear call history without wiping implementations
  vi.clearAllMocks()
  // Clear stored event listeners
  for (const key of Object.keys(eventListeners)) delete eventListeners[key]

  // Re-install default return values
  vi.mocked(App.LoadConfig).mockResolvedValue({
    host: '', port: '443', user: '', forwardPorts: '1883',
    ipVersion: '', forwardMode: 'R', verbose: false, useWslSsh: false, keyPath: '',
  })
  vi.mocked(App.SaveConfig).mockResolvedValue(undefined)
  vi.mocked(App.AppKeyExists).mockResolvedValue(false)
  vi.mocked(App.GenerateAppKey).mockResolvedValue('')
  vi.mocked(App.LoadPublicKey).mockResolvedValue('')
  vi.mocked(App.AppPrivateKeyPath).mockResolvedValue('')
  vi.mocked(App.BrowseFile).mockResolvedValue('')
  vi.mocked(App.CopyToClipboard).mockResolvedValue(undefined)
  vi.mocked(App.StartTunnel).mockResolvedValue(undefined)
  vi.mocked(App.StopTunnel).mockResolvedValue(undefined)
  vi.mocked(App.IsTunnelRunning).mockResolvedValue(false)
  vi.mocked(App.SSHSetupStart).mockResolvedValue('')
  vi.mocked(App.SSHSetupReply).mockResolvedValue(undefined)
  vi.mocked(App.SSHSetupCancel).mockResolvedValue(undefined)
  vi.mocked(App.SSHCheckKeyAuth).mockResolvedValue('')
  vi.mocked(App.P2PGenerateOffer).mockResolvedValue('')
  vi.mocked(App.P2PAcceptAnswer).mockResolvedValue('')
  vi.mocked(App.P2PProvideAnswer).mockResolvedValue('')
  vi.mocked(App.P2PStop).mockResolvedValue(undefined)
  vi.mocked(App.P2PIsActive).mockResolvedValue(false)
  vi.mocked(App.P2PSetConfig).mockResolvedValue(undefined)
  vi.mocked(App.RelayStart).mockResolvedValue('')
  vi.mocked(App.RelayStop).mockResolvedValue(undefined)
  vi.mocked(App.RelayIsRunning).mockResolvedValue(false)
  vi.mocked(App.RelayGetCredentials).mockResolvedValue(
    main.RelayCredentials.createFrom({
      turnAddr: '', turnTcpAddr: '', username: '', password: '', isRunning: false,
    }),
  )
  vi.mocked(App.GetPublicIP).mockResolvedValue('')
})

// ── Test helpers ───────────────────────────────────────────────────────────────
// Expose helper so individual tests can simulate backend events
;(globalThis as Record<string, unknown>).__testEmitEvent = (
  event: string,
  ...args: unknown[]
) => {
  ;(eventListeners[event] ?? []).forEach(cb => cb(...args))
}

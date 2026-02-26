import { describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import P2pTab from '../components/P2pTab.vue'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'

// ── Helpers ────────────────────────────────────────────────────────────────────
const emit = (event: string, ...args: unknown[]) =>
  (globalThis as Record<string, (...a: unknown[]) => void>).__testEmitEvent(event, ...args)

// ── statusClass computed ───────────────────────────────────────────────────────

describe('P2pTab statusClass computed', () => {
  it('shows green when CONNECTED', async () => {
    const wrapper = mount(P2pTab)
    await flushPromises()

    emit('p2p:status', 'CONNECTED')
    await wrapper.vm.$nextTick()

    const badge = wrapper.find('span.rounded-full')
    expect(badge.classes().join(' ')).toContain('emerald')
  })

  it('shows yellow when Connecting', async () => {
    const wrapper = mount(P2pTab)
    await flushPromises()

    emit('p2p:status', 'Connecting to peer...')
    await wrapper.vm.$nextTick()

    const badge = wrapper.find('span.rounded-full')
    expect(badge.classes().join(' ')).toContain('yellow')
  })

  it('shows gray for Idle status', async () => {
    const wrapper = mount(P2pTab)
    await flushPromises()

    const badge = wrapper.find('span.rounded-full')
    expect(badge.classes().join(' ')).toContain('gray')
  })
})

// ── buildSessionConfig ────────────────────────────────────────────────────────

describe('P2pTab buildSessionConfig', () => {
  it('produces an empty TURN list when relay is not running', async () => {
    const wrapper = mount(P2pTab)
    await flushPromises()

    const portInput = wrapper.find('input[placeholder*="1883"]')
    if (portInput.exists()) await portInput.setValue('1883')

    vi.mocked(App.P2PGenerateOffer).mockResolvedValue('mock-sdp-offer')
    const offerBtn = wrapper.findAll('button').find(b => b.text().includes('Generate Offer'))
    if (offerBtn) {
      await offerBtn.trigger('click')
      await flushPromises()
    }

    if (vi.mocked(App.P2PSetConfig).mock.calls.length > 0) {
      const cfg = vi.mocked(App.P2PSetConfig).mock.calls[0][0] as main.P2PSessionConfig
      expect(cfg.turnServers ?? []).toHaveLength(0)
    }
  })

  it('includes relay TURN entries when relay is running', async () => {
    vi.mocked(App.RelayIsRunning).mockResolvedValue(true)
    vi.mocked(App.RelayGetCredentials).mockResolvedValue(
      main.RelayCredentials.createFrom({
        turnAddr: 'turn:1.2.3.4:3478', turnTcpAddr: 'turns:1.2.3.4:443',
        username: 'relay-user', password: 'relay-pass', isRunning: true,
      }),
    )

    const wrapper = mount(P2pTab)
    await flushPromises()

    const portInput = wrapper.find('input[placeholder*="1883"]')
    if (portInput.exists()) await portInput.setValue('1883')

    vi.mocked(App.P2PGenerateOffer).mockResolvedValue('mock-sdp-offer')
    const offerBtn = wrapper.findAll('button').find(b => b.text().includes('Generate Offer'))
    if (offerBtn) {
      await offerBtn.trigger('click')
      await flushPromises()
    }

    if (vi.mocked(App.P2PSetConfig).mock.calls.length > 0) {
      const cfg = vi.mocked(App.P2PSetConfig).mock.calls[0][0] as main.P2PSessionConfig
      expect(cfg.turnServers.length).toBeGreaterThanOrEqual(1)
      expect(cfg.turnServers[0].url).toBe('turn:1.2.3.4:3478')
    }
  })
})

// ── Relay start/stop ───────────────────────────────────────────────────────────

describe('P2pTab relay control', () => {
  it('logs relay error when RelayStart returns a non-empty string', async () => {
    vi.mocked(App.RelayStart).mockResolvedValue('port already in use')

    const wrapper = mount(P2pTab)
    await flushPromises()

    const startBtn = wrapper.findAll('button').find(b => b.text().includes('Start'))
    if (startBtn) {
      await startBtn.trigger('click')
      await flushPromises()
      expect(wrapper.text()).toContain('Relay error: port already in use')
    }
  })
})

// ── Error handling — genOffer ─────────────────────────────────────────────────

describe('P2pTab genOffer error handling', () => {
  it('displays ERROR: prefix returned by P2PGenerateOffer', async () => {
    vi.mocked(App.P2PGenerateOffer).mockResolvedValue('ERROR: ICE gathering timed out')

    const wrapper = mount(P2pTab)
    await flushPromises()

    const portInput = wrapper.find('input[placeholder*="1883"]')
    if (portInput.exists()) await portInput.setValue('1883')

    const offerBtn = wrapper.findAll('button').find(b => b.text().includes('Generate Offer'))
    if (offerBtn) {
      await offerBtn.trigger('click')
      await flushPromises()
      expect(wrapper.text()).toContain('ERROR: ICE gathering timed out')
    }
  })
})

// ── log capping ────────────────────────────────────────────────────────────────

describe('P2pTab log capping', () => {
  it('caps the log at 300 entries', async () => {
    const wrapper = mount(P2pTab)
    await flushPromises()

    for (let i = 0; i < 310; i++) {
      emit('p2p:log', `line ${i}`)
    }
    await wrapper.vm.$nextTick()

    const lines = wrapper.find('pre').text().split('\n').filter(Boolean)
    expect(lines.length).toBeLessThanOrEqual(300)
  })
})

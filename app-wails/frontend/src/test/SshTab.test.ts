import { describe, it, expect, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import SshTab from '../components/SshTab.vue'
import * as App from '../../wailsjs/go/main/App'

// ── Helpers ────────────────────────────────────────────────────────────────────
const emit = (event: string, ...args: unknown[]) =>
  (globalThis as Record<string, (...a: unknown[]) => void>).__testEmitEvent(event, ...args)

// ── statusClass computed ───────────────────────────────────────────────────────

describe('SshTab statusClass computed', () => {
  it('shows green for "Connected & Active"', async () => {
    const wrapper = mount(SshTab)
    await flushPromises()

    emit('tunnel:status', 'Status: Connected & Active')
    await wrapper.vm.$nextTick()

    const badge = wrapper.find('span.rounded-full')
    expect(badge.classes().join(' ')).toContain('emerald')
  })

  it('shows yellow while Connecting', async () => {
    const wrapper = mount(SshTab)
    await flushPromises()

    emit('tunnel:status', 'Status: Connecting...')
    await wrapper.vm.$nextTick()

    const badge = wrapper.find('span.rounded-full')
    expect(badge.classes().join(' ')).toContain('yellow')
  })

  it('shows gray when Disconnected', async () => {
    const wrapper = mount(SshTab)
    await flushPromises()

    emit('tunnel:status', 'Status: Disconnected')
    await wrapper.vm.$nextTick()

    const badge = wrapper.find('span.rounded-full')
    expect(badge.classes().join(' ')).toContain('gray')
  })
})

// ── onMounted ─────────────────────────────────────────────────────────────────

describe('SshTab onMounted', () => {
  it('loads config on mount and populates form', async () => {
    vi.mocked(App.LoadConfig).mockResolvedValue({
      host: 'srv.example.com', port: '22', user: 'root', forwardPorts: '8883',
      ipVersion: '4', forwardMode: 'L', verbose: true, useWslSsh: false, keyPath: '',
    })

    const wrapper = mount(SshTab)
    await flushPromises()

    const hostInput = wrapper.find('input[placeholder="myserver.ddns.net"]')
    expect((hostInput.element as HTMLInputElement).value).toBe('srv.example.com')
  })

  it('fetches key info when AppKeyExists returns true', async () => {
    vi.mocked(App.AppKeyExists).mockResolvedValue(true)
    vi.mocked(App.LoadPublicKey).mockResolvedValue('ssh-ed25519 AAAA test')
    vi.mocked(App.AppPrivateKeyPath).mockResolvedValue('/home/user/.config/key')

    const wrapper = mount(SshTab)
    await flushPromises()

    expect(App.LoadPublicKey).toHaveBeenCalled()
    expect(App.AppPrivateKeyPath).toHaveBeenCalled()
    expect(wrapper.text()).toContain('/home/user/.config/key')
  })
})

// ── Validation — toggleTunnel ──────────────────────────────────────────────────

describe('SshTab toggleTunnel validation', () => {
  it('blocks start when host is empty', async () => {
    const wrapper = mount(SshTab)
    await flushPromises()

    // Click the START TUNNEL button (rounded-full button)
    await wrapper.find('button.rounded-full').trigger('click')
    await flushPromises()

    expect(App.StartTunnel).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('Host / DNS is required')
  })
})

// ── generateKey ────────────────────────────────────────────────────────────────

describe('SshTab generateKey', () => {
  it('logs error when GenerateAppKey returns a non-empty string', async () => {
    vi.mocked(App.GenerateAppKey).mockResolvedValue('permission denied')

    const wrapper = mount(SshTab)
    await flushPromises()

    // Find the Generate Key button by text
    const buttons = wrapper.findAll('button')
    const genBtn = buttons.find(b => b.text().includes('Generate Key'))
    expect(genBtn).toBeDefined()
    await genBtn!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Key error: permission denied')
    expect(App.LoadPublicKey).not.toHaveBeenCalled()
  })

  it('refreshes public key display on success', async () => {
    vi.mocked(App.GenerateAppKey).mockResolvedValue('')
    vi.mocked(App.LoadPublicKey).mockResolvedValue('ssh-ed25519 AAAA newly-generated')
    vi.mocked(App.AppPrivateKeyPath).mockResolvedValue('/tmp/app.key')

    const wrapper = mount(SshTab)
    await flushPromises()

    const buttons = wrapper.findAll('button')
    const genBtn = buttons.find(b => b.text().includes('Generate Key'))
    expect(genBtn).toBeDefined()
    await genBtn!.trigger('click')
    await flushPromises()

    expect(App.LoadPublicKey).toHaveBeenCalled()
    expect(wrapper.text()).toContain('/tmp/app.key')
  })
})

// ── setup:done event ───────────────────────────────────────────────────────────

describe('SshTab setup:done event', () => {
  it('shows success message on ok:true', async () => {
    const wrapper = mount(SshTab)
    await flushPromises()

    emit('setup:done', { ok: true, message: 'Public key installed successfully.' })
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('Public key installed successfully.')
  })

  it('shows failure message on ok:false', async () => {
    const wrapper = mount(SshTab)
    await flushPromises()

    emit('setup:done', { ok: false, message: 'Authentication failed.' })
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('Authentication failed.')
  })
})

// ── log capping ────────────────────────────────────────────────────────────────

describe('SshTab log capping', () => {
  it('caps the log at 400 entries', async () => {
    const wrapper = mount(SshTab)
    await flushPromises()

    for (let i = 0; i < 410; i++) {
      emit('tunnel:log', `line ${i}`)
    }
    await wrapper.vm.$nextTick()

    // The pre element renders all logs joined by newline
    const lines = wrapper.find('pre').text().split('\n').filter(Boolean)
    expect(lines.length).toBeLessThanOrEqual(400)
  })
})


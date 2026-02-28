import { describe, it, expect } from 'vitest'
import { main } from '../../wailsjs/go/models'

// ── AppSettings ───────────────────────────────────────────────────────────────

describe('AppSettings', () => {
  it('maps all fields from a plain object', () => {
    const s = main.AppSettings.createFrom({
      host: 'myhost', port: '443', forwardPorts: '1883', user: 'alice',
      ipVersion: '4', forwardMode: 'R', keyPath: '/tmp/key',
      verbose: true, useWslSsh: false,
    })
    expect(s.host).toBe('myhost')
    expect(s.port).toBe('443')
    expect(s.forwardPorts).toBe('1883')
    expect(s.user).toBe('alice')
    expect(s.ipVersion).toBe('4')
    expect(s.forwardMode).toBe('R')
    expect(s.keyPath).toBe('/tmp/key')
    expect(s.verbose).toBe(true)
    expect(s.useWslSsh).toBe(false)
  })

  it('accepts a JSON string', () => {
    const s = main.AppSettings.createFrom(
      JSON.stringify({ host: 'json-host', port: '22', forwardPorts: '', user: 'bob',
        ipVersion: '', forwardMode: 'L', keyPath: '', verbose: false, useWslSsh: true }),
    )
    expect(s.host).toBe('json-host')
    expect(s.useWslSsh).toBe(true)
  })

  it('handles missing / undefined fields without throwing', () => {
    expect(() => main.AppSettings.createFrom({})).not.toThrow()
    expect(() => main.AppSettings.createFrom(null)).not.toThrow()
  })
})

// ── TunnelSettings ────────────────────────────────────────────────────────────

describe('TunnelSettings', () => {
  it('maps all fields', () => {
    const t = main.TunnelSettings.createFrom({
      host: 'srv', port: '443', forwardPorts: '1883,8883', user: 'root',
      ipVersion: '6', forwardMode: 'L', keyPath: '/key', verbose: false, useWslSsh: true,
    })
    expect(t.host).toBe('srv')
    expect(t.ipVersion).toBe('6')
    expect(t.useWslSsh).toBe(true)
  })
})

// ── P2PTURNServer ─────────────────────────────────────────────────────────────

describe('P2PTURNServer', () => {
  it('maps url, username and credential', () => {
    const s = main.P2PTURNServer.createFrom({
      url: 'turn:1.2.3.4:3478', username: 'user1', credential: 'pass1',
    })
    expect(s.url).toBe('turn:1.2.3.4:3478')
    expect(s.username).toBe('user1')
    expect(s.credential).toBe('pass1')
  })

  it('handles empty object without throwing', () => {
    expect(() => main.P2PTURNServer.createFrom({})).not.toThrow()
  })
})

// ── P2PSessionConfig ──────────────────────────────────────────────────────────

describe('P2PSessionConfig', () => {
  it('converts nested TURN server array', () => {
    const cfg = main.P2PSessionConfig.createFrom({
      turnServers: [
        { url: 'turn:a:3478', username: 'u', credential: 'p' },
        { url: 'turns:b:5349', username: 'v', credential: 'q' },
      ],
      tcpMuxPort: 443,
      gatherTimeoutSecs: 30,
    })
    expect(cfg.turnServers).toHaveLength(2)
    expect(cfg.turnServers[0]).toBeInstanceOf(main.P2PTURNServer)
    expect(cfg.turnServers[0].url).toBe('turn:a:3478')
    expect(cfg.tcpMuxPort).toBe(443)
    expect(cfg.gatherTimeoutSecs).toBe(30)
  })

  it('handles null turnServers gracefully', () => {
    const cfg = main.P2PSessionConfig.createFrom({ turnServers: null, tcpMuxPort: 0, gatherTimeoutSecs: 0 })
    expect(cfg.turnServers).toBeNull()
  })
})

// ── RelayCredentials ──────────────────────────────────────────────────────────

describe('RelayCredentials', () => {
  it('maps all fields including turnTcpAddr', () => {
    const r = main.RelayCredentials.createFrom({
      turnAddr: 'turn:1.2.3.4:3478', turnTcpAddr: 'turns:1.2.3.4:443',
      username: 'admin', password: 'secret', isRunning: true,
    })
    expect(r.turnAddr).toBe('turn:1.2.3.4:3478')
    expect(r.turnTcpAddr).toBe('turns:1.2.3.4:443')
    expect(r.isRunning).toBe(true)
  })

  it('defaults turnTcpAddr to empty string when absent', () => {
    const r = main.RelayCredentials.createFrom({
      turnAddr: 'turn:1.2.3.4:3478', username: 'u', password: 'p', isRunning: false,
    })
    expect(r.turnTcpAddr).toBe('')
  })
})

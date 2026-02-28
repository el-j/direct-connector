import { describe, it, expect } from 'vitest'
import { validatePort, validatePorts } from '../utils/portValidation'

describe('validatePort', () => {
  it('returns null for valid ports', () => {
    expect(validatePort('1')).toBeNull()
    expect(validatePort('443')).toBeNull()
    expect(validatePort('65535')).toBeNull()
    expect(validatePort('  8080  ')).toBeNull()
  })

  it('returns null for empty string (presence check is separate)', () => {
    expect(validatePort('')).toBeNull()
    expect(validatePort('   ')).toBeNull()
  })

  it('returns error for port 0', () => {
    expect(validatePort('0')).toContain('between 1 and 65535')
  })

  it('returns error for port > 65535', () => {
    expect(validatePort('65536')).toContain('between 1 and 65535')
    expect(validatePort('99999')).toContain('between 1 and 65535')
  })

  it('returns error for non-numeric string', () => {
    expect(validatePort('abc')).toContain('between 1 and 65535')
  })

  it('uses custom label in error message', () => {
    expect(validatePort('0', 'SSH Port')).toContain('SSH Port')
  })
})

describe('validatePorts', () => {
  it('returns null when all ports are valid', () => {
    expect(validatePorts('1883')).toBeNull()
    expect(validatePorts('1883, 3391')).toBeNull()
    expect(validatePorts('80,443,8080')).toBeNull()
  })

  it('returns error when any port is out of range', () => {
    expect(validatePorts('1883, 99999')).toContain('between 1 and 65535')
  })

  it('returns null for empty string', () => {
    expect(validatePorts('')).toBeNull()
  })

  it('ignores empty segments (trailing comma)', () => {
    expect(validatePorts('1883,')).toBeNull()
  })
})

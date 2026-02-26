/**
 * Validates a TCP port string.
 * @returns An error message string, or null if the port is valid.
 */
export function validatePort(portStr: string, label = 'Port'): string | null {
  const trimmed = portStr.trim()
  if (!trimmed) return null // presence check is done separately
  const n = parseInt(trimmed, 10)
  if (isNaN(n) || n < 1 || n > 65535) {
    return `${label} must be a number between 1 and 65535`
  }
  return null
}

/**
 * Validates a comma-separated list of TCP port strings.
 * @returns An error message string, or null if all ports are valid.
 */
export function validatePorts(portsStr: string, label = 'Port'): string | null {
  for (const raw of portsStr.split(',')) {
    const err = validatePort(raw, label)
    if (err) return err
  }
  return null
}

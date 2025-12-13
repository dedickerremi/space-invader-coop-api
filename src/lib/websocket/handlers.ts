import type { ClientMessage } from '../../types.js'
import { setPlayerDirection, playerShoot } from '../game/state.js'

export function handleMessage(playerId: string, data: unknown): void {
  if (!isValidMessage(data)) {
    console.warn(`[WS] Invalid message from ${playerId}:`, data)
    return
  }

  switch (data.type) {
    case 'MOVE':
      setPlayerDirection(playerId, data.dir)
      break

    case 'STOP':
      setPlayerDirection(playerId, 0)
      break

    case 'SHOOT':
      playerShoot(playerId)
      break
  }
}

function isValidMessage(data: unknown): data is ClientMessage {
  if (!data || typeof data !== 'object') return false

  const msg = data as Record<string, unknown>

  switch (msg.type) {
    case 'MOVE':
      return msg.dir === -1 || msg.dir === 1
    case 'STOP':
    case 'SHOOT':
      return true
    default:
      return false
  }
}


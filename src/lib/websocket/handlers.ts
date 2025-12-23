import type { ClientMessage } from '../../types.js'
import { setPlayerDirection, playerShoot, pauseGame, resumeGame } from '../game/state.js'

export type HandleResult = {
  action: 'none' | 'exit'
}

export function handleMessage(matchId: string, playerId: string, data: unknown): HandleResult {
  if (!isValidMessage(data)) {
    console.warn(`[WS] Invalid message from ${playerId}:`, data)
    return { action: 'none' }
  }

  switch (data.type) {
    case 'MOVE':
      setPlayerDirection(matchId, playerId, data.dir)
      break

    case 'STOP':
      setPlayerDirection(matchId, playerId, 0)
      break

    case 'SHOOT':
      playerShoot(matchId, playerId)
      break

    case 'PAUSE':
      pauseGame(matchId, playerId)
      break

    case 'RESUME':
      resumeGame(matchId, playerId)
      break

    case 'EXIT':
      return { action: 'exit' }
  }

  return { action: 'none' }
}

function isValidMessage(data: unknown): data is ClientMessage {
  if (!data || typeof data !== 'object') return false

  const msg = data as Record<string, unknown>

  switch (msg.type) {
    case 'MOVE':
      return msg.dir === -1 || msg.dir === 1
    case 'STOP':
    case 'SHOOT':
    case 'PAUSE':
    case 'RESUME':
    case 'EXIT':
      return true
    default:
      return false
  }
}
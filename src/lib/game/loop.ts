import { tick, getState } from './state.js'
import { broadcastToMatch } from '../websocket/server.js'
import { getAllMatches } from '../match/manager.js'
import type { StateMessage } from '../../types.js'

const TICK_RATE = 30 // Hz
const TICK_INTERVAL = 1000 / TICK_RATE

let loopInterval: ReturnType<typeof setInterval> | null = null

export function startGameLoop(): void {
  if (loopInterval) return

  console.log(`[GAME] Starting game loop at ${TICK_RATE} Hz`)

  loopInterval = setInterval(() => {
    // Update all active matches
    for (const match of getAllMatches()) {
      tick(match.matchId)

      const state = getState(match.matchId)
      if (state) {
        const message: StateMessage = {
          type: 'STATE',
          state,
        }
        broadcastToMatch(match.matchId, message)
      }
    }
  }, TICK_INTERVAL)
}

export function stopGameLoop(): void {
  if (loopInterval) {
    clearInterval(loopInterval)
    loopInterval = null
    console.log('[GAME] Game loop stopped')
  }
}

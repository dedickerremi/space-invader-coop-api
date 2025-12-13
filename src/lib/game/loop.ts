import { tick, getState } from './state.js'
import { broadcast } from '../websocket/server.js'
import type { StateMessage } from '../../types.js'

const TICK_RATE = 30 // Hz
const TICK_INTERVAL = 1000 / TICK_RATE

let loopInterval: ReturnType<typeof setInterval> | null = null

export function startGameLoop(): void {
  if (loopInterval) return

  console.log(`[GAME] Starting game loop at ${TICK_RATE} Hz`)

  loopInterval = setInterval(() => {
    tick()

    const message: StateMessage = {
      type: 'STATE',
      state: getState(),
    }

    broadcast(message)
  }, TICK_INTERVAL)
}

export function stopGameLoop(): void {
  if (loopInterval) {
    clearInterval(loopInterval)
    loopInterval = null
    console.log('[GAME] Game loop stopped')
  }
}


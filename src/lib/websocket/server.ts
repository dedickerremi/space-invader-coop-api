import { WebSocketServer, WebSocket } from 'ws'
import type { ServerMessage } from '../../types.js'
import { handleMessage } from './handlers.js'
import {
  addPlayer,
  removePlayer,
  getPlayerCount,
} from '../game/state.js'

const MAX_PLAYERS = 2

type ConnectedClient = {
  ws: WebSocket
  playerId: string
}

const clients: Map<string, ConnectedClient> = new Map()

let wss: WebSocketServer | null = null

export function createServer(port: number): WebSocketServer {
  wss = new WebSocketServer({ port })

  console.log(`[WS] Server listening on port ${port}`)

  wss.on('connection', (ws) => {
    // Reject if full
    if (getPlayerCount() >= MAX_PLAYERS) {
      send(ws, { type: 'ERROR', reason: 'Game is full' })
      ws.close()
      return
    }

    // Generate player ID
    const playerId = `player-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`

    // Register player
    clients.set(playerId, { ws, playerId })
    addPlayer(playerId)

    console.log(`[WS] Player connected: ${playerId} (${getPlayerCount()}/${MAX_PLAYERS})`)

    // Send welcome
    send(ws, { type: 'WELCOME', playerId })

    // Handle messages
    ws.on('message', (raw) => {
      try {
        const data = JSON.parse(raw.toString())
        handleMessage(playerId, data)
      } catch {
        console.warn(`[WS] Failed to parse message from ${playerId}`)
      }
    })

    // Handle disconnect
    ws.on('close', () => {
      clients.delete(playerId)
      removePlayer(playerId)
      console.log(`[WS] Player disconnected: ${playerId} (${getPlayerCount()}/${MAX_PLAYERS})`)
    })

    ws.on('error', (err) => {
      console.error(`[WS] Error for ${playerId}:`, err.message)
    })
  })

  return wss
}

export function send(ws: WebSocket, message: ServerMessage): void {
  if (ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(message))
  }
}

export function broadcast(message: ServerMessage): void {
  const payload = JSON.stringify(message)

  for (const client of clients.values()) {
    if (client.ws.readyState === WebSocket.OPEN) {
      client.ws.send(payload)
    }
  }
}

export function getConnectedClients(): number {
  return clients.size
}


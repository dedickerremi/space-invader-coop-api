import { WebSocketServer, WebSocket } from 'ws'
import { IncomingMessage } from 'http'
import type { ServerMessage } from '../../types.js'
import { handleMessage } from './handlers.js'
import { registerToken, validateToken, removeMatch, getMatch } from '../match/manager.js'
import { addPlayer, removePlayer, getPlayerCount } from '../game/state.js'

type ConnectedClient = {
  ws: WebSocket
  playerId: string
  matchId: string
}

// Clients by matchId -> playerId -> client
const matchClients: Map<string, Map<string, ConnectedClient>> = new Map()

let wss: WebSocketServer | null = null

export function createServer(port: number): WebSocketServer {
  wss = new WebSocketServer({ port })

  console.log(`[WS] Server listening on port ${port}`)

  wss.on('connection', (ws: WebSocket, req: IncomingMessage) => {
    console.log(`[WS] New connection attempt: ${req.url}`)

    // Extract params from query string: token, matchId, playerId
    const url = new URL(req.url ?? '', `http://localhost:${port}`)
    const token = url.searchParams.get('token')
    const matchId = url.searchParams.get('matchId')
    const playerId = url.searchParams.get('playerId')

    console.log(`[WS] Params: token=${token?.slice(0, 20)}..., matchId=${matchId}, playerId=${playerId}`)

    if (!token || !matchId || !playerId) {
      console.log(`[WS] Missing params: token=${!!token}, matchId=${!!matchId}, playerId=${!!playerId}`)
      const errorMsg = { type: 'ERROR' as const, reason: 'Missing token, matchId or playerId' }
      send(ws, errorMsg)
      // Wait for message to be sent before closing
      setTimeout(() => {
        if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
          ws.close(1000, 'Missing parameters')
        }
      }, 200)
      return
    }

    // Register token and create match if needed
    const registered = registerToken(token, matchId, playerId)
    if (!registered) {
      console.log(`[WS] Failed to register token for ${playerId} in ${matchId}`)
      const errorMsg = { type: 'ERROR' as const, reason: 'Cannot join match (full or limit reached)' }
      send(ws, errorMsg)
      setTimeout(() => {
        if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
          ws.close(1000, 'Registration failed')
        }
      }, 200)
      return
    }

    // Check match exists
    const match = getMatch(matchId)
    if (!match) {
      console.log(`[WS] Match ${matchId} not found after registration`)
      const errorMsg = { type: 'ERROR' as const, reason: 'Match not found' }
      send(ws, errorMsg)
      setTimeout(() => {
        if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
          ws.close(1000, 'Match not found')
        }
      }, 200)
      return
    }

    // Register client
    if (!matchClients.has(matchId)) {
      matchClients.set(matchId, new Map())
    }
    matchClients.get(matchId)!.set(playerId, { ws, playerId, matchId })

    // Add player to game state
    addPlayer(matchId, playerId)

    console.log(`[WS] Player ${playerId} connected to ${matchId} (${getPlayerCount(matchId)}/2)`)

    // Send welcome
    send(ws, { type: 'WELCOME', playerId, matchId })

    // Handle messages
    ws.on('message', (raw) => {
      try {
        const data = JSON.parse(raw.toString())
        const result = handleMessage(matchId, playerId, data)

        // Handle exit action
        if (result.action === 'exit') {
          // Notify all players in match that match ended
          broadcastToMatch(matchId, { type: 'MATCH_ENDED', reason: 'Player left the game' })

          // Close all connections for this match
          const clients = matchClients.get(matchId)
          if (clients) {
            for (const client of clients.values()) {
              client.ws.close()
            }
          }
        }
      } catch {
        console.warn(`[WS] Failed to parse message from ${playerId}`)
      }
    })

    // Handle disconnect
    ws.on('close', () => {
      const clients = matchClients.get(matchId)
      if (clients) {
        clients.delete(playerId)

        // Notify remaining players
        if (clients.size > 0) {
          for (const client of clients.values()) {
            send(client.ws, { type: 'MATCH_ENDED', reason: 'Opponent disconnected' })
            client.ws.close()
          }
        }

        if (clients.size === 0) {
          matchClients.delete(matchId)
        }
      }

      removePlayer(matchId, playerId)
      console.log(`[WS] Player ${playerId} disconnected from ${matchId} (${getPlayerCount(matchId)}/2)`)

      // Clean up empty matches
      if (getPlayerCount(matchId) === 0) {
        removeMatch(matchId)
      }
    })

    ws.on('error', (err) => {
      console.error(`[WS] Error for ${playerId}:`, err.message)
    })
  })

  return wss
}

export function send(ws: WebSocket, message: ServerMessage): void {
  if (ws.readyState === WebSocket.OPEN) {
    try {
      ws.send(JSON.stringify(message))
    } catch (err) {
      console.error('[WS] Failed to send message:', err)
    }
  } else {
    console.warn(`[WS] Cannot send message, socket state: ${ws.readyState}`)
  }
}

export function broadcastToMatch(matchId: string, message: ServerMessage): void {
  const clients = matchClients.get(matchId)
  if (!clients) return

  const payload = JSON.stringify(message)

  for (const client of clients.values()) {
    if (client.ws.readyState === WebSocket.OPEN) {
      client.ws.send(payload)
    }
  }
}

export function getConnectedClientsInMatch(matchId: string): number {
  return matchClients.get(matchId)?.size ?? 0
}

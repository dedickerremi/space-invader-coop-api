import { getAllMatches, getActiveMatchCount } from './match/manager.js'
import { getConnectedClientsInMatch } from './websocket/server.js'
import { getPlayerCount } from './game/state.js'

export type ServerStats = {
  activeMatches: number
  maxMatches: number
  totalPlayers: number
  matches: Array<{
    matchId: string
    playerCount: number
    connectedPlayers: number
    started: boolean
    paused: boolean
    createdAt: number
    age: number // seconds
  }>
}

export function getServerStats(): ServerStats {
  const allMatches = getAllMatches()
  const activeMatches = getActiveMatchCount()
  const MAX_MATCHES = 10

  let totalPlayers = 0

  const matches = allMatches.map((match) => {
    const playerCount = match.state.players.length
    const connectedPlayers = getConnectedClientsInMatch(match.matchId)
    totalPlayers += connectedPlayers

    return {
      matchId: match.matchId,
      playerCount,
      connectedPlayers,
      started: match.state.started,
      paused: match.state.paused,
      createdAt: match.createdAt,
      age: Math.floor((Date.now() - match.createdAt) / 1000),
    }
  })

  return {
    activeMatches,
    maxMatches: MAX_MATCHES,
    totalPlayers,
    matches,
  }
}


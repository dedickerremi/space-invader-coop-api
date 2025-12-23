import type { Match, GameState } from '../../types.js'

const MAX_MATCHES = 10

// Active matches by matchId
const matches: Map<string, Match> = new Map()

// Token to match lookup: token -> { matchId, playerId }
const tokenToPlayer: Map<string, { matchId: string; playerId: string }> = new Map()

function createInitialState(): GameState {
  return {
    players: [],
    bullets: [],
    started: false,
    paused: false,
    pausedBy: null,
  }
}

export function canCreateMatch(): boolean {
  return matches.size < MAX_MATCHES
}

export function getActiveMatchCount(): number {
  return matches.size
}

// Register a token (called when player connects with token from frontend)
export function registerToken(token: string, matchId: string, playerId: string): boolean {
  // Check if match exists, create if needed
  if (!matches.has(matchId)) {
    if (!canCreateMatch()) {
      console.log('[MATCH] Cannot create match: max limit reached')
      return false
    }

    const match: Match = {
      matchId,
      playerIds: [],
      tokens: new Map(),
      state: createInitialState(),
      createdAt: Date.now(),
    }

    matches.set(matchId, match)
    console.log(`[MATCH] Created ${matchId}`)
  }

  const match = matches.get(matchId)!

  // Check if match is not full
  if (match.playerIds.length >= 2 && !match.playerIds.includes(playerId)) {
    console.log(`[MATCH] Match ${matchId} is full`)
    return false
  }

  // Register player if not already
  if (!match.playerIds.includes(playerId)) {
    match.playerIds.push(playerId)
  }

  // Store token mapping
  match.tokens.set(playerId, token)
  tokenToPlayer.set(token, { matchId, playerId })

  console.log(`[MATCH] Registered player ${playerId} in ${matchId} (${match.playerIds.length}/2)`)
  return true
}

export function validateToken(token: string): { matchId: string; playerId: string } | null {
  return tokenToPlayer.get(token) ?? null
}

export function getMatch(matchId: string): Match | null {
  return matches.get(matchId) ?? null
}

export function removeMatch(matchId: string): void {
  const match = matches.get(matchId)
  if (!match) return

  // Clean up token lookups
  for (const token of match.tokens.values()) {
    tokenToPlayer.delete(token)
  }

  matches.delete(matchId)
  console.log(`[MATCH] Removed ${matchId}`)
}

export function getAllMatches(): Match[] {
  return Array.from(matches.values())
}

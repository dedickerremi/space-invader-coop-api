import type { GameState, Player, Bullet } from '../../types.js'
import { getMatch } from '../match/manager.js'

// Game constants
const PLAYER_SPEED = 5
const BULLET_SPEED = 8
const GAME_WIDTH = 800
const GAME_HEIGHT = 600
const PLAYER_Y = 550

export function getState(matchId: string): GameState | null {
  const match = getMatch(matchId)
  return match?.state ?? null
}

export function addPlayer(matchId: string, playerId: string): Player | null {
  const match = getMatch(matchId)
  if (!match) return null

  // Position player based on count (left or right side)
  const x = match.state.players.length === 0 ? 200 : 600

  const player: Player = {
    id: playerId,
    x,
    alive: true,
    direction: 0,
  }

  match.state.players.push(player)

  // Auto-start when 2 players connected
  if (match.state.players.length === 2) {
    match.state.started = true
  }

  return player
}

export function removePlayer(matchId: string, playerId: string): void {
  const match = getMatch(matchId)
  if (!match) return

  match.state.players = match.state.players.filter((p) => p.id !== playerId)
  match.state.bullets = match.state.bullets.filter((b) => b.ownerId !== playerId)

  // If the player who paused leaves, resume the game
  if (match.state.pausedBy === playerId) {
    match.state.paused = false
    match.state.pausedBy = null
  }

  // Stop game if not enough players
  if (match.state.players.length < 2) {
    match.state.started = false
  }
}

export function setPlayerDirection(matchId: string, playerId: string, dir: -1 | 0 | 1): void {
  const match = getMatch(matchId)
  if (!match || match.state.paused) return

  const player = match.state.players.find((p) => p.id === playerId)
  if (player && player.alive) {
    player.direction = dir
  }
}

export function playerShoot(matchId: string, playerId: string): void {
  const match = getMatch(matchId)
  if (!match || match.state.paused) return

  const player = match.state.players.find((p) => p.id === playerId)
  if (!player || !player.alive || !match.state.started) return

  const bullet: Bullet = {
    x: player.x,
    y: PLAYER_Y - 10,
    ownerId: playerId,
  }

  match.state.bullets.push(bullet)
}

export function pauseGame(matchId: string, playerId: string): boolean {
  const match = getMatch(matchId)
  if (!match || !match.state.started) return false

  // Already paused
  if (match.state.paused) return false

  match.state.paused = true
  match.state.pausedBy = playerId
  console.log(`[GAME] Match ${matchId} paused by ${playerId}`)
  return true
}

export function resumeGame(matchId: string, playerId: string): boolean {
  const match = getMatch(matchId)
  if (!match) return false

  // Not paused
  if (!match.state.paused) return false

  // Only the player who paused can resume (or anyone if pausedBy is null)
  if (match.state.pausedBy && match.state.pausedBy !== playerId) {
    return false
  }

  match.state.paused = false
  match.state.pausedBy = null
  console.log(`[GAME] Match ${matchId} resumed by ${playerId}`)
  return true
}

export function tick(matchId: string): void {
  const match = getMatch(matchId)
  // Don't update if not started OR if paused
  if (!match || !match.state.started || match.state.paused) return

  // Update player positions
  for (const player of match.state.players) {
    if (!player.alive) continue

    player.x += player.direction * PLAYER_SPEED

    // Clamp to game bounds
    player.x = Math.max(20, Math.min(GAME_WIDTH - 20, player.x))
  }

  // Update bullets
  match.state.bullets = match.state.bullets.filter((bullet) => {
    bullet.y -= BULLET_SPEED
    return bullet.y > 0
  })
}

export function getPlayerCount(matchId: string): number {
  const match = getMatch(matchId)
  return match?.state.players.length ?? 0
}

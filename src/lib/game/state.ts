import type { GameState, Player, Bullet } from '../../types.js'

// Game constants
const PLAYER_SPEED = 5
const BULLET_SPEED = 8
const GAME_WIDTH = 800
const GAME_HEIGHT = 600
const PLAYER_Y = 550

// Single game instance
let gameState: GameState = {
  players: [],
  bullets: [],
  started: false,
}

export function getState(): GameState {
  return gameState
}

export function resetState(): void {
  gameState = {
    players: [],
    bullets: [],
    started: false,
  }
}

export function addPlayer(id: string): Player {
  // Position player based on count (left or right side)
  const x = gameState.players.length === 0 ? 200 : 600

  const player: Player = {
    id,
    x,
    alive: true,
    direction: 0,
  }

  gameState.players.push(player)

  // Auto-start when 2 players connected
  if (gameState.players.length === 2) {
    gameState.started = true
  }

  return player
}

export function removePlayer(id: string): void {
  gameState.players = gameState.players.filter((p) => p.id !== id)
  gameState.bullets = gameState.bullets.filter((b) => b.ownerId !== id)

  // Stop game if not enough players
  if (gameState.players.length < 2) {
    gameState.started = false
  }
}

export function setPlayerDirection(id: string, dir: -1 | 0 | 1): void {
  const player = gameState.players.find((p) => p.id === id)
  if (player && player.alive) {
    player.direction = dir
  }
}

export function playerShoot(id: string): void {
  const player = gameState.players.find((p) => p.id === id)
  if (!player || !player.alive || !gameState.started) return

  const bullet: Bullet = {
    x: player.x,
    y: PLAYER_Y - 10,
    ownerId: id,
  }

  gameState.bullets.push(bullet)
}

export function tick(): void {
  if (!gameState.started) return

  // Update player positions
  for (const player of gameState.players) {
    if (!player.alive) continue

    player.x += player.direction * PLAYER_SPEED

    // Clamp to game bounds
    player.x = Math.max(20, Math.min(GAME_WIDTH - 20, player.x))
  }

  // Update bullets
  gameState.bullets = gameState.bullets.filter((bullet) => {
    bullet.y -= BULLET_SPEED
    // Remove if off screen
    return bullet.y > 0
  })
}

export function getPlayerCount(): number {
  return gameState.players.length
}


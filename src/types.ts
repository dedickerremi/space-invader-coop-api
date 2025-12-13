// === GAME STATE ===

export type Player = {
  id: string
  x: number
  alive: boolean
  direction: -1 | 0 | 1 // -1 left, 0 stopped, 1 right
}

export type Bullet = {
  x: number
  y: number
  ownerId: string
}

export type GameState = {
  players: Player[]
  bullets: Bullet[]
  started: boolean
}

// === CLIENT MESSAGES (inputs) ===

export type MoveMessage = { type: 'MOVE'; dir: -1 | 1 }
export type StopMessage = { type: 'STOP' }
export type ShootMessage = { type: 'SHOOT' }

export type ClientMessage = MoveMessage | StopMessage | ShootMessage

// === SERVER MESSAGES (outputs) ===

export type StateMessage = { type: 'STATE'; state: GameState }
export type WelcomeMessage = { type: 'WELCOME'; playerId: string }
export type ErrorMessage = { type: 'ERROR'; reason: string }

export type ServerMessage = StateMessage | WelcomeMessage | ErrorMessage


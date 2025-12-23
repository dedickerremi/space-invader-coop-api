// === GAME STATE ===

export type Player = {
  id: string
  x: number
  alive: boolean
  direction: -1 | 0 | 1
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
  paused: boolean
  pausedBy: string | null // playerId who paused
}

// === MATCH ===

export type Match = {
  matchId: string
  playerIds: string[]
  tokens: Map<string, string>
  state: GameState
  createdAt: number
}

// === CLIENT MESSAGES (inputs) ===

export type MoveMessage = { type: 'MOVE'; dir: -1 | 1 }
export type StopMessage = { type: 'STOP' }
export type ShootMessage = { type: 'SHOOT' }
export type PauseMessage = { type: 'PAUSE' }
export type ResumeMessage = { type: 'RESUME' }
export type ExitMessage = { type: 'EXIT' }

export type ClientMessage = MoveMessage | StopMessage | ShootMessage | PauseMessage | ResumeMessage | ExitMessage

// === SERVER MESSAGES (outputs) ===

export type StateMessage = { type: 'STATE'; state: GameState }
export type WelcomeMessage = { type: 'WELCOME'; playerId: string; matchId: string }
export type ErrorMessage = { type: 'ERROR'; reason: string }
export type MatchEndedMessage = { type: 'MATCH_ENDED'; reason: string }

export type ServerMessage = StateMessage | WelcomeMessage | ErrorMessage | MatchEndedMessage

import { createServer } from './lib/websocket/server.js'
import { startGameLoop } from './lib/game/loop.js'

const PORT = Number(process.env.PORT) || 3001

console.log('=================================')
console.log(' Space Invaders Coop - Backend')
console.log('=================================')

createServer(PORT)
startGameLoop()

console.log('[SERVER] Ready for connections')


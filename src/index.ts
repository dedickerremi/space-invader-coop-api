import { createServer } from './lib/websocket/server.js'
import { startGameLoop } from './lib/game/loop.js'
import { createMonitoringServer } from './lib/monitoring.js'

const WS_PORT = Number(process.env.WS_PORT) || 3001
const MONITORING_PORT = Number(process.env.MONITORING_PORT) || 3002

console.log('=================================')
console.log(' Space Invaders Coop - Backend')
console.log('=================================')

createServer(WS_PORT)
startGameLoop()
createMonitoringServer(MONITORING_PORT)

console.log(`[SERVER] WebSocket server ready on port ${WS_PORT}`)
console.log(`[SERVER] Monitoring dashboard: http://localhost:${MONITORING_PORT}`)


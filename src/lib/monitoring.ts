import Fastify from 'fastify'
import { getServerStats } from './stats.js'

export function createMonitoringServer(port: number) {
  const fastify = Fastify({ logger: true })

  // Stats API endpoint (JSON)
  fastify.get('/api/stats', async () => {
    return getServerStats()
  })

  // HTML dashboard page
  fastify.get('/', async (request, reply) => {
    const stats = getServerStats()

    const html = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Space Invaders Coop - Server Status</title>
  <style>
    * {
      margin: 0;
      padding: 0;
      box-sizing: border-box;
    }

    body {
      font-family: 'JetBrains Mono', 'Fira Code', monospace;
      background: #0a0a0f;
      color: #e0e0e0;
      padding: 2rem;
      line-height: 1.6;
    }

    .container {
      max-width: 1200px;
      margin: 0 auto;
    }

    h1 {
      color: #00ff88;
      text-transform: uppercase;
      letter-spacing: 0.3em;
      text-shadow: 0 0 20px rgba(0, 255, 136, 0.5);
      margin-bottom: 2rem;
    }

    .stats-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
      gap: 1.5rem;
      margin-bottom: 2rem;
    }

    .stat-card {
      background: #1a1a24;
      border: 2px solid #333;
      border-radius: 8px;
      padding: 1.5rem;
    }

    .stat-card h2 {
      color: #888;
      font-size: 0.875rem;
      text-transform: uppercase;
      letter-spacing: 0.1em;
      margin-bottom: 0.5rem;
    }

    .stat-card .value {
      color: #00ff88;
      font-size: 2rem;
      font-weight: bold;
    }

    .matches-table {
      background: #1a1a24;
      border: 2px solid #333;
      border-radius: 8px;
      overflow: hidden;
    }

    .matches-table h2 {
      padding: 1rem 1.5rem;
      border-bottom: 2px solid #333;
      color: #00ff88;
    }

    table {
      width: 100%;
      border-collapse: collapse;
    }

    th, td {
      padding: 1rem 1.5rem;
      text-align: left;
      border-bottom: 1px solid #333;
    }

    th {
      color: #888;
      text-transform: uppercase;
      font-size: 0.75rem;
      letter-spacing: 0.1em;
    }

    td {
      color: #e0e0e0;
    }

    .match-id {
      color: #00aaff;
      font-family: monospace;
      font-size: 0.875rem;
    }

    .badge {
      display: inline-block;
      padding: 0.25rem 0.75rem;
      border-radius: 4px;
      font-size: 0.75rem;
      font-weight: bold;
      text-transform: uppercase;
    }

    .badge.started {
      background: #00ff88;
      color: #000;
    }

    .badge.waiting {
      background: #ffaa00;
      color: #000;
    }

    .badge.paused {
      background: #ff4444;
      color: #fff;
    }

    .refresh-btn {
      position: fixed;
      bottom: 2rem;
      right: 2rem;
      background: #00ff88;
      color: #000;
      border: none;
      padding: 1rem 2rem;
      border-radius: 8px;
      font-family: inherit;
      font-weight: bold;
      text-transform: uppercase;
      cursor: pointer;
      box-shadow: 0 0 20px rgba(0, 255, 136, 0.3);
      transition: all 0.2s;
    }

    .refresh-btn:hover {
      background: #00cc6a;
      transform: scale(1.05);
    }

    .empty {
      padding: 2rem;
      text-align: center;
      color: #666;
    }
  </style>
</head>
<body>
  <div class="container">
    <h1>Space Invaders Coop - Server Status</h1>

    <div class="stats-grid">
      <div class="stat-card">
        <h2>Active Matches</h2>
        <div class="value">${stats.activeMatches} / ${stats.maxMatches}</div>
      </div>
      <div class="stat-card">
        <h2>Total Players</h2>
        <div class="value">${stats.totalPlayers}</div>
      </div>
      <div class="stat-card">
        <h2>Uptime</h2>
        <div class="value" id="uptime">--</div>
      </div>
    </div>

    <div class="matches-table">
      <h2>Active Matches</h2>
      ${stats.matches.length === 0 ? '<div class="empty">No active matches</div>' : `
      <table>
        <thead>
          <tr>
            <th>Match ID</th>
            <th>Players</th>
            <th>Status</th>
            <th>Age</th>
          </tr>
        </thead>
        <tbody>
          ${stats.matches.map(match => `
            <tr>
              <td class="match-id">${match.matchId}</td>
              <td>${match.connectedPlayers} / ${match.playerCount}</td>
              <td>
                ${match.paused ? '<span class="badge paused">Paused</span>' : 
                  match.started ? '<span class="badge started">Started</span>' : 
                  '<span class="badge waiting">Waiting</span>'}
              </td>
              <td>${formatAge(match.age)}</td>
            </tr>
          `).join('')}
        </tbody>
      </table>
      `}
    </div>
  </div>

  <button class="refresh-btn" onclick="location.reload()">Refresh</button>

  <script>
    // Auto-refresh every 2 seconds
    setInterval(() => {
      location.reload();
    }, 2000);

    // Calculate uptime (simplified - would need server start time)
    const startTime = Date.now();
    function updateUptime() {
      const elapsed = Math.floor((Date.now() - startTime) / 1000);
      const hours = Math.floor(elapsed / 3600);
      const minutes = Math.floor((elapsed % 3600) / 60);
      const seconds = elapsed % 60;
      document.getElementById('uptime').textContent = 
        \`\${hours}h \${minutes}m \${seconds}s\`;
    }
    updateUptime();
    setInterval(updateUptime, 1000);
  </script>
</body>
</html>
    `

    reply.type('text/html').send(html)
  })

  fastify.listen({ port, host: '0.0.0.0' }, (err) => {
    if (err) {
      console.error('[MONITORING] Error starting server:', err)
      return
    }
    console.log(`[MONITORING] Dashboard available at http://localhost:${port}`)
  })

  return fastify
}

function formatAge(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  return `${hours}h ${minutes}m`
}


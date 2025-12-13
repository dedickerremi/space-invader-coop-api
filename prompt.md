# RÔLE
Tu es un développeur senior spécialisé dans les serveurs de jeux temps réel.

# OBJECTIF — ÉTAPE 1 (PoC BACKEND)
Créer un serveur de jeu **minimal** en **Node.js + TypeScript** utilisant **WebSocket**.
Ce serveur est destiné uniquement à un **PoC**.

Le serveur doit permettre à **2 joueurs maximum** de jouer ensemble
dans **UNE seule partie coopérative**.

Aucune authentification.
Aucune base de données.
Aucune persistance.
Aucune logique inutile.

---

# CONTRAINTES TECHNIQUES
- Node.js
- TypeScript
- Librairie WebSocket : `ws`
- Un seul processus
- Un seul match en mémoire
- Tick serveur fixe : **30 Hz**
- Code volontairement simple et lisible
- Code logic separation. websocket into lib/websocket/* etc etc

---

# COMPORTEMENT ATTENDU

## Connexions
- Le serveur accepte des connexions WebSocket
- Maximum **2 clients connectés**
- Les connexions supplémentaires sont refusées

## Partie
- La partie démarre automatiquement quand **2 joueurs sont connectés**
- Le serveur maintient un **game loop**
- Le serveur est **authoritative** (le client n’est jamais source de vérité)

---

# INPUTS CLIENT (INTENTIONS UNIQUEMENT)
Le client peut envoyer uniquement les messages suivants :

```ts
{ type: "MOVE"; dir: -1 | 1 }
{ type: "STOP" }
{ type: "SHOOT" }

Le client n’envoie jamais :
positions
score
vie
état du jeu

LOGIQUE DE JEU MINIMALE
Chaque joueur possède :

un id
une position horizontale x
un état alive
Le mouvement est uniquement horizontal
Le tir crée une balle qui monte verticalement
Les ennemis sont optionnels pour cette étape
Les collisions peuvent être très simples ou absentes

SORTIE SERVEUR
À intervalle régulier, le serveur envoie à tous les clients :

ts
Copy code
{ type: "STATE"; state: GameState }
GAME STATE MINIMAL
ts
Copy code
type GameState = {
  players: {
    id: string
    x: number
    alive: boolean
  }[]
  bullets: {
    x: number
    y: number
  }[]
}
ATTENTES IMPORTANTES
Le code doit être simple, lisible et direct

Pas d’architecture complexe

Pas d’optimisation prématurée

Pas de classes lourdes

Commentaires courts et utiles uniquement

Le but est d’obtenir rapidement un serveur fonctionnel
sur lequel on pourra itérer facilement par la suite.
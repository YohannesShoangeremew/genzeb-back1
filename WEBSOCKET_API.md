# WebSocket API Documentation

## Overview

The WebSocket API provides real-time updates for bingo games. Clients connect to receive game state changes, number draws, player events, and other game-related updates.

## Connection

### Endpoint

**Option 1: Connect by Game Type (Public Viewing - Recommended)**
```
ws://localhost:8080/api/v1/ws/game?type=VIP
```

**Option 2: Connect by Game ID**
```
ws://localhost:8080/api/v1/ws/game/:gameId
```

**Production:**
```
wss://web-production-201fa.up.railway.app/api/v1/ws/game?type=VIP
wss://web-production-201fa.up.railway.app/api/v1/ws/game/:gameId
```

### Parameters

**For Game Type Connection:**
- **type** (query parameter, required): Game type (REGULAR or VIP)
  - Automatically finds or creates an available game of that type
  - **No user authentication required** - anyone can watch

**For Game ID Connection:**
- **gameId** (path parameter, required): UUID of the game
  - **No user authentication required** - anyone can watch

### Requirements

1. Redis must be configured on the server
2. Valid WebSocket connection
3. **No need to be a player** - WebSocket is public/read-only for viewing

### Connection Example (JavaScript)

**By Game Type (Recommended):**
```javascript
// Connect to VIP game type - automatically finds available game
const wsUrl = `ws://localhost:8080/api/v1/ws/game?type=VIP`;

const ws = new WebSocket(wsUrl);
```

**By Game ID:**
```javascript
const gameId = "550e8400-e29b-41d4-a716-446655440000";
const wsUrl = `ws://localhost:8080/api/v1/ws/game/${gameId}`;

const ws = new WebSocket(wsUrl);
```

ws.onopen = () => {
  console.log("WebSocket connected");
};

ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  handleWebSocketMessage(message);
};

ws.onerror = (error) => {
  console.error("WebSocket error:", error);
};

ws.onclose = (event) => {
  console.log("WebSocket closed:", event.code, event.reason);
};
```

### Connection Example (React)

```typescript
import { useEffect, useRef } from 'react';

interface WebSocketMessage {
  event: string;
  data: any;
}

// Connect by game type
function useGameWebSocket(gameType: string) {
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    const wsUrl = `ws://localhost:8080/api/v1/ws/game?type=${gameType}`;
    const ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      console.log('WebSocket connected');
    };

    ws.onmessage = (event) => {
      const message: WebSocketMessage = JSON.parse(event.data);
      handleMessage(message);
    };

    ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };

    ws.onclose = () => {
      console.log('WebSocket closed');
      // Optionally reconnect
    };

    wsRef.current = ws;

    return () => {
      ws.close();
    };
  }, [gameType]);

  return wsRef.current;
}
```

## Message Format

All WebSocket messages follow this structure:

```json
{
  "event": "EVENT_NAME",
  "data": {
    // Event-specific data
  }
}
```

## Events

### 1. INITIAL_STATE

Sent immediately after connection is established. Contains the current game state.

**Event:** `INITIAL_STATE`

**Data:**
```json
{
  "event": "INITIAL_STATE",
  "data": {
    "game": {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "game_type": "REGULAR",
      "state": "WAITING",
      "bet_amount": 5.00,
      "min_players": 2,
      "player_count": 3,
      "prize_pool": 14.25,
      "house_cut": 0.05,
      "winner_id": null,
      "countdown_ends": null,
      "started_at": null,
      "finished_at": null,
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    },
    "drawnNumbers": [
      {
        "letter": "B",
        "number": 7,
        "drawn_at": "2024-01-01T00:05:00Z"
      }
    ],
    "takenCards": [1, 5, 12, 33],
    "playerCount": 3,
    "secondsLeft": 0
  }
}
```

**Game States:**
- `WAITING`: Game is waiting for players (minimum 2 required)
- `COUNTDOWN`: 60-second countdown before game starts (starts when 2nd player joins)
- `DRAWING`: Numbers are being drawn
- `FINISHED`: Game finished, winner determined
- `CLOSED`: Game archived
- `CANCELLED`: Game cancelled (all players eliminated or other error)

**State Transitions:**
- **WAITING** → **COUNTDOWN**: When 2nd player joins
- **COUNTDOWN** → **WAITING**: If players drop below 2 during countdown (countdown stops, remaining players stay)
- **COUNTDOWN** → **DRAWING**: When countdown reaches 0
- **DRAWING** → **FINISHED**: When valid bingo is claimed
- **DRAWING** → **CANCELLED**: If all players are eliminated

### 2. GAME_STATUS

Sent when game state changes (e.g., WAITING → COUNTDOWN → DRAWING).

**Event:** `GAME_STATUS`

**Data:**
```json
{
  "event": "GAME_STATUS",
  "data": {
    "state": "COUNTDOWN",
    "player_count": 3,
    "prize_pool": 14.25
  }
}
```

### 3. PLAYER_COUNT

Sent when the number of players changes.

**Event:** `PLAYER_COUNT`

**Data:**
```json
{
  "event": "PLAYER_COUNT",
  "data": {
    "count": 3
  }
}
```

### 4. CARDS_TAKEN

Sent when a card is taken by a player.

**Event:** `CARDS_TAKEN`

**Data:**
```json
{
  "event": "CARDS_TAKEN",
  "data": {
    "takenCards": [1, 5, 12, 33, 45]
  }
}
```

### 5. COUNTDOWN

Sent during countdown phase. Includes seconds remaining.

**Event:** `COUNTDOWN`

**Data:**
```json
{
  "event": "COUNTDOWN",
  "data": {
    "secondsLeft": 10
  }
}
```

### 6. NUMBER_DRAWN

Sent when a new number is drawn.

**Event:** `NUMBER_DRAWN`

**Data:**
```json
{
  "event": "NUMBER_DRAWN",
  "data": {
    "letter": "B",
    "number": 7,
    "drawn_at": "2024-01-01T00:05:00Z"
  }
}
```

### 7. PLAYER_JOINED

Sent when a player joins the game.

**Event:** `PLAYER_JOINED`

**Data:**
```json
{
  "event": "PLAYER_JOINED",
  "data": {
    "user_id": "770e8400-e29b-41d4-a716-446655440000",
    "card_id": 12
  }
}
```

### 8. PLAYER_ELIMINATED

Sent when a player is eliminated (invalid bingo claim).

**Event:** `PLAYER_ELIMINATED`

**Data:**
```json
{
  "event": "PLAYER_ELIMINATED",
  "data": {
    "user_id": "770e8400-e29b-41d4-a716-446655440000"
  }
}
```

### 9. WINNER

Sent when a player wins the game.

**Event:** `WINNER`

**Data:**
```json
{
  "event": "WINNER",
  "data": {
    "user_id": "770e8400-e29b-41d4-a716-446655440000",
    "prize": 14.25
  }
}
```

## Error Handling

### Connection Errors

If the connection fails, the server will return an HTTP error before upgrading to WebSocket. All error responses include both `error` and `reason` fields for detailed debugging:

**400 Bad Request:**
```json
{
  "error": "Invalid game ID format 'invalid-uuid': invalid UUID format",
  "reason": "Invalid game ID format 'invalid-uuid': invalid UUID format"
}
```

```json
{
  "error": "Invalid game type 'G10'. Must be one of: REGULAR, VIP",
  "reason": "Invalid game type 'G10'. Must be one of: REGULAR, VIP"
}
```

```json
{
  "error": "No game type or game ID provided. Use ?type=VIP or /ws/game/:gameId",
  "reason": "No game type or game ID provided. Use ?type=VIP or /ws/game/:gameId"
}
```

**500 Internal Server Error:**
```json
{
  "error": "Failed to create or get game of type VIP: database error",
  "reason": "Failed to create or get game of type VIP: database error"
}
```

**503 Service Unavailable:**
```json
{
  "error": "Redis client is nil. WebSocket requires Redis for real-time updates.",
  "reason": "Redis client is nil. WebSocket requires Redis for real-time updates."
}
```

```json
{
  "error": "Redis ping failed: connection refused. Check Redis connection and credentials.",
  "reason": "Redis ping failed: connection refused. Check Redis connection and credentials."
}
```

### WebSocket Close Codes

- `1000`: Normal closure
- `1001`: Going away (server shutdown)
- `1006`: Abnormal closure (connection lost, network error, or timeout)
- `1011`: Internal server error

**Note:** The server handles connection failures gracefully. If you see a 1006 error, it may indicate:
- Network connectivity issues
- Server restart or deployment
- Redis connection issues (though the connection should fail before upgrade if Redis is unavailable)

## Keepalive

The server sends ping messages every 54 seconds to keep the connection alive. The client should respond with pong automatically (handled by the browser WebSocket API). The read deadline is set to 60 seconds, so if no pong is received, the connection will timeout.

**Connection Management:**
- Server sends ping every 54 seconds
- Read deadline is 60 seconds (automatically extended on pong)
- Connection will close if no pong received within 60 seconds
- All connection errors are logged server-side for debugging

## Complete Example

```typescript
class GameWebSocket {
  private ws: WebSocket | null = null;
  private gameId: string; // Can be game type (REGULAR or VIP) or game ID (UUID)
  private onMessageCallback: ((message: any) => void) | null = null;

  constructor(gameTypeOrId: string) {
    // Can be either game type (REGULAR or VIP) or game ID (UUID)
    this.gameId = gameTypeOrId;
  }

  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      // Connect by game type (recommended) or game ID
      let wsUrl: string;
      if (this.gameId.match(/^G[1-7]$/)) {
        // Game type
        wsUrl = `ws://localhost:8080/api/v1/ws/game?type=${this.gameId}`;
      } else {
        // Game ID (UUID)
        wsUrl = `ws://localhost:8080/api/v1/ws/game/${this.gameId}`;
      }
      this.ws = new WebSocket(wsUrl);

      this.ws.onopen = () => {
        console.log('WebSocket connected');
        resolve();
      };

      this.ws.onmessage = (event) => {
        const message = JSON.parse(event.data);
        this.handleMessage(message);
      };

      this.ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        reject(error);
      };

      this.ws.onclose = (event) => {
        console.log('WebSocket closed:', event.code, event.reason);
        this.ws = null;
      };
    });
  }

  private handleMessage(message: any) {
    switch (message.event) {
      case 'INITIAL_STATE':
        console.log('Initial state:', message.data);
        break;
      case 'GAME_STATUS':
        console.log('Game status changed:', message.data.state);
        break;
      case 'PLAYER_COUNT':
        console.log('Player count:', message.data.count);
        break;
      case 'NUMBER_DRAWN':
        console.log('Number drawn:', message.data.letter, message.data.number);
        break;
      case 'PLAYER_JOINED':
        console.log('Player joined:', message.data.user_id);
        break;
      case 'PLAYER_ELIMINATED':
        console.log('Player eliminated:', message.data.user_id);
        break;
      case 'WINNER':
        console.log('Winner:', message.data.user_id, 'Prize:', message.data.prize);
        break;
      case 'COUNTDOWN':
        console.log('Countdown:', message.data.secondsLeft);
        break;
      case 'CARDS_TAKEN':
        console.log('Cards taken:', message.data.takenCards);
        break;
      default:
        console.log('Unknown event:', message.event);
    }

    if (this.onMessageCallback) {
      this.onMessageCallback(message);
    }
  }

  onMessage(callback: (message: any) => void) {
    this.onMessageCallback = callback;
  }

  disconnect() {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }
}

// Usage - by game type (recommended)
const gameWS = new GameWebSocket("VIP"); // Automatically finds/creates a VIP game

// Or by specific game ID
// const gameWS = new GameWebSocket("550e8400-e29b-41d4-a716-446655440000");

gameWS.onMessage((message) => {
  // Handle all messages
  updateUI(message);
});

gameWS.connect()
  .then(() => {
    console.log('Connected to game');
  })
  .catch((error) => {
    console.error('Failed to connect:', error);
  });
```

## Best Practices

1. **Reconnection**: Implement automatic reconnection on disconnect
2. **Error Handling**: Always handle connection errors gracefully
3. **State Management**: Use INITIAL_STATE to sync your local state
4. **Cleanup**: Always close WebSocket connections when component unmounts
5. **Production**: Use `wss://` (secure WebSocket) in production

## Reconnection Example

```typescript
class ReconnectingWebSocket {
  private ws: WebSocket | null = null;
  private url: string;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private reconnectDelay = 1000;

  constructor(url: string) {
    this.url = url;
  }

  connect() {
    this.ws = new WebSocket(this.url);

    this.ws.onopen = () => {
      console.log('Connected');
      this.reconnectAttempts = 0;
    };

    this.ws.onclose = () => {
      if (this.reconnectAttempts < this.maxReconnectAttempts) {
        this.reconnectAttempts++;
        setTimeout(() => this.connect(), this.reconnectDelay * this.reconnectAttempts);
      }
    };

    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };
  }
}
```

## Testing

You can test the WebSocket connection using a browser console:

**By Game Type (Recommended):**
```javascript
const ws = new WebSocket('ws://localhost:8080/api/v1/ws/game?type=VIP');

ws.onopen = () => console.log('Connected');
ws.onmessage = (e) => console.log('Message:', JSON.parse(e.data));
ws.onerror = (e) => console.error('Error:', e);
ws.onclose = (e) => console.log('Closed:', e.code, e.reason);
```

**By Game ID:**
```javascript
const ws = new WebSocket('ws://localhost:8080/api/v1/ws/game/YOUR_GAME_ID');

ws.onopen = () => console.log('Connected');
ws.onmessage = (e) => console.log('Message:', JSON.parse(e.data));
ws.onerror = (e) => console.error('Error:', e);
ws.onclose = (e) => console.log('Closed:', e.code, e.reason);
```

**Production (Railway):**
```javascript
const ws = new WebSocket('wss://web-production-201fa.up.railway.app/api/v1/ws/game?type=VIP');
```

## Notes

- The WebSocket connection is **read-only** - clients cannot send commands
- All game actions (join, leave, claim bingo) must be done via REST API
- The WebSocket only provides real-time updates
- **Public viewing** - No authentication required, anyone can watch games
- Connect by game type (REGULAR or VIP) to automatically find available games
- Redis must be configured on the server for WebSocket to work
- Error responses include both `error` and `reason` fields for detailed debugging
- Server logs all connection attempts and errors with `[WebSocket]` prefix for easy debugging
- Connection automatically handles timeouts and reconnection is recommended on client side



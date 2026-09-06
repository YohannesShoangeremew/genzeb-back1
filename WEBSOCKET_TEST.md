# WebSocket Connection Test Guide

## Quick Test

### Using Browser Console

1. Open your browser's developer console (F12)
2. Run this code (replace with your actual game ID and user ID):

```javascript
const gameId = "YOUR_GAME_ID";
const userId = "YOUR_USER_ID";
const wsUrl = `wss://web-production-201fa.up.railway.app/api/v1/ws/game/${gameId}?user_id=${userId}`;

const ws = new WebSocket(wsUrl);

ws.onopen = () => {
  console.log("✅ WebSocket connected!");
};

ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  console.log("📨 Message received:", message);
};

ws.onerror = (error) => {
  console.error("❌ WebSocket error:", error);
};

ws.onclose = (event) => {
  console.log("🔌 WebSocket closed:", event.code, event.reason);
};
```

### Using the Test HTML File

1. Open `websocket_test.html` in your browser
2. Enter your backend URL: `https://web-production-201fa.up.railway.app`
3. Enter a valid Game ID (UUID)
4. Enter a valid User ID (UUID) - must be a player in the game
5. Click "Connect"
6. Check the messages area for connection status and events

## Prerequisites

Before testing WebSocket:

1. **User must be in the game**: The user must have joined the game via:
   ```bash
   POST https://web-production-201fa.up.railway.app/api/v1/games/{gameId}/join
   {
     "user_id": "your-user-id",
     "card_id": 1
   }
   ```

2. **Redis must be configured**: WebSocket requires Redis to be running on the server

3. **Valid UUIDs**: Both gameId and userId must be valid UUIDs

## Common Issues

### Connection Refused / Failed to Connect

**Possible causes:**
- Redis is not configured on Railway
- User is not a player in the game
- Invalid game ID or user ID
- Network/firewall issues

**Solutions:**
1. Check Railway logs for errors
2. Verify Redis service is running in Railway
3. Ensure user has joined the game first
4. Verify UUIDs are correct

### 403 Forbidden

**Error:** `"User is not in this game"`

**Solution:** User must join the game via REST API before connecting via WebSocket

### 503 Service Unavailable

**Error:** `"WebSocket requires Redis to be configured"`

**Solution:** Add Redis service in Railway and configure environment variables

### Connection Closes Immediately

**Possible causes:**
- Redis connection failed
- User validation failed
- Server error

**Solution:** Check Railway logs for detailed error messages

## Testing Steps

1. **Create/Get a game:**
   ```bash
   curl https://web-production-201fa.up.railway.app/api/v1/games?type=REGULAR
   ```

2. **Join the game:**
   ```bash
   curl -X POST https://web-production-201fa.up.railway.app/api/v1/games/{gameId}/join \
     -H "Content-Type: application/json" \
     -d '{
       "user_id": "your-user-id",
       "card_id": 1
     }'
   ```

3. **Connect via WebSocket:**
   ```javascript
   const ws = new WebSocket(
     `wss://web-production-201fa.up.railway.app/api/v1/ws/game/${gameId}?user_id=${userId}`
   );
   ```

4. **Verify connection:**
   - Should receive `INITIAL_STATE` event immediately
   - Connection should remain open
   - Should receive ping messages every 54 seconds

## Expected Behavior

### On Successful Connection:

1. **Immediate:** `INITIAL_STATE` event with full game data
2. **Every 54 seconds:** Ping message (handled automatically by browser)
3. **On game events:** Real-time updates (NUMBER_DRAWN, PLAYER_JOINED, etc.)

### Example INITIAL_STATE:

```json
{
  "event": "INITIAL_STATE",
  "data": {
    "game": {
      "id": "...",
      "state": "WAITING",
      "player_count": 2,
      ...
    },
    "drawnNumbers": [],
    "takenCards": [1, 5],
    "playerCount": 2,
    "secondsLeft": 0
  }
}
```

## Railway-Specific Notes

- Railway uses HTTPS, so WebSocket must use `wss://` (secure WebSocket)
- Railway automatically handles SSL/TLS termination
- Check Railway logs if connection fails: Dashboard → Your Service → Logs
- Ensure Redis service is added and connected in Railway

## Debugging

### Check Server Logs

In Railway dashboard:
1. Go to your service
2. Click "Logs"
3. Look for WebSocket-related errors

### Test REST API First

Before testing WebSocket, verify REST API works:
```bash
# Health check
curl https://web-production-201fa.up.railway.app/health

# Get games
curl https://web-production-201fa.up.railway.app/api/v1/games?type=REGULAR
```

### Network Tab

In browser DevTools → Network tab:
1. Filter by "WS" (WebSocket)
2. Click on the WebSocket connection
3. Check "Messages" tab for sent/received messages
4. Check "Headers" for connection details


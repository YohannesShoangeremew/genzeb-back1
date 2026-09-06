# Bingo Backend

A high-performance backend service for the Bingo bot built with Go, Gin framework, PostgreSQL, and Redis.

## Tech Stack

- **Backend**: Golang (Gin)
- **Database**: PostgreSQL
- **Cache**: Redis
- **Realtime**: WebSockets
- **Security**: JWT + HTTPS + Transactions

## Architecture

This project follows Clean Architecture principles:

```
backend/
├── cmd/server/          # Application entry point
├── internal/
│   ├── domain/          # Domain entities and interfaces
│   ├── usecase/         # Business logic
│   ├── handler/         # HTTP handlers
│   └── repository/      # Data access layer
├── pkg/                 # Shared utilities
├── config/              # Configuration management
└── migrations/          # Database migrations
```

## Quick Start

**Fastest way to get started (using Docker):**

```bash
# 1. Start API + PostgreSQL + Redis
make docker-up
```

Server will be available at `http://localhost:8080`

For detailed instructions, see [QUICKSTART.md](QUICKSTART.md)

## Setup

### Prerequisites

- Go 1.21 or higher
- Docker and Docker Compose (recommended)
- OR PostgreSQL 12+ and Redis 6+ installed locally

### Installation

#### Option 1: Using Docker (Recommended)

1. Start database services:

```bash
make docker-up
```

This starts the API, PostgreSQL, and Redis together. The database is automatically initialized on first start.

#### Option 2: Local Installation

1. Install dependencies:

```bash
go mod download
```

2. Create the database:

```bash
createdb bingo
```

3. Run migrations:

```bash
make migrate-up
```

4. (Optional) Set up environment variables:
   Create a `.env` file with your configuration, or use the defaults:

- Server: `localhost:8080`
- Database: `postgres@localhost:5432/bingo`
- Redis: `localhost:6379`
- Payment verifier: set `VERIFY_API_KEY` to enable Telebirr, CBE Birr, and M-Pesa receipt verification. The [official verification guide](https://verify.leul.et/docs/verification) documents the dedicated CBE Birr route; `VERIFY_API_BASE_URL` defaults to its hosted API at `https://verifyapi.leulzenebe.pro`.

5. Run the server:

```bash
make run
# Or: go run cmd/server/main.go
```

## Authentication

### POST /api/v1/auth/login

Admin login endpoint. Returns a JWT token for accessing protected admin endpoints.

**Request Body:**

```json
{
  "telegram_id": 123456789,
  "password": "admin_password"
}
```

**Response:**

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "telegram_id": 123456789,
    "first_name": "Admin",
    "last_name": "User",
    "phone_number": "1234567890",
    "referal_code": "ADMIN001",
    "role": "admin",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**

- `400`: Invalid request data
- `401`: Invalid credentials
- `403`: User is not an admin

**Usage:**
Include the token in the Authorization header for admin endpoints:

```
Authorization: Bearer <token>
```

## API Endpoints

### POST /api/v1/user/register

Register a new user and create their wallet. This operation is atomic - both user and wallet are created in a single transaction.

**Request Body:**

```json
{
  "telegram_id": 123456789,
  "first_name": "John",
  "last_name": "Doe",
  "phone": "+1234567890"
}
```

**Response:**

```json
{
  "message": "User and wallet created successfully",
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "telegram_id": 123456789,
    "first_name": "John",
    "last_name": "Doe",
    "phone_number": "1234567890",
    "referal_code": "ABC123XY",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  },
  "wallet": {
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "balance": 5.0,
    "demo_balance": 0.0,
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Response (409):**

```json
{
  "error": "user with this telegram ID already exists"
}
```

### GET /api/v1/user/telegram/:telegram_id

Find a user by their Telegram ID.

**Path Parameters:**

- `telegram_id` (int64, required): The Telegram ID of the user

**Response:**

```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "telegram_id": 123456789,
    "first_name": "John",
    "last_name": "Doe",
    "phone_number": "1234567890",
    "referal_code": "ABC123XY",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Response (404):**

```json
{
  "error": "User not found"
}
```

### GET /api/v1/user/phone?phone=+1234567890

Find a user by their phone number. The phone number will be automatically normalized.

**Query Parameters:**

- `phone` (string, required): The phone number of the user

**Response:**

```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "telegram_id": 123456789,
    "first_name": "John",
    "last_name": "Doe",
    "phone_number": "1234567890",
    "referal_code": "ABC123XY",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Response (404):**

```json
{
  "error": "User not found"
}
```

### GET /api/v1/user/referral/:referral_code

Find a user by their referral code.

**Path Parameters:**

- `referral_code` (string, required): The referral code of the user

**Response:**

```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "telegram_id": 123456789,
    "first_name": "John",
    "last_name": "Doe",
    "phone_number": "1234567890",
    "referal_code": "ABC123XY",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Response (404):**

```json
{
  "error": "User not found"
}
```

### PUT /api/v1/user/:user_id/name

Update a user's first and last name.

**Path Parameters:**

- `user_id` (UUID, required): The user ID

**Request Body:**

```json
{
  "first_name": "Jane",
  "last_name": "Smith"
}
```

**Request Fields:**

- `first_name` (string, required): The new first name
- `last_name` (string, optional): The new last name (can be null)

**Response:**

```json
{
  "message": "User name updated successfully",
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "telegram_id": 123456789,
    "first_name": "Jane",
    "last_name": "Smith",
    "phone_number": "1234567890",
    "referal_code": "ABC123XY",
    "role": "user",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:01:00Z"
  }
}
```

**Error Responses:**

- `400`: Invalid user ID or invalid request data
- `404`: User not found
- `500`: Failed to update user

**Example:**

```bash
curl -X PUT http://localhost:8080/api/v1/user/550e8400-e29b-41d4-a716-446655440000/name \
  -H "Content-Type: application/json" \
  -d '{
    "first_name": "Jane",
    "last_name": "Smith"
  }'
```

## Wallet Endpoints

### POST /api/v1/wallet/deposit

Create a deposit request using Telebirr, CBE Birr, or M-Pesa. If `VERIFY_API_KEY` is configured, the backend verifies the submitted payment reference against the external verifier, checks its provider, destination account, and amount, then completes the deposit and credits the wallet immediately. Without verifier configuration, the transaction is created with `pending` status and balance is not updated until admin approval.

If the verifier is configured but cannot be reached (network failure, timeout, 5xx, auth, or rate-limit), the deposit falls back to a `pending` transaction for manual admin approval instead of being rejected. A definitive negative verdict (receipt not found, amount/provider mismatch) is still rejected.

**Request Body:**

```json
{
  "amount": 100.00,
  "transaction_type": "Telebirr",
  "transaction_id": "tx_123456789"
}
```

**Response:**

```json
{
  "message": "Deposit verified and completed successfully",
  "transaction": {
    "id": "660e8400-e29b-41d4-a716-446655440001",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "deposit",
    "amount": 100.00,
    "status": "completed",
    "transaction_type": "Telebirr",
    "transaction_id": "tx_123456789",
    "reference": null,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**

- `400`: Invalid amount, unsupported payment method, failed payment verification, or amount mismatch
- `404`: User not found
- `409`: Transaction reference already used

### POST /api/v1/wallet/withdraw

Create a withdrawal request. The balance is **immediately subtracted** and the transaction is created with `pending` status. If admin rejects, balance is refunded.

Payouts default to the user's **verified registration phone** (the number they shared with the bot). The player may supply a different `account_number`, but it must be a valid Ethiopian Telebirr mobile — it's normalized server-side. Requirements: `account_type` must be `Telebirr`, amount ≥ the minimum withdrawal (10 birr), the user must have at least one completed **real** cash-in deposit, and the remaining balance must stay ≥ 10.

**Request Body:**

```json
{
  "amount": 50.00,
  "account_type": "Telebirr",
  "account_number": "0997709691"
}
```

`account_number` is optional — omit it to pay out to the registered phone.

**Response:**

```json
{
  "message": "Withdrawal processed successfully",
  "transaction": {
    "id": "660e8400-e29b-41d4-a716-446655440002",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "withdraw",
    "amount": 50.00,
    "status": "pending",
    "transaction_type": null,
    "transaction_id": null,
    "reference": null,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**

- `400`: Invalid amount or insufficient balance
- `404`: User or wallet not found

### POST /api/v1/wallet/transfer

Transfer money from one user to another. This is an **atomic operation** - both wallets are updated and two transactions are created in a single database transaction.

**Request Body:**

```json
{
  "sender_id": "550e8400-e29b-41d4-a716-446655440000",
  "receiver_id": "770e8400-e29b-41d4-a716-446655440000",
  "amount": 25.00
}
```

**Response:**

```json
{
  "message": "Transfer completed successfully",
  "sender_tx": {
    "id": "660e8400-e29b-41d4-a716-446655440003",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "transfer_out",
    "amount": 25.00,
    "status": "completed",
    "transaction_type": null,
    "transaction_id": null,
    "reference": "770e8400-e29b-41d4-a716-446655440000",
    "created_at": "2024-01-01T00:00:00Z"
  },
  "receiver_tx": {
    "id": "660e8400-e29b-41d4-a716-446655440004",
    "user_id": "770e8400-e29b-41d4-a716-446655440000",
    "type": "transfer_in",
    "amount": 25.00,
    "status": "completed",
    "transaction_type": null,
    "transaction_id": null,
    "reference": "550e8400-e29b-41d4-a716-446655440000",
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**

- `400`: Invalid amount, insufficient balance, or self-transfer attempt
- `404`: Sender or receiver not found

**Transfer Rules:**

- ❌ No self-transfers
- ❌ No negative or zero amounts
- ❌ Receiver must exist
- ✅ Atomic operation (all-or-nothing)

### GET /api/v1/wallet/telegram/:telegram_id

Get wallet information by Telegram ID (convenient for bot access).

**Path Parameters:**

- `telegram_id` (int64, required): The Telegram ID of the user

**Response:**

```json
{
  "wallet": {
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "balance": 75.00,
    "demo_balance": 0.00,
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Response (404):**

```json
{
  "error": "Wallet not found"
}
```

### GET /api/v1/wallet/:user_id

Get wallet information by user UUID.

**Path Parameters:**

- `user_id` (UUID, required): The user ID

**Response:**

```json
{
  "wallet": {
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "balance": 75.00,
    "demo_balance": 0.00,
    "updated_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Response (404):**

```json
{
  "error": "Wallet not found"
}
```

### GET /api/v1/wallet/:user_id/deposits

Get the top 10 deposit transactions for a user, ordered by most recent first.

**Path Parameters:**

- `user_id` (UUID, required): The user ID

**Response:**

```json
{
  "deposits": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "deposit",
      "amount": 100.00,
      "status": "completed",
      "transaction_type": "Telebirr",
      "transaction_id": "tx_123456789",
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1
}
```

**Error Responses:**

- `400`: Invalid user ID
- `500`: Failed to fetch deposit history

### GET /api/v1/wallet/:user_id/withdrawals

Get the top 10 withdrawal transactions for a user, ordered by most recent first.

**Path Parameters:**

- `user_id` (UUID, required): The user ID

**Response:**

```json
{
  "withdrawals": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440002",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "withdraw",
      "amount": 50.00,
      "status": "completed",
      "transaction_type": null,
      "transaction_id": null,
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1
}
```

**Error Responses:**

- `400`: Invalid user ID
- `500`: Failed to fetch withdrawal history

### GET /api/v1/wallet/:user_id/transfers

Get the top 10 transfer transactions (both incoming and outgoing) for a user, ordered by most recent first. Includes user information for the other party in each transfer.

**Path Parameters:**

- `user_id` (UUID, required): The user ID

**Response:**

```json

{
  "transfers": [
    {
      "transaction": {
        "id": "660e8400-e29b-41d4-a716-446655440003",
        "user_id": "550e8400-e29b-41d4-a716-446655440000",
        "type": "transfer_out",
        "amount": 25.00,
        "status": "completed",
        "transaction_type": null,
        "transaction_id": null,
        "reference": "770e8400-e29b-41d4-a716-446655440000",
        "created_at": "2024-01-01T00:00:00Z"
      },
      "to": {
        "id": "770e8400-e29b-41d4-a716-446655440000",
        "telegram_id": 987654321,
        "first_name": "Jane",
        "last_name": "Doe",
        "phone_number": "9876543210",
        "referal_code": "XYZ789",
        "role": "user",
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
      }
    },
    {
      "transaction": {
        "id": "660e8400-e29b-41d4-a716-446655440004",
        "user_id": "550e8400-e29b-41d4-a716-446655440000",
        "type": "transfer_in",
        "amount": 15.00,
        "status": "completed",
        "transaction_type": null,
        "transaction_id": null,
        "reference": "880e8400-e29b-41d4-a716-446655440000",
        "created_at": "2024-01-01T00:00:00Z"
      },
      "to": {
        "id": "880e8400-e29b-41d4-a716-446655440000",
        "telegram_id": 111222333,
        "first_name": "Bob",
        "last_name": "Smith",
        "phone_number": "1112223333",
        "referal_code": "ABC456",
        "role": "user",
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:00:00Z"
      }
    }
  ],
  "count": 2
}
```

**Response Fields:**

- `transaction`: The transfer transaction details
- `to`: User information for the other party:
  - For `transfer_out`: The receiver (user who received the money)
  - For `transfer_in`: The sender (user who sent the money)

**Note:**

- `transfer_out`: Money sent to another user (the `to` field contains receiver's information)
- `transfer_in`: Money received from another user (the `to` field contains sender's information)
- The `to` field will be `null` if the user information cannot be found

**Error Responses:**

- `400`: Invalid user ID
- `500`: Failed to fetch transfer history

## Game Endpoints

The game system implements a real-time multiplayer bingo game with server-authoritative logic. All game operations are atomic and use Redis for real-time state management.

### Game Types

There are 2 fixed game types, each with a fixed bet amount:

| Game Type | Bet Amount |
| --------- | ---------- |
| REGULAR   | 10         |
| VIP       | 50         |

### Game States

Each game follows this lifecycle:

**WAITING** → **COUNTDOWN** → **DRAWING** → **FINISHED** → **CLOSED**

- **WAITING**: Game is open, users can join (requires minimum 2 players)
- **COUNTDOWN**: 60-second countdown starts when 2nd player joins, users can still join during countdown
- **DRAWING**: Numbers are being drawn, players can claim bingo
- **FINISHED**: Winner confirmed, prize distributed
- **CLOSED**: Game archived, Redis state cleared
- **CANCELLED**: Game cancelled (all players eliminated or other error), refunds issued

**State Transitions:**

- When 2nd player joins: **WAITING** → **COUNTDOWN** (60-second countdown starts)
- If players drop below 2 during **COUNTDOWN**: **COUNTDOWN** → **WAITING** (game reverts, countdown stops, remaining players stay in game)
- When countdown ends: **COUNTDOWN** → **DRAWING** (numbers start being drawn every 1 second)
- When winner claims bingo: **DRAWING** → **FINISHED** (prize distributed, new game created automatically)
- If all players eliminated: **DRAWING** → **CANCELLED** (all players refunded, new game created automatically)

### Bingo Rules

- **Card**: 5×5 grid with columns B-I-N-G-O
- **Number Ranges**:
  - B: 1–15
  - I: 16–30
  - N: 31–45 (center cell is free)
  - G: 46–60
  - O: 61–75
- **Cards**: 100 unique cards (ID 1–100), server-generated deterministically
- **Card Selection**: Multiple players can select the same card ID in the same game
- **Winning**: Any row, column, diagonal (5 numbers), or four corners
- **House Cut**: 20% of each bet goes to the house, 80% goes to the prize pool

### GET /api/v1/games

Get available games (WAITING or COUNTDOWN state).

**Query Parameters:**

- `type` (optional): Filter by game type (REGULAR or VIP)

**Response:**

```json
{
  "games": [
    {
      "id": "770e8400-e29b-41d4-a716-446655440000",
      "game_type": "REGULAR",
      "state": "WAITING",
      "bet_amount": 5.00,
      "min_players": 2,
      "player_count": 1,
      "prize_pool": 0.00,
      "house_cut": 0.2,
      "winner_id": null,
      "countdown_ends": null,
      "started_at": null,
      "finished_at": null,
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

**Example:**

```bash
# Get all available games
curl http://localhost:8080/api/v1/games

# Get available G1 games
curl http://localhost:8080/api/v1/games?type=REGULAR
```

### GET /api/v1/games/user/:user_id/history

Get game history for a user. Returns all games the user has participated in, ordered by most recent first.

**Path Parameters:**

- `user_id` (UUID, required): The user ID

**Query Parameters:**

- `limit` (optional): Number of games to return (default: 10)
- `offset` (optional): Number of games to skip (default: 0)

**Response:**

```json
{
  "games": [
    {
      "game": {
        "id": "770e8400-e29b-41d4-a716-446655440000",
        "game_type": "VIP",
        "state": "FINISHED",
        "bet_amount": 50.00,
        "min_players": 2,
        "player_count": 10,
        "prize_pool": 400.00,
        "house_cut": 0.2,
        "winner_id": "550e8400-e29b-41d4-a716-446655440000",
        "countdown_ends": null,
        "started_at": "2024-01-01T00:05:00Z",
        "finished_at": "2024-01-01T00:10:00Z",
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-01-01T00:10:00Z"
      },
      "card_id": 42,
      "is_eliminated": false,
      "joined_at": "2024-01-01T00:00:00Z",
      "left_at": null,
      "is_winner": true
    },
    {
      "game": {
        "id": "880e8400-e29b-41d4-a716-446655440000",
        "game_type": "REGULAR",
        "state": "FINISHED",
        "bet_amount": 10.00,
        "min_players": 2,
        "player_count": 5,
        "prize_pool": 40.00,
        "house_cut": 0.2,
        "winner_id": "660e8400-e29b-41d4-a716-446655440000",
        "countdown_ends": null,
        "started_at": "2024-01-01T00:15:00Z",
        "finished_at": "2024-01-01T00:20:00Z",
        "created_at": "2024-01-01T00:14:00Z",
        "updated_at": "2024-01-01T00:20:00Z"
      },
      "card_id": 15,
      "is_eliminated": true,
      "joined_at": "2024-01-01T00:14:00Z",
      "left_at": null,
      "is_winner": false
    }
  ],
  "count": 2,
  "limit": 10,
  "offset": 0
}
```

**Response Fields:**

- `game`: Full game details
- `card_id`: The card ID the user selected (1-100)
- `is_eliminated`: Whether the user was eliminated (invalid bingo claim)
- `joined_at`: When the user joined the game
- `left_at`: When the user left the game (null if still in game or game finished)
- `is_winner`: Whether the user won this game

**Error Responses:**

- `400`: Invalid user ID
- `500`: Failed to fetch game history

**Example:**

```bash
# Get game history for a user
curl http://localhost:8080/api/v1/games/user/550e8400-e29b-41d4-a716-446655440000/history

# Get game history with pagination
curl http://localhost:8080/api/v1/games/user/550e8400-e29b-41d4-a716-446655440000/history?limit=20&offset=0
```

### GET /api/v1/games/:gameId/state

Get the current game state (used for initial connection snapshot).

**Path Parameters:**

- `gameId` (UUID, required): The game ID

**Response:**

```json
{
  "game": {
    "id": "770e8400-e29b-41d4-a716-446655440000",
    "game_type": "REGULAR",
    "state": "DRAWING",
    "bet_amount": 5.00,
    "player_count": 3,
    "prize_pool": 12.00,
    "house_cut": 0.2,
    "winner_id": null,
    "countdown_ends": null,
    "started_at": "2024-01-01T00:05:00Z",
    "finished_at": null,
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:05:00Z"
  },
  "drawnNumbers": [
    {
      "letter": "B",
      "number": 7,
      "drawn_at": "2024-01-01T00:05:05Z"
    },
    {
      "letter": "I",
      "number": 22,
      "drawn_at": "2024-01-01T00:05:10Z"
    }
  ],
  "takenCards": [1, 5, 12, 33]
}
```

**Error Responses:**

- `400`: Invalid game ID
- `404`: Game not found

### POST /api/v1/games/:gameId/join

Join a game. The bet amount is **immediately deducted** from the user's wallet.

**Path Parameters:**

- `gameId` (UUID, required): The game ID

**Request Body:**

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "card_id": 1
}
```

**Response:**

```json
{
  "player": {
    "id": "880e8400-e29b-41d4-a716-446655440000",
    "game_id": "770e8400-e29b-41d4-a716-446655440000",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "card_id": 1,
    "is_eliminated": false,
    "joined_at": "2024-01-01T00:00:00Z",
    "left_at": null
  }
}
```

**Error Responses:**

- `400`: Invalid card ID, game not accepting players, user already in game, insufficient balance
- `404`: Game not found

**Rules:**

- ✅ Valid only in WAITING or COUNTDOWN state
- ✅ Card ID must be between 1 and 100
- ✅ Multiple players can select the same card (no unique constraint)
- ✅ Bet is deducted immediately
- ✅ If 2nd player joins, countdown starts automatically

### POST /api/v1/games/:gameId/leave

Leave a game. Refund is issued if game is in WAITING or COUNTDOWN state.

**Path Parameters:**

- `gameId` (UUID, required): The game ID

**Request Body:**

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Response:**

```json
{
  "message": "Successfully left the game"
}
```

**Error Responses:**

- `400`: User not in game, cannot leave during drawing phase
- `404`: Game not found

**Refund Rules:**

- ✅ **WAITING state**: Full refund
- ✅ **COUNTDOWN state**: Full refund
- ❌ **DRAWING state**: No refund (loss)

**State Reversion:**

- If players drop below 2 during **COUNTDOWN**, the game reverts to **WAITING** state
- The countdown stops and is cleared
- Remaining players stay in the game (no refund)
- When a 2nd player joins again, the countdown will restart automatically
- This allows games to continue naturally without cancellation
- Prize pool decreases when players leave (their bet is refunded)

### GET /api/v1/cards/:cardId

Get bingo card data for a specific card ID.

**Path Parameters:**

- `cardId` (int, required): The card ID (1-100)

**Response:**

```json
{
  "card": {
    "id": 1,
    "numbers": [
      [15, 5, 3, 1, 12],
      [28, 30, 16, 20, 18],
      [31, 45, 0, 33, 43],
      [58, 46, 48, 50, 60],
      [61, 73, 65, 75, 63]
    ]
  }
}
```

**Error Responses:**

- `400`: Invalid card ID (must be between 1 and 100)
- `500`: Server error

**Note:** The center cell (row 3, column 3) is always 0 (free space).

### POST /api/v1/games/:gameId/bingo

Claim bingo. The backend validates the claim against drawn numbers.

**Path Parameters:**

- `gameId` (UUID, required): The game ID

**Request Body:**

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "marked_numbers": [0, 1, 2, 3, 4]
}
```

**Note:** `marked_numbers` is an array of card positions (0-24), not the actual numbers. Positions are numbered left-to-right, top-to-bottom:

- Row 1: positions 0-4
- Row 2: positions 5-9
- Row 3: positions 10-14 (position 12 is the center free space)
- Row 4: positions 15-19
- Row 5: positions 20-24

**Response (Valid Bingo - Winner):**

```json
{
  "winner": true,
  "message": "Congratulations! You won!"
}
```

**Response (Invalid Bingo - Eliminated):**

```json
{
  "winner": false,
  "message": "Invalid bingo claim. You have been eliminated."
}
```

**Error Responses:**

- `400`: Game not in drawing phase, user not in game, player already eliminated
- `404`: Game not found

**Validation:**

- ✅ Server validates card against drawn numbers
- ✅ Valid bingo: Any row, column, diagonal (5 numbers), or four corners
- ✅ Invalid claim: Player is eliminated
- ✅ Winner receives: `(bet × number_of_players) × (1 - house_cut)` where house_cut is 20% (0.2)
- ✅ All balance changes are atomic

### WebSocket: ws:///api/v1/ws/game

> **📖 For detailed WebSocket documentation, see [WEBSOCKET_API.md](./WEBSOCKET_API.md)**

Connect to real-time game updates via WebSocket. **No authentication required** - WebSocket is public/read-only for viewing.

**Connection Options:**

**Option 1: Connect by Game Type (Recommended)**

```javascript
// Automatically finds or creates an available game of the specified type
const ws = new WebSocket('ws://localhost:8080/api/v1/ws/game?type=VIP');
```

**Option 2: Connect by Game ID**

```javascript
const ws = new WebSocket('ws://localhost:8080/api/v1/ws/game/{gameId}');
```

**Query Parameters (for Option 1):**

- `type` (string, required): Game type (REGULAR or VIP)

**Path Parameters (for Option 2):**

- `gameId` (UUID, required): The game ID

**WebSocket Events:**

All events follow this format:

```json
{
  "event": "EVENT_NAME",
  "data": {}
}
```

**Event Types:**

1. **INITIAL_STATE** - Sent on connection

```json
{
  "event": "INITIAL_STATE",
  "data": {
    "game": {...},
    "drawnNumbers": [...],
    "takenCards": [...],
    "playerCount": 3,
    "secondsLeft": 42
  }
}
```

2. **GAME_STATUS** - Game state changes

```json
{
  "event": "GAME_STATUS",
  "data": {
    "status": "COUNTDOWN",
    "secondsLeft": 42
  }
}
```

3. **COUNTDOWN** - Countdown updates (every second)

```json
{
  "event": "COUNTDOWN",
  "data": {
    "secondsLeft": 18
  }
}
```

4. **NUMBER_DRAWN** - New number drawn

```json
{
  "event": "NUMBER_DRAWN",
  "data": {
    "letter": "B",
    "number": 12
  }
}
```

5. **PLAYER_JOINED** - Player joined the game

```json
{
  "event": "PLAYER_JOINED",
  "data": {
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "card_id": 5
  }
}
```

6. **PLAYER_LEFT** - Player left the game

```json
{
  "event": "PLAYER_LEFT",
  "data": {
    "user_id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

7. **PLAYER_ELIMINATED** - Player eliminated (invalid bingo claim)

```json
{
  "event": "PLAYER_ELIMINATED",
  "data": {
    "userId": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

8. **WINNER** - Game finished, winner announced

```json
{
  "event": "WINNER",
  "data": {
    "userId": "550e8400-e29b-41d4-a716-446655440000",
    "prize": 960.00
  }
}
```

9. **NEW_GAME_AVAILABLE** - New game created after current game finishes

```json
{
  "event": "NEW_GAME_AVAILABLE",
  "data": {
    "gameId": "880e8400-e29b-41d4-a716-446655440000",
    "gameType": "VIP"
  }
}
```

**Note:** When a game finishes (WINNER) or is cancelled, a new game of the same type is automatically created and this event is published.

**Error Responses:**

- `400`: Invalid game ID or game type
- `500`: Server error

**Note:**

- WebSocket connection is **public and read-only** - no authentication required
- All game logic is server-authoritative
- Events are published via Redis pub/sub for scalability
- Anyone can connect to watch games in real-time

## Admin Endpoints

**All admin endpoints require JWT authentication and admin role.**

Include the JWT token in the Authorization header:

```
Authorization: Bearer <your_jwt_token>
```

**Security:**

- JWT tokens expire after 24 hours (configurable via `JWT_EXPIRATION_HOURS`)
- Only users with `role='admin'` can access these endpoints
- Invalid or expired tokens will return `401 Unauthorized`
- Non-admin users will receive `403 Forbidden`

## Admin User Management Endpoints

### GET /api/v1/admin/users

Get all users with their wallets and pagination.

**Query Parameters:**

- `limit` (optional): Number of users to return (default: 50)
- `offset` (optional): Number of users to skip (default: 0)

**Response:**

```json
{
  "users": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "telegram_id": 123456789,
      "first_name": "John",
      "last_name": "Doe",
      "phone_number": "1234567890",
      "referal_code": "ABC123XY",
      "role": "user",
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z",
      "wallet": {
        "user_id": "550e8400-e29b-41d4-a716-446655440000",
        "balance": 75.00,
        "demo_balance": 0.00,
        "updated_at": "2024-01-01T00:00:00Z"
      }
    }
  ],
  "count": 1250,
  "limit": 50,
  "offset": 0
}
```

**Response Fields:**

- `users`: Array of user objects, each containing:
  - All user fields (id, telegram_id, first_name, last_name, phone_number, referal_code, role, created_at, updated_at)
  - `wallet`: Wallet information (may be null if wallet doesn't exist)
- `count`: Total number of users in the system (not just the current page)
- `limit`: Number of users returned in this response
- `offset`: Number of users skipped

**Note:** Passwords are never included in the response for security.

**Error Responses:**

- `401`: Unauthorized (missing or invalid token)
- `403`: Forbidden (user is not an admin)
- `500`: Failed to fetch users

## Admin Transaction Query Endpoints

All query endpoints support pagination via query parameters:

- `limit` (default: 50): Number of transactions to return
- `offset` (default: 0): Number of transactions to skip

### GET /api/v1/admin/transactions

Get all transactions with pagination.

**Query Parameters:**

- `limit` (optional): Number of transactions to return (default: 50)
- `offset` (optional): Number of transactions to skip (default: 0)

**Response:**

```json
{
  "transactions": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "deposit",
      "amount": 100.00,
      "status": "pending",
      "transaction_type": "Telebirr",
      "transaction_id": "tx_123456789",
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### GET /api/v1/admin/transactions/pending/deposits

Get all pending deposit transactions.

**Query Parameters:**

- `limit` (optional): Number of transactions to return (default: 50)
- `offset` (optional): Number of transactions to skip (default: 0)

**Response:**

```json
{
  "transactions": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "deposit",
      "amount": 100.00,
      "status": "pending",
      "transaction_type": "Telebirr",
      "transaction_id": "tx_123456789",
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### GET /api/v1/admin/transactions/pending/withdrawals

Get all pending withdrawal transactions.

**Query Parameters:**

- `limit` (optional): Number of transactions to return (default: 50)
- `offset` (optional): Number of transactions to skip (default: 0)

**Response:**

```json
{
  "transactions": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440002",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "withdraw",
      "amount": 50.00,
      "status": "pending",
      "transaction_type": null,
      "transaction_id": null,
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### GET /api/v1/admin/transactions/completed/deposits

Get all completed (approved) deposit transactions.

**Query Parameters:**

- `limit` (optional): Number of transactions to return (default: 50)
- `offset` (optional): Number of transactions to skip (default: 0)

**Response:**

```json
{
  "transactions": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "deposit",
      "amount": 100.00,
      "status": "completed",
      "transaction_type": "Telebirr",
      "transaction_id": "tx_123456789",
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### GET /api/v1/admin/transactions/completed/withdrawals

Get all completed (approved) withdrawal transactions.

**Query Parameters:**

- `limit` (optional): Number of transactions to return (default: 50)
- `offset` (optional): Number of transactions to skip (default: 0)

**Response:**

```json
{
  "transactions": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440002",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "withdraw",
      "amount": 50.00,
      "status": "completed",
      "transaction_type": null,
      "transaction_id": null,
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### GET /api/v1/admin/transactions/failed

Get all failed transactions (any type).

**Query Parameters:**

- `limit` (optional): Number of transactions to return (default: 50)
- `offset` (optional): Number of transactions to skip (default: 0)

**Response:**

```json
{
  "transactions": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "deposit",
      "amount": 100.00,
      "status": "failed",
      "transaction_type": "Telebirr",
      "transaction_id": "tx_123456789",
      "reference": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

### GET /api/v1/admin/transactions/transfers

Get all transfer transactions (both `transfer_in` and `transfer_out`).

**Query Parameters:**

- `limit` (optional): Number of transactions to return (default: 50)
- `offset` (optional): Number of transactions to skip (default: 0)

**Response:**

```json
{
  "transactions": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440003",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "type": "transfer_out",
      "amount": 25.00,
      "status": "completed",
      "transaction_type": null,
      "transaction_id": null,
      "reference": "770e8400-e29b-41d4-a716-446655440000",
      "created_at": "2024-01-01T00:00:00Z"
    },
    {
      "id": "660e8400-e29b-41d4-a716-446655440004",
      "user_id": "770e8400-e29b-41d4-a716-446655440000",
      "type": "transfer_in",
      "amount": 25.00,
      "status": "completed",
      "transaction_type": null,
      "transaction_id": null,
      "reference": "550e8400-e29b-41d4-a716-446655440000",
      "created_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 2,
  "limit": 50,
  "offset": 0
}
```

## Admin Dashboard Endpoints

### GET /api/v1/admin/stats/dashboard

Get dashboard statistics for the admin panel. Returns aggregated statistics about users, transactions, games, and balances.

**Authentication:** Required (Admin only)

**Response:**

```json
{
  "pending_deposits": 5,
  "pending_withdrawals": 3,
  "total_users": 1250,
  "total_transactions": 5000,
  "total_balance": 125000.50,
  "games_by_type": {
    "REGULAR": 250,
    "VIP": 100
  },
  "total_house_cut": 15000.75
}
```

**Response Fields:**

- `pending_deposits`: Number of pending deposit transactions
- `pending_withdrawals`: Number of pending withdrawal transactions
- `total_users`: Total number of users in the system
- `total_transactions`: Total number of transactions (all types and statuses)
- `total_balance`: Sum of all wallet balances across all users
- `games_by_type`: Count of games grouped by game type (REGULAR or VIP)
- `total_house_cut`: Total house cut collected from finished games (calculated from prize pools)

**Error Responses:**

- `401`: Unauthorized (missing or invalid token)
- `403`: Forbidden (user is not an admin)
- `500`: Failed to fetch dashboard stats

**Example:**

```bash
curl -X GET http://localhost:8080/api/v1/admin/stats/dashboard \
  -H "Authorization: Bearer <admin_token>"
```

## Admin Transaction Action Endpoints

### POST /api/v1/admin/transactions/:id/approve-deposit

Approve a pending deposit transaction. This will update the transaction status to `completed` and add the amount to the user's wallet balance.

**Path Parameters:**

- `id` (UUID, required): The transaction ID

**Response:**

```json
{
  "message": "Deposit approved successfully",
  "transaction": {
    "id": "660e8400-e29b-41d4-a716-446655440001",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "deposit",
    "amount": 100.00,
    "status": "completed",
    "transaction_type": "Telebirr",
    "transaction_id": "tx_123456789",
    "reference": null,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**

- `400`: Transaction is not a deposit or not pending
- `404`: Transaction not found

**Operation Flow:**

1. Lock wallet row (FOR UPDATE)
2. Update transaction status → `completed`
3. Add amount to wallet balance
4. Commit transaction

### POST /api/v1/admin/transactions/:id/reject-deposit

Reject a pending deposit transaction. The transaction status is updated to `failed` and **no balance change** occurs.

**Path Parameters:**

- `id` (UUID, required): The transaction ID

**Response:**

```json
{
  "message": "Deposit rejected successfully",
  "transaction": {
    "id": "660e8400-e29b-41d4-a716-446655440001",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "deposit",
    "amount": 100.00,
    "status": "failed",
    "transaction_type": "Telebirr",
    "transaction_id": "tx_123456789",
    "reference": null,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**

- `400`: Transaction is not a deposit or not pending
- `404`: Transaction not found

### POST /api/v1/admin/transactions/:id/approve-withdrawal

Approve a pending withdrawal transaction. The transaction status is updated to `completed`. The balance was already subtracted when the withdrawal was created.

**Path Parameters:**

- `id` (UUID, required): The transaction ID

**Response:**

```json
{
  "message": "Withdrawal approved successfully",
  "transaction": {
    "id": "660e8400-e29b-41d4-a716-446655440002",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "withdraw",
    "amount": 50.00,
    "status": "completed",
    "transaction_type": null,
    "transaction_id": null,
    "reference": null,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**

- `400`: Transaction is not a withdrawal or not pending
- `404`: Transaction not found

### POST /api/v1/admin/transactions/:id/reject-withdrawal

Reject a pending withdrawal transaction. The transaction status is updated to `failed` and the balance is **refunded** (added back to the wallet).

**Path Parameters:**

- `id` (UUID, required): The transaction ID

**Response:**

```json
{
  "message": "Withdrawal rejected and balance refunded",
  "transaction": {
    "id": "660e8400-e29b-41d4-a716-446655440002",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "withdraw",
    "amount": 50.00,
    "status": "failed",
    "transaction_type": null,
    "transaction_id": null,
    "reference": null,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**

- `400`: Transaction is not a withdrawal or not pending
- `404`: Transaction not found

**Operation Flow:**

1. Lock wallet row (FOR UPDATE)
2. Refund balance (add amount back)
3. Update transaction status → `failed`
4. Commit transaction

### POST /api/v1/admin/transactions/:id/cancel

Cancel any pending transaction. For deposits, no balance change occurs. For withdrawals, the balance is refunded.

**Path Parameters:**

- `id` (UUID, required): The transaction ID

**Response:**

```json
{
  "message": "Transaction cancelled successfully",
  "transaction": {
    "id": "660e8400-e29b-41d4-a716-446655440001",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "type": "deposit",
    "amount": 100.00,
    "status": "cancelled",
    "transaction_type": "Telebirr",
    "transaction_id": "tx_123456789",
    "reference": null,
    "created_at": "2024-01-01T00:00:00Z"
  }
}
```

**Error Responses:**

- `400`: Transaction is not pending
- `404`: Transaction not found

**Behavior:**

- **Deposit**: Status → `cancelled`, no balance change
- **Withdrawal**: Status → `cancelled`, balance refunded

## Database Schema

The application uses the following main tables:

- **users**: User information (UUID primary key, telegram_id, phone_number, referal_code, role, password)
- **wallets**: User wallets (user_id foreign key, balance, demo_balance)
- **transactions**: Transaction ledger (deposit, withdraw, transfer_in, transfer_out)
- **games**: Game instances (UUID primary key, game_type, state, bet_amount, player_count, prize_pool, winner_id)
- **game_players**: Players in games (game_id, user_id, card_id, is_eliminated)
- **drawn_numbers**: Drawn numbers history (game_id, letter, number, drawn_at)

**User Roles:**

- `user`: Regular bot users (default)
- `admin`: Admin users with access to admin endpoints

All wallet operations use database transactions with row-level locking (`FOR UPDATE`) to ensure data consistency and prevent race conditions.

## Creating Admin Users

There are two ways to create an admin user:

### Option A: Convert an Existing User to Admin

1. **Hash a password** using the helper script:
   ```bash
   go run scripts/create_admin.go your_password_here
   ```
   This will output a hashed password.

2. **Update the user** in the database:
   ```bash
   psql "$DATABASE_URL" -c "UPDATE users SET role = 'admin', password = 'YOUR_HASHED_PASSWORD_HERE' WHERE telegram_id = YOUR_TELEGRAM_ID;"
   ```
   Replace:
   - `YOUR_HASHED_PASSWORD_HERE` with the hash from step 1
   - `YOUR_TELEGRAM_ID` with the user's Telegram ID

### Option B: Create a New User and Make Them Admin

1. **Register a user** via API:
   ```bash
   curl -X POST http://localhost:8080/api/v1/user/register \
     -H "Content-Type: application/json" \
     -d '{
       "telegram_id": 123456789,
       "first_name": "Admin",
       "last_name": "User",
       "phone": "+1234567890"
     }'
   ```

2. **Hash a password** using the helper script:
   ```bash
   go run scripts/create_admin.go your_password_here
   ```

3. **Update the user to admin** (same as Option A, step 2)

### Quick Example

```bash
# 1. Hash password
go run scripts/create_admin.go mySecurePassword123
# Output: $2a$10$l2ns2tObNnMy.whUTzi21e7u1xuJH0nEFitkI/eqUSO0Bmul/bEji

# 2. Update user (replace 123456789 with actual telegram_id)
psql "$DATABASE_URL" -c "UPDATE users SET role = 'admin', password = '\$2a\$10\$l2ns2tObNnMy.whUTzi21e7u1xuJH0nEFitkI/eqUSO0Bmul/bEji' WHERE telegram_id = 123456789;"
```

**Note:** When using `psql`, escape the `$` signs in the password hash by using `\$`.

### Verify Admin Account

After creating the admin, you can log in via:
```bash
POST /api/v1/auth/login
{
  "telegram_id": 123456789,
  "password": "your_password_here"
}
```

## Performance Optimizations

- Connection pooling for PostgreSQL (max 100 connections)
- Indexed database queries
- Efficient phone number normalization
- Unique referral code generation with collision handling
- Row-level locking for concurrent wallet operations
- Atomic transactions for transfers and game operations
- Redis caching for game state and real-time updates
- WebSocket pub/sub for scalable real-time communication
- Graceful server shutdown
- Request timeouts configured

## Game System Architecture

The game system is **server-authoritative**:

- ✅ All game logic runs on the server
- ✅ All money handling is atomic and transactional
- ✅ All winner verification is server-side
- ✅ Clients never decide outcomes
- ✅ Cards are generated server-side (100 unique cards)
- ✅ Number drawing uses secure RNG
- ✅ Bingo claims are validated server-side
- ✅ Real-time updates via WebSocket (no polling)

**State Management:**

- **Redis**: Real-time game state, countdown timers, drawn numbers, pub/sub events
- **PostgreSQL**: Game records, player records, transaction ledger (source of truth)

**Security:**

- No client-side win validation
- No exposure of other players' cards
- Bingo claims are locked to prevent race conditions
- All money operations are transactional

## Development

### Running Tests

```bash
go test ./...
```

### Building

```bash
make build
# Or manually:
# go build -o bin/server cmd/server/main.go
```

### Using Docker

Start API, PostgreSQL, and Redis:

```bash
make docker-up
# Or:
# docker compose up --build -d
```

The database will be automatically initialized with the schema on first start.

Stop services:

```bash
make docker-down
```

## License

MIT

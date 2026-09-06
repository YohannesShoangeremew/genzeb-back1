# Database Access Guide

## Option 1: Using psql Command Line (Recommended)

### Connect via Docker exec
```bash
docker exec -it bingo-postgres psql -U postgres -d bingo
```

### Connect from host machine (if psql is installed)
```bash
psql -h localhost -p 5432 -U postgres -d bingo
# Password: postgres
```

## Option 2: Using Docker exec with SQL commands

### List all databases
```bash
docker exec -it bingo-postgres psql -U postgres -c "\l"
```

### List all tables
```bash
docker exec -it bingo-postgres psql -U postgres -d bingo -c "\dt"
```

### View users table
```bash
docker exec -it bingo-postgres psql -U postgres -d bingo -c "SELECT * FROM users;"
```

### View wallets table
```bash
docker exec -it bingo-postgres psql -U postgres -d bingo -c "SELECT * FROM wallets;"
```

### View transactions table
```bash
docker exec -it bingo-postgres psql -U postgres -d bingo -c "SELECT * FROM transactions;"
```

## Option 3: Using GUI Tools

### pgAdmin (Web-based)
1. Install pgAdmin: https://www.pgadmin.org/download/
2. Add new server:
   - Host: `localhost`
   - Port: `5432`
   - Database: `bingo`
   - Username: `postgres`
   - Password: `postgres`

### DBeaver (Cross-platform)
1. Download: https://dbeaver.io/download/
2. Create new PostgreSQL connection:
   - Host: `localhost`
   - Port: `5432`
   - Database: `bingo`
   - Username: `postgres`
   - Password: `postgres`

### TablePlus (macOS/Windows/Linux)
1. Download: https://tableplus.com/
2. Create new PostgreSQL connection with same credentials

## Quick Database Queries

### Count users
```bash
docker exec -it bingo-postgres psql -U postgres -d bingo -c "SELECT COUNT(*) FROM users;"
```

### View user with wallet
```bash
docker exec -it bingo-postgres psql -U postgres -d bingo -c "
SELECT 
    u.id, 
    u.telegram_id, 
    u.first_name, 
    u.phone_number,
    w.balance, 
    w.demo_balance 
FROM users u 
LEFT JOIN wallets w ON u.id = w.user_id;"
```

### View recent transactions
```bash
docker exec -it bingo-postgres psql -U postgres -d bingo -c "
SELECT 
    id, 
    user_id, 
    type, 
    amount, 
    status, 
    created_at 
FROM transactions 
ORDER BY created_at DESC 
LIMIT 10;"
```

## Useful psql Commands (when inside psql)

```
\l          - List all databases
\dt         - List all tables
\d table    - Describe table structure
\q          - Quit psql
\?          - Show help
```


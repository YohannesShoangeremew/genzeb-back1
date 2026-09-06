#!/bin/bash
# Database access helper script
# This script sets the correct Docker socket and connects to the database

export DOCKER_HOST=unix:///run/docker.sock

if [ "$1" = "shell" ]; then
    # Interactive shell
    docker exec -it bingo-postgres psql -U postgres -d bingo
elif [ "$1" = "query" ]; then
    # Run a query
    docker exec bingo-postgres psql -U postgres -d bingo -c "$2"
elif [ "$1" = "tables" ]; then
    # List tables
    docker exec bingo-postgres psql -U postgres -d bingo -c "\dt"
elif [ "$1" = "users" ]; then
    # View users
    docker exec bingo-postgres psql -U postgres -d bingo -c "SELECT * FROM users;"
elif [ "$1" = "wallets" ]; then
    # View wallets
    docker exec bingo-postgres psql -U postgres -d bingo -c "SELECT * FROM wallets;"
elif [ "$1" = "transactions" ]; then
    # View transactions
    docker exec bingo-postgres psql -U postgres -d bingo -c "SELECT * FROM transactions ORDER BY created_at DESC LIMIT 20;"
elif [ "$1" = "stats" ]; then
    # View statistics
    docker exec bingo-postgres psql -U postgres -d bingo -c "
    SELECT 
        (SELECT COUNT(*) FROM users) as users,
        (SELECT COUNT(*) FROM wallets) as wallets,
        (SELECT COUNT(*) FROM transactions) as transactions,
        (SELECT SUM(balance) FROM wallets) as total_balance;
    "
else
    echo "Usage: ./db.sh [command]"
    echo ""
    echo "Commands:"
    echo "  shell        - Open interactive psql shell"
    echo "  tables       - List all tables"
    echo "  users        - View all users"
    echo "  wallets      - View all wallets"
    echo "  transactions - View recent transactions"
    echo "  stats        - View database statistics"
    echo "  query \"SQL\"  - Run a custom SQL query"
    echo ""
    echo "Examples:"
    echo "  ./db.sh shell"
    echo "  ./db.sh users"
    echo "  ./db.sh query \"SELECT * FROM users WHERE telegram_id = 123456789;\""
fi


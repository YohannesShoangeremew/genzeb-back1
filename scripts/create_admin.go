package main

import (
	"fmt"
	"os"

	"github.com/bingo/backend/pkg/auth"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run scripts/create_admin.go <password>")
		os.Exit(1)
	}

	password := os.Args[1]
	hashed, err := auth.HashPassword(password)
	if err != nil {
		fmt.Printf("Error hashing password: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Hashed password: %s\n", hashed)
	fmt.Println("\nUse this in SQL to update a user to admin:")
	fmt.Printf("UPDATE users SET role = 'admin', password = '%s' WHERE telegram_id = YOUR_TELEGRAM_ID;\n", hashed)
}


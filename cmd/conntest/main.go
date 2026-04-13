package main

import (
"database/sql"
"fmt"
"log"
"os"

_ "github.com/tidbcloud/lake-go"
)

func main() {
dsn := os.Getenv("LAKE_DSN")
if dsn == "" {
log.Fatal("LAKE_DSN env is not set")
}

fmt.Println("=== Connecting to Lake warehouse ===")
db, err := sql.Open("lake", dsn)
if err != nil {
log.Fatalf("sql.Open failed: %v", err)
}
defer db.Close()

// 1. Ping
fmt.Print("1. Ping... ")
if err := db.Ping(); err != nil {
log.Fatalf("FAIL: %v", err)
}
fmt.Println("OK")

// 2. SELECT 1
fmt.Print("2. SELECT 1... ")
var one int
if err := db.QueryRow("SELECT 1").Scan(&one); err != nil {
log.Fatalf("FAIL: %v", err)
}
fmt.Printf("OK (got %d)\n", one)

// 3. Show databases
fmt.Println("3. SHOW DATABASES:")
rows, err := db.Query("SHOW DATABASES")
if err != nil {
log.Fatalf("FAIL: %v", err)
}
defer rows.Close()
for rows.Next() {
var name string
rows.Scan(&name)
fmt.Printf("   - %s\n", name)
}

// 4. Query information_schema.tables
fmt.Println("4. Tables in information_schema (limit 10):")
rows2, err := db.Query("SELECT table_name FROM information_schema.tables LIMIT 10")
if err != nil {
log.Fatalf("FAIL: %v", err)
}
defer rows2.Close()
for rows2.Next() {
var tbl string
rows2.Scan(&tbl)
fmt.Printf("   - %s\n", tbl)
}

// 5. Version
fmt.Print("5. Server version: ")
var ver string
if err := db.QueryRow("SELECT version()").Scan(&ver); err != nil {
log.Fatalf("FAIL: %v", err)
}
fmt.Println(ver)

fmt.Println("\n=== All connection tests PASSED ===")
}

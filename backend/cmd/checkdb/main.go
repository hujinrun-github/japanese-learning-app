package main

import (
    "context"
    "fmt"
    "os"

    "japanese-learning-app/internal/config"
    pgdata "japanese-learning-app/internal/data/postgres"
    "japanese-learning-app/internal/store"
)

func main() {
    cfg, err := config.Load("")
    if err != nil { fmt.Println("config err:", err); os.Exit(1) }

    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" { dsn = cfg.DatabaseURL }
    if dsn == "" {
        fmt.Println("No DATABASE_URL set")
        os.Exit(1)
    }
    fmt.Printf("DATABASE_URL: %s...\n", dsn[:min(30, len(dsn))])

    adapter := pgdata.Adapter{}
    db, err := adapter.Open(context.Background(), store.DatabaseConfig{
        DatabaseURL: dsn,
        AppTimezone: cfg.AppTimezone,
    })
    if err != nil { fmt.Println("open err:", err); os.Exit(1) }
    defer db.Close()

    var cnt int
    if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM speaking_materials").Scan(&cnt); err != nil {
        fmt.Println("PG query err:", err)
    } else {
        fmt.Printf("PostgreSQL speaking_materials count: %d\n", cnt)
    }

    rows, err := db.QueryContext(context.Background(), "SELECT id, type, title, jlpt_level FROM speaking_materials LIMIT 5")
    if err != nil { fmt.Println("PG query2 err:", err); return }
    defer rows.Close()
    for rows.Next() {
        var id int; var typ, title, level string
        rows.Scan(&id, &typ, &title, &level)
        fmt.Printf("  id=%d type=%s title=%s level=%s\n", id, typ, title, level)
    }

    var wordCnt int
    if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM words").Scan(&wordCnt); err != nil {
        fmt.Println("PG words query err:", err)
    } else {
        fmt.Printf("PostgreSQL words count: %d\n", wordCnt)
    }
}

package main

import (
	"context"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/younes-alkashmi/todo/internal/api"
	fb "github.com/younes-alkashmi/todo/internal/firebase"
)

func main() {
	_ = godotenv.Load()
	ctx := context.Background()
	saPath := os.Getenv("FIREBASE_SA")
	projectID := os.Getenv("FIREBASE_PROJECT")
	if saPath == "" || projectID == "" {
		log.Fatal("set FIREBASE_SA and FIREBASE_PROJECT")
	}

	client, err := fb.New(ctx, saPath, projectID)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	r := api.NewRouter(client)
	log.Println("listening on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}

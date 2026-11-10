package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/joho/godotenv"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/younes-alkashmi/todo/internal/service"
	"github.com/younes-alkashmi/todo/internal/store"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	_ = godotenv.Load()
	// DB lives at ~/.local/share/todo/todo.db on Linux (XDG spec).
	dbPath := filepath.Join(xdg.DataHome, "todo", "todo.db")

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open db at %s: %v", dbPath, err)
	}
	defer st.Close()

	tasks := service.NewTaskService(st, "local")
	projects := service.NewProjectService(st, "local")
	appSvc := NewApp(tasks, projects, st, os.Getenv("FIREBASE_API_KEY"), "http://localhost:8080")
	app := application.New(application.Options{
		Name:        "todo",
		Description: "Offline-first todo app",
		Services: []application.Service{
			application.NewService(appSvc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Todo",
		Width:  1100,
		Height: 700,
		URL:    "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

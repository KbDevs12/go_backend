package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"backend/internal/config"
	"backend/internal/database"
	fbpkg "backend/pkg/firebase"

	"golang.ngrok.com/ngrok/v2"
)

func main() {
	config.Load()
	database.Connect()
	defer database.Close()
	fbpkg.Init()

	router := NewRouter()

	switch config.App.AppEnv {
	case "development", "dev":
		runDev(router)
	default:
		runProd(router)
	}
}

// runDev starts the server through an ngrok tunnel.
// The public URL is printed to stdout so Flutter (or Postman) can connect
// without any manual port-forwarding or local IP configuration.
//
// Required env var: NGROK_AUTHTOKEN
func runDev(handler http.Handler) {
	token := os.Getenv("NGROK_AUTHTOKEN")
	if token == "" {
		log.Fatal("[dev] NGROK_AUTHTOKEN not set in .env — cannot start ngrok tunnel")
	}
	log.Printf("[dev] NGROK_AUTHTOKEN found: %s...", token[:8])

	listener, err := ngrok.Listen(
		context.Background())
	if err != nil {
		log.Fatalf("[dev] failed to open ngrok tunnel: %v", err)
	}

	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Printf("║  🌐  Public URL : %-31s║\n", listener.URL())
	fmt.Println("║  MODE          : development (ngrok)              ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("  Paste the URL above into your Flutter app's base URL config.")
	fmt.Println()

	if err := http.Serve(listener, handler); err != nil {
		log.Fatalf("[dev] server error: %v", err)
	}
}

func runProd(handler http.Handler) {
	addr := fmt.Sprintf(":%s", config.App.ServerPort)

	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Printf("║  🚀  Listening on : %-30s║\n", addr)
	fmt.Println("║  MODE             : production                    ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("[prod] server error: %v", err)
	}
}

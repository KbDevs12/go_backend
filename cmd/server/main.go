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
	go func() {
		fmt.Println("Starting server at http://localhost:8080")

		if err := http.ListenAndServe(":8080", handler); err != nil {
			log.Fatalf("[dev] server error: %v", err)
		}
	}()

	if err := Tunneling(context.Background()); err != nil {
		log.Fatalf("[dev] tunnel error: %v", err)
	}
}

const address = "http://localhost:8080"

func Tunneling(ctx context.Context) error {
	token := os.Getenv("NGROK_AUTHTOKEN")
	if token == "" {
		return fmt.Errorf("NGROK_AUTHTOKEN is not set")
	}
	agent, err := ngrok.NewAgent(ngrok.WithAuthtoken(token))
	if err != nil {
		return fmt.Errorf("failed to create ngrok agent: %w", err)
	}

	ln, err := agent.Forward(ctx,
		ngrok.WithUpstream(address),
		ngrok.WithURL(os.Getenv("NGROK_RESERVED_DOMAIN")),
	)
	if err != nil {
		return fmt.Errorf("failed to start ngrok tunnel: %w", err)
	}

	fmt.Println("Endpoint online: forwarding from", ln.URL(), "to", address)

	<-ln.Done()
	return nil
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

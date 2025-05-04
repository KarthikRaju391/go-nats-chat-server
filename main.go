package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/karthikraju391/go-nats-chat-server/config"
	"github.com/karthikraju391/go-nats-chat-server/handlers"
	"github.com/karthikraju391/go-nats-chat-server/nats_service"
)

func main() {
	// --- Initialize NATS Service ---
	natsSvc, err := nats_service.NewNatsService()
	if err != nil {
		log.Fatalf("Failed to initialize NATS Service: %v", err)
	}
	defer natsSvc.Close()
	log.Println("NATS Service Initialized")

	// --- Initialize Fiber App ---
	app := fiber.New()
	app.Use(logger.New()) // Basic request logging

	// --- Setup WebSocket Route ---
	app.Use("/ws", func(c *fiber.Ctx) error {
		// Check if the request is a WebSocket upgrade request
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	// WebSocket endpoint: /ws/:conversationID
	app.Get("/chat/:conversationID", websocket.New(func(c *websocket.Conn) {
		// Pass the NATS service instance to the handler
		handlers.HandleWebSocket(c, natsSvc)
	}))

	// --- Start Server ---
	go func() {
		log.Printf("Starting server on %s", config.ServerAddr)
		if err := app.Listen(config.ServerAddr); err != nil {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// --- Graceful Shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit // Block until signal received

	log.Println("Shutting down server...")

	// Shutdown Fiber app
	if err := app.Shutdown(); err != nil {
		log.Printf("Error shutting down Fiber: %v", err)
	}

	// NATS connection is closed by defer in main

	log.Println("Server gracefully stopped")
}

package handlers

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/karthikraju391/go-nats-chat-server/config"
	"github.com/karthikraju391/go-nats-chat-server/models"
	"github.com/karthikraju391/go-nats-chat-server/nats_service"
)

type Client struct {
	Conn           *websocket.Conn
	NatsService    *nats_service.NatsService
	ConversationID string
	UserID         string               // Should come from authentication
	UserAlias      string               // Should come from authentication
	MessageChan    chan *models.Message // Channel for messages from NATS subscription
	DoneChan       chan struct{}        // Channel to signal closure
}

func NewClient(conn *websocket.Conn, natsSvc *nats_service.NatsService, convoID, userID, userAlias string) *Client {
	return &Client{
		Conn:           conn,
		NatsService:    natsSvc,
		ConversationID: convoID,
		UserID:         userID,
		UserAlias:      userAlias,
		MessageChan:    make(chan *models.Message, 256), // Buffered channel
		DoneChan:       make(chan struct{}),
	}
}

// HandleRead reads messages from the WebSocket connection and sends them to NATS.
func (c *Client) HandleRead(ctx context.Context) {
	defer func() {
		log.Printf("Reader closed for %s in %s", c.UserID, c.ConversationID)
		close(c.DoneChan) // Signal writer to stop
		// NATS subscription cleanup happens in HandleWebSocket
	}()
	c.Conn.SetReadLimit(config.MaxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(config.PongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(config.PongWait))
		return nil
	})

	for {
		// Read message from WebSocket client
		var clientMsg struct { // Expecting simple text messages from client
			Text string `json:"text"`
		}
		err := c.Conn.ReadJSON(&clientMsg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error for %s: %v", c.UserID, err)
			} else {
				log.Printf("WebSocket closed for %s: %v", c.UserID, err)
			}
			break // Exit loop on error or close
		}

		if clientMsg.Text == "" {
			continue // Ignore empty messages
		}

		// Create a message object
		msg := &models.Message{
			ID:             uuid.NewString(), // Generate unique ID
			ConversationID: c.ConversationID,
			SenderID:       c.UserID,
			SenderAlias:    c.UserAlias,
			Text:           clientMsg.Text,
			CreatedAt:      time.Now().UTC(),
		}

		// Publish the message to NATS
		pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := c.NatsService.PublishMessage(pubCtx, msg); err != nil {
			log.Printf("Failed to publish message from %s: %v", c.UserID, err)
			// Optionally notify the client of the failure
		}
		cancel()
	}
}

// HandleWrite writes messages from the NATS subscription (via MessageChan) to the WebSocket connection.
func (c *Client) HandleWrite() {
	ticker := time.NewTicker(config.PingPeriod)
	defer func() {
		ticker.Stop()
		log.Printf("Writer closed for %s in %s", c.UserID, c.ConversationID)
		// Connection closing is handled in HandleWebSocket's defer
	}()

	for {
		select {
		case message, ok := <-c.MessageChan:
			c.Conn.SetWriteDeadline(time.Now().Add(config.WriteWait))
			if !ok {
				// The message channel was closed.
				log.Printf("Message channel closed for %s", c.UserID)
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Write message received from NATS to WebSocket client
			if err := c.Conn.WriteJSON(message); err != nil {
				log.Printf("WebSocket write error for %s: %v", c.UserID, err)
				return // Exit loop on write error
			}

		case <-ticker.C:
			// Send ping message periodically
			c.Conn.SetWriteDeadline(time.Now().Add(config.WriteWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("WebSocket ping error for %s: %v", c.UserID, err)
				return // Exit loop on ping error
			}

		case <-c.DoneChan:
			// HandleWrite closed, stop writing
			log.Printf("Received done signal for %s", c.UserID)
			return
		}
	}
}

// HandleWebSocket manages the lifecycle of a WebSocket connection
func HandleWebSocket(c *websocket.Conn, natsSvc *nats_service.NatsService) {
	// ** IMPORTANT: Add Authentication/Authorization here! **
	// Extract UserID, UserAlias from token/session passed during upgrade
	userID := "user_" + uuid.NewString()[:6] // Placeholder
	userAlias := "Guest " + userID[5:]       // Placeholder

	conversationID := c.Params("conversationID")
	if conversationID == "" {
		log.Println("Missing conversationID parameter")
		c.WriteJSON(fiber.Map{"error": "Missing conversationID"}) // Use Fiber context if available, or handle error differently
		c.Close()
		return
	}

	client := NewClient(c, natsSvc, conversationID, userID, userAlias)
	log.Printf("Client %s connected to conversation %s", client.UserID, client.ConversationID)

	// Use a context for managing the NATS subscription lifecycle tied to the WS connection
	subCtx, cancelSub := context.WithCancel(context.Background())
	defer cancelSub() // Cancel the context when the handler exits

	// Subscribe to NATS messages for this conversation
	consumeCtx, err := client.NatsService.SubscribeToConversation(subCtx, client.ConversationID, func(msg *models.Message) {
		// This handler runs in the NATS message delivery goroutine
		// Send message to the client's buffered channel
		select {
		case client.MessageChan <- msg:
		case <-time.After(1 * time.Second): // Timeout to prevent blocking NATS delivery
			log.Printf("Timeout sending message to client %s channel", client.UserID)
		case <-client.DoneChan: // Check if client disconnected
			log.Printf("Client %s disconnected before message could be sent", client.UserID)
		}
	})
	if err != nil {
		log.Printf("Failed to subscribe client %s to %s: %v", client.UserID, client.ConversationID, err)
		c.Close()
		return
	}

	// Cleanup subscription and connection when handler exits
	defer func() {
		log.Printf("Cleaning up for client %s in %s", client.UserID, client.ConversationID)
		if consumeCtx != nil {
			consumeCtx.Stop() // Stop the NATS consumer
		}
		close(client.MessageChan) // Close channel *after* stopping consumer
		c.Close()                 // Close WebSocket connection
	}()

	// Start the write in a separate goroutine
	go client.HandleWrite()

	// Start the read (blocking call) in the main handler goroutine
	// It will exit when the connection closes or errors, triggering defers.
	client.HandleRead(subCtx) // Pass context for potential cancellation signals

	log.Printf("HandleWebSocket finished for %s", client.UserID)
}

package models

import (
	"time"
)

// Message represents a chat message
type Message struct {
	ID             string    `json:"id"`             // Unique message ID (e.g., UUID)
	ConversationID string    `json:"conversationId"` // ID of the chat room/conversation
	SenderID       string    `json:"senderId"`       // ID of the user sending the message
	SenderAlias    string    `json:"senderAlias"`    // Display name of the sender
	Text           string    `json:"text"`           // Message content
	CreatedAt      time.Time `json:"createdAt"`      // Timestamp of message creation
}

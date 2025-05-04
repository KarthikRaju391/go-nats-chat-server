package config

import "time"

const (
	NatsURL        = "nats://localhost:4222"
	StreamName     = "CHAT_MESSAGES"
	SubjectPrefix  = "chat.conversation" // e.g., chat.conversation.general
	ServerAddr     = ":8080"
	WriteWait      = 10 * time.Second    // Time allowed to write a message to the peer.
	PongWait       = 60 * time.Second    // Time allowed to read the next pong message from the peer.
	PingPeriod     = (PongWait * 9) / 10 // Send pings to peer with this period. Must be less than pongWait.
	MaxMessageSize = 512
)

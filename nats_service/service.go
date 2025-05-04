package nats_service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/karthikraju391/go-nats-chat-server/config"
	"github.com/karthikraju391/go-nats-chat-server/models"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type NatsService struct {
	js jetstream.JetStream
	nc *nats.Conn
}

// NewNatsService connects to NATS and initializes JetStream
func NewNatsService() (*NatsService, error) {
	nc, err := nats.Connect(config.NatsURL) // can also use nats.DefaultURL for localhost connections
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := jetstream.New(nc) // creating a jetstream instance for the above created nats connection
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create jetstream context: %w", err)
	}

	// Ensure stream exists
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := js.Stream(ctx, config.StreamName)
	if err != nil {
		log.Printf("Stream '%s' not found, attempting to create...", config.StreamName)
		streamCfg := jetstream.StreamConfig{
			Name:        config.StreamName,
			Description: "Stores chat messages",
			Subjects:    []string{fmt.Sprintf("%s.*", config.SubjectPrefix)}, // Subject hierarchy
			MaxAge:      24 * time.Hour,                                      // Example retention policy
			Storage:     jetstream.FileStorage,                               // also allows memory storage
		}
		stream, err = js.CreateStream(ctx, streamCfg)
		if err != nil {
			nc.Close()
			return nil, fmt.Errorf("failed to create stream '%s': %w", config.StreamName, err)
		}
		log.Printf("Stream '%s' created successfully", config.StreamName)
	} else {
		log.Printf("Found existing stream '%s'", stream.CachedInfo().Config.Name)
	}

	return &NatsService{js: js, nc: nc}, nil
}

// Close NATS connection
func (s *NatsService) Close() {
	if s.nc != nil {
		s.nc.Close()
	}
}

// PublishMessage sends a message to the appropriate NATS subject
func (s *NatsService) PublishMessage(ctx context.Context, msg *models.Message) error {
	subject := getSubject(msg.ConversationID)
	msgData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Publish message to JetStream
	_, err = s.js.Publish(ctx, subject, msgData)
	if err != nil {
		return fmt.Errorf("failed to publish message to subject '%s': %w", subject, err)
	}
	log.Printf("Published message to %s (ID: %s)", subject, msg.ID)
	return nil
}

// getSubject generates the NATS subject for a conversation
func getSubject(conversationID string) string {
	return fmt.Sprintf("%s.%s", config.SubjectPrefix, conversationID)
}

// It calls the handler function for each new message received.
func (s *NatsService) SubscribeToConversation(ctx context.Context, conversationID string, handler func(msg *models.Message)) (jetstream.ConsumeContext, error) {
	subject := getSubject(conversationID)
	// lastTimestamp := time.Now().Add(-5 * time.Minute)
	// Create a durable consumer (or ephemeral if preferred)
	// Durable consumers remember their position. Ephemeral start from now/end.
	// Using ephemeral here for simplicity, means clients only get messages sent *after* they connect.
	cons, err := s.js.CreateOrUpdateConsumer(ctx, config.StreamName, jetstream.ConsumerConfig{
		FilterSubject: subject,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		// DeliverPolicy: jetstream.DeliveryByStartTimePolicy, // Start from point in time
		AckPolicy: jetstream.AckNonePolicy,
		// OptStartTime:  lastTimestamp // this starts consuming messages added 5 minutes ago
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer for subject '%s': %w", subject, err)
	}

	log.Printf("Subscribing to %s", subject)

	// Consume messages
	consumeCtx, err := cons.Consume(func(jsMsg jetstream.Msg) {
		var msg models.Message
		if err := json.Unmarshal(jsMsg.Data(), &msg); err != nil {
			log.Printf("Error unmarshaling message from subject '%s': %v", jsMsg.Subject(), err)
			return
		}

		// Process the message using the provided handler
		handler(&msg)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start consuming from subject '%s': %w", subject, err)
	}

	// Return the context so the caller can stop it later
	return consumeCtx, nil
}

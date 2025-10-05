package broker

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jlbutler/cloud-native-encabulator/services/processing-plane/logarithmic-casing-service/internal/models"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

type NATSBroker struct {
	conn   *nats.Conn
	logger zerolog.Logger
	stats  *Stats
	mu     sync.RWMutex
}

type Stats struct {
	TotalMessages    int64
	TopicMessages    map[string]int64
	TopicSubscribers map[string]int
	mu               sync.RWMutex
}

func NewNATSBroker(url string, logger zerolog.Logger) (*NATSBroker, error) {
	conn, err := nats.Connect(url,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if err != nil {
				logger.Warn().Err(err).Msg("NATS disconnected")
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info().Str("url", nc.ConnectedUrl()).Msg("NATS reconnected")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	logger.Info().Str("url", url).Msg("Connected to NATS")

	return &NATSBroker{
		conn:   conn,
		logger: logger,
		stats: &Stats{
			TopicMessages:    make(map[string]int64),
			TopicSubscribers: make(map[string]int),
		},
	}, nil
}

func (b *NATSBroker) Publish(topic string, payload map[string]interface{}, headers map[string]string) (*models.PublishResponse, error) {
	msg := models.Message{
		ID:        uuid.New().String(),
		Topic:     topic,
		Payload:   payload,
		Timestamp: time.Now(),
		Headers:   headers,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	// Create NATS message with headers
	natsMsg := &nats.Msg{
		Subject: topic,
		Data:    data,
		Header:  make(nats.Header),
	}

	for k, v := range headers {
		natsMsg.Header.Add(k, v)
	}

	if err := b.conn.PublishMsg(natsMsg); err != nil {
		return nil, fmt.Errorf("failed to publish message: %w", err)
	}

	// Update stats
	b.stats.mu.Lock()
	b.stats.TotalMessages++
	b.stats.TopicMessages[topic]++
	b.stats.mu.Unlock()

	b.logger.Debug().
		Str("messageId", msg.ID).
		Str("topic", topic).
		Msg("Message published")

	return &models.PublishResponse{
		MessageID: msg.ID,
		Topic:     topic,
		Timestamp: msg.Timestamp,
	}, nil
}

func (b *NATSBroker) Subscribe(topic string, handler func(*models.Message) error) error {
	_, err := b.conn.Subscribe(topic, func(m *nats.Msg) {
		var msg models.Message
		if err := json.Unmarshal(m.Data, &msg); err != nil {
			b.logger.Error().Err(err).Str("topic", topic).Msg("Failed to unmarshal message")
			return
		}

		if err := handler(&msg); err != nil {
			b.logger.Error().Err(err).Str("topic", topic).Str("messageId", msg.ID).Msg("Handler error")
		}
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to topic %s: %w", topic, err)
	}

	// Update stats
	b.stats.mu.Lock()
	b.stats.TopicSubscribers[topic]++
	b.stats.mu.Unlock()

	b.logger.Info().Str("topic", topic).Msg("Subscribed to topic")
	return nil
}

func (b *NATSBroker) GetStats() *models.BrokerStats {
	b.stats.mu.RLock()
	defer b.stats.mu.RUnlock()

	topics := make([]models.TopicStats, 0, len(b.stats.TopicMessages))
	for topic, count := range b.stats.TopicMessages {
		topics = append(topics, models.TopicStats{
			Topic:           topic,
			MessagesCount:   count,
			SubscriberCount: b.stats.TopicSubscribers[topic],
		})
	}

	return &models.BrokerStats{
		TotalMessages:    b.stats.TotalMessages,
		TotalTopics:      len(b.stats.TopicMessages),
		TotalSubscribers: len(b.stats.TopicSubscribers),
		Topics:           topics,
	}
}

func (b *NATSBroker) Ping() error {
	if !b.conn.IsConnected() {
		return fmt.Errorf("not connected to NATS")
	}
	return nil
}

func (b *NATSBroker) Close() error {
	b.conn.Close()
	b.logger.Info().Msg("NATS connection closed")
	return nil
}

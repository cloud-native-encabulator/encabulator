package models

import "time"

// Message represents a message in the system
type Message struct {
	ID        string                 `json:"id"`
	Topic     string                 `json:"topic"`
	Payload   map[string]interface{} `json:"payload"`
	Timestamp time.Time              `json:"timestamp"`
	Headers   map[string]string      `json:"headers,omitempty"`
}

// PublishRequest represents a request to publish a message
type PublishRequest struct {
	Topic   string                 `json:"topic"`
	Payload map[string]interface{} `json:"payload"`
	Headers map[string]string      `json:"headers,omitempty"`
}

// PublishResponse represents the response from publishing a message
type PublishResponse struct {
	MessageID string    `json:"messageId"`
	Topic     string    `json:"topic"`
	Timestamp time.Time `json:"timestamp"`
}

// TopicStats represents statistics for a topic
type TopicStats struct {
	Topic           string `json:"topic"`
	MessagesCount   int64  `json:"messagesCount"`
	SubscriberCount int    `json:"subscriberCount"`
}

// BrokerStats represents overall broker statistics
type BrokerStats struct {
	TotalMessages    int64        `json:"totalMessages"`
	TotalTopics      int          `json:"totalTopics"`
	TotalSubscribers int          `json:"totalSubscribers"`
	Topics           []TopicStats `json:"topics"`
}

// ErrorResponse represents a standardized error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

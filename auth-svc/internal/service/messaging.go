package service

import "context"

// application layer interface for sending messages over a message channel
type OutboundMessageClient interface {
	// Publish returns once the broker has persisted the message. msgID lets the
	// broker drop duplicates of the same message (e.g. retries).
	Publish(ctx context.Context, subject string, msgID string, payload []byte) error
}

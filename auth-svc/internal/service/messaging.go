package service

// application layer interface for sending messages over a message channel
type OutboundMessageClient interface {
	Send(topic, message string) error
}

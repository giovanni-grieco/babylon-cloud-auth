package kafka

type KafkaInboundPortConfig struct {
}

type KafkaInboundPort struct {
	cfg KafkaInboundPortConfig
}

func New(cfg KafkaInboundPortConfig) *KafkaInboundPort {
	return &KafkaInboundPort{
		cfg: cfg,
	}
}

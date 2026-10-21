package messaging

type NatsInboundPortConfig struct {
}

type NatsInboundPort struct {
	cfg NatsInboundPortConfig
}

func New(cfg NatsInboundPortConfig) *NatsInboundPort {
	return &NatsInboundPort{
		cfg: cfg,
	}
}

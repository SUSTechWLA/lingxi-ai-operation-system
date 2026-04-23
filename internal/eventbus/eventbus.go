package eventbus

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/IBM/sarama"
	"go.uber.org/zap"

	"github.com/lingxi-ai/lingxi-ai-operation-system/internal/config"
)

const (
	TopicNodeReady    = "ai.node.ready"
	TopicNodeResult   = "ai.node.result"
	TopicNodeExecuted = "ai.node.executed"
	TopicNodeFailed   = "ai.node.failed"
	TopicTaskCreated  = "ai.task.created"
	TopicTaskCompleted = "ai.task.completed"
	TopicTaskFailed   = "ai.task.failed"
)

type Event struct {
	TaskID         string                 `json:"taskId"`
	NodeID         string                 `json:"nodeId,omitempty"`
	Type           string                 `json:"type,omitempty"`
	Status         string                 `json:"status,omitempty"`
	Payload        map[string]interface{} `json:"payload,omitempty"`
	Output         map[string]interface{} `json:"output,omitempty"`
	TraceID        string                 `json:"traceId,omitempty"`
	IdempotencyKey string                 `json:"idempotencyKey,omitempty"`
	ErrorMessage   string                 `json:"errorMessage,omitempty"`
}

type Producer struct {
	producer sarama.SyncProducer
}

func NewProducer(cfg config.KafkaConfig) *Producer {
	saramaCfg := sarama.NewConfig()
	saramaCfg.Producer.RequiredAcks = sarama.WaitForAll
	saramaCfg.Producer.Retry.Max = 5
	saramaCfg.Producer.Return.Successes = true

	producer, err := sarama.NewSyncProducer([]string{cfg.BootstrapServers}, saramaCfg)
	if err != nil {
		zap.L().Fatal("Failed to create Kafka producer", zap.Error(err))
	}

	zap.L().Info("Kafka producer connected", zap.String("brokers", cfg.BootstrapServers))
	return &Producer{producer: producer}
}

func (p *Producer) Publish(topic, key string, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		zap.L().Error("Failed to marshal event", zap.Error(err))
		return err
	}

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(data),
	}

	_, _, err = p.producer.SendMessage(msg)
	if err != nil {
		zap.L().Error("Failed to publish event", zap.String("topic", topic), zap.Error(err))
		return err
	}

	zap.L().Info("Event published", zap.String("topic", topic), zap.String("key", key))
	return nil
}

func (p *Producer) Close() error {
	return p.producer.Close()
}

type HandlerFunc func(event Event) error

type Consumer struct {
	consumer sarama.ConsumerGroup
	handler  HandlerFunc
	topics   []string
	groupID  string
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewConsumer(cfg config.KafkaConfig, groupID string, topics []string, handler HandlerFunc) *Consumer {
	saramaCfg := sarama.NewConfig()
	saramaCfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	saramaCfg.Consumer.Offsets.Initial = sarama.OffsetOldest

	consumer, err := sarama.NewConsumerGroup([]string{cfg.BootstrapServers}, groupID, saramaCfg)
	if err != nil {
		zap.L().Fatal("Failed to create Kafka consumer", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())

	zap.L().Info("Kafka consumer created",
		zap.String("groupID", groupID),
		zap.Strings("topics", topics),
	)

	return &Consumer{
		consumer: consumer,
		handler:  handler,
		topics:   topics,
		groupID:  groupID,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (c *Consumer) Start() {
	go func() {
		handler := &consumerGroupHandler{handlerFn: c.handler}
		for {
			select {
			case <-c.ctx.Done():
				return
			default:
				if err := c.consumer.Consume(c.ctx, c.topics, handler); err != nil {
					zap.L().Error("Consumer error", zap.Error(err))
				}
			}
		}
	}()
}

func (c *Consumer) Stop() {
	c.cancel()
	_ = c.consumer.Close()
}

type consumerGroupHandler struct {
	handlerFn HandlerFunc
}

func (h *consumerGroupHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *consumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		var event Event
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			zap.L().Error("Failed to unmarshal event", zap.Error(err))
			session.MarkMessage(msg, "")
			continue
		}

		if err := h.handlerFn(event); err != nil {
			zap.L().Error("Failed to handle event",
				zap.String("topic", msg.Topic),
				zap.Error(err),
			)
		}

		session.MarkMessage(msg, "")
	}
	return nil
}

func TopicName(topic string) string {
	return fmt.Sprintf("ai.%s", topic)
}

package testutil

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	"integration-suite/proto/sortmistake"
)

// EnsureTopic creates a Kafka topic if it doesn't already exist.
func EnsureTopic(brokerAddr string, topic string, numPartitions int, replicationFactor int) error {
	conn, err := kafka.Dial("tcp", brokerAddr)
	if err != nil {
		return fmt.Errorf("failed to dial kafka at %s: %w", brokerAddr, err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("failed to get kafka controller: %w", err)
	}

	controllerConn, err := kafka.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return fmt.Errorf("failed to dial kafka controller: %w", err)
	}
	defer controllerConn.Close()

	topicConfig := kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     numPartitions,
		ReplicationFactor: replicationFactor,
	}

	err = controllerConn.CreateTopics(topicConfig)
	if err != nil {
		return fmt.Errorf("failed to create topic %s: %w", topic, err)
	}

	return nil
}

// NewKafkaReader creates a new kafka-go reader starting from the earliest unread offset.
func NewKafkaReader(brokerAddr string, topic string, groupID string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{brokerAddr},
		Topic:          topic,
		GroupID:        groupID,
		StartOffset:    kafka.FirstOffset,
		MinBytes:       1,
		MaxBytes:       10e6, // 10MB
		CommitInterval: time.Second,
	})
}

// ReadNextSortNodeEvent reads the next message from the topic and unmarshals it as SortNodeEvents.
func ReadNextSortNodeEvent(ctx context.Context, reader *kafka.Reader) (*sortmistake.SortNodeEvents, error) {
	msg, err := reader.ReadMessage(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed reading message from kafka: %w", err)
	}

	var events sortmistake.SortNodeEvents
	if err := proto.Unmarshal(msg.Value, &events); err != nil {
		return nil, fmt.Errorf("failed unmarshaling SortNodeEvents from kafka message: %w", err)
	}

	return &events, nil
}

// PublishSortNodeEvents publishes a SortNodeEvents protobuf message to a Kafka topic.
func PublishSortNodeEvents(ctx context.Context, brokerAddr string, topic string, events *sortmistake.SortNodeEvents) error {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokerAddr),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireAll,
	}
	defer writer.Close()

	bytesVal, err := proto.Marshal(events)
	if err != nil {
		return fmt.Errorf("failed marshaling SortNodeEvents: %w", err)
	}

	key := events.SystemId
	err = writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: bytesVal,
	})
	if err != nil {
		return fmt.Errorf("failed writing message to kafka topic %s: %w", topic, err)
	}

	return nil
}

// Package kafka_producer kafka 生产者的包装
package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Shopify/sarama"
)

func main() {
	p, err := NewProducer(&Config{
		Topic:      "test-topic1",
		Broker:     "127.0.0.1:9092",
		Frequency:  500,
		MaxMessage: 1000,
	})
	if err != nil {
		log.Printf("new producer error")
		return
	}
	go p.Run()

	for i := 0; i <= 200; i++ {
		p.Log("test", fmt.Sprintf("%d", i))
		time.Sleep(20 * time.Millisecond)
	}
}

// Config 配置
type Config struct {
	Topic      string `xml:"topic"`
	Broker     string `xml:"broker"`
	Frequency  int    `xml:"frequency"`
	MaxMessage int    `xml:"max_message"`
}

type Producer struct {
	producer sarama.AsyncProducer

	topic     string
	msgQ      chan *sarama.ProducerMessage
	wg        sync.WaitGroup
	closeChan chan struct{}
}

// NewProducer 构造KafkaProducer
func NewProducer(cfg *Config) (*Producer, error) {

	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.NoResponse                                  // Only wait for the leader to ack
	config.Producer.Compression = sarama.CompressionSnappy                            // Compress messages
	config.Producer.Flush.Frequency = time.Duration(cfg.Frequency) * time.Millisecond // Flush batches every 500ms
	config.Producer.Partitioner = sarama.NewRandomPartitioner

	p, err := sarama.NewAsyncProducer(strings.Split(cfg.Broker, ","), config)
	if err != nil {
		return nil, err
	}
	ret := &Producer{
		producer:  p,
		topic:     cfg.Topic,
		msgQ:      make(chan *sarama.ProducerMessage, cfg.MaxMessage),
		closeChan: make(chan struct{}),
	}

	return ret, nil
}

// Run 运行
func (p *Producer) Run() {

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

	LOOP:
		for {
			select {
			case m := <-p.msgQ:
				p.producer.Input() <- m
			case err := <-p.producer.Errors():
				if nil != err && nil != err.Msg {
					log.Printf("[producer] err=[%s] topic=[%s] key=[%s] val=[%s]", err.Error(), err.Msg.Topic, err.Msg.Key, err.Msg.Value)
				}
			case <-p.closeChan:
				break LOOP
			}

		}
	}()

	for hasTask := true; hasTask; {
		select {
		case m := <-p.msgQ:
			p.producer.Input() <- m
		default:
			hasTask = false
		}
	}

}

// Close 关闭
func (p *Producer) Close() error {
	close(p.closeChan)
	log.Printf("[producer] is quiting")
	p.wg.Wait()
	log.Printf("[producer] quit over")

	return p.producer.Close()
}

// Log 发送log
func (p *Producer) Log(key string, val string) {
	msg := &sarama.ProducerMessage{
		Topic: p.topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.StringEncoder(val),
	}

	select {
	case p.msgQ <- msg:
		return
	default:
		log.Printf("[producer] err=[msgQ is full] key=[%s] val=[%s]", msg.Key, msg.Value)
	}
}

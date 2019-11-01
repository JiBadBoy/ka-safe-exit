package consumer

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-redis/redis"

	"github.com/Shopify/sarama"
)

// Sarama configuration options
var (
	brokers  = ""
	version  = ""
	group    = ""
	topics   = ""
	assignor = ""
	oldest   = true
	verbose  = false
	client   *redis.Client
)

func init() {
	flag.StringVar(&brokers, "brokers", "127.0.0.1:9092", "Kafka bootstrap brokers to connect to, as a comma separated list")
	flag.StringVar(&group, "group", "test3", "Kafka consumer group definition")
	flag.StringVar(&version, "version", "2.1.1", "Kafka cluster version")
	flag.StringVar(&topics, "topics", "test-topic1", "Kafka topics to be consumed, as a comma seperated list")
	flag.StringVar(&assignor, "assignor", "range", "Consumer group partition assignment strategy (range, roundrobin, sticky)")
	flag.BoolVar(&oldest, "oldest", true, "Kafka consumer consume initial offset from oldest")
	flag.BoolVar(&verbose, "verbose", false, "Sarama logging")
	flag.Parse()

	if len(brokers) == 0 {
		panic("no Kafka bootstrap brokers defined, please set the -brokers flag")
	}

	if len(topics) == 0 {
		panic("no topics given to be consumed, please set the -topics flag")
	}

	if len(group) == 0 {
		panic("no Kafka consumer group defined, please set the -group flag")
	}

	client = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

}

func main() {
	log.Println("Starting a new Sarama consumer")

	if verbose {
		sarama.Logger = log.New(os.Stdout, "[sarama] ", log.LstdFlags)
	}

	version, err := sarama.ParseKafkaVersion(version)
	if err != nil {
		log.Panicf("Error parsing Kafka version: %v", err)
	}

	/**
	 * Construct a new Sarama configuration.
	 * The Kafka cluster version has to be defined before the consumer/producer is initialized.
	 */
	config := sarama.NewConfig()
	config.Version = version

	switch assignor {
	case "sticky":
		config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategySticky
	case "roundrobin":
		config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	case "range":
		config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRange
	default:
		log.Panicf("Unrecognized consumer group partition assignor: %s", assignor)
	}

	if oldest {
		config.Consumer.Offsets.Initial = sarama.OffsetOldest
	}
	ctx, cancel := context.WithCancel(context.Background())
	consumer := Consumer{ready: make(chan bool)}
	//
	wg := &sync.WaitGroup{}
	// 主协程的WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go CreateKafkaHandler(topics, group, ctx, wg, config, consumer)
	}
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT,syscall.SIGKILL)
	select {
	case <-sigterm:
		log.Println("terminating: via signal")
		cancel()
	}
	wg.Wait()
	fmt.Println(client.Get("test").Result())
}

func CreateKafkaHandler(topic, groupId string, ctx1 context.Context, w *sync.WaitGroup, config *sarama.Config, handler Consumer) {
	defer w.Done()
	ctx, cancel := context.WithCancel(context.Background())
	client, err := sarama.NewConsumerGroup(strings.Split(brokers, ","), groupId, config)
	if err != nil {
		log.Panicf("Error creating consumer group client: %v", err)
	}
	// 子协程的WaitGroup
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			// 为了重试
			if err := client.Consume(ctx, strings.Split(topic, ","), &handler); err != nil {
				log.Panicf("Error from consumer: %v", err)
			}
			// check if context was cancelled, signaling that the consumer should stop
			if ctx.Err() != nil {
				return
			}
			//handler.ready = make(chan bool)
		}
	}()
	//<-handler.ready
	log.Printf("consumerGroup set up")

	// 监听主协程的上下文
	select {
	case <-ctx1.Done():
		log.Println("terminating: context cancelled")
	}
	// 有中断信号，就关闭当前的上下文，通知消费者停止拉取
	cancel()
	// 等待消费者处理完已拉取下来的信息
	wg.Wait()
	if err = client.Close(); err != nil {
		log.Panicf("Error closing client: %v", err)
	}

}

// Consumer represents a Sarama consumer group consumer
type Consumer struct {
	ready chan bool
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (consumer *Consumer) Setup(sarama.ConsumerGroupSession) error {
	// Mark the consumer as ready
	//close(consumer.ready)
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (consumer *Consumer) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages().
func (consumer *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {

	// NOTE:
	// Do not move the code below to a goroutine.
	// The `ConsumeClaim` itself is called within a goroutine, see:
	// https://github.com/Shopify/sarama/blob/master/consumer_group.go#L27-L29
	for message := range claim.Messages() {
		time.Sleep(100 * time.Millisecond)
		client.Incr("test")
		log.Printf("%d Message claimed: value = %s, timestamp = %v, topic = %s", GetGID(), string(message.Value), message.Timestamp, message.Topic)
		session.MarkMessage(message, "")

	}
	return nil
}

func GetGID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

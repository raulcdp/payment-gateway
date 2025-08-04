package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"payment-gateway/internal/schemas"
	"payment-gateway/internal/services"
	"payment-gateway/internal/worker"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()
	cacheClient := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_URL"),
	})

	if err := cacheClient.Ping(ctx).Err(); err != nil {
		panic(fmt.Sprintf("failed to connect to Redis: %v", err))
	}
	defer cacheClient.Close()

	paymentService := services.NewPaymentService(cacheClient)

	wp := worker.NewWorkerPool(ctx, 25, paymentService.ProcessPayment)
	wp.Start()
	defer wp.Stop()

	streamName := "payments"
	groupName := "workers"
	consumerName := "worker-1"

	err := cacheClient.XGroupCreateMkStream(ctx, streamName, groupName, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		log.Fatal(err)
	}

	for {
		fmt.Print("Waiting for messages...\n")
		entries, err := cacheClient.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    groupName,
			Consumer: consumerName,
			Streams:  []string{streamName, ">"},
			Count:    100,
			Block:    2 * time.Second,
		}).Result()

		if err != nil && err != redis.Nil {
			log.Printf("read error: %v", err)
			continue
		}

		for _, stream := range entries {
			for _, msg := range stream.Messages {
				correlationId, _ := msg.Values["correlationId"].(string)
				amountStr, _ := msg.Values["amount"].(string)
				amount, err := strconv.ParseFloat(amountStr, 64)
				if err != nil {
					log.Printf("Failed to parse amount: %v", err)
					return
				}
				paymentRequest := &schemas.PaymentRequest{
					CorrelationId: correlationId,
					Amount:        amount,
				}

				wp.Submit(paymentRequest)

				err = cacheClient.XAck(ctx, streamName, groupName, msg.ID).Err()
				if err != nil {
					log.Printf("Failed to ACK message %s: %v", msg.ID, err)
				}
			}
		}
	}
}

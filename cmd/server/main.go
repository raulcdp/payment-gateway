package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"payment-gateway/internal/schemas"
	"payment-gateway/internal/services"

	"os"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()
	r := gin.Default()
	cacheClient := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_URL"),
	})

	if err := cacheClient.Ping(ctx).Err(); err != nil {
		panic(fmt.Sprintf("failed to connect to Redis: %v", err))
	}
	defer cacheClient.Close()
	paymentservice := services.NewPaymentService(cacheClient)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Payment Gateway is running",
		})
	})

	r.GET("/payments-summary", func(c *gin.Context) {
		from := c.Query("from")
		to := c.Query("to")
		var fromDate time.Time
		var toDate time.Time
		var err error

		if from != "" {
			fromDate, err = time.Parse("2006-01-02T15:04:05.000Z", from)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"message": fmt.Sprintf("error while parsing 'from' parameter with value %s", c.Query("from")),
				})
				return
			}
		}

		if to != "" {
			toDate, err = time.Parse("2006-01-02T15:04:05.000Z", to)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"message": fmt.Sprintf("error while parsing 'to' parameter with value %s - err %v", to, err),
				})
				return
			}
		}

		paymentSummary, err := paymentservice.GetSummary(ctx, fromDate, toDate)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"message": fmt.Sprintf("error while getting payment summary: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, paymentSummary)
	})

	r.POST("/payments", func(c *gin.Context) {
		var r schemas.PaymentRequest

		if err := c.ShouldBindJSON(&r); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"message": fmt.Sprintf("Invalid payment request - error: %v", err.Error()),
			})
			return
		}

		err := paymentservice.QueuePayment(context.Background(), &r)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"message": fmt.Sprintf("error while processing payment: %v", err),
			})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{
			"message": "Payment request received",
			"data":    r,
		})
	})
	r.Run(":8080")
}

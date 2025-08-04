package services

import (
	"context"
	"fmt"
	"payment-gateway/internal/datatypes"
	"payment-gateway/internal/providers"
	"payment-gateway/internal/schemas"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	PaymentsSummaryKey = "payments-summary"
)

type IPaymentService interface {
	QueuePayment(ctx context.Context, payment *schemas.PaymentRequest) error
	ProcessPayment(ctx context.Context, payment *schemas.PaymentRequest) error
	GetSummary(ctx context.Context, from, to time.Time) (schemas.PaymentSummaryResponse, error)
}

type Option func(*PaymentService)

type PaymentService struct {
	cacheClient *redis.Client
	providers   map[string]providers.IProvider
}

func NewPaymentService(cacheClient *redis.Client) IPaymentService {
	providers := map[string]providers.IProvider{
		"default":  providers.NewProvider("default", providers.DefaultProviderURL),
		"fallback": providers.NewProvider("fallback", providers.FallbackProviderURL),
	}
	svc := &PaymentService{
		cacheClient: cacheClient,
		providers:   providers,
	}
	return svc
}

func (p *PaymentService) QueuePayment(ctx context.Context, payment *schemas.PaymentRequest) error {
	go func() {
		paymentMap, err := datatypes.StructToMap(payment)
		if err != nil {
			fmt.Printf("error while parsing payment: %v\n", err)
		}

		err = p.cacheClient.XAdd(ctx, &redis.XAddArgs{
			Stream: "payments",
			Values: paymentMap,
			MaxLen: 100000,
			Approx: true,
		}).Err()
		if err != nil {
			fmt.Printf("error while adding payment to queue: %v\n", err)
		}
	}()
	return nil
}

func (p *PaymentService) ProcessPayment(ctx context.Context, payment *schemas.PaymentRequest) error {
	if payment.Retry > 3 {
		fmt.Printf("payment %s has exceeded retry limit, skipping processing\n", payment.CorrelationId)
		return nil
	}

	provider := p.getProvider()

	requestedAt, err := provider.ProcessPayment(payment)
	if err != nil {
		payment.Retry++
		fmt.Printf("error while processing payment with provider %s, retrying (%d): %v\n", provider.GetID(), payment.Retry, err)
		err = p.QueuePayment(ctx, payment)
		if err != nil {
			fmt.Printf("error while re-queuing payment: %v\n", err)
		}
		fmt.Printf("error while processing payment with provider %s: %v\n", provider, err)
		return nil
	}

	err = p.pubPaymentSummary(ctx, &schemas.PaymentDetails{
		CorrelationID: payment.CorrelationId, Amount: payment.Amount, Provider: provider.GetID(), ProcessedAt: requestedAt,
	})
	if err != nil {
		fmt.Printf("error while publishing payment summary: %v\n", err)
	}
	fmt.Println("Payment processed successfully")
	return nil
}

func (p *PaymentService) getProvider() providers.IProvider {
	cachedProvider, _ := p.cacheClient.Get(context.Background(), "provider").Result()
	if cachedProvider != "" {
		fmt.Println("Using cached provider:", cachedProvider)
		return p.providers[cachedProvider]
	}

	df, errDefault := p.providers["default"].HealthCheck()
	fb, errFallback := p.providers["fallback"].HealthCheck()
	provider := p.providers["default"]
	defer func() {
		p.cacheClient.Set(context.Background(), "provider", provider.GetID(), 5*time.Second)
	}()

	if errDefault != nil {
		fmt.Printf("error while checking default provider health: %v\n", errDefault)
		provider = p.providers["fallback"]
		return provider
	}

	if errFallback != nil {
		fmt.Printf("error while checking fallback provider health: %v\n", errFallback)
		provider = p.providers["default"]
		return provider
	}

	if df.Failing && !fb.Failing {
		fmt.Println("Default provider is failing, using fallback provider")
		provider = p.providers["fallback"]
		return provider
	}

	if df.Failing && fb.Failing && fb.MinResponseTime*10 < df.MinResponseTime*9 {
		fmt.Println("Default provider is failing, using fallback provider")
		provider = p.providers["fallback"]
		return provider
	}

	return provider
}

func (p *PaymentService) pubPaymentSummary(ctx context.Context, payment *schemas.PaymentDetails) error {
	err := p.cacheClient.ZAdd(ctx, PaymentsSummaryKey, redis.Z{
		Score:  float64(payment.ProcessedAt.UnixMicro()),
		Member: payment,
	}).Err()
	if err != nil {
		fmt.Printf("pubPaymentSummary: error while adding payment to cache: %v\n", err)
	}
	return nil
}

func (p *PaymentService) GetSummary(ctx context.Context, from, to time.Time) (schemas.PaymentSummaryResponse, error) {
	paymentSummary := schemas.PaymentSummaryResponse{Default: &schemas.PaymentSummary{}, Fallback: &schemas.PaymentSummary{}}
	rangeby := &redis.ZRangeBy{Min: "-inf", Max: "+inf"}

	if !from.IsZero() && !to.IsZero() {
		rangeby.Min = fmt.Sprintf("%d", from.UnixMicro())
		rangeby.Max = fmt.Sprintf("%d", to.UnixMicro())
	} else if !from.IsZero() {
		rangeby.Min = fmt.Sprintf("%d", from.UnixMicro())
	} else if !to.IsZero() {
		rangeby.Max = fmt.Sprintf("%d", to.UnixMicro())
	}

	values, err := p.cacheClient.ZRangeByScore(ctx, PaymentsSummaryKey, rangeby).Result()
	if err != nil {
		fmt.Printf("pubPaymentSummary: error while getting payment summary from cache: %v\n", err)
	}

	for _, v := range values {
		pd, err := schemas.FromStringToPaymentDetails(v)
		if err != nil {
			fmt.Printf("pubPaymentSummary: error while parsing payment details: %v\n", err)
		}
		if pd.Provider == "default" {
			paymentSummary.Default.TotalRequests++
			paymentSummary.Default.TotalAmount += pd.Amount
		} else if pd.Provider == "fallback" {
			paymentSummary.Fallback.TotalRequests++
			paymentSummary.Fallback.TotalAmount += pd.Amount
		} else {
			fmt.Printf("pubPaymentSummary: unknown provider %s for payment %s\n", pd.Provider, pd.CorrelationID)
		}
	}

	fmt.Println("Default:", paymentSummary.Default)
	fmt.Println("Fallback:", paymentSummary.Fallback)
	return paymentSummary, nil
}

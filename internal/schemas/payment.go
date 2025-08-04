package schemas

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type PaymentRequest struct {
	CorrelationId string  `json:"correlationId"`
	Amount        float64 `json:"amount"`
	Retry         int     `json:"retry"`
}

type PaymentResponse struct {
}

type PaymentSummaryResponse struct {
	Default  *PaymentSummary `json:"default"`
	Fallback *PaymentSummary `json:"fallback"`
}

type PaymentSummary struct {
	TotalRequests int     `json:"totalRequests"`
	TotalAmount   float64 `json:"totalAmount"`
}

type PaymentDetails struct {
	CorrelationID string    `json:"correlationId"`
	Amount        float64   `json:"amount"`
	Provider      string    `json:"provider"`
	ProcessedAt   time.Time `json:"processedAt"`
}

func (p PaymentDetails) MarshalBinary() (data []byte, err error) {
	return fmt.Appendf([]byte{}, "%s:%.2f:%s", p.CorrelationID, p.Amount, p.Provider), nil
}

func FromStringToPaymentDetails(s string) (PaymentDetails, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return PaymentDetails{}, fmt.Errorf("invalid payment details format")
	}

	amount, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return PaymentDetails{}, fmt.Errorf("invalid amount: %v", err)
	}

	return PaymentDetails{
		CorrelationID: parts[0],
		Amount:        amount,
		Provider:      parts[2],
	}, nil
}

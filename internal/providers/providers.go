package providers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"payment-gateway/internal/schemas"
	"time"
)

var (
	DefaultProviderURL  = "http://payment-processor-default:8080"
	FallbackProviderURL = "http://payment-processor-fallback:8080"

	ErrUnprocessableEntity = errors.New("unprocessable entity")
)

type IProvider interface {
	ProcessPayment(payment *schemas.PaymentRequest) (time.Time, error)
	HealthCheck() (*ProviderHealthCheckResponse, error)
	GetID() string
}

type Provider struct {
	url    string
	client *http.Client
	id     string
}

type ProviderPaymentRequest struct {
	CorrelationId string    `json:"correlationId"`
	Amount        float64   `json:"amount"`
	RequestedAt   time.Time `json:"requestedAt"`
}

type ProviderPaymentResponse struct {
	Message string `json:"message"`
}

type ProviderHealthCheckResponse struct {
	Failing         bool  `json:"failing"`
	MinResponseTime int64 `json:"minResponseTime"`
}

func NewProvider(id, url string) IProvider {
	return &Provider{
		id:  id,
		url: url,
		client: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

func (p *Provider) GetID() string {
	return p.id
}

func (p *Provider) ProcessPayment(payment *schemas.PaymentRequest) (time.Time, error) {
	request := ProviderPaymentRequest{
		CorrelationId: payment.CorrelationId,
		Amount:        payment.Amount,
		RequestedAt:   time.Now().UTC(),
	}
	err := p.doRequest(http.MethodPost, "/payments", request, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("error while processing payment: %w", err)
	}
	return request.RequestedAt, nil
}

func (p *Provider) HealthCheck() (*ProviderHealthCheckResponse, error) {
	resp := &ProviderHealthCheckResponse{}
	err := p.doRequest(http.MethodGet, "/payments/service-health", nil, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (p *Provider) doRequest(method string, path string, request any, response any) error {
	var body io.Reader
	if request != nil {
		data, err := json.Marshal(request)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, fmt.Sprint(p.url, path), body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusUnprocessableEntity {
		return ErrUnprocessableEntity
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	if response != nil {
		defer resp.Body.Close()
		err = json.NewDecoder(resp.Body).Decode(response)
		if err != nil {
			return fmt.Errorf("error while decoding response body: %w", err)
		}
	}

	return nil
}

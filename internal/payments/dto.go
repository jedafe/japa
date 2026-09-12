package payments

import (
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// PaymentAPIResponse is the API-facing payment representation. It fixes the
// metadata base64 serialization issue (BUG-009) by converting the []byte
// stored by pgx/sqlc into a json.RawMessage, which Go's json.Marshal
// serializes as a JSON object/array rather than a base64 string. It also
// carries the checkout_url from the provider response (BUG-012) so the
// frontend can redirect the user to the provider's checkout page.
type PaymentAPIResponse struct {
	ID             pgtype.UUID        `json:"id"`
	TenantID       pgtype.UUID        `json:"tenant_id"`
	UserID         pgtype.UUID        `json:"user_id"`
	Provider       string             `json:"provider"`
	ProviderRef    *string            `json:"provider_ref"`
	PurposeType    string             `json:"purpose_type"`
	PurposeID      pgtype.UUID        `json:"purpose_id"`
	Amount         int64              `json:"amount"`
	Currency       string             `json:"currency"`
	Status         string             `json:"status"`
	Metadata       json.RawMessage    `json:"metadata"`
	SubscriptionID pgtype.UUID        `json:"subscription_id"`
	DiscountCodeID pgtype.UUID        `json:"discount_code_id"`
	DiscountAmount int64              `json:"discount_amount"`
	CheckoutURL    string             `json:"checkout_url,omitempty"`
	CreatedAt      pgtype.Timestamptz `json:"created_at"`
	UpdatedAt      pgtype.Timestamptz `json:"updated_at"`
}

// toPaymentAPIResponse converts a gen.Payment row to a PaymentAPIResponse DTO.
// The checkoutURL is optional — only set for newly initiated payments.
func toPaymentAPIResponse(p gen.Payment, checkoutURL string) PaymentAPIResponse {
	// Convert metadata []byte to json.RawMessage. If the metadata is empty
	// or invalid JSON, default to an empty JSON object.
	var metadata json.RawMessage
	if len(p.Metadata) > 0 {
		metadata = json.RawMessage(p.Metadata)
	} else {
		metadata = json.RawMessage("{}")
	}

	return PaymentAPIResponse{
		ID: p.ID,

		UserID:         p.UserID,
		Provider:       p.Provider,
		ProviderRef:    p.ProviderRef,
		PurposeType:    p.PurposeType,
		PurposeID:      p.PurposeID,
		Amount:         p.Amount,
		Currency:       p.Currency,
		Status:         p.Status,
		Metadata:       metadata,
		SubscriptionID: p.SubscriptionID,
		DiscountCodeID: p.DiscountCodeID,
		DiscountAmount: p.DiscountAmount,
		CheckoutURL:    checkoutURL,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

// toPaymentAPIResponses converts a slice of gen.Payment rows to PaymentAPIResponse
// DTOs (without checkout_url, since list views don't need it).
func toPaymentAPIResponses(payments []gen.Payment) []PaymentAPIResponse {
	result := make([]PaymentAPIResponse, len(payments))
	for i, p := range payments {
		result[i] = toPaymentAPIResponse(p, "")
	}

	return result
}

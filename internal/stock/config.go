package stock

import "time"

// ProviderConfigRow mirrors the stock_providers database row.
type ProviderConfigRow struct {
	ProviderName    string
	Enabled         bool
	APIKeyEncrypted string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

package literals

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/literals/expr"
)

// TemplateLiteral is the runtime model for a tenant token definition.
type TemplateLiteral struct {
	ID          string
	TenantID    string
	Key         string
	Kind        string
	Value       string
	Fallback    *string
	Label       *string
	Description *string
	Category    *string
	IsSystem    bool
	IsActive    bool
}

// RenderContext carries the request-time values available to expressions.
type RenderContext struct {
	Now                time.Time
	Locale             string
	SourceCountry      string
	SourceCode         string
	DestinationCountry string
	DestinationCode    string
	TenantName         string
	TenantID           pgtype.UUID
}

// ToEnv converts the render context to an expression evaluator environment.
func (rc RenderContext) ToEnv() *expr.Env {
	env := expr.NewEnv(rc.Now)
	env.Vars["Source_Country"] = rc.SourceCountry
	env.Vars["Source_Code"] = rc.SourceCode
	env.Vars["Destination_Country"] = rc.DestinationCountry
	env.Vars["Destination_Code"] = rc.DestinationCode
	env.Vars["Tenant_Name"] = rc.TenantName
	env.Vars["Locale"] = rc.Locale

	return env
}

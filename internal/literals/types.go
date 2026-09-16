package literals

import (
	"time"

	"github.com/jedafe/japa/internal/literals/expr"
)

// TemplateLiteral is the runtime model for a platform token definition.
type TemplateLiteral struct {
	ID string

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
	PlatformName         string
}

// reservedKeys are built-in variable and function names that cannot be
// shadowed by platform-defined template literal keys.
var reservedKeys = map[string]struct{}{
	"Now":                 {},
	"Current_Year":        {},
	"Current_Date":        {},
	"Current_Time":        {},
	"Current_Timestamp":   {},
	"Source_Country":      {},
	"Source_Code":         {},
	"Destination_Country": {},
	"Destination_Code":    {},
	"Platform_Name":         {},
	"Locale":              {},
	"adjective":           {},
	"year":                {},
	"month":               {},
	"day":                 {},
	"weekday":             {},
	"format":              {},
	"upper":               {},
	"lower":               {},
	"title":               {},
	"concat":              {},
	"if":                  {},
	"len":                 {},
}

// IsReservedKey reports whether a key is a protected built-in name.
func IsReservedKey(key string) bool {
	_, ok := reservedKeys[key]
	return ok
}

// ToEnv converts the render context to an expression evaluator environment.
func (rc RenderContext) ToEnv() *expr.Env {
	env := expr.NewEnv(rc.Now)
	env.Vars["Source_Country"] = rc.SourceCountry
	env.Vars["Source_Code"] = rc.SourceCode
	env.Vars["Destination_Country"] = rc.DestinationCountry
	env.Vars["Destination_Code"] = rc.DestinationCode
	env.Vars["Platform_Name"] = rc.PlatformName
	env.Vars["Locale"] = rc.Locale

	return env
}

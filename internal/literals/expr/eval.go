package expr

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/literals/countrydata"
)

type Function func(ctx context.Context, env *Env, args []any) (any, error)

type Env struct {
	Vars     map[string]any
	Funcs    map[string]Function
	Now      time.Time
	Depth    int
	MaxDepth int
}

func NewEnv(now time.Time) *Env {
	e := &Env{
		Vars:     make(map[string]any),
		Funcs:    make(map[string]Function),
		Now:      now,
		MaxDepth: 32,
	}
	e.addDefaultVars()
	e.addDefaultFuncs()
	return e
}

func (e *Env) addDefaultVars() {
	e.Vars["Now"] = e.Now
	e.Vars["Current_Year"] = float64(e.Now.Year())
	e.Vars["Current_Date"] = e.Now.Format("2006-01-02")
	e.Vars["Current_Time"] = e.Now.Format("15:04")
	e.Vars["Current_Timestamp"] = e.Now.Format("2006-01-02 15:04")
}

func (e *Env) addDefaultFuncs() {
	e.Funcs["adjective"] = func(_ context.Context, env *Env, args []any) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("adjective requires 1 argument")
		}
		s, ok := toString(args[0])
		if !ok {
			return nil, fmt.Errorf("adjective requires a string")
		}
		adj := countrydata.AdjectiveFor(s)
		if adj == "" {
			return s, nil // fallback to input
		}
		return adj, nil
	}
	e.Funcs["year"] = dateFunc(func(t time.Time) any { return float64(t.Year()) })
	e.Funcs["month"] = dateFunc(func(t time.Time) any { return float64(t.Month()) })
	e.Funcs["day"] = dateFunc(func(t time.Time) any { return float64(t.Day()) })
	e.Funcs["weekday"] = dateFunc(func(t time.Time) any { return t.Weekday().String() })
	e.Funcs["format"] = formatFunc
	e.Funcs["upper"] = stringFunc(strings.ToUpper)
	e.Funcs["lower"] = stringFunc(strings.ToLower)
	e.Funcs["title"] = stringFunc(strings.Title)
	e.Funcs["concat"] = concatFunc
	e.Funcs["if"] = ifFunc
	e.Funcs["len"] = func(_ context.Context, env *Env, args []any) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("len requires 1 argument")
		}
		s, ok := toString(args[0])
		if ok {
			return float64(len(s)), nil
		}
		if a, ok := args[0].([]any); ok {
			return float64(len(a)), nil
		}
		return nil, fmt.Errorf("len requires a string or array")
	}
}

func dateFunc(fn func(time.Time) any) Function {
	return func(_ context.Context, env *Env, args []any) (any, error) {
		t, err := toTime(args, env.Now)
		if err != nil {
			return nil, err
		}
		return fn(t), nil
	}
}

func formatFunc(_ context.Context, env *Env, args []any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("format requires 2 arguments")
	}
	t, err := toTime([]any{args[0]}, env.Now)
	if err != nil {
		return nil, err
	}
	layout, ok := toString(args[1])
	if !ok {
		return nil, fmt.Errorf("format layout must be a string")
	}
	layout = mapLayout(layout)
	return t.Format(layout), nil
}

func stringFunc(fn func(string) string) Function {
	return func(_ context.Context, env *Env, args []any) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("string function requires 1 argument")
		}
		s, ok := toString(args[0])
		if !ok {
			return nil, fmt.Errorf("expected string")
		}
		return fn(s), nil
	}
}

func concatFunc(_ context.Context, env *Env, args []any) (any, error) {
	var parts []string
	for _, a := range args {
		s, ok := toString(a)
		if !ok {
			return nil, fmt.Errorf("concat requires strings")
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ""), nil
}

func ifFunc(_ context.Context, env *Env, args []any) (any, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("if requires 3 arguments")
	}
	cond, ok := toBool(args[0])
	if !ok {
		return nil, fmt.Errorf("if condition must be boolean")
	}
	if cond {
		return args[1], nil
	}
	return args[2], nil
}

func mapLayout(l string) string {
	// Convert common aliases to Go layouts.
	switch l {
	case "YYYY":
		return "2006"
	case "MM":
		return "01"
	case "DD":
		return "02"
	case "HH:mm":
		return "15:04"
	case "HH":
		return "15"
	case "mm":
		return "04"
	case "MMMM":
		return "January"
	case "MMM":
		return "Jan"
	case "YYYY-MM-DD":
		return "2006-01-02"
	case "YYYY-MM-DD HH:mm":
		return "2006-01-02 15:04"
	}
	return l
}

func toString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(t), true
	case nil:
		return "", true
	}
	return "", false
}

// ToString converts an evaluator result to its string representation.
func ToString(v any) (string, bool) { return toString(v) }

func toBool(v any) (bool, bool) {
	if b, ok := v.(bool); ok {
		return b, true
	}
	if f, ok := v.(float64); ok {
		return f != 0, true
	}
	if s, ok := v.(string); ok {
		return s != "", true
	}
	return false, false
}

func toNumber(v any) (float64, bool) {
	if f, ok := v.(float64); ok {
		return f, true
	}
	if s, ok := v.(string); ok {
		f, err := strconv.ParseFloat(s, 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

func toTime(args []any, fallback time.Time) (time.Time, error) {
	if len(args) == 0 {
		return fallback, nil
	}
	switch t := args[0].(type) {
	case time.Time:
		return t, nil
	case string:
		for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04", "2006-01-02 15:04:05"} {
			if p, err := time.Parse(layout, t); err == nil {
				return p, nil
			}
		}
		return fallback, nil
	case float64:
		return time.Unix(int64(t), 0).UTC(), nil
	}
	return fallback, nil
}

func Eval(ctx context.Context, n Node, env *Env) (any, error) {
	if env.Depth >= env.MaxDepth {
		return nil, errors.New("expression exceeded max depth")
	}
	env.Depth++
	defer func() { env.Depth-- }()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	switch t := n.(type) {
	case NumberNode:
		return t.Val, nil
	case StringNode:
		return t.Val, nil
	case IdentNode:
		v, ok := env.Vars[t.Name]
		if !ok {
			return nil, fmt.Errorf("unknown variable %q", t.Name)
		}
		return v, nil
	case UnaryNode:
		v, err := Eval(ctx, t.Expr, env)
		if err != nil {
			return nil, err
		}
		f, ok := toNumber(v)
		if !ok {
			return nil, fmt.Errorf("unary %v on non-number", t.Op)
		}
		if t.Op == TokenMinus {
			return -f, nil
		}
		return f, nil
	case BinaryNode:
		left, err := Eval(ctx, t.Left, env)
		if err != nil {
			return nil, err
		}
		right, err := Eval(ctx, t.Right, env)
		if err != nil {
			return nil, err
		}
		return evalBinary(t.Op, left, right)
	case CallNode:
		fn, ok := env.Funcs[t.Name]
		if !ok {
			return nil, fmt.Errorf("unknown function %q", t.Name)
		}
		args := make([]any, len(t.Args))
		for i, a := range t.Args {
			v, err := Eval(ctx, a, env)
			if err != nil {
				return nil, err
			}
			args[i] = v
		}
		return fn(ctx, env, args)
	}
	return nil, fmt.Errorf("unknown node type %T", n)
}

func evalBinary(op TokenKind, left, right any) (any, error) {
	// String concatenation with +. Avoid treating two numbers as strings.
	if op == TokenPlus {
		_, leftIsNum := toNumber(left)
		_, rightIsNum := toNumber(right)
		if !leftIsNum || !rightIsNum {
			if ls, ok := toString(left); ok {
				if rs, ok := toString(right); ok {
					return ls + rs, nil
				}
			}
		}
	}

	// Comparisons: support both numbers and strings.
	switch op {
	case TokenEq, TokenNeq, TokenLt, TokenGt, TokenLte, TokenGte:
		l, lok := toNumber(left)
		r, rok := toNumber(right)
		if lok && rok {
			switch op {
			case TokenEq:
				return l == r, nil
			case TokenNeq:
				return l != r, nil
			case TokenLt:
				return l < r, nil
			case TokenGt:
				return l > r, nil
			case TokenLte:
				return l <= r, nil
			case TokenGte:
				return l >= r, nil
			}
		}
		ls, lsok := toString(left)
		rs, rsok := toString(right)
		if lsok && rsok {
			switch op {
			case TokenEq:
				return ls == rs, nil
			case TokenNeq:
				return ls != rs, nil
			case TokenLt:
				return ls < rs, nil
			case TokenGt:
				return ls > rs, nil
			case TokenLte:
				return ls <= rs, nil
			case TokenGte:
				return ls >= rs, nil
			}
		}
		return nil, fmt.Errorf("operator %v requires numbers or strings", op)
	}

	l, lok := toNumber(left)
	r, rok := toNumber(right)
	if !lok || !rok {
		return nil, fmt.Errorf("operator %v requires numbers or strings", op)
	}

	switch op {
	case TokenPlus:
		return l + r, nil
	case TokenMinus:
		return l - r, nil
	case TokenMul:
		return l * r, nil
	case TokenDiv:
		if r == 0 {
			return nil, errors.New("division by zero")
		}
		return l / r, nil
	case TokenMod:
		if r == 0 {
			return nil, errors.New("modulo by zero")
		}
		return float64(int64(l) % int64(r)), nil
	}
	return nil, fmt.Errorf("unsupported operator %v", op)
}

func EvalWithTimeout(ctx context.Context, n Node, env *Env, timeout time.Duration) (any, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return Eval(ctx, n, env)
}

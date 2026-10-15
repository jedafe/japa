package customfields

import "testing"

func TestComputedSpecFromOptions(t *testing.T) {
	if ComputedSpecFromOptions(nil) != nil {
		t.Fatal("nil options should parse to nil")
	}
	if ComputedSpecFromOptions([]byte(`{"currency_code":"USD"}`)) != nil {
		t.Fatal("options without computed should parse to nil")
	}
	if ComputedSpecFromOptions([]byte(`{"computed":{"metric":"bogus","sources":[{"group":"g","key":"k"}]}}`)) != nil {
		t.Fatal("invalid metric should parse to nil")
	}
	if ComputedSpecFromOptions([]byte(`{"computed":{"sources":[]}}`)) != nil {
		t.Fatal("empty sources should parse to nil")
	}
	if ComputedSpecFromOptions([]byte(`{"computed":{"sources":[{"group":"","key":"k"}]}}`)) != nil {
		t.Fatal("source missing group should parse to nil")
	}

	spec := ComputedSpecFromOptions([]byte(
		`{"computed":{"sources":[{"group":"wiki_city_prices_food_groceries","key":"monthly_total"},{"group":"wiki_city_housing","key":"rent_1br"}]}}`))
	if spec == nil {
		t.Fatal("valid spec misparsed")
	}
	if spec.Metric != "avg" {
		t.Fatalf("empty metric should default to avg, got %q", spec.Metric)
	}
	if len(spec.Sources) != 2 || spec.Sources[0].Group != "wiki_city_prices_food_groceries" || spec.Sources[0].Key != "monthly_total" {
		t.Fatalf("sources misparsed: %+v", spec.Sources)
	}

	spec = ComputedSpecFromOptions([]byte(`{"computed":{"metric":"sum","sources":[{"group":"g","key":"k"}]}}`))
	if spec == nil || spec.Metric != "sum" {
		t.Fatalf("sum metric misparsed: %+v", spec)
	}
}

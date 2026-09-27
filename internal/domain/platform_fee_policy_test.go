package domain

import "testing"

func TestPlatformFeePolicyValueBoundsAndCanonicalForm(t *testing.T) {
	valid := []PlatformFeePolicyValue{{0, 0}, {2000, 50000}, {500, 1000}}
	for _, value := range valid {
		if err := value.Validate(); err != nil {
			t.Fatalf("%+v rejected: %v", value, err)
		}
		parsed, err := ParsePlatformFeePolicyValue(value.Encode())
		if err != nil || parsed != value {
			t.Fatalf("round trip %+v -> %q -> %+v %v", value, value.Encode(), parsed, err)
		}
	}
	invalid := []PlatformFeePolicyValue{{-1, 0}, {2001, 0}, {0, -1}, {0, 50001}}
	for _, value := range invalid {
		if value.Validate() == nil {
			t.Fatalf("%+v accepted", value)
		}
	}
	for _, text := range []string{"", "percent_bps=5,fixed_fee=", "fixed_fee=0,percent_bps=0", "percent_bps=05,fixed_fee=0", "percent_bps=5, fixed_fee=0", "percent_bps=2001,fixed_fee=0", "percent_bps=0,fixed_fee=50001", "percent_bps=-1,fixed_fee=0", "percent_bps=99999999999999999999,fixed_fee=0"} {
		if _, err := ParsePlatformFeePolicyValue(text); err == nil {
			t.Fatalf("%q accepted", text)
		}
	}
}

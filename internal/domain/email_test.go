package domain

import (
	"encoding/json"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	for in, want := range map[string]string{
		"user@example.com":       "user@example.com",
		"User@Example.COM":       "user@example.com",
		"  USER@example.com\t\n": "user@example.com",
		"":                       "",
	} {
		if got := NormalizeEmail(in); got != want {
			t.Fatalf("NormalizeEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmailAddressDecodesCanonicalForm(t *testing.T) {
	var body struct {
		Email EmailAddress `json:"email"`
	}
	if err := json.Unmarshal([]byte(`{"email":"  Mixed.Case@Example.COM "}`), &body); err != nil {
		t.Fatal(err)
	}
	if body.Email.String() != "mixed.case@example.com" {
		t.Fatalf("decoded %q", body.Email)
	}
	if err := json.Unmarshal([]byte(`{"email":42}`), &body); err == nil {
		t.Fatal("accepted a non-string email")
	}
}

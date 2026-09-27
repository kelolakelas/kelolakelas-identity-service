package domain

import (
	"encoding/json"
	"strings"
)

// NormalizeEmail returns the canonical account form of an email address:
// surrounding whitespace removed and lowercased. Every account write and every
// email lookup uses this form so that one address maps to at most one account
// regardless of letter case (KEL-89). The database enforces the same rule with
// a unique index on lower(email); rows written before that rule may still be
// stored mixed-case, so lookups compare against LOWER(email).
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// EmailAddress is an email field of a JSON request body. Decoding normalizes
// it with NormalizeEmail before binding validation runs, so an address typed
// with surrounding spaces or a different letter case is accepted as the same
// account address instead of failing the email format check.
type EmailAddress string

func (e *EmailAddress) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*e = EmailAddress(NormalizeEmail(value))
	return nil
}

func (e EmailAddress) String() string { return string(e) }

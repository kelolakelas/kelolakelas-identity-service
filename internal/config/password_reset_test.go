package config

import (
	"github.com/spf13/viper"
	"strings"
	"testing"
)

func TestPasswordResetTTL(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		want    int
		invalid bool
	}{
		{"", 60, false}, {"15", 15, false}, {"0", 0, true}, {"-1", 0, true}, {"abc", 0, true}, {"999999999999999999999", 0, true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			viper.Reset()
			t.Chdir(t.TempDir())
			t.Setenv("JWT_SECRET", "test-secret")
			t.Setenv("PASSWORD_RESET_TTL_MINUTES", tc.raw)
			cfg, err := LoadConfig()
			if tc.invalid {
				if err == nil || !strings.Contains(err.Error(), "PASSWORD_RESET_TTL_MINUTES") {
					t.Fatalf("expected startup config error, got %v", err)
				}
				return
			}
			if err != nil || cfg.PasswordResetTTLMinutes != tc.want {
				t.Fatalf("ttl=%d err=%v want=%d", cfg.PasswordResetTTLMinutes, err, tc.want)
			}
		})
	}
}

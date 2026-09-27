package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestServerTimeoutConfig(t *testing.T) {
	keys := []string{
		"SERVER_READ_HEADER_TIMEOUT_SECONDS", "SERVER_READ_TIMEOUT_SECONDS", "SERVER_WRITE_TIMEOUT_SECONDS",
		"SERVER_IDLE_TIMEOUT_SECONDS", "SERVER_SHUTDOWN_TIMEOUT_SECONDS",
	}
	defaults := [5]int{DefaultServerReadHeaderTimeout, DefaultServerReadTimeout, DefaultServerWriteTimeout, DefaultServerIdleTimeout, DefaultServerShutdownTimeout}
	for _, tc := range []struct {
		name    string
		values  [5]string
		want    [5]int
		wantErr bool
	}{
		{name: "unset uses safe defaults", want: defaults},
		{name: "configured values are used", values: [5]string{"2", "20", "45", "90", "25"}, want: [5]int{2, 20, 45, 90, 25}},
		{name: "zero never disables a bound", values: [5]string{"0", "0", "0", "0", "0"}, want: defaults},
		{name: "negative falls back to default", values: [5]string{"-1", "-5", "-10", "-1", "-3"}, want: defaults},
		{name: "non-numeric header timeout is rejected", values: [5]string{"soon"}, wantErr: true},
		{name: "non-numeric shutdown timeout is rejected", values: [5]string{"", "", "", "", "later"}, wantErr: true},
		{name: "duration overflow is rejected", values: [5]string{"", "", "9223372036854775807"}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()
			t.Chdir(t.TempDir())
			t.Setenv("DATABASE_URL", "")
			t.Setenv("JWT_SECRET", "test-jwt-secret")
			for i, key := range keys {
				t.Setenv(key, tc.values[i])
			}
			cfg, err := LoadConfig()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected server timeout configuration error, got %+v", cfg)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := [5]int{cfg.ServerReadHeaderTimeout, cfg.ServerReadTimeout, cfg.ServerWriteTimeout, cfg.ServerIdleTimeout, cfg.ServerShutdownTimeout}
			if got != tc.want {
				t.Fatalf("timeouts=%v, want %v", got, tc.want)
			}
		})
	}
}

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestReadinessAndLiveness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := gin.New()
	r.GET("/health", healthHandler("identity-service"))
	r.GET("/ready", readinessHandler(db, nil))
	for _, tc := range []struct {
		path   string
		down   bool
		code   int
		detail string
	}{
		{"/ready", false, 200, `"redis":"degraded"`},
		{"/ready", true, 503, `"database":"unavailable"`},
		{"/health", true, 200, `"status":"healthy"`},
	} {
		if tc.path == "/ready" {
			if tc.down {
				mock.ExpectPing().WillReturnError(http.ErrServerClosed)
			} else {
				mock.ExpectPing()
			}
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.detail) {
			t.Fatalf("response: %d %s", w.Code, w.Body.String())
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}

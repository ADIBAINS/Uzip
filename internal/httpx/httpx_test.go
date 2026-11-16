package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeSingleValue(t *testing.T) {
	for _, tc := range []struct {
		body  string
		valid bool
	}{{`{"name":"ok"}`, true}, {`{"name":"ok"} {"name":"extra"}`, false}, {`{"unknown":1}`, false}, {`{"name":`, false}, {`null {}`, false}} {
		t.Run(tc.body, func(t *testing.T) {
			var input struct {
				Name string `json:"name"`
			}
			w := httptest.NewRecorder()
			if got := Decode(w, httptest.NewRequest("POST", "/", strings.NewReader(tc.body)), &input); got != tc.valid {
				t.Fatalf("Decode=%v, want %v", got, tc.valid)
			}
		})
	}
}

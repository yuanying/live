package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandler(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	want := map[int]bool{
		http.StatusNoContent:       true,
		http.StatusGone:            true,
		http.StatusNotImplemented:  true,
		http.StatusPaymentRequired: true,
	}
	seen := map[int]bool{}

	// The status is random, so ask often enough to see every one of them.
	for i := 0; i < 200; i++ {
		res, err := http.Get(srv.URL + "/foo")
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}

		if !want[res.StatusCode] {
			t.Fatalf("status = %d, want one of 204, 410, 501, 402", res.StatusCode)
		}
		seen[res.StatusCode] = true

		if got := res.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}

		// A 204 has no body, so only the others carry the JSON.
		if res.StatusCode == http.StatusNoContent {
			if len(body) != 0 {
				t.Errorf("204 body = %q, want empty", body)
			}
			continue
		}
		var got map[string]string
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("body %q is not a JSON object of strings: %v", body, err)
		}
		if len(got) != 1 || got["foo"] != http.StatusText(res.StatusCode) {
			t.Errorf("body = %v, want {\"foo\": %q}", got, http.StatusText(res.StatusCode))
		}
	}

	for code := range want {
		if !seen[code] {
			t.Errorf("status %d never returned in 200 requests", code)
		}
	}
}

package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func respWith(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestServerError(t *testing.T) {
	if got := serverError(respWith(404, `{"error":"deploy not found"}`)); got != "deploy not found (404)" {
		t.Errorf("json error = %q", got)
	}
	if got := serverError(respWith(500, "boom")); got != "boom (500)" {
		t.Errorf("plain error = %q", got)
	}
}

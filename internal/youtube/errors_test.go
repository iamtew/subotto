package youtube

import (
	"strings"
	"testing"

	"google.golang.org/api/googleapi"
)

func TestWrapAPIErrorQuota(t *testing.T) {
	err := wrapAPIError(&googleapi.Error{
		Code:   403,
		Errors: []googleapi.ErrorItem{{Reason: "quotaExceeded"}},
	}, "PLtest", "dQw4w9WgXcQ")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("expected quota message, got: %v", err)
	}
}

func TestWrapAPIErrorAuth(t *testing.T) {
	err := wrapAPIError(&googleapi.Error{Code: 401}, "", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "auth-youtube") {
		t.Fatalf("expected auth-youtube hint, got: %v", err)
	}
}

func TestIsRetryableYouTube(t *testing.T) {
	if !isRetryableYouTube(&googleapi.Error{Code: 429}) {
		t.Fatal("429 should retry")
	}
	if !isRetryableYouTube(&googleapi.Error{Code: 503}) {
		t.Fatal("503 should retry")
	}
	if isRetryableYouTube(&googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "quotaExceeded"}}}) {
		t.Fatal("quota should not retry")
	}
	if isRetryableYouTube(&googleapi.Error{Code: 404}) {
		t.Fatal("404 should not retry")
	}
}

package tools

import "testing"

func TestValidatePublicHTTPSURL(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{url: "https://example.com/path"},
		{url: "http://example.com", wantErr: true},
		{url: "file:///etc/passwd", wantErr: true},
		{url: "https://localhost/admin", wantErr: true},
		{url: "https://service.localhost/admin", wantErr: true},
		{url: "https://127.0.0.1/admin", wantErr: true},
		{url: "https://10.0.0.1/admin", wantErr: true},
		{url: "https://169.254.169.254/latest/meta-data", wantErr: true},
		{url: "https://[::1]/admin", wantErr: true},
		{url: "https://user:pass@example.com", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			err := validatePublicHTTPSURL(test.url)
			if (err != nil) != test.wantErr {
				t.Fatalf("validatePublicHTTPSURL(%q) error = %v, wantErr=%v", test.url, err, test.wantErr)
			}
		})
	}
}

func TestSanitizeUntrustedTextNormalizesAndRemovesInvisibleCharacters(t *testing.T) {
	input := "ｓａｆｅ\u200b\u202einstruction\ue000"
	got := SanitizeUntrustedText(input)
	if got != "safeinstruction" {
		t.Fatalf("sanitized text = %q", got)
	}
}

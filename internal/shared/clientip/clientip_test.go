package clientip

import (
	"net/http/httptest"
	"testing"
)

func TestFromRequest(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		header     string
		hops       int
		want       string
	}{
		{
			name:       "direct peer",
			remoteAddr: "203.0.113.10:54321",
			hops:       0,
			want:       "203.0.113.10",
		},
		{
			name:       "ipv6 direct peer",
			remoteAddr: "[2001:db8::1]:54321",
			hops:       0,
			want:       "2001:db8::1",
		},
		{
			name:       "ignores spoofed header without trusted proxy",
			remoteAddr: "203.0.113.10:54321",
			header:     "198.51.100.99",
			hops:       0,
			want:       "203.0.113.10",
		},
		{
			name:       "one trusted proxy uses the appended address",
			remoteAddr: "10.0.0.1:1234",
			header:     "198.51.100.99, 203.0.113.10",
			hops:       1,
			want:       "203.0.113.10",
		},
		{
			name:       "two trusted proxies",
			remoteAddr: "10.0.0.1:1234",
			header:     "203.0.113.10, 10.0.0.5, 10.0.0.6",
			hops:       2,
			want:       "10.0.0.5",
		},
		{
			name:       "invalid forwarded value falls back",
			remoteAddr: "203.0.113.10:54321",
			header:     "not-an-ip",
			hops:       1,
			want:       "203.0.113.10",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/", nil)
			request.RemoteAddr = test.remoteAddr
			if test.header != "" {
				request.Header.Set("X-Forwarded-For", test.header)
			}

			if got := FromRequest(request, test.hops); got != test.want {
				t.Errorf("FromRequest = %q, want %q", got, test.want)
			}
		})
	}
}

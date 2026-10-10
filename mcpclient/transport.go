package mcpclient

import "net/http"

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	cloned := request.Clone(request.Context())
	cloned.Header = request.Header.Clone()

	for key, value := range t.headers {
		cloned.Header.Set(key, value)
	}

	return base.RoundTrip(cloned)
}

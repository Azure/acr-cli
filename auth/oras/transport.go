package oras

import (
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"
)

// Transport is an http.RoundTripper that keeps track of the in-flight
// request and add hooks to report HTTP tracing events.
type Transport struct {
	http.RoundTripper
}

func NewTransport(base http.RoundTripper) *Transport {
	return &Transport{base}
}

// RoundTrip calls base roundtrip while keeping track of the current request.
func (t *Transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	logrus.Debugf(" Request URL: %q", req.URL)
	logrus.Debugf(" Request method: %q", req.Method)
	logrus.Debugf(" Request headers:")
	logHeader(req.Header)

	resp, err = t.RoundTripper.RoundTrip(req)
	if err != nil {
		logrus.Errorf("Error in getting response: %w", err)
	} else if resp == nil {
		logrus.Errorf("No response obtained for request %s %s", req.Method, req.URL)
	} else {
		logrus.Debugf(" Response Status: %q", resp.Status)
		logrus.Debugf(" Response headers:")
		logHeader(resp.Header)
	}
	return resp, err
}

// logHeader prints out the provided header keys and values, with auth header
// scrubbed.
func logHeader(header http.Header) {
	if len(header) > 0 {
		for k, v := range header {
			if strings.EqualFold(k, "Authorization") {
				v = []string{"*****"}
			}
			logrus.Debugf("   %q: %q", k, strings.Join(v, ", "))
		}
	} else {
		logrus.Debugf("   Empty header")
	}
}

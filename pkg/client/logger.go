package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// Logger is an interface representing the Logger struct
type Logger interface {
	RequestPrintf(format string, args ...interface{})
	ResponsePrintf(format string, args ...interface{})
}

type logger struct {
	RequestID string
}

func (lg logger) RequestPrintf(format string, args ...interface{}) {
	for _, v := range strings.Split(fmt.Sprintf(format, args...), "\n") {
		log.Printf("-> %s", v)
	}
}

func (lg logger) ResponsePrintf(format string, args ...interface{}) {
	for _, v := range strings.Split(fmt.Sprintf(format, args...), "\n") {
		log.Printf("<- %s", v)
	}
}

// noopLogger is a default noop logger satisfies the Logger interface
type noopLogger struct{}

// Printf is a default noop method
func (noopLogger) RequestPrintf(format string, args ...interface{}) {}

// Printf is a default noop method
func (noopLogger) ResponsePrintf(format string, args ...interface{}) {}

// RoundTripper satisfies the http.RoundTripper interface and is used to
// customize the default http client RoundTripper
type RoundTripper struct {
	// Default http.RoundTripper
	Rt http.RoundTripper
	// If Logger is not nil, then RoundTrip method will debug the JSON
	// requests and responses
	Logger Logger
}

var sensitiveHeaders = map[string]struct{}{
	"authorization":           {},
	"proxy-authorization":     {},
	"cookie":                  {},
	"set-cookie":              {},
	"x-access-session-token":  {},
	"x-access-session-id":     {},
	"x-csrf-token":            {},
	"x-xsrf-token":            {},
	"x-auth-token":            {},
	"authentication":          {},
	"www-authenticate":        {},
	"x-authorization":         {},
	"sec-websocket-protocol":  {},
	"x-api-key":               {},
	"x-amz-security-token":    {},
	"x-goog-authuser":         {},
	"x-forwarded-client-cert": {},
}

var sensitiveKeys = map[string]struct{}{
	"password":      {},
	"passwd":        {},
	"pass":          {},
	"token":         {},
	"session":       {},
	"sessionid":     {},
	"session_id":    {},
	"mrhsession":    {},
	"signature":     {},
	"auth":          {},
	"authorization": {},
}

// formatHeaders converts standard http.Header type to a string with separated headers.
// Sensitive headers are redacted.
func (rt *RoundTripper) formatHeaders(headers http.Header, separator string) string {
	result := make([]string, 0, len(headers))

	for header, data := range headers {
		if _, ok := sensitiveHeaders[strings.ToLower(header)]; ok {
			result = append(result, fmt.Sprintf("%s: %s", header, "[REDACTED]"))
			continue
		}
		result = append(result, fmt.Sprintf("%s: %s", header, strings.Join(data, " ")))
	}

	return strings.Join(result, separator)
}

// RoundTrip performs a round-trip HTTP request and logs relevant information about it.
func (rt *RoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	defer func() {
		if request.Body != nil {
			request.Body.Close()
		}
	}()

	var err error

	if rt.Logger != nil {
		rt.log().RequestPrintf("URL: %s %s", request.Method, request.URL)
		rt.log().RequestPrintf("Headers:\n%s", rt.formatHeaders(request.Header, "\n"))

		if request.Body != nil {
			request.Body, err = rt.logRequest(request.Body, request.Header.Get("Content-Type"))
			if err != nil {
				return nil, err
			}
		}
	}

	// this is concurrency safe
	ort := rt.Rt
	if ort == nil {
		return nil, fmt.Errorf("rt RoundTripper is nil, aborting")
	}
	response, err := ort.RoundTrip(request)

	if response == nil {
		if rt.Logger != nil {
			rt.log().ResponsePrintf("Connection error, retries exhausted. Aborting")
		}
		err = fmt.Errorf("connection error, retries exhausted. Aborting. Last error was: %s", err)
		return nil, err
	}

	if rt.Logger != nil {
		rt.log().ResponsePrintf("Code: %d", response.StatusCode)
		rt.log().ResponsePrintf("Headers:\n%s", rt.formatHeaders(response.Header, "\n"))

		response.Body, err = rt.logResponse(response.Body, response.Header.Get("Content-Type"))
	}

	return response, err
}

// logRequest will log the HTTP Request details.
// If the body is JSON, it will attempt to be pretty-formatted.
func (rt *RoundTripper) logRequest(original io.ReadCloser, contentType string) (io.ReadCloser, error) {
	var bs bytes.Buffer
	defer original.Close()

	if _, err := io.Copy(&bs, original); err != nil {
		return nil, err
	}

	rt.log().RequestPrintf("Body: %s", redactBody(contentType, bs.String()))

	return ioutil.NopCloser(bytes.NewReader(bs.Bytes())), nil
}

// logResponse will log the HTTP Response details.
// If the body is JSON, it will attempt to be pretty-formatted.
func (rt *RoundTripper) logResponse(original io.ReadCloser, contentType string) (io.ReadCloser, error) {
	var bs bytes.Buffer
	defer original.Close()

	if _, err := io.Copy(&bs, original); err != nil {
		return nil, err
	}

	rt.log().ResponsePrintf("Body: %s", redactBody(contentType, bs.String()))

	return ioutil.NopCloser(bytes.NewReader(bs.Bytes())), nil
}

func (rt *RoundTripper) log() Logger {
	// this is concurrency safe
	l := rt.Logger
	if l == nil {
		// noop is used, when logger pointer has been set to nil
		return &noopLogger{}
	}
	return l
}

func redactBody(contentType, body string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "application/x-www-form-urlencoded"):
		if v, err := url.ParseQuery(body); err == nil {
			for k := range v {
				if _, ok := sensitiveKeys[strings.ToLower(k)]; ok {
					v.Set(k, "[REDACTED]")
				}
			}
			return v.Encode()
		}
	case strings.Contains(ct, "application/json"):
		var payload interface{}
		if err := json.Unmarshal([]byte(body), &payload); err == nil {
			redactJSON(payload)
			if b, err := json.Marshal(payload); err == nil {
				return string(b)
			}
		}
	}
	return body
}

func redactJSON(v interface{}) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, v := range t {
			if _, ok := sensitiveKeys[strings.ToLower(k)]; ok {
				t[k] = "[REDACTED]"
				continue
			}
			redactJSON(v)
		}
	case []interface{}:
		for i := range t {
			redactJSON(t[i])
		}
	}
}

package baton

import (
	"net/http"
	"net/url"
	"strings"
)

const baggageHeader = "Baggage"

// baggageValue returns the value of member key in the W3C baggage headers of
// h, percent-decoded, or "" if it is absent or malformed.
func baggageValue(h http.Header, key string) string {
	for _, line := range h.Values(baggageHeader) {
		for _, m := range strings.Split(line, ",") {
			m, _, _ = strings.Cut(m, ";") // drop member properties
			k, v, ok := strings.Cut(m, "=")
			if !ok || strings.TrimSpace(k) != key {
				continue
			}
			v, err := url.PathUnescape(strings.TrimSpace(v))
			if err != nil {
				return ""
			}
			return v
		}
	}
	return ""
}

// hasBaggageMember reports whether any baggage header in h has member key.
func hasBaggageMember(h http.Header, key string) bool {
	for _, line := range h.Values(baggageHeader) {
		for _, m := range strings.Split(line, ",") {
			m, _, _ = strings.Cut(m, ";")
			if k, _, ok := strings.Cut(m, "="); ok && strings.TrimSpace(k) == key {
				return true
			}
		}
	}
	return false
}

// setBaggage adds key=id to h's baggage unless a member named key is already
// there. id is always valid(), and every character valid() allows is a legal
// baggage-octet, so no encoding is needed.
func setBaggage(h http.Header, key, id string) {
	if hasBaggageMember(h, key) {
		return
	}
	member := key + "=" + id
	vs := h.Values(baggageHeader)
	if len(vs) == 0 {
		h.Set(baggageHeader, member)
		return
	}
	h.Set(baggageHeader, strings.Join(append(append([]string(nil), vs...), member), ","))
}

package parse

import "strings"

// Claude Code records a session's cost only when the session ends, so a
// running session's cost is estimated from its tokens at these prices:
// Anthropic's API rates in US dollars per million tokens, as of
// 2026-10-06. A recorded cost always replaces the estimate.

type price struct {
	input, output float64
	cacheRead     float64 // a fraction of input
}

// Cache writes cost 1.25 times input for the 5-minute cache, 2 times for
// the 1-hour cache.
const (
	write5m = 1.25
	write1h = 2.0
)

// prices are matched by prefix, longest first, so a suffix such as [1m]
// or a date doesn't matter.
var prices = []struct {
	prefix string
	p      price
}{
	{"claude-fable-5-1", price{10, 50, 0.025}},
	{"claude-mythos-5-1", price{10, 50, 0.025}},
	{"claude-fable-5", price{10, 50, 0.1}},
	{"claude-opus-5-5", price{4, 20, 0.05}},
	{"claude-opus-5", price{5, 25, 0.1}},
	{"claude-opus-4-8", price{5, 25, 0.1}},
	{"claude-opus-4-7", price{5, 25, 0.1}},
	{"claude-opus-4-6", price{5, 25, 0.1}},
	{"claude-sonnet-5-5", price{2, 10, 0.1}},
	{"claude-sonnet-5", price{2, 10, 0.1}},
	{"claude-sonnet-4-6", price{3, 15, 0.1}},
	{"claude-haiku-5-5", price{0.10, 0.50, 0.1}},
	{"claude-haiku-4-5", price{1, 5, 0.1}},
}

func priceOf(model string) (price, bool) {
	for _, e := range prices {
		if strings.HasPrefix(model, e.prefix) {
			return e.p, true
		}
	}
	return price{}, false
}

// usd is what one request's usage costs at p.
func (u Usage) usd(p price) float64 {
	write5 := u.CacheCreate - u.CacheCreate1h
	return (float64(u.Input)*p.input + float64(u.Output)*p.output +
		float64(u.CacheRead)*p.input*p.cacheRead +
		float64(write5)*p.input*write5m + float64(u.CacheCreate1h)*p.input*write1h) / 1e6
}

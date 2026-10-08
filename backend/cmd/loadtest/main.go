// Command loadtest drives realistic read traffic (and design evaluations) against a running API and
// reports latency percentiles and errors per endpoint. It never creates orders, payments or
// requests, so it is safe to point at a staging copy of production.
//
//	go run ./cmd/loadtest -base https://staging-api.example -c 20 -d 60s -p95 400ms
//
// The API rate limits each client address. Run it against an environment started with a higher
// RATE_LIMIT_MULTIPLIER (for example 100), or 429 answers are counted as "limited" and skew results.
// It exits with status 1 when any endpoint's p95 is above -p95 or more than -max-errors of requests fail.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type target struct {
	name   string
	weight int
	build  func() (*http.Request, error)
}

type sample struct {
	name    string
	took    time.Duration
	status  int
	failure bool
}

func main() {
	base := flag.String("base", "http://localhost:8080", "API origin")
	conc := flag.Int("c", 10, "concurrent virtual users")
	dur := flag.Duration("d", 30*time.Second, "test duration")
	think := flag.Duration("think", 200*time.Millisecond, "pause between a user's requests")
	p95 := flag.Duration("p95", 500*time.Millisecond, "fail if any endpoint's p95 is above this")
	maxErr := flag.Float64("max-errors", 0.01, "fail if more than this share of requests fail")
	flag.Parse()
	api := strings.TrimRight(*base, "/") + "/api/v1"
	client := &http.Client{Timeout: 15 * time.Second}

	// Discover real catalogue data so requests hit existing records.
	var products struct {
		Items []struct {
			Slug string `json:"slug"`
		} `json:"items"`
	}
	var garments []struct {
		Key           string `json:"key"`
		StudioEnabled bool   `json:"studioEnabled"`
	}
	must(getJSON(client, api+"/products?limit=50", &products))
	must(getJSON(client, api+"/garments", &garments))
	var studio []string
	for _, g := range garments {
		if g.StudioEnabled {
			studio = append(studio, g.Key)
		}
	}
	if len(products.Items) == 0 || len(studio) == 0 {
		fmt.Fprintln(os.Stderr, "loadtest: the API has no products or studio garments; seed it first")
		os.Exit(2)
	}
	pick := func(xs []string) string { return xs[rand.IntN(len(xs))] }
	slugs := make([]string, len(products.Items))
	for i, p := range products.Items {
		slugs[i] = p.Slug
	}
	get := func(path func() string) func() (*http.Request, error) {
		return func() (*http.Request, error) { return http.NewRequest(http.MethodGet, api+path(), nil) }
	}
	targets := []target{
		{"config", 10, get(func() string { return "/config" })},
		{"products list", 20, get(func() string { return "/products?limit=24" })},
		{"products search", 5, get(func() string { return "/products?q=shirt&sort=price_asc" })},
		{"product", 20, get(func() string { return "/products/" + pick(slugs) })},
		{"fabrics", 8, get(func() string { return "/fabrics" })},
		{"portfolio", 5, get(func() string { return "/portfolio" })},
		{"studio config", 10, get(func() string { return "/garments/" + pick(studio) + "/studio" })},
		{"slots", 5, get(func() string { return "/appointments/slots?type=consultation" })},
		{"design evaluate", 17, func() (*http.Request, error) {
			body, _ := json.Marshal(map[string]any{"garment": pick(studio), "selections": map[string]any{}, "fitPreference": "regular"})
			r, err := http.NewRequest(http.MethodPost, api+"/designs/evaluate", bytes.NewReader(body))
			if err == nil {
				r.Header.Set("Content-Type", "application/json")
			}
			return r, err
		}},
	}
	total := 0
	for _, t := range targets {
		total += t.weight
	}
	choose := func() target {
		n := rand.IntN(total)
		for _, t := range targets {
			if n < t.weight {
				return t
			}
			n -= t.weight
		}
		return targets[0]
	}

	ctx, cancel := context.WithTimeout(context.Background(), *dur)
	defer cancel()
	results := make(chan sample, 1024)
	var wg sync.WaitGroup
	for i := 0; i < *conc; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				t := choose()
				req, err := t.build()
				if err != nil {
					continue
				}
				start := time.Now()
				resp, err := client.Do(req.WithContext(ctx))
				s := sample{name: t.name, took: time.Since(start)}
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					s.failure = true
				} else {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					s.status = resp.StatusCode
					s.failure = resp.StatusCode >= 500 || (resp.StatusCode >= 400 && resp.StatusCode != http.StatusTooManyRequests)
				}
				results <- s
				time.Sleep(*think)
			}
		}()
	}
	go func() { wg.Wait(); close(results) }()

	type agg struct {
		took            []time.Duration
		errors, limited int
	}
	byName := map[string]*agg{}
	all := &agg{}
	for s := range results {
		a := byName[s.name]
		if a == nil {
			a = &agg{}
			byName[s.name] = a
		}
		for _, x := range []*agg{a, all} {
			x.took = append(x.took, s.took)
			if s.failure {
				x.errors++
			}
			if s.status == http.StatusTooManyRequests {
				x.limited++
			}
		}
	}
	pct := func(d []time.Duration, p float64) time.Duration {
		if len(d) == 0 {
			return 0
		}
		sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
		return d[min(len(d)-1, int(float64(len(d))*p))]
	}
	fmt.Printf("%d users for %s against %s\n\n", *conc, *dur, *base)
	fmt.Printf("%-18s %8s %8s %8s %8s %7s %8s\n", "endpoint", "requests", "p50", "p95", "p99", "errors", "limited")
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)
	failed := false
	for _, n := range names {
		a := byName[n]
		p95v := pct(a.took, 0.95)
		mark := ""
		if p95v > *p95 {
			mark, failed = "  over budget", true
		}
		fmt.Printf("%-18s %8d %8s %8s %8s %7d %8d%s\n", n, len(a.took), pct(a.took, 0.5).Round(time.Millisecond),
			p95v.Round(time.Millisecond), pct(a.took, 0.99).Round(time.Millisecond), a.errors, a.limited, mark)
	}
	rate := float64(len(all.took)) / dur.Seconds()
	errShare := 0.0
	if len(all.took) > 0 {
		errShare = float64(all.errors) / float64(len(all.took))
	}
	fmt.Printf("\n%d requests, %.1f per second, p95 %s, errors %.2f%%, rate limited %d\n", len(all.took), rate,
		pct(all.took, 0.95).Round(time.Millisecond), errShare*100, all.limited)
	if errShare > *maxErr {
		failed = true
	}
	if failed {
		os.Exit(1)
	}
}

func getJSON(c *http.Client, url string, v any) error {
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "loadtest:", err)
		os.Exit(2)
	}
}

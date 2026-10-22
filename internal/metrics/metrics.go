// Package metrics writes Prometheus text-format metrics (exposition format
// 0.0.4) without a client library: a few counters and histograms updated on
// the hot path, and gauges collected at scrape time.
package metrics

import (
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Sample is one value of a metric family.
type Sample struct {
	Labels []string // name, value, name, value…
	Value  float64
}

// Registry holds metric families in registration order.
type Registry struct {
	mu       sync.Mutex
	families []family
}

type family interface {
	write(w io.Writer)
}

func NewRegistry() *Registry { return &Registry{} }

func (r *Registry) add(f family) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.families = append(r.families, f)
}

// Write renders every family.
func (r *Registry) Write(w io.Writer) {
	r.mu.Lock()
	fs := slices.Clone(r.families)
	r.mu.Unlock()
	for _, f := range fs {
		f.write(w)
	}
}

func header(w io.Writer, name, help, typ string) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
}

func labelString(labels []string, extra ...string) string {
	all := append(slices.Clone(labels), extra...)
	if len(all) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteByte('{')
	for i := 0; i+1 < len(all); i += 2 {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(all[i])
		b.WriteString(`="`)
		b.WriteString(escape(all[i+1]))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

var escaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

func escape(s string) string { return escaper.Replace(s) }

func formatFloat(v float64) string {
	switch {
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	case math.IsNaN(v):
		return "NaN"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// Func is a gauge or counter whose samples are computed at scrape time.
type Func struct {
	name, help, typ string
	collect         func() []Sample
}

// GaugeFunc registers a gauge collected at scrape time.
func (r *Registry) GaugeFunc(name, help string, collect func() []Sample) {
	r.add(&Func{name, help, "gauge", collect})
}

// CounterFunc registers a counter whose running total is kept elsewhere.
func (r *Registry) CounterFunc(name, help string, collect func() []Sample) {
	r.add(&Func{name, help, "counter", collect})
}

func (f *Func) write(w io.Writer) {
	samples := f.collect()
	header(w, f.name, f.help, f.typ)
	for _, s := range samples {
		fmt.Fprintf(w, "%s%s %s\n", f.name, labelString(s.Labels), formatFloat(s.Value))
	}
}

// CounterVec is a counter with labels.
type CounterVec struct {
	name, help string
	labels     []string
	mu         sync.Mutex
	values     map[string]float64
	keys       map[string][]string
}

func (r *Registry) Counter(name, help string, labels ...string) *CounterVec {
	c := &CounterVec{name: name, help: help, labels: labels, values: map[string]float64{}, keys: map[string][]string{}}
	r.add(c)
	return c
}

func pairs(names, values []string) []string {
	out := make([]string, 0, 2*len(names))
	for i, n := range names {
		v := ""
		if i < len(values) {
			v = values[i]
		}
		out = append(out, n, v)
	}
	return out
}

// Add adds v for the label values, given in registration order.
func (c *CounterVec) Add(v float64, values ...string) {
	k := strings.Join(values, "\xff")
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.keys[k]; !ok {
		c.keys[k] = pairs(c.labels, values)
	}
	c.values[k] += v
}

func (c *CounterVec) Inc(values ...string) { c.Add(1, values...) }

func (c *CounterVec) write(w io.Writer) {
	c.mu.Lock()
	keys := make([]string, 0, len(c.values))
	for k := range c.values {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	header(w, c.name, c.help, "counter")
	for _, k := range keys {
		fmt.Fprintf(w, "%s%s %s\n", c.name, labelString(c.keys[k]), formatFloat(c.values[k]))
	}
	c.mu.Unlock()
}

// HistogramVec is a histogram with labels.
type HistogramVec struct {
	name, help string
	labels     []string
	buckets    []float64
	mu         sync.Mutex
	series     map[string]*histSeries
}

type histSeries struct {
	labels []string
	counts []uint64 // per bucket, not cumulative
	sum    float64
	count  uint64
}

func (r *Registry) Histogram(name, help string, buckets []float64, labels ...string) *HistogramVec {
	h := &HistogramVec{name: name, help: help, labels: labels, buckets: slices.Sorted(slices.Values(buckets)), series: map[string]*histSeries{}}
	r.add(h)
	return h
}

func (h *HistogramVec) Observe(v float64, values ...string) {
	k := strings.Join(values, "\xff")
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.series[k]
	if s == nil {
		s = &histSeries{labels: pairs(h.labels, values), counts: make([]uint64, len(h.buckets))}
		h.series[k] = s
	}
	if i, _ := slices.BinarySearch(h.buckets, v); i < len(h.buckets) {
		s.counts[i]++
	}
	s.sum += v
	s.count++
}

func (h *HistogramVec) write(w io.Writer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	keys := make([]string, 0, len(h.series))
	for k := range h.series {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	header(w, h.name, h.help, "histogram")
	for _, k := range keys {
		s := h.series[k]
		var cum uint64
		for i, b := range h.buckets {
			cum += s.counts[i]
			fmt.Fprintf(w, "%s_bucket%s %d\n", h.name, labelString(s.labels, "le", formatFloat(b)), cum)
		}
		fmt.Fprintf(w, "%s_bucket%s %d\n", h.name, labelString(s.labels, "le", "+Inf"), s.count)
		fmt.Fprintf(w, "%s_sum%s %s\n", h.name, labelString(s.labels), formatFloat(s.sum))
		fmt.Fprintf(w, "%s_count%s %d\n", h.name, labelString(s.labels), s.count)
	}
}

// DefBuckets suit HTTP request durations in seconds.
var DefBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

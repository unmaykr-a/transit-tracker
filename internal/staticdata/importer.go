package staticdata

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/ingest"
	"github.com/unmaykr-a/transit-tracker/internal/store"
)

// Importer refreshes static data: on startup when missing or older than
// MaxAge, nightly at 03:30 Europe/Tallinn, and on demand.
type Importer struct {
	Fetcher      *feeds.Fetcher
	OTP          *feeds.OTP
	StopsURL     func() string // transport.tallinn.ee/data/stops.txt
	ElronGTFSURL func() string
	AgencyID     string
	Store        *store.Store
	Log          *slog.Logger
	Location     *time.Location
	Now          func() time.Time
	// OnSwap is called after a new index is installed.
	OnSwap func(*Index)

	current      atomic.Pointer[Index]
	savedVersion atomic.Value // build version of the dataset loaded from SQLite
	mu           sync.Mutex   // one import at a time
	status       atomic.Pointer[Status]
}

// Defaults for the upstream static sources.
const (
	DefaultStopsURL     = "https://transport.tallinn.ee/data/stops.txt"
	DefaultElronGTFSURL = "https://eu-gtfs.remix.com/elron.zip"
	MaxAge              = 20 * time.Hour
)

// Status is the importer's health for the status endpoint and admin page.
type Status struct {
	ImportedAt  time.Time `json:"imported_at,omitzero"`
	LastAttempt time.Time `json:"last_attempt,omitzero"`
	LastError   string    `json:"last_error,omitempty"`
	DurationMs  int64     `json:"duration_ms"`
	Stops       int       `json:"stops"`
	Areas       int       `json:"areas"`
	Routes      int       `json:"routes"`
	Patterns    int       `json:"patterns"`
}

func (im *Importer) now() time.Time {
	if im.Now != nil {
		return im.Now()
	}
	return time.Now()
}

// Index returns the current index; never nil (empty before the first import).
func (im *Importer) Index() *Index {
	if ix := im.current.Load(); ix != nil {
		return ix
	}
	return NewIndex(Dataset{})
}

// Status returns the latest import status.
func (im *Importer) Status() Status {
	if s := im.status.Load(); s != nil {
		return *s
	}
	return Status{}
}

func (im *Importer) install(d Dataset) {
	ix := NewIndex(d)
	im.current.Store(ix)
	st := im.Status()
	st.ImportedAt = d.ImportedAt
	st.Stops, st.Areas, st.Routes, st.Patterns = len(d.Stops), len(d.Areas), len(d.Routes), len(d.Patterns)
	im.status.Store(&st)
	if im.OnSwap != nil {
		im.OnSwap(ix)
	}
}

// LoadSaved installs the dataset stored in SQLite, if any.
func (im *Importer) LoadSaved(ctx context.Context) error {
	d, err := Load(ctx, im.Store)
	if err != nil {
		return err
	}
	if len(d.Stops) > 0 {
		im.savedVersion.Store(d.BuildVersion)
		im.install(d)
	}
	return nil
}

// Import fetches all sources, builds, saves and installs a new dataset. On
// failure the previous data stays in place.
func (im *Importer) Import(ctx context.Context) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	start := im.now()
	d, err := im.fetchAndBuild(ctx)
	if err == nil {
		err = Save(ctx, im.Store, d)
	}
	st := im.Status()
	st.LastAttempt, st.DurationMs = start, im.now().Sub(start).Milliseconds()
	if err != nil {
		st.LastError = err.Error()
		im.status.Store(&st)
		return err
	}
	st.LastError = ""
	im.status.Store(&st)
	im.savedVersion.Store(BuildVersion)
	im.install(d)
	im.Log.Info("static data imported", "stops", len(d.Stops), "areas", len(d.Areas), "routes", len(d.Routes), "patterns", len(d.Patterns), "took", time.Duration(st.DurationMs)*time.Millisecond)
	return nil
}

func (im *Importer) fetchAndBuild(ctx context.Context) (Dataset, error) {
	routes, err := im.OTP.AgencyRoutes(ctx, im.AgencyID)
	if err != nil {
		return Dataset{}, fmt.Errorf("city routes: %w", err)
	}
	if len(routes) == 0 {
		return Dataset{}, errors.New("city routes: none returned")
	}
	res, err := im.Fetcher.Get(ctx, im.StopsURL(), false)
	if err != nil {
		return Dataset{}, fmt.Errorf("stops: %w", err)
	}
	stops, _, err := feeds.ParseTallinnStops(bytes.NewReader(res.Body))
	if err != nil {
		return Dataset{}, fmt.Errorf("stops: %w", err)
	}
	src := Sources{CityRoutes: routes, TallinnStops: stops}
	if res, err := im.Fetcher.Get(ctx, im.ElronGTFSURL(), false); err != nil {
		// Trains are optional: keep the city importable when Elron's feed is down.
		im.Log.Warn("Elron GTFS unavailable, importing without rail", "err", err)
	} else if g, err := feeds.ReadGTFSZip(res.Body); err != nil {
		im.Log.Warn("Elron GTFS unreadable, importing without rail", "err", err)
	} else {
		src.Elron = g
	}
	return Build(src, im.now()), nil
}

// needsImport reports whether the saved data is missing, old, or built by another version.
func (im *Importer) needsImport() bool {
	st := im.Status()
	if st.ImportedAt.IsZero() || im.now().Sub(st.ImportedAt) > MaxAge {
		return true
	}
	v, _ := im.savedVersion.Load().(string)
	return v != BuildVersion
}

// NextNightly returns the next 03:30 in loc after t.
func NextNightly(t time.Time, loc *time.Location) time.Time {
	lt := t.In(loc)
	next := time.Date(lt.Year(), lt.Month(), lt.Day(), 3, 30, 0, 0, loc)
	if !next.After(t) {
		next = time.Date(lt.Year(), lt.Month(), lt.Day()+1, 3, 30, 0, 0, loc)
	}
	return next
}

// Run loads saved data, imports when it is missing or stale (retrying with
// backoff), then refreshes nightly until ctx ends.
func (im *Importer) Run(ctx context.Context) {
	if err := im.LoadSaved(ctx); err != nil {
		im.Log.Error("loading saved static data", "err", err)
	}
	importWithRetry := func() {
		for attempt := 1; ; attempt++ {
			err := im.Import(ctx)
			if err == nil || ctx.Err() != nil {
				return
			}
			delay := ingest.Backoff(time.Minute, time.Hour, attempt)
			im.Log.Warn("static import failed", "err", err, "attempt", attempt, "retry_in", delay.Round(time.Second))
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
	}
	if im.needsImport() {
		importWithRetry()
	}
	for {
		wait := time.Until(NextNightly(im.now(), im.Location))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
			importWithRetry()
		}
	}
}

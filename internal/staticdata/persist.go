package staticdata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/geo"
	"github.com/unmaykr-a/transit-tracker/internal/store"
	"github.com/unmaykr-a/transit-tracker/internal/store/db"
)

const (
	metaImportedAt   = "static.imported_at"
	metaBuildVersion = "static.build_version"
)

// BuildVersion changes whenever Build's output for the same sources changes
// (naming, grouping, ids). A saved dataset with another version is re-imported
// at startup instead of waiting for the nightly refresh.
const BuildVersion = "2"

func joinModes(ms []feeds.Mode) string {
	parts := make([]string, len(ms))
	for i, m := range ms {
		parts[i] = string(m)
	}
	return strings.Join(parts, ",")
}

func splitModes(s string) []feeds.Mode {
	var out []feeds.Mode
	for _, p := range strings.Split(s, ",") {
		if p != "" {
			out = append(out, feeds.Mode(p))
		}
	}
	return out
}

// Save replaces all static data in one transaction.
func Save(ctx context.Context, st *store.Store, d Dataset) error {
	return st.Tx(ctx, func(q *db.Queries) error {
		for _, del := range []func(context.Context) error{q.DeleteAllPatterns, q.DeleteAllRoutes, q.DeleteAllStops, q.DeleteAllStopAreas} {
			if err := del(ctx); err != nil {
				return err
			}
		}
		for _, s := range d.Stops {
			if err := q.InsertStop(ctx, db.InsertStopParams{
				ID: s.ID, Code: s.Code, SiriID: s.SiriID, Name: s.Name, Platform: s.Platform, Street: s.Street,
				Lat: s.Lat, Lon: s.Lon, Modes: joinModes(s.Modes), AreaID: s.AreaID, Aliases: strings.Join(s.Aliases, ";"), Source: s.Source,
			}); err != nil {
				return fmt.Errorf("stop %s: %w", s.ID, err)
			}
		}
		for _, a := range d.Areas {
			if err := q.InsertStopArea(ctx, db.InsertStopAreaParams{ID: a.ID, Name: a.Name, Lat: a.Lat, Lon: a.Lon, Modes: joinModes(a.Modes)}); err != nil {
				return fmt.Errorf("area %s: %w", a.ID, err)
			}
		}
		for _, r := range d.Routes {
			if err := q.InsertRoute(ctx, db.InsertRouteParams{
				ID: r.ID, Mode: string(r.Mode), ShortName: r.ShortName, LongName: r.LongName,
				Color: r.Color, TextColor: r.TextColor, Source: r.Source, Sort: int64(r.Sort),
			}); err != nil {
				return fmt.Errorf("route %s: %w", r.ID, err)
			}
		}
		for _, p := range d.Patterns {
			ids, _ := json.Marshal(p.StopIDs)
			if err := q.InsertPattern(ctx, db.InsertPatternParams{
				ID: p.ID, RouteID: p.RouteID, Direction: int64(p.Direction), Headsign: p.Headsign,
				StopIds: string(ids), Shape: geo.EncodePolyline(p.Shape), Trips: int64(p.Trips), Sort: int64(p.Sort),
			}); err != nil {
				return fmt.Errorf("pattern %s: %w", p.ID, err)
			}
		}
		if err := q.SetMeta(ctx, db.SetMetaParams{Key: metaBuildVersion, Value: BuildVersion}); err != nil {
			return err
		}
		return q.SetMeta(ctx, db.SetMetaParams{Key: metaImportedAt, Value: d.ImportedAt.UTC().Format(time.RFC3339)})
	})
}

// Load reads the last saved dataset. An empty database returns an empty dataset and no error.
func Load(ctx context.Context, st *store.Store) (Dataset, error) {
	var d Dataset
	if v, err := st.Q.GetMeta(ctx, metaImportedAt); err == nil {
		d.ImportedAt, _ = time.Parse(time.RFC3339, v)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return d, err
	}
	if v, err := st.Q.GetMeta(ctx, metaBuildVersion); err == nil {
		d.BuildVersion = v
	} else if !errors.Is(err, sql.ErrNoRows) {
		return d, err
	}
	stops, err := st.Q.ListStops(ctx)
	if err != nil {
		return d, err
	}
	areaStops := map[string][]string{}
	for _, s := range stops {
		var aliases []string
		if s.Aliases != "" {
			aliases = strings.Split(s.Aliases, ";")
		}
		d.Stops = append(d.Stops, Stop{ID: s.ID, Code: s.Code, SiriID: s.SiriID, Name: s.Name, Platform: s.Platform, Street: s.Street,
			Lat: s.Lat, Lon: s.Lon, Modes: splitModes(s.Modes), AreaID: s.AreaID, Aliases: aliases, Source: s.Source})
		areaStops[s.AreaID] = append(areaStops[s.AreaID], s.ID)
	}
	areas, err := st.Q.ListStopAreas(ctx)
	if err != nil {
		return d, err
	}
	for _, a := range areas {
		d.Areas = append(d.Areas, Area{ID: a.ID, Name: a.Name, Lat: a.Lat, Lon: a.Lon, Modes: splitModes(a.Modes), StopIDs: areaStops[a.ID]})
	}
	routes, err := st.Q.ListRoutes(ctx)
	if err != nil {
		return d, err
	}
	for _, r := range routes {
		d.Routes = append(d.Routes, Route{ID: r.ID, Mode: feeds.Mode(r.Mode), ShortName: r.ShortName, LongName: r.LongName,
			Color: r.Color, TextColor: r.TextColor, Source: r.Source, Sort: int(r.Sort)})
	}
	patterns, err := st.Q.ListPatterns(ctx)
	if err != nil {
		return d, err
	}
	for _, p := range patterns {
		var ids []string
		if err := json.Unmarshal([]byte(p.StopIds), &ids); err != nil {
			return d, fmt.Errorf("pattern %s: %w", p.ID, err)
		}
		shape, err := geo.DecodePolyline(p.Shape)
		if err != nil {
			return d, fmt.Errorf("pattern %s shape: %w", p.ID, err)
		}
		d.Patterns = append(d.Patterns, Pattern{ID: p.ID, RouteID: p.RouteID, Direction: int(p.Direction), Headsign: p.Headsign,
			StopIDs: ids, Shape: shape, Trips: int(p.Trips), Sort: int(p.Sort)})
	}
	return d, nil
}

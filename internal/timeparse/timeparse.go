// Package timeparse parses timestamp/date values that arrive over JSON APIs.
// pgtype.Timestamptz.Scan and pgtype.Date.Scan only understand Postgres text
// format ("2006-01-02 15:04:05+00" / "2006-01-02"), which rejects the RFC 3339
// timestamps JSON clients actually send. These helpers accept both.
package timeparse

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ParseTimestamptz parses a JSON timestamp value into a timestamptz. RFC 3339
// is tried first (what JSON clients send), with pgtype.Timestamptz.Scan as a
// fallback for callers that still send Postgres text format.
func ParseTimestamptz(s string) (pgtype.Timestamptz, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return pgtype.Timestamptz{Time: t, Valid: true}, nil
	}
	var ts pgtype.Timestamptz
	if err := ts.Scan(s); err != nil {
		return pgtype.Timestamptz{}, err
	}
	return ts, nil
}

// ParseDate parses a JSON date value into a pgtype.Date. pgtype.Date.Scan is
// tried first (native "2006-01-02" and other Postgres date formats, preserving
// existing accepted inputs), with an RFC 3339 fallback that keeps the calendar
// date in the sender's own offset so "+05:00" inputs don't shift the day.
func ParseDate(s string) (pgtype.Date, error) {
	var d pgtype.Date
	scanErr := d.Scan(s)
	if scanErr == nil {
		return d, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return pgtype.Date{
			Time:  time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC),
			Valid: true,
		}, nil
	}
	return pgtype.Date{}, scanErr
}

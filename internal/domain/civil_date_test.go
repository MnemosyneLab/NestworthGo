package domain

import (
	"testing"
	"time"
)

func TestResolveCivilDateStartUsesFirstActualInstant(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, date, zone, want string
	}{
		{name: "ordinary Shanghai midnight", date: "2024-09-08", zone: "Asia/Shanghai", want: "2024-09-07T16:00:00Z"},
		{name: "Santiago skips midnight", date: "2024-09-08", zone: "America/Santiago", want: "2024-09-08T04:00:00Z"},
		{name: "Havana skips midnight", date: "2026-03-08", zone: "America/Havana", want: "2026-03-08T05:00:00Z"},
		{name: "Havana repeated midnight chooses first occurrence", date: "2026-11-01", zone: "America/Havana", want: "2026-11-01T04:00:00Z"},
	} {
		t.Run(test.name, func(t *testing.T) {
			location, err := time.LoadLocation(test.zone)
			if err != nil {
				t.Fatal(err)
			}
			day, err := time.Parse(time.DateOnly, test.date)
			if err != nil {
				t.Fatal(err)
			}
			want, err := time.Parse(time.RFC3339, test.want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ResolveCivilDateStart(day, location)
			if err != nil || !got.Equal(want) {
				t.Fatalf("civil start=%s err=%v, want %s", got.Format(time.RFC3339), err, test.want)
			}
			if got.In(location).Format(time.DateOnly) != test.date {
				t.Fatalf("start %s has local label %s, want %s", got, got.In(location).Format(time.DateOnly), test.date)
			}
		})
	}
}

func TestResolveCivilDateStartAndCutoffRejectEntirelySkippedDate(t *testing.T) {
	t.Parallel()
	location, err := time.LoadLocation("Pacific/Apia")
	if err != nil {
		t.Fatal(err)
	}
	day, err := time.Parse(time.DateOnly, "2011-12-30")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveCivilDateStart(day, location); err == nil {
		t.Fatal("resolved a civil date that did not exist in Apia")
	}
	if _, err := ResolveCivilDateCutoff(day, location); err == nil {
		t.Fatal("resolved a cutoff for a civil date that did not exist in Apia")
	}
}

package dash

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/duration"
)

func TestParseAgeRoundTripsKubectlDurations(t *testing.T) {
	t.Parallel()

	for _, d := range []time.Duration{
		0, 45 * time.Second, 2*time.Minute + 30*time.Second, 9 * time.Minute, 22 * time.Minute,
		2*time.Hour + 15*time.Minute, 5 * time.Hour, 30 * time.Hour, 5*24*time.Hour + 4*time.Hour,
		212 * 24 * time.Hour, 3*365*24*time.Hour + 45*24*time.Hour, 9 * 365 * 24 * time.Hour,
	} {
		text := duration.HumanDuration(d)
		got, ok := parseAge(text)
		if !ok {
			t.Fatalf("parseAge(%q) failed", text)
		}
		// HumanDuration drops precision as it grows; the parse must land
		// within the unit it kept.
		if got > d || d-got >= unitOf(text) {
			t.Errorf("parseAge(%q) = %v, want about %v", text, got, d)
		}
	}
}

func unitOf(text string) time.Duration {
	switch text[len(text)-1] {
	case 's':
		return time.Second
	case 'm':
		return time.Minute
	case 'h':
		return time.Hour
	case 'd':
		return 24 * time.Hour
	default:
		return 365 * 24 * time.Hour
	}
}

func TestParseAgeRejectsGarbage(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"", "<invalid>", "<unknown>", "5", "m", "5x", "-3m", "3m5"} {
		if d, ok := parseAge(s); ok {
			t.Errorf("parseAge(%q) = %v, want failure", s, d)
		}
	}
}

// TestParseAgeRejectsOverflow: a cell past what a Duration holds is not
// an age. Read as one it would wrap to a negative age, a pod created in
// the future.
func TestParseAgeRejectsOverflow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, in string
	}{
		{name: "years past int64", in: "9999999999y"},
		{name: "seconds past int64", in: "9300000000000000000s"},
		{name: "days past int64", in: "106752d"},
		{name: "a sum past int64", in: "292y292y"},
		{name: "a sum past int64 in two units", in: "292y106000d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if d, ok := parseAge(tc.in); ok {
				t.Fatalf("parseAge(%q) = %v, want failure", tc.in, d)
			}
		})
	}
}

func TestParseAgeKeepsTheLargestAge(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want time.Duration
	}{
		{in: "292y", want: 292 * 365 * 24 * time.Hour},
		{in: "106751d", want: 106751 * 24 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if d, ok := parseAge(tc.in); !ok || d != tc.want {
				t.Fatalf("parseAge(%q) = %v, %v; want %v", tc.in, d, ok, tc.want)
			}
		})
	}
}

func TestParseRestarts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in     string
		count  int
		ago    time.Duration
		hasAgo bool
		ok     bool
	}{
		{"0", 0, 0, false, true},
		{"41 (2m ago)", 41, 2 * time.Minute, true, true},
		{"7 (1d ago)", 7, 24 * time.Hour, true, true},
		{"12 (45s ago)", 12, 45 * time.Second, true, true},
		{"3 (garbage)", 3, 0, false, true},
		{"", 0, 0, false, false},
		{"many", 0, 0, false, false},
	}
	for _, tc := range cases {
		count, ago, hasAgo, ok := parseRestarts(tc.in)
		if count != tc.count || ago != tc.ago || hasAgo != tc.hasAgo || ok != tc.ok {
			t.Errorf("parseRestarts(%q) = %d, %v, %v, %v; want %d, %v, %v, %v",
				tc.in, count, ago, hasAgo, ok, tc.count, tc.ago, tc.hasAgo, tc.ok)
		}
	}
}

func TestCoarsenAndFormatAgo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   time.Duration
		want time.Duration
		text string
	}{
		{-5 * time.Second, 0, "<1m"},
		{59 * time.Second, 0, "<1m"},
		{61 * time.Second, time.Minute, "1m"},
		{59*time.Minute + 59*time.Second, 59 * time.Minute, "59m"},
		{3*time.Hour + 59*time.Minute, 3 * time.Hour, "3h"},
		{25 * time.Hour, 24 * time.Hour, "1d"},
		{212*24*time.Hour + 5*time.Hour, 212 * 24 * time.Hour, "212d"},
	}
	for _, tc := range cases {
		got := Coarsen(tc.in)
		if got != tc.want {
			t.Errorf("Coarsen(%v) = %v, want %v", tc.in, got, tc.want)
		}
		if text := FormatAgo(got); text != tc.text {
			t.Errorf("FormatAgo(%v) = %q, want %q", got, text, tc.text)
		}
	}
}

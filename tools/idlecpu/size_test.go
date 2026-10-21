package main

import "testing"

func TestParseSize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    int64
		wantErr string
	}{
		{name: "decimal megabytes", in: "40MB", want: 40_000_000},
		{name: "kubernetes decimal mega", in: "40M", want: 40_000_000},
		{name: "binary mebibytes", in: "40MiB", want: 40 << 20},
		{name: "kubernetes binary mebi", in: "40Mi", want: 40 << 20},
		{name: "fraction", in: "1.5GiB", want: 3 << 29},
		{name: "kilo lower case", in: "512k", want: 512_000},
		{name: "kibibytes", in: "512KiB", want: 512 << 10},
		{name: "giga", in: "1G", want: 1_000_000_000},
		{name: "bytes", in: "4096B", want: 4096},
		{name: "spaces around", in: " 40 MB ", want: 40_000_000},
		{name: "bare number is ambiguous", in: "40", wantErr: "want a unit"},
		{name: "unknown unit", in: "40mb", wantErr: "unknown unit"},
		{name: "empty", in: "", wantErr: "want a size"},
		{name: "no number", in: "MB", wantErr: "want a size"},
		{name: "zero", in: "0MB", wantErr: "want a size above 0"},
		{name: "negative", in: "-1MB", wantErr: "want a size"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseSize(tc.in)
			checkErr(t, err, tc.wantErr)
			if tc.wantErr == "" && got != tc.want {
				t.Fatalf("parseSize(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseWinsize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		in               string
		wantCols, wantRo uint16
		wantErr          string
	}{
		{name: "spec size", in: "120x40", wantCols: 120, wantRo: 40},
		{name: "widest golden", in: "207x58", wantCols: 207, wantRo: 58},
		{name: "upper-case X", in: "80X24", wantCols: 80, wantRo: 24},
		{name: "no separator", in: "120", wantErr: "want COLSxROWS"},
		{name: "zero columns", in: "0x40", wantErr: "want COLSxROWS"},
		{name: "too large", in: "5000x40", wantErr: "want COLSxROWS"},
		{name: "letters", in: "wide x tall", wantErr: "want COLSxROWS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cols, rows, err := parseWinsize(tc.in)
			checkErr(t, err, tc.wantErr)
			if tc.wantErr == "" && (cols != tc.wantCols || rows != tc.wantRo) {
				t.Fatalf("parseWinsize(%q) = %dx%d, want %dx%d", tc.in, cols, rows, tc.wantCols, tc.wantRo)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   int64
		want string
	}{
		{in: 27340 * 1024, want: "26.7 MiB (28.0 MB)"},
		{in: 40 << 20, want: "40.0 MiB (41.9 MB)"},
		{in: 0, want: "0.0 MiB (0.0 MB)"},
	}
	for _, tc := range cases {
		if got := formatBytes(tc.in); got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

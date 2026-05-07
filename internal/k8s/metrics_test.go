package k8s

import "testing"

func TestParseCPUMilli(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  int64
	}{
		{"", 0},
		{"10m", 10},
		{"500m", 500},
		{"1", 1000},
		{"2", 2000},
		{"0.25", 250},
		{"garbage", 0},
	}
	for _, tc := range cases {
		if got := parseCPUMilli(tc.input); got != tc.want {
			t.Errorf("parseCPUMilli(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestParseMemoryBytes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  int64
	}{
		{"", 0},
		{"1024", 1024},
		{"1Ki", 1024},
		{"1Mi", 1024 * 1024},
		{"1Gi", 1024 * 1024 * 1024},
		{"512Ki", 512 * 1024},
		{"garbage", 0},
	}
	for _, tc := range cases {
		if got := parseMemoryBytes(tc.input); got != tc.want {
			t.Errorf("parseMemoryBytes(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestFormatCPU(t *testing.T) {
	t.Parallel()
	cases := []struct {
		milli int64
		want  string
	}{
		{0, "0m"},
		{10, "10m"},
		{500, "500m"},
		{1000, "1.000"},
		{1250, "1.250"},
		{2000, "2.000"},
	}
	for _, tc := range cases {
		if got := FormatCPU(tc.milli); got != tc.want {
			t.Errorf("FormatCPU(%d) = %q, want %q", tc.milli, got, tc.want)
		}
	}
}

func TestFormatMemory(t *testing.T) {
	t.Parallel()
	cases := []struct {
		bytes int64
		want  string
	}{
		{0, "0B"},
		{1024, "1Ki"},
		{1024 * 1024, "1Mi"},
		{1024 * 1024 * 1024, "1Gi"},
		{2 * 1024 * 1024, "2Mi"},
		{512 * 1024, "512Ki"},
	}
	for _, tc := range cases {
		if got := FormatMemory(tc.bytes); got != tc.want {
			t.Errorf("FormatMemory(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

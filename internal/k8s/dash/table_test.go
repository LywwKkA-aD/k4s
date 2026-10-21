package dash

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/client-go/rest"
)

const podsTableJSON = `{
 "kind":"Table","apiVersion":"meta.k8s.io/v1","metadata":{"resourceVersion":"812"},
 "columnDefinitions":[
  {"name":"Name","type":"string"},{"name":"Ready","type":"string"},{"name":"Status","type":"string"},
  {"name":"Restarts","type":"string"},{"name":"Age","type":"string"},{"name":"IP","type":"string"},
  {"name":"Node","type":"string"},{"name":"Nominated Node","type":"string"},{"name":"Readiness Gates","type":"string"}],
 "rows":[
  {"cells":["web-1","1/1","Running","0","3h","10.42.0.9","cp-1","<none>","<none>"],"object":null},
  {"cells":["db-\"0\"","0/1","CrashLoopBackOff","41 (2m ago)","5d","10.42.0.7","agent-1","<none>","<none>"],"object":null}]}`

func TestDecodeTableReadsColumnsByName(t *testing.T) {
	t.Parallel()

	tb, err := decodeTable([]byte(podsTableJSON))
	if err != nil {
		t.Fatalf("decodeTable: %v", err)
	}
	if tb.len() != 2 {
		t.Fatalf("rows = %d, want 2", tb.len())
	}
	name, status, node := tb.col("Name"), tb.col("status"), tb.col("Node")
	if name < 0 || status < 0 || node < 0 {
		t.Fatalf("columns not found: name=%d status=%d node=%d", name, status, node)
	}
	if got, ok := tb.str(1, name); !ok || got != `db-"0"` {
		t.Fatalf("escaped name = %q, %v", got, ok)
	}
	if got, ok := tb.str(1, status); !ok || got != "CrashLoopBackOff" {
		t.Fatalf("status = %q, %v", got, ok)
	}
	if tb.col("Namespace") != -1 {
		t.Fatal("a missing column must report -1")
	}
	if _, ok := tb.str(0, -1); ok {
		t.Fatal("reading a missing column must fail, not guess")
	}
	if _, ok := tb.str(7, name); ok {
		t.Fatal("reading past the last row must fail")
	}
}

func TestDecodeTableCellTypes(t *testing.T) {
	t.Parallel()

	body := `{"columnDefinitions":[{"name":"Name"},{"name":"Desired"},{"name":"Suspend"},{"name":"Text"}],
	 "rows":[{"cells":["a",3,true,"7"]},{"cells":["b",null,false,"x"]},{"cells":["c"]}],
	 "metadata":{"remainingItemCount":5,"continue":"tok"}}`
	tb, err := decodeTable([]byte(body))
	if err != nil {
		t.Fatalf("decodeTable: %v", err)
	}
	desired, suspend, text := tb.col("Desired"), tb.col("Suspend"), tb.col("Text")
	nums := []struct {
		row, col int
		want     int64
		ok       bool
	}{{0, desired, 3, true}, {0, text, 7, true}, {1, desired, 0, false}, {1, text, 0, false}}
	for _, n := range nums {
		if got, ok := tb.num(n.row, n.col); got != n.want || ok != n.ok {
			t.Errorf("num(%d, %d) = %d, %v; want %d, %v", n.row, n.col, got, ok, n.want, n.ok)
		}
	}
	flags := []struct {
		row, col int
		want, ok bool
	}{{0, suspend, true, true}, {1, suspend, false, true}, {0, text, false, false}}
	for _, f := range flags {
		if got, ok := tb.flag(f.row, f.col); got != f.want || ok != f.ok {
			t.Errorf("flag(%d, %d) = %v, %v; want %v, %v", f.row, f.col, got, ok, f.want, f.ok)
		}
	}
	for _, rc := range [][2]int{{2, desired}, {0, desired}} {
		if _, ok := tb.str(rc[0], rc[1]); ok {
			t.Errorf("str(%d, %d) read a cell that is not a string", rc[0], rc[1])
		}
	}
	if tb.remaining != 5 || !tb.truncated {
		t.Fatalf("remaining = %d truncated = %v", tb.remaining, tb.truncated)
	}
}

func TestDecodeTableRejectsGarbage(t *testing.T) {
	t.Parallel()

	if _, err := decodeTable([]byte("<html>")); err == nil {
		t.Fatal("want an error for a non-JSON body")
	}
}

func TestGetTableSendsTableAcceptAndListParams(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		opts      listOpts
		wantQuery []string
		absent    []string
	}{
		{"cached list", listOpts{}, []string{"includeObject=None", "resourceVersion=0"}, []string{"limit="}},
		{"consistent read", listOpts{consistent: true}, []string{"includeObject=None"}, []string{"resourceVersion"}},
		{"field selector and limit", listOpts{fieldSelector: "spec.type=LoadBalancer", limit: 10, consistent: true},
			[]string{"fieldSelector=spec.type%3DLoadBalancer", "limit=10"}, []string{"resourceVersion"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var gotAccept, gotQuery, gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAccept, gotQuery, gotPath = r.Header.Get("Accept"), r.URL.RawQuery, r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, podsTableJSON)
			}))
			t.Cleanup(srv.Close)

			tb, n, err := getTable(context.Background(), restFor(t, srv.URL), "/api/v1/pods", tc.opts)
			if err != nil {
				t.Fatalf("getTable: %v", err)
			}
			if n != len(podsTableJSON) || tb.len() != 2 {
				t.Fatalf("bytes = %d rows = %d", n, tb.len())
			}
			if gotPath != "/api/v1/pods" || !strings.HasPrefix(gotAccept, "application/json;as=Table;v=v1;g=meta.k8s.io") {
				t.Fatalf("path %q accept %q", gotPath, gotAccept)
			}
			for _, q := range tc.wantQuery {
				if !strings.Contains(gotQuery, q) {
					t.Errorf("query %q lacks %q", gotQuery, q)
				}
			}
			for _, q := range tc.absent {
				if strings.Contains(gotQuery, q) {
					t.Errorf("query %q must not contain %q", gotQuery, q)
				}
			}
		})
	}
}

func TestGetTableWithoutRESTClient(t *testing.T) {
	t.Parallel()

	if _, _, err := getTable(context.Background(), nil, "/api/v1/pods", listOpts{}); err == nil {
		t.Fatal("want an error without a REST client")
	}
}

func restFor(t testing.TB, host string) rest.Interface {
	t.Helper()
	rc, err := rest.UnversionedRESTClientFor(&rest.Config{
		Host:          host,
		QPS:           1000,
		Burst:         1000,
		ContentConfig: rest.ContentConfig{NegotiatedSerializer: tableCodecs()},
	})
	if err != nil {
		t.Fatalf("rest client: %v", err)
	}
	return rc
}

func BenchmarkDecodePodsTable(b *testing.B) {
	for _, rows := range []int{185, 2000, 5000} {
		body := syntheticPodsTable(rows)
		b.Run(fmt.Sprint(rows), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for b.Loop() {
				if _, err := decodeTable(body); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func syntheticPodsTable(rows int) []byte {
	var sb strings.Builder
	sb.WriteString(`{"kind":"Table","apiVersion":"meta.k8s.io/v1","metadata":{},"columnDefinitions":[` +
		`{"name":"Name"},{"name":"Ready"},{"name":"Status"},{"name":"Restarts"},{"name":"Age"},{"name":"IP"},` +
		`{"name":"Node"},{"name":"Nominated Node"},{"name":"Readiness Gates"}],"rows":[`)
	for i := range rows {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `{"cells":["app-%05d-6c9f7d8b5-xk2lp","1/1","Running","%d (3h ago)","5d","10.42.%d.%d","agent-%d","<none>","<none>"],"object":null}`,
			i, i%4, i/250, i%250, i%20)
	}
	sb.WriteString("]}")
	return []byte(sb.String())
}

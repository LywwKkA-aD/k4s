package dash

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/client-go/rest"
)

// tableAccept asks the apiserver for a server-side Table. Table over
// protobuf is answered with 406, so this always goes out as JSON.
const tableAccept = "application/json;as=Table;v=v1;g=meta.k8s.io, application/json"

// errNoREST reports a source without a REST client, such as a fake
// clientset, which cannot serve Tables or raw paths.
var errNoREST = errors.New("no REST client for raw requests")

// listOpts are the list parameters the dashboard varies per request.
type listOpts struct {
	consistent    bool // RV="" (a quorum read) instead of RV="0" (watch cache)
	fieldSelector string
	limit         int64
}

func (o listOpts) resourceVersion() string {
	if o.consistent {
		return ""
	}
	return "0"
}

// table is a decoded server-side Table with includeObject=None. Cells stay
// raw until read, so columns nobody asks for are never decoded.
type table struct {
	cols      []string
	rows      [][]json.RawMessage
	remaining int64
	truncated bool
}

type wireTable struct {
	Metadata struct {
		Continue           string `json:"continue"`
		RemainingItemCount *int64 `json:"remainingItemCount"`
	} `json:"metadata"`
	ColumnDefinitions []struct {
		Name string `json:"name"`
	} `json:"columnDefinitions"`
	Rows []struct {
		Cells []json.RawMessage `json:"cells"`
	} `json:"rows"`
}

func decodeTable(body []byte) (table, error) {
	var w wireTable
	if err := json.Unmarshal(body, &w); err != nil {
		return table{}, fmt.Errorf("decode table: %w", err)
	}
	cols := make([]string, len(w.ColumnDefinitions))
	for i, c := range w.ColumnDefinitions {
		cols[i] = c.Name
	}
	rows := make([][]json.RawMessage, len(w.Rows))
	for i, r := range w.Rows {
		rows[i] = r.Cells
	}
	t := table{cols: cols, rows: rows, truncated: w.Metadata.Continue != ""}
	if w.Metadata.RemainingItemCount != nil {
		t.remaining = *w.Metadata.RemainingItemCount
		t.truncated = true
	}
	return t, nil
}

func (t table) len() int { return len(t.rows) }

// col finds a column by name, ignoring case; -1 when the server did not send
// it, in which case every read of it fails and the metric is dropped.
func (t table) col(name string) int {
	for i, c := range t.cols {
		if strings.EqualFold(c, name) {
			return i
		}
	}
	return -1
}

func (t table) cell(row, col int) (json.RawMessage, bool) {
	if col < 0 || row < 0 || row >= len(t.rows) || col >= len(t.rows[row]) {
		return nil, false
	}
	return t.rows[row][col], true
}

func (t table) str(row, col int) (string, bool) {
	raw, ok := t.cell(row, col)
	if !ok || len(raw) < 2 || raw[0] != '"' {
		return "", false
	}
	// Most cells are plain ASCII; skip the decoder unless there is an escape.
	if bytes.IndexByte(raw, '\\') < 0 {
		return string(raw[1 : len(raw)-1]), true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// num reads an integer cell, or a string cell holding one. The counts it
// reads are int32 in the API, so a wider value is rejected rather than
// wrapped on 32-bit builds.
func (t table) num(row, col int) (int, bool) {
	raw, ok := t.cell(row, col)
	if !ok || len(raw) == 0 {
		return 0, false
	}
	s := string(raw)
	if raw[0] == '"' {
		s, _ = t.str(row, col)
	}
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, false
	}
	return int(n), true
}

func (t table) flag(row, col int) (bool, bool) {
	raw, ok := t.cell(row, col)
	if !ok {
		return false, false
	}
	switch string(raw) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

// getTable lists path as a Table without objects and returns it with the
// number of body bytes decoded.
func getTable(ctx context.Context, rc rest.Interface, path string, o listOpts) (table, int, error) {
	if !usable(rc) {
		return table{}, 0, errNoREST
	}
	req := rc.Get().AbsPath(path).Param("includeObject", "None").SetHeader("Accept", tableAccept)
	if rv := o.resourceVersion(); rv != "" {
		req = req.Param("resourceVersion", rv)
	}
	if o.fieldSelector != "" {
		req = req.Param("fieldSelector", o.fieldSelector)
	}
	if o.limit > 0 {
		req = req.Param("limit", strconv.FormatInt(o.limit, 10))
	}
	body, err := req.Do(ctx).Raw()
	if err != nil {
		return table{}, len(body), fmt.Errorf("list %s: %w", path, err)
	}
	t, err := decodeTable(body)
	return t, len(body), err
}

// getRaw GETs a non-resource path such as /readyz and returns the body even
// when the status is an error, since /readyz explains a failure in it.
func getRaw(ctx context.Context, rc rest.Interface, path string, params map[string]string) ([]byte, error) {
	if !usable(rc) {
		return nil, errNoREST
	}
	req := rc.Get().AbsPath(path).SetHeader("Accept", "application/json, */*")
	for k, v := range params {
		req = req.Param(k, v)
	}
	body, err := req.Do(ctx).Raw()
	if err != nil {
		return body, fmt.Errorf("get %s: %w", path, err)
	}
	return body, nil
}

// usable rejects a nil interface and a typed nil *rest.RESTClient, which a
// fake clientset hands out.
func usable(rc rest.Interface) bool {
	if rc == nil {
		return false
	}
	if c, ok := rc.(*rest.RESTClient); ok && c == nil {
		return false
	}
	return true
}

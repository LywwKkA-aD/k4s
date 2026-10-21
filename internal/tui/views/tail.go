package views

// MaxLogLines is how many lines the logs view keeps; older lines scroll
// out. A stream's tail never asks for more than that in total.
const MaxLogLines = 5000

// TailCap is the most lines each of pods pods may start with: MaxLogLines
// shared out, rounded up, so a tail of 100000 over 20 pods asks for 250
// lines a pod. No pods counts as one. The tail prompt shows it, and the
// logs view clamps its tail to it.
func TailCap(pods int) int64 {
	n := int64(max(pods, 1))
	return max((MaxLogLines+n-1)/n, 1)
}

// ClampTail is tail limited to TailCap(pods). Zero or less, the whole
// log, is clamped as well: the buffer could not hold it.
func ClampTail(tail int64, pods int) int64 {
	limit := TailCap(pods)
	if tail <= 0 {
		return limit
	}
	return min(tail, limit)
}

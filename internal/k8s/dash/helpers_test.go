package dash

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

// fixedNow is the wall clock every fixture is built against.
var fixedNow = time.Date(2026, 10, 21, 14, 32, 5, 0, time.UTC)

func tableCodecs() runtime.NegotiatedSerializer { return scheme.Codecs.WithoutConversion() }

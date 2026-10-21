package dash

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

func tableCodecs() runtime.NegotiatedSerializer { return scheme.Codecs.WithoutConversion() }

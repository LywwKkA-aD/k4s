package k8s

// ClassifyOnHost is Classify with the k3s-server check answered by
// k3sServer instead of the machine the tests run on.
func ClassifyOnHost(err error, k3sServer bool) Diagnosis {
	return classify(err, func() bool { return k3sServer })
}

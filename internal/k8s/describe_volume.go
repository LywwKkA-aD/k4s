package k8s

import (
	corev1 "k8s.io/api/core/v1"
)

// writeVolumes writes a "Volumes:" section for the common volume types:
// the type line kubectl prints and the fields that name what is mounted.
// Projected sources and the rarer types show their type alone.
func writeVolumes(w describeWriter, space string, volumes []corev1.Volume) {
	if len(volumes) == 0 {
		w.write(0, "%sVolumes:\t<none>\n", space)
		return
	}
	w.write(0, "%sVolumes:\n", space)
	nameIndent := ""
	if space != "" {
		nameIndent = " "
	}
	for _, v := range volumes {
		w.write(1, "%s%s:\n", nameIndent, v.Name)
		writeVolumeSource(w, v.VolumeSource)
	}
}

func writeVolumeSource(w describeWriter, src corev1.VolumeSource) {
	switch {
	case src.HostPath != nil:
		kind := "<none>"
		if src.HostPath.Type != nil {
			kind = string(*src.HostPath.Type)
		}
		w.write(2, "Type:\tHostPath (bare host directory volume)\n")
		w.write(2, "Path:\t%s\n", src.HostPath.Path)
		w.write(2, "HostPathType:\t%s\n", kind)
	case src.EmptyDir != nil:
		limit := "<unset>"
		if q := src.EmptyDir.SizeLimit; q != nil && q.Sign() > 0 {
			limit = q.String()
		}
		w.write(2, "Type:\tEmptyDir (a temporary directory that shares a pod's lifetime)\n")
		w.write(2, "Medium:\t%s\n", src.EmptyDir.Medium)
		w.write(2, "SizeLimit:\t%s\n", limit)
	case src.Secret != nil:
		w.write(2, "Type:\tSecret (a volume populated by a Secret)\n")
		w.write(2, "SecretName:\t%s\n", src.Secret.SecretName)
		w.write(2, "Optional:\t%t\n", isTrue(src.Secret.Optional))
	case src.ConfigMap != nil:
		w.write(2, "Type:\tConfigMap (a volume populated by a ConfigMap)\n")
		w.write(2, "Name:\t%s\n", src.ConfigMap.Name)
		w.write(2, "Optional:\t%t\n", isTrue(src.ConfigMap.Optional))
	case src.PersistentVolumeClaim != nil:
		w.write(2, "Type:\tPersistentVolumeClaim (a reference to a PersistentVolumeClaim in the same namespace)\n")
		w.write(2, "ClaimName:\t%s\n", src.PersistentVolumeClaim.ClaimName)
		w.write(2, "ReadOnly:\t%t\n", src.PersistentVolumeClaim.ReadOnly)
	case src.Projected != nil:
		w.write(2, "Type:\tProjected (a volume that contains injected data from multiple sources)\n")
	default:
		w.write(1, "<unknown>\n")
	}
}

func isTrue(b *bool) bool { return b != nil && *b }

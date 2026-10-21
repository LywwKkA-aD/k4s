package dash

import (
	"slices"
	"strings"
	"time"
)

// Severity orders what needs attention. Each value has exactly one glyph in
// the view, in colour and in mono alike.
type Severity int

// Severities, least to most severe.
const (
	SevOK      Severity = iota // ●
	SevInfo                    // ◇ worth knowing, e.g. restarted in the last 24 h
	SevMinor                   // · listed but never a verdict chip, e.g. Evicted
	SevWaiting                 // △ degraded or waiting
	SevFailing                 // ▲ failing
)

// Class groups pod STATUS words by what the next step is.
type Class int

// Pod classes.
const (
	ClassOK       Class = iota // Running and ready
	ClassDone                  // Completed
	ClassCrash                 // CrashLoopBackOff, Error, OOMKilled and their Init: forms
	ClassPull                  // ImagePullBackOff, ErrImagePull
	ClassConfig                // CreateContainerConfigError and other start failures
	ClassPending               // Pending, ContainerCreating, PodInitializing, Init:N/M
	ClassNotReady              // Running with READY a < b
	ClassEvicted               // Evicted
	ClassOther                 // Terminating, Unknown, NodeLost and the rest
)

// startGrace keeps pods that are merely starting out of the verdict, so a
// rollout or a CronJob run does not flip the banner every minute.
const startGrace = 2 * time.Minute

var (
	crashReasons  = []string{"CrashLoopBackOff", "Error", "OOMKilled", "RunContainerError", "ContainerCannotRun", "StartError"}
	pullReasons   = []string{"ImagePullBackOff", "ErrImagePull", "InvalidImageName"}
	configReasons = []string{"CreateContainerConfigError", "CreateContainerError"}
	startReasons  = []string{"Pending", "ContainerCreating", "PodInitializing"}
)

// classOf maps a STATUS word plus READY counts to a Class. Init: forms
// classify by their reason; Init:N/M is still starting.
func classOf(status string, ready, of int, readyKnown bool) Class {
	reason, isInit := strings.CutPrefix(status, "Init:")
	switch {
	case status == "Running" && readyKnown && ready < of:
		return ClassNotReady
	case status == "Running":
		return ClassOK
	case status == "Completed" || status == "Succeeded":
		return ClassDone
	case status == "Evicted":
		return ClassEvicted
	case slices.Contains(crashReasons, reason):
		return ClassCrash
	case slices.Contains(pullReasons, reason):
		return ClassPull
	case slices.Contains(configReasons, reason):
		return ClassConfig
	case slices.Contains(startReasons, status), isInit && strings.Contains(reason, "/"):
		return ClassPending
	default:
		return ClassOther
	}
}

// severityOf is the pod severity rule from the spec (§4 severity rules).
func severityOf(c Class) Severity {
	switch c {
	case ClassCrash, ClassPull, ClassConfig:
		return SevFailing
	case ClassPending, ClassNotReady:
		return SevWaiting
	case ClassEvicted:
		return SevMinor
	default:
		return SevOK
	}
}

// workloadSeverity: 0/N ready with N > 0 is failing, partly ready waiting.
func workloadSeverity(ready, desired int) Severity {
	switch {
	case desired > 0 && ready == 0:
		return SevFailing
	case ready < desired:
		return SevWaiting
	default:
		return SevOK
	}
}

// meterSeverity marks a node at 85% or more. The critique fixes it at △ so
// the meter glyph and the verdict chip always agree.
func meterSeverity(pct int) Severity {
	if pct >= hotPercent {
		return SevWaiting
	}
	return SevOK
}

const hotPercent = 85

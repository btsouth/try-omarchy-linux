package main

import "testing"

func TestCancelledRecoveryWaitsForHomeBeforeResuming(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cancelling bool
		prompt     string
		want       bool
	}{
		{"ordinary progress", false, "", true},
		{"cancelled progress", true, "", false},
		{"cancelled recovery prompt", true, "recovery", false},
		{"completed cancellation", true, "home", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := acceptStateAfterCancel(tc.cancelling, state{Prompt: tc.prompt}); got != tc.want {
				t.Fatalf("acceptStateAfterCancel(%v, %q) = %v, want %v", tc.cancelling, tc.prompt, got, tc.want)
			}
		})
	}
}

//go:build e2e

package e2e

import "testing"

// TestSubmissionHeadroomRegression locks in the finding behind rejection
// scenarios: a recording simulation under-counts instructions because it does
// not execute the account's __check_auth. Without extra headroom, scenario E
// can fail from budget exhaustion before reaching UnknownDelegate, making a
// regression look like a successful host rejection.
//
// This is a committed fixture, not a live test: it protects the measured
// runner policy even when a contributor reproduces only the pure test suite.
func TestSubmissionHeadroomRegression(t *testing.T) {
	tests := []struct {
		name          string
		expectFailure bool
		want          uint32
	}{
		{name: "accepted scenarios use enforcing simulation resources", expectFailure: false, want: 1},
		{name: "rejection scenarios retain the measured budget allowance", expectFailure: true, want: rejectionHeadroom},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := submissionHeadroom(tt.expectFailure); got != tt.want {
				t.Fatalf("submissionHeadroom(%v) = %d, want %d", tt.expectFailure, got, tt.want)
			}
		})
	}
}

package helpers

import "testing"

func Assert[K comparable](t testing.TB, got K, want K) {
	t.Helper()
	if got != want {
		t.Errorf("got '%v' want '%v'", got, want)
	}
}

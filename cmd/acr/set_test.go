package main

import "testing"

func TestSet(t *testing.T) {
	set := New[string]()
	// test checking a non-existing key
	key1 := "foo"
	if got, want := set.Contains(key1), false; got != want {
		t.Errorf("Set.Contains(%s) = %v, want %v", key1, got, want)
	}
	if got, want := len(set), 0; got != want {
		t.Errorf("len(Set) = %v, want %v", got, want)
	}

	// test adding a new key
	set.Add(key1)
	if got, want := set.Contains(key1), true; got != want {
		t.Errorf("Set.Contains(%s) = %v, want %v", key1, got, want)
	}
	if got, want := len(set), 1; got != want {
		t.Errorf("len(Set) = %v, want %v", got, want)
	}

	// test adding an existing key
	set.Add(key1)
	if got, want := set.Contains(key1), true; got != want {
		t.Errorf("Set.Contains(%s) = %v, want %v", key1, got, want)
	}
	if got, want := len(set), 1; got != want {
		t.Errorf("len(Set) = %v, want %v", got, want)
	}

	// test adding another key
	key2 := "bar"
	set.Add(key2)
	if got, want := set.Contains(key2), true; got != want {
		t.Errorf("Set.Contains(%s) = %v, want %v", key2, got, want)
	}
	if got, want := len(set), 2; got != want {
		t.Errorf("len(Set) = %v, want %v", got, want)
	}
}

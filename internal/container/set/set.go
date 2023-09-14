package set

// Set represents a set data structure.
type Set[T comparable] map[T]struct{}

// New returns an initialized set.
func New[T comparable](items ...T) Set[T] {
	s := make(Set[T])
	for _, item := range items {
		s.Add(item)
	}
	return s
}

// Add adds item into the set s.
func (s Set[T]) Add(item T) {
	s[item] = struct{}{}
}

// Contains returns true if the set s contains item.
func (s Set[T]) Contains(item T) bool {
	_, ok := s[item]
	return ok
}

package tree

// Node represents a tree node.
type Node struct {
	Value any
	Nodes []*Node
}

// New creates a new tree / root node.
func New(value any) *Node {
	return &Node{
		Value: value,
	}
}

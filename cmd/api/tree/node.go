package tree

import "reflect"

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

// Add adds a leaf node.
func (n *Node) Add(value any) *Node {
	node := New(value)
	n.Nodes = append(n.Nodes, node)
	return node
}

// AddPath adds a chain of nodes.
func (n *Node) AddPath(values ...any) *Node {
	if len(values) == 0 {
		return nil
	}

	current := n
	for _, value := range values {
		if node := current.Find(value); node == nil {
			current = current.Add(value)
		} else {
			current = node
		}
	}
	return current
}

// Find finds the child node with the target value.
// Nil if not found.
func (n *Node) Find(value any) *Node {
	for _, node := range n.Nodes {
		if reflect.DeepEqual(node.Value, value) {
			return node
		}
	}
	return nil
}

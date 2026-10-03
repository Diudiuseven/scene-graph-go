// Package scene implements a hierarchical 3D scene graph: nodes arranged in a
// tree, each carrying a local transform whose world transform is the product
// of the local transforms along the path to the root.
//
// The package provides frequent structural edits (create, delete, reparent,
// sibling reorder), lazy world-transform caching with dirty propagation,
// concurrent reads/writes under a single RWMutex, rejection of illegal
// structures (cycles, references to deleted parents), and versioned
// serialization with migration. See README.md for the full semantics and
// consistency guarantees.
package scene

import (
	"errors"
	"fmt"
	"sync"

	"github.com/Diudiuseven/scene-graph-go/mathx"
)

// NodeID is the stable identifier of a node within a Scene.
type NodeID string

// Node is a node in the scene tree.
type Node struct {
	id       NodeID
	name     string
	parent   NodeID // empty for the root
	children []NodeID
	local    mathx.Transform
	world    mathx.Mat4
	dirty    bool
}

// Scene is a tree of nodes with cached world transforms.
//
// Concurrency: all state is guarded by mu. Reads that find a clean cache take
// the read lock; a read that must recompute upgrades to the write lock, so a
// reader always observes one self-consistent tree state and never blocks a
// writer indefinitely (no lock is held while waiting on another lock).
type Scene struct {
	mu    sync.RWMutex
	nodes map[NodeID]*Node
	root  NodeID
	seq   uint64 // monotonic counter for generated node ids

	// recomputes counts node world-transform evaluations performed by lazy
	// reads (Snapshot recomputes all nodes and is not counted here).
	recomputes uint64
}

// Errors returned by Scene operations.
var (
	// ErrNodeNotFound is returned when a referenced node does not exist.
	ErrNodeNotFound = errors.New("scene: node not found")
	// ErrParentNotFound is returned when a node is attached to a parent that
	// does not exist (e.g. it was deleted).
	ErrParentNotFound = errors.New("scene: parent node not found")
	// ErrCycle is returned when an operation would create a parent cycle.
	ErrCycle = errors.New("scene: cycle detected")
	// ErrInvalidOperation is returned for edits that are never legal, such as
	// deleting or reparenting the root.
	ErrInvalidOperation = errors.New("scene: invalid operation")
)

// rootID is the fixed identifier of the scene root.
const rootID NodeID = "root"

// NewScene creates an empty scene containing only the root node.
func NewScene() *Scene {
	s := &Scene{
		nodes: make(map[NodeID]*Node),
		root:  rootID,
	}
	root := &Node{
		id:    rootID,
		name:  "root",
		local: mathx.DefaultTransform(),
		world: mathx.IdentityMat4(),
	}
	s.nodes[rootID] = root
	return s
}

// Root returns the id of the scene's root node.
func (s *Scene) Root() NodeID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.root
}

// nextID generates a deterministic, process-local unique node id. Callers
// must hold s.mu.
func (s *Scene) nextID() NodeID {
	s.seq++
	return NodeID(fmt.Sprintf("n%d", s.seq))
}

// CreateNode adds a node under parentID. An empty parentID means the root.
// The new node is created dirty; its world transform is computed lazily on
// the next read.
func (s *Scene) CreateNode(parentID NodeID, name string, local mathx.Transform) (NodeID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if parentID == "" {
		parentID = s.root
	}
	parent, ok := s.nodes[parentID]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrParentNotFound, parentID)
	}

	id := s.nextID()
	n := &Node{
		id:     id,
		name:   name,
		parent: parentID,
		local:  local,
		dirty:  true,
	}
	s.nodes[id] = n
	parent.children = append(parent.children, id)
	return id, nil
}

// DeleteNode removes a node. Its children are reparented to their
// grandparent, preserving sibling order. The root cannot be deleted.
func (s *Scene) DeleteNode(id NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	n, ok := s.nodes[id]
	if !ok {
		return ErrNodeNotFound
	}
	if id == s.root {
		return fmt.Errorf("%w: cannot delete root", ErrInvalidOperation)
	}

	parent := s.nodes[n.parent]
	idx := indexOf(parent.children, id)

	// Reparent the deleted node's children to their grandparent, keeping
	// their relative order at the deleted node's position.
	newChildren := make([]NodeID, 0, len(parent.children)-1+len(n.children))
	newChildren = append(newChildren, parent.children[:idx]...)
	for _, cid := range n.children {
		child := s.nodes[cid]
		child.parent = n.parent
		child.dirty = true
		newChildren = append(newChildren, cid)
	}
	newChildren = append(newChildren, parent.children[idx+1:]...)
	parent.children = newChildren

	delete(s.nodes, id)
	return nil
}

// SetParent reparents id under newParentID, preserving its local transform.
// The node (and lazily, its subtree) is marked dirty so world transforms
// stay consistent with the new parent chain.
func (s *Scene) SetParent(id, newParentID NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	n, ok := s.nodes[id]
	if !ok {
		return ErrNodeNotFound
	}
	if id == s.root {
		return fmt.Errorf("%w: cannot reparent root", ErrInvalidOperation)
	}
	if newParentID == "" {
		newParentID = s.root
	}
	p, ok := s.nodes[newParentID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrParentNotFound, newParentID)
	}
	if newParentID == id {
		return fmt.Errorf("%w: node %s cannot be its own parent", ErrCycle, id)
	}
	// Cycle check: newParent must not be id or a descendant of id.
	for cur := newParentID; cur != ""; {
		if cur == id {
			return fmt.Errorf("%w: %s is a descendant of %s", ErrCycle, newParentID, id)
		}
		p, ok := s.nodes[cur]
		if !ok {
			return fmt.Errorf("%w: %s", ErrParentNotFound, cur)
		}
		cur = p.parent
	}
	if n.parent == newParentID {
		return nil
	}

	oldParent := s.nodes[n.parent]
	oldParent.children = removeID(oldParent.children, id)
	p.children = append(p.children, id)
	n.parent = newParentID
	n.dirty = true
	return nil
}

// SetSiblingIndex moves id to the given position among its parent's
// children. Sibling order does not affect world transforms, so no node is
// marked dirty.
func (s *Scene) SetSiblingIndex(id NodeID, index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	n, ok := s.nodes[id]
	if !ok {
		return ErrNodeNotFound
	}
	if id == s.root {
		return fmt.Errorf("%w: root has no siblings", ErrInvalidOperation)
	}
	parent := s.nodes[n.parent]
	cur := indexOf(parent.children, id)
	if index < 0 || index >= len(parent.children) {
		return fmt.Errorf("%w: sibling index %d out of range [0,%d)", ErrInvalidOperation, index, len(parent.children))
	}
	if index == cur {
		return nil
	}
	// Remove then insert at the desired final position. After removal the
	// list is one element shorter; inserting at index places id at position
	// index in the final list (clamped to the new length for append).
	children := removeID(parent.children, id)
	if index > len(children) {
		index = len(children)
	}
	parent.children = insertID(children, index, id)
	return nil
}

// SetLocalTransform replaces the local transform of id and marks it dirty;
// the rest of its subtree is recomputed lazily on demand.
func (s *Scene) SetLocalTransform(id NodeID, t mathx.Transform) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	n, ok := s.nodes[id]
	if !ok {
		return ErrNodeNotFound
	}
	n.local = t
	n.dirty = true
	return nil
}

// LocalTransform returns the local transform of id.
func (s *Scene) LocalTransform(id NodeID) (mathx.Transform, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	n, ok := s.nodes[id]
	if !ok {
		return mathx.Transform{}, ErrNodeNotFound
	}
	return n.local, nil
}

// WorldTransform returns the cached world transform of id, recomputing the
// dirty portion of its ancestor chain on demand.
func (s *Scene) WorldTransform(id NodeID) (mathx.Mat4, error) {
	// Fast path: clean cache, take only the read lock.
	s.mu.RLock()
	n, ok := s.nodes[id]
	if !ok {
		s.mu.RUnlock()
		return mathx.Mat4{}, ErrNodeNotFound
	}
	clean := !n.dirty
	if clean {
		for cur := n.parent; cur != ""; {
			p := s.nodes[cur]
			if p == nil || p.dirty {
				clean = false
				break
			}
			cur = p.parent
		}
	}
	if clean {
		w := n.world
		s.mu.RUnlock()
		return w, nil
	}
	s.mu.RUnlock()

	// Slow path: recompute under the write lock.
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.recomputeLocked(id)
	if !ok {
		return mathx.Mat4{}, ErrNodeNotFound
	}
	return w, nil
}

// Snapshot computes the world transform of every node under a single lock
// acquisition, so the returned map reflects one self-consistent tree state.
// Nodes are visited breadth-first from the root, guaranteeing parents are
// computed before children.
func (s *Scene) Snapshot() (map[NodeID]mathx.Mat4, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.nodes[s.root]; !ok {
		return nil, ErrNodeNotFound
	}
	out := make(map[NodeID]mathx.Mat4, len(s.nodes))
	queue := []NodeID{s.root}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		n := s.nodes[id]
		if id == s.root {
			n.world = n.local.Mat4()
		} else {
			parent := s.nodes[n.parent]
			n.world = parent.world.Mul(n.local.Mat4())
		}
		n.dirty = false
		out[id] = n.world
		queue = append(queue, n.children...)
	}
	return out, nil
}

// NodeSnapshot is a self-contained view of one node at a single instant,
// returned by SnapshotFull.
type NodeSnapshot struct {
	ID       NodeID
	Name     string
	Parent   NodeID
	Children []NodeID
	Local    mathx.Transform
	World    mathx.Mat4
}

// SnapshotFull returns a self-contained snapshot of every node (local and
// world transforms, parent and children) computed under a single lock
// acquisition. The returned slice reflects one self-consistent tree state,
// so callers can verify parent-chain consistency without racing against
// concurrent writers. Nodes are ordered breadth-first from the root.
func (s *Scene) SnapshotFull() ([]NodeSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.nodes[s.root]; !ok {
		return nil, ErrNodeNotFound
	}
	out := make([]NodeSnapshot, 0, len(s.nodes))
	queue := []NodeID{s.root}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		n := s.nodes[id]
		if id == s.root {
			n.world = n.local.Mat4()
		} else {
			parent := s.nodes[n.parent]
			n.world = parent.world.Mul(n.local.Mat4())
		}
		n.dirty = false
		out = append(out, NodeSnapshot{
			ID:       n.id,
			Name:     n.name,
			Parent:   n.parent,
			Children: append([]NodeID{}, n.children...),
			Local:    n.local,
			World:    n.world,
		})
		queue = append(queue, n.children...)
	}
	return out, nil
}

// Children returns the ordered list of child ids of id.
func (s *Scene) Children(id NodeID) ([]NodeID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	n, ok := s.nodes[id]
	if !ok {
		return nil, ErrNodeNotFound
	}
	return append([]NodeID{}, n.children...), nil
}

// Parent returns the parent id of id (empty for the root).
func (s *Scene) Parent(id NodeID) (NodeID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	n, ok := s.nodes[id]
	if !ok {
		return "", ErrNodeNotFound
	}
	return n.parent, nil
}

// Name returns the name of id.
func (s *Scene) Name(id NodeID) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	n, ok := s.nodes[id]
	if !ok {
		return "", ErrNodeNotFound
	}
	return n.name, nil
}

// Validate checks the tree integrity: the root exists, every parent
// reference resolves, there are no cycles and every node is reachable from
// the root.
func (s *Scene) Validate() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	root, ok := s.nodes[s.root]
	if !ok {
		return fmt.Errorf("%w: root missing", ErrNodeNotFound)
	}
	if root.parent != "" {
		return fmt.Errorf("%w: root has a parent", ErrInvalidOperation)
	}
	// Walk parent chains from every node; detect cycles and dangling refs.
	for id, n := range s.nodes {
		seen := map[NodeID]bool{id: true}
		cur := n.parent
		for cur != "" {
			p, ok := s.nodes[cur]
			if !ok {
				return fmt.Errorf("%w: node %s references missing parent %s", ErrParentNotFound, id, cur)
			}
			if seen[cur] {
				return fmt.Errorf("%w: at node %s", ErrCycle, id)
			}
			seen[cur] = true
			cur = p.parent
		}
	}
	// Every node must be reachable from the root.
	reachable := map[NodeID]bool{s.root: true}
	queue := []NodeID{s.root}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, cid := range s.nodes[id].children {
			if !reachable[cid] {
				reachable[cid] = true
				queue = append(queue, cid)
			}
		}
	}
	for id := range s.nodes {
		if !reachable[id] {
			return fmt.Errorf("%w: node %s is not reachable from root", ErrInvalidOperation, id)
		}
	}
	return nil
}

// Stats exposes internal counters for diagnostics and tests.
type Stats struct {
	// Nodes is the current number of nodes in the scene.
	Nodes int
	// WorldRecomputes counts node world-transform evaluations performed by
	// lazy reads (Snapshot recomputes all nodes and is not counted here).
	WorldRecomputes uint64
}

// Stats returns a snapshot of internal counters.
func (s *Scene) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Stats{Nodes: len(s.nodes), WorldRecomputes: s.recomputes}
}

// recomputeLocked recomputes the world transform of id and every dirty node
// on its ancestor chain, top-down. Callers must hold s.mu (write lock). It
// returns false if id no longer exists.
func (s *Scene) recomputeLocked(id NodeID) (mathx.Mat4, bool) {
	n, ok := s.nodes[id]
	if !ok {
		return mathx.Mat4{}, false
	}
	// Build the path from n up to the root (or a node whose parent is
	// missing). path[0] is n, path[len(path)-1] is the root.
	var path []*Node
	cur := n
	for {
		path = append(path, cur)
		if cur.parent == "" {
			break
		}
		p, ok := s.nodes[cur.parent]
		if !ok {
			break // dangling parent: treat cur as a root-like node
		}
		cur = p
	}
	// Find the topmost dirty node on the path; everything above it is clean
	// and needs no recomputation.
	start := -1
	for i := len(path) - 1; i >= 0; i-- {
		if path[i].dirty {
			start = i
			break
		}
	}
	if start < 0 {
		return n.world, true
	}
	// Recompute top-down so parents are ready before children.
	for i := start; i >= 0; i-- {
		node := path[i]
		if node.parent == "" || s.nodes[node.parent] == nil {
			node.world = node.local.Mat4()
		} else {
			parent := s.nodes[node.parent]
			node.world = parent.world.Mul(node.local.Mat4())
		}
		node.dirty = false
		s.recomputes++
	}
	return n.world, true
}

// --- small slice helpers (callers must hold s.mu) ---

func indexOf(ids []NodeID, id NodeID) int {
	for i, v := range ids {
		if v == id {
			return i
		}
	}
	return -1
}

func removeID(ids []NodeID, id NodeID) []NodeID {
	out := make([]NodeID, 0, len(ids))
	for _, v := range ids {
		if v != id {
			out = append(out, v)
		}
	}
	return out
}

func insertID(ids []NodeID, index int, id NodeID) []NodeID {
	out := make([]NodeID, 0, len(ids)+1)
	out = append(out, ids[:index]...)
	out = append(out, id)
	out = append(out, ids[index:]...)
	return out
}

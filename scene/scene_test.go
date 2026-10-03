package scene

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/Diudiuseven/scene-graph-go/mathx"
)

const eps = 1e-9

func ident() mathx.Transform {
	return mathx.Transform{Rotation: mathx.IdentityQuat(), Scale: mathx.Vec3{X: 1, Y: 1, Z: 1}}
}

func trans(x, y, z float64) mathx.Transform {
	t := ident()
	t.Position = mathx.Vec3{X: x, Y: y, Z: z}
	return t
}

// worldOrigin returns the origin of the node's world transform.
func worldOrigin(t *testing.T, s *Scene, id NodeID) mathx.Vec3 {
	t.Helper()
	w, err := s.WorldTransform(id)
	if err != nil {
		t.Fatalf("WorldTransform(%s): %v", id, err)
	}
	return w.TransformPoint(mathx.Vec3{})
}

// --- hierarchy & world transform composition ---

func TestWorldTransformComposition(t *testing.T) {
	s := NewScene()
	root := s.Root()
	a, _ := s.CreateNode(root, "a", trans(1, 0, 0))
	b, _ := s.CreateNode(a, "b", trans(0, 2, 0))
	c, _ := s.CreateNode(b, "c", trans(0, 0, 3))

	if got := worldOrigin(t, s, c); !got.Approx(mathx.Vec3{X: 1, Y: 2, Z: 3}, eps) {
		t.Fatalf("c origin = %v", got)
	}
	// root world is identity
	if got := worldOrigin(t, s, root); !got.Approx(mathx.Vec3{}, eps) {
		t.Fatalf("root origin = %v", got)
	}
}

func TestWorldTransformWithRotation(t *testing.T) {
	s := NewScene()
	root := s.Root()
	// a rotates 90 around Z and translates (1,0,0); b is at local (1,0,0).
	a, _ := s.CreateNode(root, "a", mathx.Transform{
		Position: mathx.Vec3{X: 1, Y: 0, Z: 0},
		Rotation: mathx.QuatFromAxisAngle(mathx.Vec3{X: 0, Y: 0, Z: 1}, math.Pi/2),
		Scale:    mathx.Vec3{X: 1, Y: 1, Z: 1},
	})
	b, _ := s.CreateNode(a, "b", trans(1, 0, 0))
	// b: local (1,0,0) rotated 90 around Z -> (0,1,0), then translated (1,0,0) -> (1,1,0)
	if got := worldOrigin(t, s, b); !got.Approx(mathx.Vec3{X: 1, Y: 1, Z: 0}, eps) {
		t.Fatalf("b origin = %v, want (1,1,0)", got)
	}
}

// --- structural edits ---

func TestSetParentUpdatesWorld(t *testing.T) {
	s := NewScene()
	root := s.Root()
	a, _ := s.CreateNode(root, "a", trans(1, 0, 0))
	b, _ := s.CreateNode(root, "b", trans(10, 0, 0))
	c, _ := s.CreateNode(a, "c", trans(0, 5, 0))

	if got := worldOrigin(t, s, c); !got.Approx(mathx.Vec3{X: 1, Y: 5, Z: 0}, eps) {
		t.Fatalf("before reparent, c origin = %v", got)
	}
	// Reparent c under b: world = b(10,0,0) * c(0,5,0) = (10,5,0)
	if err := s.SetParent(c, b); err != nil {
		t.Fatal(err)
	}
	if got := worldOrigin(t, s, c); !got.Approx(mathx.Vec3{X: 10, Y: 5, Z: 0}, eps) {
		t.Fatalf("after reparent, c origin = %v", got)
	}
	// local transform preserved
	lt, err := s.LocalTransform(c)
	if err != nil {
		t.Fatal(err)
	}
	if !lt.Position.Approx(mathx.Vec3{X: 0, Y: 5, Z: 0}, eps) {
		t.Fatalf("local changed after reparent: %v", lt.Position)
	}
}

func TestDeleteNodeReparentsChildren(t *testing.T) {
	s := NewScene()
	root := s.Root()
	a, _ := s.CreateNode(root, "a", trans(1, 0, 0))
	b, _ := s.CreateNode(a, "b", trans(0, 2, 0))
	c, _ := s.CreateNode(a, "c", trans(0, 3, 0))
	d, _ := s.CreateNode(b, "d", trans(0, 0, 4))

	if err := s.DeleteNode(a); err != nil {
		t.Fatal(err)
	}
	// b and c now under root, preserving order; d still under b.
	kids, err := s.Children(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(kids) != 2 || kids[0] != b || kids[1] != c {
		t.Fatalf("root children = %v, want [b c]", kids)
	}
	// b world = local (0,2,0) under root
	if got := worldOrigin(t, s, b); !got.Approx(mathx.Vec3{X: 0, Y: 2, Z: 0}, eps) {
		t.Fatalf("b origin = %v", got)
	}
	// d world = b(0,2,0) * d(0,0,4) = (0,2,4)
	if got := worldOrigin(t, s, d); !got.Approx(mathx.Vec3{X: 0, Y: 2, Z: 4}, eps) {
		t.Fatalf("d origin = %v", got)
	}
	// a is gone
	if _, err := s.WorldTransform(a); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("expected ErrNodeNotFound for deleted a, got %v", err)
	}
}

func TestSetSiblingIndex(t *testing.T) {
	s := NewScene()
	root := s.Root()
	a, _ := s.CreateNode(root, "a", ident())
	b, _ := s.CreateNode(root, "b", ident())
	c, _ := s.CreateNode(root, "c", ident())

	if err := s.SetSiblingIndex(a, 2); err != nil {
		t.Fatal(err)
	}
	kids, _ := s.Children(root)
	if len(kids) != 3 || kids[0] != b || kids[1] != c || kids[2] != a {
		t.Fatalf("children = %v, want [b c a]", kids)
	}
	// out of range
	if err := s.SetSiblingIndex(a, 99); !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("expected ErrInvalidOperation, got %v", err)
	}
}

// --- dirty caching: unaffected nodes are not recomputed ---

func TestDirtyCachingSkipsUnaffectedNodes(t *testing.T) {
	s := NewScene()
	root := s.Root()
	a, _ := s.CreateNode(root, "a", trans(1, 0, 0))
	b, _ := s.CreateNode(a, "b", trans(0, 2, 0))
	c, _ := s.CreateNode(b, "c", trans(0, 0, 3))
	d, _ := s.CreateNode(root, "d", trans(9, 0, 0))

	// Prime the cache.
	worldOrigin(t, s, c)
	worldOrigin(t, s, d)
	base := s.Stats().WorldRecomputes

	// Move a: only the a->b->c path is dirty; d must not be recomputed.
	if err := s.SetLocalTransform(a, trans(10, 0, 0)); err != nil {
		t.Fatal(err)
	}
	worldOrigin(t, s, c)
	after := s.Stats().WorldRecomputes
	// a, b, c recomputed (3); root and d untouched.
	if after-base != 3 {
		t.Fatalf("recomputes after moving a: got %d, want 3", after-base)
	}
	// Reading c again recomputes nothing.
	worldOrigin(t, s, c)
	if s.Stats().WorldRecomputes != after {
		t.Fatalf("second read recomputed unnecessarily")
	}
	// d's world unchanged.
	if got := worldOrigin(t, s, d); !got.Approx(mathx.Vec3{X: 9}, eps) {
		t.Fatalf("d origin = %v", got)
	}
}

func TestReparentMarksSubtreeDirty(t *testing.T) {
	s := NewScene()
	root := s.Root()
	a, _ := s.CreateNode(root, "a", trans(1, 0, 0))
	b, _ := s.CreateNode(root, "b", trans(10, 0, 0))
	c, _ := s.CreateNode(a, "c", trans(0, 5, 0))
	worldOrigin(t, s, c)

	if err := s.SetParent(c, b); err != nil {
		t.Fatal(err)
	}
	// c's world must reflect the new parent immediately.
	if got := worldOrigin(t, s, c); !got.Approx(mathx.Vec3{X: 10, Y: 5, Z: 0}, eps) {
		t.Fatalf("c origin after reparent = %v", got)
	}
}

// --- illegal structure rejection ---

func TestRejectCycle(t *testing.T) {
	s := NewScene()
	root := s.Root()
	a, _ := s.CreateNode(root, "a", ident())
	b, _ := s.CreateNode(a, "b", ident())

	// Reparent a under its own descendant b -> cycle.
	err := s.SetParent(a, b)
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("expected ErrCycle, got %v", err)
	}
	// Self-parenting is also a cycle.
	if err := s.SetParent(a, a); !errors.Is(err, ErrCycle) {
		t.Fatalf("expected ErrCycle for self-parent, got %v", err)
	}
	// Tree must be unchanged after the failed edit.
	p, _ := s.Parent(a)
	if p != root {
		t.Fatalf("a parent = %v, want root (tree mutated by failed edit)", p)
	}
	kids, _ := s.Children(a)
	if len(kids) != 1 || kids[0] != b {
		t.Fatalf("a children = %v, want [b]", kids)
	}
}

func TestRejectDeletedParent(t *testing.T) {
	s := NewScene()
	root := s.Root()
	a, _ := s.CreateNode(root, "a", ident())
	if err := s.DeleteNode(a); err != nil {
		t.Fatal(err)
	}
	// CreateNode under a deleted parent.
	if _, err := s.CreateNode(a, "x", ident()); !errors.Is(err, ErrParentNotFound) {
		t.Fatalf("expected ErrParentNotFound, got %v", err)
	}
	// Reparent to a deleted parent.
	b, _ := s.CreateNode(root, "b", ident())
	if err := s.SetParent(b, a); !errors.Is(err, ErrParentNotFound) {
		t.Fatalf("expected ErrParentNotFound, got %v", err)
	}
}

func TestRejectRootEdits(t *testing.T) {
	s := NewScene()
	root := s.Root()
	if err := s.DeleteNode(root); !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("delete root: expected ErrInvalidOperation, got %v", err)
	}
	if err := s.SetParent(root, root); !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("reparent root: expected ErrInvalidOperation, got %v", err)
	}
	if err := s.SetSiblingIndex(root, 0); !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("sibling index on root: expected ErrInvalidOperation, got %v", err)
	}
}

func TestRejectUnknownNode(t *testing.T) {
	s := NewScene()
	if err := s.DeleteNode("nope"); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("expected ErrNodeNotFound, got %v", err)
	}
	if err := s.SetLocalTransform("nope", ident()); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("expected ErrNodeNotFound, got %v", err)
	}
	if _, err := s.WorldTransform("nope"); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("expected ErrNodeNotFound, got %v", err)
	}
}

// --- determinism ---

func TestDeterminism(t *testing.T) {
	build := func() map[NodeID]mathx.Mat4 {
		s := NewScene()
		root := s.Root()
		a, _ := s.CreateNode(root, "a", trans(1, 2, 3))
		b, _ := s.CreateNode(a, "b", trans(4, 5, 6))
		c, _ := s.CreateNode(root, "c", trans(7, 8, 9))
		_ = c
		s.SetSiblingIndex(a, 1)
		s.SetLocalTransform(b, trans(40, 50, 60))
		s.SetParent(b, root)
		m, _ := s.Snapshot()
		return m
	}
	m1 := build()
	m2 := build()
	if len(m1) != len(m2) {
		t.Fatalf("node count differs: %d vs %d", len(m1), len(m2))
	}
	for id, w := range m1 {
		w2, ok := m2[id]
		if !ok {
			t.Fatalf("node %s missing in second run", id)
		}
		if !w.Approx(w2, 1e-12) {
			t.Fatalf("node %s world differs across runs", id)
		}
	}
}

// --- concurrency ---

// TestConcurrentReadsWrites hammers the scene with concurrent structural
// edits, transform writes and reads. Readers verify that every Snapshot is a
// self-consistent tree state (world[child] == world[parent] * local[child]).
func TestConcurrentReadsWrites(t *testing.T) {
	s := NewScene()
	root := s.Root()
	a, _ := s.CreateNode(root, "a", trans(1, 0, 0))
	b, _ := s.CreateNode(root, "b", trans(0, 1, 0))
	c, _ := s.CreateNode(root, "c", trans(0, 0, 1))
	a1, _ := s.CreateNode(a, "a1", trans(1, 1, 1))
	a2, _ := s.CreateNode(a, "a2", trans(2, 2, 2))
	_ = a2
	b1, _ := s.CreateNode(b, "b1", trans(3, 3, 3))

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writer 1: continuously move a and b.
	wg.Add(1)
	go func() {
		defer wg.Done()
		rng := rand.New(rand.NewSource(1))
		for {
			select {
			case <-stop:
				return
			default:
			}
			s.SetLocalTransform(a, trans(rng.Float64()*10, 0, 0))
			s.SetLocalTransform(b, trans(0, rng.Float64()*10, 0))
		}
	}()

	// Writer 2: reparent a1 between a and b.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			s.SetParent(a1, b)
			s.SetParent(a1, a)
		}
	}()

	// Writer 3: create/delete temp nodes under c.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			tmp, err := s.CreateNode(c, "tmp", trans(1, 0, 0))
			if err == nil {
				s.SetLocalTransform(tmp, trans(2, 0, 0))
				s.DeleteNode(tmp)
			}
		}
	}()

	// Writer 4: reorder siblings under root.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			kids, err := s.Children(root)
			if err == nil && len(kids) > 1 {
				s.SetSiblingIndex(kids[0], len(kids)-1)
			}
		}
	}()

	// Readers: take snapshots and verify parent-chain consistency.
	readers := 8
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				snap, err := s.SnapshotFull()
				if err != nil {
					t.Errorf("SnapshotFull: %v", err)
					return
				}
				if err := verifySnapshot(snap); err != nil {
					t.Errorf("inconsistent snapshot: %v", err)
					return
				}
				// Also read individual nodes.
				for _, id := range []NodeID{root, a, b, c, a1, b1} {
					if _, err := s.WorldTransform(id); err != nil {
						t.Errorf("WorldTransform(%s): %v", id, err)
						return
					}
				}
			}
		}()
	}

	// Run for a fixed wall-clock budget; a deadlock would hang the test.
	time.Sleep(2 * time.Second)
	close(stop)
	wg.Wait()

	// Final state must still be valid.
	if err := s.Validate(); err != nil {
		t.Fatalf("scene invalid after concurrent run: %v", err)
	}
}

// verifySnapshot checks that for every node, world == parent.world * local
// within the self-contained snapshot, and that root world == root local.
func verifySnapshot(snap []NodeSnapshot) error {
	byID := make(map[NodeID]NodeSnapshot, len(snap))
	for _, n := range snap {
		byID[n.ID] = n
	}
	for _, n := range snap {
		if n.Parent == "" {
			if !n.World.Approx(n.Local.Mat4(), 1e-9) {
				return fmt.Errorf("root %s world != local", n.ID)
			}
			continue
		}
		parent, ok := byID[n.Parent]
		if !ok {
			return fmt.Errorf("parent %s of %s missing from snapshot", n.Parent, n.ID)
		}
		want := parent.World.Mul(n.Local.Mat4())
		if !n.World.Approx(want, 1e-9) {
			return fmt.Errorf("world of %s != parent.world * local", n.ID)
		}
	}
	return nil
}

// --- additional edge cases ---

func TestSetRootLocalTransformAffectsChildren(t *testing.T) {
	s := NewScene()
	root := s.Root()
	a, _ := s.CreateNode(root, "a", trans(5, 0, 0))
	// Move the root by (10,0,0); a must move with it.
	if err := s.SetLocalTransform(root, trans(10, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if got := worldOrigin(t, s, a); !got.Approx(mathx.Vec3{X: 15}, eps) {
		t.Fatalf("a origin = %v, want (15,0,0)", got)
	}
}

func TestReparentAffectsGrandchildren(t *testing.T) {
	s := NewScene()
	root := s.Root()
	a, _ := s.CreateNode(root, "a", trans(1, 0, 0))
	b, _ := s.CreateNode(root, "b", trans(100, 0, 0))
	c, _ := s.CreateNode(a, "c", trans(0, 2, 0))
	d, _ := s.CreateNode(c, "d", trans(0, 0, 3))

	// d world = a(1,0,0) * c(0,2,0) * d(0,0,3) = (1,2,3)
	if got := worldOrigin(t, s, d); !got.Approx(mathx.Vec3{X: 1, Y: 2, Z: 3}, eps) {
		t.Fatalf("d origin before = %v", got)
	}
	// Reparent c under b: d world = b(100,0,0) * c(0,2,0) * d(0,0,3) = (100,2,3)
	if err := s.SetParent(c, b); err != nil {
		t.Fatal(err)
	}
	if got := worldOrigin(t, s, d); !got.Approx(mathx.Vec3{X: 100, Y: 2, Z: 3}, eps) {
		t.Fatalf("d origin after reparent = %v", got)
	}
}

func TestNameAndParentAccessors(t *testing.T) {
	s := NewScene()
	root := s.Root()
	a, _ := s.CreateNode(root, "my-node", ident())
	if n, err := s.Name(a); err != nil || n != "my-node" {
		t.Fatalf("Name = %q, %v", n, err)
	}
	if p, err := s.Parent(a); err != nil || p != root {
		t.Fatalf("Parent = %q, %v", p, err)
	}
	if p, err := s.Parent(root); err != nil || p != "" {
		t.Fatalf("root Parent = %q, %v", p, err)
	}
}

func TestValidateRejectsCorruptScene(t *testing.T) {
	s := NewScene()
	root := s.Root()
	a, _ := s.CreateNode(root, "a", ident())
	b, _ := s.CreateNode(a, "b", ident())
	// Manually corrupt: create a cycle by pointing a's parent at b.
	s.mu.Lock()
	s.nodes[a].parent = b
	s.mu.Unlock()
	if err := s.Validate(); !errors.Is(err, ErrCycle) {
		t.Fatalf("expected ErrCycle from Validate, got %v", err)
	}
}

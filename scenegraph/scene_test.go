package scenegraph_test

import (
	"errors"
	"testing"

	sg "github.com/Diudiuseven/scene-graph-go/scenegraph"
)

const eps = 1e-9

func tr(px, py, pz float64) sg.Transform {
	t := sg.IdentityTransform()
	t.Position = sg.Vec3{X: px, Y: py, Z: pz}
	return t
}

func mustCreate(t *testing.T, s *sg.Scene, name string, parent sg.NodeID, local sg.Transform) sg.NodeID {
	t.Helper()
	id, err := s.CreateNode(name, parent, local)
	if err != nil {
		t.Fatalf("CreateNode(%s): %v", name, err)
	}
	return id
}

func mustWorld(t *testing.T, s *sg.Scene, id sg.NodeID) sg.Mat4 {
	t.Helper()
	m, err := s.WorldTransform(id)
	if err != nil {
		t.Fatalf("WorldTransform(%d): %v", id, err)
	}
	return m
}

// 世界变换 = 父链局部变换组合。
func TestWorldTransformComposition(t *testing.T) {
	s := sg.NewScene()
	root := mustCreate(t, s, "root", 0, tr(1, 0, 0))
	mid := mustCreate(t, s, "mid", root, tr(0, 2, 0))
	leaf := mustCreate(t, s, "leaf", mid, tr(0, 0, 3))

	got := mustWorld(t, s, leaf).TransformPoint(sg.Vec3{})
	want := sg.Vec3{X: 1, Y: 2, Z: 3}
	if d := got.X - want.X; d > eps || d < -eps {
		t.Fatalf("got %+v want %+v", got, want)
	}
	if got.Y != 2 || got.Z != 3 {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

// 重新挂载后世界变换立即正确。
func TestReparentUpdatesWorld(t *testing.T) {
	s := sg.NewScene()
	a := mustCreate(t, s, "a", 0, tr(10, 0, 0))
	b := mustCreate(t, s, "b", 0, tr(0, 20, 0))
	child := mustCreate(t, s, "child", a, tr(1, 1, 0))

	if p := mustWorld(t, s, child).TransformPoint(sg.Vec3{}); p.X != 11 || p.Y != 1 {
		t.Fatalf("before reparent: %+v", p)
	}
	if err := s.Reparent(child, b); err != nil {
		t.Fatal(err)
	}
	if p := mustWorld(t, s, child).TransformPoint(sg.Vec3{}); p.X != 1 || p.Y != 21 {
		t.Fatalf("after reparent: %+v", p)
	}
	// 旧父的子列表已更新。
	ch, _ := s.Children(a)
	if len(ch) != 0 {
		t.Fatalf("a still has children: %v", ch)
	}
}

// 写入局部变换只影响对应子树。
func TestSetLocalTransform(t *testing.T) {
	s := sg.NewScene()
	root := mustCreate(t, s, "root", 0, tr(0, 0, 0))
	c1 := mustCreate(t, s, "c1", root, tr(1, 0, 0))
	c2 := mustCreate(t, s, "c2", root, tr(2, 0, 0))

	_ = mustWorld(t, s, c1)
	before := mustWorld(t, s, c2)

	if err := s.SetLocalTransform(c1, tr(5, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if p := mustWorld(t, s, c1).TransformPoint(sg.Vec3{}); p.X != 5 {
		t.Fatalf("c1 world: %+v", p)
	}
	after := mustWorld(t, s, c2)
	if !before.ApproxEqual(after, eps) {
		t.Fatalf("c2 world changed: %v -> %v", before, after)
	}
}

// 删除子树后节点不可访问，引用已删除父节点被拒绝。
func TestDeleteAndDeletedParent(t *testing.T) {
	s := sg.NewScene()
	root := mustCreate(t, s, "root", 0, tr(0, 0, 0))
	child := mustCreate(t, s, "child", root, tr(1, 0, 0))
	grand := mustCreate(t, s, "grand", child, tr(1, 0, 0))

	if err := s.DeleteNode(child); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WorldTransform(grand); !errors.Is(err, sg.ErrNodeNotFound) {
		t.Fatalf("grand should be gone: %v", err)
	}
	// 在已删除节点下创建 → ErrParentDeleted。
	if _, err := s.CreateNode("x", child, tr(0, 0, 0)); !errors.Is(err, sg.ErrParentDeleted) {
		t.Fatalf("want ErrParentDeleted, got %v", err)
	}
	// 挂载到已删除节点 → ErrParentDeleted。
	if err := s.Reparent(root, child); !errors.Is(err, sg.ErrParentDeleted) {
		t.Fatalf("want ErrParentDeleted, got %v", err)
	}
	// 不存在的父节点同样报 ErrParentDeleted（无法挂载）。
	if _, err := s.CreateNode("y", 9999, tr(0, 0, 0)); !errors.Is(err, sg.ErrParentDeleted) {
		t.Fatalf("want ErrParentDeleted, got %v", err)
	}
	// 树未停在中间状态：root 仍正常。
	if p := mustWorld(t, s, root).TransformPoint(sg.Vec3{}); p != (sg.Vec3{}) {
		t.Fatalf("root corrupted: %+v", p)
	}
}

// 环路被拒绝，且树保持原状。
func TestCycleRejected(t *testing.T) {
	s := sg.NewScene()
	a := mustCreate(t, s, "a", 0, tr(1, 0, 0))
	b := mustCreate(t, s, "b", a, tr(0, 1, 0))
	c := mustCreate(t, s, "c", b, tr(0, 0, 1))

	if err := s.Reparent(a, c); !errors.Is(err, sg.ErrCycleDetected) {
		t.Fatalf("want ErrCycleDetected, got %v", err)
	}
	if err := s.Reparent(a, a); !errors.Is(err, sg.ErrCycleDetected) {
		t.Fatalf("self-parent: want ErrCycleDetected, got %v", err)
	}
	// 树状态未被破坏。
	if p := mustWorld(t, s, c).TransformPoint(sg.Vec3{}); p.X != 1 || p.Y != 1 || p.Z != 1 {
		t.Fatalf("tree corrupted after rejected reparent: %+v", p)
	}
	if par, _ := s.Parent(a); par != 0 {
		t.Fatalf("a parent changed: %d", par)
	}
}

// 同级顺序调整。
func TestReorderChild(t *testing.T) {
	s := sg.NewScene()
	root := mustCreate(t, s, "root", 0, tr(0, 0, 0))
	a := mustCreate(t, s, "a", root, tr(0, 0, 0))
	b := mustCreate(t, s, "b", root, tr(0, 0, 0))
	c := mustCreate(t, s, "c", root, tr(0, 0, 0))

	if err := s.ReorderChild(root, c, 0); err != nil {
		t.Fatal(err)
	}
	ch, _ := s.Children(root)
	want := []sg.NodeID{c, a, b}
	for i := range want {
		if ch[i] != want[i] {
			t.Fatalf("order = %v, want %v", ch, want)
		}
	}
	// 非子节点报错。
	if err := s.ReorderChild(root, root, 0); !errors.Is(err, sg.ErrNodeNotFound) {
		t.Fatalf("want ErrNodeNotFound, got %v", err)
	}
}

// 旋转与缩放的组合。
func TestRotationScaleComposition(t *testing.T) {
	s := sg.NewScene()
	rt := sg.IdentityTransform()
	rt.Scale = sg.Vec3{X: 2, Y: 2, Z: 2}
	root := mustCreate(t, s, "root", 0, rt)
	ct := sg.IdentityTransform()
	// 绕 Z 轴旋转 90 度：quat (0,0,sin45,cos45)。
	ct.Rotation = sg.Quat{Z: 0.7071067811865476, W: 0.7071067811865476}
	child := mustCreate(t, s, "child", root, ct)

	p := mustWorld(t, s, child).TransformPoint(sg.Vec3{X: 1, Y: 0, Z: 0})
	// 点 (1,0,0) 旋转 90° -> (0,1,0)，再缩放 2 -> (0,2,0)。
	if p.X > eps || p.X < -eps || p.Y-2 > eps || p.Y-2 < -eps {
		t.Fatalf("got %+v want (0,2,0)", p)
	}
}

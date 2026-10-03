package scenegraph

import "testing"

// 缓存失效范围：写入局部变换只使对应子树变脏，兄弟与父节点缓存保持有效。
func TestDirtyScopeOnLocalWrite(t *testing.T) {
	s := NewScene()
	root, _ := s.CreateNode("root", 0, IdentityTransform())
	c1, _ := s.CreateNode("c1", root, IdentityTransform())
	c2, _ := s.CreateNode("c2", root, IdentityTransform())
	g, _ := s.CreateNode("g", c1, IdentityTransform())

	// 填充全部缓存。
	for _, id := range []NodeID{root, c1, c2, g} {
		if _, err := s.WorldTransform(id); err != nil {
			t.Fatal(err)
		}
	}
	// 写入 c1。
	if err := s.SetLocalTransform(c1, IdentityTransform()); err != nil {
		t.Fatal(err)
	}
	if s.nodes[c1].dirty != true || s.nodes[g].dirty != true {
		t.Fatal("c1 subtree should be dirty")
	}
	if s.nodes[root].dirty || s.nodes[c2].dirty {
		t.Fatal("root and c2 must keep valid cache")
	}
}

// 重新挂载只使被移动子树变脏。
func TestDirtyScopeOnReparent(t *testing.T) {
	s := NewScene()
	root, _ := s.CreateNode("root", 0, IdentityTransform())
	a, _ := s.CreateNode("a", root, IdentityTransform())
	b, _ := s.CreateNode("b", root, IdentityTransform())
	child, _ := s.CreateNode("child", a, IdentityTransform())

	for _, id := range []NodeID{root, a, b, child} {
		if _, err := s.WorldTransform(id); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Reparent(child, b); err != nil {
		t.Fatal(err)
	}
	if !s.nodes[child].dirty {
		t.Fatal("moved child should be dirty")
	}
	if s.nodes[root].dirty || s.nodes[a].dirty || s.nodes[b].dirty {
		t.Fatal("unrelated nodes must keep valid cache")
	}
}

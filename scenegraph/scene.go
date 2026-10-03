package scenegraph

import "sync"

// Scene 是场景图，持有全部节点并维护世界变换缓存。
//
// 一致性保证：
//   - 所有公开方法在内部持有同一把读写锁，任一读取（如 WorldTransform）
//     观察到的都是某个完整提交后的树状态，不会看到半更新结果。
//   - 结构编辑与变换写入只会将受影响子树标记为脏，未受影响节点的
//     世界变换缓存保持不变，不会被无谓重算。
type Scene struct {
	mu     sync.RWMutex
	nodes  map[NodeID]*node
	nextID NodeID
}

// NewScene 创建空场景图。
func NewScene() *Scene {
	return &Scene{nodes: make(map[NodeID]*node), nextID: 1}
}

// CreateNode 新建节点并挂到 parent 下（parent 为 0 表示作为根节点）。
func (s *Scene) CreateNode(name string, parent NodeID, local Transform) (NodeID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if parent != 0 {
		p, ok := s.nodes[parent]
		if !ok || p.deleted {
			return 0, ErrParentDeleted
		}
	}
	id := s.nextID
	s.nextID++
	n := &node{id: id, name: name, parent: parent, local: local, dirty: true}
	s.nodes[id] = n
	if parent != 0 {
		p := s.nodes[parent]
		p.children = append(p.children, id)
	}
	return id, nil
}

// DeleteNode 删除节点及其整棵子树。被删除节点不可再作为父节点使用。
func (s *Scene) DeleteNode(id NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nodes[id]
	if !ok || n.deleted {
		return ErrNodeNotFound
	}
	// 从父节点的子列表中摘除。
	if n.parent != 0 {
		if p, ok := s.nodes[n.parent]; ok {
			p.children = removeChild(p.children, id)
		}
	}
	// 先收集整棵子树，再统一标记为已删除。
	// 删除的节点保留在 nodes 中，以便后续引用时能区分
	// ErrParentDeleted（已删除）与 ErrNodeNotFound（不存在）。
	var collect func(nid NodeID, out *[]NodeID)
	collect = func(nid NodeID, out *[]NodeID) {
		*out = append(*out, nid)
		for _, c := range s.nodes[nid].children {
			collect(c, out)
		}
	}
	var all []NodeID
	collect(id, &all)
	for _, nid := range all {
		cn := s.nodes[nid]
		cn.deleted = true
		cn.parent = 0
		cn.children = nil
	}
	return nil
}

// Reparent 将节点重新挂载到新父节点下（newParent 为 0 表示提升为根）。
// 若操作会产生环路或目标父节点已删除，则拒绝且树保持原状。
func (s *Scene) Reparent(id, newParent NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nodes[id]
	if !ok || n.deleted {
		return ErrNodeNotFound
	}
	if newParent == id {
		return ErrCycleDetected
	}
	if newParent != 0 {
		np, ok := s.nodes[newParent]
		if !ok || np.deleted {
			return ErrParentDeleted
		}
		// 环路检测：newParent 不能是 id 的后代。
		if s.isDescendantLocked(newParent, id) {
			return ErrCycleDetected
		}
	}
	if n.parent == newParent {
		return nil // 无变化
	}
	// 从旧父摘除。
	if n.parent != 0 {
		if op, ok := s.nodes[n.parent]; ok {
			op.children = removeChild(op.children, id)
		}
	}
	n.parent = newParent
	if newParent != 0 {
		np := s.nodes[newParent]
		np.children = append(np.children, id)
	}
	// 父链变化，整个子树的世界变换失效。
	s.markDirtyLocked(id)
	return nil
}

// ReorderChild 将 parent 的子节点 id 移动到 index 位置（同级顺序调整）。
func (s *Scene) ReorderChild(parent, id NodeID, index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.nodes[parent]
	if !ok || p.deleted {
		return ErrNodeNotFound
	}
	pos := -1
	for i, c := range p.children {
		if c == id {
			pos = i
			break
		}
	}
	if pos < 0 {
		return ErrNodeNotFound
	}
	if index < 0 {
		index = 0
	}
	if index >= len(p.children) {
		index = len(p.children) - 1
	}
	p.children = removeChild(p.children, id)
	// 在删除后重新计算插入位置。
	if index > len(p.children) {
		index = len(p.children)
	}
	p.children = append(p.children, 0)
	copy(p.children[index+1:], p.children[index:])
	p.children[index] = id
	return nil
}

// SetLocalTransform 写入节点局部变换，仅使该节点子树的世界缓存失效。
func (s *Scene) SetLocalTransform(id NodeID, t Transform) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nodes[id]
	if !ok || n.deleted {
		return ErrNodeNotFound
	}
	n.local = t
	s.markDirtyLocked(id)
	return nil
}

// LocalTransform 读取节点局部变换。
func (s *Scene) LocalTransform(id NodeID) (Transform, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[id]
	if !ok || n.deleted {
		return Transform{}, ErrNodeNotFound
	}
	return n.local, nil
}

// WorldTransform 读取节点世界变换。结果等于父链上各局部变换的组合，
// 并使用缓存避免重复计算；缓存失效时沿父链重算。
func (s *Scene) WorldTransform(id NodeID) (Mat4, error) {
	// 升级为写锁以填充缓存；读世界变换是幂等的，写锁保证缓存一致。
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.worldLocked(id)
}

func (s *Scene) worldLocked(id NodeID) (Mat4, error) {
	n, ok := s.nodes[id]
	if !ok || n.deleted {
		return Mat4{}, ErrNodeNotFound
	}
	if !n.dirty && n.worldValid {
		return n.world, nil
	}
	var w Mat4
	if n.parent != 0 {
		pw, err := s.worldLocked(n.parent)
		if err != nil {
			return Mat4{}, err
		}
		w = pw.Mul(n.local.ToMatrix())
	} else {
		w = n.local.ToMatrix()
	}
	n.world = w
	n.worldValid = true
	n.dirty = false
	return w, nil
}

// Children 返回 parent 的子节点列表（按同级顺序）。
func (s *Scene) Children(parent NodeID) ([]NodeID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.nodes[parent]
	if !ok || p.deleted {
		return nil, ErrNodeNotFound
	}
	return append([]NodeID(nil), p.children...), nil
}

// Parent 返回节点父节点，0 表示无父。
func (s *Scene) Parent(id NodeID) (NodeID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[id]
	if !ok || n.deleted {
		return 0, ErrNodeNotFound
	}
	return n.parent, nil
}

// --- 内部辅助（调用时必须已持有锁） ---

// isDescendantLocked 判断 cand 是否在 ancestor 的子树中（含自身）。
func (s *Scene) isDescendantLocked(cand, ancestor NodeID) bool {
	for cur := cand; cur != 0; {
		if cur == ancestor {
			return true
		}
		n, ok := s.nodes[cur]
		if !ok {
			return false
		}
		cur = n.parent
	}
	return false
}

// markDirtyLocked 将 id 及其全部后代标记为脏。
// 不变量：节点为脏时其后代必然已为脏（标记总是整棵子树传播，
// 而缓存填充只会清除被查询节点及其祖先），因此遇到已脏节点可提前返回。
func (s *Scene) markDirtyLocked(id NodeID) {
	n := s.nodes[id]
	if n == nil || n.dirty {
		return
	}
	n.dirty = true
	for _, c := range n.children {
		s.markDirtyLocked(c)
	}
}

func removeChild(list []NodeID, id NodeID) []NodeID {
	for i, c := range list {
		if c == id {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list
}

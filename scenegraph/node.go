package scenegraph

// NodeID 是节点的唯一标识。
type NodeID uint64

// node 是场景图内部节点。
type node struct {
	id       NodeID
	name     string
	parent   NodeID // 0 表示无父（根或游离）
	children []NodeID
	local    Transform
	deleted  bool

	dirty      bool // 世界变换缓存是否失效
	world      Mat4
	worldValid bool
}

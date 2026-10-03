package scenegraph

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
)

// CurrentVersion 是当前序列化格式版本。
const CurrentVersion = 2

// 文件头魔数 "SGPH"。
var magic = []byte{'S', 'G', 'P', 'H'}

// 序列化格式：
//   字节 0-3 : 魔数 "SGPH"
//   字节 4-7 : uint32 版本号（小端）
//   其余     : JSON 编码的负载
//
// v1 负载：节点不保存同级顺序（children 无序），升级时按节点 ID 排序重建顺序。
// v2 负载：节点保存有序 children 列表。

type nodeV1 struct {
	ID       uint64 `json:"id"`
	Name     string `json:"name"`
	Parent   uint64 `json:"parent"`
	Position Vec3   `json:"position"`
	Rotation Quat   `json:"rotation"`
	Scale    Vec3   `json:"scale"`
}

type payloadV1 struct {
	Nodes []nodeV1 `json:"nodes"`
}

type nodeV2 struct {
	ID       uint64    `json:"id"`
	Name     string    `json:"name"`
	Parent   uint64    `json:"parent"`
	Children []uint64  `json:"children,omitempty"`
	Local    Transform `json:"local"`
}

type payloadV2 struct {
	NextID uint64   `json:"next_id"`
	Nodes  []nodeV2 `json:"nodes"`
}

// Marshal 将场景序列化为字节流（当前版本格式）。
// 输出按节点 ID 排序，保证同一状态多次序列化结果一致（可复现）。
func (s *Scene) Marshal() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]int, 0, len(s.nodes))
	for id, n := range s.nodes {
		if !n.deleted {
			ids = append(ids, int(id))
		}
	}
	sort.Ints(ids)

	p := payloadV2{NextID: uint64(s.nextID)}
	for _, i := range ids {
		n := s.nodes[NodeID(i)]
		children := make([]uint64, 0, len(n.children))
		for _, c := range n.children {
			children = append(children, uint64(c))
		}
		p.Nodes = append(p.Nodes, nodeV2{
			ID:       uint64(n.id),
			Name:     n.name,
			Parent:   uint64(n.parent),
			Children: children,
			Local:    n.local,
		})
	}
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.Write(magic)
	var ver [4]byte
	binary.LittleEndian.PutUint32(ver[:], CurrentVersion)
	buf.Write(ver[:])
	buf.Write(body)
	return buf.Bytes(), nil
}

// Unmarshal 从字节流恢复场景，兼容 v1 / v2 数据。
// v1 数据会被升级：同级顺序按节点 ID 升序重建，世界变换语义保持不变。
func Unmarshal(data []byte) (*Scene, error) {
	if len(data) < 8 || !bytes.Equal(data[:4], magic) {
		return nil, fmt.Errorf("scenegraph: bad magic: %w", ErrInvalidVersion)
	}
	version := binary.LittleEndian.Uint32(data[4:8])
	body := data[8:]
	switch version {
	case 1:
		return unmarshalV1(body)
	case 2:
		return unmarshalV2(body)
	default:
		return nil, fmt.Errorf("scenegraph: version %d: %w", version, ErrInvalidVersion)
	}
}

func unmarshalV1(body []byte) (*Scene, error) {
	var p payloadV1
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	s := NewScene()
	var maxID NodeID
	for _, nv := range p.Nodes {
		id := NodeID(nv.ID)
		s.nodes[id] = &node{
			id:    id,
			name:  nv.Name,
			local: Transform{Position: nv.Position, Rotation: nv.Rotation, Scale: nv.Scale},
			dirty: true,
		}
		if id > maxID {
			maxID = id
		}
	}
	// 建立父子链接；v1 无同级顺序，按 ID 升序重建。
	children := make(map[NodeID][]NodeID)
	for _, nv := range p.Nodes {
		id, par := NodeID(nv.ID), NodeID(nv.Parent)
		n := s.nodes[id]
		if par != 0 {
			pn, ok := s.nodes[par]
			if !ok {
				return nil, fmt.Errorf("scenegraph: node %d references missing parent %d", id, par)
			}
			n.parent = par
			children[pn.id] = append(children[pn.id], id)
		}
	}
	for pid, list := range children {
		sort.Slice(list, func(a, b int) bool { return list[a] < list[b] })
		s.nodes[pid].children = list
	}
	s.nextID = maxID + 1
	return s, nil
}

func unmarshalV2(body []byte) (*Scene, error) {
	var p payloadV2
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	s := NewScene()
	var maxID NodeID
	for _, nv := range p.Nodes {
		id := NodeID(nv.ID)
		children := make([]NodeID, 0, len(nv.Children))
		for _, c := range nv.Children {
			children = append(children, NodeID(c))
		}
		s.nodes[id] = &node{
			id:       id,
			name:     nv.Name,
			parent:   NodeID(nv.Parent),
			children: children,
			local:    nv.Local,
			dirty:    true,
		}
		if id > maxID {
			maxID = id
		}
	}
	// 校验父引用完整性，拒绝损坏数据。
	for _, n := range s.nodes {
		if n.parent != 0 {
			if _, ok := s.nodes[n.parent]; !ok {
				return nil, fmt.Errorf("scenegraph: node %d references missing parent %d", n.id, n.parent)
			}
		}
	}
	s.nextID = NodeID(p.NextID)
	if s.nextID <= maxID {
		s.nextID = maxID + 1
	}
	return s, nil
}

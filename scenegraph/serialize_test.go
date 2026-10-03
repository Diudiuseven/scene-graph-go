package scenegraph_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"

	sg "github.com/Diudiuseven/scene-graph-go/scenegraph"
)

func buildSample(t *testing.T) *sg.Scene {
	t.Helper()
	s := sg.NewScene()
	root := mustCreate(t, s, "root", 0, tr(1, 2, 3))
	a := mustCreate(t, s, "a", root, tr(4, 5, 6))
	b := mustCreate(t, s, "b", root, tr(7, 8, 9))
	mustCreate(t, s, "leaf", a, tr(0.5, 0.25, 0.125))
	// 调整同级顺序，验证顺序被保留。
	if err := s.ReorderChild(root, b, 0); err != nil {
		t.Fatal(err)
	}
	return s
}

func worldOfAll(t *testing.T, s *sg.Scene, ids []sg.NodeID) []sg.Mat4 {
	t.Helper()
	out := make([]sg.Mat4, len(ids))
	for i, id := range ids {
		out[i] = mustWorld(t, s, id)
	}
	return out
}

// v2 往返：序列化再恢复后，世界变换与同级顺序完全一致。
func TestSerializeRoundTrip(t *testing.T) {
	s := buildSample(t)
	data, err := s.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	s2, err := sg.Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	ids := []sg.NodeID{1, 2, 3, 4}
	w1 := worldOfAll(t, s, ids)
	w2 := worldOfAll(t, s2, ids)
	for i := range w1 {
		if !w1[i].ApproxEqual(w2[i], eps) {
			t.Fatalf("node %d world mismatch:\n%v\n%v", ids[i], w1[i], w2[i])
		}
	}
	ch, err := s2.Children(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ch) != 2 || ch[0] != 3 || ch[1] != 2 {
		t.Fatalf("sibling order not preserved: %v", ch)
	}
	// 恢复后可继续编辑，ID 不冲突。
	if _, err := s2.CreateNode("new", 1, tr(0, 0, 0)); err != nil {
		t.Fatal(err)
	}
}

// v1 数据可读取并升级，世界变换等价。
func TestUnmarshalV1Upgrade(t *testing.T) {
	type nodeV1 struct {
		ID       uint64  `json:"id"`
		Name     string  `json:"name"`
		Parent   uint64  `json:"parent"`
		Position sg.Vec3 `json:"position"`
		Rotation sg.Quat `json:"rotation"`
		Scale    sg.Vec3 `json:"scale"`
	}
	payload := map[string]any{
		"nodes": []nodeV1{
			{ID: 1, Name: "root", Position: sg.Vec3{X: 1}, Scale: sg.Vec3{1, 1, 1}, Rotation: sg.Quat{W: 1}},
			{ID: 2, Name: "child", Parent: 1, Position: sg.Vec3{Y: 2}, Scale: sg.Vec3{1, 1, 1}, Rotation: sg.Quat{W: 1}},
		},
	}
	body, _ := json.Marshal(payload)
	var buf bytes.Buffer
	buf.Write([]byte("SGPH"))
	var ver [4]byte
	binary.LittleEndian.PutUint32(ver[:], 1)
	buf.Write(ver[:])
	buf.Write(body)

	s, err := sg.Unmarshal(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	p := mustWorld(t, s, 2).TransformPoint(sg.Vec3{})
	if p.X != 1 || p.Y != 2 {
		t.Fatalf("v1 upgrade world wrong: %+v", p)
	}
	// 升级后可继续编辑。
	if _, err := s.CreateNode("n", 2, tr(0, 0, 0)); err != nil {
		t.Fatal(err)
	}
}

// 未知版本被拒绝。
func TestUnmarshalBadVersion(t *testing.T) {
	var buf bytes.Buffer
	buf.Write([]byte("SGPH"))
	var ver [4]byte
	binary.LittleEndian.PutUint32(ver[:], 99)
	buf.Write(ver[:])
	if _, err := sg.Unmarshal(buf.Bytes()); !errors.Is(err, sg.ErrInvalidVersion) {
		t.Fatalf("want ErrInvalidVersion, got %v", err)
	}
	if _, err := sg.Unmarshal([]byte("garbage")); !errors.Is(err, sg.ErrInvalidVersion) {
		t.Fatalf("bad magic: want ErrInvalidVersion, got %v", err)
	}
}

// 可复现性：固定输入与固定操作序列，多次运行结果逐节点一致。
func TestDeterministicReproduction(t *testing.T) {
	run := func() ([]sg.Mat4, []byte) {
		s := sg.NewScene()
		root := mustCreate(t, s, "root", 0, tr(1, 0, 0))
		a := mustCreate(t, s, "a", root, tr(0, 2, 0))
		b := mustCreate(t, s, "b", root, tr(0, 0, 3))
		c := mustCreate(t, s, "c", a, tr(1, 1, 1))
		// 固定操作序列。
		if err := s.Reparent(c, b); err != nil {
			t.Fatal(err)
		}
		if err := s.SetLocalTransform(root, tr(5, 0, 0)); err != nil {
			t.Fatal(err)
		}
		if err := s.ReorderChild(root, b, 0); err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteNode(a); err != nil {
			t.Fatal(err)
		}
		ids := []sg.NodeID{root, b, c}
		w := worldOfAll(t, s, ids)
		data, err := s.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		return w, data
	}
	w1, d1 := run()
	w2, d2 := run()
	for i := range w1 {
		if !w1[i].ApproxEqual(w2[i], 0) {
			t.Fatalf("run mismatch at %d:\n%v\n%v", i, w1[i], w2[i])
		}
	}
	if !bytes.Equal(d1, d2) {
		t.Fatal("serialized bytes differ between identical runs")
	}
}

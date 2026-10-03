package scene

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"

	"github.com/Diudiuseven/scene-graph-go/mathx"
)

func buildSampleScene(t *testing.T) *Scene {
	t.Helper()
	s := NewScene()
	root := s.Root()
	a, err := s.CreateNode(root, "a", mathx.Transform{
		Position: mathx.Vec3{X: 1, Y: 0, Z: 0},
		Rotation: mathx.QuatFromAxisAngle(mathx.Vec3{X: 0, Y: 0, Z: 1}, math.Pi/2),
		Scale:    mathx.Vec3{X: 2, Y: 2, Z: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateNode(a, "b", mathx.Transform{Position: mathx.Vec3{X: 1, Y: 0, Z: 0}, Rotation: mathx.IdentityQuat(), Scale: mathx.Vec3{X: 1, Y: 1, Z: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateNode(a, "c", mathx.DefaultTransform()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateNode(b, "d", mathx.Transform{Position: mathx.Vec3{X: 0, Y: 3, Z: 0}, Rotation: mathx.IdentityQuat(), Scale: mathx.Vec3{X: 1, Y: 1, Z: 1}}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSaveV2RoundTrip(t *testing.T) {
	s := buildSampleScene(t)
	var buf bytes.Buffer
	if err := s.Save(&buf); err != nil {
		t.Fatal(err)
	}
	t.Logf("v2 data:\n%s", buf.String())

	loaded, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatal(err)
	}

	want, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	got, err := loaded.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != len(got) {
		t.Fatalf("node count mismatch: %d vs %d", len(want), len(got))
	}
	for id, wm := range want {
		gm, ok := got[id]
		if !ok {
			t.Fatalf("missing node %s in loaded scene", id)
		}
		if !wm.Approx(gm, 1e-9) {
			t.Fatalf("world mismatch for %s: %v vs %v", id, wm, gm)
		}
	}
}

func TestSaveV1Migration(t *testing.T) {
	s := buildSampleScene(t)
	var v1buf bytes.Buffer
	if err := s.SaveV1(&v1buf); err != nil {
		t.Fatal(err)
	}
	t.Logf("v1 data:\n%s", v1buf.String())

	loaded, err := Load(&v1buf)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatal(err)
	}

	want, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	got, err := loaded.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != len(got) {
		t.Fatalf("node count mismatch: %d vs %d", len(want), len(got))
	}
	// v1 ids are numeric strings; map them back by world-transform equality.
	for id, wm := range want {
		// find the loaded node with matching world transform
		found := false
		for lid, gm := range got {
			if wm.Approx(gm, 1e-9) {
				found = true
				_ = lid
				break
			}
		}
		if !found {
			t.Fatalf("no loaded node matches world of %s: %v", id, wm)
		}
	}
}

func TestLoadRejectsUnsupportedVersion(t *testing.T) {
	data := bytes.NewBufferString(`{"version": 99, "nodes": []}`)
	_, err := Load(data)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
	if !errorIs(err, ErrUnsupportedVersion) {
		t.Fatalf("expected ErrUnsupportedVersion, got %v", err)
	}
}

func TestLoadRejectsCorruptData(t *testing.T) {
	cases := []string{
		`not json`,
		`{"version": 2, "nodes": []}`,
		`{"version": 2, "nodes": [{"id":"root","parentId":"","children":["missing"],"local":{"position":[0,0,0],"rotation":[0,0,0,1],"scale":[1,1,1]}}]}`,
		`{"version": 2, "nodes": [{"id":"root","parentId":"","children":[],"local":{"position":[0,0,0],"rotation":[0,0,0,1],"scale":[1,1,1]}},{"id":"a","parentId":"root","children":[],"local":{"position":[0,0,0],"rotation":[0,0,0,1],"scale":[1,1,1]}},{"id":"b","parentId":"a","children":[],"local":{"position":[0,0,0],"rotation":[0,0,0,1],"scale":[1,1,1]}},{"id":"c","parentId":"b","children":[],"local":{"position":[0,0,0],"rotation":[0,0,0,1],"scale":[1,1,1]}},{"id":"a","parentId":"c","children":[],"local":{"position":[0,0,0],"rotation":[0,0,0,1],"scale":[1,1,1]}}]}`,
	}
	for i, c := range cases {
		_, err := Load(bytes.NewBufferString(c))
		if err == nil {
			t.Fatalf("case %d: expected error for corrupt data", i)
		}
	}
}

func errorIs(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func TestSaveAfterStructuralEdits(t *testing.T) {
	s := buildSampleScene(t)
	root := s.Root()
	// Perform a mix of structural edits.
	a := NodeID("n1")
	b := NodeID("n2")
	c := NodeID("n3")
	if err := s.SetParent(c, root); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteNode(b); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSiblingIndex(a, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLocalTransform(a, mathx.Transform{Position: mathx.Vec3{X: 42}, Rotation: mathx.IdentityQuat(), Scale: mathx.Vec3{X: 1, Y: 1, Z: 1}}); err != nil {
		t.Fatal(err)
	}

	want, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	// v2 round-trip
	var v2buf bytes.Buffer
	if err := s.Save(&v2buf); err != nil {
		t.Fatal(err)
	}
	loaded2, err := Load(&v2buf)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded2.Validate(); err != nil {
		t.Fatal(err)
	}
	got2, err := loaded2.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != len(got2) {
		t.Fatalf("v2 node count: %d vs %d", len(want), len(got2))
	}
	for id, wm := range want {
		gm, ok := got2[id]
		if !ok {
			t.Fatalf("v2: missing node %s", id)
		}
		if !wm.Approx(gm, 1e-9) {
			t.Fatalf("v2: world mismatch for %s", id)
		}
	}

	// v1 round-trip (cross-version compatibility)
	var v1buf bytes.Buffer
	if err := s.SaveV1(&v1buf); err != nil {
		t.Fatal(err)
	}
	loaded1, err := Load(&v1buf)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded1.Validate(); err != nil {
		t.Fatal(err)
	}
	got1, err := loaded1.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != len(got1) {
		t.Fatalf("v1 node count: %d vs %d", len(want), len(got1))
	}
	// v1 ids are numeric strings; match by world-transform equality.
	for id, wm := range want {
		matched := false
		for _, gm := range got1 {
			if wm.Approx(gm, 1e-9) {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("v1: no node matches world of %s: %v", id, wm)
		}
	}
}

func TestSaveV1UsesIntegerIds(t *testing.T) {
	s := NewScene()
	root := s.Root()
	a, _ := s.CreateNode(root, "a", ident())
	_, _ = s.CreateNode(a, "b", ident())
	var buf bytes.Buffer
	if err := s.SaveV1(&buf); err != nil {
		t.Fatal(err)
	}
	// Decode into a generic map to inspect raw ids.
	var raw struct {
		Version int `json:"version"`
		Nodes   []struct {
			ID       int64 `json:"id"`
			ParentID int64 `json:"parentId"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw.Version != 1 {
		t.Fatalf("version = %d, want 1", raw.Version)
	}
	wantIDs := map[int64]bool{0: true, 1: true, 2: true}
	for _, n := range raw.Nodes {
		if !wantIDs[n.ID] {
			t.Fatalf("unexpected v1 id %d", n.ID)
		}
	}
}

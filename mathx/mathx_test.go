package mathx

import (
	"math"
	"testing"
)

const eps = 1e-9

func TestVec3Basics(t *testing.T) {
	a := Vec3{1, 2, 3}
	b := Vec3{4, 5, 6}
	if got := a.Add(b); !got.Approx(Vec3{5, 7, 9}, eps) {
		t.Fatalf("Add = %v", got)
	}
	if got := a.Sub(b); !got.Approx(Vec3{-3, -3, -3}, eps) {
		t.Fatalf("Sub = %v", got)
	}
	if got := a.Scale(2); !got.Approx(Vec3{2, 4, 6}, eps) {
		t.Fatalf("Scale = %v", got)
	}
	if got := a.Dot(b); math.Abs(got-32) > eps {
		t.Fatalf("Dot = %v", got)
	}
	if got := a.Cross(b); !got.Approx(Vec3{-3, 6, -3}, eps) {
		t.Fatalf("Cross = %v", got)
	}
	if math.Abs(a.Length()-math.Sqrt(14)) > eps {
		t.Fatalf("Length = %v", a.Length())
	}
}

func TestQuatAxisAngle(t *testing.T) {
	// Rotate (1,0,0) by 90 degrees around Z -> (0,1,0)
	q := QuatFromAxisAngle(Vec3{0, 0, 1}, math.Pi/2)
	got := q.Rotate(Vec3{1, 0, 0})
	if !got.Approx(Vec3{0, 1, 0}, eps) {
		t.Fatalf("rotate x by 90 around z = %v", got)
	}
	// Rotate (0,1,0) by 90 degrees around X -> (0,0,1)
	qx := QuatFromAxisAngle(Vec3{1, 0, 0}, math.Pi/2)
	got = qx.Rotate(Vec3{0, 1, 0})
	if !got.Approx(Vec3{0, 0, 1}, eps) {
		t.Fatalf("rotate y by 90 around x = %v", got)
	}
	// Identity rotation leaves vectors unchanged.
	if got := IdentityQuat().Rotate(Vec3{1, 2, 3}); !got.Approx(Vec3{1, 2, 3}, eps) {
		t.Fatalf("identity rotate = %v", got)
	}
}

func TestQuatComposition(t *testing.T) {
	// Two 90-degree rotations around Z equal one 180-degree rotation.
	q := QuatFromAxisAngle(Vec3{0, 0, 1}, math.Pi/2)
	got := q.Mul(q).Rotate(Vec3{1, 0, 0})
	if !got.Approx(Vec3{-1, 0, 0}, eps) {
		t.Fatalf("composed rotate = %v", got)
	}
}

func TestMat4Multiplication(t *testing.T) {
	// T(1,2,3) * T(4,5,6) == T(5,7,9)
	a := TranslationMat4(Vec3{1, 2, 3})
	b := TranslationMat4(Vec3{4, 5, 6})
	got := a.Mul(b).TransformPoint(Vec3{0, 0, 0})
	if !got.Approx(Vec3{5, 7, 9}, eps) {
		t.Fatalf("T*T origin = %v", got)
	}
	// Scale then translate: T(1,0,0)*S(2,2,2) maps (1,0,0) to (3,0,0)
	m := TranslationMat4(Vec3{1, 0, 0}).Mul(ScaleMat4(Vec3{2, 2, 2}))
	got = m.TransformPoint(Vec3{1, 0, 0})
	if !got.Approx(Vec3{3, 0, 0}, eps) {
		t.Fatalf("T*S point = %v", got)
	}
}

func TestTransformMat4(t *testing.T) {
	// T*R*S: scale (2,2,2), rotate 90 around Z, translate (10,0,0)
	tr := Transform{
		Position: Vec3{10, 0, 0},
		Rotation: QuatFromAxisAngle(Vec3{0, 0, 1}, math.Pi/2),
		Scale:    Vec3{2, 2, 2},
	}
	// Point (1,0,0): scale -> (2,0,0); rotate -> (0,2,0); translate -> (10,2,0)
	got := tr.Mat4().TransformPoint(Vec3{1, 0, 0})
	if !got.Approx(Vec3{10, 2, 0}, eps) {
		t.Fatalf("T*R*S point = %v", got)
	}
}

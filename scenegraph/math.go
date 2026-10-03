package scenegraph

import "math"

// Vec3 是三维向量。
type Vec3 struct {
	X, Y, Z float64
}

// Quat 是单位四元数，表示旋转。
type Quat struct {
	X, Y, Z, W float64
}

// Mat4 是 4x4 列主序齐次变换矩阵：m[col*4+row]。
type Mat4 [16]float64

// Transform 是 TRS（平移/旋转/缩放）形式的局部变换。
type Transform struct {
	Position Vec3
	Rotation Quat
	Scale    Vec3
}

// IdentityTransform 返回单位局部变换。
func IdentityTransform() Transform {
	return Transform{Rotation: Quat{W: 1}, Scale: Vec3{1, 1, 1}}
}

// IdentityMat4 返回单位矩阵。
func IdentityMat4() Mat4 {
	return Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
}

// ToMatrix 将 TRS 变换转换为矩阵 M = T * R * S。
func (t Transform) ToMatrix() Mat4 {
	q := t.Rotation.normalized()
	x, y, z, w := q.X, q.Y, q.Z, q.W
	xx, yy, zz := x*x, y*y, z*z
	xy, xz, yz := x*y, x*z, y*z
	wx, wy, wz := w*x, w*y, w*z
	s := t.Scale
	return Mat4{
		(1 - 2*(yy+zz)) * s.X, (2 * (xy + wz)) * s.X, (2 * (xz - wy)) * s.X, 0,
		(2 * (xy - wz)) * s.Y, (1 - 2*(xx+zz)) * s.Y, (2 * (yz + wx)) * s.Y, 0,
		(2 * (xz + wy)) * s.Z, (2 * (yz - wx)) * s.Z, (1 - 2*(xx+yy)) * s.Z, 0,
		t.Position.X, t.Position.Y, t.Position.Z, 1,
	}
}

func (q Quat) normalized() Quat {
	n := q.X*q.X + q.Y*q.Y + q.Z*q.Z + q.W*q.W
	if n == 0 {
		return Quat{W: 1}
	}
	inv := 1.0 / math.Sqrt(n)
	return Quat{q.X * inv, q.Y * inv, q.Z * inv, q.W * inv}
}

// Mul 返回 a*b 的矩阵乘积（先应用 b，再应用 a），列主序。
func (a Mat4) Mul(b Mat4) Mat4 {
	var r Mat4
	for c := 0; c < 4; c++ {
		for row := 0; row < 4; row++ {
			var sum float64
			for k := 0; k < 4; k++ {
				sum += a[k*4+row] * b[c*4+k]
			}
			r[c*4+row] = sum
		}
	}
	return r
}

// TransformPoint 用矩阵变换一个点（含平移，除以 w）。
func (m Mat4) TransformPoint(p Vec3) Vec3 {
	x := m[0]*p.X + m[4]*p.Y + m[8]*p.Z + m[12]
	y := m[1]*p.X + m[5]*p.Y + m[9]*p.Z + m[13]
	z := m[2]*p.X + m[6]*p.Y + m[10]*p.Z + m[14]
	w := m[3]*p.X + m[7]*p.Y + m[11]*p.Z + m[15]
	if w != 0 && w != 1 {
		x, y, z = x/w, y/w, z/w
	}
	return Vec3{x, y, z}
}

// ApproxEqual 判断两个矩阵在容差内是否逐元素相等。
func (m Mat4) ApproxEqual(o Mat4, eps float64) bool {
	for i := 0; i < 16; i++ {
		d := m[i] - o[i]
		if d < 0 {
			d = -d
		}
		if d > eps {
			return false
		}
	}
	return true
}

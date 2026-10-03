package mathx

import "math"

// Mat4 is a column-major 4x4 matrix: the element at row r, column c is
// m[c*4+r]. Translation lives in elements 12,13,14.
type Mat4 [16]float64

// IdentityMat4 returns the identity matrix.
func IdentityMat4() Mat4 {
	return Mat4{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
}

// TranslationMat4 returns a matrix that translates by v.
func TranslationMat4(v Vec3) Mat4 {
	m := IdentityMat4()
	m[12] = v.X
	m[13] = v.Y
	m[14] = v.Z
	return m
}

// ScaleMat4 returns a matrix that scales along the axes by v.
func ScaleMat4(v Vec3) Mat4 {
	return Mat4{
		v.X, 0, 0, 0,
		0, v.Y, 0, 0,
		0, 0, v.Z, 0,
		0, 0, 0, 1,
	}
}

// Mul returns the matrix product m*n.
func (m Mat4) Mul(n Mat4) Mat4 {
	var r Mat4
	for col := 0; col < 4; col++ {
		for row := 0; row < 4; row++ {
			s := 0.0
			for k := 0; k < 4; k++ {
				s += m[k*4+row] * n[col*4+k]
			}
			r[col*4+row] = s
		}
	}
	return r
}

// TransformPoint applies m to the point (v, 1) and returns the resulting
// point; perspective divide is not performed (affine transforms only).
func (m Mat4) TransformPoint(v Vec3) Vec3 {
	return Vec3{
		X: m[0]*v.X + m[4]*v.Y + m[8]*v.Z + m[12],
		Y: m[1]*v.X + m[5]*v.Y + m[9]*v.Z + m[13],
		Z: m[2]*v.X + m[6]*v.Y + m[10]*v.Z + m[14],
	}
}

// Approx reports whether m and n are element-wise equal within eps.
func (m Mat4) Approx(n Mat4, eps float64) bool {
	for i := range m {
		if math.Abs(m[i]-n[i]) > eps {
			return false
		}
	}
	return true
}

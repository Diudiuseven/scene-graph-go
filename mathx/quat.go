package mathx

import "math"

// Quat is a unit quaternion stored as (X, Y, Z, W), W being the scalar part.
// The identity quaternion is {X:0, Y:0, Z:0, W:1}.
type Quat struct {
	X, Y, Z, W float64
}

// IdentityQuat returns the quaternion representing no rotation.
func IdentityQuat() Quat {
	return Quat{W: 1}
}

// QuatFromAxisAngle builds a rotation of angle radians around axis. The axis
// need not be normalized.
func QuatFromAxisAngle(axis Vec3, angle float64) Quat {
	a := axis.Normalize()
	s := math.Sin(angle / 2)
	c := math.Cos(angle / 2)
	return Quat{a.X * s, a.Y * s, a.Z * s, c}
}

// Mul returns the Hamilton product q*o, which (for column-vector conventions)
// corresponds to applying o first and q second.
func (q Quat) Mul(o Quat) Quat {
	return Quat{
		X: q.W*o.X + q.X*o.W + q.Y*o.Z - q.Z*o.Y,
		Y: q.W*o.Y - q.X*o.Z + q.Y*o.W + q.Z*o.X,
		Z: q.W*o.Z + q.X*o.Y - q.Y*o.X + q.Z*o.W,
		W: q.W*o.W - q.X*o.X - q.Y*o.Y - q.Z*o.Z,
	}
}

// Normalize returns q scaled to unit length; the zero quaternion is returned
// unchanged.
func (q Quat) Normalize() Quat {
	l := math.Sqrt(q.X*q.X + q.Y*q.Y + q.Z*q.Z + q.W*q.W)
	if l == 0 {
		return q
	}
	return Quat{q.X / l, q.Y / l, q.Z / l, q.W / l}
}

// Rotate applies the rotation represented by q to vector v.
func (q Quat) Rotate(v Vec3) Vec3 {
	// t = 2 * (xyz x v)
	xyz := Vec3{q.X, q.Y, q.Z}
	t := xyz.Cross(v).Scale(2)
	// v' = v + w*t + xyz x t
	return v.Add(t.Scale(q.W)).Add(xyz.Cross(t))
}

// ToMat4 converts q to a column-major rotation matrix.
func (q Quat) ToMat4() Mat4 {
	x, y, z, w := q.X, q.Y, q.Z, q.W
	xx, yy, zz := x*x, y*y, z*z
	xy, xz, yz := x*y, x*z, y*z
	wx, wy, wz := w*x, w*y, w*z
	return Mat4{
		// column 0
		1 - 2*(yy+zz), 2 * (xy + wz), 2 * (xz - wy), 0,
		// column 1
		2 * (xy - wz), 1 - 2*(xx+zz), 2 * (yz + wx), 0,
		// column 2
		2 * (xz + wy), 2 * (yz - wx), 1 - 2*(xx+yy), 0,
		// column 3
		0, 0, 0, 1,
	}
}

// Approx reports whether q and o are component-wise equal within eps. Note
// that q and -q represent the same rotation.
func (q Quat) Approx(o Quat, eps float64) bool {
	return math.Abs(q.X-o.X) <= eps &&
		math.Abs(q.Y-o.Y) <= eps &&
		math.Abs(q.Z-o.Z) <= eps &&
		math.Abs(q.W-o.W) <= eps
}
